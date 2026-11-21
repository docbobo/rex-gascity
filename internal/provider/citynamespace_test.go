package provider_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/docbobo/rex-gascity/internal/provider"
	"github.com/docbobo/rex-gascity/internal/rex"
	"github.com/docbobo/rex-gascity/internal/rex/faketest"
)

// CityFromEnv must not depend on the incidental GC_CITY_PATH/GC_CITY that only
// some gc callers carry: start (a bare launchd controller) and the lookup ops
// (a session or nudge-poller process) have to agree on one gc:<city> session
// and one state directory. GC_REX_CITY stays the explicit per-city pin.
// namespacesOption resolves cities and returns the one namespace option,
// surfacing a state-dir error instead of letting the provider fall back to
// memory.
func namespacesOption(t *testing.T, cities []string) provider.Option {
	t.Helper()
	ns, err := provider.NewNamespace(cities)
	if err != nil {
		t.Fatal(err)
	}
	return provider.WithNamespace(ns)
}

func TestCityFromEnvIsCallerIndependent(t *testing.T) {
	for _, tc := range []struct {
		rex, path, city, want string
	}{
		{"", "", "", "default"},
		{"", "/a/b/mycity", "", "default"},
		{"", "", "c", "default"},
		{"", "/a/b/mycity", "c", "default"},
		{"r", "/a/b/mycity", "c", "r"},
	} {
		t.Setenv("GC_EXEC_CITY_NAME", "")
		t.Setenv("GC_EXEC_CITY_PATH", "")
		t.Setenv("GC_REX_CITY", tc.rex)
		t.Setenv("GC_CITY_PATH", tc.path)
		t.Setenv("GC_CITY", tc.city)
		if got := provider.CityFromEnv(); got != tc.want {
			t.Errorf("rex=%q path=%q city=%q: CityFromEnv = %q, want %q", tc.rex, tc.path, tc.city, got, tc.want)
		}
	}
}

// gc's per-op city contract names the city (GC_EXEC_CITY_NAME) and gives its
// path (GC_EXEC_CITY_PATH): the name selects the canonical city, and a caller
// that carries only the path is named by its last component. A pin is a name,
// never a path — gc:<absolute path> is not a city name. Both outrank the
// provider's own GC_REX_CITY pin, which stays as a fallback.
func TestCityFromEnvPrefersExecCityName(t *testing.T) {
	for _, tc := range []struct {
		name, exec, rex, want string
	}{
		{"my-city", "/a/b/mycity", "", "my-city"},
		{"my-city", "/a/b/mycity", "pin", "my-city"},
		{"my-city", "", "pin", "my-city"},
		{"", "/a/b/mycity", "", "mycity"},
		{"", "/a/b/mycity/", "", "mycity"},
		{"", "/a/b/mycity", "pin", "mycity"},
		{"", "", "pin", "pin"},
		{"", "", "", "default"},
	} {
		t.Setenv("GC_EXEC_CITY_NAME", tc.name)
		t.Setenv("GC_EXEC_CITY_PATH", tc.exec)
		t.Setenv("GC_REX_CITY", tc.rex)
		if got := provider.CityFromEnv(); got != tc.want {
			t.Errorf("name=%q exec=%q rex=%q: CityFromEnv = %q, want %q", tc.name, tc.exec, tc.rex, got, tc.want)
		}
	}
}

// Whatever gc supplies, the pin is a name: the Rex session label never carries
// a path separator. GCREX-40's review flagged the path-shaped label
// (gc:<absolute path>) as unverified against the 128-char label rule.
func TestCityLabelIsNeverPathShaped(t *testing.T) {
	for _, tc := range []struct{ name, exec string }{
		{"my-city", "/a/b/mycity"},
		{"", "/a/b/mycity"},
		{"", "/Users/boris/my-city"},
	} {
		t.Setenv("GC_EXEC_CITY_NAME", tc.name)
		t.Setenv("GC_EXEC_CITY_PATH", tc.exec)
		t.Setenv("GC_REX_CITY", "")
		labels := provider.SessionLabels(provider.CityCandidates())
		if len(labels) != 1 {
			t.Fatalf("name=%q exec=%q: labels = %v, want exactly one", tc.name, tc.exec, labels)
		}
		if strings.Contains(labels[0], "/") {
			t.Errorf("name=%q exec=%q: label %q carries a path", tc.name, tc.exec, labels[0])
		}
	}
}

