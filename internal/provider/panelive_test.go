package provider_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/docbobo/rex-gascity/internal/prochost"
	"github.com/docbobo/rex-gascity/internal/provider"
	"github.com/docbobo/rex-gascity/internal/rex"
	"github.com/docbobo/rex-gascity/internal/rex/faketest"
)

// paneLifeFixture starts Indigo--dog-1 on a fresh fake and puts its pane in
// one liveness case: running, exited (LastExit set), vanished (Rex answers
// ErrNotFound) or unavailable (every process read fails with ErrUnavailable).
// Each op gets its own fixture, because a resolve may drop a stale record and
// change what the next op sees.
func paneLifeFixture(t *testing.T, pane string) *provider.Provider {
	t.Helper()
	fake := faketest.New()
	p := provider.New("acme", fake, provider.WithProcessHost(&prochost.Fake{}))
	name := "Indigo--dog-1"
	if err := p.Start(context.Background(), name, provider.Config{Argv: []string{"sleep", "300"}}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	block := blockOf(t, p, name)
	fake.Formats = map[rex.BlockID]string{block: "hello\n"}
	switch pane {
	case "running":
		fake.Processes = map[rex.BlockID]rex.ProcessInfo{block: {Foreground: &rex.Process{PID: 100, Name: "sleep"}}}
	case "exited":
		fake.Processes = map[rex.BlockID]rex.ProcessInfo{block: {LastExit: &rex.ExitStatus{}}}
	case "vanished":
		fake.VanishedBlocks = map[rex.BlockID]bool{block: true}
	case "unavailable":
		fake.ProcessErr = rex.ErrUnavailable
	default:
		t.Fatalf("unknown pane case %q", pane)
	}
	return p
}

// TestPaneLifeAcrossOps characterizes every op's answer for a running, an
// exited, a vanished and an unreadable pane, as the provider gave them at
// e6ebf9f. The pane-liveness module (panelive.go) must not change any of them:
// each op keeps its own "cannot tell" answer.
func TestPaneLifeAcrossOps(t *testing.T) {
	ctx := context.Background()
	name := "Indigo--dog-1"
	cases := []struct {
		pane         string
		observeState string
		observeAgent string
		observeObj   bool
		invComplete  bool
		invSessions  int
		invState     string
		processAlive provider.Liveness
		peek         string
		peekErr      error
		running      bool
		runningErr   error
		listRunning  []string
	}{
		{
			pane: "running", observeState: provider.ObserveRunning, observeObj: true,
			invComplete: true, invSessions: 1, invState: provider.ObserveRunning,
			processAlive: provider.LivenessAlive, peek: "hello\n",
			running: true, listRunning: []string{name},
		},
		{
			pane: "exited", observeState: provider.ObserveDead, observeAgent: "dead", observeObj: true,
			invComplete: true, invSessions: 1, invState: provider.ObserveDead,
			processAlive: provider.LivenessDead, peek: "hello\n",
			listRunning: []string{},
		},
		{
			pane: "vanished", observeState: provider.ObserveAbsent,
			invComplete: true, invSessions: 0,
			processAlive: provider.LivenessDead, peekErr: provider.ErrNoScreen,
			listRunning: []string{},
		},
		{
			pane: "unavailable", observeState: provider.ObserveUnknown,
			invComplete: false, invSessions: 1, invState: "",
			processAlive: provider.LivenessUnknown, peekErr: provider.ErrNoScreen,
			runningErr: rex.ErrUnavailable, listRunning: []string{},
		},
	}
	for _, tc := range cases {
		t.Run(tc.pane, func(t *testing.T) {
			ans, err := paneLifeFixture(t, tc.pane).Observe(ctx, name, nil)
			if err != nil || ans.State != tc.observeState || ans.Agent != tc.observeAgent || (ans.Object != nil) != tc.observeObj {
				t.Errorf("Observe = %+v, %v; want state %q, agent %q, object %v", ans, err, tc.observeState, tc.observeAgent, tc.observeObj)
			}

			inv, err := paneLifeFixture(t, tc.pane).Inventory(ctx)
			if err != nil || inv.Complete != tc.invComplete || len(inv.Sessions) != tc.invSessions {
				t.Errorf("Inventory = %+v, %v; want complete %v with %d sessions", inv, err, tc.invComplete, tc.invSessions)
			} else if tc.invSessions == 1 && (inv.Sessions[0].Name != name || inv.Sessions[0].State != tc.invState) {
				t.Errorf("Inventory entry = %+v; want %s with state %q", inv.Sessions[0], name, tc.invState)
			}

			live, err := paneLifeFixture(t, tc.pane).ProcessAlive(ctx, name, nil)
			if err != nil || live != tc.processAlive {
				t.Errorf("ProcessAlive = %v, %v; want %v, nil", live, err, tc.processAlive)
			}

			out, err := paneLifeFixture(t, tc.pane).PeekLines(ctx, name, 0)
			if out != tc.peek || !errors.Is(err, tc.peekErr) || (tc.peekErr == nil && err != nil) {
				t.Errorf("PeekLines = %q, %v; want %q, %v", out, err, tc.peek, tc.peekErr)
			}

			running, err := paneLifeFixture(t, tc.pane).IsRunning(ctx, name)
			if running != tc.running || !errors.Is(err, tc.runningErr) || (tc.runningErr == nil && err != nil) {
				t.Errorf("IsRunning = %v, %v; want %v, %v", running, err, tc.running, tc.runningErr)
			}

			names, err := paneLifeFixture(t, tc.pane).ListRunning(ctx, "")
			if err != nil || names == nil || !slices.Equal(names, tc.listRunning) {
				t.Errorf("ListRunning = %#v, %v; want %#v, nil", names, err, tc.listRunning)
			}
		})
	}

	// The recordless exited pane: a restored pane that already exited, with no
	// binding and a stale identity left in the meta store. It moved here from
	// the internal TestObserveRecordlessDeadPaneHasNoIdentity with every
	// assertion. SetMeta cannot seed the key (it refuses a name with no session
	// record), so the store is seeded directly.
	t.Run("recordless exited pane", func(t *testing.T) {
		fake := faketest.New()
		meta := provider.NewMemoryMetaStore()
		if err := meta.Set(provider.SessionName(name), "GC_SESSION_ID", "sid-stale"); err != nil {
			t.Fatalf("meta.Set: %v", err)
		}
		p := provider.New("acme", fake, provider.WithMetaStore(meta), provider.WithProcessHost(&prochost.Fake{}))
		fake.Sessions = []rex.Session{{ID: "session:acme", Label: "gc:acme"}}
		fake.View = rex.SessionView{
			Label:          "gc:acme",
			ActiveWindowID: "window:Indigo",
			Windows: []rex.Window{{ID: "window:Indigo", Label: "Rig: Indigo",
				Blocks: []rex.Block{{ID: "block:dog-1", Label: "dog-1"}}}},
		}
		fake.Processes = map[rex.BlockID]rex.ProcessInfo{
			"block:dog-1": {LastExit: &rex.ExitStatus{}},
		}

		ans, err := p.Observe(ctx, name, nil)
		if err != nil {
			t.Fatalf("Observe: %v", err)
		}
		if ans.State != provider.ObserveDead || ans.Agent != "dead" || ans.Object == nil || ans.Object.ID != "block:dog-1" {
			t.Fatalf("Observe = %+v, want the recordless pane dead", ans)
		}
		if ans.Identity == nil || len(ans.Identity) != 0 {
			t.Errorf("identity = %v, want an empty set", ans.Identity)
		}
		if _, ok := p.Binding(name); ok {
			t.Error("Observe saved a binding, but discovery must stay read-only")
		}
	})
}
