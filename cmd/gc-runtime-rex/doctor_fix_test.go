package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// testProvider is the real provider the release stand-in delegates to. The
// fix path itself needs no toolchain; the test needs a working inventory and
// observe to prove the installed shim delegates instead of answering exit 2.
var testProvider, testRex string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "gc-runtime-rex-fix-test")
	if err != nil {
		fmt.Fprintf(os.Stderr, "mkdir: %v\n", err)
		os.Exit(1)
	}
	testProvider = filepath.Join(dir, "gc-runtime-rex")
	testRex = filepath.Join(dir, "rex")
	if out, err := exec.Command("go", "build", "-o", testRex, "../../internal/rex/fakerex/fakebinary").CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "build fake rex: %v\n%s", err, out)
		os.RemoveAll(dir)
		os.Exit(1)
	}
	if out, err := exec.Command("go", "build", "-o", testProvider, ".").CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "build provider: %v\n%s", err, out)
		os.RemoveAll(dir)
		os.Exit(1)
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// fakeCurl serves a release directory: curl -sSL -o <out> -w <fmt> <url>.
// FAKE_HTTP != 200 fails the transfer, standing in for a private repository.
const fakeCurl = `#!/bin/sh
out= url=
while [ $# -gt 0 ]; do
  case "$1" in
    -o) out=$2; shift 2 ;;
    -w|--retry) shift 2 ;;
    -*) shift ;;
    *) url=$1; shift ;;
  esac
done
code=${FAKE_HTTP:-200}
if [ "$code" != 200 ]; then
  printf '%s' "$code"
  exit 22
fi
cp "$FAKE_RELEASE/${url##*/}" "$out" || exit 1
printf '%s' 200
`

// fakeGH serves the same release through gh release download.
const fakeGH = `#!/bin/sh
dir= names=
while [ $# -gt 0 ]; do
  case "$1" in
    -R) shift 2 ;;
    -p|--pattern) names="$names $2"; shift 2 ;;
    --dir) dir=$2; shift 2 ;;
    --clobber) shift ;;
    *) shift ;;
  esac
done
for n in $names; do
  cp "$FAKE_RELEASE/$n" "$dir/$n" || exit 1
done
`

// stubAsset answers only the ops the doctor check reads and exits 2 for
// everything else: the shape of the shim that broke the supervisor.
func stubAsset(version string) string {
	return fmt.Sprintf(`#!/bin/sh
case "$1" in
  protocol) printf '%%s\n' '{"version":0,"capabilities":["report-attachment"]}' ;;
  version) printf '%%s\n' '{"version":"%s"}' ;;
  *) exit 2 ;;
esac
`, version)
}

// releaseAsset is a stand-in release: it answers protocol and version itself
// and delegates everything else to the real provider, so the installed shim
// still serves inventory and observe.
func releaseAsset(version, provider string) string {
	return fmt.Sprintf(`#!/bin/sh
case "$1" in
  protocol) printf '%%s\n' '{"version":0,"capabilities":["report-attachment"]}' ;;
  version) printf '%%s\n' '{"version":"%s"}' ;;
  *) exec %q "$@" ;;
esac
`, version, provider)
}

// writeRelease writes one release asset and a checksums.txt for it.
func writeRelease(t *testing.T, releaseDir, asset, body string) {
	t.Helper()
	writeExecutable(t, filepath.Join(releaseDir, asset), []byte(body))
	sum := sha256.Sum256([]byte(body))
	checksums := hex.EncodeToString(sum[:]) + "  " + asset + "\n"
	if err := os.WriteFile(filepath.Join(releaseDir, "checksums.txt"), []byte(checksums), 0o644); err != nil {
		t.Fatalf("write checksums.txt: %v", err)
	}
}

// fixLayout builds the pack layout gc uses: pack/pack.toml, pack/install.sh and
// pack/doctor/rex-binary/fix.sh, plus a fake bin directory and release.
func fixLayout(t *testing.T, version string) (packDir, home, binDir, releaseDir string) {
	t.Helper()
	root := t.TempDir()
	packDir = filepath.Join(root, "pack")
	checkDir := filepath.Join(packDir, "doctor", "rex-binary")
	home = filepath.Join(root, "home")
	binDir = filepath.Join(root, "bin")
	releaseDir = filepath.Join(root, "release")
	for _, d := range []string{checkDir, home, binDir, releaseDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}
	installBytes, err := os.ReadFile(filepath.Join("..", "..", "pack", "install.sh"))
	if err != nil {
		t.Fatalf("read install.sh: %v", err)
	}
	fixBytes, err := os.ReadFile(filepath.Join("..", "..", "pack", "doctor", "rex-binary", "fix.sh"))
	if err != nil {
		t.Fatalf("read fix.sh: %v", err)
	}
	writeExecutable(t, filepath.Join(packDir, "install.sh"), installBytes)
	writeExecutable(t, filepath.Join(checkDir, "fix.sh"), fixBytes)
	writeExecutable(t, filepath.Join(binDir, "curl"), []byte(fakeCurl))
	if err := os.WriteFile(filepath.Join(packDir, "pack.toml"),
		[]byte("[pack]\nname = \"rex\"\nversion = \""+version+"\"\nschema = 2\n"), 0o644); err != nil {
		t.Fatalf("write pack.toml: %v", err)
	}

	asset := "gc-runtime-rex_" + runtime.GOOS + "_" + runtime.GOARCH
	writeRelease(t, releaseDir, asset, releaseAsset(version, testProvider))
	return packDir, home, binDir, releaseDir
}

