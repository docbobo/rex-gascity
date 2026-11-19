package provider_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/docbobo/rex-gascity/internal/provider"
	"github.com/docbobo/rex-gascity/internal/rex"
	"github.com/docbobo/rex-gascity/internal/rex/faketest"
)

// writeTestFile writes content at dir/rel, creating parents.
func writeTestFile(t *testing.T, dir, rel, content string) string {
	t.Helper()
	path := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

func readTestFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

// TestStartStagesOverlayDirAndCopyFiles pins GCREX-56 AC2: an overlay_dir's
// universal and per-provider trees, plus every copy_files entry, land in the
// work directory before the launch.
func TestStartStagesOverlayDirAndCopyFiles(t *testing.T) {
	overlay := t.TempDir()
	writeTestFile(t, overlay, "notes/top.txt", "top")
	writeTestFile(t, filepath.Join(overlay, "per-provider", "pi"), "hook.js", "override")
	writeTestFile(t, filepath.Join(overlay, runtimeMirrorName()), "runtime.json", "mirror")

	srcDir := t.TempDir()
	srcFile := writeTestFile(t, srcDir, "seed.txt", "seed")
	srcTree := filepath.Join(srcDir, "tree")
	writeTestFile(t, srcTree, "a.txt", "a")

	workDir := filepath.Join(t.TempDir(), "work")
	fake := faketest.New()
	p := provider.New("acme", fake)

	cfg := provider.Config{
		Argv:       []string{"pi"},
		Env:        map[string]string{"GC_PROVIDER": "pi"},
		WorkDir:    workDir,
		OverlayDir: overlay,
		CopyFiles: []provider.CopyFile{
			{Src: srcFile, RelDst: "sub/seed.txt"},
			{Src: srcTree, RelDst: "dest"},
		},
	}
	if err := p.Start(context.Background(), "pi-mc-wisp-abc", cfg); err != nil {
		t.Fatalf("Start: %v", err)
	}

	if got := readTestFile(t, filepath.Join(workDir, "notes", "top.txt")); got != "top" {
		t.Errorf("universal overlay file = %q, want top", got)
	}
	if got := readTestFile(t, filepath.Join(workDir, "hook.js")); got != "override" {
		t.Errorf("per-provider overlay file = %q, want override", got)
	}
	if _, err := os.Stat(filepath.Join(workDir, ".gc", "runtime.json")); !os.IsNotExist(err) {
		t.Errorf(".gc was staged into the work dir (err=%v)", err)
	}
	if got := readTestFile(t, filepath.Join(workDir, "sub", "seed.txt")); got != "seed" {
		t.Errorf("copied file = %q, want seed", got)
	}
	if got := readTestFile(t, filepath.Join(workDir, "dest", "a.txt")); got != "a" {
		t.Errorf("copied tree file = %q, want a", got)
	}
	if len(fake.CreatedWindows) == 0 {
		t.Error("pane not launched after staging")
	}
}

// runtimeMirrorName mirrors the provider's unexported constant so the test does
// not import internals.
func runtimeMirrorName() string { return ".gc" }

// TestStartCopyFilesSkipsASourceAlreadyAtItsDestination pins gc's sameFile
// guard: a copy_files entry whose source already is its destination is
// skipped, so staging a work dir onto itself is a no-op rather than a walk of
// its own tree (gc v1.5.1 internal/runtime/staging.go:110-124,331-341;
// herdrcore/staging.py:216-233).
func TestStartCopyFilesSkipsASourceAlreadyAtItsDestination(t *testing.T) {
	workDir := filepath.Join(t.TempDir(), "work")
	writeTestFile(t, filepath.Join(workDir, "sub"), "merged.txt", "merged")
	srcFile := writeTestFile(t, workDir, "loose.txt", "loose")

	fake := faketest.New()
	p := provider.New("acme", fake)
	cfg := provider.Config{
		Argv:    []string{"pi"},
		WorkDir: workDir,
		CopyFiles: []provider.CopyFile{
			{Src: filepath.Join(workDir, "sub"), RelDst: "sub"},
			{Src: srcFile, RelDst: "loose.txt"},
		},
	}
	if err := p.Start(context.Background(), "pi-mc-wisp-abc", cfg); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if got := readTestFile(t, filepath.Join(workDir, "sub", "merged.txt")); got != "merged" {
		t.Errorf("self-staged tree changed, got %q", got)
	}
	if got := readTestFile(t, filepath.Join(workDir, "loose.txt")); got != "loose" {
		t.Errorf("self-staged file changed, got %q", got)
	}
}

// TestStartOverlayDirNotADirectoryFails pins the failed start for a non-dir
// overlay source, as both gc and herdr-core treat it.
func TestStartOverlayDirNotADirectoryFails(t *testing.T) {
	srcDir := t.TempDir()
	notADir := writeTestFile(t, srcDir, "plain.txt", "x")
	fake := faketest.New()
	p := provider.New("acme", fake)
	cfg := provider.Config{
		Argv:       []string{"pi"},
		WorkDir:    filepath.Join(t.TempDir(), "work"),
		OverlayDir: notADir,
	}
	if err := p.Start(context.Background(), "pi-mc-wisp-abc", cfg); err == nil {
		t.Fatal("Start succeeded with a non-directory overlay source")
	}
}

// TestStartStagesBeforePreStart pins the documented phase order: staging runs
// before pre_start, so a hook can rely on the staged work dir. The pre_start
// hook fails unless the staged overlay and copy_files files are already there;
// a reordering therefore leaves a pre_start warning even though the start
// continues (herdrcore/lifecycle.py:789).
func TestStartStagesBeforePreStart(t *testing.T) {
	overlay := t.TempDir()
	writeTestFile(t, overlay, "notes/top.txt", "top")
	srcDir := t.TempDir()
	srcFile := writeTestFile(t, srcDir, "seed.txt", "seed")
	workDir := filepath.Join(t.TempDir(), "work")

	fake := faketest.New()
	var warnings bytes.Buffer
	p := provider.New("acme", fake, provider.WithWarnings(&warnings))
	cfg := provider.Config{
		Argv:       []string{"pi"},
		WorkDir:    workDir,
		OverlayDir: overlay,
		CopyFiles:  []provider.CopyFile{{Src: srcFile, RelDst: "sub/seed.txt"}},
		PreStart:   []string{"test -f " + filepath.Join(workDir, "notes", "top.txt") + " && test -f " + filepath.Join(workDir, "sub", "seed.txt")},
	}
	if err := p.Start(context.Background(), "pi-mc-wisp-abc", cfg); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if strings.Contains(warnings.String(), "pre_start") {
		t.Fatalf("pre_start did not see the staged work dir: %s", warnings.String())
	}
}

// TestStartSessionSetupOnlyAfterALaunch pins that the setup hooks depend on a
// successful launch: when the launch fails, no setup hook runs, so a pane is
// always up before session_setup (herdr-core runs them after _create;
// herdrcore/lifecycle.py:810-818).
func TestStartSessionSetupOnlyAfterALaunch(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "setup.txt")
	fake := faketest.New()
	fake.SessionViewErr = errors.New("rex unavailable")
	p := provider.New("acme", fake)
	cfg := provider.Config{
		Argv:         []string{"pi"},
		WorkDir:      dir,
		SessionSetup: []string{"printf done > " + out},
	}
	if err := p.Start(context.Background(), "pi-mc-wisp-abc", cfg); err == nil {
		t.Fatal("Start succeeded despite a failing launch")
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Errorf("session_setup ran without a launched pane (err=%v)", err)
	}
}

