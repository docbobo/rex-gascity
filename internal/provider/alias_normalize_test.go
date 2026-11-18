package provider

import (
	"slices"
	"testing"
)

// A pool seat must never be launched with a literal empty GC_ALIAS (which would
// mask gc's own fallbacks) and must never be launched with a synthesized bare
// template. gc's hook claim writer records the claim under alias first
// (cmd/gc/cmd_hook.go:866, hookClaimAssigneeIdentity at v1.5.1), so a
// template-derived alias makes every pool slot claim under one shared string
// that no open session bead answers to and gc's releaseOrphanedPoolAssignments
// reopens the work (GCREX-32). The provider therefore passes gc's identity
// through and projects the empty GC_ALIAS as `env -u`.
func TestLaunchCommandUnaliasedPoolIdentityPassesThrough(t *testing.T) {
	in := map[string]string{"GC_ALIAS": "", "GC_TEMPLATE": "r/pi", "GC_AGENT": "bead1", "GC_SESSION_ID": "bead1"}
	got := launchCommand([]string{"pi"}, in)
	want := []string{"env", "-u", "GC_ALIAS", "GC_AGENT=bead1", "GC_SESSION_ID=bead1", "GC_TEMPLATE=r/pi", "pi"}
	if !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	for _, arg := range got {
		if arg == "GC_ALIAS=r/pi" || arg == "GC_ALIAS=" {
			t.Errorf("launch command carries a GC_ALIAS value: %v", got)
		}
	}
}

func TestLaunchCommandEmptyEnvIsUnset(t *testing.T) {
	got := launchCommand([]string{"pi"}, map[string]string{"B": "1", "GC_ALIAS": "", "A": ""})
	want := []string{"env", "-u", "A", "-u", "GC_ALIAS", "B=1", "pi"}
	if !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}
