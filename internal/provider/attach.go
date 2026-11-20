package provider

import (
	"context"
	"slices"

	"github.com/docbobo/rex-gascity/internal/rex"
)

// IsAttached reports whether a human is watching the agent's pane. Attachment
// has the two halves gc's own model implies. The session half is the block's
// claim: the size method's owner is the client the block is displayed for, and
// is empty when the app never showed the box even though the app is attached to
// the session. The block half is the pane's window being the session's active
// window.
//
// An app client that does not own the block is not watching it. A client kind we
// cannot classify answers true, because gc reads any is-attached error as "not
// attached" (fail-open) and must not close a pane a human may be watching. A
// stale owner no longer in the client list also answers true: ownership is a
// last-requested claim, not a heartbeat, so it errs toward true. A Rex transport
// error is returned as is, so the op exits non-zero rather than answering false.
func (p *Provider) IsAttached(ctx context.Context, name string) (bool, error) {
	binding, ok, err := p.binder.Resolve(ctx, ResolveRequest{Name: SessionName(name), Mode: ResolveFresh})
	if err != nil {
		return false, err
	}
	if !ok {
		return false, nil
	}
	size, err := p.client.BlockSize(ctx, binding.SessionID, binding.BlockID)
	if err != nil {
		return false, err
	}
	if size.Owner == "" {
		return false, nil
	}
	clients, err := p.client.ListClients(ctx, binding.SessionID)
	if err != nil {
		return false, err
	}
	kind, found := "", false
	for _, c := range clients {
		if c.ID == size.Owner {
			kind, found = c.Kind, true
			break
		}
	}
	switch {
	case !found:
		return true, nil
	case kind == "app":
	case kind == "cli" || kind == "peer":
		return false, nil
	default:
		return true, nil
	}
	view, err := p.client.SessionView(ctx, binding.SessionID)
	if err != nil {
		return false, err
	}
	return watchesAgent(view, binding), nil
}

// watchesAgent reports whether the agent's rig tab is the active window and
// still holds the agent's block. Rex has one server-side active window, and a
// rig tab tiles its agents as panes, so a human watching the rig's tab sees
// every pane in it. The focused block of an inactive tab is not attachment:
// that pane is not visible. A window or block gone from view is false.
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