// TestStartStagingRefusesDestinationSymlink pins the containment guarantee the
// docs claim: a work-dir component that is a symlink is refused instead of
// followed, so nothing is written outside the work directory through it
// (herdrcore/staging.py:17,68-129).
func TestStartStagingRefusesDestinationSymlink(t *testing.T) {
	out := t.TempDir()
	workDir := filepath.Join(t.TempDir(), "work")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.Symlink(out, filepath.Join(workDir, "sub")); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	srcDir := t.TempDir()
	srcTree := filepath.Join(srcDir, "tree")
	writeTestFile(t, srcTree, "a.txt", "a")
	srcFile := writeTestFile(t, srcDir, "seed.txt", "seed")

	fake := faketest.New()
	p := provider.New("acme", fake)
	cfg := provider.Config{
		Argv:      []string{"pi"},
		WorkDir:   workDir,
		CopyFiles: []provider.CopyFile{{Src: srcTree, RelDst: "sub"}, {Src: srcFile, RelDst: "sub/seed.txt"}},
	}
	if err := p.Start(context.Background(), "pi-mc-wisp-abc", cfg); err == nil {
		t.Fatal("Start succeeded through a destination symlink, want a staging failure")
	}
	if _, err := os.Stat(filepath.Join(out, "a.txt")); !os.IsNotExist(err) {
		t.Errorf("staging wrote through the symlink outside the work dir (err=%v)", err)
	}
	if _, err := os.Stat(filepath.Join(out, "seed.txt")); !os.IsNotExist(err) {
		t.Errorf("staging wrote through the symlink outside the work dir (err=%v)", err)
	}
}

