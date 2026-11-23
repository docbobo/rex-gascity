package provider_test

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"

	"github.com/docbobo/rex-gascity/internal/prochost"
	"github.com/docbobo/rex-gascity/internal/provider"
	"github.com/docbobo/rex-gascity/internal/rex"
	"github.com/docbobo/rex-gascity/internal/rex/faketest"
)

// reapStopDog is the session name the reap-through-the-seam tests start.
const reapStopDog = "Indigo--dog-1"

// TestStopRefusesRecycledPID proves that a captured pid which now carries the
// same name but a new lstart is a different process (pid reuse). Stop sends it
// no signal, returns success, and keeps no survivor record.
func TestStopRefusesRecycledPID(t *testing.T) {
	const captured = "Fri Nov 20 18:00:00 2026"
	const reused = "Sat Nov 21 09:00:00 2026"
	host := &prochost.Fake{OnTable: func(call int, _ bool) (prochost.Table, error) {
		start := captured
		if call > 1 {
			start = reused // the pid was reused after the snapshot
		}
		return prochost.Table{
			100: {PID: 100, PPID: 200, Comm: "/usr/local/bin/claude", Start: start},
			200: {PID: 200, PPID: 1, Comm: "/bin/zsh", Start: start},
		}, nil
	}}
	ctx := context.Background()
	fake := faketest.New()
	p := provider.New("acme", fake, provider.WithProcessHost(host))
	if err := p.Start(ctx, reapStopDog, provider.Config{Argv: []string{"claude"}}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	stored, ok := p.Binding(reapStopDog)
	if !ok {
		t.Fatal("no binding after Start")
	}
	fake.Processes = map[rex.BlockID]rex.ProcessInfo{stored.BlockID: {
		Foreground: &rex.Process{PID: 100, Name: "claude"},
		Ancestors:  []rex.Process{{PID: 200, Name: "zsh"}},
	}}

	if err := p.Stop(ctx, reapStopDog); err != nil {
		t.Fatalf("Stop = %v, want success: the reused pid is not the captured process", err)
	}
	if len(host.Signals) != 0 {
		t.Errorf("Stop sent %v, want no signal to a reused pid", host.Signals)
	}
	if kept, ok := p.Binding(reapStopDog); ok {
		t.Errorf("binding kept (%+v), want it cleared with no survivor record", kept)
	}
}

// reapStart is the lstart the reap-through-the-seam rows carry, so a pid's
// identity is stable across the table reads of one stop. A test that reuses a
// pid returns a different start.
const reapStart = "Fri Nov 20 18:00:00 2026"

// reapRow is one host table row for the reap tests. It is a child of init.
func reapRow(pid int, comm string) prochost.Row {
	return prochost.Row{PID: pid, PPID: 1, Comm: comm, Start: reapStart}
}

// reapTableOf builds a host table from rows.
func reapTableOf(rows ...prochost.Row) prochost.Table {
	table := make(prochost.Table, len(rows))
	for _, row := range rows {
		table[row.PID] = row
	}
	return table
}

// reapOneTree is a pane whose foreground is the only process.
func reapOneTree(pid int, name string) rex.ProcessInfo {
	return rex.ProcessInfo{Foreground: &rex.Process{PID: pid, Name: name}}
}

// reapTableAt builds a host table from [pid, ppid, state, comm] rows sharing one
// start time.
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

// reapTable is reapTableAt at the shared reapStart.
func reapTable(rows ...[4]string) prochost.Table { return reapTableAt(reapStart, rows...) }

// reapLiveTree is a pane shell (200) with the agent (100) and two descendants,
// one of them below the other (300 -> 400).
var reapLiveTree = rex.ProcessInfo{
	Foreground: &rex.Process{PID: 100, Name: "claude"},
	Ancestors:  []rex.Process{{PID: 200, Name: "zsh"}},
}

// signalLog renders the host's signals as "terminated:300" for assertions.
func signalLog(host *prochost.Fake) []string {
	log := make([]string, len(host.Signals))
	for i, call := range host.Signals {
		log[i] = call.Sig.String() + ":" + strconv.Itoa(call.PID)
	}
	return log
}

// reapStartDog starts reapStopDog over host and makes its pane report tree.
func reapStartDog(t *testing.T, host *prochost.Fake, tree rex.ProcessInfo) (*faketest.Fake, *provider.Provider) {
	t.Helper()
	fake := faketest.New()
	p := provider.New("acme", fake, provider.WithProcessHost(host))
	if err := p.Start(context.Background(), reapStopDog, provider.Config{Argv: []string{"claude"}}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	stored, ok := p.Binding(reapStopDog)
	if !ok {
		t.Fatal("no binding after Start")
	}
	fake.Processes = map[rex.BlockID]rex.ProcessInfo{stored.BlockID: tree}
	return fake, p
}

// errReapTable is the read failure the reap-through-the-seam tests inject.
var errReapTable = errors.New("host table unavailable")

// wantSurvivors asserts the exact stored survivor records, not only that a
// record exists (spec review finding 2).
func wantSurvivors(t *testing.T, p *provider.Provider, want ...provider.Survivor) {
	t.Helper()
	kept, ok := p.Binding(reapStopDog)
	if !ok {
		t.Fatal("binding gone, want the survivor record kept")
	}
	if !slices.Equal(kept.Survivors, want) {
		t.Errorf("survivors = %+v, want %+v", kept.Survivors, want)
	}
}

// TestStopReuseBeforeKill proves a pid reused between TERM and the pre-KILL
// read gets no KILL and Stop succeeds without a record (§2a row 2).
func TestStopReuseBeforeKill(t *testing.T) {
	const reused = "Sat Nov 21 09:00:00 2026"
	host := &prochost.Fake{OnTable: func(call int, _ bool) (prochost.Table, error) {
		switch {
		case call <= 3:
			return reapTableOf(reapRow(300, "bun")), nil
		case call == 4:
			// An opaque read keeps the set alive without a signal.
			return prochost.Table{300: {PID: 300, PPID: 1, Comm: "bash"}}, nil
		default:
			// The pid is back with a new start: a reused pid.
			return prochost.Table{300: {PID: 300, PPID: 1, Comm: "bun", Start: reused}}, nil
		}
	}}
	_, p := reapStartDog(t, host, reapOneTree(300, "bun"))
	if err := p.Stop(context.Background(), reapStopDog); err != nil {
		t.Fatalf("Stop = %v, want success: a reused pid is not the captured process", err)
	}
	if len(host.Signals) != 1 || host.Signals[0].Sig != syscall.SIGTERM {
		t.Errorf("signals = %v, want one TERM and no KILL", host.Signals)
	}
	if kept, ok := p.Binding(reapStopDog); ok {
		t.Errorf("binding kept (%+v), want it cleared", kept)
	}
}

// TestStopPersistentZombie proves a zombie is gone: no signal and no record
// (§2a row 3).
func TestStopPersistentZombie(t *testing.T) {
	host := &prochost.Fake{OnTable: func(int, bool) (prochost.Table, error) {
		row := reapRow(300, "bun")
		row.Zombie = true
		return prochost.Table{300: row}, nil
	}}
	_, p := reapStartDog(t, host, reapOneTree(300, "bun"))
	if err := p.Stop(context.Background(), reapStopDog); err != nil {
		t.Fatalf("Stop = %v, want success: a zombie is gone", err)
	}
	if len(host.Signals) != 0 {
		t.Errorf("signals = %v, want none for a zombie", host.Signals)
	}
	if kept, ok := p.Binding(reapStopDog); ok {
		t.Errorf("binding kept (%+v), want it cleared", kept)
	}
}

// TestStopReportsOpaqueSurvivor proves a present process whose identity is not
// provable gets no signal, fails the stop, and keeps the record (§2a row 4).
func TestStopReportsOpaqueSurvivor(t *testing.T) {
	host := &prochost.Fake{OnTable: func(call int, _ bool) (prochost.Table, error) {
		if call <= 2 {
			return reapTableOf(reapRow(300, "bun")), nil
		}
		return prochost.Table{300: {PID: 300, PPID: 1, Comm: "bash"}}, nil
	}}
	_, p := reapStartDog(t, host, reapOneTree(300, "bun"))
	err := p.Stop(context.Background(), reapStopDog)
	if err == nil {
		t.Fatal("Stop = nil, want a survivor error for an opaque identity")
	}
	if !strings.Contains(err.Error(), "300 (bun)") {
		t.Errorf("Stop error %q does not name the survivor 300 (bun)", err)
	}
	if len(host.Signals) != 0 {
		t.Errorf("signals = %v, want none for an opaque identity", host.Signals)
	}
	if _, ok := p.Binding(reapStopDog); !ok {
		t.Error("binding gone, want the survivor record kept")
	}
}

// TestStopShDoesNotMatchBash proves the exact name match: sh is not bash, so
// the row is an opaque survivor, not the captured process (§2a row 5).
func TestStopShDoesNotMatchBash(t *testing.T) {
	host := &prochost.Fake{OnTable: func(call int, _ bool) (prochost.Table, error) {
		if call <= 2 {
			return reapTableOf(reapRow(300, "sh")), nil
		}
		return prochost.Table{300: {PID: 300, PPID: 1, Comm: "bash"}}, nil
	}}
	_, p := reapStartDog(t, host, reapOneTree(300, "sh"))
	err := p.Stop(context.Background(), reapStopDog)
	if err == nil {
		t.Fatal("Stop = nil, want a survivor error: sh is not bash")
	}
	if !strings.Contains(err.Error(), "300 (sh)") {
		t.Errorf("Stop error %q does not name the survivor 300 (sh)", err)
	}
	if len(host.Signals) != 0 {
		t.Errorf("signals = %v, want none for sh versus bash", host.Signals)
	}
	if _, ok := p.Binding(reapStopDog); !ok {
		t.Error("binding gone, want the survivor record kept")
	}
}

// TestStopGoneAfterTerm proves the subtree that dies on TERM leaves no
// survivor and the stop succeeds (§2a row 6).
func TestStopGoneAfterTerm(t *testing.T) {
	host := &prochost.Fake{OnTable: func(call int, _ bool) (prochost.Table, error) {
		if call <= 3 {
			return reapTableOf(reapRow(300, "bun")), nil
		}
		return prochost.Table{}, nil
	}}
	_, p := reapStartDog(t, host, reapOneTree(300, "bun"))
	if err := p.Stop(context.Background(), reapStopDog); err != nil {
		t.Fatalf("Stop = %v, want success once the subtree is gone", err)
	}
	if len(host.Signals) != 1 || host.Signals[0].Sig != syscall.SIGTERM {
		t.Errorf("signals = %v, want one TERM", host.Signals)
	}
	if kept, ok := p.Binding(reapStopDog); ok {
		t.Errorf("binding kept (%+v), want it cleared", kept)
	}
}

// TestStopReadErrorBeforeTerm proves a table read error before TERM sends no
// signal, returns an error, and keeps the survivor record (§2a row 7).
func TestStopReadErrorBeforeTerm(t *testing.T) {
	host := &prochost.Fake{OnTable: func(call int, _ bool) (prochost.Table, error) {
		if call == 3 {
			return nil, errReapTable
		}
		return reapTableOf(reapRow(300, "bun")), nil
	}}
	_, p := reapStartDog(t, host, reapOneTree(300, "bun"))
	err := p.Stop(context.Background(), reapStopDog)
	if err == nil {
		t.Fatal("Stop = nil, want the table read error")
	}
	if !errors.Is(err, errReapTable) {
		t.Errorf("Stop error = %v, want it to wrap the table read error", err)
	}
	if len(host.Signals) != 0 {
		t.Errorf("signals = %v, want none before the read error", host.Signals)
	}
	wantSurvivors(t, p, provider.Survivor{PID: 300, Name: "bun", Start: reapStart})
}

// TestStopReadErrorDuringPoll proves a read error while waiting for TERM
// leaves the TERM in place, returns an error, and keeps the record.
func TestStopReadErrorDuringPoll(t *testing.T) {
	host := &prochost.Fake{OnTable: func(call int, _ bool) (prochost.Table, error) {
		if call == 4 {
			return nil, errReapTable
		}
		return reapTableOf(reapRow(300, "bun")), nil
	}}
	_, p := reapStartDog(t, host, reapOneTree(300, "bun"))
	err := p.Stop(context.Background(), reapStopDog)
	if err == nil {
		t.Fatal("Stop = nil, want the table read error")
	}
	if !errors.Is(err, errReapTable) {
		t.Errorf("Stop error = %v, want it to wrap the table read error", err)
	}
	if len(host.Signals) != 1 || host.Signals[0].Sig != syscall.SIGTERM {
		t.Errorf("signals = %v, want one TERM before the poll error", host.Signals)
	}
	wantSurvivors(t, p, provider.Survivor{PID: 300, Name: "bun", Start: reapStart})
}

// TestStopReadErrorBeforeKill proves a read error at the pre-KILL read leaves
// the TERM in place, sends no KILL, and keeps the record.
func TestStopReadErrorBeforeKill(t *testing.T) {
	host := &prochost.Fake{OnTable: func(call int, _ bool) (prochost.Table, error) {
		switch call {
		case 1, 2, 3:
			return reapTableOf(reapRow(300, "bun")), nil
		case 4:
			return prochost.Table{300: {PID: 300, PPID: 1, Comm: "bash"}}, nil
		case 5:
			return nil, errReapTable
		}
		return prochost.Table{}, nil
	}}
	_, p := reapStartDog(t, host, reapOneTree(300, "bun"))
	err := p.Stop(context.Background(), reapStopDog)
	if err == nil {
		t.Fatal("Stop = nil, want the table read error")
	}
	if !errors.Is(err, errReapTable) {
		t.Errorf("Stop error = %v, want it to wrap the table read error", err)
	}
	if len(host.Signals) != 1 || host.Signals[0].Sig != syscall.SIGTERM {
		t.Errorf("signals = %v, want one TERM before the pre-KILL read error", host.Signals)
	}
	wantSurvivors(t, p, provider.Survivor{PID: 300, Name: "bun", Start: reapStart})
}

// TestStopReadErrorAfterKill proves a read error after KILL returns the error
// and keeps the record.
func TestStopReadErrorAfterKill(t *testing.T) {
	host := &prochost.Fake{OnTable: func(call int, _ bool) (prochost.Table, error) {
		switch call {
		case 1, 2, 3:
			return reapTableOf(reapRow(300, "bun")), nil
		case 4:
			return prochost.Table{300: {PID: 300, PPID: 1, Comm: "bash"}}, nil
		case 5:
			return reapTableOf(reapRow(300, "bun")), nil
		case 6:
			return nil, errReapTable
		}
		return prochost.Table{}, nil
	}}
	_, p := reapStartDog(t, host, reapOneTree(300, "bun"))
	err := p.Stop(context.Background(), reapStopDog)
	if err == nil {
		t.Fatal("Stop = nil, want the table read error")
	}
	if !errors.Is(err, errReapTable) {
		t.Errorf("Stop error = %v, want it to wrap the table read error", err)
	}
	if len(host.Signals) != 2 || host.Signals[0].Sig != syscall.SIGTERM || host.Signals[1].Sig != syscall.SIGKILL {
		t.Errorf("signals = %v, want TERM then KILL before the read error", host.Signals)
	}
	wantSurvivors(t, p, provider.Survivor{PID: 300, Name: "bun", Start: reapStart})
}

// reapTwoRoots is a pane that reports two root processes: the foreground is
// absent and neither reported pid is below the other.
func reapTwoRoots(a, b rex.Process) rex.ProcessInfo {
	return rex.ProcessInfo{Ancestors: []rex.Process{a, b}}
}

// TestStopGonePIDNotKilledAfterReuse proves a capture proven gone in the TERM
// poll gets no KILL when its pid is back with the captured name but no start
// (spec review finding 1). The fresh pre-KILL read must see only the alive set.
func TestStopGonePIDNotKilledAfterReuse(t *testing.T) {
	const reused = "Sat Nov 21 09:00:00 2026"
	host := &prochost.Fake{OnTable: func(call int, _ bool) (prochost.Table, error) {
		switch call {
		case 1, 2, 3:
			return reapTableOf(reapRow(300, "bun"), reapRow(400, "sleep")), nil
		case 4:
			// 300 was reused; 400 stays present but unverified.
			return prochost.Table{
				300: {PID: 300, PPID: 1, Comm: "bun", Start: reused},
				400: {PID: 400, PPID: 1, Comm: "bash"},
			}, nil
		case 5:
			// The replacement at 300 now has no start at all.
			return prochost.Table{
				300: {PID: 300, PPID: 1, Comm: "bun"},
				400: {PID: 400, PPID: 1, Comm: "bash"},
			}, nil
		}
		return prochost.Table{400: {PID: 400, PPID: 1, Comm: "bash"}}, nil
	}}
	_, p := reapStartDog(t, host, reapTwoRoots(rex.Process{PID: 300, Name: "bun"}, rex.Process{PID: 400, Name: "sleep"}))
	err := p.Stop(context.Background(), reapStopDog)
	if err == nil || !strings.Contains(err.Error(), "400 (sleep)") {
		t.Fatalf("Stop = %v, want a survivor error naming 400 (sleep)", err)
	}
	for _, sig := range host.Signals {
		if sig.Sig == syscall.SIGKILL {
			t.Errorf("signals = %v, want no KILL for a capture proven gone", host.Signals)
		}
	}
	wantSurvivors(t, p, provider.Survivor{PID: 400, Name: "sleep", Start: reapStart})
}

// TestStopGonePIDNotKilledDuringLaterTermPoll proves a capture proven gone in
// one TERM poll is not re-admitted by a later poll of the same phase. Without
// that, a replacement with the captured name and no start would receive KILL
// (code review finding).
func TestStopGonePIDNotKilledDuringLaterTermPoll(t *testing.T) {
	const reused = "Sat Nov 21 09:00:00 2026"
	host := &prochost.Fake{OnTable: func(call int, _ bool) (prochost.Table, error) {
		switch call {
		case 1, 2, 3:
			// Pane snapshot, post-close snapshot, initial scan.
			return reapTableOf(reapRow(300, "bun"), reapRow(400, "sleep")), nil
		case 4:
			// First TERM poll: 300 was reused, 400 is still alive.
			return prochost.Table{
				300: {PID: 300, PPID: 1, Comm: "bun", Start: reused},
				400: reapRow(400, "sleep"),
			}, nil
		case 5:
			// Second TERM poll: the replacement at 300 has no start, 400 is
			// still alive.
			return prochost.Table{
				300: {PID: 300, PPID: 1, Comm: "bun"},
				400: reapRow(400, "sleep"),
			}, nil
		}
		// 400 exits; the replacement at 300 stays without a start.
		return prochost.Table{300: {PID: 300, PPID: 1, Comm: "bun"}}, nil
	}}
	_, p := reapStartDog(t, host, reapTwoRoots(rex.Process{PID: 300, Name: "bun"}, rex.Process{PID: 400, Name: "sleep"}))
	if err := p.Stop(context.Background(), reapStopDog); err != nil {
		t.Fatalf("Stop = %v, want no error once both captures are gone", err)
	}
	want := []prochost.SignalCall{{PID: 300, Sig: syscall.SIGTERM}, {PID: 400, Sig: syscall.SIGTERM}}
	if !slices.Equal(host.Signals, want) {
		t.Errorf("signals = %v, want TERM to both captures and no KILL", host.Signals)
	}
}

// TestStopKillPollErrorKeepsAliveSet proves the first read error after KILL
// keeps the latest alive set, not captures the pre-KILL read proved gone
// (spec review finding 2).
func TestStopKillPollErrorKeepsAliveSet(t *testing.T) {
	const reused = "Sat Nov 21 09:00:00 2026"
	host := &prochost.Fake{OnTable: func(call int, _ bool) (prochost.Table, error) {
		switch call {
		case 1, 2, 3:
			return reapTableOf(reapRow(300, "bun"), reapRow(400, "sleep")), nil
		case 4:
			// 300 was reused; 400 stays present but unverified.
			return prochost.Table{
				300: {PID: 300, PPID: 1, Comm: "bun", Start: reused},
				400: {PID: 400, PPID: 1, Comm: "bash"},
			}, nil
		case 5:
			// The pre-KILL read finds 400 alive and 300 back with no start.
			return prochost.Table{
				300: {PID: 300, PPID: 1, Comm: "bun"},
				400: reapRow(400, "sleep"),
			}, nil
		case 6:
			return nil, errReapTable
		}
		return prochost.Table{}, nil
	}}
	_, p := reapStartDog(t, host, reapTwoRoots(rex.Process{PID: 300, Name: "bun"}, rex.Process{PID: 400, Name: "sleep"}))
	err := p.Stop(context.Background(), reapStopDog)
	if !errors.Is(err, errReapTable) {
		t.Fatalf("Stop error = %v, want the post-KILL table read error", err)
	}
	for _, sig := range host.Signals {
		if sig.Sig == syscall.SIGKILL && sig.PID == 300 {
			t.Errorf("signals = %v, want no KILL for the gone capture 300", host.Signals)
		}
	}
	wantSurvivors(t, p, provider.Survivor{PID: 400, Name: "sleep", Start: reapStart})
}

// TestStopKillsObjectFromFakeHost proves the stop path reaches the fake host
// seam: TERM, then KILL, through WithProcessHost (REQ-006).
func TestStopKillsObjectFromFakeHost(t *testing.T) {
	host := &prochost.Fake{OnTable: func(call int, _ bool) (prochost.Table, error) {
		switch call {
		case 1, 2, 3:
			return reapTableOf(reapRow(300, "bun")), nil
		case 4:
			return prochost.Table{300: {PID: 300, PPID: 1, Comm: "bash"}}, nil
		case 5:
			return reapTableOf(reapRow(300, "bun")), nil
		}
		return prochost.Table{}, nil
	}}
	fake, p := reapStartDog(t, host, reapOneTree(300, "bun"))
	if err := p.Stop(context.Background(), reapStopDog); err != nil {
		t.Fatalf("Stop = %v, want nil once the object is reaped", err)
	}
	if len(host.Signals) != 2 || host.Signals[0].Sig != syscall.SIGTERM || host.Signals[1].Sig != syscall.SIGKILL {
		t.Errorf("signals = %v, want TERM then KILL from the fake host", host.Signals)
	}
	if len(fake.ClosedBlocks) != 1 {
		t.Errorf("closed = %v, want one close", fake.ClosedBlocks)
	}
	if _, ok := p.Binding(reapStopDog); ok {
		t.Error("binding survived a fully reaped stop")
	}
}

// TestStopSucceedsWhenTheSubtreeDies proves the success path: descendants that
// die on TERM leave no survivor, so the stop succeeds and retires the
// incarnation.
func TestStopSucceedsWhenTheSubtreeDies(t *testing.T) {
	termSent := false
	host := &prochost.Fake{OnTable: func(call int, _ bool) (prochost.Table, error) {
		if call == 1 {
			// Before the close: the pane shell, the foreground agent, and
			// the agent's child.
			return reapTableOf(
				reapRow(200, "/bin/zsh"),
				prochost.Row{PID: 100, PPID: 200, Comm: "claude", Start: reapStart},
				prochost.Row{PID: 300, PPID: 100, Comm: "bun", Start: reapStart},
			), nil
		}
		if termSent {
			return prochost.Table{}, nil
		}
		// After the close: the child is reparented to init.
		return reapTableOf(reapRow(300, "bun")), nil
	}}
	host.OnSignal = func(_ int, _ int, sig syscall.Signal) error {
		if sig == syscall.SIGTERM {
			termSent = true
		}
		return nil
	}
	tree := rex.ProcessInfo{
		Foreground: &rex.Process{PID: 100, Name: "claude"},
		Ancestors:  []rex.Process{{PID: 200, Name: "zsh"}},
	}
	fake, p := reapStartDog(t, host, tree)

	if err := p.Stop(context.Background(), reapStopDog); err != nil {
		t.Fatalf("Stop = %v, want nil once the subtree is gone", err)
	}
	if len(host.Signals) != 1 || host.Signals[0].PID != 300 || host.Signals[0].Sig != syscall.SIGTERM {
		t.Errorf("signals = %v, want one TERM for 300", host.Signals)
	}
	if len(fake.ClosedBlocks) != 1 {
		t.Errorf("closed = %v, want one close", fake.ClosedBlocks)
	}
	if _, ok := p.Binding(reapStopDog); ok {
		t.Error("binding survived a fully reaped stop")
	}
}

// TestStopAlreadyDeadSessionScansNoProcessTable is GCREX-50 AC#3: a name with
// no live pane stops with exit 0 and never touches the process table.
func TestStopAlreadyDeadSessionScansNoProcessTable(t *testing.T) {
	host := &prochost.Fake{OnTable: func(int, bool) (prochost.Table, error) {
		t.Error("process table read for a session with no pane")
		return prochost.Table{}, nil
	}}
	fake, p := reapStartDog(t, host, reapOneTree(300, "bun"))
	// The pane is gone: no block in the view and no process to ask about.
	fake.View.Windows = nil
	fake.Processes = nil

	if err := p.Stop(context.Background(), reapStopDog); err != nil {
		t.Fatalf("Stop = %v, want nil for an already-dead pane", err)
	}
	if len(host.Signals) != 0 {
		t.Errorf("signals = %v, want none", host.Signals)
	}
}

// TestProcessTableKeepsRowsWithAnEmptyOrSpacedComm is review F1: a ps row
// whose comm is empty or contains runs of spaces must still be captured, so it
// is reaped and reported instead of silently passing the stop.
func TestProcessTableKeepsRowsWithAnEmptyOrSpacedComm(t *testing.T) {
	ctx := context.Background()
	host := &prochost.Fake{OnTable: func(call int, _ bool) (prochost.Table, error) {
		if call == 1 {
			// Before the close: an empty comm and a comm with repeated spaces.
			return reapTable(
				[4]string{"200", "1", "Ss", "/bin/zsh"},
				[4]string{"100", "200", "S+", "claude"},
				[4]string{"300", "100", "S", ""},
				[4]string{"400", "100", "S", "Google  Chrome  Helper"},
			), nil
		}
		return reapTable(
			[4]string{"300", "1", "S", ""},
			[4]string{"400", "1", "S", "Google  Chrome  Helper"},
		), nil
	}}
	fake, p := reapStartDog(t, host, reapLiveTree)
	err := p.Stop(ctx, reapStopDog)
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
	host := &prochost.Fake{OnTable: func(int, bool) (prochost.Table, error) {
		return prochost.Table{}, nil
	}}
	fake, p := reapStartDog(t, host, rex.ProcessInfo{})
	fake.ProcessErr = errors.New("process: unavailable")
	ctx := context.Background()
	if err := p.SetMeta(ctx, reapStopDog, "suspended", "yes"); err != nil {
		t.Fatalf("SetMeta: %v", err)
	}

	err := p.Stop(ctx, reapStopDog)
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
	if len(host.Signals) != 0 {
		t.Errorf("signals = %v, want none without an observed tree", host.Signals)
	}
	if _, ok := p.Binding(reapStopDog); !ok {
		t.Error("binding forgotten although the pane was never closed")
	}
	if got, _ := p.GetMeta(ctx, reapStopDog, "suspended"); got != "yes" {
		t.Errorf("meta suspended = %q, want it kept for the retry", got)
	}
}
