package provider

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Pane text delivery: type text into a resolved pane, observe whether a turn
// started, and run the caller's resend policy. NudgeReport (nudge.go) and
// startup delivery (nudge_delivery.go) both enter through deliverPaneText.
// They supply only policy (mode, backoffs, clear key) and map the result.
// The screen and verdict reads go through panestate.go, the status read
// through programStatusSnapshot (programstatus.go), and the secret guard
// through readingSecret (inputmode.go).

// paneTextRequest is one delivery of text into a resolved pane: the gates, the
// write and submit under one deadline, the confirmation poll, and the optional
// resend ladder. Nudge and startup delivery both build one, so the ladder's
// call order lives in one place; the caller maps the result to its own marker
// or error.
type paneTextRequest struct {
	Binding Binding
	Text    string
	Mode    NudgeMode
	// Backoffs is the delay before each resend after a delivered-but-
	// unconfirmed attempt. Empty disables resending.
	Backoffs []time.Duration
	// ClearKey empties the draft line before each resend; empty presses none.
	// A failed clear stops the ladder, because pasting onto a draft that may
	// still hold the first copy duplicates the text.
	ClearKey string
}

// paneTextAttempt is one attempt in the shared ladder: the backoff that
// preceded it (zero for the first), its report, and the error that ended it.
type paneTextAttempt struct {
	Attempt int
	Backoff time.Duration
	Report  NudgeReport
	Err     error
}

// paneTextResult is the last attempt's report and error, plus the attempts
// that ran. LateTurn and Withheld name the two ladder stops that are not a
// delivery failure: a turn that started after the window, and a pane reading
// a secret.
type paneTextResult struct {
	Report   NudgeReport
	Err      error
	Attempts []paneTextAttempt
	LateTurn bool
	Withheld bool
}

// deliverPaneText runs the shared pane-delivery ladder: one gated write,
// submit and confirmation per attempt, with the caller's resend delays. It
// stops on a confirmed delivery, a failure, a secret read, or a late turn, and
// reports every attempt so the caller can record its timing.
func (p *Provider) deliverPaneText(ctx context.Context, req paneTextRequest) paneTextResult {
	var res paneTextResult
	res.Attempts = make([]paneTextAttempt, 0, 1+len(req.Backoffs))
	for attempt := 1; ; attempt++ {
		var backoff time.Duration
		if attempt > 1 {
			backoff = req.Backoffs[attempt-2]
			if !p.sleepStartupBackoff(ctx, backoff) {
				return res
			}
			// The turn may have started late: gc checks for a busy pane after
			// the backoff and before resending, so a nudge that landed a moment
			// after the confirmation window is not typed twice.
			snap, err := p.programStatusSnapshot(ctx, req.Binding)
			if err != nil {
				snap = programStatusSnapshot{}
			}
			if lateTurnVerdict(p.observePane(ctx, req.Binding, snap)) == verdictBusy {
				res.LateTurn = true
				return res
			}
			// ctrl+u is VKILL in a cooked read: pressed into a password prompt
			// it would erase what a person is typing. Read the mode before the
			// clear and stop resending on a secret read (#48). A failed read
			// fails open.
			if p.readingSecret(ctx, req.Binding) {
				res.Withheld = true
				return res
			}
			if req.ClearKey != "" {
				if err := p.client.SendKeys(ctx, req.Binding.SessionID, req.Binding.BlockID, req.ClearKey); err != nil {
					res.Err = fmt.Errorf("clear the draft line before a resend: %w", err)
					return res
				}
			}
		}
		report, err := p.deliverPaneTextOnce(ctx, req.Binding, req.Text, req.Mode)
		res.Report, res.Err = report, err
		res.Attempts = append(res.Attempts, paneTextAttempt{Attempt: attempt, Backoff: backoff, Report: report, Err: err})
		if err == nil && report.Confirmed {
			return res
		}
		// The pre-clear read passed, but an attempt's own read then saw a
		// secret read: the guard is a snapshot check, so a ctrl+u already
		// pressed stays pressed. Nothing more is typed and resending stops.
		if errors.Is(err, ErrSecretRead) {
			res.Withheld = true
			return res
		}
		if err != nil {
			return res
		}
		if attempt-1 >= len(req.Backoffs) {
			return res
		}
	}
}

