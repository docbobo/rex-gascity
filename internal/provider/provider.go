// Package provider is the Rex-backed Gas City session provider core. It maps
// gc sessions (city/rig/agent) onto the Rex hierarchy (session/tab/pane) and
// will be wrapped by the RPP v0 op dispatcher in cmd/gc-runtime-rex.
package provider

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/docbobo/rex-gas-city/internal/rex"
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

// Binding returns the stored binding for name without contacting Rex. It is
// the dispatcher-facing lookup; Resolve-based ops re-validate it against Rex.
func (p *Provider) Binding(name string) (Binding, bool) {
	return p.bindings.Load(SessionName(name))
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

func (p *Provider) start(ctx context.Context, name string, cfg Config) error {
	n := SessionName(name)

	old, found, err := p.binder.Resolve(ctx, n)
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
	spec := rex.LaunchSpec{
		Command: cfg.Argv,
		Cwd:     cfg.WorkDir,
		Label:   n.PaneLabel(),
	}
	for _, w := range view.Windows {
		if w.Label == n.TabLabel() && len(w.Blocks) > 0 {
			block, err := p.client.NewSplit(ctx, sid, w.Blocks[0].ID, "horizontal", "after", spec)
			if err != nil {
				return err
			}
			p.record(ctx, n, sid, w.ID, block, "split", cfg)
			return nil
		}
	}
	wid, block, err := p.client.NewWindow(ctx, sid, n.TabLabel(), spec)
	if err != nil {
		return err
	}
	p.record(ctx, n, sid, wid, block, "window", cfg)
	return nil
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
		LaunchMode:   mode,
		CreatedAt:    time.Now(),
		ReadyPrompt:  ready,
	})
}

// Stop closes the agent's pane and leaves the city session and other rigs
// intact. It is a no-op when the agent has no pane, and a pane that vanished
// already counts as stopped. Closing a rig's last pane removes its tab.
func (p *Provider) Stop(ctx context.Context, name string) error {
	binding, ok, err := p.binder.Resolve(ctx, SessionName(name))
	if err != nil || !ok {
		return err
	}
	if err := p.client.CloseBlock(ctx, binding.SessionID, binding.BlockID); err != nil && !errors.Is(err, rex.ErrNotFound) {
		return err
	}
	p.binder.Forget(SessionName(name))
	return nil
}

// IsRunning reports whether the agent's program is still running. An agent
// with no pane is not running.
func (p *Provider) IsRunning(ctx context.Context, name string) (bool, error) {
	binding, ok, err := p.binder.Resolve(ctx, SessionName(name))
	if err != nil || !ok {
		return false, err
	}
	return p.paneAlive(ctx, binding)
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
		name := string(b.Name)
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		alive, err := p.paneAlive(ctx, b)
		if errors.Is(err, rex.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if alive {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names, nil
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
