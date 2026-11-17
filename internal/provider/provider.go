// Package provider is the Rex-backed Gas City session provider core. It maps
// gc sessions (city/rig/agent) onto the Rex hierarchy (session/tab/pane) and
// will be wrapped by the RPP v0 op dispatcher in cmd/gc-runtime-rex.
package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/docbobo/rex-gas-city/internal/rex"
)

// ErrSessionNotFound reports that an input op found no live agent for the
// named session: the pane is gone, or its program has exited. The RPP
// dispatcher maps it to a failed op with a clear message.
var ErrSessionNotFound = errors.New("session not found")

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
}

// Provider drives Rex for one gc city.
type Provider struct {
	city   string
	client rex.Client
	binder *Binder
}

// New builds a Provider for city, driving client.
func New(city string, client rex.Client) *Provider {
	return &Provider{
		city:   city,
		client: client,
		binder: NewBinder(client, NewMemoryStore(), rexSessionLabel(city)),
	}
}

// Start creates the city session, the rig's tab, and the agent's pane, and
// records the pane's binding.
func (p *Provider) Start(ctx context.Context, name string, cfg Config) error {
	n := SessionName(name)

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
			p.record(n, sid, w.ID, block)
			return nil
		}
	}
	wid, block, err := p.client.NewWindow(ctx, sid, n.TabLabel(), spec)
	if err != nil {
		return err
	}
	p.record(n, sid, wid, block)
	return nil
}

// record stores a binding for a new pane. A pane with no block ID is not
// recorded.
func (p *Provider) record(n SessionName, sid rex.SessionID, wid rex.WindowID, block rex.BlockID) {
	if block == "" {
		return
	}
	p.binder.Record(Binding{
		Name:         n,
		SessionID:    sid,
		SessionLabel: rexSessionLabel(p.city),
		WindowID:     wid,
		BlockID:      block,
	})
}

// Stop closes the agent's pane and leaves the city session and other rigs
// intact. It is a no-op when the agent has no pane.
func (p *Provider) Stop(ctx context.Context, name string) error {
	binding, ok, err := p.binder.Resolve(ctx, SessionName(name))
	if err != nil || !ok {
		return err
	}
	if err := p.client.CloseBlock(ctx, binding.SessionID, binding.BlockID); err != nil {
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
	info, err := p.client.BlockProcess(ctx, binding.SessionID, binding.BlockID)
	if err != nil {
		return false, err
	}
	return info.LastExit == nil, nil
}

// Peek returns the agent's rendered terminal output. An agent with no pane
// has no output.
func (p *Provider) Peek(ctx context.Context, name string) (string, error) {
	binding, ok, err := p.binder.Resolve(ctx, SessionName(name))
	if err != nil || !ok {
		return "", err
	}
	return p.client.BlockFormat(ctx, binding.SessionID, binding.BlockID)
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
	info, err := p.client.BlockProcess(ctx, binding.SessionID, binding.BlockID)
	if err != nil {
		return Binding{}, err
	}
	if info.LastExit != nil {
		return Binding{}, fmt.Errorf("%w: %s agent program exited", ErrSessionNotFound, name)
	}
	return binding, nil
}
