package provider

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/docbobo/rex-gascity/internal/prochost"
)

// Reaping bounds. herdr-core bounds the reap at these waits; the vars let tests
// shorten them.
var (
	reapTermWait = 2 * time.Second
	reapKillWait = 2 * time.Second
	reapPoll     = 50 * time.Millisecond
)

// survivorError fails a stop when a captured process may still run. herdr-core
// keeps the record for a retry and uses this wording (herdrcore/procs.py).
type survivorError struct {
	name      string
	survivors []prochost.Ident
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
			names[i] = fmt.Sprintf("%d (%s)", s.PID, s.Name)
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
func survivorRecords(procs []prochost.Ident) []Survivor {
	if len(procs) == 0 {
		return nil
	}
	out := make([]Survivor, len(procs))
	for i, proc := range procs {
		out[i] = Survivor{PID: proc.PID, Name: proc.Name, Start: proc.Start}
	}
	return out
}

// survivorProcs turns the recorded survivors back into a tree to reap.
func survivorProcs(recs []Survivor) []prochost.Ident {
	if len(recs) == 0 {
		return nil
	}
	out := make([]prochost.Ident, len(recs))
	for i, rec := range recs {
		out[i] = prochost.Ident{PID: rec.PID, Name: rec.Name, Start: rec.Start}
	}
	return out
}

// paneSubtree captures what the pane could leave behind: the foreground Rex
// reports and every host process below it. Ancestors are reported too, but
// they are the pane's shell and its parents, so a sibling under one of them is
// not the pane's process and is not signalled. A captured name is the host
// table's comm, not Rex's Name (which differs on Darwin, U1), so the scan
// compares like with like. It must run before block.close: a descendant that
// block.close fails to reap is still visible here. A pane that block.close
// already removed reports no process and is the no-pane case.
func (p *Provider) paneSubtree(ctx context.Context, b Binding) ([]prochost.Ident, error) {
	life := readPaneLife(ctx, p.client, b.SessionID, b.BlockID)
	switch life.state {
	case paneGone:
		return nil, nil
	case paneUnknown:
		return nil, life.err
	}
	reported := life.reported()
	if len(reported) == 0 {
		return nil, nil
	}
	table, err := p.processHost.Table(ctx, false)
	if err != nil {
		return nil, err
	}
	// The walk starts at the reported foreground. Without one, it starts at
	// every reported pid with no reported ancestor.
	var roots []int
	if fg, ok := life.foreground(); ok {
		roots = append(roots, fg)
	} else {
		set := make(map[int]bool, len(reported))
		for pid := range reported {
			set[pid] = true
		}
		for pid := range reported {
			if !prochost.AncestorIn(table, pid, set) {
				roots = append(roots, pid)
			}
		}
	}
	out := make([]prochost.Ident, 0, len(roots))
	for _, pid := range prochost.Subtree(table, roots) {
		row := table[pid]
		// Name the process from this ps snapshot whenever it is there. Rex's
		// name is the fallback for a reported pid the table lacks.
		name := filepath.Base(row.Comm)
		start := row.Start
		if name == "" || name == "." {
			name = reported[pid]
			start = ""
		}
		if name == "" || name == "." {
			name = "?"
		}
		out = append(out, prochost.Ident{PID: pid, Name: name, Start: start})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].PID < out[j].PID })
	return out, nil
}

