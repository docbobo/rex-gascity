package provider

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/docbobo/rex-gascity/internal/rex"
	"github.com/docbobo/rex-gascity/internal/rex/faketest"
)

const testLabel = "gc:acme"

func newTestBinder(t *testing.T, windows ...rex.Window) (*Binder, *MemoryStore, *faketest.Fake) {
	t.Helper()
	fake := faketest.New()
	fake.Sessions = []rex.Session{{ID: "session:acme", Label: testLabel}}
	fake.View = rex.SessionView{Label: testLabel, Windows: windows}
	store := NewMemoryStore()
	return NewBinder(fake, store, testLabel), store, fake
}

func indigoWindow() rex.Window {
	return rex.Window{ID: "window:Indigo", Label: "Indigo",
		Blocks: []rex.Block{{ID: "block:dog-1", Label: "dog-1"}}}
}

func indigoBinding() Binding {
	return Binding{Name: "Indigo--dog-1", SessionID: "session:acme", SessionLabel: testLabel,
		WindowID: "window:Indigo", BlockID: "block:dog-1",
		TabLabel: "Indigo", PaneLabel: "dog-1"}
}

func TestResolveCacheHit(t *testing.T) {
	b, store, fake := newTestBinder(t, indigoWindow())
	want := indigoBinding()
	store.Save(want)

	got, ok, err := b.Resolve(context.Background(), ResolveRequest{Name: want.Name, Mode: ResolveFresh})
	if err != nil || !ok || !reflect.DeepEqual(got, want) {
		t.Fatalf("Resolve = %+v, %v, %v", got, ok, err)
	}
	if fake.ListSessionsCalls != 0 || fake.SessionViewCalls != 1 {
		t.Errorf("calls: list=%d view=%d", fake.ListSessionsCalls, fake.SessionViewCalls)
	}
}

func TestResolveScanFallback(t *testing.T) {
	b, store, _ := newTestBinder(t, indigoWindow())

	got, ok, err := b.Resolve(context.Background(), ResolveRequest{Name: "Indigo--dog-1", Mode: ResolveFresh})
	if err != nil || !ok || !reflect.DeepEqual(got, indigoBinding()) {
		t.Fatalf("Resolve = %+v, %v, %v", got, ok, err)
	}
	if saved, ok := store.Load("Indigo--dog-1"); !ok || !reflect.DeepEqual(saved, got) {
		t.Errorf("saved = %+v, %v", saved, ok)
	}
}

func TestResolveNotFound(t *testing.T) {
	b, _, _ := newTestBinder(t, indigoWindow())
	_, ok, err := b.Resolve(context.Background(), ResolveRequest{Name: "Indigo--cat", Mode: ResolveFresh})
	if ok || err != nil {
		t.Fatalf("Resolve = %v, %v", ok, err)
	}
}

func TestResolveStaleBinding(t *testing.T) {
	b, store, _ := newTestBinder(t, rex.Window{ID: "window:Indigo", Label: "Indigo",
		Blocks: []rex.Block{{ID: "block:dog-1b", Label: "dog-1"}}})
	store.Save(indigoBinding())

	got, ok, err := b.Resolve(context.Background(), ResolveRequest{Name: "Indigo--dog-1", Mode: ResolveFresh})
	if err != nil || !ok || got.BlockID != "block:dog-1b" {
		t.Fatalf("Resolve = %+v, %v, %v", got, ok, err)
	}
	if saved, _ := store.Load("Indigo--dog-1"); saved.BlockID != "block:dog-1b" {
		t.Errorf("store not refreshed: %+v", saved)
	}
}

func TestResolveStaleBindingGone(t *testing.T) {
	b, store, _ := newTestBinder(t)
	store.Save(indigoBinding())

	_, ok, err := b.Resolve(context.Background(), ResolveRequest{Name: "Indigo--dog-1", Mode: ResolveFresh})
	if ok || err != nil {
		t.Fatalf("Resolve = %v, %v", ok, err)
	}
	if _, ok := store.Load("Indigo--dog-1"); ok {
		t.Error("stale binding not deleted")
	}
}

