package provider

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/docbobo/rex-gascity/internal/rex"
)

const (
	lockPollInterval = 10 * time.Millisecond
	lockTimeout      = 60 * time.Second
)

// withStateLock runs fn while holding an exclusive flock on <stateDir>/lock,
// so separate dispatcher processes do not mutate shared state at once. Without
// a state dir it just runs fn. A canceled ctx returns ctx.Err().
//
// The state lock is deliberately short-lived: callers hold it only around the
// store and session mutations, never across a pre_start hook or the blocking
// Rex launch. Those run under withNameLock/withTabLock instead, so a slow hook
// or launch for one session cannot stall every other start and stop.
func (p *Provider) withStateLock(ctx context.Context, fn func() error) error {
	if p.ns.CanonicalDir() == "" {
		return fn()
	}
	return withFlock(ctx, filepath.Join(p.ns.CanonicalDir(), "lock"), fn)
}

// withNameLock runs fn while holding an exclusive flock for one session name at
// <stateDir>/locks/<name>.lock. It serializes every start and stop of that name
// across dispatcher processes without blocking other names, so a slow hook or
// launch for one session does not delay the rest of the city. Without a state
// dir it just runs fn.
func (p *Provider) withNameLock(ctx context.Context, n SessionName, fn func() error) error {
	if p.ns.CanonicalDir() == "" {
		return fn()
	}
	return withFlock(ctx, p.nameLockPath(n), fn)
}

// withTabLock runs fn while holding an exclusive flock for one Rex tab (rig) at
// <stateDir>/locks/tab-<rig>.lock. Two agents that share a rig share one tab,
// and the launch decides between NewWindow (create the tab) and NewSplit (join
// it) by reading the session view. The tab lock keeps that read-then-launch
// atomic per rig, so concurrent starts of different names in one rig cannot
// both create the tab, while starts in other rigs still proceed. Without a
// state dir it just runs fn.
func (p *Provider) withTabLock(ctx context.Context, tab string, fn func() error) error {
	if p.ns.CanonicalDir() == "" {
		return fn()
	}
	return withFlock(ctx, filepath.Join(p.ns.CanonicalDir(), "locks", "tab-"+nameKey(SessionName(tab))+".lock"), fn)
}

// nameLockPath is the per-name lock file under the canonical state dir.
func (p *Provider) nameLockPath(n SessionName) string {
	return filepath.Join(p.ns.CanonicalDir(), "locks", nameKey(n)+".lock")
}

// launchLockPath is the per-Rex-session launch lock file under the canonical
// state dir.
func (p *Provider) launchLockPath(sid rex.SessionID) string {
	return filepath.Join(p.ns.CanonicalDir(), "locks", "launch-"+nameKey(SessionName(sid))+".lock")
}

// beginLaunch holds the launch lock shared for as long as the caller's launch
// runs. Shared, because starts of different names may launch into one city
// session at once; a stop's tryLaunchLock then tells whether any of them has a
// pane still to come. Without a state dir there is nothing to coordinate, so
// it returns a no-op release. A canceled ctx returns ctx.Err().
func (p *Provider) beginLaunch(ctx context.Context, sid rex.SessionID) (func(), error) {
	if p.ns.CanonicalDir() == "" {
		return func() {}, nil
	}
	return withFlockShared(ctx, p.launchLockPath(sid))
}

// tryLaunchLock takes the launch lock exclusively without waiting. held is
// true when a launch already holds it shared, which means the session is not
// empty even when its view shows no window yet. Without a state dir it reports
// held=false and a no-op release.
func (p *Provider) tryLaunchLock(sid rex.SessionID) (release func(), held bool, err error) {
	if p.ns.CanonicalDir() == "" {
		return func() {}, false, nil
	}
	return flockExclusiveNow(p.launchLockPath(sid))
}

// withFlock runs fn while holding an exclusive flock on path, creating the
// parent directory and the lock file as needed. A canceled ctx returns
// ctx.Err(); a wait longer than lockTimeout fails with the lock path.
func withFlock(ctx context.Context, path string, fn func() error) error {
	release, err := flock(ctx, path, syscall.LOCK_EX)
	if err != nil {
		return err
	}
	defer release()
	return fn()
}

// withFlockShared is withFlock with a shared lock: holders exclude an exclusive
// holder but not each other.
func withFlockShared(ctx context.Context, path string) (func(), error) {
	return flock(context.Background(), path, syscall.LOCK_SH)
}

// flock takes an flock on path in mode, creating the parent directory and the
// lock file as needed, and returns the release. A canceled ctx returns
// ctx.Err(); a wait longer than lockTimeout fails with the lock path.
func flock(ctx context.Context, path string, mode int) (func(), error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}

	deadline := time.Now().Add(lockTimeout)
	for {
		if err := ctx.Err(); err != nil {
			_ = f.Close()
			return nil, err
		}
		err := syscall.Flock(int(f.Fd()), mode|syscall.LOCK_NB)
		if err == nil {
			break
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EINTR) {
			_ = f.Close()
			return nil, err
		}
		if time.Now().After(deadline) {
			_ = f.Close()
			return nil, fmt.Errorf("provider: timed out waiting for lock %s", f.Name())
		}
		select {
		case <-ctx.Done():
			_ = f.Close()
			return nil, ctx.Err()
		case <-time.After(lockPollInterval):
		}
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	}, nil
}

// flockExclusiveNow takes an exclusive flock on path without waiting. held is
// true when another holder has it. release is non-nil only when this call
// holds the lock.
func flockExclusiveNow(path string) (release func(), held bool, err error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, false, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, false, err
	}
	err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EINTR) {
		_ = f.Close()
		return nil, true, nil
	}
	if err != nil {
		_ = f.Close()
		return nil, false, err
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	}, false, nil
}
