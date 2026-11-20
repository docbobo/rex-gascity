package provider_test

import (
	"context"
	"testing"

	"github.com/docbobo/rex-gascity/internal/provider"
	"github.com/docbobo/rex-gascity/internal/rex"
	"github.com/docbobo/rex-gascity/internal/rex/faketest"
)

// TestStopKeepsCitySession is the SPEC-3 regression: the live probe
// (docs/REX-CONTRACT.md §12) lists a server-wide Rex app client on every
// session, so the approved skip branch applies and neither Stop path may
// destroy the city session. rex.Client has no session-destroy op, so the
// observable boundary is the post-condition: the session Start created is
// still listed with the same ID after Stop, with and without a pane.
func TestStopKeepsCitySession(t *testing.T) {
	for _, tc := range []struct {
		name   string
		kind   string
		noPane bool
	}{
		{"pane, cli only", "cli", false},
		{"pane, app listed", "app", false},
		{"no pane, cli only", "cli", true},
		{"no pane, app listed", "app", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := faketest.New()
			p := provider.New("acme", fake)
			teardownStart(t, p, teardownDog)
			fake.Clients = []rex.ClientInfo{{ID: "client:1", Kind: tc.kind}}
			if tc.noPane {
				fake.View.Windows = nil
			}
			if err := p.Stop(context.Background(), teardownDog); err != nil {
				t.Fatalf("Stop: %v", err)
			}
			for _, s := range fake.Sessions {
				if s.ID == "session:gc:acme" {
					return
				}
			}
			t.Errorf("sessions = %v, want gc:acme left in place after Stop", fake.Sessions)
		})
	}
}
