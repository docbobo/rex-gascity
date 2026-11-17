package cli_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/docbobo/rex-gas-city/internal/rex"
	"github.com/docbobo/rex-gas-city/internal/rex/cli"
)

// argsFor returns the decoded JSON argument of the first call to method.
func argsFor(t *testing.T, calls [][]string, method string) map[string]any {
	t.Helper()
	for _, c := range calls {
		if len(c) >= 3 && c[0] == "api" && c[1] == "call" && c[2] == method {
			if len(c) < 4 {
				t.Fatalf("%s was called without JSON args", method)
			}
			var m map[string]any
			if err := json.Unmarshal([]byte(c[3]), &m); err != nil {
				t.Fatalf("decode %s args %q: %v", method, c[3], err)
			}
			return m
		}
	}
	t.Fatalf("no call to %s; calls = %v", method, calls)
	return nil
}

func TestCreateSession(t *testing.T) {
	fake := newFakeRex(t)
	fake.respond(t, "session.create", `{"session_id":"session:abc"}`)
	c := &cli.Client{Bin: fake.bin}

	id, err := c.CreateSession(context.Background(), "gc:acme")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if id != "session:abc" {
		t.Errorf("session id = %q, want session:abc", id)
	}
	args := argsFor(t, fake.calls(t), "session.create")
	if args["label"] != "gc:acme" {
		t.Errorf("label = %v, want gc:acme", args["label"])
	}
}

func TestListSessions(t *testing.T) {
	fake := newFakeRex(t)
	fake.respond(t, "session.list", `{"sessions":[{"session_id":"session:a","label":"gc:acme"}]}`)
	c := &cli.Client{Bin: fake.bin}

	sessions, err := c.ListSessions(context.Background())
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	want := []rex.Session{{ID: "session:a", Label: "gc:acme"}}
	if !reflect.DeepEqual(sessions, want) {
		t.Errorf("sessions = %v, want %v", sessions, want)
	}
}

func TestNewWindow(t *testing.T) {
	fake := newFakeRex(t)
	fake.respond(t, "session.new_window", `{"window_id":"window:Indigo","block_ids":["block:dog-1"],"revision":1}`)
	c := &cli.Client{Bin: fake.bin}

	win, block, err := c.NewWindow(context.Background(), "session:a", "Indigo", rex.LaunchSpec{
		Command: []string{"claude", "--flag"},
		Cwd:     "/work/indigo",
		Label:   "dog-1",
	})
	if err != nil {
		t.Fatalf("NewWindow: %v", err)
	}
	if win != "window:Indigo" || block != "block:dog-1" {
		t.Errorf("window/block = %q/%q, want window:Indigo/block:dog-1", win, block)
	}

	args := argsFor(t, fake.calls(t), "session.new_window")
	if args["session_id"] != "session:a" {
		t.Errorf("session_id = %v, want session:a", args["session_id"])
	}
	if args["window_label"] != "Indigo" {
		t.Errorf("window_label = %v, want Indigo", args["window_label"])
	}
	blockNode := nodeBlock(t, args)
	if blockNode["flavor"] != "com.superlogical.terminal.shell" {
		t.Errorf("flavor = %v, want terminal shell", blockNode["flavor"])
	}
	if blockNode["label"] != "dog-1" {
		t.Errorf("block label = %v, want dog-1", blockNode["label"])
	}
	opts, _ := blockNode["options"].(map[string]any)
	if !reflect.DeepEqual(opts["command"], []any{"claude", "--flag"}) {
		t.Errorf("command = %v, want [claude --flag]", opts["command"])
	}
	if opts["cwd"] != "/work/indigo" {
		t.Errorf("cwd = %v, want /work/indigo", opts["cwd"])
	}
	if focus, ok := args["focus"].(bool); !ok || focus {
		t.Errorf("focus = %v, want false by default", args["focus"])
	}
}

func TestNewWindowFocus(t *testing.T) {
	fake := newFakeRex(t)
	fake.respond(t, "session.new_window", `{"window_id":"window:Indigo","block_ids":["block:dog-1"],"revision":1}`)
	c := &cli.Client{Bin: fake.bin}

	if _, _, err := c.NewWindow(context.Background(), "session:a", "Indigo", rex.LaunchSpec{Focus: true}); err != nil {
		t.Fatalf("NewWindow: %v", err)
	}
	args := argsFor(t, fake.calls(t), "session.new_window")
	if focus, ok := args["focus"].(bool); !ok || !focus {
		t.Errorf("focus = %v, want true", args["focus"])
	}
}