// TestStartCopyFilesOmittedRelDstStagesIntoWorkDir pins the gc StagePath
// contract: with no rel_dst a directory source merges into the work directory
// itself, and a file lands in it under its base name (gc v1.5.1
// internal/runtime/staging.go:110-124, herdrcore/staging.py:216-233).
func TestStartCopyFilesOmittedRelDstStagesIntoWorkDir(t *testing.T) {
	srcDir := t.TempDir()
	srcTree := filepath.Join(srcDir, "tree")
	writeTestFile(t, srcTree, "merged.txt", "merged")
	srcFile := writeTestFile(t, srcDir, "loose.txt", "loose")

	workDir := filepath.Join(t.TempDir(), "work")
	fake := faketest.New()
	p := provider.New("acme", fake)
	cfg := provider.Config{
		Argv:      []string{"pi"},
		WorkDir:   workDir,
		CopyFiles: []provider.CopyFile{{Src: srcTree}, {Src: srcFile}},
	}
	if err := p.Start(context.Background(), "pi-mc-wisp-abc", cfg); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if got := readTestFile(t, filepath.Join(workDir, "merged.txt")); got != "merged" {
		t.Errorf("tree with no rel_dst merges into the work dir, got %q", got)
	}
	if got := readTestFile(t, filepath.Join(workDir, "loose.txt")); got != "loose" {
		t.Errorf("file with no rel_dst lands under its base name, got %q", got)
	}
}

// TestStartSessionSetupSeesProjectedLaunchEnv pins that the setup hooks run
// with the launch plan's projected env (pool routing applied), not the raw
// config env: BEADS_ACTOR must be the template the launched pane carries
// (internal/launch/launch.go:projectPoolRouting; herdrcore/envplan.py:51-76).
func TestStartSessionSetupSeesProjectedLaunchEnv(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "actor.txt")
	fake := faketest.New()
	p := provider.New("acme", fake)
	cfg := provider.Config{
		Argv: []string{"pi"},
		Env: map[string]string{
			"GC_TEMPLATE":   "gastownhall/rex",
			"GC_AGENT":      "dog-1",
			"GC_SESSION_ID": "bead-123",
		},
		SessionSetup: []string{"printf '%s' \"$BEADS_ACTOR\" > " + out},
	}
	if err := p.Start(context.Background(), "pi-mc-wisp-abc", cfg); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if got := readTestFile(t, out); got != "gastownhall/rex" {
		t.Errorf("setup BEADS_ACTOR = %q, want the projected template", got)
	}
}

