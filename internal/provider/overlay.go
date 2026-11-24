package provider

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// runtimeMirrorDir is gc's runtime mirror: a top-level .gc in an overlay is
// never staged (herdrcore/staging.py:27,100).
const runtimeMirrorDir = ".gc"

// perProviderDir is an overlay's provider subtree directory name
// (herdrcore/staging.py:26).
const perProviderDir = "per-provider"

// stagePackOverlays copies each pack's per-provider overlay subtree into the
// session work directory. gc stages pack overlays itself, but its managed-file
// preserve predicate keeps an existing current file and can skip the pack's
// override. The provider writes the per-provider subtree after pre_start and
// after the work_dir check, so a city-configured override replaces a current
// managed file on every start. The writer does not compare versions. Later
// pack directories win on the same path.
func stagePackOverlays(workDir string, dirs []string, providerName string) error {
	if workDir == "" || providerName == "" {
		return nil
	}
	dest, err := newStageDest(workDir)
	if err != nil {
		return fmt.Errorf("stage pack overlays: %w", err)
	}
	defer func() { _ = dest.close() }()
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
		if err := copyTreeInto(dest, src, ".", nil); err != nil {
			return fmt.Errorf("stage pack overlay %s: %w", src, err)
		}
	}
	return nil
}

// stageOverlayDir stages one gc `overlay_dir` into workDir: its universal
// tree (minus the top-level per-provider and .gc subtrees), then its
// per-provider/<provider> subtree flattened into workDir. This mirrors
// herdr-core's Stager.overlay (herdrcore/staging.py:225-246); unlike gc it
// overwrites rather than JSON-merging the managed hook/settings files, the
// same limitation stagePackOverlays already has. A source that is not a
// directory is a failed start, as both gc and herdr-core treat it.
func stageOverlayDir(workDir, dir, providerName string) error {
	if workDir == "" || dir == "" {
		return nil
	}
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stage overlay %s: %w", dir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("stage overlay %s: not a directory", dir)
	}
	dest, err := newStageDest(workDir)
	if err != nil {
		return fmt.Errorf("stage overlay %s: %w", dir, err)
	}
	defer func() { _ = dest.close() }()
	universal := func(rel string) bool {
		top, _, _ := strings.Cut(rel, string(filepath.Separator))
		return top == perProviderDir || top == runtimeMirrorDir
	}
	if err := copyTreeInto(dest, dir, ".", universal); err != nil {
		return fmt.Errorf("stage overlay %s: %w", dir, err)
	}
	if providerName == "" || strings.ContainsRune(providerName, filepath.Separator) || providerName == "." || providerName == ".." {
		return nil
	}
	sub := filepath.Join(dir, perProviderDir, providerName)
	if subInfo, err := os.Stat(sub); err != nil || !subInfo.IsDir() {
		return nil
	}
	onlyMirror := func(rel string) bool {
		top, _, _ := strings.Cut(rel, string(filepath.Separator))
		return top == runtimeMirrorDir
	}
	if err := copyTreeInto(dest, sub, ".", onlyMirror); err != nil {
		return fmt.Errorf("stage overlay %s: %w", sub, err)
	}
	return nil
}