func TestNewSplit(t *testing.T) {
	fake := newFakeRex(t)
	fake.respond(t, "session.new_split", `{"block_ids":["block:dog-2"],"revision":2}`)
	c := &cli.Client{Bin: fake.bin}

	block, err := c.NewSplit(context.Background(), "session:a", "block:dog-1", "horizontal", "after", rex.LaunchSpec{
		Command: []string{"claude"},
		Label:   "dog-2",
	})
	if err != nil {
		t.Fatalf("NewSplit: %v", err)
	}
	if block != "block:dog-2" {
		t.Errorf("block = %q, want block:dog-2", block)
	}
	args := argsFor(t, fake.calls(t), "session.new_split")
	if args["anchor_id"] != "block:dog-1" {
		t.Errorf("anchor_id = %v, want block:dog-1", args["anchor_id"])
	}
	if args["direction"] != "horizontal" || args["side"] != "after" {
		t.Errorf("direction/side = %v/%v, want horizontal/after", args["direction"], args["side"])
	}
	if focus, ok := args["focus"].(bool); !ok || focus {
		t.Errorf("focus = %v, want false by default", args["focus"])
	}
}

func TestNewSplitFocus(t *testing.T) {
	fake := newFakeRex(t)
	fake.respond(t, "session.new_split", `{"block_ids":["block:dog-2"],"revision":2}`)
	c := &cli.Client{Bin: fake.bin}

	if _, err := c.NewSplit(context.Background(), "session:a", "block:dog-1", "horizontal", "after", rex.LaunchSpec{Focus: true}); err != nil {
		t.Fatalf("NewSplit: %v", err)
	}
	args := argsFor(t, fake.calls(t), "session.new_split")
	if focus, ok := args["focus"].(bool); !ok || !focus {
		t.Errorf("focus = %v, want true", args["focus"])
	}
}

func TestCloseBlock(t *testing.T) {
	fake := newFakeRex(t)
	fake.respond(t, "block.close", `{"block_id":"block:dog-1","revision":3}`)
	c := &cli.Client{Bin: fake.bin}

	if err := c.CloseBlock(context.Background(), "session:a", "block:dog-1"); err != nil {
		t.Fatalf("CloseBlock: %v", err)
	}
	args := argsFor(t, fake.calls(t), "block.close")
	if args["session_id"] != "session:a" || args["block_id"] != "block:dog-1" {
		t.Errorf("args = %v, want session:a / block:dog-1", args)
	}
}

func TestSessionViewFlattensLayers(t *testing.T) {
	fake := newFakeRex(t)
	fake.respond(t, "session.view", `{
		"session_id":"session:a","label":"gc:acme","active_window_id":"window:Indigo","revision":4,
		"windows":[{"window_id":"window:Indigo","label":"Indigo","focused_block_id":"block:dog-1",
			"layers":[{"blocks":[{"block_id":"block:dog-1","label":"dog-1"}]}]}]}`)
	c := &cli.Client{Bin: fake.bin}

	view, err := c.SessionView(context.Background(), "session:a")
	if err != nil {
		t.Fatalf("SessionView: %v", err)
	}
	if view.Label != "gc:acme" || view.ActiveWindowID != "window:Indigo" {
		t.Errorf("view = %+v", view)
	}
	if len(view.Windows) != 1 {
		t.Fatalf("windows = %d, want 1", len(view.Windows))
	}
	win := view.Windows[0]
	if win.Label != "Indigo" || win.FocusedBlockID != "block:dog-1" {
		t.Errorf("window = %+v", win)
	}
	if len(win.Blocks) != 1 || win.Blocks[0].ID != "block:dog-1" {
		t.Errorf("blocks = %+v", win.Blocks)
	}
}

func TestBlockProcess(t *testing.T) {
	fake := newFakeRex(t)
	fake.respond(t, "process", `{"last_exit":null,"foreground":{"pid":42,"name":"claude"}}`)
	c := &cli.Client{Bin: fake.bin}

	info, err := c.BlockProcess(context.Background(), "session:a", "block:dog-1")
	if err != nil {
		t.Fatalf("BlockProcess: %v", err)
	}
	if info.LastExit != nil {
		t.Errorf("LastExit = %v, want nil", info.LastExit)
	}
	if info.Foreground == nil || info.Foreground.PID != 42 {
		t.Errorf("Foreground = %+v", info.Foreground)
	}
	args := blockMethodArgs(t, fake.calls(t))
	if args["method"] != "process" {
		t.Errorf("method = %v, want process", args["method"])
	}
}