// isolatedPath puts binDir first and then a shim directory holding only the
// tools the fix needs. A real /usr/bin would let a host gc-runtime-rex decide
// the install destination. The test expects ~/.local/bin.
func isolatedPath(t *testing.T, binDir string) string {
	t.Helper()
	shim := t.TempDir()
	for _, name := range []string{
		"sh", "uname", "dirname", "mktemp", "awk", "cut", "cat", "mkdir",
		"cp", "chmod", "mv", "rm", "sed", "sha256sum", "shasum",
	} {
		src, err := exec.LookPath(name)
		if err != nil {
			continue
		}
		if err := os.Symlink(src, filepath.Join(shim, name)); err != nil {
			t.Fatalf("symlink %s: %v", name, err)
		}
	}
	return binDir + ":" + shim
}

// TestRexBinaryDoctorFix drives the real fix.sh the way gc does: the script
// itself, cwd at the pack directory. The fix installs the release asset, so
// these cases exercise no Go toolchain in the fix: curl succeeds, curl 404
// falls back to gh, and a wrong checksum aborts before anything is installed.
func TestRexBinaryDoctorFix(t *testing.T) {
	// The fix writes into $HOME/.local/bin. A duplicate HOME in the child
	// environment once sent it to the developer's real home, so prove that
	// directory is untouched.
	realBin := filepath.Join(os.Getenv("HOME"), ".local", "bin")
	before := snapshotBinDir(t, realBin)
	t.Cleanup(func() {
		if after := snapshotBinDir(t, realBin); after != before {
			t.Errorf("the fix changed the real %s:\nbefore:\n%s\nafter:\n%s", realBin, before, after)
		}
	})
	t.Run("installs the release asset", func(t *testing.T) {
		packDir, home, binDir, releaseDir := fixLayout(t, "0.1.0")
		out := runFix(t, filepath.Join(packDir, "doctor", "rex-binary", "fix.sh"), packDir, map[string]string{
			"PATH":         isolatedPath(t, binDir),
			"HOME":         home,
			"FAKE_RELEASE": releaseDir,
			"FAKE_HTTP":    "200",
		}, 0)
		bin := filepath.Join(home, ".local", "bin", "gc-runtime-rex")
		if _, err := os.Stat(bin); err != nil {
			t.Fatalf("binary not installed at %s: %v\noutput:\n%s", bin, err, out)
		}
		version, err := exec.Command(bin, "version").Output()
		if err != nil {
			t.Fatalf("run installed binary: %v", err)
		}
		if got := strings.TrimSpace(string(version)); !strings.Contains(got, "0.1.0") {
			t.Fatalf("installed binary reports %s, want the pack version", got)
		}
		// The installed shim must serve the ops the supervisor calls every
		// cycle, not only the two the doctor check reads.
		if got := runOp(t, bin, "inventory"); !strings.Contains(got, `"complete":true`) {
			t.Fatalf("inventory through the installed shim = %q", got)
		}
		if got := runOp(t, bin, "observe", "probe-session"); !strings.Contains(got, `"state":`) {
			t.Fatalf("observe through the installed shim = %q", got)
		}
	})

	t.Run("falls back to gh when curl cannot read the release", func(t *testing.T) {
		packDir, home, binDir, releaseDir := fixLayout(t, "0.1.0")
		writeExecutable(t, filepath.Join(binDir, "gh"), []byte(fakeGH))
		out := runFix(t, filepath.Join(packDir, "doctor", "rex-binary", "fix.sh"), packDir, map[string]string{
			"PATH":         isolatedPath(t, binDir),
			"HOME":         home,
			"FAKE_RELEASE": releaseDir,
			"FAKE_HTTP":    "404",
		}, 0)
		bin := filepath.Join(home, ".local", "bin", "gc-runtime-rex")
		if _, err := os.Stat(bin); err != nil {
			t.Fatalf("gh fallback did not install the provider: %v\noutput:\n%s", err, out)
		}
	})

	t.Run("a wrong checksum aborts", func(t *testing.T) {
		packDir, home, binDir, releaseDir := fixLayout(t, "0.1.0")
		asset := "gc-runtime-rex_" + runtime.GOOS + "_" + runtime.GOARCH
		bad := "0000000000000000000000000000000000000000000000000000000000000000  " + asset + "\n"
		if err := os.WriteFile(filepath.Join(releaseDir, "checksums.txt"), []byte(bad), 0o644); err != nil {
			t.Fatalf("write checksums.txt: %v", err)
		}
		out := runFix(t, filepath.Join(packDir, "doctor", "rex-binary", "fix.sh"), packDir, map[string]string{
			"PATH":         isolatedPath(t, binDir),
			"HOME":         home,
			"FAKE_RELEASE": releaseDir,
			"FAKE_HTTP":    "200",
		}, 1)
		if !strings.Contains(out, "sha256") {
			t.Fatalf("output %q does not name the checksum failure", out)
		}
		if _, err := os.Stat(filepath.Join(home, ".local", "bin", "gc-runtime-rex")); err == nil {
			t.Fatal("a binary was installed despite a checksum failure")
		}
	})

	t.Run("a stub asset does not replace a working provider", func(t *testing.T) {
		packDir, home, binDir, releaseDir := fixLayout(t, "0.1.0")
		bin := filepath.Join(home, ".local", "bin", "gc-runtime-rex")
		if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		writeExecutable(t, bin, []byte(releaseAsset("0.1.0", testProvider)))
		asset := "gc-runtime-rex_" + runtime.GOOS + "_" + runtime.GOARCH
		writeRelease(t, releaseDir, asset, stubAsset("0.1.0"))
		out := runFix(t, filepath.Join(packDir, "doctor", "rex-binary", "fix.sh"), packDir, map[string]string{
			"PATH":         isolatedPath(t, binDir),
			"HOME":         home,
			"FAKE_RELEASE": releaseDir,
			"FAKE_HTTP":    "200",
		}, 1)
		if !strings.Contains(out, "refusing to replace") {
			t.Fatalf("output %q does not report the conflict", out)
		}
		if got := runOp(t, bin, "inventory"); !strings.Contains(got, `"complete":true`) {
			t.Fatalf("the working provider was replaced: inventory = %q", got)
		}
	})
}

