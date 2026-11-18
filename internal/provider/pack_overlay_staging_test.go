package provider

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestStagePackOverlaysReplacesManagedHook proves the provider stages the real
// rex pack per-provider Pi overlay over a workdir that already holds a managed
// v9 hook. gc's preserve predicate would keep that current managed file, so
// the provider's writer is what makes the claim guard reach the session.
func TestStagePackOverlaysReplacesManagedHook(t *testing.T) {
	workDir := t.TempDir()
	hookPath := filepath.Join(workDir, ".pi", "extensions", "gc-hooks.js")
	if err := os.MkdirAll(filepath.Dir(hookPath), 0o755); err != nil {
		t.Fatalf("mkdir hook dir: %v", err)
	}
	managedV9 := "// gc managed Pi hook\nconst GC_PI_HOOK_VERSION = 9;\n"
	if err := os.WriteFile(hookPath, []byte(managedV9), 0o644); err != nil {
		t.Fatalf("write managed v9 hook: %v", err)
	}

	packOverlay, err := filepath.Abs("../../pack/overlay")
	if err != nil {
		t.Fatalf("resolve pack overlay: %v", err)
	}
	if err := stagePackOverlays(workDir, []string{packOverlay}, "pi"); err != nil {
		t.Fatalf("stage pack overlays: %v", err)
	}

	got, err := os.ReadFile(hookPath)
	if err != nil {
		t.Fatalf("read staged hook: %v", err)
	}
	text := string(got)
	if !strings.Contains(text, "const GC_PI_HOOK_VERSION = 100;") {
		t.Fatalf("managed v9 hook was not replaced by the rex v100 override:\n%s", text)
	}
	if !strings.Contains(text, "currentClaimID") {
		t.Fatalf("staged hook is missing the claim guard:\n%s", text)
	}
}
