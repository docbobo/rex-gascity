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
	if strings.TrimSpace(wide) == "" {
		return nil, ErrUnreadableSnapshot
	}
	rows := make(Table)
	eachRow(wide, 2, func(cols []string, rest string) {
		pid, ppid, ok := parseIDs(cols[0], cols[1])
		if !ok {
			return
		}
		args := strings.TrimSpace(rest)
		rows[pid] = Row{PID: pid, PPID: ppid, Args: args, Reliable: reliableArgs(args)}
	})
	for pid, row := range rows {
		if id, ok := ident[pid]; ok {
			row.Comm, row.Start, row.Zombie = id.Comm, id.Start, id.Zombie
			rows[pid] = row
		}
	}
	return rows, nil
}

// lstartLen is the width of ps's lstart column, "%a %b %e %H:%M:%S %Y"
// ("Fri Nov 20 19:44:50 2026"); both BSD and GNU ps print it fixed-width.
const lstartLen = 24

// parseRows reads the identity read. Identity rows carry no wide command line,
// so their Reliable is true; a usable Comm is enough for the fallback rule.
func parseRows(out string) Table {
	rows := make(Table)
	eachRow(out, 3, func(cols []string, rest string) {
		pid, ppid, ok := parseIDs(cols[0], cols[1])
		if !ok {
			return
		}
		comm, start := splitStart(rest)
		rows[pid] = Row{
			PID:      pid,
			PPID:     ppid,
			Zombie:   strings.HasPrefix(cols[2], "Z"),
			Comm:     comm,
			Start:    start,
			Reliable: true,
		}
	})
	return rows
}

// eachRow is the one row parser. It splits every ps line into the first n
// columns plus the untouched remainder, and skips lines that do not split so
// each layout stays a caller concern.
func eachRow(out string, n int, fn func(cols []string, rest string)) {
	for _, line := range strings.Split(out, "\n") {
		if cols, rest, ok := splitRow(line, n); ok {
			fn(cols, rest)
		}
	}
}

// splitRow splits the first n whitespace-delimited columns from the rest of a
// ps line. The remainder is not field-split, because a comm or args value may
// contain spaces.
func splitRow(line string, n int) (cols []string, rest string, ok bool) {
	rest = strings.TrimRight(line, "\r\n")
	cols = make([]string, 0, n)
	for i := 0; i < n; i++ {
		rest = strings.TrimLeft(rest, " \t")
		if rest == "" {
			return nil, "", false
		}
		end := strings.IndexAny(rest, " \t")
		if end < 0 {
			cols = append(cols, rest)
			rest = ""
			continue
		}
		cols = append(cols, rest[:end])
		rest = rest[end:]
	}
	return cols, rest, true
}

// parseIDs decodes the pid and ppid columns both ps layouts share.
func parseIDs(pidStr, ppidStr string) (pid, ppid int, ok bool) {
	pid, err := strconv.Atoi(pidStr)
	if err != nil || pid <= 0 {
		return 0, 0, false
	}
	ppid, err = strconv.Atoi(ppidStr)
	if err != nil {
		return 0, 0, false
	}
	return pid, ppid, true
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