// TestStartCopyFilesRefusesEscapingDestination pins that a destination which
// leaves the work directory is a warning, not a failed start, and writes
// nothing outside.
func TestStartCopyFilesRefusesEscapingDestination(t *testing.T) {
	srcDir := t.TempDir()
	srcFile := writeTestFile(t, srcDir, "seed.txt", "seed")
	workDir := filepath.Join(t.TempDir(), "work")

	var warnings bytes.Buffer
	fake := faketest.New()
	p := provider.New("acme", fake, provider.WithWarnings(&warnings))

	cfg := provider.Config{
		Argv:      []string{"pi"},
		WorkDir:   workDir,
		CopyFiles: []provider.CopyFile{{Src: srcFile, RelDst: "../escaped.txt"}},
	}
	if err := p.Start(context.Background(), "pi-mc-wisp-abc", cfg); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(workDir), "escaped.txt")); !os.IsNotExist(err) {
		t.Errorf("escaping copy_files wrote outside the work dir (err=%v)", err)
	}
	if !strings.Contains(warnings.String(), "leaves the work directory") {
		t.Errorf("warnings = %q, want a refusal", warnings.String())
	}
}

// TestStartSessionSetupRunsAsWarningsOnly pins GCREX-56 AC2: session_setup and
// session_setup_script run after the launch, and a failing hook is a warning,
// never a failed start (the pane is already up).
func TestStartSessionSetupRunsAsWarningsOnly(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "setup.txt")
	var warnings bytes.Buffer
	fake := faketest.New()
	p := provider.New("acme", fake, provider.WithWarnings(&warnings))

	cfg := provider.Config{
		Argv:               []string{"pi"},
		WorkDir:            dir,
		SessionSetup:       []string{"printf 'one\\n' >> " + out},
		SessionSetupScript: "printf 'script\\n' >> " + out,
	}
	if err := p.Start(context.Background(), "pi-mc-wisp-abc", cfg); err != nil {
		t.Fatalf("Start: %v", err)
	}
	got := readTestFile(t, out)
	if got != "one\nscript\n" {
		t.Errorf("setup output = %q, want one\\nscript\\n", got)
	}

	// A failing setup hook must not fail the start; the pane is already up.
	warnings.Reset()
	cfg2 := provider.Config{
		Argv:         []string{"pi"},
		WorkDir:      dir,
		SessionSetup: []string{"exit 3"},
	}
	if err := p.Start(context.Background(), "pi-mc-wisp-def", cfg2); err != nil {
		t.Fatalf("Start with failing session_setup: %v", err)
	}
	if !strings.Contains(warnings.String(), "session_setup[0]") {
		t.Errorf("warnings = %q, want a session_setup failure warning", warnings.String())
	}

	// A failing script hook is a warning too, named as a script failure.
	warnings.Reset()
	err := p.Start(context.Background(), "pi-mc-wisp-ghi", provider.Config{
		Argv:               []string{"pi"},
		WorkDir:            dir,
		SessionSetupScript: "exit 4",
	})
	if err != nil {
		t.Fatalf("Start with failing session_setup_script: %v", err)
	}
	if !strings.Contains(warnings.String(), "session_setup_script") {
		t.Errorf("warnings = %q, want a session_setup_script failure warning", warnings.String())
	}
}

// TestStartWarnsSessionLiveSkipped pins GCREX-56 AC2: a non-empty session_live
// emits exactly one SESSION_LIVE_SKIPPED warning and is never applied.
func TestStartWarnsSessionLiveSkipped(t *testing.T) {
	var warnings bytes.Buffer
	fake := faketest.New()
	p := provider.New("acme", fake, provider.WithWarnings(&warnings))

	cfg := provider.Config{
		Argv:        []string{"pi"},
		SessionLive: []string{"set -g status-right '#H'"},
	}
	if err := p.Start(context.Background(), "pi-mc-wisp-abc", cfg); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if n := strings.Count(warnings.String(), "session_live is not applied"); n != 1 {
		t.Errorf("session_live warnings = %d, want 1 (got %q)", n, warnings.String())
	}
}

