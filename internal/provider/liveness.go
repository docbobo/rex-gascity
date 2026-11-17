package provider

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/docbobo/rex-gascity/internal/rex"
)

// procNameAliases mirrors gc's process-name aliases (runtime-herdr-core
// herdrcore/procs.py NAME_ALIASES): a configured hint also matches these names.
var procNameAliases = map[string][]string{"kimi": {"kimi-code"}}

// gcBinBasename returns the basename of the GC_BIN the agent was launched with,
// or "" when the start config carried none.
func gcBinBasename(env map[string]string) string {
	if v := env["GC_BIN"]; v != "" {
		return filepath.Base(v)
	}
	return ""
}

// expandProcessNames returns names plus gc's aliases. Every name is kept and
// its alias set added, and the literal "gc" hint also matches the wrapped
// binary gc launched under (basename of GC_BIN, e.g. ".gc-wrapped"). This
// mirrors runtime-herdr-core's expand_names (herdrcore/procs.py), which is what
// keeps gc's process-alive observation true for a wrapped gc and stops the
// reconciler from reaping a live control dispatcher as a dead seat.
func expandProcessNames(names []string, gcBin string) []string {
	out := make([]string, 0, len(names)*2+1)
	seen := make(map[string]bool, len(names)*2+1)
	add := func(n string) {
		if n == "" || seen[n] {
			return
		}
		seen[n] = true
		out = append(out, n)
	}
	for _, n := range names {
		n = strings.TrimSpace(n)
		add(n)
		for _, alias := range procNameAliases[n] {
			add(alias)
		}
		if n == "gc" && gcBin != "" {
			add(gcBin)
		}
	}
	return out
}

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
	// Legacy bindings started before GCBin was persisted carry none; fall back
	// to the provider's own environment so an already-running wrapped gc is
	// still recognised (herdr-core reads gc_bin from stored meta).
	gcBin := b.GCBin
	if gcBin == "" {
		gcBin = gcBinBasename(map[string]string{"GC_BIN": os.Getenv("GC_BIN")})
	}
	return processAlive(info, expandProcessNames(names, gcBin)), nil
}
