package prochost

import (
	"context"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
)

// Row is one host process, merged from the identity and command-line reads.
// Reliable is false for a bracketed command line ([kernel] style) or when a
// value-taking interpreter flag has no following value. This bit lets the
// observe path reproduce `!reliable || processOpaque(proc)`: the parser sets
// it, so the retired narrow `ps -axo pid=,comm=` read is no longer needed.
type Row struct {
	PID      int
	PPID     int
	Zombie   bool
	Comm     string // ps comm, which may be a path
	Start    string // ps lstart, empty when ps printed none
	Args     string // wide command line, empty when no read supplied it
	Reliable bool   // false for bracketed args or unresolved interpreter flag
}

// Table is the host process table keyed by pid.
type Table map[int]Row

// Ident is what a caller recorded about a process before it acted on it.
type Ident struct {
	PID   int
	Name  string
	Start string
}

// Host is the one seam over the host process table. needArgs=false (reap
// path) runs only the identity read, so the ~80 living() polls inside
// waitGone do not amplify ps calls. needArgs=true (observe path) runs both.
type Host interface {
	Table(ctx context.Context, needArgs bool) (Table, error)
	Signal(pid int, sig syscall.Signal) error
}

// interpreters are the programs whose first non-option argument names the
// process. This is gc's set (internal/runtime/proctable/names.go).
var interpreters = map[string]bool{"node": true, "bun": true, "npx": true, "deno": true}

// runnerArgs are the interpreter subcommands skipped before the script operand.
var runnerArgs = map[string]bool{"run": true, "exec": true, "x": true}

// valueArgs are interpreter options whose value is the next argument, so the
// script operand after them is not that value.
var valueArgs = map[string]bool{
	"-r": true, "--require": true,
	"-e": true, "--eval": true, "-p": true, "--print": true,
	"--import": true, "--loader": true, "--experimental-loader": true,
	"--preload": true, "--config": true, "-c": true,
	"--import-map": true, "-C": true, "--conditions": true,
	"--inspect-port": true, "--title": true,
	"--max-old-space-size": true, "--max-semi-space-size": true,
	"-L": true, "--log-level": true, "--cert": true, "--key": true,
}

// Candidates builds the exact name list for a process identity: name, argv0,
// the path and argv0 base names, and the interpreter script with and without
// its extension.
func Candidates(name, argv0, path, script string) []string {
	out := []string{name, argv0}
	if path != "" {
		out = append(out, filepath.Base(path))
	}
	if argv0 != "" {
		out = append(out, filepath.Base(argv0))
	}
	if script != "" {
		base := filepath.Base(script)
		out = append(out, base)
		if ext := filepath.Ext(base); ext != "" {
			out = append(out, strings.TrimSuffix(base, ext))
		}
	}
	return out
}

// Match is the single name-matching rule: exact candidate equality. Empty and
// "?" never match, and there is no suffix match, so sh does not match bash.
func Match(candidates []string, names []string) bool {
	for _, c := range candidates {
		if c == "" || c == "?" {
			continue
		}
		for _, n := range names {
			if n == "" || n == "?" {
				continue
			}
			if c == n {
				return true
			}
		}
	}
	return false
}

// RowCandidates builds the candidates for a ps row: the comm base name, the
// args argv0 base name, and the interpreter script base with and without its
// extension. The caller checks r.Reliable separately.
func RowCandidates(r Row) []string {
	var name, argv0, script string
	if r.Comm != "" {
		name = filepath.Base(r.Comm)
	}
	if args := strings.TrimSpace(r.Args); args != "" && !strings.HasPrefix(args, "[") {
		fields := strings.Fields(args)
		argv0 = filepath.Base(fields[0])
		if isInterpreter(argv0) {
			script, _ = interpreterScript(fields)
		}
	}
	return Candidates(name, argv0, "", script)
}

// Same reports whether the row is still the captured process. A zombie is
// never the living target. When both sides carry a start time it decides;
// otherwise the exact name rule decides. Different starts on the same pid are
// a different process.
func Same(id Ident, r Row) bool {
	if r.Zombie {
		return false
	}
	if id.Start != "" && r.Start != "" {
		return id.Start == r.Start
	}
	return Match(RowCandidates(r), []string{id.Name})
}

// Subtree returns each root plus every process below it. A root missing from
// the table is kept, because Rex reported it.
func Subtree(t Table, roots []int) []int {
	below := make(map[int]bool, len(roots))
	for _, pid := range roots {
		if pid > 0 {
			below[pid] = true
		}
	}
	for changed := true; changed; {
		changed = false
		for pid, row := range t {
			if below[pid] || !below[row.PPID] {
				continue
			}
			below[pid] = true
			changed = true
		}
	}
	out := make([]int, 0, len(below))
	for pid := range below {
		out = append(out, pid)
	}
	sort.Ints(out)
	return out
}

// AncestorIn reports whether one of the set is above pid in the table. It
// stops at init or a cycle.
func AncestorIn(t Table, pid int, set map[int]bool) bool {
	seen := make(map[int]bool)
	row, ok := t[pid]
	for ok {
		if row.PPID <= 1 || seen[row.PPID] {
			return false
		}
		seen[row.PPID] = true
		if set[row.PPID] {
			return true
		}
		row, ok = t[row.PPID]
	}
	return false
}

// isInterpreter reports whether argv0 is an interpreter, including Node's
// "<name>-MainThread" thread name.
func isInterpreter(argv0 string) bool {
	if interpreters[argv0] {
		return true
	}
	name, ok := strings.CutSuffix(argv0, "-MainThread")
	return ok && interpreters[name]
}

// interpreterScript returns the first non-option argument after the
// interpreter. ok is false when a value-taking option has no value, so the
// script operand is unidentifiable and the row is not reliable.
func interpreterScript(fields []string) (script string, ok bool) {
	for i := 1; i < len(fields); i++ {
		arg := fields[i]
		if valueArgs[arg] {
			if i+1 >= len(fields) || strings.HasPrefix(fields[i+1], "-") {
				return "", false
			}
			i++
			continue
		}
		if strings.HasPrefix(arg, "-") || runnerArgs[arg] {
			continue
		}
		return arg, true
	}
	return "", true
}

// reliableArgs reports whether a ps args value can prove a process identity:
// false when ps bracketed it or an interpreter option lost its value.
func reliableArgs(args string) bool {
	args = strings.TrimSpace(args)
	if strings.HasPrefix(args, "[") {
		return false
	}
	fields := strings.Fields(args)
	if len(fields) == 0 || !isInterpreter(filepath.Base(fields[0])) {
		return true
	}
	_, ok := interpreterScript(fields)
	return ok
}
