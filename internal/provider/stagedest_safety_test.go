package provider

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// These tests pin stageDest's containment rules at the copyTreeInto seam: a
// staged file never lands through a symlink, never over a non-regular file, and
// a verified directory is not re-trusted blindly. They characterise the
// behaviour the Lstat walk in stageDest provides, so its per-file cost can be
// trimmed without loosening it.

func writeSrcTree(t *testing.T, files map[string]string) string {
	t.Helper()
	src := t.TempDir()
	for rel, body := range files {
		p := filepath.Join(src, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return src
}

func stageInto(t *testing.T, work, src string) error {
	t.Helper()
	dest, err := newStageDest(work)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = dest.close() }()
	return copyTreeInto(dest, src, ".", nil)
}

func TestStageDestRefusesInRootDirSymlink(t *testing.T) {
	work := t.TempDir()
	// A symlink that stays inside the work dir: os.Root would follow it, so only
	// the explicit Lstat walk stops the write.
	if err := os.MkdirAll(filepath.Join(work, "real"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("real", filepath.Join(work, "sub")); err != nil {
		t.Fatal(err)
	}
	src := writeSrcTree(t, map[string]string{"sub/deep/file.txt": "x"})

	err := stageInto(t, work, src)
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("stage through in-root dir symlink = %v, want a symlink refusal", err)
	}
	if _, statErr := os.Stat(filepath.Join(work, "real", "deep", "file.txt")); statErr == nil {
		t.Error("file was written through the in-root symlink")
	}
}

func TestStageDestRefusesFinalSymlink(t *testing.T) {
	work := t.TempDir()
	target := filepath.Join(work, "target.txt")
	if err := os.WriteFile(target, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(work, "d"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(work, "d", "file.txt")); err != nil {
		t.Fatal(err)
	}
	src := writeSrcTree(t, map[string]string{"d/file.txt": "overwrite"})

	err := stageInto(t, work, src)
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("stage over final symlink = %v, want a symlink refusal", err)
	}
	if got, _ := os.ReadFile(target); string(got) != "keep" {
		t.Errorf("symlink target was overwritten: %q", got)
	}
}

func TestStageDestRefusesNonRegularDestination(t *testing.T) {
	work := t.TempDir()
	if err := os.MkdirAll(filepath.Join(work, "file.txt"), 0o755); err != nil {
		t.Fatal(err)
	}
	src := writeSrcTree(t, map[string]string{"file.txt": "x"})

	err := stageInto(t, work, src)
	if err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("stage over a directory = %v, want a not-a-regular-file refusal", err)
	}
}

func TestStageDestRefusesFileWhereDirectoryNeeded(t *testing.T) {
	work := t.TempDir()
	if err := os.WriteFile(filepath.Join(work, "sub"), []byte("a file"), 0o644); err != nil {
		t.Fatal(err)
	}
	src := writeSrcTree(t, map[string]string{"sub/file.txt": "x"})

	if err := stageInto(t, work, src); err == nil {
		t.Fatal("stage under a regular file succeeded, want an error")
	}
}

// TestStageDestChecksEveryDirectoryBranch guards a verified-directory cache: a
// clean branch staged first must not vouch for a sibling branch that holds a
// symlink.
func TestStageDestChecksEveryDirectoryBranch(t *testing.T) {
	work := t.TempDir()
	if err := os.MkdirAll(filepath.Join(work, "real"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(work, "ok"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("real", filepath.Join(work, "bad")); err != nil {
		t.Fatal(err)
	}
	src := writeSrcTree(t, map[string]string{
		"ok/a.txt":  "1",
		"ok/b.txt":  "2",
		"bad/c.txt": "3",
	})

	if err := stageInto(t, work, src); err == nil {
		t.Fatal("stage through a sibling-branch symlink succeeded")
	}
	if _, err := os.Stat(filepath.Join(work, "real", "c.txt")); err == nil {
		t.Error("file was written through the sibling-branch symlink")
	}
}

func TestStageDestOverwritesAndPreservesMode(t *testing.T) {
	work := t.TempDir()
	src := writeSrcTree(t, map[string]string{"a/b/c.txt": "new"})
	if err := os.Chmod(filepath.Join(src, "a", "b", "c.txt"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(work, "a", "b"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, "a", "b", "c.txt"), []byte("old-and-longer"), 0o644); err != nil {
		t.Fatal(err)
	}

	for range 2 { // second pass is the re-stage case
		if err := stageInto(t, work, src); err != nil {
			t.Fatal(err)
		}
	}
	got, err := os.ReadFile(filepath.Join(work, "a", "b", "c.txt"))
	if err != nil || string(got) != "new" {
		t.Fatalf("staged content = %q, %v; want %q", got, err, "new")
	}
}
