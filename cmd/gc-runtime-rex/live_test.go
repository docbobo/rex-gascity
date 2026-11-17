package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestE2EDispatcherLifecycle drives the live Rex server through the
// dispatcher: start, is-running, peek, stop, each as a separate run call over a
// shared state dir, the way gc invokes one process per op. It is opt-in because
// it mutates the user's Rex server:
//
//	REX_E2E=1 go test ./cmd/gc-runtime-rex -run TestE2E -v
func TestE2EDispatcherLifecycle(t *testing.T) {
	if os.Getenv("REX_E2E") == "" {
		t.Skip("set REX_E2E=1 to run against the live Rex server")
	}

	city := fmt.Sprintf("e2e-rpp-%d", time.Now().UnixNano())
	t.Setenv("GC_REX_STATE_DIR", t.TempDir())
	t.Setenv("GC_REX_CITY", city)
	d := liveDeps()

	call := func(stdin string, args ...string) result {
		t.Helper()
		var stdout, stderr bytes.Buffer
		code := run(args, strings.NewReader(stdin), &stdout, &stderr, d)
		return result{code: code, stdout: stdout.String(), stderr: stderr.String()}
	}

	const (
		name     = "Indigo--dog-1"
		sentinel = "RPP-E2E-SENTINEL"
	)
	defer func() {
		call("", "stop", name)
		// Best-effort cleanup of the scratch city session.
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = exec.CommandContext(ctx, "rex", "session", "kill", "gc:"+city).Run()
	}()

	cfg := fmt.Sprintf(`{"work_dir":%q,"command":"sh -c 'echo %s; sleep 300'"}`, os.TempDir(), sentinel)
	if r := call(cfg, "start", name); r.code != exitOK {
		t.Fatalf("start: exit %d, stderr %q", r.code, r.stderr)
	}
	if r := call("", "is-running", name); r.code != exitOK || r.stdout != "true\n" {
		t.Fatalf("is-running after start: exit %d, stdout %q, stderr %q", r.code, r.stdout, r.stderr)
	}

	var out string
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		r := call("", "peek", name, "50")
		if r.code != exitOK {
			t.Fatalf("peek: exit %d, stderr %q", r.code, r.stderr)
		}
		if out = r.stdout; strings.Contains(out, sentinel) {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if !strings.Contains(out, sentinel) {
		t.Fatalf("peek never showed %q; last output:\n%s", sentinel, out)
	}

	if r := call("", "stop", name); r.code != exitOK {
		t.Fatalf("stop: exit %d, stderr %q", r.code, r.stderr)
	}
	if r := call("", "is-running", name); r.code != exitOK || r.stdout != "false\n" {
		t.Fatalf("is-running after stop: exit %d, stdout %q, stderr %q", r.code, r.stdout, r.stderr)
	}
	if r := call("", "stop", name); r.code != exitOK {
		t.Fatalf("second stop: exit %d, stderr %q", r.code, r.stderr)
	}
}
