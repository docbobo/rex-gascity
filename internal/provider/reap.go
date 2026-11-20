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
	pid   int
	name  string
	start string // ps lstart of the snapshot row; identity against pid reuse
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

// survivorRecords is what the binding keeps of a reaped set, so a retry whose
// pane is already closed still has the pids to reap.
func survivorRecords(procs []paneProc) []Survivor {
	if len(procs) == 0 {
		return nil
	}
	out := make([]Survivor, len(procs))
	for i, proc := range procs {
		out[i] = Survivor{PID: proc.pid, Name: proc.name, Start: proc.start}
	}
	return out
}

// survivorProcs turns the recorded survivors back into a tree to reap.
func survivorProcs(recs []Survivor) []paneProc {
	if len(recs) == 0 {
		return nil
	}
	out := make([]paneProc, len(recs))
	for i, rec := range recs {
		out[i] = paneProc{pid: rec.PID, name: rec.Name, start: rec.Start}
	}
	return out
}

// processRow is one row of a host process table snapshot.
type processRow struct {
	ppid   int
	zombie bool
	name   string
	start  string
}

// processTable reads the host process table. Rex panes are local, so a
// descendant that block.close fails to reap is still visible here.
func (p *Provider) processTable(ctx context.Context) (map[int]processRow, error) {
	out, err := p.runner.Run(ctx, "", nil, "ps", "-axo", "pid=,ppid=,stat=,comm=,lstart=")
	if err != nil {
		return nil, err
	}
	rows := make(map[int]processRow)
	for _, line := range strings.Split(out, "\n") {
		pidStr, ppidStr, stat, name, start, ok := splitProcessRow(line)
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
			start:  start,
		}
	}
	return rows, nil
}

// lstartLen is the width of ps's lstart column, "%a %b %e %H:%M:%S %Y"
// ("Fri Nov 20 19:44:50 2026"); both BSD and GNU ps print it fixed-width.
const lstartLen = 24

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
// row without one (ps called without lstart) keeps the whole rest as the name.
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

// paneSubtree captures what the pane could leave behind: the foreground Rex
// reports and every host process below it. Ancestors are reported too, but
// they are the pane's shell and its parents, so a sibling under one of them is
// not the pane's process and must not be signalled. A captured name is the
// host table's comm, not Rex's Name (which differs from it on Darwin, U1), so
// living compares like with like. It must run before
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
		// Name the process from this ps snapshot whenever it is there: living
		// compares against the same field, and Rex disagrees with it — Rex
		// reports the executable (`node`) while Darwin's comm is argv0 (`pi`),
		// so a process named from Rex would never match its live row (U1).
		// Rex's name is only the fallback for a reported pid the table lacks.
		name := filepath.Base(table[pid].name)
		start := table[pid].start
		if name == "" || name == "." {
			name = reported[pid]
			start = ""
		}
		if name == "" || name == "." {
			name = "?"
		}
		out = append(out, paneProc{pid: pid, name: name, start: start})
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

// descendants extends procs with the host processes that descend from them: a
// process recorded by an earlier stop can spawn a child after the pane closed,
// and a closed pane reports no tree. Identity is pid plus lstart, so a recycled
// pid is never followed. An unreadable table leaves procs as the caller gave it.
func (p *Provider) descendants(ctx context.Context, procs []paneProc) []paneProc {
	if len(procs) == 0 {
		return procs
	}
	table, err := p.processTable(ctx)
	if err != nil {
		return procs
	}
	children := make(map[int][]int, len(table))
	for pid, row := range table {
		children[row.ppid] = append(children[row.ppid], pid)
	}
	seen := make(map[int]bool, len(procs))
	var queue []int
	for _, proc := range procs {
		row, ok := table[proc.pid]
		if !ok || row.zombie || !sameProcess(proc, row) {
			continue
		}
		seen[proc.pid] = true
		queue = append(queue, proc.pid)
	}
	for len(queue) > 0 {
		pid := queue[0]
		queue = queue[1:]
		for _, child := range children[pid] {
			if seen[child] {
				continue
			}
			seen[child] = true
			queue = append(queue, child)
			row := table[child]
			if row.zombie {
				continue
			}
			procs = append(procs, paneProc{pid: child, name: filepath.Base(row.name), start: row.start})
		}
	}
	return procs
}

// living returns the captured processes still running. A zombie counts as
// gone, and so does a pid the host reused: identity is the pid plus the lstart
// captured in the same snapshot, so a renamed descendant still counts (review
// round 4) while a recycled pid is never signalled.
func (p *Provider) living(ctx context.Context, tree []paneProc) []paneProc {
	table, err := p.processTable(ctx)
	if err != nil {
		return tree // cannot prove them gone: report them
	}
	var out []paneProc
	for _, proc := range tree {
		row, ok := table[proc.pid]
		if !ok || row.zombie || !sameProcess(proc, row) {
			continue
		}
		out = append(out, proc)
	}
	return out
}

// sameProcess reports whether the table row is still the captured process. The
// start time decides when both reads printed one; without it the name decides.
func sameProcess(proc paneProc, row processRow) bool {
	if proc.start != "" && row.start != "" {
		return proc.start == row.start
	}
	return sameProgram(proc.name, row.name)
}

// unionProcs appends the processes not already in tree, so a second pane read
// can extend, never shrink, what the first captured. Identity is pid plus
// lstart (review round 4): the same pid with a different start is a different
// process, and dropping one of the two would lose a real survivor.
func unionProcs(tree, extra []paneProc) []paneProc {
	// paneProc is comparable; the name is not part of identity, so zero it.
	seen := make(map[paneProc]bool, len(tree))
	for _, proc := range tree {
		proc.name = ""
		seen[proc] = true
	}
	for _, proc := range extra {
		key := proc
		key.name = ""
		if !seen[key] {
			seen[key] = true
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
