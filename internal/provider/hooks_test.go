package provider_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/docbobo/rex-gascity/internal/provider"
	"github.com/docbobo/rex-gascity/internal/rex/faketest"
)

// TestHookTimeoutCappedByStartBudget is the seam proof that the per-hook
// timeout is capped by the remaining start budget: the fake observes the
// deadline the hook would run under, without starting /bin/sh.
func TestHookTimeoutCappedByStartBudget(t *testing.T) {
	runner := &provider.FakeRunner{}
	p := provider.New("acme", faketest.New(),
		provider.WithRunner(runner),
		provider.WithStartBudget(5*time.Second),
		provider.WithPreStartHookTimeout(time.Minute))

	start := time.Now()
	cfg := provider.Config{Argv: []string{"pi"}, PreStart: []string{"true"}}
	if err := p.Start(context.Background(), "pi-mc-wisp-abc", cfg); err != nil {
		t.Fatalf("Start: %v", err)
	}
	calls := runner.HookCalls()
	if len(calls) != 1 {
		t.Fatalf("hook calls = %d, want 1", len(calls))
	}
	got := calls[0].Deadline
	if got.IsZero() {
		t.Fatal("hook call carried no deadline")
	}
	if d := got.Sub(start); d < 4*time.Second || d > 6*time.Second {
		t.Errorf("hook deadline is %v from start, want about the 5s start budget", d)
	}
	if !got.Before(start.Add(time.Minute)) {
		t.Errorf("hook deadline %v not capped below the 1m per-hook timeout", got)
	}
}

// TestSessionSetupHookCappedByStartBudget is the same proof for session_setup,
// and pins the environment a setup hook sees: the projected launch env plus
// GC_SESSION.
func TestSessionSetupHookCappedByStartBudget(t *testing.T) {
	runner := &provider.FakeRunner{}
	p := provider.New("acme", faketest.New(),
		provider.WithRunner(runner),
		provider.WithStartBudget(5*time.Second),
		provider.WithPreStartHookTimeout(time.Minute))

	start := time.Now()
	cfg := provider.Config{
		Argv: []string{"pi"},
		Env: map[string]string{
			"GC_TEMPLATE":   "gastownhall/rex",
			"GC_AGENT":      "dog-1",
			"GC_SESSION_ID": "bead-123",
		},
		SessionSetup: []string{"true", "true"},
	}
	if err := p.Start(context.Background(), "pi-mc-wisp-abc", cfg); err != nil {
		t.Fatalf("Start: %v", err)
	}
	calls := runner.HookCalls()
	if len(calls) != 2 {
		t.Fatalf("hook calls = %d, want 2", len(calls))
	}
	for _, c := range calls {
		if d := c.Deadline.Sub(start); d < 4*time.Second || d > 6*time.Second {
			t.Errorf("setup hook deadline is %v from start, want about the 5s start budget", d)
		}
		if !slices.Contains(c.Env, "GC_SESSION=pi-mc-wisp-abc") {
			t.Errorf("setup hook env missing GC_SESSION: %v", c.Env)
		}
		if !slices.Contains(c.Env, "BEADS_ACTOR=gastownhall/rex") {
			t.Errorf("setup hook env missing the projected launch env: %v", c.Env)
		}
	}
}

// TestPreStartHookResolvesWorkDirPerHook is the regression proof that each hook
// resolves its working directory right before it runs: a first hook that creates
// a missing WorkDir must make the next hook run inside it, not in "/".
func TestPreStartHookResolvesWorkDirPerHook(t *testing.T) {
	workDir := filepath.Join(t.TempDir(), "new-worktree")
	var dirs []string
	runner := &provider.FakeRunner{
		HookHandle: func(_ context.Context, c provider.HookCall) error {
			dirs = append(dirs, c.Dir)
			if len(dirs) == 1 {
				return os.MkdirAll(workDir, 0o755)
			}
			return nil
		},
	}
	p := provider.New("acme", faketest.New(), provider.WithRunner(runner))

	cfg := provider.Config{
		Argv:    []string{"pi"},
		WorkDir: workDir,
		PreStart: []string{
			"mkdir -p " + workDir,
			"touch .prepared",
		},
	}
	if err := p.Start(context.Background(), "pi-mc-wisp-abc", cfg); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if len(dirs) != 2 {
		t.Fatalf("hook dirs = %v, want 2 hooks", dirs)
	}
	if dirs[0] != "/" {
		t.Errorf("first hook dir = %q, want %q for a missing work dir", dirs[0], "/")
	}
	if dirs[1] != workDir {
		t.Errorf("second hook dir = %q, want the created work dir %q", dirs[1], workDir)
	}
}
