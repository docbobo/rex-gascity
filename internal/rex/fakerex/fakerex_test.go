package fakerex_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/docbobo/rex-gascity/internal/rex"
	"github.com/docbobo/rex-gascity/internal/rex/cli"
	"github.com/docbobo/rex-gascity/internal/rex/fakerex"
	"github.com/docbobo/rex-gascity/internal/rex/fakerex/fakestate"
)

// spec is a single-pane launch carrying argv, the shape internal/launch sends.
func spec(label string, argv ...string) rex.LaunchSpec {
	return rex.LaunchSpec{Layout: rex.Layout{Block: &rex.LayoutBlock{
		Flavor:  "com.superlogical.terminal.shell",
		Options: map[string]any{"command": argv},
		Label:   label,
	}}}
}

func ctx(t *testing.T) context.Context {
	t.Helper()
	c, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	return c
}

// start creates a session with one window running argv.
func start(t *testing.T, f *fakerex.Fake, label string, argv ...string) (*cli.Client, rex.SessionID, rex.BlockID) {
	t.Helper()
	c := &cli.Client{Bin: f.Bin}
	sid, err := c.CreateSession(ctx(t), "gc:acme")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	_, bid, err := c.NewWindow(ctx(t), sid, "Rig: Indigo", spec(label, argv...))
	if err != nil {
		t.Fatalf("NewWindow: %v", err)
	}
	return c, sid, bid
}

func blocksByLabel(t *testing.T, v rex.SessionView) map[string]rex.Block {
	t.Helper()
	if len(v.Windows) != 1 {
		t.Fatalf("windows = %d, want 1", len(v.Windows))
	}
	out := map[string]rex.Block{}
	for _, b := range v.Windows[0].Blocks {
		out[b.Label] = b
	}
	return out
}

// TestSurface covers version, api describe and the server methods.
func TestSurface(t *testing.T) {
	f := fakerex.New(t)

	out, _, code := f.Exec(t, "version")
	if code != 0 || !strings.Contains(out, "api: revision 14") {
		t.Fatalf("version = (%q, %d), want the api revision line", out, code)
	}
	out, _, code = f.Exec(t, "version", "--json")
	var ver struct {
		Version     string `json:"version"`
		APIRevision int    `json:"api_revision"`
	}
	if code != 0 || json.Unmarshal([]byte(out), &ver) != nil || ver.APIRevision != 14 {
		t.Fatalf("version --json = (%q, %d)", out, code)
	}

	out, _, code = f.Exec(t, "api", "describe", "session.view")
	var desc struct {
		Method string `json:"method"`
	}
	if code != 0 || json.Unmarshal([]byte(out), &desc) != nil || desc.Method != "session.view" {
		t.Fatalf("api describe = (%q, %d)", out, code)
	}
	if _, stderr, code := f.Exec(t, "api", "describe", "no.such.method"); code != 3 || !strings.Contains(stderr, "no such method") {
		t.Errorf("api describe unknown = exit %d, stderr %q, want 3", code, stderr)
	}

	// server.status stayed out of the fake: it is not in the contract doc, so
	// the drift guard would flag it. server.version covers the same reply shape.
	out, _, code = f.Exec(t, "api", "call", "server.version")
	if code != 0 || !strings.Contains(out, `"api_revision":14`) {
		t.Errorf("server.version = (%q, %d)", out, code)
	}
}

// TestAPIVersionOverride proves the server's api_revision is configurable, so
// the version-skew case can be driven both matched and mismatched.
func TestAPIVersionOverride(t *testing.T) {
	f := fakerex.New(t)
	f.Update(t, func(st *fakestate.State) { st.APIVersion = 99 })

	out, _, code := f.Exec(t, "version")
	if code != 0 || !strings.Contains(out, "api: revision 99 (supports 0 to 14)") {
		t.Fatalf("version = (%q, %d), want the configured revision", out, code)
	}
	out, _, code = f.Exec(t, "version", "--json")
	var ver struct {
		APIRevision int `json:"api_revision"`
	}
	if code != 0 || json.Unmarshal([]byte(out), &ver) != nil || ver.APIRevision != 99 {
		t.Fatalf("version --json = (%q, %d)", out, code)
	}
	out, _, code = f.Exec(t, "api", "call", "server.version")
	if code != 0 || !strings.Contains(out, `"api_revision":99`) {
		t.Errorf("server.version = (%q, %d)", out, code)
	}
}

