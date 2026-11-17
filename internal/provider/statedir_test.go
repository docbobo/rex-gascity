package provider

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestStateDirEnvOverride(t *testing.T) {
	t.Setenv("GC_REX_STATE_DIR", "/tmp/gcrex")
	t.Setenv("XDG_STATE_HOME", "/tmp/xdg")
	got, err := StateDir("acme")
	if err != nil || got != filepath.Join("/tmp/gcrex", "acme") {
		t.Fatalf("StateDir = %q, %v", got, err)
	}
}

func TestStateDirXDGFallback(t *testing.T) {
	t.Setenv("GC_REX_STATE_DIR", "")
	t.Setenv("XDG_STATE_HOME", "/tmp/xdg")
	got, err := StateDir("acme")
	if err != nil || got != filepath.Join("/tmp/xdg", "rex-gas-city", "acme") {
		t.Fatalf("StateDir = %q, %v", got, err)
	}
}

func TestStateDirHomeFallback(t *testing.T) {
	t.Setenv("GC_REX_STATE_DIR", "")
	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv("HOME", "/home/x")
	got, err := StateDir("acme")
	if err != nil || got != filepath.Join("/home/x", ".local", "state", "rex-gas-city", "acme") {
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
	if safeName("") != "%00" {
		t.Errorf("empty = %q", safeName(""))
	}
}