// stageCopyFiles stages each copy_files entry into workDir before pre_start. A
// source that does not exist is skipped, matching gc's StagePath; a
// destination that would leave workDir is refused with a warning, not a failed
// start, matching herdr-core's staging warning
// (herdrcore/staging.py:250-251). A source that is neither a regular file nor a
// directory is an error, like an overlay symlink on this side.
func stageCopyFiles(workDir string, files []CopyFile, warn func(string)) error {
	if workDir == "" || len(files) == 0 {
		return nil
	}
	dest, err := newStageDest(workDir)
	if err != nil {
		return fmt.Errorf("stage copy_files: %w", err)
	}
	defer func() { _ = dest.close() }()
	for _, entry := range files {
		if entry.Src == "" {
			continue
		}
		info, err := os.Lstat(entry.Src)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("stage copy_files %s: %w", entry.Src, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("stage copy_files %s: refusing symlink", entry.Src)
		}
		clean, ok := relBelow(entry.RelDst)
		if !ok {
			if warn != nil {
				warn(fmt.Sprintf("did not stage %s: %s leaves the work directory", entry.Src, entry.RelDst))
			}
			continue
		}
		if info.IsDir() {
			// A directory merges into its destination; with no rel_dst that is
			// the work directory itself (herdrcore/staging.py:216-233). A
			// source that already is the destination is skipped, so staging a
			// work directory onto itself is a no-op, not a walk of its own
			// tree (gc v1.5.1 internal/runtime/staging.go:110-124,331-341).
			if sameFile(entry.Src, filepath.Join(workDir, clean)) {
				continue
			}
			if err := copyTreeInto(dest, entry.Src, clean, nil); err != nil {
				return fmt.Errorf("stage copy_files %s: %w", entry.Src, err)
			}
			continue
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("stage copy_files %s: unsupported file type", entry.Src)
		}
		// A file lands at rel_dst, or inside it when that names a directory,
		// and under its base name with no rel_dst, as gc's StagePath does.
		if clean == "." {
			clean = filepath.Base(entry.Src)
		} else if dest.isDir(clean) {
			clean = filepath.Join(clean, filepath.Base(entry.Src))
		}
		if sameFile(entry.Src, filepath.Join(workDir, clean)) {
			continue
		}
		if err := dest.writeFile(clean, entry.Src, info.Mode().Perm()); err != nil {
			return fmt.Errorf("stage copy_files %s: %w", entry.Src, err)
		}
	}
	return nil
}

// sameFile reports whether two paths name the same file on disk, as gc's
// sameFile does (gc v1.5.1 internal/runtime/staging.go:331-341): a copy_files
// entry whose source already is its destination is skipped.
func sameFile(a, b string) bool {
	ai, err := os.Stat(a)
	if err != nil {
		return false
	}
	bi, err := os.Stat(b)
	if err != nil {
		return false
	}
	return os.SameFile(ai, bi)
}

// relBelow reports rel as a clean path below the work directory, and whether
// it stays there. A leading separator is stripped, so an absolute rel lands
// below the work dir, as gc's StagePath and herdr-core's _parts do
// (herdrcore/staging.py:52-58); the work directory itself is ".".
func relBelow(rel string) (string, bool) {
	rel = strings.TrimLeft(filepath.FromSlash(rel), string(filepath.Separator))
	clean := filepath.Clean(rel)
	if clean == "" || clean == "." {
		return ".", true
	}
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", false
	}
	return clean, true
}

// stageDest writes staging output inside one work directory: it never writes
// outside it and never through a symlink, mirroring herdr-core's
// component-at-a-time O_NOFOLLOW staging (herdrcore/staging.py:17,68-129).
// os.Root enforces the boundary in the kernel; the explicit Lstat walk turns a
// destination symlink into an error instead of a followed link.
//
// A stageDest lives for one staging pass. It remembers each directory it has
// verified (no symlink components) or created, so a directory holding many
// files is checked once, not once per file. The cache holds only directories
// this pass vetted; a path it has not seen is always checked in full. Source
// trees never carry symlinks (copyTreeInto refuses them), so nothing in a pass
// can plant one; a concurrent writer racing the work directory is outside what
// the per-file Lstat walk covered either (it was check-then-use too), and
// os.Root still keeps every operation inside the work directory.
type stageDest struct {
	root *os.Root
	// dirs holds work-dir-relative directories already vetted in this pass.
	dirs map[string]struct{}
}

// newStageDest opens workDir as the staging root, creating it if needed.
func newStageDest(workDir string) (*stageDest, error) {
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(workDir)
	if err != nil {
		return nil, err
	}
	return &stageDest{root: root, dirs: make(map[string]struct{})}, nil
}

