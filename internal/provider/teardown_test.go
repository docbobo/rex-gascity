package provider_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/docbobo/rex-gascity/internal/provider"
	"github.com/docbobo/rex-gascity/internal/rex"
	"github.com/docbobo/rex-gascity/internal/rex/faketest"
)

const (
	teardownDog = "Indigo--dog-1"
	teardownCat = "Indigo--cat-1"
)

var teardownCfg = provider.Config{Argv: []string{"/usr/local/bin/claude", "--x"}}

// teardownStarted returns a provider with name started as a "claude" agent.
func teardownStarted(t *testing.T, name string) (*faketest.Fake, *provider.Provider) {
	t.Helper()
	fake := faketest.New()
	p := provider.New("acme", fake)
	teardownStart(t, p, name)
	return fake, p
}

func teardownStart(t *testing.T, p *provider.Provider, name string) {
	t.Helper()
	if err := p.Start(context.Background(), name, teardownCfg); err != nil {
		t.Fatalf("Start %s: %v", name, err)
	}
}

// teardownExit marks block's program as exited.
func teardownExit(fake *faketest.Fake, block rex.BlockID) {
	if fake.Processes == nil {
		fake.Processes = map[rex.BlockID]rex.ProcessInfo{}
	}
	fake.Processes[block] = rex.ProcessInfo{LastExit: &rex.ExitStatus{}}
}

// teardownShellOnly leaves the terminal alive with only a shell in the
// foreground.
func teardownShellOnly(fake *faketest.Fake, block rex.BlockID) {
	if fake.Processes == nil {
		fake.Processes = map[rex.BlockID]rex.ProcessInfo{}
	}
	fake.Processes[block] = rex.ProcessInfo{Foreground: &rex.Process{Name: "zsh", Path: "/bin/zsh"}}
}

func TestStartTwiceCreatesNoDuplicatePane(t *testing.T) {
	fake, p := teardownStarted(t, teardownDog)
	first, _ := p.Binding(teardownDog)

	teardownStart(t, p, teardownDog)

	if len(fake.CreatedWindows) != 1 || len(fake.CreatedSplits) != 0 {
		t.Errorf("windows=%d splits=%d, want 1 and 0", len(fake.CreatedWindows), len(fake.CreatedSplits))
	}
	if len(fake.ClosedBlocks) != 0 {
		t.Errorf("closed = %v, want none", fake.ClosedBlocks)
	}
	got, ok := p.Binding(teardownDog)
	if !ok || got.BlockID != first.BlockID {
		t.Errorf("binding = %+v, want unchanged %+v", got, first)
	}
}

func TestStartAfterExitRelaunchesInNewTab(t *testing.T) {
	fake, p := teardownStarted(t, teardownDog)
	teardownExit(fake, "block:dog-1")

	teardownStart(t, p, teardownDog)

	if len(fake.ClosedBlocks) != 1 || fake.ClosedBlocks[0] != "block:dog-1" {
		t.Errorf("closed = %v, want [block:dog-1]", fake.ClosedBlocks)
	}
	// The dead pane was the last one, so its tab is gone and a new one opens.
	if len(fake.CreatedWindows) != 2 || len(fake.CreatedSplits) != 0 {
		t.Errorf("windows=%d splits=%d, want 2 and 0", len(fake.CreatedWindows), len(fake.CreatedSplits))
	}
	if len(fake.View.Windows) != 1 {
		t.Errorf("windows in view = %d, want 1", len(fake.View.Windows))
	}
	if b, ok := p.Binding(teardownDog); !ok || b.LaunchMode != "window" {
		t.Errorf("binding = %+v, %v, want a window launch", b, ok)
	}
}

func TestStartAfterExitRelaunchesAsSplitWhenRigTabRemains(t *testing.T) {
	fake, p := teardownStarted(t, teardownDog)
	teardownStart(t, p, teardownCat)
	if len(fake.CreatedSplits) != 1 {
		t.Fatalf("setup splits = %d, want 1", len(fake.CreatedSplits))
	}
	teardownExit(fake, "block:dog-1")

	teardownStart(t, p, teardownDog)

	if len(fake.ClosedBlocks) != 1 || fake.ClosedBlocks[0] != "block:dog-1" {
		t.Errorf("closed = %v, want [block:dog-1]", fake.ClosedBlocks)
	}
	if len(fake.CreatedWindows) != 1 || len(fake.CreatedSplits) != 2 {
		t.Errorf("windows=%d splits=%d, want 1 and 2", len(fake.CreatedWindows), len(fake.CreatedSplits))
	}
	if b, ok := p.Binding(teardownDog); !ok || b.LaunchMode != "split" {
		t.Errorf("binding = %+v, %v, want a split launch", b, ok)
	}
}

