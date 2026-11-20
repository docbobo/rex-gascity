package provider

import (
	"context"
	"errors"
	"os"
	"strconv"
	"sync"
	"testing"

	"github.com/docbobo/rex-gascity/internal/rex"
	"github.com/docbobo/rex-gascity/internal/rex/faketest"
)

// reviewPane gives the fake one city session holding the Indigo pane, with no
// binding written: the recordless pane an operator or a restore leaves behind.
func reviewPane(fake *faketest.Fake) {
	fake.Sessions = []rex.Session{{ID: "session:acme", Label: testLabel}}
	fake.View = rex.SessionView{
		Label:          testLabel,
		ActiveWindowID: "window:Indigo",
		Windows:        []rex.Window{indigoWindow()},
	}
}

// TestObserveRecordlessPaneHasNoIdentity proves the parity document's observe
// rule: a pane that took the name without Start has no gc identity, even when
// the sidecar still holds the retired incarnation's keys.
func TestObserveRecordlessPaneHasNoIdentity(t *testing.T) {
	fake := faketest.New()
	p := New("acme", fake)
	reviewPane(fake)
	if err := p.metaSet("Indigo--dog-1", "GC_SESSION_ID", "sid-stale"); err != nil {
		t.Fatalf("metaSet: %v", err)
	}

	ans, err := p.Observe(context.Background(), "Indigo--dog-1", nil)
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if ans.State != ObserveRunning || ans.Object == nil || ans.Object.ID != "block:dog-1" {
		t.Fatalf("Observe = %+v, want the recordless pane running", ans)
	}
	if ans.Identity == nil || len(ans.Identity) != 0 {
		t.Errorf("identity = %v, want {}: a pane the provider did not start has no gc identity", ans.Identity)
	}
	if _, ok := p.Binding("Indigo--dog-1"); ok {
		t.Error("Observe saved a binding; discovery must stay read-only")
	}
}

// unreadableMetaStore fails every read, standing in for a corrupt sidecar.
type unreadableMetaStore struct{ *memoryMetaStore }

func (s unreadableMetaStore) Read(metaKey) (string, bool, error) {
	return "", false, errors.New("sidecar unreadable")
}

func (s unreadableMetaStore) Clear(SessionName) error { return nil }

// TestObserveRecordlessPaneUnreadableSidecarHasNoIdentity proves the same
// answer holds when the sidecar cannot be read: the identity is {} (a box with
// no gc identity), never null ("not yet readable"), because the provider did
// not start the pane and must not wait on its sidecar.
func TestObserveRecordlessPaneUnreadableSidecarHasNoIdentity(t *testing.T) {
	fake := faketest.New()
	p := New("acme", fake, WithMetaStore(unreadableMetaStore{NewMemoryMetaStore()}))
	reviewPane(fake)

	ans, err := p.Observe(context.Background(), "Indigo--dog-1", nil)
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if ans.Identity == nil || len(ans.Identity) != 0 {
		t.Errorf("identity = %v, want {} for a recordless pane with an unreadable sidecar", ans.Identity)
	}
}

// TestObserveRecordlessDeadPaneHasNoIdentity proves the dead half of the
// recordless rule. A restored pane that already exited answers dead with an
// empty identity, and Observe saves no binding for it.
func TestObserveRecordlessDeadPaneHasNoIdentity(t *testing.T) {
	fake := faketest.New()
	p := New("acme", fake)
	reviewPane(fake)
	fake.Processes = map[rex.BlockID]rex.ProcessInfo{
		"block:dog-1": {LastExit: &rex.ExitStatus{}},
	}
	if err := p.metaSet("Indigo--dog-1", "GC_SESSION_ID", "sid-stale"); err != nil {
		t.Fatalf("metaSet: %v", err)
	}

	ans, err := p.Observe(context.Background(), "Indigo--dog-1", nil)
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if ans.State != ObserveDead || ans.Agent != "dead" || ans.Object == nil || ans.Object.ID != "block:dog-1" {
		t.Fatalf("Observe = %+v, want the recordless pane dead", ans)
	}
	if ans.Identity == nil || len(ans.Identity) != 0 {
		t.Errorf("identity = %v, want an empty set", ans.Identity)
	}
	if _, ok := p.Binding("Indigo--dog-1"); ok {
		t.Error("Observe saved a binding, but discovery must stay read-only")
	}
}

