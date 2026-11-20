package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/docbobo/rex-gascity/internal/rex"
)

// The observe.v1 profile's read and kill ops. The profile is gated by the
// handshake's observe.v1 token, which the dispatcher declares only when
// GCREX_OBSERVE_V1=1 until GCREX-111 retires the switch.

// Observe states, matching gc's wire vocabulary.
const (
	ObserveAbsent  = "absent"
	ObserveRunning = "running"
	ObserveDead    = "dead"
	ObserveUnknown = "unknown"
)

// ObserveObject is a box's identity on the backend: stable for its life and
// new for every new box under the name.
type ObserveObject struct {
	ID      string `json:"id"`
	Created string `json:"created"`
}

// ObserveAnswer is one `observe` answer. Object and Identity are present only
// for a box that holds the name (running or dead).
type ObserveAnswer struct {
	State    string            `json:"state"`
	Object   *ObserveObject    `json:"object,omitempty"`
	Agent    string            `json:"agent,omitempty"`
	AgentPID string            `json:"agent_pid,omitempty"`
	Identity map[string]string `json:"identity,omitempty"`
}

// InventoryEntry is one session in an `inventory` snapshot. A session whose
// attributes cannot be read carries only its name.
type InventoryEntry struct {
	Name     string            `json:"name"`
	State    string            `json:"state,omitempty"`
	Object   *ObserveObject    `json:"object,omitempty"`
	Agent    string            `json:"agent,omitempty"`
	Identity map[string]string `json:"identity,omitempty"`
}

// InventoryAnswer is the `inventory` wire object.
type InventoryAnswer struct {
	Complete bool             `json:"complete"`
	Sessions []InventoryEntry `json:"sessions"`
}

// KillObjectRequest is `kill-object`'s stdin.
type KillObjectRequest struct {
	Object   ObserveObject `json:"object"`
	Mode     string        `json:"mode"`
	AgentPID string        `json:"agent_pid,omitempty"`
}

// observeObjectFor maps a stored binding onto its wire object. The pane's
// block is the object; a provisional record without a block falls back to its
// window. The creation time is the binding's record time, formatted so a
// second observe or a kill-object re-check compares equal.
func observeObjectFor(b Binding) ObserveObject {
	id := string(b.BlockID)
	if id == "" {
		id = string(b.WindowID)
	}
	return ObserveObject{ID: id, Created: observeCreated(b.CreatedAt)}
}

// observeCreated formats a binding's creation time as the wire's created
// string. A zero time reads as the zero instant rather than an empty field,
// because a present box must carry a non-empty object.
func observeCreated(t time.Time) string {
	if t.IsZero() {
		return time.Time{}.UTC().Format(time.RFC3339)
	}
	return t.UTC().Format(time.RFC3339Nano)
}

// observeIdentity reads the start-config identity the provider seeded into the
// meta sidecar before the launch. It returns nil when the sidecar has none. The
// sidecar holds one file per key, so the whole identity is one snapshot of the
// session's sidecar rather than one read per key.
func (p *Provider) observeIdentity(n SessionName) map[string]string {
	all := map[string]string{}
	if snap, ok := p.meta.(snapshotter); ok {
		all = snap.Snapshot(n)
	} else {
		for _, key := range startIdentityMetaKeys {
			if v, ok, _ := p.metaGet(n, key); ok {
				all[key] = v
			}
		}
	}
	var id map[string]string
	for _, key := range startIdentityMetaKeys {
		if v := all[key]; v != "" {
			if id == nil {
				id = map[string]string{}
			}
			id[key] = v
		}
	}
	return id
}

