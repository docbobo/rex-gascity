package provider

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"
	"unicode/utf8"

	"github.com/docbobo/rex-gascity/internal/rex"
)

// Startup delivery: the policy Start applies to cfg.Nudge after the pane is
// launched. It waits for readiness, then hands the text to the shared pane
// delivery ladder in pane_delivery.go (deliverPaneText), which it also uses
// for the resend attempts; it shares the nudge tunables (nudgeConfig).

// Defaults for the bounded startup readiness wait. The window mirrors gc's
// own defaultReadyProbeTimeout (tmux adapter); the shorter settle delay covers
// a pane program that reports no program_status at all.
const (
	defaultStartupReadyWindow = 15 * time.Second
	defaultStartupSettleDelay = 3 * time.Second
)

// defaultStartupDialogClearWindow is the clear window's default, the same 15 s
// as the readiness window (#205, REQ-002).
const defaultStartupDialogClearWindow = defaultStartupReadyWindow

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
	// startup dialog. After the clear wait (#205) it means the last read still
	// showed the dialog, and the seed was not typed, and the turn's Enter would
	// pick the dialog's selected option (herdrcore/lifecycle.py:1078-1096).
	readinessDialog = "dialog"
	// readinessDialogCleared is a wait that saw a startup dialog, then had a
	// poll with a clear screen tail and a ready verdict (idle past the floor,
	// or the configured prompt) before the clear window ended. The seed is
	// then delivered through the ladder (#205, REQ-003).
	readinessDialogCleared = "dialog-cleared"
	// readinessDialogUnresolved is a wait that saw a startup dialog and ended
	// at the clear window's deadline on a read that was clear but not ready,
	// or that failed. The seed is not typed (REQ-004 (b)).
	readinessDialogUnresolved = "dialog-unresolved"
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

