package provider_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/docbobo/rex-gascity/internal/provider"
	"github.com/docbobo/rex-gascity/internal/rex"
	"github.com/docbobo/rex-gascity/internal/rex/faketest"
)

// TestStopRemovesEmptyCitySession pins GCREX-31: once its last pane goes, the
// canonical gc:<city> session is removed from `rex session ls`. A server-wide
// app client is listed on every session (docs/REX-CONTRACT.md §12), so it
// cannot keep an empty session alive. Both the pane and the no-pane Stop path
// remove it.
func TestStopRemovesEmptyCitySession(t *testing.T) {
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
			if !slices.Equal(fake.DestroyedSessions, []rex.SessionID{"session:[city] acme"}) {
				t.Errorf("destroyed = %v, want the empty city session", fake.DestroyedSessions)
			}
			if len(fake.Sessions) != 0 {
				t.Errorf("sessions = %v, want the empty city session gone", fake.Sessions)
			}
		})
	}
}

// TestStopReportsDestroyFailure pins the fail-closed half of GCREX-31: a failed
// destroy keeps the session and returns the error, so the retry sees it.
func TestStopReportsDestroyFailure(t *testing.T) {
	fake := faketest.New()
	p := provider.New("acme", fake)
	teardownStart(t, p, teardownDog)
	boom := errors.New("session.destroy: conflict")
	fake.DestroySessionErr = boom

	if err := p.Stop(context.Background(), teardownDog); !errors.Is(err, boom) {
		t.Fatalf("Stop = %v, want the destroy error", err)
	}
	if len(fake.Sessions) != 1 || fake.Sessions[0].ID != "session:[city] acme" {
		t.Errorf("sessions = %v, want the city session kept", fake.Sessions)
	}
}