// TestLifecycle drives create/window/split/labels/focus/close/destroy through
// the real cli.Client, one short-lived process per call.
func TestLifecycle(t *testing.T) {
	f := fakerex.New(t)
	c, sid, bid := start(t, f, "dog-1", "/bin/sh", "-c", "sleep 60")

	view, err := c.SessionView(ctx(t), sid)
	if err != nil {
		t.Fatalf("SessionView: %v", err)
	}
	if view.Label != "gc:acme" || len(view.Windows) != 1 || view.Windows[0].Label != "Rig: Indigo" {
		t.Fatalf("view = %+v", view)
	}
	if got := blocksByLabel(t, view)["dog-1"]; got.Rect != [4]float64{0, 0, 1, 1} {
		t.Errorf("dog-1 rect = %v, want the whole window", got.Rect)
	}

	f.Update(t, func(st *fakestate.State) {
		s, _ := st.Session(string(sid))
		s.Clients = []fakestate.Client{{ID: "client:a", Kind: "app", Name: "Rex"}}
	})
	clients, err := c.ListClients(ctx(t), sid)
	if err != nil || len(clients) != 1 || clients[0].Kind != "app" {
		t.Fatalf("ListClients = (%+v, %v)", clients, err)
	}

	// Rex's "horizontal" places panes side by side; "vertical" stacks them.
	dog2, err := c.NewSplit(ctx(t), sid, bid, "horizontal", "after", spec("dog-2", "/bin/sh", "-c", "sleep 60"))
	if err != nil {
		t.Fatalf("NewSplit horizontal: %v", err)
	}
	dog3, err := c.NewSplit(ctx(t), sid, bid, "vertical", "after", spec("dog-3", "/bin/sh", "-c", "sleep 60"))
	if err != nil {
		t.Fatalf("NewSplit horizontal: %v", err)
	}
	view, err = c.SessionView(ctx(t), sid)
	if err != nil {
		t.Fatalf("SessionView: %v", err)
	}
	got := blocksByLabel(t, view)
	want := map[string][4]float64{
		"dog-1": {0, 0, 0.5, 0.5},
		"dog-3": {0, 0.5, 0.5, 0.5},
		"dog-2": {0.5, 0, 0.5, 1},
	}
	for label, rect := range want {
		if got[label].Rect != rect {
			t.Errorf("%s rect = %v, want %v", label, got[label].Rect, rect)
		}
	}

	// Label and focus methods: the adapter has no typed call, so use the raw
	// surface.
	call := func(method string, body string) {
		t.Helper()
		if _, stderr, code := f.Exec(t, "api", "call", method, body); code != 0 {
			t.Fatalf("%s: exit %d: %s", method, code, stderr)
		}
	}
	call("session.set_window_label", fmt.Sprintf(`{"session_id":%q,"window_id":%q,"label":"Rig: Renamed"}`, sid, view.Windows[0].ID))
	call("session.set_block_label", fmt.Sprintf(`{"session_id":%q,"block_id":%q,"label":"dog-1b"}`, sid, bid))
	call("session.focus_block", fmt.Sprintf(`{"session_id":%q,"block_id":%q}`, sid, dog2))
	view, err = c.SessionView(ctx(t), sid)
	if err != nil {
		t.Fatalf("SessionView: %v", err)
	}
	if view.Windows[0].Label != "Rig: Renamed" || view.Windows[0].FocusedBlockID != dog2 {
		t.Errorf("window = %+v, want the new label and focus", view.Windows[0])
	}
	if _, ok := blocksByLabel(t, view)["dog-1b"]; !ok {
		t.Errorf("blocks = %+v, want the renamed dog-1b", view.Windows[0].Blocks)
	}

	if err := c.CloseBlock(ctx(t), sid, dog3); err != nil {
		t.Fatalf("CloseBlock: %v", err)
	}
	view, err = c.SessionView(ctx(t), sid)
	if err != nil || len(view.Windows[0].Blocks) != 2 {
		t.Fatalf("blocks after close = %d, err %v", len(view.Windows[0].Blocks), err)
	}

	if err := c.DestroySession(ctx(t), sid); err != nil {
		t.Fatalf("DestroySession: %v", err)
	}
	sessions, err := c.ListSessions(ctx(t))
	if err != nil || len(sessions) != 0 {
		t.Fatalf("ListSessions after destroy = (%+v, %v)", sessions, err)
	}

	// `session kill` removes a session by id.
	sid2, err := c.CreateSession(ctx(t), "gc:other")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if _, _, code := f.Exec(t, "session", "kill", string(sid2)); code != 0 {
		t.Fatalf("session kill: exit %d", code)
	}
	if sessions, _ := c.ListSessions(ctx(t)); len(sessions) != 0 {
		t.Errorf("sessions after kill = %+v, want none", sessions)
	}
}

