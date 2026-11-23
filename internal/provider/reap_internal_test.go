package provider

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/docbobo/rex-gascity/internal/prochost"
	"github.com/docbobo/rex-gascity/internal/rex"
	"github.com/docbobo/rex-gascity/internal/rex/faketest"
)

const reapDog = "Indigo--dog-1"

// reapStart is the lstart every reapTable row carries, so the pid identity is
// stable across the two process-table reads in a test. A test that needs a
// different start time (pid reuse) uses reapTableAt.
const reapStart = "Fri Nov 20 18:00:00 2026"

func reapTable(rows ...[4]string) prochost.Table { return reapTableAt(reapStart, rows...) }

func reapTableAt(start string, rows ...[4]string) prochost.Table {
	table := make(prochost.Table, len(rows))
	for _, r := range rows {
		pid, _ := strconv.Atoi(r[0])
		ppid, _ := strconv.Atoi(r[1])
		table[pid] = prochost.Row{
			PID:      pid,
			PPID:     ppid,
			Zombie:   strings.HasPrefix(r[2], "Z"),
			Comm:     r[3],
			Start:    start,
			Reliable: true,
		}
	}
	return table
}

// signalLog renders the host's signals as "terminated:300" for assertions.
func signalLog(host *prochost.Fake) []string {
	log := make([]string, len(host.Signals))
	for i, call := range host.Signals {
		log[i] = call.Sig.String() + ":" + strconv.Itoa(call.PID)
	}
	return log
}

// reapProvider starts reapDog and returns a provider whose panes report the
// given process tree and whose host process table the givePs func answers.
func reapProvider(t *testing.T, tree rex.ProcessInfo, givePs func(call int) prochost.Table) (*faketest.Fake, *Provider, *prochost.Fake) {
	t.Helper()
	ctx := context.Background()
	fake := faketest.New()
	host := &prochost.Fake{OnTable: func(call int, _ bool) (prochost.Table, error) {
		return givePs(call), nil
	}}
	p := New("acme", fake, WithProcessHost(host))
	if err := p.Start(ctx, reapDog, Config{Argv: []string{"claude"}}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	fake.Processes = map[rex.BlockID]rex.ProcessInfo{"block:dog-1": tree}
	return fake, p, host
}

// reapShortWaits shortens the reap waits so a test never waits the
// production 2 s.
func reapShortWaits(t *testing.T) {
	t.Helper()
	term, kill, poll := reapTermWait, reapKillWait, reapPoll
	reapTermWait, reapKillWait, reapPoll = 30*time.Millisecond, 30*time.Millisecond, 5*time.Millisecond
	t.Cleanup(func() { reapTermWait, reapKillWait, reapPoll = term, kill, poll })
}

// liveTree is a pane shell (200) with the agent (100) and two descendants,
// one of them below the other (300 -> 400).
var liveTree = rex.ProcessInfo{
	Foreground: &rex.Process{PID: 100, Name: "claude"},
	Ancestors:  []rex.Process{{PID: 200, Name: "zsh"}},
}

// TestStopFailsWhileDescendantsSurvive is GCREX-50 AC#1+2: after block.close
// the pane's subtree is re-checked, survivors are reaped TERM then KILL, and a
// subtree that still lives fails the stop with the survivors named while the
// binding, meta and activity are kept for a retry.
func TestStopFailsWhileDescendantsSurvive(t *testing.T) {
	reapShortWaits(t)
	ctx := context.Background()
	fake, p, host := reapProvider(t, liveTree, func(call int) prochost.Table {
		if call == 1 {
			// Before the close: pane shell, agent and two descendants.
			return reapTable(
				[4]string{"200", "1", "Ss", "/bin/zsh"},
				[4]string{"100", "200", "S+", "claude"},
				[4]string{"300", "100", "S", "bun"},
				[4]string{"400", "300", "S", "node"},
			)
		}
		// After the close: the pane's own processes are gone, the detached
		// descendants survive both TERM and KILL.
		return reapTable(
			[4]string{"300", "1", "S", "bun"},
			[4]string{"400", "1", "S", "node"},
		)
	})
	if err := p.SetMeta(ctx, reapDog, "suspended", "yes"); err != nil {
		t.Fatalf("SetMeta: %v", err)
	}
	fake.Formats = map[rex.BlockID]string{"block:dog-1": "screen"}
	if _, err := p.GetLastActivity(ctx, reapDog); err != nil {
		t.Fatalf("GetLastActivity: %v", err)
	}

	err := p.Stop(ctx, reapDog)
	if err == nil {
		t.Fatal("Stop = nil, want a survivor failure")
	}
	for _, want := range []string{"descendants may survive", "300 (bun)", "400 (node)"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("Stop error %q does not name %q", err, want)
		}
	}
	if len(signalLog(host)) != 4 {
		t.Fatalf("signals = %v, want TERM then KILL for both survivors", signalLog(host))
	}
	if got := signalLog(host)[0]; got != "terminated:300" && got != "terminated:400" {
		t.Errorf("first signal = %q, want a TERM", got)
	}
	if got := signalLog(host)[3]; got != "killed:300" && got != "killed:400" {
		t.Errorf("last signal = %q, want a KILL", got)
	}
	if len(fake.ClosedBlocks) != 1 {
		t.Errorf("closed = %v, want one close", fake.ClosedBlocks)
	}
	if _, ok := p.Binding(reapDog); !ok {
		t.Error("binding forgotten although descendants may survive")
	}
	if got, _ := p.GetMeta(ctx, reapDog, "suspended"); got != "yes" {
		t.Errorf("meta suspended = %q, want it kept for the retry", got)
	}
	if _, ok, err := p.activity.Read(SessionName(reapDog)); !ok || err != nil {
		t.Errorf("activity = %v, %v, want it kept for the retry", ok, err)
	}
}