// Observe answers `observe <name>`: absent when nothing holds the name,
// running or dead for the box that holds it, unknown when Rex cannot be
// reached. It is a read and never mutates the session; a vanished record is
// left for the next Start or Stop to replace.
func (p *Provider) Observe(ctx context.Context, name string, _ []string) (ObserveAnswer, error) {
	n := SessionName(name)
	bd, ok := p.Binding(name)
	if !ok {
		// No cached record: scan for a live pane under the name's labels.
		var err error
		bd, ok, err = p.binder.Resolve(ctx, ResolveRequest{Name: n, Mode: ResolveLive})
		if err != nil {
			if errors.Is(err, rex.ErrUnavailable) {
				return ObserveAnswer{State: ObserveUnknown}, nil
			}
			return ObserveAnswer{}, err
		}
		if !ok {
			return ObserveAnswer{State: ObserveAbsent}, nil
		}
	}
	state, holder, err := p.boxState(ctx, bd)
	if err != nil {
		if errors.Is(err, rex.ErrUnavailable) {
			return ObserveAnswer{State: ObserveUnknown}, nil
		}
		return ObserveAnswer{}, err
	}
	if state == ObserveAbsent {
		return ObserveAnswer{State: ObserveAbsent}, nil
	}
	idName := holder.Name
	if idName == "" {
		idName = n
	}
	obj := observeObjectFor(holder)
	ans := ObserveAnswer{State: state, Object: &obj}
	// Identity is the started incarnation's start config; a pane that took the
	// name without Start has none of its own.
	if holder.BlockID == bd.BlockID {
		ans.Identity = p.observeIdentity(idName)
	}
	if state == ObserveDead {
		ans.Agent = "dead"
	}
	return ans, nil
}

// boxState reports what holds the name a binding records. Rex proving the
// recorded block gone does not prove the name is free (observe.v1 rule 7): a
// restore or an operator can put another pane under the same labels, so the
// live scan decides. It claims no state of its own; unknown is the caller's
// answer for an unreadable backend. It is a read: a vanished record is left for
// the next Start or Stop to replace.
func (p *Provider) boxState(ctx context.Context, bd Binding) (string, Binding, error) {
	info, err := p.client.BlockProcess(ctx, bd.SessionID, bd.BlockID)
	switch {
	case errors.Is(err, rex.ErrNotFound):
		// Find a live pane under the record's labels, without ResolveLive's
		// stale-record drop or its save: Observe is a read, and only Start or
		// Stop replaces the record.
		live, ok, err := p.binder.find(ctx, bd.Name, bd, nil)
		if err != nil {
			return "", Binding{}, err
		}
		if !ok {
			return ObserveAbsent, Binding{}, nil
		}
		info, err := p.client.BlockProcess(ctx, live.SessionID, live.BlockID)
		if errors.Is(err, rex.ErrNotFound) {
			return ObserveAbsent, Binding{}, nil
		}
		if err != nil {
			return "", Binding{}, err
		}
		if info.LastExit != nil {
			return ObserveDead, live, nil
		}
		return ObserveRunning, live, nil
	case err != nil:
		return "", Binding{}, err
	case info.LastExit != nil:
		return ObserveDead, bd, nil
	default:
		return ObserveRunning, bd, nil
	}
}

// Inventory snapshots every session the provider tracks for the city. It is a
// read: no meta is written and no session changes. Each entry carries the state
// and identity observe would report for it, so gc need not observe every box.
//
// A box whose liveness cannot be read is listed with no state and the answer
// claims no completeness, because a backend fault proves nothing absent. So
// does a pane under a city label that names a session no binding holds.
//
// ponytail: one Rex block-process call per session, since Rex has no bulk
// liveness query. Move to a bulk query (or a trusted last_exit cache) if a city
// grows past the inventory call's time budget.
func (p *Provider) Inventory(ctx context.Context) (InventoryAnswer, error) {
	lister, ok := p.bindings.(Lister[Binding])
	if !ok {
		return InventoryAnswer{Complete: false, Sessions: []InventoryEntry{}}, nil
	}
	bds := lister.List()
	complete := true
	listed := make(map[SessionName]bool, len(bds))
	sessions := make([]InventoryEntry, 0, len(bds))
	for _, bd := range bds {
		state, holder, err := p.boxState(ctx, bd)
		switch {
		case errors.Is(err, rex.ErrUnavailable):
			complete = false
			// The entry still carries the binding's object; only its state is
			// unknown.
			holder = bd
		case err != nil:
			return InventoryAnswer{}, err
		case state == ObserveAbsent:
			// Rex proved nothing holds the name: omit it, as rule 7 allows.
			continue
		}
		obj := observeObjectFor(holder)
		e := InventoryEntry{
			Name:   string(bd.Name),
			State:  state,
			Object: &obj,
		}
		if holder.BlockID == bd.BlockID {
			e.Identity = p.observeIdentity(bd.Name)
		}
		if state == ObserveDead {
			e.Agent = "dead"
		}
		listed[bd.Name] = true
		sessions = append(sessions, e)
	}
	panes, err := p.cityPanes(ctx)
	if err != nil {
		complete = false
	}
	for name := range panes {
		if !listed[name] {
			complete = false
		}
	}
	return InventoryAnswer{Complete: complete, Sessions: sessions}, nil
}

