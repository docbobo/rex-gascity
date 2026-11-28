package provider

import (
	"bytes"
	"context"
	"errors"
	"maps"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/docbobo/rex-gascity/internal/launch"
	"github.com/docbobo/rex-gascity/internal/rex"
	"github.com/docbobo/rex-gascity/internal/rex/faketest"
)

// TestRecordOrDiscardRetiresPaneWhenRecordFails is REQ-003: the one
// record-or-discard seam retires the pane whose final record failed under the
// discardLaunchPolicy (snapshot, close, re-read, reap; no clear, no forget, no
// destroy), returns the record error, keeps the provisional record and the
// seeded key for start's rollback, and destroys no session under the launch
// lock.
func TestRecordOrDiscardRetiresPaneWhenRecordFails(t *testing.T) {
	reapShortWaits(t)
	ctx := context.Background()
	c := &retireClient{Fake: faketest.New(), failProcess: map[int]error{}}
	c.View = rex.SessionView{Label: testLabel, Windows: []rex.Window{{
		ID:     "window:Indigo",
		Label:  "Rig: Indigo",
		Blocks: []rex.Block{{ID: "block:dog-1", Label: "dog-1"}},
	}}}
	store := &failOnceRecordStore{MemoryStore: NewMemoryStore()}
	p := New("acme", c, WithBindingStore(store))
	p.recordProvisionalBinding(reapDog, "session:acme", "", "attempt", Config{})
	bd := p.newBinding(reapDog, "session:acme", "window:Indigo", "block:dog-1", "split", "attempt", Config{})

	key := metaKey{reapDog, startMetaKeys()[0]}
	if err := p.meta.Write(key, "seeded"); err != nil {
		t.Fatalf("seed identity: %v", err)
	}

	if err := p.recordOrDiscard(ctx, reapDog, bd); !errors.Is(err, errStoreWrite) {
		t.Fatalf("recordOrDiscard = %v, want the record error", err)
	}
	if !slices.Contains(c.ClosedBlocks, "block:dog-1") {
		t.Errorf("closed = %v, want the created pane retired", c.ClosedBlocks)
	}
	if !slices.Contains(c.got(), "process") {
		t.Errorf("calls = %v, want a process snapshot", c.got())
	}
	if _, ok := mustBinding(t)(p.Binding(reapDog)); !ok {
		t.Error("binding dropped; the discard policy leaves it for start's rollback")
	}
	got, ok, readErr := p.meta.Read(key)
	if readErr != nil {
		t.Fatalf("meta read: %v", readErr)
	}
	if got != "seeded" {
		t.Errorf("identity key = %q (present=%v), want %q kept for start's rollback", got, ok, "seeded")
	}
	if len(c.DestroyedSessions) != 0 {
		t.Errorf("destroyed = %v, want none under the launch lock", c.DestroyedSessions)
	}
}

// launchCfg is the start config the launchPane seam tests use: distinct,
// non-empty identity inputs, so a cleared key is observable.
func launchCfg() Config {
	return Config{Argv: []string{"claude"}, Env: map[string]string{
		"GC_SESSION_ID":     "mc-launch",
		"GC_INSTANCE_TOKEN": "token-launch",
	}}
}

// launchVia runs the launch step for name through the launchPane seam, as a
// fresh start (no prior binding) would.
func launchVia(ctx context.Context, p *Provider, name string, cfg Config) error {
	_, _, err := launchPaneFor(ctx, p, name, cfg, false)
	return err
}

// failLaunch fails every NewWindow after running before, which a test uses to
// stand in for state another step leaves behind during the launch. calls
// counts NewWindow calls.
type failLaunch struct {
	*faketest.Fake
	err    error
	before func()
	calls  int
}

func (f *failLaunch) NewWindow(context.Context, rex.SessionID, string, rex.LaunchSpec) (rex.WindowID, rex.BlockID, error) {
	f.calls++
	if f.before != nil {
		f.before()
	}
	return "", "", f.err
}

// wantIdentityCleared asserts that the identity keys launchCfg seeds are gone.
func wantIdentityCleared(t *testing.T, p *Provider, name string) {
	t.Helper()
	for _, key := range []string{"GC_SESSION_ID", "GC_INSTANCE_TOKEN"} {
		if v, ok, err := p.metaGet(SessionName(name), key); err != nil || ok {
			t.Errorf("%s = %q, %v, %v after the failed launch; want cleared", key, v, ok, err)
		}
	}
}