func TestResolveTransportErrorKeepsBinding(t *testing.T) {
	b, store, fake := newTestBinder(t, indigoWindow())
	store.Save(indigoBinding())
	boom := errors.New("boom")
	fake.SessionViewErr = boom

	_, ok, err := b.Resolve(context.Background(), ResolveRequest{Name: "Indigo--dog-1", Mode: ResolveFresh})
	if ok || !errors.Is(err, boom) {
		t.Fatalf("Resolve = %v, %v", ok, err)
	}
	if _, ok := store.Load("Indigo--dog-1"); !ok {
		t.Error("binding was cleared on transport error")
	}
}

func TestResolveStaleSessionIDRescansOnNotFound(t *testing.T) {
	b, store, fake := newTestBinder(t, indigoWindow())
	fake.Sessions = []rex.Session{{ID: "session:new", Label: testLabel}}
	fake.NotFoundSessions = map[rex.SessionID]bool{"session:old": true}
	old := indigoBinding()
	old.SessionID = "session:old"
	store.Save(old)

	got, ok, err := b.Resolve(context.Background(), ResolveRequest{Name: "Indigo--dog-1", Mode: ResolveFresh})
	if err != nil || !ok || got.BlockID != "block:dog-1" || got.SessionID != "session:new" {
		t.Fatalf("Resolve = %+v, %v, %v", got, ok, err)
	}
	if saved, _ := store.Load("Indigo--dog-1"); saved.SessionID != "session:new" {
		t.Errorf("store not refreshed: %+v", saved)
	}
}

func TestResolveForeignSessionLabelIgnored(t *testing.T) {
	b, store, fake := newTestBinder(t, indigoWindow())
	foreign := indigoBinding()
	foreign.SessionID = "session:other"
	foreign.SessionLabel = "gc:other"
	store.Save(foreign)

	got, ok, err := b.Resolve(context.Background(), ResolveRequest{Name: "Indigo--dog-1", Mode: ResolveFresh})
	if err != nil || !ok || got.SessionID != "session:acme" || got.SessionLabel != testLabel {
		t.Fatalf("Resolve = %+v, %v, %v", got, ok, err)
	}
	if fake.ListSessionsCalls != 1 {
		t.Errorf("ListSessions calls = %d, want a rescan", fake.ListSessionsCalls)
	}
	if saved, _ := store.Load("Indigo--dog-1"); saved.SessionLabel != testLabel {
		t.Errorf("store not refreshed: %+v", saved)
	}
}

// TestResolveLegacyBindingUsesNameLabels resolves a record written before
// TabLabel/PaneLabel existed: the labels fall back to the name split.
func TestResolveLegacyBindingUsesNameLabels(t *testing.T) {
	b, store, _ := newTestBinder(t, indigoWindow())
	legacy := indigoBinding()
	legacy.TabLabel, legacy.PaneLabel = "", ""
	store.Save(legacy)

	got, ok, err := b.Resolve(context.Background(), ResolveRequest{Name: "Indigo--dog-1", Mode: ResolveFresh})
	if err != nil || !ok || got.BlockID != "block:dog-1" {
		t.Fatalf("Resolve = %+v, %v, %v", got, ok, err)
	}
}

// TestResolveAliasFindsPrimary checks a name stored only as an alias resolves
// to the primary binding without scanning by the alias's derived labels.
func TestResolveAliasFindsPrimary(t *testing.T) {
	b, store, _ := newTestBinder(t, rex.Window{ID: "window:Rig", Label: "Rig",
		Blocks: []rex.Block{{ID: "block:dog-1", Label: "dog-1"}}})
	primary := indigoBinding()
	primary.WindowID = "window:Rig"
	primary.TabLabel, primary.PaneLabel = "Rig", "dog-1"
	primary.Aliases = []SessionName{"Rig--dog-1"}
	store.Save(primary)

	got, ok, err := b.Resolve(context.Background(), ResolveRequest{Name: "Rig--dog-1", Mode: ResolveFresh})
	if err != nil || !ok || got.Name != "Indigo--dog-1" || got.BlockID != "block:dog-1" {
		t.Fatalf("Resolve(alias) = %+v, %v, %v", got, ok, err)
	}
	if b, ok := store.Load("Indigo--dog-1"); !ok || b.Name != "Indigo--dog-1" {
		t.Errorf("primary record missing: %+v, %v", b, ok)
	}
}

