package provider

import (
	"context"
	"errors"
	"time"

	"github.com/docbobo/rex-gascity/internal/rex"
)

// recordOrDiscard records the binding of the pane a launch just created. When
// that record fails it retires the pane under discardLaunchPolicy, so the pane
// does not run on after start's deferred rollback forgets its binding. It
// returns the record error joined with the retirement error. The provisional
// record and the seeded key stay for start's rollback.
func (p *Provider) recordOrDiscard(ctx context.Context, n SessionName, bd Binding) error {
	if recErr := p.binder.Record(bd); recErr != nil {
		_, err := p.retire(ctx, n, bd, discardLaunchPolicy())
		return errors.Join(recErr, err)
	}
	return nil
}

// findOrCreateCitySession returns this city's canonical session, creating it
// when none exists, under the short state lock: two starts of different names
// may run concurrently, and both would otherwise create the shared gc:<city>
// session. On error the returned id is "".
func (p *Provider) findOrCreateCitySession(ctx context.Context) (rex.SessionID, error) {
	var sid rex.SessionID
	err := p.withStateLock(ctx, func() error {
		var ok bool
		var err error
		sid, ok, err = p.binder.FindSession(ctx)
		if err != nil {
			return err
		}
		if !ok {
			sid, err = p.client.CreateSession(ctx, p.binder.canonicalLabel())
		}
		return err
	})
	return sid, err
}

// launchRequest is what launchPane needs from start's planning steps.
type launchRequest struct {
	name    SessionName
	attempt string        // the one token per start call; the retry reuses it
	session rex.SessionID // from start's first find-or-create
	tab     string        // the rig tab label (start's hint.TabLabel)
	spec    rex.LaunchSpec
	// relaunch is true when a binding already answered to the name (a dead
	// pane is relaunched); such a start must not steal focus.
	relaunch bool
	cfg      Config // cfg.Env is the projected launch env (plan.Env)
}

// launchPane seeds the start identity and a provisional binding, launches the
// agent's pane, records it or discards it, and on any failure rolls back what
// this start seeded. It retries once, in a recreated city session, when the
// first launch answers rex.ErrNotFound.
//
// Locks: the caller holds the per-name lock (Start and StartNew do); the
// discard retire needs it and launchPane does not take it again. The state
// lock is taken in four short sections, none of them while a launch lock is
// held: the seed and provisional record; on the retry, find-or-create; on the
// retry, the provisional re-record; on a launch failure, the empty-session
// cleanup, which takes the launch lock with the non-blocking tryLaunchLock
// after launchOnce has released its shared hold. Each launchOnce holds the
// session's shared launch lock around the tab lock, as before.
//
// Cleanup has two paths. A seed or provisional-record failure returns at once
// and only the deferred rollback runs. A launch or retry failure first runs
// the bounded empty-session check, then the rollback.
func (p *Provider) launchPane(ctx context.Context, req launchRequest) (res startResult, err error) {
	n := req.name
	// keepProvisional stays true when a failed launch could not remove the
	// empty city session. A later Stop then still finds the session id.
	keepProvisional := false
	defer func() {
		if err == nil {
			return
		}
		// A failed launch or a canceled start must not leave a seeded identity
		// or a provisional binding behind. Stop clears the same keys on the
		// successful retire path. A survivor record is the only name for a
		// descendant the retire could not reap, so it stays.
		// The pane itself was already retired by discardLaunchPolicy when
		// its record failed; this undoes only start's own provisional state.
		_ = p.clearStartIdentity(n)
		if bd, ok := p.Binding(string(n)); ok && len(bd.Survivors) > 0 {
			return
		}
		if !keepProvisional {
			p.binder.Forget(n)
		}
	}()

	// Seed the identity and a provisional binding under the short state lock,
	// before the launch call (which blocks while Rex dismisses startup dialogs).
	// A reconciler tick in that window can then read the current
	// GC_SESSION_ID/GC_INSTANCE_TOKEN instead of an empty identity and must not
	// roll the pending create back.
	if err := p.withStateLock(ctx, func() error {
		if err := p.seedStartIdentity(n, req.cfg.Env); err != nil {
			return err
		}
		return p.recordProvisionalBinding(n, req.session, "", req.attempt, req.cfg)
	}); err != nil {
		return startResult{}, err
	}

	sid := req.session
	launchStart := time.Now()
	launched, err := p.launchOnce(ctx, req, sid)
	if errors.Is(err, rex.ErrNotFound) {
		// A concurrent Stop of another name destroyed the empty city session
		// after find-or-create released the state lock. Create it again and
		// launch once more; a second miss is returned as is.
		if sid, err = p.findOrCreateCitySession(ctx); err == nil {
			if err = p.withStateLock(ctx, func() error {
				return p.recordProvisionalBinding(n, sid, "", req.attempt, req.cfg)
			}); err == nil {
				launched, err = p.launchOnce(ctx, req, sid)
			}
		}
	}
	if err != nil {
		// A failed launch leaves no pane, and the stop that skipped this
		// session because the launch was in flight will not run again: remove
		// the empty city session here, or it outlives its last pane
		// (GCREX-31 review). The launch lock is released by now.
		cleanupCtx, cancelCleanup := context.WithTimeout(context.WithoutCancel(ctx), startCleanupTimeout)
		defer cancelCleanup()
		if derr := p.withStateLock(cleanupCtx, func() error {
			return p.destroyEmptyCitySession(cleanupCtx, sid)
		}); derr != nil {
			// Keep the provisional record: the deferred handler must not
			// forget it. A later Stop retries the removal.
			keepProvisional = true
			p.warnf("start %s: remove the empty city session %s: %v", n, sid, derr)
		}
		return startResult{}, err
	}
	launched.launchDur = time.Since(launchStart)
	launched.env = req.cfg.Env
	return launched, nil
}

