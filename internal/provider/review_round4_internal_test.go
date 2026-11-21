package provider

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/docbobo/rex-gascity/internal/rex"
	"github.com/docbobo/rex-gascity/internal/rex/faketest"
)

// TestStartFailsWhilePendingSurvivorsSurvive is CodeRabbit review
// PRRT_kwDOVyObGc66TLlt: a failed launch whose pane was closed but whose
// descendant could not be reaped keeps the pid on the binding. ResolveFresh
// drops that stale record, and Start would write a new one without the pid,
// leaving the process running with no recovery record. A survivor that will
// not die must fail the start and keep the binding for the retry.
func TestStartFailsWhilePendingSurvivorsSurvive(t *testing.T) {
	reapShortWaits(t)
	ctx := context.Background()
	fake := faketest.New()
	store := &failOnceRecordStore{MemoryStore: NewMemoryStore()}
	p := New("acme", fake, WithBindingStore(store))
	p.signal = func(int, os.Signal) error { return nil }
	p.runner = &FakeRunner{Handle: func(_ context.Context, c RunCall) (string, error) {
		if c.Name != "ps" {
			return "", nil
		}
		// The pane's shell is a zombie after the close; pid 300 still runs.
		return reapTable(
			[4]string{"100", "1", "Z", "claude"},
			[4]string{"300", "100", "S", "bun"},
		), nil
	}}
	fake.Processes = map[rex.BlockID]rex.ProcessInfo{
		"block:dog-1": {Foreground: &rex.Process{PID: 100, Name: "claude"}},
	}

	err := p.Start(ctx, reapDog, Config{Argv: []string{"claude"}})
	if !errors.Is(err, errStoreWrite) {
		t.Fatalf("first Start = %v, want the record write error", err)
	}
	bd, ok := p.Binding(reapDog)
	if !ok {
		t.Fatal("binding gone after the failed launch, want it kept for the survivor")
	}
	if len(bd.Survivors) != 1 || bd.Survivors[0].PID != 300 {
		t.Fatalf("survivors = %+v, want pid 300 kept", bd.Survivors)
	}

	// The pane is gone and pid 300 still runs: the second start must not drop
	// the only record that names it.
	err = p.Start(ctx, reapDog, Config{Argv: []string{"claude"}})
	if err == nil {
		t.Fatal("second Start = nil, want the pending survivor to fail it")
	}
	for _, want := range []string{"300 (bun)", "retry"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("second Start error %q does not name %q", err, want)
		}
	}
	got, ok := p.Binding(reapDog)
	if !ok {
		t.Fatal("binding dropped although pid 300 survives")
	}
	if len(got.Survivors) != 1 || got.Survivors[0].PID != 300 {
		t.Errorf("survivors = %+v, want pid 300 kept for the retry", got.Survivors)
	}
}
