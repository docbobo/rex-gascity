package prochost

import (
	"context"
	"errors"
	"reflect"
	"syscall"
	"testing"
)

func TestFakeRecordsSignals(t *testing.T) {
	f := &Fake{SignalErr: syscall.EPERM}
	if err := f.Signal(11, syscall.SIGTERM); !errors.Is(err, syscall.EPERM) {
		t.Fatalf("Signal error = %v, want EPERM", err)
	}
	if err := f.Signal(12, syscall.SIGKILL); !errors.Is(err, syscall.EPERM) {
		t.Fatalf("Signal error = %v, want EPERM", err)
	}
	want := []SignalCall{
		{PID: 11, Sig: syscall.SIGTERM},
		{PID: 12, Sig: syscall.SIGKILL},
	}
	if !reflect.DeepEqual(f.Signals, want) {
		t.Fatalf("Signals = %#v, want %#v", f.Signals, want)
	}
	if f.signalCalls != 2 {
		t.Fatalf("signal calls = %d, want 2", f.signalCalls)
	}
}

func TestFakeOnTableSequence(t *testing.T) {
	first := Table{1: {PID: 1, Comm: "one", Reliable: true}}
	second := Table{2: {PID: 2, Comm: "two", Reliable: true}}
	f := &Fake{
		TableField: first,
		OnTable: func(call int) (Table, error) {
			switch call {
			case 1:
				return first, nil
			case 2:
				return second, nil
			default:
				return nil, ErrUnreadableSnapshot
			}
		},
	}
	got, err := f.Table(context.Background(), false)
	if err != nil || !reflect.DeepEqual(got, first) {
		t.Fatalf("call 1 = %#v, %v", got, err)
	}
	got, err = f.Table(context.Background(), true)
	if err != nil || !reflect.DeepEqual(got, second) {
		t.Fatalf("call 2 = %#v, %v", got, err)
	}
	if _, err = f.Table(context.Background(), false); !errors.Is(err, ErrUnreadableSnapshot) {
		t.Fatalf("call 3 error = %v, want ErrUnreadableSnapshot", err)
	}

	// Without OnTable the fixed TableField answers every call.
	g := &Fake{TableField: first}
	for i := 0; i < 2; i++ {
		got, err = g.Table(context.Background(), false)
		if err != nil || !reflect.DeepEqual(got, first) {
			t.Fatalf("fixed table call %d = %#v, %v", i+1, got, err)
		}
	}
}
