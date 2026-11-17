package provider_test

import (
	"context"
	"testing"

	"github.com/docbobo/rex-gascity/internal/provider"
	"github.com/docbobo/rex-gascity/internal/rex"
	"github.com/docbobo/rex-gascity/internal/rex/faketest"
)

// wrappedGC is the basename gc runs under when the city binary is wrapped
// (gc 1.5.1 exports GC_BIN=/nix/store/.../bin/.gc-wrapped). The configured
// process_names hint for the control dispatcher is the bare "gc".
const wrappedGC = ".gc-wrapped"

// retireStarted starts name with GC_BIN set to a wrapped gc, mirroring how the
// control dispatcher is launched, and returns the started pane's block.
func retireStarted(t *testing.T, name string) (*faketest.Fake, *provider.Provider, rex.BlockID) {
	t.Helper()
	fake := faketest.New()
	p := provider.New("acme", fake)
	cfg := provider.Config{
		Argv: []string{"/nix/store/x/bin/gc", "convoy", "control", "--serve"},
		Env:  map[string]string{"GC_BIN": "/nix/store/x/bin/" + wrappedGC},
	}
	if err := p.Start(context.Background(), name, cfg); err != nil {
		t.Fatalf("Start %s: %v", name, err)
	}
	b, ok := p.Binding(name)
	if !ok {
		t.Fatalf("no binding recorded for %s", name)
	}
	return fake, p, b.BlockID
}

// wrappedForeground is the process tree Rex reports for a live control
// dispatcher: the foreground program's name, path and argv0 are all the
// wrapped binary, so only a GC_BIN expansion matches the "gc" hint.
func wrappedForeground(block rex.BlockID) map[rex.BlockID]rex.ProcessInfo {
	path := "/nix/store/x/bin/" + wrappedGC
	return map[rex.BlockID]rex.ProcessInfo{
		block: {Foreground: &rex.Process{PID: 42, Name: wrappedGC, Path: path, Argv0: path}},
	}
}

// TestProcessAliveRecognizesWrappedGC is the core regression: gc's
// process-alive observation for the control dispatcher must be true while the
// wrapped gc runs. A false here is read by gc's reconciler as a dead/zombie
// seat, which closes the session bead without calling provider stop and leaves
// the pane live.
func TestProcessAliveRecognizesWrappedGC(t *testing.T) {
	const name = "core__control-dispatcher-mc-wisp-1cvx"
	fake, p, block := retireStarted(t, name)
	fake.Processes = wrappedForeground(block)

	alive, err := p.ProcessAlive(context.Background(), name, []string{"gc"})
	if err != nil {
		t.Fatalf("ProcessAlive: %v", err)
	}
	if !alive {
		t.Fatal("ProcessAlive(gc) = false with a live wrapped gc; gc would reap the pane as a zombie")
	}
}

func TestProcessAliveWrappedGCStillDiesOnExit(t *testing.T) {
	const name = "core__control-dispatcher-mc-wisp-1cvx"
	fake, p, block := retireStarted(t, name)
	fake.Processes = map[rex.BlockID]rex.ProcessInfo{
		block: {LastExit: &rex.ExitStatus{ExitCode: 1}},
	}

	alive, err := p.ProcessAlive(context.Background(), name, []string{"gc"})
	if err != nil {
		t.Fatalf("ProcessAlive: %v", err)
	}
	if alive {
		t.Error("ProcessAlive(gc) = true for an exited program; want false")
	}
}

func TestProcessAliveStillMatchesLiteralGC(t *testing.T) {
	const name = "core__control-dispatcher-mc-wisp-1cvx"
	fake, p, block := retireStarted(t, name)
	fake.Processes = map[rex.BlockID]rex.ProcessInfo{
		block: {Foreground: &rex.Process{Name: "gc", Path: "/usr/bin/gc"}},
	}

	alive, err := p.ProcessAlive(context.Background(), name, []string{"gc"})
	if err != nil || !alive {
		t.Fatalf("ProcessAlive(literal gc) = %v, %v, want true, nil", alive, err)
	}
}

func TestProcessAliveKimiAlias(t *testing.T) {
	const name = "pi-mc-wisp-1"
	fake, p, block := retireStarted(t, name)
	fake.Processes = map[rex.BlockID]rex.ProcessInfo{
		block: {Foreground: &rex.Process{Name: "kimi-code", Path: "/usr/bin/kimi-code"}},
	}

	alive, err := p.ProcessAlive(context.Background(), name, []string{"kimi"})
	if err != nil || !alive {
		t.Fatalf("ProcessAlive(kimi->kimi-code) = %v, %v, want true, nil", alive, err)
	}
}

// TestRetireDrainClosesWrappedGCDispatcher covers the chosen retire path end to
// end: while the dispatcher is alive, gc takes its stop-pending branch and
// calls provider stop; stop closes the pane and is-running then reports false.
func TestRetireDrainClosesWrappedGCDispatcher(t *testing.T) {
	const name = "core__control-dispatcher-mc-wisp-1cvx"
	fake, p, block := retireStarted(t, name)
	fake.Processes = wrappedForeground(block)
	ctx := context.Background()

	// gc observes a live agent process, so the reconciler does not treat the
	// drain-ack as a zombie and does reach provider stop.
	alive, err := p.ProcessAlive(ctx, name, []string{"gc"})
	if err != nil || !alive {
		t.Fatalf("ProcessAlive = %v, %v, want true, nil", alive, err)
	}
	running, err := p.IsRunning(ctx, name)
	if err != nil || !running {
		t.Fatalf("IsRunning before stop = %v, %v, want true, nil", running, err)
	}

	if err := p.Stop(ctx, name); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if len(fake.ClosedBlocks) != 1 {
		t.Fatalf("ClosedBlocks = %v, want one close", fake.ClosedBlocks)
	}
	running, err = p.IsRunning(ctx, name)
	if err != nil {
		t.Fatalf("IsRunning after stop: %v", err)
	}
	if running {
		t.Error("IsRunning after stop = true; the retired pane is still live")
	}
}

// TestProcessAliveLegacyBindingFallsBackToEnv covers records written before
// GCBin was persisted: the provider falls back to its own GC_BIN so an
// already-running wrapped gc is still recognised.
func TestProcessAliveLegacyBindingFallsBackToEnv(t *testing.T) {
	const name = "core__control-dispatcher-mc-wisp-1cvx"
	t.Setenv("GC_BIN", "/nix/store/x/bin/"+wrappedGC)
	fake := faketest.New()
	p := provider.New("acme", fake)
	// No env in the start config, so the binding carries no GCBin.
	if err := p.Start(context.Background(), name, provider.Config{Argv: []string{"/bin/gc"}}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	b, ok := p.Binding(name)
	if !ok {
		t.Fatal("no binding recorded")
	}
	fake.Processes = wrappedForeground(b.BlockID)

	alive, err := p.ProcessAlive(context.Background(), name, []string{"gc"})
	if err != nil || !alive {
		t.Fatalf("ProcessAlive = %v, %v, want true, nil", alive, err)
	}
}
