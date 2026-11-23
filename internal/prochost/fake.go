package prochost

import (
	"context"
	"syscall"
)

// SignalCall is one recorded Fake.Signal call.
type SignalCall struct {
	PID int
	Sig syscall.Signal
}

// Fake is the in-memory Host the provider tests inject.
type Fake struct {
	// OnTable, when set, answers table call N (1-based) with a table or an
	// error. It receives the call's needArgs flag, so a fixture can answer the
	// reap read and the observe read differently.
	OnTable func(call int, needArgs bool) (Table, error)
	// TableField answers every table call when OnTable is nil.
	TableField Table

	// Signals records every Signal call.
	Signals []SignalCall
	// OnSignal, when set, answers signal call N (1-based). It overrides
	// SignalErr.
	OnSignal func(call int, pid int, sig syscall.Signal) error
	// SignalErr is returned for every Signal call when OnSignal is nil.
	SignalErr error

	tableCalls  int
	signalCalls int
}

// Table answers one table read.
func (f *Fake) Table(ctx context.Context, needArgs bool) (Table, error) {
	f.tableCalls++
	if f.OnTable != nil {
		return f.OnTable(f.tableCalls, needArgs)
	}
	if f.TableField == nil {
		return Table{}, nil
	}
	return f.TableField, nil
}

// Signal records one signal call and answers its configured error.
func (f *Fake) Signal(pid int, sig syscall.Signal) error {
	f.signalCalls++
	f.Signals = append(f.Signals, SignalCall{PID: pid, Sig: sig})
	if f.OnSignal != nil {
		return f.OnSignal(f.signalCalls, pid, sig)
	}
	return f.SignalErr
}