// TestInventoryAdoptedDiscoveryHasNoIdentity proves the saved-discovery path:
// IsRunning adopts the recordless pane, and the stored record must remember it
// was adopted so Inventory does not hand back the retired identity.
func TestInventoryAdoptedDiscoveryHasNoIdentity(t *testing.T) {
	fake := faketest.New()
	p := New("acme", fake)
	reviewPane(fake)
	if err := p.metaSet("Indigo--dog-1", "GC_SESSION_ID", "sid-stale"); err != nil {
		t.Fatalf("metaSet: %v", err)
	}
	running, err := p.IsRunning(context.Background(), "Indigo--dog-1")
	if err != nil || !running {
		t.Fatalf("IsRunning = %v, %v; want the recordless pane alive", running, err)
	}
	bd, ok := p.Binding("Indigo--dog-1")
	if !ok {
		t.Fatal("IsRunning did not save the adopted binding")
	}
	if !bd.Adopted {
		t.Errorf("binding = %+v, want Adopted for a pane this provider did not start", bd)
	}

	inv, err := p.Inventory(context.Background())
	if err != nil {
		t.Fatalf("Inventory: %v", err)
	}
	if len(inv.Sessions) != 1 {
		t.Fatalf("sessions = %+v, want the adopted pane", inv.Sessions)
	}
	if id := inv.Sessions[0].Identity; id == nil || len(id) != 0 {
		t.Errorf("identity = %v, want {} for the adopted pane", id)
	}
}

// swapViewClient runs the test's replacement just before the view answer, so
// the replacement lands after kill-object's holder check and before its close.
type swapViewClient struct {
	*faketest.Fake
	mu   sync.Mutex
	swap func()
}

func (c *swapViewClient) SessionView(ctx context.Context, sid rex.SessionID) (rex.SessionView, error) {
	c.mu.Lock()
	swap := c.swap
	c.swap = nil
	c.mu.Unlock()
	if swap != nil {
		swap()
	}
	return c.Fake.SessionView(ctx, sid)
}

