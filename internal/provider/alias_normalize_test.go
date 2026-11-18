package provider

import (
	"slices"
	"testing"
)

// An empty env value is unset at the launch boundary, never set to empty: gc's
// RPP contract and its tmux adapter both treat "" as unset. The GCREX-32
// regression test is TestStartPassesUnaliasedPoolIdentityThrough.
func TestLaunchCommandEmptyEnvIsUnset(t *testing.T) {
	got := launchCommand([]string{"pi"}, map[string]string{"B": "1", "GC_ALIAS": "", "A": ""})
	want := []string{"env", "-u", "A", "-u", "GC_ALIAS", "B=1", "pi"}
	if !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}
