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

func TestStartCreatesCitySessionRigTabAndAgentPane(t *testing.T) {
	fake := faketest.New()
	p := provider.New("acme", fake)

	workDir := t.TempDir()
	err := p.Start(context.Background(), "Indigo--dog-1", provider.Config{
		Argv:    []string{"claude", "--flag"},
		WorkDir: workDir,
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
	if w.Label != "Rig: Indigo" {
		t.Errorf("tab label = %q, want %q", w.Label, "Rig: Indigo")
	}
	if w.Spec.Label != "dog-1" {
		t.Errorf("pane label = %q, want %q", w.Spec.Label, "dog-1")
	}
	if !slices.Equal(w.Spec.Command, []string{"claude", "--flag"}) {
		t.Errorf("pane command = %v, want [claude --flag]", w.Spec.Command)
	}
	if w.Spec.Cwd != workDir {
		t.Errorf("pane cwd = %q, want %q", w.Spec.Cwd, workDir)
	}
}

func TestStartAppliesEnvToLaunchCommand(t *testing.T) {
	fake := faketest.New()
	p := provider.New("acme", fake)

	argv := []string{"sleep", "300"}
	err := p.Start(context.Background(), "Indigo--dog-1", provider.Config{
		Argv: argv,
		// B keeps a space, '=' and a quote to prove the wrapper leaves each
		// mapping as one argv element.
		Env: map[string]string{"GC_CITY": "/tmp/c", "A": "1", "B": "x y=z'q"},
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if len(fake.CreatedWindows) != 1 {
		t.Fatalf("created windows = %d, want 1", len(fake.CreatedWindows))
	}
	want := []string{"env", "A=1", "B=x y=z'q", "GC_CITY=/tmp/c", "sleep", "300"}
	if got := fake.CreatedWindows[0].Spec.Command; !slices.Equal(got, want) {
		t.Errorf("pane command = %v, want %v", got, want)
	}
	if !slices.Equal(argv, []string{"sleep", "300"}) {
		t.Errorf("caller argv mutated: %v", argv)
	}
}

func TestStartWithoutEnvLeavesLaunchCommand(t *testing.T) {
	fake := faketest.New()
	p := provider.New("acme", fake)

	err := p.Start(context.Background(), "Indigo--dog-1", provider.Config{Argv: []string{"sleep", "300"}})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if got := fake.CreatedWindows[0].Spec.Command; !slices.Equal(got, []string{"sleep", "300"}) {
		t.Errorf("pane command = %v, want [sleep 300]", got)
	}
}

func TestStartJoinsExistingRigTab(t *testing.T) {
	fake := faketest.New()
	fake.Sessions = []rex.Session{{ID: "session:gc:acme", Label: "gc:acme"}}
	fake.View = rex.SessionView{
		Windows: []rex.Window{{
			ID:     "window:Indigo",
			Label:  "Rig: Indigo",
			Blocks: []rex.Block{{ID: "block:dog-1", Label: "dog-1"}},
		}},
	}
	p := provider.New("acme", fake)

	err := p.Start(context.Background(), "Indigo--dog-2", provider.Config{
		Argv:    []string{"claude"},
		WorkDir: t.TempDir(),
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
			{ID: "window:Indigo", Label: "Rig: Indigo", Blocks: []rex.Block{{ID: "block:dog-1", Label: "dog-1"}}},
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
			{ID: "window:Indigo", Label: "Rig: Indigo", Blocks: []rex.Block{{ID: "block:dog-1", Label: "dog-1"}}},
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
			{ID: "window:Indigo", Label: "Rig: Indigo", Blocks: []rex.Block{{ID: "block:dog-1", Label: "dog-1"}}},
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
			{ID: "window:Indigo", Label: "Rig: Indigo", Blocks: []rex.Block{{ID: "block:dog-1", Label: "dog-1"}}},
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
			{ID: "window:Indigo", Label: "Rig: Indigo", Blocks: []rex.Block{{ID: "block:dog-1", Label: "dog-1"}}},
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
	// Peek must scope the block calls to the city session Rex requires.
	var formats []faketest.BlockCall
	for _, call := range fake.BlockCalls {
		if call.Op != "format" {
			continue
		}
		formats = append(formats, call)
	}
	if len(formats) != 1 {
		t.Fatalf("block calls = %v, want one format", fake.BlockCalls)
	}
	if call := formats[0]; call.SessionID != "session:gc:acme" || call.BlockID != "block:dog-1" {
		t.Errorf("block call = %+v, want session:gc:acme/block:dog-1/format", call)
	}
}

func TestPeekNoScreenWhenNoPane(t *testing.T) {
	fake := faketest.New()
	p := provider.New("acme", fake)

	out, err := p.Peek(context.Background(), "Indigo--ghost")
	if !errors.Is(err, provider.ErrNoScreen) {
		t.Fatalf("Peek err = %v, want ErrNoScreen for a missing pane", err)
	}
	if out != "" {
		t.Errorf("Peek = %q, want empty for a missing pane", out)
	}
}

// TestPeekDeadBoxWithScreen proves the op is not gated on liveness: v2 runs
// peek whatever is-running says, and Rex keeps a box's rendered screen after
// the program exits. A dead box with a screen answers it with exit 0.
func TestPeekDeadBoxWithScreen(t *testing.T) {
	fake := faketest.New()
	p := provider.New("acme", fake)
	name := "Indigo--dog-1"
	if err := p.Start(context.Background(), name, provider.Config{Argv: []string{"sleep", "300"}}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	block := rex.BlockID("block:dog-1")
	fake.Processes = map[rex.BlockID]rex.ProcessInfo{block: {LastExit: &rex.ExitStatus{}}}
	fake.Formats = map[rex.BlockID]string{block: "fatal: rate limit\nretry later\n"}

	out, err := p.Peek(context.Background(), name)
	if err != nil {
		t.Fatalf("Peek: %v", err)
	}
	if out != "fatal: rate limit\nretry later\n" {
		t.Errorf("Peek = %q, want the dead box's screen", out)
	}
}

// TestPeekDeadBoxEmptyCaptureIsAFailure proves a dead box's capture that
// rendered nothing is a failure, never an empty success and never no screen:
// gc reads exit 0 with no lines as nothing to classify why the box died.
func TestPeekDeadBoxEmptyCaptureIsAFailure(t *testing.T) {
	fake := faketest.New()
	p := provider.New("acme", fake)
	name := "Indigo--dog-1"
	if err := p.Start(context.Background(), name, provider.Config{Argv: []string{"sleep", "300"}}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	block := rex.BlockID("block:dog-1")
	fake.Processes = map[rex.BlockID]rex.ProcessInfo{block: {LastExit: &rex.ExitStatus{}}}
	fake.Formats = map[rex.BlockID]string{block: ""}

	out, err := p.Peek(context.Background(), name)
	if err == nil {
		t.Fatalf("Peek = %q, want a failure for a dead box's empty capture", out)
	}
	if errors.Is(err, provider.ErrNoScreen) {
		t.Errorf("Peek err = %v, want an unreadable-capture failure, not ErrNoScreen", err)
	}
}

// TestPeekEmptyCaptureAfterExitIsAFailure proves the empty-capture check reads
// the process state again after the capture: a program that exits between
// BlockProcess and BlockFormat leaves a stale running state behind, and its
// blank capture must not pass as a running blank screen.
func TestPeekEmptyCaptureAfterExitIsAFailure(t *testing.T) {
	fake := faketest.New()
	p := provider.New("acme", fake)
	name := "Indigo--dog-1"
	if err := p.Start(context.Background(), name, provider.Config{Argv: []string{"sleep", "300"}}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	exited := false
	fake.FormatFunc = func(rex.BlockID) string {
		// The program exits while the capture renders.
		exited = true
		return ""
	}
	fake.ProcessFunc = func(rex.BlockID) rex.ProcessInfo {
		if exited {
			return rex.ProcessInfo{LastExit: &rex.ExitStatus{}}
		}
		return rex.ProcessInfo{}
	}

	out, err := p.Peek(context.Background(), name)
	if err == nil {
		t.Fatalf("Peek = %q, want a failure for a box that died during the capture", out)
	}
	if errors.Is(err, provider.ErrNoScreen) {
		t.Errorf("Peek err = %v, want an unreadable-capture failure, not ErrNoScreen", err)
	}
}

// TestPeekRunningBlankScreenSucceeds proves a running box that has rendered
// nothing yet is an empty screen, not a failure: RPP-OBSERVE-016 reads a
// running box's peek as a success.
func TestPeekRunningBlankScreenSucceeds(t *testing.T) {
	fake := faketest.New()
	p := provider.New("acme", fake)
	name := "Indigo--dog-1"
	if err := p.Start(context.Background(), name, provider.Config{Argv: []string{"sleep", "300"}}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	fake.Formats = map[rex.BlockID]string{"block:dog-1": ""}

	out, err := p.Peek(context.Background(), name)
	if err != nil {
		t.Fatalf("Peek: %v", err)
	}
	if out != "" {
		t.Errorf("Peek = %q, want an empty screen", out)
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
	// One view resolves the recorded pane. Stop no longer touches the session
	// after the close (GCREX-65 skip branch), so nothing scans for it.
	if fake.ListSessionsCalls != 0 || fake.SessionViewCalls != 1 {
		t.Errorf("calls: list=%d view=%d", fake.ListSessionsCalls, fake.SessionViewCalls)
	}
	if !slices.Equal(fake.ClosedBlocks, []rex.BlockID{"block:dog-1"}) {
		t.Errorf("closed = %v", fake.ClosedBlocks)
	}
}
