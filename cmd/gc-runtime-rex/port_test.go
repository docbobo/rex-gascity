package main

import (
	"reflect"
	"testing"
)

// TestOpsTableHandlesThroughThePort pins AC-1: every op is dispatched through
// the rppProvider port, not a concrete *provider.Provider (REQ-001).
func TestOpsTableHandlesThroughThePort(t *testing.T) {
	for name, spec := range ops {
		arg := reflect.TypeOf(spec.handle).In(1)
		if arg.Kind() != reflect.Interface || arg.Name() != "rppProvider" {
			t.Errorf("ops[%q].handle takes %v, want the rppProvider port", name, arg)
		}
	}
}
