package provider_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
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
	p := provider.New("acme", fake, provider.WithNamespace(provider.NewNamespaceForDirs(dir)))

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
	p := provider.New("acme", fake, provider.WithNamespace(provider.NewNamespaceForDirs(dir)))

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

// barrierLaunchFake holds every NewWindow until two launches are in flight, so
// a start that waits behind another start's lock never reaches the barrier and
// fails instead of passing slowly. It proves distinct-name starts overlap.
type barrierLaunchFake struct {
	*faketest.Fake
	mu      sync.Mutex
	arrived int
	open    chan struct{}
}

func (f *barrierLaunchFake) NewWindow(ctx context.Context, sid rex.SessionID, label string, spec rex.LaunchSpec) (rex.WindowID, rex.BlockID, error) {
	f.mu.Lock()
	f.arrived++
	if f.arrived == 2 {
		close(f.open)
	}
	f.mu.Unlock()
	select {
	case <-f.open:
	case <-ctx.Done():
		return "", "", ctx.Err()
	case <-time.After(5 * time.Second):
		return "", "", errors.New("launch waited for another start: starts serialized")
	}
	return f.Fake.NewWindow(ctx, sid, label, spec)
}

// TestDistinctStartsLaunchConcurrently proves RPP-CONCURRENCY-001 by wall
// time: two starts of distinct names in distinct rigs reach their launches
// together, so neither waits for the other's locks.
func TestDistinctStartsLaunchConcurrently(t *testing.T) {
	dir := t.TempDir()
	fake := &barrierLaunchFake{Fake: faketest.New(), open: make(chan struct{})}
	names := []string{"Indigo--a", "Zephyr--b"}
	errs := make(chan error, len(names))
	var wg sync.WaitGroup
	for _, name := range names {
		p := provider.New("acme", fake, provider.WithNamespace(provider.NewNamespaceForDirs(dir)))
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- p.Start(context.Background(), name, provider.Config{Argv: []string{"pi"}})
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("Start: %v", err)
		}
	}
}

// blockingCloseFake stalls CloseBlock for one block until released, the way a
// slow Rex close behaves while it holds whatever lock the provider took for it.
type blockingCloseFake struct {
	*faketest.Fake
	block   rex.BlockID
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (f *blockingCloseFake) CloseBlock(ctx context.Context, sid rex.SessionID, block rex.BlockID) error {
	if block == f.block {
		f.once.Do(func() { close(f.started) })
		select {
		case <-f.release:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return f.Fake.CloseBlock(ctx, sid, block)
}

// TestSlowStopDoesNotHoldStateLock pins rule 9 for stop: while one name's close
// is stuck, a stop of another name must not wait on the city-wide state lock.
func TestSlowStopDoesNotHoldStateLock(t *testing.T) {
	dir := t.TempDir()
	fake := &blockingCloseFake{
		Fake:    faketest.New(),
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	p := provider.New("acme", fake, provider.WithNamespace(provider.NewNamespaceForDirs(dir)))
	ctx := context.Background()
	for _, name := range []string{"Indigo--a", "Zephyr--b"} {
		if err := p.Start(ctx, name, provider.Config{Argv: []string{"pi"}}); err != nil {
			t.Fatalf("Start %s: %v", name, err)
		}
	}
	bd, ok := mustBinding(t)(p.Binding("Indigo--a"))
	if !ok {
		t.Fatal("no binding for Indigo--a")
	}
	fake.block = bd.BlockID

	slowDone := make(chan error, 1)
	go func() { slowDone <- p.Stop(ctx, "Indigo--a") }()
	select {
	case <-fake.started:
	case <-time.After(5 * time.Second):
		t.Fatal("stop never reached CloseBlock")
	}

	otherDone := make(chan error, 1)
	go func() { otherDone <- p.Stop(ctx, "Zephyr--b") }()
	select {
	case err := <-otherDone:
		if err != nil {
			t.Fatalf("Stop: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Stop blocked behind another name's close: the state lock is held across it")
	}

	close(fake.release)
	if err := <-slowDone; err != nil {
		t.Fatalf("slow Stop: %v", err)
	}
}
