package provider

import (
	"context"
	"errors"
	"path/filepath"

	"github.com/docbobo/rex-gascity/internal/rex"
)

// processAlive reports whether the agent program is running in a pane. A
// non-nil LastExit is always dead. With no names, a nil LastExit is alive.
// With names, the foreground process or an ancestor must match one; the check
// is strict, so a pane that reports no matching process is dead.
func processAlive(info rex.ProcessInfo, names []string) bool {
	if info.LastExit != nil {
		return false
	}
	if len(names) == 0 {
		return true
	}
	if info.Foreground != nil && processMatches(*info.Foreground, names) {
		return true
	}
	for _, a := range info.Ancestors {
		if processMatches(a, names) {
			return true
		}
	}
	return false
}

// processMatches reports whether p is one of names, by name, argv0, or the
// basename of its path or argv0. Empty names never match.
func processMatches(p rex.Process, names []string) bool {
	candidates := []string{p.Name, p.Argv0}
	if p.Path != "" {
		candidates = append(candidates, filepath.Base(p.Path))
	}
	if p.Argv0 != "" {
		candidates = append(candidates, filepath.Base(p.Argv0))
	}
	for _, name := range names {
		if name == "" {
			continue
		}
		for _, c := range candidates {
			if c == name {
				return true
			}
		}
	}
	return false
}

// paneAlive reports whether the pane of b still has a live terminal. It only
// inspects LastExit: a live probe showed Rex runs a launched command as the
// default shell with argv0 set to the command, so process-name heuristics
// misclassify live panes. last_exit is the v1 liveness truth. A pane Rex no
// longer knows (its command exited and closed the block) is not running.
func (p *Provider) paneAlive(ctx context.Context, b Binding) (bool, error) {
	info, err := p.client.BlockProcess(ctx, b.SessionID, b.BlockID)
	if errors.Is(err, rex.ErrNotFound) {
		// A command that exits closes its block, so a missing pane is dead.
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return info.LastExit == nil, nil
}

// ProcessAlive reports whether one of names is the foreground process or an
// ancestor in the agent's pane. It backs the RPP process-alive op; empty names
// means alive. An agent with no pane is not alive.
func (p *Provider) ProcessAlive(ctx context.Context, name string, names []string) (bool, error) {
	b, ok, err := p.binder.Resolve(ctx, SessionName(name))
	if err != nil || !ok {
		return false, err
	}
	info, err := p.client.BlockProcess(ctx, b.SessionID, b.BlockID)
	if err != nil {
		return false, err
	}
	return processAlive(info, names), nil
}
