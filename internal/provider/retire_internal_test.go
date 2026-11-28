package provider

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/docbobo/rex-gascity/internal/prochost"
	"github.com/docbobo/rex-gascity/internal/rex"
	"github.com/docbobo/rex-gascity/internal/rex/faketest"
)

// errRetireBoom is the injected failure in the retire interface tests.
var errRetireBoom = errors.New("injected retire failure")

// retireClient wraps the fake Rex and logs the calls retirement makes, in
// order: "process" (BlockProcess), "close", "view" (SessionView) and
// "destroy". failProcess fails BlockProcess call N (1-based, counted from the
// last reset). onClose runs before each CloseBlock.
type retireClient struct {
	*faketest.Fake
	mu          sync.Mutex
	calls       []string
	processN    int
	failProcess map[int]error
	onClose     func()
	onProcess   func(n int)
}

func (c *retireClient) reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls, c.processN = nil, 0
}

func (c *retireClient) log(op string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls = append(c.calls, op)
}

func (c *retireClient) got() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.calls)
}

func (c *retireClient) BlockProcess(ctx context.Context, sid rex.SessionID, block rex.BlockID) (rex.ProcessInfo, error) {
	c.mu.Lock()
	c.processN++
	n := c.processN
	err := c.failProcess[n]
	c.calls = append(c.calls, "process")
	onProcess := c.onProcess
	c.mu.Unlock()
	if onProcess != nil {
		onProcess(n)
	}
	if err != nil {
		return rex.ProcessInfo{}, err
	}
	return c.Fake.BlockProcess(ctx, sid, block)
}

func (c *retireClient) CloseBlock(ctx context.Context, sid rex.SessionID, block rex.BlockID) error {
	c.log("close")
	if c.onClose != nil {
		c.onClose()
	}
	return c.Fake.CloseBlock(ctx, sid, block)
}

func (c *retireClient) SessionView(ctx context.Context, sid rex.SessionID) (rex.SessionView, error) {
	c.log("view")
	return c.Fake.SessionView(ctx, sid)
}

func (c *retireClient) DestroySession(ctx context.Context, sid rex.SessionID) error {
	c.log("destroy")
	return c.Fake.DestroySession(ctx, sid)
}

// retireSetup starts reapDog on a logging fake Rex, sets one meta key so a
// test can see whether the sidecar survives, and returns the client (its log
// reset), the provider and the started binding. A nil host is an empty
// process table.
func retireSetup(t *testing.T, host *prochost.Fake, opts ...Option) (*retireClient, *Provider, Binding) {
	return retireSetupIn(t, host, "", opts...)
}

