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
		TabLabel:    "Indigo",
		PaneLabel:   "dog-1",
		Aliases:     []SessionName{"Indigo--dog", "Indigo--dog-1"},
		LaunchMode:  "resume",
		CreatedAt:   time.Date(2026, 11, 17, 19, 0, 0, 0, time.UTC),
		ReadyPrompt: "ready ▶ ",
	}
}

// TestFileStoreLegacyRecordFallsBackToNameLabels writes a record in the shape
// produced before TabLabel, PaneLabel and Aliases existed and checks it still
// loads with the new fields empty, so callers fall back to name-derived labels.
func TestFileStoreLegacyRecordFallsBackToNameLabels(t *testing.T) {
	dir := t.TempDir()
	name := SessionName("Indigo--dog-1")
	raw := `{"gc_name":"Indigo--dog-1","rex_session_id":"session:acme",` +
		`"rex_session_label":"gc:acme","window_id":"window:Indigo",` +
		`"block_id":"block:dog-1","launch_mode":"window"}`
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, nameKey(name)+".json"), []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}

	got, ok := NewFileStore(dir).Load(name)
	if !ok {
		t.Fatal("legacy record did not load")
	}
	if got.TabLabel != "" || got.PaneLabel != "" || got.Aliases != nil {
		t.Errorf("legacy record decoded new fields = %q/%q/%v, want empty", got.TabLabel, got.PaneLabel, got.Aliases)
	}
	if got.tabLabel() != "Indigo" || got.paneLabel() != "dog-1" {
		t.Errorf("legacy labels = %q/%q, want Indigo/dog-1", got.tabLabel(), got.paneLabel())
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
		"block_id", "tab_label", "pane_label", "aliases", "launch_mode", "created_at", "ready_prompt"} {
		if !strings.Contains(string(data), `"`+f+`"`) {
			t.Errorf("record missing field %q: %s", f, data)
		}
	}
}
