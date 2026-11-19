package provider_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
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
	// Simulate a crashed prior incarnation: its session record and identity
	// sidecar survive together, so setting the stale identity is allowed.
	bs := provider.NewMemoryStore()
	bs.Save(provider.Binding{Name: provider.SessionName(name), SessionID: "session:stale", BlockID: "block:stale"})
	p := provider.New("acme", faketest.New(), provider.WithBindingStore(bs))

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

// startupProvider builds a provider for the startup-nudge tests: a short
// readiness wait, a short confirmation window and no resend ladder, so a test
// only pays for the behavior it exercises. Tests that need resends pass
// provider.WithStartupNudgeRetries themselves.
func startupProvider(fake rex.Client, opts ...provider.Option) *provider.Provider {
	base := []provider.Option{
		provider.WithStartupReadiness(2*time.Second, 20*time.Millisecond),
		provider.WithNudgeConfirmation(20*time.Millisecond, 2*time.Millisecond),
		provider.WithStartupNudgeRetries(nil),
	}
	return provider.New("acme", fake, append(base, opts...)...)
}

func TestStartupNudgeDeliveredAfterLaunch(t *testing.T) {
	const name = "Indigo--dog-1"
	ctx := context.Background()
	fake := faketest.New()
	p := startupProvider(fake)

	if err := p.Start(ctx, name, provider.Config{
		Argv:  []string{"claude"},
		Nudge: "hello agent",
	}); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// The nudge writes the text then submits Enter, using the block from the
	// launch result.
	var calls []string
	for _, c := range fake.BlockCalls {
		calls = append(calls, c.Op)
	}
	hasWrite := false
	hasSubmit := false
	for _, c := range fake.BlockCalls {
		if c.Op == "write" && c.BlockID == "block:dog-1" && c.SessionID == "session:gc:acme" {
			hasWrite = true
		}
		if c.Op == "send-key" && c.BlockID == "block:dog-1" && c.SessionID == "session:gc:acme" {
			hasSubmit = true
		}
	}
	if !hasWrite {
		t.Errorf("BlockCalls missing write for startup nudge: %v", calls)
	}
	for _, c := range fake.BlockCalls {
		if c.Op == "write" && c.Text != "hello agent" {
			t.Errorf("nudge write text = %q, want %q", c.Text, "hello agent")
		}
	}
	if !hasSubmit {
		t.Errorf("BlockCalls missing send-key for startup nudge: %v", calls)
	}
	if len(fake.SentKeys) == 0 {
		t.Fatal("no send-keys call recorded")
	}
	last := fake.SentKeys[len(fake.SentKeys)-1]
	if len(last.Keys) != 1 || last.Keys[0] != "enter" {
		t.Errorf("submit keys = %v, want [enter]", last.Keys)
	}
}

// TestStartupNudgeWaitsForIdleBeforeDelivering covers a pane whose program
// reports program_status only after the first poll: Start must not type the
// nudge until the program reports idle.
//
// The assertion is on the observed status sequence, not just that some
// program_status call preceded the write: NudgeReport's own baseline poll is
// a program_status call, so "a poll happened" passes even without the wait.
func TestStartupNudgeWaitsForIdleBeforeDelivering(t *testing.T) {
	const name = "Indigo--dog-1"
	ctx := context.Background()
	fake := faketest.New()
	var states []string
	fake.ProgramStatusFunc = func(rex.BlockID) rex.ProgramStatus {
		state := ""
		if len(states) >= 1 {
			state = "idle"
		}
		states = append(states, state)
		if state == "" {
			// Still booting: no program_status record yet.
			return rex.ProgramStatus{}
		}
		return rex.ProgramStatus{Records: []rex.ProgramStatusRecord{{
			State:     state,
			UpdatedAt: time.Now(),
		}}}
	}
	p := startupProvider(fake)

	if err := p.Start(ctx, name, provider.Config{
		Argv:  []string{"pi"},
		Nudge: "hello agent",
	}); err != nil {
		t.Fatalf("Start: %v", err)
	}

	writeAt := -1
	for i, c := range fake.BlockCalls {
		if c.Op == "write" && c.Text == "hello agent" {
			writeAt = i
			break
		}
	}
	if writeAt < 0 {
		t.Fatalf("startup nudge not written after idle: %+v", fake.BlockCalls)
	}
	preWritePolls := 0
	for _, c := range fake.BlockCalls[:writeAt] {
		if c.Op == "program_status" {
			preWritePolls++
		}
	}
	if preWritePolls == 0 {
		t.Fatalf("nudge written before readiness was checked: %+v", fake.BlockCalls)
	}
	sawIdle := false
	for _, s := range states[:preWritePolls] {
		if s == "idle" {
			sawIdle = true
		}
	}
	if !sawIdle {
		t.Fatalf("nudge written before the program reported idle: pre-write states=%v calls=%+v", states[:preWritePolls], fake.BlockCalls)
	}
	if len(fake.SentKeys) == 0 || fake.SentKeys[len(fake.SentKeys)-1].Keys[0] != "enter" {
		t.Errorf("idle nudge not submitted: %+v", fake.SentKeys)
	}
}

