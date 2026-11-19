package main

import (
	"errors"
	"testing"

	"github.com/docbobo/rex-gascity/internal/rex"
)

// TestIdleOpTriState pins the `idle` op's wire contract: one word, idle, busy
// or indeterminate, with exit 0 for every state because the state is the
// payload. The program_status record decides first; with none the rendered pane
// is classified.
func TestIdleOpTriState(t *testing.T) {
	t.Run("program_status idle", func(t *testing.T) {
		h, fake := newHarness(t)
		h.start(dog)
		fake.ProgramStatuses = map[rex.BlockID]rex.ProgramStatus{
			"block:dog-1": {Records: []rex.ProgramStatusRecord{{ID: "a", State: "idle"}}},
		}
		if r := h.must("", "idle", dog); r.stdout != "idle\n" {
			t.Errorf("stdout = %q, want %q", r.stdout, "idle\n")
		}
	})
	t.Run("program_status working", func(t *testing.T) {
		h, fake := newHarness(t)
		h.start(dog)
		fake.ProgramStatuses = map[rex.BlockID]rex.ProgramStatus{
			"block:dog-1": {Records: []rex.ProgramStatusRecord{{ID: "a", State: "working"}}},
		}
		if r := h.must("", "idle", dog); r.stdout != "busy\n" {
			t.Errorf("stdout = %q, want %q", r.stdout, "busy\n")
		}
	})
	t.Run("no status, ready prompt", func(t *testing.T) {
		h, fake := newHarness(t)
		h.start(dog)
		fake.Formats = map[rex.BlockID]string{"block:dog-1": "output\n❯ "}
		if r := h.must("", "idle", dog); r.stdout != "idle\n" {
			t.Errorf("stdout = %q, want %q", r.stdout, "idle\n")
		}
	})
	t.Run("no status, busy indicator", func(t *testing.T) {
		h, fake := newHarness(t)
		h.start(dog)
		fake.Formats = map[rex.BlockID]string{"block:dog-1": "working (12s · esc to interrupt)\n❯ "}
		if r := h.must("", "idle", dog); r.stdout != "busy\n" {
			t.Errorf("stdout = %q, want %q", r.stdout, "busy\n")
		}
	})
	t.Run("no status, unrecognised pane", func(t *testing.T) {
		h, fake := newHarness(t)
		h.start(dog)
		fake.Formats = map[rex.BlockID]string{"block:dog-1": "just transcript text"}
		if r := h.must("", "idle", dog); r.stdout != "indeterminate\n" {
			t.Errorf("stdout = %q, want %q", r.stdout, "indeterminate\n")
		}
	})
}

// TestIdleOpFailedReadIsAnError pins that a failed read exits 1 and never
// prints a state, so gc cannot read it as idle.
func TestIdleOpFailedReadIsAnError(t *testing.T) {
	t.Run("missing session", func(t *testing.T) {
		h, _ := newHarness(t)
		assertFailure(t, h.run("", "idle", dog), exitFail)
	})
	t.Run("program status read fails", func(t *testing.T) {
		h, fake := newHarness(t)
		h.start(dog)
		fake.ProgramStatusErr = errors.New("transport down")
		assertFailure(t, h.run("", "idle", dog), exitFail)
	})
}
