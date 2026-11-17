// Package provider is the Rex-backed Gas City session provider core. It maps
// gc sessions (city/rig/agent) onto the Rex hierarchy (session/tab/pane) and
// will be wrapped by the RPP v0 op dispatcher in cmd/gc-runtime-rex.
package provider

import (
	"context"
	"strings"

	"github.com/docbobo/rex-gas-city/internal/rex"
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
}

// New builds a Provider for city, driving client.
func New(city string, client rex.Client) *Provider {
	return &Provider{city: city, client: client}
}

// sessionLabel is the Rex session label for this city (D1: one session/city).
func (p *Provider) sessionLabel() string { return "gc:" + p.city }

// parseName splits a gc session name into its rig tab label and agent pane
// label. Rig agents are "<rig>--<agent>"; everything else is town-level.
func parseName(name string) (rig, agent string) {
	if i := strings.Index(name, "--"); i >= 0 {
		return name[:i], name[i+2:]
	}
	return "__town", name
}

// findSession returns the city's Rex session, or "" when it does not exist.
func (p *Provider) findSession(ctx context.Context) (rex.SessionID, error) {
	sessions, err := p.client.ListSessions(ctx)
	if err != nil {
		return "", err
	}
	for _, s := range sessions {
		if s.Label == p.sessionLabel() {
			return s.ID, nil
		}
	}
	return "", nil
}

// findAgentBlock returns the pane running the named agent, or "" when the
// agent has no pane in the city's session.
func (p *Provider) findAgentBlock(ctx context.Context, sid rex.SessionID, name string) (rex.BlockID, error) {
	rig, agent := parseName(name)
	view, err := p.client.SessionView(ctx, sid)
	if err != nil {
		return "", err
	}
	for _, w := range view.Windows {
		if w.Label != rig {
			continue
		}
		for _, b := range w.Blocks {
			if b.Label == agent {
				return b.ID, nil
			}
		}
	}
	return "", nil
}

// Start creates the city session, the rig's tab, and the agent's pane.
func (p *Provider) Start(ctx context.Context, name string, cfg Config) error {
	rig, agent := parseName(name)

	sid, err := p.findSession(ctx)
	if err != nil {
		return err
	}
	if sid == "" {
		sid, err = p.client.CreateSession(ctx, p.sessionLabel())
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
		Label:   agent,
	}
	for _, w := range view.Windows {
		if w.Label == rig && len(w.Blocks) > 0 {
			_, err = p.client.NewSplit(ctx, sid, w.Blocks[0].ID, "horizontal", "after", spec)
			return err
		}
	}
	_, _, err = p.client.NewWindow(ctx, sid, rig, spec)
	return err
}

// Stop closes the agent's pane and leaves the city session and other rigs
// intact. It is a no-op when the agent has no pane.
func (p *Provider) Stop(ctx context.Context, name string) error {
	sid, err := p.findSession(ctx)
	if err != nil || sid == "" {
		return err
	}
	block, err := p.findAgentBlock(ctx, sid, name)
	if err != nil || block == "" {
		return err
	}
	return p.client.CloseBlock(ctx, sid, block)
}

// IsRunning reports whether the agent's program is still running. An agent
// with no pane is not running.
func (p *Provider) IsRunning(ctx context.Context, name string) (bool, error) {
	sid, err := p.findSession(ctx)
	if err != nil || sid == "" {
		return false, err
	}
	block, err := p.findAgentBlock(ctx, sid, name)
	if err != nil || block == "" {
		return false, err
	}
	info, err := p.client.BlockProcess(ctx, block)
	if err != nil {
		return false, err
	}
	return info.LastExit == nil, nil
}

// Peek returns the agent's rendered terminal output. An agent with no pane
// has no output.
func (p *Provider) Peek(ctx context.Context, name string) (string, error) {
	sid, err := p.findSession(ctx)
	if err != nil || sid == "" {
		return "", err
	}
	block, err := p.findAgentBlock(ctx, sid, name)
	if err != nil || block == "" {
		return "", err
	}
	return p.client.BlockFormat(ctx, block)
}