// TestStopRetriesTheSurvivorsItRecorded proves the retry half of GCREX-50: the
// failed stop keeps the pids on the binding, and the retry reaps them although
// the closed pane (here: gone from the view) no longer names any process. The
// retry must not clear the record while the survivors run.
func TestStopRetriesTheSurvivorsItRecorded(t *testing.T) {
	reapShortWaits(t)
	ctx := context.Background()
	alive := true
	fake, p, host := reapProvider(t, liveTree, func(call int) prochost.Table {
		if call == 1 {
			return reapTable(
				[4]string{"200", "1", "Ss", "/bin/zsh"},
				[4]string{"100", "200", "S+", "claude"},
				[4]string{"300", "100", "S", "bun"},
				[4]string{"400", "300", "S", "node"},
			)
		}
		if !alive {
			return prochost.Table{}
		}
		return reapTable(
			[4]string{"300", "1", "S", "bun"},
			[4]string{"400", "1", "S", "node"},
		)
	})
	host.OnSignal = func(_ int, pid int, sig syscall.Signal) error {
		if sig == syscall.SIGTERM && len(host.Signals) > 2 {
			alive = false // the retry's TERM kills what the first round could not
		}
		return nil
	}

	if err := p.Stop(ctx, reapDog); err == nil {
		t.Fatal("Stop = nil, want a survivor failure")
	}
	bd, ok := p.Binding(reapDog)
	if !ok {
		t.Fatal("binding gone after a survivor failure, want it kept for the retry")
	}
	pids := make([]int, 0, len(bd.Survivors))
	for _, s := range bd.Survivors {
		pids = append(pids, s.PID)
	}
	slices.Sort(pids)
	if !slices.Equal(pids, []int{300, 400}) {
		t.Fatalf("recorded survivors = %v, want pids 300 and 400", bd.Survivors)
	}

	// The close removed the pane, so only the record still names the
	// survivors.
	fake.View = rex.SessionView{}
	fake.Processes = nil
	if err := p.Stop(ctx, reapDog); err != nil {
		t.Fatalf("retry Stop = %v, want nil once the recorded survivors are reaped", err)
	}
	if len(signalLog(host)) != 6 {
		t.Errorf("signals = %v, want the first round's 4 and the retry's TERM on both survivors", signalLog(host))
	}
	if _, ok := p.Binding(reapDog); ok {
		t.Error("binding survived the successful retry")
	}
}

