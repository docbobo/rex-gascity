package provider

import (
	"context"
	"slices"

	"github.com/docbobo/rex-gascity/internal/rex"
)

// IsAttached reports whether a human is watching the agent's pane: an app
// client exists and the agent's rig tab is the active window containing the
// pane. A missing
// pane is not attached. A Rex transport error is returned as is — gc reads any
// is-attached error as "not attached" (fail-open), so the caller must not
// mistake that error for a safe "nobody is watching" answer. A client of a
// kind we cannot classify answers true, because that fail-open reading must
// not close a pane a human may be watching.
func (p *Provider) IsAttached(ctx context.Context, name string) (bool, error) {
	binding, ok, err := p.binder.Resolve(ctx, ResolveRequest{Name: SessionName(name), Mode: ResolveFresh})
	if err != nil {
		return false, err
	}
	if !ok {
		return false, nil
	}
	clients, err := p.client.ListClients(ctx, binding.SessionID)
	if err != nil {
		return false, err
	}
	hasApp := false
	for _, c := range clients {
		switch c.Kind {
		case "app":
			hasApp = true
		case "cli", "peer":
		default:
			// We cannot classify this client, so we cannot tell whether a
			// human is watching. gc reads any is-attached error as "not
			// attached" (fail-open) and then lets config-drift restart,
			// progress-stall recycle and the drain-ack escalation kill
			// proceed, so answer true instead.
			return true, nil
		}
	}
	if !hasApp {
		return false, nil
	}
	view, err := p.client.SessionView(ctx, binding.SessionID)
	if err != nil {
		return false, err
	}
	return watchesAgent(view, binding), nil
}

// watchesAgent reports whether an app client is looking at the agent. Rex has
// one server-side active window, and a rig tab tiles its agents as panes, so a
// human watching the rig's tab sees every pane in it. The agent is therefore
// watched only when its rig tab is the active window and that window still
// holds the agent's block. The focused block of an inactive tab is not
// attachment: that pane is not visible. The provider focuses a brand-new
// session's tab on create and never moves focus for a re-attach, a relaunch or
// a split into an existing tab, so attachment reflects what the user is looking
// at. A window or block gone from view is false.
func watchesAgent(view rex.SessionView, b Binding) bool {
	if view.ActiveWindowID != b.WindowID {
		return false
	}
	for _, w := range view.Windows {
		if w.ID == b.WindowID {
			return slices.ContainsFunc(w.Blocks, func(blk rex.Block) bool { return blk.ID == b.BlockID })
		}
	}
	return false
}
