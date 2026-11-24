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
	fake, p, host := reapProvider(t, rex.ProcessInfo{}, func(int) prochost.Table {
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
	if !slices.Contains(signalLog(host), "terminated:300") {
		t.Errorf("signals = %v, want a TERM for the recorded survivor 300", signalLog(host))
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
	host := &prochost.Fake{OnTable: func(int, bool) (prochost.Table, error) {
		return reapTable([4]string{"300", "1", "S", "bun"}), nil
	}}
	p = New("acme", client, WithProcessHost(host))
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
	if !slices.Contains(signalLog(host), "terminated:300") {
		t.Errorf("signals = %v, want a TERM for pid 300", signalLog(host))
	}
}

// failOnceRecordStore fails the first write that carries a block ID, standing
// in for a binding write that fails after the pane was created. onFail, when
// set, runs inside that failing Write, before it returns; it must not read the
// binding store (s.mu is held).
type failOnceRecordStore struct {
	*MemoryStore
	mu     sync.Mutex
	failed bool
	onFail func()
}

func (s *failOnceRecordStore) Write(k SessionName, v Binding) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v.BlockID != "" && !s.failed {
		s.failed = true
		if s.onFail != nil {
			s.onFail()
		}
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

// vanishAfterLaunch marks the block each NewWindow creates as vanished, so its
// BlockProcess answers rex.ErrNotFound (the missing-block snapshot case), and
// checks on CloseBlock entry that a BlockProcess call for that block came
// first. Both overrides touch Fake fields only while no Fake method is in
// flight: after NewWindow returned, and before CloseBlock delegates.
type vanishAfterLaunch struct {
	*faketest.Fake
	launched            rex.BlockID
	closeChecked        bool
	snapshotBeforeClose bool
}

func (v *vanishAfterLaunch) NewWindow(ctx context.Context, sid rex.SessionID, label string, spec rex.LaunchSpec) (rex.WindowID, rex.BlockID, error) {
	wid, block, err := v.Fake.NewWindow(ctx, sid, label, spec)
	if err != nil {
		return wid, block, err
	}
	v.launched = block
	if v.VanishedBlocks == nil {
		v.VanishedBlocks = map[rex.BlockID]bool{}
	}
	v.VanishedBlocks[block] = true
	return wid, block, nil
}

func (v *vanishAfterLaunch) CloseBlock(ctx context.Context, sid rex.SessionID, block rex.BlockID) error {
	if block == v.launched {
		v.closeChecked = true
		for _, call := range v.BlockCalls {
			if call.BlockID == block && call.Op == "process" {
				v.snapshotBeforeClose = true
			}
		}
	}
	return v.Fake.CloseBlock(ctx, sid, block)
}

// TestStartRecordFailureDoesNotRetryAsVanishedSession is issue #76 slice 1
// edge (BR-2). The final record fails after the pane was created, and the
// discard retire's BlockProcess answers rex.ErrNotFound. paneSubtree swallows
// that answer (reap.go:87-94), so the start fails with the record error, not
// as a vanished session, and launches no second pane. Whether an ErrNotFound
// can escape record/discard and reach the retry guard stays unverified: no
// production step on this path produces one.
func TestStartRecordFailureDoesNotRetryAsVanishedSession(t *testing.T) {
	const (
		sessionID = "mc-record-fail"
		token     = "token-record-fail"
	)
	want := map[string]string{"GC_SESSION_ID": sessionID, "GC_INSTANCE_TOKEN": token}
	ctx := context.Background()
	client := &vanishAfterLaunch{Fake: faketest.New()}
	store := &failOnceRecordStore{MemoryStore: NewMemoryStore()}
	p := New("acme", client, WithBindingStore(store))

	type metaRead struct {
		v   string
		ok  bool
		err error
	}
	reads := map[string]metaRead{}
	store.onFail = func() {
		// metaGet reads only the meta store. The public GetMeta would resolve
		// the name through the binding store whose Write is running.
		for key := range want {
			v, ok, err := p.metaGet(SessionName(reapDog), key)
			reads[key] = metaRead{v: v, ok: ok, err: err}
		}
	}

	err := p.Start(ctx, reapDog, Config{Argv: []string{"claude"}, Env: map[string]string{
		"GC_SESSION_ID": sessionID, "GC_INSTANCE_TOKEN": token,
	}})

	for key, v := range want {
		got, seen := reads[key]
		if !seen || got.err != nil || !got.ok || got.v != v {
			t.Errorf("%s before the record failed = %+v (seen %v); want %q present", key, got, seen, v)
		}
	}
	if !client.closeChecked {
		t.Fatal("CloseBlock never ran for the launched block: the pane was not discarded")
	}
	if !client.snapshotBeforeClose {
		t.Error("no BlockProcess call for the launched block before its CloseBlock: the pre-close snapshot did not run")
	}
	if !errors.Is(err, errStoreWrite) {
		t.Fatalf("Start = %v, want the record write error", err)
	}
	if errors.Is(err, rex.ErrNotFound) {
		t.Errorf("Start = %v, matches rex.ErrNotFound: a record failure would read as a vanished session", err)
	}
	if n := len(client.CreatedWindows) + len(client.CreatedSplits); n != 1 {
		t.Errorf("launches = %d, want 1: a record failure must not retry the launch", n)
	}
	for key := range want {
		if v, err := p.GetMeta(ctx, reapDog, key); err != nil || v != "" {
			t.Errorf("%s after the failed start = %q, %v; want cleared", key, v, err)
		}
	}
}
