package provider_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/docbobo/rex-gascity/internal/provider"
	"github.com/docbobo/rex-gascity/internal/rex/faketest"
)

func TestPreStartCommandsRunBeforeLaunch(t *testing.T) {
	dir := t.TempDir()
	fake := faketest.New()
	p := provider.New("acme", fake)

	a := filepath.Join(dir, "a")
	b := filepath.Join(dir, "b")
	cfg := provider.Config{
		Argv:    []string{"pi"},
		WorkDir: dir,
		PreStart: []string{
			"echo one > " + a,
			"echo two > " + b,
		},
	}
	if err := p.Start(context.Background(), "pi-mc-wisp-abc", cfg); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Both files must exist: pre_start ran in order before the launch.
	for _, f := range []string{a, b} {
		if _, err := os.Stat(f); err != nil {
			t.Errorf("pre_start file %s missing: %v", f, err)
		}
	}
	// The pane must have been launched after pre_start.
	if len(fake.CreatedWindows) == 0 {
		t.Error("NewWindow not called after successful pre_start")
	}
}

func TestPreStartFailureAborts(t *testing.T) {
	fake := faketest.New()
	p := provider.New("acme", fake)

	cfg := provider.Config{
		PreStart: []string{"false"},
	}
	err := p.Start(context.Background(), "pi-mc-wisp-abc", cfg)
	if err == nil {
		t.Fatal("expected error from failing pre_start, got nil")
	}

	// No pane must have been launched.
	if len(fake.CreatedWindows) > 0 || len(fake.CreatedSplits) > 0 {
		t.Error("NewWindow or NewSplit called after pre_start failure")
	}
	// No binding must remain.
	if _, ok := p.Binding("pi-mc-wisp-abc"); ok {
		t.Error("binding exists after failed start")
	}
	// No seeded identity.
	for _, key := range []string{"GC_SESSION_ID", "GC_INSTANCE_TOKEN", "GC_RUNTIME_EPOCH"} {
		if got, _ := p.GetMeta(context.Background(), "pi-mc-wisp-abc", key); got != "" {
			t.Errorf("%s = %q after failed start, want empty", key, got)
		}
	}
}

func TestPreStartNoOp(t *testing.T) {
	fake := faketest.New()
	p := provider.New("acme", fake)

	// No PreStart at all: must behave exactly as before.
	cfg := provider.Config{Argv: []string{"pi"}}
	if err := p.Start(context.Background(), "pi-mc-wisp-abc", cfg); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if len(fake.CreatedWindows) == 0 {
		t.Error("NewWindow not called with no pre_start")
	}
}

func TestPreStartEmptySlice(t *testing.T) {
	fake := faketest.New()
	p := provider.New("acme", fake)

	// An explicit empty slice must also be a no-op.
	cfg := provider.Config{Argv: []string{"pi"}, PreStart: []string{}}
	if err := p.Start(context.Background(), "pi-mc-wisp-abc", cfg); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if len(fake.CreatedWindows) == 0 {
		t.Error("NewWindow not called with empty pre_start")
	}
}

func TestPreStartEnv(t *testing.T) {
	dir := t.TempDir()
	fake := faketest.New()
	p := provider.New("acme", fake)

	outFile := filepath.Join(dir, "env.txt")
	cfg := provider.Config{
		Argv:    []string{"pi"},
		WorkDir: dir,
		Env:     map[string]string{"MY_TEST_VAR": "hello-world"},
		PreStart: []string{
			// Write the env var to a file; the shell expands $MY_TEST_VAR.
			"printf '%s' \"$MY_TEST_VAR\" > " + outFile,
		},
	}
	if err := p.Start(context.Background(), "pi-mc-wisp-abc", cfg); err != nil {
		t.Fatalf("Start: %v", err)
	}
	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("read env output: %v", err)
	}
	if got := strings.TrimSpace(string(data)); got != "hello-world" {
		t.Errorf("env output = %q, want %q", got, "hello-world")
	}
}

