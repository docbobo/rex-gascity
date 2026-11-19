package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/docbobo/rex-gascity/internal/rex"
)

// programStatusLine is the op's JSON payload, decoded for assertions.
type programStatusLine struct {
	Validity string `json:"validity"`
	State    string `json:"state"`
	Kind     string `json:"kind"`
}

// TestProgramStatusOpReading pins the `program-status` op's wire contract: one
// JSON line with the judged reading, exit 0 for every reading because the
// reading is the payload. A known root record is valid; no record is
// not_armed; a dead pane is exited and keeps no live state.
func TestProgramStatusOpReading(t *testing.T) {
	decode := func(t *testing.T, r result) programStatusLine {
		t.Helper()
		var line programStatusLine
		if err := json.Unmarshal([]byte(r.stdout), &line); err != nil {
			t.Fatalf("stdout = %q, want one JSON line: %v", r.stdout, err)
		}
		return line
	}

	t.Run("a known record is valid", func(t *testing.T) {
		h, fake := newHarness(t)
		h.start(dog)
		fake.ProgramStatuses = map[rex.BlockID]rex.ProgramStatus{
			"block:dog-1": {Records: []rex.ProgramStatusRecord{{ID: "a", State: "blocked", Kind: "permission"}}},
		}
		r := h.must("", "program-status", dog)
		if !strings.HasSuffix(r.stdout, "\n") {
			t.Errorf("stdout = %q, want a trailing newline", r.stdout)
		}
		line := decode(t, r)
		if line.Validity != "valid" || line.State != "blocked" || line.Kind != "permission" {
			t.Errorf("reading = %+v, want valid blocked permission", line)
		}
	})
	t.Run("no record is not_armed", func(t *testing.T) {
		h, _ := newHarness(t)
		h.start(dog)
		line := decode(t, h.must("", "program-status", dog))
		if line.Validity != "not_armed" || line.State != "" {
			t.Errorf("reading = %+v, want not_armed with no state", line)
		}
	})
	t.Run("a dead pane is exited", func(t *testing.T) {
		h, fake := newHarness(t)
		h.start(dog)
		fake.ProgramStatuses = map[rex.BlockID]rex.ProgramStatus{
			"block:dog-1": {Records: []rex.ProgramStatusRecord{{ID: "a", State: "working"}}},
		}
		fake.Processes = map[rex.BlockID]rex.ProcessInfo{"block:dog-1": {LastExit: &rex.ExitStatus{}}}
		line := decode(t, h.must("", "program-status", dog))
		if line.Validity != "exited" || line.State != "" {
			t.Errorf("reading = %+v, want exited with no state", line)
		}
	})
}

// TestProgramStatusOpFailedReadIsAnError pins that a failed read exits 1 and
// prints no reading, so gc cannot read it as a state.
func TestProgramStatusOpFailedReadIsAnError(t *testing.T) {
	t.Run("missing session", func(t *testing.T) {
		h, _ := newHarness(t)
		assertFailure(t, h.run("", "program-status", dog), exitFail)
	})
}
