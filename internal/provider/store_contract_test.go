package provider

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

// bindingAdapters returns a fresh BindingStore (and its lister) per adapter.
func bindingAdapters(t *testing.T) map[string]func() BindingStore {
	return map[string]func() BindingStore{
		"memory": func() BindingStore { return NewMemoryStore() },
		"file":   func() BindingStore { return NewFileStore(t.TempDir()) },
	}
}

func activityAdapters(t *testing.T) map[string]func() ActivityStore {
	return map[string]func() ActivityStore{
		"memory": NewMemoryActivityStore,
		"file":   func() ActivityStore { return NewFileActivityStore(t.TempDir()) },
	}
}

func metaAdapters(t *testing.T) map[string]func() MetaStore {
	return map[string]func() MetaStore{
		"memory": NewMemoryMetaStore,
		"file":   func() MetaStore { return NewFileMetaStore(t.TempDir()) },
	}
}

// TestBindingStoreContract runs the BindingStore and BindingLister contract
// against the memory and file adapters.
func TestBindingStoreContract(t *testing.T) {
	for kind, mk := range bindingAdapters(t) {
		t.Run(kind, func(t *testing.T) {
			s := mk()
			if _, ok := s.Load("a"); ok {
				t.Fatal("Load before Save found a binding")
			}
			a, b := fullBinding("a"), fullBinding("b")
			s.Save(b)
			s.Save(a)
			if got, ok := s.Load("a"); !ok || !reflect.DeepEqual(got, a) {
				t.Fatalf("Load a = %+v, %v; want %+v", got, ok, a)
			}
			a2 := a
			a2.BlockID = "block:replaced"
			s.Save(a2)
			if got, _ := s.Load("a"); got.BlockID != "block:replaced" {
				t.Errorf("Save did not replace: %+v", got)
			}
			lister, ok := s.(BindingLister)
			if !ok {
				t.Fatal("store is not a BindingLister")
			}
			list := lister.List()
			if len(list) != 2 || list[0].Name != "a" || list[1].Name != "b" {
				t.Errorf("List = %+v; want a, b sorted", list)
			}
			s.Delete("a")
			s.Delete("missing")
			if _, ok := s.Load("a"); ok {
				t.Error("Delete left the binding")
			}
			if got := lister.List(); len(got) != 1 || got[0].Name != "b" {
				t.Errorf("List after Delete = %+v", got)
			}
		})
	}
}

// TestActivityStoreContract runs the ActivityStore contract against both adapters.
func TestActivityStoreContract(t *testing.T) {
	rec := ActivityRecord{Identity: "block:1", Fingerprint: "fp", At: time.Unix(1, 0).UTC(), InputAt: time.Unix(2, 0).UTC()}
	for kind, mk := range activityAdapters(t) {
		t.Run(kind, func(t *testing.T) {
			s := mk()
			if _, ok := s.Load("a"); ok {
				t.Fatal("Load before Save found a record")
			}
			s.Save("a", rec)
			if got, ok := s.Load("a"); !ok || !reflect.DeepEqual(got, rec) {
				t.Fatalf("Load = %+v, %v; want %+v", got, ok, rec)
			}
			s.Save("a", ActivityRecord{Fingerprint: "newer"})
			if got, _ := s.Load("a"); got.Fingerprint != "newer" || got.Identity != "" {
				t.Errorf("Save did not replace: %+v", got)
			}
			s.Save("b", rec)
			if err := s.Delete("a"); err != nil {
				t.Fatalf("Delete: %v", err)
			}
			if err := s.Delete("a"); err != nil {
				t.Errorf("Delete of a missing record: %v", err)
			}
			if _, ok := s.Load("a"); ok {
				t.Error("Delete left the record")
			}
			if _, ok := s.Load("b"); !ok {
				t.Error("Delete removed another name's record")
			}
		})
	}
}

