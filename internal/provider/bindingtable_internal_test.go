package provider

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/docbobo/rex-gascity/internal/rex/faketest"
)

// errRead and errClear are the sentinels a deliberately failing store returns;
// the tests assert the provider reports them instead of reading them as
// absence.
var (
	errRead  = errors.New("binding read failed")
	errClear = errors.New("meta clear failed")
)

// failingRead is a full binding-record store whose reads fail. List, Write and
// Drop still answer, so a failure reaches the caller from Read alone.
type failingRead struct{}

func (failingRead) Read(SessionName) (Binding, bool, error) { return Binding{}, false, errRead }
func (failingRead) Write(SessionName, Binding) error        { return nil }
func (failingRead) Drop(SessionName) error                  { return nil }
func (failingRead) List() ([]Binding, error)                { return nil, nil }
func (failingRead) Mutate(SessionName, func(Binding) Binding) error {
	return errRead
}

// TestStartFreshOnlyReportsBindingReadError pins REQ-005/REQ-009: a store
// whose Read fails must surface that error from a fresh_only start, not be read
// as "no record holds the name" and answered with ErrFreshCollision.
func TestStartFreshOnlyReportsBindingReadError(t *testing.T) {
	p := New("acme", faketest.New(), WithBindingStore(failingRead{}))
	err := p.StartNew(context.Background(), "b", Config{Argv: []string{"pi"}, FreshOnly: true})
	if !errors.Is(err, errRead) {
		t.Fatalf("StartNew fresh_only with an unreadable store = %v; want %v", err, errRead)
	}
}

// TestBindingTableLooksUpAliases pins REQ-002/REQ-006 across every adapter:
// Lookup answers to an alias through the table alone, and a fresh_only start
// refuses a name a record holds only as an alias.
func TestBindingTableLooksUpAliases(t *testing.T) {
	adapters := map[string]func(t *testing.T) (bindingRecords, MetaStore){
		"memory": func(t *testing.T) (bindingRecords, MetaStore) {
			return NewMemoryStore(), NewMemoryMetaStore()
		},
		"file": func(t *testing.T) (bindingRecords, MetaStore) {
			return NewFileStore(t.TempDir()), NewFileMetaStore(t.TempDir())
		},
		"composite": func(t *testing.T) (bindingRecords, MetaStore) {
			return newCompositeBindingStore(NewMemoryStore(), NewFileStore(t.TempDir())),
				newCompositeMetaStore(NewMemoryMetaStore(), NewFileMetaStore(t.TempDir()))
		},
	}
	for kind, mk := range adapters {
		t.Run(kind, func(t *testing.T) {
			recs, meta := mk(t)
			table := newBindingTable(recs, meta)
			a := fullBinding("a")
			a.Aliases = []SessionName{"b"}
			if err := table.Write(a.Name, a); err != nil {
				t.Fatalf("Write: %v", err)
			}
			got, err := table.Lookup("b")
			if err != nil {
				t.Fatalf("Lookup(b): %v", err)
			}
			if len(got) != 1 || got[0].Name != "a" {
				t.Fatalf("Lookup(b) = %+v; want the record named a", got)
			}
			if got, err := table.Lookup("a"); err != nil || len(got) != 1 || got[0].Name != "a" {
				t.Fatalf("Lookup(a) = %+v, %v; want the primary record", got, err)
			}
			if got, err := table.Lookup("missing"); err != nil || len(got) != 0 {
				t.Fatalf("Lookup(missing) = %+v, %v; want no records", got, err)
			}

			p := New("acme", faketest.New(), WithBindingStore(recs), WithMetaStore(meta))
			err = p.StartNew(context.Background(), "b", Config{Argv: []string{"pi"}, FreshOnly: true})
			if !errors.Is(err, ErrFreshCollision) {
				t.Fatalf("StartNew fresh_only alias = %v; want ErrFreshCollision", err)
			}
		})
	}
}

