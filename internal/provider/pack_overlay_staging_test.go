package provider

import (
	"os"
	"path/filepath"
	"testing"
)

// TestStagePackOverlaysStagesConfiguredSubtree proves the provider stages a
// pack's per-provider subtree over a workdir that already holds a managed
// file: gc's managed-file preserve predicate keeps that existing file, so the
// provider's writer is what makes a city-configured pack_overlay_dirs override
// reach the session. The fixture is synthetic; the pack ships no overlay.
func TestStagePackOverlaysStagesConfiguredSubtree(t *testing.T) {
	workDir := t.TempDir()
	hookPath := filepath.Join(workDir, ".pi", "extensions", "gc-hooks.js")
	if err := os.MkdirAll(filepath.Dir(hookPath), 0o755); err != nil {
		t.Fatalf("mkdir hook dir: %v", err)
	}
	managedV9 := "// gc managed Pi hook\nconst GC_PI_HOOK_VERSION = 9;\n"
	if err := os.WriteFile(hookPath, []byte(managedV9), 0o644); err != nil {
		t.Fatalf("write managed v9 hook: %v", err)
	}

	packOverlay := t.TempDir()
	override := "// synthetic pack override\nconst GC_PI_HOOK_VERSION = 100;\n"
	writeOverlayFixture(t, packOverlay, filepath.Join(perProviderDir, "pi", ".pi", "extensions", "gc-hooks.js"), override)
	writeOverlayFixture(t, packOverlay, filepath.Join(perProviderDir, "other", ".other", "settings.json"), "other provider")
	writeOverlayFixture(t, packOverlay, ".pi/extensions/universal.js", "universal")

	if err := stagePackOverlays(workDir, []string{packOverlay}, "pi"); err != nil {
		t.Fatalf("stage pack overlays: %v", err)
	}

	got, err := os.ReadFile(hookPath)
	if err != nil {
		t.Fatalf("read staged hook: %v", err)
	}
	if string(got) != override {
		t.Fatalf("managed hook was not replaced by the configured override:\n%s", got)
	}
	for _, rel := range []string{".pi/extensions/universal.js", ".other/settings.json"} {
		if _, err := os.Stat(filepath.Join(workDir, rel)); !os.IsNotExist(err) {
			t.Fatalf("%s was staged; only the per-provider/pi subtree may land", rel)
		}
	}
}

func writeOverlayFixture(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
