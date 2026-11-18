package provider_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// piHookOverlayPath is the rex pack's per-provider Pi hook override. `go test`
// runs with the package directory as the working directory, so the path is
// relative to internal/provider.
const piHookOverlayPath = "../../pack/overlay/per-provider/pi/.pi/extensions/gc-hooks.js"

// fakeGC scripts the gc commands the Pi hook invokes. It records whether the
// queued-nudge drain ran, answers `hook current --id-only` with FAKE_CLAIM (or
// exit 1 when it is empty, matching gc's no-claim answer), and answers
// `bd list --id ... --all --json` with FAKE_BEAD_JSON ([] when unset).
const fakeGC = `#!/bin/sh
set -eu
case "$1 $2" in
  "hook current")
    if [ -n "${FAKE_CLAIM:-}" ]; then
      printf '%s\n' "$FAKE_CLAIM"
      exit 0
    fi
    echo "gc hook current: no current claim" >&2
    exit 1
    ;;
  "bd list")
    printf '%s' "${FAKE_BEAD_JSON:-[]}"
    exit 0
    ;;
  "nudge drain")
    : > "${FAKE_DRAIN_MARKER:?}"
    printf '<system-reminder>%s</system-reminder>\n' \
      "Run gc hook --claim --drain-ack --json now; if it returns work, execute it immediately."
    exit 0
    ;;
esac
exit 0
`

// piHookDriver loads the extension, captures its before_agent_start handler
// and prints the system prompt the handler produced.
const piHookDriver = `
const ext = require(process.argv[2]);
const handlers = {};
ext({ on(name, fn) { handlers[name] = fn; } });
const ctx = { cwd: process.cwd(), sessionManager: { getSessionId: () => "test-session" } };
const out = handlers.before_agent_start({ systemPrompt: "BASE" }, ctx);
process.stdout.write((out && out.systemPrompt) || "BASE");
`

// TestPiHookOverlayDefersNudgesWhileClaimIsLive proves the rex pack hook guard:
// a seat that still holds a live gc claim never has the queued pool-claim nudge
// injected, and the drain is delivered as soon as the stamp stops naming a held
// claim (closed, released, foreign-owned, or absent; so the nudge is deferred,
// not lost). Without the guard the extension drains and injects unconditionally
// and the first subtest fails.
func TestPiHookOverlayDefersNudgesWhileClaimIsLive(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not available")
	}
	hookPath, err := filepath.Abs(piHookOverlayPath)
	if err != nil {
		t.Fatalf("resolve hook overlay: %v", err)
	}
	if _, err := os.Stat(hookPath); err != nil {
		t.Fatalf("hook overlay %s: %v", hookPath, err)
	}

	runHook := func(t *testing.T, claim, beadJSON string) (output string, drained bool) {
		t.Helper()
		dir := t.TempDir()
		gcPath := filepath.Join(dir, "gc")
		if err := os.WriteFile(gcPath, []byte(fakeGC), 0o755); err != nil {
			t.Fatalf("write fake gc: %v", err)
		}
		driverPath := filepath.Join(dir, "driver.js")
		if err := os.WriteFile(driverPath, []byte(piHookDriver), 0o644); err != nil {
			t.Fatalf("write node driver: %v", err)
		}
		marker := filepath.Join(dir, "drained")

		cmd := exec.Command(node, driverPath, hookPath)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GC_BIN="+gcPath,
			"GC_BEAD_ID=mc-stale-dispatch", // claim detection must come from gc, not this env
			"GC_SESSION_ID=test-session",
			"GC_PI_TRANSCRIPT_DIR=",
			"FAKE_CLAIM="+claim,
			"FAKE_BEAD_JSON="+beadJSON,
			"FAKE_DRAIN_MARKER="+marker,
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("run hook under node: %v\n%s", err, out)
		}
		_, statErr := os.Stat(marker)
		return string(out), statErr == nil
	}

	t.Run("live claim defers the nudge", func(t *testing.T) {
		out, drained := runHook(t, "mc-claim-live", `[{"status":"in_progress","assignee":"seat-a","metadata":{"gc.session_id":"test-session"}}]`)
		if drained {
			t.Fatalf("nudge drain ran while a claim was held; output:\n%s", out)
		}
		if strings.Contains(out, "Run gc hook --claim --drain-ack") {
			t.Fatalf("pool-claim nudge injected while a claim was held; output:\n%s", out)
		}
	})

	t.Run("closed claim drains the nudge", func(t *testing.T) {
		out, drained := runHook(t, "mc-claim-stale", `[{"status":"closed","assignee":"seat-a","metadata":{"gc.session_id":"test-session"}}]`)
		if !drained {
			t.Fatalf("nudge drain did not run for a closed claim; output:\n%s", out)
		}
		if !strings.Contains(out, "Run gc hook --claim --drain-ack") {
			t.Fatalf("pool-claim nudge not delivered for a closed claim; output:\n%s", out)
		}
	})

	t.Run("released claim drains the nudge", func(t *testing.T) {
		out, drained := runHook(t, "mc-claim-released", `[{"status":"open","assignee":"","metadata":{}}]`)
		if !drained {
			t.Fatalf("nudge drain did not run for a released claim; output:\n%s", out)
		}
	})

	t.Run("no claim drains the nudge", func(t *testing.T) {
		out, drained := runHook(t, "", `[]`)
		if !drained {
			t.Fatalf("nudge drain did not run without a claim; output:\n%s", out)
		}
		if !strings.Contains(out, "Run gc hook --claim --drain-ack") {
			t.Fatalf("pool-claim nudge not delivered without a claim; output:\n%s", out)
		}
	})
}