// signalAll delivers sig to every captured process and returns the first
// error. A failure does not shield the rest of the set.
func (p *Provider) signalAll(procs []prochost.Ident, sig syscall.Signal) error {
	var first error
	for _, proc := range procs {
		if proc.PID <= 1 || proc.PID == os.Getpid() {
			continue
		}
		if err := p.processHost.Signal(proc.PID, sig); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// descendants appends the host processes below the captured set. A survivor
// recorded by an earlier stop can spawn a child before the retry, and a closed
// pane no longer reports that child. A zombie and a recycled pid are skipped.
func (p *Provider) descendants(ctx context.Context, procs []prochost.Ident) []prochost.Ident {
	if len(procs) == 0 {
		return procs
	}
	table, err := p.processHost.Table(ctx, false)
	if err != nil {
		return procs
	}
	existing := make(map[int]bool, len(procs))
	var roots []int
	for _, proc := range procs {
		existing[proc.PID] = true
		row, ok := table[proc.PID]
		if !ok || row.Zombie || !prochost.Same(proc, row) {
			continue
		}
		roots = append(roots, proc.PID)
	}
	for _, pid := range prochost.Subtree(table, roots) {
		if existing[pid] {
			continue
		}
		row := table[pid]
		if row.Zombie {
			continue
		}
		procs = append(procs, prochost.Ident{PID: pid, Name: filepath.Base(row.Comm), Start: row.Start})
	}
	return procs
}

// scan reads the host table once and classifies each captured process per the
// plan's §2a table: signal (present, not a zombie, Same is true), gone (absent,
// a zombie, or a different start on the same pid), or unverified (present but
// identity is not provable). alive is the signal set plus the unverified set.
func (p *Provider) scan(ctx context.Context, tree []prochost.Ident) (sig, alive, unverified []prochost.Ident, readErr error) {
	table, err := p.processHost.Table(ctx, false)
	if err != nil {
		return nil, tree, nil, err
	}
	for _, id := range tree {
		row, ok := table[id.PID]
		if !ok || row.Zombie {
			continue
		}
		if id.Start != "" && row.Start != "" && id.Start != row.Start {
			continue
		}
		if prochost.Same(id, row) {
			sig = append(sig, id)
			continue
		}
		unverified = append(unverified, id)
	}
	alive = append(append([]prochost.Ident(nil), sig...), unverified...)
	return sig, alive, unverified, nil
}

// unionProcs appends the extra set to tree, so a second read never shrinks the
// captured set. Identity is pid plus start; the name is not part of identity.
func unionProcs(tree, extra []prochost.Ident) []prochost.Ident {
	seen := make(map[prochost.Ident]bool, len(tree))
	for _, proc := range tree {
		proc.Name = ""
		seen[proc] = true
	}
	for _, proc := range extra {
		key := proc
		key.Name = ""
		if seen[key] {
			continue
		}
		seen[key] = true
		tree = append(tree, proc)
	}
	return tree
}

// waitGone polls scan over the latest alive set until the signal set is empty
// or the wait expires. It starts from live and keeps only what the last poll
// proved alive, so a capture already proven gone never re-enters the set. A
// read error returns the last alive set and the error.
func (p *Provider) waitGone(ctx context.Context, live []prochost.Ident, wait time.Duration) ([]prochost.Ident, error) {
	deadline := time.Now().Add(wait)
	alive := live
	for {
		sig, current, _, err := p.scan(ctx, alive)
		if err != nil {
			return alive, err
		}
		alive = current
		if len(sig) == 0 || !time.Now().Before(deadline) {
			return alive, nil
		}
		select {
		case <-ctx.Done():
			return alive, nil
		case <-time.After(reapPoll):
		}
	}
}

// reap gives the captured set TERM, waits, runs one fresh scan before KILL, and
// waits again. left is every capture not proven gone at the last successful
// read. A read error stops the sequence and returns the current alive set.
func (p *Provider) reap(ctx context.Context, tree []prochost.Ident) (left []prochost.Ident, readErr, signalErr error) {
	if len(tree) == 0 {
		return nil, nil, nil
	}
	sig, alive, _, readErr := p.scan(ctx, tree)
	if readErr != nil {
		return tree, readErr, nil
	}
	if len(sig) == 0 {
		return alive, nil, nil
	}
	signalErr = p.signalAll(sig, syscall.SIGTERM)
	left, readErr = p.waitGone(ctx, alive, reapTermWait)
	if readErr != nil {
		return left, readErr, signalErr
	}
	if len(left) == 0 {
		return nil, nil, signalErr
	}
	sig, alive, _, readErr = p.scan(ctx, left)
	if readErr != nil {
		return left, readErr, signalErr
	}
	if len(sig) == 0 {
		return alive, nil, signalErr
	}
	if killErr := p.signalAll(sig, syscall.SIGKILL); signalErr == nil {
		signalErr = killErr
	}
	left, readErr = p.waitGone(ctx, alive, reapKillWait)
	return left, readErr, signalErr
}

// reapSurvivors reaps tree and, while any of it survives, records the pids on
// the binding so the retry stop herdr-core allows still has them: the pane is
// closed by then and reports nothing. It returns the failure to report, nil
// when the tree is gone.
func (p *Provider) reapSurvivors(ctx context.Context, binding Binding, tree []prochost.Ident) error {
	// A survivor recorded by an earlier stop can spawn a child before the
	// retry, and the closed pane no longer reports that child: read the host
	// table so the retry reaps what the tree grew, not only what it was given.
	tree = p.descendants(ctx, tree)
	left, readErr, signalErr := p.reap(ctx, tree)
	binding.Survivors = survivorRecords(left)
	if len(left) == 0 {
		// A signal error with nothing left is a success: everything died anyway
		// (the caller's contract), so the failed signal is not reported.
		return readErr
	}
	survived := &survivorError{name: string(binding.Name), survivors: left, reason: errors.Join(readErr, signalErr)}
	// A start may have replaced the binding while this reap ran. Record only
	// while the stored binding is still the one we reaped: a replacement owns
	// the name and must not carry the old pane's survivors.
	if _, err := p.binder.RecordIfCurrent(binding); err != nil {
		// The stop fails either way: the record that would name these pids for
		// the next retry was not saved.
		return errors.Join(err, survived)
	}
	return survived
}
