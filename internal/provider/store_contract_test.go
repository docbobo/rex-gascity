package provider

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

// tableAdapters returns a fresh BindingTable and the metadata store behind it
// per adapter, so the one table contract runs against the memory, file and
// composite shapes.
func tableAdapters(t *testing.T) map[string]func() (BindingTable, MetaStore) {
	return map[string]func() (BindingTable, MetaStore){
		"memory": func() (BindingTable, MetaStore) {
			meta := NewMemoryMetaStore()
			return newBindingTable(NewMemoryStore(), meta), meta
		},
		"file": func() (BindingTable, MetaStore) {
			meta := NewFileMetaStore(t.TempDir())
			return newBindingTable(NewFileStore(t.TempDir()), meta), meta
		},
		"composite": func() (BindingTable, MetaStore) {
			meta := newCompositeMetaStore(NewMemoryMetaStore(), NewFileMetaStore(t.TempDir()))
			return newBindingTable(newCompositeBindingStore(NewMemoryStore(), NewFileStore(t.TempDir())), meta), meta
		},
	}
}

func activityAdapters(t *testing.T) map[string]func() Store[SessionName, ActivityRecord] {
	return map[string]func() Store[SessionName, ActivityRecord]{
		"memory": func() Store[SessionName, ActivityRecord] { return NewMemoryActivityStore() },
		"file": func() Store[SessionName, ActivityRecord] {
			return NewFileActivityStore(t.TempDir())
		},
	}
}

func metaAdapters(t *testing.T) map[string]func() MetaStore {
	return map[string]func() MetaStore{
		"memory": func() MetaStore { return NewMemoryMetaStore() },
		"file":   func() MetaStore { return NewFileMetaStore(t.TempDir()) },
	}
}

// TestBindingTableContract runs the one binding-table contract against the
// memory, file and composite adapters: Read, Write, Drop, List, alias Lookup,
// duplicate handling and the meta sweep.
func TestBindingTableContract(t *testing.T) {
	for kind, mk := range tableAdapters(t) {
		t.Run(kind, func(t *testing.T) {
			s, meta := mk()
			if _, ok, _ := s.Read("a"); ok {
				t.Fatal("Read before Write found a binding")
			}
			a, b := fullBinding("a"), fullBinding("b")
			if err := s.Write(b.Name, b); err != nil {
				t.Fatal(err)
			}
			if err := s.Write(a.Name, a); err != nil {
				t.Fatal(err)
			}
			if got, ok, _ := s.Read("a"); !ok || !reflect.DeepEqual(got, a) {
				t.Fatalf("Read a = %+v, %v; want %+v", got, ok, a)
			}
			a2 := a
			a2.BlockID = "block:replaced"
			_ = s.Write(a2.Name, a2)
			if got, _, _ := s.Read("a"); got.BlockID != "block:replaced" {
				t.Errorf("Write did not replace: %+v", got)
			}
			list, err := s.List()
			if err != nil {
				t.Fatalf("List: %v", err)
			}
			if len(list) != 2 || list[0].Name != "a" || list[1].Name != "b" {
				t.Errorf("List = %+v; want a, b sorted", list)
			}
			_ = s.Drop("a")
			_ = s.Drop("missing")
			if _, ok, _ := s.Read("a"); ok {
				t.Error("Drop left the binding")
			}
			if got, _ := s.List(); len(got) != 1 || got[0].Name != "b" {
				t.Errorf("List after Drop = %+v", got)
			}

			// A record answers to its alias through the table alone, and a
			// duplicate write replaces instead of adding a second record.
			aliased := fullBinding("c")
			aliased.Aliases = []SessionName{"c-alias"}
			if err := s.Write(aliased.Name, aliased); err != nil {
				t.Fatalf("Write aliased: %v", err)
			}
			got, err := s.Lookup("c-alias")
			if err != nil {
				t.Fatalf("Lookup alias: %v", err)
			}
			if len(got) != 1 || got[0].Name != "c" {
				t.Fatalf("Lookup(c-alias) = %+v; want the record named c", got)
			}
			if err := s.Write(aliased.Name, aliased); err != nil {
				t.Fatalf("rewrite c: %v", err)
			}
			list, err = s.List()
			if err != nil {
				t.Fatalf("List: %v", err)
			}
			seen := map[SessionName]int{}
			for _, rec := range list {
				seen[rec.Name]++
			}
			if seen["c"] != 1 {
				t.Errorf("List holds %d records named c; want 1", seen["c"])
			}

			// The sweep removes every key of one name and no key of another.
			for _, kv := range [][3]string{{"c", "k1", "v1"}, {"c", "k2", "v2"}, {"d", "k1", "d1"}} {
				if err := meta.Write(metaKey{SessionName(kv[0]), kv[1]}, kv[2]); err != nil {
					t.Fatalf("meta Write: %v", err)
				}
			}
			if err := s.ClearMeta("c"); err != nil {
				t.Fatalf("ClearMeta: %v", err)
			}
			if _, ok, _ := meta.Read(metaKey{"c", "k1"}); ok {
				t.Error("ClearMeta left a key")
			}
			if v, ok, _ := meta.Read(metaKey{"d", "k1"}); !ok || v != "d1" {
				t.Error("ClearMeta removed another name's key")
			}
		})
	}
}

