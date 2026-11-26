package provider

import (
	"testing"
	"time"

	"github.com/docbobo/rex-gascity/internal/rex"
)

// TestReduceProgramStatusNewestRecord pins the one "newest program_status
// record" rule every reader shares: the newest UpdatedAt wins, and a tie
// (including all-zero timestamps) goes to the later record in slice order.
// It is a pin on a moved rule, not a behavior proof: before the refactor its
// only red is the missing reduceProgramStatus symbol.
func TestReduceProgramStatusNewestRecord(t *testing.T) {
	t1 := time.Date(2026, 11, 26, 10, 0, 0, 0, time.UTC)
	t2 := t1.Add(time.Minute)
	cases := []struct {
		name    string
		records []rex.ProgramStatusRecord
		want    programStatusSnapshot
	}{
		{
			name:    "a no records",
			records: nil,
			want:    programStatusSnapshot{},
		},
		{
			name: "b older first, newer wins",
			records: []rex.ProgramStatusRecord{
				{State: "working", UpdatedAt: t1},
				{State: "idle", UpdatedAt: t2},
			},
			want: programStatusSnapshot{Present: true, State: "idle", Latest: t2},
		},
		{
			name: "c equal time, later record wins",
			records: []rex.ProgramStatusRecord{
				{State: "idle", UpdatedAt: t1},
				{State: "working", UpdatedAt: t1},
			},
			want: programStatusSnapshot{Present: true, State: "working", Latest: t1},
		},
		{
			name: "d zero times, later record wins",
			records: []rex.ProgramStatusRecord{
				{State: "idle"},
				{State: "working"},
			},
			want: programStatusSnapshot{Present: true, State: "working"},
		},
		{
			name: "e newer first, newer wins with its kind",
			records: []rex.ProgramStatusRecord{
				{State: "blocked", Kind: "permission", UpdatedAt: t2},
				{State: "idle", UpdatedAt: t1},
			},
			want: programStatusSnapshot{Present: true, State: "blocked", Kind: "permission", Latest: t2},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := reduceProgramStatus(tc.records)
			if got.Present != tc.want.Present || got.State != tc.want.State ||
				got.Kind != tc.want.Kind || !got.Latest.Equal(tc.want.Latest) {
				t.Fatalf("reduceProgramStatus() = %+v, want %+v", got, tc.want)
			}
		})
	}
}
