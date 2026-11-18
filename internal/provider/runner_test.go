package provider_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/docbobo/rex-gascity/internal/provider"
)

// ExecRunner runs in dir, adds env to the inherited environment, and returns
// stdout; a non-zero exit is an error.
func TestExecRunner(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("RUNNER_INHERITED", "yes")
	out, err := provider.ExecRunner{}.Run(context.Background(), dir, []string{"RUNNER_EXTRA=1"},
		"sh", "-c", `echo "$PWD $RUNNER_INHERITED $RUNNER_EXTRA"`)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	want, _ := filepath.EvalSymlinks(dir)
	got := strings.Fields(out)
	if len(got) != 3 || got[1] != "yes" || got[2] != "1" {
		t.Fatalf("output = %q, want <dir> yes 1", out)
	}
	if gotDir, _ := filepath.EvalSymlinks(got[0]); gotDir != want {
		t.Errorf("ran in %q, want %q", got[0], want)
	}
	if _, err := (provider.ExecRunner{}).Run(context.Background(), "", nil, "sh", "-c", "exit 3"); err == nil {
		t.Error("Run of a failing command returned nil error")
	}
}
