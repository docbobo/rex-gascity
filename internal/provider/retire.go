package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/docbobo/rex-gascity/internal/prochost"
	"github.com/docbobo/rex-gascity/internal/rex"
)

// reapScope selects the process tree the reap step signals.
type reapScope int

const (
	// reapNone skips the reap.
	reapNone reapScope = iota
	// reapTree reaps the pre-close snapshot, the recorded survivors and the
	// post-close re-read.
	reapTree
)

// clearScope selects the sidecar state the clear step removes.
type clearScope int

const (
	// clearNone skips the clear.
	clearNone clearScope = iota
	// clearIncarnation removes the whole meta sidecar and the activity record
	// under the caller's name and the binding's name (clearIncarnations).
	clearIncarnation
)

// forgetMode selects the stored binding the last step drops.
type forgetMode int

const (
	// forgetNone keeps every record.
	forgetNone forgetMode = iota
	// forgetBinding drops the record under the binding's own name.
	forgetBinding
)

// retirePolicy says which steps of the fixed retirement order a caller runs.
// A field can only switch its step off or pick the step's scope; nothing in it
// reorders the steps. Policies are built only by the named constructors below,
// never as literals at a call site.
type retirePolicy struct {
	// diagnostic samples the work tree before the close (writeDrainDiagnostic).
	diagnostic bool
	// snapshot reads the pane's process tree before the close and unions the
	// recorded survivors, then re-reads the pane after the close. An
	// unreadable pre-close tree fails before any CloseBlock; a failed re-read
	// leaves the pre-close tree authoritative.
	snapshot bool
	// wrapSnapshotErr reports a pre-close read failure as "could not inspect
	// the pane's process tree, leaving it open: %w" (stop and kill-object).
	// Off, the read error is returned as is.
	wrapSnapshotErr bool
	// close closes the exact block. ErrNotFound sets
	// retireResult.alreadyClosed and the retirement continues; any other
	// error is returned.
	close bool
	// reap selects the tree the reap step signals. Survivors are recorded
	// only through RecordIfCurrent (reapSurvivors).
	reap reapScope
	// clear selects the sidecar state the clear step removes.
	clear clearScope
	// destroy removes the canonical city session once it is empty
	// (destroyEmptyCitySession). The caller must not hold that session's
	// launch lock: destroyEmptyCitySession takes it itself.
	destroy bool
	// forget selects the stored binding dropped last.
	forget forgetMode
}

// retireResult reports how a finished retirement went.
type retireResult struct {
	// alreadyClosed is true only when the close step ran, Rex answered
	// ErrNotFound, and the retirement then finished without error.
	alreadyClosed bool
}

// stopPolicy retires a pane that is still there: Stop of a name that
// resolved to a pane, and kill-object's exact box. It runs every step and
// skips none.
//
// Lock state: Stop's caller holds the per-name locks (Stop →
// stopUnderPrimaryLock, lock.go:40); kill-object's caller holds the name lock
// and, inside withSameBinding, the state lock. Neither holds the session's
// launch lock, which destroy takes. Whether the stop path should also hold the
// state lock is unverified: destroyEmptyCitySession's doc and the parity doc
// `### stop` row 5 say it does, the code takes none, and gc v1.5.1 names no
// lock for stop (internal/runtime/runtime.go:163-165) (REQ-017 follow-up).
func stopPolicy() retirePolicy {
	return retirePolicy{
		diagnostic:      true,
		snapshot:        true,
		wrapSnapshotErr: true,
		close:           true,
		reap:            reapTree,
		clear:           clearIncarnation,
		destroy:         true,
		forget:          forgetBinding,
	}
}

// retire retires one incarnation in the fixed order: diagnostic, snapshot and
// union of the recorded survivors, close, re-read, reap, clear, destroy,
// forget. pol switches steps off or picks their scope; it cannot reorder them.
// Any step's error returns at once, so a later step never runs after a
// failure and the record is never forgotten after one. retire takes no lock
// itself; destroyEmptyCitySession takes the launch lock.
func (p *Provider) retire(ctx context.Context, caller SessionName, bd Binding, pol retirePolicy) (retireResult, error) {
	var res retireResult
	if pol.diagnostic {
		// Sample the work tree before the pane closes, while the worker is
		// alive.
		p.writeDrainDiagnostic(ctx, bd)
	}
	var tree []prochost.Ident
	if pol.snapshot {
		// A close is not proof that the tree is gone, and a detached
		// descendant can no longer be attributed to the pane once it is
		// reparented (GCREX-50). A tree we cannot read is not a tree we can
		// clear, so leave the pane open for the retry.
		before, err := p.paneSubtree(ctx, bd)
		if err != nil {
			if pol.wrapSnapshotErr {
				return res, fmt.Errorf("could not inspect the pane's process tree, leaving it open: %w", err)
			}
			return res, err
		}
		// A survivor an earlier retirement recorded was captured before this
		// pane closed, and a closed pane no longer reports it.
		tree = unionProcs(before, survivorProcs(bd.Survivors))
	}
	closedNotFound := false
	if pol.close {
		err := p.client.CloseBlock(ctx, bd.SessionID, bd.BlockID)
		if err != nil && !errors.Is(err, rex.ErrNotFound) {
			return res, err
		}
		closedNotFound = errors.Is(err, rex.ErrNotFound)
	}
	if pol.snapshot {
		// A process the pane spawned after the snapshot, or one Rex only
		// reports now, would otherwise pass unreaped. A closed pane answers
		// ErrNotFound (no tree) and a failed re-read leaves the pre-close
		// snapshot authoritative (review D1).
		if after, readErr := p.paneSubtree(ctx, bd); readErr == nil {
			tree = unionProcs(tree, after)
		}
	}
	switch pol.reap {
	case reapNone:
	case reapTree:
		if err := p.reapSurvivors(ctx, bd, tree); err != nil {
			return res, err
		}
	}
	switch pol.clear {
	case clearNone:
	case clearIncarnation:
		if err := p.clearIncarnations(caller, bd.Name); err != nil {
			return res, err
		}
	}
	if pol.destroy {
		// The record goes only after a successful removal: a failed one must
		// keep the session id for the retry (GCREX-31 review).
		if err := p.destroyEmptyCitySession(ctx, bd.SessionID); err != nil {
			return res, err
		}
	}
	switch pol.forget {
	case forgetNone:
	case forgetBinding:
		p.binder.Forget(bd.Name)
	}
	res.alreadyClosed = closedNotFound
	return res, nil
}
