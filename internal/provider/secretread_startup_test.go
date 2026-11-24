package provider_test

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/docbobo/rex-gascity/internal/provider"
	"github.com/docbobo/rex-gascity/internal/rex"
	"github.com/docbobo/rex-gascity/internal/rex/faketest"
)

// secretReadWithheld is the startup marker a secret read leaves (#48 REQ-007,
// REQ-008). It is never "delivery failed".
const secretReadWithheld = "withheld: the pane is reading a secret"

// startupBlock is the pane startupProvider's Start launches for
// "Indigo--dog-1".
const startupBlock = rex.BlockID("block:dog-1")

// idleStatus reports an idle program, so the readiness wait ends at once and
// a delivered nudge stays unconfirmed (no turn start is ever observed).
func idleStatus(rex.BlockID) rex.ProgramStatus {
	return rex.ProgramStatus{Records: []rex.ProgramStatusRecord{{State: "idle"}}}
}

// clearKeyPresses counts ctrl+u presses: the resend's draft-line clear.
func clearKeyPresses(fake *faketest.Fake) int {
	n := 0
	for _, k := range fake.SentKeys {
		if slices.Equal(k.Keys, []string{"ctrl+u"}) {
			n++
		}
	}
	return n
}

// TestStartWithholdsStartupNudgeDuringSecretRead covers REQ-007 (E14): a
// secret read at the first attempt types nothing, records the withhold
// marker, and Start still succeeds.
func TestStartWithholdsStartupNudgeDuringSecretRead(t *testing.T) {
	const name = "Indigo--dog-1"
	ctx := context.Background()
	fake := faketest.New()
	fake.ProgramStatusFunc = idleStatus
	fake.InputModes = map[rex.BlockID]rex.InputMode{startupBlock: secretReadMode}
	p := startupProvider(fake)

	if err := p.Start(ctx, name, provider.Config{Argv: []string{"pi"}, Nudge: "hello agent"}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if wroteText(fake) || len(fake.SentKeys) != 0 {
		t.Errorf("ops = %v, keys = %+v; want nothing typed into a secret read", ops(fake), fake.SentKeys)
	}
	if got, _ := p.GetMeta(ctx, name, provider.StartupDeliveryUnconfirmedMetaKey); got != secretReadWithheld {
		t.Errorf("%s = %q, want %q", provider.StartupDeliveryUnconfirmedMetaKey, got, secretReadWithheld)
	}
}

// TestStartupResendSkipsClearKeyDuringSecretRead covers REQ-008 (E15): when a
// secret read appears before a resend, the loop presses no ctrl+u (VKILL in a
// cooked read would erase a password being typed), records the withhold and
// stops resending.
func TestStartupResendSkipsClearKeyDuringSecretRead(t *testing.T) {
	const name = "Indigo--dog-1"
	ctx := context.Background()
	fake := faketest.New()
	fake.ProgramStatusFunc = idleStatus
	writes := 0
	// The first attempt types into a normal pane; right after it, the program
	// starts a password prompt. WriteFunc runs under the fake's lock, so it
	// sets the field directly.
	fake.WriteFunc = func(rex.SessionID, rex.BlockID, string) {
		writes++
		fake.InputModes = map[rex.BlockID]rex.InputMode{startupBlock: secretReadMode}
	}
	p := startupProvider(fake, provider.WithStartupNudgeRetries([]time.Duration{time.Millisecond, time.Millisecond}))

	if err := p.Start(ctx, name, provider.Config{Argv: []string{"pi"}, Nudge: "hello agent"}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if n := clearKeyPresses(fake); n != 0 {
		t.Errorf("ctrl+u pressed %d time(s) into a secret read, want 0", n)
	}
	if writes != 1 {
		t.Errorf("writes = %d, want only the first attempt", writes)
	}
	if got := len(fake.InputModeReads); got != 2 {
		t.Errorf("input_mode reads = %d, want 2 (first attempt, one pre-clear read, then stop)", got)
	}
	if got, _ := p.GetMeta(ctx, name, provider.StartupDeliveryUnconfirmedMetaKey); got != secretReadWithheld {
		t.Errorf("%s = %q, want %q", provider.StartupDeliveryUnconfirmedMetaKey, got, secretReadWithheld)
	}
}

// scriptedInputModeFake answers input_mode read n with modes[n-1]; later reads
// repeat the last mode.
type scriptedInputModeFake struct {
	*faketest.Fake
	mu    sync.Mutex
	modes []rex.InputMode
	reads int
}

func (f *scriptedInputModeFake) BlockInputMode(context.Context, rex.SessionID, rex.BlockID) (rex.InputMode, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reads++
	return f.modes[min(f.reads, len(f.modes))-1], nil
}

// TestStartupResendWithholdsWhenNudgeProbeSeesSecretRead covers REQ-008 (E16):
// the pre-clear read passes, ctrl+u is pressed, and then the resend nudge's own
// read sees a secret read. Nothing more is typed, the marker is the withhold
// (not "delivery failed"), resending stops, and Start returns nil.
func TestStartupResendWithholdsWhenNudgeProbeSeesSecretRead(t *testing.T) {
	const name = "Indigo--dog-1"
	ctx := context.Background()
	fake := &scriptedInputModeFake{
		Fake: faketest.New(),
		// first attempt, pre-clear read, resend nudge read
		modes: []rex.InputMode{faketest.DefaultInputMode, faketest.DefaultInputMode, secretReadMode},
	}
	fake.ProgramStatusFunc = idleStatus
	writes := 0
	fake.WriteFunc = func(rex.SessionID, rex.BlockID, string) { writes++ }
	p := startupProvider(fake, provider.WithStartupNudgeRetries([]time.Duration{time.Millisecond, time.Millisecond}))

	if err := p.Start(ctx, name, provider.Config{Argv: []string{"pi"}, Nudge: "hello agent"}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if writes != 1 {
		t.Errorf("writes = %d, want only the first attempt", writes)
	}
	if n := clearKeyPresses(fake.Fake); n != 1 {
		t.Errorf("ctrl+u presses = %d, want 1 (the snapshot check passed before the clear)", n)
	}
	if fake.reads != 3 {
		t.Errorf("input_mode reads = %d, want 3 (resending stops after the withhold)", fake.reads)
	}
	if got, _ := p.GetMeta(ctx, name, provider.StartupDeliveryUnconfirmedMetaKey); got != secretReadWithheld {
		t.Errorf("%s = %q, want %q", provider.StartupDeliveryUnconfirmedMetaKey, got, secretReadWithheld)
	}
}
