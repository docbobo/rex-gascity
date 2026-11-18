package provider_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/docbobo/rex-gascity/internal/provider"
	"github.com/docbobo/rex-gascity/internal/rex"
	"github.com/docbobo/rex-gascity/internal/rex/faketest"
)

const (
	teardownDog = "Indigo--dog-1"
	teardownCat = "Indigo--cat-1"
)

var teardownCfg = provider.Config{Argv: []string{"/usr/local/bin/claude", "--x"}}

// teardownStarted returns a provider with name started as a "claude" agent.
func teardownStarted(t *testing.T, name string) (*faketest.Fake, *provider.Provider) {
	t.Helper()
	fake := faketest.New()
	p := provider.New("acme", fake)
	teardownStart(t, p, name)
	return fake, p
}

func teardownStart(t *testing.T, p *provider.Provider, name string) {
	t.Helper()
	if err := p.Start(context.Background(), name, teardownCfg); err != nil {
		t.Fatalf("Start %s: %v", name, err)
	}
}

// teardownExit marks block's program as exited.
func teardownExit(fake *faketest.Fake, block rex.BlockID) {
	if fake.Processes == nil {
		fake.Processes = map[rex.BlockID]rex.ProcessInfo{}
	}
	fake.Processes[block] = rex.ProcessInfo{LastExit: &rex.ExitStatus{}}
}

// teardownShellOnly leaves the terminal alive with only a shell in the
// foreground.
func teardownShellOnly(fake *faketest.Fake, block rex.BlockID) {
	if fake.Processes == nil {
		fake.Processes = map[rex.BlockID]rex.ProcessInfo{}
	}
	fake.Processes[block] = rex.ProcessInfo{Foreground: &rex.Process{Name: "zsh", Path: "/bin/zsh"}}
}

func TestStartTwiceCreatesNoDuplicatePane(t *testing.T) {
	fake, p := teardownStarted(t, teardownDog)
	first, _ := p.Binding(teardownDog)

	teardownStart(t, p, teardownDog)

	if len(fake.CreatedWindows) != 1 || len(fake.CreatedSplits) != 0 {
		t.Errorf("windows=%d splits=%d, want 1 and 0", len(fake.CreatedWindows), len(fake.CreatedSplits))
	}
	if len(fake.ClosedBlocks) != 0 {
		t.Errorf("closed = %v, want none", fake.ClosedBlocks)
	}
	got, ok := p.Binding(teardownDog)
	if !ok || got.BlockID != first.BlockID {
		t.Errorf("binding = %+v, want unchanged %+v", got, first)
	}
}

func TestStartAfterExitRelaunchesInNewTab(t *testing.T) {
	fake, p := teardownStarted(t, teardownDog)
	teardownExit(fake, "block:dog-1")

	teardownStart(t, p, teardownDog)

	if len(fake.ClosedBlocks) != 1 || fake.ClosedBlocks[0] != "block:dog-1" {
		t.Errorf("closed = %v, want [block:dog-1]", fake.ClosedBlocks)
	}
	// The dead pane was the last one, so its tab is gone and a new one opens.
	if len(fake.CreatedWindows) != 2 || len(fake.CreatedSplits) != 0 {
		t.Errorf("windows=%d splits=%d, want 2 and 0", len(fake.CreatedWindows), len(fake.CreatedSplits))
	}
	if len(fake.View.Windows) != 1 {
		t.Errorf("windows in view = %d, want 1", len(fake.View.Windows))
	}
	if b, ok := p.Binding(teardownDog); !ok || b.LaunchMode != "window" {
		t.Errorf("binding = %+v, %v, want a window launch", b, ok)
	}
}

func TestStartAfterExitRelaunchesAsSplitWhenRigTabRemains(t *testing.T) {
	fake, p := teardownStarted(t, teardownDog)
	teardownStart(t, p, teardownCat)
	if len(fake.CreatedSplits) != 1 {
		t.Fatalf("setup splits = %d, want 1", len(fake.CreatedSplits))
	}
	teardownExit(fake, "block:dog-1")

	teardownStart(t, p, teardownDog)

	if len(fake.ClosedBlocks) != 1 || fake.ClosedBlocks[0] != "block:dog-1" {
		t.Errorf("closed = %v, want [block:dog-1]", fake.ClosedBlocks)
	}
	if len(fake.CreatedWindows) != 1 || len(fake.CreatedSplits) != 2 {
		t.Errorf("windows=%d splits=%d, want 1 and 2", len(fake.CreatedWindows), len(fake.CreatedSplits))
	}
	if b, ok := p.Binding(teardownDog); !ok || b.LaunchMode != "split" {
		t.Errorf("binding = %+v, %v, want a split launch", b, ok)
	}
}

