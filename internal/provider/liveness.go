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

// Liveness is the three-valued answer for the RPP process-alive op. gc's
// reader treats an empty stdout as alive and only an explicit "false" (or an
// error) as dead (gc v1.5.1 internal/runtime/exec/exec.go:709-718), so a
// definite "no match" must be kept distinct from the pack's "cannot tell".
type Liveness int

const (
	// LivenessUnknown is the pack's "cannot tell": the process table could not
	// be read, or an opaque process could not be ruled out. The op answers exit
	// 0 with empty stdout, which gc reads as alive. It is the zero value so a
	// failure to probe defaults to the safe answer.
	LivenessUnknown Liveness = iota
	// LivenessAlive means a process in the pane matched one of the names.
	LivenessAlive
	// LivenessDead means a fully readable process table held no matching
	// process, or the program had exited.
	LivenessDead
)

// processAlive is the strict foreground/ancestor match plus the pack's
// cannot-tell cases. A non-nil LastExit is always dead. With no names, a nil
// LastExit is alive. With names, the foreground process or an ancestor must
// match one: a fully readable table with no match is dead, while an empty
// table or a table holding an opaque (unreadable) process is unknown, because
// we cannot rule out that the agent is that process and gc reads the empty
// answer as alive.
func processAlive(info rex.ProcessInfo, names []string) Liveness {
	if info.LastExit != nil {
		return LivenessDead
	}
	if len(names) == 0 {
		return LivenessAlive
	}
	procs := make([]rex.Process, 0, len(info.Ancestors)+1)
	if info.Foreground != nil {
		procs = append(procs, *info.Foreground)
	}
	procs = append(procs, info.Ancestors...)
	for _, p := range procs {
		if processMatches(p, names) {
			return LivenessAlive
		}
	}
	if len(procs) == 0 {
		return LivenessUnknown
	}
	for _, p := range procs {
		if processOpaque(p) {
			return LivenessUnknown
		}
	}
	return LivenessDead
}

// processOpaque reports whether a process carries no readable identity, so it
// can neither match a name nor be ruled out as the agent.
func processOpaque(p rex.Process) bool {
	return p.Name == "" && p.Path == "" && p.Argv0 == ""
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
// longer knows (its command exited and closed the block) is not running. It
// delegates to the Binder's liveOnly so the provider and the resolver share one
// liveness check.
func (p *Provider) paneAlive(ctx context.Context, b Binding) (bool, error) {
	_, alive, err := p.binder.liveOnly(ctx, b)
	return alive, err
}

// ProcessAlive reports whether one of names is the foreground process or an
// ancestor in the agent's pane. It backs the RPP process-alive op; empty names
// means alive. An agent with no pane is dead. A BlockProcess failure other
// than a vanished pane is LivenessUnknown: gc reads the empty answer as alive,
// which is safer than reporting a transient error as dead.
func (p *Provider) ProcessAlive(ctx context.Context, name string, names []string) (Liveness, error) {
	b, ok, err := p.binder.Resolve(ctx, ResolveRequest{Name: SessionName(name), Mode: ResolveFresh})
	if err != nil || !ok {
		return LivenessDead, err
	}
	info, err := p.client.BlockProcess(ctx, b.SessionID, b.BlockID)
	if errors.Is(err, rex.ErrNotFound) {
		// A pane that vanished between resolve and process has no live agent.
		return LivenessDead, nil
	}
	if err != nil {
		// The process table could not be read (a transient Rex error or a lost
		// reply). Cannot tell: do not report the agent dead.
		return LivenessUnknown, nil
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
