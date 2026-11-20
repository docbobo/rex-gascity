package provider_test

import (
	"context"
	"testing"

	"github.com/docbobo/rex-gascity/internal/provider"
	"github.com/docbobo/rex-gascity/internal/rex/faketest"
)

// TestObserveAbsentForUnknownName proves RPP-OBSERVE-002: a name no record and
// no live pane holds answers absent, with no object.
func TestObserveAbsentForUnknownName(t *testing.T) {
	ctx := context.Background()
	fake := faketest.New()
	p := provider.New("acme", fake)

	ans, err := p.Observe(ctx, "Indigo--dog-1", nil)
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if ans.State != provider.ObserveAbsent {
		t.Errorf("state = %q, want %q", ans.State, provider.ObserveAbsent)
	}
	if ans.Object != nil {
		t.Errorf("object = %+v, want nil for an absent name", ans.Object)
	}
}

// TestObserveRunningCarriesObjectAndIdentity proves RPP-OBSERVE-003: a started
// box answers running with its object and the start env's identity, read from
// the meta sidecar seedStartIdentity wrote before the launch.
func TestObserveRunningCarriesObjectAndIdentity(t *testing.T) {
	ctx := context.Background()
	fake := faketest.New()
	p := provider.New("acme", fake)
	name := "Indigo--dog-1"
	env := map[string]string{
		"GC_SESSION_ID":     "sid-1",
		"GC_INSTANCE_TOKEN": "tok-1",
	}
	if err := p.Start(ctx, name, provider.Config{Argv: []string{"sleep", "300"}, Env: env}); err != nil {
		t.Fatalf("Start: %v", err)
	}

	ans, err := p.Observe(ctx, name, nil)
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if ans.State != provider.ObserveRunning {
		t.Fatalf("state = %q, want %q", ans.State, provider.ObserveRunning)
	}
	if ans.Object == nil || ans.Object.ID == "" || ans.Object.Created == "" {
		t.Errorf("object = %+v, want a non-empty id and created", ans.Object)
	}
	if ans.Identity["GC_SESSION_ID"] != "sid-1" || ans.Identity["GC_INSTANCE_TOKEN"] != "tok-1" {
		t.Errorf("identity = %v, want the start env's", ans.Identity)
	}
}

// TestKillObjectGoneForUnknownObject proves the "never kill by name" rule: an
// object that does not match the stored binding answers gone and kills nothing.
func TestKillObjectGoneForUnknownObject(t *testing.T) {
	ctx := context.Background()
	fake := faketest.New()
	p := provider.New("acme", fake)
	name := "Indigo--dog-1"
	if err := p.Start(ctx, name, provider.Config{Argv: []string{"sleep", "300"}}); err != nil {
		t.Fatalf("Start: %v", err)
	}

	res, err := p.KillObject(ctx, name, provider.KillObjectRequest{
		Object: provider.ObserveObject{ID: "not-the-object", Created: "1970-01-01T00:00:00Z"},
		Mode:   "corpse",
	})
	if err != nil {
		t.Fatalf("KillObject: %v", err)
	}
	if res != "gone" {
		t.Errorf("result = %q, want gone", res)
	}
	// The box survives a mismatch.
	if running, err := p.IsRunning(ctx, name); err != nil || !running {
		t.Errorf("IsRunning after a mismatched kill = %v, %v; want true, nil", running, err)
	}
}
