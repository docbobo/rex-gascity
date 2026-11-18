package provider

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"
)

// defaultNudgeSubmitBudget bounds the write and the submit together when the
// op context carries no deadline. gc's own op timeout is 30 s; this leaves
// room for the confirmation window without risking a split submit.
const defaultNudgeSubmitBudget = 10 * time.Second

// NudgeSubmitFailedMetaKey records that a nudge typed its text but failed to
// deliver the submit key. gc reads only the exit code, so an operator finds
// this marker in the provider metadata when a prompt was left unsubmitted.
const NudgeSubmitFailedMetaKey = "GC_NUDGE_SUBMIT_FAILED"

// WithNudgeSubmitBudget sets how long the write and the submit may take
// together when the op context has no earlier deadline. A value <= 0 keeps the
// default.
func WithNudgeSubmitBudget(d time.Duration) Option {
	return func(p *Provider) {
		if d > 0 {
			p.nudgeSubmitBudget = d
		}
	}
}

// ErrNudgeBusy reports that the pane program is running a turn, so the text
// was not typed. The caller should retry later; nothing was injected, so a
// retry cannot duplicate the message.
var ErrNudgeBusy = errors.New("provider: pane program is working")

// ErrNudgeBlocked reports that the pane program is waiting on an interaction
// (for example a permission prompt), so the text was not typed over it. The
// error names the blocked kind when the program reports one.
var ErrNudgeBlocked = errors.New("provider: pane program is blocked")

// NudgeMode controls what Nudge does when the pane program is busy.
type NudgeMode int

const (
	// NudgeModeWait refuses to type into a working or blocked pane.
	NudgeModeWait NudgeMode = iota
	// NudgeModeImmediate types regardless of the program's current state.
	NudgeModeImmediate
)

// NudgeSource names what confirmed a delivered nudge.
type NudgeSource string

const (
	// NudgeSourceProgramStatus is a program_status turn-start observation.
	NudgeSourceProgramStatus NudgeSource = "program_status"
	// NudgeSourceFormatPoll is a change in the pane's rendered output.
	NudgeSourceFormatPoll NudgeSource = "format_poll"
)

// NudgeReport is what the provider observed while delivering a nudge.
// Delivered means the text and submit reached the pane. Confirmed means a
// turn start was then observed; an unconfirmed delivery is still a delivery
// and is not an error, so gc does not retype it.
type NudgeReport struct {
	Delivered bool
	Confirmed bool
	State     string
	Kind      string
	Source    NudgeSource
}

// programStatusSnapshot is the newest program_status record of a pane.
type programStatusSnapshot struct {
	Present bool
	State   string
	Kind    string
	Latest  time.Time
}

// Nudge types text into the agent's pane and submits it. It reads the pane
// program's program_status first: a working or blocked pane is refused with a
// typed error and nothing is typed, so gc can retry without duplicating input.
func (p *Provider) Nudge(ctx context.Context, name, text string) error {
	_, err := p.NudgeReport(ctx, name, text, NudgeModeWait)
	return err
}

// NudgeImmediate is Nudge with the busy/blocked gate bypassed. It is the
// caller's explicit override for a pane that must receive the text now.
func (p *Provider) NudgeImmediate(ctx context.Context, name, text string) error {
	_, err := p.NudgeReport(ctx, name, text, NudgeModeImmediate)
	return err
}

