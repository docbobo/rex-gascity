package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/docbobo/rex-gascity/internal/rex"
	"github.com/docbobo/rex-gascity/internal/rex/faketest"
)

// result is one dispatcher invocation's observable outcome.
type result struct {
	code   int
	stdout string
	stderr string
}

// harness runs the dispatcher against one shared fake Rex client, the way
// separate gc-runtime-rex processes share one Rex server. Each call builds a
// fresh provider, so state crosses calls only through the on-disk state dir.
type harness struct {
	t      *testing.T
	client rex.Client
}

func newHarness(t *testing.T) (*harness, *faketest.Fake) {
	t.Helper()
	t.Setenv("GC_REX_STATE_DIR", t.TempDir())
	t.Setenv("GC_REX_CITY", "testcity")
	fake := faketest.New()
	return &harness{t: t, client: fake}, fake
}

func (h *harness) run(stdin string, args ...string) result {
	h.t.Helper()
	var stdout, stderr bytes.Buffer
	d := deps{newClient: func() rex.Client { return h.client }}
	code := run(args, strings.NewReader(stdin), &stdout, &stderr, d)
	return result{code: code, stdout: stdout.String(), stderr: stderr.String()}
}

// must runs args and fails the test unless it exits 0.
func (h *harness) must(stdin string, args ...string) result {
	h.t.Helper()
	r := h.run(stdin, args...)
	if r.code != exitOK {
		h.t.Fatalf("%v: exit %d, stderr %q", args, r.code, r.stderr)
	}
	return r
}

func (h *harness) start(name string) {
	h.t.Helper()
	h.must(`{"command":"sleep 300"}`, "start", name)
}

func assertFailure(t *testing.T, r result, code int) {
	t.Helper()
	if r.code != code {
		t.Errorf("exit = %d, want %d (stderr %q)", r.code, code, r.stderr)
	}
	if r.stdout != "" {
		t.Errorf("stdout = %q on failure, want empty", r.stdout)
	}
	if !strings.HasPrefix(r.stderr, "gc-runtime-rex: ") {
		t.Errorf("stderr = %q, want a gc-runtime-rex: message", r.stderr)
	}
}

const dog = "Indigo--dog-1"

func TestProtocolHandshake(t *testing.T) {
	h, _ := newHarness(t)
	r := h.must("", "protocol")
	want := `{"version":0,"capabilities":["report-attachment","report-activity","idle-boundary","carrier.ops","input-outcome"]}` + "\n"
	if r.stdout != want {
		t.Fatalf("stdout = %q, want %q", r.stdout, want)
	}
	var hs map[string]any
	if err := json.Unmarshal([]byte(r.stdout), &hs); err != nil {
		t.Fatalf("handshake is not JSON: %v", err)
	}
	if _, ok := hs["locality"]; ok {
		t.Error("handshake has a locality field")
	}
	if v, _ := hs["version"].(float64); v != 0 {
		t.Errorf("version = %v, want 0", hs["version"])
	}
	var caps []string
	for _, c := range hs["capabilities"].([]any) {
		caps = append(caps, c.(string))
	}
	if !slices.Equal(caps, []string{"report-attachment", "report-activity", "idle-boundary", "carrier.ops", "input-outcome"}) {
		t.Errorf("capabilities = %v", caps)
	}
}

func TestUnknownAndUnimplementedOps(t *testing.T) {
	h, _ := newHarness(t)
	for _, args := range [][]string{
		{"nosuchop", dog},
		{"exec", dog},
		{"provision", dog},
		{"watch-startup", dog},
		{},
	} {
		r := h.run("echo hi", args...)
		if r.code != exitUnknown || r.stdout != "" {
			t.Errorf("%v: exit %d stdout %q, want 2 and empty", args, r.code, r.stdout)
		}
	}
}

func TestMissingNameIsFailure(t *testing.T) {
	h, _ := newHarness(t)
	for _, args := range [][]string{
		{"start"}, {"stop"}, {"is-running"}, {"interrupt"}, {"nudge"},
		{"send-keys"}, {"clear-scrollback"}, {"process-alive"}, {"peek"},
		{"get-last-activity"}, {"is-attached"},
		{"set-meta"}, {"get-meta"}, {"remove-meta"},
		{"set-meta", dog}, {"get-meta", dog}, {"remove-meta", dog},
		{"stop", ""},
	} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			assertFailure(t, h.run("", args...), exitFail)
		})
	}
}

// TestMetaOpsRejectEmptyKey pins herdr-core's rule: set-meta/get-meta/
// remove-meta reject an empty key with exit 1 and a stderr message, instead of
// silently operating on a key named "".
func TestMetaOpsRejectEmptyKey(t *testing.T) {
	h, _ := newHarness(t)
	for _, op := range []string{"set-meta", "get-meta", "remove-meta"} {
		t.Run(op, func(t *testing.T) {
			r := h.run("value", op, dog, "")
			assertFailure(t, r, exitFail)
			if !strings.Contains(r.stderr, "key") {
				t.Errorf("stderr = %q, want a key diagnostic", r.stderr)
			}
		})
	}
}

