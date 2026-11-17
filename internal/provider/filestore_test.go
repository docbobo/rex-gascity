package provider

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func fullBinding(name SessionName) Binding {
	return Binding{
		Name: name, SessionID: "session:acme", SessionLabel: "gc:acme",
		WindowID: "window:Indigo", BlockID: "block:dog-1",
		LaunchMode:  "resume",
		CreatedAt:   time.Date(2026, 11, 17, 19, 0, 0, 0, time.UTC),
		ReadyPrompt: "ready ▶ ",
	}
}

func TestFileStoreRoundTrip(t *testing.T) {
	s := NewFileStore(filepath.Join(t.TempDir(), "bindings"))
	want := fullBinding("Indigo--dog-1")
	if _, ok := s.Load(want.Name); ok {
		t.Fatal("Load before Save found a binding")
	}
	s.Save(want)
	got, ok := s.Load(want.Name)
	if !ok || !reflect.DeepEqual(got, want) {
		t.Fatalf("Load = %+v, %v; want %+v", got, ok, want)
	}
	s.Delete(want.Name)
	if _, ok := s.Load(want.Name); ok {
		t.Error("Load after Delete found a binding")
	}
	s.Delete(want.Name) // missing file is not an error
}

func TestFileStorePersistsAcrossInstances(t *testing.T) {
	dir := t.TempDir()
	want := fullBinding("Indigo--dog-1")
	NewFileStore(dir).Save(want)

	got, ok := NewFileStore(dir).Load(want.Name)
	if !ok || !reflect.DeepEqual(got, want) {
		t.Fatalf("second instance Load = %+v, %v", got, ok)
	}
}

func TestFileStoreNamesDoNotCollide(t *testing.T) {
	s := NewFileStore(t.TempDir())
	a, b := fullBinding("Indigo--dog-1"), fullBinding("Indigo--dog-2")
	b.BlockID = "block:dog-2"
	s.Save(a)
	s.Save(b)
	if got, _ := s.Load(a.Name); got.BlockID != "block:dog-1" {
		t.Errorf("a = %+v", got)
	}
	if got, _ := s.Load(b.Name); got.BlockID != "block:dog-2" {
		t.Errorf("b = %+v", got)
	}
	s.Delete(a.Name)
	if _, ok := s.Load(b.Name); !ok {
		t.Error("deleting a removed b")
	}
}

func TestFileStoreCorruptFileReadsAsNotFound(t *testing.T) {
	dir := t.TempDir()
	s := NewFileStore(dir)
	name := SessionName("Indigo--dog-1")
	if err := os.WriteFile(filepath.Join(dir, nameKey(name)+".json"), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Load(name); ok {
		t.Fatal("corrupt file loaded")
	}
	want := fullBinding(name)
	s.Save(want)
	if got, ok := s.Load(name); !ok || !reflect.DeepEqual(got, want) {
		t.Errorf("Save did not overwrite corrupt file: %+v, %v", got, ok)
	}
}

func TestFileStoreRecordFieldNames(t *testing.T) {
	dir := t.TempDir()
	s := NewFileStore(dir)
	b := fullBinding("Indigo--dog-1")
	s.Save(b)
	data, err := os.ReadFile(filepath.Join(dir, nameKey(b.Name)+".json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"gc_name", "rex_session_id", "rex_session_label", "window_id",
		"block_id", "launch_mode", "created_at", "ready_prompt"} {
		if !strings.Contains(string(data), `"`+f+`"`) {
			t.Errorf("record missing field %q: %s", f, data)
		}
	}
}
