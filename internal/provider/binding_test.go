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

const (
	testLabel = "gc:acme"
	testTown  = "Acme"
)

func newTestBinder(t *testing.T, windows ...rex.Window) (*Binder, *MemoryStore, *faketest.Fake) {
	t.Helper()
	fake := faketest.New()
	fake.Sessions = []rex.Session{{ID: "session:acme", Label: testLabel}}
	fake.View = rex.SessionView{Label: testLabel, Windows: windows}
	store := NewMemoryStore()
	return NewBinder(fake, store, testLabel, testTown), store, fake
}

func indigoWindow() rex.Window {
	return rex.Window{ID: "window:Indigo", Label: "Rig: Indigo",
		Blocks: []rex.Block{{ID: "block:dog-1", Label: "dog-1"}}}
}

func indigoBinding() Binding {
	return Binding{Name: "Indigo--dog-1", SessionID: "session:acme", SessionLabel: testLabel,
		WindowID: "window:Indigo", BlockID: "block:dog-1",
		TabLabel: "Rig: Indigo", PaneLabel: "dog-1"}
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
	// No record and no hint labels: this provider cannot prove it started the
	// pane, so the rebuilt binding is an adoption (unknown creation, no gc
	// identity), the same answer Observe gives a recordless pane.
	want := indigoBinding()
	want.Adopted = true
	if err != nil || !ok || !reflect.DeepEqual(got, want) {
		t.Fatalf("Resolve = %+v, %v, %v", got, ok, err)
	}
	if saved, ok := store.Load("Indigo--dog-1"); !ok || !reflect.DeepEqual(saved, got) {
		t.Errorf("saved = %+v, %v", saved, ok)
	}
}

// TestResolveFreshStartEnvFindsEnvDerivedPane proves a lost sidecar record
// still finds a pane whose labels came from the start env: the short pool name
// alone cannot derive the rig tab.
func TestResolveFreshStartEnvFindsEnvDerivedPane(t *testing.T) {
	b, _, _ := newTestBinder(t, rex.Window{ID: "window:Indigo", Label: "Rig: Indigo",
		Blocks: []rex.Block{{ID: "block:dog-1", Label: "dog-1"}}})

	got, ok, err := b.Resolve(context.Background(), ResolveRequest{
		Name: "dog-1", StartEnv: map[string]string{"GC_RIG": "Indigo"}, Mode: ResolveFresh,
	})
	if err != nil || !ok || got.BlockID != "block:dog-1" || got.TabLabel != "Rig: Indigo" {
		t.Fatalf("Resolve = %+v, %v, %v", got, ok, err)
	}
}

