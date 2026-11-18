package provider

import (
	"context"
	"path/filepath"
	"time"

	"github.com/docbobo/rex-gascity/internal/rex"
)

// Startup delivery: the policy Start applies to cfg.Nudge after the pane is
// launched. It sits on the nudge core in nudge.go (nudgeReportBinding) and
// shares its tunables (nudgeConfig).

// Defaults for the bounded startup readiness wait. The window mirrors gc's
// own defaultReadyProbeTimeout (tmux adapter); the shorter settle delay covers
// a pane program that reports no program_status at all.
const (
	defaultStartupReadyWindow = 15 * time.Second
	defaultStartupSettleDelay = 3 * time.Second
)

// defaultStartupNudgeBackoffs mirrors gc's startupNudgeRetryBackoffs: a submit
// injected right after readiness is observed can still miss the TUI's input
// line, and the whole write+submit cycle is re-run after each delay (gc
// internal/runtime/tmux/adapter.go:1528 at v1.5.1).
var defaultStartupNudgeBackoffs = []time.Duration{
	2 * time.Second,
	4 * time.Second,
	6 * time.Second,
	8 * time.Second,
}

// StartupDeliveryUnconfirmedMetaKey records that a startup nudge was typed but
// its submit was not confirmed within the bounded confirmation window. It is
// cleared when a later startup delivery is confirmed. gc reads only the exit
// code, so this marker is how an operator finds a stranded first prompt.
const StartupDeliveryUnconfirmedMetaKey = "GC_STARTUP_DELIVERY_UNCONFIRMED"

// Readiness outcomes recorded in the startup-timing diagnostic. "idle" and the
// two bound outcomes are the distinction that matters: a start whose pane never
// reports idle is capped by the readiness window (or the shorter settle delay),
// so the bound, not the launch, is where its cost sits.
const (
	readinessIdle        = "idle"
	readinessWindowBound = "window-bound"
	readinessSettleBound = "settle-bound"
	readinessCanceled    = "canceled"
	readinessDisabled    = "disabled"
)

// WithStartupReadiness sets how long Start waits for the pane program to
// report idle before delivering the startup nudge, and the shorter settle
// delay it waits when the program reports no status at all. A non-positive
// value keeps the default.
func WithStartupReadiness(window, settle time.Duration) Option {
	return func(p *Provider) {
		if window > 0 {
			p.nudge.startupReadyWindow = window
		}
		if settle > 0 {
			p.nudge.startupSettleDelay = settle
		}
	}
}

// WithStartupNudgeRetries sets the delays between startup-nudge resends after
// a delivered-but-unconfirmed submit. An empty ladder disables resending; nil
// keeps the default. It exists so tests can shrink the ladder.
func WithStartupNudgeRetries(backoffs []time.Duration) Option {
	return func(p *Provider) {
		p.nudge.startupNudgeBackoffs = backoffs
	}
}

