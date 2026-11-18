package provider_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/docbobo/rex-gascity/internal/provider"
	"github.com/docbobo/rex-gascity/internal/rex"
	"github.com/docbobo/rex-gascity/internal/rex/cli"
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

// TestE2EIdentityActivityAttachment drives a live Rex server through the
// Phase 3 surface: binding identity across a process boundary, stale-binding
// re-resolution, last activity, attachment, and teardown that leaves the
// shared city session alone. It is opt-in:
//
//	REX_E2E=1 go test ./internal/provider -run TestE2EIdentityActivityAttachment -v
func TestE2EIdentityActivityAttachment(t *testing.T) {
	if os.Getenv("REX_E2E") == "" {
		t.Skip("set REX_E2E=1 to run against the live Rex server")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	city := fmt.Sprintf("e2e-p3-%d", time.Now().UnixNano())
	stateDir := t.TempDir()
	p := provider.New(city, cli.New(), provider.WithNamespace(provider.NewNamespaceForDirs(stateDir)))

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

	// AC #1: a second provider over the same state dir sees the same identity.
	b1, ok := p.Binding(name)
	if !ok {
		t.Fatal("Binding after Start: not found")
	}
	p2 := provider.New(city, cli.New(), provider.WithNamespace(provider.NewNamespaceForDirs(stateDir)))
	b2, ok := p2.Binding(name)
	if !ok {
		t.Fatal("second provider: Binding not found in shared state dir")
	}
	if b2.SessionID != b1.SessionID || b2.WindowID != b1.WindowID || b2.BlockID != b1.BlockID {
		t.Errorf("binding across providers differs: %+v vs %+v", b2, b1)
	}

	// AC #2/#7: a stale sidecar binding is cleared and re-resolved by label.
	e2eP3WriteStale(t, stateDir, name, b1, "gc:"+city)
	var out string
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		var err error
		if out, err = p2.Peek(ctx, name); err == nil && out != "" {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if out == "" {
		t.Fatal("Peek with stale binding never returned output; pane not re-resolved")
	}
	if fresh, ok := p2.Binding(name); !ok || fresh.BlockID == "block:bogus" {
		t.Errorf("stale binding not replaced: %+v (ok=%v)", fresh, ok)
	}

	// AC #3/#6: activity is reported with a real source and advances on output.
	first, err := p.GetLastActivity(ctx, name)
	if err != nil {
		t.Fatalf("GetLastActivity: %v", err)
	}
	if first.Source == provider.ActivityNone || first.At.IsZero() {
		t.Fatalf("first activity = %+v, want a source and non-zero time", first)
	}
	if err := p.Nudge(ctx, name, "echo PHASE3-ACTIVE"); err != nil {
		t.Fatalf("Nudge: %v", err)
	}
	second := first
	advanced := false
	deadline = time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if second, err = p.GetLastActivity(ctx, name); err != nil {
			t.Fatalf("GetLastActivity after nudge: %v", err)
		}
		if second.At.After(first.At) {
			advanced = true
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Logf("activity source=%s advanced=%v", second.Source, advanced)
	if second.At.IsZero() || second.Source == provider.ActivityNone {
		t.Errorf("second activity = %+v, want a source and non-zero time", second)
	}
	if !advanced && second.Source != provider.ActivityProgramStatus {
		t.Errorf("activity did not advance after output (source %s)", second.Source)
	}

	// AC #4: attachment is true only when the session has an app client and
	// the agent's rig tab is the active window still holding its block. A
	// fresh single-window scratch session reads true: its first tab is active.
	// A live call must never error.
	attached, err := p.IsAttached(ctx, name)
	if err != nil {
		t.Fatalf("IsAttached: %v", err)
	}
	clients, err := cli.New().ListClients(ctx, b1.SessionID)
	if err != nil {
		t.Fatalf("ListClients: %v", err)
	}
	hasApp := false
	for _, c := range clients {
		if c.Kind == "app" {
			hasApp = true
		}
	}
	t.Logf("IsAttached=%v appClient=%v", attached, hasApp)
	if attached {
		if !hasApp {
			t.Error("IsAttached = true with no app client on the session")
		}
		view, err := cli.New().SessionView(ctx, b1.SessionID)
		if err != nil {
			t.Fatalf("SessionView: %v", err)
		}
		if view.ActiveWindowID != b1.WindowID {
			t.Errorf("IsAttached = true but active window = %s, want %s", view.ActiveWindowID, b1.WindowID)
		}
		holds := false
		for _, w := range view.Windows {
			if w.ID != b1.WindowID {
				continue
			}
			for _, blk := range w.Blocks {
				if blk.ID == b1.BlockID {
					holds = true
				}
			}
		}
		if !holds {
			t.Errorf("IsAttached = true but window %s does not contain %s", b1.WindowID, b1.BlockID)
		}
	} else {
		t.Log("not attached: no app client, or the agent's tab is not the active window")
	}

	// AC #5: Stop removes the pane but leaves the shared city session.
	if err := p.Stop(ctx, name); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if running, err := p.IsRunning(ctx, name); err != nil || running {
		t.Errorf("IsRunning after Stop = %v, %v; want false, nil", running, err)
	}
	if out, err := p.Peek(ctx, name); err != nil || out != "" {
		t.Errorf("Peek after Stop = %q, %v; want empty, nil", out, err)
	}
	sessions, err := cli.New().ListSessions(ctx)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	found := false
	for _, s := range sessions {
		if s.Label == "gc:"+city {
			found = true
		}
	}
	if !found {
		t.Errorf("session gc:%s gone after Stop; teardown must leave the shared session", city)
	}
}

// e2eP3WriteStale overwrites name's sidecar binding with bogus window and
// block ids but the real session id and label.
func e2eP3WriteStale(t *testing.T, stateDir, name string, real provider.Binding, label string) {
	t.Helper()
	sum := sha256.Sum256([]byte(name))
	path := filepath.Join(stateDir, "bindings", hex.EncodeToString(sum[:])+".json")
	data, err := json.Marshal(map[string]any{
		"gc_name":           name,
		"rex_session_id":    string(real.SessionID),
		"rex_session_label": label,
		"window_id":         "window:bogus",
		"block_id":          "block:bogus",
		"launch_mode":       real.LaunchMode,
		"created_at":        time.Now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		t.Fatalf("marshal stale binding: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write stale binding: %v", err)
	}
}

// e2eHasSession reports whether a Rex session labelled label exists.
func e2eHasSession(ctx context.Context, t *testing.T, label string) bool {
	t.Helper()
	sessions, err := cli.New().ListSessions(ctx)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	for _, s := range sessions {
		if s.Label == label {
			return true
		}
	}
	return false
}

// e2eWindowsLabelled returns the windows in the session view whose label is
// label.
func e2eWindowsLabelled(ctx context.Context, t *testing.T, sessionID rex.SessionID, label string) []rex.Window {
	t.Helper()
	view, err := cli.New().SessionView(ctx, sessionID)
	if err != nil {
		t.Fatalf("SessionView: %v", err)
	}
	var out []rex.Window
	for _, w := range view.Windows {
		if w.Label == label {
			out = append(out, w)
		}
	}
	return out
}

// TestE2EAgentExitLiveness starts an agent whose command exits on its own.
// A launched command that exits closes its block, and the last block closing
// removes the tab. IsRunning must report false without an error, Peek must
// return nothing, the city session must survive, and the empty rig tab must be
// gone. It is opt-in because it mutates the user's Rex server:
//
//	REX_E2E=1 go test ./internal/provider -run TestE2EAgentExitLiveness -v
func TestE2EAgentExitLiveness(t *testing.T) {
	if os.Getenv("REX_E2E") == "" {
		t.Skip("set REX_E2E=1 to run against the live Rex server")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	city := fmt.Sprintf("e2e-exit-%d", time.Now().UnixNano())
	p := provider.New(city, cli.New(), provider.WithNamespace(provider.NewNamespaceForDirs(t.TempDir())))

	const name = "Indigo--dog-1"
	defer func() {
		_ = p.Stop(ctx, name)
		_ = exec.CommandContext(ctx, "rex", "session", "kill", "gc:"+city).Run()
	}()

	if err := p.Start(ctx, name, provider.Config{
		Argv:    []string{"/bin/sh", "-c", "echo EXIT-MARK; sleep 1; exit 4"},
		WorkDir: os.TempDir(),
	}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	b, ok := p.Binding(name)
	if !ok {
		t.Fatal("Binding after Start: not found")
	}

	// Poll until the command exits and its pane vanishes. A vanished pane is
	// not an error: the call that flips must be (false, nil).
	deadline := time.Now().Add(60 * time.Second)
	for {
		running, err := p.IsRunning(ctx, name)
		if err != nil {
			t.Fatalf("IsRunning: %v; a vanished pane must not be an error", err)
		}
		if !running {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("IsRunning still true 60s after the agent command exited")
		}
		time.Sleep(500 * time.Millisecond)
	}

	if out, err := p.Peek(ctx, name); err != nil || out != "" {
		t.Errorf("Peek after exit = %q, %v; want empty, nil", out, err)
	}

	// Teardown is block-only: the city session survives the tab going away.
	if !e2eHasSession(ctx, t, "gc:"+city) {
		t.Errorf("session gc:%s gone after the agent exited; it must survive", city)
	}

	// Closing the last pane removes the rig tab. Allow a moment for the
	// server to finish the removal.
	var left []rex.Window
	deadline = time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if left = e2eWindowsLabelled(ctx, t, b.SessionID, "Indigo"); len(left) == 0 {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if len(left) != 0 {
		t.Errorf("tab Indigo still present after its last pane closed: %+v", left)
	}
}

// TestE2EParallelStarts starts four agents in one rig concurrently, as gc
// does, and checks that topology creation does not race: one rig tab holding
// four distinct blocks. It is opt-in because it mutates the user's Rex server:
//
//	REX_E2E=1 go test ./internal/provider -run TestE2EParallelStarts -v
func TestE2EParallelStarts(t *testing.T) {
	if os.Getenv("REX_E2E") == "" {
		t.Skip("set REX_E2E=1 to run against the live Rex server")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	const agents = 4
	city := fmt.Sprintf("e2e-par-%d", time.Now().UnixNano())
	p := provider.New(city, cli.New(), provider.WithNamespace(provider.NewNamespaceForDirs(t.TempDir())))

	names := make([]string, agents)
	for i := range names {
		names[i] = fmt.Sprintf("Indigo--dog-%d", i+1)
	}
	defer func() {
		for _, name := range names {
			_ = p.Stop(ctx, name)
		}
		_ = exec.CommandContext(ctx, "rex", "session", "kill", "gc:"+city).Run()
	}()

	type startResult struct {
		name string
		err  error
	}
	results := make(chan startResult, agents)
	for _, name := range names {
		go func() {
			results <- startResult{name, p.Start(ctx, name, provider.Config{
				Argv:    []string{"/bin/sh"},
				WorkDir: os.TempDir(),
			})}
		}()
	}
	for range names {
		if r := <-results; r.err != nil {
			t.Errorf("Start %s: %v", r.name, r.err)
		}
	}
	if t.Failed() {
		t.FailNow()
	}

	first, ok := p.Binding(names[0])
	if !ok {
		t.Fatalf("Binding %s: not found", names[0])
	}
	windows := e2eWindowsLabelled(ctx, t, first.SessionID, "Indigo")
	if len(windows) != 1 {
		t.Fatalf("got %d windows labelled Indigo, want exactly 1: %+v", len(windows), windows)
	}
	if got := len(windows[0].Blocks); got != agents {
		t.Fatalf("Indigo window holds %d blocks, want %d", got, agents)
	}
	seen := map[rex.BlockID]bool{}
	for _, blk := range windows[0].Blocks {
		seen[blk.ID] = true
	}
	if len(seen) != agents {
		t.Errorf("got %d distinct block ids, want %d", len(seen), agents)
	}
	for _, name := range names {
		if _, ok := p.Binding(name); !ok {
			t.Errorf("Binding %s: not found", name)
		}
	}

	for _, name := range names {
		if err := p.Stop(ctx, name); err != nil {
			t.Errorf("Stop %s: %v", name, err)
		}
	}
	if left := e2eWindowsLabelled(ctx, t, first.SessionID, "Indigo"); len(left) != 0 {
		t.Errorf("tab Indigo still present after stopping all agents: %+v", left)
	}
	if !e2eHasSession(ctx, t, "gc:"+city) {
		t.Errorf("session gc:%s gone after Stop; teardown must leave the shared session", city)
	}
}