// TestLaunchPaneRollsBackIdentityAndBindingOnFailure is issue #76 BR-4: a
// launch failure that is not rex.ErrNotFound runs no retry, removes the empty
// city session, clears the seeded identity and forgets the provisional record.
func TestLaunchPaneRollsBackIdentityAndBindingOnFailure(t *testing.T) {
	ctx := context.Background()
	boom := errors.New("launch failed")
	fake := &failLaunch{Fake: faketest.New(), err: boom}
	p := New("acme", fake)

	if err := launchVia(ctx, p, reapDog, launchCfg()); !errors.Is(err, boom) {
		t.Fatalf("launch = %v, want %v", err, boom)
	}
	if fake.calls != 1 {
		t.Errorf("NewWindow calls = %d, want 1: a generic failure must not retry", fake.calls)
	}
	if len(fake.DestroyedSessions) != 1 {
		t.Errorf("destroyed = %v, want the empty city session removed", fake.DestroyedSessions)
	}
	wantIdentityCleared(t, p, reapDog)
	if bd, ok := mustBinding(t)(p.bindings.Read(SessionName(reapDog))); ok {
		t.Errorf("binding = %+v, want the provisional record forgotten", bd)
	}
}

// TestLaunchPaneKeepsProvisionalWhenEmptySessionRemovalFails is issue #76
// BR-4: when the failed launch cannot remove the empty city session, the
// provisional record stays so a later Stop still finds the session id, and a
// warning names the failure. Identity is still cleared.
func TestLaunchPaneKeepsProvisionalWhenEmptySessionRemovalFails(t *testing.T) {
	ctx := context.Background()
	boom := errors.New("launch failed")
	fake := &failLaunch{Fake: faketest.New(), err: boom}
	fake.DestroySessionErr = errors.New("destroy down")
	var warnings bytes.Buffer
	p := New("acme", fake, WithWarnings(&warnings))

	if err := launchVia(ctx, p, reapDog, launchCfg()); !errors.Is(err, boom) {
		t.Fatalf("launch = %v, want %v", err, boom)
	}
	wantIdentityCleared(t, p, reapDog)
	if bd, ok := mustBinding(t)(p.bindings.Read(SessionName(reapDog))); !ok || bd.SessionID == "" || bd.BlockID != "" {
		t.Errorf("binding = %+v, %v; want the provisional record kept with its session id", bd, ok)
	}
	if !strings.Contains(warnings.String(), "remove the empty city session") {
		t.Errorf("warnings = %q, want the failed removal named", warnings.String())
	}
}

// TestLaunchPaneKeepsBindingWithSurvivors is issue #76 BR-4: a binding that
// records survivors is the only name for a process that could not be reaped,
// so a failed launch clears identity but does not forget it.
func TestLaunchPaneKeepsBindingWithSurvivors(t *testing.T) {
	ctx := context.Background()
	boom := errors.New("launch failed")
	fake := &failLaunch{Fake: faketest.New(), err: boom}
	p := New("acme", fake)
	n := SessionName(reapDog)
	fake.before = func() {
		// Stand in for a discard retire that could not reap a descendant: it
		// records the survivor on the binding before the launch fails.
		bd, ok := mustBinding(t)(p.bindings.Read(n))
		if !ok {
			t.Error("no provisional binding during the launch")
			return
		}
		bd.Survivors = []Survivor{{PID: 4242, Name: "bun"}}
		if err := p.bindings.Write(n, bd); err != nil {
			t.Errorf("write survivor: %v", err)
		}
	}

	if err := launchVia(ctx, p, reapDog, launchCfg()); !errors.Is(err, boom) {
		t.Fatalf("launch = %v, want %v", err, boom)
	}
	wantIdentityCleared(t, p, reapDog)
	if bd, ok := mustBinding(t)(p.bindings.Read(n)); !ok || len(bd.Survivors) != 1 || bd.Survivors[0].PID != 4242 {
		t.Errorf("binding = %+v, %v; want it kept with survivor 4242", bd, ok)
	}
}

// launchPaneFor builds the launchRequest start would build for name and cfg,
// finds or creates the city session, and calls launchPane under the name
// lock, as Start does.
func launchPaneFor(ctx context.Context, p *Provider, name string, cfg Config, relaunch bool) (startResult, launchRequest, error) {
	n := SessionName(name)
	env := launch.Project(cfg.Env)
	plan := launch.NewPlan(launch.Config{Argv: cfg.Argv, Env: env, WorkDir: cfg.WorkDir, Label: paneLabelFor(n, env)})
	cfg.Env = plan.Env
	req := launchRequest{
		name:     n,
		attempt:  "attempt-seam",
		tab:      tabLabel(n, cfg.Env, p.townTab),
		spec:     plan.Spec,
		relaunch: relaunch,
		cfg:      cfg,
	}
	var res startResult
	err := p.withNameLock(ctx, n, func() error {
		sid, err := p.findOrCreateCitySession(ctx)
		if err != nil {
			return err
		}
		req.session = sid
		res, err = p.launchPane(ctx, req)
		return err
	})
	return res, req, err
}