// TestStartNewFreshOnlyRefusesAHeldName pins GCREX-56 AC2: fresh_only refuses a
// name a record already holds (even a dead one) instead of recycling it.
func TestStartNewFreshOnlyRefusesAHeldName(t *testing.T) {
	fake := faketest.New()
	p := provider.New("acme", fake)
	ctx := context.Background()

	if err := p.Start(ctx, "pi-mc-wisp-abc", provider.Config{Argv: []string{"pi"}}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	err := p.StartNew(ctx, "pi-mc-wisp-abc", provider.Config{Argv: []string{"pi"}, FreshOnly: true})
	if !errors.Is(err, provider.ErrFreshCollision) {
		t.Fatalf("StartNew fresh_only live = %v, want ErrFreshCollision", err)
	}

	// Let the recorded pane appear dead but keep the record: fresh_only still
	// refuses, where a normal StartNew would recycle.
	fake.ProcessErr = rex.ErrNotFound
	err = p.StartNew(ctx, "pi-mc-wisp-abc", provider.Config{Argv: []string{"pi"}, FreshOnly: true})
	if !errors.Is(err, provider.ErrFreshCollision) {
		t.Fatalf("StartNew fresh_only dead record = %v, want ErrFreshCollision", err)
	}
}

// TestStartReadinessUsesReadyDelayMs pins GCREX-56 AC2: ready_delay_ms is a
// floor before the startup nudge may be delivered.
func TestStartReadinessUsesReadyDelayMs(t *testing.T) {
	fake := faketest.New()
	fake.ProgramStatuses = map[rex.BlockID]rex.ProgramStatus{
		"block:dog-1": {Records: []rex.ProgramStatusRecord{{State: "idle", UpdatedAt: time.Now()}}},
	}
	p := startupProvider(fake, provider.WithStartupReadiness(2*time.Second, 20*time.Millisecond))

	start := time.Now()
	if err := p.Start(context.Background(), "Indigo--dog-1", provider.Config{
		Argv:         []string{"pi"},
		Nudge:        "hello agent",
		ReadyDelayMS: 150,
	}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if elapsed := time.Since(start); elapsed < 120*time.Millisecond {
		t.Errorf("start returned after %v, want the 150ms ready_delay_ms to elapse", elapsed)
	}
}

// TestStartReadinessUsesReadyPromptPrefix pins GCREX-56 AC2: with a
// ready_prompt_prefix set, the pane's rendered prompt, not program_status,
// decides readiness.
func TestStartReadinessUsesReadyPromptPrefix(t *testing.T) {
	const prefix = "❯ "
	fake := faketest.New()
	// The program_status is working, not idle: only the prompt can declare
	// readiness here, so the positive half cannot pass through the idle path
	// if the prefix handling regresses.
	fake.ProgramStatuses = map[rex.BlockID]rex.ProgramStatus{
		"block:dog-1": {Records: []rex.ProgramStatusRecord{{State: "working", UpdatedAt: time.Now()}}},
	}
	fake.Formats = map[rex.BlockID]string{"block:dog-1": "booting\n" + prefix}
	p := startupProvider(fake, provider.WithStartupReadiness(2*time.Second, 20*time.Millisecond))

	start := time.Now()
	if err := p.Start(context.Background(), "Indigo--dog-1", provider.Config{
		Argv:              []string{"pi"},
		Nudge:             "hello agent",
		ReadyPromptPrefix: prefix,
	}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if elapsed := time.Since(start); elapsed >= time.Second {
		t.Errorf("start took %v with the ready prompt already on screen", elapsed)
	}

	// Without the prefix on screen, readiness is never declared, so the nudge
	// waits the bounded window and is then delivered best-effort.
	fake2 := faketest.New()
	fake2.ProgramStatuses = fake.ProgramStatuses
	p2 := startupProvider(fake2, provider.WithStartupReadiness(150*time.Millisecond, 20*time.Millisecond))
	start = time.Now()
	if err := p2.Start(context.Background(), "Indigo--dog-1", provider.Config{
		Argv:              []string{"pi"},
		Nudge:             "hello agent",
		ReadyPromptPrefix: prefix,
	}); err != nil {
		t.Fatalf("Start without prompt: %v", err)
	}
	if elapsed := time.Since(start); elapsed < 120*time.Millisecond {
		t.Errorf("start returned after %v, want it to wait the readiness window", elapsed)
	}
}