// CityCandidates is the ordered, deduplicated read set: the canonical
// namespace first, then the incidental namespaces a legacy record may live
// under. A pinned city resolves to exactly its pin, so pinned cities sharing
// one supervisor never read each other's state.
func TestCityCandidates(t *testing.T) {
	for _, tc := range []struct {
		rex, path, city string
		want            []string
	}{
		{"", "", "", []string{"default"}},
		{"", "/a/b/mycity", "", []string{"default", "mycity"}},
		{"", "", "c", []string{"default", "c"}},
		{"", "/a/b/mycity", "c", []string{"default", "mycity", "c"}},
		{"", "/a/b/default", "", []string{"default"}},
		{"", "/a/b/mycity", "mycity", []string{"default", "mycity"}},
		{"r", "/a/b/mycity", "c", []string{"r"}},
	} {
		t.Setenv("GC_EXEC_CITY_NAME", "")
		t.Setenv("GC_EXEC_CITY_PATH", "")
		t.Setenv("GC_REX_CITY", tc.rex)
		t.Setenv("GC_CITY_PATH", tc.path)
		t.Setenv("GC_CITY", tc.city)
		if got := provider.CityCandidates(); !slices.Equal(got, tc.want) {
			t.Errorf("rex=%q path=%q city=%q: CityCandidates = %v, want %v", tc.rex, tc.path, tc.city, got, tc.want)
		}
	}
}

// A caller pinned by gc's city contract reads exactly that city, like the
// GC_REX_CITY pin: the incidental GC_CITY_PATH/GC_CITY candidates are not read.
// The pin is the name gc gives the city, not the path it serves it from.
func TestCityCandidatesExecCityPathIsPin(t *testing.T) {
	t.Setenv("GC_EXEC_CITY_NAME", "")
	t.Setenv("GC_EXEC_CITY_PATH", "/a/b/mycity")
	t.Setenv("GC_REX_CITY", "")
	t.Setenv("GC_CITY_PATH", "/a/b/mycity")
	t.Setenv("GC_CITY", "mycity")
	if got := provider.CityCandidates(); !slices.Equal(got, []string{"mycity"}) {
		t.Errorf("CityCandidates = %v, want [mycity]", got)
	}
	t.Setenv("GC_EXEC_CITY_PATH", "")
	if got := provider.CityCandidates(); !slices.Equal(got, []string{"default", "mycity"}) {
		t.Errorf("without exec path: CityCandidates = %v, want [default mycity]", got)
	}
}

