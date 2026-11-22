package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTraceOffWritesNothing(t *testing.T) {
	t.Setenv("GC_REX_TRACE", "")
	if tr := newTracer("c", &bytes.Buffer{}); tr != nil {
		t.Fatalf("newTracer with GC_REX_TRACE unset: got %+v, want nil", tr)
	}
}

func TestTraceLiteralPathAppends(t *testing.T) {
	path := filepath.Join(t.TempDir(), "t.jsonl")
	t.Setenv("GC_REX_TRACE", path)
	var stderr bytes.Buffer
	tr := newTracer("my-city", &stderr)
	if tr == nil {
		t.Fatalf("newTracer: nil; stderr %q", stderr.String())
	}
	tr.record(&stderr, traceLine{Op: "peek", Method: "PeekLines", Outcome: "ok"})
	tr.record(&stderr, traceLine{Op: "stop", Method: "Stop", Outcome: "error", ExitCode: 1})

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("want 2 lines, got %d: %q", len(lines), data)
	}
	var got traceLine
	if err := json.Unmarshal([]byte(lines[0]), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.City != "my-city" || got.V != 1 || got.TS == "" {
		t.Errorf("line = %+v, want city my-city v 1 ts set", got)
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want empty", stderr.String())
	}
}

func TestTraceDefaultSentinelPath(t *testing.T) {
	t.Setenv("GC_REX_STATE_DIR", t.TempDir())
	t.Setenv("GC_REX_TRACE", "1")
	var stderr bytes.Buffer
	tr := newTracer("sentinel-city", &stderr)
	if tr == nil {
		t.Fatalf("newTracer: nil; stderr %q", stderr.String())
	}
	wantDir := filepath.Join(os.Getenv("GC_REX_STATE_DIR"), "sentinel-city", "trace")
	if filepath.Dir(tr.path) != wantDir {
		t.Errorf("path dir = %q, want %q", filepath.Dir(tr.path), wantDir)
	}
	if !strings.HasPrefix(filepath.Base(tr.path), "provider-") || !strings.HasSuffix(tr.path, ".jsonl") {
		t.Errorf("path = %q, want provider-<date>.jsonl", tr.path)
	}
}

func TestTraceRejectsPipe(t *testing.T) {
	t.Setenv("GC_REX_TRACE", "/dev/null")
	var stderr bytes.Buffer
	// /dev/null is a character device: not a regular file.
	if tr := newTracer("c", &stderr); tr != nil {
		t.Fatalf("newTracer on /dev/null: got %+v, want nil", tr)
	}
	if !strings.Contains(stderr.String(), "not a regular file") {
		t.Errorf("stderr = %q, want a not-a-regular-file warning", stderr.String())
	}
}

func TestTraceCapSkipsLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "t.jsonl")
	if err := os.WriteFile(path, make([]byte, traceCap), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	t.Setenv("GC_REX_TRACE", path)
	var stderr bytes.Buffer
	tr := newTracer("c", &stderr)
	tr.record(&stderr, traceLine{Op: "peek", Method: "PeekLines", Outcome: "ok"})

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Size() != traceCap {
		t.Errorf("size = %d, want unchanged %d", info.Size(), traceCap)
	}
	if !strings.Contains(stderr.String(), "skipped") {
		t.Errorf("stderr = %q, want a cap warning", stderr.String())
	}
}

func TestTraceSessionOmission(t *testing.T) {
	for _, tc := range []struct {
		name string
		spec opSpec
		args []string
		want string
	}{
		{"named", ops["peek"], []string{"Indigo--dog-1"}, "Indigo--dog-1"},
		{"no name", ops["list-running"], []string{"web"}, ""},
		{"sentinel", ops["inventory"], []string{"objects"}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got, _ := traceSession(tc.spec, tc.args); got != tc.want {
				t.Errorf("traceSession = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestTraceRecordKeepsStartTS proves REQ-004: ts is the start of the call, so a
// caller-provided TS must survive record instead of being overwritten at append
// time.
func TestTraceRecordKeepsStartTS(t *testing.T) {
	path := filepath.Join(t.TempDir(), "t.jsonl")
	t.Setenv("GC_REX_TRACE", path)
	var stderr bytes.Buffer
	tr := newTracer("c", &stderr)
	if tr == nil {
		t.Fatalf("newTracer: nil; stderr %q", stderr.String())
	}
	const ts = "2026-11-22T12:00:00.123456789Z"
	tr.record(&stderr, traceLine{TS: ts, Op: "peek", Method: "PeekLines", Outcome: "ok"})

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var got traceLine
	if err := json.Unmarshal(bytes.TrimSpace(data), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.TS != ts {
		t.Errorf("ts = %q, want the caller-provided start %q", got.TS, ts)
	}
}
