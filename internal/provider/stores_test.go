package provider

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func activityStores(t *testing.T) map[string]func() ActivityStore {
	dir := t.TempDir()
	return map[string]func() ActivityStore{
		"memory": NewMemoryActivityStore,
		"file":   func() ActivityStore { return NewFileActivityStore(dir) },
	}
}

func metaStores(t *testing.T) map[string]func() MetaStore {
	dir := t.TempDir()
	return map[string]func() MetaStore{
		"memory": NewMemoryMetaStore,
		"file":   func() MetaStore { return NewFileMetaStore(dir) },
	}
}

func TestActivityStoreRoundTrip(t *testing.T) {
	for kind, mk := range activityStores(t) {
		t.Run(kind, func(t *testing.T) {
			s := mk()
			if _, ok := s.Load("a"); ok {
				t.Fatal("Load before Save found a record")
			}
			rec := ActivityRecord{Fingerprint: "abc123", At: time.Date(2026, 11, 17, 19, 0, 0, 0, time.UTC)}
			s.Save("a", rec)
			got, ok := s.Load("a")
			if !ok || got.Fingerprint != rec.Fingerprint || !got.At.Equal(rec.At) {
				t.Fatalf("Load = %+v, %v", got, ok)
			}
			if _, ok := s.Load("b"); ok {
				t.Error("other name found a record")
			}
		})
	}
}

func TestActivityFileStorePersists(t *testing.T) {
	dir := t.TempDir()
	rec := ActivityRecord{Fingerprint: "fp", At: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)}
	NewFileActivityStore(dir).Save("a", rec)
	got, ok := NewFileActivityStore(dir).Load("a")
	if !ok || got.Fingerprint != "fp" || !got.At.Equal(rec.At) {
		t.Fatalf("Load = %+v, %v", got, ok)
	}
}

func TestMetaStoreRoundTripAndDelete(t *testing.T) {
	prompts := []string{"  leading and trailing  \n", "ready ▶ héllo 日本語 🚀", ""}
	for kind, mk := range metaStores(t) {
		t.Run(kind, func(t *testing.T) {
			s := mk()
			if _, ok := s.Get("a", "GC_READY_PROMPT_PREFIX"); ok {
				t.Fatal("Get before Set found a value")
			}
			for _, v := range prompts {
				if err := s.Set("a", "GC_READY_PROMPT_PREFIX", v); err != nil {
					t.Fatalf("Set: %v", err)
				}
				got, ok := s.Get("a", "GC_READY_PROMPT_PREFIX")
				if !ok || got != v {
					t.Errorf("Get = %q, %v; want %q", got, ok, v)
				}
			}
			for _, kv := range [][3]string{{"a", "GC_READY_PROMPT_PREFIX", "> "}, {"a", "other", "keep"}, {"b", "GC_READY_PROMPT_PREFIX", "b-value"}} {
				if err := s.Set(SessionName(kv[0]), kv[1], kv[2]); err != nil {
					t.Fatalf("Set: %v", err)
				}
			}
			if err := s.Delete("a", "GC_READY_PROMPT_PREFIX"); err != nil {
				t.Fatalf("Delete: %v", err)
			}
			if _, ok := s.Get("a", "GC_READY_PROMPT_PREFIX"); ok {
				t.Error("deleted key still set")
			}
			if v, ok := s.Get("a", "other"); !ok || v != "keep" {
				t.Errorf("other key = %q, %v", v, ok)
			}
			if v, ok := s.Get("b", "GC_READY_PROMPT_PREFIX"); !ok || v != "b-value" {
				t.Errorf("other name = %q, %v", v, ok)
			}
			if err := s.Delete("a", "missing"); err != nil {
				t.Errorf("Delete missing: %v", err)
			}
		})
	}
}

func TestMetaFileStorePersists(t *testing.T) {
	dir := t.TempDir()
	if err := NewFileMetaStore(dir).Set("a", "k", "v ▶ "); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if v, ok := NewFileMetaStore(dir).Get("a", "k"); !ok || v != "v ▶ " {
		t.Fatalf("Get = %q, %v", v, ok)
	}
}

func TestActivityFileStorePersistsIdentity(t *testing.T) {
	dir := t.TempDir()
	NewFileActivityStore(dir).Save("a", ActivityRecord{Identity: "block:1", Fingerprint: "fp", At: time.Unix(1, 0).UTC()})
	got, ok := NewFileActivityStore(dir).Load("a")
	if !ok || got.Identity != "block:1" {
		t.Errorf("Load = %+v, %v; want identity block:1", got, ok)
	}
}

func TestMetaFileStoreSetFailureIsReported(t *testing.T) {
	file := filepath.Join(t.TempDir(), "plainfile")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := NewFileMetaStore(filepath.Join(file, "sub"))
	if err := s.Set("a", "k", "v"); err == nil {
		t.Error("Set into an uncreatable dir returned nil")
	}
}
