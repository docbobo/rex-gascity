package provider_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/docbobo/rex-gascity/internal/provider"
	"github.com/docbobo/rex-gascity/internal/rex/faketest"
)

func TestMetaRoundTripsBytes(t *testing.T) {
	ctx := context.Background()
	p := provider.New("acme", faketest.New())
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

func TestRemoveMetaRemovesOnlyNamedKey(t *testing.T) {
	ctx := context.Background()
	p := provider.New("acme", faketest.New())
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
	p1 := provider.New("acme", faketest.New(), provider.WithStateDir(dir))
	p2 := provider.New("acme", faketest.New(), provider.WithStateDir(dir))
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
	p := provider.New("acme", faketest.New(), provider.WithMetaStore(provider.NewFileMetaStore(filepath.Join(file, "sub"))))
	if err := p.SetMeta(context.Background(), "a", "k", "v"); err == nil {
		t.Error("SetMeta = nil, want write error")
	}
}
