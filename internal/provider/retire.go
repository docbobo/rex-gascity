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
	// reapRecorded reaps only the survivors the binding records: the pane
	// is already gone and reports nothing.
	reapRecorded
)

// clearScope selects the sidecar state the clear step removes.
type clearScope int

const (
	// clearNone skips the clear.
	clearNone clearScope = iota
	// clearIncarnation removes the whole meta sidecar and the activity record
	// under the caller's name and the binding's name (clearIncarnations).
	clearIncarnation
	// clearIdentity removes only the start identity keys
	// (clearStartIdentity) under the caller's name and the binding's name,
	// deduplicated and skipping "". It takes the state lock around the clear
	// only, so the caller must not hold the state lock.
	clearIdentity
)

// forgetMode selects the stored binding the last step drops.
type forgetMode int

const (
	// forgetNone keeps every record.
	forgetNone forgetMode = iota
	// forgetBinding drops the record under the binding's own name.
	forgetBinding
	// forgetCaller drops the record that answers to the caller's name (the
	// primary it names when the caller is an alias).
	forgetCaller
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
	// reRecordOnDestroyFail writes bd back when the destroy fails: the
	// no-pane stop's ResolveFresh can drop a stale record before retirement
	// starts, and the retry reads the session id from it (GCREX-31 review).
	// Errors are joined.
	reRecordOnDestroyFail bool
	// forget selects the stored binding dropped last. forgetBinding drops the
	// record only while it still is bd's incarnation (ForgetIfCurrent).
	// forgetCaller drops the caller's record without that guard.
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

// stopNoPanePolicy retires a name whose pane is already gone (the usual
// drain case). It skips the snapshot, the close and the re-read: there is no
// pane to read or close. It reaps only the recorded survivors, because a
// retry after a stop whose pane has since vanished still owes them a reap.
// The drain diagnostic stays in stop, gated on hadStored. hadStored gates
// the destroy-failure re-record (REQ-015); with no stored binding, bd is the
// zero Binding and every step but the clear and forget of the caller's name
// is a no-op.
//
// Lock state: the caller holds the per-name locks (Stop →
// stopUnderPrimaryLock). The code takes no state lock on this path; the
// parity doc `### stop` row 5 and destroyEmptyCitySession's doc say the
// state lock is held. gc v1.5.1 names no lock for stop
// (internal/runtime/runtime.go:163-165). Unverified, REQ-017 follow-up.
func stopNoPanePolicy(hadStored bool) retirePolicy {
	return retirePolicy{
		reap:                  reapRecorded,
		clear:                 clearIncarnation,
		destroy:               true,
		reRecordOnDestroyFail: hadStored,
		forget:                forgetCaller,
	}
}

// stalePolicy finishes the retirement of a record whose block is already
// gone: kill-object's absent box. It skips the snapshot, the close and the
// re-read, so it never touches Rex and cannot disturb a replacement pane. It
// skips the drain diagnostic because the recorded pane is already gone. It
// skips the destroy, as the absent-box path does today. It reaps only the
// recorded survivors, because a previous stop may have recorded pids it could
// not reap before the pane vanished, and the record is their only name.
//
// Lock state: the caller holds the name lock and, inside withSameBinding,
// the state lock; it passes the binding re-read under that lock as both bd
// and caller.
func stalePolicy() retirePolicy {
	return retirePolicy{
		reap:   reapRecorded,
		clear:  clearIncarnation,
		forget: forgetBinding,
	}
}

// discardLaunchPolicy retires a pane NewSplit or NewWindow created whose
// final record failed: the pane would otherwise run on after start's deferred
// rollback forgets its binding. It snapshots, closes, re-reads and reaps. It
// returns a snapshot error raw; the caller joins every error with its cause.
// It skips the diagnostic (the worker never ran), the clear and the forget
// (start's deferred rollback owns them), and the destroy (the caller holds
// the session's launch lock, so the session cannot be checked empty here).
//
// Lock state: the caller holds the name lock, the tab lock and the session's
// shared launch lock; not the state lock.
func discardLaunchPolicy() retirePolicy {
	return retirePolicy{
		snapshot: true,
		close:    true,
		reap:     reapTree,
	}
}

// startDeadPanePolicy retires the dead pane a Start found under the name
// before it launches a new one. It closes the block and clears only the start
// identity, so a start that fails before seeding cannot leave a GC_SESSION_ID
// for the next pane to inherit, then forgets the caller's record. It skips the
// snapshot, the re-read and the reap: the pre-resolve reapStartSurvivors sweep
// already reaped the recorded survivors, and start's liveness read (paneAlive)
// already chose this branch. It skips the destroy: start reuses the session.
// It skips the diagnostic: nothing is draining.
//
// Lock state: the caller holds the name lock and not the state lock, which
// the identity clear takes around the clear only (REQ-014).
func startDeadPanePolicy() retirePolicy {
	return retirePolicy{
		close:  true,
		clear:  clearIdentity,
		forget: forgetCaller,
	}
}

// retire retires one incarnation in the fixed order: diagnostic, snapshot and
// union of the recorded survivors, close, re-read, reap, clear, destroy,
// forget. pol switches steps off or picks their scope; it cannot reorder them.
// Any step's error returns at once, so a later step never runs after a
// failure and the record is never forgotten after one. retire takes no lock
// itself; destroyEmptyCitySession takes the launch lock.
//
// Caller contract: retire takes no per-name lock, so the caller MUST hold the
// per-name lock of every name it retires (caller and bd.Name) for the whole
// call. The clear and destroy steps mutate per-name state and are not
// incarnation-guarded. forgetBinding is guarded: bd is the incarnation
// argument, and ForgetIfCurrent drops the record only while it still records
// that pane, so a caller that lost the lock race cannot drop a start's
// replacement binding. forgetCaller carries no such guard, and no clear or
// destroy race is closed. The guard does not make a lock-free caller safe; it
// closes the forgetBinding half of the race the review named.
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
	case reapRecorded:
		if err := p.reapSurvivors(ctx, bd, survivorProcs(bd.Survivors)); err != nil {
			return res, err
		}
	}
	switch pol.clear {
	case clearNone:
	case clearIncarnation:
		if err := p.clearIncarnations(caller, bd.Name); err != nil {
			return res, err
		}
	case clearIdentity:
		// The retired identity can sit under the canonical name the caller
		// reached by alias, not only under the caller's: clear both before
		// the record is dropped, or a failed relaunch leaves the old
		// GC_SESSION_ID readable.
		names := []SessionName{caller}
		if bd.Name != "" && bd.Name != caller {
			names = append(names, bd.Name)
		}
		if err := p.withStateLock(ctx, func() error {
			var errs []error
			for _, name := range names {
				errs = append(errs, p.clearStartIdentity(name))
			}
			return errors.Join(errs...)
		}); err != nil {
			return res, err
		}
	}
	if pol.destroy {
		// The record goes only after a successful removal: a failed one must
		// keep the session id for the retry (GCREX-31 review).
		if err := p.destroyEmptyCitySession(ctx, bd.SessionID); err != nil {
			if pol.reRecordOnDestroyFail {
				if recErr := p.binder.Record(bd); recErr != nil {
					return res, errors.Join(err, recErr)
				}
			}
			return res, err
		}
	}
	switch pol.forget {
	case forgetNone:
	case forgetBinding:
		// bd is the incarnation argument: a start that replaced the record
		// during this retirement keeps its binding.
		if _, err := p.binder.ForgetIfCurrent(bd); err != nil {
			return res, err
		}
	case forgetCaller:
		p.binder.Forget(caller)
	}
	res.alreadyClosed = closedNotFound
	return res, nil
}