// deliverStartupNudge waits for the launched pane to become ready, then types
// and submits the startup nudge. It is best effort: no error from the wait or
// the delivery is returned to Start. It targets the pane Start just launched,
// resolved from launch rather than re-looked-up by name, so a concurrent stop
// and start of the same name cannot receive a stale nudge. A delivery whose
// submit is not confirmed is resent down a bounded ladder, because readiness
// detection and the TUI accepting input are not the same moment; each resend
// clears the draft line first so the text is not duplicated. When every
// attempt is unconfirmed, StartupDeliveryUnconfirmedMetaKey marks the pane;
// a confirmed delivery clears it. A zero launch result means Start found the
// pane already alive and must not deliver a second nudge.
func (p *Provider) deliverStartupNudge(ctx context.Context, name string, launch startResult, text string) {
	if text == "" || launch.blockID == "" {
		return
	}
	binding := Binding{Name: SessionName(name), SessionID: launch.sessionID, BlockID: launch.blockID}
	timing := startupTiming{
		Session:  string(binding.Name),
		At:       time.Now().UTC(),
		LaunchMS: millis(launch.launchDur),
	}
	// The timing is written for every delivery, confirmed or not: the slow case
	// this exists to attribute is usually a successful start whose readiness
	// wait consumed most of the time, so recording only failures would miss it.
	start := time.Now()
	defer func() {
		timing.TotalMS = millis(time.Since(start)) + timing.LaunchMS
		p.writeStartupTimingDiagnostic(binding, timing)
	}()
	outcome, readiness := p.waitForStartupReady(ctx, launch.sessionID, launch.blockID)
	timing.ReadinessOutcome = outcome
	timing.ReadinessMS = millis(readiness)

	report, err := p.nudgeReportBinding(ctx, binding, text, NudgeModeImmediate)
	timing.WriteMS = millis(report.Write)
	timing.ConfirmMS = millis(report.Confirm)
	timing.Confirmed = err == nil && report.Confirmed
	if err == nil && report.Confirmed {
		p.clearStartupDeliveryUnconfirmed(ctx, name)
		return
	}
	if err != nil {
		p.recordStartupDeliveryUnconfirmed(ctx, binding, report, err)
		return
	}

	attempt := 1
	for _, backoff := range p.nudge.startupNudgeBackoffs {
		if ctx.Err() != nil {
			break
		}
		if !p.sleepStartupBackoff(ctx, backoff) {
			break
		}
		// The turn may have started late: gc checks for a busy pane after
		// the backoff and before resending, so a nudge that landed a
		// moment after the confirmation window is not typed twice.
		if snap, snapErr := p.programStatusSnapshot(ctx, binding); snapErr == nil && (snap.State == "working" || snap.State == "blocked") {
			p.clearStartupDeliveryUnconfirmed(ctx, name)
			return
		}
		attempt++
		rt := retryTiming{Attempt: attempt, BackoffMS: millis(backoff)}
		// Clear the still-drafted line before re-typing, mirroring gc's
		// clear+paste+submit resend cycle.
		_ = p.client.SendKeys(ctx, binding.SessionID, binding.BlockID, startupClearKey)
		report, err = p.nudgeReportBinding(ctx, binding, text, NudgeModeImmediate)
		rt.WriteMS = millis(report.Write)
		rt.ConfirmMS = millis(report.Confirm)
		rt.Confirmed = err == nil && report.Confirmed
		timing.Attempts = append(timing.Attempts, rt)
		if err == nil && report.Confirmed {
			p.clearStartupDeliveryUnconfirmed(ctx, name)
			return
		}
		if err != nil {
			break
		}
	}
	p.recordStartupDeliveryUnconfirmed(ctx, binding, report, err)
}

// startupTiming is the per-stage startup-nudge timing left in
// <stateDir>/diagnostics/<name>-startup-timing.json. gc records only the total
// start_resolved duration; this separates the pane launch, the readiness wait,
// the first nudge write and submit confirmation, and each resend, so a slow
// start can be attributed without a debugger (GCREX-58). Durations are
// milliseconds; readiness_outcome says whether the wait ended on idle or on
// its bound, which decides whether the bound is a lever at all.
type startupTiming struct {
	Session          string        `json:"session"`
	Aliases          []string      `json:"aliases,omitempty"`
	At               time.Time     `json:"at"`
	LaunchMS         int64         `json:"launch_ms"`
	ReadinessMS      int64         `json:"readiness_ms"`
	ReadinessOutcome string        `json:"readiness_outcome"`
	WriteMS          int64         `json:"write_ms"`
	ConfirmMS        int64         `json:"confirm_ms"`
	Confirmed        bool          `json:"confirmed"`
	Attempts         []retryTiming `json:"attempts,omitempty"`
	TotalMS          int64         `json:"total_ms"`
}

// retryTiming is one resend of a delivered-but-unconfirmed startup nudge. The
// primary attempt is the top-level write_ms/confirm_ms; Attempt starts at 2.
type retryTiming struct {
	Attempt   int   `json:"attempt"`
	BackoffMS int64 `json:"backoff_ms"`
	WriteMS   int64 `json:"write_ms"`
	ConfirmMS int64 `json:"confirm_ms"`
	Confirmed bool  `json:"confirmed"`
}

// millis renders a stage duration as whole milliseconds, clamped at zero so a
// clock oddity cannot write a negative duration.
func millis(d time.Duration) int64 {
	if d < 0 {
		return 0
	}
	return d.Milliseconds()
}

