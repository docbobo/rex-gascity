package launch

import (
	"slices"
	"testing"
)

func TestMergeEnvOverlayReplacesAndSorts(t *testing.T) {
	got := mergeEnv([]string{"B=1", "A=old", "junk"}, map[string]string{"A": "new"})
	want := []string{"A=new", "B=1"}
	if !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// An empty overlay value means the variable must be unset, matching gc's env
// contract (docs/reference/exec-session-provider.md:269 at v1.5.1) and the
// pack's envplan: the key is removed rather than set to empty, so a pre_start
// hook sees it unset instead of K= (GCREX-55).
func TestMergeEnvEmptyValueUnsets(t *testing.T) {
	got := mergeEnv([]string{"B=1", "A=old"}, map[string]string{"A": "", "C": ""})
	want := []string{"B=1"}
	if !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// A non-empty overlay value is passed through unchanged.
func TestMergeEnvNonEmptyValuePassedThrough(t *testing.T) {
	got := mergeEnv(nil, map[string]string{"K": "v"})
	want := []string{"K=v"}
	if !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestCommandDoesNotMutateArgv(t *testing.T) {
	argv := make([]string, 1, 8)
	argv[0] = "pi"
	_ = command(argv, map[string]string{"A": "1"})
	if !slices.Equal(argv, []string{"pi"}) || argv[:2][1] != "" {
		t.Errorf("argv mutated: %v", argv[:2])
	}
}

func TestCommandNoEnvOrNoArgvUnchanged(t *testing.T) {
	if got := command(nil, map[string]string{"A": "1"}); got != nil {
		t.Errorf("empty argv = %v", got)
	}
	if got := command([]string{"pi"}, nil); !slices.Equal(got, []string{"pi"}) {
		t.Errorf("no env = %v", got)
	}
}
