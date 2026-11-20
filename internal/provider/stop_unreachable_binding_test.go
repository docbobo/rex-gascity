package provider

import (
	"context"
	"errors"
	"testing"

	"github.com/docbobo/rex-gascity/internal/rex"
	"github.com/docbobo/rex-gascity/internal/rex/faketest"
)

// TestStopUnreachableAliasBindingKeepsError pins REQ-002 for a name that only
// matches a stored binding through its alias: that is still a binding present,
// so Stop with Rex down keeps the error and the record, not just for an exact
// name. It fails against an implementation that tests only the exact-name
// binding (bindingRead) instead of Binder.lookup.
func TestStopUnreachableAliasBindingKeepsError(t *testing.T) {
	const primary, alias = SessionName("Indigo--dog-1"), SessionName("rig/dog-1")
	fake := faketest.New()
	p := New("acme", fake)
	if err := p.Start(context.Background(), string(primary), Config{Argv: []string{"/usr/local/bin/claude"}}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	b, ok := p.bindingRead(primary)
	if !ok {
		t.Fatal("no binding after Start")
	}
	b.Aliases = []SessionName{alias}
	if err := p.bindings.Write(primary, b); err != nil {
		t.Fatalf("write alias: %v", err)
	}
	p.activityWrite(primary, ActivityRecord{Identity: "block:keep"})
	fake.SessionViewErr = rex.ErrUnavailable

	if err := p.Stop(context.Background(), string(alias)); !errors.Is(err, rex.ErrUnavailable) {
		t.Fatalf("Stop = %v, want ErrUnavailable", err)
	}
	if _, ok := p.bindingRead(primary); !ok {
		t.Error("binding dropped although Stop failed")
	}
	if rec, ok := p.activityRead(primary); !ok || rec.Identity != "block:keep" {
		t.Errorf("activity = %+v, %v, want it kept", rec, ok)
	}
	if len(fake.ClosedBlocks) != 0 {
		t.Errorf("closed = %v, want none", fake.ClosedBlocks)
	}
}
