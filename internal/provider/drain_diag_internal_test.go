package provider

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/docbobo/rex-gascity/internal/rex/faketest"
)

func TestParsePorcelainZ(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"empty", "", nil},
		{"plain modified", " M a.txt\x00", []string{"a.txt"}},
		{"staged modified", "M  a.txt\x00", []string{"a.txt"}},
		{"staged and worktree modified", "MM a.txt\x00", []string{"a.txt"}},
		{"rename keeps the new path and skips the old entry", "R  new.txt\x00old.txt\x00 M b.txt\x00", []string{"new.txt", "b.txt"}},
		{"rename in the worktree column", " R new.txt\x00old.txt\x00", []string{"new.txt"}},
		{"copy keeps the new path and skips the source entry", "C  copy.txt\x00src.txt\x00 M b.txt\x00", []string{"copy.txt", "b.txt"}},
		{"deletion", " D gone.txt\x00D  staged-gone.txt\x00", []string{"gone.txt", "staged-gone.txt"}},
		{"filename with spaces", " M dir with space/a b.txt\x00", []string{"dir with space/a b.txt"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parsePorcelainZ(tt.in); !slices.Equal(got, tt.want) {
				t.Errorf("parsePorcelainZ(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// fakeGit is a Runner that answers rev-parse and status like a work tree with
// one modified file, after waiting delay (or until the call's context ends).
func fakeGit(delay time.Duration) *FakeRunner {
	return &FakeRunner{Handle: func(ctx context.Context, c RunCall) (string, error) {
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return "", ctx.Err()
		}
		switch c.Args[0] {
		case "rev-parse":
			return "/tmp\n", nil
		case "status":
			return " M a.txt\x00", nil
		}
		return "", nil
	}}
}

func drainDiagProvider(t *testing.T, git Runner) (*Provider, string) {
	t.Helper()
	state := t.TempDir()
	return New("acme", faketest.New(), WithNamespace(NewNamespaceForDirs(state)), WithRunner(git)), state
}

// The drain diagnostic must not take git's optional locks (a worker may be
// running `git add` in the same tree) and must ignore untracked files, which
// would make an always-present untracked directory a false positive.
func TestDrainDiagnosticGitCallsAreReadOnlyAndTrackedOnly(t *testing.T) {
	git := fakeGit(0)
	p, state := drainDiagProvider(t, git)

	p.writeDrainDiagnostic(context.Background(), Binding{Name: "Indigo--dog-1", WorkDir: t.TempDir()})

	got := git.Calls()
	if len(got) != 2 {
		t.Fatalf("git calls = %+v, want rev-parse and status", got)
	}
	for _, c := range got {
		if c.Name != "git" || !slices.Contains(c.Env, "GIT_OPTIONAL_LOCKS=0") {
			t.Errorf("git call %+v ran without GIT_OPTIONAL_LOCKS=0", c)
		}
	}
	if status := strings.Join(got[1].Args, " "); !strings.Contains(status, "-z") || !strings.Contains(status, "--untracked-files=no") {
		t.Errorf("status call = %q, want -z and --untracked-files=no", status)
	}
	if m, _ := filepath.Glob(filepath.Join(state, "diagnostics", "*")); len(m) != 1 {
		t.Errorf("diagnostics = %v, want one", m)
	}
}

// One 3 s budget covers both git calls; two calls that each fit the budget but
// not together must not produce a diagnostic.
func TestDrainDiagnosticSharesOneTimeoutAcrossGitCalls(t *testing.T) {
	orig := drainDiagTimeout
	drainDiagTimeout = 300 * time.Millisecond
	t.Cleanup(func() { drainDiagTimeout = orig })
	p, state := drainDiagProvider(t, fakeGit(200*time.Millisecond))

	start := time.Now()
	p.writeDrainDiagnostic(context.Background(), Binding{Name: "Indigo--dog-1", WorkDir: t.TempDir()})

	if m, _ := filepath.Glob(filepath.Join(state, "diagnostics", "*")); len(m) != 0 {
		t.Errorf("diagnostics = %v, want none: the two calls together exceed the shared budget", m)
	}
	if d := time.Since(start); d > 600*time.Millisecond {
		t.Errorf("diagnostic took %v, want it bounded by the shared %v budget", d, drainDiagTimeout)
	}
}
