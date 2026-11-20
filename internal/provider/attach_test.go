package provider_test

import (
	"context"
	"errors"
	"testing"

	"github.com/docbobo/rex-gascity/internal/provider"
	"github.com/docbobo/rex-gascity/internal/rex"
	"github.com/docbobo/rex-gascity/internal/rex/faketest"
)

const (
	attachName  = "Indigo--dog-1"
	attachBlock = rex.BlockID("block:dog-1")
)

var (
	attachApp = rex.ClientInfo{ID: "c1", Kind: "app", Name: "Rex"}
	attachCLI = rex.ClientInfo{ID: "c2", Kind: "cli", Name: "gc"}
)

// attachStarted starts the agent, adds a second tab, and sets the clients and
// which tab is active and which block the agent's tab focuses. The block has no
// owner unless a test makes one with attachOwned.
func attachStarted(t *testing.T, active rex.WindowID, focused rex.BlockID, clients ...rex.ClientInfo) *provider.Provider {
	t.Helper()
	_, p := attachOwned(t, "", active, focused, clients...)
	return p
}

// attachOwned is attachStarted with the block's size owner set: the client the
// block is displayed for, empty when no client has displayed it.
func attachOwned(t *testing.T, owner string, active rex.WindowID, focused rex.BlockID, clients ...rex.ClientInfo) (*faketest.Fake, *provider.Provider) {
	t.Helper()
	fake, p := started(t)
	fake.Clients = clients
	fake.Sizes = map[rex.BlockID]rex.BlockSize{attachBlock: {Owner: owner}}
	fake.View.Windows = append(fake.View.Windows, rex.Window{ID: "window:Other", Label: "Other"})
	fake.View.ActiveWindowID = active
	fake.View.Windows[0].FocusedBlockID = focused
	return fake, p
}

func attachWant(t *testing.T, p *provider.Provider, want bool) {
	t.Helper()
	got, err := p.IsAttached(context.Background(), attachName)
	if err != nil {
		t.Fatalf("IsAttached: %v", err)
	}
	if got != want {
		t.Errorf("IsAttached = %v, want %v", got, want)
	}
}

func TestIsAttachedNoClients(t *testing.T) {
	attachWant(t, attachStarted(t, "window:Rig: Indigo", "block:dog-1"), false)
}

func TestIsAttachedCLIOnly(t *testing.T) {
	attachWant(t, attachStarted(t, "window:Rig: Indigo", "block:dog-1", attachCLI), false)
}

func TestIsAttachedOwnedByAppInActiveTab(t *testing.T) {
	_, p := attachOwned(t, attachApp.ID, "window:Rig: Indigo", "", attachApp, attachCLI)
	attachWant(t, p, true)
}

func TestIsAttachedOwnedByAppInInactiveTab(t *testing.T) {
	_, p := attachOwned(t, attachApp.ID, "window:Other", "block:dog-1", attachApp)
	attachWant(t, p, false)
}

func TestIsAttachedOwnedByAppOtherRigActive(t *testing.T) {
	_, p := attachOwned(t, attachApp.ID, "window:Other", "", attachApp)
	attachWant(t, p, false)
}

func TestIsAttachedOwnedBlockGoneFromActiveTab(t *testing.T) {
	fake, p := attachOwned(t, attachApp.ID, "window:Rig: Indigo", "", attachApp)
	fake.View.Windows[0].Blocks = nil
	attachWant(t, p, false)
}

// TestIsAttachedAppAttachedButBlockUnowned is the GCREX-108 bug: the app client
// is attached to the session but never displayed this box, so nobody is
// watching it. The old provider treated the session-level app client as
// attachment.
func TestIsAttachedAppAttachedButBlockUnowned(t *testing.T) {
	_, p := attachOwned(t, "", "window:Rig: Indigo", "", attachApp, attachCLI)
	attachWant(t, p, false)
}

func TestIsAttachedOwnedByCLI(t *testing.T) {
	_, p := attachOwned(t, attachCLI.ID, "window:Rig: Indigo", "", attachApp, attachCLI)
	attachWant(t, p, false)
}

func TestIsAttachedOwnedByPeer(t *testing.T) {
	peer := rex.ClientInfo{ID: "p1", Kind: "peer", Name: "peer"}
	_, p := attachOwned(t, peer.ID, "window:Rig: Indigo", "", attachApp, peer)
	attachWant(t, p, false)
}

// TestIsAttachedStaleOwner: ownership is a last-requested claim, not a
// heartbeat, so an owner no longer in the client list errs toward true rather
// than closing a pane someone may still be watching.
func TestIsAttachedStaleOwner(t *testing.T) {
	_, p := attachOwned(t, "c9", "window:Rig: Indigo", "", attachApp)
	attachWant(t, p, true)
}

func TestIsAttachedUnknownOwnerKind(t *testing.T) {
	for _, kind := range []string{"", "mystery"} {
		_, p := attachOwned(t, "x", "window:Rig: Indigo", "block:dog-1", attachApp, rex.ClientInfo{ID: "x", Kind: kind})
		got, err := p.IsAttached(context.Background(), attachName)
		if err != nil {
			t.Errorf("kind %q: IsAttached error = %v, want nil", kind, err)
		}
		if !got {
			t.Errorf("kind %q: IsAttached = false, want true (answer true when the kind is unclassifiable)", kind)
		}
	}
}

func TestIsAttachedListClientsError(t *testing.T) {
	fake, p := attachOwned(t, attachApp.ID, "window:Rig: Indigo", "", attachApp)
	boom := errors.New("rex down")
	fake.ListClientsErr = boom
	got, err := p.IsAttached(context.Background(), attachName)
	if !errors.Is(err, boom) || got {
		t.Errorf("IsAttached = %v, %v; want false and the transport error", got, err)
	}
}

func TestIsAttachedBlockSizeError(t *testing.T) {
	fake, p := attachOwned(t, attachApp.ID, "window:Rig: Indigo", "", attachApp)
	boom := errors.New("rex down")
	fake.BlockSizeErr = boom
	got, err := p.IsAttached(context.Background(), attachName)
	if !errors.Is(err, boom) || got {
		t.Errorf("IsAttached = %v, %v; want false and the transport error", got, err)
	}
}

func TestIsAttachedMissingPane(t *testing.T) {
	fake, p := started(t)
	fake.Clients = []rex.ClientInfo{attachApp}
	got, err := p.IsAttached(context.Background(), "Indigo--ghost")
	if got || err != nil {
		t.Errorf("IsAttached = %v, %v; want false, nil", got, err)
	}
}