// TestStopReapsAProcessThePaneStartedAfterTheSnapshot is review D1: a
// process the pane spawned between the pre-close snapshot and block.close is
// not in that snapshot, so stop must re-read the pane's tree after the close
// and reap what it still reports instead of reporting a false success.
func TestStopReapsAProcessThePaneStartedAfterTheSnapshot(t *testing.T) {
	reapShortWaits(t)
	ctx := context.Background()
	fake, p, host := reapProvider(t, liveTree, func(call int) prochost.Table {
		if call == 1 {
			// Before the close: the snapshot's shell, agent and descendant.
			return reapTable(
				[4]string{"200", "1", "Ss", "/bin/zsh"},
				[4]string{"100", "200", "S+", "claude"},
				[4]string{"300", "100", "S", "bun"},
			)
		}
		// After the close: the snapshot's processes are gone; 900 outlives
		// both TERM and KILL.
		return reapTable([4]string{"900", "1", "S", "rapper"})
	})
	reads := 0
	fake.ProcessFunc = func(rex.BlockID) rex.ProcessInfo {
		reads++
		if reads == 1 {
			return liveTree
		}
		return rex.ProcessInfo{Foreground: &rex.Process{PID: 900, Name: "rapper"}}
	}

	err := p.Stop(ctx, reapDog)
	if err == nil {
		t.Fatal("Stop = nil, want a survivor failure for the process started after the snapshot")
	}
	if !strings.Contains(err.Error(), "900 (rapper)") {
		t.Errorf("Stop error %q does not name the survivor 900 (rapper)", err)
	}
	if !slices.Contains(signalLog(host), "terminated:900") {
		t.Errorf("signals = %v, want a TERM for 900", signalLog(host))
	}
	if !slices.Contains(signalLog(host), "killed:900") {
		t.Errorf("signals = %v, want a KILL for 900", signalLog(host))
	}
}

// TestStopMatchesTheProcessTableNameNotRexsName is review U1, recorded from a
// live run (macOS 25.6, BSD ps): Rex reports the executable's name (`node`),
// while `ps -o comm=` prints argv0 (`pi`) because a Rex pane runs its command
// as the default shell with argv0 set to it. A captured name taken from Rex
// never matches that live row, so living drops the survivor as gone and the
// stop reports a false success. The snapshot takes each name from the same ps
// table living compares against.
func TestStopMatchesTheProcessTableNameNotRexsName(t *testing.T) {
	reapShortWaits(t)
	ctx := context.Background()
	_, p, host := reapProvider(t, rex.ProcessInfo{
		Foreground: &rex.Process{PID: 300, Name: "node"},
		Ancestors:  []rex.Process{{PID: 200, Name: "fish"}},
	}, func(call int) prochost.Table {
		if call == 1 {
			return reapTable(
				[4]string{"200", "1", "Ss", "-fish"},
				[4]string{"300", "200", "+", "pi"},
			)
		}
		return reapTable([4]string{"300", "1", "S", "pi"})
	})
	err := p.Stop(ctx, reapDog)
	if err == nil {
		t.Fatal("Stop = nil, want a survivor failure for the live 300 that Rex calls \"node\" and ps calls \"pi\"")
	}
	if !strings.Contains(err.Error(), "300 (pi)") {
		t.Errorf("Stop error %q does not name the live survivor 300 (pi)", err)
	}
	if !slices.Contains(signalLog(host), "terminated:300") {
		t.Errorf("signals = %v, want a TERM for 300", signalLog(host))
	}
}

// TestStopKeepsAProcessThatRenamedItself is review round 4: a captured pid
// that is still running must not be dropped just because it exec'd into a
// different argv0/comm. Its lstart, captured in the same snapshot, proves it is
// the same process, so it is reaped instead of reported gone.
func TestStopKeepsAProcessThatRenamedItself(t *testing.T) {
	reapShortWaits(t)
	ctx := context.Background()
	_, p, host := reapProvider(t, liveTree, func(call int) prochost.Table {
		if call == 1 {
			return reapTable(
				[4]string{"200", "1", "Ss", "/bin/zsh"},
				[4]string{"100", "200", "S+", "claude"},
				[4]string{"300", "100", "S", "sh"},
			)
		}
		// Same pid and start time, new comm: the shell exec'd into the agent.
		return reapTable(
			[4]string{"200", "1", "Ss", "/bin/zsh"},
			[4]string{"100", "200", "S+", "claude"},
			[4]string{"300", "1", "S", "bun"},
		)
	})
	err := p.Stop(ctx, reapDog)
	if err == nil {
		t.Fatal("Stop = nil, want a survivor failure for the renamed 300")
	}
	if !strings.Contains(err.Error(), "300 (sh)") {
		t.Errorf("Stop error %q does not name the renamed survivor 300 (sh)", err)
	}
	if !slices.Contains(signalLog(host), "terminated:300") {
		t.Errorf("signals = %v, want a TERM for 300", signalLog(host))
	}
}