func TestForgetAliasClearsPrimary(t *testing.T) {
	b, store, _ := newTestBinder(t, rex.Window{ID: "window:Rig", Label: "Rig",
		Blocks: []rex.Block{{ID: "block:dog-1", Label: "dog-1"}}})
	primary := indigoBinding()
	primary.WindowID = "window:Rig"
	primary.Aliases = []SessionName{"Rig--dog-1"}
	store.Save(primary)

	b.Forget("Rig--dog-1")
	if _, ok := store.Load("Indigo--dog-1"); ok {
		t.Error("Forget(alias) left the primary record")
	}
}

func TestResolveTownAgent(t *testing.T) {
	b, _, _ := newTestBinder(t, rex.Window{ID: "window:__town", Label: "__town",
		Blocks: []rex.Block{{ID: "block:mayor", Label: "mayor"}}})

	got, ok, err := b.Resolve(context.Background(), ResolveRequest{Name: "mayor", Mode: ResolveFresh})
	if err != nil || !ok || got.WindowID != "window:__town" || got.BlockID != "block:mayor" {
		t.Fatalf("Resolve = %+v, %v, %v", got, ok, err)
	}
}

func TestSanitizeQualified(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"my-project/core.control-dispatcher", "my-project--core__control-dispatcher"},
		{"my-project/pi", "my-project--pi"},
		{"mayor", "mayor"},
	} {
		if got := sanitizeQualified(tc.in); got != tc.want {
			t.Errorf("sanitizeQualified(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestAliasesFor(t *testing.T) {
	// All four sources distinct so a mutation that drops one is caught.
	env := map[string]string{
		"GC_RIG":      "rig",
		"GC_TEMPLATE": "rig/template",
		"GC_ALIAS":    "rig/alias",
		"GC_AGENT":    "rig/owner",
	}
	got := aliasesFor("mc-wisp-1", env)
	want := []SessionName{"rig--mc-wisp-1", "rig--owner", "rig--template", "rig--alias"}
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("aliasesFor = %v, want %v", got, want)
	}

	// A town agent with no rig keeps only the qualified sources; the primary
	// name and a duplicate template alias are dropped.
	got = aliasesFor("mayor", map[string]string{"GC_TEMPLATE": "mayor", "GC_ALIAS": "mayor"})
	if len(got) != 0 {
		t.Errorf("aliasesFor(mayor) = %v, want none", got)
	}
}

// TestScanRebuiltBindingKeepsHintFields rediscovers a pane by label after its
// stored address went stale. The rebuilt binding must carry the hint's launch
// details, or a later Stop loses WorkDir and skips the drain diagnostic.
func TestScanRebuiltBindingKeepsHintFields(t *testing.T) {
	b, store, _ := newTestBinder(t, indigoWindow())
	stale := indigoBinding()
	stale.SessionID = "session:old"
	stale.WindowID = "window:old"
	stale.BlockID = "block:old"
	stale.WorkDir = "/work/dog-1"
	stale.GCBin = ".gc-wrapped"
	stale.LaunchMode = "split"
	stale.CreatedAt = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	store.Save(stale)

	got, ok, err := b.Resolve(context.Background(), ResolveRequest{Name: "Indigo--dog-1", Mode: ResolveFresh})
	if err != nil || !ok || got.BlockID != "block:dog-1" {
		t.Fatalf("Resolve = %+v, %v, %v", got, ok, err)
	}
	if got.WorkDir != stale.WorkDir || got.GCBin != stale.GCBin ||
		got.LaunchMode != stale.LaunchMode || !got.CreatedAt.Equal(stale.CreatedAt) {
		t.Errorf("rebuilt binding lost hint fields: %+v", got)
	}
	if saved, _ := store.Load("Indigo--dog-1"); saved.WorkDir != stale.WorkDir {
		t.Errorf("saved WorkDir = %q, want %q", saved.WorkDir, stale.WorkDir)
	}
}

func rigDogBinding() Binding {
	b := indigoBinding()
	b.Aliases = []SessionName{"Rig--dog-1"}
	return b
}

// TestBinderCachedPrimaryAndAlias pins Cached: the primary record wins, an alias
// resolves to its primary, an unknown name is absent, and Rex is never called.
func TestBinderCachedPrimaryAndAlias(t *testing.T) {
	b, store, fake := newTestBinder(t, indigoWindow())
	primary := rigDogBinding()
	store.Save(primary)

	for _, name := range []SessionName{"Indigo--dog-1", "Rig--dog-1"} {
		got, ok, _ := b.Resolve(context.Background(), ResolveRequest{Name: name})
		if !ok || !reflect.DeepEqual(got, primary) {
			t.Errorf("Cached(%q) = %+v, %v", name, got, ok)
		}
	}
	if _, ok, _ := b.Resolve(context.Background(), ResolveRequest{Name: "Indigo--cat"}); ok {
		t.Error("Cached(unknown) found a binding")
	}
	if fake.ListSessionsCalls != 0 || fake.SessionViewCalls != 0 {
		t.Errorf("Cached contacted Rex: list=%d view=%d", fake.ListSessionsCalls, fake.SessionViewCalls)
	}
}

// TestBinderCachedPrefersPrimaryRecord checks a record stored under the name
// beats another primary that lists the same name as an alias.
func TestBinderCachedPrefersPrimaryRecord(t *testing.T) {
	b, store, _ := newTestBinder(t)
	other := indigoBinding()
	other.Name = "Indigo--dog-2"
	other.Aliases = []SessionName{"Indigo--dog-1"}
	store.Save(other)
	own := indigoBinding()
	store.Save(own)

	got, ok, _ := b.Resolve(context.Background(), ResolveRequest{Name: "Indigo--dog-1"})
	if !ok || got.Name != "Indigo--dog-1" {
		t.Errorf("Cached = %+v, %v", got, ok)
	}
}

// TestBinderLiveAliasRevalidates checks Live finds a live pane through an alias
// and returns the primary binding after one session-view check.
func TestBinderLiveAliasRevalidates(t *testing.T) {
	b, store, fake := newTestBinder(t, indigoWindow())
	primary := rigDogBinding()
	store.Save(primary)

	got, ok, err := b.Resolve(context.Background(), ResolveRequest{Name: "Rig--dog-1", Mode: ResolveLive})
	if err != nil || !ok || !reflect.DeepEqual(got, primary) {
		t.Fatalf("Live = %+v, %v, %v", got, ok, err)
	}
	if fake.ListSessionsCalls != 0 || fake.SessionViewCalls != 1 {
		t.Errorf("calls: list=%d view=%d", fake.ListSessionsCalls, fake.SessionViewCalls)
	}
}

// TestBinderLiveRescansStaleAddress checks a stale block id is rediscovered by
// label, the record refreshed, and the new pane judged alive.
func TestBinderLiveRescansStaleAddress(t *testing.T) {
	b, store, _ := newTestBinder(t, rex.Window{ID: "window:Indigo", Label: "Indigo",
		Blocks: []rex.Block{{ID: "block:dog-1b", Label: "dog-1"}}})
	store.Save(indigoBinding())

	got, ok, err := b.Resolve(context.Background(), ResolveRequest{Name: "Indigo--dog-1", Mode: ResolveLive})
	if err != nil || !ok || got.BlockID != "block:dog-1b" {
		t.Fatalf("Live = %+v, %v, %v", got, ok, err)
	}
	if saved, _ := store.Load("Indigo--dog-1"); saved.BlockID != "block:dog-1b" {
		t.Errorf("store not refreshed: %+v", saved)
	}
}

// TestBinderLiveDeletesGoneRecord checks a record whose pane is gone is deleted
// and reported not alive without an error.
func TestBinderLiveDeletesGoneRecord(t *testing.T) {
	b, store, _ := newTestBinder(t)
	store.Save(indigoBinding())

	_, ok, err := b.Resolve(context.Background(), ResolveRequest{Name: "Indigo--dog-1", Mode: ResolveLive})
	if ok || err != nil {
		t.Fatalf("Live = %v, %v", ok, err)
	}
	if _, ok := store.Load("Indigo--dog-1"); ok {
		t.Error("stale record not deleted")
	}
}

// TestBinderLiveNoCacheScans checks the no-candidate fallback scans by label and
// returns the live pane it finds.
func TestBinderLiveNoCacheScans(t *testing.T) {
	b, store, _ := newTestBinder(t, indigoWindow())

	got, ok, err := b.Resolve(context.Background(), ResolveRequest{Name: "Indigo--dog-1", Mode: ResolveLive})
	if err != nil || !ok || got.BlockID != "block:dog-1" {
		t.Fatalf("Live = %+v, %v, %v", got, ok, err)
	}
	if _, ok := store.Load("Indigo--dog-1"); !ok {
		t.Error("scan did not save the binding")
	}
	if _, ok, err := b.Resolve(context.Background(), ResolveRequest{Name: "Indigo--cat", Mode: ResolveLive}); ok || err != nil {
		t.Errorf("Live(unknown) = %v, %v", ok, err)
	}
}

// TestBinderLiveExitedPane checks a pane whose program exited, or whose block
// Rex no longer knows, is not alive, and its record is kept.
func TestBinderLiveExitedPane(t *testing.T) {
	b, store, fake := newTestBinder(t, indigoWindow())
	store.Save(indigoBinding())
	fake.Processes = map[rex.BlockID]rex.ProcessInfo{"block:dog-1": {LastExit: &rex.ExitStatus{ExitCode: 1}}}

	if _, ok, err := b.Resolve(context.Background(), ResolveRequest{Name: "Indigo--dog-1", Mode: ResolveLive}); ok || err != nil {
		t.Fatalf("Live(exited) = %v, %v", ok, err)
	}
	fake.Processes = nil
	fake.ProcessErr = rex.ErrNotFound
	if _, ok, err := b.Resolve(context.Background(), ResolveRequest{Name: "Indigo--dog-1", Mode: ResolveLive}); ok || err != nil {
		t.Fatalf("Live(closed block) = %v, %v", ok, err)
	}
	if _, ok := store.Load("Indigo--dog-1"); !ok {
		t.Error("record dropped for a dead-but-present pane")
	}
}

// TestBinderLiveTransportErrors checks view and process errors are returned and
// leave the record alone.
func TestBinderLiveTransportErrors(t *testing.T) {
	boom := errors.New("boom")
	b, store, fake := newTestBinder(t, indigoWindow())
	store.Save(indigoBinding())

	fake.SessionViewErr = boom
	if _, ok, err := b.Resolve(context.Background(), ResolveRequest{Name: "Indigo--dog-1", Mode: ResolveLive}); ok || !errors.Is(err, boom) {
		t.Fatalf("Live(view err) = %v, %v", ok, err)
	}
	fake.SessionViewErr = nil
	fake.ProcessErr = boom
	if _, ok, err := b.Resolve(context.Background(), ResolveRequest{Name: "Indigo--dog-1", Mode: ResolveLive}); ok || !errors.Is(err, boom) {
		t.Fatalf("Live(process err) = %v, %v", ok, err)
	}
	if _, ok := store.Load("Indigo--dog-1"); !ok {
		t.Error("record cleared on transport error")
	}
}

// TestBinderLiveSkipsOtherNameRecord checks a record stored under name that
// answers to a different name is not trusted.
func TestBinderLiveSkipsOtherNameRecord(t *testing.T) {
	b, store, _ := newTestBinder(t, indigoWindow())
	bd := indigoBinding()
	bd.Name = "Indigo--other"
	// A corrupt key: the store hands back a record for another name.
	store.t.put("Indigo--dog-1", bd)

	if _, ok, err := b.Resolve(context.Background(), ResolveRequest{Name: "Indigo--dog-1", Mode: ResolveLive}); ok || err != nil {
		t.Fatalf("Live = %v, %v", ok, err)
	}
}
