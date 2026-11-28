// Package conformance holds the behavior suite both Rex test doubles run: the
// in-process faketest.Fake and the real cli.Client against the fake rex
// binary. It is test-only, so no shipped binary gains a test dependency.
package conformance

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/docbobo/rex-gascity/internal/rex"
	"github.com/docbobo/rex-gascity/internal/rex/cli"
	"github.com/docbobo/rex-gascity/internal/rex/fakerex"
	"github.com/docbobo/rex-gascity/internal/rex/faketest"
)

// spec is a single-pane launch carrying a cheap long-running command, the
// shape internal/launch sends. Label is set on both the spec and the layout
// block so the in-process fake and the binary fake name the pane the same.
func spec(label string) rex.LaunchSpec {
	return rex.LaunchSpec{
		Command: []string{"/bin/sh", "-c", "sleep 60"},
		Label:   label,
		Layout: rex.Layout{Block: &rex.LayoutBlock{
			Flavor:  "com.superlogical.terminal.shell",
			Options: map[string]any{"command": []string{"/bin/sh", "-c", "sleep 60"}},
			Label:   label,
		}},
	}
}

func testContext(t *testing.T) context.Context {
	t.Helper()
	c, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	return c
}

// TestCloseBlockMovesFocus is the named REQ-006 proof: closing the focused
// block must move focus to the window's first surviving block, as the binary
// fake does. On the pre-change tree the faketest arm leaves focus stale.
func TestCloseBlockMovesFocus(t *testing.T) {
	runCloseBlockMovesFocus(t, faketest.New())
}

// TestCloseBlockMovesFocusBinary runs the same suite against the fake rex
// binary through the real cli.Client, so the in-process fake is measured
// against the contract the binary implements.
func TestCloseBlockMovesFocusBinary(t *testing.T) {
	f := fakerex.New(t)
	runCloseBlockMovesFocus(t, &cli.Client{Bin: f.Bin})
}

func runCloseBlockMovesFocus(t *testing.T, c rex.Client) {
	t.Helper()
	ctx := testContext(t)

	sid, err := c.CreateSession(ctx, "gc:focus")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	wid, first, err := c.NewWindow(ctx, sid, "Rig: One", spec("dog-1"))
	if err != nil {
		t.Fatalf("NewWindow: %v", err)
	}
	second, err := c.NewSplit(ctx, sid, first, "horizontal", "after", spec("dog-2"))
	if err != nil {
		t.Fatalf("NewSplit: %v", err)
	}

	if err := c.CloseBlock(ctx, sid, first); err != nil {
		t.Fatalf("CloseBlock: %v", err)
	}

	view, err := c.SessionView(ctx, sid)
	if err != nil {
		t.Fatalf("SessionView: %v", err)
	}
	if len(view.Windows) != 1 || view.Windows[0].ID != wid {
		t.Fatalf("windows = %v, want the one window %s", windowIDs(view), wid)
	}
	if got := view.Windows[0].FocusedBlockID; got != second {
		t.Errorf("FocusedBlockID after closing the focused block = %q, want the surviving %s", got, second)
	}
	if got := view.Windows[0].Blocks; len(got) != 1 || got[0].ID != second {
		t.Errorf("blocks = %v, want just %s", got, second)
	}
}

// TestClientSuite runs the shared behavior suite against both doubles. The
// binary arm is the reference; the in-process arm must agree with it. Revision
// is deliberately not asserted for the in-process adapter (plan open question
// 3), and the unknown-session SessionView fallback is out of scope because
// faketest's field-seeded view pins it (REQ-007).
func TestClientSuite(t *testing.T) {
	t.Run("faketest", func(t *testing.T) {
		RunClientSuite(t, func(t *testing.T) rex.Client { return faketest.New() })
	})
	t.Run("binary", func(t *testing.T) {
		RunClientSuite(t, func(t *testing.T) rex.Client {
			return &cli.Client{Bin: fakerex.New(t).Bin}
		})
	})
}