// TestFileStoreListReportsCorruptFile pins the List error shape: a corrupt
// record file is an error naming the path, while a missing directory is an
// empty store with no error.
func TestFileStoreListReportsCorruptFile(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(bad, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	list, err := NewFileStore(dir).List()
	if err == nil || !strings.Contains(err.Error(), "bad.json") {
		t.Fatalf("List over a corrupt record = %+v, %v; want an error naming bad.json", list, err)
	}

	got, err := NewFileStore(t.TempDir()).List()
	if err != nil {
		t.Fatalf("List of a missing directory: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("List of a missing directory = %+v; want empty", got)
	}
}

// TestFileStoreListReportsUnreadableDir pins the List error shape for a
// directory that exists but cannot be read: the failure names the directory
// instead of reading as an empty store. filepath.Glob answered this with no
// matches and a nil error, so the store reported no bindings.
func TestFileStoreListReportsUnreadableDir(t *testing.T) {
	dir := t.TempDir()
	// A regular file where the store expects its directory: opening it as a
	// directory fails with ENOTDIR, which is not fs.ErrNotExist.
	notADir := filepath.Join(dir, "bindings")
	if err := os.WriteFile(notADir, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	list, err := NewFileStore(notADir).List()
	if err == nil || !strings.Contains(err.Error(), notADir) {
		t.Fatalf("List over an unreadable directory = %+v, %v; want an error naming %s", list, err, notADir)
	}
}

// TestFileStoreListKeepsHealthyRecordsBesideCorruptOnes pins the mixed case:
// a corrupt record is reported while every healthy record is still returned.
func TestFileStoreListKeepsHealthyRecordsBesideCorruptOnes(t *testing.T) {
	dir := t.TempDir()
	good := fullBinding("good")
	if err := NewFileStore(dir).Write(good.Name, good); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bad.json"), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	list, err := NewFileStore(dir).List()
	if err == nil || !strings.Contains(err.Error(), "bad.json") {
		t.Fatalf("List = %+v, %v; want an error naming bad.json", list, err)
	}
	if len(list) != 1 || list[0].Name != good.Name {
		t.Fatalf("List = %+v; want the healthy record beside the error", list)
	}
}

// lockedStore counts the Mutate calls that reach the backing store, so a test
// can prove the table forwarded the merge instead of read-then-writing it.
type lockedStore struct {
	*MemoryStore
	mutates int
}

func (s *lockedStore) Mutate(name SessionName, fn func(Binding) Binding) error {
	s.mutates++
	return s.MemoryStore.Mutate(name, fn)
}

// TestSaveKeepingSurvivorsUsesTheStoreLock pins the survivor merge to the
// store lock. The table is the only path from the Binder to the records, so a
// table that did not forward Mutate would silently fall back to an unlocked
// read-then-write and lose a survivor a stop recorded in between.
func TestSaveKeepingSurvivorsUsesTheStoreLock(t *testing.T) {
	records := &lockedStore{MemoryStore: NewMemoryStore()}
	binder := NewBinder(faketest.New(), newBindingTable(records, NewMemoryMetaStore()), testLabel, testTown)
	if err := binder.saveKeepingSurvivors(fullBinding("a")); err != nil {
		t.Fatalf("saveKeepingSurvivors: %v", err)
	}
	if records.mutates != 1 {
		t.Fatalf("backing store Mutate calls = %d; want 1 (the merge must run under the store lock)", records.mutates)
	}
}

// failingClear is a metadata store whose sweep fails. Everything else is the
// real memory store, so the start that precedes the stop still records its
// identity.
type failingClear struct {
	MetaStore
	err error
}

func (f failingClear) Clear(SessionName) error { return f.err }

// TestRetireReportsSweepError pins REQ-004: a failing meta sweep is reported by
// stop instead of being swallowed. The sweep still drops the activity record it
// can drop, and the binding stays so a retry of the failed sweep has a record
// to clear (retire returns before the binding is forgotten).
func TestRetireReportsSweepError(t *testing.T) {
	fake := faketest.New()
	meta := failingClear{MetaStore: NewMemoryMetaStore(), err: errClear}
	p := New("acme", fake, WithMetaStore(meta))
	ctx := context.Background()
	const name = "pi-mc-wisp-abc"
	if err := p.Start(ctx, name, Config{Argv: []string{"pi"}}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := p.activity.Write(SessionName(name), ActivityRecord{Identity: "block:1"}); err != nil {
		t.Fatalf("record activity: %v", err)
	}

	err := p.Stop(ctx, name)
	if !errors.Is(err, errClear) {
		t.Fatalf("Stop = %v; want the sweep error %v", err, errClear)
	}
	if _, ok, err := p.activity.Read(SessionName(name)); err != nil || ok {
		t.Errorf("Stop left the activity stamp (ok=%v, err=%v); the sweep still drops it", ok, err)
	}
	if _, ok, err := p.bindings.Read(SessionName(name)); err != nil || !ok {
		t.Errorf("Stop binding = ok %v, err %v; want the record kept for the sweep retry", ok, err)
	}
}
