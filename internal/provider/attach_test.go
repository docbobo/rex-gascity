package provider_test

import (
	"context"
	"errors"
	"testing"

	"github.com/docbobo/rex-gascity/internal/provider"
	"github.com/docbobo/rex-gascity/internal/rex"
)

const attachName = "Indigo--dog-1"

var (
	attachApp = rex.ClientInfo{ID: "c1", Kind: "app", Name: "Rex"}
	attachCLI = rex.ClientInfo{ID: "c2", Kind: "cli", Name: "gc"}
)

// attachStarted starts the agent, adds a second tab, and sets the clients and
// which tab is active and which block the agent's tab focuses.
func attachStarted(t *testing.T, active rex.WindowID, focused rex.BlockID, clients ...rex.ClientInfo) *provider.Provider {
	t.Helper()
	fake, p := started(t)
	fake.Clients = clients
	fake.View.Windows = append(fake.View.Windows, rex.Window{ID: "window:Other", Label: "Other"})
	fake.View.ActiveWindowID = active
	fake.View.Windows[0].FocusedBlockID = focused
	return p
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

func TestIsAttachedAgentTabActive(t *testing.T) {
	attachWant(t, attachStarted(t, "window:Rig: Indigo", "", attachApp, attachCLI), true)
}

func TestIsAttachedFocusedBlockInInactiveTab(t *testing.T) {
	attachWant(t, attachStarted(t, "window:Other", "block:dog-1", attachApp), false)
}

func TestIsAttachedOtherRigActive(t *testing.T) {
	attachWant(t, attachStarted(t, "window:Other", "", attachApp), false)
}

func TestIsAttachedBlockGoneFromActiveTab(t *testing.T) {
	fake, p := started(t)
	fake.Clients = []rex.ClientInfo{attachApp}
	fake.View.ActiveWindowID = "window:Rig: Indigo"
	fake.View.Windows[0].Blocks = nil
	attachWant(t, p, false)
}

func TestIsAttachedPeerOnly(t *testing.T) {
	attachWant(t, attachStarted(t, "window:Rig: Indigo", "block:dog-1", rex.ClientInfo{ID: "p1", Kind: "peer", Name: "peer"}), false)
}

func TestIsAttachedUnknownKind(t *testing.T) {
	for _, kind := range []string{"", "mystery"} {
		p := attachStarted(t, "window:Rig: Indigo", "block:dog-1", attachApp, rex.ClientInfo{ID: "x", Kind: kind})
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
	fake, p := started(t)
	boom := errors.New("rex down")
	fake.ListClientsErr = boom
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
