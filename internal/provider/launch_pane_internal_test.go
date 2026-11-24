package provider

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/docbobo/rex-gascity/internal/rex"
	"github.com/docbobo/rex-gascity/internal/rex/faketest"
)

// TestRecordOrDiscardRetiresPaneWhenRecordFails is REQ-003: the one
// record-or-discard seam retires the pane whose final record failed under the
// discardLaunchPolicy (snapshot, close, re-read, reap; no clear, no forget, no
// destroy), returns the record error, keeps the provisional record and the
// seeded key for start's rollback, and destroys no session under the launch
// lock.
func TestRecordOrDiscardRetiresPaneWhenRecordFails(t *testing.T) {
	reapShortWaits(t)
	ctx := context.Background()
	c := &retireClient{Fake: faketest.New(), failProcess: map[int]error{}}
	c.View = rex.SessionView{Label: testLabel, Windows: []rex.Window{{
		ID:     "window:Indigo",
		Label:  "Rig: Indigo",
		Blocks: []rex.Block{{ID: "block:dog-1", Label: "dog-1"}},
	}}}
	store := &failOnceRecordStore{MemoryStore: NewMemoryStore()}
	p := New("acme", c, WithBindingStore(store))
	p.recordProvisionalBinding(reapDog, "session:acme", "", "attempt", Config{})
	bd := p.newBinding(reapDog, "session:acme", "window:Indigo", "block:dog-1", "split", "attempt", Config{})

	if err := p.recordOrDiscard(ctx, reapDog, bd); !errors.Is(err, errStoreWrite) {
		t.Fatalf("recordOrDiscard = %v, want the record error", err)
	}
	if !slices.Contains(c.ClosedBlocks, "block:dog-1") {
		t.Errorf("closed = %v, want the created pane retired", c.ClosedBlocks)
	}
	if !slices.Contains(c.got(), "process") {
		t.Errorf("calls = %v, want a process snapshot", c.got())
	}
	if _, ok := p.Binding(reapDog); !ok {
		t.Error("binding dropped; the discard policy leaves it for start's rollback")
	}
	key := metaKey{reapDog, startMetaKeys()[0]}
	if err := p.meta.Write(key, "seeded"); err != nil {
		t.Fatalf("seed identity: %v", err)
	}
	_, ok, readErr := p.meta.Read(key)
	if readErr != nil {
		t.Fatalf("meta read: %v", readErr)
	}
	if !ok {
		t.Error("seeded key dropped; the discard policy leaves it for start's rollback")
	}
	if len(c.DestroyedSessions) != 0 {
		t.Errorf("destroyed = %v, want none under the launch lock", c.DestroyedSessions)
	}
}
