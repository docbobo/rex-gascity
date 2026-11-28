package fakestate_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/docbobo/rex-gascity/internal/rex/fakerex/fakestate"
)

// block returns a pane with id.
func block(id string) *fakestate.Block { return &fakestate.Block{ID: id} }

// window returns a window holding blocks, focused on the first one.
func window(id string, blocks ...string) *fakestate.Window {
	w := &fakestate.Window{ID: id}
	for _, b := range blocks {
		w.Blocks = append(w.Blocks, block(b))
	}
	if len(blocks) > 0 {
		w.FocusedBlock = blocks[0]
	}
	return w
}

// session returns a session holding windows, with the first window active.
func session(windows ...*fakestate.Window) *fakestate.Session {
	s := &fakestate.Session{ID: "session:test"}
	for _, w := range windows {
		s.AddWindow(w)
	}
	return s
}

func windowIDs(s *fakestate.Session) []string {
	ids := make([]string, 0, len(s.Windows))
	for _, w := range s.Windows {
		ids = append(ids, w.ID)
	}
	return ids
}

// TestNewStateDefaults pins the in-memory constructor both adapters use: the
// next generated id is 1 and the paged-result map is ready for writes.
func TestNewStateDefaults(t *testing.T) {
	st := fakestate.NewState()
	if st.Next != 1 {
		t.Errorf("Next = %d, want 1", st.Next)
	}
	if st.Results == nil {
		t.Error("Results is nil, want an empty map")
	}
	if len(st.Sessions) != 0 {
		t.Errorf("Sessions = %d, want none", len(st.Sessions))
	}
	if st.Revision != 0 {
		t.Errorf("Revision = %d, want 0", st.Revision)
	}
}

// TestRemoveReportsRemoval pins Remove's result: true only when a block was
// actually removed, so the adapter can answer not-found for an unknown block.
func TestRemoveReportsRemoval(t *testing.T) {
	s := session(window("window:a", "block:a"))

	if removed := s.Remove("block:a"); !removed {
		t.Fatal("Remove(known block) = false, want true")
	}
	if removed := s.Remove("block:a"); removed {
		t.Error("Remove(removed block) = true, want false")
	}
	if removed := s.Remove("block:missing"); removed {
		t.Error("Remove(unknown block) = true, want false")
	}
}

// TestRemoveClearsActiveWindow pins Rex's rule: an emptied window is removed,
// and the active window id is cleared when it was the one that emptied.
func TestRemoveClearsActiveWindow(t *testing.T) {
	s := session(window("window:a", "block:a"), window("window:b", "block:b"))
	if s.ActiveWindow != "window:a" {
		t.Fatalf("ActiveWindow = %q, want window:a", s.ActiveWindow)
	}

	s.Remove("block:a")

	if len(s.Windows) != 1 || s.Windows[0].ID != "window:b" {
		t.Errorf("windows = %v, want [window:b]", windowIDs(s))
	}
	if s.ActiveWindow != "" {
		t.Errorf("ActiveWindow = %q, want cleared", s.ActiveWindow)
	}
}

// TestRemoveFallsBackToFirstBlock pins the focus rule the issue paraphrased as
// "the next block": a closed focused block leaves focus on the window's first
// surviving block, and a non-focused removal leaves focus alone.
func TestRemoveFallsBackToFirstBlock(t *testing.T) {
	s := session(window("window:a", "block:a", "block:b", "block:c"))
	s.Windows[0].FocusedBlock = "block:b"

	s.Remove("block:b")

	if got := s.Windows[0].FocusedBlock; got != "block:a" {
		t.Errorf("FocusedBlock = %q, want the first surviving block block:a", got)
	}
	if got := len(s.Windows[0].Blocks); got != 2 {
		t.Errorf("blocks = %d, want 2", got)
	}

	s.Remove("block:c")

	if got := s.Windows[0].FocusedBlock; got != "block:a" {
		t.Errorf("FocusedBlock after non-focused removal = %q, want block:a", got)
	}
}

// TestAddWindowFocusesWhenEmpty pins the active-window rule a new window gets:
// the first window of a session becomes active; a later one does not.
func TestAddWindowFocusesWhenEmpty(t *testing.T) {
	s := &fakestate.Session{ID: "session:test"}

	s.AddWindow(window("window:a", "block:a"))
	if s.ActiveWindow != "window:a" {
		t.Fatalf("ActiveWindow after first AddWindow = %q, want window:a", s.ActiveWindow)
	}

	s.AddWindow(window("window:b", "block:b"))
	if s.ActiveWindow != "window:a" {
		t.Errorf("ActiveWindow after second AddWindow = %q, want window:a", s.ActiveWindow)
	}
}

// TestMoveWindowOrderAndFaults pins the shared reorder rule and its fault
// classes: before == id is ErrFail (a bad request), unknown ids are
// ErrNotFound, and a failed move leaves the order untouched.
func TestMoveWindowOrderAndFaults(t *testing.T) {
	s := session(
		window("window:a", "block:a"),
		window("window:b", "block:b"),
		window("window:c", "block:c"),
	)

	changed, err := s.MoveWindow("window:c", "window:a")
	if err != nil || !changed {
		t.Fatalf("MoveWindow before = (%v, %v), want (true, nil)", changed, err)
	}
	if got, want := windowIDs(s), []string{"window:c", "window:a", "window:b"}; !slices.Equal(got, want) {
		t.Fatalf("order = %v, want %v", got, want)
	}

	if _, err := s.MoveWindow("window:a", "window:a"); !errors.Is(err, fakestate.ErrFail) {
		t.Errorf("before == id = %v, want ErrFail", err)
	}
	if _, err := s.MoveWindow("window:zz", ""); !errors.Is(err, fakestate.ErrNotFound) {
		t.Errorf("unknown window = %v, want ErrNotFound", err)
	}
	if _, err := s.MoveWindow("window:c", "window:zz"); !errors.Is(err, fakestate.ErrNotFound) {
		t.Errorf("unknown before = %v, want ErrNotFound", err)
	}
	if got, want := windowIDs(s), []string{"window:c", "window:a", "window:b"}; !slices.Equal(got, want) {
		t.Errorf("order after failed moves = %v, want unchanged %v", got, want)
	}

	changed, err = s.MoveWindow("window:c", "")
	if err != nil || !changed {
		t.Fatalf("MoveWindow to end = (%v, %v), want (true, nil)", changed, err)
	}
	if got, want := windowIDs(s), []string{"window:a", "window:b", "window:c"}; !slices.Equal(got, want) {
		t.Fatalf("order = %v, want %v", got, want)
	}

	if changed, err = s.MoveWindow("window:c", ""); err != nil || changed {
		t.Errorf("MoveWindow already last = (%v, %v), want (false, nil)", changed, err)
	}
	if changed, err = s.MoveWindow("window:a", "window:b"); err != nil || changed {
		t.Errorf("MoveWindow in place = (%v, %v), want (false, nil)", changed, err)
	}
}
