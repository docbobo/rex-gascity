package provider

import (
	"context"
	"fmt"
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
	// readinessEnded is a wait cut short because the pane's program had already
	// exited: the agent ended before its first turn.
	readinessEnded = "ended"
	// readinessPrompt is a wait that ended because the pane's screen showed the
	// start config's ready_prompt_prefix (herdr-core's prompt readiness).
	readinessPrompt = "prompt"
	// readinessPromptBound is a wait that ended on its window because a set
	// ready_prompt_prefix never appeared on screen.
	readinessPromptBound = "prompt-bound"
	// readinessDialog is a wait that ended because the pane's screen showed a
	// startup dialog. The nudge is withheld, not delivered: gc answers the
	// dialog only once start returns, and the turn's Enter would pick the
	// dialog's selected option (herdrcore/lifecycle.py:1078-1096).
	readinessDialog = "dialog"
)

// maxReadyDelay caps ready_delay_ms, mirroring herdr-core READY_WAIT_S
// (herdrcore/lifecycle.py:171,1064).
const maxReadyDelay = 60 * time.Second

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
// and submits the startup nudge. Delivery itself is best effort: no error from
// the wait or the delivery is returned. The one exception is agent liveness: an
// agent that ends before its first turn fails the start, unless the lifecycle is
// one_shot, mirroring herdr-core's first_turn (herdrcore/lifecycle.py:1113-1125).
// It targets the pane Start just launched,
// resolved from launch rather than re-looked-up by name, so a concurrent stop
// and start of the same name cannot receive a stale nudge. A delivery whose
// submit is not confirmed is resent down a bounded ladder, because readiness
// detection and the TUI accepting input are not the same moment; each resend
// clears the draft line first so the text is not duplicated. When every
// attempt is unconfirmed, StartupDeliveryUnconfirmedMetaKey marks the pane;
// a confirmed delivery clears it. A zero launch result means Start found the
// pane already alive and must not deliver a second nudge.
func (p *Provider) deliverStartupNudge(ctx context.Context, name string, launch startResult, cfg Config) error {
	if cfg.Nudge == "" || launch.blockID == "" {
		return nil
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
	outcome, readiness, ended := p.waitForStartupReady(ctx, launch.sessionID, launch.blockID, cfg)
	timing.ReadinessOutcome = outcome
	timing.ReadinessMS = millis(readiness)
	// An agent that ended before its first turn: fail unless the lifecycle is
	// one_shot, and never type a nudge into a dead pane. The timing diagnostic
	// is still written by the defer above.
	if ended {
		if cfg.Lifecycle == lifecycleOneShot {
			return nil
		}
		return fmt.Errorf("%s: the agent exited during startup, before its first turn", name)
	}
	// A startup dialog is gc's to answer once start returns; typing the turn
	// now would press whichever option the dialog has selected. The withhold is
	// recorded, never a failed start, so a healthy pane is not torn down.
	if outcome == readinessDialog {
		p.recordStartupNudgeWithheld(ctx, binding, "the pane shows a startup dialog")
		return nil
	}

	report, err := p.nudgeReportBinding(ctx, binding, cfg.Nudge, NudgeModeImmediate)
	timing.WriteMS = millis(report.Write)
	timing.ConfirmMS = millis(report.Confirm)
	timing.Confirmed = err == nil && report.Confirmed
	if err == nil && report.Confirmed {
		p.clearStartupDeliveryUnconfirmed(ctx, name)
		return nil
	}
	if err != nil {
		p.recordStartupDeliveryUnconfirmed(ctx, binding, report, err)
		return nil
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
		snap, snapErr := p.programStatusSnapshot(ctx, binding)
		if snapErr != nil {
			snap = programStatusSnapshot{}
		}
		if lateTurnVerdict(p.observePane(ctx, binding, snap)) == verdictBusy {
			p.clearStartupDeliveryUnconfirmed(ctx, name)
			return nil
		}
		attempt++
		rt := retryTiming{Attempt: attempt, BackoffMS: millis(backoff)}
		// Clear the still-drafted line before re-typing, mirroring gc's
		// clear+paste+submit resend cycle.
		_ = p.client.SendKeys(ctx, binding.SessionID, binding.BlockID, startupClearKey)
		report, err = p.nudgeReportBinding(ctx, binding, cfg.Nudge, NudgeModeImmediate)
		rt.WriteMS = millis(report.Write)
		rt.ConfirmMS = millis(report.Confirm)
		rt.Confirmed = err == nil && report.Confirmed
		timing.Attempts = append(timing.Attempts, rt)
		if err == nil && report.Confirmed {
			p.clearStartupDeliveryUnconfirmed(ctx, name)
			return nil
		}
		if err != nil {
			break
		}
	}
	p.recordStartupDeliveryUnconfirmed(ctx, binding, report, err)
	return nil
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

// recordStartupNudgeWithheld marks a pane whose startup nudge was withheld, so
// the withhold is observable and gc's queued-nudge backstop can cover the first
// prompt. It reuses the unconfirmed marker: the first turn never reached the
// agent, which is exactly what an operator greps for. Best effort; a metadata
// failure must not turn a live start into a failure.
func (p *Provider) recordStartupNudgeWithheld(ctx context.Context, binding Binding, why string) {
	_ = p.SetMeta(ctx, string(binding.Name), StartupDeliveryUnconfirmedMetaKey, "withheld: "+why)
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
// program_status record, the pane's program ends, the bounded window elapses,
// or the start context is done. It never returns an error: the startup nudge is
// best effort, so a missing readiness signal falls through to delivery. When the
// program reports no program_status at all, only the shorter settle delay is
// spent, so an agent that never emits status is not held for the whole window.
//
// It returns how the wait ended (idle, a window or settle bound, canceled,
// ended, disabled, or a startup dialog), how long it took, and whether the
// agent ended. The caller records the bound and, on an end, fails the start
// unless the lifecycle is one_shot (GCREX-53, GCREX-58); a dialog withholds the
// nudge (GCREX-59).
// readinessBounds are the three times the wait can end on: the ready_delay_ms
// floor, the full window, and the shorter settle delay used before the program
// has reported any status. promptWatch records whether a ready_prompt_prefix
// is set, which makes the prompt the only readiness signal and keeps the full
// window (the wait is not tied to program_status).
type readinessBounds struct {
	delay       time.Duration
	window      time.Duration
	settle      time.Duration
	promptWatch bool
}

// bound is how long the wait may run before it ends on a bound: the full
// window once the program has reported a status (or for a prompt watch), else
// the settle delay. It is widened to the ready_delay_ms floor, so that floor
// can actually elapse before the wait gives up.
func (r readinessBounds) bound(sawRecord bool) time.Duration {
	base := r.settle
	if r.promptWatch || sawRecord {
		base = r.window
	}
	if r.delay > base {
		base = r.delay
	}
	return base
}

// readyDelay is the ready_delay_ms floor, clamped before conversion so a huge
// value cannot overflow a Duration, and zero when a ready_prompt_prefix is set:
// gc sends ready_delay_ms only when there is no prompt to watch for, and
// herdr-core's _shows_ready consults the prompt first (gc v1.5.1
// docs/reference/exec-session-provider.md:228-229; herdrcore/lifecycle.py
// :1059-1064).
func readyDelay(cfg Config) time.Duration {
	if cfg.ReadyPromptPrefix != "" {
		return 0
	}
	ms := cfg.ReadyDelayMS
	if maxMS := int(maxReadyDelay / time.Millisecond); ms > maxMS {
		ms = maxMS
	}
	if ms <= 0 {
		return 0
	}
	return time.Duration(ms) * time.Millisecond
}

// readinessNote decides whether the wait is over and how it ended: the pane's
// prompt when ready_prompt_prefix is set (herdr-core _shows_ready), else an
// idle program_status once the ready_delay_ms floor has passed, else a bound.
// A screen showing a startup dialog is never ready (readinessDialog), in any
// mode, because the first turn must not land in it (herdr-core
// withholds_first_turn; herdrcore/lifecycle.py:1078-1096). startupVerdict
// (panestate.go) makes the decision; this maps it to an outcome and names the
// bound. A failed status read counts as no record. It returns ready=false to
// keep waiting.
func (p *Provider) readinessNote(ctx context.Context, sid rex.SessionID, block rex.BlockID, cfg Config, r readinessBounds, snap programStatusSnapshot, snapErr error, sawRecord bool, elapsed time.Duration) (string, bool) {
	if snapErr != nil {
		snap = programStatusSnapshot{}
	}
	o := p.observePane(ctx, Binding{SessionID: sid, BlockID: block}, snap)
	switch startupVerdict(o, cfg.ReadyPromptPrefix, elapsed >= r.delay, elapsed >= r.bound(sawRecord)) {
	case verdictDialog:
		return readinessDialog, true
	case verdictPromptReady:
		return readinessPrompt, true
	case verdictIdle:
		return readinessIdle, true
	default:
		return p.readinessBound(r, sawRecord, elapsed)
	}
}

// readinessBound names the bound the wait ended on, or reports not-over yet.
func (p *Provider) readinessBound(r readinessBounds, sawRecord bool, elapsed time.Duration) (string, bool) {
	if elapsed < r.bound(sawRecord) {
		return "", false
	}
	switch {
	case r.promptWatch:
		return readinessPromptBound, true
	case sawRecord:
		return readinessWindowBound, true
	default:
		return readinessSettleBound, true
	}
}

func (p *Provider) waitForStartupReady(ctx context.Context, sid rex.SessionID, block rex.BlockID, cfg Config) (string, time.Duration, bool) {
	if p.nudge.startupReadyWindow <= 0 {
		return readinessDisabled, 0, false
	}
	poll := p.nudge.nudgePollInterval
	if poll <= 0 {
		poll = defaultNudgePollInterval
	}
	r := readinessBounds{
		delay:       readyDelay(cfg),
		window:      p.nudge.startupReadyWindow,
		settle:      p.nudge.startupSettleDelay,
		promptWatch: cfg.ReadyPromptPrefix != "",
	}
	start := time.Now()
	sawRecord := false
	for {
		// A pane whose program has ended will never report readiness, so stop
		// the wait at once rather than spending the bound on a dead pane.
		if alive, err := p.paneAlive(ctx, Binding{SessionID: sid, BlockID: block}); err == nil && !alive {
			return readinessEnded, time.Since(start), true
		}
		snap, err := p.programStatusSnapshot(ctx, Binding{SessionID: sid, BlockID: block})
		if err == nil && snap.Present {
			sawRecord = true
		}
		elapsed := time.Since(start)
		if note, ready := p.readinessNote(ctx, sid, block, cfg, r, snap, err, sawRecord, elapsed); ready {
			return note, elapsed, false
		}
		wait := poll
		if remaining := r.bound(sawRecord) - elapsed; wait > remaining {
			wait = remaining
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return readinessCanceled, time.Since(start), false
		case <-timer.C:
		}
	}
}

// paneScreen reads the pane's rendered output, the bounded screen read the
// readiness wait uses for both the ready prompt and the startup-dialog check.
// A read failure is the caller's to fall back from, never a failed start.
func (p *Provider) paneScreen(ctx context.Context, sid rex.SessionID, block rex.BlockID) (string, error) {
	return p.client.BlockFormat(ctx, sid, block)
}