// TestActivityStoreContract runs the Store contract against both adapters.
func TestActivityStoreContract(t *testing.T) {
	rec := ActivityRecord{Identity: "block:1", Fingerprint: "fp", At: time.Unix(1, 0).UTC(), InputAt: time.Unix(2, 0).UTC()}
	for kind, mk := range activityAdapters(t) {
		t.Run(kind, func(t *testing.T) {
			s := mk()
			if _, ok, _ := s.Read("a"); ok {
				t.Fatal("Read before Write found a record")
			}
			_ = s.Write("a", rec)
			if got, ok, _ := s.Read("a"); !ok || !reflect.DeepEqual(got, rec) {
				t.Fatalf("Read = %+v, %v; want %+v", got, ok, rec)
			}
			_ = s.Write("a", ActivityRecord{Fingerprint: "newer"})
			if got, _, _ := s.Read("a"); got.Fingerprint != "newer" || got.Identity != "" {
				t.Errorf("Write did not replace: %+v", got)
			}
			_ = s.Write("b", rec)
			if err := s.Drop("a"); err != nil {
				t.Fatalf("Drop: %v", err)
			}
			if err := s.Drop("a"); err != nil {
				t.Errorf("Drop of a missing record: %v", err)
			}
			if _, ok, _ := s.Read("a"); ok {
				t.Error("Drop left the record")
			}
			if _, ok, _ := s.Read("b"); !ok {
				t.Error("Drop removed another name's record")
			}
		})
	}
}

