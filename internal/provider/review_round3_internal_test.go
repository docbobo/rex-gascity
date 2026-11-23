package provider

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/docbobo/rex-gascity/internal/prochost"
	"github.com/docbobo/rex-gascity/internal/rex"
	"github.com/docbobo/rex-gascity/internal/rex/faketest"
)

// TestStopReapsRecordedSurvivorsThroughAReplacementScan is CodeRabbit review
// 4648933679: a stale record's survivors are the only name for processes a
// closed pane no longer reports. When the rediscovery scan replaces that record
// with a pane that now holds the labels, the replacement must keep the pending
// survivor set, or the stop forgets pid 300 while it runs.
func TestStopReapsRecordedSurvivorsThroughAReplacementScan(t *testing.T) {
	reapShortWaits(t)
	ctx := context.Background()
	fake, p, signals := reapProvider(t, rex.ProcessInfo{}, func(int) prochost.Table {
		return reapTable([4]string{"300", "1", "S", "bun"})
	})
	recordSurvivor(t, p, SessionName(reapDog), 300, "bun")

	// The old block is gone; a replacement pane now holds the same labels.
	fake.Processes = nil
	fake.VanishedBlocks = map[rex.BlockID]bool{"block:dog-1": true}
	fake.View = rex.SessionView{Label: testLabel, Windows: []rex.Window{{
		ID:     "window:Indigo",
		Label:  "Rig: Indigo",
		Blocks: []rex.Block{{ID: "block:replacement", Label: "dog-1"}},
	}}}

	err := p.Stop(ctx, reapDog)
	if err == nil {
		t.Fatal("Stop = nil, want a survivor failure: pid 300 still runs")
	}
	if !strings.Contains(err.Error(), "300 (bun)") {
		t.Errorf("Stop error %q does not name the surviving 300 (bun)", err)
	}
	if !slices.Contains(*signals, "terminated:300") {
		t.Errorf("signals = %v, want a TERM for the recorded survivor 300", *signals)
	}
	bd, ok := p.Binding(reapDog)
	if !ok {
		t.Fatal("binding gone although pid 300 survives, want it kept for the retry")
	}
	if len(bd.Survivors) != 1 || bd.Survivors[0].PID != 300 {
		t.Errorf("survivors = %+v, want the replacement record to keep pid 300", bd.Survivors)
	}
}

// TestKillObjectUsesTheBindingReadUnderTheLock is CodeRabbit review
// 4648933685: a canonical Stop can record survivors after kill-object's first
// read. The callback reaps and retires with the record it reads under the
// state lock, not the snapshot from before it, or it deletes the only record
// that names pid 300.
func TestKillObjectUsesTheBindingReadUnderTheLock(t *testing.T) {
	reapShortWaits(t)
	ctx := context.Background()
	fake := faketest.New()
	var p *Provider
	client := &swapViewClient{Fake: fake}
	p = New("acme", client)
	if err := p.Start(ctx, "Indigo--dog-1", Config{Argv: []string{"sleep", "300"}}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	bd, ok := p.Binding("Indigo--dog-1")
	if !ok {
		t.Fatal("no binding after Start")
	}
	bd.Aliases = []SessionName{"Rig--dog-1"}
	if err := p.binder.Record(bd); err != nil {
		t.Fatalf("Record: %v", err)
	}
	// The pane is gone. The swap stands in for a concurrent canonical Stop
	// that records survivor 300 after kill-object's first read.
	fake.View = rex.SessionView{}
	fake.Processes = nil
	fake.VanishedBlocks = map[rex.BlockID]bool{bd.BlockID: true}
	client.swap = func() {
		got, ok := p.Binding("Indigo--dog-1")
		if !ok {
			t.Error("binding vanished in the swap")
			return
		}
		got.Survivors = append(got.Survivors, Survivor{PID: 300, Name: "bun", Start: reapStart})
		if err := p.binder.Record(got); err != nil {
			t.Errorf("Record: %v", err)
		}
	}
	var signals []string
	p.processHost = &reapTestHost{givePs: func(int) prochost.Table {
		return reapTable([4]string{"300", "1", "S", "bun"})
	}, signals: &signals}

	_, err := p.KillObject(ctx, "Rig--dog-1", KillObjectRequest{Object: observeObjectFor(bd), Mode: "corpse"})
	if err == nil {
		t.Fatal("KillObject = nil, want a survivor failure: the record read under the lock names pid 300")
	}
	if !strings.Contains(err.Error(), "300 (bun)") {
		t.Errorf("KillObject error %q does not name the surviving 300 (bun)", err)
	}
	got, ok := p.Binding("Indigo--dog-1")
	if !ok {
		t.Fatal("binding gone although pid 300 survives")
	}
	if len(got.Survivors) != 1 || got.Survivors[0].PID != 300 {
		t.Errorf("survivors = %+v, want pid 300 kept for the retry", got.Survivors)
	}
	if !slices.Contains(signals, "terminated:300") {
		t.Errorf("signals = %v, want a TERM for pid 300", signals)
	}
}

// failOnceRecordStore fails the first write that carries a block ID, standing
// in for a binding write that fails after the pane was created.
type failOnceRecordStore struct {
	*MemoryStore
	mu     sync.Mutex
	failed bool
}

func (s *failOnceRecordStore) Write(k SessionName, v Binding) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v.BlockID != "" && !s.failed {
		s.failed = true
		return errStoreWrite
	}
	return s.MemoryStore.Write(k, v)
}

// TestStartRetiresThePaneWhoseRecordFailed is CodeRabbit review 4648933688: a
// failed final record must not leave the pane it just created running with no
// start identity for the next Start to adopt.
func TestStartRetiresThePaneWhoseRecordFailed(t *testing.T) {
	for _, tc := range []struct {
		name  string
		view  rex.SessionView
		block rex.BlockID
	}{
		{
			name: "split",
			view: rex.SessionView{Label: testLabel, Windows: []rex.Window{{
				ID:     "window:Indigo",
				Label:  "Rig: Indigo",
				Blocks: []rex.Block{{ID: "block:other", Label: "other"}},
			}}},
			block: "block:dog-1",
		},
		{
			name:  "window",
			view:  rex.SessionView{Label: testLabel},
			block: "block:dog-1",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			fake := faketest.New()
			fake.Sessions = []rex.Session{{ID: "session:acme", Label: testLabel}}
			fake.View = tc.view
			store := &failOnceRecordStore{MemoryStore: NewMemoryStore()}
			p := New("acme", fake, WithBindingStore(store))

			err := p.Start(ctx, reapDog, Config{Argv: []string{"claude"}})
			if !errors.Is(err, errStoreWrite) {
				t.Fatalf("Start = %v, want the record write error", err)
			}
			if !slices.Contains(fake.ClosedBlocks, tc.block) {
				t.Errorf("closed = %v, want the created pane %s retired", fake.ClosedBlocks, tc.block)
			}
			for _, w := range fake.View.Windows {
				for _, b := range w.Blocks {
					if b.ID == tc.block {
						t.Errorf("pane %s still in the view after the failed record", tc.block)
					}
				}
			}
			if _, ok := p.Binding(reapDog); ok {
				t.Error("binding kept although its pane was never recorded")
			}
		})
	}
}
