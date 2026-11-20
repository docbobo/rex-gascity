package provider

import (
	"context"
	"errors"
	"os"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/docbobo/rex-gascity/internal/rex"
	"github.com/docbobo/rex-gascity/internal/rex/faketest"
)

const reapDog = "Indigo--dog-1"

// reapTable renders a ps table from pid/ppid/stat/comm rows.
func reapTable(rows ...[4]string) string {
	var b strings.Builder
	for _, r := range rows {
		b.WriteString(r[0] + " " + r[1] + " " + r[2] + " " + r[3] + "\n")
	}
	return b.String()
}

// reapProvider starts reapDog and returns a provider whose panes report the
// given process tree and whose host process table the givePs func answers.
func reapProvider(t *testing.T, tree rex.ProcessInfo, givePs func(call int) string) (*faketest.Fake, *Provider, *[]string) {
	t.Helper()
	ctx := context.Background()
	fake := faketest.New()
	runner := &FakeRunner{}
	psCalls := 0
	runner.Handle = func(_ context.Context, c RunCall) (string, error) {
		if c.Name != "ps" {
			return "", nil
		}
		psCalls++
		return givePs(psCalls), nil
	}
	p := New("acme", fake, WithRunner(runner))
	if err := p.Start(ctx, reapDog, Config{Argv: []string{"claude"}}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	fake.Processes = map[rex.BlockID]rex.ProcessInfo{"block:dog-1": tree}
	var signals []string
	p.signal = func(pid int, sig os.Signal) error {
		signals = append(signals, sig.String()+":"+strconv.Itoa(pid))
		return nil
	}
	return fake, p, &signals
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
	fake, p, signals := reapProvider(t, liveTree, func(call int) string {
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
	if len(*signals) != 4 {
		t.Fatalf("signals = %v, want TERM then KILL for both survivors", *signals)
	}
	if got := (*signals)[0]; got != "terminated:300" && got != "terminated:400" {
		t.Errorf("first signal = %q, want a TERM", got)
	}
	if got := (*signals)[3]; got != "killed:300" && got != "killed:400" {
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

// TestStopSucceedsWhenTheSubtreeDies proves the success path: descendants that
// die on TERM leave no survivor, so the stop succeeds and retires the
// incarnation.
func TestStopSucceedsWhenTheSubtreeDies(t *testing.T) {
	reapShortWaits(t)
	ctx := context.Background()
	termSent := false
	fake, p, signals := reapProvider(t, liveTree, func(call int) string {
		if call == 1 {
			return reapTable(
				[4]string{"200", "1", "Ss", "/bin/zsh"},
				[4]string{"100", "200", "S+", "claude"},
				[4]string{"300", "100", "S", "bun"},
			)
		}
		if termSent {
			return ""
		}
		return reapTable([4]string{"300", "1", "S", "bun"})
	})
	real := p.signal
	p.signal = func(pid int, sig os.Signal) error {
		if sig == syscall.SIGTERM {
			termSent = true
		}
		return real(pid, sig)
	}

	if err := p.Stop(ctx, reapDog); err != nil {
		t.Fatalf("Stop = %v, want nil once the subtree is gone", err)
	}
	if len(*signals) != 1 || (*signals)[0] != "terminated:300" {
		t.Errorf("signals = %v, want one TERM for 300", *signals)
	}
	if len(fake.ClosedBlocks) != 1 {
		t.Errorf("closed = %v, want one close", fake.ClosedBlocks)
	}
	if _, ok := p.Binding(reapDog); ok {
		t.Error("binding survived a fully reaped stop")
	}
}

// TestStopReapsAProcessThePaneStartedAfterTheSnapshot is review D1: a
// process the pane spawned between the pre-close snapshot and block.close is
// not in that snapshot, so stop must re-read the pane's tree after the close
// and reap what it still reports instead of reporting a false success.
func TestStopReapsAProcessThePaneStartedAfterTheSnapshot(t *testing.T) {
	reapShortWaits(t)
	ctx := context.Background()
	fake, p, signals := reapProvider(t, liveTree, func(call int) string {
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
	if !slices.Contains(*signals, "terminated:900") {
		t.Errorf("signals = %v, want a TERM for 900", *signals)
	}
	if !slices.Contains(*signals, "killed:900") {
		t.Errorf("signals = %v, want a KILL for 900", *signals)
	}
}

// TestStopAlreadyDeadSessionScansNoProcessTable is GCREX-50 AC#3: a name with
// no live pane stops with exit 0 and never touches the process table.
func TestStopAlreadyDeadSessionScansNoProcessTable(t *testing.T) {
	_, p, signals := reapProvider(t, liveTree, func(int) string {
		t.Error("process table read for a session with no pane")
		return ""
	})
	fake := p.client.(*faketest.Fake)
	// The pane is gone: no block in the view and no process to ask about.
	fake.View.Windows = nil
	delete(fake.Processes, "block:dog-1")

	if err := p.Stop(context.Background(), reapDog); err != nil {
		t.Fatalf("Stop = %v, want nil for an already-dead pane", err)
	}
	if len(*signals) != 0 {
		t.Errorf("signals = %v, want none", *signals)
	}
}

// TestProcessTableKeepsRowsWithAnEmptyOrSpacedComm is review F1: a ps row
// whose comm is empty or contains runs of spaces must still be captured, so it
// is reaped and reported instead of silently passing the stop.
func TestProcessTableKeepsRowsWithAnEmptyOrSpacedComm(t *testing.T) {
	reapShortWaits(t)
	ctx := context.Background()
	fake, p, signals := reapProvider(t, liveTree, func(call int) string {
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
	if len(*signals) != 4 {
		t.Errorf("signals = %v, want TERM then KILL for both survivors", *signals)
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
	fake, p, signals := reapProvider(t, liveTree, func(int) string { return "" })
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
	if len(*signals) != 0 {
		t.Errorf("signals = %v, want none without an observed tree", *signals)
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
	_, p, _ := reapProvider(t, liveTree, func(int) string {
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
		pids = append(pids, proc.pid)
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
	_, p, signals := reapProvider(t, liveTree, func(call int) string {
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
	p.signal = func(pid int, sig os.Signal) error {
		if pid == 300 {
			return syscall.EPERM
		}
		*signals = append(*signals, sig.String()+":"+strconv.Itoa(pid))
		return nil
	}

	err := p.Stop(ctx, reapDog)
	if err == nil {
		t.Fatal("Stop = nil, want a survivor failure")
	}
	if got, want := *signals, []string{"terminated:400", "killed:400"}; !slices.Equal(got, want) {
		t.Errorf("signals = %v, want %v despite the 300 failure", got, want)
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
	fake, p, signals := reapProvider(t, liveTree, func(int) string {
		t.Error("process table read for a vanished pane")
		return ""
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
	if len(*signals) != 0 {
		t.Errorf("signals = %v, want none", *signals)
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
	}, func(int) string {
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
	if len(tree) != 1 || tree[0].pid != 300 {
		t.Errorf("captured = %v, want only the leaf foreground 300", tree)
	}
}
