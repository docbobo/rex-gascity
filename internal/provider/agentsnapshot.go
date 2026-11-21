package provider

import (
	"context"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/docbobo/rex-gascity/internal/rex"
)

// gc's agent verdicts on the wire (docs/reference/exec-session-provider.md,
// "Agent liveness"). Only alive and dead are certain; unknown is omitted from
// the answer, because gc reads an absent agent as "cannot tell", never dead.
const (
	agentAlive   = "alive"
	agentDead    = "dead"
	agentUnknown = "unknown"
)

// snapshotInterpreters are the programs gc's matcher looks through to the
// argument that names the process (gc internal/runtime/proctable/names.go).
var snapshotInterpreters = map[string]bool{"node": true, "bun": true, "npx": true, "deno": true}

// snapshotRunnerArgs are the interpreter subcommands gc skips before the
// argument that names the process.
var snapshotRunnerArgs = map[string]bool{"run": true, "exec": true, "x": true}

// snapshotValueArgs are interpreter options whose value is the next argument,
// so the script operand after them is not that value. Node, bun and deno
// accept both; the set covers the options that precede a script in practice.
var snapshotValueArgs = map[string]bool{
	"-r": true, "--require": true,
	"-e": true, "--eval": true, "-p": true, "--print": true,
	"--import": true, "--loader": true, "--experimental-loader": true,
	"--preload": true, "--config": true, "-c": true,
	"--import-map": true, "-C": true, "--conditions": true,
	"--inspect-port": true, "--title": true,
	"--max-old-space-size": true, "--max-semi-space-size": true,
	"-L": true, "--log-level": true, "--cert": true, "--key": true,
}

// snapshotMainThreadInterpreter reports whether argv0 is Node's main thread
// name, which carries the interpreter name plus "-MainThread". Node 25 names
// its main thread, and the thread name can reach ps as argv0, so the script
// operand must still be found behind it.
func snapshotMainThreadInterpreter(argv0 string) bool {
	name, ok := strings.CutSuffix(argv0, "-MainThread")
	return ok && snapshotInterpreters[name]
}

// snapshotProcessTable is gc's one-pass view of the host process table: a wide
// snapshot of pid, ppid and command line, and a narrow one of pid and command
// name. -ww is load-bearing: without it procps cuts the command line at
// COLUMNS, and a cut line basenames to nothing, which reads a live agent dead.
// The bare pid/ppid/args columns keep the wide row unambiguous, and the narrow
// row carries the command name, which a 16-byte comm column can truncate.
func (p *Provider) snapshotProcessTable(ctx context.Context) (string, string, error) {
	wide, err := p.runner.Run(ctx, "", nil, "ps", "-ww", "-axo", "pid=,ppid=,args=")
	if err != nil {
		return "", "", err
	}
	comms, err := p.runner.Run(ctx, "", nil, "ps", "-axo", "pid=,comm=")
	if err != nil {
		return "", "", err
	}
	return wide, comms, nil
}

// snapshotAgent answers gc's agent state for the box whose root process is
// root, from a snapshot the caller already took. It walks the root and every
// descendant (a Rex pane's foreground process can be the agent itself) and
// matches each one the way gc's table does. It answers unknown, never dead,
// whenever it cannot be sure: no names, an unreadable or empty snapshot, a root
// missing from the snapshot (its subtree is not observable), or an opaque
// descendant that could be the agent. names must already be expanded.
func snapshotAgent(wide, comms string, root int, names []string) (string, int) {
	if root <= 0 || len(names) == 0 {
		return agentUnknown, 0
	}
	rows, ok := parseWideRows(wide)
	if !ok {
		return agentUnknown, 0
	}
	if _, ok := rows[root]; !ok {
		return agentUnknown, 0
	}
	commByPID := parseCommRows(comms)
	children := make(map[int][]int, len(rows))
	for pid, r := range rows {
		children[r.ppid] = append(children[r.ppid], pid)
	}
	opaque := false
	queue := []int{root}
	for i := 0; i < len(queue); i++ {
		pid := queue[i]
		proc, reliable := snapshotProcess(rows[pid].args, commByPID[pid])
		if processMatches(proc, names) {
			return agentAlive, pid
		}
		if !reliable || processOpaque(proc) {
			opaque = true
		}
		queue = append(queue, children[pid]...)
	}
	if opaque {
		return agentUnknown, 0
	}
	return agentDead, 0
}

