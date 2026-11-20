package provider

import (
	"context"
	"os"
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
