package provider

import (
	"context"
	"errors"
	"maps"
	"testing"

	"github.com/docbobo/rex-gascity/internal/rex"
	"github.com/docbobo/rex-gascity/internal/rex/faketest"
)

const (
	paneLifeSID   rex.SessionID = "session:acme"
	paneLifeBlock rex.BlockID   = "block:dog-1"
)

// errPaneLifeBoom is a process-read error that is neither ErrNotFound nor
// ErrUnavailable.
var errPaneLifeBoom = errors.New("boom")

// paneLifeReads counts the BlockProcess calls the fake recorded.
func paneLifeReads(fake *faketest.Fake) int {
	n := 0
	for _, c := range fake.BlockCalls {
		if c.Op == "process" {
			n++
		}
	}
	return n
}

// TestReadPaneLife proves the pane-liveness module's one classification and
// the answers its methods give each op: ErrNotFound is gone, any other error
// is unknown and keeps its cause, LastExit set is exited, otherwise running.
func TestReadPaneLife(t *testing.T) {
	running := rex.ProcessInfo{
		Foreground: &rex.Process{PID: 100, Name: "claude"},
		Ancestors:  []rex.Process{{PID: 90, Name: "zsh"}, {PID: 1, Name: "launchd"}},
	}
	cases := []struct {
		name      string
		setup     func(*faketest.Fake)
		state     paneLifeState
		errIs     error // nil: l.err must be nil
		alive     bool
		fgPID     int
		fgOK      bool
		reported  map[int]string
		match     Liveness
		peekErrIs error // nil: peekErr() must be nil
	}{
		{
			name:      "vanished block is gone",
			setup:     func(f *faketest.Fake) { f.VanishedBlocks = map[rex.BlockID]bool{paneLifeBlock: true} },
			state:     paneGone,
			reported:  map[int]string{},
			match:     LivenessDead,
			peekErrIs: ErrNoScreen,
		},
		{
			name:      "unavailable is unknown and keeps the cause",
			setup:     func(f *faketest.Fake) { f.ProcessErr = rex.ErrUnavailable },
			state:     paneUnknown,
			errIs:     rex.ErrUnavailable,
			reported:  map[int]string{},
			match:     LivenessUnknown,
			peekErrIs: ErrNoScreen,
		},
		{
			name:      "another error is unknown and keeps the cause",
			setup:     func(f *faketest.Fake) { f.ProcessErr = errPaneLifeBoom },
			state:     paneUnknown,
			errIs:     errPaneLifeBoom,
			reported:  map[int]string{},
			match:     LivenessUnknown,
			peekErrIs: errPaneLifeBoom,
		},
		{
			name:     "deadline is unknown and keeps the cause",
			setup:    func(f *faketest.Fake) { f.ProcessErr = context.DeadlineExceeded },
			state:    paneUnknown,
			errIs:    context.DeadlineExceeded,
			reported: map[int]string{},
			match:    LivenessUnknown,
			// PeekLines returns a non-ErrUnavailable read error as is.
			peekErrIs: context.DeadlineExceeded,
		},
		{
			name: "last exit set is exited",
			setup: func(f *faketest.Fake) {
				f.Processes = map[rex.BlockID]rex.ProcessInfo{paneLifeBlock: {LastExit: &rex.ExitStatus{}}}
			},
			state:    paneExited,
			reported: map[int]string{},
			match:    LivenessDead,
		},
		{
			name: "exited keeps the reported foreground",
			setup: func(f *faketest.Fake) {
				f.Processes = map[rex.BlockID]rex.ProcessInfo{paneLifeBlock: {
					LastExit:   &rex.ExitStatus{ExitCode: 1},
					Foreground: &rex.Process{PID: 300, Name: "sleep"},
				}}
			},
			state:    paneExited,
			fgPID:    300,
			fgOK:     true,
			reported: map[int]string{300: "sleep"},
			match:    LivenessDead,
		},
		{
			name: "no last exit is running",
			setup: func(f *faketest.Fake) {
				f.Processes = map[rex.BlockID]rex.ProcessInfo{paneLifeBlock: running}
			},
			state:    paneRunning,
			alive:    true,
			fgPID:    100,
			fgOK:     true,
			reported: map[int]string{100: "claude", 90: "zsh", 1: "launchd"},
			match:    LivenessAlive,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := faketest.New()
			tc.setup(fake)
			l := readPaneLife(context.Background(), fake, paneLifeSID, paneLifeBlock)

			if l.sid != paneLifeSID || l.block != paneLifeBlock {
				t.Errorf("reading names %s/%s, want %s/%s", l.sid, l.block, paneLifeSID, paneLifeBlock)
			}
			if l.state != tc.state {
				t.Errorf("state = %v, want %v", l.state, tc.state)
			}
			if tc.errIs == nil && l.err != nil || tc.errIs != nil && !errors.Is(l.err, tc.errIs) {
				t.Errorf("err = %v, want %v", l.err, tc.errIs)
			}
			alive, err := l.alive()
			wantAliveErr := tc.state == paneUnknown
			if alive != tc.alive || (err != nil) != wantAliveErr || wantAliveErr && !errors.Is(err, tc.errIs) {
				t.Errorf("alive() = %v, %v; want %v with error %v", alive, err, tc.alive, wantAliveErr)
			}
			if got := l.foregroundPID(); got != tc.fgPID {
				t.Errorf("foregroundPID() = %d, want %d", got, tc.fgPID)
			}
			if pid, ok := l.foreground(); pid != tc.fgPID || ok != tc.fgOK {
				t.Errorf("foreground() = %d, %v; want %d, %v", pid, ok, tc.fgPID, tc.fgOK)
			}
			if got := l.reported(); !maps.Equal(got, tc.reported) {
				t.Errorf("reported() = %v, want %v", got, tc.reported)
			}
			if got := l.match([]string{"claude"}); got != tc.match {
				t.Errorf("match(claude) = %v, want %v", got, tc.match)
			}
			if err := l.peekErr(); tc.peekErrIs == nil && err != nil || tc.peekErrIs != nil && !errors.Is(err, tc.peekErrIs) {
				t.Errorf("peekErr() = %v, want %v", err, tc.peekErrIs)
			}
		})
	}

	t.Run("zero value is unknown", func(t *testing.T) {
		var l paneLife
		if l.state != paneUnknown {
			t.Errorf("zero paneLife state = %v, want paneUnknown", l.state)
		}
	})

	// The empty-capture re-read PeekLines relies on: an exited reading needs no
	// re-read; otherwise one re-read decides, and a failed re-read keeps the
	// earlier answer.
	t.Run("exitedAfterEmptyCapture", func(t *testing.T) {
		runningInfo := rex.ProcessInfo{Foreground: &rex.Process{PID: 100, Name: "sleep"}}
		exitedInfo := rex.ProcessInfo{LastExit: &rex.ExitStatus{}}
		cases := []struct {
			name      string
			first     rex.ProcessInfo
			later     rex.ProcessInfo
			failLater bool
			want      bool
			reads     int
		}{
			{name: "exited at the first read", first: exitedInfo, later: exitedInfo, want: true, reads: 1},
			{name: "exited at the re-read only", first: runningInfo, later: exitedInfo, want: true, reads: 2},
			{name: "running at both reads", first: runningInfo, later: runningInfo, want: false, reads: 2},
			{name: "failed re-read keeps the earlier answer", first: runningInfo, failLater: true, want: false, reads: 2},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				ctx := context.Background()
				fake := faketest.New()
				calls := 0
				fake.ProcessFunc = func(rex.BlockID) rex.ProcessInfo {
					calls++
					if calls == 1 {
						return tc.first
					}
					return tc.later
				}
				l := readPaneLife(ctx, fake, paneLifeSID, paneLifeBlock)
				if tc.failLater {
					fake.ProcessErr = errPaneLifeBoom
				}
				if got := l.exitedAfterEmptyCapture(ctx, fake, paneLifeSID, paneLifeBlock); got != tc.want {
					t.Errorf("exitedAfterEmptyCapture = %v, want %v", got, tc.want)
				}
				if got := paneLifeReads(fake); got != tc.reads {
					t.Errorf("process reads = %d, want %d", got, tc.reads)
				}
			})
		}
	})
}
