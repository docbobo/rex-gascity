package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestStartFreshOnlyExits73 pins GCREX-56 AC2 through the real dispatcher:
// fresh_only refuses a held name with herdr-core's FRESH_COLLISION (73,
// asserted literally so a production constant rename cannot hide a change;
// herdrcore/exitcodes.py:24-25), and the message carries gc's "already exists"
// collision phrase, which gc lower-cases and matches as a substring
// (gc v1.5.1 internal/runtime/exec/exec.go:144).
func TestStartFreshOnlyExits73(t *testing.T) {
	h, _ := newHarness(t)
	h.start(dog)
	r := h.run(`{"command":"sleep 300","fresh_only":true}`, "start", dog)
	assertFailure(t, r, 73)
	if !strings.Contains(r.stderr, "already exists") {
		t.Errorf("stderr = %q, want gc's \"already exists\" collision phrase", r.stderr)
	}
	if !strings.Contains(r.stderr, "fresh_only") {
		t.Errorf("stderr = %q, want a fresh_only collision message", r.stderr)
	}
}

// TestStartRunsSessionSetupAndWarnsSessionLive pins GCREX-56 AC2 through the
// dispatcher: session_setup runs, and a non-empty session_live writes its
// SESSION_LIVE_SKIPPED warning to stderr on an otherwise successful start.
func TestStartRunsSessionSetupAndWarnsSessionLive(t *testing.T) {
	h, _ := newHarness(t)
	dir := t.TempDir()
	out := filepath.Join(dir, "setup.txt")
	cfg := fmt.Sprintf(`{"command":"sleep 300","work_dir":%q,"session_setup":["printf done > %s"],"session_live":["set -g status"]}`, dir, out)
	r := h.must(cfg, "start", dog)
	if r.stdout != "" {
		t.Errorf("start stdout = %q, want empty", r.stdout)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("session_setup did not run: %v", err)
	}
	if strings.TrimSpace(string(data)) != "done" {
		t.Errorf("session_setup output = %q, want done", data)
	}
	if !strings.Contains(r.stderr, "session_live is not applied") {
		t.Errorf("stderr = %q, want the SESSION_LIVE_SKIPPED warning", r.stderr)
	}
}

// TestStartStagesOverlayDirThroughDispatcher pins GCREX-56 AC2 end to end: an
// overlay_dir's per-provider subtree reaches the work dir before the launch.
func TestStartStagesOverlayDirThroughDispatcher(t *testing.T) {
	h, _ := newHarness(t)
	overlay := t.TempDir()
	if err := os.MkdirAll(filepath.Join(overlay, "per-provider", "pi"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(overlay, "per-provider", "pi", "hook.js"), []byte("override"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	workDir := filepath.Join(t.TempDir(), "work")
	cfg := fmt.Sprintf(`{"command":"sleep 300","work_dir":%q,"env":{"GC_PROVIDER":"pi"},"overlay_dir":%q}`, workDir, overlay)
	h.must(cfg, "start", dog)
	data, err := os.ReadFile(filepath.Join(workDir, "hook.js"))
	if err != nil {
		t.Fatalf("per-provider overlay not staged: %v", err)
	}
	if string(data) != "override" {
		t.Errorf("staged override = %q, want override", data)
	}
}
