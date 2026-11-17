package provider_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/docbobo/rex-gas-city/internal/provider"
	"github.com/docbobo/rex-gas-city/internal/rex/cli"
)

// TestE2EAgentLifecycle drives a live Rex server through Start, Peek,
// IsRunning and Stop. It is the Phase 1 smoke gate and the evidence for AC #5:
// the agent prints well past Rex's single-reply cap, so Peek only returns the
// sentinel at the end if the adapter follows format paging to completion.
//
// It is opt-in because it mutates the user's Rex server:
//
//	REX_E2E=1 go test ./internal/provider -run TestE2E -v
func TestE2EAgentLifecycle(t *testing.T) {
	if os.Getenv("REX_E2E") == "" {
		t.Skip("set REX_E2E=1 to run against the live Rex server")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	city := fmt.Sprintf("e2e-%d", time.Now().UnixNano())
	client := cli.New()
	p := provider.New(city, client)

	const (
		name     = "Indigo--dog-1"
		sentinel = "E2E-DONE-SENTINEL"
	)
	// ~4 MiB of output: far past the 960 KiB reply cap, so page 1 always has
	// an id and the rest must come from blob_read.
	script := fmt.Sprintf(
		`for i in $(seq 1 40000); do printf '%%0100d\n' "$i"; done; echo %s; sleep 300`,
		sentinel,
	)

	defer func() {
		_ = p.Stop(ctx, name)
		// Best-effort cleanup of the scratch city session.
		_ = exec.CommandContext(ctx, "rex", "session", "kill", "gc:"+city).Run()
	}()

	if err := p.Start(ctx, name, provider.Config{
		Argv:    []string{"/bin/sh", "-c", script},
		WorkDir: os.TempDir(),
	}); err != nil {
		t.Fatalf("Start: %v", err)
	}

	out, err := waitForSentinel(ctx, t, p, name, sentinel)
	if err != nil {
		t.Fatalf("Peek: %v", err)
	}
	if len(out) <= 983040 {
		t.Errorf("Peek returned %d bytes, want paging past the 960 KiB cap", len(out))
	}
	if !strings.Contains(out, sentinel) {
		t.Errorf("Peek output is missing the end sentinel; output was truncated")
	}

	running, err := p.IsRunning(ctx, name)
	if err != nil {
		t.Fatalf("IsRunning: %v", err)
	}
	if !running {
		t.Error("IsRunning = false, want true while the agent sleeps")
	}

	if err := p.Stop(ctx, name); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if running, err = p.IsRunning(ctx, name); err != nil {
		t.Fatalf("IsRunning after Stop: %v", err)
	}
	if running {
		t.Error("IsRunning = true after Stop, want false")
	}
	if out, err = p.Peek(ctx, name); err != nil {
		t.Fatalf("Peek after Stop: %v", err)
	}
	if out != "" {
		t.Errorf("Peek after Stop = %q, want empty", out)
	}
}

// waitForSentinel polls Peek until the agent's output ends with sentinel.
func waitForSentinel(ctx context.Context, t *testing.T, p *provider.Provider, name, sentinel string) (string, error) {
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	var out string
	var err error
	for time.Now().Before(deadline) {
		if out, err = p.Peek(ctx, name); err != nil {
			return "", err
		}
		if strings.Contains(out, sentinel) {
			return out, nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return "", fmt.Errorf("sentinel %q not seen within 60s; last Peek length %d", sentinel, len(out))
}
