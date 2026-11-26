package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
)

// defaultPoolClaimNudge is gc's stalled-claim fallback text (v1.5.1
// cmd/gc/idle_nudge.go defaultPoolClaimNudge). gc's reconcile backstop types it
// through the runtime provider's Nudge op (v1.5.1 cmd/gc/nudge_backstop.go
// runNudgeBackstop: sp.Nudge(sessName, ...)) after a 90 s grace once a pool
// slot's routed trigger bead changes while the seat is still on its first
// claim. The provider refuses to type it into a session that already holds a
// live claim, so the re-armed nudge cannot send a working seat back through
// `gc hook --claim` and make it abandon its in-flight task (GCREX-33).
const defaultPoolClaimNudge = "Run gc hook --claim --drain-ack --json now; if it returns work, execute it immediately."

// poolClaimNudgeSignature matches gc's fallback pool-claim nudge and an agent
// template that configures the same "run gc hook --claim" instruction. The
// prompt is specific to claiming work; matching it does not widen the guard to
// ordinary mail or reminder nudges.
const poolClaimNudgeSignature = "gc hook --claim --drain-ack"

// claimProbeTimeout bounds the `gc hook current --id-only` read Nudge makes
// before delivering a pool-claim nudge. It is short: a slow or unreachable gc
// must not hold up the nudge, and a failed probe is treated as "no claim" so
// delivery falls back to the previous behavior.
const claimProbeTimeout = 5 * time.Second

// nudgeConfig holds the tunables and external seam of nudge and startup
// delivery. Provider holds it in a named field, so the fields read as
// p.nudge.nudgeConfirmWindow and so on; nudge.go, nudge_delivery.go and
// pane_delivery.go are its only users.
type nudgeConfig struct {
	// nudgeConfirmWindow bounds how long Nudge waits for a turn start before
	// reporting the delivery unconfirmed. nudgePollInterval spaces the
	// observations within that window.
	nudgeConfirmWindow time.Duration
	nudgePollInterval  time.Duration
	nudgeSubmitBudget  time.Duration

	// claimProbe reads back the work bead a session most recently claimed
	// through `gc hook --claim`, and returns it only while the session still
	// holds it (mirroring gc's claimedWorkStillHeldBy: a closed, released, or
	// foreign-owned claim is not held). Nudge consults it before delivering
	// gc's fallback pool-claim nudge so a seat that already holds a claim is
	// not re-prompted mid-task, and a stale stamp does not suppress the nudge
	// forever (GCREX-33).
	claimProbe func(context.Context, Binding) (string, error)

	// startupReadyWindow bounds how long Start waits for the pane program to
	// report idle before delivering the startup nudge.
	// startupSettleDelay is the shorter bound Start applies when the program
	// reports no program_status at all, so an agent that never emits one is
	// not held for the whole window.
	startupReadyWindow time.Duration
	startupSettleDelay time.Duration

	// startupNudgeBackoffs is the ladder of delays between startup-nudge
	// resends after a delivered-but-unconfirmed submit. An empty ladder
	// disables resending.
	startupNudgeBackoffs []time.Duration
}

// defaultNudgeConfig returns the production nudge and startup-delivery config.
func defaultNudgeConfig() nudgeConfig {
	return nudgeConfig{
		nudgeConfirmWindow:   defaultNudgeConfirmWindow,
		nudgePollInterval:    defaultNudgePollInterval,
		nudgeSubmitBudget:    defaultNudgeSubmitBudget,
		startupReadyWindow:   defaultStartupReadyWindow,
		startupSettleDelay:   defaultStartupSettleDelay,
		startupNudgeBackoffs: defaultStartupNudgeBackoffs,
	}
}

// Defaults for the bounded nudge confirmation window.
const (
	defaultNudgeConfirmWindow = 3 * time.Second
	defaultNudgePollInterval  = 100 * time.Millisecond
)