// blockMethodArgs returns the args of the single block.method call, checking
// that the adapter scoped it to a session and a block as Rex requires.
func blockMethodArgs(t *testing.T, calls [][]string) map[string]any {
	t.Helper()
	args := argsFor(t, calls, "block.method")
	if args["session_id"] != "session:a" {
		t.Errorf("session_id = %v, want session:a", args["session_id"])
	}
	if args["block_id"] != "block:dog-1" {
		t.Errorf("block_id = %v, want block:dog-1", args["block_id"])
	}
	return args
}

func TestBlockFormatFollowsPaging(t *testing.T) {
	fake := newFakeRex(t)
	fake.respond(t, "format", `{"content":"part1","id":7,"total":15}`)
	fake.respond(t, "blob_read", `{"data":"part2","id":7,"total":15,"eof":false}`)
	fake.respond(t, "blob_read.2", `{"data":"part3","id":7,"total":15,"eof":true}`)
	c := &cli.Client{Bin: fake.bin}

	out, err := c.BlockFormat(context.Background(), "session:a", "block:dog-1")
	if err != nil {
		t.Fatalf("BlockFormat: %v", err)
	}
	if out != "part1part2part3" {
		t.Errorf("output = %q, want part1part2part3", out)
	}
	calls := fake.calls(t)
	if len(calls) != 3 {
		t.Fatalf("calls = %d, want 3 (format plus two blob_reads)", len(calls))
	}
	// The format request must ask for pages, or a large capture fails with
	// too_large instead of paging.
	format := blockMethodArgs(t, calls[:1])
	inner, _ := format["args"].(map[string]any)
	if inner["max_bytes"] == nil || inner["max_bytes"] == float64(0) {
		t.Errorf("format max_bytes = %v, want a positive byte budget", inner["max_bytes"])
	}
	// Each blob_read continues from the previous offset in the same result.
	second := argsFor(t, calls[1:2], "block.method")
	inner, _ = second["args"].(map[string]any)
	if inner["offset"] != float64(len("part1")) {
		t.Errorf("second offset = %v, want %d", inner["offset"], len("part1"))
	}
}

func TestBlockFormatSinglePage(t *testing.T) {
	fake := newFakeRex(t)
	fake.respond(t, "format", `{"content":"all of it","eof":true}`)
	c := &cli.Client{Bin: fake.bin}

	out, err := c.BlockFormat(context.Background(), "session:a", "block:dog-1")
	if err != nil {
		t.Fatalf("BlockFormat: %v", err)
	}
	if out != "all of it" {
		t.Errorf("output = %q, want %q", out, "all of it")
	}
	if calls := fake.calls(t); len(calls) != 1 {
		t.Errorf("calls = %d, want 1 (no blob_read when eof)", len(calls))
	}
}

func TestBlockWrite(t *testing.T) {
	fake := newFakeRex(t)
	fake.respond(t, "write", `{"bytes_written":5}`)
	c := &cli.Client{Bin: fake.bin}

	if err := c.BlockWrite(context.Background(), "session:a", "block:dog-1", "hello"); err != nil {
		t.Fatalf("BlockWrite: %v", err)
	}
	args := blockMethodArgs(t, fake.calls(t))
	inner, _ := args["args"].(map[string]any)
	if inner["data"] != "hello" {
		t.Errorf("data = %v, want hello", inner["data"])
	}
}

func TestBlockEvent(t *testing.T) {
	fake := newFakeRex(t)
	fake.respond(t, "block.event", `{}`)
	c := &cli.Client{Bin: fake.bin}

	if err := c.BlockEvent(context.Background(), "session:a", "block:dog-1", rex.Event{Type: "key"}); err != nil {
		t.Fatalf("BlockEvent: %v", err)
	}
	args := argsFor(t, fake.calls(t), "block.event")
	if args["session_id"] != "session:a" || args["block_id"] != "block:dog-1" {
		t.Errorf("session_id/block_id = %v/%v, want session:a/block:dog-1", args["session_id"], args["block_id"])
	}
	ev, _ := args["event"].(map[string]any)
	if ev["type"] != "key" {
		t.Errorf("event.type = %v, want key", ev["type"])
	}
}

func TestSendKeys(t *testing.T) {
	fake := newFakeRex(t)
	c := &cli.Client{Bin: fake.bin}

	if err := c.SendKeys(context.Background(), "session:a", "block:dog-1", "ctrl+c", "enter"); err != nil {
		t.Fatalf("SendKeys: %v", err)
	}
	calls := fake.calls(t)
	if len(calls) != 1 {
		t.Fatalf("calls = %d, want 1", len(calls))
	}
	want := []string{"send-key", "-s", "session:a", "-b", "block:dog-1", "--", "ctrl+c", "enter"}
	if !reflect.DeepEqual(calls[0], want) {
		t.Errorf("argv = %v, want %v", calls[0], want)
	}
}