func TestLifecycle(t *testing.T) {
	h, fake := newHarness(t)
	workDir := t.TempDir()
	cfg := fmt.Sprintf(`{"work_dir":%q,"command":"claude --model 'opus 4'","ready_prompt_prefix":"> "}`, workDir)
	r := h.must(cfg, "start", dog)
	if r.stdout != "" {
		t.Errorf("start stdout = %q", r.stdout)
	}
	if len(fake.CreatedWindows) != 1 {
		t.Fatalf("created windows = %d, want 1", len(fake.CreatedWindows))
	}
	spec := fake.CreatedWindows[0].Spec
	if !slices.Equal(spec.Command, []string{"claude", "--model", "opus 4"}) || spec.Cwd != workDir {
		t.Errorf("launch spec = %+v", spec)
	}
	if got := h.must("", "is-running", dog).stdout; got != "true\n" {
		t.Errorf("is-running after start = %q", got)
	}
	h.must("", "stop", dog)
	if got := h.must("", "is-running", dog).stdout; got != "false\n" {
		t.Errorf("is-running after stop = %q", got)
	}
	h.must("", "stop", dog)
	if got := h.must("", "is-running", "never-started").stdout; got != "false\n" {
		t.Errorf("is-running never started = %q", got)
	}
}

func TestStartAppliesEnvFromStartConfig(t *testing.T) {
	h, fake := newHarness(t)
	h.must(`{"command":"sleep 300","env":{"GC_CITY":"/tmp/city","GC_SESSION_ID":"s-1"}}`, "start", dog)
	if len(fake.CreatedWindows) != 1 {
		t.Fatalf("created windows = %d, want 1", len(fake.CreatedWindows))
	}
	run := fake.CreatedWindows[0].Spec.Command
	if len(run) == 0 || run[0] != "env" {
		t.Fatalf("launch command = %v, want an env wrapper", run)
	}
	for _, want := range []string{"GC_CITY=/tmp/city", "GC_SESSION_ID=s-1"} {
		if !slices.Contains(run, want) {
			t.Errorf("launch command %v missing %q", run, want)
		}
	}
}

func TestDuplicateStartFails(t *testing.T) {
	h, fake := newHarness(t)
	h.start(dog)
	r := h.run(`{"command":"sleep 300"}`, "start", dog)
	assertFailure(t, r, exitFail)
	if !strings.Contains(r.stderr, "already running") {
		t.Errorf("stderr = %q, want already running", r.stderr)
	}
	if len(fake.CreatedWindows)+len(fake.CreatedSplits) != 1 {
		t.Errorf("duplicate start launched a second pane")
	}
}

func TestPeekLines(t *testing.T) {
	h, fake := newHarness(t)
	h.start(dog)
	fake.Formats = map[rex.BlockID]string{"block:dog-1": "a\nb\nc\nd\n"}
	if got := h.must("", "peek", dog, "2").stdout; got != "c\nd\n" {
		t.Errorf("peek 2 = %q", got)
	}
	if got := h.must("", "peek", dog).stdout; got != "a\nb\nc\nd\n" {
		t.Errorf("peek = %q", got)
	}
	assertFailure(t, h.run("", "peek", dog, "many"), exitFail)
}

// assertNoScreen pins herdr-core's peek failure: exit 66, empty stdout, and
// `no screen for '<name>'` on stderr.
func assertNoScreen(t *testing.T, r result, name string) {
	t.Helper()
	assertFailure(t, r, exitNoScreen)
	if want := fmt.Sprintf("no screen for '%s'", name); !strings.Contains(r.stderr, want) {
		t.Errorf("stderr = %q, want %q", r.stderr, want)
	}
}

func TestPeekNoScreenWhenNoRecord(t *testing.T) {
	h, _ := newHarness(t)
	assertNoScreen(t, h.run("", "peek", dog), dog)
	assertNoScreen(t, h.run("", "peek", dog, "5"), dog)
}

func TestPeekNoScreenWhenPaneVanished(t *testing.T) {
	h, fake := newHarness(t)
	h.start(dog)
	fake.FormatErr = fmt.Errorf("rex block.method: %w", rex.ErrNotFound)
	assertNoScreen(t, h.run("", "peek", dog, "2"), dog)
}

func TestPeekNoScreenWhenResolveNotFound(t *testing.T) {
	h, fake := newHarness(t)
	fake.ListSessionsErr = fmt.Errorf("rex session.list: %w", rex.ErrNotFound)
	assertNoScreen(t, h.run("", "peek", dog), dog)
}

func TestPeekNoScreenWhenRexUnavailable(t *testing.T) {
	h, fake := newHarness(t)
	h.start(dog)
	fake.FormatErr = fmt.Errorf("rex block.method: %w", rex.ErrUnavailable)
	assertNoScreen(t, h.run("", "peek", dog), dog)

	fake.FormatErr = nil
	fake.SessionViewErr = fmt.Errorf("rex session.view: %w", rex.ErrUnavailable)
	assertNoScreen(t, h.run("", "peek", dog), dog)

	// An unrelated failure is still a plain exit 1, not "no screen".
	fake.SessionViewErr = nil
	fake.FormatErr = errors.New("boom")
	assertFailure(t, h.run("", "peek", dog), exitFail)
}

func TestListRunning(t *testing.T) {
	h, _ := newHarness(t)
	h.start(dog)
	h.start("Indigo--dog-2")
	h.start("Other--cat")
	h.must("", "stop", "Indigo--dog-2")
	if got := h.must("", "list-running").stdout; got != "Indigo--dog-1\nOther--cat\n" {
		t.Errorf("list-running = %q", got)
	}
	if got := h.must("", "list-running", "Indigo--").stdout; got != "Indigo--dog-1\n" {
		t.Errorf("list-running Indigo-- = %q", got)
	}
}

