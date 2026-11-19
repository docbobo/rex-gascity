package provider_test

import (
	"context"
	"errors"
	"testing"

	"github.com/docbobo/rex-gascity/internal/provider"
	"github.com/docbobo/rex-gascity/internal/rex"
	"github.com/docbobo/rex-gascity/internal/rex/faketest"
)

// TestProgramStatusReadingPerState pins the judged reading from the pane's
// program_status records: a known root word is valid with its blocked kind, no
// record or an unknown/empty state is not_armed, and a dead pane is exited.
func TestProgramStatusReadingPerState(t *testing.T) {
	for _, tc := range []struct {
		name       string
		state      string
		kind       string
		wantValid  provider.ProgramStatusValidity
		wantState  string
		wantKind   string
		wantReport bool
	}{
		{"idle is valid", "idle", "", provider.ProgramStatusValid, "idle", "", true},
		{"working is valid", "working", "", provider.ProgramStatusValid, "working", "", true},
		{"done is valid", "done", "", provider.ProgramStatusValid, "done", "", true},
		{"error is valid", "error", "", provider.ProgramStatusValid, "error", "", true},
		{"blocked keeps a named kind", "blocked", "permission", provider.ProgramStatusValid, "blocked", "permission", true},
		{"blocked drops an unknown kind", "blocked", "wat", provider.ProgramStatusValid, "blocked", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake, p := started(t)
			fake.ProgramStatuses = map[rex.BlockID]rex.ProgramStatus{
				activityBlock: {Records: []rex.ProgramStatusRecord{{ID: "a", State: tc.state, Kind: tc.kind}}},
			}

			got, err := p.ProgramStatus(context.Background(), activityName)
			if err != nil {
				t.Fatalf("ProgramStatus: %v", err)
			}
			if got.Validity != tc.wantValid {
				t.Errorf("validity = %q, want %q", got.Validity, tc.wantValid)
			}
			if got.State != tc.wantState {
				t.Errorf("state = %q, want %q", got.State, tc.wantState)
			}
			if got.Kind != tc.wantKind {
				t.Errorf("kind = %q, want %q", got.Kind, tc.wantKind)
			}
			if got.Reported != tc.wantReport {
				t.Errorf("reported = %v, want %v", got.Reported, tc.wantReport)
			}
		})
	}

	t.Run("no record is not_armed", func(t *testing.T) {
		_, p := started(t)
		got, err := p.ProgramStatus(context.Background(), activityName)
		if err != nil {
			t.Fatalf("ProgramStatus: %v", err)
		}
		if got.Validity != provider.ProgramStatusNotArmed {
			t.Errorf("validity = %q, want %q", got.Validity, provider.ProgramStatusNotArmed)
		}
	})
	t.Run("empty state is not_armed", func(t *testing.T) {
		fake, p := started(t)
		fake.ProgramStatuses = map[rex.BlockID]rex.ProgramStatus{
			activityBlock: {Records: []rex.ProgramStatusRecord{{ID: "a", State: ""}}},
		}
		got, err := p.ProgramStatus(context.Background(), activityName)
		if err != nil {
			t.Fatalf("ProgramStatus: %v", err)
		}
		if got.Validity != provider.ProgramStatusNotArmed {
			t.Errorf("validity = %q, want %q", got.Validity, provider.ProgramStatusNotArmed)
		}
	})
	t.Run("dead pane is exited", func(t *testing.T) {
		fake, p := started(t)
		fake.ProgramStatuses = map[rex.BlockID]rex.ProgramStatus{
			activityBlock: {Records: []rex.ProgramStatusRecord{{ID: "a", State: "working"}}},
		}
		fake.Processes = map[rex.BlockID]rex.ProcessInfo{activityBlock: {LastExit: &rex.ExitStatus{}}}
		got, err := p.ProgramStatus(context.Background(), activityName)
		if err != nil {
			t.Fatalf("ProgramStatus: %v", err)
		}
		if got.Validity != provider.ProgramStatusExited {
			t.Errorf("validity = %q, want %q", got.Validity, provider.ProgramStatusExited)
		}
	})
}

// TestProgramStatusFailedReadsAreErrors pins that a read the provider cannot
// make is an error, never a valid reading: a missing session, a failed liveness
// probe and a failed program_status read all return their error.
func TestProgramStatusFailedReadsAreErrors(t *testing.T) {
	t.Run("missing session", func(t *testing.T) {
		p := provider.New("acme", faketest.New())
		if _, err := p.ProgramStatus(context.Background(), activityName); !errors.Is(err, provider.ErrSessionNotFound) {
			t.Errorf("err = %v, want ErrSessionNotFound", err)
		}
	})
	t.Run("liveness probe fails", func(t *testing.T) {
		fake, p := started(t)
		boom := errors.New("transport down")
		fake.ProcessErr = boom
		if _, err := p.ProgramStatus(context.Background(), activityName); !errors.Is(err, boom) {
			t.Errorf("err = %v, want %v", err, boom)
		}
	})
	t.Run("program status read fails", func(t *testing.T) {
		fake, p := started(t)
		boom := errors.New("record read failed")
		fake.ProgramStatusErr = boom
		if _, err := p.ProgramStatus(context.Background(), activityName); !errors.Is(err, boom) {
			t.Errorf("err = %v, want %v", err, boom)
		}
	})
}

// TestProgramStatusNeverReportsAMissingOrStaleRecordAsLive pins that a reading
// only carries a live state when a live pane holds a known root record: an
// absent record and a dead pane's leftover record both read not valid.
func TestProgramStatusNeverReportsAMissingOrStaleRecordAsLive(t *testing.T) {
	t.Run("missing record", func(t *testing.T) {
		_, p := started(t)
		got, err := p.ProgramStatus(context.Background(), activityName)
		if err != nil {
			t.Fatalf("ProgramStatus: %v", err)
		}
		if got.Validity == provider.ProgramStatusValid || got.State != "" {
			t.Errorf("reading = %+v, want a non-valid reading with no state", got)
		}
	})
	t.Run("dead pane keeps no live state", func(t *testing.T) {
		fake, p := started(t)
		fake.ProgramStatuses = map[rex.BlockID]rex.ProgramStatus{
			activityBlock: {Records: []rex.ProgramStatusRecord{{ID: "a", State: "working"}}},
		}
		fake.Processes = map[rex.BlockID]rex.ProcessInfo{activityBlock: {LastExit: &rex.ExitStatus{}}}
		got, err := p.ProgramStatus(context.Background(), activityName)
		if err != nil {
			t.Fatalf("ProgramStatus: %v", err)
		}
		if got.Validity == provider.ProgramStatusValid || got.State != "" {
			t.Errorf("reading = %+v, want a non-valid reading with no state", got)
		}
	})
}