// TestResolveReadWritesNothing checks the read-only seam: a stored record
// whose address went stale still finds the pane restored under its labels, and
// the store keeps the record as it was. Proof for AC-3 and BR-9.
func TestResolveReadWritesNothing(t *testing.T) {
	b, store, _ := newTestBinder(t, indigoWindow())
	stale := indigoBinding()
	stale.SessionID = "session:old"
	store.Save(stale)

	got, ok, err := b.ResolveRead(context.Background(), stale.Name, stale)
	if err != nil || !ok || got.BlockID != "block:dog-1" {
		t.Fatalf("ResolveRead = %+v, %v, %v", got, ok, err)
	}
	if !got.CreatedAt.IsZero() {
		t.Errorf("read-only binding claims a creation time: %+v", got)
	}
	saved, found := store.Load(stale.Name)
	if !found {
		t.Fatal("ResolveRead dropped the record")
	}
	if !reflect.DeepEqual(saved, stale) {
		t.Errorf("ResolveRead wrote the store: %+v", saved)
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
	b, store, _ := newTestBinder(t, rex.Window{ID: "window:Indigo", Label: "Rig: Indigo",
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

func TestResolveKeepsStaleBindingWithPendingSurvivors(t *testing.T) {
	b, store, _ := newTestBinder(t)
	stale := indigoBinding()
	stale.Survivors = []Survivor{{PID: 4242, Start: "Mon Nov 17 10:00:00 2026"}}
	store.Save(stale)

	_, ok, err := b.Resolve(context.Background(), ResolveRequest{Name: "Indigo--dog-1", Mode: ResolveFresh})
	if ok || err != nil {
		t.Fatalf("Resolve = %v, %v", ok, err)
	}
	saved, found := store.Load("Indigo--dog-1")
	if !found || len(saved.Survivors) != 1 || saved.Survivors[0].PID != 4242 {
		t.Fatalf("stale record with survivors gone: %+v, %v", saved, found)
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

func TestResolveScanUnavailableKeepsStaleBinding(t *testing.T) {
	b, store, fake := newTestBinder(t)
	store.Save(indigoBinding())
	fake.NotFoundSessions = map[rex.SessionID]bool{"session:acme": true}
	fake.ListSessionsErr = rex.ErrUnavailable

	_, ok, err := b.Resolve(context.Background(), ResolveRequest{Name: "Indigo--dog-1", Mode: ResolveFresh})
	if ok || !errors.Is(err, rex.ErrUnavailable) {
		t.Fatalf("Resolve = %v, %v", ok, err)
	}
	if _, ok := store.Load("Indigo--dog-1"); !ok {
		t.Error("stale binding dropped although the scan failed")
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
	b, store, _ := newTestBinder(t, rex.Window{ID: "window:Indigo", Label: "Rig: Indigo",
		Blocks: []rex.Block{{ID: "block:dog-1", Label: "dog-1"}}})
	legacy := indigoBinding()
	legacy.TabLabel, legacy.PaneLabel = "", ""
	store.Save(legacy)

	got, ok, err := b.Resolve(context.Background(), ResolveRequest{Name: "Indigo--dog-1", Mode: ResolveFresh})
	if err != nil || !ok || got.BlockID != "block:dog-1" {
		t.Fatalf("Resolve = %+v, %v, %v", got, ok, err)
	}
}

// TestResolveLegacyWindowLabel resolves a record written before TabLabel
// existed against a window that still carries the pre-GCREX-72 bare rig label,
// and a town record against a window labelled __town. Neither may lose a live
// pane, which would make Start open a duplicate tab.
func TestResolveLegacyWindowLabel(t *testing.T) {
	for _, tc := range []struct {
		name, windowLabel, blockLabel, paneLabel string
	}{
		{"Indigo--dog-1", "Indigo", "dog-1", "dog-1"},
		{"mayor", legacyTownTab, "mayor", "mayor"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, store, _ := newTestBinder(t, rex.Window{ID: "window:old", Label: tc.windowLabel,
				Blocks: []rex.Block{{ID: "block:old", Label: tc.blockLabel}}})
			legacy := Binding{Name: SessionName(tc.name), SessionID: "session:acme", SessionLabel: testLabel,
				WindowID: "window:old", BlockID: "block:old"}
			store.Save(legacy)

			got, ok, err := b.Resolve(context.Background(), ResolveRequest{Name: legacy.Name, Mode: ResolveFresh})
			if err != nil || !ok || got.BlockID != "block:old" {
				t.Fatalf("Resolve = %+v, %v, %v", got, ok, err)
			}
			if got.TabLabel != "" {
				t.Errorf("TabLabel = %q, want the record left as a legacy record", got.TabLabel)
			}
		})
	}
}

// TestScanFindsLegacyWindowLabel covers the lost-record path Start takes: no
// stored binding, a live pane under the pre-GCREX-72 label.
func TestScanFindsLegacyWindowLabel(t *testing.T) {
	for _, tc := range []struct {
		name, windowLabel, blockLabel string
		// rig is the start env's GC_RIG. The env derives the same labels
		// Start used to supply by hand, so the pre-GCREX-72 window label
		// must still be found from legacyTabLabelFor(name, env).
		rig string
	}{
		{"Indigo--dog-1", "Indigo", "dog-1", ""},
		{"mayor", legacyTownTab, "mayor", ""},
		{"Indigo--dog-1", "Indigo", "dog-1", "Indigo"},
		{"mayor", "my-city", "mayor", "my-city"},
		{"dog-1", "Indigo", "dog-1", "Indigo"},
		{"Indigo--dog-1", "other-rig", "dog-1", "other-rig"},
	} {
		t.Run(tc.name+"/rig="+tc.rig, func(t *testing.T) {
			b, _, _ := newTestBinder(t, rex.Window{ID: "window:old", Label: tc.windowLabel,
				Blocks: []rex.Block{{ID: "block:old", Label: tc.blockLabel}}})

			got, ok, err := b.Resolve(context.Background(), ResolveRequest{
				Name: SessionName(tc.name), StartEnv: map[string]string{"GC_RIG": tc.rig}, Mode: ResolveFresh,
			})
			if err != nil || !ok || got.BlockID != "block:old" || got.TabLabel != tc.windowLabel {
				t.Fatalf("Resolve = %+v, %v, %v", got, ok, err)
			}
		})
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
	b, _, _ := newTestBinder(t, rex.Window{ID: "window:__town", Label: testTown,
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
		got.LaunchMode != stale.LaunchMode {
		t.Errorf("rebuilt binding lost hint fields: %+v", got)
	}
	// The stale record's creation time is not the replacement pane's: an
	// adopted pane reports its own (unknown) creation, so Observe keeps
	// answering the same `created` a read-only scan did (CodeRabbit fix).
	if !got.CreatedAt.IsZero() || !got.Adopted {
		t.Errorf("rebuilt binding kept the retired box's creation: %+v", got)
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
	b, store, _ := newTestBinder(t, rex.Window{ID: "window:Indigo", Label: "Rig: Indigo",
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

// TestScanFindsPaneInLegacySession is the CodeRabbit fix for binding.go: the
// canonical session exists, but the replacement pane lives only in a legacy
// accepted session. Absence from the canonical session is not absence.
func TestScanFindsPaneInLegacySession(t *testing.T) {
	fake := faketest.New()
	fake.Sessions = []rex.Session{
		{ID: "session:acme", Label: testLabel},
		{ID: "session:legacy", Label: "gc:legacy"},
	}
	fake.Views = map[rex.SessionID]rex.SessionView{
		"session:acme":   {Label: testLabel},
		"session:legacy": {Label: "gc:legacy", Windows: []rex.Window{indigoWindow()}},
	}
	b := NewBinderWithLabels(fake, NewMemoryStore(), []string{testLabel, "gc:legacy"}, testTown)
	got, ok, err := b.Resolve(context.Background(), ResolveRequest{Name: "Indigo--dog-1", Mode: ResolveFresh})
	if err != nil || !ok {
		t.Fatalf("Resolve = %+v, %v, %v; want the legacy session's pane", got, ok, err)
	}
	if got.BlockID != "block:dog-1" || got.SessionID != "session:legacy" {
		t.Errorf("Resolve = %+v, want block:dog-1 in session:legacy", got)
	}
}

// TestRecordIfCurrentKeepsAReplacementBinding proves a stale survivor write
// cannot replace a binding a concurrent start installed under the same name.
func TestRecordIfCurrentKeepsAReplacementBinding(t *testing.T) {
	binder, store, _ := newTestBinder(t)
	old := indigoBinding()
	if err := store.Write(old.Name, old); err != nil {
		t.Fatal(err)
	}
	newer := old
	newer.BlockID = "block:dog-2"
	if err := store.Write(newer.Name, newer); err != nil {
		t.Fatal(err)
	}
	stale := old
	stale.Survivors = []Survivor{{PID: 4242, Name: "worker"}}
	wrote, err := binder.RecordIfCurrent(stale)
	if err != nil {
		t.Fatalf("RecordIfCurrent: %v", err)
	}
	if wrote {
		t.Fatal("RecordIfCurrent wrote a stale survivor record over a replacement binding")
	}
	got, ok := store.Load(newer.Name)
	if !ok {
		t.Fatal("replacement binding is gone")
	}
	if got.BlockID != newer.BlockID {
		t.Fatalf("stored binding = %q, want the replacement %q", got.BlockID, newer.BlockID)
	}
	if len(got.Survivors) != 0 {
		t.Fatalf("stored binding carries stale survivors: %v", got.Survivors)
	}
}

// TestRecordIfCurrentKeepsAnotherLaunchesProvisionalBinding proves the
// provisional exception belongs to the launch that wrote the record: a stale
// survivor write from an earlier attempt cannot replace a newer start's
// pending record, which would then reject that start's own write and leave its
// surviving descendants with no record.
func TestRecordIfCurrentKeepsAnotherLaunchesProvisionalBinding(t *testing.T) {
	binder, store, _ := newTestBinder(t)
	provisional := indigoBinding()
	provisional.BlockID = ""
	provisional.Attempt = "attempt-new"
	if err := store.Write(provisional.Name, provisional); err != nil {
		t.Fatal(err)
	}
	stale := indigoBinding()
	stale.Attempt = "attempt-old"
	stale.Survivors = []Survivor{{PID: 4242, Name: "worker"}}
	wrote, err := binder.RecordIfCurrent(stale)
	if err != nil {
		t.Fatalf("RecordIfCurrent: %v", err)
	}
	if wrote {
		t.Fatal("RecordIfCurrent wrote a stale survivor record over another launch's provisional binding")
	}
	got, ok := store.Load(provisional.Name)
	if !ok || got.Attempt != "attempt-new" || got.BlockID != "" {
		t.Fatalf("stored binding = %+v, want the new launch's provisional record", got)
	}
	if len(got.Survivors) != 0 {
		t.Fatalf("provisional binding carries stale survivors: %v", got.Survivors)
	}
	// The launch that wrote the provisional record still replaces it.
	own := provisional
	own.BlockID = "block:dog-2"
	wrote, err = binder.RecordIfCurrent(own)
	if err != nil || !wrote {
		t.Fatalf("RecordIfCurrent for the provisional record's own launch = %v, %v; want true, nil", wrote, err)
	}
}

func TestSaveKeepingSurvivorsKeepsTheStoredSet(t *testing.T) {
	binder, store, _ := newTestBinder(t)
	stored := indigoBinding()
	stored.Survivors = []Survivor{{PID: 300}}
	store.Save(stored)

	fresh := indigoBinding() // the scan snapshot, taken before the stop wrote 300
	fresh.BlockID = "block:dog-1b"
	if err := binder.saveKeepingSurvivors(fresh); err != nil {
		t.Fatalf("saveKeepingSurvivors = %v", err)
	}
	saved, _ := store.Load(fresh.Name)
	if saved.BlockID != "block:dog-1b" || len(saved.Survivors) != 1 || saved.Survivors[0].PID != 300 {
		t.Fatalf("saved = %+v; want the fresh record with pid 300", saved)
	}
}

func TestSaveKeepingSurvivorsKeepsAnotherLaunchesProvisional(t *testing.T) {
	binder, store, _ := newTestBinder(t)
	provisional := indigoBinding()
	provisional.BlockID = ""
	provisional.Attempt = "launch-a"
	store.Save(provisional)

	scanned := indigoBinding() // a scan for the same name, from another launch
	scanned.BlockID = "block:dog-1b"
	scanned.Attempt = "launch-b"
	if err := binder.saveKeepingSurvivors(scanned); err != nil {
		t.Fatalf("saveKeepingSurvivors = %v", err)
	}
	saved, _ := store.Load(scanned.Name)
	if saved.Attempt != "launch-a" || saved.BlockID != "" {
		t.Fatalf("saved = %+v; want launch-a's provisional record", saved)
	}
}

func TestSaveKeepingSurvivorsReplacesAFinishedRecord(t *testing.T) {
	binder, store, _ := newTestBinder(t)
	finished := indigoBinding() // a start that committed: BlockID is set
	finished.Attempt = "launch-a"
	store.Save(finished)

	scanned := indigoBinding()
	scanned.BlockID = "block:dog-1b"
	if err := binder.saveKeepingSurvivors(scanned); err != nil {
		t.Fatalf("saveKeepingSurvivors = %v", err)
	}
	saved, _ := store.Load(scanned.Name)
	if saved.BlockID != "block:dog-1b" {
		t.Fatalf("saved = %+v; want the scanned record", saved)
	}
}

func TestDropStaleIfUnchanged(t *testing.T) {
	withSurvivor := indigoBinding()
	withSurvivor.Survivors = []Survivor{{PID: 300}}
	replaced := indigoBinding()
	replaced.BlockID = "block:replaced"

	for _, tc := range []struct {
		name    string
		current Binding
		want    bool // true when the record is dropped
	}{
		{"unchanged", indigoBinding(), true},
		{"survivor recorded after the read", withSurvivor, false},
		{"replaced by another start", replaced, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, store, _ := newTestBinder(t)
			store.Save(tc.current)
			b.dropStaleIfUnchanged(indigoBinding()) // the snapshot the resolve read
			if _, found := store.Load(tc.current.Name); found != !tc.want {
				t.Fatalf("record found = %v, want dropped = %v", found, tc.want)
			}
		})
	}
}
