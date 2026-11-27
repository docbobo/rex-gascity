// Package provider is the Rex-backed Gas City session provider core. It maps
// gc sessions (city/rig/agent) onto the Rex hierarchy (session/tab/pane) and
// will be wrapped by the RPP v0 op dispatcher in cmd/gc-runtime-rex.
package provider

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/docbobo/rex-gascity/internal/launch"
	"github.com/docbobo/rex-gascity/internal/prochost"
	"github.com/docbobo/rex-gascity/internal/rex"
)

// ErrSessionNotFound reports that an input op found no live agent for the
// named session: the pane is gone, or its program has exited. The RPP
// dispatcher maps it to a failed op with a clear message.
var ErrSessionNotFound = errors.New("session not found")

// ErrAlreadyRunning reports that StartNew found a live pane for the session.
// The RPP dispatcher maps it to gc's "already running" start collision.
var ErrAlreadyRunning = errors.New("provider: already running")

// ErrFreshCollision reports that a start with fresh_only found a name any
// record holds (live or dead) and refused to recycle it. The RPP dispatcher
// maps it to herdr-core's FRESH_COLLISION exit 73.
var ErrFreshCollision = errors.New("provider: session already exists (fresh_only)")

// ErrNoScreen reports that peek found no screen to read: the session has no
// record, its pane vanished, or Rex is unreachable. The RPP dispatcher maps it
// to herdr-core's exit 66 (`no screen for '<name>'`).
var ErrNoScreen = errors.New("provider: no screen")

// warnf writes one start warning. Warnings never fail a start; gc drops a
// successful start's stderr, so this is a diagnostic channel only.
func (p *Provider) warnf(format string, args ...any) {
	if p.warn == nil {
		return
	}
	fmt.Fprintf(p.warn, "start: "+format+"\n", args...)
}

// InputOutcomeError carries an RPP typed input outcome (herdr-core's exit 80)
// for an input op. Outcome is the wire word, one of "gone", "not_sent",
// "unavailable", "busy", "pending_interaction" or "rejected"; Detail is the
// human-readable reason. The wrapped Err keeps classification available to
// callers.
type InputOutcomeError struct {
	Outcome string
	Detail  string
	Err     error
}

func (e *InputOutcomeError) Error() string {
	if e.Detail != "" {
		return e.Detail
	}
	return e.Outcome
}

func (e *InputOutcomeError) Unwrap() error { return e.Err }

// submitKey submits typed input; interruptKey is the soft interrupt keystroke.
// Rex has no signal API, so both are keys.
const (
	submitKey    = "enter"
	interruptKey = "ctrl+c"
	// startCleanupTimeout bounds the detached cleanup of a failed launch.
	// context.WithoutCancel keeps the work after caller cancellation but also
	// drops the deadline; without this bound the cleanup could wait out the
	// state lock and Rex calls while it holds the city-wide lock.
	startCleanupTimeout = 30 * time.Second
)

// CopyFile is one gc `copy_files` entry: Src is the source path and RelDst is
// the destination relative to the launch work_dir (empty means the source's
// base name in work_dir).
type CopyFile struct {
	Src    string `json:"src"`
	RelDst string `json:"rel_dst,omitempty"`
}