// runOp runs a provider with a fake rex on PATH and a clean environment, so
// inventory and observe answer without the test host's real state.
func runOp(t *testing.T, bin string, args ...string) string {
	t.Helper()
	rexDir := t.TempDir()
	if err := os.Symlink(testRex, filepath.Join(rexDir, "rex")); err != nil {
		t.Fatalf("symlink rex: %v", err)
	}
	cmd := exec.Command(bin, args...)
	cmd.Env = []string{
		"HOME=" + t.TempDir(),
		"PATH=" + rexDir,
		"FAKEREX_STATE=" + t.TempDir(),
	}
	out, err := cmd.CombinedOutput()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatalf("run %s: %v", bin, err)
	}
	return fmt.Sprintf("exit %d\n%s", code, out)
}

// snapshotBinDir records every entry in dir with its mode and content hash.
func snapshotBinDir(t *testing.T, dir string) string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return ""
	}
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	lines := make([]string, 0, len(entries))
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			t.Fatalf("stat %s: %v", e.Name(), err)
		}
		line := e.Name() + " " + info.Mode().String()
		if info.Mode().IsRegular() {
			data, err := os.ReadFile(filepath.Join(dir, e.Name()))
			if err != nil {
				t.Fatalf("read %s: %v", e.Name(), err)
			}
			sum := sha256.Sum256(data)
			line += " " + hex.EncodeToString(sum[:])
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

// runFix executes fix.sh from the pack directory, as gc does
// (internal/doctor/pack_checks.go PackScriptCheck.Fix: exec.Command(script),
// cmd.Dir = pack dir), and returns combined stdout and stderr.
func runFix(t *testing.T, script, packDir string, extra map[string]string, wantCode int) string {
	t.Helper()
	cmd := exec.Command(script)
	cmd.Dir = packDir
	// A clean environment: the host's REX_* variables point at a live session,
	// and the provider would call that session's API instead of answering from
	// local state. The installer needs only what extra names.
	for k, v := range extra {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	out, err := cmd.CombinedOutput()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatalf("run fix: %v\n%s", err, out)
	}
	if code != wantCode {
		t.Fatalf("exit = %d, want %d\noutput:\n%s", code, wantCode, out)
	}
	return string(out)
}
