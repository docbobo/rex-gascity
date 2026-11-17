package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strconv"
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

var cliAPILine = regexp.MustCompile(`api: revision (\d+) \(supports (\d+) to (\d+)\)`)

// TestE2EVersionSkew checks the protocol handshake and that the live server's
// API revision is inside the range the installed rex CLI supports. It does not
// create sessions, so it is safe against a shared server. It is opt-in:
//
//	REX_E2E=1 go test ./cmd/gc-runtime-rex -run TestE2EVersionSkew -v
func TestE2EVersionSkew(t *testing.T) {
	if os.Getenv("REX_E2E") == "" {
		t.Skip("set REX_E2E=1 to run against the live Rex server")
	}

	var stdout, stderr bytes.Buffer
	if code := run([]string{"protocol"}, strings.NewReader(""), &stdout, &stderr, liveDeps()); code != exitOK {
		t.Fatalf("protocol: exit %d, stderr %q", code, stderr.String())
	}
	var hs struct {
		Version      int      `json:"version"`
		Capabilities []string `json:"capabilities"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &hs); err != nil {
		t.Fatalf("protocol stdout is not JSON: %v\n%s", err, stdout.String())
	}
	if hs.Version != 0 {
		t.Errorf("protocol version = %d, want 0", hs.Version)
	}
	for _, want := range []string{"report-attachment", "report-activity"} {
		if !slices.Contains(hs.Capabilities, want) {
			t.Errorf("capabilities %v missing %q", hs.Capabilities, want)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	verOut, err := exec.CommandContext(ctx, "rex", "version").Output()
	if err != nil {
		t.Fatalf("rex version: %v", err)
	}
	verText := string(verOut)
	m := cliAPILine.FindStringSubmatch(verText)
	if m == nil {
		t.Fatalf("no %q line in rex version output:\n%s", cliAPILine, verText)
	}
	cliRev, _ := strconv.Atoi(m[1])
	minRev, _ := strconv.Atoi(m[2])
	maxRev, _ := strconv.Atoi(m[3])

	statOut, err := exec.CommandContext(ctx, "rex", "api", "call", "server.status").Output()
	if err != nil {
		t.Fatalf("rex api call server.status: %v", err)
	}
	var status struct {
		APIRevision *int `json:"api_revision"`
	}
	if err := json.Unmarshal(statOut, &status); err != nil {
		t.Fatalf("server.status is not JSON: %v\n%s", err, statOut)
	}
	if status.APIRevision == nil {
		t.Fatalf("server.status has no api_revision:\n%s", statOut)
	}
	serverRev := *status.APIRevision

	first, _, _ := strings.Cut(strings.TrimSpace(verText), "\n")
	t.Logf("rex CLI: %s; api revision %d, supports %d to %d; server api_revision %d",
		first, cliRev, minRev, maxRev, serverRev)
	if serverRev < minRev || serverRev > maxRev {
		t.Errorf("server api_revision %d is outside the CLI's supported range %d..%d", serverRev, minRev, maxRev)
	}
}
