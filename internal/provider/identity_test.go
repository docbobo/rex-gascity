package provider_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/docbobo/rex-gascity/internal/provider"
	"github.com/docbobo/rex-gascity/internal/rex"
	"github.com/docbobo/rex-gascity/internal/rex/faketest"
)

// gatedNewWindow blocks NewWindow until the test releases it, so the test can
// probe metadata during the same launch window that used to make gc roll the
// pending create back.
type gatedNewWindow struct {
	*faketest.Fake
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (g *gatedNewWindow) NewWindow(ctx context.Context, sid rex.SessionID, label string, spec rex.LaunchSpec) (rex.WindowID, rex.BlockID, error) {
	g.once.Do(func() { close(g.entered) })
	select {
	case <-g.release:
		return g.Fake.NewWindow(ctx, sid, label, spec)
	case <-ctx.Done():
		return "", "", ctx.Err()
	}
}

// failingNewWindow fails the launch after the provider has seeded identity and
// a provisional binding.
type failingNewWindow struct {
	*faketest.Fake
	err error
}

func (f *failingNewWindow) NewWindow(context.Context, rex.SessionID, string, rex.LaunchSpec) (rex.WindowID, rex.BlockID, error) {
	return "", "", f.err
}

func identityEnv() map[string]string {
	return map[string]string{
		"GC_RIG":            "my-project",
		"GC_TEMPLATE":       "my-project/pi",
		"GC_ALIAS":          "my-project/pi",
		"GC_AGENT":          "my-project/pi",
		"GC_SESSION_ID":     "mc-wisp-abc",
		"GC_INSTANCE_TOKEN": "token-abc",
		"GC_RUNTIME_EPOCH":  "7",
	}
}

func TestStartSeedsIdentityBeforeLaunch(t *testing.T) {
	const name = "pi-mc-wisp-abc"
	gate := &gatedNewWindow{
		Fake:    faketest.New(),
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
	p := provider.New("acme", gate)
	cfg := provider.Config{Argv: []string{"pi"}, Env: identityEnv()}

	done := make(chan error, 1)
	go func() { done <- p.Start(context.Background(), name, cfg) }()

	select {
	case <-gate.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not reach the blocking launch")
	}

	// A reconciler tick in this window reads the identity through get-meta.
	for _, probe := range []string{name, "my-project--pi"} {
		for key, want := range map[string]string{
			"GC_SESSION_ID":     "mc-wisp-abc",
			"GC_INSTANCE_TOKEN": "token-abc",
			"GC_RUNTIME_EPOCH":  "7",
		} {
			got, err := p.GetMeta(context.Background(), probe, key)
			if err != nil {
				t.Fatalf("GetMeta(%s, %s): %v", probe, key, err)
			}
			if got != want {
				t.Errorf("GetMeta(%s, %s) = %q, want %q", probe, key, got, want)
			}
		}
	}

	close(gate.release)
	if err := <-done; err != nil {
		t.Fatalf("Start: %v", err)
	}
	b, ok := p.Binding(name)
	if !ok {
		t.Fatal("no final binding after launch")
	}
	if b.BlockID == "" {
		t.Error("final binding has no block id")
	}
}

func TestStartClearsStaleIdentityWhenEnvHasEmptyValues(t *testing.T) {
	const name = "pi-mc-wisp-abc"
	ctx := context.Background()
	p := provider.New("acme", faketest.New())

	for key, old := range map[string]string{
		"GC_SESSION_ID":     "old-session",
		"GC_INSTANCE_TOKEN": "old-token",
		"GC_RUNTIME_EPOCH":  "1",
	} {
		if err := p.SetMeta(ctx, name, key, old); err != nil {
			t.Fatalf("SetMeta(%s): %v", key, err)
		}
	}

	cfg := provider.Config{Argv: []string{"pi"}, Env: map[string]string{
		"GC_SESSION_ID":     "new-session",
		"GC_INSTANCE_TOKEN": "",
		"GC_RUNTIME_EPOCH":  "",
	}}
	if err := p.Start(ctx, name, cfg); err != nil {
		t.Fatalf("Start: %v", err)
	}

	if got, _ := p.GetMeta(ctx, name, "GC_SESSION_ID"); got != "new-session" {
		t.Errorf("GC_SESSION_ID = %q, want new-session", got)
	}
	for _, key := range []string{"GC_INSTANCE_TOKEN", "GC_RUNTIME_EPOCH"} {
		if got, _ := p.GetMeta(ctx, name, key); got != "" {
			t.Errorf("%s = %q, want empty (stale prior-life value survived)", key, got)
		}
	}
}

func TestStopClearsSeededIdentity(t *testing.T) {
	const name = "pi-mc-wisp-abc"
	ctx := context.Background()
	p := provider.New("acme", faketest.New())
	if err := p.Start(ctx, name, provider.Config{Argv: []string{"pi"}, Env: identityEnv()}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	for _, key := range []string{"GC_SESSION_ID", "GC_INSTANCE_TOKEN", "GC_RUNTIME_EPOCH"} {
		if got, _ := p.GetMeta(ctx, name, key); got == "" {
			t.Fatalf("%s not seeded before Stop", key)
		}
	}

	if err := p.Stop(ctx, name); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	for _, key := range []string{"GC_SESSION_ID", "GC_INSTANCE_TOKEN", "GC_RUNTIME_EPOCH"} {
		if got, _ := p.GetMeta(ctx, name, key); got != "" {
			t.Errorf("%s = %q after Stop, want empty", key, got)
		}
	}
	if _, ok := p.Binding(name); ok {
		t.Error("binding still present after Stop")
	}
}

func TestStartFailureClearsSeededIdentityAndProvisionalBinding(t *testing.T) {
	const name = "pi-mc-wisp-abc"
	ctx := context.Background()
	boom := errors.New("launch failed")
	p := provider.New("acme", &failingNewWindow{Fake: faketest.New(), err: boom})

	if err := p.Start(ctx, name, provider.Config{Argv: []string{"pi"}, Env: identityEnv()}); !errors.Is(err, boom) {
		t.Fatalf("Start = %v, want %v", err, boom)
	}
	for _, key := range []string{"GC_SESSION_ID", "GC_INSTANCE_TOKEN", "GC_RUNTIME_EPOCH"} {
		if got, _ := p.GetMeta(ctx, name, key); got != "" {
			t.Errorf("%s = %q after failed start, want empty", key, got)
		}
	}
	if _, ok := p.Binding(name); ok {
		t.Error("provisional binding survived a failed start")
	}
}