// writeStartupTimingDiagnostic leaves the per-stage startup timing in
// <stateDir>/diagnostics/<name>-startup-timing.json. It is best effort and
// never changes the start result; an empty state dir writes nothing.
func (p *Provider) writeStartupTimingDiagnostic(b Binding, t startupTiming) {
	if p.ns.CanonicalDir() == "" {
		return
	}
	for _, a := range b.Aliases {
		t.Aliases = append(t.Aliases, string(a))
	}
	name := unsafeNameChars.ReplaceAllString(string(b.Name), "_")
	_ = writeJSON(filepath.Join(p.ns.CanonicalDir(), "diagnostics", name+"-startup-timing.json"), t)
}

// startupClearKey clears the pane's draft input line before a startup-nudge
// resend. gc's send cycle starts with C-u for the same reason (its tmux
// adapter's sendKeys); Rex key syntax spells it ctrl+u.
const startupClearKey = "ctrl+u"

// clearStartupDeliveryUnconfirmed removes the stranded-startup marker after a
// confirmed delivery. Best effort: a metadata failure must not fail a start
// that already succeeded.
func (p *Provider) clearStartupDeliveryUnconfirmed(ctx context.Context, name string) {
	_ = p.RemoveMeta(ctx, name, StartupDeliveryUnconfirmedMetaKey)
}

// sleepStartupBackoff waits out one resend delay. It reports false when the
// start context is done first, so a canceled start does not sleep through the
// ladder only to record the same unconfirmed delivery later.
func (p *Provider) sleepStartupBackoff(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return ctx.Err() == nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

// recordStartupDeliveryUnconfirmed leaves a durable marker on the pane after a
// startup nudge could not be confirmed. The marker is best effort; a metadata
// write failure must not turn a live start into a failure.
func (p *Provider) recordStartupDeliveryUnconfirmed(ctx context.Context, binding Binding, report NudgeReport, err error) {
	detail := "delivered but not confirmed"
	if err != nil {
		detail = "delivery failed: " + err.Error()
	} else if report.State != "" {
		detail = "delivered but not confirmed (state=" + report.State + ")"
		if report.Kind != "" {
			detail += ", kind=" + report.Kind
		}
		detail += ")"
	}
	_ = p.SetMeta(ctx, string(binding.Name), StartupDeliveryUnconfirmedMetaKey, detail)
}

// waitForStartupReady blocks until the pane program reports an idle
// program_status record, the bounded window elapses, or the start context is
// done. It never returns an error: the startup nudge is best effort, so a
// missing readiness signal falls through to delivery. When the program
// reports no program_status at all, only the shorter settle delay is spent,
// so an agent that never emits status is not held for the whole window.
//
// It returns how the wait ended (idle, a window or settle bound, canceled, or
// disabled) and how long it took, so the caller can record whether the bound
// was the cost (GCREX-58).
func (p *Provider) waitForStartupReady(ctx context.Context, sid rex.SessionID, block rex.BlockID) (string, time.Duration) {
	window := p.nudge.startupReadyWindow
	if window <= 0 {
		return readinessDisabled, 0
	}
	poll := p.nudge.nudgePollInterval
	if poll <= 0 {
		poll = defaultNudgePollInterval
	}
	settle := p.nudge.startupSettleDelay
	start := time.Now()
	sawRecord := false
	for {
		snap, err := p.programStatusSnapshot(ctx, Binding{SessionID: sid, BlockID: block})
		if err == nil && snap.Present {
			sawRecord = true
			if snap.State == "idle" {
				return readinessIdle, time.Since(start)
			}
		}
		elapsed := time.Since(start)
		// The full window applies once the program has reported a status;
		// before that the shorter settle delay bounds the wait.
		bound := window
		if !sawRecord {
			bound = settle
		}
		if elapsed >= bound {
			if sawRecord {
				return readinessWindowBound, elapsed
			}
			return readinessSettleBound, elapsed
		}
		wait := poll
		if remaining := bound - elapsed; wait > remaining {
			wait = remaining
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return readinessCanceled, time.Since(start)
		case <-timer.C:
		}
	}
}