// A binding stored under an incidental namespace for a city must be found by
// the lookup ops invoked under the canonical namespace, and its metadata must
// be readable and writable too (writes land in the canonical store).
func TestLookupSpansCandidateNamespaces(t *testing.T) {
	ctx := context.Background()
	base := t.TempDir()
	t.Setenv("GC_REX_STATE_DIR", base)
	t.Setenv("GC_REX_CITY", "")
	t.Setenv("GC_CITY_PATH", "/a/b/mycity")
	t.Setenv("GC_CITY", "")

	name := provider.SessionName("Indigo--dog-1")
	legacyDir, err := provider.StateDir("mycity")
	if err != nil {
		t.Fatal(err)
	}
	// A legacy record lives under the incidental mycity namespace, carrying
	// the legacy session label.
	provider.NewFileStore(filepath.Join(legacyDir, "bindings")).Save(provider.Binding{
		Name:         name,
		SessionID:    "session:gc:mycity",
		SessionLabel: "gc:mycity",
		WindowID:     "window:Indigo",
		BlockID:      "block:dog-1",
		TabLabel:     "Indigo",
		PaneLabel:    "dog-1",
	})
	if err := provider.NewFileMetaStore(filepath.Join(legacyDir, "meta")).Set(name, "GC_SESSION_ID", "legacy-1"); err != nil {
		t.Fatal(err)
	}

	// Rex still serves the legacy session.
	fake := faketest.New()
	fake.Formats = map[rex.BlockID]string{"block:dog-1": "legacy output\n"}
	fake.Sessions = []rex.Session{{ID: "session:gc:mycity", Label: "gc:mycity"}}
	fake.View = rex.SessionView{
		Windows: []rex.Window{{
			ID:     "window:Indigo",
			Label:  "Indigo",
			Blocks: []rex.Block{{ID: "block:dog-1", Label: "dog-1"}},
		}},
	}

	p := provider.New("default", fake, namespacesOption(t, provider.CityCandidates()))

	running, err := p.IsRunning(ctx, string(name))
	if err != nil || !running {
		t.Fatalf("IsRunning across candidates = %v, %v; want true, nil", running, err)
	}
	if got, err := p.GetMeta(ctx, string(name), "GC_SESSION_ID"); err != nil || got != "legacy-1" {
		t.Errorf("GetMeta across candidates = %q, %v; want %q", got, err, "legacy-1")
	}
	if got, err := p.Peek(ctx, string(name)); err != nil || got != "legacy output\n" {
		t.Errorf("Peek across candidates = %q, %v", got, err)
	}
	if got, err := p.ListRunning(ctx, "Indigo--"); err != nil || !slices.Equal(got, []string{string(name)}) {
		t.Errorf("ListRunning across candidates = %v, %v", got, err)
	}
	if _, err := p.GetLastActivity(ctx, string(name)); err != nil {
		t.Errorf("GetLastActivity across candidates: %v", err)
	}
	if err := p.SetMeta(ctx, string(name), "GC_SESSION_ID", "new-2"); err != nil {
		t.Fatal(err)
	}
	canonicalDir, err := provider.StateDir("default")
	if err != nil {
		t.Fatal(err)
	}
	if got, ok, err := provider.NewFileMetaStore(filepath.Join(canonicalDir, "meta")).Get(name, "GC_SESSION_ID"); err != nil || !ok || got != "new-2" {
		t.Errorf("SetMeta wrote to the canonical store: %q, %v, %v", got, ok, err)
	}
	if err := p.Stop(ctx, string(name)); err != nil {
		t.Fatalf("Stop across candidates: %v", err)
	}
	if !slices.Contains(fake.ClosedBlocks, rex.BlockID("block:dog-1")) {
		t.Errorf("Stop did not close the legacy pane: %v", fake.ClosedBlocks)
	}
}

// Two cities pinned to different GC_REX_CITY values, sharing one supervisor,
// must never cross-resolve each other's sessions.
func TestPinnedNamespacesDoNotCrossResolve(t *testing.T) {
	ctx := context.Background()
	base := t.TempDir()
	t.Setenv("GC_REX_STATE_DIR", base)
	t.Setenv("GC_CITY", "")
	t.Setenv("GC_CITY_PATH", "/a/b/mycity")

	fake := faketest.New()

	t.Setenv("GC_REX_CITY", "a")
	if got := provider.CityCandidates(); !slices.Equal(got, []string{"a"}) {
		t.Fatalf("pinned candidates = %v, want [a]", got)
	}
	pA := provider.New("a", fake, namespacesOption(t, provider.CityCandidates()))
	if err := pA.Start(ctx, "Indigo--dog-1", provider.Config{Argv: []string{"sh"}}); err != nil {
		t.Fatalf("start in city a: %v", err)
	}

	t.Setenv("GC_REX_CITY", "b")
	pB := provider.New("b", fake, namespacesOption(t, provider.CityCandidates()))
	running, err := pB.IsRunning(ctx, "Indigo--dog-1")
	if err != nil {
		t.Fatalf("IsRunning in city b: %v", err)
	}
	if running {
		t.Error("city b resolved city a's session")
	}
}

// A queued nudge whose target is running must deliver even when the controller
// and the nudge poller see different candidate names from the env. This is the
// incident GCREX-17 records: start wrote under one namespace while the poller
// looked under another. It runs over a shared state dir and a shared fake Rex
// server, like the real single-server city.
func TestNudgeDeliversAcrossCandidateNamespaces(t *testing.T) {
	ctx := context.Background()
	base := t.TempDir()
	t.Setenv("GC_REX_STATE_DIR", base)
	t.Setenv("GC_REX_CITY", "")
	t.Setenv("GC_CITY", "")

	fake := faketest.New()
	const name = "Indigo--dog-1"

	// The nudge poller carries an incidental GC_CITY_PATH.
	t.Setenv("GC_CITY_PATH", "/data/my-city")
	poller := provider.New(provider.CityFromEnv(), fake, namespacesOption(t, provider.CityCandidates()))
	if err := poller.Start(ctx, name, provider.Config{Argv: []string{"sh"}}); err != nil {
		t.Fatalf("poller Start: %v", err)
	}

	// The bare launchd controller carries no city env and resolves the
	// canonical namespace.
	t.Setenv("GC_CITY_PATH", "")
	controller := provider.New(provider.CityFromEnv(), fake, namespacesOption(t, provider.CityCandidates()))

	running, err := controller.IsRunning(ctx, name)
	if err != nil || !running {
		t.Fatalf("controller IsRunning = %v, %v; want true, nil", running, err)
	}
	if err := controller.Nudge(ctx, name, "queued nudge"); err != nil {
		t.Fatalf("controller Nudge: %v", err)
	}
	wrote, submitted := false, false
	for _, c := range fake.BlockCalls {
		if c.Op == "write" && c.Text == "queued nudge" {
			wrote = true
		}
	}
	for _, k := range fake.SentKeys {
		if slices.Contains(k.Keys, "enter") {
			submitted = true
		}
	}
	if !wrote || !submitted {
		t.Errorf("nudge not delivered: wrote=%v submitted=%v calls=%+v keys=%+v", wrote, submitted, fake.BlockCalls, fake.SentKeys)
	}
}

