package provider

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/docbobo/rex-gascity/internal/rex"
)

// Reaping bounds. herdr-core reaps the pane's subtree TERM then KILL two
// seconds later and rescans up to three times (herdrcore/lifecycle.py:327-368,
// herdrcore/procs.py:639-643). The vars exist so tests can shorten them.
var (
	reapTermWait = 2 * time.Second
	reapKillWait = 2 * time.Second
	reapPoll     = 50 * time.Millisecond
)

// paneProc is one process the pane could leave behind: the pid and the name
// the pane's own process tree reported for it.
type paneProc struct {
	pid  int
	name string
}

// survivorError fails a stop whose pane subtree may still be running. herdr-
// core keeps the record for a retry and exits 1 with this wording
// (herdrcore/lifecycle.py:523-524); the binding, meta and activity stay too.
type survivorError struct {
	name      string
	survivors []paneProc
	reason    error
}

func (e *survivorError) Error() string {
	reason := "the process table is unreadable"
	if e.reason != nil {
		reason = e.reason.Error()
	}
	if len(e.survivors) > 0 {
		names := make([]string, len(e.survivors))
		for i, s := range e.survivors {
			names[i] = fmt.Sprintf("%d (%s)", s.pid, s.name)
		}
		reason = strings.Join(names, ", ")
		if e.reason != nil {
			reason += ": " + e.reason.Error()
		}
	}
	return fmt.Sprintf("stopped '%s', but descendants may survive (the pane stays for a retry): %s", e.name, reason)
}

func (e *survivorError) Unwrap() error { return e.reason }

// processRow is one row of a host process table snapshot.
type processRow struct {
	ppid   int
	zombie bool
	name   string
}

// processTable reads the host process table. Rex panes are local, so a
// descendant that block.close fails to reap is still visible here.
func (p *Provider) processTable(ctx context.Context) (map[int]processRow, error) {
	out, err := p.runner.Run(ctx, "", nil, "ps", "-axo", "pid=,ppid=,stat=,comm=")
	if err != nil {
		return nil, err
	}
	rows := make(map[int]processRow)
	for _, line := range strings.Split(out, "\n") {
		pidStr, ppidStr, stat, name, ok := splitProcessRow(line)
		if !ok {
			continue
		}
		pid, pidErr := strconv.Atoi(pidStr)
		ppid, ppidErr := strconv.Atoi(ppidStr)
		if pidErr != nil || ppidErr != nil || pid <= 0 {
			continue
		}
		rows[pid] = processRow{
			ppid:   ppid,
			zombie: strings.HasPrefix(stat, "Z"),
			name:   name,
		}
	}
	return rows, nil
}

// splitProcessRow splits one ps row into its three fixed columns and the rest
// of the line, which is the command name. The name may be empty or contain
// spaces, so it must not be field-split like the pid and ppid.
func splitProcessRow(line string) (pid, ppid, stat, comm string, ok bool) {
	var col [3]string
	rest := strings.TrimRight(line, "\r\n")
	for i := range col {
		rest = strings.TrimLeft(rest, " \t")
		if rest == "" {
			return "", "", "", "", false
		}
		end := strings.IndexAny(rest, " \t")
		if end < 0 {
			col[i], rest = rest, ""
			continue
		}
		col[i], rest = rest[:end], rest[end:]
	}
	return col[0], col[1], col[2], strings.TrimSpace(rest), true
}

// paneSubtree captures what the pane could leave behind: the foreground Rex
// reports and every host process below it. Ancestors are reported too, but
// they are the pane's shell and its parents, so a sibling under one of them is
// not the pane's process and must not be signalled. It must run before
// block.close: a detached descendant is reparented to init when the pane dies
// and can then no longer be attributed to this session. An unreadable process
// listing is returned so stop can leave the pane open (GCREX-50). A pane that
// vanished before the snapshot is the no-pane case, not an unreadable tree.
func (p *Provider) paneSubtree(ctx context.Context, b Binding) ([]paneProc, error) {
	info, err := p.client.BlockProcess(ctx, b.SessionID, b.BlockID)
	if errors.Is(err, rex.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	reported := make(map[int]string)
	if fg := info.Foreground; fg != nil && fg.PID > 0 {
		reported[fg.PID] = fg.Name
	}
	for _, a := range info.Ancestors {
		if a.PID > 0 {
			if _, ok := reported[a.PID]; !ok {
				reported[a.PID] = a.Name
			}
		}
	}
	if len(reported) == 0 {
		return nil, nil
	}
	table, err := p.processTable(ctx)
	if err != nil {
		return nil, err
	}
	// The walk starts at the foreground. Without one, fall back to the
	// reported process with no reported ancestor, so it does not depend on
	// Rex's ancestors order.
	below := make(map[int]bool, len(reported))
	if fg := info.Foreground; fg != nil && fg.PID > 0 {
		below[fg.PID] = true
	} else {
		for pid := range reported {
			if !hasReportedAncestor(pid, reported, table) {
				below[pid] = true
			}
		}
	}
	// Add every process whose parent is already in the set, until it settles.
	for changed := true; changed; {
		changed = false
		for pid, row := range table {
			if below[pid] || !below[row.ppid] {
				continue
			}
			below[pid] = true
			changed = true
		}
	}
	out := make([]paneProc, 0, len(below))
	for pid := range below {
		name := reported[pid]
		if name == "" {
			name = filepath.Base(table[pid].name)
		}
		if name == "" || name == "." {
			name = "?"
		}
		out = append(out, paneProc{pid: pid, name: name})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].pid < out[j].pid })
	return out, nil
}