func TestStopToleratesVanishedPane(t *testing.T) {
	fake, p := teardownStarted(t, teardownDog)
	fake.CloseBlockErr = rex.ErrNotFound

	if err := p.Stop(context.Background(), teardownDog); err != nil {
		t.Fatalf("Stop = %v, want nil for a vanished pane", err)
	}
	if _, ok := p.Binding(teardownDog); ok {
		t.Error("binding still stored after Stop")
	}
}

func TestStopPropagatesOtherCloseErrors(t *testing.T) {
	fake, p := teardownStarted(t, teardownDog)
	boom := errors.New("transport down")
	fake.CloseBlockErr = boom

	if err := p.Stop(context.Background(), teardownDog); !errors.Is(err, boom) {
		t.Fatalf("Stop = %v, want %v", err, boom)
	}
	if _, ok := p.Binding(teardownDog); !ok {
		t.Error("binding forgotten although the close failed")
	}
}

func TestSecondStopIsNoOp(t *testing.T) {
	fake, p := teardownStarted(t, teardownDog)
	if err := p.Stop(context.Background(), teardownDog); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if err := p.Stop(context.Background(), teardownDog); err != nil {
		t.Fatalf("second Stop = %v, want nil", err)
	}
	if len(fake.ClosedBlocks) != 1 {
		t.Errorf("closed = %v, want one close", fake.ClosedBlocks)
	}
}

func TestIsRunningMissingPane(t *testing.T) {
	p := provider.New("acme", faketest.New())
	running, err := p.IsRunning(context.Background(), teardownDog)
	if err != nil || running {
		t.Errorf("IsRunning = %v, %v, want false, nil", running, err)
	}
}

func TestStartRecordsLaunchModeAndCreatedAt(t *testing.T) {
	_, p := teardownStarted(t, teardownDog)

	b, ok := p.Binding(teardownDog)
	if !ok {
		t.Fatal("no binding recorded")
	}
	if b.LaunchMode != "window" {
		t.Errorf("LaunchMode = %q, want window", b.LaunchMode)
	}
	if b.CreatedAt.IsZero() {
		t.Error("CreatedAt not set")
	}
}

// v1 rule: liveness is last_exit, not process names. A live terminal with only
// a shell in the foreground is running.
func TestIsRunningTrueWhileTerminalLive(t *testing.T) {
	fake, p := teardownStarted(t, teardownDog)
	teardownShellOnly(fake, "block:dog-1")

	running, err := p.IsRunning(context.Background(), teardownDog)
	if err != nil || !running {
		t.Errorf("IsRunning = %v, %v, want true, nil", running, err)
	}
}

func TestIsRunningFalseAfterExit(t *testing.T) {
	fake, p := teardownStarted(t, teardownDog)
	teardownExit(fake, "block:dog-1")

	running, err := p.IsRunning(context.Background(), teardownDog)
	if err != nil || running {
		t.Errorf("IsRunning = %v, %v, want false, nil", running, err)
	}
}

func TestProcessAliveMatchesByName(t *testing.T) {
	fake, p := teardownStarted(t, teardownDog)
	fake.Processes = map[rex.BlockID]rex.ProcessInfo{"block:dog-1": {
		Foreground: &rex.Process{Name: "git", Path: "/usr/bin/git"},
		Ancestors:  []rex.Process{{Name: "node", Argv0: "/usr/local/bin/claude"}},
	}}

	state, err := p.ProcessAlive(context.Background(), teardownDog, []string{"claude"})
	if err != nil || state != provider.LivenessAlive {
		t.Errorf("ProcessAlive(claude) = %v, %v, want alive, nil", state, err)
	}
	state, err = p.ProcessAlive(context.Background(), teardownDog, []string{"codex"})
	if err != nil || state != provider.LivenessDead {
		t.Errorf("ProcessAlive(codex) = %v, %v, want dead, nil", state, err)
	}
	state, err = p.ProcessAlive(context.Background(), teardownDog, nil)
	if err != nil || state != provider.LivenessAlive {
		t.Errorf("ProcessAlive(no names) = %v, %v, want alive, nil", state, err)
	}
}