// TestMetaStoreContract runs the MetaStore contract (including Clear) against
// both adapters.
func TestMetaStoreContract(t *testing.T) {
	for kind, mk := range metaAdapters(t) {
		t.Run(kind, func(t *testing.T) {
			s := mk()
			for _, kv := range [][3]string{{"a", "k1", "v1"}, {"a", "k2", "v2"}, {"b", "k1", "b1"}} {
				if err := s.Set(SessionName(kv[0]), kv[1], kv[2]); err != nil {
					t.Fatalf("Set: %v", err)
				}
			}
			if v, ok, err := s.Get("a", "k1"); !ok || err != nil || v != "v1" {
				t.Fatalf("Get = %q, %v, %v", v, ok, err)
			}
			if err := s.Delete("a", "k1"); err != nil {
				t.Fatalf("Delete: %v", err)
			}
			if _, ok, _ := s.Get("a", "k1"); ok {
				t.Error("Delete left the key")
			}
			if v, ok, _ := s.Get("a", "k2"); !ok || v != "v2" {
				t.Error("Delete removed another key")
			}
			if err := s.Clear("a"); err != nil {
				t.Fatalf("Clear: %v", err)
			}
			if err := s.Clear("a"); err != nil {
				t.Errorf("Clear of an empty name: %v", err)
			}
			if _, ok, _ := s.Get("a", "k2"); ok {
				t.Error("Clear left a key")
			}
			if v, ok, _ := s.Get("b", "k1"); !ok || v != "b1" {
				t.Error("Clear removed another name's key")
			}
		})
	}
}

// TestFileStoreOnDiskBytes pins the exact JSON each file store writes, so
// records written by older builds keep loading and new ones stay readable by
// them.
func TestFileStoreOnDiskBytes(t *testing.T) {
	read := func(path string) string {
		t.Helper()
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}

	dir := t.TempDir()
	b := fullBinding("n")
	b.GCBin, b.WorkDir, b.ClaimSessionID = "gc", "/w", "c1"
	NewFileStore(dir).Save(b)
	const wantBinding = `{"gc_name":"n","rex_session_id":"session:acme","rex_session_label":"gc:acme","window_id":"window:Indigo","block_id":"block:dog-1","tab_label":"Indigo","pane_label":"dog-1","aliases":["Indigo--dog","Indigo--dog-1"],"launch_mode":"resume","created_at":"2026-11-17T19:00:00Z","gc_bin":"gc","work_dir":"/w","claim_session_id":"c1"}`
	if got := read(filepath.Join(dir, nameKey("n")+".json")); got != wantBinding {
		t.Errorf("binding record:\n got %s\nwant %s", got, wantBinding)
	}

	dir = t.TempDir()
	NewFileStore(dir).Save(Binding{Name: "n"})
	const wantMinimal = `{"gc_name":"n","rex_session_id":"","rex_session_label":"","window_id":"","block_id":"","launch_mode":"","created_at":"0001-01-01T00:00:00Z"}`
	if got := read(filepath.Join(dir, nameKey("n")+".json")); got != wantMinimal {
		t.Errorf("minimal binding record:\n got %s\nwant %s", got, wantMinimal)
	}

	dir = t.TempDir()
	NewFileActivityStore(dir).Save("n", ActivityRecord{Identity: "block:1", Fingerprint: "fp", At: time.Unix(1, 0).UTC(), InputAt: time.Unix(2, 0).UTC()})
	const wantActivity = `{"identity":"block:1","fingerprint":"fp","updated_at":"1970-01-01T00:00:01Z","input_updated_at":"1970-01-01T00:00:02Z"}`
	if got := read(filepath.Join(dir, nameKey("n")+".json")); got != wantActivity {
		t.Errorf("activity record:\n got %s\nwant %s", got, wantActivity)
	}

	dir = t.TempDir()
	if err := NewFileMetaStore(dir).Set("n", "k", "v"); err != nil {
		t.Fatal(err)
	}
	files, _ := filepath.Glob(filepath.Join(dir, nameKey("n"), "*.json"))
	if len(files) != 1 {
		t.Fatalf("meta files = %v; want one under dir/<name hash>/", files)
	}
	const wantMeta = `{"key":"k","value_b64":"dg=="}`
	if got := read(files[0]); got != wantMeta {
		t.Errorf("meta record:\n got %s\nwant %s", got, wantMeta)
	}
}