// WithStartupDialogClear sets the clear window. Once the startup wait has seen
// a dialog, Start keeps polling, never typing, until a poll shows a clear
// screen and a ready verdict, or until the window, measured from the first
// sighting, ends. d <= 0 disables the clear wait: the first sighting leaves the
// seed undelivered at once, as before #205.
func WithStartupDialogClear(d time.Duration) Option {
	return func(p *Provider) {
		p.nudge.startupDialogClear = d
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
	// The launch identity the ladder's gates read: the pool-claim probe needs
	// the seat's gc session, binary and work dir, as a stored binding has them
	// (newBinding, provider.go).
	binding := Binding{
		Name:           SessionName(name),
		SessionID:      launch.sessionID,
		BlockID:        launch.blockID,
		GCBin:          gcBinBasename(launch.env),
		WorkDir:        cfg.WorkDir,
		ClaimSessionID: launch.env["GC_SESSION_ID"],
	}
	timing := startupTiming{
		Session:  string(binding.Name),
		At:       time.Now().UTC(),
		LaunchMS: millis(launch.launchDur),
	}
	// The timing is written for every delivery, confirmed or not: the slow case
	// this exists to attribute is usually a successful start whose readiness
	// wait consumed most of the time, so recording only failures would miss it.
	w := p.newStartupDialogWatch(cfg)
	start := time.Now()
	defer func() {
		w.record(&timing)
		timing.TotalMS = millis(time.Since(start)) + timing.LaunchMS
		p.writeStartupTimingDiagnostic(binding, timing)
	}()
	gate := startupDialogGate{p: p, b: binding, w: w}
	outcome, ended := p.startupReadiness(ctx, binding, cfg, w, start)
	for {
		timing.ReadinessOutcome = outcome
		// readiness_ms covers the readiness wait and any clear wait.
		timing.ReadinessMS = millis(time.Since(start))
		if ended {
			if cfg.Lifecycle == lifecycleOneShot {
				return nil
			}
			return fmt.Errorf("%s: the agent exited during startup, before its first turn", name)
		}
		switch outcome {
		case readinessDialog:
			p.recordStartupNudgeWithheld(ctx, binding, startupDialogShownWhy(w.name))
			return nil
		case readinessDialogUnresolved:
			p.recordStartupNudgeWithheld(ctx, binding, startupDialogUnresolvedWhy(w.name))
			return nil
		}
		res := p.deliverPaneText(ctx, paneTextRequest{
			Binding:  binding,
			Text:     cfg.Nudge,
			Mode:     NudgeModeImmediate,
			Backoffs: p.nudge.startupNudgeBackoffs,
			ClearKey: startupClearKey,
			Gate:     gate,
		})
		p.recordStartupAttempts(&timing, res)
		// A pre-submit or pre-resend stop: no submit key follows and no
		// further seed is typed, so the last seed written stays typed but
		// unsubmitted. A pre-resend stop can come before or after the clear
		// key; the first seed was already written and submitted either way.
		// Record it as a delivery stop, never a failed start (REQ-001). The
		// stop is handled first because the gate reads under the submit key's
		// own context, and a resend's read runs under the parent context: a
		// parent cancel can arrive with the stop, and the stop keeps the
		// readiness outcome the wait ended on (REQ-001 (b), (c)).
		if res.GateStop != nil && res.GateStop.Point != gatePreWrite {
			p.finishStartupDelivery(ctx, name, binding, res)
			return nil
		}
		// A cancel before any input is readiness that never finished. A cancel
		// after the write is not: the ladder already delivered, so the
		// diagnostic keeps the readiness outcome it reached (REQ-002).
		if errors.Is(res.Err, errBeforeTyping) {
			timing.ReadinessOutcome = readinessCanceled
		}
		if res.GateStop != nil {
			outcome, ended = p.afterPreWriteStop(ctx, binding, cfg, w, start, res.GateStop)
			continue
		}
		p.finishStartupDelivery(ctx, name, binding, res)
		return nil
	}
}

// recordStartupAttempts copies the shared ladder's per-attempt timing into the
// startup diagnostic. The first attempt is the top-level write/confirm pair;
// each resend is a retry row.
func (p *Provider) recordStartupAttempts(timing *startupTiming, res paneTextResult) {
	for i, a := range res.Attempts {
		if i == 0 {
			timing.WriteMS = millis(a.Report.Write)
			timing.ConfirmMS = millis(a.Report.Confirm)
			timing.Confirmed = a.Err == nil && a.Report.Confirmed
			continue
		}
		timing.Attempts = append(timing.Attempts, retryTiming{
			Attempt:   a.Attempt,
			BackoffMS: millis(a.Backoff),
			WriteMS:   millis(a.Report.Write),
			ConfirmMS: millis(a.Report.Confirm),
			Confirmed: a.Err == nil && a.Report.Confirmed,
		})
	}
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
	// Dialog evidence, written only when this start saw a startup dialog
	// (#205, REQ-006): the matched dialog's name, the screen tail the decision
	// read (capped at startupDialogScreenCap), the gate that saw it (empty for
	// the readiness wait) and "seed" when the match may be the seed's echo.
	// DialogScreen is raw pane text and can hold seed text after a write; it
	// stays in this private diagnostic.
	Dialog        string `json:"dialog,omitempty"`
	DialogScreen  string `json:"dialog_screen,omitempty"`
	DialogGate    string `json:"dialog_gate,omitempty"`
	DialogSuspect string `json:"dialog_suspect,omitempty"`
	TotalMS       int64  `json:"total_ms"`
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

// startupSecretReadWhy is the withhold reason a secret read records on the
// startup marker: "withheld: the pane is reading a secret".
const startupSecretReadWhy = "the pane is reading a secret"

// startupClearKey clears the pane's draft input line before a startup-nudge
// resend. gc's send cycle starts with C-u for the same reason (its tmux
// adapter's sendKeys); Rex key syntax spells it ctrl+u.
const startupClearKey = "ctrl+u"

// startupDialogScreenCap caps the dialog_screen evidence. The cut keeps the
// last bytes, where the live screen is (REQ-006).
const startupDialogScreenCap = 4096

// startupDialogWatch is one start's record of the startup dialogs it saw. One
// watch spans the readiness wait, any clear wait and the startup dialog gate,
// so they share one clock and one record. The clock starts at the first
// sighting, that is, when the read that first showed a dialog returned
// (REQ-002). The evidence is the most recent sighting's (REQ-006); its screen
// is raw pane text and may hold seed text after a write, so it goes to the
// private diagnostic only, never to a marker, a NudgeReport or a log line.
type startupDialogWatch struct {
	window time.Duration // the clear window; <= 0 disables the clear wait
	poll   time.Duration // the nudge poll interval: the hard stop's margin
	prefix string        // ready_prompt_prefix; "" selects status mode

	seen     bool
	deadline time.Time // first sighting + window
	hardStop time.Time // deadline + poll: the local bound on Rex calls

	name    string
	screen  string
	gate    string
	suspect string
	seed    string // the startup nudge, for the seed-ambiguous check
}

// newStartupDialogWatch starts a start's watch with the configured window.
func (p *Provider) newStartupDialogWatch(cfg Config) *startupDialogWatch {
	return &startupDialogWatch{
		window: p.nudge.startupDialogClear,
		poll:   p.startupPollInterval(),
		prefix: cfg.ReadyPromptPrefix,
		seed:   cfg.Nudge,
	}
}

// sight records a dialog that a read returning at at showed. The first
// sighting starts the clear window; later ones only refresh the evidence. A
// failed read is never a sighting.
func (w *startupDialogWatch) sight(at time.Time, name, screen, gate, suspect string) {
	if !w.seen {
		w.seen = true
		w.deadline = at.Add(max(w.window, 0))
		w.hardStop = w.deadline.Add(w.poll)
	}
	w.name, w.screen, w.gate, w.suspect = name, capTail(screen, startupDialogScreenCap), gate, suspect
}

// pastDeadline reports whether a read that returned at at is at or after the
// clear window's deadline.
func (w *startupDialogWatch) pastDeadline(at time.Time) bool {
	return w.seen && !at.Before(w.deadline)
}

// capped is the context the Rex calls run under until a startup delivery's
// pre-write gate has a clear result: the parent before any sighting, else the
// parent cut at the hard stop (REQ-002). A call cut off there is a failed read.
func (w *startupDialogWatch) capped(parent context.Context) (context.Context, context.CancelFunc) {
	if !w.seen {
		return parent, func() {}
	}
	return context.WithDeadline(parent, w.hardStop)
}

// startupPollInterval is the poll interval of the startup waits.
func (p *Provider) startupPollInterval() time.Duration {
	if p.nudge.nudgePollInterval > 0 {
		return p.nudge.nudgePollInterval
	}
	return defaultNudgePollInterval
}

// record copies the most recent sighting into the timing diagnostic. A start
// that saw no dialog writes none of the fields.
func (w *startupDialogWatch) record(t *startupTiming) {
	if !w.seen {
		return
	}
	t.Dialog, t.DialogScreen, t.DialogGate, t.DialogSuspect = w.name, w.screen, w.gate, w.suspect
}

// capTail keeps the last n bytes of s, moved forward to a rune start so the
// cut never splits a UTF-8 sequence.
func capTail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	cut := len(s) - n
	for cut < len(s) && !utf8.RuneStart(s[cut]) {
		cut++
	}
	return s[cut:]
}

// startupDialogShownWhy is the withhold reason when the last read showed a
// dialog: "withheld: the pane shows a startup dialog (<name>)".
func startupDialogShownWhy(name string) string {
	return "the pane shows a startup dialog (" + name + ")"
}

// startupDialogUnresolvedWhy is the withhold reason for dialog-unresolved. It
// names the last sighting and does not claim the dialog is still visible.
func startupDialogUnresolvedWhy(name string) string {
	return "a startup dialog was seen (" + name + "); the pane was not clear and ready when the clear window ended"
}

// clearStartupDeliveryUnconfirmed removes the stranded-startup marker after a
// confirmed delivery. Best effort: a metadata failure must not fail a start
// that already succeeded.
func (p *Provider) clearStartupDeliveryUnconfirmed(ctx context.Context, name string) {
	_ = p.RemoveMeta(ctx, name, StartupDeliveryUnconfirmedMetaKey)
}

// recordStartupNudgeWithheld marks a pane whose startup seed was not typed, so
// the withhold is observable. It reuses the unconfirmed marker: the first turn
// never reached the agent, which is what an operator greps for. For a named
// session this marker is the only signal: gc stamps primed_at from its routing
// plan and does not read the marker, and gc's queued-nudge backstop covers pool
// slots only (gc edge cmd/gc/idle_nudge.go:88-136). Best effort; a metadata
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

// Startup dialog gate points (REQ-001).
const (
	gatePreWrite  = "pre-write"
	gatePreSubmit = "pre-submit"
	gatePreResend = "pre-resend"
)

// startupDialogSuspectSeed marks a gate match the seed's own echo may explain.
const startupDialogSuspectSeed = "seed"

// startupGateStop is a startup dialog gate's stop: no input followed. Dialog
// reports that the read showed a dialog; false means the read failed after
// this start had seen one. Name is the dialog reported: this read's, or the
// last sighting's on a failed read. At is when the read returned.
type startupGateStop struct {
	Point  string
	Dialog bool
	Name   string
	// Suspect marks a match after a write that the seed's own text would
	// produce (REQ-005).
	Suspect bool
	At      time.Time
}

func (s *startupGateStop) Error() string {
	return "startup dialog gate " + s.Point + ": " + s.reason()
}

// reason is the marker's <reason> (REQ-001).
func (s *startupGateStop) reason() string {
	switch {
	case !s.Dialog:
		return "the screen could not be read after a startup dialog was seen (" + s.Name + ")"
	case s.Suspect:
		return "a startup dialog may have appeared (" + s.Name + "); the seed contains its marker text"
	default:
		return "a startup dialog appeared (" + s.Name + ")"
	}
}

// startupDialogGate is the startup-only dialog gate (REQ-001). Before an input
// call of a startup delivery it reads the screen tail and stops the ladder on
// a startup dialog, or on a failed read once this start has seen one. Before
// any sighting a failed read fails open, as status mode does. It is a snapshot
// check: the read and the input call after it are separate Rex calls, so a
// dialog that draws between them is not caught.
type startupDialogGate struct {
	p *Provider
	b Binding
	w *startupDialogWatch
}

func (g startupDialogGate) capped(parent context.Context) (context.Context, context.CancelFunc) {
	return g.w.capped(parent)
}

// check reads the screen tail before the input call at point. typed reports
// that seed text may already be on screen (after the first write): a match the
// seed alone would produce is then seed-ambiguous. It still stops the ladder,
// because the gate cannot tell a real dialog from the seed's echo and must not
// bypass a real one (REQ-005).
func (g startupDialogGate) check(ctx context.Context, point string, typed bool) *startupGateStop {
	screen, err := g.p.paneScreen(ctx, g.b.SessionID, g.b.BlockID)
	at := time.Now()
	if err != nil {
		if !g.w.seen {
			return nil
		}
		return &startupGateStop{Point: point, Name: g.w.name, At: at}
	}
	tail := startupScreenTail(screen)
	if name, ok := matchedStartupDialog(tail, g.w.prefix); ok {
		suspect := typed && seedEchoes(g.w.seed, g.w.prefix, name)
		tag := ""
		if suspect {
			tag = startupDialogSuspectSeed
		}
		g.w.sight(at, name, tail, point, tag)
		return &startupGateStop{Point: point, Dialog: true, Name: name, Suspect: suspect, At: at}
	}
	// A clear reply that returned at or after the hard stop is stale: the read
	// was cut off there, and the CLI can decode a reply after its command
	// ends. Treat it as a failed read, so the ladder types nothing (REQ-002).
	// The hard stop bounds only the pre-write gate's reads ("resend" runs under
	// ctx), so the check applies there alone.
	if point == gatePreWrite && g.w.seen && !at.Before(g.w.hardStop) {
		return &startupGateStop{Point: point, Name: g.w.name, At: at}
	}
	return nil
}

// seedEchoes reports whether the seed's own text matches the named dialog, so
// a gate match after a write may be the seed's echo (REQ-005).
func seedEchoes(seed, prefix, name string) bool {
	got, ok := matchedStartupDialog(seed, prefix)
	return ok && got == name
}

// recordStartupGateStop marks a startup delivery that a dialog gate stopped
// after the write. Best effort; it never fails the start, and the readiness
// outcome stays as the wait left it (REQ-001).
func (p *Provider) recordStartupGateStop(ctx context.Context, binding Binding, stop *startupGateStop) {
	detail := "delivery failed: the seed was typed but not submitted, " + stop.reason()
	if stop.Point == gatePreResend {
		detail = "delivered but not confirmed: " + stop.reason() + " before a resend"
	}
	_ = p.SetMeta(ctx, string(binding.Name), StartupDeliveryUnconfirmedMetaKey, detail)
}

// afterPreWriteStop is the wait's next step after the pre-write gate blocked:
// the gate is part of the wait (REQ-001 (a), REQ-004 (c)). With the clear wait
// disabled, or at or after the deadline (a read cut off by the hard stop is
// past it), the gate's read ends the wait: a dialog gives dialog, a failed
// read dialog-unresolved. Before the deadline the wait polls again until the
// same deadline.
func (p *Provider) afterPreWriteStop(ctx context.Context, b Binding, cfg Config, w *startupDialogWatch, readyStart time.Time, stop *startupGateStop) (string, bool) {
	if w.window <= 0 || w.pastDeadline(stop.At) {
		if stop.Dialog {
			return readinessDialog, false
		}
		return readinessDialogUnresolved, false
	}
	return p.waitForDialogClear(ctx, b, cfg, w, readyStart)
}

// finishStartupDelivery maps a ladder result that got past the pre-write gate
// to the startup marker. It never fails the start.
func (p *Provider) finishStartupDelivery(ctx context.Context, name string, binding Binding, res paneTextResult) {
	switch {
	case res.GateStop != nil:
		// A pre-submit or pre-resend stop (the caller handles pre-write).
		p.recordStartupGateStop(ctx, binding, res.GateStop)
	case res.LateTurn:
		// The turn started after the confirmation window: the earlier submit
		// landed, so a resend would enqueue a second copy.
		p.clearStartupDeliveryUnconfirmed(ctx, name)
	case res.Withheld:
		// A pane reading a secret got nothing typed (#48).
		p.recordStartupNudgeWithheld(ctx, binding, startupSecretReadWhy)
	case res.Err == nil && res.Report.Confirmed:
		p.clearStartupDeliveryUnconfirmed(ctx, name)
	default:
		p.recordStartupDeliveryUnconfirmed(ctx, binding, res.Report, res.Err)
	}
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
// nudge (GCREX-59) and ends this wait at its first sighting, so the caller then
// runs the clear wait (#205).
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
func (p *Provider) readinessNote(ctx context.Context, sid rex.SessionID, block rex.BlockID, cfg Config, r readinessBounds, w *startupDialogWatch, snap programStatusSnapshot, snapErr error, sawRecord bool, elapsed time.Duration) (string, bool) {
	if snapErr != nil {
		snap = programStatusSnapshot{}
	}
	o := p.observePane(ctx, Binding{SessionID: sid, BlockID: block}, snap)
	switch startupVerdict(o, cfg.ReadyPromptPrefix, elapsed >= r.delay, elapsed >= r.bound(sawRecord)) {
	case verdictDialog:
		// The window starts when the read returned, not when the verdict is
		// classified (REQ-002). startupDialog has read the memo, so its time is
		// set.
		if name, screen, ok, err := startupDialog(o, cfg.ReadyPromptPrefix); err == nil && ok {
			w.sight(o.screen.at, name, screen, "", "")
		}
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

func (p *Provider) waitForStartupReady(ctx context.Context, sid rex.SessionID, block rex.BlockID, cfg Config, w *startupDialogWatch) (string, time.Duration, bool) {
	if p.nudge.startupReadyWindow <= 0 {
		return readinessDisabled, 0, false
	}
	poll := p.startupPollInterval()
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
		if note, ready := p.readinessNote(ctx, sid, block, cfg, r, w, snap, err, sawRecord, elapsed); ready {
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

// startupReadiness runs the readiness wait and, after a first dialog sighting
// with the clear wait enabled, the clear wait. It returns the wait's outcome
// and whether the pane's program ended.
func (p *Provider) startupReadiness(ctx context.Context, b Binding, cfg Config, w *startupDialogWatch, readyStart time.Time) (string, bool) {
	outcome, _, ended := p.waitForStartupReady(ctx, b.SessionID, b.BlockID, cfg, w)
	if outcome == readinessDialog && w.window > 0 {
		return p.waitForDialogClear(ctx, b, cfg, w, readyStart)
	}
	return outcome, ended
}

// waitForDialogClear runs once this start has seen a startup dialog. It polls
// at the nudge poll interval until a poll has a clear read and a positive ready
// verdict (dialog-cleared), or until the deadline poll, the first poll whose
// screen read returns at or after the deadline, ends the wait undelivered
// (REQ-002, REQ-003, REQ-004). The bound fallbacks never deliver here. A
// cancelled ctx gives canceled and an ended pane ended, as in the readiness
// wait.
func (p *Provider) waitForDialogClear(ctx context.Context, b Binding, cfg Config, w *startupDialogWatch, readyStart time.Time) (string, bool) {
	floor := readyDelay(cfg)
	for {
		if ctx.Err() != nil {
			return readinessCanceled, false
		}
		if outcome, done, ended := p.dialogClearPoll(ctx, b, cfg, w, time.Since(readyStart) >= floor); done {
			return outcome, ended
		}
		timer := time.NewTimer(min(w.poll, max(time.Until(w.deadline), 0)))
		select {
		case <-ctx.Done():
			timer.Stop()
			return readinessCanceled, false
		case <-timer.C:
		}
	}
}

// dialogClearPoll is one poll of the clear wait. Its Rex calls run under the
// hard stop, and it reads the screen tail in both modes, whatever the program
// status, so the deadline poll always has a read (REQ-003). The clear-and-ready
// check runs before the deadline check, so the deadline poll can still deliver
// (REQ-004).
func (p *Provider) dialogClearPoll(ctx context.Context, b Binding, cfg Config, w *startupDialogWatch, pastFloor bool) (outcome string, done, ended bool) {
	pctx, cancel := w.capped(ctx)
	defer cancel()
	if alive, err := p.paneAlive(pctx, b); err == nil && !alive {
		return readinessEnded, true, true
	}
	snap, err := p.programStatusSnapshot(pctx, b)
	if err != nil {
		snap = programStatusSnapshot{}
	}
	o := p.observePane(pctx, b, snap)
	verdict := startupVerdict(o, cfg.ReadyPromptPrefix, pastFloor, false)
	name, screen, dialog, readErr := startupDialog(o, cfg.ReadyPromptPrefix)
	// The deadline is decided by when the screen read returned (REQ-002), so a
	// delay after the read cannot end the wait early or extend it.
	at := o.screen.at
	if ctx.Err() != nil {
		return readinessCanceled, true, false
	}
	if dialog {
		w.sight(at, name, screen, "", "")
	}
	// A failed read is never a clear read, even when status mode's verdict
	// failed open to idle.
	if readErr == nil && !dialog && (verdict == verdictIdle || verdict == verdictPromptReady) {
		return readinessDialogCleared, true, false
	}
	if w.pastDeadline(at) {
		if dialog {
			return readinessDialog, true, false
		}
		return readinessDialogUnresolved, true, false
	}
	return "", false, false
}

// paneScreen reads the pane's rendered output, the bounded screen read the
// readiness wait uses for both the ready prompt and the startup-dialog check.
// A read failure is the caller's to fall back from, never a failed start.
func (p *Provider) paneScreen(ctx context.Context, sid rex.SessionID, block rex.BlockID) (string, error) {
	return p.client.BlockFormat(ctx, sid, block)
}