func TestStopToleratesVanishedPane(t *testing.T) {
	fake, p := teardownStarted(t, teardownDog)
	fake.CloseBlockErr = rex.ErrNotFound

	if err := p.Stop(context.Background(), teardownDog); err != nil {
		t.Fatalf("Stop = %v, want nil for a vanished pane", err)
	}
	if _, ok := p.Binding(teardownDog); ok {
		t.Error("binding still stored after Stop")
	}
}

func TestStopPropagatesOtherCloseErrors(t *testing.T) {
	fake, p := teardownStarted(t, teardownDog)
	boom := errors.New("transport down")
	fake.CloseBlockErr = boom

	if err := p.Stop(context.Background(), teardownDog); !errors.Is(err, boom) {
		t.Fatalf("Stop = %v, want %v", err, boom)
	}
	if _, ok := p.Binding(teardownDog); !ok {
		t.Error("binding forgotten although the close failed")
	}
}

func TestSecondStopIsNoOp(t *testing.T) {
	fake, p := teardownStarted(t, teardownDog)
	if err := p.Stop(context.Background(), teardownDog); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if err := p.Stop(context.Background(), teardownDog); err != nil {
		t.Fatalf("second Stop = %v, want nil", err)
	}
	if len(fake.ClosedBlocks) != 1 {
		t.Errorf("closed = %v, want one close", fake.ClosedBlocks)
	}
}

func TestIsRunningMissingPane(t *testing.T) {
	p := provider.New("acme", faketest.New())
	running, err := p.IsRunning(context.Background(), teardownDog)
	if err != nil || running {
		t.Errorf("IsRunning = %v, %v, want false, nil", running, err)
	}
}

func TestStartRecordsLaunchModeAndCreatedAt(t *testing.T) {
	_, p := teardownStarted(t, teardownDog)

	b, ok := p.Binding(teardownDog)
	if !ok {
		t.Fatal("no binding recorded")
	}
	if b.LaunchMode != "window" {
		t.Errorf("LaunchMode = %q, want window", b.LaunchMode)
	}
	if b.CreatedAt.IsZero() {
		t.Error("CreatedAt not set")
	}
}

// v1 rule: liveness is last_exit, not process names. A live terminal with only
// a shell in the foreground is running.
func TestIsRunningTrueWhileTerminalLive(t *testing.T) {
	fake, p := teardownStarted(t, teardownDog)
	teardownShellOnly(fake, "block:dog-1")

	running, err := p.IsRunning(context.Background(), teardownDog)
	if err != nil || !running {
		t.Errorf("IsRunning = %v, %v, want true, nil", running, err)
	}
}

func TestIsRunningFalseAfterExit(t *testing.T) {
	fake, p := teardownStarted(t, teardownDog)
	teardownExit(fake, "block:dog-1")

	running, err := p.IsRunning(context.Background(), teardownDog)
	if err != nil || running {
		t.Errorf("IsRunning = %v, %v, want false, nil", running, err)
	}
}

func TestProcessAliveMatchesByName(t *testing.T) {
	fake, p := teardownStarted(t, teardownDog)
	fake.Processes = map[rex.BlockID]rex.ProcessInfo{"block:dog-1": {
		Foreground: &rex.Process{Name: "git", Path: "/usr/bin/git"},
		Ancestors:  []rex.Process{{Name: "node", Argv0: "/usr/local/bin/claude"}},
	}}

	alive, err := p.ProcessAlive(context.Background(), teardownDog, []string{"claude"})
	if err != nil || !alive {
		t.Errorf("ProcessAlive(claude) = %v, %v, want true, nil", alive, err)
	}
	alive, err = p.ProcessAlive(context.Background(), teardownDog, []string{"codex"})
	if err != nil || alive {
		t.Errorf("ProcessAlive(codex) = %v, %v, want false, nil", alive, err)
	}
	alive, err = p.ProcessAlive(context.Background(), teardownDog, nil)
	if err != nil || !alive {
		t.Errorf("ProcessAlive(no names) = %v, %v, want true, nil", alive, err)
	}
}

func TestProcessAliveMissingPane(t *testing.T) {
	p := provider.New("acme", faketest.New())
	alive, err := p.ProcessAlive(context.Background(), teardownDog, []string{"claude"})
	if err != nil || alive {
		t.Errorf("ProcessAlive = %v, %v, want false, nil", alive, err)
	}
}

func TestProcessAliveVanishedPane(t *testing.T) {
	fake, p := teardownStarted(t, teardownDog)
	fake.ProcessErr = fmt.Errorf("rex block.method: %w", rex.ErrNotFound)
	alive, err := p.ProcessAlive(context.Background(), teardownDog, []string{"claude"})
	if err != nil || alive {
		t.Errorf("ProcessAlive after pane vanished = %v, %v; want false, nil", alive, err)
	}
}