// savePinnedBinding writes a binding and one meta key under city's state dir,
// the way a pinned controller's start op does.
func savePinnedBinding(t *testing.T, city string, b provider.Binding, metaKey, metaValue string) {
	t.Helper()
	dir, err := provider.StateDir(city)
	if err != nil {
		t.Fatal(err)
	}
	provider.NewFileStore(filepath.Join(dir, "bindings")).Save(b)
	if metaKey != "" {
		if err := provider.NewFileMetaStore(filepath.Join(dir, "meta")).Set(b.Name, metaKey, metaValue); err != nil {
			t.Fatal(err)
		}
	}
}

// An unpinned caller (no GC_REX_CITY/GC_CITY_PATH/GC_CITY) must discover the
// pin a sibling controller wrote under. This is the GCREX-35 incident: the
// binding lives in the pinned city's state dir and the Rex server has a live
// gc:<city> session, but the caller's env names no city, so CityCandidates
// alone only sees "default" and every lookup answers "not running".
func TestUnpinnedLookupDiscoversPinnedCity(t *testing.T) {
	ctx := context.Background()
	base := t.TempDir()
	t.Setenv("GC_REX_STATE_DIR", base)
	t.Setenv("GC_REX_CITY", "")
	t.Setenv("GC_CITY_PATH", "")
	t.Setenv("GC_CITY", "")

	const name = "Indigo--dog-1"
	savePinnedBinding(t, "my-city", provider.Binding{
		Name:         name,
		SessionID:    "session:gc:my-city",
		SessionLabel: "gc:my-city",
		WindowID:     "window:Indigo",
		BlockID:      "block:dog-1",
		TabLabel:     "Indigo",
		PaneLabel:    "dog-1",
	}, "GC_SESSION_ID", "pinned-1")

	fake := faketest.New()
	fake.Sessions = []rex.Session{{ID: "session:gc:my-city", Label: "gc:my-city"}}
	fake.View = rex.SessionView{Windows: []rex.Window{{
		ID:     "window:Indigo",
		Label:  "Indigo",
		Blocks: []rex.Block{{ID: "block:dog-1", Label: "dog-1"}},
	}}}
	fake.Formats = map[rex.BlockID]string{"block:dog-1": "pinned output\n"}

	cities := provider.CityCandidates()
	cities = append(cities, provider.SiblingCityNamespaces(cities)...)
	if !slices.Equal(cities, []string{"default", "my-city"}) {
		t.Fatalf("candidates = %v, want [default my-city]", cities)
	}
	p := provider.New(provider.CityFromEnv(), fake, namespacesOption(t, cities))

	running, err := p.IsRunning(ctx, name)
	if err != nil || !running {
		t.Fatalf("IsRunning = %v, %v; want true, nil", running, err)
	}
	if got, err := p.Peek(ctx, name); err != nil || got != "pinned output\n" {
		t.Errorf("Peek = %q, %v; want the pinned pane output", got, err)
	}
	if got, err := p.GetMeta(ctx, name, "GC_SESSION_ID"); err != nil || got != "pinned-1" {
		t.Errorf("GetMeta = %q, %v; want pinned-1", got, err)
	}
	if names, err := p.ListRunning(ctx, "Indigo--"); err != nil || !slices.Equal(names, []string{name}) {
		t.Errorf("ListRunning = %v, %v; want [%s]", names, err, name)
	}

	// AC #5: a genuinely absent session has no screen (exit 66 at the CLI).
	if got, err := p.Peek(ctx, "Indigo--absent"); !errors.Is(err, provider.ErrNoScreen) || got != "" {
		t.Errorf("Peek(absent) = %q, %v; want empty, ErrNoScreen", got, err)
	}

	// set-meta resolves the pinned name; writes stay in the canonical store.
	if err := p.SetMeta(ctx, name, "suspended", "true"); err != nil {
		t.Fatalf("SetMeta: %v", err)
	}
	canonicalDir, err := provider.StateDir("default")
	if err != nil {
		t.Fatal(err)
	}
	if got, ok, err := provider.NewFileMetaStore(filepath.Join(canonicalDir, "meta")).Get(name, "suspended"); err != nil || !ok || got != "true" {
		t.Errorf("SetMeta wrote to the canonical store: %q, %v, %v", got, ok, err)
	}
}

