package provider_test

import (
	"context"
	"fmt"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/docbobo/rex-gascity/internal/provider"
)

// TestE2ETownTabFirst is #47 AC-2: a rig session starts first, then a town
// session, and session.view shows the town tab first. It runs against the
// fake rex binary by default; REX_E2E=1 selects the live server, which
// mutates it:
//
//	go test ./internal/provider -run TestE2ETownTabFirst -v
func TestE2ETownTabFirst(t *testing.T) {
	// No city pin: the town tab is "Gas City" whatever the caller's env holds.
	for _, k := range []string{"GC_EXEC_CITY_NAME", "GC_EXEC_CITY_PATH", "GC_REX_CITY"} {
		t.Setenv(k, "")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	city := fmt.Sprintf("e2e-town-%d", time.Now().UnixNano())
	client, _ := e2eServer(t)
	p := provider.New(city, client, provider.WithNamespace(provider.NewNamespaceForDirs(t.TempDir())))
	const rig, town = "Indigo--dog-1", "mayor"
	defer func() {
		_ = p.Stop(ctx, town)
		_ = p.Stop(ctx, rig)
		e2eKillCity(ctx, client, city)
	}()

	cfg := provider.Config{Argv: []string{"/bin/sh"}, WorkDir: os.TempDir()}
	if err := p.Start(ctx, rig, cfg); err != nil {
		t.Fatalf("Start %s: %v", rig, err)
	}
	if err := p.Start(ctx, town, cfg); err != nil {
		t.Fatalf("Start %s: %v", town, err)
	}

	bd, ok := mustBinding(t)(p.Binding(town))
	if !ok {
		t.Fatalf("Binding %s: not found", town)
	}
	view, err := client.SessionView(ctx, bd.SessionID)
	if err != nil {
		t.Fatalf("SessionView: %v", err)
	}
	labels := make([]string, 0, len(view.Windows))
	for _, w := range view.Windows {
		labels = append(labels, w.Label)
	}
	if want := []string{"[city] Gas City", "[rig] Indigo"}; !slices.Equal(labels, want) {
		t.Errorf("window order = %q, want %q", labels, want)
	}
}
