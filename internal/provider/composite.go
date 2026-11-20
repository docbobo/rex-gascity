package provider

import (
	"errors"
	"sort"
)

// compositeStore is the one namespace fan-out behind every composite store:
// the canonical store is first, any others are legacy namespaces. Reads fall
// back in order (first hit wins), writes go to the canonical store only, and
// drops cover every store so a retired record cannot be resurrected from a
// legacy directory.
type compositeStore[K comparable, V any] struct {
	stores []Store[K, V]
}

func newCompositeStore[K comparable, V any](stores ...Store[K, V]) *compositeStore[K, V] {
	return &compositeStore[K, V]{stores: stores}
}

// Read returns the first hit across the stores. A read error stops the
// fallback and is returned, so an unreadable store never hides behind a later
// one.
func (c *compositeStore[K, V]) Read(k K) (V, bool, error) {
	for _, s := range c.stores {
		v, ok, err := s.Read(k)
		if err != nil || ok {
			return v, ok, err
		}
	}
	var zero V
	return zero, false, nil
}

// Write applies v to the canonical store. With no stores it is a no-op.
func (c *compositeStore[K, V]) Write(k K, v V) error {
	if len(c.stores) == 0 {
		return nil
	}
	return c.stores[0].Write(k, v)
}

// Drop applies to every store and joins the errors.
func (c *compositeStore[K, V]) Drop(k K) error {
	var errs []error
	for _, s := range c.stores {
		if err := s.Drop(k); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// compositeBindingStore is the composite binding store. Its List is the union
// of every namespace, canonical first so a primary record stored in more than
// one place keeps its canonical copy.
type compositeBindingStore struct {
	*compositeStore[SessionName, Binding]
}

func newCompositeBindingStore(stores ...Store[SessionName, Binding]) *compositeBindingStore {
	return &compositeBindingStore{newCompositeStore(stores...)}
}

// List implements Lister: the union of every namespace, canonical first.
func (c *compositeBindingStore) List() []Binding {
	out := []Binding{}
	seen := map[SessionName]bool{}
	for _, store := range c.stores {
		lister, ok := store.(Lister[Binding])
		if !ok {
			continue
		}
		for _, b := range lister.List() {
			if seen[b.Name] {
				continue
			}
			seen[b.Name] = true
			out = append(out, b)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// compositeMetaStore is the composite metadata store. Clear sweeps every
// namespace that can clear.
type compositeMetaStore struct {
	*compositeStore[metaKey, string]
}

func newCompositeMetaStore(stores ...Store[metaKey, string]) *compositeMetaStore {
	return &compositeMetaStore{newCompositeStore(stores...)}
}

// newCompositeActivityStore is the composite activity store.
func newCompositeActivityStore(stores ...Store[SessionName, ActivityRecord]) *compositeStore[SessionName, ActivityRecord] {
	return newCompositeStore(stores...)
}

// Snapshot merges every namespace that can snapshot, canonical first so the
// first hit wins, as Read does.
func (c *compositeMetaStore) Snapshot(name SessionName) map[string]string {
	out := map[string]string{}
	for _, s := range c.stores {
		snap, ok := s.(snapshotter)
		if !ok {
			continue
		}
		for k, v := range snap.Snapshot(name) {
			if _, seen := out[k]; !seen {
				out[k] = v
			}
		}
	}
	return out
}

// Clear removes every key for name from every namespace.
func (c *compositeMetaStore) Clear(name SessionName) error {
	var errs []error
	for _, s := range c.stores {
		if cl, ok := s.(clearer); ok {
			if err := cl.Clear(name); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}
