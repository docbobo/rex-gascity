package provider_test

import (
	"context"
	"path/filepath"
	"slices"
	"testing"

	"github.com/docbobo/rex-gascity/internal/provider"
	"github.com/docbobo/rex-gascity/internal/rex"
	"github.com/docbobo/rex-gascity/internal/rex/faketest"
)

// CityFromEnv must not depend on the incidental GC_CITY_PATH/GC_CITY that only
// some gc callers carry: start (a bare launchd controller) and the lookup ops
// (a session or nudge-poller process) have to agree on one gc:<city> session
// and one state directory. GC_REX_CITY stays the explicit per-city pin.
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
		t.Setenv("GC_REX_CITY", tc.rex)
		t.Setenv("GC_CITY_PATH", tc.path)
		t.Setenv("GC_CITY", tc.city)
		if got := provider.CityFromEnv(); got != tc.want {
			t.Errorf("rex=%q path=%q city=%q: CityFromEnv = %q, want %q", tc.rex, tc.path, tc.city, got, tc.want)
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
		t.Setenv("GC_REX_CITY", tc.rex)
		t.Setenv("GC_CITY_PATH", tc.path)
		t.Setenv("GC_CITY", tc.city)
		if got := provider.CityCandidates(); !slices.Equal(got, tc.want) {
			t.Errorf("rex=%q path=%q city=%q: CityCandidates = %v, want %v", tc.rex, tc.path, tc.city, got, tc.want)
		}
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

	p := provider.New("default", fake, provider.WithCityNamespaces(provider.CityCandidates()))

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
	pA := provider.New("a", fake, provider.WithCityNamespaces(provider.CityCandidates()))
	if err := pA.Start(ctx, "Indigo--dog-1", provider.Config{Argv: []string{"sh"}}); err != nil {
		t.Fatalf("start in city a: %v", err)
	}

	t.Setenv("GC_REX_CITY", "b")
	pB := provider.New("b", fake, provider.WithCityNamespaces(provider.CityCandidates()))
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
	poller := provider.New(provider.CityFromEnv(), fake, provider.WithCityNamespaces(provider.CityCandidates()))
	if err := poller.Start(ctx, name, provider.Config{Argv: []string{"sh"}}); err != nil {
		t.Fatalf("poller Start: %v", err)
	}

	// The bare launchd controller carries no city env and resolves the
	// canonical namespace.
	t.Setenv("GC_CITY_PATH", "")
	controller := provider.New(provider.CityFromEnv(), fake, provider.WithCityNamespaces(provider.CityCandidates()))

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