func TestProcessAliveMissingPane(t *testing.T) {
	p := provider.New("acme", faketest.New())
	state, err := p.ProcessAlive(context.Background(), teardownDog, []string{"claude"})
	if err != nil || state != provider.LivenessDead {
		t.Errorf("ProcessAlive = %v, %v, want dead, nil", state, err)
	}
}

func TestProcessAliveVanishedPane(t *testing.T) {
	fake, p := teardownStarted(t, teardownDog)
	fake.ProcessErr = fmt.Errorf("rex block.method: %w", rex.ErrNotFound)
	state, err := p.ProcessAlive(context.Background(), teardownDog, []string{"claude"})
	if err != nil || state != provider.LivenessDead {
		t.Errorf("ProcessAlive after pane vanished = %v, %v; want dead, nil", state, err)
	}
}

// TestProcessAliveTransientProcessErrorIsUnknown: a BlockProcess failure other
// than a vanished pane must not report the agent dead. gc reads the empty
// cannot-tell answer as alive.
func TestProcessAliveTransientProcessErrorIsUnknown(t *testing.T) {
	fake, p := teardownStarted(t, teardownDog)
	fake.ProcessErr = errors.New("rex block.method: connection reset")
	state, err := p.ProcessAlive(context.Background(), teardownDog, []string{"claude"})
	if err != nil || state != provider.LivenessUnknown {
		t.Errorf("ProcessAlive after a transient error = %v, %v; want unknown, nil", state, err)
	}
}

func TestStopClosesOnlyTheStoppedPane(t *testing.T) {
	fake, p := teardownStarted(t, teardownDog)
	teardownStart(t, p, teardownCat)

	if err := p.Stop(context.Background(), teardownDog); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if len(fake.ClosedBlocks) != 1 {
		t.Errorf("closed = %v, want only the stopped pane", fake.ClosedBlocks)
	}
	if len(fake.Sessions) != 1 {
		t.Errorf("sessions = %v, want the session with the live pane left in place", fake.Sessions)
	}
	if len(fake.DestroyedSessions) != 0 {
		t.Errorf("destroyed = %v, want none while a live pane remains", fake.DestroyedSessions)
	}
}

func TestStopNeverDestroysLegacyLabelledSession(t *testing.T) {
	fake := faketest.New()
	fake.Sessions = []rex.Session{{ID: "session:gc:oldacme", Label: "gc:oldacme"}}
	p := provider.New("acme", fake, namespacesOption(t, []string{"acme", "oldacme"}))
	teardownStart(t, p, teardownDog)
	if len(fake.CreatedSessions) != 0 {
		t.Fatalf("Start created %v, want it to adopt the legacy session", fake.CreatedSessions)
	}

	if err := p.Stop(context.Background(), teardownDog); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if len(fake.DestroyedSessions) != 0 {
		t.Errorf("destroyed = %v, want the legacy session kept", fake.DestroyedSessions)
	}
	if len(fake.Sessions) != 1 || fake.Sessions[0].ID != "session:gc:oldacme" || fake.Sessions[0].Label != "gc:oldacme" {
		t.Errorf("sessions = %v, want the adopted session unchanged", fake.Sessions)
	}
}

func TestStopToleratesLookupFailures(t *testing.T) {
	for name, err := range map[string]error{
		"list unavailable": rex.ErrUnavailable,
		"list other":       errors.New("boom"),
		"clients other":    errors.New("boom"),
	} {
		t.Run(name, func(t *testing.T) {
			fake, p := teardownStarted(t, teardownDog)
			fake.ListSessionsErr = err
			fake.ListClientsErr = err
			if err := p.Stop(context.Background(), teardownDog); err != nil {
				t.Errorf("Stop = %v, want nil: the pane is closed", err)
			}
		})
	}
}

func TestStopKeepsDuplicateCanonicalSessions(t *testing.T) {
	fake, p := teardownStarted(t, teardownDog)
	fake.Sessions = append(fake.Sessions, rex.Session{ID: "session:dup", Label: fake.CreatedSessions[0]})

	if err := p.Stop(context.Background(), teardownDog); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	// Only the exact session the stop used is a removal candidate, so the
	// duplicate stays: removing by label could destroy another server's shell.
	if len(fake.Sessions) != 1 || fake.Sessions[0].ID != "session:dup" {
		t.Errorf("sessions = %v, want only the untouched duplicate left", fake.Sessions)
	}
}

