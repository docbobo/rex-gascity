package provider

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/docbobo/rex-gascity/internal/rex"
)

// ErrAttachmentUnknown is returned by IsAttached when Rex answered but no
// client's kind identifies a human app client, so answering false would be
// dishonest.
var ErrAttachmentUnknown = errors.New("provider: attachment cannot be determined")

// IsAttached reports whether a human is watching the agent's pane: an app
// client exists and the agent's rig tab is the active window containing the
// pane. A missing
// pane is not attached. A Rex transport error is returned as is, and a client
// of an unknown kind yields ErrAttachmentUnknown.
func (p *Provider) IsAttached(ctx context.Context, name string) (bool, error) {
	binding, ok, err := p.binder.Resolve(ctx, SessionName(name))
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
			return false, fmt.Errorf("attached %s: client kind %q: %w", name, c.Kind, ErrAttachmentUnknown)
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
// attachment: that pane is not visible. Provider launches use focus=false so
// starting an agent never makes its tab active; attachment reflects only what
// the user chose to look at (a brand-new session's first tab is active
// regardless). A window or block gone from view is false.
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
