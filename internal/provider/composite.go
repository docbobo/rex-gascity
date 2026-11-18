package provider

import (
	"errors"
	"sort"
)

// composite is the namespace fan-out shared by every store: the canonical
// store is first, any others are legacy namespaces. Reads fall back in order
// (first hit wins), writes go to the canonical store only, and deletes cover
// every store so a retired record cannot be resurrected from a legacy
// directory.
type composite[S any] struct {
	stores []S
}

// readFirst returns the first hit across the stores. A read error stops the
// fallback and is returned, so an unreadable store never hides behind a later
// one.
func readFirst[S, T any](c composite[S], read func(S) (T, bool, error)) (T, bool, error) {
	for _, s := range c.stores {
		v, ok, err := read(s)
		if err != nil || ok {
			return v, ok, err
		}
	}
	var zero T
	return zero, false, nil
}

// writeCanonical applies write to the canonical store. With no stores it is a
// no-op.
func (c composite[S]) writeCanonical(write func(S) error) error {
	if len(c.stores) == 0 {
		return nil
	}
	return write(c.stores[0])
}

// deleteAll applies del to every store and joins the errors.
func (c composite[S]) deleteAll(del func(S) error) error {
	var errs []error
	for _, s := range c.stores {
		if err := del(s); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// compositeBindingStore is the composite BindingStore.
type compositeBindingStore struct{ composite[BindingStore] }

func newCompositeBindingStore(stores ...BindingStore) BindingStore {
	return &compositeBindingStore{composite[BindingStore]{stores}}
}

func (c *compositeBindingStore) Load(name SessionName) (Binding, bool) {
	b, ok, _ := readFirst(c.composite, func(s BindingStore) (Binding, bool, error) {
		b, ok := s.Load(name)
		return b, ok, nil
	})
	return b, ok
}

func (c *compositeBindingStore) Save(b Binding) {
	_ = c.writeCanonical(func(s BindingStore) error { s.Save(b); return nil })
}

func (c *compositeBindingStore) Delete(name SessionName) {
	_ = c.deleteAll(func(s BindingStore) error { s.Delete(name); return nil })
}

// List implements BindingLister: the union of every namespace, canonical first
// so a primary record stored in more than one place keeps its canonical copy.
func (c *compositeBindingStore) List() []Binding {
	out := []Binding{}
	seen := map[SessionName]bool{}
	for _, store := range c.stores {
		lister, ok := store.(BindingLister)
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

// compositeActivityStore is the composite ActivityStore.
type compositeActivityStore struct{ composite[ActivityStore] }

func newCompositeActivityStore(stores ...ActivityStore) ActivityStore {
	return &compositeActivityStore{composite[ActivityStore]{stores}}
}

func (c *compositeActivityStore) Load(name SessionName) (ActivityRecord, bool) {
	rec, ok, _ := readFirst(c.composite, func(s ActivityStore) (ActivityRecord, bool, error) {
		rec, ok := s.Load(name)
		return rec, ok, nil
	})
	return rec, ok
}

func (c *compositeActivityStore) Save(name SessionName, rec ActivityRecord) {
	_ = c.writeCanonical(func(s ActivityStore) error { s.Save(name, rec); return nil })
}

func (c *compositeActivityStore) Delete(name SessionName) error {
	return c.deleteAll(func(s ActivityStore) error { return s.Delete(name) })
}

// compositeMetaStore is the composite MetaStore.
type compositeMetaStore struct{ composite[MetaStore] }

func newCompositeMetaStore(stores ...MetaStore) MetaStore {
	return &compositeMetaStore{composite[MetaStore]{stores}}
}

func (c *compositeMetaStore) Get(name SessionName, key string) (string, bool, error) {
	return readFirst(c.composite, func(s MetaStore) (string, bool, error) { return s.Get(name, key) })
}

func (c *compositeMetaStore) Set(name SessionName, key, value string) error {
	return c.writeCanonical(func(s MetaStore) error { return s.Set(name, key, value) })
}

func (c *compositeMetaStore) Delete(name SessionName, key string) error {
	return c.deleteAll(func(s MetaStore) error { return s.Delete(name, key) })
}

// Clear removes every key for name from every namespace.
func (c *compositeMetaStore) Clear(name SessionName) error {
	return c.deleteAll(func(s MetaStore) error { return s.Clear(name) })
}
