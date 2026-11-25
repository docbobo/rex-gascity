package fakerex_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/docbobo/rex-gascity/internal/rex"
	"github.com/docbobo/rex-gascity/internal/rex/cli"
	"github.com/docbobo/rex-gascity/internal/rex/fakerex"
	"github.com/docbobo/rex-gascity/internal/rex/fakerex/fakestate"
)

// contractDoc is docs/REX-CONTRACT.md, the pinned contract the fake must
// follow. It is the source of the method lists; no hand-written copy lives in
// this repo.
func contractDoc(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "docs", "REX-CONTRACT.md"))
	if err != nil {
		t.Fatalf("read contract doc: %v", err)
	}
	return string(data)
}

// docTableMethods returns the backticked names in the first cell of every table
// row in one section of the contract doc. The first cell of a section's table
// is where that section lists its method names.
func docTableMethods(t *testing.T, doc, from, to string) []string {
	t.Helper()
	start := strings.Index(doc, from)
	if start < 0 {
		t.Fatalf("contract doc has no %q section", from)
	}
	end := strings.Index(doc[start:], to)
	if end < 0 {
		t.Fatalf("contract doc has no %q section after %q", to, from)
	}
	section := doc[start : start+end]

	name := regexp.MustCompile("`([a-z][a-z_.]*)`")
	seen := map[string]bool{}
	var names []string
	for _, line := range strings.Split(section, "\n") {
		if !strings.HasPrefix(line, "|") {
			continue
		}
		cell, _, ok := strings.Cut(strings.TrimPrefix(line, "|"), "|")
		if !ok {
			continue
		}
		for _, m := range name.FindAllStringSubmatch(cell, -1) {
			if !seen[m[1]] {
				seen[m[1]] = true
				names = append(names, m[1])
			}
		}
	}
	sort.Strings(names)
	return names
}

