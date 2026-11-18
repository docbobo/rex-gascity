// Package provider is the Rex-backed Gas City session provider core. It maps
// gc sessions (city/rig/agent) onto the Rex hierarchy (session/tab/pane) and
// will be wrapped by the RPP v0 op dispatcher in cmd/gc-runtime-rex.
package provider

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/docbobo/rex-gascity/internal/rex"
)

// ErrSessionNotFound reports that an input op found no live agent for the
// named session: the pane is gone, or its program has exited. The RPP
// dispatcher maps it to a failed op with a clear message.
var ErrSessionNotFound = errors.New("session not found")

// ErrAlreadyRunning reports that StartNew found a live pane for the session.
// The RPP dispatcher maps it to gc's "already running" start collision.
var ErrAlreadyRunning = errors.New("provider: already running")

// submitKey submits typed input; interruptKey is the soft interrupt keystroke.
// Rex has no signal API, so both are keys.
const (
	submitKey    = "enter"
	interruptKey = "ctrl+c"
)

// Config is the launch configuration the RPP `start` op carries as JSON.
type Config struct {
	Argv    []string
	WorkDir string
	// Env is the per-session environment gc ships in the start config. Rex
	// launch options carry no env, so Start applies it by prefixing the launch
	// argv with `env K=V ...` (see launchCommand).
	Env map[string]string
	// PreStart is an ordered list of shell commands run before the agent pane
	// launches, with cwd set to WorkDir and the start-config env applied.
	// A non-zero exit or start error aborts the launch and leaves no seeded
	// identity or binding behind. An empty or absent slice is a no-op.
	PreStart []string
	// Nudge is the text delivered to the pane after a successful launch,
	// typed in and submitted with Enter so the agent gets its first prompt
	// immediately. Delivery is best effort: a transport failure does not fail
	// the start. An empty string is a no-op.
	Nudge string
}

// Provider drives Rex for one gc city.
type Provider struct {
	city   string
	client rex.Client
	binder *Binder

	bindings BindingStore
	activity ActivityStore
	meta     MetaStore

	// stateDir is the canonical root state dir; it holds the cross-process
	// lock file. Empty means no cross-process locking.
	stateDir string

	// sessionLabels are the accepted Rex session labels, canonical first. New
	// sessions use the first label; lookups and binding revalidation accept
	// all of them so a legacy namespace still resolves.
	sessionLabels []string

	// nudgeConfirmWindow bounds how long Nudge waits for a turn start before
	// reporting the delivery unconfirmed. nudgePollInterval spaces the
	// observations within that window.
	nudgeConfirmWindow time.Duration
	nudgePollInterval  time.Duration
	nudgeSubmitBudget  time.Duration

	// startupReadyWindow bounds how long Start waits for the pane program to
	// report idle before delivering the startup nudge.
	// startupSettleDelay is the shorter bound Start applies when the program
	// reports no program_status at all, so an agent that never emits one is
	// not held for the whole window.
	startupReadyWindow time.Duration
	startupSettleDelay time.Duration

	// startupNudgeBackoffs is the ladder of delays between startup-nudge
	// resends after a delivered-but-unconfirmed submit. An empty ladder
	// disables resending.
	startupNudgeBackoffs []time.Duration
}

// Defaults for the bounded nudge confirmation window.
const (
	defaultNudgeConfirmWindow = 3 * time.Second
	defaultNudgePollInterval  = 100 * time.Millisecond
)

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

// Option configures a Provider.
type Option func(*Provider)

// WithBindingStore sets the binding store.
func WithBindingStore(s BindingStore) Option { return func(p *Provider) { p.bindings = s } }

// WithActivityStore sets the activity store.
func WithActivityStore(s ActivityStore) Option { return func(p *Provider) { p.activity = s } }

// WithMetaStore sets the metadata store.
func WithMetaStore(s MetaStore) Option { return func(p *Provider) { p.meta = s } }

// WithNudgeConfirmation sets how long Nudge waits for a turn start and how
// often it observes the pane while waiting. A window <= 0 takes one
// observation and returns immediately.
func WithNudgeConfirmation(window, interval time.Duration) Option {
	return func(p *Provider) {
		p.nudgeConfirmWindow = window
		if interval > 0 {
			p.nudgePollInterval = interval
		}
	}
}