// WithNudgeConfirmation sets how long Nudge waits for a turn start and how
// often it observes the pane while waiting. A window <= 0 takes one
// observation and returns immediately.
func WithNudgeConfirmation(window, interval time.Duration) Option {
	return func(p *Provider) {
		p.nudge.nudgeConfirmWindow = window
		if interval > 0 {
			p.nudge.nudgePollInterval = interval
		}
	}
}

// ErrNudgeClaimHeld reports that gc's fallback pool-claim nudge was not typed
// because the target session already holds a live claim. It is answered as the
// RPP "busy" input outcome (exit 80): the seat is live, not idle. At gc
// `890a58a3e` / `544beb8d6` (#9137 only, not in `v1.5.1`; inert at the pin)
// that word makes the stalled-claim backstop refund the reservation as a busy
// deferral and retry; at `v1.5.1` exit 80 is a plain error and the attempt is
// spent, the same as the old exit 1. A failed-delivery outcome would instead
// let the backstop re-dispatch the seat's claim and abandon the in-flight work
// (GCREX-69).
var ErrNudgeClaimHeld = errors.New("provider: session holds a live claim")

// defaultNudgeSubmitBudget bounds the write and the submit together when no
// submit budget is configured. gc's own op timeout is 30 s; this leaves
// room for the confirmation window without risking a split submit.
const defaultNudgeSubmitBudget = 10 * time.Second

// NudgeSubmitFailedMetaKey records that a nudge typed its text but failed to
// deliver the submit key. gc reads only the exit code, so an operator finds
// this marker in the provider metadata when a prompt was left unsubmitted.
const NudgeSubmitFailedMetaKey = "GC_NUDGE_SUBMIT_FAILED"