// TestStopDoesNotSignalAReusedPid is the other half of review round 4: when
// the pid still exists but its lstart differs, the host reused it, so it is
// not the captured process and must never be signalled or reported.
func TestStopDoesNotSignalAReusedPid(t *testing.T) {
	reapShortWaits(t)
	ctx := context.Background()
	_, p, host := reapProvider(t, liveTree, func(call int) prochost.Table {
		if call == 1 {
			return reapTable(
				[4]string{"200", "1", "Ss", "/bin/zsh"},
				[4]string{"100", "200", "S+", "claude"},
				[4]string{"300", "100", "S", "bun"},
			)
		}
		// 300 is back with the same name but a later start: a reused pid.
		return reapTableAt("Fri Nov 20 19:00:00 2026",
			[4]string{"300", "1", "S", "bun"},
		)
	})
	if err := p.Stop(ctx, reapDog); err != nil {
		t.Fatalf("Stop = %v, want nil: a reused pid is not the captured process", err)
	}
	if len(signalLog(host)) != 0 {
		t.Errorf("signals = %v, want none for a reused pid", signalLog(host))
	}
}

// TestUnionProcsKeepsBothStartsOfAReusedPid is review round 4: the second pane
// read may name a pid the first read saw with another lstart (the pid was
// reused mid-teardown). Both are real, so neither may be dropped.
func TestUnionProcsKeepsBothStartsOfAReusedPid(t *testing.T) {
	tree := []prochost.Ident{{PID: 300, Name: "sh", Start: "A"}}
	union := unionProcs(tree, []prochost.Ident{
		{PID: 300, Name: "bun", Start: "A"}, // same process and start: deduped
		{PID: 300, Name: "bun", Start: "B"}, // reused pid: a second process
		{PID: 400, Name: "sh", Start: "A"},  // unseen pid
	})
	if len(union) != 3 {
		t.Fatalf("union = %v, want 3 entries", union)
	}
}

// TestProcessTableKeepsRowsWithAnEmptyOrSpacedComm is review F1: a ps row
// whose comm is empty or contains runs of spaces must still be captured, so it
// is reaped and reported instead of silently passing the stop.
func TestProcessTableKeepsRowsWithAnEmptyOrSpacedComm(t *testing.T) {
	reapShortWaits(t)
	ctx := context.Background()
	fake, p, host := reapProvider(t, liveTree, func(call int) prochost.Table {
		if call == 1 {
			// Before the close: an empty comm and a comm with repeated spaces.
			return reapTable(
				[4]string{"200", "1", "Ss", "/bin/zsh"},
				[4]string{"100", "200", "S+", "claude"},
				[4]string{"300", "100", "S", ""},
				[4]string{"400", "100", "S", "Google  Chrome  Helper"},
			)
		}
		return reapTable(
			[4]string{"300", "1", "S", ""},
			[4]string{"400", "1", "S", "Google  Chrome  Helper"},
		)
	})
	err := p.Stop(ctx, reapDog)
	if err == nil {
		t.Fatal("Stop = nil, want a survivor failure for the dropped rows")
	}
	for _, want := range []string{"300 (?)", "400 (Google  Chrome  Helper)"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("Stop error %q does not name %q", err, want)
		}
	}
	// The empty comm is an opaque identity: it is reported as a survivor but not
	// signalled. The named row 400 is signalled TERM then KILL.
	if len(signalLog(host)) != 2 {
		t.Errorf("signals = %v, want TERM then KILL for the named survivor 400", signalLog(host))
	}
	if len(fake.ClosedBlocks) != 1 {
		t.Errorf("closed = %v, want one close", fake.ClosedBlocks)
	}
}

