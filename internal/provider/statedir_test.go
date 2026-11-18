package provider

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestStateDirEnvOverride(t *testing.T) {
	t.Setenv("GC_EXEC_STATE_DIR", "")
	t.Setenv("GC_REX_STATE_DIR", "/tmp/gcrex")
	t.Setenv("XDG_STATE_HOME", "/tmp/xdg")
	got, err := StateDir("acme")
	if err != nil || got != filepath.Join("/tmp/gcrex", "acme") {
		t.Fatalf("StateDir = %q, %v", got, err)
	}
}

// GC_EXEC_STATE_DIR is gc's per-op state root and outranks the provider's own
// GC_REX_STATE_DIR; both are bases whose per-city subdirectory StateDir joins.
func TestStateDirExecStateDirOverride(t *testing.T) {
	t.Setenv("GC_EXEC_STATE_DIR", "/tmp/exec")
	t.Setenv("GC_REX_STATE_DIR", "/tmp/gcrex")
	t.Setenv("XDG_STATE_HOME", "/tmp/xdg")
	got, err := StateDir("acme")
	if err != nil || got != filepath.Join("/tmp/exec", "acme") {
		t.Fatalf("StateDir = %q, %v", got, err)
	}
}

// Without GC_EXEC_STATE_DIR the GC_REX_STATE_DIR fallback still wins over XDG.
func TestStateDirExecFallsBackToRex(t *testing.T) {
	t.Setenv("GC_EXEC_STATE_DIR", "")
	t.Setenv("GC_REX_STATE_DIR", "/tmp/gcrex")
	t.Setenv("XDG_STATE_HOME", "/tmp/xdg")
	got, err := StateDir("acme")
	if err != nil || got != filepath.Join("/tmp/gcrex", "acme") {
		t.Fatalf("StateDir = %q, %v", got, err)
	}
}

// GC_EXEC_STATE_DIR must be absolute, per gc's contract; a relative root is a
// state-dir error rather than a root resolved against the test's cwd.
func TestStateDirExecRelativeRejected(t *testing.T) {
	t.Setenv("GC_EXEC_STATE_DIR", filepath.Join("relative", "state"))
	t.Setenv("GC_REX_STATE_DIR", "/tmp/gcrex")
	if _, err := StateDir("acme"); err == nil {
		t.Fatal("StateDir with a relative GC_EXEC_STATE_DIR = nil error, want a state-dir error")
	}
}

func TestStateDirXDGFallback(t *testing.T) {
	t.Setenv("GC_EXEC_STATE_DIR", "")
	t.Setenv("GC_REX_STATE_DIR", "")
	t.Setenv("XDG_STATE_HOME", "/tmp/xdg")
	got, err := StateDir("acme")
	if err != nil || got != filepath.Join("/tmp/xdg", "rex-gascity", "acme") {
		t.Fatalf("StateDir = %q, %v", got, err)
	}
}

func TestStateDirHomeFallback(t *testing.T) {
	t.Setenv("GC_EXEC_STATE_DIR", "")
	t.Setenv("GC_REX_STATE_DIR", "")
	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv("HOME", "/home/x")
	got, err := StateDir("acme")
	if err != nil || got != filepath.Join("/home/x", ".local", "state", "rex-gascity", "acme") {
		t.Fatalf("StateDir = %q, %v", got, err)
	}
}

