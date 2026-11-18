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
const readyPromptMetaKey = "GC_READY_PROMPT_PREFIX"

const (
	submitKey    = "enter"
	interruptKey = "ctrl+c"
)

// Config is the launch configuration the RPP `start` op carries as JSON.
type Config struct {
	Argv    []string
	WorkDir string
	// ReadyPromptPrefix is the prompt text the agent shows when ready. When
	// empty, Start falls back to the GC_READY_PROMPT_PREFIX meta key.
	ReadyPromptPrefix string
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

	// stateDir is the root state dir set by WithStateDir; it holds the
	// cross-process lock file. Empty means no cross-process locking.
	stateDir string
}

// Option configures a Provider.
type Option func(*Provider)

// WithBindingStore sets the binding store.
func WithBindingStore(s BindingStore) Option { return func(p *Provider) { p.bindings = s } }

// WithActivityStore sets the activity store.
func WithActivityStore(s ActivityStore) Option { return func(p *Provider) { p.activity = s } }

// WithMetaStore sets the metadata store.
func WithMetaStore(s MetaStore) Option { return func(p *Provider) { p.meta = s } }

// WithStateDir keeps bindings, activity and metadata on disk under
// dir/bindings, dir/activity and dir/meta.
func WithStateDir(dir string) Option {
	return func(p *Provider) {
		p.stateDir = dir
		p.bindings = NewFileStore(filepath.Join(dir, "bindings"))
		p.activity = NewFileActivityStore(filepath.Join(dir, "activity"))
		p.meta = NewFileMetaStore(filepath.Join(dir, "meta"))
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
	}
	for _, opt := range opts {
		opt(p)
	}
	p.binder = NewBinder(client, p.bindings, rexSessionLabel(city))
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
func (p *Provider) Start(ctx context.Context, name string, cfg Config) error {
	return p.withStateLock(ctx, func() error { return p.start(ctx, name, cfg) })
}

// StartNew starts the agent like Start, but fails with ErrAlreadyRunning when
// its pane is already live. The liveness check and the launch run under one
// state lock, so of two concurrent StartNew calls for one name sharing a state
// dir, exactly one launches and the other sees ErrAlreadyRunning.
func (p *Provider) StartNew(ctx context.Context, name string, cfg Config) error {
	return p.withStateLock(ctx, func() error {
		running, err := p.IsRunning(ctx, name)
		if err != nil {
			return err
		}
		if running {
			return fmt.Errorf("%w: %s", ErrAlreadyRunning, name)
		}
		return p.start(ctx, name, cfg)
	})
}

func (p *Provider) start(ctx context.Context, name string, cfg Config) (err error) {
	n := SessionName(name)
	// gc's identity is passed through unchanged. For an unaliased pool or
	// demand seat gc already ships the concrete identity: GC_ALIAS is empty,
	// and GC_AGENT/BEADS_ACTOR are the session bead id (RuntimeEnvWithSessionContext,
	// internal/session/lifecycle.go:67-78, merged at cmd/gc/session_lifecycle_parallel.go:1261).
	// Synthesizing a claimable GC_ALIAS from GC_TEMPLATE is wrong: gc's claim
	// writer hookClaimAssigneeIdentity (cmd/gc/cmd_hook.go:866 v1.5.1) records
	// the claim under alias first, so a bare-template alias makes every pool
	// slot claim under one shared string no open session bead answers to, and
	// releaseOrphanedPoolAssignments (cmd/gc/pool_session_name.go:211) reopens
	// the work (GCREX-32). launchCommand turns the empty GC_ALIAS into
	// `env -u GC_ALIAS`, matching RPP and gc's tmux adapter.
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
		return err
	}
	if found {
		alive, err := p.paneAlive(ctx, old)
		if err != nil {
			return err
		}
		if alive {
			return nil
		}
		if err := p.client.CloseBlock(ctx, old.SessionID, old.BlockID); err != nil && !errors.Is(err, rex.ErrNotFound) {
			return err
		}
		p.binder.Forget(n)
	}

	sid, ok, err := p.binder.FindSession(ctx)
	if err != nil {
		return err
	}
	if !ok {
		sid, err = p.client.CreateSession(ctx, rexSessionLabel(p.city))
		if err != nil {
			return err
		}
	}

	view, err := p.client.SessionView(ctx, sid)
	if err != nil {
		return err
	}

	// Run pre_start hooks before seeding identity or a provisional binding,
	// so a failed hook leaves nothing behind.
	if len(cfg.PreStart) > 0 {
		merged := mergeEnv(os.Environ(), cfg.Env)
		for i, cmd := range cfg.PreStart {
			if err := runPreStart(ctx, cmd, cfg.WorkDir, merged); err != nil {
				return fmt.Errorf("pre_start[%d]: %w", i, err)
			}
		}
	}
	// A pre_start hook is the documented way to create the launch work dir
	// (the gastown worktree recipe points work_dir at a worktree that
	// pre_start creates). Mirror runtime-herdr-core: once the hooks have run,
	// a configured work_dir must exist or the launch would run elsewhere.
	if cfg.WorkDir != "" {
		if info, statErr := os.Stat(cfg.WorkDir); statErr != nil || !info.IsDir() {
			return fmt.Errorf("work_dir %q is not an existing directory after pre_start", cfg.WorkDir)
		}
	}

	// Seed the identity and a provisional binding before the launch call,
	// which blocks while Rex dismisses startup dialogs. A reconciler tick in
	// that window can then read the current GC_SESSION_ID/GC_INSTANCE_TOKEN
	// instead of an empty identity and must not roll the pending create back.
	seeded = true
	if err := p.seedStartIdentity(n, cfg.Env); err != nil {
		return err
	}
	p.recordProvisionalBinding(n, sid, "", cfg)

	// Deliver the startup nudge after a successful launch, using the
	// just-recorded binding's SessionID/BlockID rather than a fresh
	// livePane re-resolve, so the nudge does not race the pane
	// program's readiness.
	var launchedBlock rex.BlockID
	defer func() {
		if err != nil || cfg.Nudge == "" || launchedBlock == "" {
			return
		}
		// Best effort, like the nudge op: the pane launched, so a delivery
		// failure must not fail the start and let gc tear down a healthy
		// pane. gc's queued-nudge backstop covers an undelivered first
		// prompt.
		if wErr := p.client.BlockWrite(ctx, sid, launchedBlock, cfg.Nudge); wErr == nil {
			_ = p.client.SendKeys(ctx, sid, launchedBlock, submitKey)
		}
	}()

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
				return err
			}
			p.record(ctx, n, sid, w.ID, block, "split", cfg)
			launchedBlock = block
			return nil
		}
	}
	wid, block, err := p.client.NewWindow(ctx, sid, tab, spec)
	if err != nil {
		return err
	}
	p.record(ctx, n, sid, wid, block, "window", cfg)
	launchedBlock = block
	return nil
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
	ready := cfg.ReadyPromptPrefix
	if ready == "" {
		// Best effort: a meta read failure must not fail Start.
		ready, _, _ = p.meta.Get(n, readyPromptMetaKey)
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
		ReadyPrompt:  ready,
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
	binding, ok, err := p.binder.Resolve(ctx, n)
	if err != nil {
		return err
	}
	if !ok {
		// Even with no pane, a seed left by a failed or rolled-back start must
		// not outlive its life.
		return p.clearStartIdentity(n)
	}
	if err := p.client.CloseBlock(ctx, binding.SessionID, binding.BlockID); err != nil && !errors.Is(err, rex.ErrNotFound) {
		return err
	}
	p.writeDrainDiagnostic(ctx, binding)
	if err := p.clearStartIdentity(binding.Name); err != nil {
		return err
	}
	p.binder.Forget(binding.Name)
	return nil
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

// Nudge types text into the agent's pane and submits it. The text and the
// submit are two steps: a pane that closes between them is reported, not
// silently treated as a delivered nudge. The submit is confirmed at the
// transport: Rex returns from send-key once the pane has received the key, so
// a nil error means the Enter keystroke reached the pane. Whether the agent
// acted on it needs idle/program_status and is Phase 3 work.
func (p *Provider) Nudge(ctx context.Context, name, text string) error {
	if text == "" {
		return nil
	}
	binding, err := p.livePane(ctx, name)
	if err != nil {
		return err
	}
	if err := p.client.BlockWrite(ctx, binding.SessionID, binding.BlockID, text); err != nil {
		return fmt.Errorf("nudge %s: type text: %w", name, err)
	}
	if err := p.client.SendKeys(ctx, binding.SessionID, binding.BlockID, submitKey); err != nil {
		return fmt.Errorf("nudge %s: submit key: %w", name, err)
	}
	return nil
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
