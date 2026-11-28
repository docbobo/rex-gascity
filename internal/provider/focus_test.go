package provider

import (
	"context"
	"testing"

	"github.com/docbobo/rex-gascity/internal/rex"
	"github.com/docbobo/rex-gascity/internal/rex/faketest"
)

// TestStartFocusesOnlyOnCreate pins the focus rule for a launch: a brand-new
// session's tab becomes the active window so the operator lands in it, while a
// resume of a live pane and a relaunch of a dead one leave the current tab
// alone. gc's start contract has no explicit create-vs-reattach signal, so the
// rule keys off whether the name already answered to a binding.
func TestStartFocusesOnlyOnCreate(t *testing.T) {
	ctx := context.Background()
	fake := faketest.New()
	p := New("acme", fake)
	cfg := Config{Argv: []string{"pi"}, Env: map[string]string{"GC_RIG": "rex-gascity"}}

	if err := p.Start(ctx, "agent-1", cfg); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if len(fake.CreatedWindows) != 1 {
		t.Fatalf("created windows = %d, want 1", len(fake.CreatedWindows))
	}
	if !fake.CreatedWindows[0].Spec.Focus {
		t.Error("new session launched with focus=false, want true")
	}

	// Re-attach: the pane is alive, so the start is idempotent and cannot move
	// focus.
	if err := p.Start(ctx, "agent-1", cfg); err != nil {
		t.Fatalf("Start (alive): %v", err)
	}
	if n := len(fake.CreatedWindows); n != 1 {
		t.Fatalf("alive resume launched again: windows = %d, want 1", n)
	}

	// The pane's program exits: the next start relaunches an existing session,
	// which must not steal focus.
	b, ok := mustBinding(t)(p.Binding("agent-1"))
	if !ok {
		t.Fatal("Binding(agent-1) = false after Start")
	}
	fake.Processes = map[rex.BlockID]rex.ProcessInfo{
		b.BlockID: {LastExit: &rex.ExitStatus{ExitCode: 0}},
	}
	if err := p.Start(ctx, "agent-1", cfg); err != nil {
		t.Fatalf("Start (dead pane): %v", err)
	}
	if len(fake.CreatedWindows) != 2 {
		t.Fatalf("dead-pane restart created %d windows, want 2", len(fake.CreatedWindows))
	}
	if fake.CreatedWindows[1].Spec.Focus {
		t.Error("relaunch of an existing session launched with focus=true, want false")
	}
}