// TestListRunningUnreachableRexIsEmptyExitZero pins AC1 through the real
// dispatcher: a recorded binding exists, but Rex is unreachable, so
// list-running must answer the parked-city inventory, empty stdout and exit 0,
// not exit 1 (GCREX-41).
func TestListRunningUnreachableRexIsEmptyExitZero(t *testing.T) {
	h, fake := newHarness(t)
	h.start(dog)
	fake.ProcessErr = rex.ErrUnavailable
	r := h.run("", "list-running")
	if r.code != exitOK || r.stdout != "" {
		t.Errorf("list-running with Rex unreachable = exit %d, stdout %q; want exit 0, empty", r.code, r.stdout)
	}
}

// TestStopUnreachableRexUnknownNameExitsZero pins GCREX-51 REQ-001 through the
// real dispatcher: Rex is down and the name was never started, so stop is a
// no-op success with empty stdout and stderr.
func TestStopUnreachableRexUnknownNameExitsZero(t *testing.T) {
	h, fake := newHarness(t)
	fake.ListSessionsErr = rex.ErrUnavailable
	r := h.run("", "stop", dog)
	if r.code != exitOK || r.stdout != "" || r.stderr != "" {
		t.Errorf("stop with Rex unreachable, no binding = exit %d, stdout %q, stderr %q; want exit 0, empty", r.code, r.stdout, r.stderr)
	}
}

// TestSetMetaUnknownSessionFailsExitOne pins GCREX-49: set-meta requires a
// session record, so a name that was never started exits 1, writes nothing to
// stdout, and leaves no value behind.
func TestSetMetaUnknownSessionFailsExitOne(t *testing.T) {
	h, _ := newHarness(t)
	assertFailure(t, h.run("value", "set-meta", "Indigo--ghost-1", "GC_KEY"), exitFail)
	if got := h.must("", "get-meta", "Indigo--ghost-1", "GC_KEY").stdout; got != "" {
		t.Errorf("value stored by a rejected set-meta: %q", got)
	}
}

func TestMetaRoundTripAcrossCalls(t *testing.T) {
	h, _ := newHarness(t)
	// set-meta requires a session record, so start the pane first.
	h.start(dog)
	const value = "line one\n  spaced\ttab\n\n"
	h.must(value, "set-meta", dog, "GC_KEY")
	if got := h.must("", "get-meta", dog, "GC_KEY").stdout; got != value {
		t.Errorf("get-meta = %q, want %q", got, value)
	}
	h.must("", "remove-meta", dog, "GC_KEY")
	if got := h.must("", "get-meta", dog, "GC_KEY").stdout; got != "" {
		t.Errorf("get-meta after remove = %q, want empty", got)
	}
	if got := h.must("", "get-meta", dog, "never-set").stdout; got != "" {
		t.Errorf("get-meta unset = %q", got)
	}
}

func TestProcessAlive(t *testing.T) {
	h, fake := newHarness(t)
	h.start(dog)
	fake.Processes = map[rex.BlockID]rex.ProcessInfo{
		"block:dog-1": {Foreground: &rex.Process{Name: "sleep"}},
	}
	if got := h.must("sleep\n", "process-alive", dog).stdout; got != "true\n" {
		t.Errorf("process-alive sleep = %q", got)
	}
	if got := h.must("claude\nnode\n", "process-alive", dog).stdout; got != "false\n" {
		t.Errorf("process-alive claude = %q", got)
	}
	if got := h.must("sleep\n", "process-alive", "never-started").stdout; got != "false\n" {
		t.Errorf("process-alive missing = %q", got)
	}
	fake.ProcessErr = fmt.Errorf("rex block.method: %w", rex.ErrNotFound)
	if got := h.must("sleep\n", "process-alive", dog).stdout; got != "false\n" {
		t.Errorf("process-alive after pane vanished = %q, want false", got)
	}
}

// TestProcessAliveTransientErrorIsIndeterminate pins the medium deviation: an
// unreadable process table must answer exit 0 with empty stdout (gc reads that
// as alive), not exit 1 with stderr (gc reads an error as dead).
func TestProcessAliveTransientErrorIsIndeterminate(t *testing.T) {
	h, fake := newHarness(t)
	h.start(dog)
	fake.Processes = map[rex.BlockID]rex.ProcessInfo{
		"block:dog-1": {Foreground: &rex.Process{Name: "sleep"}},
	}
	fake.ProcessErr = errors.New("rex block.method: connection reset")
	r := h.run("claude\n", "process-alive", dog)
	if r.code != exitOK {
		t.Errorf("exit = %d, want %d (stderr %q)", r.code, exitOK, r.stderr)
	}
	if r.stdout != "" {
		t.Errorf("stdout = %q, want empty (cannot tell)", r.stdout)
	}
}

// TestProcessAliveEmptyTableIsIndeterminate: with no readable process at all
// the answer is cannot-tell (exit 0, empty stdout), not a spurious dead.
func TestProcessAliveEmptyTableIsIndeterminate(t *testing.T) {
	h, fake := newHarness(t)
	h.start(dog)
	fake.Processes = map[rex.BlockID]rex.ProcessInfo{"block:dog-1": {}}
	r := h.must("claude\n", "process-alive", dog)
	if r.stdout != "" {
		t.Errorf("stdout = %q, want empty (cannot tell)", r.stdout)
	}
}

