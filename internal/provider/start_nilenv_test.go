package provider

import (
	"context"
	"testing"

	"github.com/docbobo/rex-gascity/internal/rex/faketest"
)

// TestStartWithoutEnvResolvesStaleRecordToLivePane pins the nil-env start path.
// A start config may omit env (the exec-session-provider contract calls it
// optional and omitted when empty), and launch.Project keeps a nil map nil.
// Start must still resolve a fresh start against the labels the name implies:
// when it resolves without a start env, it rescans under the stale record's
// labels instead, misses a live pane whose labels differ, and launches a
// duplicate. With the empty env normalized, the second start finds the pane and
// is idempotent.
func TestStartWithoutEnvResolvesStaleRecordToLivePane(t *testing.T) {
	pinCity(t, "")
	fake := faketest.New()
	store := NewMemoryStore()
	p := New("acme", fake, WithBindingStore(store))
	ctx := context.Background()
	const name = "Indigo--dog-1"

	if err := p.Start(ctx, name, Config{Argv: []string{"claude"}}); err != nil {
		t.Fatalf("first Start: %v", err)
	}
	launched, ok := p.Binding(name)
	if !ok {
		t.Fatalf("Binding(%q) = false after start", name)
	}
	if len(fake.CreatedWindows) != 1 || len(fake.CreatedSplits) != 0 {
		t.Fatalf("launches after first start = %d windows, %d splits; want 1 and 0",
			len(fake.CreatedWindows), len(fake.CreatedSplits))
	}

	// A stale record whose address is gone and whose labels differ from the
	// live pane. Without a start env, ResolveFresh rescans under these labels.
	stale := launched
	stale.WindowID = "window:gone"
	stale.BlockID = "block:gone"
	stale.TabLabel = "Rig: Wrong"
	store.Save(stale)

	if err := p.Start(ctx, name, Config{Argv: []string{"claude"}}); err != nil {
		t.Fatalf("second Start: %v", err)
	}
	if len(fake.CreatedWindows) != 1 || len(fake.CreatedSplits) != 0 {
		t.Fatalf("launches after second start = %d windows, %d splits; want 1 and 0 (no duplicate)",
			len(fake.CreatedWindows), len(fake.CreatedSplits))
	}
	got, ok := p.Binding(name)
	if !ok || got.BlockID != launched.BlockID || got.TabLabel != launched.TabLabel {
		t.Fatalf("Binding after second start = %+v, %v; want the live pane %+v", got, ok, launched)
	}
}