// assertSameSet fails with the names missing from and extra in got.
func assertSameSet(t *testing.T, what string, want, got []string) {
	t.Helper()
	wantSet := map[string]bool{}
	gotSet := map[string]bool{}
	for _, s := range want {
		wantSet[s] = true
	}
	for _, s := range got {
		gotSet[s] = true
	}
	var missing, extra []string
	for _, s := range want {
		if !gotSet[s] {
			missing = append(missing, s)
		}
	}
	for _, s := range got {
		if !wantSet[s] {
			extra = append(extra, s)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)
	if len(missing) > 0 || len(extra) > 0 {
		t.Errorf("%s drifted from docs/REX-CONTRACT.md: missing %v, extra %v", what, missing, extra)
	}
}

// TestMethodTableMatchesContract is the drift guard: the fake's declared api
// methods and block sub-methods must equal the lists in the contract doc.
func TestMethodTableMatchesContract(t *testing.T) {
	doc := contractDoc(t)
	topology := docTableMethods(t, doc, "## 3. Topology methods", "## 4. Terminal block methods")
	block := docTableMethods(t, doc, "## 4. Terminal block methods", "## 5. Terminal block events")

	// server.version is not a topology method; the §10 probe calls it, so the
	// fake serves it next to the §3 table.
	server := map[string]bool{"server.version": true}
	for m := range server {
		if !strings.Contains(doc, "api call "+m) {
			t.Fatalf("contract doc no longer documents %s; update this drift rule", m)
		}
	}
	var want []string
	for _, m := range fakestate.Methods {
		if !server[m] {
			want = append(want, m)
		}
	}
	assertSameSet(t, "api method table", topology, want)
	assertSameSet(t, "block sub-method table", block, fakestate.BlockMethods)
}

// TestUnknownTargetsAndArgs proves a miss fails the way Rex does and never
// answers with a silent empty reply.
func TestUnknownTargetsAndArgs(t *testing.T) {
	f := fakerex.New(t)
	_, sid, bid := start(t, f, "dog-1", "/bin/sh", "-c", "sleep 60")

	if out, stderr, code := f.Exec(t, "api", "describe", "no.such.method"); code != 3 || out != "" || !strings.Contains(stderr, "no such method") {
		t.Errorf("api describe unknown method = (%q, %q, %d), want stderr only and exit 3", out, stderr, code)
	}
	if out, _, code := f.Exec(t, "api", "call", "no.such.method"); code != 3 || out != "" {
		t.Errorf("api call unknown method = (%q, %d), want exit 3 and no reply", out, code)
	}
	if out, _, code := f.Exec(t, "block", "inspect", "block:deadbeef"); code != 3 || out != "" {
		t.Errorf("block inspect unknown block = (%q, %d), want exit 3 and no reply", out, code)
	}
	if out, _, code := f.Exec(t, "block", "call", "-b", "block:deadbeef", "format"); code != 3 || out != "" {
		t.Errorf("block call unknown block = (%q, %d), want exit 3 and no reply", out, code)
	}
	if out, _, code := f.Exec(t, "api", "call", "block.method", fmt.Sprintf(`{"session_id":%q,"block_id":%q,"method":"no_such_method"}`, sid, bid)); code != 3 || out != "" {
		t.Errorf("unknown block method = (%q, %d), want exit 3 and no reply", out, code)
	}

	for _, argv := range [][]string{
		{"api", "call", "session.view", `{not json`},
		{"block", "call", "-s", string(sid), "-b", string(bid), "format", `nope`},
		{"api", "call", "session.view", `{"session_id":"deadbeef"}`}, // §8: a bad JSON id is exit 2
	} {
		if out, _, code := f.Exec(t, argv...); code != 2 || out != "" {
			t.Errorf("%v = (%q, %d), want exit 2 and no reply", argv, out, code)
		}
	}
}

// TestScriptedFaults covers every fault kind a test can queue per method.
func TestScriptedFaults(t *testing.T) {
	t.Run("error reply", func(t *testing.T) {
		f := fakerex.New(t)
		c := &cli.Client{Bin: f.Bin}
		f.Fault(t, "session.create", fakerex.FaultError)
		if _, err := c.CreateSession(ctx(t), "gc:acme"); err == nil {
			t.Fatal("CreateSession succeeded under a scripted error reply")
		}
		if got := f.State(t).Sessions; len(got) != 0 {
			t.Errorf("sessions = %d, want 0: an error reply must not run the call", len(got))
		}
	})

	t.Run("unreachable server", func(t *testing.T) {
		f := fakerex.New(t)
		c := &cli.Client{Bin: f.Bin}
		f.Fault(t, "session.list", fakerex.FaultUnreachable)
		if _, err := c.ListSessions(ctx(t)); !errors.Is(err, rex.ErrUnavailable) {
			t.Fatalf("ListSessions = %v, want ErrUnavailable", err)
		}
	})

	t.Run("lost reply after the side effect", func(t *testing.T) {
		f := fakerex.New(t)
		c := &cli.Client{Bin: f.Bin}
		f.Fault(t, "session.create", fakerex.FaultLostReply)
		if _, err := c.CreateSession(ctx(t), "gc:acme"); err == nil {
			t.Fatal("CreateSession succeeded although its reply was lost")
		}
		if got := f.State(t).Sessions; len(got) != 1 {
			t.Fatalf("sessions = %d, want 1: the side effect must run before the reply is lost", len(got))
		}
		// The retry path: the fault was one-shot, so the next call is clean.
		sid, err := c.CreateSession(ctx(t), "gc:acme")
		if err != nil || sid == "" {
			t.Fatalf("retry CreateSession = (%q, %v), want success", sid, err)
		}
	})

	t.Run("delayed reply", func(t *testing.T) {
		f := fakerex.New(t)
		c := &cli.Client{Bin: f.Bin}
		f.Fault(t, "session.list", fakerex.FaultDelay)
		slow, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
		defer cancel()
		begin := time.Now()
		if _, err := c.ListSessions(slow); err == nil {
			t.Fatal("ListSessions ignored the context deadline on a delayed reply")
		}
		if took := time.Since(begin); took > 2*time.Second {
			t.Errorf("delayed call took %v, want the context to cancel it", took)
		}
		if _, err := c.ListSessions(ctx(t)); err != nil {
			t.Fatalf("call after the delayed reply = %v, want success", err)
		}
	})

	t.Run("garbled JSON", func(t *testing.T) {
		f := fakerex.New(t)
		c := &cli.Client{Bin: f.Bin}
		f.Fault(t, "session.list", fakerex.FaultGarbled)
		_, err := c.ListSessions(ctx(t))
		if err == nil || !strings.Contains(err.Error(), "decode session.list reply") {
			t.Fatalf("ListSessions = %v, want a reply decode error", err)
		}
	})
}

// TestClientErrorClassification proves the cli adapter still maps the fake's
// exit codes to rex.ErrNotFound and rex.ErrUnavailable.
func TestClientErrorClassification(t *testing.T) {
	f := fakerex.New(t)
	c, sid, _ := start(t, f, "dog-1", "/bin/sh", "-c", "sleep 60")

	if _, err := c.BlockProcess(ctx(t), sid, "block:deadbeef"); !errors.Is(err, rex.ErrNotFound) {
		t.Errorf("missing block = %v, want ErrNotFound", err)
	}
	if _, err := c.SessionView(ctx(t), "session:deadbeef"); !errors.Is(err, rex.ErrNotFound) {
		t.Errorf("missing session = %v, want ErrNotFound", err)
	}

	f.Fault(t, "session.list", fakerex.FaultUnreachable)
	if _, err := c.ListSessions(ctx(t)); !errors.Is(err, rex.ErrUnavailable) {
		t.Errorf("unreachable server = %v, want ErrUnavailable", err)
	}
}

// runFake runs the fake binary directly and returns its exit code and stderr.
// It reaches the fault names, which cli.Client drops when it maps an exit code.
func runFake(t *testing.T, f *fakerex.Fake, args ...string) (int, string) {
	t.Helper()
	cmd := exec.Command(f.Bin, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		return 0, stderr.String()
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("run fake %v: %v", args, err)
	}
	return exitErr.ExitCode(), stderr.String()
}

// TestMoveWindowContract drives session.move_window through cli.Client against
// the fake binary (#47, REQ-004): a reorder, an in-place call that leaves the
// revision unchanged, a move to the end, and the three faults
// docs/REX-CONTRACT.md §3 lists.
func TestMoveWindowContract(t *testing.T) {
	f := fakerex.New(t)
	c, sid, _ := start(t, f, "dog-1", "/bin/sh", "-c", "sleep 60")
	town, _, err := c.NewWindow(ctx(t), sid, "Gas City", spec("mayor", "/bin/sh", "-c", "sleep 60"))
	if err != nil {
		t.Fatalf("NewWindow: %v", err)
	}
	order := func() ([]rex.WindowID, int64) {
		t.Helper()
		view, err := c.SessionView(ctx(t), sid)
		if err != nil {
			t.Fatalf("SessionView: %v", err)
		}
		ids := make([]rex.WindowID, 0, len(view.Windows))
		for _, w := range view.Windows {
			ids = append(ids, w.ID)
		}
		return ids, view.Revision
	}
	ids, _ := order()
	if len(ids) != 2 || ids[1] != town {
		t.Fatalf("order = %v, want [rig, %s]", ids, town)
	}
	rig := ids[0]

	// Reorder: the town window moves before the rig window.
	if err := c.MoveWindow(ctx(t), sid, town, rig); err != nil {
		t.Fatalf("MoveWindow before: %v", err)
	}
	ids, rev := order()
	if !slices.Equal(ids, []rex.WindowID{town, rig}) {
		t.Fatalf("order after move = %v, want [%s %s]", ids, town, rig)
	}

	// Already in place: no change, no revision bump.
	if err := c.MoveWindow(ctx(t), sid, town, rig); err != nil {
		t.Fatalf("MoveWindow in place: %v", err)
	}
	if ids, got := order(); got != rev || !slices.Equal(ids, []rex.WindowID{town, rig}) {
		t.Errorf("in-place move: order %v revision %d, want [%s %s] revision %d", ids, got, town, rig, rev)
	}

	// No before: move to the end, and the revision moves.
	if err := c.MoveWindow(ctx(t), sid, town, ""); err != nil {
		t.Fatalf("MoveWindow to end: %v", err)
	}
	if ids, got := order(); got <= rev || !slices.Equal(ids, []rex.WindowID{rig, town}) {
		t.Errorf("move to end: order %v revision %d, want [%s %s] revision > %d", ids, got, rig, town, rev)
	}

	// Faults: unknown window, unknown before, unknown session -> not found
	// (exit 3); before == window -> bad_request (server-side, exit 1).
	if err := c.MoveWindow(ctx(t), sid, "window:999", rig); !errors.Is(err, rex.ErrNotFound) {
		t.Errorf("unknown window = %v, want ErrNotFound", err)
	}
	if err := c.MoveWindow(ctx(t), sid, town, "window:999"); !errors.Is(err, rex.ErrNotFound) {
		t.Errorf("unknown before = %v, want ErrNotFound", err)
	}
	// Fault names: a raw CLI call must name the fault, not just exit 3. The
	// contract (§3) distinguishes an unknown session (session_not_found) from a
	// window not in the session (not_found), and the fake must too.
	if code, stderr := runFake(t, f, "api", "call", "session.move_window",
		`{"session_id":"session:999","window_id":"window:999"}`); code != 3 || !strings.Contains(stderr, "code=session_not_found") {
		t.Errorf("missing session = exit %d %q, want exit 3 naming session_not_found", code, stderr)
	}
	if code, stderr := runFake(t, f, "api", "call", "session.move_window",
		fmt.Sprintf(`{"session_id":%q,"window_id":"window:999"}`, sid)); code != 3 || !strings.Contains(stderr, "code=not_found") {
		t.Errorf("missing window = exit %d %q, want exit 3 naming not_found", code, stderr)
	}
	if err := c.MoveWindow(ctx(t), "session:999", town, rig); !errors.Is(err, rex.ErrNotFound) {
		t.Errorf("unknown session = %v, want ErrNotFound", err)
	}
	err = c.MoveWindow(ctx(t), sid, town, town)
	if err == nil || errors.Is(err, rex.ErrNotFound) || !strings.Contains(err.Error(), "bad_request") {
		t.Errorf("before == window = %v, want a bad_request failure that is not ErrNotFound", err)
	}
	if ids, _ := order(); !slices.Equal(ids, []rex.WindowID{rig, town}) {
		t.Errorf("order after failed moves = %v, want it unchanged [%s %s]", ids, rig, town)
	}
}

// TestReplySchemas drives every method through the real cli.Client and asserts
// the fields it decodes are populated. A renamed or retyped field decodes to
// the zero value, so a typo in the fake fails here instead of producing a zero
// value a caller would mistake for data.
func TestReplySchemas(t *testing.T) {
	f := fakerex.New(t)
	c := &cli.Client{Bin: f.Bin}
	covered := map[string]bool{}
	raw := func(method, body string) map[string]any {
		t.Helper()
		out, stderr, code := f.Exec(t, "api", "call", method, body)
		if code != 0 {
			t.Fatalf("%s: exit %d: %s", method, code, stderr)
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(out), &m); err != nil {
			t.Fatalf("%s reply %q: %v", method, out, err)
		}
		return m
	}

	for _, method := range []string{"server.version"} {
		reply := raw(method, "")
		if reply["version"] == "" || reply["api_revision"] != float64(14) {
			t.Errorf("%s = %v, want a version and api_revision 14", method, reply)
		}
	}

	sid, err := c.CreateSession(ctx(t), "gc:acme")
	covered["session.create"] = true
	if err != nil || sid == "" {
		t.Fatalf("CreateSession = (%q, %v)", sid, err)
	}
	wid, bid, err := c.NewWindow(ctx(t), sid, "Rig: Indigo", spec("dog-1", "/bin/sh", "-c", "cat"))
	covered["session.new_window"] = true
	if err != nil || wid == "" || bid == "" {
		t.Fatalf("NewWindow = (%q, %q, %v)", wid, bid, err)
	}

	view, err := c.SessionView(ctx(t), sid)
	covered["session.view"] = true
	if err != nil {
		t.Fatalf("SessionView: %v", err)
	}
	if view.Label != "gc:acme" || view.ActiveWindowID != wid || view.Revision == 0 || len(view.Windows) != 1 {
		t.Fatalf("view = %+v", view)
	}
	if w := view.Windows[0]; w.ID != wid || w.Label != "Rig: Indigo" || w.FocusedBlockID != bid {
		t.Fatalf("window = %+v", w)
	}
	if b := view.Windows[0].Blocks[0]; b.ID != bid || b.Label != "dog-1" || b.Rect != [4]float64{0, 0, 1, 1} {
		t.Fatalf("block = %+v, want the label and full-window rect", b)
	}

	dog2, err := c.NewSplit(ctx(t), sid, bid, "horizontal", "after", spec("dog-2", "/bin/sh", "-c", "sleep 60"))
	covered["session.new_split"] = true
	if err != nil || dog2 == "" {
		t.Fatalf("NewSplit = (%q, %v)", dog2, err)
	}
	view, err = c.SessionView(ctx(t), sid)
	if err != nil {
		t.Fatalf("SessionView: %v", err)
	}
	if b := blocksByLabel(t, view)["dog-2"]; b.Rect != [4]float64{0.5, 0, 0.5, 1} {
		t.Errorf("dog-2 rect = %v, want the right half", b.Rect)
	}

	sessions, err := c.ListSessions(ctx(t))
	covered["session.list"] = true
	if err != nil || len(sessions) != 1 || sessions[0].ID != sid || sessions[0].Label != "gc:acme" {
		t.Fatalf("ListSessions = (%+v, %v)", sessions, err)
	}

	f.Update(t, func(st *fakestate.State) {
		s, _ := st.Session(string(sid))
		s.Clients = []fakestate.Client{{ID: "client:a", Kind: "app", Name: "Rex"}}
	})
	clients, err := c.ListClients(ctx(t), sid)
	covered["session.list_clients"] = true
	if err != nil || len(clients) != 1 || clients[0].ID != "client:a" || clients[0].Kind != "app" || clients[0].Name != "Rex" {
		t.Fatalf("ListClients = (%+v, %v)", clients, err)
	}

	raw("session.focus_block", fmt.Sprintf(`{"session_id":%q,"block_id":%q}`, sid, dog2))
	raw("session.set_block_label", fmt.Sprintf(`{"session_id":%q,"block_id":%q,"label":"dog-2b"}`, sid, dog2))
	raw("session.set_window_label", fmt.Sprintf(`{"session_id":%q,"window_id":%q,"label":"Rig: Renamed"}`, sid, wid))
	raw("session.set_label", fmt.Sprintf(`{"session_id":%q,"label":"gc:renamed"}`, sid))
	covered["session.focus_block"] = true
	covered["session.set_block_label"] = true
	covered["session.set_window_label"] = true
	covered["session.set_label"] = true
	view, err = c.SessionView(ctx(t), sid)
	if err != nil {
		t.Fatalf("SessionView: %v", err)
	}
	if view.Label != "gc:renamed" || view.Windows[0].Label != "Rig: Renamed" || view.Windows[0].FocusedBlockID != dog2 {
		t.Errorf("view after labels and focus = %+v", view)
	}
	if _, ok := blocksByLabel(t, view)["dog-2b"]; !ok {
		t.Errorf("blocks = %+v, want the renamed dog-2b", view.Windows[0].Blocks)
	}

	wid2, _, err := c.NewWindow(ctx(t), sid, "Rig: Other", spec("dog-3", "/bin/sh", "-c", "sleep 60"))
	if err != nil {
		t.Fatalf("NewWindow: %v", err)
	}
	raw("session.focus_window", fmt.Sprintf(`{"session_id":%q,"window_id":%q}`, sid, wid2))
	covered["session.focus_window"] = true
	if view, err = c.SessionView(ctx(t), sid); err != nil || view.ActiveWindowID != wid2 {
		t.Fatalf("active window = %v (%v), want %s", view.ActiveWindowID, err, wid2)
	}
	if reply := raw("session.move_window", fmt.Sprintf(`{"session_id":%q,"window_id":%q}`, sid, wid2)); reply["revision"] == nil {
		t.Errorf("session.move_window reply = %v, want a revision", reply)
	}
	covered["session.move_window"] = true

	if err := c.BlockWrite(ctx(t), sid, bid, "hello"); err != nil {
		t.Fatalf("BlockWrite: %v", err)
	}
	covered["block.method.write"] = true
	if err := c.BlockEvent(ctx(t), sid, bid, rex.Event{Type: "paste", Payload: json.RawMessage(`{"text":" world"}`)}); err != nil {
		t.Fatalf("BlockEvent: %v", err)
	}
	covered["block.event"] = true
	waitOutput(t, c, sid, bid, "hello world")
	out, err := c.BlockFormat(ctx(t), sid, bid)
	covered["block.method.format"] = true
	if err != nil || !strings.Contains(out, "hello world") {
		t.Fatalf("BlockFormat = (%q, %v)", out, err)
	}
	if err := c.BlockClear(ctx(t), sid, bid); err != nil {
		t.Fatalf("BlockClear: %v", err)
	}
	covered["block.method.clear"] = true
	if out, err := c.BlockFormat(ctx(t), sid, bid); err != nil || out != "" {
		t.Errorf("format after clear = (%q, %v), want empty", out, err)
	}

	_, exited, err := c.NewWindow(ctx(t), sid, "Rig: Exit", spec("dog-5", "/bin/sh", "-c", "echo bye; exit 5"))
	if err != nil {
		t.Fatalf("NewWindow: %v", err)
	}
	f.WaitExit(t, string(exited))
	info, err := c.BlockProcess(ctx(t), sid, exited)
	covered["block.method.process"] = true
	if err != nil {
		t.Fatalf("BlockProcess: %v", err)
	}
	fg := info.Foreground
	if fg == nil || fg.PID <= 0 || fg.Name == "" || fg.Path == "" {
		t.Fatalf("foreground = %+v, want pid, name and path", fg)
	}
	if info.LastExit == nil || info.LastExit.ExitCode != 5 {
		t.Errorf("last_exit = %+v, want exit 5", info.LastExit)
	}

	f.Update(t, func(st *fakestate.State) {
		_, _, b, err := st.Block(string(exited))
		if err != nil {
			t.Fatalf("seed block: %v", err)
		}
		b.ProgramStatus = []fakestate.Record{{
			ID: "ps1", State: "idle", App: "pi",
			UpdatedAt: time.Now().UTC().Format(time.RFC3339),
		}}
	})
	status, err := c.BlockProgramStatus(ctx(t), sid, exited)
	covered["block.method.program_status"] = true
	if err != nil || len(status.Records) != 1 {
		t.Fatalf("BlockProgramStatus = (%+v, %v)", status, err)
	}
	if r := status.Records[0]; r.ID != "ps1" || r.State != "idle" || r.App != "pi" || r.UpdatedAt.IsZero() {
		t.Errorf("record = %+v", r)
	}

	// input_mode answers the default until a test sets the block's mode, then
	// answers that mode for that block only (#48 REQ-010).
	mode, err := c.BlockInputMode(ctx(t), sid, exited)
	covered["block.method.input_mode"] = true
	if err != nil || mode != (rex.InputMode{Echo: true, Canonical: false, Signals: true}) {
		t.Fatalf("default BlockInputMode = (%+v, %v), want echo, not canonical, signals", mode, err)
	}
	f.Update(t, func(st *fakestate.State) {
		_, _, b, err := st.Block(string(exited))
		if err != nil {
			t.Fatalf("seed block: %v", err)
		}
		b.InputMode = &fakestate.InputMode{Echo: false, Canonical: true, Signals: true}
	})
	if mode, err = c.BlockInputMode(ctx(t), sid, exited); err != nil || mode != (rex.InputMode{Echo: false, Canonical: true, Signals: true}) {
		t.Errorf("seeded BlockInputMode = (%+v, %v), want a secret read", mode, err)
	}

	_, big, err := c.NewWindow(ctx(t), sid, "Rig: Big", spec("big", "/bin/sh", "-c", "yes x | head -c 1100000"))
	if err != nil {
		t.Fatalf("NewWindow: %v", err)
	}
	f.WaitExit(t, string(big))
	out, err = c.BlockFormat(ctx(t), sid, big)
	covered["block.method.blob_read"] = true
	if err != nil || len(out) != 1100000 {
		t.Fatalf("paged BlockFormat = %d bytes, %v; want all 1100000", len(out), err)
	}

	if err := c.CloseBlock(ctx(t), sid, exited); err != nil {
		t.Fatalf("CloseBlock: %v", err)
	}
	covered["block.close"] = true
	if view, err = c.SessionView(ctx(t), sid); err != nil || len(view.Windows) != 3 {
		t.Fatalf("windows after close = %d (%v), want 3", len(view.Windows), err)
	}

	for _, method := range []string{"target.resolve", "target.complete"} {
		reply := raw(method, `{"kinds":["session"],"queries":["gc:renamed"]}`)
		targets, _ := reply["targets"].([]any)
		if len(targets) != 1 {
			t.Fatalf("%s targets = %v, want one", method, reply["targets"])
		}
		target, _ := targets[0].(map[string]any)
		if target["kind"] != "session" || target["id"] != string(sid) || target["label"] != "gc:renamed" {
			t.Errorf("%s target = %v", method, target)
		}
		covered[method] = true
	}

	// block.method is covered by its sub-method calls; the loop after this one
	// fails if any sub-method the provider uses is unasserted.
	covered["block.method"] = true
	if err := c.DestroySession(ctx(t), sid); err != nil {
		t.Fatalf("DestroySession: %v", err)
	}
	covered["session.destroy"] = true
	if sessions, err := c.ListSessions(ctx(t)); err != nil || len(sessions) != 0 {
		t.Errorf("sessions after destroy = (%+v, %v)", sessions, err)
	}

	server := map[string]bool{"server.version": true}
	for _, method := range fakestate.Methods {
		if !server[method] && !covered[method] {
			t.Errorf("no schema assertion for fake method %s", method)
		}
	}
	// The provider calls exactly these block sub-methods (docs/REX-CONTRACT.md
	// §4); the rest are diagnostics the cli.Client never decodes.
	for _, method := range []string{"process", "format", "blob_read", "write", "clear", "program_status", "input_mode"} {
		if !covered["block.method."+method] {
			t.Errorf("no schema assertion for block.method %s", method)
		}
	}
}
