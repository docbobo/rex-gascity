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
	want := `{"version":0,"capabilities":["report-attachment","report-activity"]}` + "\n"
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
	if !slices.Equal(caps, []string{"report-attachment", "report-activity"}) {
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

func TestMetaRoundTripAcrossCalls(t *testing.T) {
	h, _ := newHarness(t)
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
	assertFailure(t, h.run("", "is-attached", dog), exitFail)
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

func TestInputOpsOnMissingSessionSucceed(t *testing.T) {
	h, fake := newHarness(t)
	for _, c := range []struct {
		stdin string
		args  []string
	}{
		{"", []string{"interrupt", "ghost"}},
		{"hello\n", []string{"nudge", "ghost"}},
		{"", []string{"send-keys", "ghost", "C-c", "Enter"}},
		{"", []string{"send-keys", "ghost"}},
		{"", []string{"clear-scrollback", "ghost"}},
	} {
		r := h.run(c.stdin, c.args...)
		if r.code != exitOK || r.stdout != "" {
			t.Errorf("%v: exit %d stdout %q stderr %q", c.args, r.code, r.stdout, r.stderr)
		}
	}
	if len(fake.SentKeys) != 0 {
		t.Errorf("keys sent to a missing session: %+v", fake.SentKeys)
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

func TestCommandArgv(t *testing.T) {
	sh := func(c string) []string { return []string{"/bin/sh", "-lc", c} }
	for _, c := range []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"   ", nil},
		{"sleep 300", []string{"sleep", "300"}},
		{`claude  --flag='a b' "c \"d\"" e\ f`, []string{"claude", "--flag=a b", `c "d"`, "e f"}},
		{`echo ''`, []string{"echo", ""}},
		{`echo "unterminated`, sh(`echo "unterminated`)},
		{`echo 'unterminated`, sh(`echo 'unterminated`)},
		{`echo trailing\`, sh(`echo trailing\`)},
		{"a && b", sh("a && b")},
		{"echo $HOME", sh("echo $HOME")},
		{`echo "$HOME"`, sh(`echo "$HOME"`)},
		{`echo '$HOME'`, []string{"echo", "$HOME"}},
		{"FOO=bar claude", sh("FOO=bar claude")},
		{"ls *.go", sh("ls *.go")},
	} {
		if got := commandArgv(c.in); !slices.Equal(got, c.want) {
			t.Errorf("commandArgv(%q) = %q, want %q", c.in, got, c.want)
		}
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
	assertFailure(t, r, exitFail)
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
