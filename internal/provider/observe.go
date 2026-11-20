package provider

import (
	"context"
	"errors"
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
// meta sidecar before the launch. It returns nil when the sidecar has none.
func (p *Provider) observeIdentity(n SessionName) map[string]string {
	var id map[string]string
	for _, key := range startIdentityMetaKeys {
		if v, ok, _ := p.metaGet(n, key); ok && v != "" {
			if id == nil {
				id = map[string]string{}
			}
			id[key] = v
		}
	}
	return id
}

// Observe answers `observe <name>`: absent when nothing holds the name,
// running or dead for a recorded box, unknown when Rex cannot be reached. It
// is a read and never mutates the session.
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
	alive, err := p.paneAlive(ctx, bd)
	if err != nil {
		if errors.Is(err, rex.ErrUnavailable) {
			return ObserveAnswer{State: ObserveUnknown}, nil
		}
		return ObserveAnswer{}, err
	}
	obj := observeObjectFor(bd)
	ans := ObserveAnswer{Object: &obj, Identity: p.observeIdentity(bd.Name)}
	if alive {
		ans.State = ObserveRunning
	} else {
		ans.State = ObserveDead
		ans.Agent = "dead"
	}
	return ans, nil
}

// Inventory snapshots every session the provider tracks for the city. It is a
// read: no meta is written and no session changes. A session whose attributes
// cannot be read carries only its name.
func (p *Provider) Inventory(_ context.Context) (InventoryAnswer, error) {
	lister, ok := p.bindings.(Lister[Binding])
	if !ok {
		return InventoryAnswer{Complete: true, Sessions: []InventoryEntry{}}, nil
	}
	bds := lister.List()
	sessions := make([]InventoryEntry, 0, len(bds))
	for _, bd := range bds {
		obj := observeObjectFor(bd)
		sessions = append(sessions, InventoryEntry{
			Name:     string(bd.Name),
			Object:   &obj,
			Identity: p.observeIdentity(bd.Name),
		})
	}
	return InventoryAnswer{Complete: true, Sessions: sessions}, nil
}

// KillObject re-checks that the exact object still holds the name and, if so,
// kills it. It never kills by name: an object that names no box, or whose id
// or creation time does not match the stored binding, answers `gone`.
func (p *Provider) KillObject(ctx context.Context, name string, req KillObjectRequest) (string, error) {
	if strings.TrimSpace(req.Object.ID) == "" || strings.TrimSpace(req.Object.Created) == "" {
		return "", errors.New("kill-object: object id and created are required")
	}
	bd, ok := p.Binding(name)
	if !ok {
		return "gone", nil
	}
	obj := observeObjectFor(bd)
	if req.Object.ID != obj.ID || req.Object.Created != obj.Created {
		return "gone", nil
	}
	alive, err := p.paneAlive(ctx, bd)
	if err != nil {
		if errors.Is(err, rex.ErrUnavailable) {
			return "", err
		}
		return "", err
	}
	if !alive {
		return "gone", nil
	}
	if err := p.Stop(ctx, name); err != nil {
		return "", err
	}
	return "killed", nil
}