func TestSendKeysNoKeysIsANoOp(t *testing.T) {
	fake := newFakeRex(t)
	c := &cli.Client{Bin: fake.bin}

	if err := c.SendKeys(context.Background(), "session:a", "block:dog-1"); err != nil {
		t.Fatalf("SendKeys: %v", err)
	}
	if calls := fake.calls(t); len(calls) != 0 {
		t.Errorf("calls = %v, want none", calls)
	}
}

func TestListClients(t *testing.T) {
	fake := newFakeRex(t)
	fake.respond(t, "session.list_clients", `{"clients":[{"client_id":"client:a","info":{"kind":"app","name":"Rex Beta"}},{"client_id":"client:cli","info":{"kind":"cli","name":"rex"}}]}`)
	c := &cli.Client{Bin: fake.bin}

	clients, err := c.ListClients(context.Background(), "session:a")
	if err != nil {
		t.Fatalf("ListClients: %v", err)
	}
	want := []rex.ClientInfo{
		{ID: "client:a", Kind: "app", Name: "Rex Beta"},
		{ID: "client:cli", Kind: "cli", Name: "rex"},
	}
	if !reflect.DeepEqual(clients, want) {
		t.Errorf("clients = %+v, want %+v", clients, want)
	}
	args := argsFor(t, fake.calls(t), "session.list_clients")
	if args["session_id"] != "session:a" {
		t.Errorf("session_id = %v, want session:a", args["session_id"])
	}
}

func TestBlockProgramStatus(t *testing.T) {
	fake := newFakeRex(t)
	fake.respond(t, "program_status", `{"records":[{"id":"r1","state":"working","app":"pi","updated_at":"2026-11-17T20:18:53.239854+01:00"}]}`)
	c := &cli.Client{Bin: fake.bin}

	status, err := c.BlockProgramStatus(context.Background(), "session:a", "block:dog-1")
	if err != nil {
		t.Fatalf("BlockProgramStatus: %v", err)
	}
	if len(status.Records) != 1 {
		t.Fatalf("records = %d, want 1", len(status.Records))
	}
	rec := status.Records[0]
	if rec.ID != "r1" || rec.State != "working" || rec.App != "pi" {
		t.Errorf("record = %+v", rec)
	}
	if rec.UpdatedAt.IsZero() {
		t.Errorf("UpdatedAt is zero, want parsed time")
	}
	if want := time.Date(2026, 11, 17, 20, 18, 53, 239854000, time.FixedZone("", 3600)); !rec.UpdatedAt.Equal(want) {
		t.Errorf("UpdatedAt = %v, want %v", rec.UpdatedAt, want)
	}
	args := blockMethodArgs(t, fake.calls(t))
	if args["method"] != "program_status" {
		t.Errorf("method = %v, want program_status", args["method"])
	}
}