// cityPanes returns the session names this city's panes name in their Rex
// labels, so Inventory can tell a pane no binding lists from the boxes it
// enumerates. A label without the "<agent> (<name>)" suffix names no session
// and is left to whatever else shares the city session (an operator's shell).
func (p *Provider) cityPanes(ctx context.Context) (map[SessionName]bool, error) {
	sessions, err := p.client.ListSessions(ctx)
	if err != nil {
		return nil, err
	}
	names := map[SessionName]bool{}
	for _, s := range sessions {
		if !p.binder.acceptsLabel(s.Label) {
			continue
		}
		view, err := p.client.SessionView(ctx, s.ID)
		if errors.Is(err, rex.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, w := range view.Windows {
			for _, blk := range w.Blocks {
				if n, ok := paneSessionName(blk.Label); ok {
					names[n] = true
				}
			}
		}
	}
	return names, nil
}

// paneSessionName recovers the session name from a "<agent> (<name>)" pane
// label. A label without that suffix names no session.
func paneSessionName(label string) (SessionName, bool) {
	i := strings.LastIndex(label, " (")
	if i < 0 || !strings.HasSuffix(label, ")") {
		return "", false
	}
	name := strings.TrimSpace(label[i+2 : len(label)-1])
	if name == "" {
		return "", false
	}
	return SessionName(name), true
}

// KillObject re-checks that the exact object still holds the name and, if so,
// kills it. It never kills by name: an object that names no box, or whose id
// or creation time does not match the stored binding, answers `gone`.
//
// The whole check-then-kill runs under the name lock, so a start cannot slip a
// new box under the name between the re-check and the close. Corpse mode kills
// only a box nothing runs in and no client has open, and answers `live`
// otherwise. Zombie mode needs the agent check RPP-OBSERVE-006 does not
// implement yet, so it never kills and answers `changed`.
func (p *Provider) KillObject(ctx context.Context, name string, req KillObjectRequest) (string, error) {
	if strings.TrimSpace(req.Object.ID) == "" || strings.TrimSpace(req.Object.Created) == "" {
		return "", errors.New("kill-object: object id and created are required")
	}
	if req.Mode != "corpse" && req.Mode != "zombie" {
		return "", fmt.Errorf("kill-object: mode %q, want corpse or zombie", req.Mode)
	}
	var res string
	err := p.withNameLock(ctx, SessionName(name), func() error {
		var err error
		res, err = p.killObjectLocked(ctx, name, req)
		return err
	})
	if err != nil {
		return "", err
	}
	return res, nil
}

// killObjectLocked is KillObject's re-check and kill, run under the name lock.
func (p *Provider) killObjectLocked(ctx context.Context, name string, req KillObjectRequest) (string, error) {
	bd, ok := p.Binding(name)
	if !ok {
		return "gone", nil
	}
	obj := observeObjectFor(bd)
	if req.Object.ID != obj.ID || req.Object.Created != obj.Created {
		return "gone", nil
	}
	if req.Mode == "zombie" {
		return "changed", nil
	}
	state, holder, err := p.boxState(ctx, bd)
	if err != nil {
		return "", err
	}
	if state == ObserveRunning && holder.BlockID == bd.BlockID && holder.WindowID == bd.WindowID {
		return "live", nil
	}
	if state != ObserveDead {
		// The recorded block is gone or a new box holds the name: the object
		// names no box, and the exact-binding close has nothing to close.
		return "gone", nil
	}
	if holder.BlockID != bd.BlockID || holder.WindowID != bd.WindowID {
		return "gone", nil
	}
	attached, err := p.attached(ctx, bd)
	if err != nil {
		// An unreadable attachment is not proof nobody is watching.
		return "", err
	}
	if attached {
		return "live", nil
	}
	// Close exactly the object that was re-checked, never by name: a fresh
	// resolve could close a pane that took the name's labels after this one
	// vanished. Same locking as Stop, which this close is: the name lock is
	// already held, so only the city-wide state lock is taken here.
	if err := p.withStateLock(ctx, func() error { return p.stopBinding(ctx, SessionName(name), bd) }); err != nil {
		return "", err
	}
	return "killed", nil
}