// failingActivity and failingMeta fail every mutation, to check that the
// composites join the errors of every namespace.
type failingActivity struct{ err error }

func (failingActivity) Load(SessionName) (ActivityRecord, bool) { return ActivityRecord{}, false }
func (failingActivity) Save(SessionName, ActivityRecord)        {}
func (f failingActivity) Delete(SessionName) error              { return f.err }

type failingMeta struct{ err error }

func (f failingMeta) Get(SessionName, string) (string, bool, error) { return "", false, f.err }
func (f failingMeta) Set(SessionName, string, string) error         { return f.err }
func (f failingMeta) Delete(SessionName, string) error              { return f.err }
func (f failingMeta) Clear(SessionName) error                       { return f.err }

// TestCompositeBindingStoreContract: first store wins on reads, writes go to
// the canonical store only, deletes cover every store, and List is the union
// with the canonical copy kept, sorted by name.
func TestCompositeBindingStoreContract(t *testing.T) {
	canon, legacy := NewMemoryStore(), NewMemoryStore()
	c := newCompositeBindingStore(canon, legacy)

	fromLegacy := fullBinding("only-legacy")
	legacy.Save(fromLegacy)
	if got, ok := c.Load("only-legacy"); !ok || !reflect.DeepEqual(got, fromLegacy) {
		t.Fatalf("fallback Load = %+v, %v", got, ok)
	}

	oldCopy, newCopy := fullBinding("both"), fullBinding("both")
	oldCopy.BlockID, newCopy.BlockID = "block:legacy", "block:canonical"
	legacy.Save(oldCopy)
	canon.Save(newCopy)
	if got, _ := c.Load("both"); got.BlockID != "block:canonical" {
		t.Errorf("Load = %s; canonical store must win", got.BlockID)
	}

	c.Save(fullBinding("fresh"))
	if _, ok := canon.Load("fresh"); !ok {
		t.Error("Save did not reach the canonical store")
	}
	if _, ok := legacy.Load("fresh"); ok {
		t.Error("Save reached the legacy store")
	}

	list := c.(BindingLister).List()
	var names []SessionName
	for _, b := range list {
		names = append(names, b.Name)
	}
	if want := []SessionName{"both", "fresh", "only-legacy"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("List names = %v; want %v", names, want)
	}
	if list[0].BlockID != "block:canonical" {
		t.Errorf("List kept %s for a duplicated name; want the canonical copy", list[0].BlockID)
	}

	c.Delete("both")
	if _, ok := canon.Load("both"); ok {
		t.Error("Delete left the canonical copy")
	}
	if _, ok := legacy.Load("both"); ok {
		t.Error("Delete left the legacy copy")
	}

	if _, ok := newCompositeBindingStore().Load("x"); ok {
		t.Error("empty composite found a binding")
	}
	newCompositeBindingStore().Save(fullBinding("x")) // must not panic
	if got := newCompositeBindingStore().(BindingLister).List(); got == nil || len(got) != 0 {
		t.Errorf("empty List = %#v; want empty non-nil", got)
	}
	// A store that cannot list is skipped.
	if got := newCompositeBindingStore(bindingOnly{}, canon).(BindingLister).List(); len(got) != 1 {
		t.Errorf("List with a non-lister = %d entries; want 1", len(got))
	}
}

type bindingOnly struct{}

func (bindingOnly) Load(SessionName) (Binding, bool) { return Binding{}, false }
func (bindingOnly) Save(Binding)                     {}
func (bindingOnly) Delete(SessionName)               {}

