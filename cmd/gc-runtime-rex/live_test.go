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

	"github.com/docbobo/rex-gascity/internal/rex/fakerex"
	"github.com/docbobo/rex-gascity/internal/rex/fakerex/fakestate"
)

// e2eLive reports whether the e2e tests drive the live Rex server. The switch
// matches internal/provider's e2e helper: REX_E2E unset or "fake" is the
// fake-rex harness; "1" or "live" is the live server; any other value fails
// the test.
func e2eLive(t *testing.T) bool {
	t.Helper()
	switch v := os.Getenv("REX_E2E"); v {
	case "", "fake":
		return false
	case "1", "live":
		if _, err := exec.LookPath("rex"); err != nil {
			t.Skipf("REX_E2E=%s selects the live Rex server, but rex is not on PATH", v)
		}
		return true
	default:
		t.Fatalf("REX_E2E=%q: want unset, \"fake\", \"1\", or \"live\"", v)
		return false
	}
}

// e2eEnv points the dispatcher at the fake-rex binary by default and returns
// the fake so a test can inspect its state. In live mode it returns the live
// deps and a nil fake. The binary is the raw `rex` path the version-skew test
// calls directly.
func e2eEnv(t *testing.T) (deps, string, *fakerex.Fake) {
	t.Helper()
	if e2eLive(t) {
		return liveDeps(), "rex", nil
	}
	f := fakerex.New(t)
	t.Setenv("GC_REX_BIN", f.Bin)
	return liveDeps(), f.Bin, f
}

// TestE2EDispatcherLifecycle drives the Rex server through the dispatcher:
// start, is-running, peek, stop, each as a separate run call over a shared
// state dir, the way gc invokes one process per op. It runs against the
// fake-rex harness by default. Set REX_E2E=1 or REX_E2E=live to drive the
// live server, which mutates the user's Rex server:
//
//	REX_E2E=1 go test ./cmd/gc-runtime-rex -run TestE2E -v
func TestE2EDispatcherLifecycle(t *testing.T) {
	d, _, f := e2eEnv(t)

	city := fmt.Sprintf("e2e-rpp-%d", time.Now().UnixNano())
	t.Setenv("GC_REX_STATE_DIR", t.TempDir())
	t.Setenv("GC_REX_CITY", city)

	call := func(stdin string, args ...string) result {
		t.Helper()
		var stdout, stderr bytes.Buffer
		code := run(args, strings.NewReader(stdin), &stdout, &stderr, d)
		return result{code: code, stdout: stdout.String(), stderr: stderr.String()}
	}

	const (
		name        = "Indigo--dog-1"
		sentinel    = "RPP-E2E-SENTINEL"
		envSentinel = "RPP_E2E_ENV"
		envValue    = "RPP-E2E-ENV-VALUE"
	)
	// printenv emits only the seeded variable, so the pane output stays two
	// lines and no unrelated environment leaks into a failure log.
	defer func() {
		call("", "stop", name)
		if e2eLive(t) {
			// Best-effort cleanup of the scratch city session.
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			_ = exec.CommandContext(ctx, "rex", "session", "kill", "gc:"+city).Run()
		}
	}()

	cfg := fmt.Sprintf(`{"work_dir":%q,"command":"sh -c 'printenv %s; echo %s; sleep 300'","env":{%q:%q}}`,
		os.TempDir(), envSentinel, sentinel, envSentinel, envValue)
	if r := call(cfg, "start", name); r.code != exitOK {
		t.Fatalf("start: exit %d, stderr %q", r.code, r.stderr)
	}
	if f != nil {
		// The provider resolved the fake, not a PATH rex: the fake's own state
		// dir carries the session the start created.
		if got := len(f.State(t).Sessions); got != 1 {
			t.Fatalf("fake-rex saw %d sessions after start, want 1; GC_REX_BIN did not resolve the fake", got)
		}
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
		if out = r.stdout; strings.Contains(out, sentinel) && strings.Contains(out, envValue) {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if !strings.Contains(out, sentinel) {
		t.Fatalf("peek never showed %q; last output:\n%s", sentinel, out)
	}
	if !strings.Contains(out, envValue) {
		t.Fatalf("peek never showed the seeded env value in the pane; last output:\n%s", out)
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

// e2eVersionSkew checks the protocol handshake and that the server's API
// revision is inside the range the installed rex CLI supports. It returns an
// error on a mismatch so the fake and live cases can share it.
func e2eVersionSkew(t *testing.T, bin string) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	verOut, err := exec.CommandContext(ctx, bin, "version").Output()
	if err != nil {
		return fmt.Errorf("rex version: %w", err)
	}
	verText := string(verOut)
	m := cliAPILine.FindStringSubmatch(verText)
	if m == nil {
		return fmt.Errorf("no %q line in rex version output:\n%s", cliAPILine, verText)
	}
	cliRev, _ := strconv.Atoi(m[1])
	minRev, _ := strconv.Atoi(m[2])
	maxRev, _ := strconv.Atoi(m[3])

	statOut, err := exec.CommandContext(ctx, bin, "api", "call", "server.version").Output()
	if err != nil {
		return fmt.Errorf("rex api call server.version: %w", err)
	}
	var status struct {
		APIRevision *int `json:"api_revision"`
	}
	if err := json.Unmarshal(statOut, &status); err != nil {
		return fmt.Errorf("server.version is not JSON: %w\n%s", err, statOut)
	}
	if status.APIRevision == nil {
		return fmt.Errorf("server.version has no api_revision:\n%s", statOut)
	}
	serverRev := *status.APIRevision

	first, _, _ := strings.Cut(strings.TrimSpace(verText), "\n")
	t.Logf("rex CLI: %s; api revision %d, supports %d to %d; server api_revision %d",
		first, cliRev, minRev, maxRev, serverRev)
	if serverRev < minRev || serverRev > maxRev {
		return fmt.Errorf("server api_revision %d is outside the CLI's supported range %d..%d", serverRev, minRev, maxRev)
	}
	return nil
}

// TestE2EVersionSkew checks the protocol handshake and that the server's API
// revision is inside the range the installed rex CLI supports. It does not
// create sessions. It runs against the fake-rex harness by default; set
// REX_E2E=1 or REX_E2E=live to drive the live server:
//
//	REX_E2E=1 go test ./cmd/gc-runtime-rex -run TestE2EVersionSkew -v
func TestE2EVersionSkew(t *testing.T) {
	d, bin, _ := e2eEnv(t)

	var stdout, stderr bytes.Buffer
	if code := run([]string{"protocol"}, strings.NewReader(""), &stdout, &stderr, d); code != exitOK {
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
	for _, want := range []string{"report-attachment", "report-activity", "city-scoped"} {
		if !slices.Contains(hs.Capabilities, want) {
			t.Errorf("capabilities %v missing %q", hs.Capabilities, want)
		}
	}

	if err := e2eVersionSkew(t, bin); err != nil {
		t.Error(err)
	}
}

// TestE2EVersionSkewMismatch drives the fake's server api_revision past the
// CLI's supported range and proves the skew check rejects it. The live server's
// revision cannot be changed, so this case is fake-only.
func TestE2EVersionSkewMismatch(t *testing.T) {
	if e2eLive(t) {
		t.Skip("the live server's revision cannot be changed")
	}
	f := fakerex.New(t)
	f.Update(t, func(st *fakestate.State) { st.APIVersion = 99 })
	if err := e2eVersionSkew(t, f.Bin); err == nil {
		t.Fatal("version skew check passed with server api_revision 99 outside the CLI's 0..14 range")
	}
}