// WithStartupReadiness sets how long Start waits for the pane program to
// report idle before delivering the startup nudge, and the shorter settle
// delay it waits when the program reports no status at all. A non-positive
// value keeps the default.
func WithStartupReadiness(window, settle time.Duration) Option {
	return func(p *Provider) {
		if window > 0 {
			p.startupReadyWindow = window
		}
		if settle > 0 {
			p.startupSettleDelay = settle
		}
	}
}

// WithStartupNudgeRetries sets the delays between startup-nudge resends after
// a delivered-but-unconfirmed submit. An empty ladder disables resending; nil
// keeps the default. It exists so tests can shrink the ladder.
func WithStartupNudgeRetries(backoffs []time.Duration) Option {
	return func(p *Provider) {
		p.startupNudgeBackoffs = backoffs
	}
}

// WithStateDir keeps bindings, activity and metadata on disk under
// dir/bindings, dir/activity and dir/meta.
func WithStateDir(dir string) Option {
	return WithStateDirs(dir)
}

// WithStateDirs keeps the sidecar stores in several city-namespace state dirs.
// The first is canonical: writes go there and its lock guards mutations.
// Reads fall back through the rest in order, and deletes cover every dir, so a
// legacy record under an incidental namespace still resolves and is cleaned
// up.
func WithStateDirs(dirs ...string) Option {
	return func(p *Provider) {
		if len(dirs) == 0 {
			return
		}
		bindings := make([]BindingStore, 0, len(dirs))
		activities := make([]ActivityStore, 0, len(dirs))
		metas := make([]MetaStore, 0, len(dirs))
		for _, dir := range dirs {
			bindings = append(bindings, NewFileStore(filepath.Join(dir, "bindings")))
			activities = append(activities, NewFileActivityStore(filepath.Join(dir, "activity")))
			metas = append(metas, NewFileMetaStore(filepath.Join(dir, "meta")))
		}
		p.stateDir = dirs[0]
		p.bindings = newCompositeBindingStore(bindings...)
		p.activity = newCompositeActivityStore(activities...)
		p.meta = newCompositeMetaStore(metas...)
	}
}

// WithSessionLabels sets the accepted Rex session labels, canonical first.
// The default is gc:<city>. Callers pass the labels for every namespace they
// read so a binding written under an older label still revalidates.
func WithSessionLabels(labels ...string) Option {
	return func(p *Provider) { p.sessionLabels = normalizeSessionLabels(labels) }
}

// WithCityNamespaces configures the provider for a city from its ordered
// namespace candidates (canonical first), resolving each one's state dir and
// Rex session label. It is the constructor the RPP dispatcher uses to keep
// start and every lookup op on the same store regardless of which incidental
// GC_CITY_PATH/GC_CITY the caller carries.
func WithCityNamespaces(cities []string) Option {
	return func(p *Provider) {
		dirs, err := StateDirs(cities)
		if err != nil {
			// Without a resolvable state dir the provider stays in memory.
			// run() resolves the same dirs and reports the error before any op.
			return
		}
		WithStateDirs(dirs...)(p)
		WithSessionLabels(SessionLabels(cities)...)(p)
	}
}

// New builds a Provider for city, driving client. Without options all state is
// held in memory.
func New(city string, client rex.Client, opts ...Option) *Provider {
	p := &Provider{
		city:                 city,
		client:               client,
		bindings:             NewMemoryStore(),
		activity:             NewMemoryActivityStore(),
		meta:                 NewMemoryMetaStore(),
		nudgeConfirmWindow:   defaultNudgeConfirmWindow,
		nudgePollInterval:    defaultNudgePollInterval,
		nudgeSubmitBudget:    defaultNudgeSubmitBudget,
		startupReadyWindow:   defaultStartupReadyWindow,
		startupSettleDelay:   defaultStartupSettleDelay,
		startupNudgeBackoffs: defaultStartupNudgeBackoffs,
	}
	for _, opt := range opts {
		opt(p)
	}
	labels := p.sessionLabels
	if len(labels) == 0 {
		labels = []string{rexSessionLabel(city)}
	}
	p.binder = NewBinderWithLabels(client, p.bindings, labels)
	return p
}