func TestGetLastActivity(t *testing.T) {
	h, fake := newHarness(t)
	h.start(dog)
	at := time.Date(2026, 11, 17, 19, 0, 0, 0, time.FixedZone("CET", 3600))
	fake.ProgramStatuses = map[rex.BlockID]rex.ProgramStatus{
		"block:dog-1": {Records: []rex.ProgramStatusRecord{{ID: "x", State: "idle", UpdatedAt: at}}},
	}
	if got := h.must("", "get-last-activity", dog).stdout; got != "2026-11-17T18:00:00Z\n" {
		t.Errorf("get-last-activity = %q", got)
	}
	if got := h.must("", "get-last-activity", "never-started").stdout; got != "" {
		t.Errorf("get-last-activity missing = %q, want empty", got)
	}
	fake.ProgramStatusErr = errors.New("boom")
	assertFailure(t, h.run("", "get-last-activity", dog), exitFail)
}

func TestIsAttached(t *testing.T) {
	h, fake := newHarness(t)
	h.start(dog)
	if got := h.must("", "is-attached", dog).stdout; got != "false\n" {
		t.Errorf("is-attached without clients = %q", got)
	}
	fake.Clients = []rex.ClientInfo{{ID: "c1", Kind: "app"}}
	fake.View.ActiveWindowID = fake.View.Windows[0].ID
	if got := h.must("", "is-attached", dog).stdout; got != "true\n" {
		t.Errorf("is-attached with app client = %q", got)
	}
	fake.Clients = []rex.ClientInfo{{ID: "c1", Kind: "martian"}}
	if got := h.must("", "is-attached", dog).stdout; got != "true\n" {
		t.Errorf("is-attached with unclassifiable client kind = %q, want true", got)
	}
}

// downRex fails the read calls attach might use, modeling an unreachable Rex
// server. attach must not surface that as a failure: it answers the hint
// without consulting Rex at all.
type downRex struct{ *faketest.Fake }

func (downRex) SessionView(context.Context, rex.SessionID) (rex.SessionView, error) {
	return rex.SessionView{}, errors.New("rex: connection refused")
}

func (downRex) ListClients(context.Context, rex.SessionID) ([]rex.ClientInfo, error) {
	return nil, errors.New("rex: connection refused")
}

func TestAttachSucceedsWithHint(t *testing.T) {
	for _, c := range []struct {
		name   string
		client func() rex.Client
		start  bool
	}{
		{"live pane", func() rex.Client { return faketest.New() }, true},
		{"unknown session", func() rex.Client { return faketest.New() }, false},
		{"rex unreachable", func() rex.Client { return downRex{faketest.New()} }, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			h, _ := newHarness(t)
			if c.start {
				h.must(`{"command":"sleep 300"}`, "start", dog)
			}
			h.client = c.client()
			r := h.must("", "attach", dog)
			if !strings.Contains(r.stdout, dog) {
				t.Errorf("attach stdout = %q, want the hint naming %q", r.stdout, dog)
			}
			if r.stderr != "" {
				t.Errorf("attach stderr = %q, want empty", r.stderr)
			}
		})
	}
}

// TestInputOpsOnMissingSessionAnswerGone pins the declared input-outcome
// contract: gc reads a driving op's exit 80 and its one outcome line only when
// the handshake declares input-outcome and carrier.ops
// (gc internal/runtime/exec/outcome.go:193, outcomesRead), so a missing pane
// answers `gone` instead of the old best-effort exit 0. Zero keys stay a no-op.
func TestInputOpsOnMissingSessionAnswerGone(t *testing.T) {
	h, fake := newHarness(t)
	for _, c := range []struct {
		stdin string
		args  []string
	}{
		{"hello\n", []string{"nudge", "ghost"}},
		{"", []string{"interrupt", "ghost"}},
		{"", []string{"send-keys", "ghost", "C-c", "Enter"}},
	} {
		r := h.run(c.stdin, c.args...)
		if r.code != exitInputOutcome {
			t.Errorf("%v: exit %d, want %d (stderr %q)", c.args, r.code, exitInputOutcome, r.stderr)
			continue
		}
		var line struct {
			Outcome string `json:"outcome"`
		}
		if err := json.Unmarshal([]byte(r.stdout), &line); err != nil {
			t.Errorf("%v: stdout = %q, want one JSON line: %v", c.args, r.stdout, err)
			continue
		}
		if line.Outcome != "gone" {
			t.Errorf("%v: outcome = %q, want gone", c.args, line.Outcome)
		}
	}
	// Zero keys is no input, so it never looks up the pane.
	if r := h.run("", "send-keys", "ghost"); r.code != exitOK || r.stdout != "" {
		t.Errorf("zero keys: exit %d stdout %q, want 0 and empty", r.code, r.stdout)
	}
	if len(fake.SentKeys) != 0 {
		t.Errorf("keys sent to a missing session: %+v", fake.SentKeys)
	}
}

// TestSendKeysGoneExits80 pins the declared input-outcome contract: send-keys
// answers a missing pane with exit 80 and one {"outcome":"gone",...} stdout
// line instead of the baseline best-effort exit 0.
func TestSendKeysGoneExits80(t *testing.T) {
	h, _ := newHarness(t)
	r := h.run("", "send-keys", "ghost", "C-c", "Enter")
	if r.code != exitInputOutcome {
		t.Fatalf("exit = %d, want %d (stderr %q)", r.code, exitInputOutcome, r.stderr)
	}
	var line struct {
		Outcome string `json:"outcome"`
		Detail  string `json:"detail"`
	}
	if err := json.Unmarshal([]byte(r.stdout), &line); err != nil {
		t.Fatalf("stdout = %q, want one JSON line: %v", r.stdout, err)
	}
	if line.Outcome != "gone" {
		t.Errorf("outcome = %q, want gone", line.Outcome)
	}
	if !strings.Contains(r.stderr, "gone") {
		t.Errorf("stderr = %q, want the gone word", r.stderr)
	}
}