// perSessionViews overrides SessionView so one fake Rex server can serve two
// cities with different panes; faketest exposes only one shared View.
type perSessionViews struct {
	*faketest.Fake
	views map[rex.SessionID]rex.SessionView
}

func (f *perSessionViews) SessionView(_ context.Context, sid rex.SessionID) (rex.SessionView, error) {
	if view, ok := f.views[sid]; ok {
		return view, nil
	}
	return f.Fake.SessionView(context.Background(), sid)
}

// Two live pinned cities must not cross-resolve when the caller is unpinned:
// a name that only city alpha holds resolves alpha's pane, and city beta's
// session stays separate. (Unpinned duplicate names are documented below.)
func TestUnpinnedLookupKeepsDistinctCitiesApart(t *testing.T) {
	ctx := context.Background()
	base := t.TempDir()
	t.Setenv("GC_REX_STATE_DIR", base)
	t.Setenv("GC_REX_CITY", "")
	t.Setenv("GC_CITY_PATH", "")
	t.Setenv("GC_CITY", "")

	const nameA, nameB = "RigA--agent-a", "RigB--agent-b"
	savePinnedBinding(t, "alpha", provider.Binding{
		Name:         nameA,
		SessionID:    "session:gc:alpha",
		SessionLabel: "gc:alpha",
		WindowID:     "window:RigA",
		BlockID:      "block:agent-a",
		TabLabel:     "RigA",
		PaneLabel:    "agent-a",
	}, "", "")
	savePinnedBinding(t, "beta", provider.Binding{
		Name:         nameB,
		SessionID:    "session:gc:beta",
		SessionLabel: "gc:beta",
		WindowID:     "window:RigB",
		BlockID:      "block:agent-b",
		TabLabel:     "RigB",
		PaneLabel:    "agent-b",
	}, "", "")

	fake := &perSessionViews{
		Fake: faketest.New(),
		views: map[rex.SessionID]rex.SessionView{
			"session:gc:alpha": {Label: "gc:alpha", Windows: []rex.Window{{
				ID: "window:RigA", Label: "RigA", Blocks: []rex.Block{{ID: "block:agent-a", Label: "agent-a"}},
			}}},
			"session:gc:beta": {Label: "gc:beta", Windows: []rex.Window{{
				ID: "window:RigB", Label: "RigB", Blocks: []rex.Block{{ID: "block:agent-b", Label: "agent-b"}},
			}}},
		},
	}
	fake.Sessions = []rex.Session{
		{ID: "session:gc:alpha", Label: "gc:alpha"},
		{ID: "session:gc:beta", Label: "gc:beta"},
	}

	cities := provider.CityCandidates()
	cities = append(cities, provider.SiblingCityNamespaces(cities)...)
	if !slices.Equal(cities, []string{"default", "alpha", "beta"}) {
		t.Fatalf("candidates = %v, want [default alpha beta]", cities)
	}
	p := provider.New(provider.CityFromEnv(), fake, namespacesOption(t, cities))

	for _, name := range []string{nameA, nameB} {
		running, err := p.IsRunning(ctx, name)
		if err != nil || !running {
			t.Errorf("IsRunning(%s) = %v, %v; want true, nil", name, running, err)
		}
	}
	bdA, ok := p.Binding(nameA)
	if !ok || bdA.SessionID != "session:gc:alpha" {
		t.Errorf("Binding(%s) = %+v, %v; want alpha's session", nameA, bdA, ok)
	}
	bdB, ok := p.Binding(nameB)
	if !ok || bdB.SessionID != "session:gc:beta" {
		t.Errorf("Binding(%s) = %+v, %v; want beta's session", nameB, bdB, ok)
	}
}

