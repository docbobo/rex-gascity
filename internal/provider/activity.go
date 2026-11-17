package provider

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"time"
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

// ActivityPolicy states which activity path is in force in v1. gc 1.5.1 ships
// no OSC 7501 emitter, so the provider emits none either: it reads
// program_status when Rex reports records and otherwise polls the pane.
const ActivityPolicy = "program_status preferred; format_poll fallback (gc 1.5.1 ships no OSC 7501 emitter)"

// LastActivity is a session's last-activity time and the source it came from.
type LastActivity struct {
	At     time.Time
	Source ActivitySource
}

// GetLastActivity reports when the agent's pane was last active. It prefers
// the latest program_status record. Without one it fingerprints the rendered
// output and stores the time the fingerprint last changed, so an idle pane
// reports a stable timestamp across provider processes. A session with no
// pane reports ActivityNone and a zero time.
func (p *Provider) GetLastActivity(ctx context.Context, name string) (LastActivity, error) {
	binding, ok, err := p.binder.Resolve(ctx, SessionName(name))
	if err != nil {
		return LastActivity{}, err
	}
	if !ok {
		return LastActivity{Source: ActivityNone}, nil
	}

	status, err := p.client.BlockProgramStatus(ctx, binding.SessionID, binding.BlockID)
	if err != nil {
		return LastActivity{}, err
	}
	var latest time.Time
	for _, rec := range status.Records {
		if rec.UpdatedAt.After(latest) {
			latest = rec.UpdatedAt
		}
	}
	if !latest.IsZero() {
		return LastActivity{At: latest, Source: ActivityProgramStatus}, nil
	}

	out, err := p.client.BlockFormat(ctx, binding.SessionID, binding.BlockID)
	if err != nil {
		return LastActivity{}, err
	}
	sum := sha256.Sum256([]byte(out))
	fp := hex.EncodeToString(sum[:])
	if rec, found := p.activity.Load(binding.Name); found && rec.Identity == string(binding.BlockID) && rec.Fingerprint == fp {
		return LastActivity{At: rec.At, Source: ActivityFormatPoll}, nil
	}
	now := time.Now().UTC()
	p.activity.Save(binding.Name, ActivityRecord{Identity: string(binding.BlockID), Fingerprint: fp, At: now})
	return LastActivity{At: now, Source: ActivityFormatPoll}, nil
}