// TestClearScrollbackReturnsGoneForMissingOrDeadSession pins the RPP failure
// case for a vanished pane: exit 80 with one JSON outcome line naming it gone,
// and no clear call (GCREX-57).
func TestClearScrollbackReturnsGoneForMissingOrDeadSession(t *testing.T) {
	for _, c := range []struct {
		name  string
		start bool
		dead  bool
	}{
		{"missing pane", false, false},
		{"dead pane", true, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			h, fake := newHarness(t)
			if c.start {
				h.start(dog)
			}
			if c.dead {
				fake.Processes = map[rex.BlockID]rex.ProcessInfo{
					"block:dog-1": {LastExit: &rex.ExitStatus{ExitCode: 0}},
				}
			}
			r := h.run("", "clear-scrollback", dog)
			if r.code != exitInputOutcome {
				t.Fatalf("exit = %d, want %d (stderr %q)", r.code, exitInputOutcome, r.stderr)
			}
			var line struct {
				Outcome string `json:"outcome"`
				Detail  string `json:"detail"`
			}
			if err := json.Unmarshal([]byte(r.stdout), &line); err != nil {
				t.Fatalf("stdout = %q, want one JSON line: %v", r.stdout, err)
			}
			if line.Outcome != "gone" {
				t.Errorf("outcome = %q, want gone", line.Outcome)
			}
			if !strings.Contains(r.stderr, "gone") {
				t.Errorf("stderr = %q, want the gone word", r.stderr)
			}
			for _, call := range fake.BlockCalls {
				if call.Op == "clear" {
					t.Errorf("cleared %s, want no mutation for a gone pane", call.BlockID)
				}
			}
		})
	}
}

// TestClearScrollbackDeliveredExitsZero keeps the live-pane path at exit 0 with
// empty stdout and stderr, matching the RPP success envelope.
func TestClearScrollbackDeliveredExitsZero(t *testing.T) {
	h, fake := newHarness(t)
	h.start(dog)
	r := h.run("", "clear-scrollback", dog)
	if r.code != exitOK || r.stdout != "" || r.stderr != "" {
		t.Errorf("clear-scrollback: exit %d stdout %q stderr %q, want 0 and empty", r.code, r.stdout, r.stderr)
	}
	cleared := false
	for _, call := range fake.BlockCalls {
		if call.Op == "clear" && call.BlockID == "block:dog-1" {
			cleared = true
		}
	}
	if !cleared {
		t.Error("BlockClear not called for a live pane")
	}
}

// TestInterruptReturnsInputOutcomes pins the typed exit-80 words interrupt
// answers: `gone` for a missing or dead pane, `unavailable` for a down server
// and `not_sent` for a pre-send failure. The handshake declares input-outcome
// and carrier.ops, so gc reads the word rather than treating 80 as a plain
// error (gc internal/runtime/exec/outcome.go:193).
func TestInterruptReturnsInputOutcomes(t *testing.T) {
	for _, c := range []struct {
		name    string
		start   bool
		dead    bool
		listErr error
		code    int
		outcome string
	}{
		{"missing pane", false, false, nil, exitInputOutcome, "gone"},
		{"dead pane", true, true, nil, exitInputOutcome, "gone"},
		{"server unreachable", false, false, rex.ErrUnavailable, exitInputOutcome, "unavailable"},
		{"pre-send failure", false, false, errors.New("rex: protocol error"), exitInputOutcome, "not_sent"},
	} {
		t.Run(c.name, func(t *testing.T) {
			h, fake := newHarness(t)
			if c.start {
				h.start(dog)
			}
			if c.dead {
				fake.Processes = map[rex.BlockID]rex.ProcessInfo{
					"block:dog-1": {LastExit: &rex.ExitStatus{ExitCode: 0}},
				}
			}
			if c.listErr != nil {
				fake.ListSessionsErr = c.listErr
			}
			r := h.run("", "interrupt", dog)
			if r.code != c.code {
				t.Fatalf("exit = %d, want %d (stderr %q)", r.code, c.code, r.stderr)
			}
			if c.code == exitOK {
				if r.stdout != "" || r.stderr != "" {
					t.Errorf("stdout %q stderr %q, want empty for a best-effort exit 0", r.stdout, r.stderr)
				}
			} else {
				var line struct {
					Outcome string `json:"outcome"`
					Detail  string `json:"detail"`
				}
				if err := json.Unmarshal([]byte(r.stdout), &line); err != nil {
					t.Fatalf("stdout = %q, want one JSON line: %v", r.stdout, err)
				}
				if line.Outcome != c.outcome {
					t.Errorf("outcome = %q, want %q", line.Outcome, c.outcome)
				}
			}
			if len(fake.SentKeys) != 0 {
				t.Errorf("keys sent = %+v, want none", fake.SentKeys)
			}
		})
	}
}

// TestInterruptDeliveredExitsZero keeps the delivered path at exit 0 with no
// stdout, matching the RPP success envelope.
func TestInterruptDeliveredExitsZero(t *testing.T) {
	h, fake := newHarness(t)
	h.start(dog)
	r := h.run("", "interrupt", dog)
	if r.code != exitOK || r.stdout != "" || r.stderr != "" {
		t.Errorf("interrupt: exit %d stdout %q stderr %q, want 0 and empty", r.code, r.stdout, r.stderr)
	}
	if len(fake.SentKeys) != 1 || fake.SentKeys[0].Keys[0] != "ctrl+c" {
		t.Errorf("sent keys = %+v, want one ctrl+c", fake.SentKeys)
	}
}

