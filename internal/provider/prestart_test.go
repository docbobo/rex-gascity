package provider_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

func TestPreStartFailureWarnsAndContinues(t *testing.T) {
	fake := faketest.New()
	var warnings bytes.Buffer
	p := provider.New("acme", fake, provider.WithWarnings(&warnings))

	cfg := provider.Config{
		Argv:     []string{"pi"},
		PreStart: []string{"false"},
	}
	if err := p.Start(context.Background(), "pi-mc-wisp-abc", cfg); err != nil {
		t.Fatalf("Start: %v (a failing pre_start is non-fatal)", err)
	}

	// A failing hook must still surface a warning naming the hook.
	if !strings.Contains(warnings.String(), "pre_start[0]") {
		t.Errorf("warnings = %q, want a pre_start[0] failure warning", warnings.String())
	}
	// The launch must still happen and the binding must remain.
	if len(fake.CreatedWindows) == 0 && len(fake.CreatedSplits) == 0 {
		t.Error("no pane launched after a failing pre_start")
	}
	if _, ok := p.Binding("pi-mc-wisp-abc"); !ok {
		t.Error("binding missing after a non-fatal pre_start failure")
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

// An empty env value must reach the pre_start hook as unset, not as K=
// (GCREX-55). `${VAR+x}` is set only when the variable exists at all.
func TestPreStartEnvEmptyValueIsUnset(t *testing.T) {
	dir := t.TempDir()
	fake := faketest.New()
	p := provider.New("acme", fake)

	outFile := filepath.Join(dir, "unset.txt")
	cfg := provider.Config{
		Argv:    []string{"pi"},
		WorkDir: dir,
		Env:     map[string]string{"MY_EMPTY_VAR": ""},
		PreStart: []string{
			`if [ -z "${MY_EMPTY_VAR+x}" ]; then printf unset > ` + outFile + `; else printf set > ` + outFile + `; fi`,
		},
	}
	if err := p.Start(context.Background(), "pi-mc-wisp-abc", cfg); err != nil {
		t.Fatalf("Start: %v", err)
	}
	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("read env output: %v", err)
	}
	if got := strings.TrimSpace(string(data)); got != "unset" {
		t.Errorf("hook saw MY_EMPTY_VAR %q, want unset (an empty value must unset the variable)", got)
	}
}

func TestPreStartHookTimeoutWarnsAndContinues(t *testing.T) {
	fake := faketest.New()
	var warnings bytes.Buffer
	p := provider.New("acme", fake,
		provider.WithWarnings(&warnings),
		provider.WithPreStartHookTimeout(50*time.Millisecond))

	cfg := provider.Config{
		Argv:     []string{"pi"},
		PreStart: []string{"sleep 10"},
	}
	start := time.Now()
	if err := p.Start(context.Background(), "pi-mc-wisp-abc", cfg); err != nil {
		t.Fatalf("Start: %v (a pre_start timeout is non-fatal)", err)
	}
	if !strings.Contains(warnings.String(), "pre_start[0]") || !strings.Contains(warnings.String(), "timed out after") {
		t.Errorf("warnings = %q, want a pre_start[0] timeout", warnings.String())
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Errorf("Start took %v with a 50ms hook timeout", elapsed)
	}
	if len(fake.CreatedWindows) == 0 && len(fake.CreatedSplits) == 0 {
		t.Error("no pane launched after a pre_start timeout")
	}
}

func TestPreStartHookCappedByStartBudget(t *testing.T) {
	// The per-hook timeout is 10s but the start budget is 50ms: the hook must
	// be bounded by the budget, not the per-hook constant. The timeout warns
	// and the start continues.
	fake := faketest.New()
	var warnings bytes.Buffer
	p := provider.New("acme", fake,
		provider.WithWarnings(&warnings),
		provider.WithStartBudget(50*time.Millisecond),
		provider.WithPreStartHookTimeout(10*time.Second))

	cfg := provider.Config{Argv: []string{"pi"}, PreStart: []string{"sleep 10"}}
	start := time.Now()
	if err := p.Start(context.Background(), "pi-mc-wisp-abc", cfg); err != nil {
		t.Fatalf("Start: %v (a pre_start timeout is non-fatal)", err)
	}
	if !strings.Contains(warnings.String(), "timed out after") {
		t.Errorf("warnings = %q, want a pre_start timeout", warnings.String())
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Errorf("Start took %v, want it capped by the 50ms start budget", elapsed)
	}
}

func TestPreStartHookCappedByContextDeadline(t *testing.T) {
	fake := faketest.New()
	p := provider.New("acme", fake,
		provider.WithStartBudget(10*time.Second),
		provider.WithPreStartHookTimeout(10*time.Second))
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	cfg := provider.Config{Argv: []string{"pi"}, PreStart: []string{"sleep 10"}}
	start := time.Now()
	err := p.Start(ctx, "pi-mc-wisp-abc", cfg)
	if err == nil || !strings.Contains(err.Error(), "start canceled") {
		t.Fatalf("Start = %v, want a canceled start", err)
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Errorf("Start took %v, want it capped by the 50ms context deadline", elapsed)
	}
}

func TestPreStartHookWithinBudgetRuns(t *testing.T) {
	dir := t.TempDir()
	fake := faketest.New()
	p := provider.New("acme", fake, provider.WithPreStartHookTimeout(5*time.Second))

	out := filepath.Join(dir, "ran")
	cfg := provider.Config{
		Argv:     []string{"pi"},
		WorkDir:  dir,
		PreStart: []string{"echo ok > " + out},
	}
	if err := p.Start(context.Background(), "pi-mc-wisp-abc", cfg); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if _, err := os.Stat(out); err != nil {
		t.Errorf("hook within budget did not run: %v", err)
	}
	if len(fake.CreatedWindows) == 0 {
		t.Error("launch did not happen after a hook within budget")
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

func TestPreStartFailureWithIdentityEnvStillStarts(t *testing.T) {
	// When the start config carries identity env, a pre_start failure warns
	// but the launch still happens and the identity is seeded.
	fake := faketest.New()
	var warnings bytes.Buffer
	p := provider.New("acme", fake, provider.WithWarnings(&warnings))

	cfg := provider.Config{
		Argv:     []string{"pi"},
		PreStart: []string{"false"},
		Env:      identityEnv(),
	}
	if err := p.Start(context.Background(), "pi-mc-wisp-abc", cfg); err != nil {
		t.Fatalf("Start: %v (a failing pre_start is non-fatal)", err)
	}
	if !strings.Contains(warnings.String(), "pre_start[0]") {
		t.Errorf("warnings = %q, want a pre_start[0] failure warning", warnings.String())
	}
	if len(fake.CreatedWindows) == 0 && len(fake.CreatedSplits) == 0 {
		t.Error("no pane launched after a failing pre_start")
	}
	if _, ok := p.Binding("pi-mc-wisp-abc"); !ok {
		t.Error("binding missing after a non-fatal pre_start failure")
	}
	if got, _ := p.GetMeta(context.Background(), "pi-mc-wisp-abc", "GC_SESSION_ID"); got == "" {
		t.Error("identity not seeded after a non-fatal pre_start failure")
	}
}

func TestPreStartSecondCommandWarnsAndLaterHooksRun(t *testing.T) {
	dir := t.TempDir()
	fake := faketest.New()
	var warnings bytes.Buffer
	p := provider.New("acme", fake, provider.WithWarnings(&warnings))

	first := filepath.Join(dir, "first")
	third := filepath.Join(dir, "third")
	cfg := provider.Config{
		Argv:    []string{"pi"},
		WorkDir: dir,
		PreStart: []string{
			"echo ran > " + first,
			"false",
			"echo ran > " + third,
		},
	}
	if err := p.Start(context.Background(), "pi-mc-wisp-abc", cfg); err != nil {
		t.Fatalf("Start: %v (a failing pre_start is non-fatal)", err)
	}
	if !strings.Contains(warnings.String(), "pre_start[1]") {
		t.Errorf("warnings = %q, want a pre_start[1] failure warning", warnings.String())
	}

	// The first and third commands ran; the failed second did not stop the run.
	for _, f := range []string{first, third} {
		if _, statErr := os.Stat(f); statErr != nil {
			t.Errorf("pre_start file %s missing: %v", f, statErr)
		}
	}
	if len(fake.CreatedWindows) == 0 && len(fake.CreatedSplits) == 0 {
		t.Error("no pane launched after a failing pre_start")
	}
	if _, ok := p.Binding("pi-mc-wisp-abc"); !ok {
		t.Error("binding missing after a non-fatal pre_start failure")
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

// TestPreStartWaitsForBackgroundChild pins the timing a hook that backgrounds
// its work and exits relies on: the child holds the output pipe, so the hook
// runs to completion and its work dir exists before the launch is checked.
func TestPreStartWaitsForBackgroundChild(t *testing.T) {
	root := t.TempDir()
	work := filepath.Join(root, "worktree")
	fake := faketest.New()
	p := provider.New("acme", fake)

	cfg := provider.Config{
		Argv:     []string{"pi"},
		WorkDir:  work,
		PreStart: []string{"(sleep 0.3; mkdir -p " + work + ") &"},
	}
	if err := p.Start(context.Background(), "pi-mc-wisp-abc", cfg); err != nil {
		t.Fatalf("Start: %v (the background child must finish before the check)", err)
	}
	if len(fake.CreatedWindows) == 0 {
		t.Error("launch did not happen after a backgrounded pre_start")
	}
}

// TestPreStartCancelKillsBackgroundChild pins the grace branch of
// runPreStart: once cmd.Wait has seen the shell exit, cmd.Cancel no longer
// runs, so a backgrounded child that still holds the output pipe must be
// killed by hand. Without that kill the child survives the cancellation.
func TestPreStartCancelKillsBackgroundChild(t *testing.T) {
	root := t.TempDir()
	marker := filepath.Join(root, "alive")
	// The child writes ready before its delay, so the cancel lands after the
	// background subshell exists. A fixed delay can cancel too early and let the
	// absent marker pass without the grace-branch kill.
	ready := filepath.Join(root, "ready")
	fake := faketest.New()
	p := provider.New("acme", fake,
		provider.WithStartBudget(10*time.Second),
		provider.WithPreStartHookTimeout(10*time.Second))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		for i := 0; i < 500; i++ {
			if _, err := os.Stat(ready); err == nil {
				cancel()
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()

	cfg := provider.Config{
		Argv:     []string{"pi"},
		WorkDir:  root,
		PreStart: []string{"(echo ready > " + ready + "; sleep 0.6; echo alive > " + marker + ") &"},
	}
	if err := p.Start(ctx, "pi-mc-wisp-abc", cfg); err == nil {
		t.Fatal("Start succeeded, want a canceled start")
	}
	time.Sleep(time.Second)
	if _, err := os.Stat(marker); err == nil {
		t.Error("the backgrounded child survived the cancellation and wrote its marker")
	}
}
