package provider

import (
	"context"
	"errors"

	"github.com/docbobo/rex-gascity/internal/rex"
)

// paneLifeState is what one Rex `process` read says about a pane's program.
type paneLifeState int

const (
	// paneUnknown means the read failed for a reason other than a vanished
	// pane: Rex unreachable, a lost reply, the op's deadline. It is the zero
	// value, so a reading nobody made is never mistaken for an answer.
	paneUnknown paneLifeState = iota
	// paneGone means Rex no longer knows the block (ErrNotFound): its command
	// exited and closed the block.
	paneGone
	// paneExited means the read succeeded and reported a last exit.
	paneExited
	// paneRunning means the read succeeded and reported no last exit.
	paneRunning
)

// paneLife is one classified Rex `process` read of a pane. It is process
// liveness: whether the pane's program still runs, and which processes Rex
// reports for it. panestate.go answers a different question, what the pane's
// screen shows; the two are not merged.
type paneLife struct {
	sid   rex.SessionID
	block rex.BlockID
	state paneLifeState
	err   error           // the read's error, set only when state is paneUnknown
	info  rex.ProcessInfo // the reply, set only on paneExited and paneRunning
}

// readPaneLife makes the provider's one BlockProcess call and classifies it.
// The error policy lives here and nowhere else: only rex.ErrNotFound is gone;
// every other error is unknown and keeps its cause, so errors.Is still matches
// rex.ErrUnavailable or context.DeadlineExceeded; a successful read is exited
// when LastExit is set and running otherwise. An error never becomes exited:
// uncertain liveness must read unknown, never dead (internal/provider/AGENTS.md).
// It takes a rex.Client, not a *Provider, so the Binder and the Provider share
// it. Each op maps the reading to its own wire answer.
func readPaneLife(ctx context.Context, c rex.Client, sid rex.SessionID, block rex.BlockID) paneLife {
	l := paneLife{sid: sid, block: block}
	info, err := c.BlockProcess(ctx, sid, block)
	switch {
	case errors.Is(err, rex.ErrNotFound):
		l.state = paneGone
	case err != nil:
		l.state, l.err = paneUnknown, err
	case info.LastExit != nil:
		l.state, l.info = paneExited, info
	default:
		l.state, l.info = paneRunning, info
	}
	return l
}

// replied reports whether Rex answered the read, exited or running.
func (l paneLife) replied() bool {
	return l.state == paneExited || l.state == paneRunning
}

// alive reports whether the pane's program still runs. A gone or exited pane
// is not alive. An unknown reading returns its error, so a caller can still
// tell rex.ErrUnavailable apart.
func (l paneLife) alive() (bool, error) {
	switch l.state {
	case paneRunning:
		return true, nil
	case paneUnknown:
		return false, l.err
	default:
		return false, nil
	}
}

// foregroundPID is the foreground pid Rex reported, the root of the agent
// subtree, or 0 when there was no reply or no foreground. It ignores LastExit:
// Rex can report both, and kill-object's zombie walk runs from that pid.
func (l paneLife) foregroundPID() int {
	if !l.replied() || l.info.Foreground == nil {
		return 0
	}
	return l.info.Foreground.PID
}

// foreground is the reported foreground pid when it is positive, the root of
// the reap walk. Like foregroundPID it ignores LastExit.
func (l paneLife) foreground() (int, bool) {
	if !l.replied() || l.info.Foreground == nil || l.info.Foreground.PID <= 0 {
		return 0, false
	}
	return l.info.Foreground.PID, true
}

// reported maps each positive pid Rex reported, the foreground and then its
// ancestors, to Rex's name for it. The foreground's name wins a duplicate pid.
// It is empty when there was no reply.
func (l paneLife) reported() map[int]string {
	reported := make(map[int]string)
	if !l.replied() {
		return reported
	}
	if fg := l.info.Foreground; fg != nil && fg.PID > 0 {
		reported[fg.PID] = fg.Name
	}
	for _, a := range l.info.Ancestors {
		if a.PID > 0 {
			if _, ok := reported[a.PID]; !ok {
				reported[a.PID] = a.Name
			}
		}
	}
	return reported
}

// match is process-alive's answer for names. A gone or exited pane is dead; an
// unknown reading is LivenessUnknown, which gc reads as alive; a running pane
// is matched by processAlive.
func (l paneLife) match(names []string) Liveness {
	switch l.state {
	case paneGone, paneExited:
		return LivenessDead
	case paneRunning:
		return processAlive(l.info, names)
	default:
		return LivenessUnknown
	}
}

// peekErr is peek's answer for the read: no screen for a gone pane or an
// unreachable Rex, the read's own error otherwise, nil when Rex replied.
func (l paneLife) peekErr() error {
	switch l.state {
	case paneGone:
		return ErrNoScreen
	case paneUnknown:
		if errors.Is(l.err, rex.ErrUnavailable) {
			return ErrNoScreen
		}
		return l.err
	default:
		return nil
	}
}

// exitedAfterEmptyCapture reports whether the pane's program has exited, for
// a capture that rendered nothing. The program can exit between the process
// read and the capture, so a reading that is not exited is re-read once. A
// re-read that fails keeps the earlier answer.
func (l paneLife) exitedAfterEmptyCapture(ctx context.Context, c rex.Client, sid rex.SessionID, block rex.BlockID) bool {
	if l.state == paneExited {
		return true
	}
	return readPaneLife(ctx, c, sid, block).state == paneExited
}
