package provider

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/docbobo/rex-gascity/internal/rex"
)

// ProgramStatusValidity is the judged validity of a program-status reading, the
// provider half of gc's runtime.ProgramStatusProvider (GCREX-64). The words
// mirror gc's runtime.ProgramStatusValidity
// (internal/runtime/program_status.go:39-58 in gc's working tree, ahead of the
// v1.5.1 pin; the pin has no ProgramStatusProvider at all). The provider only
// takes the reading; it never decides a session's phase or hold.
//
//   - ProgramStatusUnread: no reading was taken (gc's zero value, returned when
//     the city switch is off). The provider does not produce it: with the switch
//     off gc never asks, and a session it cannot observe is an error, not
//     Unread.
//   - ProgramStatusNotArmed: the pane is live but holds no root record of ours
//     to judge, so there is no state. A Rex pane never carries a gc tap nonce
//     (the provider does not arm a capture), so an absent, empty or unknown
//     record is the honest not_armed answer.
//   - ProgramStatusValid: a live pane's newest record carries a known root word.
//   - ProgramStatusExited: the pane's program has exited. It is not a valid
//     reading: a dead pane's record must never read as a live state.
type ProgramStatusValidity string

const (
	// ProgramStatusUnread is gc's zero reading: no reading was taken.
	ProgramStatusUnread ProgramStatusValidity = ""
	// ProgramStatusValid is a live pane's known root record.
	ProgramStatusValid ProgramStatusValidity = "valid"
	// ProgramStatusNotArmed is a live pane with no root record to judge.
	ProgramStatusNotArmed ProgramStatusValidity = "not_armed"
	// ProgramStatusExited is a pane whose program has exited.
	ProgramStatusExited ProgramStatusValidity = "exited"
)

// ProgramStatusReading is the judged program-status reading of one session. It
// is a reduced runtime.ProgramStatus: Rex captures OSC 7501 itself, so the
// provider has the program's record but not gc's tap nonce, pipe liveness or
// capture arm, and cannot judge gc's silent, cleared or reader_lost states.
// Only State is a state; a non-valid reading carries none of the record's
// claims. Since is the newest record's UpdatedAt (Rex has no separate
// root-entry clock), which a consumer bounds with gc's ProgramStatusHoldLimit
// as it does for tmux.
type ProgramStatusReading struct {
	Validity ProgramStatusValidity `json:"validity"`
	State    string                `json:"state,omitempty"`
	Kind     string                `json:"kind,omitempty"`
	App      string                `json:"app,omitempty"`
	Since    time.Time             `json:"since,omitempty"`
	Updated  time.Time             `json:"updated,omitempty"`
	Reported bool                  `json:"reported,omitempty"`
}

// ProgramStatus reports, in a single observation, the judged program-status
// reading of a session's agent pane. It reuses the provider's existing
// program_status access: it resolves the binding fresh, confirms the pane is
// live, and reads the newest record (the same data get-last-activity and nudge
// gating read).
//
// A session with no pane is ErrSessionNotFound; a pane whose program has
// exited reads ProgramStatusExited; a transport or observation failure is
// returned as is, so a failed read is never a valid reading. A live pane with
// no record, or a newest record whose state is empty or not a known root word,
// reads ProgramStatusNotArmed. A known root word reads ProgramStatusValid with
// its state and, for a blocked program, its kind. The provider holds no reading
// across calls, so a stale reading cannot be served.
func (p *Provider) ProgramStatus(ctx context.Context, name string) (ProgramStatusReading, error) {
	binding, ok, err := p.binder.Resolve(ctx, ResolveRequest{Name: SessionName(name), Mode: ResolveFresh})
	if err != nil {
		return ProgramStatusReading{}, err
	}
	if !ok {
		return ProgramStatusReading{}, fmt.Errorf("%w: %s has no pane", ErrSessionNotFound, name)
	}
	alive, err := p.paneAlive(ctx, binding)
	if err != nil {
		return ProgramStatusReading{}, err
	}
	if !alive {
		return ProgramStatusReading{Validity: ProgramStatusExited}, nil
	}
	snap, err := p.programStatusSnapshot(ctx, binding)
	if err != nil {
		return ProgramStatusReading{}, err
	}
	state := strings.TrimSpace(snap.State)
	if !snap.Present || !knownProgramState(state) {
		return ProgramStatusReading{Validity: ProgramStatusNotArmed}, nil
	}
	return ProgramStatusReading{
		Validity: ProgramStatusValid,
		State:    state,
		Kind:     programBlockKind(state, snap.Kind),
		App:      "", // a Rex record carries an App but the judged reading does not expose it
		Since:    snap.Latest,
		Updated:  snap.Latest,
		Reported: true,
	}, nil
}

// programStatusSnapshot is the newest program_status record of a pane.
type programStatusSnapshot struct {
	Present bool
	State   string
	Kind    string
	Latest  time.Time
}

// programStatusSnapshot returns the newest program_status record of a pane.
// It reports Present false when Rex has no records for the pane. It is the
// provider's only BlockProgramStatus read: delivery, readiness, idle,
// activity and ProgramStatus all go through it, so they share one rule for
// "newest" (reduceProgramStatus).
func (p *Provider) programStatusSnapshot(ctx context.Context, b Binding) (programStatusSnapshot, error) {
	status, err := p.client.BlockProgramStatus(ctx, b.SessionID, b.BlockID)
	if err != nil {
		return programStatusSnapshot{}, err
	}
	return reduceProgramStatus(status.Records), nil
}

// reduceProgramStatus picks the newest record. ">=" keeps the last record on
// a tie, including all-zero timestamps.
func reduceProgramStatus(records []rex.ProgramStatusRecord) programStatusSnapshot {
	var snap programStatusSnapshot
	for _, rec := range records {
		if !snap.Present || !rec.UpdatedAt.Before(snap.Latest) {
			snap = programStatusSnapshot{Present: true, State: rec.State, Kind: rec.Kind, Latest: rec.UpdatedAt}
		}
	}
	return snap
}

// knownProgramState reports whether state is an OSC 7501 root word
// (gc runtime.ProgramState: idle, working, done, blocked, error).
func knownProgramState(state string) bool {
	switch state {
	case "idle", "working", "done", "blocked", "error":
		return true
	default:
		return false
	}
}

// programBlockKind keeps a blocked program's interaction kind when spec rev 0.4
// names it, mirroring gc's runtime.blockKind.
func programBlockKind(state, kind string) string {
	if state != "blocked" {
		return ""
	}
	switch kind {
	case "permission", "question", "auth":
		return kind
	default:
		return ""
	}
}
