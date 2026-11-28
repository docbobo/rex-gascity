package provider

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// newLockOrderProvider is a provider with a state dir under a temp root, so
// every section has a file to flock, and the lock-order check armed.
func newLockOrderProvider(t *testing.T) *Provider {
	t.Helper()
	p := New("test", nil, WithNamespace(NewNamespaceForDirs(t.TempDir())))
	armLockOrder(t)
	return p
}

// armLockOrder turns the order guard on for one test and restores the built
// provider's setting when it ends.
func armLockOrder(t *testing.T) {
	t.Helper()
	lockOrderEnabled = true
	t.Cleanup(func() { lockOrderEnabled = false })
}

// TestLockOutOfOrderFails is the check the issue asks for: taking the tab lock
// and then the name lock fails with ErrLockOrder naming both kinds. Tab is the
// innermost section, name is an outer one, so the pair is not allowed.
func TestLockOutOfOrderFails(t *testing.T) {
	p := newLockOrderProvider(t)
	ctx := withLockOperation(context.Background())

	err := p.withTabLock(ctx, "rig-a", func() error {
		inner := p.withNameLock(ctx, SessionName("worker"), func() error { return nil })
		if inner == nil {
			t.Fatal("name lock under tab lock: want out-of-order error, got nil")
		}
		if !errors.Is(inner, ErrLockOrder) {
			t.Fatalf("name lock under tab lock: want ErrLockOrder, got %v", inner)
		}
		if got := inner.Error(); !strings.Contains(got, "tab") || !strings.Contains(got, "name") {
			t.Fatalf("error %q does not name both kinds", got)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("tab lock: %v", err)
	}
}

// TestLockStateInsideLaunchFails pins the rule the prose states: the state
// lock is never held inside the launch lock. Taking it there is the bug that
// would hold the city-wide lock across a blocking launch.
func TestLockStateInsideLaunchFails(t *testing.T) {
	p := newLockOrderProvider(t)
	ctx := withLockOperation(context.Background())

	release, err := p.beginLaunch(ctx, "sid-a")
	if err != nil {
		t.Fatalf("beginLaunch: %v", err)
	}
	defer release()
	inner := p.withStateLock(ctx, func() error { return nil })
	if !errors.Is(inner, ErrLockOrder) {
		t.Fatalf("state lock under launch lock: want ErrLockOrder, got %v", inner)
	}
	if got := inner.Error(); !strings.Contains(got, "launch") || !strings.Contains(got, "state") {
		t.Fatalf("error %q does not name both kinds", got)
	}
}

// TestLockLaunchInsideTabFails pins the launch -> tab direction: tab is the
// innermost section, so the launch lock must not be taken under it.
func TestLockLaunchInsideTabFails(t *testing.T) {
	p := newLockOrderProvider(t)
	ctx := withLockOperation(context.Background())

	err := p.withTabLock(ctx, "rig-a", func() error {
		release, err := p.beginLaunch(ctx, "sid-a")
		if err == nil {
			release()
			t.Fatal("launch lock under tab lock: want out-of-order error, got nil")
		}
		if !errors.Is(err, ErrLockOrder) {
			t.Fatalf("launch lock under tab lock: want ErrLockOrder, got %v", err)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("tab lock: %v", err)
	}
}

// TestLockSecondStateFails pins that the state lock is not reentrant: a second
// state section inside a state section is refused. Start's cleanup step nests a
// launch section, not another state section, so nothing needs it.
func TestLockSecondStateFails(t *testing.T) {
	p := newLockOrderProvider(t)
	ctx := withLockOperation(context.Background())

	err := p.withStateLock(ctx, func() error {
		inner := p.withStateLock(ctx, func() error { return nil })
		if !errors.Is(inner, ErrLockOrder) {
			t.Fatalf("state lock inside state lock: want ErrLockOrder, got %v", inner)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("state lock: %v", err)
	}
}

// TestLockOrderAllowedNestingsPass runs every nesting the provider actually
// takes, so the guard cannot reject correct code. The last case is the one a
// naive rank order would get wrong: name -> state, which start and KillObject
// take, while the reverse (state -> name) never occurs.
func TestLockOrderAllowedNestingsPass(t *testing.T) {
	ctx := withLockOperation(context.Background())
	cases := []struct {
		name string
		run  func(*Provider) error
	}{
		{"name -> state", func(p *Provider) error {
			return p.withNameLock(ctx, "worker", func() error {
				return p.withStateLock(ctx, func() error { return nil })
			})
		}},
		{"name -> launch", func(p *Provider) error {
			return p.withNameLock(ctx, "worker", func() error {
				release, err := p.beginLaunch(ctx, "sid-a")
				if err != nil {
					return err
				}
				release()
				return nil
			})
		}},
		{"name -> launch -> tab", func(p *Provider) error {
			return p.withNameLock(ctx, "worker", func() error {
				release, err := p.beginLaunch(ctx, "sid-a")
				if err != nil {
					return err
				}
				defer release()
				return p.withTabLock(ctx, "rig-a", func() error { return nil })
			})
		}},
		{"state -> launch", func(p *Provider) error {
			return p.withStateLock(ctx, func() error {
				release, busy, err := p.tryLaunchLock(ctx, "sid-a")
				if err != nil {
					return err
				}
				if busy {
					t.Fatal("tryLaunchLock answered busy with no other holder")
				}
				release()
				return nil
			})
		}},
		{"launch -> tab", func(p *Provider) error {
			release, err := p.beginLaunch(ctx, "sid-a")
			if err != nil {
				return err
			}
			defer release()
			return p.withTabLock(ctx, "rig-a", func() error { return nil })
		}},
		{"name -> name", func(p *Provider) error {
			return p.withNameLock(ctx, "a", func() error {
				return p.withNameLock(ctx, "a1", func() error { return nil })
			})
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := newLockOrderProvider(t)
			if err := tc.run(p); err != nil {
				t.Fatalf("allowed nesting %s: %v", tc.name, err)
			}
		})
	}
}

// TestLockOrderOffInBuiltProvider pins that the guard is off unless a test arms
// it: a correct call must never fail in the built provider because of it.
func TestLockOrderOffInBuiltProvider(t *testing.T) {
	lockOrderEnabled = false
	p := New("test", nil, WithNamespace(NewNamespaceForDirs(t.TempDir())))
	ctx := withLockOperation(context.Background())
	err := p.withTabLock(ctx, "rig-a", func() error {
		return p.withNameLock(ctx, "worker", func() error { return nil })
	})
	if err != nil {
		t.Fatalf("out-of-order call with the guard off: %v", err)
	}
}

// TestTryLaunchLockReportsHeldAfterRelease guards the held/release contract
// that the order wiring touches: a fresh lock reports held=false and a working
// release, and a second attempt while it is held reports held=true with no
// release.
func TestTryLaunchLockReportsHeldAfterRelease(t *testing.T) {
	p := New("test", nil, WithNamespace(NewNamespaceForDirs(t.TempDir())))
	ctx := withLockOperation(context.Background())

	release, held, err := p.tryLaunchLock(ctx, "sid-a")
	if err != nil {
		t.Fatalf("tryLaunchLock: %v", err)
	}
	if held {
		t.Fatal("first tryLaunchLock: held=true, want false")
	}
	if release == nil {
		t.Fatal("first tryLaunchLock: release=nil, want a release")
	}

	if _, held2, err := p.tryLaunchLock(ctx, "sid-a"); err != nil || !held2 {
		t.Fatalf("second tryLaunchLock while held: held=%v err=%v, want true, nil", held2, err)
	}

	release()

	release3, held3, err := p.tryLaunchLock(ctx, "sid-a")
	if err != nil {
		t.Fatalf("third tryLaunchLock: %v", err)
	}
	if held3 {
		t.Fatal("third tryLaunchLock after release: held=true, want false")
	}
	release3()
}

// TestLockOrderIndependentOperationsDoNotInterfere is the concurrency bug the
// per-provider counter had: one goroutine holding a tab section made an
// unrelated goroutine's name section fail with ErrLockOrder, as if the name
// lock were taken under the tab lock. Each operation carries its own nesting
// tracker, so the two must not see each other's sections.
func TestLockOrderIndependentOperationsDoNotInterfere(t *testing.T) {
	p := newLockOrderProvider(t)
	ctxTab := withLockOperation(context.Background())
	ctxName := withLockOperation(context.Background())

	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- p.withTabLock(ctxTab, "rig-a", func() error {
			close(started)
			<-release
			return nil
		})
	}()
	<-started

	// While the tab section is held by the other goroutine, this operation
	// takes a name section. They are independent, so the table must not apply.
	err := p.withNameLock(ctxName, "worker", func() error { return nil })
	close(release)
	if tabErr := <-done; tabErr != nil {
		t.Fatalf("tab section: %v", tabErr)
	}
	if err != nil {
		t.Fatalf("independent name section: %v", err)
	}
}

// TestLockCanceledSectionDoesNotLeakTracking guards the second review finding:
// a canceled wrapper incremented the tracker, then withFlock returned before
// its end ran. A retry in the same operation then failed with ErrLockOrder even
// though no flock was held. Each wrapper now defers its end immediately after a
// successful lockSection, so a canceled acquire unwinds the count.
func TestLockCanceledSectionDoesNotLeakTracking(t *testing.T) {
	cases := []struct {
		name string
		lock func(*Provider, context.Context) error
	}{
		{"state", func(p *Provider, ctx context.Context) error {
			return p.withStateLock(ctx, func() error { return nil })
		}},
		{"name", func(p *Provider, ctx context.Context) error {
			return p.withNameLock(ctx, "worker", func() error { return nil })
		}},
		{"tab", func(p *Provider, ctx context.Context) error {
			return p.withTabLock(ctx, "rig-a", func() error { return nil })
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := newLockOrderProvider(t)
			ctx := withLockOperation(context.Background())
			canceled, cancel := context.WithCancel(ctx)
			cancel()

			if err := tc.lock(p, canceled); !errors.Is(err, context.Canceled) {
				t.Fatalf("canceled acquire: want context.Canceled, got %v", err)
			}
			if err := tc.lock(p, ctx); err != nil {
				t.Fatalf("retry after canceled acquire: %v", err)
			}
		})
	}
}
