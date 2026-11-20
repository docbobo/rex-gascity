package provider_test

import (
	"context"
	"testing"

	"github.com/docbobo/rex-gascity/internal/provider"
	"github.com/docbobo/rex-gascity/internal/rex"
	"github.com/docbobo/rex-gascity/internal/rex/faketest"
)

// spyFake records any destroy Stop attempts, so the SPEC-3 regression can
// assert zero DestroySession calls rather than only that the fake's session
// list is unchanged.
type spyFake struct {
	*faketest.Fake
	destroyed []rex.SessionID
}

func (s *spyFake) DestroySession(ctx context.Context, sid rex.SessionID) error {
	s.destroyed = append(s.destroyed, sid)
	return nil
}

// TestStopCallsNoDestroy is the SPEC-3 regression: the live probe
// (docs/REX-CONTRACT.md §12) lists a server-wide Rex app client on every
// session, so the approved skip branch applies and neither Stop path may call
// session.destroy. Red on both paths when the calls are restored.
func TestStopCallsNoDestroy(t *testing.T) {
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
			spy := &spyFake{Fake: fake}
			p := provider.New("acme", spy)
			teardownStart(t, p, teardownDog)
			fake.Clients = []rex.ClientInfo{{ID: "client:1", Kind: tc.kind}}
			if tc.noPane {
				fake.View.Windows = nil
			}
			if err := p.Stop(context.Background(), teardownDog); err != nil {
				t.Fatalf("Stop: %v", err)
			}
			if len(spy.destroyed) != 0 {
				t.Errorf("destroyed = %v, want zero DestroySession calls", spy.destroyed)
			}
		})
	}
}