// Unpinned callers have no city, so a name stored in more than one live city
// resolves in candidate order: canonical "default" first, then the sorted
// sibling namespaces. This is the documented relaxation of AC #4; pinned
// callers never read siblings.
func TestUnpinnedLookupDuplicateNamePrecedence(t *testing.T) {
	base := t.TempDir()
	t.Setenv("GC_REX_STATE_DIR", base)
	t.Setenv("GC_REX_CITY", "")
	t.Setenv("GC_CITY_PATH", "")
	t.Setenv("GC_CITY", "")

	const name = "Indigo--dog-1"
	for _, city := range []string{"alpha", "beta"} {
		savePinnedBinding(t, city, provider.Binding{
			Name:         name,
			SessionID:    rex.SessionID("session:gc:" + city),
			SessionLabel: "gc:" + city,
			WindowID:     "window:Indigo",
			BlockID:      "block:dog-1",
			TabLabel:     "Indigo",
			PaneLabel:    "dog-1",
		}, "", "")
	}
	fake := &perSessionViews{
		Fake: faketest.New(),
		views: map[rex.SessionID]rex.SessionView{
			"session:gc:alpha": {Label: "gc:alpha", Windows: []rex.Window{{ID: "window:Indigo", Label: "Indigo", Blocks: []rex.Block{{ID: "block:dog-1", Label: "dog-1"}}}}},
			"session:gc:beta":  {Label: "gc:beta", Windows: []rex.Window{{ID: "window:Indigo", Label: "Indigo", Blocks: []rex.Block{{ID: "block:dog-1", Label: "dog-1"}}}}},
		},
	}
	fake.Sessions = []rex.Session{
		{ID: "session:gc:alpha", Label: "gc:alpha"},
		{ID: "session:gc:beta", Label: "gc:beta"},
	}

	cities := provider.CityCandidates()
	cities = append(cities, provider.SiblingCityNamespaces(cities)...)
	p := provider.New(provider.CityFromEnv(), fake, namespacesOption(t, cities))

	bd, ok := p.Binding(name)
	if !ok || bd.SessionID != "session:gc:alpha" {
		t.Errorf("duplicate name resolved %+v, %v; want the first sibling (alpha)", bd, ok)
	}
}

// TestStopAliasTakesThePrimaryLock proves a stop of an alias also holds the
// lock of the record's own name: a start of the primary must not run beside it
// and have the fresh record cleared. With the primary lock held elsewhere, the
// alias stop waits for it instead of finishing.
func TestStopAliasTakesThePrimaryLock(t *testing.T) {
	base := t.TempDir()
	t.Setenv("GC_REX_STATE_DIR", base)
	t.Setenv("GC_CITY_PATH", "/a/b/acme")
	t.Setenv("GC_CITY", "")
	fake := faketest.New()
	ns, err := provider.NewNamespace([]string{"acme"})
	if err != nil {
		t.Fatal(err)
	}
	p := provider.New("acme", fake, provider.WithNamespace(ns))
	if err := p.Start(context.Background(), "Indigo--dog-1", provider.Config{
		Argv: []string{"sleep", "300"},
		Env:  map[string]string{"GC_ALIAS": "dog"},
	}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	locks, err := filepath.Glob(filepath.Join(base, "*", "locks", "*.lock"))
	if err != nil || len(locks) == 0 {
		t.Fatalf("locks = %v, %v; want the start's name lock", locks, err)
	}
	var nameLock string
	for _, l := range locks {
		if !strings.HasPrefix(filepath.Base(l), "tab-") {
			nameLock = l
		}
	}
	if nameLock == "" {
		t.Fatalf("locks = %v, want a name lock", locks)
	}
	f, err := os.OpenFile(nameLock, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		t.Fatalf("open %s: %v", nameLock, err)
	}
	defer func() { _ = f.Close() }()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatalf("flock %s: %v", nameLock, err)
	}
	defer func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN) }()

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if err := p.Stop(ctx, "dog"); err == nil {
		t.Fatal("Stop(alias) = nil while the primary lock is held, want it to wait for that lock")
	}
}