// RunClientSuite is every behavior the two doubles must share: create, launch,
// split, close, move, view, list and destroy, each case on a fresh client. A
// fresh client per case is required rather than convenient: faketest models
// one city session per instance with a single facet View (D1), and REQ-007
// pins that view's field-seeded fallback, so one fake shared across sessions
// would assert multi-session isolation the in-process fake does not model. The
// arms' id schemes (label-derived in faketest, counters in the binary) never
// leak into assertions because every case uses the ids the client returned.
func RunClientSuite(t *testing.T, newClient func(t *testing.T) rex.Client) {
	t.Run("create session", func(t *testing.T) {
		c := newClient(t)
		ctx := testContext(t)

		sid, err := c.CreateSession(ctx, "suite:create")
		if err != nil {
			t.Fatalf("CreateSession: %v", err)
		}
		if sid == "" {
			t.Fatal("CreateSession returned an empty id")
		}
		sessions, err := c.ListSessions(ctx)
		if err != nil {
			t.Fatalf("ListSessions: %v", err)
		}
		if !hasSession(sessions, sid, "suite:create") {
			t.Errorf("sessions = %v, want %s with label suite:create", sessions, sid)
		}
	})

	t.Run("new window twice", func(t *testing.T) {
		c := newClient(t)
		ctx, sid := session(t, c, "suite:windows")

		first, firstBlock, err := c.NewWindow(ctx, sid, "tab-one", spec("win-1"))
		if err != nil {
			t.Fatalf("NewWindow: %v", err)
		}
		second, secondBlock, err := c.NewWindow(ctx, sid, "tab-two", spec("win-2"))
		if err != nil {
			t.Fatalf("NewWindow: %v", err)
		}
		view := sessionView(t, c, sid)
		if got := windowIDs(view); len(got) != 2 || got[0] != first || got[1] != second {
			t.Fatalf("window order = %v, want [%s %s]", got, first, second)
		}
		for i, want := range []struct {
			window rex.WindowID
			block  rex.BlockID
		}{{first, firstBlock}, {second, secondBlock}} {
			w := view.Windows[i]
			if w.ID != want.window {
				t.Errorf("window %d = %s, want %s", i, w.ID, want.window)
			}
			if w.FocusedBlockID != want.block {
				t.Errorf("window %s focused %q, want its new block %s", w.ID, w.FocusedBlockID, want.block)
			}
			if len(w.Blocks) != 1 || w.Blocks[0].ID != want.block {
				t.Errorf("window %s blocks = %v, want just %s", w.ID, blockIDs(w), want.block)
			}
		}
		if view.ActiveWindowID != first {
			t.Errorf("ActiveWindowID = %q, want the first window %s", view.ActiveWindowID, first)
		}
	})

	t.Run("new split", func(t *testing.T) {
		c := newClient(t)
		ctx, sid := session(t, c, "suite:split")

		wid, anchor, err := c.NewWindow(ctx, sid, "tab-one", spec("split-1"))
		if err != nil {
			t.Fatalf("NewWindow: %v", err)
		}
		added, err := c.NewSplit(ctx, sid, anchor, "horizontal", "after", spec("split-2"))
		if err != nil {
			t.Fatalf("NewSplit: %v", err)
		}
		view := sessionView(t, c, sid)
		if got := windowIDs(view); len(got) != 1 || got[0] != wid {
			t.Fatalf("windows = %v, want the anchor's one window %s", got, wid)
		}
		w := view.Windows[0]
		if got := blockIDs(w); len(got) != 2 || got[0] != anchor || got[1] != added {
			t.Errorf("blocks = %v, want [%s %s]", got, anchor, added)
		}
		if w.FocusedBlockID != anchor {
			t.Errorf("FocusedBlockID after split = %q, want the anchor %s", w.FocusedBlockID, anchor)
		}
	})

	t.Run("close non-focused block", func(t *testing.T) {
		c := newClient(t)
		ctx, sid := session(t, c, "suite:close-other")

		wid, anchor, added := splitWindow(t, c, sid, "close-other", "close-other-2")
		if err := c.CloseBlock(ctx, sid, added); err != nil {
			t.Fatalf("CloseBlock: %v", err)
		}
		view := sessionView(t, c, sid)
		if got := windowIDs(view); len(got) != 1 || got[0] != wid {
			t.Fatalf("windows = %v, want the one window %s", got, wid)
		}
		w := view.Windows[0]
		if got := blockIDs(w); len(got) != 1 || got[0] != anchor {
			t.Errorf("blocks = %v, want just %s", got, anchor)
		}
		if w.FocusedBlockID != anchor {
			t.Errorf("FocusedBlockID = %q, want the untouched %s", w.FocusedBlockID, anchor)
		}
	})

	t.Run("close focused block", func(t *testing.T) {
		c := newClient(t)
		ctx, sid := session(t, c, "suite:close-focused")

		wid, anchor, added := splitWindow(t, c, sid, "close-focused", "close-focused-2")
		if err := c.CloseBlock(ctx, sid, anchor); err != nil {
			t.Fatalf("CloseBlock: %v", err)
		}
		view := sessionView(t, c, sid)
		if got := windowIDs(view); len(got) != 1 || got[0] != wid {
			t.Fatalf("windows = %v, want the one window %s", got, wid)
		}
		w := view.Windows[0]
		if got := blockIDs(w); len(got) != 1 || got[0] != added {
			t.Errorf("blocks = %v, want just the survivor %s", got, added)
		}
		if w.FocusedBlockID != added {
			t.Errorf("FocusedBlockID = %q, want the survivor %s", w.FocusedBlockID, added)
		}
	})

	t.Run("close last block of a window", func(t *testing.T) {
		c := newClient(t)
		ctx, sid := session(t, c, "suite:close-last")

		first, firstBlock, err := c.NewWindow(ctx, sid, "tab-one", spec("last-1"))
		if err != nil {
			t.Fatalf("NewWindow: %v", err)
		}
		second, secondBlock, err := c.NewWindow(ctx, sid, "tab-two", spec("last-2"))
		if err != nil {
			t.Fatalf("NewWindow: %v", err)
		}

		if err := c.CloseBlock(ctx, sid, secondBlock); err != nil {
			t.Fatalf("CloseBlock non-active window: %v", err)
		}
		view := sessionView(t, c, sid)
		if got := windowIDs(view); len(got) != 1 || got[0] != first {
			t.Fatalf("windows = %v, want only the untouched %s (the emptied %s gone)", got, first, second)
		}
		if view.ActiveWindowID != first {
			t.Errorf("ActiveWindowID = %q, want %s after closing another tab", view.ActiveWindowID, first)
		}

		if err := c.CloseBlock(ctx, sid, firstBlock); err != nil {
			t.Fatalf("CloseBlock active window: %v", err)
		}
		view = sessionView(t, c, sid)
		if got := len(view.Windows); got != 0 {
			t.Errorf("windows = %d, want the emptied tab gone", got)
		}
		if view.ActiveWindowID != "" {
			t.Errorf("ActiveWindowID = %q, want cleared with the emptied tab", view.ActiveWindowID)
		}
	})

	t.Run("move window", func(t *testing.T) {
		c := newClient(t)
		ctx, sid := session(t, c, "suite:move")

		first := newWindow(t, c, sid, "move-1")
		second := newWindow(t, c, sid, "move-2")
		third := newWindow(t, c, sid, "move-3")

		steps := []struct {
			name   string
			window rex.WindowID
			before rex.WindowID
			want   []rex.WindowID
		}{
			{"before", third, first, []rex.WindowID{third, first, second}},
			{"to the end", third, "", []rex.WindowID{first, second, third}},
			{"no-op", second, third, []rex.WindowID{first, second, third}},
		}
		for _, step := range steps {
			if err := c.MoveWindow(ctx, sid, step.window, step.before); err != nil {
				t.Fatalf("MoveWindow %s: %v", step.name, err)
			}
			if got := windowIDs(sessionView(t, c, sid)); !equalIDs(got, step.want) {
				t.Errorf("order after %s = %v, want %v", step.name, got, step.want)
			}
		}
	})

	t.Run("move before itself", func(t *testing.T) {
		c := newClient(t)
		ctx, sid := session(t, c, "suite:move-self")

		first := newWindow(t, c, sid, "self-1")
		newWindow(t, c, sid, "self-2")
		before := windowIDs(sessionView(t, c, sid))

		err := c.MoveWindow(ctx, sid, first, first)
		if err == nil {
			t.Fatal("MoveWindow before itself succeeded, want a bad_request failure")
		}
		if errors.Is(err, rex.ErrNotFound) {
			t.Errorf("MoveWindow before itself = %v, want a non-not_found business failure", err)
		}
		if !strings.Contains(err.Error(), "bad_request") {
			t.Errorf("MoveWindow before itself = %v, want it to name bad_request", err)
		}
		if got := windowIDs(sessionView(t, c, sid)); !equalIDs(got, before) {
			t.Errorf("order after a failed move = %v, want unchanged %v", got, before)
		}
	})

	t.Run("move unknown window", func(t *testing.T) {
		c := newClient(t)
		ctx, sid := session(t, c, "suite:move-unknown")

		first := newWindow(t, c, sid, "unknown-1")
		newWindow(t, c, sid, "unknown-2")
		before := windowIDs(sessionView(t, c, sid))

		if err := c.MoveWindow(ctx, sid, "window:zz", first); !errors.Is(err, rex.ErrNotFound) {
			t.Errorf("MoveWindow unknown window = %v, want rex.ErrNotFound", err)
		}
		if err := c.MoveWindow(ctx, sid, first, "window:zz"); !errors.Is(err, rex.ErrNotFound) {
			t.Errorf("MoveWindow unknown before = %v, want rex.ErrNotFound", err)
		}
		if got := windowIDs(sessionView(t, c, sid)); !equalIDs(got, before) {
			t.Errorf("order after unknown moves = %v, want unchanged %v", got, before)
		}
	})

	t.Run("move unknown session", func(t *testing.T) {
		c := newClient(t)
		ctx, sid := session(t, c, "suite:move-session")

		first := newWindow(t, c, sid, "session-1")
		before := windowIDs(sessionView(t, c, sid))

		if err := c.MoveWindow(ctx, "session:zz", first, ""); !errors.Is(err, rex.ErrNotFound) {
			t.Errorf("MoveWindow unknown session = %v, want rex.ErrNotFound", err)
		}
		if got := windowIDs(sessionView(t, c, sid)); !equalIDs(got, before) {
			t.Errorf("order after the failed call = %v, want unchanged %v", got, before)
		}
	})

	t.Run("destroy session twice", func(t *testing.T) {
		c := newClient(t)
		ctx, sid := session(t, c, "suite:destroy")

		newWindow(t, c, sid, "destroy-1")
		if err := c.DestroySession(ctx, sid); err != nil {
			t.Fatalf("DestroySession: %v", err)
		}
		sessions, err := c.ListSessions(ctx)
		if err != nil {
			t.Fatalf("ListSessions: %v", err)
		}
		if hasSession(sessions, sid, "suite:destroy") {
			t.Errorf("sessions = %v, want the destroyed session gone", sessions)
		}
		if err := c.DestroySession(ctx, sid); !errors.Is(err, rex.ErrNotFound) {
			t.Errorf("second DestroySession = %v, want rex.ErrNotFound", err)
		}
	})

	t.Run("session view", func(t *testing.T) {
		c := newClient(t)
		_, sid := session(t, c, "suite:view")

		wid := newWindow(t, c, sid, "view-1")
		view := sessionView(t, c, sid)
		if view.SessionID != sid {
			t.Errorf("SessionID = %q, want %s", view.SessionID, sid)
		}
		if view.Label != "suite:view" {
			t.Errorf("Label = %q, want suite:view", view.Label)
		}
		if got := windowIDs(view); len(got) != 1 || got[0] != wid {
			t.Errorf("windows = %v, want [%s]", got, wid)
		}
	})

	t.Run("list sessions", func(t *testing.T) {
		c := newClient(t)
		ctx := testContext(t)

		first, err := c.CreateSession(ctx, "suite:list-one")
		if err != nil {
			t.Fatalf("CreateSession: %v", err)
		}
		second, err := c.CreateSession(ctx, "suite:list-two")
		if err != nil {
			t.Fatalf("CreateSession: %v", err)
		}
		sessions, err := c.ListSessions(ctx)
		if err != nil {
			t.Fatalf("ListSessions: %v", err)
		}
		if !hasSession(sessions, first, "suite:list-one") {
			t.Errorf("sessions = %v, want %s", sessions, first)
		}
		if !hasSession(sessions, second, "suite:list-two") {
			t.Errorf("sessions = %v, want %s", sessions, second)
		}
	})
}

