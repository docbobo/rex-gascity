package provider_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/docbobo/rex-gascity/internal/provider"
	"github.com/docbobo/rex-gascity/internal/rex"
	"github.com/docbobo/rex-gascity/internal/rex/faketest"
)

// blockingLaunchFake blocks NewWindow until released or its context is
// canceled, the way a launch stuck on a Rex startup dialog behaves while it
// holds whatever locks the provider took for the launch.
type blockingLaunchFake struct {
	*faketest.Fake
	started chan struct{}
	release chan struct{}
}

func (f *blockingLaunchFake) NewWindow(ctx context.Context, sid rex.SessionID, label string, spec rex.LaunchSpec) (rex.WindowID, rex.BlockID, error) {
	close(f.started)
	select {
	case <-f.release:
	case <-ctx.Done():
		return "", "", ctx.Err()
	}
	return f.Fake.NewWindow(ctx, sid, label, spec)
}

// TestSlowLaunchDoesNotHoldStateLock pins acceptance criterion #2 for the
// blocking launch: while one rig's NewWindow is stuck, a stop of a different
// name in a different rig must not wait on the city-wide state lock.
func TestSlowLaunchDoesNotHoldStateLock(t *testing.T) {
	dir := t.TempDir()
	fake := &blockingLaunchFake{
		Fake:    faketest.New(),
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	p := provider.New("acme", fake, provider.WithStateDir(dir))

	startDone := make(chan error, 1)
	go func() {
		startDone <- p.Start(context.Background(), "Indigo--a", provider.Config{Argv: []string{"pi"}})
	}()
	select {
	case <-fake.started:
	case <-time.After(5 * time.Second):
		t.Fatal("launch never started")
	}

	stopDone := make(chan error, 1)
	go func() { stopDone <- p.Stop(context.Background(), "Zephyr--b") }()
	select {
	case err := <-stopDone:
		if err != nil {
			t.Fatalf("Stop: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Stop blocked behind a launch in another rig: the state lock is held across the launch")
	}

	close(fake.release)
	if err := <-startDone; err != nil {
		t.Fatalf("Start: %v", err)
	}
}

// TestSlowPreStartDoesNotHoldStateLock pins acceptance criterion #2 for
// pre_start: while one name's hook is still running, a stop of a different
// name must not wait on the city-wide state lock.
func TestSlowPreStartDoesNotHoldStateLock(t *testing.T) {
	dir := t.TempDir()
	fake := faketest.New()
	p := provider.New("acme", fake, provider.WithStateDir(dir))

	hookStarted := filepath.Join(dir, "hook-started")
	hookRelease := filepath.Join(dir, "hook-release")
	cfg := provider.Config{
		Argv: []string{"pi"},
		PreStart: []string{
			"touch " + hookStarted + "; while [ ! -f " + hookRelease + " ]; do sleep 0.05; done",
		},
	}
	startDone := make(chan error, 1)
	go func() { startDone <- p.Start(context.Background(), "Indigo--a", cfg) }()
	waitForFile(t, hookStarted, 5*time.Second)

	stopDone := make(chan error, 1)
	go func() { stopDone <- p.Stop(context.Background(), "Zephyr--b") }()
	select {
	case err := <-stopDone:
		if err != nil {
			t.Fatalf("Stop: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Stop blocked behind a pre_start hook: the state lock is held across pre_start")
	}

	if err := os.WriteFile(hookRelease, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := <-startDone; err != nil {
		t.Fatalf("Start: %v", err)
	}
}

// waitForFile polls until path exists or the deadline passes.
func waitForFile(t *testing.T, path string, within time.Duration) {
	t.Helper()
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("file %s did not appear within %s", path, within)
}