// NudgeReport delivers text like Nudge and reports whether a turn start was
// observed within the provider's bounded confirmation window. It falls back
// to polling the pane's rendered output when the program reports no
// program_status. A delivered but unconfirmed nudge returns a report with
// Confirmed false and a nil error; returning an error would make gc retype
// the message on its next attempt.
func (p *Provider) NudgeReport(ctx context.Context, name, text string, mode NudgeMode) (NudgeReport, error) {
	if text == "" {
		return NudgeReport{}, nil
	}
	if err := ctx.Err(); err != nil {
		return NudgeReport{}, fmt.Errorf("nudge %s: before typing: %w", name, err)
	}
	binding, err := p.livePane(ctx, name)
	if err != nil {
		return NudgeReport{}, err
	}

	baseline, _ := p.programStatusSnapshot(ctx, binding)
	if mode == NudgeModeWait {
		switch baseline.State {
		case "working":
			return NudgeReport{State: baseline.State, Kind: baseline.Kind}, fmt.Errorf("%w: %s", ErrNudgeBusy, name)
		case "blocked":
			report := NudgeReport{State: baseline.State, Kind: baseline.Kind}
			if baseline.Kind != "" {
				return report, fmt.Errorf("%w: %s (%s)", ErrNudgeBlocked, name, baseline.Kind)
			}
			return report, fmt.Errorf("%w: %s", ErrNudgeBlocked, name)
		}
	}

	// Without a program_status record, the pane's rendered output is the only
	// available confirmation signal. Capture its baseline before typing.
	var baseFingerprint string
	haveFormatBaseline := false
	if !baseline.Present {
		if out, err := p.client.BlockFormat(ctx, binding.SessionID, binding.BlockID); err == nil {
			baseFingerprint = formatFingerprint(out)
			haveFormatBaseline = true
		}
	}

	// One deadline covers the write and the Enter. Without it a stalled Rex
	// call or a gc cancel between the two could leave the text typed but
	// unsubmitted, and gc's retry would then duplicate the prompt.
	submitCtx, cancelSubmit := p.nudgeSubmitContext(ctx)
	defer cancelSubmit()
	if err := ctx.Err(); err != nil {
		return NudgeReport{State: baseline.State, Kind: baseline.Kind}, fmt.Errorf("nudge %s: before typing: %w", name, err)
	}
	if err := p.client.BlockWrite(submitCtx, binding.SessionID, binding.BlockID, text); err != nil {
		return NudgeReport{State: baseline.State, Kind: baseline.Kind}, fmt.Errorf("nudge %s: type text: %w", name, err)
	}
	// The text is in the pane now: stamp the input floor so the delivery is
	// visible to idle detection before the program's output renders.
	p.noteInput(binding)
	// The text is in the pane now, so the Enter must be attempted even if the
	// parent was cancelled. It keeps the same submit deadline, so a cancel or
	// stall cannot make this run forever.
	submitDeadline, _ := submitCtx.Deadline()
	enterCtx, cancelEnter := context.WithDeadline(context.WithoutCancel(ctx), submitDeadline)
	defer cancelEnter()
	if err := p.client.SendKeys(enterCtx, binding.SessionID, binding.BlockID, submitKey); err != nil {
		p.recordNudgeSubmitFailure(binding, err)
		return NudgeReport{State: baseline.State, Kind: baseline.Kind},
			fmt.Errorf("nudge %s: text was typed but submit key failed: %w", name, err)
	}
	_ = p.meta.Delete(binding.Name, NudgeSubmitFailedMetaKey)

	report := NudgeReport{Delivered: true}
	deadline := time.Now().Add(p.nudgeConfirmWindow)
	first := true
	for {
		confirmed := false
		if snapshot, err := p.programStatusSnapshot(ctx, binding); err == nil && snapshot.Present {
			report.State, report.Kind = snapshot.State, snapshot.Kind
			if turnStarted(baseline, snapshot) {
				confirmed = true
				report.Source = NudgeSourceProgramStatus
			}
		}
		if !confirmed && haveFormatBaseline {
			if out, err := p.client.BlockFormat(ctx, binding.SessionID, binding.BlockID); err == nil &&
				formatFingerprint(out) != baseFingerprint {
				confirmed = true
				report.Source = NudgeSourceFormatPoll
			}
		}
		if confirmed {
			report.Confirmed = true
			return report, nil
		}
		if !first && !time.Now().Before(deadline) {
			break
		}
		first = false
		remaining := time.Until(deadline)
		if remaining <= 0 {
			break
		}
		wait := p.nudgePollInterval
		if wait > remaining {
			wait = remaining
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			// The text and submit already reached the pane. Report the
			// delivery instead of an error that would duplicate it.
			p.writeNudgeDiagnostic(binding, text, report)
			return report, nil
		case <-timer.C:
		}
	}

	p.writeNudgeDiagnostic(binding, text, report)
	return report, nil
}