// WithNudgeSubmitBudget sets how long the write and the submit may take
// together, capped by the op context's earlier deadline. A value <= 0 keeps
// the default.
func WithNudgeSubmitBudget(d time.Duration) Option {
	return func(p *Provider) {
		if d > 0 {
			p.nudge.nudgeSubmitBudget = d
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

	// Write is how long the text write took; Confirm is how long the
	// post-submit confirmation poll took before it confirmed or its window
	// elapsed. Both are zero when the stage did not run (a refused or failed
	// delivery). Startup delivery records them per attempt in the timing
	// diagnostic so a slow start can be attributed to a stage.
	Write   time.Duration
	Confirm time.Duration
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
// the message on its next attempt. Text that is not UTF-8 is rejected before
// any Rex call (ErrNudgeNotUTF8).
func (p *Provider) NudgeReport(ctx context.Context, name, text string, mode NudgeMode) (NudgeReport, error) {
	if text == "" {
		return NudgeReport{}, nil
	}
	if err := checkNudgeText(text); err != nil {
		return NudgeReport{}, err
	}
	if err := ctx.Err(); err != nil {
		return NudgeReport{}, fmt.Errorf("nudge %s: before typing: %w", name, err)
	}
	binding, err := p.resolveInputPane(ctx, inputNudge, name)
	if err != nil {
		return NudgeReport{}, err
	}
	res := p.deliverPaneText(ctx, paneTextRequest{Binding: binding, Text: text, Mode: mode})
	return res.Report, res.Err
}

// checkNudgeText rejects text that is not UTF-8 before any Rex call, so no
// pane receives U+FFFD-corrupted text (#43 REQ-003).
func checkNudgeText(text string) error {
	if !utf8.ValidString(text) {
		return classifyInput(inputNudge, stageValidate, ErrNudgeNotUTF8)
	}
	return nil
}

// isPoolClaimNudge reports whether text is gc's fallback pool-claim nudge (or
// an agent template that configures the same claim instruction). The match is
// a substring of `poolClaimNudgeSignature`, so any text carrying that
// instruction matches; a false positive only refuses a nudge as a refundable
// `busy` while a claim is live.
func isPoolClaimNudge(text string) bool {
	t := strings.TrimSpace(text)
	return t == defaultPoolClaimNudge || strings.Contains(t, poolClaimNudgeSignature)
}

// claimBeadRecord is the subset of `gc bd list --all --json` the liveness check
// reads. Field names match bd's JSON output.
type claimBeadRecord struct {
	Status   string            `json:"status"`
	Assignee string            `json:"assignee"`
	Metadata map[string]string `json:"metadata"`
}

// probeLiveClaim reads the work bead b's session most recently claimed through
// `gc hook --claim`, and returns it only while the session still holds it. It is
// the default claimProbe. A legacy binding with no ClaimSessionID resolves to
// "", so the guard delivers exactly as before.
//
// gc stamps current_claim_bead_id on the session bead and does not clear it when
// the claimed work bead closes (v1.5.1 internal/session/store.go:356-362), so a
// non-empty stamp alone is not liveness: a seat that finished its bead keeps the
// stamp. gc's own reconciler therefore re-reads the work bead and treats a
// closed, unassigned, or foreign-owned claim as not held (v1.5.1
// cmd/gc/session_ownership.go sessionOwnsLiveClaim/claimedWorkStillHeldBy:78-134).
// The probe mirrors that predicate with `gc bd list --id <claim> --all --json`,
// so a completed claim stops deferring the backstop nudge (AC #4) instead of
// suppressing it until the session ends.
func (p *Provider) probeLiveClaim(ctx context.Context, b Binding) (string, error) {
	sessionID := strings.TrimSpace(b.ClaimSessionID)
	if sessionID == "" {
		return "", nil
	}
	bin := strings.TrimSpace(os.Getenv("GC_BIN"))
	if bin == "" {
		bin = "gc"
	}
	claim, err := p.runGCProbe(ctx, bin, b, sessionID, "hook", "current", "--id-only")
	if err != nil || claim == "" {
		return "", nil
	}
	out, err := p.runGCProbe(ctx, bin, b, sessionID, "bd", "list", "--id", claim, "--all", "--json")
	if err != nil {
		// Mirror sessionOwnsLiveClaim: a read error reports the claim as held,
		// so a transient store failure never clobbers a working seat.
		return claim, nil
	}
	if claimRecordHeld(out, sessionID) {
		return claim, nil
	}
	return "", nil
}

// runGCProbe runs one gc probe and returns its trimmed stdout. gc hook current
// names the calling session from GC_SESSION_ID alone; the provider op process
// carries the controller's identity, not the target seat's, so the started
// session id is supplied explicitly.
func (p *Provider) runGCProbe(ctx context.Context, bin string, b Binding, sessionID string, args ...string) (string, error) {
	probeCtx, cancel := context.WithTimeout(ctx, claimProbeTimeout)
	defer cancel()
	out, err := p.runner.Run(probeCtx, b.WorkDir, []string{"GC_SESSION_ID=" + sessionID}, bin, args...)
	return strings.TrimSpace(out), err
}

// claimRecordHeld mirrors gc's claimedWorkStillHeldBy predicate (v1.5.1
// cmd/gc/session_ownership.go:123-133) against the `gc bd list --all --json`
// record for the stamped claim. An empty list is a missing or closed bead: not
// held. A garbled non-empty answer is treated as held so it cannot make a
// working seat look idle.
func claimRecordHeld(out string, sessionID string) bool {
	if strings.TrimSpace(out) == "" {
		return false
	}
	var records []claimBeadRecord
	if err := json.Unmarshal([]byte(out), &records); err != nil {
		return true
	}
	if len(records) == 0 {
		return false
	}
	w := records[0]
	if w.Status == "closed" {
		return false
	}
	if w.Status == "open" && strings.TrimSpace(w.Assignee) == "" {
		return false
	}
	if owner := strings.TrimSpace(w.Metadata["gc.session_id"]); owner != "" && owner != sessionID {
		return false
	}
	return true
}

// recordNudgeSubmitFailure marks the pane's metadata after the text reached it
// but the Enter did not, so the split is visible after the error is gone.
func (p *Provider) recordNudgeSubmitFailure(b Binding, err error) {
	_ = p.metaSet(b.Name, NudgeSubmitFailedMetaKey, err.Error())
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
	if p.ns.CanonicalDir() == "" {
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
	_ = writeJSON(filepath.Join(p.ns.CanonicalDir(), "diagnostics", name+"-nudge-unconfirmed.json"), d)
}