// deliverPaneTextOnce is one try of the ladder against an already-resolved
// pane: the UTF-8 check, the pool-claim gate, the busy/blocked gate, the
// secret guard, the baseline read, the write and Enter under one submit
// deadline, and the confirmation poll. It takes a binding, not a name, so
// startup delivery types into the pane it launched, not whatever a name
// lookup resolves to after a concurrent stop and start of the same name.
func (p *Provider) deliverPaneTextOnce(ctx context.Context, binding Binding, text string, mode NudgeMode) (NudgeReport, error) {
	name := string(binding.Name)
	// Startup delivery enters here without NudgeReport, so the UTF-8 check
	// runs again: no caller can type corrupted text.
	if err := checkNudgeText(text); err != nil {
		return NudgeReport{}, err
	}
	// gc's stalled-claim backstop re-arms the fallback pool-claim nudge for a
	// pool slot whose routed trigger bead changes while the seat is still on
	// its first claim. Refuse to type that prompt while a claim is live: the
	// seat is not idle, and the prompt would send it back through
	// `gc hook --claim` to abandon its in-flight work (GCREX-33).
	if isPoolClaimNudge(text) {
		if claim, err := p.nudge.claimProbe(ctx, binding); err == nil && claim != "" {
			err := fmt.Errorf("%w: %s holds %s", ErrNudgeClaimHeld, name, claim)
			return NudgeReport{}, classifyInput(inputNudge, stageGate, err)
		}
	}
	baseline, _ := p.programStatusSnapshot(ctx, binding)
	if mode == NudgeModeWait {
		switch baseline.State {
		case "working":
			err := fmt.Errorf("%w: %s", ErrNudgeBusy, name)
			return NudgeReport{State: baseline.State, Kind: baseline.Kind},
				classifyInput(inputNudge, stageGate, err)
		case "blocked":
			report := NudgeReport{State: baseline.State, Kind: baseline.Kind}
			var err error
			if baseline.Kind != "" {
				err = fmt.Errorf("%w: %s (%s)", ErrNudgeBlocked, name, baseline.Kind)
			} else {
				err = fmt.Errorf("%w: %s", ErrNudgeBlocked, name)
			}
			return report, classifyInput(inputNudge, stageGate, err)
		}
	}

	// A pane reading a secret (a password prompt) must not receive the text:
	// it would be submitted as a password attempt (#48). The check runs in
	// both modes, after the Wait gate and right before the baseline read and
	// the write, to keep the check-then-write window short. A failed read
	// fails open (readingSecret).
	if p.readingSecret(ctx, binding) {
		err := fmt.Errorf("%w: %s (input_mode canonical, echo off)", ErrSecretRead, name)
		return NudgeReport{State: baseline.State, Kind: baseline.Kind},
			classifyInput(inputNudge, stageGate, err)
	}

	// Without a program_status record, the pane's rendered output is the only
	// available confirmation signal. turnBaseline reads it now, before typing.
	base := turnBaseline(p.observePane(ctx, binding, baseline))

	// One deadline covers the write and the Enter. Without it a stalled Rex
	// call or a gc cancel between the two could leave the text typed but
	// unsubmitted, and gc's retry would then duplicate the prompt.
	submitCtx, cancelSubmit := p.nudgeSubmitContext(ctx)
	defer cancelSubmit()
	// A cancel here is not a send failure: it stays a plain error (exit 1),
	// never classified (#43 REQ-010).
	if err := ctx.Err(); err != nil {
		return NudgeReport{State: baseline.State, Kind: baseline.Kind}, fmt.Errorf("nudge %s: before typing: %w", name, err)
	}
	writeStart := time.Now()
	if err := p.client.BlockWrite(submitCtx, binding.SessionID, binding.BlockID, text); err != nil {
		return NudgeReport{State: baseline.State, Kind: baseline.Kind, Write: time.Since(writeStart)},
			classifyInput(inputNudge, stageSend, fmt.Errorf("nudge %s: type text: %w", name, err))
	}
	writeDur := time.Since(writeStart)
	// The text is in the pane now: stamp the input floor so the delivery is
	// visible to idle detection before the program's output renders.
	if inputPolicies[inputNudge].notesInput {
		p.noteInput(binding)
	}
	// The text is in the pane now, so the Enter must be attempted even if the
	// parent was cancelled. It keeps the same submit deadline, so a cancel or
	// stall cannot make this run forever.
	submitDeadline, _ := submitCtx.Deadline()
	enterCtx, cancelEnter := context.WithDeadline(context.WithoutCancel(ctx), submitDeadline)
	defer cancelEnter()
	if err := p.client.SendKeys(enterCtx, binding.SessionID, binding.BlockID, submitKey); err != nil {
		p.recordNudgeSubmitFailure(binding, err)
		return NudgeReport{State: baseline.State, Kind: baseline.Kind, Write: writeDur},
			classifyInput(inputNudge, stageSubmit, fmt.Errorf("nudge %s: text was typed but submit key failed: %w", name, err))
	}
	_ = p.metaDelete(binding.Name, NudgeSubmitFailedMetaKey)

	confirmStart := time.Now()
	report := NudgeReport{Delivered: true, Write: writeDur}
	deadline := time.Now().Add(p.nudge.nudgeConfirmWindow)
	first := true
	for {
		snapshot, err := p.programStatusSnapshot(ctx, binding)
		if err != nil {
			snapshot = programStatusSnapshot{}
		}
		if snapshot.Present {
			report.State, report.Kind = snapshot.State, snapshot.Kind
		}
		if verdict, source := turnVerdict(base, p.observePane(ctx, binding, snapshot)); verdict == verdictBusy {
			report.Source = source
			report.Confirmed = true
			report.Confirm = time.Since(confirmStart)
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
		wait := p.nudge.nudgePollInterval
		if wait > remaining {
			wait = remaining
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			// The text and submit already reached the pane. Report the
			// delivery instead of an error that would duplicate it.
			report.Confirm = time.Since(confirmStart)
			p.writeNudgeDiagnostic(binding, text, report)
			return report, nil
		case <-timer.C:
		}
	}

	report.Confirm = time.Since(confirmStart)
	p.writeNudgeDiagnostic(binding, text, report)
	return report, nil
}

// nudgeSubmitContext bounds the write and the submit together. It uses the op
// context's deadline when it has one, else a fixed default, so neither call
// can wait forever.
func (p *Provider) nudgeSubmitContext(parent context.Context) (context.Context, context.CancelFunc) {
	budget := p.nudge.nudgeSubmitBudget
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

// sleepStartupBackoff waits out one resend delay. It reports false when the
// start context is done first, so a canceled start does not sleep through the
// ladder only to record the same unconfirmed delivery later.
func (p *Provider) sleepStartupBackoff(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return ctx.Err() == nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
