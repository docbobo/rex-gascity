package provider

import (
	"context"
	"sync"

	"github.com/docbobo/rex-gas-city/internal/rex"
)

// Binding ties a gc session name to its Rex address.
type Binding struct {
	Name         SessionName
	SessionID    rex.SessionID
	SessionLabel string
	WindowID     rex.WindowID
	BlockID      rex.BlockID
}

// BindingStore persists bindings. The in-memory store is the only
// implementation today; a file sidecar can replace it later.
type BindingStore interface {
	// Load returns the binding for name, if any.
	Load(name SessionName) (Binding, bool)
	// Save stores b, replacing any binding with the same name.
	Save(b Binding)
	// Delete removes the binding for name.
	Delete(name SessionName)
}

// MemoryStore is a BindingStore held in memory. It is safe for concurrent use.
type MemoryStore struct {
	mu sync.RWMutex
	m  map[SessionName]Binding
}

// NewMemoryStore returns an empty MemoryStore.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{m: make(map[SessionName]Binding)}
}

// Load implements BindingStore.
func (s *MemoryStore) Load(name SessionName) (Binding, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	b, ok := s.m[name]
	return b, ok
}

// Save implements BindingStore.
func (s *MemoryStore) Save(b Binding) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[b.Name] = b
}

// Delete implements BindingStore.
func (s *MemoryStore) Delete(name SessionName) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, name)
}

// Binder resolves gc session names to Rex addresses, using a store as a
// cache and the Rex session view as the source of truth.
type Binder struct {
	client       rex.Client
	store        BindingStore
	sessionLabel string
}

// NewBinder builds a Binder that looks for the Rex session labelled
// sessionLabel and caches bindings in store.
func NewBinder(client rex.Client, store BindingStore, sessionLabel string) *Binder {
	return &Binder{client: client, store: store, sessionLabel: sessionLabel}
}

// FindSession returns the city's Rex session. The bool is false when it does
// not exist.
func (b *Binder) FindSession(ctx context.Context) (rex.SessionID, bool, error) {
	sessions, err := b.client.ListSessions(ctx)
	if err != nil {
		return "", false, err
	}
	for _, s := range sessions {
		if s.Label == b.sessionLabel {
			return s.ID, true, nil
		}
	}
	return "", false, nil
}

// Resolve returns the binding for name. A stored binding is checked against
// the session view; if it is gone it is deleted and the session is scanned by
// label. A view error is returned as is and leaves the stored binding alone.
// A name with no pane is reported as false with a nil error.
func (b *Binder) Resolve(ctx context.Context, name SessionName) (Binding, bool, error) {
	if stored, ok := b.store.Load(name); ok {
		view, err := b.client.SessionView(ctx, stored.SessionID)
		if err != nil {
			return Binding{}, false, err
		}
		if stillBound(view, stored, name) {
			return stored, true, nil
		}
		b.store.Delete(name)
	}
	return b.scan(ctx, name)
}

// stillBound reports whether view still holds the binding's block under the
// labels the name implies.
func stillBound(view rex.SessionView, bd Binding, name SessionName) bool {
	if view.Label != bd.SessionLabel {
		return false
	}
	for _, w := range view.Windows {
		if w.ID != bd.WindowID || w.Label != name.TabLabel() {
			continue
		}
		for _, blk := range w.Blocks {
			if blk.ID == bd.BlockID && blk.Label == name.PaneLabel() {
				return true
			}
		}
	}
	return false
}

// scan finds name by session label, tab label and pane label, and saves it.
func (b *Binder) scan(ctx context.Context, name SessionName) (Binding, bool, error) {
	sid, ok, err := b.FindSession(ctx)
	if err != nil || !ok {
		return Binding{}, false, err
	}
	view, err := b.client.SessionView(ctx, sid)
	if err != nil {
		return Binding{}, false, err
	}
	for _, w := range view.Windows {
		if w.Label != name.TabLabel() {
			continue
		}
		for _, blk := range w.Blocks {
			if blk.Label == name.PaneLabel() {
				bd := Binding{
					Name:         name,
					SessionID:    sid,
					SessionLabel: b.sessionLabel,
					WindowID:     w.ID,
					BlockID:      blk.ID,
				}
				b.store.Save(bd)
				return bd, true, nil
			}
		}
	}
	return Binding{}, false, nil
}

// Record stores binding, replacing any earlier one for the same name.
func (b *Binder) Record(binding Binding) { b.store.Save(binding) }

// Forget drops the stored binding for name.
func (b *Binder) Forget(name SessionName) { b.store.Delete(name) }
