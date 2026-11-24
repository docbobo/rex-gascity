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