func TestCompositeActivityStoreContract(t *testing.T) {
	canon, legacy := NewMemoryActivityStore(), NewMemoryActivityStore()
	c := newCompositeActivityStore(canon, legacy)

	legacy.Save("a", ActivityRecord{Fingerprint: "legacy"})
	if got, ok := c.Load("a"); !ok || got.Fingerprint != "legacy" {
		t.Fatalf("fallback Load = %+v, %v", got, ok)
	}
	canon.Save("a", ActivityRecord{Fingerprint: "canon"})
	if got, _ := c.Load("a"); got.Fingerprint != "canon" {
		t.Errorf("Load = %q; canonical must win", got.Fingerprint)
	}
	c.Save("b", ActivityRecord{Fingerprint: "b"})
	if _, ok := canon.Load("b"); !ok {
		t.Error("Save did not reach the canonical store")
	}
	if _, ok := legacy.Load("b"); ok {
		t.Error("Save reached the legacy store")
	}
	if err := c.Delete("a"); err != nil {
		t.Fatal(err)
	}
	if _, ok := canon.Load("a"); ok {
		t.Error("Delete left the canonical record")
	}
	if _, ok := legacy.Load("a"); ok {
		t.Error("Delete left the legacy record")
	}

	e1, e2 := errors.New("e1"), errors.New("e2")
	err := newCompositeActivityStore(failingActivity{e1}, canon, failingActivity{e2}).Delete("x")
	if !errors.Is(err, e1) || !errors.Is(err, e2) {
		t.Errorf("Delete error = %v; want both errors joined", err)
	}
	newCompositeActivityStore().Save("x", ActivityRecord{}) // must not panic
}

func TestCompositeMetaStoreContract(t *testing.T) {
	canon, legacy := NewMemoryMetaStore(), NewMemoryMetaStore()
	c := newCompositeMetaStore(canon, legacy)

	_ = legacy.Set("a", "k", "legacy")
	if v, ok, err := c.Get("a", "k"); !ok || err != nil || v != "legacy" {
		t.Fatalf("fallback Get = %q, %v, %v", v, ok, err)
	}
	_ = canon.Set("a", "k", "canon")
	if v, _, _ := c.Get("a", "k"); v != "canon" {
		t.Errorf("Get = %q; canonical must win", v)
	}
	if _, ok, err := c.Get("a", "missing"); ok || err != nil {
		t.Errorf("missing Get = %v, %v", ok, err)
	}
	if err := c.Set("a", "new", "v"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := canon.Get("a", "new"); !ok {
		t.Error("Set did not reach the canonical store")
	}
	if _, ok, _ := legacy.Get("a", "new"); ok {
		t.Error("Set reached the legacy store")
	}
	if err := c.Delete("a", "k"); err != nil {
		t.Fatal(err)
	}
	for name, s := range map[string]MetaStore{"canonical": canon, "legacy": legacy} {
		if _, ok, _ := s.Get("a", "k"); ok {
			t.Errorf("Delete left the key in the %s store", name)
		}
	}
	_ = legacy.Set("a", "old", "v")
	if err := c.Clear("a"); err != nil {
		t.Fatal(err)
	}
	for name, s := range map[string]MetaStore{"canonical": canon, "legacy": legacy} {
		if _, ok, _ := s.Get("a", "new"); ok {
			t.Errorf("Clear left a key in the %s store", name)
		}
		if _, ok, _ := s.Get("a", "old"); ok {
			t.Errorf("Clear left a key in the %s store", name)
		}
	}

	e1, e2 := errors.New("e1"), errors.New("e2")
	for op, fn := range map[string]func(MetaStore) error{
		"Delete": func(s MetaStore) error { return s.Delete("x", "k") },
		"Clear":  func(s MetaStore) error { return s.Clear("x") },
	} {
		err := fn(newCompositeMetaStore(failingMeta{e1}, canon, failingMeta{e2}))
		if !errors.Is(err, e1) || !errors.Is(err, e2) {
			t.Errorf("%s error = %v; want both errors joined", op, err)
		}
	}
	// A read error stops the fallback and is returned.
	if _, ok, err := newCompositeMetaStore(failingMeta{e1}, canon).Get("a", "new"); ok || !errors.Is(err, e1) {
		t.Errorf("Get = %v, %v; want the first store's error", ok, err)
	}
	if err := newCompositeMetaStore().Set("x", "k", "v"); err != nil {
		t.Errorf("empty Set = %v; want nil", err)
	}
}
