package provider

import (
	"context"
	"testing"

	"github.com/docbobo/rex-gascity/internal/rex"
	"github.com/docbobo/rex-gascity/internal/rex/faketest"
)

// TestStopUnreachableWithoutBindingClearsMeta pins GCREX-51 REQ-001: the
// no-binding unreachable branch still clears the name's meta sidecar and
// activity record, so nothing of a retired incarnation outlives stop. Public
// SetMeta cannot seed a binding-less name, hence the internal test.
func TestStopUnreachableWithoutBindingClearsMeta(t *testing.T) {
	const n SessionName = "Indigo--dog-1"
	fake := faketest.New()
	p := New("acme", fake)
	if err := p.metaSet(n, "GC_USER_KEY", "stale"); err != nil {
		t.Fatal(err)
	}
	p.activityWrite(n, ActivityRecord{Identity: "block:orphan"})
	fake.ListSessionsErr = rex.ErrUnavailable

	if err := p.Stop(context.Background(), string(n)); err != nil {
		t.Fatalf("Stop = %v, want nil", err)
	}
	if v, ok, _ := p.metaGet(n, "GC_USER_KEY"); ok {
		t.Errorf("meta survived Stop: %q", v)
	}
	if rec, ok := p.activityRead(n); ok {
		t.Errorf("activity survived Stop: %+v", rec)
	}
}