// TestStopLeavesThePaneOpenWhenTheTreeCannotBeRead is review F5: a tree that
// cannot be inspected must fail before block.close, never close the pane and
// then blame descendants that were never observed.
func TestStopLeavesThePaneOpenWhenTheTreeCannotBeRead(t *testing.T) {
	reapShortWaits(t)
	ctx := context.Background()
	fake, p, host := reapProvider(t, liveTree, func(int) prochost.Table { return prochost.Table{} })
	fake.ProcessErr = errors.New("process: unavailable")
	if err := p.SetMeta(ctx, reapDog, "suspended", "yes"); err != nil {
		t.Fatalf("SetMeta: %v", err)
	}

	err := p.Stop(ctx, reapDog)
	if err == nil {
		t.Fatal("Stop = nil, want the inspection failure")
	}
	if !strings.Contains(err.Error(), "could not inspect the pane's process tree") {
		t.Errorf("Stop error %q does not say the tree could not be inspected", err)
	}
	if !strings.Contains(err.Error(), "process: unavailable") {
		t.Errorf("Stop error %q does not carry the cause", err)
	}
	if len(fake.ClosedBlocks) != 0 {
		t.Errorf("closed = %v, want the pane left open", fake.ClosedBlocks)
	}
	if len(signalLog(host)) != 0 {
		t.Errorf("signals = %v, want none without an observed tree", signalLog(host))
	}
	if _, ok := p.Binding(reapDog); !ok {
		t.Error("binding forgotten although the pane was never closed")
	}
	if got, _ := p.GetMeta(ctx, reapDog, "suspended"); got != "yes" {
		t.Errorf("meta suspended = %q, want it kept for the retry", got)
	}
}

// TestPaneSubtreeSkipsAncestorsSiblings is review F4: a sibling under the
// pane's shell (a reported ancestor) is not a pane process and must not be
// captured or signalled.
func TestPaneSubtreeSkipsAncestorsSiblings(t *testing.T) {
	ctx := context.Background()
	_, p, _ := reapProvider(t, liveTree, func(int) prochost.Table {
		return reapTable(
			[4]string{"200", "1", "Ss", "/bin/zsh"},
			[4]string{"100", "200", "S+", "claude"},
			[4]string{"300", "100", "S", "bun"},
			[4]string{"400", "300", "S", "node"},
			[4]string{"500", "200", "S", "ssh-agent"},
		)
	})
	binding, ok, err := p.binder.Resolve(ctx, ResolveRequest{Name: SessionName(reapDog), Mode: ResolveFresh})
	if err != nil || !ok {
		t.Fatalf("Resolve = %v, %v", ok, err)
	}
	tree, err := p.paneSubtree(ctx, binding)
	if err != nil {
		t.Fatalf("paneSubtree: %v", err)
	}
	var pids []int
	for _, proc := range tree {
		pids = append(pids, proc.PID)
	}
	if want := []int{100, 300, 400}; !slices.Equal(pids, want) {
		t.Errorf("captured pids = %v, want %v (no ancestor 200, no sibling 500)", pids, want)
	}
}

// TestStopKeepsSignallingAfterASignalFailure is review R1: a process that
// cannot be signalled must not shield the rest of the tree from TERM and KILL,
// and the survivor error still names the first failure.
func TestStopKeepsSignallingAfterASignalFailure(t *testing.T) {
	reapShortWaits(t)
	ctx := context.Background()
	_, p, host := reapProvider(t, liveTree, func(call int) prochost.Table {
		if call == 1 {
			return reapTable(
				[4]string{"200", "1", "Ss", "/bin/zsh"},
				[4]string{"100", "200", "S+", "claude"},
				[4]string{"300", "100", "S", "bun"},
				[4]string{"400", "300", "S", "node"},
			)
		}
		return reapTable(
			[4]string{"300", "1", "S", "bun"},
			[4]string{"400", "1", "S", "node"},
		)
	})
	// 300 is the lower survivor pid, so it is signalled first; it is the one
	// that fails, like an EPERM from a process that changed uid.
	host.OnSignal = func(_ int, pid int, _ syscall.Signal) error {
		if pid == 300 {
			return syscall.EPERM
		}
		return nil
	}

	err := p.Stop(ctx, reapDog)
	if err == nil {
		t.Fatal("Stop = nil, want a survivor failure")
	}
	for _, want := range []string{"terminated:400", "killed:400"} {
		if !slices.Contains(signalLog(host), want) {
			t.Errorf("signals = %v, want %q despite the 300 failure", signalLog(host), want)
		}
	}
	for _, want := range []string{"300 (bun)", "400 (node)", "operation not permitted"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("Stop error %q does not carry %q", err, want)
		}
	}
}