// Config is the launch configuration the RPP `start` op carries as JSON.
type Config struct {
	Argv    []string
	WorkDir string
	// Env is the per-session environment gc ships in the start config. Rex
	// launch options carry no env, so Start applies it by prefixing the launch
	// argv with `env K=V ...` (see launch.Command).
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
	// Lifecycle is gc's start-config lifecycle. Only "one_shot" is
	// distinguished: a one-shot command is expected to exit on its own, so it
	// skips the post-launch and first-turn agent-liveness failures. Any other
	// value, including empty, is the default lifecycle that fails a start whose
	// agent ends during startup or before its first turn (parity doc ### start
	// row 14).
	Lifecycle string
	// PackOverlayDirs lists pack overlay directories. The provider stages each
	// directory's per-provider/<GC_PROVIDER> subtree into WorkDir after pre_start,
	// so a pack's per-provider override reaches a workdir that already holds a
	// managed file gc's preserve predicate would keep. Later directories win.
	PackOverlayDirs []string
	// SessionSetup is an ordered list of shell commands run after the agent pane
	// launches and before the startup nudge. A failure is a warning, not a
	// failed start, mirroring herdr-core (herdrcore/lifecycle.py:811-814).
	SessionSetup []string
	// SessionSetupScript is one shell command run with SessionSetup's semantics.
	SessionSetupScript string
	// SessionLive is gc's tmux live configuration (theming, keybindings, status
	// bars). It is never applied on Rex; a non-empty value yields one
	// SESSION_LIVE_SKIPPED warning, mirroring herdr-core
	// (herdrcore/lifecycle.py:162-164,823-824).
	SessionLive []string
	// OverlayDir is a gc overlay directory staged into WorkDir before pre_start:
	// its universal tree (minus the top-level per-provider and .gc subtrees),
	// then its per-provider/<GC_PROVIDER> subtree flattened into WorkDir
	// (herdrcore/staging.py:225-246).
	OverlayDir string
	// CopyFiles stages individual files (or trees) into WorkDir before pre_start.
	CopyFiles []CopyFile
	// FreshOnly refuses a name any record already holds, with RPP exit 73
	// (herdr-core FRESH_COLLISION), instead of recycling a dead record. It is
	// enforced by StartNew, the op gc's start maps to; Start does not consult it
	// because a plain start is allowed to adopt a live pane.
	FreshOnly bool
	// ReadyPromptPrefix, when set, is the pane prompt prefix that marks the agent
	// ready for its first turn. It gates the startup-nudge readiness wait and
	// replaces the program_status signal while set.
	ReadyPromptPrefix string
	// ReadyDelayMS is a minimum delay before readiness is declared, in
	// milliseconds. It is capped at 60 s, mirroring herdr-core READY_WAIT_S,
	// and ignored while ReadyPromptPrefix is set, because gc sends ready_delay_ms
	// instead of a prompt to watch for and herdr-core consults the prompt first
	// (gc v1.5.1 docs/reference/exec-session-provider.md:228-229).
	ReadyDelayMS int
}

// Provider drives Rex for one gc city.
type Provider struct {
	city   string
	client rex.Client
	binder *Binder

	// townTab is the town-level tab label, resolved once from the process
	// city pin at construction so every start and record agrees even when the
	// caller passes a different city.
	townTab string

	bindings Store[SessionName, Binding]
	activity Store[SessionName, ActivityRecord]
	meta     Store[metaKey, string]

	// ns is the city namespace: the state dirs (the canonical one holds the
	// cross-process lock; none means no locking) and the accepted Rex session
	// labels (new sessions use the canonical one; lookups and binding
	// revalidation accept all so a legacy namespace still resolves).
	ns Namespace

	// runner runs the external processes the provider shells out to: the gc
	// claim probe (nudge.go), the git drain-diagnostic calls (drain_diag.go)
	// and the process table of the stop reap (reap.go). It is shared, so it
	// lives on Provider rather than in nudgeConfig.
	runner Runner

	// processHost is the seam over the host process table and signals. The
	// reap path reads the table and signals through it. The default adapter
	// reads the table through runner and signals through the real adapter;
	// tests inject a prochost.Fake with WithProcessHost.
	processHost prochost.Host

	// nudge holds the nudge and startup-delivery tunables (see nudge.go).
	// It is a named field, not embedded, so the tunables are not promoted
	// onto Provider.
	nudge nudgeConfig

	// inputModeProbeTimeout bounds one input_mode read of the secret-read
	// guard (inputmode.go). Zero means defaultInputModeProbeTimeout.
	inputModeProbeTimeout time.Duration
	// startBudget bounds the whole start op when the op context carries no
	// earlier deadline; preStartHookTimeout bounds one pre_start hook, capped
	// by whatever remains of startBudget (see start).
	startBudget         time.Duration
	preStartHookTimeout time.Duration

	// warn receives start warnings (ignored session_setup hooks, session_live,
	// a refused copy_files destination). It is stderr in production; tests can
	// capture it with WithWarnings.
	warn io.Writer
}

// Option configures a Provider.
type Option func(*Provider)

// WithBindingStore sets the binding store.
func WithBindingStore(s Store[SessionName, Binding]) Option {
	return func(p *Provider) { p.bindings = s }
}

// WithActivityStore sets the activity store.
func WithActivityStore(s Store[SessionName, ActivityRecord]) Option {
	return func(p *Provider) { p.activity = s }
}

// WithMetaStore sets the metadata store.
func WithMetaStore(s Store[metaKey, string]) Option { return func(p *Provider) { p.meta = s } }

// WithNamespace configures the provider for ns: its state dirs back the
// sidecar stores and its labels are the accepted Rex session labels. It is the
// one namespace option; a namespace with no dirs leaves the stores in memory.
// Callers that build a namespace from city names use NewNamespace, which
// reports a state-dir error instead of dropping a namespace silently.
func WithNamespace(ns Namespace) Option {
	return func(p *Provider) {
		applyNamespace(ns)(p)
	}
}