func TestSendKeysRejectsBadKeyWithOutcome(t *testing.T) {
	h, fake := newHarness(t)
	h.start(dog)
	before := len(fake.BlockCalls)

	r := h.run("", "send-keys", dog, "!")
	if r.code != exitInputOutcome {
		t.Fatalf("exit = %d, want %d (stderr %q)", r.code, exitInputOutcome, r.stderr)
	}
	if !strings.Contains(r.stdout, `"outcome":"rejected"`) {
		t.Errorf("stdout = %q, want a rejected outcome line", r.stdout)
	}
	if !strings.Contains(r.stderr, "rejected") {
		t.Errorf("stderr = %q, want the rejected word", r.stderr)
	}
	// A rejected key is caught before the lookup, so no Rex call follows.
	if len(fake.BlockCalls) != before {
		t.Errorf("Rex calls after rejection = %d, want %d (no lookup)", len(fake.BlockCalls), before)
	}
}

func TestInputOpsReachPane(t *testing.T) {
	h, fake := newHarness(t)
	h.start(dog)
	h.must("", "send-keys", dog, "C-c")
	h.must("", "interrupt", dog)
	h.must("say hi", "nudge", dog)
	if len(fake.SentKeys) != 3 {
		t.Fatalf("sent keys = %+v", fake.SentKeys)
	}
	fake.SendKeysErr = errors.New("transport down")
	assertFailure(t, h.run("", "interrupt", dog), exitFail)
}

// failingWindow is a Rex client whose pane launch fails.
type failingWindow struct{ *faketest.Fake }

func (failingWindow) NewWindow(context.Context, rex.SessionID, string, rex.LaunchSpec) (rex.WindowID, rex.BlockID, error) {
	return "", "", errors.New("rex: launch refused")
}

func TestStartClientErrorFails(t *testing.T) {
	h, _ := newHarness(t)
	h.client = failingWindow{faketest.New()}
	r := h.run(`{"command":"sleep 300"}`, "start", dog)
	assertFailure(t, r, exitFail)
	if !strings.Contains(r.stderr, "launch refused") {
		t.Errorf("stderr = %q", r.stderr)
	}
}

func TestStartBadConfigFails(t *testing.T) {
	h, _ := newHarness(t)
	assertFailure(t, h.run(`{not json`, "start", dog), exitFail)
}

// TestStartEmptyStdinFails covers the document's start failure case: an empty
// or whitespace-only body is an invalid start config, not a request to launch
// a default shell.
func TestStartEmptyStdinFails(t *testing.T) {
	for _, body := range []string{"", "  \n\t "} {
		t.Run(fmt.Sprintf("%q", body), func(t *testing.T) {
			h, fake := newHarness(t)
			r := h.run(body, "start", dog)
			assertFailure(t, r, exitFail)
			if !strings.Contains(r.stderr, "start config") {
				t.Errorf("stderr = %q, want a start config message", r.stderr)
			}
			if len(fake.CreatedWindows)+len(fake.CreatedSplits) != 0 {
				t.Errorf("launched a pane for an empty start config: windows=%d splits=%d",
					len(fake.CreatedWindows), len(fake.CreatedSplits))
			}
		})
	}
}

// TestStartInstantExitFails covers the document's post-launch liveness failure:
// a pane whose program has already exited is not a successful start, and the
// dead pane is left in place for gc's stop.
func TestStartInstantExitFails(t *testing.T) {
	h, fake := newHarness(t)
	fake.Processes = map[rex.BlockID]rex.ProcessInfo{
		"block:dog-1": {LastExit: &rex.ExitStatus{ExitCode: 1}},
	}
	r := h.run(`{"command":"false"}`, "start", dog)
	assertFailure(t, r, exitFail)
	if !strings.Contains(r.stderr, "exited during startup") {
		t.Errorf("stderr = %q, want an exited-during-startup message", r.stderr)
	}
	if n := len(fake.CreatedWindows) + len(fake.CreatedSplits); n != 1 {
		t.Errorf("panes launched = %d, want 1 (the dead pane is left for gc's stop)", n)
	}
}

// TestStartOneShotInstantExitSucceeds covers lifecycle=one_shot: a batch job
// that exits at once is expected to end, so the start still succeeds.
func TestStartOneShotInstantExitSucceeds(t *testing.T) {
	h, fake := newHarness(t)
	fake.Processes = map[rex.BlockID]rex.ProcessInfo{
		"block:dog-1": {LastExit: &rex.ExitStatus{ExitCode: 0}},
	}
	r := h.run(`{"command":"true","lifecycle":"one_shot"}`, "start", dog)
	if r.code != exitOK {
		t.Fatalf("exit = %d, want 0 (stderr %q)", r.code, r.stderr)
	}
}

// livenessAfter returns a fake ProcessFunc that reports a live pane for the
// first aliveCalls calls and a dead one afterwards, modelling an agent that
// exits after the post-launch probe but before its first turn.
func livenessAfter(aliveCalls int) func(rex.BlockID) rex.ProcessInfo {
	var calls int
	return func(rex.BlockID) rex.ProcessInfo {
		calls++
		if calls > aliveCalls {
			return rex.ProcessInfo{LastExit: &rex.ExitStatus{ExitCode: 1}}
		}
		return rex.ProcessInfo{}
	}
}

