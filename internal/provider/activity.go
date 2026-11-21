package provider

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/docbobo/rex-gascity/internal/rex"
)

// ActivitySource names where a LastActivity timestamp came from.
type ActivitySource string

const (
	// ActivityProgramStatus is a program_status record reported by the program.
	ActivityProgramStatus ActivitySource = "program_status"
	// ActivityFormatPoll is the last time the pane's rendered output changed.
	ActivityFormatPoll ActivitySource = "format_poll"
	// ActivityNone means the session has no pane, so there is no activity.
	ActivityNone ActivitySource = "none"
)

// ActivityPolicy states which activity path is in force in v1. The OSC 7501
// emitter is the program in the pane, not the provider: Pi implements the
// protocol and gc-managed Pi panes report idle/working/blocked/done/error
// under Rex, so program_status is normally present. The provider reads it when
// Rex reports records and otherwise polls the pane.
const ActivityPolicy = "program_status preferred; format_poll fallback (OSC 7501 is emitted by the pane program, e.g. Pi)"

// formatFingerprint identifies one rendered-output sample so a later poll can
// tell whether the pane changed.
func formatFingerprint(out string) string {
	sum := sha256.Sum256([]byte(out))
	return hex.EncodeToString(sum[:])
}

// LastActivity is a session's last-activity time and the source it came from.
type LastActivity struct {
	At     time.Time
	Source ActivitySource
}

// activityRead and activityWrite are the provider's one place to reach the
// generic activity store.
func (p *Provider) activityRead(name SessionName) (ActivityRecord, bool) {
	rec, ok, _ := p.activity.Read(name)
	return rec, ok
}

func (p *Provider) activityWrite(name SessionName, rec ActivityRecord) {
	_ = p.activity.Write(name, rec)
}

// GetLastActivity reports when the agent's pane was last active. It prefers
// the latest program_status record; a newest state of working or blocked means
// now, so a silent but busy program is not mistaken for idle. Without a
// program_status record it fingerprints the rendered output and stores the
// time the fingerprint last changed, so an idle pane reports a stable
// timestamp across provider processes. In both paths a recorded pack-input
// time from the same incarnation is a floor, so typing into a pane is visible
// before its output changes. A session with no pane reports ActivityNone and a
// zero time.
func (p *Provider) GetLastActivity(ctx context.Context, name string) (LastActivity, error) {
	binding, ok, err := p.binder.Resolve(ctx, ResolveRequest{Name: SessionName(name), Mode: ResolveFresh})
	if err != nil {
		return LastActivity{}, err
	}
	if !ok {
		return LastActivity{Source: ActivityNone}, nil
	}

	at, ok, err := p.statusActivity(ctx, binding)
	if err != nil {
		return LastActivity{}, err
	}
	if ok {
		return LastActivity{At: at, Source: ActivityProgramStatus}, nil
	}

	out, err := p.client.BlockFormat(ctx, binding.SessionID, binding.BlockID)
	if err != nil {
		return LastActivity{}, err
	}
	fp := formatFingerprint(out)
	if rec, found := p.activityRead(binding.Name); found && rec.Identity == string(binding.BlockID) && rec.Fingerprint == fp {
		return LastActivity{At: laterOf(rec.At, rec.InputAt), Source: ActivityFormatPoll}, nil
	}
	now := time.Now().UTC()
	rec, found := p.activityRead(binding.Name)
	if !found || rec.Identity != string(binding.BlockID) {
		// A new incarnation starts clean: a floor from a recycled pane must
		// not leak into this one.
		rec = ActivityRecord{}
	}
	rec.Identity = string(binding.BlockID)
	rec.Fingerprint = fp
	rec.At = now
	p.activityWrite(binding.Name, rec)
	return LastActivity{At: laterOf(now, rec.InputAt), Source: ActivityFormatPoll}, nil
}

// statusActivity reports what a program_status record answers for binding: now
// when the newest state is working or blocked, so a silent but busy program is
// not mistaken for idle, else the newest record's own time. In both cases a
// recorded pack-input time from the same incarnation is a floor. It reports
// false when the box has no record, which leaves the caller to its store or
// its screen. It is the read half of GetLastActivity, shared with inventory so
// both report the same time for a box that has reported one.
func (p *Provider) statusActivity(ctx context.Context, binding Binding) (time.Time, bool, error) {
	status, err := p.client.BlockProgramStatus(ctx, binding.SessionID, binding.BlockID)
	if err != nil {
		return time.Time{}, false, err
	}
	latest, state, present := newestProgramStatus(status.Records)
	if !present {
		return time.Time{}, false, nil
	}
	inputAt := p.inputFloor(binding)
	if state == "working" || state == "blocked" {
		return laterOf(time.Now().UTC(), inputAt), true, nil
	}
	if latest.IsZero() {
		return time.Time{}, false, nil
	}
	return laterOf(latest, inputAt), true, nil
}

// newestProgramStatus returns the newest program_status record's updated time
// and state. A tie keeps the later record.
func newestProgramStatus(records []rex.ProgramStatusRecord) (time.Time, string, bool) {
	var latest time.Time
	var state string
	present := false
	for _, rec := range records {
		if !present || !rec.UpdatedAt.Before(latest) {
			latest, state, present = rec.UpdatedAt, rec.State, true
		}
	}
	return latest, state, present
}

// inputFloor is the recorded pack-input time for the pane's current
// incarnation, or the zero time when there is none. A record from another
// incarnation is ignored so a stale stamp cannot leak across a recycle.
func (p *Provider) inputFloor(b Binding) time.Time {
	rec, ok := p.activityRead(b.Name)
	if !ok || rec.Identity != string(b.BlockID) {
		return time.Time{}
	}
	return rec.InputAt
}

// noteInput records that the pack typed into the pane. The stored time is a
// floor for GetLastActivity, so input whose output has not rendered yet is not
// read as idle. It keeps the existing fingerprint and screen time for the same
// incarnation.
func (p *Provider) noteInput(b Binding) {
	rec, found := p.activityRead(b.Name)
	if !found || rec.Identity != string(b.BlockID) {
		rec = ActivityRecord{Identity: string(b.BlockID)}
	}
	rec.InputAt = time.Now().UTC()
	p.activityWrite(b.Name, rec)
}

// laterOf returns the later of two times, so an input floor can never be
// reported below a real activity stamp.
func laterOf(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}