func TestNotFoundMapsToErrNotFound(t *testing.T) {
	fake := newFakeRex(t)
	if err := os.WriteFile(filepath.Join(fake.responses, "session.view.exit"), []byte("3"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := &cli.Client{Bin: fake.bin}

	_, err := c.SessionView(context.Background(), "session:gone")
	if !errors.Is(err, rex.ErrNotFound) {
		t.Fatalf("SessionView err = %v, want rex.ErrNotFound", err)
	}
}

func TestGenericFailureIsNotErrNotFound(t *testing.T) {
	fake := newFakeRex(t)
	if err := os.WriteFile(filepath.Join(fake.responses, "session.view.exit"), []byte("1"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := &cli.Client{Bin: fake.bin}

	_, err := c.SessionView(context.Background(), "session:a")
	if err == nil {
		t.Fatal("SessionView succeeded, want error")
	}
	if errors.Is(err, rex.ErrNotFound) {
		t.Errorf("exit 1 err = %v, must not match rex.ErrNotFound", err)
	}
}

// nodeBlock pulls the layout's block node out of a decoded call.
func nodeBlock(t *testing.T, args map[string]any) map[string]any {
	t.Helper()
	layout, ok := args["layout"].(map[string]any)
	if !ok {
		t.Fatalf("layout = %v, want object", args["layout"])
	}
	block, ok := layout["block"].(map[string]any)
	if !ok {
		t.Fatalf("layout.block = %v, want object", layout["block"])
	}
	return block
}

func TestSessionViewResolvesLayoutRects(t *testing.T) {
	fake := newFakeRex(t)
	fake.respond(t, "session.view", `{
		"session_id":"session:a","label":"gc:acme","active_window_id":"window:Indigo","revision":4,
		"windows":[{"window_id":"window:Indigo","label":"Indigo","focused_block_id":"block:dog-1",
			"layers":[{
				"blocks":[{"block_id":"block:dog-1","label":"dog-1"},{"block_id":"block:dog-2","label":"dog-2"},{"block_id":"block:dog-3","label":"dog-3"}],
				"layout":{"rect":[0,0,1,1],"split":{"direction":"horizontal","ratio":0.5,
					"before":{"rect":[0,0,0.5,1],"block_id":"block:dog-1"},
					"after":{"rect":[0.5,0,0.5,1],"split":{"direction":"vertical","ratio":0.5,
						"before":{"rect":[0.5,0,0.5,0.5],"block_id":"block:dog-2"},
						"after":{"rect":[0.5,0.5,0.5,0.5],"block_id":"block:dog-3"}}}}}
			}]}]}`)
	c := &cli.Client{Bin: fake.bin}

	view, err := c.SessionView(context.Background(), "session:a")
	if err != nil {
		t.Fatalf("SessionView: %v", err)
	}
	want := []rex.Block{
		{ID: "block:dog-1", Label: "dog-1", Rect: [4]float64{0, 0, 0.5, 1}},
		{ID: "block:dog-2", Label: "dog-2", Rect: [4]float64{0.5, 0, 0.5, 0.5}},
		{ID: "block:dog-3", Label: "dog-3", Rect: [4]float64{0.5, 0.5, 0.5, 0.5}},
	}
	if got := view.Windows[0].Blocks; !slices.Equal(got, want) {
		t.Errorf("blocks = %+v, want %+v", got, want)
	}
}

func TestSessionViewBlockMissingFromLayoutHasZeroRect(t *testing.T) {
	fake := newFakeRex(t)
	fake.respond(t, "session.view", `{
		"session_id":"session:a","label":"gc:acme","active_window_id":"window:Indigo","revision":4,
		"windows":[{"window_id":"window:Indigo","label":"Indigo","focused_block_id":"block:dog-1",
			"layers":[{
				"blocks":[{"block_id":"block:dog-1","label":"dog-1"},{"block_id":"block:dog-2","label":"dog-2"}],
				"layout":{"rect":[0,0,1,1],"block_id":"block:dog-1"}
			},{
				"blocks":[{"block_id":"block:dog-1","label":"dog-1"}]
			}]}]}`)
	c := &cli.Client{Bin: fake.bin}

	view, err := c.SessionView(context.Background(), "session:a")
	if err != nil {
		t.Fatalf("SessionView: %v", err)
	}
	want := []rex.Block{
		{ID: "block:dog-1", Label: "dog-1", Rect: [4]float64{0, 0, 1, 1}},
		{ID: "block:dog-2", Label: "dog-2"},
	}
	if got := view.Windows[0].Blocks; !slices.Equal(got, want) {
		t.Errorf("blocks = %+v, want %+v", got, want)
	}
}

func TestSessionViewLayoutOnlyBlockStillAppears(t *testing.T) {
	fake := newFakeRex(t)
	fake.respond(t, "session.view", `{
		"session_id":"session:a","windows":[{"window_id":"window:Indigo","label":"Indigo",
			"layers":[{
				"blocks":[{"block_id":"block:dog-1","label":"dog-1"}],
				"layout":{"rect":[0,0,1,1],"split":{"direction":"horizontal","ratio":0.5,
					"before":{"rect":[0,0,0.5,1],"block_id":"block:dog-1"},
					"after":{"rect":[0.5,0,0.5,1],"block_id":"block:dog-2"}}}
			}]}]}`)
	c := &cli.Client{Bin: fake.bin}

	view, err := c.SessionView(context.Background(), "session:a")
	if err != nil {
		t.Fatalf("SessionView: %v", err)
	}
	want := []rex.Block{
		{ID: "block:dog-1", Label: "dog-1", Rect: [4]float64{0, 0, 0.5, 1}},
		{ID: "block:dog-2", Rect: [4]float64{0.5, 0, 0.5, 1}},
	}
	if got := view.Windows[0].Blocks; !slices.Equal(got, want) {
		t.Errorf("blocks = %+v, want %+v", got, want)
	}
}