// destroyedBeforeLaunch models the city session vanishing after Start found it
// but before Start's launch read the view: a concurrent Stop of the last pane
// in another rig removes the empty session, or an operator does (GCREX-31).
type destroyedBeforeLaunch struct {
	*faketest.Fake
	once sync.Once
}

func (d *destroyedBeforeLaunch) SessionView(ctx context.Context, sid rex.SessionID) (rex.SessionView, error) {
	gone := false
	d.once.Do(func() {
		d.Sessions = nil
		gone = true
	})
	if gone {
		return rex.SessionView{}, rex.ErrNotFound
	}
	return d.Fake.SessionView(ctx, sid)
}

func TestStartRecreatesSessionDestroyedBeforeLaunch(t *testing.T) {
	fake := faketest.New()
	p := provider.New("acme", &destroyedBeforeLaunch{Fake: fake})

	if err := p.Start(context.Background(), teardownDog, teardownCfg); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if len(fake.CreatedSessions) != 2 {
		t.Errorf("created sessions = %v, want the session recreated once", fake.CreatedSessions)
	}
	if len(fake.CreatedWindows) != 1 {
		t.Errorf("windows = %d, want 1", len(fake.CreatedWindows))
	}
	if b, ok := p.Binding(teardownDog); !ok || b.SessionID == "" {
		t.Errorf("binding = %+v, %v; want a recorded pane", b, ok)
	}
}

func TestStopToleratesSessionGoneBeforeClose(t *testing.T) {
	fake, p := teardownStarted(t, teardownDog)
	fake.Sessions = nil

	if err := p.Stop(context.Background(), teardownDog); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if _, ok := p.Binding(teardownDog); ok {
		t.Error("binding still stored after Stop")
	}
}

// TestStopUnreachableWithoutBindingSucceeds pins GCREX-51 REQ-001: with Rex
// unreachable and no stored binding answering to the name, stop succeeds, as
// herdr-core does for an absent name with the core down. This is the
// stored-binding policy, not proof that no process is alive (parity stop
// deviation 8).
func TestStopUnreachableWithoutBindingSucceeds(t *testing.T) {
	fake := faketest.New()
	fake.ListSessionsErr = rex.ErrUnavailable
	p := provider.New("acme", fake)

	if err := p.Stop(context.Background(), teardownDog); err != nil {
		t.Fatalf("Stop with Rex unreachable and no binding = %v, want nil", err)
	}
	if _, ok := p.Binding(teardownDog); ok {
		t.Error("binding exists after Stop, want none")
	}
	if len(fake.ClosedBlocks) != 0 {
		t.Errorf("closed = %v, want none", fake.ClosedBlocks)
	}
}

// TestStopUnreachableWithBindingKeepsError pins REQ-002: while Rex is down a
// bound pane may still be alive and stop cannot close it, so success would hide
// a leak. Stop keeps the error and leaves the binding and meta for a retry.
// SessionViewErr is the load-bearing fault: revalidate uses SessionView, and a
// successful empty view would drop the record as a stale pane.
func TestStopUnreachableWithBindingKeepsError(t *testing.T) {
	fake, p := teardownStarted(t, teardownDog)
	if err := p.SetMeta(context.Background(), teardownDog, "GC_USER_KEY", "keep"); err != nil {
		t.Fatalf("SetMeta: %v", err)
	}
	fake.SessionViewErr = rex.ErrUnavailable

	if err := p.Stop(context.Background(), teardownDog); !errors.Is(err, rex.ErrUnavailable) {
		t.Fatalf("Stop = %v, want ErrUnavailable", err)
	}
	if _, ok := p.Binding(teardownDog); !ok {
		t.Error("binding dropped although Stop failed")
	}
	if v, err := p.GetMeta(context.Background(), teardownDog, "GC_USER_KEY"); err != nil || v != "keep" {
		t.Errorf("meta = %q, %v, want it kept", v, err)
	}
	if len(fake.ClosedBlocks) != 0 {
		t.Errorf("closed = %v, want none", fake.ClosedBlocks)
	}
}