// TestKillObjectKeepsAReplacementThatLandsBeforeTheClose proves the exact
// binding is re-proved under the state lock: a Start of the canonical name can
// replace the record between the holder check and the close, and the kill must
// answer gone instead of retiring the replacement's meta and binding.
func TestKillObjectKeepsAReplacementThatLandsBeforeTheClose(t *testing.T) {
	ctx := context.Background()
	fake := faketest.New()
	var p *Provider
	client := &swapViewClient{Fake: fake}
	p = New("acme", client)
	p.nudge = defaultNudgeConfig()
	if err := p.Start(ctx, "Indigo--dog-1", Config{Argv: []string{"sleep", "300"}}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	// Arm the replacement only now: Start reads the view too.
	client.swap = func() {
		if err := p.binder.Record(Binding{
			Name:         "Indigo--dog-1",
			SessionID:    "session:acme",
			SessionLabel: testLabel,
			WindowID:     "window:Indigo",
			BlockID:      "block:replacement",
		}); err != nil {
			t.Errorf("Record: %v", err)
		}
		if err := p.metaSet("Indigo--dog-1", "GC_SESSION_ID", "sid-new"); err != nil {
			t.Errorf("metaSet: %v", err)
		}
	}
	// An app client makes the attachment check read the view, where the swap
	// lands; the dead pane has no owner, so attachment stays false.
	fake.Clients = []rex.ClientInfo{{ID: "c1", Kind: "app"}}
	bd, ok := p.Binding("Indigo--dog-1")
	if !ok {
		t.Fatal("no binding after Start")
	}
	// A dead pane and an app client make the corpse path read the view.
	fake.Processes = map[rex.BlockID]rex.ProcessInfo{bd.BlockID: {LastExit: &rex.ExitStatus{}}}
	fake.Clients = []rex.ClientInfo{{ID: "c1", Kind: "app"}}
	fake.View.ActiveWindowID = "window:other"

	res, err := p.KillObject(ctx, "Indigo--dog-1", KillObjectRequest{
		Object: observeObjectFor(bd),
		Mode:   "corpse",
	})
	if err != nil {
		t.Fatalf("KillObject: %v", err)
	}
	if res != "gone" {
		t.Errorf("result = %q, want gone: the record changed before the close", res)
	}
	if len(fake.ClosedBlocks) != 0 {
		t.Errorf("closed blocks = %v, want none", fake.ClosedBlocks)
	}
	got, ok := p.Binding("Indigo--dog-1")
	if !ok || got.BlockID != "block:replacement" {
		t.Errorf("binding = %+v, %v; want the replacement kept", got, ok)
	}
	if meta, _ := p.GetMeta(ctx, "Indigo--dog-1", "GC_SESSION_ID"); meta != "sid-new" {
		t.Errorf("GC_SESSION_ID = %q, want the replacement's sid-new kept", meta)
	}
}

// recordSurvivor stores a survivor on the current binding.
func recordSurvivor(t *testing.T, p *Provider, name SessionName, pid int, proc string) Binding {
	t.Helper()
	bd, ok := p.Binding(string(name))
	if !ok {
		t.Fatalf("no binding for %s", name)
	}
	bd.Survivors = append(bd.Survivors, Survivor{PID: pid, Name: proc, Start: reapStart})
	p.binder.Record(bd)
	return bd
}

// TestKillObjectRetiresTheSurvivorsItRecorded proves the kill-object half of
// the survivor retry: an absent pane whose record still names unreaped
// descendants must reap them before the record is retired.
func TestKillObjectRetiresTheSurvivorsItRecorded(t *testing.T) {
	reapShortWaits(t)
	ctx := context.Background()
	fake, p, signals := reapProvider(t, liveTree, func(int) string { return "" })
	bd := recordSurvivor(t, p, reapDog, 300, "bun")
	// The pane is gone: the record is now the only name for the survivor.
	fake.View = rex.SessionView{}
	fake.Processes = nil
	fake.VanishedBlocks = map[rex.BlockID]bool{bd.BlockID: true}
	alive := true
	psCalls := 0
	p.runner = &FakeRunner{Handle: func(_ context.Context, c RunCall) (string, error) {
		if c.Name != "ps" {
			return "", nil
		}
		psCalls++
		if !alive {
			return "", nil
		}
		return reapTable([4]string{"300", "1", "S", "bun"}), nil
	}}
	p.signal = func(pid int, sig os.Signal) error {
		*signals = append(*signals, sig.String()+":"+strconv.Itoa(pid))
		if sig.String() == "terminated" {
			alive = false
		}
		return nil
	}

	res, err := p.KillObject(ctx, string(reapDog), KillObjectRequest{Object: observeObjectFor(bd), Mode: "corpse"})
	if err != nil {
		t.Fatalf("KillObject: %v", err)
	}
	if res != "gone" {
		t.Errorf("result = %q, want gone after the retry reaped the record", res)
	}
	if len(*signals) == 0 || (*signals)[0] != "terminated:300" {
		t.Errorf("signals = %v, want the recorded survivor TERMed", *signals)
	}
	if _, ok := p.Binding(string(reapDog)); ok {
		t.Error("binding survived the successful survivor reap")
	}
	_ = psCalls
}

// TestKillObjectFailsWhileRecordedSurvivorsSurvive proves the failed retry:
// kill-object must not answer gone while the recorded descendants still run.
func TestKillObjectFailsWhileRecordedSurvivorsSurvive(t *testing.T) {
	reapShortWaits(t)
	ctx := context.Background()
	fake, p, _ := reapProvider(t, liveTree, func(int) string {
		return reapTable([4]string{"300", "1", "S", "bun"})
	})
	bd := recordSurvivor(t, p, reapDog, 300, "bun")
	fake.View = rex.SessionView{}
	fake.Processes = nil
	fake.VanishedBlocks = map[rex.BlockID]bool{bd.BlockID: true}

	if _, err := p.KillObject(ctx, string(reapDog), KillObjectRequest{Object: observeObjectFor(bd), Mode: "corpse"}); err == nil {
		t.Fatal("KillObject = nil, want a survivor failure while pid 300 runs")
	}
	if _, ok := p.Binding(string(reapDog)); !ok {
		t.Error("binding gone after a failed survivor reap, want it kept for the retry")
	}
}

// TestStopRetryReadsTheAliasRecord proves Stop finds the primary record and its
// survivors when the caller names it by alias.
func TestStopRetryReadsTheAliasRecord(t *testing.T) {
	reapShortWaits(t)
	ctx := context.Background()
	alive := true
	_, p, signals := reapProvider(t, liveTree, func(int) string {
		if !alive {
			return ""
		}
		return reapTable([4]string{"300", "1", "S", "bun"})
	})
	bd := recordSurvivor(t, p, reapDog, 300, "bun")
	bd.Aliases = []SessionName{"Rig--dog-1"}
	p.binder.Record(bd)
	fake := p.client.(*faketest.Fake)
	fake.View = rex.SessionView{}
	fake.Processes = nil
	p.signal = func(pid int, sig os.Signal) error {
		*signals = append(*signals, sig.String()+":"+strconv.Itoa(pid))
		if pid == 300 && sig.String() == "terminated" {
			alive = false
		}
		return nil
	}

	if err := p.Stop(ctx, "Rig--dog-1"); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if len(*signals) == 0 || (*signals)[0] != "terminated:300" {
		t.Errorf("signals = %v, want the alias's primary record reaped", *signals)
	}
}

// errStoreWrite is the injected binding-store failure.
var errStoreWrite = errors.New("binding store write failed")

// gatedBindingStore fails writes while fail is set.
type gatedBindingStore struct {
	*MemoryStore
	mu   sync.Mutex
	fail bool
}

func (s *gatedBindingStore) Write(k SessionName, v Binding) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fail {
		return errStoreWrite
	}
	return s.MemoryStore.Write(k, v)
}