// TestStartMissedFirstTurnFails covers the document's second liveness failure:
// an agent that ends while the first-turn readiness wait runs, before its first
// turn, fails the start.
func TestStartMissedFirstTurnFails(t *testing.T) {
	h, fake := newHarness(t)
	fake.ProcessFunc = livenessAfter(1) // alive at the post-launch probe, dead in the readiness wait
	r := h.run(`{"command":"sleep 300","nudge":"hi"}`, "start", dog)
	assertFailure(t, r, exitFail)
	if !strings.Contains(r.stderr, "before its first turn") {
		t.Errorf("stderr = %q, want a before-its-first-turn message", r.stderr)
	}
}

// TestStartOneShotMissedFirstTurnSucceeds covers lifecycle=one_shot for the
// same case: the agent ending before its first turn is expected, not a failure.
func TestStartOneShotMissedFirstTurnSucceeds(t *testing.T) {
	h, fake := newHarness(t)
	fake.ProcessFunc = func(rex.BlockID) rex.ProcessInfo {
		return rex.ProcessInfo{LastExit: &rex.ExitStatus{ExitCode: 0}}
	}
	r := h.run(`{"command":"sleep 300","nudge":"hi","lifecycle":"one_shot"}`, "start", dog)
	if r.code != exitOK {
		t.Fatalf("exit = %d, want 0 (stderr %q)", r.code, r.stderr)
	}
}

// failWriter fails every write.
type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errors.New("broken pipe") }

func TestProtocolWriteErrorFails(t *testing.T) {
	var stderr bytes.Buffer
	d := deps{newClient: func() rex.Client { return faketest.New() }}
	code := run([]string{"protocol"}, strings.NewReader(""), failWriter{}, &stderr, d)
	if code != exitFail {
		t.Errorf("exit = %d, want %d", code, exitFail)
	}
	if !strings.Contains(stderr.String(), "gc-runtime-rex: protocol: write output") {
		t.Errorf("stderr = %q, want a write error message", stderr.String())
	}
}

func TestConcurrentDuplicateStartOneWins(t *testing.T) {
	for range 20 {
		h, fake := newHarness(t)
		results := make(chan result, 2)
		release := make(chan struct{})
		for range 2 {
			go func() {
				<-release
				results <- h.run(`{"command":"sleep 300"}`, "start", dog)
			}()
		}
		close(release)
		ok, already := 0, 0
		for range 2 {
			r := <-results
			switch {
			case r.code == exitOK:
				ok++
			case r.code == exitFail && strings.Contains(r.stderr, "already running"):
				already++
			default:
				t.Fatalf("start: exit %d, stderr %q", r.code, r.stderr)
			}
		}
		if ok != 1 || already != 1 {
			t.Fatalf("succeeded=%d already-running=%d, want 1 and 1", ok, already)
		}
		if n := len(fake.CreatedWindows) + len(fake.CreatedSplits); n != 1 {
			t.Fatalf("panes launched = %d, want 1", n)
		}
	}
}

func TestGetMetaUnreadableStoreFails(t *testing.T) {
	h, _ := newHarness(t)
	file := filepath.Join(t.TempDir(), "plainfile")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GC_REX_STATE_DIR", file)
	r := h.run("", "get-meta", "s", "k")
	assertFailure(t, r, exitFail)
	if !strings.Contains(r.stderr, "get-meta") {
		t.Errorf("stderr = %q, want a get-meta diagnostic", r.stderr)
	}
}

func TestGetMetaUnsetKeyIsEmpty(t *testing.T) {
	h, _ := newHarness(t)
	if r := h.must("", "get-meta", "s", "k"); r.stdout != "" {
		t.Errorf("stdout = %q, want empty", r.stdout)
	}
}

func TestNudgeRejectsInvalidUTF8(t *testing.T) {
	h, fake := newHarness(t)
	h.start(dog)
	before := len(fake.BlockCalls)

	r := h.run("prompt \xff bytes", "nudge", dog)
	if r.code != exitInputOutcome {
		t.Fatalf("exit = %d, want %d (stderr %q)", r.code, exitInputOutcome, r.stderr)
	}
	if !strings.Contains(r.stdout, `"outcome":"rejected"`) {
		t.Errorf("stdout = %q, want a rejected outcome line", r.stdout)
	}
	if !strings.Contains(r.stderr, "UTF-8") {
		t.Errorf("stderr = %q, want it to name UTF-8", r.stderr)
	}
	if len(fake.BlockCalls) != before {
		t.Errorf("block calls = %v, want none after bad stdin", fake.BlockCalls[before:])
	}
	if len(fake.SentKeys) != 0 {
		t.Errorf("send-keys calls = %d, want 0 after bad stdin", len(fake.SentKeys))
	}
}