// retireSetupIn is retireSetup with a work dir for the started binding, so a
// test can make the drain diagnostic write.
func retireSetupIn(t *testing.T, host *prochost.Fake, workDir string, opts ...Option) (*retireClient, *Provider, Binding) {
	t.Helper()
	if host == nil {
		host = &prochost.Fake{}
	}
	c := &retireClient{Fake: faketest.New(), failProcess: map[int]error{}}
	p := New("acme", c, append([]Option{WithProcessHost(host)}, opts...)...)
	ctx := context.Background()
	if err := p.Start(ctx, reapDog, Config{Argv: []string{"claude"}, WorkDir: workDir}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := p.SetMeta(ctx, reapDog, "suspended", "yes"); err != nil {
		t.Fatalf("SetMeta: %v", err)
	}
	bd, ok := mustBinding(t)(p.Binding(reapDog))
	if !ok {
		t.Fatal("no binding after Start")
	}
	c.reset()
	return c, p, bd
}

// gitWorkDir builds a git work tree with one modified tracked file, so the
// drain diagnostic writes a file.
func gitWorkDir(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not available; the drain diagnostic needs it")
	}
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q")
	run("config", "user.email", "t@example.com")
	run("config", "user.name", "t")
	run("config", "commit.gpgsign", "false")
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-q", "-m", "init")
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("two\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func wantRexCalls(t *testing.T, c *retireClient, want ...string) {
	t.Helper()
	if got := c.got(); !slices.Equal(got, want) {
		t.Errorf("rex calls = %v, want %v", got, want)
	}
}

func bindingKept(t *testing.T, p *Provider) bool {
	t.Helper()
	_, ok := mustBinding(t)(p.Binding(reapDog))
	return ok
}

func metaKept(t *testing.T, p *Provider) bool {
	t.Helper()
	_, ok, err := p.meta.Read(metaKey{SessionName(reapDog), "suspended"})
	if err != nil {
		t.Fatalf("meta read: %v", err)
	}
	return ok
}

// stateLockHeld reports whether another holder has the provider's state lock.
func stateLockHeld(t *testing.T, p *Provider) bool {
	t.Helper()
	release, held, err := flockExclusiveNow(filepath.Join(p.ns.CanonicalDir(), "lock"))
	if err != nil {
		t.Errorf("probe the state lock: %v", err)
		return false
	}
	if release != nil {
		release()
	}
	return held
}

// dropFailActivity fails every activity Drop, so clearIncarnations fails.
type dropFailActivity struct {
	Store[SessionName, ActivityRecord]
	err error
}

func (s dropFailActivity) Drop(SessionName) error { return s.err }

// survivingTables answers the reap reads once armed: the first armed read is
// the pre-close pane tree, every later one shows the two detached
// descendants still alive past TERM and KILL.
func survivingTables(armed *bool, onCall func(n int)) *prochost.Fake {
	n := 0
	return &prochost.Fake{OnTable: func(int, bool) (prochost.Table, error) {
		if !*armed {
			return prochost.Table{}, nil
		}
		n++
		if onCall != nil {
			onCall(n)
		}
		if n == 1 {
			return reapTable(
				[4]string{"200", "1", "Ss", "/bin/zsh"},
				[4]string{"100", "200", "S+", "claude"},
				[4]string{"300", "100", "S", "bun"},
				[4]string{"400", "300", "S", "node"},
			), nil
		}
		return reapTable(
			[4]string{"300", "1", "S", "bun"},
			[4]string{"400", "1", "S", "node"},
		), nil
	}}
}

// TestRetirePolicyTable pins every named constructor's fields to the
// caller-policy table (REQ-006). Each later task adds its row.
func TestRetirePolicyTable(t *testing.T) {
	for _, tc := range []struct {
		name      string
		got, want retirePolicy
	}{
		{"stopPolicy", stopPolicy(), retirePolicy{
			diagnostic: true, snapshot: true, wrapSnapshotErr: true, close: true,
			reap: reapTree, clear: clearIncarnation, destroy: true, forget: forgetBinding,
		}},
		{"stopNoPanePolicy(true)", stopNoPanePolicy(true), retirePolicy{
			reap: reapRecorded, clear: clearIncarnation, destroy: true,
			reRecordOnDestroyFail: true, forget: forgetCaller,
		}},
		{"stopNoPanePolicy(false)", stopNoPanePolicy(false), retirePolicy{
			reap: reapRecorded, clear: clearIncarnation, destroy: true,
			forget: forgetCaller,
		}},
		{"stalePolicy", stalePolicy(), retirePolicy{
			reap: reapRecorded, clear: clearIncarnation, forget: forgetBinding,
		}},
		{"discardLaunchPolicy", discardLaunchPolicy(), retirePolicy{
			snapshot: true, close: true, reap: reapTree,
		}},
		{"startDeadPanePolicy", startDeadPanePolicy(), retirePolicy{
			close: true, clear: clearIdentity, forget: forgetCaller,
		}},
	} {
		if tc.got != tc.want {
			t.Errorf("%s = %+v, want %+v", tc.name, tc.got, tc.want)
		}
	}
}

// TestRetireStopPolicyRunsTheFixedOrder pins the step order (REQ-002,
// REQ-005): the diagnostic, snapshot, close, re-read, then the empty-session
// check and destroy, and the record and sidecar go last. The diagnostic must
// exist by the first snapshot.
func TestRetireStopPolicyRunsTheFixedOrder(t *testing.T) {
	repo := gitWorkDir(t)
	state := t.TempDir()
	c, p, bd := retireSetupIn(t, nil, repo, WithNamespace(NewNamespaceForDirs(state)))
	var diagAtSnapshot []string
	c.onProcess = func(n int) {
		if n == 1 {
			diagAtSnapshot, _ = filepath.Glob(filepath.Join(state, "diagnostics", "*.json"))
		}
	}
	res, err := p.retire(context.Background(), SessionName(reapDog), bd, stopPolicy())
	if err != nil {
		t.Fatalf("retire: %v", err)
	}
	if res.alreadyClosed {
		t.Error("alreadyClosed = true after a clean close")
	}
	wantRexCalls(t, c, "process", "close", "process", "view", "destroy")
	if len(diagAtSnapshot) != 1 {
		t.Errorf("diagnostics at the first snapshot = %v, want one: the diagnostic must run before the snapshot", diagAtSnapshot)
	}
	if bindingKept(t, p) {
		t.Error("binding kept after a clean retire")
	}
	if metaKept(t, p) {
		t.Error("meta kept after a clean retire")
	}
}

// TestRetireReportsAlreadyClosedWhenRexAnswersNotFound is REQ-007: a close
// Rex answers with ErrNotFound finishes the retirement and reports it.
func TestRetireReportsAlreadyClosedWhenRexAnswersNotFound(t *testing.T) {
	c, p, bd := retireSetup(t, nil)
	c.CloseBlockErr = rex.ErrNotFound
	res, err := p.retire(context.Background(), SessionName(reapDog), bd, stopPolicy())
	if err != nil {
		t.Fatalf("retire: %v", err)
	}
	if !res.alreadyClosed {
		t.Error("alreadyClosed = false, want true for a block Rex no longer knows")
	}
	if bindingKept(t, p) {
		t.Error("binding kept although the retirement finished")
	}
}

// TestRetireSnapshotFailureLeavesThePaneOpen is REQ-009.3: an unreadable
// tree fails before any CloseBlock, with stop's error text.
func TestRetireSnapshotFailureLeavesThePaneOpen(t *testing.T) {
	c, p, bd := retireSetup(t, nil)
	c.failProcess[1] = errRetireBoom
	_, err := p.retire(context.Background(), SessionName(reapDog), bd, stopPolicy())
	if !errors.Is(err, errRetireBoom) || !strings.Contains(err.Error(), "could not inspect the pane's process tree, leaving it open") {
		t.Fatalf("retire = %v, want the wrapped snapshot error", err)
	}
	wantRexCalls(t, c, "process")
	if !bindingKept(t, p) || !metaKept(t, p) {
		t.Error("record or meta lost although the pane stayed open")
	}
}

// TestRetireCloseErrorStopsEverything: a close error other than NotFound is
// returned and nothing after the close runs.
func TestRetireCloseErrorStopsEverything(t *testing.T) {
	c, p, bd := retireSetup(t, nil)
	c.CloseBlockErr = errRetireBoom
	_, err := p.retire(context.Background(), SessionName(reapDog), bd, stopPolicy())
	if !errors.Is(err, errRetireBoom) {
		t.Fatalf("retire = %v, want the close error", err)
	}
	wantRexCalls(t, c, "process", "close")
	if !bindingKept(t, p) || !metaKept(t, p) {
		t.Error("record or meta lost after a failed close")
	}
}

// dyingOnTerm builds a host whose table holds rows while alive is true and
// answers no rows after the first TERM, so a reaped process is gone.
func dyingOnTerm(alive *bool, rows ...[4]string) *prochost.Fake {
	host := &prochost.Fake{}
	host.OnTable = func(int, bool) (prochost.Table, error) {
		if !*alive {
			return prochost.Table{}, nil
		}
		return reapTable(rows...), nil
	}
	host.OnSignal = func(_ int, _ int, sig syscall.Signal) error {
		if sig == syscall.SIGTERM {
			*alive = false
		}
		return nil
	}
	return host
}

// TestRetireIgnoresAFailedReRead: a post-close read error leaves the
// pre-close tree authoritative — a process captured before the close is still
// signalled — and the retirement continues.
func TestRetireIgnoresAFailedReRead(t *testing.T) {
	reapShortWaits(t)
	alive := true
	host := dyingOnTerm(&alive,
		[4]string{"200", "1", "Ss", "/bin/zsh"},
		[4]string{"100", "200", "S+", "claude"},
		[4]string{"300", "100", "S", "bun"},
	)
	c, p, bd := retireSetup(t, host)
	c.Processes = map[rex.BlockID]rex.ProcessInfo{bd.BlockID: liveTree}
	c.failProcess[2] = errRetireBoom
	if _, err := p.retire(context.Background(), SessionName(reapDog), bd, stopPolicy()); err != nil {
		t.Fatalf("retire: %v, want the re-read failure ignored", err)
	}
	if !slices.Contains(signalLog(host), "terminated:300") {
		t.Errorf("signals = %v, want the pre-close process signalled after a failed re-read", signalLog(host))
	}
	wantRexCalls(t, c, "process", "close", "process", "view", "destroy")
	if bindingKept(t, p) {
		t.Error("binding kept although the retirement finished")
	}
}

// TestRetireReapsAProcessTheReReadFoundOnly is the code-quality finding: the
// post-close re-read adds its processes to the reap. One the pane started
// after the snapshot, absent from the pre-close tree, is still signalled.
func TestRetireReapsAProcessTheReReadFoundOnly(t *testing.T) {
	reapShortWaits(t)
	alive := true
	host := dyingOnTerm(&alive, [4]string{"900", "1", "S", "rapper"})
	c, p, bd := retireSetup(t, host)
	reads := 0
	c.ProcessFunc = func(rex.BlockID) rex.ProcessInfo {
		reads++
		if reads == 1 {
			return rex.ProcessInfo{}
		}
		return rex.ProcessInfo{Foreground: &rex.Process{PID: 900, Name: "rapper"}}
	}
	if _, err := p.retire(context.Background(), SessionName(reapDog), bd, stopPolicy()); err != nil {
		t.Fatalf("retire: %v", err)
	}
	if !slices.Contains(signalLog(host), "terminated:900") {
		t.Errorf("signals = %v, want the process only the re-read reported signalled", signalLog(host))
	}
}

// TestRetireSurvivorKeepsTheRecord is REQ-009.2: a survivor fails the
// retirement, its pids are recorded, and nothing after the reap runs.
func TestRetireSurvivorKeepsTheRecord(t *testing.T) {
	reapShortWaits(t)
	armed := false
	c, p, bd := retireSetup(t, survivingTables(&armed, nil))
	c.Processes = map[rex.BlockID]rex.ProcessInfo{bd.BlockID: liveTree}
	armed = true
	_, err := p.retire(context.Background(), SessionName(reapDog), bd, stopPolicy())
	var surv *survivorError
	if !errors.As(err, &surv) {
		t.Fatalf("retire = %v, want a survivorError", err)
	}
	wantRexCalls(t, c, "process", "close", "process")
	got, ok := mustBinding(t)(p.Binding(reapDog))
	if !ok || len(got.Survivors) != 2 {
		t.Errorf("binding = %+v, %v; want it kept with both survivors", got, ok)
	}
	if !metaKept(t, p) {
		t.Error("meta cleared although descendants survive")
	}
}

// TestRetireReapsRecordedSurvivors is REQ-009.4: a retry whose pane reads
// nothing still reaps the survivor an earlier retirement recorded, and the
// failed reap keeps the record.
func TestRetireReapsRecordedSurvivors(t *testing.T) {
	reapShortWaits(t)
	host := &prochost.Fake{OnTable: func(int, bool) (prochost.Table, error) {
		return reapTable([4]string{"300", "1", "S", "bun"}), nil
	}}
	c, p, bd := retireSetup(t, host)
	bd.Survivors = []Survivor{{PID: 300, Name: "bun", Start: reapStart}}
	if err := p.binder.Record(bd); err != nil {
		t.Fatalf("record the survivor: %v", err)
	}
	_, err := p.retire(context.Background(), SessionName(reapDog), bd, stopPolicy())
	var surv *survivorError
	if !errors.As(err, &surv) {
		t.Fatalf("retire = %v, want a survivorError", err)
	}
	if !slices.Contains(signalLog(host), "terminated:300") {
		t.Errorf("signals = %v, want the recorded survivor signalled", signalLog(host))
	}
	wantRexCalls(t, c, "process", "close", "process")
	got, ok := mustBinding(t)(p.Binding(reapDog))
	if !ok || len(got.Survivors) != 1 || got.Survivors[0].PID != 300 {
		t.Errorf("binding = %+v, %v; want it kept with the recorded survivor", got, ok)
	}
	if !metaKept(t, p) {
		t.Error("meta cleared although the recorded survivor lives")
	}
}

// TestRetireAlreadyClosedOnlyOnSuccess is REQ-007: a close Rex answers with
// ErrNotFound is not reported as alreadyClosed when a later step fails.
func TestRetireAlreadyClosedOnlyOnSuccess(t *testing.T) {
	c, p, bd := retireSetup(t, nil)
	c.CloseBlockErr = rex.ErrNotFound
	p.activity = dropFailActivity{Store: p.activity, err: errRetireBoom}
	res, err := p.retire(context.Background(), SessionName(reapDog), bd, stopPolicy())
	if !errors.Is(err, errRetireBoom) {
		t.Fatalf("retire = %v, want the clear error", err)
	}
	if res.alreadyClosed {
		t.Error("alreadyClosed = true although the retirement failed after the close")
	}
	if !bindingKept(t, p) {
		t.Error("binding forgotten after a failed retirement")
	}
}

// TestRetireLeavesAReplacementRecordUntouched is REQ-009.5: a start that
// replaced the record during the reap keeps its record, and the old pane's
// survivors are not written over it.
func TestRetireLeavesAReplacementRecordUntouched(t *testing.T) {
	reapShortWaits(t)
	armed := false
	var p *Provider
	var replacement Binding
	host := survivingTables(&armed, func(n int) {
		if n == 2 {
			if err := p.binder.Record(replacement); err != nil {
				t.Errorf("record the replacement: %v", err)
			}
		}
	})
	c, pp, bd := retireSetup(t, host)
	p = pp
	replacement = bd
	replacement.BlockID = "block:replacement"
	replacement.Survivors = nil
	c.Processes = map[rex.BlockID]rex.ProcessInfo{bd.BlockID: liveTree}
	armed = true
	_, err := p.retire(context.Background(), SessionName(reapDog), bd, stopPolicy())
	var surv *survivorError
	if !errors.As(err, &surv) {
		t.Fatalf("retire = %v, want a survivorError", err)
	}
	got, ok := mustBinding(t)(p.Binding(reapDog))
	if !ok || got.BlockID != "block:replacement" || len(got.Survivors) != 0 {
		t.Errorf("binding = %+v, %v; want the replacement unchanged", got, ok)
	}
}

// TestRetireForgetKeepsAReplacementRecord is the retire.go:179 follow-up: a
// start that replaced the binding during the lock-free retirement must not
// have its record dropped by the final Forget. The retired binding is the
// incarnation argument, so Forget drops only the record it retired.
func TestRetireForgetKeepsAReplacementRecord(t *testing.T) {
	_, p, bd := retireSetup(t, nil)
	replacement := bd
	replacement.BlockID = "block:replacement"
	replacement.WindowID = "window:replacement"
	replacement.CreatedAt = bd.CreatedAt.Add(time.Minute)
	if err := p.binder.Record(replacement); err != nil {
		t.Fatalf("record the replacement: %v", err)
	}
	_, err := p.retire(context.Background(), SessionName(reapDog), bd, retirePolicy{forget: forgetBinding})
	if err != nil {
		t.Fatalf("retire: %v", err)
	}
	got, ok := mustBinding(t)(p.Binding(reapDog))
	if !ok {
		t.Fatal("the replacement binding was dropped by the retirement's Forget")
	}
	if got.BlockID != replacement.BlockID {
		t.Errorf("binding block = %q, want the replacement %q", got.BlockID, replacement.BlockID)
	}
}

// TestRetireForgetsLastAfterAClearFailure is REQ-009.1: a clear failure
// returns before destroy and forget, so the record stays.
func TestRetireForgetsLastAfterAClearFailure(t *testing.T) {
	c, p, bd := retireSetup(t, nil)
	p.activity = dropFailActivity{Store: p.activity, err: errRetireBoom}
	_, err := p.retire(context.Background(), SessionName(reapDog), bd, stopPolicy())
	if !errors.Is(err, errRetireBoom) {
		t.Fatalf("retire = %v, want the clear error", err)
	}
	wantRexCalls(t, c, "process", "close", "process")
	if !bindingKept(t, p) {
		t.Error("binding forgotten after a failed clear")
	}
}

// TestRetireForgetsLastAfterADestroyFailure is REQ-009.1: a destroy failure
// returns before forget, so the retry still has the session id.
func TestRetireForgetsLastAfterADestroyFailure(t *testing.T) {
	c, p, bd := retireSetup(t, nil)
	c.DestroySessionErr = errRetireBoom
	_, err := p.retire(context.Background(), SessionName(reapDog), bd, stopPolicy())
	if !errors.Is(err, errRetireBoom) {
		t.Fatalf("retire = %v, want the destroy error", err)
	}
	wantRexCalls(t, c, "process", "close", "process", "view", "destroy")
	if !bindingKept(t, p) {
		t.Error("binding forgotten after a failed destroy")
	}
}

// TestRetireTakesNoStateLockAroundTheClose is REQ-010: retire itself takes
// no state lock; a caller that holds none closes without it.
func TestRetireTakesNoStateLockAroundTheClose(t *testing.T) {
	c, p, bd := retireSetup(t, nil, WithNamespace(NewNamespaceForDirs(t.TempDir())))
	heldAtClose := false
	c.onClose = func() { heldAtClose = stateLockHeld(t, p) }
	if _, err := p.retire(context.Background(), SessionName(reapDog), bd, stopPolicy()); err != nil {
		t.Fatalf("retire: %v", err)
	}
	if heldAtClose {
		t.Error("retire held the state lock across CloseBlock")
	}
}

// TestRetireDestroyBacksOffWhileALaunchHoldsTheSession is REQ-010's one
// lock exception: destroyEmptyCitySession takes the launch lock itself and
// leaves a session a launch is using.
func TestRetireDestroyBacksOffWhileALaunchHoldsTheSession(t *testing.T) {
	c, p, bd := retireSetup(t, nil, WithNamespace(NewNamespaceForDirs(t.TempDir())))
	release, held, err := flockExclusiveNow(p.launchLockPath(bd.SessionID))
	if err != nil || held {
		t.Fatalf("take the launch lock: held=%v err=%v", held, err)
	}
	defer release()
	if _, err := p.retire(context.Background(), SessionName(reapDog), bd, stopPolicy()); err != nil {
		t.Fatalf("retire: %v", err)
	}
	wantRexCalls(t, c, "process", "close", "process")
	if bindingKept(t, p) {
		t.Error("binding kept although the retirement finished")
	}
}

// TestRetireStopNoPaneKeepsAnExistingRecordWhenClearFails is REQ-009.2: the
// stored record names a survivor, so ResolveFresh kept it; a clear failure
// returns before destroy and forget, and the record stays.
func TestRetireStopNoPaneKeepsAnExistingRecordWhenClearFails(t *testing.T) {
	c, p, bd := retireSetup(t, nil)
	bd.Survivors = []Survivor{{PID: 4242, Name: "bun", Start: reapStart}}
	if err := p.binder.Record(bd); err != nil {
		t.Fatalf("Record: %v", err)
	}
	p.activity = dropFailActivity{Store: p.activity, err: errRetireBoom}
	_, err := p.retire(context.Background(), SessionName(reapDog), bd, stopNoPanePolicy(true))
	if !errors.Is(err, errRetireBoom) {
		t.Fatalf("retire = %v, want the clear error", err)
	}
	wantRexCalls(t, c)
	if !bindingKept(t, p) {
		t.Error("record lost after a failed clear")
	}
}

// TestRetireStopNoPaneDoesNotRestoreADroppedRecordWhenClearFails is
// REQ-009.2: preserving a record is not restoring one.
func TestRetireStopNoPaneDoesNotRestoreADroppedRecordWhenClearFails(t *testing.T) {
	c, p, bd := retireSetup(t, nil)
	if err := p.binder.Forget(bd.Name); err != nil {
		t.Fatalf("Forget: %v", err)
	} // ResolveFresh dropped the stale record
	p.activity = dropFailActivity{Store: p.activity, err: errRetireBoom}
	_, err := p.retire(context.Background(), SessionName(reapDog), bd, stopNoPanePolicy(true))
	if !errors.Is(err, errRetireBoom) {
		t.Fatalf("retire = %v, want the clear error", err)
	}
	wantRexCalls(t, c)
	if bindingKept(t, p) {
		t.Error("a clear failure restored a dropped record")
	}
}

// closeOnFake closes bd's block on the underlying fake without logging it,
// so the session is empty: the no-pane stop's starting state.
func closeOnFake(t *testing.T, c *retireClient, bd Binding) {
	t.Helper()
	if err := c.Fake.CloseBlock(context.Background(), bd.SessionID, bd.BlockID); err != nil {
		t.Fatalf("CloseBlock: %v", err)
	}
}

// TestRetireStopNoPaneReRecordsOnDestroyFailure is REQ-015: ResolveFresh
// dropped the record, the destroy fails, and the record is put back for the
// retry.
func TestRetireStopNoPaneReRecordsOnDestroyFailure(t *testing.T) {
	c, p, bd := retireSetup(t, nil)
	closeOnFake(t, c, bd)
	if err := p.binder.Forget(bd.Name); err != nil {
		t.Fatalf("Forget: %v", err)
	}
	c.DestroySessionErr = errRetireBoom
	_, err := p.retire(context.Background(), SessionName(reapDog), bd, stopNoPanePolicy(true))
	if !errors.Is(err, errRetireBoom) {
		t.Fatalf("retire = %v, want the destroy error", err)
	}
	wantRexCalls(t, c, "view", "destroy")
	if !bindingKept(t, p) {
		t.Error("record not restored after a failed destroy")
	}
}

// TestRetireStopNoPaneReRecordsAKeptRecordOnDestroyFailure is REQ-015: the
// gate is hadStored alone, so a record ResolveFresh kept is written again.
func TestRetireStopNoPaneReRecordsAKeptRecordOnDestroyFailure(t *testing.T) {
	c, p, bd := retireSetup(t, nil)
	closeOnFake(t, c, bd)
	bd.Survivors = []Survivor{{PID: 4242, Name: "bun", Start: reapStart}}
	if err := p.binder.Record(bd); err != nil {
		t.Fatalf("Record: %v", err)
	}
	c.DestroySessionErr = errRetireBoom
	_, err := p.retire(context.Background(), SessionName(reapDog), bd, stopNoPanePolicy(true))
	if !errors.Is(err, errRetireBoom) {
		t.Fatalf("retire = %v, want the destroy error", err)
	}
	got, ok := mustBinding(t)(p.Binding(reapDog))
	if !ok || got.BlockID != bd.BlockID || len(got.Survivors) != 1 {
		t.Errorf("binding = %+v, %v; want the stored record written again", got, ok)
	}
}

// TestRetireStopNoPaneWithoutAStoredBindingDoesNotReRecord pins REQ-015's
// hadStored gate: with no stored binding a destroy failure re-records nothing.
func TestRetireStopNoPaneWithoutAStoredBindingDoesNotReRecord(t *testing.T) {
	c, p, bd := retireSetup(t, nil)
	closeOnFake(t, c, bd)
	if err := p.binder.Forget(bd.Name); err != nil {
		t.Fatalf("Forget: %v", err)
	}
	c.DestroySessionErr = errRetireBoom
	_, err := p.retire(context.Background(), SessionName(reapDog), bd, stopNoPanePolicy(false))
	if !errors.Is(err, errRetireBoom) {
		t.Fatalf("retire = %v, want the destroy error", err)
	}
	if bindingKept(t, p) {
		t.Error("record written although no stored binding existed")
	}
}

// TestRetireStopNoPaneZeroBindingOnlyClearsAndForgets is REQ-015's
// no-stored-binding case: no Rex call, no error.
func TestRetireStopNoPaneZeroBindingOnlyClearsAndForgets(t *testing.T) {
	c, p, _ := retireSetup(t, nil)
	if _, err := p.retire(context.Background(), "Nobody--x", Binding{}, stopNoPanePolicy(false)); err != nil {
		t.Fatalf("retire: %v", err)
	}
	wantRexCalls(t, c)
	if !bindingKept(t, p) {
		t.Error("an unrelated name's retirement forgot reapDog")
	}
}

// TestRetireRetryAfterAPartialClear is the review's partial-clear retry: the
// clear step drops the meta sidecar and then fails on the activity record, so
// the first retirement stops before destroy and forget. The retry re-clears
// the already-gone meta and finishes.
func TestRetireRetryAfterAPartialClear(t *testing.T) {
	c, p, bd := retireSetup(t, nil)
	good := p.activity
	p.activity = dropFailActivity{Store: good, err: errRetireBoom}
	_, err := p.retire(context.Background(), SessionName(reapDog), bd, stopPolicy())
	if !errors.Is(err, errRetireBoom) {
		t.Fatalf("first retire = %v, want the clear error", err)
	}
	if !bindingKept(t, p) {
		t.Fatal("binding forgotten after the partial clear")
	}
	if metaKept(t, p) {
		t.Fatal("the meta sidecar survived the first clear attempt")
	}

	p.activity = good
	c.reset()
	if _, err := p.retire(context.Background(), SessionName(reapDog), bd, stopPolicy()); err != nil {
		t.Fatalf("retry: %v", err)
	}
	if bindingKept(t, p) {
		t.Error("binding kept after the retry finished")
	}
}

// TestRetireDestroyReturnWithoutDeletionIsStillASuccess is the review's other
// retry case: destroyEmptyCitySession returns nil when the session is not
// empty (a window is still there) or a launch holds it, so a nil retire does
// not prove deletion. The record still goes, and a retry is safe.
func TestRetireDestroyReturnWithoutDeletionIsStillASuccess(t *testing.T) {
	c, p, bd := retireSetup(t, nil)
	c.Views = map[rex.SessionID]rex.SessionView{
		bd.SessionID: {Windows: []rex.Window{{ID: "window:still-open"}}},
	}
	if _, err := p.retire(context.Background(), SessionName(reapDog), bd, stopPolicy()); err != nil {
		t.Fatalf("retire: %v", err)
	}
	if len(c.DestroyedSessions) != 0 {
		t.Errorf("destroyed %v, want the non-empty session left alone", c.DestroyedSessions)
	}
	if bindingKept(t, p) {
		t.Error("binding kept although the retirement finished")
	}

	c.reset()
	if _, err := p.retire(context.Background(), SessionName(reapDog), bd, stopPolicy()); err != nil {
		t.Fatalf("retry: %v", err)
	}
}

// TestRetireStalePolicyMakesNoRexCallsAndForgets is REQ-016: the absent-box
// retirement reaps the recorded survivors, then clears and forgets the
// record, and never touches Rex.
func TestRetireStalePolicyMakesNoRexCallsAndForgets(t *testing.T) {
	c, p, bd := retireSetup(t, nil)
	bd.Survivors = []Survivor{{PID: 4242, Name: "bun", Start: reapStart}}
	if err := p.binder.Record(bd); err != nil {
		t.Fatalf("Record: %v", err)
	}
	if _, err := p.retire(context.Background(), bd.Name, bd, stalePolicy()); err != nil {
		t.Fatalf("retire: %v", err)
	}
	wantRexCalls(t, c)
	if bindingKept(t, p) || metaKept(t, p) {
		t.Error("record or meta kept after a stale retirement")
	}
}

// TestRetireStalePolicyKeepsTheRecordWhileASurvivorLives is REQ-009.4:
// recorded survivors are reaped before their record goes, and one that will
// not die keeps it.
func TestRetireStalePolicyKeepsTheRecordWhileASurvivorLives(t *testing.T) {
	reapShortWaits(t)
	host := &prochost.Fake{TableField: reapTable([4]string{"4242", "1", "S", "bun"})}
	c, p, bd := retireSetup(t, host)
	bd.Survivors = []Survivor{{PID: 4242, Name: "bun", Start: reapStart}}
	if err := p.binder.Record(bd); err != nil {
		t.Fatalf("Record: %v", err)
	}
	_, err := p.retire(context.Background(), bd.Name, bd, stalePolicy())
	var surv *survivorError
	if !errors.As(err, &surv) {
		t.Fatalf("retire = %v, want a survivorError", err)
	}
	wantRexCalls(t, c)
	if !bindingKept(t, p) || !metaKept(t, p) {
		t.Error("record or meta lost while a survivor lives")
	}
}

// TestRetireDiscardLaunchPolicyReturnsTheRawSnapshotError is REQ-008: the
// discard path reports the read error unwrapped (the caller joins it with
// its cause) and closes nothing.
func TestRetireDiscardLaunchPolicyReturnsTheRawSnapshotError(t *testing.T) {
	c, p, bd := retireSetup(t, nil)
	c.failProcess[1] = errRetireBoom
	_, err := p.retire(context.Background(), bd.Name, bd, discardLaunchPolicy())
	if !errors.Is(err, errRetireBoom) || err.Error() != errRetireBoom.Error() {
		t.Fatalf("retire = %v, want the raw snapshot error", err)
	}
	wantRexCalls(t, c, "process")
}

// TestRetireDiscardLaunchPolicyOnlyClosesAndReaps is REQ-006's discard row:
// snapshot, close, re-read and reap only; the record, meta and session stay
// for start's deferred rollback.
func TestRetireDiscardLaunchPolicyOnlyClosesAndReaps(t *testing.T) {
	c, p, bd := retireSetup(t, nil)
	res, err := p.retire(context.Background(), bd.Name, bd, discardLaunchPolicy())
	if err != nil {
		t.Fatalf("retire: %v", err)
	}
	if res.alreadyClosed {
		t.Error("alreadyClosed = true after a clean close")
	}
	wantRexCalls(t, c, "process", "close", "process")
	if !bindingKept(t, p) || !metaKept(t, p) {
		t.Error("discard cleared the record or meta it does not own")
	}
	if len(c.DestroyedSessions) != 0 {
		t.Errorf("destroyed = %v, want none under the launch lock", c.DestroyedSessions)
	}
}

// TestRetireDiscardLaunchPolicyReturnsTheCloseError: a close error other
// than NotFound is returned raw for the caller to join.
func TestRetireDiscardLaunchPolicyReturnsTheCloseError(t *testing.T) {
	c, p, bd := retireSetup(t, nil)
	c.CloseBlockErr = errRetireBoom
	_, err := p.retire(context.Background(), bd.Name, bd, discardLaunchPolicy())
	if err == nil || err.Error() != errRetireBoom.Error() {
		t.Fatalf("retire = %v, want the raw close error", err)
	}
	wantRexCalls(t, c, "process", "close")
}

// lockProbeMeta runs probe before every meta Drop, the call clearStartIdentity
// makes per identity key.
type lockProbeMeta struct {
	MetaStore
	probe func()
}

func (s lockProbeMeta) Drop(k metaKey) error {
	s.probe()
	return s.MetaStore.Drop(k)
}

// TestRetireStartDeadPanePolicyClosesClearsIdentityAndForgets is the
// dead-pane example (REQ-008): close only, no BlockProcess, the identity keys
// go but the rest of the sidecar stays, and the caller's record is forgotten.
func TestRetireStartDeadPanePolicyClosesClearsIdentityAndForgets(t *testing.T) {
	c, p, bd := retireSetup(t, nil)
	key := metaKey{SessionName(reapDog), startMetaKeys()[0]}
	if err := p.meta.Write(key, "old-identity"); err != nil {
		t.Fatalf("seed identity: %v", err)
	}
	if _, err := p.retire(context.Background(), SessionName(reapDog), bd, startDeadPanePolicy()); err != nil {
		t.Fatalf("retire: %v", err)
	}
	wantRexCalls(t, c, "close")
	if _, ok, _ := p.meta.Read(key); ok {
		t.Errorf("%s kept after the dead pane's retirement", key.key)
	}
	if !metaKept(t, p) {
		t.Error("identity clear removed non-identity meta")
	}
	if bindingKept(t, p) {
		t.Error("record kept after the dead pane's retirement")
	}
}

// TestRetireStartDeadPanePolicyClearsTheAliasAndPrimaryIdentity: the
// retired identity can sit under the primary name the caller reached by
// alias, so both are cleared.
func TestRetireStartDeadPanePolicyClearsTheAliasAndPrimaryIdentity(t *testing.T) {
	_, p, bd := retireSetup(t, nil)
	alias := SessionName("Indigo--alias")
	keys := []metaKey{{alias, startMetaKeys()[0]}, {SessionName(reapDog), startMetaKeys()[0]}}
	for _, k := range keys {
		if err := p.meta.Write(k, "old-identity"); err != nil {
			t.Fatalf("seed identity: %v", err)
		}
	}
	if _, err := p.retire(context.Background(), alias, bd, startDeadPanePolicy()); err != nil {
		t.Fatalf("retire: %v", err)
	}
	for _, k := range keys {
		if _, ok, _ := p.meta.Read(k); ok {
			t.Errorf("identity under %s kept", k.name)
		}
	}
}

// TestRetireStartDeadPanePolicyToleratesAClosedBlock: Rex answering
// NotFound for the close is a pane already gone, not a failure.
func TestRetireStartDeadPanePolicyToleratesAClosedBlock(t *testing.T) {
	c, p, bd := retireSetup(t, nil)
	c.CloseBlockErr = rex.ErrNotFound
	if _, err := p.retire(context.Background(), SessionName(reapDog), bd, startDeadPanePolicy()); err != nil {
		t.Fatalf("retire: %v, want NotFound tolerated", err)
	}
	if bindingKept(t, p) {
		t.Error("record kept after the dead pane's retirement")
	}
}

// TestRetireStartDeadPanePolicyHoldsTheStateLockOnlyAroundTheIdentityClear
// is REQ-014: the identity clear runs under the state lock, the close does
// not.
func TestRetireStartDeadPanePolicyHoldsTheStateLockOnlyAroundTheIdentityClear(t *testing.T) {
	c, p, bd := retireSetup(t, nil, WithNamespace(NewNamespaceForDirs(t.TempDir())))
	heldAtClose := false
	c.onClose = func() { heldAtClose = stateLockHeld(t, p) }
	var heldAtClear []bool
	p.meta = lockProbeMeta{MetaStore: p.meta, probe: func() { heldAtClear = append(heldAtClear, stateLockHeld(t, p)) }}
	if _, err := p.retire(context.Background(), SessionName(reapDog), bd, startDeadPanePolicy()); err != nil {
		t.Fatalf("retire: %v", err)
	}
	if heldAtClose {
		t.Error("state lock held across CloseBlock")
	}
	if len(heldAtClear) == 0 {
		t.Fatal("identity clear dropped no meta key")
	}
	for i, held := range heldAtClear {
		if !held {
			t.Errorf("identity Drop %d ran without the state lock", i+1)
		}
	}
	if bindingKept(t, p) {
		t.Error("record kept after the dead pane's retirement")
	}
}