// applyNamespace installs ns's state dirs and labels on p. It is the single
// place that builds the file-backed composite stores, so every namespace
// option goes through it.
func applyNamespace(ns Namespace) Option {
	return func(p *Provider) {
		if len(ns.dirs) > 0 {
			bindings := make([]Store[SessionName, Binding], 0, len(ns.dirs))
			activities := make([]Store[SessionName, ActivityRecord], 0, len(ns.dirs))
			metas := make([]Store[metaKey, string], 0, len(ns.dirs))
			for _, dir := range ns.dirs {
				bindings = append(bindings, NewFileStore(filepath.Join(dir, "bindings")))
				activities = append(activities, NewFileActivityStore(filepath.Join(dir, "activity")))
				metas = append(metas, NewFileMetaStore(filepath.Join(dir, "meta")))
			}
			p.ns.dirs = slices.Clone(ns.dirs)
			p.bindings = newCompositeBindingStore(bindings...)
			p.activity = newCompositeActivityStore(activities...)
			p.meta = newCompositeMetaStore(metas...)
		}
		if len(ns.labels) > 0 {
			p.ns.labels = normalizeSessionLabels(ns.labels)
		}
	}
}

// Defaults for the start-path budgets. gc's contract gives start a 120 s op
// deadline (docs/REX-CONTRACT.md §op deadlines); herdr-core bounds one
// pre_start hook to 30 s. The GC_EXEC_DEADLINE_MS env var, if gc ever sends
// one, is not read: the pinned gc 1.5.1 baseline does not.
const (
	defaultStartBudget         = 120 * time.Second
	defaultPreStartHookTimeout = 30 * time.Second
)

// WithStartBudget sets the whole-op budget start derives its per-hook timeout
// from when the op context has no earlier deadline. A value <= 0 keeps the
// default.
func WithStartBudget(d time.Duration) Option {
	return func(p *Provider) {
		if d > 0 {
			p.startBudget = d
		}
	}
}

// WithPreStartHookTimeout sets the per-hook pre_start timeout. A value <= 0
// keeps the default. The hook timeout is further capped by the remaining start
// budget, so a chain of hooks cannot outlive the op.
func WithPreStartHookTimeout(d time.Duration) Option {
	return func(p *Provider) {
		if d > 0 {
			p.preStartHookTimeout = d
		}
	}
}

// WithWarnings sets where start warnings are written. A nil writer keeps the
// default (os.Stderr). Warnings never change the start result: gc drops a
// successful start's stderr, so they are diagnostics, not a failure channel.
func WithWarnings(w io.Writer) Option {
	return func(p *Provider) {
		if w != nil {
			p.warn = w
		}
	}
}

// WithProcessHost sets the host process seam. A nil host keeps the default
// adapter over the Runner and signal hook.
func WithProcessHost(h prochost.Host) Option {
	return func(p *Provider) {
		if h != nil {
			p.processHost = h
		}
	}
}

// New builds a Provider for city, driving client. Without options all state is
// held in memory.
func New(city string, client rex.Client, opts ...Option) *Provider {
	p := &Provider{
		city:     city,
		client:   client,
		bindings: NewMemoryStore(),
		activity: NewMemoryActivityStore(),
		meta:     NewMemoryMetaStore(),
		runner:   ExecRunner{},
		nudge:    defaultNudgeConfig(),

		startBudget:         defaultStartBudget,
		preStartHookTimeout: defaultPreStartHookTimeout,
		warn:                os.Stderr,
	}
	p.nudge.claimProbe = p.probeLiveClaim
	p.townTab = townTabLabel(cityPin())
	for _, opt := range opts {
		opt(p)
	}
	if p.processHost == nil {
		p.processHost = prochost.NewPSHost(p.runner)
	}
	labels := p.ns.labels
	if len(labels) == 0 {
		labels = []string{rexSessionLabel(city)}
	}
	p.binder = NewBinderWithLabels(client, p.bindings, labels, p.townTab)
	return p
}

// Binding returns the stored binding that answers to name (its primary name
// or one of its aliases) without contacting Rex. It is the dispatcher-facing
// lookup; Resolve-based ops re-validate it against Rex.
func (p *Provider) Binding(name string) (Binding, bool) {
	bd, ok, _ := p.binder.Resolve(context.Background(), ResolveRequest{Name: SessionName(name)})
	return bd, ok
}

