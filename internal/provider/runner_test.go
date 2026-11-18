package provider_test

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

// TestExecRunnerBoundsAChildHoldingStdout checks that Run does not wait for a
// child that outlives the command and keeps stdout open: WaitDelay bounds it.
// The gc claim probe and the git drain calls rely on that bound.
func TestExecRunnerBoundsAChildHoldingStdout(t *testing.T) {
	start := time.Now()
	_, err := (provider.ExecRunner{}).Run(context.Background(), "", nil, "sh", "-c", "sleep 5 & exit 0")
	if err == nil {
		t.Fatal("Run returned nil error, want the WaitDelay error")
	}
	if !errors.Is(err, exec.ErrWaitDelay) {
		t.Errorf("Run error = %v, want exec.ErrWaitDelay", err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("Run took %v, want it bounded by WaitDelay", elapsed)
	}
}
