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

// releaseAsset is a stand-in provider that answers the ops the doctor check
// reads, with the pack version stamped in.
func releaseAsset(version string) string {
	return fmt.Sprintf(`#!/bin/sh
case "$1" in
  protocol) printf '%%s\n' '{"version":0,"capabilities":["report-attachment"]}' ;;
  version) printf '%%s\n' '{"version":"%s"}' ;;
esac
`, version)
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
	for _, d := range []string{checkDir, filepath.Join(packDir, "bin"), home, binDir, releaseDir} {
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
	libBytes, err := os.ReadFile(filepath.Join("..", "..", "pack", "bin", "_lib.sh"))
	if err != nil {
		t.Fatalf("read _lib.sh: %v", err)
	}
	writeExecutable(t, filepath.Join(packDir, "bin", "_lib.sh"), libBytes)
	if err := os.WriteFile(filepath.Join(packDir, "pack.toml"),
		[]byte("[pack]\nname = \"rex\"\nversion = \""+version+"\"\nschema = 2\n"), 0o644); err != nil {
		t.Fatalf("write pack.toml: %v", err)
	}

	asset := "gc-runtime-rex_" + runtime.GOOS + "_" + runtime.GOARCH
	body := []byte(releaseAsset(version))
	writeExecutable(t, filepath.Join(releaseDir, asset), body)
	sum := sha256.Sum256(body)
	checksums := hex.EncodeToString(sum[:]) + "  " + asset + "\n"
	if err := os.WriteFile(filepath.Join(releaseDir, "checksums.txt"), []byte(checksums), 0o644); err != nil {
		t.Fatalf("write checksums.txt: %v", err)
	}
	return packDir, home, binDir, releaseDir
}

// cacheBinPath is the version-keyed cache binary fix.sh must warm, laid out as
// pack/bin/_lib.sh lays it out: $HOME/.cache/rex-gascity/<version>/<os>-<arch>.
func cacheBinPath(t *testing.T, home, version string) string {
	t.Helper()
	return filepath.Join(home, ".cache", "rex-gascity", version,
		runtime.GOOS+"-"+runtime.GOARCH, "gc-runtime-rex")
}

// isolatedPath puts binDir first and then a shim directory holding only the
// tools the fix needs. A real /usr/bin would let a host gc-runtime-rex decide
// the install destination, and the fix warms the version-keyed cache under HOME.
func isolatedPath(t *testing.T, binDir string) string {
	t.Helper()
	shim := t.TempDir()
	for _, name := range []string{
		"sh", "uname", "dirname", "mktemp", "awk", "cut", "cat", "mkdir",
		"cp", "chmod", "mv", "rm", "sed", "sha256sum", "shasum", "date", "sleep",
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
// these cases need no Go toolchain: curl succeeds, curl 404 falls back to gh,
// and a wrong checksum aborts before anything is installed.
func TestRexBinaryDoctorFix(t *testing.T) {
	t.Run("installs the release asset", func(t *testing.T) {
		packDir, home, binDir, releaseDir := fixLayout(t, "0.1.0")
		out := runFix(t, filepath.Join(packDir, "doctor", "rex-binary", "fix.sh"), packDir, map[string]string{
			"PATH":           isolatedPath(t, binDir),
			"HOME":           home,
			"XDG_CACHE_HOME": filepath.Join(home, ".cache"),
			"FAKE_RELEASE":   releaseDir,
			"FAKE_HTTP":      "200",
		}, 0)
		bin := cacheBinPath(t, home, "0.1.0")
		if _, err := os.Stat(bin); err != nil {
			t.Fatalf("cache binary not installed at %s: %v\noutput:\n%s", bin, err, out)
		}
		version, err := exec.Command(bin, "version").Output()
		if err != nil {
			t.Fatalf("run installed binary: %v", err)
		}
		if got := strings.TrimSpace(string(version)); !strings.Contains(got, "0.1.0") {
			t.Fatalf("installed binary reports %s, want the pack version", got)
		}
	})

	t.Run("honors a decimal deadline override with leading zeros", func(t *testing.T) {
		packDir, home, binDir, releaseDir := fixLayout(t, "0.1.0")
		out := runFix(t, filepath.Join(packDir, "doctor", "rex-binary", "fix.sh"), packDir, map[string]string{
			"PATH":                        isolatedPath(t, binDir),
			"HOME":                        home,
			"XDG_CACHE_HOME":              filepath.Join(home, ".cache"),
			"FAKE_RELEASE":                releaseDir,
			"FAKE_HTTP":                   "200",
			"GC_RUNTIME_INSTALL_DEADLINE": "0308",
		}, 0)
		if _, err := os.Stat(cacheBinPath(t, home, "0.1.0")); err != nil {
			t.Fatalf("leading-zero override did not warm the cache: %v\noutput:\n%s", err, out)
		}
	})

	t.Run("falls back to gh when curl cannot read the release", func(t *testing.T) {
		packDir, home, binDir, releaseDir := fixLayout(t, "0.1.0")
		writeExecutable(t, filepath.Join(binDir, "gh"), []byte(fakeGH))
		out := runFix(t, filepath.Join(packDir, "doctor", "rex-binary", "fix.sh"), packDir, map[string]string{
			"PATH":           isolatedPath(t, binDir),
			"HOME":           home,
			"XDG_CACHE_HOME": filepath.Join(home, ".cache"),
			"FAKE_RELEASE":   releaseDir,
			"FAKE_HTTP":      "404",
		}, 0)
		bin := cacheBinPath(t, home, "0.1.0")
		if _, err := os.Stat(bin); err != nil {
			t.Fatalf("gh fallback did not warm the cache: %v\noutput:\n%s", err, out)
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
			"PATH":           isolatedPath(t, binDir),
			"HOME":           home,
			"XDG_CACHE_HOME": filepath.Join(home, ".cache"),
			"FAKE_RELEASE":   releaseDir,
			"FAKE_HTTP":      "200",
		}, 1)
		if !strings.Contains(out, "sha256") {
			t.Fatalf("output %q does not name the checksum failure", out)
		}
		if _, err := os.Stat(cacheBinPath(t, home, "0.1.0")); err == nil {
			t.Fatal("a binary was installed despite a checksum failure")
		}
	})
}

// envWithout drops every KEY=... entry so an override replaces the inherited
// value instead of becoming a duplicate the child may resolve the wrong way.
func envWithout(env []string, key string) []string {
	kept := env[:0:0]
	for _, e := range env {
		if !strings.HasPrefix(e, key+"=") {
			kept = append(kept, e)
		}
	}
	return kept
}

// runFix executes fix.sh from the pack directory, as gc does
// (internal/doctor/pack_checks.go PackScriptCheck.Fix: exec.Command(script),
// cmd.Dir = pack dir), and returns combined stdout and stderr.
func runFix(t *testing.T, script, packDir string, extra map[string]string, wantCode int) string {
	t.Helper()
	cmd := exec.Command(script)
	cmd.Dir = packDir
	cmd.Env = os.Environ()
	for k, v := range extra {
		cmd.Env = append(envWithout(cmd.Env, k), k+"="+v)
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
