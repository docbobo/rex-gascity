package provider_test

import (
	"context"
	"strings"
	"testing"

	"github.com/docbobo/rex-gascity/internal/provider"
	"github.com/docbobo/rex-gascity/internal/rex"
	"github.com/docbobo/rex-gascity/internal/rex/faketest"
)

// TestStartupDialogMatchRecorded pins #205 AC5: a withheld seed's marker names
// the dialog by its stable name, and the timing diagnostic keeps the name and
// the screen the decision read, capped at 4096 bytes from the end.
func TestStartupDialogMatchRecorded(t *testing.T) {
	stateDir := t.TempDir()
	fake := faketest.New()
	fake.ProgramStatuses = idleProgramStatuses()
	filler := strings.Repeat(strings.Repeat("y", 99)+"\n", 150)
	fake.Formats = map[rex.BlockID]string{dogBlock: filler + trustDialogScreen}
	p := startupProvider(fake, provider.WithNamespace(provider.NewNamespaceForDirs(stateDir)))

	if err := p.Start(context.Background(), dogName, provider.Config{Argv: []string{"pi"}, Nudge: "hello agent"}); err != nil {
		t.Fatalf("Start: %v", err)
	}

	want := "withheld: the pane shows a startup dialog (" + trustName + ")"
	if got, _ := p.GetMeta(context.Background(), dogName, provider.StartupDeliveryUnconfirmedMetaKey); got != want {
		t.Errorf("marker = %q, want %q", got, want)
	}
	timing := readStartupTiming(t, stateDir, dogName)
	if timing["readiness_outcome"] != "dialog" {
		t.Errorf("readiness_outcome = %v, want dialog", timing["readiness_outcome"])
	}
	if timing["dialog"] != trustName {
		t.Errorf("dialog = %v, want %q", timing["dialog"], trustName)
	}
	screen, _ := timing["dialog_screen"].(string)
	if len(screen) != 4096 || !strings.HasSuffix(screen, trustDialogScreen) {
		t.Errorf("dialog_screen: %d bytes, suffix ok %v; want the last 4096 bytes ending in the dialog",
			len(screen), strings.HasSuffix(screen, trustDialogScreen))
	}
	if _, ok := timing["dialog_gate"]; ok {
		t.Errorf("dialog_gate = %v, want absent for a readiness sighting", timing["dialog_gate"])
	}
}