// nudgeSubmitContext bounds the write and the submit together. It uses the op
// context's deadline when it has one, else a fixed default, so neither call
// can wait forever.
func (p *Provider) nudgeSubmitContext(parent context.Context) (context.Context, context.CancelFunc) {
	budget := p.nudgeSubmitBudget
	if budget <= 0 {
		budget = defaultNudgeSubmitBudget
	}
	if dl, ok := parent.Deadline(); ok {
		if remaining := time.Until(dl); remaining < budget {
			budget = remaining
		}
	}
	return context.WithTimeout(parent, budget)
}

// recordNudgeSubmitFailure marks the pane's metadata after the text reached it
// but the Enter did not, so the split is visible after the error is gone.
func (p *Provider) recordNudgeSubmitFailure(b Binding, err error) {
	_ = p.meta.Set(b.Name, NudgeSubmitFailedMetaKey, err.Error())
}

// programStatusSnapshot returns the newest program_status record of a pane.
// It reports Present false when Rex has no records for the pane.
func (p *Provider) programStatusSnapshot(ctx context.Context, b Binding) (programStatusSnapshot, error) {
	status, err := p.client.BlockProgramStatus(ctx, b.SessionID, b.BlockID)
	if err != nil {
		return programStatusSnapshot{}, err
	}
	var snap programStatusSnapshot
	for _, rec := range status.Records {
		// ">=" keeps the last record on a tie, including all-zero timestamps.
		if !snap.Present || !rec.UpdatedAt.Before(snap.Latest) {
			snap = programStatusSnapshot{Present: true, State: rec.State, Kind: rec.Kind, Latest: rec.UpdatedAt}
		}
	}
	return snap, nil
}

// turnStarted reports whether after a nudge the program moved from not-working
// to working, or reported a newer updated_at. A baseline that was already
// working (an immediate override) can only be confirmed by a newer timestamp.
func turnStarted(before, after programStatusSnapshot) bool {
	if before.Present && after.Present && !after.Latest.IsZero() && after.Latest.After(before.Latest) {
		return true
	}
	return after.State == "working" && before.State != "working"
}

// nudgeDiagnostic is written when a nudge was delivered but its turn start was
// not observed within the bounded window. It mirrors gc's own unconfirmed
// nudge artifact so an operator can find it, without pretending the delivery
// failed.
type nudgeDiagnostic struct {
	Session string    `json:"session"`
	Aliases []string  `json:"aliases,omitempty"`
	Text    string    `json:"text,omitempty"`
	State   string    `json:"state,omitempty"`
	Kind    string    `json:"kind,omitempty"`
	At      time.Time `json:"at"`
}

// writeNudgeDiagnostic leaves <stateDir>/diagnostics/<name>-nudge-unconfirmed.json
// after an unconfirmed delivery. It is best effort and never changes the
// nudge result.
func (p *Provider) writeNudgeDiagnostic(b Binding, text string, report NudgeReport) {
	if p.stateDir == "" {
		return
	}
	d := nudgeDiagnostic{
		Session: string(b.Name),
		Text:    text,
		State:   report.State,
		Kind:    report.Kind,
		At:      time.Now().UTC(),
	}
	for _, a := range b.Aliases {
		d.Aliases = append(d.Aliases, string(a))
	}
	name := unsafeNameChars.ReplaceAllString(string(b.Name), "_")
	_ = writeJSON(filepath.Join(p.stateDir, "diagnostics", name+"-nudge-unconfirmed.json"), d)
}
