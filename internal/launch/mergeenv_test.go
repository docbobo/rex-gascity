package launch

import (
	"slices"
	"testing"
)

func TestMergeEnvOverlayReplacesAndSorts(t *testing.T) {
	got := MergeEnv([]string{"B=1", "A=old", "junk"}, map[string]string{"A": "new", "C": ""})
	want := []string{"A=new", "B=1", "C="}
	if !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestCommandDoesNotMutateArgv(t *testing.T) {
	argv := make([]string, 1, 8)
	argv[0] = "pi"
	_ = Command(argv, map[string]string{"A": "1"})
	if !slices.Equal(argv, []string{"pi"}) || argv[:2][1] != "" {
		t.Errorf("argv mutated: %v", argv[:2])
	}
}

func TestCommandNoEnvOrNoArgvUnchanged(t *testing.T) {
	if got := Command(nil, map[string]string{"A": "1"}); got != nil {
		t.Errorf("empty argv = %v", got)
	}
	if got := Command([]string{"pi"}, nil); !slices.Equal(got, []string{"pi"}) {
		t.Errorf("no env = %v", got)
	}
}
