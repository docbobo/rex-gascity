package provider_test

import (
	"context"
	"slices"
	"testing"

	"github.com/docbobo/rex-gas-city/internal/provider"
	"github.com/docbobo/rex-gas-city/internal/rex"
	"github.com/docbobo/rex-gas-city/internal/rex/faketest"
)

func TestStartCreatesCitySessionRigTabAndAgentPane(t *testing.T) {
	fake := faketest.New()
	p := provider.New("acme", fake)

	err := p.Start(context.Background(), "Indigo--dog-1", provider.Config{
		Argv:    []string{"claude", "--flag"},
		WorkDir: "/work/indigo",
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	if got, want := fake.CreatedSessions, []string{"gc:acme"}; !slices.Equal(got, want) {
		t.Errorf("created sessions = %v, want %v", got, want)
	}
	if len(fake.CreatedWindows) != 1 {
		t.Fatalf("created windows = %d, want 1", len(fake.CreatedWindows))
	}
	w := fake.CreatedWindows[0]
	if w.Label != "Indigo" {
		t.Errorf("tab label = %q, want %q", w.Label, "Indigo")
	}
	if w.Spec.Label != "dog-1" {
		t.Errorf("pane label = %q, want %q", w.Spec.Label, "dog-1")
	}
	if !slices.Equal(w.Spec.Command, []string{"claude", "--flag"}) {
		t.Errorf("pane command = %v, want [claude --flag]", w.Spec.Command)
	}
	if w.Spec.Cwd != "/work/indigo" {
		t.Errorf("pane cwd = %q, want %q", w.Spec.Cwd, "/work/indigo")
	}
}

func TestStartJoinsExistingRigTab(t *testing.T) {
	fake := faketest.New()
	fake.Sessions = []rex.Session{{ID: "session:gc:acme", Label: "gc:acme"}}
	fake.View = rex.SessionView{
		Windows: []rex.Window{{
			ID:     "window:Indigo",
			Label:  "Indigo",
			Blocks: []rex.Block{{ID: "block:dog-1", Label: "dog-1"}},
		}},
	}
	p := provider.New("acme", fake)

	err := p.Start(context.Background(), "Indigo--dog-2", provider.Config{
		Argv:    []string{"claude"},
		WorkDir: "/work/indigo",
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	if len(fake.CreatedSessions) != 0 {
		t.Errorf("created sessions = %v, want none (city session exists)", fake.CreatedSessions)
	}
	if len(fake.CreatedWindows) != 0 {
		t.Errorf("created windows = %d, want 0 (rig tab exists)", len(fake.CreatedWindows))
	}
	if len(fake.CreatedSplits) != 1 {
		t.Fatalf("created splits = %d, want 1", len(fake.CreatedSplits))
	}
	s := fake.CreatedSplits[0]
	if s.Anchor != "block:dog-1" {
		t.Errorf("split anchor = %q, want %q", s.Anchor, "block:dog-1")
	}
	if s.Spec.Label != "dog-2" {
		t.Errorf("pane label = %q, want %q", s.Spec.Label, "dog-2")
	}
}

func TestStopClosesOnlyTheAgentPane(t *testing.T) {
	fake := faketest.New()
	fake.Sessions = []rex.Session{{ID: "session:gc:acme", Label: "gc:acme"}}
	fake.View = rex.SessionView{
		Windows: []rex.Window{
			{ID: "window:Indigo", Label: "Indigo", Blocks: []rex.Block{{ID: "block:dog-1", Label: "dog-1"}}},
			{ID: "window:Saffron", Label: "Saffron", Blocks: []rex.Block{{ID: "block:dog-3", Label: "dog-3"}}},
		},
	}
	p := provider.New("acme", fake)

	if err := p.Stop(context.Background(), "Indigo--dog-1"); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	if got, want := fake.ClosedBlocks, []rex.BlockID{"block:dog-1"}; !slices.Equal(got, want) {
		t.Errorf("closed blocks = %v, want %v", got, want)
	}
}

func TestStopIsIdempotentForUnknownSession(t *testing.T) {
	fake := faketest.New()
	fake.Sessions = []rex.Session{{ID: "session:gc:acme", Label: "gc:acme"}}
	fake.View = rex.SessionView{
		Windows: []rex.Window{
			{ID: "window:Indigo", Label: "Indigo", Blocks: []rex.Block{{ID: "block:dog-1", Label: "dog-1"}}},
		},
	}
	p := provider.New("acme", fake)

	if err := p.Stop(context.Background(), "Indigo--ghost"); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if len(fake.ClosedBlocks) != 0 {
		t.Errorf("closed blocks = %v, want none", fake.ClosedBlocks)
	}
}

func TestIsRunningTrueWhileProgramRuns(t *testing.T) {
	fake := faketest.New()
	fake.Sessions = []rex.Session{{ID: "session:gc:acme", Label: "gc:acme"}}
	fake.View = rex.SessionView{
		Windows: []rex.Window{
			{ID: "window:Indigo", Label: "Indigo", Blocks: []rex.Block{{ID: "block:dog-1", Label: "dog-1"}}},
		},
	}
	fake.Processes = map[rex.BlockID]rex.ProcessInfo{
		"block:dog-1": {Foreground: &rex.Process{PID: 42, Name: "claude"}},
	}
	p := provider.New("acme", fake)

	running, err := p.IsRunning(context.Background(), "Indigo--dog-1")
	if err != nil {
		t.Fatalf("IsRunning: %v", err)
	}
	if !running {
		t.Error("IsRunning = false, want true while the program runs")
	}
}

func TestIsRunningFalseWhenProgramExited(t *testing.T) {
	fake := faketest.New()
	fake.Sessions = []rex.Session{{ID: "session:gc:acme", Label: "gc:acme"}}
	fake.View = rex.SessionView{
		Windows: []rex.Window{
			{ID: "window:Indigo", Label: "Indigo", Blocks: []rex.Block{{ID: "block:dog-1", Label: "dog-1"}}},
		},
	}
	fake.Processes = map[rex.BlockID]rex.ProcessInfo{
		"block:dog-1": {LastExit: &rex.ExitStatus{ExitCode: 0}},
	}
	p := provider.New("acme", fake)

	running, err := p.IsRunning(context.Background(), "Indigo--dog-1")
	if err != nil {
		t.Fatalf("IsRunning: %v", err)
	}
	if running {
		t.Error("IsRunning = true, want false after the program exited")
	}
}

func TestIsRunningFalseWhenNoPane(t *testing.T) {
	fake := faketest.New()
	p := provider.New("acme", fake)

	running, err := p.IsRunning(context.Background(), "Indigo--ghost")
	if err != nil {
		t.Fatalf("IsRunning: %v", err)
	}
	if running {
		t.Error("IsRunning = true, want false when the agent has no pane")
	}
}

func TestPeekReturnsAgentOutput(t *testing.T) {
	fake := faketest.New()
	fake.Sessions = []rex.Session{{ID: "session:gc:acme", Label: "gc:acme"}}
	fake.View = rex.SessionView{
		Windows: []rex.Window{
			{ID: "window:Indigo", Label: "Indigo", Blocks: []rex.Block{{ID: "block:dog-1", Label: "dog-1"}}},
		},
	}
	fake.Formats = map[rex.BlockID]string{"block:dog-1": "hello from dog-1\n"}
	p := provider.New("acme", fake)

	out, err := p.Peek(context.Background(), "Indigo--dog-1")
	if err != nil {
		t.Fatalf("Peek: %v", err)
	}
	if out != "hello from dog-1\n" {
		t.Errorf("Peek = %q, want %q", out, "hello from dog-1\n")
	}
	// Peek must scope the block call to the city session Rex requires.
	if len(fake.BlockCalls) != 1 {
		t.Fatalf("block calls = %v, want one", fake.BlockCalls)
	}
	if call := fake.BlockCalls[0]; call.SessionID != "session:gc:acme" || call.BlockID != "block:dog-1" || call.Op != "format" {
		t.Errorf("block call = %+v, want session:gc:acme/block:dog-1/format", call)
	}
}

func TestPeekEmptyWhenNoPane(t *testing.T) {
	fake := faketest.New()
	p := provider.New("acme", fake)

	out, err := p.Peek(context.Background(), "Indigo--ghost")
	if err != nil {
		t.Fatalf("Peek: %v", err)
	}
	if out != "" {
		t.Errorf("Peek = %q, want empty for a missing pane", out)
	}
}

func TestStopUnknownSessionIsANoOp(t *testing.T) {
	fake := faketest.New()
	p := provider.New("acme", fake)

	if err := p.Stop(context.Background(), "Indigo--ghost"); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if len(fake.ClosedBlocks) != 0 {
		t.Errorf("closed blocks = %v, want none", fake.ClosedBlocks)
	}
}

func TestStopUsesRecordedBindingWithoutScan(t *testing.T) {
	ctx := context.Background()
	fake := faketest.New()
	p := provider.New("acme", fake)
	if err := p.Start(ctx, "Indigo--dog-1", provider.Config{Argv: []string{"x"}}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	fake.ListSessionsCalls, fake.SessionViewCalls = 0, 0

	if err := p.Stop(ctx, "Indigo--dog-1"); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if fake.ListSessionsCalls != 0 || fake.SessionViewCalls != 1 {
		t.Errorf("calls: list=%d view=%d", fake.ListSessionsCalls, fake.SessionViewCalls)
	}
	if !slices.Equal(fake.ClosedBlocks, []rex.BlockID{"block:dog-1"}) {
		t.Errorf("closed = %v", fake.ClosedBlocks)
	}
}
