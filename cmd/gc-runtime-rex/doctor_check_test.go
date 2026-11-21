package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// fakeDoctorProvider answers the two ops the rex:rex-binary check uses:
// `protocol` with an RPP v0 handshake, and `version` with FAKE_VERSION_JSON.
// FAKE_VERSION_EXIT makes `version` fail instead, standing in for an older
// binary that has no `version` op.
const fakeDoctorProvider = `#!/bin/sh
case "$1" in
  protocol)
    printf '%s\n' '{"version":0,"capabilities":["report-attachment"]}'
    exit 0
    ;;
  version)
    if [ -n "${FAKE_VERSION_EXIT:-}" ]; then
      exit "$FAKE_VERSION_EXIT"
    fi
    printf '%s\n' "$FAKE_VERSION_JSON"
    exit 0
    ;;
esac
exit 2
`

// TestRexBinaryDoctorCheckVersionComparison drives the real run.sh in a temp
// pack layout against a fake provider, pinning the version comparison: a
// matching version (with or without a leading v, and a dirty tree) exits 0, a
// determined mismatch exits 2, and an undeterminable version warns (exit 1).
func TestRexBinaryDoctorCheckVersionComparison(t *testing.T) {
	scriptBytes, err := os.ReadFile(filepath.Join("..", "..", "pack", "doctor", "rex-binary", "run.sh"))
	if err != nil {
		t.Fatalf("read doctor script: %v", err)
	}

	cases := []struct {
		name         string
		packVersion  string
		versionJSON  string
		versionExit  string
		wantCode     int
		wantContains string
		wantAlso     string
	}{
		{"matching version passes", "0.1.0", `{"version":"0.1.0"}`, "", 0, "version 0.1.0", ""},
		{"tagged v prefix matches", "0.1.0", `{"version":"v0.1.0"}`, "", 0, "version v0.1.0", ""},
		{"dirty build still matches", "0.1.0", `{"version":"v0.1.0+dirty","modified":true}`, "", 0, "modified working tree", ""},
		{"mismatch errors", "0.1.0", `{"version":"9.9.9"}`, "", 2, "9.9.9", "install.sh"},
		{"undeterminable warns", "0.1.0", `{"version":""}`, "", 1, "could not be determined", ""},
		{"missing version op warns", "0.1.0", "", "2", 1, "could not be determined", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			checkDir := filepath.Join(dir, "doctor", "rex-binary")
			if err := os.MkdirAll(checkDir, 0o755); err != nil {
				t.Fatalf("mkdir: %v", err)
			}
			writeExecutable(t, filepath.Join(checkDir, "run.sh"), scriptBytes)
			provider := filepath.Join(dir, "gc-runtime-rex")
			writeExecutable(t, provider, []byte(fakeDoctorProvider))
			rex := filepath.Join(dir, "rex")
			writeExecutable(t, rex, []byte("#!/bin/sh\nexit 0\n"))
			packToml := "[pack]\nname = \"rex\"\nversion = \"" + tc.packVersion + "\"\nschema = 2\n\n" +
				"[runtimes.rex]\ncommand = \"" + provider + "\"\nprotocol = 0\n"
			if err := os.WriteFile(filepath.Join(dir, "pack.toml"), []byte(packToml), 0o644); err != nil {
				t.Fatalf("write pack.toml: %v", err)
			}

			cmd := exec.Command("sh", filepath.Join(checkDir, "run.sh"))
			cmd.Env = append(os.Environ(),
				"GC_REX_BIN="+rex,
				"FAKE_VERSION_JSON="+tc.versionJSON,
				"FAKE_VERSION_EXIT="+tc.versionExit,
			)
			out, err := cmd.CombinedOutput()
			code := 0
			if ee, ok := err.(*exec.ExitError); ok {
				code = ee.ExitCode()
			} else if err != nil {
				t.Fatalf("run doctor check: %v\n%s", err, out)
			}
			if code != tc.wantCode {
				t.Fatalf("exit = %d, want %d\noutput:\n%s", code, tc.wantCode, out)
			}
			if tc.wantContains != "" && !strings.Contains(string(out), tc.wantContains) {
				t.Errorf("output %q does not contain %q", out, tc.wantContains)
			}
			if tc.wantAlso != "" && !strings.Contains(string(out), tc.wantAlso) {
				t.Errorf("output %q does not contain %q", out, tc.wantAlso)
			}
			if strings.Contains(string(out), "go install") {
				t.Errorf("output %q still tells the operator to build from source", out)
			}
		})
	}
}

// TestRexBinaryDoctorCheckBadRexBin pins the failing rex-CLI case: GC_REX_BIN
// names a path that is not executable, so the check must report the rex CLI
// missing and say how to fix it, not accept the bad value.
func TestRexBinaryDoctorCheckBadRexBin(t *testing.T) {
	scriptBytes, err := os.ReadFile(filepath.Join("..", "..", "pack", "doctor", "rex-binary", "run.sh"))
	if err != nil {
		t.Fatalf("read doctor script: %v", err)
	}
	dir := t.TempDir()
	checkDir := filepath.Join(dir, "doctor", "rex-binary")
	if err := os.MkdirAll(checkDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeExecutable(t, filepath.Join(checkDir, "run.sh"), scriptBytes)
	provider := filepath.Join(dir, "gc-runtime-rex")
	writeExecutable(t, provider, []byte(fakeDoctorProvider))
	packToml := "[pack]\nname = \"rex\"\nversion = \"0.1.0\"\nschema = 2\n\n" +
		"[runtimes.rex]\ncommand = \"" + provider + "\"\nprotocol = 0\n"
	if err := os.WriteFile(filepath.Join(dir, "pack.toml"), []byte(packToml), 0o644); err != nil {
		t.Fatalf("write pack.toml: %v", err)
	}

	badRex := filepath.Join(dir, "not-rex")
	cmd := exec.Command("sh", filepath.Join(checkDir, "run.sh"))
	cmd.Env = append(os.Environ(),
		"GC_REX_BIN="+badRex,
		"FAKE_VERSION_JSON={\"version\":\"0.1.0\"}",
	)
	out, err := cmd.CombinedOutput()
	ee, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("run doctor check: %v\n%s", err, out)
	}
	if ee.ExitCode() != 2 {
		t.Fatalf("exit = %d, want 2\noutput:\n%s", ee.ExitCode(), out)
	}
	for _, want := range []string{"the rex CLI is not installed", "set GC_REX_BIN to its path"} {
		if !strings.Contains(string(out), want) {
			t.Errorf("output %q does not contain %q", out, want)
		}
	}
}

func writeExecutable(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o755); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
