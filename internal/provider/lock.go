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
// so separate dispatcher processes do not mutate Rex state at once. Without a
// state dir it just runs fn. A canceled ctx returns ctx.Err().
func (p *Provider) withStateLock(ctx context.Context, fn func() error) error {
	if p.ns.CanonicalDir() == "" {
		return fn()
	}
	if err := os.MkdirAll(p.ns.CanonicalDir(), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(p.ns.CanonicalDir(), "lock"), os.O_CREATE|os.O_RDWR, 0o644)
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
			return fmt.Errorf("provider: timed out waiting for state lock %s", f.Name())
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
