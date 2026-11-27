package provider

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/docbobo/rex-gascity/internal/rex"
)

// stopUnderPrimaryLock runs stop with the lock of every record's own name that
// answers to the caller's as well as the caller's lock. An alias and its
// primary names map to different lock files, so a start of one of them would
// otherwise run beside this stop and write a replacement record that the stop's
// retirement then clears. Every such name is locked, not just the first stored
// candidate: ResolveFresh skips a stale candidate and can retire a later live
// one. The locks are taken in caller-then-primary order, sorted to a fixed order
// among themselves, and a primary stop takes no second lock, so no two paths
// take them in opposite order.
func (p *Provider) stopUnderPrimaryLock(ctx context.Context, name string) error {
	n := SessionName(name)
	var primaries []SessionName
	for _, bd := range p.binder.lookup(n) {
		if bd.Name != n && !slices.Contains(primaries, bd.Name) {
			primaries = append(primaries, bd.Name)
		}
	}
	slices.Sort(primaries)
	var lock func(i int) error
	lock = func(i int) error {
		if i == len(primaries) {
			return p.stop(ctx, name)
		}
		return p.withNameLock(ctx, primaries[i], func() error { return lock(i + 1) })
	}
	return lock(0)
}

func (p *Provider) stop(ctx context.Context, name string) error {
	n := SessionName(name)
	// Load the stored binding before Resolve: Resolve drops a stale record when
	// its pane is gone, and a drained worker's process has often exited already.
	// Resolve answers to a primary record or an alias, so the retry's drain
	// diagnostic and survivors must come from the same record: a plain read of
	// an alias name has no entry and would lose them.
	cands := p.binder.lookup(n) // before Resolve: Resolve may drop records
	known := len(cands) > 0
	var stored Binding
	hadStored := known
	if known {
		stored = cands[0]
	}
	binding, ok, err := p.binder.Resolve(ctx, ResolveRequest{Name: n, Mode: ResolveFresh})
	if errors.Is(err, rex.ErrUnavailable) && !known {
		// Rex is down and no stored binding answers to the name: herdr-core
		// answers an absent name with exit 0, so stop does too and clears the
		// name's state. This is a stored-binding policy, not proof that no
		// process is alive: a lost binding with a live pane reports success
		// (parity stop deviation 8).
		// stored.Name is empty when !known, and clearIncarnations skips "",
		// so this is the same cleanup as clearIncarnations(n, stored.Name).
		return p.clearIncarnations(n)
	}
	if err != nil {
		return err
	}
	if !ok {
		// No pane: the worker already exited, which is the usual drain case.
		// Sample its work tree from the stored binding.
		if hadStored {
			p.writeDrainDiagnostic(ctx, stored)
		}
		_, err = p.retire(ctx, n, stored, stopNoPanePolicy(hadStored))
		return err
	}
	// Stop stays idempotent: a pane another client already closed
	// (retireResult.alreadyClosed) is a finished retirement, not an error.
	_, err = p.retire(ctx, n, binding, stopPolicy())
	return err
}

// recycleName clears what an earlier incarnation of n left behind, then
// resolves the name's record for this start. A binding kept by a stop or a
// failed launch whose descendant could not be reaped is the only name for that
// process once its pane is gone. ResolveFresh drops such a stale record and
// Start then writes a new one, so the survivor reap finishes first or the pid
// runs on with no record. A reap error is returned as is. Otherwise relaunch,
// done and err are resolveStartTarget's.
func (p *Provider) recycleName(ctx context.Context, n SessionName, cfg Config) (relaunch, done bool, err error) {
	if err := p.reapStartSurvivors(ctx, n); err != nil {
		return false, false, err
	}
	return p.resolveStartTarget(ctx, n, cfg)
}

// reapStartSurvivors finishes the cleanup a prior stop or failed launch left
// pending on a binding that answers to n. A survivor the closed pane can no
// longer report must be reaped before ResolveFresh drops or replaces that
// record, or the next Start forgets the pid while it still runs. A survivor
// that will not die keeps the binding and fails the start for a retry.
func (p *Provider) reapStartSurvivors(ctx context.Context, n SessionName) error {
	for _, bd := range p.binder.lookup(n) {
		if len(bd.Survivors) == 0 {
			continue
		}
		if err := p.reapSurvivors(ctx, bd, survivorProcs(bd.Survivors)); err != nil {
			return fmt.Errorf("start %s: reap the pending survivors and retry: %w", n, err)
		}
	}
	return nil
}

// resolveStartTarget resolves the name's existing record and retires a dead
// pane. It runs after the survivor reap, so ResolveFresh sees only cleaned
// records. done reports a live pane that already serves the name: the start is
// idempotent and must return before it stages or launches anything. relaunch
// reports that a record existed, which launchPane turns into the relaunch
// handoff.
func (p *Provider) resolveStartTarget(ctx context.Context, n SessionName, cfg Config) (relaunch, done bool, err error) {
	old, found, err := p.binder.Resolve(ctx, ResolveRequest{
		Name: n, StartEnv: cfg.Env, Mode: ResolveFresh,
	})
	if err != nil {
		return false, false, err
	}
	if !found {
		return false, false, nil
	}
	alive, err := p.paneAlive(ctx, old)
	if err != nil {
		return false, false, err
	}
	if alive {
		return false, true, nil
	}
	// The liveness read above chose this branch; the retirement makes no
	// BlockProcess call of its own.
	if _, err := p.retire(ctx, n, old, startDeadPanePolicy()); err != nil {
		return false, false, err
	}
	return true, false, nil
}