// hasReportedAncestor reports whether one of the pane's other reported
// processes is above pid in the host table.
func hasReportedAncestor(pid int, reported map[int]string, table map[int]processRow) bool {
	seen := make(map[int]bool)
	for next, ok := table[pid]; ok; next, ok = table[next.ppid] {
		if next.ppid <= 1 || seen[next.ppid] {
			return false // top of the table, or a cycle
		}
		seen[next.ppid] = true
		if _, ok := reported[next.ppid]; ok {
			return true
		}
	}
	return false
}

// signalAll delivers sig to every captured process and returns the first
// failure, so a reap that could not signal can say why a process survived. A
// failure does not stop the loop: one unsignalable process must not shield the
// rest of the tree from the signal.
func (p *Provider) signalAll(procs []paneProc, sig syscall.Signal) error {
	var first error
	for _, proc := range procs {
		if proc.pid <= 1 || proc.pid == os.Getpid() {
			continue
		}
		if err := p.signal(proc.pid, sig); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// living returns the captured processes still running. A zombie counts as
// gone (init reaps it), and a pid whose name changed is not ours any more.
func (p *Provider) living(ctx context.Context, tree []paneProc) []paneProc {
	table, err := p.processTable(ctx)
	if err != nil {
		return tree // cannot prove them gone: report them
	}
	var out []paneProc
	for _, proc := range tree {
		row, ok := table[proc.pid]
		if !ok || row.zombie || !sameProgram(proc.name, row.name) {
			continue
		}
		out = append(out, proc)
	}
	return out
}

// unionProcs appends the processes whose pid is not already in tree, so a
// second pane read can extend, never shrink, what the first captured.
func unionProcs(tree, extra []paneProc) []paneProc {
	seen := make(map[int]bool, len(tree))
	for _, proc := range tree {
		seen[proc.pid] = true
	}
	for _, proc := range extra {
		if !seen[proc.pid] {
			seen[proc.pid] = true
			tree = append(tree, proc)
		}
	}
	return tree
}

// sameProgram reports whether the table row still names the captured process,
// so a recycled pid is never signalled.
func sameProgram(name, comm string) bool {
	if name == "" || name == "?" {
		return true
	}
	return comm == name || filepath.Base(comm) == name || strings.HasSuffix(comm, name)
}

// waitGone polls the process table until no captured process is left or the
// wait elapses, returning what survived.
func (p *Provider) waitGone(ctx context.Context, tree []paneProc, wait time.Duration) []paneProc {
	deadline := time.Now().Add(wait)
	for {
		left := p.living(ctx, tree)
		if len(left) == 0 || !time.Now().Before(deadline) || ctx.Err() != nil {
			return left
		}
		select {
		case <-ctx.Done():
			return left
		case <-time.After(reapPoll):
		}
	}
}

// reap gives the pane's reported process tree TERM, waits, then KILL, waits
// and rescans. It returns the processes that are still alive, plus the first
// signal failure when there was one; an empty result proves the tree is gone.
func (p *Provider) reap(ctx context.Context, tree []paneProc) ([]paneProc, error) {
	if len(tree) == 0 {
		return nil, nil
	}
	left := p.living(ctx, tree)
	if len(left) == 0 {
		return nil, nil
	}
	termErr := p.signalAll(left, syscall.SIGTERM)
	if left = p.waitGone(ctx, tree, reapTermWait); len(left) == 0 {
		return nil, termErr
	}
	if killErr := p.signalAll(left, syscall.SIGKILL); termErr == nil {
		termErr = killErr
	}
	return p.waitGone(ctx, tree, reapKillWait), termErr
}

// osSignal delivers sig to pid. The pane's descendants run on this host, so a
// plain process signal is the only reap mechanism Rex leaves the pack.
func osSignal(pid int, sig os.Signal) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return proc.Signal(sig)
}
