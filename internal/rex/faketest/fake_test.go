package faketest_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/docbobo/rex-gascity/internal/rex"
	"github.com/docbobo/rex-gascity/internal/rex/faketest"
)

// The fake must stay a complete rex.Client (REQ-001).
var _ rex.Client = (*faketest.Fake)(nil)

func threeWindows() *faketest.Fake {
	f := faketest.New()
	f.View.Windows = []rex.Window{{ID: "window:a"}, {ID: "window:b"}, {ID: "window:c"}}
	return f
}

func order(f *faketest.Fake) []rex.WindowID {
	ids := make([]rex.WindowID, 0, len(f.View.Windows))
	for _, w := range f.View.Windows {
		ids = append(ids, w.ID)
	}
	return ids
}

// TestFakeMoveWindowReorders pins REQ-003's reorder semantics: before a
// window, to the end, and in place (a no-op), each recorded.
func TestFakeMoveWindowReorders(t *testing.T) {
	ctx := context.Background()
	f := threeWindows()

	if err := f.MoveWindow(ctx, "session:s", "window:c", "window:a"); err != nil {
		t.Fatalf("MoveWindow before: %v", err)
	}
	if got, want := order(f), []rex.WindowID{"window:c", "window:a", "window:b"}; !slices.Equal(got, want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
	if err := f.MoveWindow(ctx, "session:s", "window:c", ""); err != nil {
		t.Fatalf("MoveWindow to end: %v", err)
	}
	if got, want := order(f), []rex.WindowID{"window:a", "window:b", "window:c"}; !slices.Equal(got, want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
	if err := f.MoveWindow(ctx, "session:s", "window:a", "window:b"); err != nil {
		t.Fatalf("MoveWindow in place: %v", err)
	}
	if got, want := order(f), []rex.WindowID{"window:a", "window:b", "window:c"}; !slices.Equal(got, want) {
		t.Fatalf("in-place order = %v, want %v", got, want)
	}
	want := []faketest.MoveWindowCall{
		{SessionID: "session:s", WindowID: "window:c", Before: "window:a"},
		{SessionID: "session:s", WindowID: "window:c", Before: ""},
		{SessionID: "session:s", WindowID: "window:a", Before: "window:b"},
	}
	if !slices.Equal(f.MoveWindowCalls, want) {
		t.Errorf("MoveWindowCalls = %+v, want %+v", f.MoveWindowCalls, want)
	}
}

// TestFakeMoveWindowRecordsFailedCalls pins REQ-003's record-first rule: a
// call that fails (before == window, an unknown id, an injected error) is
// still recorded, and only a successful call changes View.Windows. An empty
// MoveWindowCalls therefore proves "no call at all".
func TestFakeMoveWindowRecordsFailedCalls(t *testing.T) {
	ctx := context.Background()
	f := threeWindows()
	before := order(f)

	if err := f.MoveWindow(ctx, "session:s", "window:b", "window:b"); err == nil {
		t.Error("before == window succeeded, want an error")
	}
	if err := f.MoveWindow(ctx, "session:s", "window:zz", "window:a"); !errors.Is(err, rex.ErrNotFound) {
		t.Errorf("unknown window = %v, want rex.ErrNotFound", err)
	}
	if err := f.MoveWindow(ctx, "session:s", "window:b", "window:zz"); !errors.Is(err, rex.ErrNotFound) {
		t.Errorf("unknown before = %v, want rex.ErrNotFound", err)
	}
	boom := errors.New("boom")
	f.MoveWindowErr = boom
	if err := f.MoveWindow(ctx, "session:s", "window:c", "window:a"); !errors.Is(err, boom) {
		t.Errorf("injected error = %v, want %v", err, boom)
	}

	if got := len(f.MoveWindowCalls); got != 4 {
		t.Errorf("MoveWindowCalls = %d (%+v), want 4: every failed call is recorded", got, f.MoveWindowCalls)
	}
	if got := order(f); !slices.Equal(got, before) {
		t.Errorf("order = %v, want it unchanged %v", got, before)
	}
}

// TestFakeFormatCtxFunc pins the context-aware format hook: it receives the
// call's context, can fail one call, runs without the fake's lock held, and
// the call is still recorded as a "format" block call.
func TestFakeFormatCtxFunc(t *testing.T) {
	f := faketest.New()
	f.Formats = map[rex.BlockID]string{"block:a": "static"}
	boom := errors.New("boom")
	calls := 0
	f.FormatCtxFunc = func(ctx context.Context, _ rex.BlockID) (string, error) {
		calls++
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if calls == 2 {
			return "", boom
		}
		// The fake's lock is not held here; with it held this call deadlocks.
		_, _ = f.ListSessions(context.Background())
		return "screen", nil
	}
	ctx := context.Background()
	if got, err := f.BlockFormat(ctx, "s", "block:a"); err != nil || got != "screen" {
		t.Fatalf("first read = %q, %v; want screen, nil", got, err)
	}
	if _, err := f.BlockFormat(ctx, "s", "block:a"); !errors.Is(err, boom) {
		t.Fatalf("second read error = %v, want boom", err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := f.BlockFormat(cancelled, "s", "block:a"); !errors.Is(err, context.Canceled) {
		t.Fatalf("read under a cancelled context = %v, want context.Canceled", err)
	}
	formats := 0
	for _, c := range f.BlockCalls {
		if c.Op == "format" {
			formats++
		}
	}
	if formats != 3 {
		t.Fatalf("recorded %d format calls, want 3", formats)
	}
}

// twoWindows seeds the field-based View every older faketest caller uses: no
// Sessions, no Views, the active window first, focus on window one's first
// block.
func twoWindows() *faketest.Fake {
	f := faketest.New()
	f.View.ActiveWindowID = "window:one"
	f.View.Windows = []rex.Window{
		{
			ID:             "window:one",
			Label:          "Rig: One",
			FocusedBlockID: "block:dog-1",
			Blocks: []rex.Block{
				{ID: "block:dog-1", Label: "dog-1", Owner: "rex-gascity/rig/dog-1"},
				{ID: "block:dog-2", Label: "dog-2"},
			},
		},
		{
			ID:             "window:two",
			Label:          "Rig: Two",
			FocusedBlockID: "block:dog-3",
			Blocks:         []rex.Block{{ID: "block:dog-3", Label: "dog-3"}},
		},
	}
	return f
}

// TestFakeCloseBlockMovesFocus pins REQ-006 on the in-process fake over the
// field-seeded View: the closed focused block hands focus to the window's
// first survivor, an emptied tab goes away, and ActiveWindowID clears when the
// tab that emptied was active.
func TestFakeCloseBlockMovesFocus(t *testing.T) {
	ctx := context.Background()
	f := twoWindows()

	if err := f.CloseBlock(ctx, "session:s", "block:dog-1"); err != nil {
		t.Fatalf("CloseBlock: %v", err)
	}
	if got := f.View.Windows[0].FocusedBlockID; got != "block:dog-2" {
		t.Errorf("FocusedBlockID = %q, want the surviving block:dog-2", got)
	}
	if got := f.View.Windows[0].Blocks[0].Owner; got != "" {
		t.Errorf("survivor Owner = %q, want it untouched", got)
	}

	if err := f.CloseBlock(ctx, "session:s", "block:dog-3"); err != nil {
		t.Fatalf("CloseBlock last block of window two: %v", err)
	}
	if got := len(f.View.Windows); got != 1 {
		t.Fatalf("windows = %d, want the emptied tab gone", got)
	}
	if got := f.View.ActiveWindowID; got != "window:one" {
		t.Errorf("ActiveWindowID = %q, want window:one (it was not the tab closed)", got)
	}

	if err := f.CloseBlock(ctx, "session:s", "block:dog-2"); err != nil {
		t.Fatalf("CloseBlock last block of the active window: %v", err)
	}
	if got := len(f.View.Windows); got != 0 {
		t.Errorf("windows = %d, want none", got)
	}
	if got := f.View.ActiveWindowID; got != "" {
		t.Errorf("ActiveWindowID = %q, want cleared", got)
	}
	if got, want := len(f.ClosedBlocks), 3; got != want {
		t.Errorf("ClosedBlocks = %v, want all %d recorded", f.ClosedBlocks, want)
	}
}

// TestFakeCloseBlockErrors pins the failure paths: an injected error is
// returned and still recorded first, and an unknown block is ErrNotFound.
func TestFakeCloseBlockErrors(t *testing.T) {
	ctx := context.Background()
	f := twoWindows()
	boom := errors.New("boom")
	f.CloseBlockErr = boom
	if err := f.CloseBlock(ctx, "session:s", "block:dog-1"); !errors.Is(err, boom) {
		t.Errorf("injected error = %v, want %v", err, boom)
	}
	if got := len(f.View.Windows[0].Blocks); got != 2 {
		t.Errorf("blocks = %d, want the injected failure to change nothing", got)
	}
	f.CloseBlockErr = nil
	if err := f.CloseBlock(ctx, "session:s", "block:zz"); !errors.Is(err, rex.ErrNotFound) {
		t.Errorf("unknown block = %v, want rex.ErrNotFound", err)
	}
	if got := len(f.ClosedBlocks); got != 2 {
		t.Errorf("ClosedBlocks = %d, want both calls recorded", got)
	}
}

// TestFakeCloseBlockClearsLiveness pins REQ-008: a closed pane has no process,
// so a block ID reused by a relaunched pane starts live.
func TestFakeCloseBlockClearsLiveness(t *testing.T) {
	ctx := context.Background()
	f := twoWindows()
	f.Processes = map[rex.BlockID]rex.ProcessInfo{
		"block:dog-1": {Foreground: &rex.Process{Name: "sleep"}},
		"block:dog-2": {Foreground: &rex.Process{Name: "sleep"}},
	}

	if err := f.CloseBlock(ctx, "session:s", "block:dog-1"); err != nil {
		t.Fatalf("CloseBlock: %v", err)
	}
	if _, ok := f.Processes["block:dog-1"]; ok {
		t.Error("closed block:dog-1 still has a process record")
	}
	if _, ok := f.Processes["block:dog-2"]; !ok {
		t.Error("block:dog-2 lost its process record")
	}
}

// TestFakeLaunchMatchesBinaryFocus pins the launch shapes the binary fake
// implements: a new window focuses its own block and becomes active when the
// session had none, while a split leaves focus on the anchor. A split with an
// unknown anchor is ErrNotFound, as the binary's is.
func TestFakeLaunchMatchesBinaryFocus(t *testing.T) {
	ctx := context.Background()

	f := faketest.New()
	wid, block, err := f.NewWindow(ctx, "session:s", "Rig: One", rex.LaunchSpec{Label: "dog-1"})
	if err != nil {
		t.Fatalf("NewWindow: %v", err)
	}
	if got := f.View.ActiveWindowID; got != wid {
		t.Errorf("ActiveWindowID = %q, want the new window %s", got, wid)
	}
	if got := f.View.Windows[0].FocusedBlockID; got != block {
		t.Errorf("FocusedBlockID = %q, want the new block %s", got, block)
	}

	split, err := f.NewSplit(ctx, "session:s", block, "horizontal", "after", rex.LaunchSpec{Label: "dog-2"})
	if err != nil {
		t.Fatalf("NewSplit: %v", err)
	}
	if got := f.View.Windows[0].FocusedBlockID; got != block {
		t.Errorf("FocusedBlockID after split = %q, want the anchor untouched %s", got, block)
	}
	if got, want := len(f.View.Windows[0].Blocks), 2; got != want {
		t.Errorf("blocks = %d, want %d", got, want)
	}
	if got := f.View.Windows[0].Blocks[1].ID; got != split {
		t.Errorf("split block = %q, want %s", got, split)
	}

	if _, err := f.NewSplit(ctx, "session:s", "block:zz", "horizontal", "after", rex.LaunchSpec{Label: "dog-3"}); !errors.Is(err, rex.ErrNotFound) {
		t.Errorf("NewSplit on an unknown anchor = %v, want rex.ErrNotFound", err)
	}
	if got := len(f.CreatedSplits); got != 2 {
		t.Errorf("CreatedSplits = %d, want both calls recorded", got)
	}
}

// TestFakeMoveWindowActsOnSessionView pins that per-session Views stay
// authoritative: a move against a Views entry changes only that entry, and the
// facet View the rest of the suite reads is untouched.
func TestFakeMoveWindowActsOnSessionView(t *testing.T) {
	ctx := context.Background()
	f := threeWindows()
	f.Views = map[rex.SessionID]rex.SessionView{
		"session:s": {Windows: []rex.Window{{ID: "window:a"}, {ID: "window:b"}, {ID: "window:c"}}},
	}

	if err := f.MoveWindow(ctx, "session:s", "window:c", "window:a"); err != nil {
		t.Fatalf("MoveWindow: %v", err)
	}
	got := []rex.WindowID{f.Views["session:s"].Windows[0].ID, f.Views["session:s"].Windows[1].ID, f.Views["session:s"].Windows[2].ID}
	if want := []rex.WindowID{"window:c", "window:a", "window:b"}; !slices.Equal(got, want) {
		t.Errorf("Views order = %v, want %v", got, want)
	}
	if got, want := order(f), []rex.WindowID{"window:a", "window:b", "window:c"}; !slices.Equal(got, want) {
		t.Errorf("facet View order = %v, want it untouched %v", got, want)
	}
}

// TestCreateSessionThenNewWindow pins the create → launch path: the session id
// the fake returns is the one the next call acts on, and ListSessions and
// SessionView report it.
func TestCreateSessionThenNewWindow(t *testing.T) {
	ctx := context.Background()
	f := faketest.New()

	sid, err := f.CreateSession(ctx, "s")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	wid, block, err := f.NewWindow(ctx, sid, "Rig: One", rex.LaunchSpec{Label: "one"})
	if err != nil {
		t.Fatalf("NewWindow: %v", err)
	}
	sessions, err := f.ListSessions(ctx)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(sessions) != 1 || sessions[0].ID != sid || sessions[0].Label != "s" {
		t.Fatalf("sessions = %+v, want only %s with label s", sessions, sid)
	}
	view, err := f.SessionView(ctx, sid)
	if err != nil {
		t.Fatalf("SessionView: %v", err)
	}
	if view.SessionID != sid || view.Label != "s" {
		t.Errorf("view identity = %q/%q, want %s/s", view.SessionID, view.Label, sid)
	}
	if len(view.Windows) != 1 || view.Windows[0].ID != wid {
		t.Fatalf("windows = %+v, want the one launched %s", view.Windows, wid)
	}
	if view.Windows[0].FocusedBlockID != block {
		t.Errorf("FocusedBlockID = %q, want the launched block %s", view.Windows[0].FocusedBlockID, block)
	}
}

// TestDestroySessionUnknownNotFound pins the controller rule: destroying a
// session the fake does not hold, or destroying it a second time, is
// ErrNotFound, never a silent success.
func TestDestroySessionUnknownNotFound(t *testing.T) {
	ctx := context.Background()
	f := faketest.New()

	if err := f.DestroySession(ctx, "session:zz"); !errors.Is(err, rex.ErrNotFound) {
		t.Errorf("DestroySession unknown = %v, want rex.ErrNotFound", err)
	}
	sid, err := f.CreateSession(ctx, "s")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if err := f.DestroySession(ctx, sid); err != nil {
		t.Fatalf("DestroySession: %v", err)
	}
	if err := f.DestroySession(ctx, sid); !errors.Is(err, rex.ErrNotFound) {
		t.Errorf("second DestroySession = %v, want rex.ErrNotFound", err)
	}
}

// TestListSessionsClones pins the list as a snapshot: mutating a returned
// session must not write through to the fake's state.
func TestListSessionsClones(t *testing.T) {
	ctx := context.Background()
	f := faketest.New()
	if _, err := f.CreateSession(ctx, "a"); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	first, err := f.ListSessions(ctx)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	first[0].Label = "mutated"

	second, err := f.ListSessions(ctx)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(second) != 1 || second[0].Label != "a" {
		t.Errorf("sessions after mutating a returned copy = %+v, want label a", second)
	}
}
