package provider

import (
	"context"
	"errors"
	"testing"

	"github.com/docbobo/rex-gascity/internal/rex/faketest"
)

// TestLockOrderHoldsAcrossStartAndStop arms the guard and drives the real
// Start and Stop against the fake, so the nesting table is checked against the
// order the code actually takes, not only against synthetic sections. A start
// or stop that violated the table would fail here with ErrLockOrder instead of
// only showing up as a wrong doc comment.
func TestLockOrderHoldsAcrossStartAndStop(t *testing.T) {
	armLockOrder(t)
	ctx := context.Background()
	p := New("acme", faketest.New(), WithNamespace(NewNamespaceForDirs(t.TempDir())))

	if err := p.Start(ctx, "Indigo--a", Config{Argv: []string{"pi"}}); err != nil {
		t.Fatalf("Start under the guard: %v", err)
	}
	if err := p.Stop(ctx, "Indigo--a"); err != nil {
		t.Fatalf("Stop under the guard: %v", err)
	}
}

// TestLockOrderHoldsAcrossStartAndStartNew drives both start entry points under
// the guard. StartNew re-enters the name lock and then the launch, which is the
// name -> launch edge of the table.
func TestLockOrderHoldsAcrossStartAndStartNew(t *testing.T) {
	armLockOrder(t)
	ctx := context.Background()
	p := New("acme", faketest.New(), WithNamespace(NewNamespaceForDirs(t.TempDir())))

	if err := p.Start(ctx, "Indigo--a", Config{Argv: []string{"pi"}}); err != nil {
		t.Fatalf("Start under the guard: %v", err)
	}
	if err := p.StartNew(ctx, "Violet--b", Config{Argv: []string{"pi"}}); err != nil {
		t.Fatalf("StartNew under the guard: %v", err)
	}
}

// TestLockOrderGuardFailureSurfaces confirms that when the guard does fire on a
// real call it surfaces ErrLockOrder, not some later flock hang. A provider with
// the guard armed and a poisoned held count (pretending a tab lock is still
// held) must refuse the next tab section with ErrLockOrder.
func TestLockOrderGuardFailureSurfaces(t *testing.T) {
	p := newLockOrderProvider(t)
	p.lockTracking.held[lockTab] = 1 // a leaked tab section
	err := p.withTabLock(context.Background(), "rig-a", func() error { return nil })
	if !errors.Is(err, ErrLockOrder) {
		t.Fatalf("second tab section with a leaked hold: want ErrLockOrder, got %v", err)
	}
}
