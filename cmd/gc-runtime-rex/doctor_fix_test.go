package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// fakeFixModule is a stand-in for this repository: a module whose
// cmd/gc-runtime-rex stamps main.buildVersion, which is what the doctor fix
// passes through -ldflags. It keeps the test fast and offline while proving
// the wiring: version parsed from pack.toml, build from the module root beside
// the pack, binary written to GOBIN.
const fakeFixModule = `package main

import "fmt"

var buildVersion = "devel"

func main() { fmt.Printf("{\"version\":%q}\n", buildVersion) }
`

// TestRexBinaryDoctorFix drives the real fix.sh the way gc does: cwd is the
// pack directory, and the module root sits one level up. It pins the two
// failure contracts (no Go toolchain, no go.mod beside the pack) and the
// success path that installs a version-stamped binary into GOBIN.
func TestRexBinaryDoctorFix(t *testing.T) {
	scriptBytes, err := os.ReadFile(filepath.Join("..", "..", "pack", "doctor", "rex-binary", "fix.sh"))
	if err != nil {
		t.Fatalf("read fix script: %v", err)
	}

	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("no Go toolchain on PATH; the success case needs one")
	}

	t.Run("installs a version-stamped binary", func(t *testing.T) {
		root := t.TempDir()
		packDir := filepath.Join(root, "pack")
		checkDir := filepath.Join(packDir, "doctor", "rex-binary")
		mainDir := filepath.Join(root, "cmd", "gc-runtime-rex")
		for _, d := range []string{checkDir, mainDir, filepath.Join(root, "out")} {
			if err := os.MkdirAll(d, 0o755); err != nil {
				t.Fatalf("mkdir %s: %v", d, err)
			}
		}
		writeExecutable(t, filepath.Join(checkDir, "fix.sh"), scriptBytes)
		if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/fake\n\ngo 1.21\n"), 0o644); err != nil {
			t.Fatalf("write go.mod: %v", err)
		}
		if err := os.WriteFile(filepath.Join(mainDir, "main.go"), []byte(fakeFixModule), 0o644); err != nil {
			t.Fatalf("write main.go: %v", err)
		}
		if err := os.WriteFile(filepath.Join(packDir, "pack.toml"), []byte("[pack]\nname = \"rex\"\nversion = \"0.1.0\"\nschema = 2\n"), 0o644); err != nil {
			t.Fatalf("write pack.toml: %v", err)
		}

		out := runFix(t, filepath.Join(checkDir, "fix.sh"), packDir, map[string]string{
			"PATH":     filepath.Dir(goBin) + ":" + os.Getenv("PATH"),
			"GOBIN":    filepath.Join(root, "out"),
			"GOCACHE":  filepath.Join(root, "gocache"),
			"GOPATH":   filepath.Join(root, "gopath"),
			"GOTMPDIR": "",
		}, 0)

		bin := filepath.Join(root, "out", "gc-runtime-rex")
		if _, err := os.Stat(bin); err != nil {
			t.Fatalf("binary not installed at %s: %v\noutput:\n%s", bin, err, out)
		}
		version, err := exec.Command(bin, "version").Output()
		if err != nil {
			t.Fatalf("run installed binary: %v", err)
		}
		if got := strings.TrimSpace(string(version)); got != `{"version":"0.1.0"}` {
			t.Fatalf("installed binary reports %s, want the pack version stamped", got)
		}
	})

	t.Run("no Go toolchain fails with a remediation", func(t *testing.T) {
		root := t.TempDir()
		packDir := filepath.Join(root, "pack")
		checkDir := filepath.Join(packDir, "doctor", "rex-binary")
		if err := os.MkdirAll(checkDir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		writeExecutable(t, filepath.Join(checkDir, "fix.sh"), scriptBytes)
		if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/fake\n"), 0o644); err != nil {
			t.Fatalf("write go.mod: %v", err)
		}
		if err := os.WriteFile(filepath.Join(packDir, "pack.toml"), []byte("[pack]\nname = \"rex\"\nversion = \"0.1.0\"\n"), 0o644); err != nil {
			t.Fatalf("write pack.toml: %v", err)
		}

		// A PATH with the shell utilities the script needs but no go.
		out := runFix(t, filepath.Join(checkDir, "fix.sh"), packDir, map[string]string{
			"PATH": "/usr/bin:/bin",
		}, 1)
		if !strings.Contains(out, "Go toolchain") {
			t.Fatalf("output %q does not name the missing Go toolchain", out)
		}
	})

	t.Run("no go.mod beside the pack fails clearly", func(t *testing.T) {
		root := t.TempDir()
		packDir := filepath.Join(root, "pack")
		checkDir := filepath.Join(packDir, "doctor", "rex-binary")
		if err := os.MkdirAll(checkDir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		writeExecutable(t, filepath.Join(checkDir, "fix.sh"), scriptBytes)
		if err := os.WriteFile(filepath.Join(packDir, "pack.toml"), []byte("[pack]\nname = \"rex\"\nversion = \"0.1.0\"\n"), 0o644); err != nil {
			t.Fatalf("write pack.toml: %v", err)
		}

		out := runFix(t, filepath.Join(checkDir, "fix.sh"), packDir, map[string]string{
			"PATH": filepath.Dir(goBin) + ":" + os.Getenv("PATH"),
		}, 1)
		if !strings.Contains(out, "go.mod") {
			t.Fatalf("output %q does not name the missing go.mod", out)
		}
	})
}

// runFix executes fix.sh from the pack directory, as gc does
// (internal/doctor/pack_checks.go `PackScriptCheck.Fix`: exec.Command(script),
// cmd.Dir = pack dir), and returns combined stdout and stderr. The child env
// is only the inherited environment plus extra, so the test controls PATH and
// the Go build cache directories.
func runFix(t *testing.T, script, packDir string, extra map[string]string, wantCode int) string {
	t.Helper()
	cmd := exec.Command(script)
	cmd.Dir = packDir
	cmd.Env = os.Environ()
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
