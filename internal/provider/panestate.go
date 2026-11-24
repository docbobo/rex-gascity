package provider

import (
	"context"
	"strings"
)

// The pane-state module is meant to answer every question the provider asks
// about what a pane shows: is it idle (IdleSnapshot), is it ready for the first
// turn (the startup readiness wait), and did a nudged turn start (nudge
// confirmation and the resend ladder). Each question has one entry point that
// takes one observation of the pane and returns one verdict. The screen
// predicates in readiness.go are called from here and nowhere else.
//
// This is the intended end state. IdleSnapshot uses idleVerdict and the startup
// readiness wait uses startupVerdict. nudge_delivery.go still calls the
// readiness predicates directly for nudge confirmation and the resend ladder.
// Those callers move here in later tasks.
//
// The callers combine program_status and the screen differently on purpose.
// Those divergences (D1-D6, issue #44) are kept, and TestPaneStateVerdict pins
// each one as a named row:
//
//   - D1: the idle snapshot trusts a status record over a dialog on screen.
//   - D2: the idle snapshot checks menu cursors with "" only; startup uses the
//     configured ready_prompt_prefix in prompt mode.
//   - D3: startup does not consult busyMarkers.
//   - D4: the idle snapshot reads readyPromptPrefixes; startup only the
//     configured prefix.
//   - D5: the idle snapshot trims the status state; startup and the resend
//     ladder do not.
//   - D6: startup in status mode reads no screen before the floor and the
//     bound.

// paneVerdict is the pane-state module's one answer.
type paneVerdict string

const (
	verdictWaiting     paneVerdict = "waiting"      // evidence of nothing yet, or not decided
	verdictDialog      paneVerdict = "dialog"       // a startup dialog owns the input
	verdictPromptReady paneVerdict = "prompt-ready" // the configured ready prompt shows
	verdictIdle        paneVerdict = "idle"         // an idle boundary
	verdictBusy        paneVerdict = "busy"         // a turn runs, or is blocked on an interaction
)

// paneObservation is one observation of a pane: the newest program_status
// record (Present=false when there is none, or the status read failed) and a
// screen read that runs at most once, and only when a rule asks for it.
type paneObservation struct {
	status programStatusSnapshot
	screen *screenRead
}

// screenRead memoizes one screen read. The first get runs read; later gets
// return the same text and error without reading again.
type screenRead struct {
	read func() (string, error)
	done bool
	text string
	err  error
}

// newScreenRead wraps read so that it runs at most once.
func newScreenRead(read func() (string, error)) *screenRead {
	return &screenRead{read: read}
}

// get returns the screen, reading it on the first call only.
func (s *screenRead) get() (string, error) {
	if !s.done {
		s.done = true
		s.text, s.err = s.read()
	}
	return s.text, s.err
}

// observePane builds one observation of a pane: status is the caller's
// program_status snapshot (the zero snapshot when the status read failed), and
// the screen is read through paneScreen only if a rule asks for it.
func (p *Provider) observePane(ctx context.Context, b Binding, status programStatusSnapshot) paneObservation {
	return paneObservation{
		status: status,
		screen: newScreenRead(func() (string, error) { return p.paneScreen(ctx, b.SessionID, b.BlockID) }),
	}
}

// idleVerdict answers IdleSnapshot. A status record decides alone (trimmed,
// D5) and the screen is not read (D1). Without a record the screen decides: a
// startup dialog is dialog, a busyMarkers line is busy, a bare ready prompt over
// readyPromptPrefixes is idle (D4), anything else is waiting. Menu cursors are
// checked with an empty prefix (D2) and busy markers are consulted (D3). A
// failed screen read is returned as the error and is never a verdict.
func idleVerdict(o paneObservation) (paneVerdict, error) {
	if o.status.Present {
		switch strings.TrimSpace(o.status.State) {
		case "idle":
			return verdictIdle, nil
		case "working", "blocked":
			return verdictBusy, nil
		default:
			return verdictWaiting, nil
		}
	}
	screen, err := o.screen.get()
	if err != nil {
		return "", err
	}
	switch {
	case screen == "":
		return verdictWaiting, nil
	case withholdsFirstTurn(screen, ""):
		return verdictDialog, nil
	case screenShowsBusyMarker(screen):
		return verdictBusy, nil
	case screenShowsReadyPrompt(screen):
		return verdictIdle, nil
	default:
		return verdictWaiting, nil
	}
}

// startupVerdict answers one poll of the startup readiness wait. prefix is
// ready_prompt_prefix; "" selects status mode. pastFloor reports that the
// ready_delay_ms floor has elapsed, atBound that the wait's bound has. waiting
// with atBound means "deliver on the bound".
//
// Prompt mode reads the screen on every poll: a dialog withholds, the
// configured prompt is prompt-ready, anything else (a failed read too) waits.
// Status mode reads the screen only when it can decide (D6): an untrimmed
// "idle" record past the floor (D5) is idle unless a dialog shows (GCREX-59),
// and at the bound a dialog withholds. Busy markers are not consulted (D3). A
// failed read there means "no dialog".
func startupVerdict(o paneObservation, prefix string, pastFloor, atBound bool) paneVerdict {
	if prefix != "" {
		screen, err := o.screen.get()
		switch {
		case err != nil:
			return verdictWaiting
		case withholdsFirstTurn(screen, prefix):
			return verdictDialog
		case screenHasPromptPrefix(screen, prefix):
			return verdictPromptReady
		default:
			return verdictWaiting
		}
	}
	if pastFloor && o.status.Present && o.status.State == "idle" {
		if screenWithholdsFirstTurn(o) {
			return verdictDialog
		}
		return verdictIdle
	}
	if atBound && screenWithholdsFirstTurn(o) {
		return verdictDialog
	}
	return verdictWaiting
}

// screenWithholdsFirstTurn is status mode's dialog check: the screen reads and
// shows a dialog with no configured prefix. A failed read is no dialog.
func screenWithholdsFirstTurn(o paneObservation) bool {
	screen, err := o.screen.get()
	return err == nil && withholdsFirstTurn(screen, "")
}

// screenShowsBusyMarker reports whether a busyMarkers line is on screen.
func screenShowsBusyMarker(screen string) bool {
	for _, marker := range busyMarkers {
		if strings.Contains(screen, marker) {
			return true
		}
	}
	return false
}

// screenShowsReadyPrompt reports whether the bottom-most prompt-prefixed line is
// a bare ready composer. It reuses screenHasPromptPrefix over
// readyPromptPrefixes. Text after the prefix is accepted; only a numbered-menu
// cursor row is excluded. That is what gc's paneShowsReadyPrompt does in its
// current source. Parity against the pinned gc v1.5.1 is unverified, because
// that tag has no paneShowsReadyPrompt.
func screenShowsReadyPrompt(screen string) bool {
	for _, prefix := range readyPromptPrefixes {
		if screenHasPromptPrefix(screen, prefix) {
			return true
		}
	}
	return false
}
