package provider

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// NewNamespace bundles the state dirs and session labels of the ordered
// cities, canonical first. Labels are deduplicated; the first of each is the
// canonical one.
func TestNewNamespaceCanonicalFirst(t *testing.T) {
	base := t.TempDir()
	t.Setenv("GC_REX_STATE_DIR", base)

	ns, err := NewNamespace([]string{"default", "mycity", "default", "c"})
	if err != nil {
		t.Fatal(err)
	}
	wantDirs := []string{
		filepath.Join(base, "default"),
		filepath.Join(base, "mycity"),
		filepath.Join(base, "default"),
		filepath.Join(base, "c"),
	}
	if !slices.Equal(ns.Dirs(), wantDirs) {
		t.Errorf("Dirs = %v, want %v", ns.Dirs(), wantDirs)
	}
	if got := ns.CanonicalDir(); got != wantDirs[0] {
		t.Errorf("CanonicalDir = %q, want %q", got, wantDirs[0])
	}
	wantLabels := []string{rexSessionLabel("default"), rexSessionLabel("mycity"), rexSessionLabel("c")}
	if !slices.Equal(ns.Labels(), wantLabels) {
		t.Errorf("Labels = %v, want %v", ns.Labels(), wantLabels)
	}
	if got := ns.CanonicalLabel(); got != wantLabels[0] {
		t.Errorf("CanonicalLabel = %q, want %q", got, wantLabels[0])
	}
}

func TestNewNamespaceMatchesHelpers(t *testing.T) {
	t.Setenv("GC_REX_STATE_DIR", t.TempDir())
	cities := []string{"default", "a", "b"}
	ns, err := NewNamespace(cities)
	if err != nil {
		t.Fatal(err)
	}
	dirs, err := StateDirs(cities)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(ns.Dirs(), dirs) || !slices.Equal(ns.Labels(), SessionLabels(cities)) {
		t.Errorf("namespace %+v differs from StateDirs/SessionLabels", ns)
	}
}

func TestZeroNamespaceHasNoDirOrLabel(t *testing.T) {
	var ns Namespace
	if ns.CanonicalDir() != "" || ns.CanonicalLabel() != "" || len(ns.Dirs()) != 0 || len(ns.Labels()) != 0 {
		t.Errorf("zero Namespace not empty: %+v", ns)
	}
}

// An unresolvable state base fails NewNamespace instead of dropping a city.
func TestNewNamespaceUnresolvableDir(t *testing.T) {
	t.Setenv("GC_REX_STATE_DIR", "")
	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv("HOME", "")
	if _, err := NewNamespace([]string{"default"}); err == nil {
		t.Skip("home directory resolvable on this platform")
	}
}

// NamespaceForCaller derives the city and namespace from the env. A pinned
// caller keeps exactly its pin; an unpinned one appends the sibling
// namespaces found beside it, after the canonical candidates.
func TestNamespaceForCaller(t *testing.T) {
	base := t.TempDir()
	t.Setenv("GC_REX_STATE_DIR", base)
	t.Setenv("GC_REX_CITY", "")
	t.Setenv("GC_CITY_PATH", "/a/b/mycity")
	t.Setenv("GC_CITY", "")

	// A sibling controller pinned to "pinned" left a state dir.
	sibling, err := StateDir("pinned")
	if err != nil {
		t.Fatal(err)
	}
	mustMkdir(t, sibling)

	city, ns, err := NamespaceForCaller()
	if err != nil {
		t.Fatal(err)
	}
	if city != "default" {
		t.Errorf("city = %q, want default", city)
	}
	cities := []string{"default", "mycity", "pinned"}
	wantDirs, _ := StateDirs(cities)
	if !slices.Equal(ns.Dirs(), wantDirs) {
		t.Errorf("Dirs = %v, want %v", ns.Dirs(), wantDirs)
	}
	if !slices.Equal(ns.Labels(), SessionLabels(cities)) {
		t.Errorf("Labels = %v, want %v", ns.Labels(), SessionLabels(cities))
	}

	t.Setenv("GC_REX_CITY", "r")
	city, ns, err = NamespaceForCaller()
	if err != nil {
		t.Fatal(err)
	}
	rDir, _ := StateDir("r")
	if city != "r" || !slices.Equal(ns.Dirs(), []string{rDir}) || !slices.Equal(ns.Labels(), []string{rexSessionLabel("r")}) {
		t.Errorf("pinned: city=%q ns=%+v", city, ns)
	}
}

// WithNamespace is WithStateDirs plus WithSessionLabels over one value.
func TestWithNamespaceSetsLockDirAndLabels(t *testing.T) {
	t.Setenv("GC_REX_STATE_DIR", t.TempDir())
	ns, err := NewNamespace([]string{"default", "old"})
	if err != nil {
		t.Fatal(err)
	}
	p := New("ignored", nil, WithNamespace(ns))
	if p.ns.CanonicalDir() != ns.CanonicalDir() || !slices.Equal(p.ns.Labels(), ns.Labels()) {
		t.Errorf("provider ns = %+v, want %+v", p.ns, ns)
	}
	if p.binder.canonicalLabel() != rexSessionLabel("default") {
		t.Errorf("binder label = %q", p.binder.canonicalLabel())
	}
	// Zero namespace: in memory, label falls back to the city's.
	q := New("acme", nil, WithNamespace(Namespace{}))
	if q.ns.CanonicalDir() != "" || q.binder.canonicalLabel() != rexSessionLabel("acme") {
		t.Errorf("zero namespace: dir=%q label=%q", q.ns.CanonicalDir(), q.binder.canonicalLabel())
	}
}

func mustMkdir(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
}