// TestStopTreatsAVanishedPaneAsAnEmptyTree is review R2: a pane that vanishes
// between Resolve and BlockProcess is the no-pane case, not a tree we could not
// read, so the stop proceeds to the close and succeeds.
func TestStopTreatsAVanishedPaneAsAnEmptyTree(t *testing.T) {
	reapShortWaits(t)
	ctx := context.Background()
	fake, p, host := reapProvider(t, liveTree, func(int) prochost.Table {
		t.Error("process table read for a vanished pane")
		return prochost.Table{}
	})

	binding, ok, err := p.binder.Resolve(ctx, ResolveRequest{Name: SessionName(reapDog), Mode: ResolveFresh})
	if err != nil || !ok {
		t.Fatalf("Resolve = %v, %v", ok, err)
	}
	fake.ProcessErr = rex.ErrNotFound
	tree, err := p.paneSubtree(ctx, binding)
	if err != nil || len(tree) != 0 {
		t.Fatalf("paneSubtree = %v, %v, want an empty tree", tree, err)
	}
	if err := p.Stop(ctx, reapDog); err != nil {
		t.Fatalf("Stop = %v, want nil for a vanished pane", err)
	}
	if len(signalLog(host)) != 0 {
		t.Errorf("signals = %v, want none", signalLog(host))
	}
	if _, ok := p.Binding(reapDog); ok {
		t.Error("binding survived a vanished pane's stop")
	}
}

// TestPaneSubtreePinsAForegroundLeaf is review R3: a foreground that is a leaf
// narrows the snapshot to that one process; the agent's other children are
// siblings of it, not below it, and stay out.
func TestPaneSubtreePinsAForegroundLeaf(t *testing.T) {
	ctx := context.Background()
	_, p, _ := reapProvider(t, rex.ProcessInfo{
		Foreground: &rex.Process{PID: 300, Name: "go"},
		Ancestors:  []rex.Process{{PID: 200, Name: "zsh"}, {PID: 100, Name: "claude"}},
	}, func(int) prochost.Table {
		return reapTable(
			[4]string{"200", "1", "Ss", "/bin/zsh"},
			[4]string{"100", "200", "S+", "claude"},
			[4]string{"300", "100", "S", "go"},
			[4]string{"400", "100", "S", "bun"},
		)
	})
	binding, ok, err := p.binder.Resolve(ctx, ResolveRequest{Name: SessionName(reapDog), Mode: ResolveFresh})
	if err != nil || !ok {
		t.Fatalf("Resolve = %v, %v", ok, err)
	}
	tree, err := p.paneSubtree(ctx, binding)
	if err != nil {
		t.Fatalf("paneSubtree: %v", err)
	}
	if len(tree) != 1 || tree[0].PID != 300 {
		t.Errorf("captured = %v, want only the leaf foreground 300", tree)
	}
}

// TestNewBuildsThePSHostOnce proves New stores the real ps adapter after the
// options run, and WithProcessHost still overrides it (spec review finding 3).
func TestNewBuildsThePSHostOnce(t *testing.T) {
	p := New("acme", faketest.New())
	if _, ok := p.processHost.(*prochost.PSHost); !ok {
		t.Fatalf("default processHost = %T, want *prochost.PSHost", p.processHost)
	}
	host := &prochost.Fake{}
	if got := New("acme", faketest.New(), WithProcessHost(host)); got.processHost != prochost.Host(host) {
		t.Errorf("override processHost = %T, want the injected fake", got.processHost)
	}
}
