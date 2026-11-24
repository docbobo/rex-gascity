package provider

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// Overlay staging runs on every session start, so its cost scales with the
// size of the configured overlay trees. These benchmarks use synthetic trees in
// b.TempDir(); they never touch a real overlay directory.

// writeBenchTree creates files regular files of size bytes spread over
// dirs sub-directories (two levels deep) under root.
func writeBenchTree(b *testing.B, root string, files, dirs, size int) {
	b.Helper()
	body := make([]byte, size)
	for i := range body {
		body[i] = byte('a' + i%26)
	}
	for i := range files {
		dir := filepath.Join(root, fmt.Sprintf("d%d", i%dirs), fmt.Sprintf("s%d", i%3))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			b.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("f%d.txt", i)), body, 0o644); err != nil {
			b.Fatal(err)
		}
	}
}

type overlayShape struct {
	name        string
	files, dirs int
}

var overlayShapes = []overlayShape{
	{"files-10", 10, 2},
	{"files-100", 100, 10},
	{"files-500", 500, 25},
}

func BenchmarkStageOverlayDir(b *testing.B) {
	for _, shape := range overlayShapes {
		src := b.TempDir()
		writeBenchTree(b, src, shape.files, shape.dirs, 1024)
		// Skipped subtrees: the top-level per-provider and .gc trees are pruned
		// from the universal copy, and the provider subtree is flattened in.
		writeBenchTree(b, filepath.Join(src, perProviderDir, "pi"), 5, 1, 1024)
		writeBenchTree(b, filepath.Join(src, runtimeMirrorDir), 5, 1, 1024)

		b.Run(shape.name+"/cold", func(b *testing.B) {
			base := b.TempDir()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				work := filepath.Join(base, fmt.Sprintf("w%d", i))
				if err := stageOverlayDir(work, src, "pi"); err != nil {
					b.Fatal(err)
				}
				b.StopTimer()
				_ = os.RemoveAll(work)
				b.StartTimer()
			}
		})
		b.Run(shape.name+"/restage", func(b *testing.B) {
			work := b.TempDir()
			if err := stageOverlayDir(work, src, "pi"); err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := stageOverlayDir(work, src, "pi"); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkStagePackOverlays(b *testing.B) {
	for _, shape := range overlayShapes {
		// Two pack directories that overlap, so the later one overwrites.
		packs := []string{b.TempDir(), b.TempDir()}
		for _, p := range packs {
			writeBenchTree(b, filepath.Join(p, perProviderDir, "pi"), shape.files, shape.dirs, 1024)
		}
		b.Run(shape.name+"/restage", func(b *testing.B) {
			work := b.TempDir()
			if err := stagePackOverlays(work, packs, "pi"); err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := stagePackOverlays(work, packs, "pi"); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkCopyTreeInto isolates the walk-and-copy loop from the Stat and
// destination-open work the two stage entry points add.
func BenchmarkCopyTreeInto(b *testing.B) {
	for _, shape := range overlayShapes {
		src := b.TempDir()
		writeBenchTree(b, src, shape.files, shape.dirs, 1024)
		b.Run(shape.name, func(b *testing.B) {
			dest, err := newStageDest(b.TempDir())
			if err != nil {
				b.Fatal(err)
			}
			defer func() { _ = dest.close() }()
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				// Every staging entry point builds a fresh stageDest, so each measured
				// copy starts with an empty directory cache. Keep the root open so
				// destination-open work stays outside the measurement.
				clear(dest.dirs)
				if err := copyTreeInto(dest, src, ".", nil); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
