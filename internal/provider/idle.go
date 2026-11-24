package provider

import "context"

// IdleState is one point-in-time idle reading of a session, the provider's
// answer to a gc runtime's IdleSnapshotProvider (SnapshotIdle). It is the
// provider half of GCREX-63. It mirrors the
// tri-state contract gc documents for its idle snapshots:
//
//   - IdleStateIdle: the session is at an idle interactive boundary right now
//     (a ready prompt, and no active turn). gc's snapshotHoldFor may then trust
//     the activity clock's quiet verdict.
//   - IdleStateBusy: the agent is working or blocked on an interaction, so the
//     session is not quiet even when its output has aged past the grace window.
//   - IdleStateIndeterminate: the observation was made but matched neither, so
//     it is evidence of neither. gc falls back to the signal it would use
//     without a snapshot (gc runtime.ErrIdleIndeterminate).
//
// A reading that could not be made is an error, never an IdleState; callers must
// not read an error as idle (gc runtime.IdleSnapshotProvider: a session that
// has gone away is reported as an error, not as idle).
type IdleState string

const (
	IdleStateIdle          IdleState = "idle"
	IdleStateBusy          IdleState = "busy"
	IdleStateIndeterminate IdleState = "indeterminate"
)

// busyMarkers are the literal active-processing indicators a pane shows while a
// turn runs. They carry the three literal clauses of gc's
// paneContainsBusyIndicator (internal/runtime/tmux/tmux.go:6240-6251 at the gc
// edge this provider is verified against); gc's two spinner regexes
// (claudeBusySpinnerRe and claudeBareGerundSpinnerRe) are not ported, so a
// spinner-only pane that emits no OSC 7501 status reads indeterminate here
// where tmux reads busy. That is the safe direction: an unclassified pane is
// evidence of neither state, and the program_status record is the primary
// reading; this scan only runs for an agent that emits no OSC 7501 status.
var busyMarkers = []string{
	"esc to interrupt",
	"Press Esc or Ctrl+C to cancel",
	"[current working directory ",
}

// readyPromptPrefixes are the prompt glyphs a ready composer can show: ❯ is the
// default idle prefix gc uses for tmux (`internal/runtime/tmux/tmux.go:5738`,
// gc's `defaultReadyPromptPrefix`), and the `›`/`>` pair is this provider's
// readiness glyph set (`startupPromptGlyphs` in readiness.go:68-73, from
// readiness.py's `PROMPT_GLYPHS`). The provider has no per-session ready prefix
// at op time, so a conservative default set keeps the fallback from calling a
// pane idle on a glyph it does not recognise.
var readyPromptPrefixes = []string{"❯ ", "› ", "> "}

// IdleSnapshot reports, in a single non-blocking observation, whether a session
// is at an idle interactive boundary now. It prefers the newest program_status
// record the pane program reports (the same data get-last-activity and nudge
// gating read): idle is IdleStateIdle, working and blocked are IdleStateBusy,
// and any other state (done, error) is IdleStateIndeterminate because it is not
// a safe idle boundary. With no record it classifies the rendered pane: a
// busy-indicator line is IdleStateBusy, a startup dialog is
// IdleStateIndeterminate, a bare ready prompt is IdleStateIdle, and anything
// else is IdleStateIndeterminate.
//
// A session that is not there is ErrSessionNotFound, and a failed program-status
// or pane read is returned as-is: a failed read is never reported as idle.
func (p *Provider) IdleSnapshot(ctx context.Context, name string) (IdleState, error) {
	binding, ok, err := p.binder.Resolve(ctx, ResolveRequest{Name: SessionName(name), Mode: ResolveFresh})
	if err != nil {
		return "", err
	}
	if !ok {
		return "", ErrSessionNotFound
	}

	snap, err := p.programStatusSnapshot(ctx, binding)
	if err != nil {
		return "", err
	}
	verdict, err := idleVerdict(p.observePane(ctx, binding, snap))
	if err != nil {
		return "", err
	}
	switch verdict {
	case verdictIdle:
		return IdleStateIdle, nil
	case verdictBusy:
		return IdleStateBusy, nil
	default:
		return IdleStateIndeterminate, nil
	}
}