// GCREX-35: the gc CLI fallback provider is invoked with no city env at all,
// while the session was started under a GC_REX_CITY pin. Every lookup op must
// discover the pin's state dir from the sibling namespaces and answer for the
// live pane; a genuinely absent session still answers empty/false with exit 0.
func TestLookupResolvesPinnedBindingFromUnpinnedCaller(t *testing.T) {
	h, fake := newHarness(t) // the harness starts pinned to testcity
	h.must(`{"command":"sleep 300","env":{"GC_SESSION_ID":"sess-test"}}`, "start", dog)
	fake.Formats = map[rex.BlockID]string{"block:dog-1": "pinned output\n"}

	// Become the unpinned gc caller: clear every city env the provider reads.
	t.Setenv("GC_REX_CITY", "")
	t.Setenv("GC_CITY_PATH", "")
	t.Setenv("GC_CITY", "")

	if got := strings.TrimSpace(h.must("", "is-running", dog).stdout); got != "true" {
		t.Fatalf("is-running = %q, want true", got)
	}
	if got := h.must("", "peek", dog).stdout; got != "pinned output\n" {
		t.Errorf("peek = %q, want the pinned pane output", got)
	}
	if got := strings.TrimSpace(h.must("", "list-running", "Indigo--").stdout); got != dog {
		t.Errorf("list-running = %q, want %s", got, dog)
	}
	if got := h.must("", "get-meta", dog, "GC_SESSION_ID").stdout; got != "sess-test" {
		t.Errorf("get-meta GC_SESSION_ID = %q, want sess-test (pinned meta store not discovered)", got)
	}
	h.must("true", "set-meta", dog, "suspended")
	if got := h.must("", "get-meta", dog, "suspended").stdout; got != "true" {
		t.Errorf("set-meta/get-meta round trip = %q, want true", got)
	}

	// AC #5: a genuinely absent session has no screen (exit 66) and is not running.
	assertNoScreen(t, h.run("", "peek", "Indigo--absent"), "Indigo--absent")
	if got := strings.TrimSpace(h.must("", "is-running", "Indigo--absent").stdout); got != "false" {
		t.Errorf("is-running(absent) = %q, want false", got)
	}
}

// TestNudgeReturnsInputOutcomes pins the typed exit-80 words nudge answers for
// blocked, busy, an unreachable server, and an unwritten send. Every case
// types nothing.
func TestNudgeReturnsInputOutcomes(t *testing.T) {
	for _, c := range []struct {
		name     string
		start    bool
		status   *rex.ProgramStatus
		listErr  error
		writeErr error
		outcome  string
	}{
		{"busy", true, &rex.ProgramStatus{Records: []rex.ProgramStatusRecord{{ID: "a", State: "working"}}}, nil, nil, "busy"},
		{"blocked", true, &rex.ProgramStatus{Records: []rex.ProgramStatusRecord{{ID: "a", State: "blocked", Kind: "permission"}}}, nil, nil, "pending_interaction"},
		// No pane is started: a down server cannot list, so the lookup itself is
		// what is unreachable.
		{"server unreachable", false, nil, rex.ErrUnavailable, nil, "unavailable"},
		{"unwritten send", true, nil, nil, errors.New("rex: protocol error"), "not_sent"},
	} {
		t.Run(c.name, func(t *testing.T) {
			h, fake := newHarness(t)
			if c.start {
				h.start(dog)
			}
			if c.status != nil {
				fake.ProgramStatuses = map[rex.BlockID]rex.ProgramStatus{"block:dog-1": *c.status}
			}
			if c.listErr != nil {
				fake.ListSessionsErr = c.listErr
			}
			if c.writeErr != nil {
				fake.WriteErr = c.writeErr
			}
			r := h.run("hello", "nudge", dog)
			if r.code != exitInputOutcome {
				t.Fatalf("exit = %d, want %d (stderr %q)", r.code, exitInputOutcome, r.stderr)
			}
			var line struct {
				Outcome string `json:"outcome"`
				Detail  string `json:"detail"`
			}
			if err := json.Unmarshal([]byte(r.stdout), &line); err != nil {
				t.Fatalf("stdout = %q, want one JSON line: %v", r.stdout, err)
			}
			if line.Outcome != c.outcome {
				t.Errorf("outcome = %q, want %q", line.Outcome, c.outcome)
			}
			if !strings.Contains(r.stderr, c.outcome) {
				t.Errorf("stderr = %q, want the %q word", r.stderr, c.outcome)
			}
			if len(fake.SentKeys) != 0 {
				t.Errorf("keys sent = %+v, want none", fake.SentKeys)
			}
		})
	}
}

// TestNudgeUnconfirmedExitsZeroWithOutcome pins the delivered_unobserved
// success form: exit 0, one JSON line, and the diagnostic file kept.
func TestNudgeUnconfirmedExitsZeroWithOutcome(t *testing.T) {
	h, fake := newHarness(t)
	h.start(dog)
	fake.ProgramStatuses = map[rex.BlockID]rex.ProgramStatus{
		"block:dog-1": {Records: []rex.ProgramStatusRecord{{ID: "a", State: "idle", UpdatedAt: time.Unix(1, 0).UTC()}}},
	}
	r := h.run("hello", "nudge", dog)
	if r.code != exitOK {
		t.Fatalf("exit = %d, want 0 (stderr %q)", r.code, r.stderr)
	}
	var line struct {
		Outcome string `json:"outcome"`
		Detail  string `json:"detail"`
	}
	if err := json.Unmarshal([]byte(r.stdout), &line); err != nil {
		t.Fatalf("stdout = %q, want one JSON line: %v", r.stdout, err)
	}
	if line.Outcome != "delivered_unobserved" {
		t.Errorf("outcome = %q, want delivered_unobserved", line.Outcome)
	}
	base := os.Getenv("GC_REX_STATE_DIR")
	found := false
	if err := filepath.WalkDir(base, func(_ string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(d.Name(), "-nudge-unconfirmed.json") {
			found = true
		}
		return nil
	}); err != nil {
		t.Fatalf("walk state dir: %v", err)
	}
	if !found {
		t.Errorf("no *-nudge-unconfirmed.json under %s, want the diagnostic kept", base)
	}
}
