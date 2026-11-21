package provider

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

type activityAPI interface {
	Load(SessionName) (ActivityRecord, bool)
	Save(SessionName, ActivityRecord)
	Delete(SessionName) error
}

func activityStores(t *testing.T) map[string]func() activityAPI {
	dir := t.TempDir()
	return map[string]func() activityAPI{
		"memory": func() activityAPI { return NewMemoryActivityStore() },
		"file":   func() activityAPI { return NewFileActivityStore(dir) },
	}
}

type metaAPI interface {
	Get(SessionName, string) (string, bool, error)
	Set(SessionName, string, string) error
	Delete(SessionName, string) error
	Clear(SessionName) error
}

func metaStores(t *testing.T) map[string]func() metaAPI {
	dir := t.TempDir()
	return map[string]func() metaAPI{
		"memory": func() metaAPI { return NewMemoryMetaStore() },
		"file":   func() metaAPI { return NewFileMetaStore(dir) },
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
			if _, ok, err := s.Get("a", "GC_READY_PROMPT_PREFIX"); ok || err != nil {
				t.Fatalf("Get before Set = %v, %v; want not set, nil", ok, err)
			}
			for _, v := range prompts {
				if err := s.Set("a", "GC_READY_PROMPT_PREFIX", v); err != nil {
					t.Fatalf("Set: %v", err)
				}
				got, ok, err := s.Get("a", "GC_READY_PROMPT_PREFIX")
				if !ok || err != nil || got != v {
					t.Errorf("Get = %q, %v, %v; want %q", got, ok, err, v)
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
			if _, ok, err := s.Get("a", "GC_READY_PROMPT_PREFIX"); ok || err != nil {
				t.Errorf("deleted key = %v, %v; want not set, nil", ok, err)
			}
			if v, ok, err := s.Get("a", "other"); !ok || err != nil || v != "keep" {
				t.Errorf("other key = %q, %v, %v", v, ok, err)
			}
			if v, ok, err := s.Get("b", "GC_READY_PROMPT_PREFIX"); !ok || err != nil || v != "b-value" {
				t.Errorf("other name = %q, %v, %v", v, ok, err)
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
	if v, ok, err := NewFileMetaStore(dir).Get("a", "k"); !ok || err != nil || v != "v ▶ " {
		t.Fatalf("Get = %q, %v, %v", v, ok, err)
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

func TestMetaFileStoreKeepsInvalidUTF8(t *testing.T) {
	dir := t.TempDir()
	want := string([]byte{0x80, 0xff})
	if err := NewFileMetaStore(dir).Set("a", "k", want); err != nil {
		t.Fatal(err)
	}
	got, ok, err := NewFileMetaStore(dir).Get("a", "k")
	if !ok || err != nil || got != want {
		t.Errorf("Get = %q, %v, %v; want %q", got, ok, err, want)
	}
}

func TestMetaFileStoreBadEntries(t *testing.T) {
	dir := t.TempDir()
	s := NewFileMetaStore(dir)
	for _, tc := range []struct {
		body    string
		wantErr bool
	}{
		{"not json", true},
		{`{"key":"k","value_b64":"!!"}`, true},
		// An old-format entry (no value_b64) is not set, not an error.
		{`{"key":"k","value":"old"}`, false},
	} {
		path := s.path("a", "k")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(tc.body), 0o644); err != nil {
			t.Fatal(err)
		}
		got, ok, err := s.Get("a", "k")
		if ok || got != "" || (err != nil) != tc.wantErr {
			t.Errorf("%s: Get = %q, %v, %v; want not set, error=%v", tc.body, got, ok, err, tc.wantErr)
		}
	}
}

func TestMetaFileStoreMissingKeyIsUnset(t *testing.T) {
	s := NewFileMetaStore(t.TempDir())
	if got, ok, err := s.Get("a", "missing"); ok || err != nil || got != "" {
		t.Errorf("Get = %q, %v, %v; want \"\", false, nil", got, ok, err)
	}
}

func TestMetaFileStoreUnreadableDirIsError(t *testing.T) {
	file := filepath.Join(t.TempDir(), "plainfile")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := NewFileMetaStore(filepath.Join(file, "meta"))
	if got, ok, err := s.Get("a", "k"); err == nil || ok || got != "" {
		t.Errorf("Get = %q, %v, %v; want a read error", got, ok, err)
	}
}

// TestMetaSnapshotIgnoresTempResidue is the CodeRabbit fix for stores.go: a
// crash between CreateTemp and Rename leaves a `.tmp-*` file holding a complete
// entry. Get does not read that path, so Snapshot must not report it either.
func TestMetaSnapshotIgnoresTempResidue(t *testing.T) {
	dir := t.TempDir()
	s := NewFileMetaStore(dir)
	name := SessionName("Indigo--dog-1")
	if err := s.Set(name, "GC_SESSION_ID", "real"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	// A completed entry for a key with no .json file, parked in a temp name.
	entry := `{"key":"GC_INSTANCE_TOKEN","value_b64":"c3RhbGU="}`
	if err := os.WriteFile(filepath.Join(dir, nameKey(name), ".tmp-crash"), []byte(entry), 0o644); err != nil {
		t.Fatalf("write residue: %v", err)
	}
	snap := s.Snapshot(name)
	if _, ok := snap["GC_INSTANCE_TOKEN"]; ok {
		t.Errorf("Snapshot returned a key from a temp-file residue: %v", snap)
	}
	if got := snap["GC_SESSION_ID"]; got != "real" {
		t.Errorf("Snapshot[GC_SESSION_ID] = %q, want real", got)
	}
}