// TestFormatPagesLargeOutput proves format carries an id while output remains
// and blob_read covers the rest, past the 960 KiB page cap.
func TestFormatPagesLargeOutput(t *testing.T) {
	f := fakerex.New(t)
	c, sid, bid := start(t, f, "big", "/bin/sh", "-c", "yes x | head -c 1500000")
	f.WaitExit(t, string(bid))

	type page struct {
		Content string `json:"content"`
		Data    string `json:"data"`
		ID      int64  `json:"id"`
		Total   int64  `json:"total"`
		EOF     bool   `json:"eof"`
	}
	raw, _, code := f.Exec(t, "block", "call", "-s", string(sid), "-b", string(bid),
		"format", `{"format":"text","trim":true,"max_bytes":983040}`)
	var first page
	if code != 0 || json.Unmarshal([]byte(raw), &first) != nil {
		t.Fatalf("format = (%q, %d)", raw, code)
	}
	if first.Total != 1500000 || first.ID == 0 || first.EOF || len(first.Content) != 983040 {
		t.Fatalf("format page = total %d id %d eof %v len %d", first.Total, first.ID, first.EOF, len(first.Content))
	}

	raw, _, code = f.Exec(t, "block", "call", "-s", string(sid), "-b", string(bid),
		"blob_read", fmt.Sprintf(`{"id":%d,"offset":%d}`, first.ID, len(first.Content)))
	var next page
	if code != 0 || json.Unmarshal([]byte(raw), &next) != nil || next.Total != first.Total {
		t.Fatalf("blob_read = (%q, %d)", raw, code)
	}
	if len(next.Data) == 0 || next.EOF {
		t.Errorf("first blob page = %d bytes, eof %v, want a non-final page", len(next.Data), next.EOF)
	}

	out, err := c.BlockFormat(ctx(t), sid, bid)
	if err != nil {
		t.Fatalf("BlockFormat: %v", err)
	}
	if len(out) != 1500000 || !strings.HasPrefix(out, "x\n") {
		t.Fatalf("BlockFormat returned %d bytes, want all 1500000", len(out))
	}

	// Without max_bytes a capture over the cap fails as too_large (exit 1).
	if _, stderr, code := f.Exec(t, "block", "call", "-s", string(sid), "-b", string(bid),
		"format", `{"format":"text"}`); code != 1 || !strings.Contains(stderr, "too_large") {
		t.Errorf("unpaged format = exit %d, stderr %q, want 1 too_large", code, stderr)
	}
	// An expired result is not found (exit 3).
	if _, _, code := f.Exec(t, "block", "call", "-s", string(sid), "-b", string(bid),
		"blob_read", `{"id":99,"offset":0}`); code != 3 {
		t.Errorf("blob_read unknown id = exit %d, want 3", code)
	}
}

// TestProcessAndProgramStatus covers last_exit, foreground, clear and the
// program-status records.
func TestProcessAndProgramStatus(t *testing.T) {
	f := fakerex.New(t)
	c, sid, bid := start(t, f, "exit5", "/bin/sh", "-c", "echo bye; exit 5")
	f.WaitExit(t, string(bid))

	info, err := c.BlockProcess(ctx(t), sid, bid)
	if err != nil {
		t.Fatalf("BlockProcess: %v", err)
	}
	if info.LastExit == nil || info.LastExit.ExitCode != 5 {
		t.Errorf("LastExit = %+v, want exit 5", info.LastExit)
	}
	if info.Foreground == nil || info.Foreground.PID == 0 {
		t.Errorf("Foreground = %+v, want the launched process", info.Foreground)
	}

	f.Update(t, func(st *fakestate.State) {
		_, _, b, err := st.Block(string(bid))
		if err != nil {
			t.Fatalf("seed block: %v", err)
		}
		b.ProgramStatus = []fakestate.Record{{
			ID: "ps1", State: "idle", App: "pi",
			UpdatedAt: time.Now().UTC().Format(time.RFC3339),
		}}
	})
	status, err := c.BlockProgramStatus(ctx(t), sid, bid)
	if err != nil {
		t.Fatalf("BlockProgramStatus: %v", err)
	}
	if len(status.Records) != 1 || status.Records[0].State != "idle" ||
		status.Records[0].App != "pi" || status.Records[0].UpdatedAt.IsZero() {
		t.Errorf("program status = %+v", status.Records)
	}

	if err := c.BlockClear(ctx(t), sid, bid); err != nil {
		t.Fatalf("BlockClear: %v", err)
	}
	if out, err := c.BlockFormat(ctx(t), sid, bid); err != nil || out != "" {
		t.Errorf("format after clear = (%q, %v), want empty", out, err)
	}
}

