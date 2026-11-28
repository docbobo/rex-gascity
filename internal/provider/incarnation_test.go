package provider_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/docbobo/rex-gascity/internal/provider"
	"github.com/docbobo/rex-gascity/internal/rex"
	"github.com/docbobo/rex-gascity/internal/rex/faketest"
)

// TestStopClearsAllMetaAndActivity proves that Stop retires the whole
// incarnation: every meta key for the name and its activity record go, not
// just the identity keys seedStartIdentity wrote. herdr-core deletes the meta
// file and activity with the incarnation (parity doc ### stop row 4).
func TestStopClearsAllMetaAndActivity(t *testing.T) {
	ctx := context.Background()
	fake := faketest.New()
	act := provider.NewMemoryActivityStore()
	p := provider.New("acme", fake, provider.WithActivityStore(act))
	name := "Indigo--dog-1"

	if err := p.Start(ctx, name, provider.Config{Argv: []string{"claude"}}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	keys := []string{"suspended", "GC_DRAIN_ACK", "GC_READY_PROMPT_PREFIX", "GC_USER_KEY"}
	for _, key := range keys {
		if err := p.SetMeta(ctx, name, key, "stale"); err != nil {
			t.Fatalf("SetMeta %s: %v", key, err)
		}
	}
	fake.Formats = map[rex.BlockID]string{"block:dog-1": "screen"}
	if _, err := p.GetLastActivity(ctx, name); err != nil {
		t.Fatalf("GetLastActivity: %v", err)
	}
	if _, ok := act.Load(provider.SessionName(name)); !ok {
		t.Fatal("activity not recorded before Stop")
	}

	if err := p.Stop(ctx, name); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	for _, key := range keys {
		if got, _ := p.GetMeta(ctx, name, key); got != "" {
			t.Errorf("meta %s = %q after Stop, want empty", key, got)
		}
	}
	if _, ok := act.Load(provider.SessionName(name)); ok {
		t.Error("activity record survived Stop")
	}
}

// TestStartAfterStopObservesNoMetaOrActivity proves a fresh incarnation of the
// same name sees nothing from the retired one, across provider processes: the
// on-disk meta and activity sidecars are gone before the restart reads them.
func TestStartAfterStopObservesNoMetaOrActivity(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	fake := faketest.New()
	name := "Indigo--dog-1"

	p1 := provider.New("acme", fake, provider.WithNamespace(provider.NewNamespaceForDirs(dir)))
	if err := p1.Start(ctx, name, provider.Config{Argv: []string{"claude"}}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	for _, key := range []string{"suspended", "GC_DRAIN_ACK"} {
		if err := p1.SetMeta(ctx, name, key, "stale"); err != nil {
			t.Fatalf("SetMeta %s: %v", key, err)
		}
	}
	fake.Formats = map[rex.BlockID]string{"block:dog-1": "screen"}
	if _, err := p1.GetLastActivity(ctx, name); err != nil {
		t.Fatalf("GetLastActivity: %v", err)
	}
	if err := p1.Stop(ctx, name); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	// The retired incarnation's sidecars are gone on disk.
	meta := provider.NewFileMetaStore(filepath.Join(dir, "meta"))
	if _, ok, err := meta.Get(provider.SessionName(name), "suspended"); err != nil || ok {
		t.Errorf("suspended in meta store after Stop: set=%v err=%v", ok, err)
	}
	act := provider.NewFileActivityStore(filepath.Join(dir, "activity"))
	if _, ok := act.Load(provider.SessionName(name)); ok {
		t.Error("activity record survived Stop on disk")
	}

	// A fresh provider and start of the same name observes nothing stale.
	p2 := provider.New("acme", fake, provider.WithNamespace(provider.NewNamespaceForDirs(dir)))
	if err := p2.Start(ctx, name, provider.Config{Argv: []string{"claude"}}); err != nil {
		t.Fatalf("restart: %v", err)
	}
	for _, key := range []string{"suspended", "GC_DRAIN_ACK"} {
		if got, _ := p2.GetMeta(ctx, name, key); got != "" {
			t.Errorf("fresh start sees %s = %q, want empty", key, got)
		}
	}
	if _, ok := act.Load(provider.SessionName(name)); ok {
		t.Error("fresh start observes a retained activity record")
	}
}

// TestStopWithoutPaneClearsAllMetaAndActivity proves the no-pane branch of
// Stop retires the whole incarnation too: a drained worker whose pane already
// vanished must not leave its meta keys or activity record for the next agent
// of the same name. The live-pane branch is covered by
// TestStopClearsAllMetaAndActivity; this pins the `!ok` path in stop
// (parity doc ### stop row 4, AC#1).
func TestStopWithoutPaneClearsAllMetaAndActivity(t *testing.T) {
	ctx := context.Background()
	fake := faketest.New()
	act := provider.NewMemoryActivityStore()
	p := provider.New("acme", fake, provider.WithActivityStore(act))
	name := "Indigo--dog-1"

	if err := p.Start(ctx, name, provider.Config{Argv: []string{"claude"}}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	keys := []string{"suspended", "GC_DRAIN_ACK", "GC_READY_PROMPT_PREFIX", "GC_USER_KEY"}
	for _, key := range keys {
		if err := p.SetMeta(ctx, name, key, "stale"); err != nil {
			t.Fatalf("SetMeta %s: %v", key, err)
		}
	}
	fake.Formats = map[rex.BlockID]string{"block:dog-1": "screen"}
	if _, err := p.GetLastActivity(ctx, name); err != nil {
		t.Fatalf("GetLastActivity: %v", err)
	}
	if _, ok := act.Load(provider.SessionName(name)); !ok {
		t.Fatal("activity not recorded before Stop")
	}

	// The worker's process exited, so its pane is gone and Stop takes the
	// no-pane branch.
	fake.View.Windows = nil
	if err := p.Stop(ctx, name); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	for _, key := range keys {
		if got, _ := p.GetMeta(ctx, name, key); got != "" {
			t.Errorf("meta %s = %q after no-pane Stop, want empty", key, got)
		}
	}
	if _, ok := act.Load(provider.SessionName(name)); ok {
		t.Error("activity record survived a no-pane Stop")
	}
	if _, ok := mustBinding(t)(p.Binding(name)); ok {
		t.Error("binding still stored after a no-pane Stop")
	}
}