func TestPreStartInheritsBaseEnv(t *testing.T) {
	dir := t.TempDir()
	fake := faketest.New()
	p := provider.New("acme", fake)

	outFile := filepath.Join(dir, "home.txt")
	cfg := provider.Config{
		Argv:    []string{"pi"},
		WorkDir: dir,
		PreStart: []string{
			// $HOME comes from os.Environ(), not cfg.Env.
			"printf '%s' \"$HOME\" > " + outFile,
		},
	}
	if err := p.Start(context.Background(), "pi-mc-wisp-abc", cfg); err != nil {
		t.Fatalf("Start: %v", err)
	}
	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("read base env output: %v", err)
	}
	if got, want := strings.TrimSpace(string(data)), os.Getenv("HOME"); got != want {
		t.Errorf("pre_start HOME = %q, want %q (base env not inherited)", got, want)
	}
}

func TestPreStartCwd(t *testing.T) {
	dir := t.TempDir()
	fake := faketest.New()
	p := provider.New("acme", fake)

	outFile := "cwd.txt" // relative to WorkDir
	cfg := provider.Config{
		Argv:    []string{"pi"},
		WorkDir: dir,
		PreStart: []string{
			"pwd > " + outFile,
		},
	}
	if err := p.Start(context.Background(), "pi-mc-wisp-abc", cfg); err != nil {
		t.Fatalf("Start: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, outFile))
	if err != nil {
		t.Fatalf("read cwd output: %v", err)
	}
	got := strings.TrimSpace(string(data))
	// macOS symlinks /var → /private/var, so resolve both paths.
	resolvedGot, _ := filepath.EvalSymlinks(got)
	resolvedDir, _ := filepath.EvalSymlinks(dir)
	if resolvedGot != resolvedDir {
		t.Errorf("pre_start cwd = %q, want %q", got, dir)
	}
}

func TestPreStartFailureAbortsWithIdentityEnv(t *testing.T) {
	// When the start config carries identity env, a pre_start failure must
	// leave no seed behind even though the env was available.
	fake := faketest.New()
	p := provider.New("acme", fake)

	cfg := provider.Config{
		PreStart: []string{"false"},
		Env:      identityEnv(),
	}
	err := p.Start(context.Background(), "pi-mc-wisp-abc", cfg)
	if err == nil {
		t.Fatal("expected error from failing pre_start, got nil")
	}

	if len(fake.CreatedWindows) > 0 || len(fake.CreatedSplits) > 0 {
		t.Error("NewWindow or NewSplit called after pre_start failure")
	}
	if _, ok := p.Binding("pi-mc-wisp-abc"); ok {
		t.Error("binding exists after failed start")
	}
	for _, key := range []string{"GC_SESSION_ID", "GC_INSTANCE_TOKEN", "GC_RUNTIME_EPOCH"} {
		if got, _ := p.GetMeta(context.Background(), "pi-mc-wisp-abc", key); got != "" {
			t.Errorf("%s = %q after failed start, want empty", key, got)
		}
	}
}

func TestPreStartSecondCommandFails(t *testing.T) {
	dir := t.TempDir()
	fake := faketest.New()
	p := provider.New("acme", fake)

	first := filepath.Join(dir, "first")
	cfg := provider.Config{
		WorkDir: dir,
		PreStart: []string{
			"echo ran > " + first,
			"false",
		},
	}
	err := p.Start(context.Background(), "pi-mc-wisp-abc", cfg)
	if err == nil {
		t.Fatal("expected error from second pre_start command, got nil")
	}
	if !strings.Contains(err.Error(), "pre_start[1]") {
		t.Errorf("error %q does not mention pre_start[1]", err)
	}

	// The first command ran; the second aborted before any launch.
	if _, statErr := os.Stat(first); statErr != nil {
		t.Errorf("first pre_start file missing: %v", statErr)
	}
	if len(fake.CreatedWindows) > 0 || len(fake.CreatedSplits) > 0 {
		t.Error("NewWindow or NewSplit called after pre_start failure")
	}
	if _, ok := p.Binding("pi-mc-wisp-abc"); ok {
		t.Error("binding exists after failed start")
	}
}

// TestPreStartMayCreateWorkDir mirrors runtime-herdr-core's
// test_a_pre_start_may_create_the_work_dir: the gastown worktree recipe points
// work_dir at a worktree that pre_start creates, so a missing work_dir must
// not stop the hook from running.
func TestPreStartMayCreateWorkDir(t *testing.T) {
	root := t.TempDir()
	work := filepath.Join(root, "worktree")
	fake := faketest.New()
	p := provider.New("acme", fake)

	cfg := provider.Config{
		Argv:     []string{"pi"},
		WorkDir:  work,
		PreStart: []string{"mkdir -p " + work + " && echo pre > " + filepath.Join(work, "pre.txt")},
	}
	if err := p.Start(context.Background(), "pi-mc-wisp-abc", cfg); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if data, err := os.ReadFile(filepath.Join(work, "pre.txt")); err != nil || strings.TrimSpace(string(data)) != "pre" {
		t.Errorf("pre_start did not create the work dir: err=%v data=%q", err, string(data))
	}
	if len(fake.CreatedWindows) == 0 {
		t.Fatal("NewWindow not called")
	}
	if got := fake.CreatedWindows[0].Spec.Cwd; got != work {
		t.Errorf("launch cwd = %q, want %q", got, work)
	}
}

// TestMissingWorkDirAfterPreStartFails mirrors herdr's contract: a configured
// work_dir that no pre_start created fails the start before any pane.
func TestMissingWorkDirAfterPreStartFails(t *testing.T) {
	work := filepath.Join(t.TempDir(), "never")
	fake := faketest.New()
	p := provider.New("acme", fake)

	cfg := provider.Config{
		Argv:     []string{"pi"},
		WorkDir:  work,
		Env:      identityEnv(),
		PreStart: []string{"true"},
	}
	err := p.Start(context.Background(), "pi-mc-wisp-abc", cfg)
	if err == nil || !strings.Contains(err.Error(), "work_dir") {
		t.Fatalf("Start = %v, want a work_dir error", err)
	}
	if len(fake.CreatedWindows) > 0 || len(fake.CreatedSplits) > 0 {
		t.Error("a pane was launched with a missing work_dir")
	}
	if _, ok := p.Binding("pi-mc-wisp-abc"); ok {
		t.Error("binding exists after a missing-work_dir failure")
	}
	if got, _ := p.GetMeta(context.Background(), "pi-mc-wisp-abc", "GC_SESSION_ID"); got != "" {
		t.Errorf("identity seeded despite a missing work_dir: %q", got)
	}
}

// TestPreStartCwdFallsBackWhenWorkDirMissing checks the cwd fallback: a
// work_dir that does not exist yet cannot be the hook's cwd, so the hook runs
// in "/" (herdr's _setup_cwd) and is still able to create the work dir.
func TestPreStartCwdFallsBackWhenWorkDirMissing(t *testing.T) {
	root := t.TempDir()
	work := filepath.Join(root, "worktree")
	cwdFile := filepath.Join(root, "cwd.txt")
	fake := faketest.New()
	p := provider.New("acme", fake)

	cfg := provider.Config{
		Argv:     []string{"pi"},
		WorkDir:  work,
		PreStart: []string{"pwd > " + cwdFile + " && mkdir -p " + work},
	}
	if err := p.Start(context.Background(), "pi-mc-wisp-abc", cfg); err != nil {
		t.Fatalf("Start: %v", err)
	}
	data, err := os.ReadFile(cwdFile)
	if err != nil {
		t.Fatalf("read cwd output: %v", err)
	}
	if got := strings.TrimSpace(string(data)); got != "/" {
		t.Errorf("pre_start cwd = %q, want %q", got, "/")
	}
}