func TestStateDirCityCannotEscape(t *testing.T) {
	t.Setenv("GC_REX_STATE_DIR", "/tmp/gcrex")
	for _, city := range []string{"../../etc", `a\b`, "a/b", "a\x00b"} {
		got, err := StateDir(city)
		if err != nil {
			t.Fatal(err)
		}
		if filepath.Dir(got) != "/tmp/gcrex" || strings.ContainsAny(filepath.Base(got), `/\`+"\x00") {
			t.Errorf("StateDir(%q) = %q escapes the base", city, got)
		}
	}
}

func TestStateDirNamesDoNotCollide(t *testing.T) {
	t.Setenv("GC_REX_STATE_DIR", "/tmp/gcrex")
	dir := func(city string) string {
		t.Helper()
		got, err := StateDir(city)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	if dir("a/b") == dir("a_b") {
		t.Error(`"a/b" and "a_b" share a state dir`)
	}
	if dir("Acme") == dir("acme") {
		t.Error(`"Acme" and "acme" share a state dir`)
	}
	// On a case-insensitive filesystem (macOS APFS) dirs that differ only
	// by case are the same directory, so compare case-folded.
	cases := []string{"acme", "Acme", "ACME", "aCmE", "acmE", "a-b", "A-B", "rex9", "REX9"}
	for i, a := range cases {
		for _, b := range cases[i+1:] {
			if strings.EqualFold(dir(a), dir(b)) {
				t.Errorf("StateDir(%q) and StateDir(%q) collide case-insensitively: %q, %q", a, b, dir(a), dir(b))
			}
		}
	}
	if got := dir("acme"); got != filepath.Join("/tmp/gcrex", "acme") {
		t.Errorf("plain name changed: %q", got)
	}
	for _, city := range []string{"../..", "..", ".", "", ".hidden"} {
		got := dir(city)
		if filepath.Dir(got) != "/tmp/gcrex" {
			t.Errorf("StateDir(%q) = %q escapes the base", city, got)
		}
		for _, other := range []string{"_", "__", "..", "city"} {
			if other != city && got == dir(other) {
				t.Errorf("StateDir(%q) equals StateDir(%q)", city, other)
			}
		}
	}
}

func TestDecodeSafeNameRoundTrips(t *testing.T) {
	for _, city := range []string{"my-city", "Acme", "a/b", "a_b", ".hidden", "Grüße", "%25"} {
		enc := safeName(city)
		got, ok := decodeSafeName(enc)
		if !ok || got != city {
			t.Errorf("decodeSafeName(safeName(%q)) = %q, %v", city, got, ok)
		}
	}
	// Names safeName would never emit are rejected, so a stray directory is
	// never trusted as a namespace.
	for _, bad := range []string{"", "a/b", "Acme", "a_b", "a%2", "a%ZZ", "a%2f", "."} {
		if got, ok := decodeSafeName(bad); ok {
			t.Errorf("decodeSafeName(%q) = %q, want rejection", bad, got)
		}
	}
}

func TestSiblingCityNamespaces(t *testing.T) {
	base := t.TempDir()
	t.Setenv("GC_REX_STATE_DIR", base)
	t.Setenv("XDG_STATE_HOME", "")
	for _, city := range []string{"my-city", "other", "Acme"} {
		dir, err := StateDir(city)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// A directory name safeName would not produce is skipped.
	if err := os.MkdirAll(filepath.Join(base, "Bad_Name"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "not-a-dir"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	got := SiblingCityNamespaces([]string{"default", "other"})
	// Sorted, excludes a candidate and ignores the malformed entry.
	want := []string{"Acme", "my-city"}
	if !slices.Equal(got, want) {
		t.Errorf("SiblingCityNamespaces = %v, want %v", got, want)
	}
}

func TestSiblingCityNamespacesEmptyBase(t *testing.T) {
	t.Setenv("GC_REX_STATE_DIR", filepath.Join(t.TempDir(), "missing"))
	if got := SiblingCityNamespaces(nil); got != nil {
		t.Errorf("SiblingCityNamespaces on a missing base = %v, want nil", got)
	}
}

func TestSafeNameInjective(t *testing.T) {
	if got := safeName("acme"); got != "acme" {
		t.Errorf("safeName(acme) = %q", got)
	}
	if safeName("a/b") == safeName("a_b") {
		t.Error(`"a/b" and "a_b" collide`)
	}
	if safeName("a/b") == safeName("a_b-c14cddc0") {
		t.Error(`"a/b" and "a_b-c14cddc0" collide`)
	}
	for _, c := range []string{"..", "."} {
		got := safeName(c)
		if got == "." || got == ".." || strings.ContainsAny(got, `/\.`) {
			t.Errorf("safeName(%q) = %q", c, got)
		}
		for _, o := range []string{"", "_", "city", "%2E", "%2E%2E", "a"} {
			if o != c && safeName(o) == got {
				t.Errorf("safeName(%q) == safeName(%q)", c, o)
			}
		}
	}
	if got := safeName("Acme"); got != "%41cme" {
		t.Errorf("safeName(Acme) = %q, want %%41cme", got)
	}
	for _, c := range []string{"a", "z", "0", "9", "-"} {
		if safeName(c) != c {
			t.Errorf("safeName(%q) = %q, want kept", c, safeName(c))
		}
	}
	if safeName("") != "%00" {
		t.Errorf("empty = %q", safeName(""))
	}
}
