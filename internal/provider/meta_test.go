package provider_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/docbobo/rex-gascity/internal/provider"
	"github.com/docbobo/rex-gascity/internal/rex/faketest"
)

// sessionProvider builds a provider whose metadata ops have a session record to
// write to. gc only stores metadata for a session the pack has started, so a
// name has a binding here (see the set-meta precondition, GCREX-49).
func sessionProvider(t *testing.T, names ...string) *provider.Provider {
	t.Helper()
	bs := provider.NewMemoryStore()
	for _, n := range names {
		bs.Save(provider.Binding{Name: provider.SessionName(n), SessionID: "session:stale", BlockID: "block:stale"})
	}
	return provider.New("acme", faketest.New(), provider.WithBindingStore(bs))
}

func TestMetaRoundTripsBytes(t *testing.T) {
	ctx := context.Background()
	p := sessionProvider(t, "a")
	const want = "❯ "
	if err := p.SetMeta(ctx, "a", "GC_READY_PROMPT_PREFIX", want); err != nil {
		t.Fatalf("SetMeta: %v", err)
	}
	got, err := p.GetMeta(ctx, "a", "GC_READY_PROMPT_PREFIX")
	if err != nil || got != want {
		t.Errorf("GetMeta = %q, %v; want %q", got, err, want)
	}
}

func TestMetaUnsetKeyIsEmpty(t *testing.T) {
	p := provider.New("acme", faketest.New())
	got, err := p.GetMeta(context.Background(), "a", "nope")
	if got != "" || err != nil {
		t.Errorf("GetMeta = %q, %v; want empty, nil", got, err)
	}
}

// TestSetMetaRequiresSessionRecord pins herdr-core's set-meta precondition: a
// name with no session record (never started, or stopped) is an error and
// stores nothing. The provider's cached binding is the session record; no Rex
// call is made (GCREX-49).
func TestSetMetaRequiresSessionRecord(t *testing.T) {
	ctx := context.Background()
	p := sessionProvider(t)
	if err := p.SetMeta(ctx, "Indigo--ghost-1", "GC_KEY", "value"); err == nil {
		t.Fatal("SetMeta without a session record = nil, want error")
	}
	if got, err := p.GetMeta(ctx, "Indigo--ghost-1", "GC_KEY"); err != nil || got != "" {
		t.Errorf("meta stored without a session record: %q, %v; want empty", got, err)
	}
}

// TestSetMetaStoresForRecordedSession is the other side of the precondition: a
// name with a session record still stores.
func TestSetMetaStoresForRecordedSession(t *testing.T) {
	ctx := context.Background()
	p := sessionProvider(t, "Indigo--dog-1")
	if err := p.SetMeta(ctx, "Indigo--dog-1", "GC_KEY", "value"); err != nil {
		t.Fatalf("SetMeta with a session record: %v", err)
	}
	if got, err := p.GetMeta(ctx, "Indigo--dog-1", "GC_KEY"); err != nil || got != "value" {
		t.Errorf("GetMeta = %q, %v; want value", got, err)
	}
}

// TestSetMetaRejectedAfterStop pins the stopped-session half: Stop retires the
// session record, so a later set-meta fails and stores nothing.
func TestSetMetaRejectedAfterStop(t *testing.T) {
	ctx := context.Background()
	_, p := teardownStarted(t, teardownDog)
	if err := p.SetMeta(ctx, teardownDog, "GC_KEY", "value"); err != nil {
		t.Fatalf("SetMeta while running: %v", err)
	}
	if err := p.Stop(ctx, teardownDog); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if err := p.SetMeta(ctx, teardownDog, "GC_KEY2", "value2"); err == nil {
		t.Error("SetMeta after Stop = nil, want error")
	}
	if got, _ := p.GetMeta(ctx, teardownDog, "GC_KEY2"); got != "" {
		t.Errorf("meta stored after Stop: %q", got)
	}
}

func TestRemoveMetaRemovesOnlyNamedKey(t *testing.T) {
	ctx := context.Background()
	p := sessionProvider(t, "a")
	_ = p.SetMeta(ctx, "a", "k1", "v1")
	_ = p.SetMeta(ctx, "a", "k2", "v2")
	if err := p.RemoveMeta(ctx, "a", "k1"); err != nil {
		t.Fatalf("RemoveMeta: %v", err)
	}
	if got, _ := p.GetMeta(ctx, "a", "k1"); got != "" {
		t.Errorf("k1 = %q, want removed", got)
	}
	if got, _ := p.GetMeta(ctx, "a", "k2"); got != "v2" {
		t.Errorf("k2 = %q, want v2", got)
	}
}

func TestMetaSharedAcrossProvidersOnOneStateDir(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	ns := provider.NewNamespaceForDirs(dir)
	bs := provider.NewMemoryStore()
	bs.Save(provider.Binding{Name: provider.SessionName("a"), SessionID: "session:stale", BlockID: "block:stale"})
	p1 := provider.New("acme", faketest.New(), provider.WithNamespace(ns), provider.WithBindingStore(bs))
	p2 := provider.New("acme", faketest.New(), provider.WithNamespace(ns), provider.WithBindingStore(bs))
	if err := p1.SetMeta(ctx, "a", "k", "❯ x"); err != nil {
		t.Fatalf("SetMeta: %v", err)
	}
	if got, _ := p2.GetMeta(ctx, "a", "k"); got != "❯ x" {
		t.Errorf("p2 sees %q, want %q", got, "❯ x")
	}
}

func TestSetMetaReturnsWriteError(t *testing.T) {
	file := filepath.Join(t.TempDir(), "plainfile")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	bs := provider.NewMemoryStore()
	bs.Save(provider.Binding{Name: provider.SessionName("a"), SessionID: "session:stale", BlockID: "block:stale"})
	p := provider.New("acme", faketest.New(), provider.WithBindingStore(bs), provider.WithMetaStore(provider.NewFileMetaStore(filepath.Join(file, "sub"))))
	if err := p.SetMeta(context.Background(), "a", "k", "v"); err == nil {
		t.Error("SetMeta = nil, want write error")
	}
}