// Binding returns the stored binding that answers to name (its primary name
// or one of its aliases) without contacting Rex. It is the dispatcher-facing
// lookup; Resolve-based ops re-validate it against Rex.
func (p *Provider) Binding(name string) (Binding, bool) {
	cands := p.binder.lookup(SessionName(name))
	if len(cands) == 0 {
		return Binding{}, false
	}
	return cands[0], true
}

// Start creates the city session, the rig's tab, and the agent's pane, and
// records the pane's binding. It is idempotent: a live pane is left alone, and
// a pane whose agent has died is closed before a new one is launched.
//
// The state lock covers only the state mutations and the launch. The startup
// nudge is delivered after Start releases the lock, so its bounded readiness
// wait cannot stall every other Start or Stop in the city.
func (p *Provider) Start(ctx context.Context, name string, cfg Config) error {
	var launch startResult
	err := p.withStateLock(ctx, func() error {
		var startErr error
		launch, startErr = p.start(ctx, name, cfg)
		return startErr
	})
	if err != nil {
		return err
	}
	p.deliverStartupNudge(ctx, name, launch, cfg.Nudge)
	return nil
}

// StartNew starts the agent like Start, but fails with ErrAlreadyRunning when
// its pane is already live. The liveness check and the launch run under one
// state lock, so of two concurrent StartNew calls for one name sharing a state
// dir, exactly one launches and the other sees ErrAlreadyRunning. The startup
// nudge is delivered after the lock is released, like Start.
func (p *Provider) StartNew(ctx context.Context, name string, cfg Config) error {
	var launch startResult
	err := p.withStateLock(ctx, func() error {
		running, err := p.IsRunning(ctx, name)
		if err != nil {
			return err
		}
		if running {
			return fmt.Errorf("%w: %s", ErrAlreadyRunning, name)
		}
		var startErr error
		launch, startErr = p.start(ctx, name, cfg)
		return startErr
	})
	if err != nil {
		return err
	}
	p.deliverStartupNudge(ctx, name, launch, cfg.Nudge)
	return nil
}

// startResult identifies the pane a start launched. A zero result means the
// call was idempotent and found the pane already alive.
type startResult struct {
	sessionID rex.SessionID
	blockID   rex.BlockID
}

