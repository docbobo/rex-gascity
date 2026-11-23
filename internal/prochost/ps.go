package prochost

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
)

// CommandRunner is the exact method set of provider.Runner, so ExecRunner and
// FakeRunner satisfy it without importing the provider package.
type CommandRunner interface {
	Run(ctx context.Context, dir string, env []string, name string, args ...string) (string, error)
}

// ErrUnreadableSnapshot is returned when needArgs is set and the wide read is
// empty or failed. It is never an identity-only table: an empty wide snapshot
// must not read as "no process matched".
var ErrUnreadableSnapshot = errors.New("prochost: unreadable process snapshot")

// PSHost is the real Host adapter over ps and a signal.
type PSHost struct {
	cmd CommandRunner
}

// NewPSHost builds the real adapter.
func NewPSHost(cmd CommandRunner) *PSHost { return &PSHost{cmd: cmd} }

// Table runs the identity read always and the wide read only when needArgs.
// -ww stays on the args read only, because only the args column can be cut at
// COLUMNS.
func (h *PSHost) Table(ctx context.Context, needArgs bool) (Table, error) {
	out, err := h.cmd.Run(ctx, "", nil, "ps", "-axo", "pid=,ppid=,stat=,comm=,lstart=")
	if err != nil {
		return nil, err
	}
	ident := parseRows(out)
	if !needArgs {
		return ident, nil
	}
	wide, err := h.cmd.Run(ctx, "", nil, "ps", "-ww", "-axo", "pid=,ppid=,args=")
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnreadableSnapshot, err)
	}
	return mergeRows(ident, wide)
}

// Signal delivers sig to pid. Panes are local to this host, so a plain process
// signal is the only reap mechanism Rex leaves the pack.
func (h *PSHost) Signal(pid int, sig syscall.Signal) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return proc.Signal(sig)
}

// mergeRows applies the merge rule: wide rows supply membership and PPIDs, and
// identity rows add Comm, Start and Zombie. A wide-only row stays. An
// identity-only row drops.
func mergeRows(ident Table, wide string) (Table, error) {
	type wideRow struct {
		ppid int
		args string
	}
	rows := make(map[int]wideRow)
	if strings.TrimSpace(wide) == "" {
		return nil, ErrUnreadableSnapshot
	}
	for _, line := range strings.Split(wide, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		pid, pidErr := strconv.Atoi(fields[0])
		ppid, ppidErr := strconv.Atoi(fields[1])
		if pidErr != nil || ppidErr != nil || pid <= 0 {
			continue
		}
		rows[pid] = wideRow{ppid: ppid, args: strings.Join(fields[2:], " ")}
	}
	out := make(Table, len(rows))
	for pid, w := range rows {
		row := Row{PID: pid, PPID: w.ppid, Args: w.args, Reliable: reliableArgs(w.args)}
		if id, ok := ident[pid]; ok {
			row.Comm, row.Start, row.Zombie = id.Comm, id.Start, id.Zombie
		}
		out[pid] = row
	}
	return out, nil
}

// lstartLen is the width of ps's lstart column, "%a %b %e %H:%M:%S %Y"
// ("Fri Nov 20 19:44:50 2026"); both BSD and GNU ps print it fixed-width.
const lstartLen = 24

// parseRows reads the identity read. Identity rows carry no wide command line,
// so their Reliable is true; a usable Comm is enough for the fallback rule.
func parseRows(out string) Table {
	rows := make(Table)
	for _, line := range strings.Split(out, "\n") {
		pidStr, ppidStr, stat, comm, start, ok := splitProcessRow(line)
		if !ok {
			continue
		}
		pid, pidErr := strconv.Atoi(pidStr)
		ppid, ppidErr := strconv.Atoi(ppidStr)
		if pidErr != nil || ppidErr != nil || pid <= 0 {
			continue
		}
		rows[pid] = Row{
			PID:      pid,
			PPID:     ppid,
			Zombie:   strings.HasPrefix(stat, "Z"),
			Comm:     comm,
			Start:    start,
			Reliable: true,
		}
	}
	return rows
}

// splitProcessRow splits one ps row into its three fixed columns and the rest
// of the line, which is the command name and, when ps printed it, the trailing
// lstart column. The name may be empty or contain spaces, so it must not be
// field-split like the pid and ppid.
func splitProcessRow(line string) (pid, ppid, stat, comm, start string, ok bool) {
	var col [3]string
	rest := strings.TrimRight(line, "\r\n")
	for i := range col {
		rest = strings.TrimLeft(rest, " \t")
		if rest == "" {
			return "", "", "", "", "", false
		}
		end := strings.IndexAny(rest, " \t")
		if end < 0 {
			col[i], rest = rest, ""
			continue
		}
		col[i], rest = rest[:end], rest[end:]
	}
	comm, start = splitStart(rest)
	return col[0], col[1], col[2], comm, start, true
}

// splitStart separates the trailing lstart column from the command name. A
// row without one keeps the whole rest as the name.
func splitStart(rest string) (comm, start string) {
	rest = strings.TrimSpace(rest)
	if len(rest) >= lstartLen {
		tail := rest[len(rest)-lstartLen:]
		if f := strings.Fields(tail); len(f) == 5 && strings.Count(f[3], ":") == 2 {
			return strings.TrimSpace(rest[:len(rest)-lstartLen]), tail
		}
	}
	return rest, ""
}
