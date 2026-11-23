package provider_test

import (
	"context"
	"errors"
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
	host := &prochost.Fake{OnTable: func(call int) (prochost.Table, error) {
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

// TestStopReuseBeforeKill proves a pid reused between TERM and the pre-KILL
// read gets no KILL and Stop succeeds without a record (§2a row 2).
func TestStopReuseBeforeKill(t *testing.T) {
	const reused = "Sat Nov 21 09:00:00 2026"
	host := &prochost.Fake{OnTable: func(call int) (prochost.Table, error) {
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
	host := &prochost.Fake{OnTable: func(int) (prochost.Table, error) {
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
	host := &prochost.Fake{OnTable: func(call int) (prochost.Table, error) {
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
	host := &prochost.Fake{OnTable: func(call int) (prochost.Table, error) {
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
	host := &prochost.Fake{OnTable: func(call int) (prochost.Table, error) {
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
	host := &prochost.Fake{OnTable: func(call int) (prochost.Table, error) {
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
	if _, ok := p.Binding(reapStopDog); !ok {
		t.Error("binding gone, want the survivor record kept")
	}
}

// TestStopReadErrorDuringPoll proves a read error while waiting for TERM
// leaves the TERM in place, returns an error, and keeps the record.
func TestStopReadErrorDuringPoll(t *testing.T) {
	host := &prochost.Fake{OnTable: func(call int) (prochost.Table, error) {
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
	if _, ok := p.Binding(reapStopDog); !ok {
		t.Error("binding gone, want the survivor record kept")
	}
}

// TestStopReadErrorBeforeKill proves a read error at the pre-KILL read leaves
// the TERM in place, sends no KILL, and keeps the record.
func TestStopReadErrorBeforeKill(t *testing.T) {
	host := &prochost.Fake{OnTable: func(call int) (prochost.Table, error) {
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
	if _, ok := p.Binding(reapStopDog); !ok {
		t.Error("binding gone, want the survivor record kept")
	}
}

// TestStopReadErrorAfterKill proves a read error after KILL returns the error
// and keeps the record.
func TestStopReadErrorAfterKill(t *testing.T) {
	host := &prochost.Fake{OnTable: func(call int) (prochost.Table, error) {
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
	if _, ok := p.Binding(reapStopDog); !ok {
		t.Error("binding gone, want the survivor record kept")
	}
}

// TestStopKillsObjectFromFakeHost proves the stop path reaches the fake host
// seam: TERM, then KILL, through WithProcessHost (REQ-006).
func TestStopKillsObjectFromFakeHost(t *testing.T) {
	host := &prochost.Fake{OnTable: func(call int) (prochost.Table, error) {
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
