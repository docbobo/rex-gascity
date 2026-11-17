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

// TestE2EInputPath drives a live Rex server through the Phase 2 input path:
// nudge a shell through a command, start a long foreground command, interrupt
// it, and prove the shell regained control. It is opt-in because it mutates the
// user's Rex server:
//
//	REX_E2E=1 go test ./internal/provider -run TestE2EInputPath -v
func TestE2EInputPath(t *testing.T) {
	if os.Getenv("REX_E2E") == "" {
		t.Skip("set REX_E2E=1 to run against the live Rex server")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	city := fmt.Sprintf("e2e-in-%d", time.Now().UnixNano())
	client := cli.New()
	p := provider.New(city, client)

	const name = "Indigo--dog-1"
	defer func() {
		_ = p.Stop(ctx, name)
		_ = exec.CommandContext(ctx, "rex", "session", "kill", "gc:"+city).Run()
	}()

	if err := p.Start(ctx, name, provider.Config{
		Argv:    []string{"/bin/sh"},
		WorkDir: os.TempDir(),
	}); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// The output must differ from the typed line, or a screen echo would match
	// and the test would pass even if the submit never ran. A shell may not be
	// ready the instant the pane exists, so nudge the first command until its
	// computed output shows up; once it does, the shell is ready.
	const marker = "E2E-42"
	nudgeUntilOutput(ctx, t, p, name, "echo E2E-$((20+22))", marker)

	// Start a long foreground command and wait until it really holds the
	// terminal, so the interrupt has something to interrupt.
	if err := p.Nudge(ctx, name, "sleep 300"); err != nil {
		t.Fatalf("Nudge sleep: %v", err)
	}
	waitForForeground(ctx, t, client, "gc:"+city, "sleep")

	if err := p.Interrupt(ctx, name); err != nil {
		t.Fatalf("Interrupt: %v", err)
	}

	// With sleep interrupted, the shell regains the terminal and this command
	// runs. Had ctrl+c not landed, sleep would still hold the shell and the
	// output below would never appear.
	nudgeUntilOutput(ctx, t, p, name, "echo AFTER-$((1+1))", "AFTER-2")
}

// waitForForeground polls the pane's process tree until a process named want is
// the terminal's foreground process.
func waitForForeground(ctx context.Context, t *testing.T, client *cli.Client, sessionLabel, want string) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if foregroundIs(ctx, client, sessionLabel, want) {
			return
		}
		time.Sleep(300 * time.Millisecond)
	}
	t.Fatalf("foreground process %q never appeared in %s", want, sessionLabel)
}

// foregroundIs reports whether any block in the session has want as its
// foreground process.
func foregroundIs(ctx context.Context, client *cli.Client, sessionLabel, want string) bool {
	sessions, err := client.ListSessions(ctx)
	if err != nil {
		return false
	}
	for _, s := range sessions {
		if s.Label != sessionLabel {
			continue
		}
		view, err := client.SessionView(ctx, s.ID)
		if err != nil {
			return false
		}
		for _, w := range view.Windows {
			for _, b := range w.Blocks {
				info, err := client.BlockProcess(ctx, s.ID, b.ID)
				if err != nil || info.Foreground == nil {
					continue
				}
				if info.Foreground.Name == want || info.Foreground.Argv0 == want || strings.HasSuffix(info.Foreground.Path, "/"+want) {
					return true
				}
			}
		}
	}
	return false
}

// nudgeUntilOutput nudges text and polls Peek for needle, re-nudging while the
// pane may still be starting up. It fails the test if needle never appears.
func nudgeUntilOutput(ctx context.Context, t *testing.T, p *provider.Provider, name, text, needle string) string {
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	var last string
	for time.Now().Before(deadline) {
		if err := p.Nudge(ctx, name, text); err != nil {
			t.Fatalf("Nudge %q: %v", text, err)
		}
		for i := 0; i < 10; i++ {
			out, err := p.Peek(ctx, name)
			if err != nil {
				t.Fatalf("Peek: %v", err)
			}
			last = out
			if strings.Contains(out, needle) {
				return out
			}
			time.Sleep(300 * time.Millisecond)
		}
	}
	t.Fatalf("output missing %q within 60s; last output:\n%s", needle, last)
	return ""
}