// TestStopSurvivorWriteFailureFailsTheStop proves a stop does not report a
// plain survivor error when the survivor record could not be saved: a lost
// record would make the retry report success while the processes run.
func TestStopSurvivorWriteFailureFailsTheStop(t *testing.T) {
	reapShortWaits(t)
	ctx := context.Background()
	fake := faketest.New()
	runner := &FakeRunner{}
	runner.Handle = func(_ context.Context, c RunCall) (string, error) {
		if c.Name != "ps" {
			return "", nil
		}
		return reapTable(
			[4]string{"200", "1", "Ss", "/bin/zsh"},
			[4]string{"100", "200", "S+", "claude"},
		), nil
	}
	store := &gatedBindingStore{MemoryStore: NewMemoryStore()}
	p := New("acme", fake, WithRunner(runner), WithBindingStore(store))
	if err := p.Start(ctx, reapDog, Config{Argv: []string{"claude"}}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	fake.Processes = map[rex.BlockID]rex.ProcessInfo{"block:dog-1": liveTree}
	p.signal = func(int, os.Signal) error { return nil }
	store.fail = true

	err := p.Stop(ctx, reapDog)
	if !errors.Is(err, errStoreWrite) {
		t.Fatalf("Stop = %v, want the survivor-record write error", err)
	}
}

// TestReapRetryIncludesDescendantsOfRecordedSurvivors proves the retry reads the
// host table: a survivor recorded by an earlier stop can spawn a child before
// the retry, and the closed pane cannot report that child.
func TestReapRetryIncludesDescendantsOfRecordedSurvivors(t *testing.T) {
	reapShortWaits(t)
	ctx := context.Background()
	alive := true
	_, p, signals := reapProvider(t, liveTree, func(int) string {
		if !alive {
			return ""
		}
		return reapTable(
			[4]string{"300", "1", "S", "bun"},
			[4]string{"500", "300", "S", "node"},
		)
	})
	bd := recordSurvivor(t, p, reapDog, 300, "bun")
	fake := p.client.(*faketest.Fake)
	fake.View = rex.SessionView{}
	fake.Processes = nil
	fake.VanishedBlocks = map[rex.BlockID]bool{bd.BlockID: true}
	p.signal = func(pid int, sig os.Signal) error {
		*signals = append(*signals, sig.String()+":"+strconv.Itoa(pid))
		if pid == 300 && sig.String() == "terminated" {
			alive = false
		}
		return nil
	}
	_ = bd

	// The pane is closed; only the record names pid 300, whose new child 500
	// must be signalled with it.
	if err := p.Stop(ctx, reapDog); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	want := "terminated:500"
	if !containsSignal(*signals, want) {
		t.Errorf("signals = %v, want %s: a new descendant of a recorded survivor", *signals, want)
	}
}

func containsSignal(signals []string, want string) bool {
	for _, s := range signals {
		if s == want {
			return true
		}
	}
	return false
}

// TestStartClearsTheRetiredAliasIdentity proves the alias start clears the
// canonical name's identity too: Forget drops the record, so a failed relaunch
// would otherwise leave the retired GC_SESSION_ID readable under the primary.
func TestStartClearsTheRetiredAliasIdentity(t *testing.T) {
	const canonical = "Indigo--dog-1"
	const alias = "Rig--dog-1"
	env := map[string]string{"GC_SESSION_ID": "mc-wisp-abc", "GC_INSTANCE_TOKEN": "token-abc"}
	ctx := context.Background()
	fake := faketest.New()
	p := New("acme", fake)
	if err := p.Start(ctx, canonical, Config{Argv: []string{"pi"}, Env: env}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	bd, ok := p.Binding(canonical)
	if !ok {
		t.Fatal("no binding after Start")
	}
	bd.Aliases = []SessionName{alias}
	p.binder.Record(bd)
	fake.Processes = map[rex.BlockID]rex.ProcessInfo{bd.BlockID: {LastExit: &rex.ExitStatus{}}}

	cfg := Config{Argv: []string{"pi"}, Env: env, WorkDir: t.TempDir() + "/missing"}
	if err := p.Start(ctx, alias, cfg); err == nil {
		t.Fatal("Start with a missing work_dir succeeded")
	}
	if got, _ := p.GetMeta(ctx, canonical, "GC_SESSION_ID"); got != "" {
		t.Errorf("canonical GC_SESSION_ID = %q, want empty: the retired identity survived", got)
	}
}