// TestMetaStoreContract runs the Store contract (including Clear) against both
// adapters.
func TestMetaStoreContract(t *testing.T) {
	for kind, mk := range metaAdapters(t) {
		t.Run(kind, func(t *testing.T) {
			s := mk()
			for _, kv := range [][3]string{{"a", "k1", "v1"}, {"a", "k2", "v2"}, {"b", "k1", "b1"}} {
				if err := s.Write(metaKey{SessionName(kv[0]), kv[1]}, kv[2]); err != nil {
					t.Fatalf("Write: %v", err)
				}
			}
			if v, ok, err := s.Read(metaKey{"a", "k1"}); !ok || err != nil || v != "v1" {
				t.Fatalf("Read = %q, %v, %v", v, ok, err)
			}
			if err := s.Drop(metaKey{"a", "k1"}); err != nil {
				t.Fatalf("Drop: %v", err)
			}
			if _, ok, _ := s.Read(metaKey{"a", "k1"}); ok {
				t.Error("Drop left the key")
			}
			if v, ok, _ := s.Read(metaKey{"a", "k2"}); !ok || v != "v2" {
				t.Error("Drop removed another key")
			}
			if err := s.Clear("a"); err != nil {
				t.Fatalf("Clear: %v", err)
			}
			if err := s.Clear("a"); err != nil {
				t.Errorf("Clear of an empty name: %v", err)
			}
			if _, ok, _ := s.Read(metaKey{"a", "k2"}); ok {
				t.Error("Clear left a key")
			}
			if v, ok, _ := s.Read(metaKey{"b", "k1"}); !ok || v != "b1" {
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

func (f failingActivity) Read(SessionName) (ActivityRecord, bool, error) {
	return ActivityRecord{}, false, f.err
}
func (f failingActivity) Write(SessionName, ActivityRecord) error { return f.err }
func (f failingActivity) Drop(SessionName) error                  { return f.err }

type failingMeta struct{ err error }

func (f failingMeta) Read(metaKey) (string, bool, error) { return "", false, f.err }
func (f failingMeta) Write(metaKey, string) error        { return f.err }
func (f failingMeta) Drop(metaKey) error                 { return f.err }

func (f failingMeta) Clear(SessionName) error { return f.err }

// TestCompositeBindingStoreContract: first store wins on reads, writes go to
// the canonical store only, drops cover every store, and List is the union
// with the canonical copy kept, sorted by name.
func TestCompositeBindingStoreContract(t *testing.T) {
	canon, legacy := NewMemoryStore(), NewMemoryStore()
	c := newCompositeBindingStore(canon, legacy)

	fromLegacy := fullBinding("only-legacy")
	legacy.Save(fromLegacy)
	if got, ok, _ := c.Read("only-legacy"); !ok || !reflect.DeepEqual(got, fromLegacy) {
		t.Fatalf("fallback Read = %+v, %v", got, ok)
	}

	oldCopy, newCopy := fullBinding("both"), fullBinding("both")
	oldCopy.BlockID, newCopy.BlockID = "block:legacy", "block:canonical"
	legacy.Save(oldCopy)
	canon.Save(newCopy)
	if got, _, _ := c.Read("both"); got.BlockID != "block:canonical" {
		t.Errorf("Read = %s; canonical store must win", got.BlockID)
	}

	_ = c.Write("fresh", fullBinding("fresh"))
	if _, ok := canon.Load("fresh"); !ok {
		t.Error("Write did not reach the canonical store")
	}
	if _, ok := legacy.Load("fresh"); ok {
		t.Error("Write reached the legacy store")
	}

	list, err := c.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
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

	_ = c.Drop("both")
	if _, ok := canon.Load("both"); ok {
		t.Error("Drop left the canonical copy")
	}
	if _, ok := legacy.Load("both"); ok {
		t.Error("Drop left the legacy copy")
	}

	if _, ok, _ := newCompositeBindingStore().Read("x"); ok {
		t.Error("empty composite found a binding")
	}
	_ = newCompositeBindingStore().Write("x", fullBinding("x")) // must not panic
	if got, err := newCompositeBindingStore().List(); err != nil || got == nil || len(got) != 0 {
		t.Errorf("empty List = %#v, %v; want empty non-nil", got, err)
	}
	// A list failure in one store is reported, never hidden as an empty list.
	failing := errors.New("list failed")
	if _, err := newCompositeBindingStore(failingList{failing}, canon).List(); !errors.Is(err, failing) {
		t.Errorf("List error = %v; want the store's error joined", err)
	}
}

type failingList struct{ err error }

func (f failingList) Read(SessionName) (Binding, bool, error) { return Binding{}, false, nil }
func (f failingList) Write(SessionName, Binding) error        { return nil }
func (f failingList) Drop(SessionName) error                  { return nil }
func (f failingList) List() ([]Binding, error)                { return nil, f.err }

func TestCompositeActivityStoreContract(t *testing.T) {
	canon, legacy := NewMemoryActivityStore(), NewMemoryActivityStore()
	c := newCompositeActivityStore(canon, legacy)

	legacy.Save("a", ActivityRecord{Fingerprint: "legacy"})
	if got, ok, _ := c.Read("a"); !ok || got.Fingerprint != "legacy" {
		t.Fatalf("fallback Read = %+v, %v", got, ok)
	}
	canon.Save("a", ActivityRecord{Fingerprint: "canon"})
	if got, _, _ := c.Read("a"); got.Fingerprint != "canon" {
		t.Errorf("Read = %q; canonical must win", got.Fingerprint)
	}
	_ = c.Write("b", ActivityRecord{Fingerprint: "b"})
	if _, ok := canon.Load("b"); !ok {
		t.Error("Write did not reach the canonical store")
	}
	if _, ok := legacy.Load("b"); ok {
		t.Error("Write reached the legacy store")
	}
	if err := c.Drop("a"); err != nil {
		t.Fatal(err)
	}
	if _, ok := canon.Load("a"); ok {
		t.Error("Drop left the canonical record")
	}
	if _, ok := legacy.Load("a"); ok {
		t.Error("Drop left the legacy record")
	}

	e1, e2 := errors.New("e1"), errors.New("e2")
	err := newCompositeActivityStore(failingActivity{e1}, canon, failingActivity{e2}).Drop("x")
	if !errors.Is(err, e1) || !errors.Is(err, e2) {
		t.Errorf("Drop error = %v; want both errors joined", err)
	}
	_ = newCompositeActivityStore().Write("x", ActivityRecord{}) // must not panic
}

func TestCompositeMetaStoreContract(t *testing.T) {
	canon, legacy := NewMemoryMetaStore(), NewMemoryMetaStore()
	c := newCompositeMetaStore(canon, legacy)

	_ = legacy.Set("a", "k", "legacy")
	if v, ok, err := c.Read(metaKey{"a", "k"}); !ok || err != nil || v != "legacy" {
		t.Fatalf("fallback Read = %q, %v, %v", v, ok, err)
	}
	_ = canon.Set("a", "k", "canon")
	if v, _, _ := c.Read(metaKey{"a", "k"}); v != "canon" {
		t.Errorf("Read = %q; canonical must win", v)
	}
	if _, ok, err := c.Read(metaKey{"a", "missing"}); ok || err != nil {
		t.Errorf("missing Read = %v, %v", ok, err)
	}
	if err := c.Write(metaKey{"a", "new"}, "v"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := canon.Get("a", "new"); !ok {
		t.Error("Write did not reach the canonical store")
	}
	if _, ok, _ := legacy.Get("a", "new"); ok {
		t.Error("Write reached the legacy store")
	}
	if err := c.Drop(metaKey{"a", "k"}); err != nil {
		t.Fatal(err)
	}
	for name, s := range map[string]*memoryMetaStore{"canonical": canon, "legacy": legacy} {
		if _, ok, _ := s.Get("a", "k"); ok {
			t.Errorf("Drop left the key in the %s store", name)
		}
	}
	_ = legacy.Set("a", "old", "v")
	if err := c.Clear("a"); err != nil {
		t.Fatal(err)
	}
	for name, s := range map[string]*memoryMetaStore{"canonical": canon, "legacy": legacy} {
		if _, ok, _ := s.Get("a", "new"); ok {
			t.Errorf("Clear left a key in the %s store", name)
		}
		if _, ok, _ := s.Get("a", "old"); ok {
			t.Errorf("Clear left a key in the %s store", name)
		}
	}

	e1, e2 := errors.New("e1"), errors.New("e2")
	for op, fn := range map[string]func(MetaStore) error{
		"Drop":  func(s MetaStore) error { return s.Drop(metaKey{"x", "k"}) },
		"Clear": func(s MetaStore) error { return s.Clear("x") },
	} {
		err := fn(newCompositeMetaStore(failingMeta{e1}, canon, failingMeta{e2}))
		if !errors.Is(err, e1) || !errors.Is(err, e2) {
			t.Errorf("%s error = %v; want both errors joined", op, err)
		}
	}
	// A read error stops the fallback and is returned.
	if _, ok, err := newCompositeMetaStore(failingMeta{e1}, canon).Read(metaKey{"a", "new"}); ok || !errors.Is(err, e1) {
		t.Errorf("Read = %v, %v; want the first store's error", ok, err)
	}
	if err := newCompositeMetaStore().Write(metaKey{"x", "k"}, "v"); err != nil {
		t.Errorf("empty Write = %v; want nil", err)
	}
}