// TestSendKeys covers the documented key mapping: literal text plus enter
// reaches stdin, and ctrl+c sends SIGINT.
func TestSendKeys(t *testing.T) {
	f := fakerex.New(t)
	c, sid, bid := start(t, f, "reader", "/bin/sh", "-c", `read line; printf 'GOT:%s\n' "$line"`)
	if err := c.SendKeys(ctx(t), sid, bid, "hello", "enter"); err != nil {
		t.Fatalf("SendKeys: %v", err)
	}
	waitOutput(t, c, sid, bid, "GOT:hello")

	_, sid2, bid2 := start(t, f, "sleeper", "/bin/sh", "-c", "sleep 60")
	if err := c.SendKeys(ctx(t), sid2, bid2, "ctrl+c"); err != nil {
		t.Fatalf("SendKeys ctrl+c: %v", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if info, err := c.BlockProcess(ctx(t), sid2, bid2); err == nil && info.LastExit != nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("ctrl+c did not terminate the pane")
}

func waitOutput(t *testing.T, c *cli.Client, sid rex.SessionID, bid rex.BlockID, want string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if out, err := c.BlockFormat(ctx(t), sid, bid); err == nil && strings.Contains(out, want) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("output never contained %q", want)
}

// TestExitCodes pins the contract's code space: 2 usage, 3 missing target,
// 4 no server, 1 server failure.
func TestExitCodes(t *testing.T) {
	f := fakerex.New(t)
	c, sid, bid := start(t, f, "dog-1", "/bin/sh", "-c", "sleep 60")
	_ = c

	if _, _, code := f.Exec(t, "bogus"); code != 2 {
		t.Errorf("unknown command = %d, want 2", code)
	}
	if _, _, code := f.Exec(t, "api", "call", "session.list", `{not json`); code != 2 {
		t.Errorf("invalid JSON = %d, want 2", code)
	}
	if _, stderr, code := f.Exec(t, "send-key", "-s", string(sid), "-b", string(bid), "--", "ctrl+bogus"); code != 2 || !strings.Contains(stderr, "unknown key") {
		t.Errorf("bad key = %d %q, want 2", code, stderr)
	}
	if _, _, code := f.Exec(t, "api", "call", "session.view", `{"session_id":"session:99"}`); code != 3 {
		t.Errorf("missing session = %d, want 3", code)
	}
	if _, _, code := f.Exec(t, "api", "call", "block.method", fmt.Sprintf(`{"session_id":%q,"block_id":%q,"method":"nope"}`, sid, bid)); code != 3 {
		t.Errorf("missing block method = %d, want 3", code)
	}
	if _, stderr, code := f.Exec(t, "api", "call", "block.close", fmt.Sprintf(`{"session_id":%q,"block_id":%q,"keep_last":true}`, sid, bid)); code != 1 || !strings.Contains(stderr, "last block") {
		t.Errorf("close last block = %d %q, want 1", code, stderr)
	}

	t.Setenv("FAKEREX_STATE", t.TempDir()+"/gone")
	if _, stderr, code := f.Exec(t, "api", "call", "session.list"); code != 4 || !strings.Contains(stderr, "no Rex server") {
		t.Errorf("unreachable server = %d %q, want 4", code, stderr)
	}
}

// TestConcurrentInvocations proves the state file lock: many short-lived CLI
// processes create sessions at once without losing one another's writes.
func TestConcurrentInvocations(t *testing.T) {
	f := fakerex.New(t)
	const n = 8
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			c := &cli.Client{Bin: f.Bin}
			sid, err := c.CreateSession(context.Background(), fmt.Sprintf("gc:city-%d", i))
			if err != nil {
				errs <- err
				return
			}
			_, _, err = c.NewWindow(context.Background(), sid, "Rig", spec(fmt.Sprintf("dog-%d", i), "/bin/sh", "-c", "sleep 60"))
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent call: %v", err)
		}
	}

	c := &cli.Client{Bin: f.Bin}
	sessions, err := c.ListSessions(context.Background())
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(sessions) != n {
		t.Fatalf("sessions = %d, want %d", len(sessions), n)
	}
	st := f.State(t)
	if len(st.Sessions) != n {
		t.Errorf("state sessions = %d, want %d", len(st.Sessions), n)
	}
	for _, s := range st.Sessions {
		if len(s.Windows) != 1 || len(s.Windows[0].Blocks) != 1 {
			t.Errorf("session %s = %+v, want one window and block", s.ID, s.Windows)
		}
	}
}

