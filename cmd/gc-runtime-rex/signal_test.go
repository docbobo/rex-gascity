package main

import (
	"bytes"
	"context"
	"os/signal"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/docbobo/rex-gascity/internal/provider"
	"github.com/docbobo/rex-gascity/internal/rex"
	"github.com/docbobo/rex-gascity/internal/rex/faketest"
)

// blockingLaunchFake blocks NewWindow until its context is canceled, the way a
// launch stuck on a Rex startup dialog behaves while gc's SIGTERM grace runs.
type blockingLaunchFake struct {
	*faketest.Fake
	started chan struct{}
}

func (f *blockingLaunchFake) NewWindow(ctx context.Context, _ rex.SessionID, _ string, _ rex.LaunchSpec) (rex.WindowID, rex.BlockID, error) {
	close(f.started)
	<-ctx.Done()
	return "", "", ctx.Err()
}

// TestSIGTERMCancelsStartAndRollsBack pins acceptance criterion #3: a SIGTERM
// to the dispatcher cancels the start and the provider's rollback defer runs
// before exit, leaving no seeded identity or binding behind.
func TestSIGTERMCancelsStartAndRollsBack(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GC_REX_STATE_DIR", dir)
	t.Setenv("GC_REX_CITY", "sigcity")

	// main installs the same NotifyContext; installing it here means the
	// SIGTERM below cancels this context instead of killing the test process.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM)
	defer stop()

	fake := &blockingLaunchFake{Fake: faketest.New(), started: make(chan struct{})}
	d := deps{newClient: func() rex.Client { return fake }}

	var stderr bytes.Buffer
	done := make(chan int, 1)
	go func() {
		done <- runWithContext(ctx, []string{"start", dog}, strings.NewReader(`{"command":"sleep 300"}`), &bytes.Buffer{}, &stderr, d)
	}()

	select {
	case <-fake.started:
	case <-time.After(5 * time.Second):
		t.Fatal("launch never started")
	}
	if err := syscall.Kill(syscall.Getpid(), syscall.SIGTERM); err != nil {
		t.Fatalf("kill: %v", err)
	}

	select {
	case code := <-done:
		if code != exitFail {
			t.Errorf("start exit = %d after SIGTERM, want %d (stderr %q)", code, exitFail, stderr.String())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("run did not return after SIGTERM")
	}

	// The rollback ran: no binding and no seeded identity survives.
	p := provider.New("sigcity", fake, provider.WithNamespace(provider.NewNamespaceForDirs(dir)))
	if b, ok, err := p.Binding(dog); err != nil {
		t.Fatalf("Binding: %v", err)
	} else if ok {
		t.Errorf("binding %+v survived the cancelled start", b)
	}
	got, err := p.GetMeta(context.Background(), dog, "GC_SESSION_ID")
	if err != nil {
		t.Fatalf("GetMeta: %v", err)
	}
	if got != "" {
		t.Errorf("GC_SESSION_ID = %q survived the cancelled start", got)
	}
}