// session creates a session under a label no other case uses and returns the
// context its callers share.
func session(t *testing.T, c rex.Client, label string) (context.Context, rex.SessionID) {
	t.Helper()
	ctx := testContext(t)
	sid, err := c.CreateSession(ctx, label)
	if err != nil {
		t.Fatalf("CreateSession(%q): %v", label, err)
	}
	return ctx, sid
}

// newWindow adds one window and returns its id, failing on any error.
func newWindow(t *testing.T, c rex.Client, sid rex.SessionID, label string) rex.WindowID {
	t.Helper()
	wid, _, err := c.NewWindow(testContext(t), sid, label, spec(label))
	if err != nil {
		t.Fatalf("NewWindow(%q): %v", label, err)
	}
	return wid
}

// splitWindow opens one window and splits it once, returning the window, the
// anchor block and the block the split added.
func splitWindow(t *testing.T, c rex.Client, sid rex.SessionID, window, added string) (rex.WindowID, rex.BlockID, rex.BlockID) {
	t.Helper()
	ctx := testContext(t)
	wid, anchor, err := c.NewWindow(ctx, sid, window, spec(window))
	if err != nil {
		t.Fatalf("NewWindow: %v", err)
	}
	block, err := c.NewSplit(ctx, sid, anchor, "horizontal", "after", spec(added))
	if err != nil {
		t.Fatalf("NewSplit: %v", err)
	}
	return wid, anchor, block
}

func sessionView(t *testing.T, c rex.Client, sid rex.SessionID) rex.SessionView {
	t.Helper()
	view, err := c.SessionView(testContext(t), sid)
	if err != nil {
		t.Fatalf("SessionView: %v", err)
	}
	return view
}

func hasSession(sessions []rex.Session, sid rex.SessionID, label string) bool {
	for _, s := range sessions {
		if s.ID == sid && s.Label == label {
			return true
		}
	}
	return false
}

func windowIDs(v rex.SessionView) []rex.WindowID {
	ids := make([]rex.WindowID, 0, len(v.Windows))
	for _, w := range v.Windows {
		ids = append(ids, w.ID)
	}
	return ids
}

func blockIDs(w rex.Window) []rex.BlockID {
	ids := make([]rex.BlockID, 0, len(w.Blocks))
	for _, b := range w.Blocks {
		ids = append(ids, b.ID)
	}
	return ids
}

func equalIDs[T comparable](a, b []T) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