// gatedLaunch pauses NewWindow while gate is set, so a test can run a stop in
// the launch window before the new pane commits (GCREX-31 review: the stop and
// start TOCTOU).
type gatedLaunch struct {
	*faketest.Fake
	gate    atomic.Bool
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (g *gatedLaunch) NewWindow(ctx context.Context, sid rex.SessionID, label string, spec rex.LaunchSpec) (rex.WindowID, rex.BlockID, error) {
	g.wait()
	return g.Fake.NewWindow(ctx, sid, label, spec)
}

func (g *gatedLaunch) NewSplit(ctx context.Context, sid rex.SessionID, anchor rex.BlockID, direction, side string, spec rex.LaunchSpec) (rex.BlockID, error) {
	g.wait()
	return g.Fake.NewSplit(ctx, sid, anchor, direction, side, spec)
}

func (g *gatedLaunch) wait() {
	if g.gate.Load() {
		g.once.Do(func() { close(g.entered) })
		<-g.release
	}
}

// TestStopDoesNotDestroySessionDuringLaunch pins the fix for the stop/start
// race: the city session looks empty to the stop while another start's launch
// has not committed its window yet, but the session must survive so the
// launch does not fill a removed session.
func TestStopDoesNotDestroySessionDuringLaunch(t *testing.T) {
	fake := &gatedLaunch{
		Fake:    faketest.New(),
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
	p := provider.New("acme", fake)
	teardownStart(t, p, teardownDog) // one live pane, in the dog rig tab

	fake.gate.Store(true)
	started := make(chan error, 1)
	go func() { started <- p.Start(context.Background(), teardownCat, teardownCfg) }()
	<-fake.entered

	// The ox launch holds no window yet, so the stop sees an empty session.
	if err := p.Stop(context.Background(), teardownDog); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	close(fake.release)
	if err := <-started; err != nil {
		t.Fatalf("Start %s: %v", teardownCat, err)
	}

	if len(fake.DestroyedSessions) != 0 {
		t.Errorf("destroyed = %v, want none while a launch is in flight", fake.DestroyedSessions)
	}
	if len(fake.Sessions) != 1 {
		t.Errorf("sessions = %v, want the city session kept", fake.Sessions)
	}
	if _, ok := p.Binding(teardownCat); !ok {
		t.Error("no binding for the launched pane")
	}
}

// viewErrAfterClose fails SessionView once a block has closed, so the failure
// lands in the empty-session check, after the stop's own resolve.
type viewErrAfterClose struct {
	*faketest.Fake
	closed atomic.Bool
	boom   atomic.Bool
}

func (v *viewErrAfterClose) CloseBlock(ctx context.Context, sid rex.SessionID, block rex.BlockID) error {
	if err := v.Fake.CloseBlock(ctx, sid, block); err != nil {
		return err
	}
	v.closed.Store(true)
	return nil
}

func (v *viewErrAfterClose) SessionView(ctx context.Context, sid rex.SessionID) (rex.SessionView, error) {
	if v.closed.Load() && v.boom.Load() {
		return rex.SessionView{}, errors.New("session view down")
	}
	return v.Fake.SessionView(ctx, sid)
}

// TestStopKeepsBindingWhenEmptySessionCheckFails pins the fix for forgetting
// the binding before the destroy: a failed check must keep the session id a
// retry needs, or the empty session is never removed.
func TestStopKeepsBindingWhenEmptySessionCheckFails(t *testing.T) {
	fake := &viewErrAfterClose{Fake: faketest.New()}
	fake.boom.Store(true)
	p := provider.New("acme", fake)
	teardownStart(t, p, teardownDog)

	if err := p.Stop(context.Background(), teardownDog); err == nil {
		t.Fatal("Stop = nil, want the empty-session check error")
	}
	if _, ok := p.Binding(teardownDog); !ok {
		t.Error("binding forgotten although the session was not removed")
	}
	if len(fake.DestroyedSessions) != 0 {
		t.Errorf("destroyed = %v, want none", fake.DestroyedSessions)
	}

	fake.boom.Store(false)
	if err := p.Stop(context.Background(), teardownDog); err != nil {
		t.Fatalf("retry Stop: %v", err)
	}
	if len(fake.DestroyedSessions) != 1 {
		t.Errorf("destroyed = %v, want the retry to remove the empty session", fake.DestroyedSessions)
	}
	if _, ok := p.Binding(teardownDog); ok {
		t.Error("binding still stored after the retry")
	}
}

func TestStopToleratesSessionGoneAtDestroy(t *testing.T) {
	fake, p := teardownStarted(t, teardownDog)
	fake.DestroySessionErr = rex.ErrNotFound

	if err := p.Stop(context.Background(), teardownDog); err != nil {
		t.Fatalf("Stop = %v, want nil for a session another client removed", err)
	}
	if _, ok := p.Binding(teardownDog); ok {
		t.Error("binding still stored after Stop")
	}
}
