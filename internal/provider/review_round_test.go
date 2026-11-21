package provider_test

import (
	"context"
	"testing"

	"github.com/docbobo/rex-gascity/internal/provider"
	"github.com/docbobo/rex-gascity/internal/rex"
)

// TestKillObjectLiveWhenDeadPaneMovedToTheActiveWindow proves attachment follows
// the pane's live location: a human who moved the agent's pane into another
// window and made that window active is watching it, so corpse mode must answer
// live, not kill the screen the human is looking at.
func TestKillObjectLiveWhenDeadPaneMovedToTheActiveWindow(t *testing.T) {
	fake, p := started(t)
	name := "Indigo--dog-1"
	ans, err := p.Observe(context.Background(), name, nil)
	if err != nil || ans.Object == nil {
		t.Fatalf("Observe = %+v, %v; want a box with an object", ans, err)
	}
	fake.Processes = map[rex.BlockID]rex.ProcessInfo{
		blockOf(t, p, name): {LastExit: &rex.ExitStatus{ExitCode: 0}},
	}
	fake.Clients = []rex.ClientInfo{{ID: "c1", Kind: "app", Name: "Rex"}}
	// The human dragged the pane into a new window and switched to it; the
	// binding still records the window the pane was started in.
	fake.View.Windows = []rex.Window{{
		ID:     "window:moved",
		Blocks: []rex.Block{{ID: blockOf(t, p, name), Owner: "c1"}},
	}}
	fake.View.ActiveWindowID = "window:moved"

	res, err := p.KillObject(context.Background(), name, provider.KillObjectRequest{Object: *ans.Object, Mode: "corpse"})
	if err != nil {
		t.Fatalf("KillObject: %v", err)
	}
	if res != "live" {
		t.Errorf("result = %q, want live for a dead pane in the active window", res)
	}
	if len(fake.ClosedBlocks) != 0 {
		t.Errorf("closed blocks = %v, want none while a human watches the pane", fake.ClosedBlocks)
	}
}

// TestKillObjectGoneWhenTheCloseRacesAnotherClient proves the close race: Rex
// answering not-found means another client closed the exact block first, which
// is what the caller asked for, so kill-object answers gone instead of failing.
func TestKillObjectGoneWhenTheCloseRacesAnotherClient(t *testing.T) {
	fake, p := started(t)
	name := "Indigo--dog-1"
	ans, err := p.Observe(context.Background(), name, nil)
	if err != nil || ans.Object == nil {
		t.Fatalf("Observe = %+v, %v; want a box with an object", ans, err)
	}
	fake.Processes = map[rex.BlockID]rex.ProcessInfo{
		blockOf(t, p, name): {LastExit: &rex.ExitStatus{ExitCode: 0}},
	}
	fake.CloseBlockErr = rex.ErrNotFound

	res, err := p.KillObject(context.Background(), name, provider.KillObjectRequest{Object: *ans.Object, Mode: "corpse"})
	if err != nil {
		t.Fatalf("KillObject: %v, want nil when another client closed the block", err)
	}
	if res != "gone" {
		t.Errorf("result = %q, want gone for the vanished block", res)
	}
	if _, ok := p.Binding(name); ok {
		t.Error("binding survived a close another client won")
	}
}