// TestCloseBlockStopsTermIgnoringPane proves block.close does not orphan a pane
// that ignores SIGTERM: the fake escalates after a bounded grace period.
func TestCloseBlockStopsTermIgnoringPane(t *testing.T) {
	f := fakerex.New(t)
	c, sid, bid := start(t, f, "stubborn", "/bin/sh", "-c", `trap "" TERM; while :; do sleep 0.1; done`)
	info, err := c.BlockProcess(ctx(t), sid, bid)
	if err != nil || info.Foreground == nil || info.Foreground.PID <= 0 {
		t.Fatalf("BlockProcess = (%+v, %v), want the launched pid", info, err)
	}
	pid := info.Foreground.PID
	// Let the shell install its trap before the close signal arrives.
	time.Sleep(200 * time.Millisecond)
	if err := c.CloseBlock(ctx(t), sid, bid); err != nil {
		t.Fatalf("CloseBlock: %v", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if err := syscall.Kill(pid, 0); err != nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("pane %d still runs after close", pid)
}
// TestCloseExpandsSurvivingPane proves closing a pane hands the split's rect
// to the surviving subtree and scales its descendants, so later splits divide
// the whole window.
func TestCloseExpandsSurvivingPane(t *testing.T) {
	f := fakerex.New(t)
	c, sid, a := start(t, f, "a", "/bin/sh", "-c", "sleep 10")
	b, err := c.NewSplit(ctx(t), sid, a, "horizontal", "after", spec("b", "/bin/sh", "-c", "sleep 10"))
	if err != nil {
		t.Fatalf("NewSplit b: %v", err)
	}
	cc, err := c.NewSplit(ctx(t), sid, b, "vertical", "after", spec("c", "/bin/sh", "-c", "sleep 10"))
	if err != nil {
		t.Fatalf("NewSplit c: %v", err)
	}
	if err := c.CloseBlock(ctx(t), sid, a); err != nil {
		t.Fatalf("CloseBlock a: %v", err)
	}
	leaf := func(id rex.BlockID) fakestate.Node {
		t.Helper()
		n := f.State(t).Sessions[0].Windows[0].Layout.Leaf(string(id))
		if n == nil {
			t.Fatalf("block %s has no layout leaf", id)
		}
		return *n
	}
	if got, want := leaf(b).Rect, [4]float64{0, 0, 1, 0.5}; got != want {
		t.Errorf("b rect = %v, want %v", got, want)
	}
	if got, want := leaf(cc).Rect, [4]float64{0, 0.5, 1, 0.5}; got != want {
		t.Errorf("c rect = %v, want %v", got, want)
	}
	next, err := c.NewSplit(ctx(t), sid, b, "horizontal", "after", spec("next", "/bin/sh", "-c", "sleep 10"))
	if err != nil {
		t.Fatalf("NewSplit next: %v", err)
	}
	if got, want := leaf(b).Rect, [4]float64{0, 0, 0.5, 0.5}; got != want {
		t.Errorf("b rect after split = %v, want %v", got, want)
	}
	if got, want := leaf(next).Rect, [4]float64{0.5, 0, 0.5, 0.5}; got != want {
		t.Errorf("next rect = %v, want %v", got, want)
	}
}
