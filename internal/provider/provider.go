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

// Start creates the city session, the rig's tab, and the agent's pane.
func (p *Provider) Start(ctx context.Context, name string, cfg Config) error {
	rig, agent := parseName(name)

	sessions, err := p.client.ListSessions(ctx)
	if err != nil {
		return err
	}
	var sid rex.SessionID
	for _, s := range sessions {
		if s.Label == p.sessionLabel() {
			sid = s.ID
			break
		}
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
