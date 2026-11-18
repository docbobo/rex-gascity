package provider_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/docbobo/rex-gascity/internal/provider"
	"github.com/docbobo/rex-gascity/internal/rex/cli"
)

// TestE2ERigSplitGrid starts several agents in one rig on a live Rex server
// and checks the balanced split policy: every pane keeps a usable share of the
// tab. The old rule always halved the first pane, so the smallest pane would
// shrink exponentially. It is opt-in because it mutates the user's Rex server:
//
//	REX_E2E=1 go test ./internal/provider -run TestE2ERigSplitGrid -v
func TestE2ERigSplitGrid(t *testing.T) {
	if os.Getenv("REX_E2E") == "" {
		t.Skip("set REX_E2E=1 to run against the live Rex server")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	const agents = 8
	city := fmt.Sprintf("e2e-grid-%d", time.Now().UnixNano())
	client := cli.New()
	p := provider.New(city, client, provider.WithNamespace(provider.NewNamespaceForDirs(t.TempDir())))

	names := make([]string, agents)
	for i := range names {
		names[i] = fmt.Sprintf("Indigo--dog-%d", i+1)
	}
	defer func() {
		for _, name := range names {
			_ = p.Stop(ctx, name)
		}
		_ = exec.CommandContext(ctx, "rex", "session", "kill", "gc:"+city).Run()
	}()

	for _, name := range names {
		if err := p.Start(ctx, name, provider.Config{
			Argv:    []string{"/bin/sh"},
			WorkDir: os.TempDir(),
		}); err != nil {
			t.Fatalf("Start %s: %v", name, err)
		}
	}

	first, ok := p.Binding(names[0])
	if !ok {
		t.Fatalf("Binding %s: not found", names[0])
	}
	view, err := client.SessionView(ctx, first.SessionID)
	if err != nil {
		t.Fatalf("SessionView: %v", err)
	}
	if len(view.Windows) != 1 {
		t.Fatalf("windows = %d, want 1", len(view.Windows))
	}
	win := view.Windows[0]
	if win.Label != "Indigo" {
		t.Errorf("window label = %q, want Indigo", win.Label)
	}
	if len(win.Blocks) != agents {
		t.Fatalf("blocks = %d, want %d", len(win.Blocks), agents)
	}

	minArea := 1.0
	for _, b := range win.Blocks {
		w, h := b.Rect[2], b.Rect[3]
		if w <= 0 || h <= 0 {
			t.Errorf("block %s has rect %v, want non-zero", b.ID, b.Rect)
			continue
		}
		minArea = min(minArea, w*h)
	}
	if want := 0.5 / agents; minArea < want {
		t.Errorf("min pane area = %.5f, want >= %.5f (balanced splits)", minArea, want)
	}

	for _, name := range names {
		if _, ok := p.Binding(name); !ok {
			t.Errorf("Binding %s: not found", name)
		}
	}
}
