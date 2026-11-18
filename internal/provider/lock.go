package provider

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
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

// withFlock runs fn while holding an exclusive flock on path, creating the
// parent directory and the lock file as needed. A canceled ctx returns
// ctx.Err(); a wait longer than lockTimeout fails with the lock path.
func withFlock(ctx context.Context, path string, fn func() error) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	deadline := time.Now().Add(lockTimeout)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			break
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EINTR) {
			return err
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("provider: timed out waiting for lock %s", f.Name())
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(lockPollInterval):
		}
	}
	defer func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN) }()
	return fn()
}