// Start creates the city session, the rig's tab, and the agent's pane, and
// records the pane's binding. It is idempotent: a live pane is left alone, and
// a pane whose agent has died is closed before a new one is launched.
//
// The per-name lock serializes every start and stop of this name; the
// city-wide state lock is taken only around short store and session mutations,
// never across pre_start or the blocking launch. The startup nudge is delivered
// after the locks are released, so its bounded readiness wait cannot stall
// other starts or stops.
func (p *Provider) Start(ctx context.Context, name string, cfg Config) error {
	var launch startResult
	err := p.withNameLock(ctx, SessionName(name), func() error {
		var startErr error
		launch, startErr = p.start(ctx, name, cfg)
		return startErr
	})
	if err != nil {
		return err
	}
	if err := p.failIfAgentEnded(ctx, name, launch, cfg); err != nil {
		return err
	}
	p.runSessionSetup(ctx, name, launch, cfg)
	return p.deliverStartupNudge(ctx, name, launch, cfg)
}

// StartNew starts the agent like Start, but fails with ErrAlreadyRunning when
// its pane is already live. The per-name lock is held across the liveness check
// and the launch, so of two concurrent StartNew calls for one name sharing a
// state dir, exactly one launches and the other sees ErrAlreadyRunning. The
// startup nudge is delivered after the lock is released, like Start.
func (p *Provider) StartNew(ctx context.Context, name string, cfg Config) error {
	var launch startResult
	err := p.withNameLock(ctx, SessionName(name), func() error {
		if cfg.FreshOnly {
			// fresh_only refuses a name any record holds, live or dead, and
			// never recycles or reuses it (herdr-core _check_name,
			// herdrcore/lifecycle.py:755-756). A stale binding is a record, and
			// so is a binding that holds the name only as an alias, so the
			// check goes through lookup, not the primary-key read alone.
			if len(p.binder.lookup(SessionName(name))) > 0 {
				return fmt.Errorf("%w: %s", ErrFreshCollision, name)
			}
		}
		running, err := p.IsRunning(ctx, name)
		if err != nil {
			return err
		}
		if cfg.FreshOnly && running {
			return fmt.Errorf("%w: %s", ErrFreshCollision, name)
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
	if err := p.failIfAgentEnded(ctx, name, launch, cfg); err != nil {
		return err
	}
	p.runSessionSetup(ctx, name, launch, cfg)
	return p.deliverStartupNudge(ctx, name, launch, cfg)
}

// lifecycleOneShot is the only start-config lifecycle this provider
// distinguishes; gc sends no other value (internal/runtime/exec/json.go:29 at
// v1.5.1). A one-shot command is expected to exit on its own, so it skips the
// post-launch and first-turn agent-liveness failures (parity doc ### start row
// 14; herdr-core ONE_SHOT, herdrcore/lifecycle.py:161).
const lifecycleOneShot = "one_shot"

// failIfAgentEnded fails a start whose agent program already exited, unless the
// lifecycle is one_shot. It mirrors herdr-core's _fail_if_ended
// (herdrcore/lifecycle.py:1027-1036): a single liveness read right after the
// launch, not a wait, and only a definite end is a failure. The dead pane is
// left in place for gc's post-failure stop, so the binding stays recorded.
func (p *Provider) failIfAgentEnded(ctx context.Context, name string, launch startResult, cfg Config) error {
	if cfg.Lifecycle == lifecycleOneShot || launch.blockID == "" {
		return nil
	}
	b := Binding{Name: SessionName(name), SessionID: launch.sessionID, BlockID: launch.blockID}
	alive, err := p.paneAlive(ctx, b)
	if err != nil || alive {
		// An observation error cannot prove the agent ended, so it must not
		// fail a start that might be healthy.
		return nil
	}
	return fmt.Errorf("%s: the agent exited during startup", name)
}

// startResult identifies the pane a start launched. A zero result means the
// call was idempotent and found the pane already alive.
type startResult struct {
	sessionID rex.SessionID
	blockID   rex.BlockID
	// launchDur is how long the pane launch (NewSplit/NewWindow) took. It is a
	// stage in the startup-timing diagnostic (GCREX-58); zero on the idempotent
	// already-alive path, which delivers no nudge.
	launchDur time.Duration
	// env is the launch plan's projected environment (pool routing applied).
	// The post-launch setup hooks run with it, so they see the same identity
	// the launched pane carries (herdr-core applies the session env to setup;
	// herdrcore/envplan.py:51-76).
	env map[string]string
}

func (p *Provider) start(ctx context.Context, name string, cfg Config) (startResult, error) {
	n := SessionName(name)
	plan, attempt := p.startPlan(name, &cfg)

	relaunch, done, err := p.recycleName(ctx, n, cfg)
	if err != nil {
		return startResult{}, err
	}
	if done {
		return startResult{}, nil
	}

	// Find or create the city session under the short state lock.
	sid, err := p.findOrCreateCitySession(ctx)
	if err != nil {
		return startResult{}, err
	}

	if err := p.stageStartFiles(cfg); err != nil {
		return startResult{}, err
	}
	if err := p.runPreStartPhase(ctx, plan, cfg); err != nil {
		return startResult{}, err
	}
	if err := p.requireWorkDir(cfg.WorkDir); err != nil {
		return startResult{}, err
	}
	if err := p.stagePackOverlayPhase(cfg); err != nil {
		return startResult{}, err
	}

	// Seed identity, launch the pane, and record it or roll back. launchPane
	// owns that step, its lock order, the vanished-session retry and the
	// rollback of everything this start seeded.
	return p.launchPane(ctx, launchRequest{
		name:     n,
		attempt:  attempt,
		session:  sid,
		tab:      tabLabel(n, cfg.Env, p.townTab),
		spec:     plan.Spec,
		relaunch: relaunch,
		cfg:      cfg,
	})
}

// startPlan builds the one launch plan for this start and its attempt token.
// It runs first because the plan projects pool routing for every later step and
// owns the Rex block layout: the launch label must come from the same projected
// env as the start labels and the record, or pool routing makes them disagree
// and a restart launches a duplicate pane. cfg.Env is replaced with the
// projected env for the steps that follow.
//
// One token is minted per start call. The provisional record this start writes
// is writable only by this attempt, so a stale survivor write from another
// launch cannot take it, and launchPane's retry reuses the same token.
func (p *Provider) startPlan(name string, cfg *Config) (launch.Plan, string) {
	n := SessionName(name)
	attempt := rand.Text()
	env := launch.Project(cfg.Env)
	plan := launch.NewPlan(launch.Config{
		Argv:    cfg.Argv,
		Env:     env,
		WorkDir: cfg.WorkDir,
		Label:   paneLabelFor(n, env),
	})
	cfg.Env = plan.Env
	return plan, attempt
}

// stageStartFiles stages gc's overlay_dir and copy_files before pre_start, so a
// hook sees the staged work_dir (herdrcore/lifecycle.py:789).
// stagePackOverlayPhase stays after pre_start because its per-provider subtree
// deliberately overwrites a managed hook the hook may have rewritten.
func (p *Provider) stageStartFiles(cfg Config) error {
	if err := stageOverlayDir(cfg.WorkDir, cfg.OverlayDir, cfg.Env["GC_PROVIDER"]); err != nil {
		return err
	}
	return stageCopyFiles(cfg.WorkDir, cfg.CopyFiles, func(msg string) { p.warnf("%s", msg) })
}

// runPreStartPhase runs the pre_start hooks before identity or a provisional
// binding is seeded. gc's exec-session-provider contract makes a failing
// pre_start non-fatal: warn on stderr and keep starting
// (docs/reference/exec-session-provider.md at v1.5.1, "pre_start"). Only
// cancellation of the whole start aborts; a hook's own failure or timeout does
// not. The state lock is not held here: a slow hook must not stall the rest of
// the city.
func (p *Provider) runPreStartPhase(ctx context.Context, plan launch.Plan, cfg Config) error {
	return p.runPreStartHooks(ctx, plan, cfg)
}

// requireWorkDir checks the configured work dir after pre_start. A pre_start
// hook is the documented way to create the launch work dir (the gastown
// worktree recipe points work_dir at a worktree that pre_start creates). Mirror
// runtime-herdr-core: once the hooks have run, a configured work_dir must exist
// or the launch would run elsewhere.
func (p *Provider) requireWorkDir(workDir string) error {
	if workDir == "" {
		return nil
	}
	if info, err := os.Stat(workDir); err != nil || !info.IsDir() {
		return fmt.Errorf("work_dir %q is not an existing directory after pre_start", workDir)
	}
	return nil
}

// stagePackOverlayPhase writes each pack's per-provider overlay subtree after
// pre_start. gc's managed-file preserve predicate can keep an existing current
// file, so the controller may never write a city-configured pack overlay. The
// write runs here so the override lands even when the workdir already holds
// that managed file.
func (p *Provider) stagePackOverlayPhase(cfg Config) error {
	return stagePackOverlays(cfg.WorkDir, cfg.PackOverlayDirs, cfg.Env["GC_PROVIDER"])
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

// newBinding builds the record for a new pane, started in the given mode
// ("window" or "split").
func (p *Provider) newBinding(n SessionName, sid rex.SessionID, wid rex.WindowID, block rex.BlockID, mode, attempt string, cfg Config) Binding {
	return Binding{
		Name:           n,
		SessionID:      sid,
		SessionLabel:   rexSessionLabel(p.city),
		WindowID:       wid,
		BlockID:        block,
		TabLabel:       tabLabel(n, cfg.Env, p.townTab),
		PaneLabel:      paneLabelFor(n, cfg.Env),
		Aliases:        aliasesFor(n, cfg.Env),
		LaunchMode:     mode,
		CreatedAt:      time.Now(),
		GCBin:          gcBinBasename(cfg.Env),
		WorkDir:        cfg.WorkDir,
		ClaimSessionID: cfg.Env["GC_SESSION_ID"],
		Attempt:        attempt,
	}
}

// Stop closes the agent's pane and leaves the city session and other rigs
// intact. It is a no-op when the agent has no pane, and a pane that vanished
// already counts as stopped. Closing a rig's last pane removes its tab.
//
// The per-name lock serializes a stop with a concurrent start of the same name.
// Every record a stop mutates belongs to that one name, so no city-wide state
// lock is needed: the close, the reap and the re-read run while stops and starts
// of other names proceed (observe.v1 rule 9).
func (p *Provider) Stop(ctx context.Context, name string) error {
	return p.withNameLock(ctx, SessionName(name), func() error { return p.stopUnderPrimaryLock(ctx, name) })
}

// destroyEmptyCitySession removes this city's canonical gc:<city> session once
// its last pane is gone (GCREX-31). The caller holds the state lock. This
// function takes the session's launch lock before it reads the view, so no
// launch can add a pane between the empty check and the removal. It fails
// closed: a session with any window, a label that is not the canonical one (a
// legacy, foreign or other-city session), or any Rex error keeps the session
// and returns the error.
func (p *Provider) destroyEmptyCitySession(ctx context.Context, sid rex.SessionID) error {
	if sid == "" {
		return nil
	}
	// The launch lock comes first: a launch holds it shared across its
	// SessionView and NewWindow/NewSplit, so an empty view below cannot go
	// stale under us. Rex removes a tab with its last block
	// (docs/plans/native-rex-provider.md:387, live-verified), so zero windows
	// is a true empty session.
	release, busy, lockErr := p.tryLaunchLock(sid)
	if lockErr != nil {
		return fmt.Errorf("check whether %s is empty: %w", sid, lockErr)
	}
	if busy {
		return nil
	}
	defer release()
	view, err := p.client.SessionView(ctx, sid)
	if errors.Is(err, rex.ErrNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("check whether %s is empty: %w", sid, err)
	}
	if view.Label != p.binder.canonicalLabel() || len(view.Windows) != 0 {
		return nil
	}
	if err := p.client.DestroySession(ctx, sid); err != nil && !errors.Is(err, rex.ErrNotFound) {
		return fmt.Errorf("remove the empty city session %s: %w", sid, err)
	}
	return nil
}

// reapSurvivors reaps tree and, while any of it survives, records the pids on
// the binding so the retry stop herdr-core allows still has them: the pane is
// closed by then and reports nothing. It returns the failure to report, nil
// when the tree is gone.
func (p *Provider) reapSurvivors(ctx context.Context, binding Binding, tree []prochost.Ident) error {
	// A survivor recorded by an earlier stop can spawn a child before the
	// retry, and the closed pane no longer reports that child: read the host
	// table so the retry reaps what the tree grew, not only what it was given.
	tree = p.descendants(ctx, tree)
	left, readErr, signalErr := p.reap(ctx, tree)
	binding.Survivors = survivorRecords(left)
	if len(left) == 0 {
		// A signal error with nothing left is a success: everything died anyway
		// (the caller's contract), so the failed signal is not reported.
		return readErr
	}
	survived := &survivorError{name: string(binding.Name), survivors: left, reason: errors.Join(readErr, signalErr)}
	// A start may have replaced the binding while this reap ran. Record only
	// while the stored binding is still the one we reaped: a replacement owns
	// the name and must not carry the old pane's survivors.
	if _, err := p.binder.RecordIfCurrent(binding); err != nil {
		// The stop fails either way: the record that would name these pids for
		// the next retry was not saved.
		return errors.Join(err, survived)
	}
	return survived
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
		if err := p.metaClear(n); err != nil {
			errs = append(errs, err)
		}
		if err := p.activity.Drop(n); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// IsRunning reports whether the agent's program is still running. It is
// alias-aware: a rig-qualified name is running while any instance bound to it
// is alive. An agent with no pane is not running.
func (p *Provider) IsRunning(ctx context.Context, name string) (bool, error) {
	_, ok, err := p.binder.Resolve(ctx, ResolveRequest{Name: SessionName(name), Mode: ResolveLive})
	return ok, err
}

// Peek returns the agent's rendered terminal output, or ErrNoScreen when the
// agent has no pane to read.
func (p *Provider) Peek(ctx context.Context, name string) (string, error) {
	return p.PeekLines(ctx, name, 0)
}

// PeekLines returns the last lines lines of the agent's rendered output, or
// all of it when lines <= 0. It returns ErrNoScreen when there is no screen to
// read: no binding, a pane that vanished before it could be formatted, or Rex
// unreachable (from the resolve, the liveness read or the format call). A dead
// box's capture that rendered nothing is an error, never an empty screen: v2
// reads exit 0 with no lines as nothing to classify why the box died. A
// running box that has rendered nothing yet is a blank screen, which succeeds.
func (p *Provider) PeekLines(ctx context.Context, name string, lines int) (string, error) {
	binding, ok, err := p.binder.Resolve(ctx, ResolveRequest{Name: SessionName(name), Mode: ResolveFresh})
	if err != nil && !errors.Is(err, rex.ErrUnavailable) && !errors.Is(err, rex.ErrNotFound) {
		return "", err
	}
	if err != nil || !ok {
		return "", ErrNoScreen
	}
	life := readPaneLife(ctx, p.client, binding.SessionID, binding.BlockID)
	if err := life.peekErr(); err != nil {
		return "", err
	}
	out, err := p.client.BlockFormat(ctx, binding.SessionID, binding.BlockID)
	if errors.Is(err, rex.ErrNotFound) || errors.Is(err, rex.ErrUnavailable) {
		return "", ErrNoScreen
	}
	if err != nil {
		return "", err
	}
	if out == "" {
		// A dead box's screen is the only record of why it died, so a capture
		// that wrote nothing is unreadable, never a blank screen. The program
		// can exit between the two calls, so an empty capture re-reads the
		// process state before the blank screen is accepted; a re-read that
		// fails leaves the earlier answer standing.
		if life.exitedAfterEmptyCapture(ctx, p.client, binding.SessionID, binding.BlockID) {
			return "", fmt.Errorf("peek %s: the dead box's capture wrote nothing", name)
		}
	}
	if lines <= 0 {
		return out, nil
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

// ClearScrollback clears the agent's pane scrollback. A missing pane, or one
// whose program has exited, is gone (an RPP input outcome) rather than a
// silent success, so a caller can tell a cleared scrollback from a vanished
// pane (GCREX-57). A mid-call ErrNotFound is also gone: the pane vanished
// before Rex could clear it. An unreachable server is unavailable; any other
// pre-send failure is not_sent. The words come from classifyInput (input.go).
func (p *Provider) ClearScrollback(ctx context.Context, name string) error {
	return p.deliverInput(ctx, inputCall{
		op:   inputClearScrollback,
		name: name,
		send: func(ctx context.Context, b Binding) error {
			if err := p.client.BlockClear(ctx, b.SessionID, b.BlockID); err != nil {
				return fmt.Errorf("clear-scrollback %s: %w", name, err)
			}
			return nil
		},
	})
}

// ListRunning returns the sorted names of known sessions with the given
// prefix (all when empty) whose pane is still alive.
func (p *Provider) ListRunning(ctx context.Context, prefix string) ([]string, error) {
	lister, ok := p.bindings.(Lister[Binding])
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
		if errors.Is(err, rex.ErrUnavailable) {
			// Rex is down, so no pane can be confirmed running. Answer with
			// the parked-city inventory, empty and exit 0 (parity doc
			// list-running deviation 5): gc reads an error here as a fleet it
			// could not enumerate and refuses to tear the city down.
			return []string{}, nil
		}
		if errors.Is(err, rex.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if !alive {
			continue
		}
		if strings.HasPrefix(string(b.Name), prefix) && readableName(string(b.Name)) {
			names = append(names, string(b.Name))
		}
		// Only a non-empty prefix lists aliases. gc's adoption barrier and dead-
		// session reconciler call ListRunning("") and treat every returned name
		// as a real session, so an alias there would mint a phantom orphan bead.
		if prefix == "" {
			continue
		}
		for _, alias := range b.Aliases {
			if strings.HasPrefix(string(alias), prefix) && readableName(string(alias)) {
				names = append(names, string(alias))
			}
		}
	}
	sort.Strings(names)
	return slices.Compact(names), nil
}

// readableName reports whether name is safe to print as one list-running line:
// non-empty, without leading or trailing whitespace, and without a control
// character. It mirrors herdr-core's readable_name (rpp/wire.py:194), so a name
// gc cannot parse back (a stray newline would split one session into two) is
// never emitted.
func readableName(name string) bool {
	if name == "" || name != strings.TrimSpace(name) {
		return false
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
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
// No keys is a no-op. Every key is validated before the pane lookup, so a key
// with no Rex spelling rejects the call without touching Rex: a "rejected"
// *InputOutcomeError wrapping the KeyRejectedError.
func (p *Provider) SendKeys(ctx context.Context, name string, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}
	rexKeys, err := translateKeys(keys)
	if err != nil {
		return classifyInput(inputSendKeys, stageValidate, err)
	}
	// A pane reading a secret gets no keys (#48): an Enter there submits
	// whatever was typed as a password attempt. A list of only ctrl+c is the
	// exception and makes no read, because ctrl+c is the way out of a password
	// prompt. A mixed list is withheld whole, never split.
	return p.deliverInput(ctx, inputCall{
		op:    inputSendKeys,
		name:  name,
		guard: !onlyInterrupt(rexKeys),
		send: func(ctx context.Context, b Binding) error {
			if err := p.client.SendKeys(ctx, b.SessionID, b.BlockID, rexKeys...); err != nil {
				return fmt.Errorf("send-keys %s: %w", name, err)
			}
			return nil
		},
	})
}

// Interrupt sends ctrl+c to the agent's pane. Rex has no signal API, so this
// is a keystroke, not a signal; an agent with no live pane yields a "gone"
// outcome wrapping ErrSessionNotFound.
func (p *Provider) Interrupt(ctx context.Context, name string) error {
	return p.deliverInput(ctx, inputCall{
		op:   inputInterrupt,
		name: name,
		send: func(ctx context.Context, b Binding) error {
			if err := p.client.SendKeys(ctx, b.SessionID, b.BlockID, interruptKey); err != nil {
				return fmt.Errorf("interrupt %s: %w", name, err)
			}
			return nil
		},
	})
}

// livePane resolves name to a pane whose program is still running. A missing
// pane, or one whose program has exited, yields ErrSessionNotFound: input
// would reach no agent. A transport or observation error is returned as is.
func (p *Provider) livePane(ctx context.Context, name string) (Binding, error) {
	binding, ok, err := p.binder.Resolve(ctx, ResolveRequest{Name: SessionName(name), Mode: ResolveFresh})
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

// sessionLiveSkipped is the one warning a non-empty session_live produces.
// gc's session_live styles and binds a tmux server; the Rex provider runs no
// tmux server, so applying it would reach an unrelated one
// (herdrcore/lifecycle.py:162-164).
const sessionLiveSkipped = "session_live is not applied on this runtime: it styles and binds a tmux server, and the Rex provider runs no tmux server"

// runSessionSetup runs the start config's post-launch hooks and reports the
// ignored session_live. A hook failure is a warning, never a failed start,
// mirroring herdr-core (herdrcore/lifecycle.py:811-824): gc drops a successful
// start's stderr, and the startup nudge must still be attempted. It is a no-op
// when the start launched nothing (an idempotent Start) or the config names no
// setup work.
func (p *Provider) runSessionSetup(ctx context.Context, name string, res startResult, cfg Config) {
	if res.blockID == "" {
		return
	}
	if len(cfg.SessionSetup) == 0 && cfg.SessionSetupScript == "" && len(cfg.SessionLive) == 0 {
		return
	}
	// res.env is the launch plan's projected environment: merging it over the
	// process environment gives the hooks the same environment the launched
	// pane carries (including pool routing), plus GC_SESSION, matching
	// herdr-core's setup env (herdrcore/lifecycle.py:599-613).
	env := append(launch.Plan{Env: res.env}.MergedEnv(os.Environ()), "GC_SESSION="+name)
	// Each hook is bounded like a pre_start hook: the per-hook timeout, capped
	// by whatever remains of the start budget, so a slow setup hook cannot run
	// until the outer deadline cuts the whole op (herdr-core reserves the
	// first-turn margin; herdrcore/lifecycle.py:616-624).
	commands := append([]string(nil), cfg.SessionSetup...)
	labels := make([]string, len(commands))
	for i := range labels {
		labels[i] = fmt.Sprintf("session_setup[%d]", i)
	}
	if cfg.SessionSetupScript != "" {
		commands = append(commands, cfg.SessionSetupScript)
		labels = append(labels, "session_setup_script")
	}
	for _, r := range runHooks(ctx, p.runner, p.hookBatch(ctx, commands, "session_setup", cfg.WorkDir, env)) {
		switch {
		case r.BudgetExhausted:
			p.warnf("%s start budget %s exhausted before the hook ran", labels[r.Index], p.startBudget)
		case r.Err != nil:
			p.warnf("%s %v", labels[r.Index], r.Err)
		}
	}
	if len(cfg.SessionLive) > 0 {
		p.warnf("%s", sessionLiveSkipped)
	}
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
