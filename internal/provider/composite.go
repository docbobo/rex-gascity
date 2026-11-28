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

// Mutate applies fn to the canonical store. With no stores it is a no-op.
func (c *compositeStore[K, V]) Mutate(k K, fn func(V) V) error {
	if len(c.stores) == 0 {
		return nil
	}
	if ms, ok := c.stores[0].(interface{ Mutate(K, func(V) V) error }); ok {
		return ms.Mutate(k, fn)
	}
	cur, _, err := c.stores[0].Read(k)
	if err != nil {
		return err
	}
	return c.stores[0].Write(k, fn(cur))
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
// one place keeps its canonical copy. Every namespace must be able to list: a
// store that cannot is not installable.
type compositeBindingStore struct {
	*compositeStore[SessionName, Binding]
	records []bindingRecords
}

func newCompositeBindingStore(records ...bindingRecords) *compositeBindingStore {
	stores := make([]Store[SessionName, Binding], 0, len(records))
	for _, r := range records {
		stores = append(stores, r)
	}
	return &compositeBindingStore{
		compositeStore: newCompositeStore(stores...),
		records:        records,
	}
}

// List returns the union of every namespace, canonical first and name-sorted.
// It returns the healthy records it read plus an error joining every
// namespace's list error, so a caller never mistakes a truncated union for the
// whole set.
func (c *compositeBindingStore) List() ([]Binding, error) {
	out := []Binding{}
	seen := map[SessionName]bool{}
	var errs []error
	for _, records := range c.records {
		list, err := records.List()
		if err != nil {
			errs = append(errs, err)
		}
		for _, b := range list {
			if seen[b.Name] {
				continue
			}
			seen[b.Name] = true
			out = append(out, b)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, errors.Join(errs...)
}

// compositeMetaStore is the composite metadata store. Clear sweeps every
// namespace, joining their errors.
type compositeMetaStore struct {
	*compositeStore[metaKey, string]
	metas []MetaStore
}

func newCompositeMetaStore(metas ...MetaStore) *compositeMetaStore {
	stores := make([]Store[metaKey, string], 0, len(metas))
	for _, m := range metas {
		stores = append(stores, m)
	}
	return &compositeMetaStore{
		compositeStore: newCompositeStore(stores...),
		metas:          metas,
	}
}

// newCompositeActivityStore is the composite activity store.
func newCompositeActivityStore(stores ...Store[SessionName, ActivityRecord]) *compositeStore[SessionName, ActivityRecord] {
	return newCompositeStore(stores...)
}

// Clear removes every key for name from every namespace, joining the errors.
func (c *compositeMetaStore) Clear(name SessionName) error {
	var errs []error
	for _, meta := range c.metas {
		if err := meta.Clear(name); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