// destroyEmptyCitySession removes this city's canonical gc:<city> session once
// its last pane is gone (GCREX-31). The caller holds the state lock. This
// function takes the session's launch lock before it reads the view, so no
// launch can add a pane between the empty check and the removal. It fails
// closed: a session with any window, a label that is not the canonical one (a
// legacy, foreign or other-city session), or any Rex error keeps the session
// and returns the error.
func (p *Provider) destroyEmptyCitySession(ctx context.Context, sid rex.SessionID) error {
	if sid == "" {
		return nil
	}
	// The launch lock comes first: a launch holds it shared across its
	// SessionView and NewWindow/NewSplit, so an empty view below cannot go
	// stale under us. Rex removes a tab with its last block
	// (docs/plans/native-rex-provider.md:387, live-verified), so zero windows
	// is a true empty session.
	release, busy, lockErr := p.tryLaunchLock(sid)
	if lockErr != nil {
		return fmt.Errorf("check whether %s is empty: %w", sid, lockErr)
	}
	if busy {
		return nil
	}
	defer release()
	view, err := p.client.SessionView(ctx, sid)
	if errors.Is(err, rex.ErrNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("check whether %s is empty: %w", sid, err)
	}
	if view.Label != p.binder.canonicalLabel() || len(view.Windows) != 0 {
		return nil
	}
	if err := p.client.DestroySession(ctx, sid); err != nil && !errors.Is(err, rex.ErrNotFound) {
		return fmt.Errorf("remove the empty city session %s: %w", sid, err)
	}
	return nil
}

// clearIncarnations removes the whole metadata sidecar and the activity record
// for each name (deduplicated). Stop calls it with the caller's name and the
// resolved primary name, so every key and activity stamp of a retired
// incarnation is gone before the next start of the same name reads them. This
// matches herdr-core's per-incarnation forget (parity doc ### stop row 4).
func (p *Provider) clearIncarnations(names ...SessionName) error {
	seen := map[SessionName]bool{}
	var errs []error
	for _, n := range names {
		if n == "" || seen[n] {
			continue
		}
		seen[n] = true
		if err := p.metaClear(n); err != nil {
			errs = append(errs, err)
		}
		if err := p.activity.Drop(n); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