// TestStartupNudgeDeliveredAfterNonIdleWindow covers a pane program that
// reports a record but never `idle`: only the shorter settle path is for no
// record at all, so the nudge waits the full readiness window and is still
// delivered best-effort when it elapses.
func TestStartupNudgeDeliveredAfterNonIdleWindow(t *testing.T) {
	const name = "Indigo--dog-1"
	ctx := context.Background()
	fake := faketest.New()
	fake.ProgramStatusFunc = func(rex.BlockID) rex.ProgramStatus {
		return rex.ProgramStatus{Records: []rex.ProgramStatusRecord{{State: "working"}}}
	}
	p := startupProvider(fake, provider.WithStartupReadiness(60*time.Millisecond, 10*time.Millisecond))

	start := time.Now()
	if err := p.Start(ctx, name, provider.Config{
		Argv:  []string{"pi"},
		Nudge: "hello agent",
	}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if elapsed := time.Since(start); elapsed < 50*time.Millisecond {
		t.Errorf("Start returned in %v; want the full window for a present non-idle record", elapsed)
	}
	wrote := false
	for _, c := range fake.BlockCalls {
		if c.Op == "write" && c.Text == "hello agent" {
			wrote = true
		}
	}
	if !wrote {
		t.Errorf("nudge not delivered after the non-idle window: %+v", fake.BlockCalls)
	}
}

// TestStartupNudgeDeliveredAfterReadinessTimeout covers a pane program that
// never reports program_status: the settle delay bounds the wait, then the
// nudge is still delivered best-effort and Start succeeds.
func TestStartupNudgeDeliveredAfterReadinessTimeout(t *testing.T) {
	const name = "Indigo--dog-1"
	ctx := context.Background()
	fake := faketest.New()
	p := startupProvider(fake)

	if err := p.Start(ctx, name, provider.Config{
		Argv:  []string{"claude"},
		Nudge: "hello agent",
	}); err != nil {
		t.Fatalf("Start: %v", err)
	}

	sawStatusPoll := false
	for _, c := range fake.BlockCalls {
		if c.Op == "program_status" {
			sawStatusPoll = true
		}
	}
	if !sawStatusPoll {
		t.Error("Start did not probe program_status before delivering the nudge")
	}
	wrote := false
	for _, c := range fake.BlockCalls {
		if c.Op == "write" && c.Text == "hello agent" {
			wrote = true
		}
	}
	if !wrote {
		t.Errorf("nudge not delivered after the readiness timeout: %+v", fake.BlockCalls)
	}
	if len(fake.SentKeys) == 0 {
		t.Error("nudge not submitted after the readiness timeout")
	}
}

// TestStartupNudgeReadinessHonoursContext covers a program stuck in a
// non-idle state: the readiness wait must return when the start context is
// canceled instead of blocking for the whole window.
func TestStartupNudgeReadinessHonoursContext(t *testing.T) {
	const name = "Indigo--dog-1"
	fake := faketest.New()
	fake.ProgramStatuses = map[rex.BlockID]rex.ProgramStatus{
		"block:dog-1": {Records: []rex.ProgramStatusRecord{{
			State:     "working",
			UpdatedAt: time.Now(),
		}}},
	}
	p := startupProvider(fake, provider.WithStartupReadiness(10*time.Second, time.Second))
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	start := time.Now()
	if err := p.Start(ctx, name, provider.Config{
		Argv:  []string{"pi"},
		Nudge: "hello agent",
	}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("Start took %v with a 10s readiness window and a canceled context", elapsed)
	}
}

func TestStartupNudgeNotDeliveredOnLaunchFailure(t *testing.T) {
	const name = "Indigo--dog-1"
	ctx := context.Background()
	boom := errors.New("launch failed")
	fake := &failingNewWindow{Fake: faketest.New(), err: boom}
	p := provider.New("acme", fake)

	if err := p.Start(ctx, name, provider.Config{
		Argv:  []string{"claude"},
		Nudge: "hello agent",
	}); !errors.Is(err, boom) {
		t.Fatalf("Start = %v, want %v", err, boom)
	}

	for _, c := range fake.BlockCalls {
		if c.Op == "write" || c.Op == "send-key" {
			t.Errorf("nudge delivered on failed launch: op=%s", c.Op)
		}
	}
}

func TestStartupNudgeNotDeliveredOnAlreadyAlive(t *testing.T) {
	const name = "Indigo--dog-1"
	ctx := context.Background()
	fake := faketest.New()
	p := provider.New("acme", fake)

	// First start: no nudge, just set up a live pane.
	if err := p.Start(ctx, name, provider.Config{Argv: []string{"claude"}}); err != nil {
		t.Fatalf("first Start: %v", err)
	}
	callCount := len(fake.BlockCalls)

	// Second start: same name, now with a nudge. The pane is already alive so
	// start is idempotent — no new launch, no nudge (the liveness probe does
	// call process, but no write or send-key should appear).
	if err := p.Start(ctx, name, provider.Config{
		Argv:  []string{"claude"},
		Nudge: "hello again",
	}); err != nil {
		t.Fatalf("second Start: %v", err)
	}

	for _, c := range fake.BlockCalls[callCount:] {
		if c.Op == "write" || c.Op == "send-key" {
			t.Errorf("nudge delivered on idempotent start: op=%s", c.Op)
		}
	}
}

func TestStartupNudgeNotDeliveredOnErrAlreadyRunning(t *testing.T) {
	const name = "Indigo--dog-1"
	ctx := context.Background()
	fake := faketest.New()
	p := startupProvider(fake)

	// First StartNew succeeds.
	if err := p.StartNew(ctx, name, provider.Config{
		Argv:  []string{"claude"},
		Nudge: "first prompt",
	}); err != nil {
		t.Fatalf("first StartNew: %v", err)
	}
	callCount := len(fake.BlockCalls)

	// Second StartNew must fail ErrAlreadyRunning and not deliver any nudge.
	// The IsRunning check does call process for the liveness probe, but no
	// write or send-key should appear.
	err := p.StartNew(ctx, name, provider.Config{
		Argv:  []string{"claude"},
		Nudge: "second prompt",
	})
	if !errors.Is(err, provider.ErrAlreadyRunning) {
		t.Fatalf("StartNew = %v, want ErrAlreadyRunning", err)
	}

	for _, c := range fake.BlockCalls[callCount:] {
		if c.Op == "write" || c.Op == "send-key" {
			t.Errorf("nudge delivered on ErrAlreadyRunning: op=%s", c.Op)
		}
	}
}

func TestStartupNudgeEmptyIsNoOp(t *testing.T) {
	const name = "Indigo--dog-1"
	ctx := context.Background()
	fake := faketest.New()
	p := provider.New("acme", fake)

	if err := p.Start(ctx, name, provider.Config{
		Argv:  []string{"claude"},
		Nudge: "",
	}); err != nil {
		t.Fatalf("Start: %v", err)
	}

	for _, c := range fake.BlockCalls {
		if c.Op == "write" || c.Op == "send-key" {
			t.Errorf("empty nudge delivered: op=%s", c.Op)
		}
	}
}

// TestStartupNudgeDeliveryFailureKeepsPane covers the best-effort contract: a
// transport failure while delivering the startup nudge must not fail the start
// or clear the launched pane's binding and seeded identity.
func TestStartupNudgeDeliveryFailureKeepsPane(t *testing.T) {
	const name = "pi-mc-wisp-abc"
	ctx := context.Background()
	for _, tc := range []struct {
		label string
		set   func(*faketest.Fake)
	}{
		{"write", func(f *faketest.Fake) { f.WriteErr = errors.New("write failed") }},
		{"send-key", func(f *faketest.Fake) { f.SendKeysErr = errors.New("send failed") }},
	} {
		t.Run(tc.label, func(t *testing.T) {
			fake := faketest.New()
			tc.set(fake)
			p := startupProvider(fake)
			err := p.Start(ctx, name, provider.Config{
				Argv:  []string{"pi"},
				Env:   identityEnv(),
				Nudge: "hi",
			})
			if err != nil {
				t.Fatalf("Start = %v, want nil (startup nudge is best effort)", err)
			}
			if _, ok := p.Binding(name); !ok {
				t.Error("binding missing after best-effort nudge failure")
			}
			if len(fake.CreatedWindows) == 0 {
				t.Error("pane not launched")
			}
			if got, _ := p.GetMeta(ctx, name, "GC_SESSION_ID"); got != "mc-wisp-abc" {
				t.Errorf("seeded identity cleared after best-effort nudge failure: %q", got)
			}
		})
	}
}

// TestStartupNudgeUnconfirmedRecordsMarker covers a startup nudge whose text
// and Enter reach the pane but whose turn start is not observed. The delivery
// must still succeed best-effort and leave GC_STARTUP_DELIVERY_UNCONFIRMED on
// the pane so an operator can find the stranded prompt.
func TestStartupNudgeUnconfirmedRecordsMarker(t *testing.T) {
	const name = "Indigo--dog-1"
	ctx := context.Background()
	fake := faketest.New()
	p := startupProvider(fake)

	if err := p.Start(ctx, name, provider.Config{
		Argv:  []string{"pi"},
		Nudge: "hello agent",
	}); err != nil {
		t.Fatalf("Start: %v", err)
	}

	got, err := p.GetMeta(ctx, name, provider.StartupDeliveryUnconfirmedMetaKey)
	if err != nil {
		t.Fatalf("GetMeta: %v", err)
	}
	if got == "" {
		t.Error("unconfirmed startup delivery did not record GC_STARTUP_DELIVERY_UNCONFIRMED")
	}
	if !strings.Contains(got, "not confirmed") {
		t.Errorf("marker = %q, want an unconfirmed-delivery detail", got)
	}
}

// TestStartupNudgeConfirmedClearsMarker covers a startup nudge that the pane
// confirms by starting a turn. A marker left by an earlier unconfirmed delivery
// must be cleared, so a later read does not report a strand that is gone.
func TestStartupNudgeConfirmedClearsMarker(t *testing.T) {
	const name = "Indigo--dog-1"
	ctx := context.Background()
	fake := faketest.New()
	var calls int
	fake.ProgramStatusFunc = func(rex.BlockID) rex.ProgramStatus {
		calls++
		if calls >= 3 {
			return rex.ProgramStatus{Records: []rex.ProgramStatusRecord{{
				State:     "working",
				UpdatedAt: time.Now(),
			}}}
		}
		return rex.ProgramStatus{Records: []rex.ProgramStatusRecord{{
			State:     "idle",
			UpdatedAt: time.Now(),
		}}}
	}
	bs := provider.NewMemoryStore()
	bs.Save(provider.Binding{Name: provider.SessionName(name), SessionID: "session:stale", BlockID: "block:stale"})
	p := startupProvider(fake, provider.WithNudgeConfirmation(200*time.Millisecond, 2*time.Millisecond), provider.WithBindingStore(bs))
	if err := p.SetMeta(ctx, name, provider.StartupDeliveryUnconfirmedMetaKey, "old strand"); err != nil {
		t.Fatalf("SetMeta: %v", err)
	}

	if err := p.Start(ctx, name, provider.Config{
		Argv:  []string{"pi"},
		Nudge: "hello agent",
	}); err != nil {
		t.Fatalf("Start: %v", err)
	}

	got, err := p.GetMeta(ctx, name, provider.StartupDeliveryUnconfirmedMetaKey)
	if err != nil {
		t.Fatalf("GetMeta: %v", err)
	}
	if got != "" {
		t.Errorf("confirmed startup delivery left marker %q", got)
	}
}

// TestStartupNudgeResendsUnconfirmedSubmit covers the bug the task was opened
// for: readiness can be visible while the TUI still drops the Enter, so a
// delivered but unconfirmed startup nudge is cleared and re-sent down the
// bounded ladder. The second attempt is confirmed and the marker is cleared.
func TestStartupNudgeResendsUnconfirmedSubmit(t *testing.T) {
	const name = "Indigo--dog-1"
	ctx := context.Background()
	fake := faketest.New()
	var writes int
	fake.WriteFunc = func(rex.SessionID, rex.BlockID, string) { writes++ }
	fake.ProgramStatusFunc = func(rex.BlockID) rex.ProgramStatus {
		state := "idle"
		if writes >= 2 {
			state = "working"
		}
		return rex.ProgramStatus{Records: []rex.ProgramStatusRecord{{State: state}}}
	}
	p := startupProvider(fake, provider.WithStartupNudgeRetries([]time.Duration{5 * time.Millisecond}))

	if err := p.Start(ctx, name, provider.Config{
		Argv:  []string{"pi"},
		Nudge: "hello agent",
	}); err != nil {
		t.Fatalf("Start: %v", err)
	}

	if writes < 2 {
		t.Fatalf("startup nudge not resent: %d write(s), calls=%+v", writes, fake.BlockCalls)
	}
	sawClear := false
	for _, k := range fake.SentKeys {
		if len(k.Keys) == 1 && k.Keys[0] == "ctrl+u" {
			sawClear = true
		}
	}
	if !sawClear {
		t.Errorf("resend did not clear the drafted line: %+v", fake.SentKeys)
	}
	if got, err := p.GetMeta(ctx, name, provider.StartupDeliveryUnconfirmedMetaKey); err != nil || got != "" {
		t.Errorf("confirmed resend left marker %q (err %v)", got, err)
	}
}

// TestStartupTimingDiagnosticRecordsBoundReadiness covers the number the task
// exists to produce: a startup whose pane program never reports idle must
// record how long the readiness wait took and that it ended on its bound, not
// on idle, so a slow start can be attributed without a debugger.
func TestStartupTimingDiagnosticRecordsBoundReadiness(t *testing.T) {
	const name = "Indigo--dog-1"
	stateDir := t.TempDir()
	fake := faketest.New()
	fake.ProgramStatusFunc = func(rex.BlockID) rex.ProgramStatus {
		return rex.ProgramStatus{Records: []rex.ProgramStatusRecord{{
			State:     "working",
			UpdatedAt: time.Now(),
		}}}
	}
	p := startupProvider(fake,
		provider.WithNamespace(provider.NewNamespaceForDirs(stateDir)),
		provider.WithStartupReadiness(80*time.Millisecond, 10*time.Millisecond),
	)

	if err := p.Start(context.Background(), name, provider.Config{
		Argv:  []string{"pi"},
		Nudge: "hello agent",
	}); err != nil {
		t.Fatalf("Start: %v", err)
	}

	got := readStartupTiming(t, stateDir, name)
	if got["readiness_outcome"] != "window-bound" {
		t.Errorf("readiness_outcome = %v, want window-bound", got["readiness_outcome"])
	}
	if ms, _ := got["readiness_ms"].(float64); ms < 60 {
		t.Errorf("readiness_ms = %v, want >= 60 for a record present but never idle", got["readiness_ms"])
	}
	for _, key := range []string{"launch_ms", "readiness_ms", "write_ms", "confirm_ms", "total_ms"} {
		if _, ok := got[key]; !ok {
			t.Errorf("startup timing missing %q: %v", key, got)
		}
	}
}

// TestStartupTimingRecordsEachRetryAttempt covers AC1's "each retry attempt":
// when the submit is not confirmed and the nudge is resent, every attempt's
// backoff, write and confirmation duration must be recorded, not just the
// first.
func TestStartupTimingRecordsEachRetryAttempt(t *testing.T) {
	const name = "Indigo--dog-1"
	stateDir := t.TempDir()
	fake := faketest.New()
	p := startupProvider(fake,
		provider.WithNamespace(provider.NewNamespaceForDirs(stateDir)),
		provider.WithStartupNudgeRetries([]time.Duration{time.Millisecond, time.Millisecond}),
	)

	if err := p.Start(context.Background(), name, provider.Config{
		Argv:  []string{"pi"},
		Nudge: "hello agent",
	}); err != nil {
		t.Fatalf("Start: %v", err)
	}

	got := readStartupTiming(t, stateDir, name)
	attempts, ok := got["attempts"].([]any)
	if !ok {
		t.Fatalf("startup timing attempts missing: %v", got)
	}
	if len(attempts) != 2 {
		t.Fatalf("attempts = %d, want 2 (one per backoff): %v", len(attempts), got)
	}
	for i, raw := range attempts {
		a, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("attempt %d is not an object: %v", i, raw)
		}
		for _, key := range []string{"backoff_ms", "write_ms", "confirm_ms"} {
			if _, ok := a[key]; !ok {
				t.Errorf("attempt %d missing %q: %v", i, key, a)
			}
		}
	}
}

// readStartupTiming reads and decodes the startup-timing diagnostic. It fails
// the test when the file is absent, so a regression that stops writing the
// timing on a best-effort path is caught rather than silently skipped.
func readStartupTiming(t *testing.T, stateDir, name string) map[string]any {
	t.Helper()
	path := filepath.Join(stateDir, "diagnostics", name+"-startup-timing.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read startup timing diagnostic: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("decode startup timing diagnostic: %v (%s)", err, raw)
	}
	return got
}
