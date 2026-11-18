package provider_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/docbobo/rex-gascity/internal/provider"
	"github.com/docbobo/rex-gascity/internal/rex"
	"github.com/docbobo/rex-gascity/internal/rex/faketest"
)

func gitRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not available; the drain diagnostic needs it")
	}
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q")
	run("config", "user.email", "t@example.com")
	run("config", "user.name", "t")
	run("config", "commit.gpgsign", "false")
	for _, f := range []string{"a.txt", "b.txt"} {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("one\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	run("add", ".")
	run("commit", "-q", "-m", "init")
	return dir
}

// drainStarted starts a pool seat whose work dir is workDir and returns the
// provider, its state dir, and the diagnostics dir path.
func drainStarted(t *testing.T, workDir, stateDir string) *provider.Provider {
	t.Helper()
	return drainStartedWith(t, faketest.New(), workDir, stateDir)
}

func drainStartedWith(t *testing.T, client rex.Client, workDir, stateDir string) *provider.Provider {
	t.Helper()
	var opts []provider.Option
	if stateDir != "" {
		opts = append(opts, provider.WithNamespace(provider.NewNamespaceForDirs(stateDir)))
	}
	p := provider.New("acme", client, opts...)
	err := p.Start(context.Background(), "Indigo--dog-1", provider.Config{
		Argv:    []string{"claude"},
		WorkDir: workDir,
		Env:     map[string]string{"GC_SESSION_ID": "bead1", "GC_TEMPLATE": "Indigo/dog"},
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	return p
}

func diagFiles(t *testing.T, stateDir string) []string {
	t.Helper()
	m, err := filepath.Glob(filepath.Join(stateDir, "diagnostics", "*"))
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestStopWritesDiagnosticForUncommittedEdits(t *testing.T) {
	repo := gitRepo(t)
	now := time.Now()
	for f, age := range map[string]time.Duration{"a.txt": time.Hour, "b.txt": time.Minute} {
		path := filepath.Join(repo, f)
		if err := os.WriteFile(path, []byte("two\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, now.Add(-age), now.Add(-age)); err != nil {
			t.Fatal(err)
		}
	}
	state := t.TempDir()
	p := drainStarted(t, repo, state)

	if err := p.Stop(context.Background(), "Indigo--dog-1"); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	files := diagFiles(t, state)
	if len(files) != 1 {
		t.Fatalf("diagnostic files = %v, want exactly one", files)
	}
	data, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	var d struct {
		Worker     string   `json:"worker"`
		Aliases    []string `json:"aliases"`
		SessionID  string   `json:"session_id"`
		WorkDir    string   `json:"work_dir"`
		Changed    []string `json:"changed_files"`
		LastEdited string   `json:"last_edited_file"`
	}
	if err := json.Unmarshal(data, &d); err != nil {
		t.Fatalf("diagnostic is not JSON: %v\n%s", err, data)
	}
	if d.Worker != "Indigo--dog-1" {
		t.Errorf("worker = %q", d.Worker)
	}
	if d.SessionID != "bead1" {
		t.Errorf("session_id = %q, want bead1", d.SessionID)
	}
	if d.LastEdited != "b.txt" {
		t.Errorf("last_edited_file = %q, want b.txt (newest mtime)", d.LastEdited)
	}
	if !strings.Contains(strings.Join(d.Changed, ","), "a.txt") {
		t.Errorf("changed_files = %v, want a.txt listed", d.Changed)
	}
	if !strings.Contains(strings.Join(d.Aliases, ","), "Indigo--dog") {
		t.Errorf("aliases = %v, want the template alias", d.Aliases)
	}
}

func TestStopWritesNoDiagnosticWhenNothingToReport(t *testing.T) {
	clean := gitRepo(t)
	dirty := gitRepo(t)
	if err := os.WriteFile(filepath.Join(dirty, "a.txt"), []byte("two\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	untracked := gitRepo(t)
	if err := os.MkdirAll(filepath.Join(untracked, ".pi"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(untracked, ".pi", "x"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name    string
		workDir string
		state   func(t *testing.T) string
	}{
		{"clean repo", clean, func(t *testing.T) string { return t.TempDir() }},
		{"non-git dir", t.TempDir(), func(t *testing.T) string { return t.TempDir() }},
		{"no state dir", dirty, func(*testing.T) string { return "" }},
		{"empty work dir", "", func(t *testing.T) string { return t.TempDir() }},
		{"untracked files only", untracked, func(t *testing.T) string { return t.TempDir() }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := tt.state(t)
			p := drainStarted(t, tt.workDir, state)
			if err := p.Stop(context.Background(), "Indigo--dog-1"); err != nil {
				t.Fatalf("Stop: %v", err)
			}
			if state != "" {
				if files := diagFiles(t, state); len(files) != 0 {
					t.Errorf("diagnostic files = %v, want none", files)
				}
			}
		})
	}
}

// The work dir must survive a process restart: gc runs one provider process
// per op, so stop reads the binding the start process saved.
func TestBindingWorkDirRoundTripsThroughFileStore(t *testing.T) {
	repo := gitRepo(t)
	state := t.TempDir()
	drainStarted(t, repo, state)
	reader := provider.New("acme", faketest.New(), provider.WithNamespace(provider.NewNamespaceForDirs(state)))
	b, ok := reader.Binding("Indigo--dog-1")
	if !ok || b.WorkDir != repo {
		t.Errorf("reloaded WorkDir = %q (ok=%v), want %q", b.WorkDir, ok, repo)
	}
}

// closeProbe records whether the diagnostic already existed when the pane was
// closed: the tree must be sampled while the worker process is still alive.
type closeProbe struct {
	*faketest.Fake
	diagDir        string
	diagAtClose    []string
	closeWasCalled bool
}

func (c *closeProbe) CloseBlock(ctx context.Context, sid rex.SessionID, blk rex.BlockID) error {
	c.closeWasCalled = true
	c.diagAtClose, _ = filepath.Glob(filepath.Join(c.diagDir, "*.json"))
	return c.Fake.CloseBlock(ctx, sid, blk)
}

func TestStopWritesDiagnosticBeforeClosingPane(t *testing.T) {
	repo := gitRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "a.txt"), []byte("two\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	state := t.TempDir()
	probe := &closeProbe{Fake: faketest.New(), diagDir: filepath.Join(state, "diagnostics")}
	p := drainStartedWith(t, probe, repo, state)

	if err := p.Stop(context.Background(), "Indigo--dog-1"); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if !probe.closeWasCalled {
		t.Fatal("pane was not closed")
	}
	if len(probe.diagAtClose) != 1 {
		t.Errorf("diagnostics present at CloseBlock = %v, want one: the diagnostic must run before the pane closes", probe.diagAtClose)
	}
}

// A drained worker's process has often exited already, so its pane is gone
// when Stop runs. The diagnostic must still be written from the stored binding.
func TestStopWritesDiagnosticWhenPaneIsAlreadyGone(t *testing.T) {
	repo := gitRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "a.txt"), []byte("two\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	state := t.TempDir()
	fake := faketest.New()
	p := drainStartedWith(t, fake, repo, state)
	fake.View.Windows = nil // the pane vanished

	if err := p.Stop(context.Background(), "Indigo--dog-1"); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if files := diagFiles(t, state); len(files) != 1 {
		t.Fatalf("diagnostic files = %v, want exactly one", files)
	}
	if len(fake.ClosedBlocks) != 0 {
		t.Errorf("closed = %v, want none for a vanished pane", fake.ClosedBlocks)
	}
	if _, ok := p.Binding("Indigo--dog-1"); ok {
		t.Error("binding still stored after Stop")
	}
}