// launchOnce launches the pane into sid once: a split of the rig tab when it
// has a pane to split, else a new window. The session's shared launch lock is
// held across the view read and the create/split call, so a stop of any name
// can tell that a pane is still to come even while the session view shows no
// window yet (GCREX-31 review). It is cross-process, and the kernel drops it
// if this process dies. The per-rig (tab) lock makes the view read and the
// create/split decision atomic per tab, so two concurrent starts in one rig
// cannot both create the tab; a slow launch for one rig does not stall other
// rigs' starts or any stop.
func (p *Provider) launchOnce(ctx context.Context, req launchRequest, sid rex.SessionID) (startResult, error) {
	releaseLaunch, err := p.beginLaunch(ctx, sid)
	if err != nil {
		return startResult{}, err
	}
	defer releaseLaunch()
	var launched startResult
	err = p.withTabLock(ctx, req.tab, func() error {
		view, err := p.client.SessionView(ctx, sid)
		if err != nil {
			return err
		}
		for _, w := range view.Windows {
			if w.Label != req.tab {
				continue
			}
			anchor, direction, ok := nextSplit(w)
			if !ok {
				continue
			}
			block, err := p.client.NewSplit(ctx, sid, anchor, direction, "after", req.spec)
			if err != nil {
				return err
			}
			if block != "" {
				if err := p.recordOrDiscard(ctx, req.name, p.newBinding(req.name, sid, w.ID, block, "split", req.attempt, req.cfg)); err != nil {
					return err
				}
			}
			launched = startResult{sessionID: sid, blockID: block}
			return nil
		}
		// A brand-new session's tab becomes active so the operator lands in
		// it. A name that already answered to a binding (a live pane returned
		// earlier, a dead one is relaunched here) is an existing session and
		// must not steal focus. A split never moves the active tab either.
		windowSpec := req.spec
		windowSpec.Focus = !req.relaunch
		wid, block, err := p.client.NewWindow(ctx, sid, req.tab, windowSpec)
		if err != nil {
			return err
		}
		if block != "" {
			if err := p.recordOrDiscard(ctx, req.name, p.newBinding(req.name, sid, wid, block, "window", req.attempt, req.cfg)); err != nil {
				return err
			}
			// The town tab goes first once, when it is created (#47). Still
			// under the tab lock, after the binding is recorded.
			if newLaunchIdentity(req.name, req.cfg.Env, p.townTab).IsTownTab() {
				p.placeTownTabFirst(ctx, req.name, sid, wid)
			}
		}
		launched = startResult{sessionID: sid, blockID: block}
		return nil
	})
	if err != nil {
		return startResult{}, err
	}
	return launched, nil
}

// placeTownTabFirst moves the town window wid, which launchOnce just created
// in sid, in front of the session's current first window (#47). It reads the
// view after the create, because where session.new_window inserts a window is
// not documented. It moves nothing in a session other than this city's
// canonical one (a legacy session keeps Rex's order), in an empty view, or
// when wid is already first: the API rejects before == window. It is
// best-effort: a failed read or move is a start warning, never a start
// failure, and there is no retry.
func (p *Provider) placeTownTabFirst(ctx context.Context, name SessionName, sid rex.SessionID, wid rex.WindowID) {
	view, err := p.client.SessionView(ctx, sid)
	if err != nil {
		p.warnf("start %s: place town tab first: %v", name, err)
		return
	}
	if view.Label != p.binder.canonicalLabel() || len(view.Windows) == 0 || view.Windows[0].ID == wid {
		return
	}
	if err := p.client.MoveWindow(ctx, sid, wid, view.Windows[0].ID); err != nil {
		p.warnf("start %s: place town tab first: %v", name, err)
	}
}
