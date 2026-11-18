package provider_test

import (
	"context"
	"testing"

	"github.com/docbobo/rex-gascity/internal/provider"
	"github.com/docbobo/rex-gascity/internal/rex/faketest"
)

// A binding recorded by one Provider is readable by a fresh Provider over the
// same state dir, with no Rex calls: this is what lets a one-process-per-op
// dispatcher find the pane again.
func TestBindingSurvivesNewProvider(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	first := provider.New("acme", faketest.New(), provider.WithNamespace(provider.NewNamespaceForDirs(dir)))
	if err := first.Start(ctx, "Indigo--dog-1", provider.Config{Argv: []string{"claude"}}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	want, ok := first.Binding("Indigo--dog-1")
	if !ok || want.BlockID == "" || want.WindowID == "" || want.SessionID == "" {
		t.Fatalf("first Binding = %+v, %v", want, ok)
	}

	fake2 := faketest.New() // knows no sessions or views
	second := provider.New("acme", fake2, provider.WithNamespace(provider.NewNamespaceForDirs(dir)))
	got, ok := second.Binding("Indigo--dog-1")
	if !ok {
		t.Fatal("second provider lost the binding")
	}
	if got.SessionID != want.SessionID || got.WindowID != want.WindowID ||
		got.BlockID != want.BlockID || got.SessionLabel != "gc:acme" {
		t.Errorf("second Binding = %+v, want %+v", got, want)
	}
	if fake2.ListSessionsCalls != 0 || fake2.SessionViewCalls != 0 {
		t.Errorf("lookup called Rex: list=%d view=%d", fake2.ListSessionsCalls, fake2.SessionViewCalls)
	}
}

func TestDefaultProviderKeepsStateInMemory(t *testing.T) {
	ctx := context.Background()
	p := provider.New("acme", faketest.New())
	if err := p.Start(ctx, "Indigo--dog-1", provider.Config{Argv: []string{"claude"}}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if _, ok := p.Binding("Indigo--dog-1"); !ok {
		t.Error("in-memory binding missing")
	}
	if _, ok := provider.New("acme", faketest.New()).Binding("Indigo--dog-1"); ok {
		t.Error("a default provider shared state with another")
	}
}

func TestWithBindingStoreOverridesDefault(t *testing.T) {
	store := provider.NewMemoryStore()
	p := provider.New("acme", faketest.New(), provider.WithBindingStore(store))
	if err := p.Start(context.Background(), "Indigo--dog-1", provider.Config{Argv: []string{"claude"}}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if _, ok := store.Load("Indigo--dog-1"); !ok {
		t.Error("binding not saved to the supplied store")
	}
}