// snapshotRow is one row of the wide process snapshot.
type snapshotRow struct {
	ppid int
	args string
}

// parseWideRows reads the pid, ppid and command line of every row. It reports
// false for an empty snapshot: nothing was read, so nothing can be ruled out.
func parseWideRows(out string) (map[int]snapshotRow, bool) {
	rows := make(map[int]snapshotRow)
	if strings.TrimSpace(out) == "" {
		return rows, false
	}
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		pid, pidErr := strconv.Atoi(fields[0])
		ppid, ppidErr := strconv.Atoi(fields[1])
		if pidErr != nil || ppidErr != nil || pid <= 0 {
			continue
		}
		rows[pid] = snapshotRow{ppid: ppid, args: strings.Join(fields[2:], " ")}
	}
	return rows, true
}

// pidOutsideSubtree reports whether pid is a live row of the snapshot that is
// not root and not one of root's descendants. An absent pid is not outside: it
// may have exited, and an exited agent is exactly what the zombie check looks
// for. An empty snapshot is not outside either, so the caller's own proof
// requirement still decides.
func pidOutsideSubtree(wide string, root, pid int) bool {
	rows, ok := parseWideRows(wide)
	if !ok || root <= 0 || pid <= 0 {
		return false
	}
	row, ok := rows[pid]
	if !ok {
		return false
	}
	for i := 0; i <= len(rows); i++ {
		if pid == root {
			return false
		}
		if pid <= 1 {
			return true
		}
		parent, ok := rows[row.ppid]
		if !ok {
			return true
		}
		pid, row = row.ppid, parent
	}
	return false
}

// parseCommRows reads the pid and command name of every row of the narrow
// snapshot. A row that does not parse carries no name and is skipped.
func parseCommRows(out string) map[int]string {
	rows := make(map[int]string)
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		pid, err := strconv.Atoi(fields[0])
		if err != nil || pid <= 0 {
			continue
		}
		rows[pid] = fields[1]
	}
	return rows
}

// snapshotProcess turns a snapshot row into the process identity gc's matcher
// reads: the command name, the argv0 basename, and, for an interpreter, the
// first argument that names the program it runs. The bool is false when a
// value-taking option has no value: the script operand is then unidentifiable,
// and the caller must read the row as opaque, never as proof of death.
func snapshotProcess(args, comm string) (rex.Process, bool) {
	var proc rex.Process
	if comm != "" {
		proc.Name = filepath.Base(comm)
	}
	// ps brackets the command line when it cannot read it (a kernel thread, a
	// zombie). The brackets are not an argv0, and a live agent in that state
	// must not read as absent.
	if strings.HasPrefix(strings.TrimSpace(args), "[") {
		return proc, false
	}
	fields := strings.Fields(args)
	if len(fields) == 0 {
		return proc, true
	}
	argv0 := filepath.Base(fields[0])
	proc.Argv0 = argv0
	if !snapshotInterpreters[argv0] && !snapshotMainThreadInterpreter(argv0) {
		return proc, true
	}
	for i := 1; i < len(fields); i++ {
		arg := fields[i]
		if snapshotValueArgs[arg] {
			if i+1 >= len(fields) || strings.HasPrefix(fields[i+1], "-") {
				return proc, false
			}
			i++
			continue
		}
		if strings.HasPrefix(arg, "-") || snapshotRunnerArgs[arg] {
			continue
		}
		proc.Script = arg
		return proc, true
	}
	return proc, true
}

// agentState takes gc's snapshot and answers the box's agent state. An
// unreadable snapshot is unknown, never dead.
func (p *Provider) agentState(ctx context.Context, root int, names []string) (string, int) {
	if root <= 0 || len(names) == 0 {
		return agentUnknown, 0
	}
	wide, comms, err := p.snapshotProcessTable(ctx)
	if err != nil {
		return agentUnknown, 0
	}
	return snapshotAgent(wide, comms, root, names)
}