func (d *stageDest) close() error { return d.root.Close() }

// rejectSymlink fails when any existing component of rel is a symlink.
func (d *stageDest) rejectSymlink(rel string) error {
	_, err := d.lstatChecked(rel)
	return err
}

// lstatChecked walks rel one component at a time and fails when an existing
// component is a symlink. It returns the Lstat of the final component, or nil
// when that component does not exist. Directories vetted earlier in the pass
// are skipped, so only components not seen before cost a syscall.
func (d *stageDest) lstatChecked(rel string) (os.FileInfo, error) {
	if rel == "" || rel == "." {
		return nil, nil
	}
	cur := "."
	var info os.FileInfo
	parts := strings.Split(rel, string(filepath.Separator))
	for i, part := range parts {
		if part == "" || part == "." {
			continue
		}
		cur = filepath.Join(cur, part)
		// The final component is always checked: callers need its type, and a
		// vetted directory at a file's path must still read as a non-file.
		if _, ok := d.dirs[cur]; ok && i < len(parts)-1 {
			continue
		}
		var err error
		info, err = d.root.Lstat(cur)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil, nil
			}
			return nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("%s is a symlink or not a directory", cur)
		}
	}
	return info, nil
}

// markDirs records rel and every ancestor as vetted directories.
func (d *stageDest) markDirs(rel string) {
	cur := "."
	for part := range strings.SplitSeq(rel, string(filepath.Separator)) {
		if part == "" || part == "." {
			continue
		}
		cur = filepath.Join(cur, part)
		d.dirs[cur] = struct{}{}
	}
}

// mkdirAll creates rel inside the root, refusing to pass through a symlink.
func (d *stageDest) mkdirAll(rel string) error {
	if rel == "" || rel == "." {
		return nil
	}
	if _, ok := d.dirs[filepath.Clean(rel)]; ok {
		return nil
	}
	if err := d.rejectSymlink(rel); err != nil {
		return err
	}
	if err := d.root.MkdirAll(rel, 0o755); err != nil {
		return err
	}
	d.markDirs(rel)
	return nil
}

// isDir reports whether rel is an existing directory in the root.
func (d *stageDest) isDir(rel string) bool {
	if rel == "" {
		return true
	}
	info, err := d.root.Stat(rel)
	return err == nil && info.IsDir()
}

// writeFile copies src to rel with mode, creating parents, and refuses a
// symlink or a non-regular destination, as herdr-core does.
func (d *stageDest) writeFile(rel, src string, mode os.FileMode) error {
	if rel == "" || rel == "." {
		return fmt.Errorf("cannot stage %s: no destination", src)
	}
	if err := d.mkdirAll(filepath.Dir(rel)); err != nil {
		return err
	}
	info, err := d.lstatChecked(rel)
	if err != nil {
		return err
	}
	if info != nil && !info.Mode().IsRegular() {
		return fmt.Errorf("cannot stage %s over %s: not a regular file", src, rel)
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return d.root.WriteFile(rel, data, mode)
}

// copyTreeInto copies the regular files under src into dest at rel, preserving
// each file's permission bits. The destination is written through dest, so
// nothing lands outside the work directory. A skip predicate prunes a relative
// path and everything below it. Source symlinks and special files are refused
// so an overlay cannot smuggle a link into the work directory.
func copyTreeInto(dest *stageDest, src, rel string, skip func(rel string) bool) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		sub, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if skip != nil && sub != "." && skip(sub) {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		target := rel
		if sub != "." {
			target = filepath.Join(rel, sub)
		}
		if d.IsDir() {
			return dest.mkdirAll(target)
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing symlink %s", path)
		}
		if !d.Type().IsRegular() {
			return fmt.Errorf("unsupported file type %s", path)
		}
		mode := os.FileMode(0o644)
		if info, err := d.Info(); err == nil {
			mode = info.Mode().Perm()
		}
		return dest.writeFile(target, path, mode)
	})
}
