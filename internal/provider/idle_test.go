package provider_test

import (
	"context"
	"errors"
	"testing"

	"github.com/docbobo/rex-gascity/internal/provider"
	"github.com/docbobo/rex-gascity/internal/rex"
	"github.com/docbobo/rex-gascity/internal/rex/faketest"
)

// TestIdleSnapshotFromProgramStatus pins the primary reading: the newest
// program_status record decides idle, busy or indeterminate without reading the
// pane. Only an idle record is idle; working and blocked are busy; a finished or
// errored program is not a safe idle boundary.
func TestIdleSnapshotFromProgramStatus(t *testing.T) {
	for _, tc := range []struct {
		name  string
		state string
		want  provider.IdleState
	}{
		{"idle is idle", "idle", provider.IdleStateIdle},
		{"working is busy", "working", provider.IdleStateBusy},
		{"blocked is busy", "blocked", provider.IdleStateBusy},
		{"done is indeterminate", "done", provider.IdleStateIndeterminate},
		{"error is indeterminate", "error", provider.IdleStateIndeterminate},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake, p := started(t)
			fake.ProgramStatuses = map[rex.BlockID]rex.ProgramStatus{
				activityBlock: {Records: []rex.ProgramStatusRecord{{ID: "a", State: tc.state}}},
			}

			got, err := p.IdleSnapshot(context.Background(), activityName)
			if err != nil {
				t.Fatalf("IdleSnapshot: %v", err)
			}
			if got != tc.want {
				t.Errorf("state = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestIdleSnapshotFallsBackToThePane pins the secondary reading: with no
// program_status record the provider classifies the rendered pane, so an agent
// that emits no OSC 7501 status still gets a snapshot.
func TestIdleSnapshotFallsBackToThePane(t *testing.T) {
	for _, tc := range []struct {
		name   string
		screen string
		want   provider.IdleState
	}{
		{"ready prompt is idle", "some output\nstill output\n❯ ", provider.IdleStateIdle},
		{"alt ready prompt is idle", "output\n› ", provider.IdleStateIdle},
		{"busy indicator is busy", "working... (12s · esc to interrupt)\n❯ ", provider.IdleStateBusy},
		{"cancel prompt is busy", "Press Esc or Ctrl+C to cancel\n❯ ", provider.IdleStateBusy},
		{"startup dialog is indeterminate", "Do you trust the files in this folder?\n❯ 1. Yes\n  2. No", provider.IdleStateIndeterminate},
		{"nothing recognised is indeterminate", "just some transcript text", provider.IdleStateIndeterminate},
		{"empty screen is indeterminate", "", provider.IdleStateIndeterminate},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake, p := started(t)
			fake.Formats = map[rex.BlockID]string{activityBlock: tc.screen}

			got, err := p.IdleSnapshot(context.Background(), activityName)
			if err != nil {
				t.Fatalf("IdleSnapshot: %v", err)
			}
			if got != tc.want {
				t.Errorf("state = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestIdleSnapshotFailedReadsAreErrors pins that a failed read is never
// reported as idle, mirroring gc's tri-state contract.
func TestIdleSnapshotFailedReadsAreErrors(t *testing.T) {
	t.Run("missing session", func(t *testing.T) {
		p := provider.New("acme", faketest.New())
		if _, err := p.IdleSnapshot(context.Background(), activityName); !errors.Is(err, provider.ErrSessionNotFound) {
			t.Errorf("err = %v, want ErrSessionNotFound", err)
		}
	})
	t.Run("program status read fails", func(t *testing.T) {
		fake, p := started(t)
		boom := errors.New("transport down")
		fake.ProgramStatusErr = boom
		if _, err := p.IdleSnapshot(context.Background(), activityName); !errors.Is(err, boom) {
			t.Errorf("err = %v, want %v", err, boom)
		}
	})
	t.Run("pane fallback read fails", func(t *testing.T) {
		fake, p := started(t)
		boom := errors.New("capture failed")
		fake.FormatErr = boom
		if _, err := p.IdleSnapshot(context.Background(), activityName); !errors.Is(err, boom) {
			t.Errorf("err = %v, want %v", err, boom)
		}
	})
}