// TestLaunchPaneFocusesOnlyFreshWindow is issue #76 BR-4: a new window takes
// focus only on a fresh start; a relaunch window and any split never do.
func TestLaunchPaneFocusesOnlyFreshWindow(t *testing.T) {
	ctx := context.Background()
	fake := faketest.New()
	p := New("acme", fake)
	cfg := Config{Argv: []string{"claude"}}

	// Fresh window in the Indigo rig tab.
	if _, _, err := launchPaneFor(ctx, p, reapDog, cfg, false); err != nil {
		t.Fatalf("fresh window: %v", err)
	}
	// Same rig tab: a split.
	if _, _, err := launchPaneFor(ctx, p, "Indigo--cat-1", cfg, false); err != nil {
		t.Fatalf("split: %v", err)
	}
	// Another rig tab on a relaunch: a window that must not steal focus.
	if _, _, err := launchPaneFor(ctx, p, "Zephyr--fox-1", cfg, true); err != nil {
		t.Fatalf("relaunch window: %v", err)
	}

	if len(fake.CreatedWindows) != 2 || len(fake.CreatedSplits) != 1 {
		t.Fatalf("windows=%d splits=%d, want 2 and 1", len(fake.CreatedWindows), len(fake.CreatedSplits))
	}
	if !fake.CreatedWindows[0].Spec.Focus {
		t.Error("fresh window launched with focus=false, want true")
	}
	if fake.CreatedSplits[0].Spec.Focus {
		t.Error("split launched with focus=true, want false")
	}
	if fake.CreatedWindows[1].Spec.Focus {
		t.Error("relaunch window launched with focus=true, want false")
	}
}

// slowMissThenRecreate fails the first SessionView with rex.ErrNotFound after
// a delay, emptying Sessions as a concurrent Stop would, and gives the
// recreated session a distinct id. It touches Fake fields only while no Fake
// method is in flight.
type slowMissThenRecreate struct {
	*faketest.Fake
	delay     time.Duration
	once      sync.Once
	creates   int
	recreated rex.SessionID
}

func (s *slowMissThenRecreate) SessionView(ctx context.Context, sid rex.SessionID) (rex.SessionView, error) {
	miss := false
	s.once.Do(func() { miss = true })
	if miss {
		time.Sleep(s.delay)
		s.Sessions = nil
		return rex.SessionView{}, rex.ErrNotFound
	}
	return s.Fake.SessionView(ctx, sid)
}

func (s *slowMissThenRecreate) CreateSession(ctx context.Context, label string) (rex.SessionID, error) {
	sid, err := s.Fake.CreateSession(ctx, label)
	if err != nil {
		return "", err
	}
	s.creates++
	if s.creates == 1 {
		return sid, nil
	}
	s.recreated = rex.SessionID(string(sid) + "-recreated")
	for i := range s.Sessions {
		if s.Sessions[i].ID == sid {
			s.Sessions[i].ID = s.recreated
		}
	}
	return s.recreated, nil
}

// TestLaunchPaneReturnsFullResultAcrossRetry is issue #76 BR-4 "Result": after
// one ErrNotFound retry the result names the recreated session and the new
// block, carries the projected env the launch used, and measures launchDur
// from before the first try. It also carries the assertions of the removed
// Start-level vanished-session test (one recreate, one window, a recorded
// binding).
func TestLaunchPaneReturnsFullResultAcrossRetry(t *testing.T) {
	const delay = 50 * time.Millisecond
	ctx := context.Background()
	fake := &slowMissThenRecreate{Fake: faketest.New(), delay: delay}
	p := New("acme", fake)

	res, req, err := launchPaneFor(ctx, p, reapDog, launchCfg(), false)
	if err != nil {
		t.Fatalf("launchPane: %v", err)
	}
	if len(fake.CreatedSessions) != 2 {
		t.Fatalf("created sessions = %v, want the session recreated once", fake.CreatedSessions)
	}
	if len(fake.CreatedWindows) != 1 {
		t.Fatalf("windows = %d, want the retry's one", len(fake.CreatedWindows))
	}
	if fake.recreated == "" || res.sessionID != fake.recreated || fake.CreatedWindows[0].SessionID != fake.recreated {
		t.Errorf("result session %s, window session %s; want both the recreated %s",
			res.sessionID, fake.CreatedWindows[0].SessionID, fake.recreated)
	}
	bd, ok := mustBinding(t)(p.bindings.Read(SessionName(reapDog)))
	if !ok || bd.SessionID != fake.recreated || bd.BlockID == "" || res.blockID != bd.BlockID {
		t.Errorf("binding = %+v, %v; result block %s; want the recorded pane in the recreated session", bd, ok, res.blockID)
	}
	if !maps.Equal(res.env, req.cfg.Env) {
		t.Errorf("result env = %v, want the projected launch env %v", res.env, req.cfg.Env)
	}
	if res.launchDur < delay {
		t.Errorf("launchDur = %v, want at least the first attempt's %v", res.launchDur, delay)
	}
}
