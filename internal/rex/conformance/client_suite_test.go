// Package conformance holds the behavior suite both Rex test doubles run: the
// in-process faketest.Fake and the real cli.Client against the fake rex
// binary. It is test-only, so no shipped binary gains a test dependency.
package conformance

import (
	"context"
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

func ctx(t *testing.T) context.Context {
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
	ctx := ctx(t)

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

func windowIDs(v rex.SessionView) []rex.WindowID {
	ids := make([]rex.WindowID, 0, len(v.Windows))
	for _, w := range v.Windows {
		ids = append(ids, w.ID)
	}
	return ids
}
