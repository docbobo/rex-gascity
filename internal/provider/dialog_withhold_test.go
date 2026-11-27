package provider_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/docbobo/rex-gascity/internal/provider"
	"github.com/docbobo/rex-gascity/internal/rex"
	"github.com/docbobo/rex-gascity/internal/rex/faketest"
)

// trustDialogScreen is a startup dialog that marks its selected option with the
// prompt glyph, exactly the shape that swallowed the turn's Enter in the live
// GCREX-59 report ("❯ 1. Yes, proceed", Enter picks option 1).
const trustDialogScreen = "Do you trust the files in this folder?\n❯ 1. Yes, proceed\n  2. No, exit\n"

// TestStartupNudgeWithheldOnStartupDialog pins GCREX-59 AC1/AC3: a pane whose
// screen shows a startup dialog at readiness time gets no write and no submit,
// and the withhold is recorded on the existing marker so gc's queued-nudge
// backstop can cover the first prompt.
func TestStartupNudgeWithheldOnStartupDialog(t *testing.T) {
	const name = "Indigo--dog-1"
	ctx := context.Background()
	fake := faketest.New()
	// The program reports idle even though a dialog is up: the readiness wait
	// would otherwise type the turn straight into the dialog.
	fake.ProgramStatuses = map[rex.BlockID]rex.ProgramStatus{
		"block:dog-1": {Records: []rex.ProgramStatusRecord{{State: "idle", UpdatedAt: time.Now()}}},
	}
	fake.Formats = map[rex.BlockID]string{"block:dog-1": trustDialogScreen}
	p := startupProvider(fake, provider.WithStartupDialogClear(50*time.Millisecond))

	if err := p.Start(ctx, name, provider.Config{Argv: []string{"pi"}, Nudge: "hello agent"}); err != nil {
		t.Fatalf("Start: %v", err)
	}

	for _, c := range fake.BlockCalls {
		if c.Op == "write" {
			t.Errorf("nudge written into a startup dialog: %+v", c)
		}
	}
	for _, c := range fake.SentKeys {
		if len(c.Keys) == 1 && c.Keys[0] == "enter" {
			t.Errorf("nudge submitted into a startup dialog: %+v", c)
		}
	}
	got, _ := p.GetMeta(ctx, name, provider.StartupDeliveryUnconfirmedMetaKey)
	if !strings.HasPrefix(got, "withheld:") {
		t.Errorf("%s = %q, want a withheld reason", provider.StartupDeliveryUnconfirmedMetaKey, got)
	}
}

// TestStartupNudgeWithheldOnStartupDialogWithPromptPrefix pins GCREX-59 AC2:
// even when a ready_prompt_prefix is configured, a dialog in the same screen
// withholds the first turn.
func TestStartupNudgeWithheldOnStartupDialogWithPromptPrefix(t *testing.T) {
	const name = "Indigo--dog-1"
	ctx := context.Background()
	fake := faketest.New()
	// A numbered menu's cursor row is not a ready prompt, and the dialog as a
	// whole withholds first, so the prefix cannot talk the provider into typing.
	fake.ProgramStatuses = map[rex.BlockID]rex.ProgramStatus{
		"block:dog-1": {Records: []rex.ProgramStatusRecord{{State: "working", UpdatedAt: time.Now()}}},
	}
	fake.Formats = map[rex.BlockID]string{"block:dog-1": trustDialogScreen}
	p := startupProvider(fake, provider.WithStartupDialogClear(50*time.Millisecond))

	if err := p.Start(ctx, name, provider.Config{
		Argv:              []string{"pi"},
		Nudge:             "hello agent",
		ReadyPromptPrefix: "❯ ",
	}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	for _, c := range fake.BlockCalls {
		if c.Op == "write" {
			t.Errorf("nudge written into a startup dialog with a prompt prefix: %+v", c)
		}
	}
	if got, _ := p.GetMeta(ctx, name, provider.StartupDeliveryUnconfirmedMetaKey); !strings.HasPrefix(got, "withheld:") {
		t.Errorf("%s = %q, want a withheld reason", provider.StartupDeliveryUnconfirmedMetaKey, got)
	}
}

// TestStartupNudgeDeliveredWhenDialogGone pins GCREX-59 AC3's second half: once
// the pane no longer shows a dialog, delivery proceeds as before, and no
// withhold marker is left behind.
func TestStartupNudgeDeliveredWhenDialogGone(t *testing.T) {
	const name = "Indigo--dog-1"
	ctx := context.Background()
	fake := faketest.New()
	fake.ProgramStatuses = map[rex.BlockID]rex.ProgramStatus{
		"block:dog-1": {Records: []rex.ProgramStatusRecord{{State: "idle", UpdatedAt: time.Now()}}},
	}
	// The dialog is gone: the composer is live and shows nothing to press.
	fake.Formats = map[rex.BlockID]string{"block:dog-1": "booting\n❯ "}
	p := startupProvider(fake)

	if err := p.Start(ctx, name, provider.Config{Argv: []string{"pi"}, Nudge: "hello agent"}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	wrote := false
	for _, c := range fake.BlockCalls {
		if c.Op == "write" && c.Text == "hello agent" {
			wrote = true
		}
	}
	if !wrote {
		t.Errorf("nudge not delivered with no dialog on screen: %+v", fake.BlockCalls)
	}
	if got, _ := p.GetMeta(ctx, name, provider.StartupDeliveryUnconfirmedMetaKey); strings.HasPrefix(got, "withheld:") {
		t.Errorf("%s = %q with no dialog, want no withhold", provider.StartupDeliveryUnconfirmedMetaKey, got)
	}
}
