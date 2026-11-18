package provider

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// stagePackOverlays copies each pack's per-provider overlay subtree into the
// session work directory. gc stages pack overlays itself, but its managed-file
// preserve predicate keeps an existing current hook and can skip the rex
// override. The provider writes the per-provider subtree after pre_start and
// after the work_dir check, so a managed v9 hook is replaced by the pack's v100
// override on every start. The writer does not compare versions; if a future
// managed hook exceeds v100, re-bump the override's marker. Later pack
// directories win on the same path.
func stagePackOverlays(workDir string, dirs []string, providerName string) error {
	if workDir == "" || providerName == "" {
		return nil
	}
	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		src := filepath.Join(dir, "per-provider", providerName)
		info, err := os.Stat(src)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("stage pack overlay %s: %w", src, err)
		}
		if !info.IsDir() {
			continue
		}
		if err := copyTree(src, workDir); err != nil {
			return fmt.Errorf("stage pack overlay %s: %w", src, err)
		}
	}
	return nil
}

// copyTree copies the regular files under src into dst, preserving each file's
// permission bits. It refuses symlinks and special files so an overlay cannot
// write outside the work directory through a link.
func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing symlink %s", path)
		}
		if !d.Type().IsRegular() {
			return fmt.Errorf("unsupported file type %s", path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		mode := os.FileMode(0o644)
		if info, err := d.Info(); err == nil {
			mode = info.Mode().Perm()
		}
		return os.WriteFile(target, data, mode)
	})
}