func (p *Provider) start(ctx context.Context, name string, cfg Config) (launch startResult, err error) {
	n := SessionName(name)
	cfg.Env = projectPoolRouting(cfg.Env)
	seeded := false
	defer func() {
		if err == nil || !seeded {
			return
		}
		// A failed launch or a canceled start must not leave a seeded identity
		// or a provisional binding behind. Stop clears the same keys on the
		// successful retire path.
		_ = p.clearStartIdentity(n)
		p.binder.Forget(n)
	}()
	// The env-derived labels let Resolve rediscover a live pane whose sidecar
	// record was lost (gc 1.5.1's pool names carry no rig, so the name alone
	// cannot place the pane in its rig tab).
	hint := Binding{
		TabLabel:  tabLabelFor(n, cfg.Env),
		PaneLabel: n.PaneLabel(),
		Aliases:   aliasesFor(n, cfg.Env),
	}

	old, found, err := p.binder.ResolveHint(ctx, n, hint)
	if err != nil {
		return startResult{}, err
	}
	if found {
		alive, err := p.paneAlive(ctx, old)
		if err != nil {
			return startResult{}, err
		}
		if alive {
			return startResult{}, nil
		}
		if err := p.client.CloseBlock(ctx, old.SessionID, old.BlockID); err != nil && !errors.Is(err, rex.ErrNotFound) {
			return startResult{}, err
		}
		p.binder.Forget(n)
	}

	sid, ok, err := p.binder.FindSession(ctx)
	if err != nil {
		return startResult{}, err
	}
	if !ok {
		sid, err = p.client.CreateSession(ctx, p.binder.canonicalLabel())
		if err != nil {
			return startResult{}, err
		}
	}

	view, err := p.client.SessionView(ctx, sid)
	if err != nil {
		return startResult{}, err
	}

	// Run pre_start hooks before seeding identity or a provisional binding,
	// so a failed hook leaves nothing behind.
	if len(cfg.PreStart) > 0 {
		merged := mergeEnv(os.Environ(), cfg.Env)
		for i, cmd := range cfg.PreStart {
			if err := runPreStart(ctx, cmd, cfg.WorkDir, merged); err != nil {
				return startResult{}, fmt.Errorf("pre_start[%d]: %w", i, err)
			}
		}
	}
	// A pre_start hook is the documented way to create the launch work dir
	// (the gastown worktree recipe points work_dir at a worktree that
	// pre_start creates). Mirror runtime-herdr-core: once the hooks have run,
	// a configured work_dir must exist or the launch would run elsewhere.
	if cfg.WorkDir != "" {
		if info, statErr := os.Stat(cfg.WorkDir); statErr != nil || !info.IsDir() {
			return startResult{}, fmt.Errorf("work_dir %q is not an existing directory after pre_start", cfg.WorkDir)
		}
	}

	// Seed the identity and a provisional binding before the launch call,
	// which blocks while Rex dismisses startup dialogs. A reconciler tick in
	// that window can then read the current GC_SESSION_ID/GC_INSTANCE_TOKEN
	// instead of an empty identity and must not roll the pending create back.
	seeded = true
	if err := p.seedStartIdentity(n, cfg.Env); err != nil {
		return startResult{}, err
	}
	p.recordProvisionalBinding(n, sid, "", cfg)

	tab := hint.TabLabel
	spec := rex.LaunchSpec{
		Command: launchCommand(cfg.Argv, cfg.Env),
		Cwd:     cfg.WorkDir,
		Label:   n.PaneLabel(),
	}
	for _, w := range view.Windows {
		if w.Label == tab {
			anchor, direction, ok := nextSplit(w)
			if !ok {
				continue
			}
			block, err := p.client.NewSplit(ctx, sid, anchor, direction, "after", spec)
			if err != nil {
				return startResult{}, err
			}
			p.record(ctx, n, sid, w.ID, block, "split", cfg)
			return startResult{sessionID: sid, blockID: block}, nil
		}
	}
	wid, block, err := p.client.NewWindow(ctx, sid, tab, spec)
	if err != nil {
		return startResult{}, err
	}
	p.record(ctx, n, sid, wid, block, "window", cfg)
	return startResult{sessionID: sid, blockID: block}, nil
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
	p.waitForStartupReady(ctx, launch.sessionID, launch.blockID)
	report, err := p.nudgeReportBinding(ctx, binding, text, NudgeModeImmediate)
	if err == nil && report.Confirmed {
		p.clearStartupDeliveryUnconfirmed(ctx, name)
		return
	}
	if err != nil {
		p.recordStartupDeliveryUnconfirmed(ctx, binding, report, err)
		return
	}

	for _, backoff := range p.startupNudgeBackoffs {
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
		// Clear the still-drafted line before re-typing, mirroring gc's
		// clear+paste+submit resend cycle.
		_ = p.client.SendKeys(ctx, binding.SessionID, binding.BlockID, startupClearKey)
		report, err = p.nudgeReportBinding(ctx, binding, text, NudgeModeImmediate)
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
func (p *Provider) waitForStartupReady(ctx context.Context, sid rex.SessionID, block rex.BlockID) {
	window := p.startupReadyWindow
	if window <= 0 {
		return
	}
	poll := p.nudgePollInterval
	if poll <= 0 {
		poll = defaultNudgePollInterval
	}
	settle := p.startupSettleDelay
	start := time.Now()
	sawRecord := false
	for {
		snap, err := p.programStatusSnapshot(ctx, Binding{SessionID: sid, BlockID: block})
		if err == nil && snap.Present {
			sawRecord = true
			if snap.State == "idle" {
				return
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
			return
		}
		wait := poll
		if remaining := bound - elapsed; wait > remaining {
			wait = remaining
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

// launchCommand returns argv with the environment applied at the command
// boundary, since Rex terminal launch options have no per-pane env. With no
// env, or no command, argv is returned unchanged: a bare shell cannot be
// seeded (a documented limitation), and an empty argv must stay empty so the
// pane starts its default shell. Keys are sorted for deterministic output,
// and the caller's slice is never mutated.
func launchCommand(argv []string, env map[string]string) []string {
	if len(argv) == 0 || len(env) == 0 {
		return argv
	}
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(env)+1+len(argv))
	out = append(out, "env")
	// `env` stops parsing options at the first KEY=V assignment, so every
	// -u option must precede every assignment.
	for _, k := range keys {
		if env[k] == "" {
			// Empty means unset, like RPP and gc's tmux adapter; a literal
			// empty variable would mask gc's own fallbacks.
			out = append(out, "-u", k)
		}
	}
	for _, k := range keys {
		if env[k] != "" {
			out = append(out, k+"="+env[k])
		}
	}
	return append(out, argv...)
}

// projectPoolRouting makes an unaliased pool or demand seat's work query reach
// its routed work without giving up the per-seat claim identity.
//
// gc 1.5.1 ships GC_ALIAS empty and GC_AGENT/BEADS_ACTOR as the session bead
// id for an unaliased pool seat, and expects `gc hook --claim` to find work
// routed to GC_TEMPLATE. It does not: hookSessionAgentForQuery
// (cmd/gc/cmd_hook.go:818 at v1.5.1) falls back to BEADS_ACTOR/GC_AGENT (the
// bead id), and hookWorkQueryStores (cmd/gc/hook_cross_store.go:107) then
// cannot resolve the rig from that agent query, so the query never reads the
// store the routed step lives in (gastownhall/gascity#5716).
//
// Routing the agent query back to the template fixes discovery. GC_ALIAS stays
// empty, so gc's claim writer hookClaimAssigneeIdentity
// (cmd/gc/cmd_hook.go:866) records the claim under GC_SESSION_ID, the concrete
// session bead id: a live session answers to it (sessionBeadAssigneeIdentities,
// cmd/gc/pool_session_name.go:29) and every pool slot has its own, so seats
// single-flight. Setting GC_ALIAS to the template instead (the GCREX-18
// deviation) made every slot claim under one shared string no session bead
// answers to, so a second seat adopted the first seat's in_progress bead and
// releaseOrphanedPoolAssignments (cmd/gc/pool_session_name.go:211) reopened it
// (GCREX-32).
//
// It applies only when GC_ALIAS is empty, GC_TEMPLATE is set and differs from
// GC_AGENT, and GC_SESSION_ID is set: only BEADS_ACTOR is set to GC_TEMPLATE.
// A non-empty GC_ALIAS, GC_AGENT, GC_SESSION_ID, the tokens and the caller's
// map are never changed. With no GC_SESSION_ID there is no durable claim
// identity to protect, so the env is passed through unchanged.
func projectPoolRouting(env map[string]string) map[string]string {
	if env["GC_ALIAS"] != "" {
		return env
	}
	tmpl := env["GC_TEMPLATE"]
	if tmpl == "" || tmpl == env["GC_AGENT"] {
		return env
	}
	if env["GC_SESSION_ID"] == "" || env["BEADS_ACTOR"] == tmpl {
		return env
	}
	out := make(map[string]string, len(env)+1)
	for k, v := range env {
		out[k] = v
	}
	out["BEADS_ACTOR"] = tmpl
	return out
}

// nextSplit picks where to add a pane to a rig's tab: the block with the
// largest area, split along its longer axis, so panes stay balanced. Ties go
// to the lowest BlockID. When no block carries a usable rect it falls back to
// the first block, split horizontally. ok is false for a window with no blocks.
func nextSplit(w rex.Window) (anchor rex.BlockID, direction string, ok bool) {
	if len(w.Blocks) == 0 {
		return "", "", false
	}
	best := -1
	bestArea := 0.0
	for i, b := range w.Blocks {
		area, usable := rectArea(b.Rect)
		if !usable {
			continue
		}
		if best < 0 || area > bestArea || (area == bestArea && b.ID < w.Blocks[best].ID) {
			best, bestArea = i, area
		}
	}
	if best < 0 {
		return w.Blocks[0].ID, "horizontal", true
	}
	r := w.Blocks[best].Rect
	direction = "vertical"
	if r[2] >= r[3] {
		direction = "horizontal"
	}
	return w.Blocks[best].ID, direction, true
}

// rectArea returns the area of a normalized rect [x, y, w, h]. It is not
// usable when the size is not positive or the rect leaves the 0..1 window.
func rectArea(r [4]float64) (float64, bool) {
	const eps = 1e-6
	x, y, w, h := r[0], r[1], r[2], r[3]
	if w <= 0 || h <= 0 || x < -eps || y < -eps || x+w > 1+eps || y+h > 1+eps {
		return 0, false
	}
	return w * h, true
}

// record stores a binding for a new pane, started in the given mode ("window"
// or "split"). A pane with no block ID is not recorded.
func (p *Provider) record(_ context.Context, n SessionName, sid rex.SessionID, wid rex.WindowID, block rex.BlockID, mode string, cfg Config) {
	if block == "" {
		return
	}
	p.binder.Record(Binding{
		Name:         n,
		SessionID:    sid,
		SessionLabel: rexSessionLabel(p.city),
		WindowID:     wid,
		BlockID:      block,
		TabLabel:     tabLabelFor(n, cfg.Env),
		PaneLabel:    n.PaneLabel(),
		Aliases:      aliasesFor(n, cfg.Env),
		LaunchMode:   mode,
		CreatedAt:    time.Now(),
		GCBin:        gcBinBasename(cfg.Env),
		WorkDir:      cfg.WorkDir,
	})
}

// Stop closes the agent's pane and leaves the city session and other rigs
// intact. It is a no-op when the agent has no pane, and a pane that vanished
// already counts as stopped. Closing a rig's last pane removes its tab.
func (p *Provider) Stop(ctx context.Context, name string) error {
	return p.withStateLock(ctx, func() error { return p.stop(ctx, name) })
}

func (p *Provider) stop(ctx context.Context, name string) error {
	n := SessionName(name)
	// Load the stored binding before Resolve: Resolve drops a stale record when
	// its pane is gone, and a drained worker's process has often exited already.
	stored, hadStored := p.bindings.Load(n)
	binding, ok, err := p.binder.Resolve(ctx, n)
	if err != nil {
		return err
	}
	if !ok {
		// No pane: the worker already exited, which is the usual drain case.
		// Sample its work tree from the stored binding.
		if hadStored {
			p.writeDrainDiagnostic(ctx, stored)
		}
		// Even with no pane, nothing the incarnation left must outlive its
		// life: clear the whole meta sidecar and the activity record, not just
		// the identity seed a failed or rolled-back start may have written.
		return p.clearIncarnations(n, stored.Name)
	}
	// Sample the work tree before the pane closes, while the worker is alive.
	p.writeDrainDiagnostic(ctx, binding)
	if err := p.client.CloseBlock(ctx, binding.SessionID, binding.BlockID); err != nil && !errors.Is(err, rex.ErrNotFound) {
		return err
	}
	if err := p.clearIncarnations(n, binding.Name); err != nil {
		return err
	}
	p.binder.Forget(binding.Name)
	return nil
}

// clearIncarnations removes the whole metadata sidecar and the activity record
// for each name (deduplicated). Stop calls it with the caller's name and the
// resolved primary name, so every key and activity stamp of a retired
// incarnation is gone before the next start of the same name reads them. This
// matches herdr-core's per-incarnation forget (parity doc ### stop row 4).
func (p *Provider) clearIncarnations(names ...SessionName) error {
	seen := map[SessionName]bool{}
	var errs []error
	for _, n := range names {
		if n == "" || seen[n] {
			continue
		}
		seen[n] = true
		if err := p.meta.Clear(n); err != nil {
			errs = append(errs, err)
		}
		if err := p.activity.Delete(n); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// IsRunning reports whether the agent's program is still running. It is
// alias-aware: a rig-qualified name is running while any instance bound to it
// is alive. An agent with no pane is not running.
func (p *Provider) IsRunning(ctx context.Context, name string) (bool, error) {
	n := SessionName(name)
	cands := p.binder.lookup(n)
	if len(cands) == 0 {
		// Nothing cached: fall back to the label scan Resolve performs.
		binding, ok, err := p.binder.Resolve(ctx, n)
		if err != nil || !ok {
			return false, err
		}
		return p.paneAlive(ctx, binding)
	}
	for _, bd := range cands {
		if !matchesName(bd, n) {
			continue
		}
		got, ok, err := p.binder.revalidate(ctx, bd)
		if err != nil {
			return false, err
		}
		if !ok {
			// The stored address is stale; rediscover the pane under the
			// labels it had, so a recreated instance still counts as alive.
			if got, ok, err = p.binder.scan(ctx, bd.Name, bd); err != nil {
				return false, err
			}
		}
		if !ok {
			p.binder.store.Delete(bd.Name)
			continue
		}
		alive, err := p.paneAlive(ctx, got)
		if err != nil {
			return false, err
		}
		if alive {
			return true, nil
		}
	}
	return false, nil
}

// Peek returns the agent's rendered terminal output. An agent with no pane
// has no output.
func (p *Provider) Peek(ctx context.Context, name string) (string, error) {
	return p.PeekLines(ctx, name, 0)
}

// PeekLines returns the last lines lines of the agent's rendered output, or
// all of it when lines <= 0. An agent with no pane has no output.
func (p *Provider) PeekLines(ctx context.Context, name string, lines int) (string, error) {
	binding, ok, err := p.binder.Resolve(ctx, SessionName(name))
	if err != nil || !ok {
		return "", err
	}
	out, err := p.client.BlockFormat(ctx, binding.SessionID, binding.BlockID)
	if errors.Is(err, rex.ErrNotFound) {
		// A pane that vanished between resolve and format has no output, the
		// same as an already-missing pane.
		return "", nil
	}
	if err != nil || lines <= 0 {
		return out, err
	}
	return lastLines(out, lines), nil
}

// lastLines returns the last n lines of s, keeping a trailing newline.
func lastLines(s string, n int) string {
	body := strings.TrimSuffix(s, "\n")
	parts := strings.Split(body, "\n")
	if len(parts) <= n {
		return s
	}
	out := strings.Join(parts[len(parts)-n:], "\n")
	if len(body) != len(s) {
		out += "\n"
	}
	return out
}

// ClearScrollback clears the agent's pane scrollback. It is best effort: an
// agent with no pane, or a pane that vanished, is not an error.
func (p *Provider) ClearScrollback(ctx context.Context, name string) error {
	binding, ok, err := p.binder.Resolve(ctx, SessionName(name))
	if err != nil || !ok {
		return err
	}
	if err := p.client.BlockClear(ctx, binding.SessionID, binding.BlockID); err != nil && !errors.Is(err, rex.ErrNotFound) {
		return err
	}
	return nil
}

// ListRunning returns the sorted names of known sessions with the given
// prefix (all when empty) whose pane is still alive.
func (p *Provider) ListRunning(ctx context.Context, prefix string) ([]string, error) {
	lister, ok := p.bindings.(BindingLister)
	if !ok {
		return nil, errors.New("provider: binding store cannot list")
	}
	names := []string{}
	for _, b := range lister.List() {
		// Skip a binding no name answers for before paying for a liveness probe:
		// gc polls list-running with narrow prefixes constantly.
		if !prefixMatches(b, prefix) {
			continue
		}
		alive, err := p.paneAlive(ctx, b)
		if errors.Is(err, rex.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if !alive {
			continue
		}
		if strings.HasPrefix(string(b.Name), prefix) {
			names = append(names, string(b.Name))
		}
		// Only a non-empty prefix lists aliases. gc's adoption barrier and dead-
		// session reconciler call ListRunning("") and treat every returned name
		// as a real session, so an alias there would mint a phantom orphan bead.
		if prefix == "" {
			continue
		}
		for _, alias := range b.Aliases {
			if strings.HasPrefix(string(alias), prefix) {
				names = append(names, string(alias))
			}
		}
	}
	sort.Strings(names)
	return slices.Compact(names), nil
}

// prefixMatches reports whether b is worth probing for prefix: its primary name
// or one of its aliases carries the prefix. An empty prefix matches everything
// and always probes.
func prefixMatches(b Binding, prefix string) bool {
	if prefix == "" {
		return true
	}
	if strings.HasPrefix(string(b.Name), prefix) {
		return true
	}
	for _, a := range b.Aliases {
		if strings.HasPrefix(string(a), prefix) {
			return true
		}
	}
	return false
}

// SendKeys presses keys in the agent's pane without submitting. gc passes
// tmux-style names ("C-c", "Enter"); they are translated to Rex's syntax.
// No keys is a no-op.
func (p *Provider) SendKeys(ctx context.Context, name string, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}
	binding, err := p.livePane(ctx, name)
	if err != nil {
		return err
	}
	rexKeys := make([]string, len(keys))
	for i, key := range keys {
		rexKeys[i] = rexKey(key)
	}
	if err := p.client.SendKeys(ctx, binding.SessionID, binding.BlockID, rexKeys...); err != nil {
		return fmt.Errorf("send-keys %s: %w", name, err)
	}
	// Keys pressed into the pane are input even when the screen does not
	// change, so record the floor for idle detection.
	p.noteInput(binding)
	return nil
}

// Interrupt sends ctrl+c to the agent's pane. Rex has no signal API, so this
// is a keystroke, not a signal; an agent with no live pane yields
// ErrSessionNotFound.
func (p *Provider) Interrupt(ctx context.Context, name string) error {
	binding, err := p.livePane(ctx, name)
	if err != nil {
		return err
	}
	if err := p.client.SendKeys(ctx, binding.SessionID, binding.BlockID, interruptKey); err != nil {
		return fmt.Errorf("interrupt %s: %w", name, err)
	}
	return nil
}

// livePane resolves name to a pane whose program is still running. A missing
// pane, or one whose program has exited, yields ErrSessionNotFound: input
// would reach no agent. A transport or observation error is returned as is.
func (p *Provider) livePane(ctx context.Context, name string) (Binding, error) {
	binding, ok, err := p.binder.Resolve(ctx, SessionName(name))
	if err != nil {
		return Binding{}, err
	}
	if !ok {
		return Binding{}, fmt.Errorf("%w: %s has no pane", ErrSessionNotFound, name)
	}
	alive, err := p.paneAlive(ctx, binding)
	if err != nil {
		return Binding{}, err
	}
	if !alive {
		return Binding{}, fmt.Errorf("%w: %s agent program exited", ErrSessionNotFound, name)
	}
	return binding, nil
}

// mergeEnv merges the base environment slice with an overlay map, returning a
// new sorted slice. Overlay entries replace base entries with the same key.
func mergeEnv(base []string, overlay map[string]string) []string {
	m := make(map[string]string, len(base)+len(overlay))
	for _, kv := range base {
		if idx := strings.IndexByte(kv, '='); idx >= 0 {
			m[kv[:idx]] = kv[idx+1:]
		}
	}
	for k, v := range overlay {
		m[k] = v
	}
	out := make([]string, 0, len(m))
	for k, v := range m {
		out = append(out, k+"="+v)
	}
	sort.Strings(out)
	return out
}

// runPreStart runs one pre_start shell command. It honours ctx cancellation.
// The command runs in workDir when that directory already exists and in "/"
// otherwise, mirroring runtime-herdr-core's _setup_cwd, so a pre_start can
// create the work dir before the launch.
func runPreStart(ctx context.Context, command, workDir string, env []string) error {
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", command)
	cmd.Dir = preStartCwd(workDir)
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil {
		if len(out) > 0 {
			return fmt.Errorf("%s: %w\n%s", command, err, out)
		}
		return fmt.Errorf("%s: %w", command, err)
	}
	return nil
}

// preStartCwd returns workDir when it is an existing directory, else "/". A
// work_dir that pre_start is meant to create does not exist yet, so it cannot
// be the hook's cwd.
func preStartCwd(workDir string) string {
	if workDir != "" {
		if info, err := os.Stat(workDir); err == nil && info.IsDir() {
			return workDir
		}
	}
	return "/"
}
