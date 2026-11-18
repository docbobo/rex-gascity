package provider

import (
	"errors"
	"sort"
)

// compositeBindingStore reads bindings from every namespace store in order and
// writes only to the canonical (first) one. Deletes cover every namespace so a
// retired binding cannot be resurrected from a legacy directory.
type compositeBindingStore struct {
	stores []BindingStore
}

func newCompositeBindingStore(stores ...BindingStore) BindingStore {
	return &compositeBindingStore{stores: stores}
}

func (s *compositeBindingStore) Load(name SessionName) (Binding, bool) {
	for _, store := range s.stores {
		if b, ok := store.Load(name); ok {
			return b, true
		}
	}
	return Binding{}, false
}

func (s *compositeBindingStore) Save(b Binding) {
	if len(s.stores) > 0 {
		s.stores[0].Save(b)
	}
}

func (s *compositeBindingStore) Delete(name SessionName) {
	for _, store := range s.stores {
		store.Delete(name)
	}
}

// List implements BindingLister: the union of every namespace, canonical first
// so a primary record stored in more than one place keeps its canonical copy.
func (s *compositeBindingStore) List() []Binding {
	out := []Binding{}
	seen := map[SessionName]bool{}
	for _, store := range s.stores {
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

// compositeActivityStore reads the first activity record found across the
// namespace stores and writes to the canonical one.
type compositeActivityStore struct {
	stores []ActivityStore
}

func newCompositeActivityStore(stores ...ActivityStore) ActivityStore {
	return &compositeActivityStore{stores: stores}
}

func (s *compositeActivityStore) Load(name SessionName) (ActivityRecord, bool) {
	for _, store := range s.stores {
		if rec, ok := store.Load(name); ok {
			return rec, true
		}
	}
	return ActivityRecord{}, false
}

func (s *compositeActivityStore) Save(name SessionName, rec ActivityRecord) {
	if len(s.stores) > 0 {
		s.stores[0].Save(name, rec)
	}
}

// compositeMetaStore reads the first namespace that has the key, writes to the
// canonical namespace, and deletes from every namespace.
type compositeMetaStore struct {
	stores []MetaStore
}

func newCompositeMetaStore(stores ...MetaStore) MetaStore {
	return &compositeMetaStore{stores: stores}
}

func (s *compositeMetaStore) Get(name SessionName, key string) (string, bool, error) {
	for _, store := range s.stores {
		value, ok, err := store.Get(name, key)
		if err != nil {
			return "", false, err
		}
		if ok {
			return value, true, nil
		}
	}
	return "", false, nil
}

func (s *compositeMetaStore) Set(name SessionName, key, value string) error {
	if len(s.stores) == 0 {
		return nil
	}
	return s.stores[0].Set(name, key, value)
}

func (s *compositeMetaStore) Delete(name SessionName, key string) error {
	var errs []error
	for _, store := range s.stores {
		if err := store.Delete(name, key); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
