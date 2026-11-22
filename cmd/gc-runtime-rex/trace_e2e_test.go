package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/docbobo/rex-gascity/internal/rex"
	"github.com/docbobo/rex-gascity/internal/rex/cli"
	"github.com/docbobo/rex-gascity/internal/rex/fakerex"
)

// traceRecord is one parsed JSONL trace line (REQ-004). The fields stay
// pointers so a missing duration_us or exit_code fails the test instead of
// reading as zero.
type traceRecord struct {
	TS         string `json:"ts"`
	Op         string `json:"op"`
	Method     string `json:"method"`
	DurationUS *int64 `json:"duration_us"`
	Outcome    string `json:"outcome"`
	ExitCode   *int   `json:"exit_code"`
	City       string `json:"city"`
	Session    string `json:"session"`
	Agent      string `json:"agent"`
	V          int    `json:"v"`
}

func readTrace(t *testing.T, path string) []traceRecord {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read trace %s: %v", path, err)
	}
	var lines []traceRecord
	for _, raw := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var l traceRecord
		if err := json.Unmarshal([]byte(raw), &l); err != nil {
			t.Fatalf("trace line %q: %v", raw, err)
		}
		lines = append(lines, l)
	}
	return lines
}

// TestE2ETraceStartAndPeek is the REQ-010 proof: against the fake rex, a start
// and a peek each append one correct line to the GC_REX_TRACE path, and with
// the switch unset no trace file appears.
func TestE2ETraceStartAndPeek(t *testing.T) {
	fake := fakerex.New(t)
	client := &cli.Client{Bin: fake.Bin}
	t.Setenv("GC_REX_STATE_DIR", t.TempDir())
	t.Setenv("GC_REX_CITY", "tracecity")
	d := deps{newClient: func() rex.Client { return client }}

	dir := t.TempDir()
	tracePath := filepath.Join(dir, "trace.jsonl")

	// Off by default: no file, no line.
	t.Setenv("GC_REX_TRACE", "")
	var stdout, stderr strings.Builder
	if code := run([]string{"list-running"}, strings.NewReader(""), &stdout, &stderr, d); code != exitOK {
		t.Fatalf("list-running: exit %d, stderr %q", code, stderr.String())
	}
	if entries, err := os.ReadDir(dir); err != nil || len(entries) != 0 {
		t.Fatalf("trace off: dir %s not empty (%v, %v)", dir, entries, err)
	}

	// On: start then peek.
	t.Setenv("GC_REX_TRACE", tracePath)
	const dog = "Indigo--dog-1"
	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"start", dog}, strings.NewReader(`{"command":"sleep 300"}`), &stdout, &stderr, d); code != exitOK {
		t.Fatalf("start: exit %d, stderr %q", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"peek", dog}, strings.NewReader(""), &stdout, &stderr, d); code != exitOK {
		t.Fatalf("peek: exit %d, stderr %q", code, stderr.String())
	}

	lines := readTrace(t, tracePath)
	if len(lines) != 2 {
		t.Fatalf("want 2 trace lines, got %d: %+v", len(lines), lines)
	}
	for i, want := range []struct{ op, method string }{{"start", "StartNew"}, {"peek", "PeekLines"}} {
		l := lines[i]
		if l.Op != want.op || l.Method != want.method {
			t.Errorf("line %d: op/method = %q/%q, want %q/%q", i, l.Op, l.Method, want.op, want.method)
		}
		if l.Outcome != "ok" || l.ExitCode == nil || *l.ExitCode != 0 {
			t.Errorf("line %d: outcome/exit = %q/%v, want ok/0", i, l.Outcome, l.ExitCode)
		}
		if l.DurationUS == nil || *l.DurationUS < 0 {
			t.Errorf("line %d: duration_us = %v, want integer >= 0", i, l.DurationUS)
		}
		if l.City != "tracecity" {
			t.Errorf("line %d: city = %q, want tracecity", i, l.City)
		}
		if l.Session != dog || l.Agent != "dog-1" {
			t.Errorf("line %d: session/agent = %q/%q, want %q/dog-1", i, l.Session, l.Agent, dog)
		}
		if l.TS == "" || l.V != 1 {
			t.Errorf("line %d: ts=%q v=%d, want non-empty/1", i, l.TS, l.V)
		}
	}

	// An erroring op still traces with its real exit code.
	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"peek", "Indigo--ghost-1"}, strings.NewReader(""), &stdout, &stderr, d); code == exitOK {
		t.Fatalf("peek ghost: want failure, got exit 0")
	}
	lines = readTrace(t, tracePath)
	last := lines[len(lines)-1]
	if last.Op != "peek" || last.Outcome != "error" || last.ExitCode == nil || *last.ExitCode == 0 {
		t.Errorf("error line: %+v, want peek/error/non-zero exit", last)
	}

	// A keyed op with a missing key errors in the dispatcher; tracing records
	// the error and the op's own exit code.
	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"set-meta", dog, ""}, strings.NewReader(""), &stdout, &stderr, d); code != exitFail {
		t.Fatalf("set-meta with empty key: exit %d, stderr %q", code, stderr.String())
	}
	lines = readTrace(t, tracePath)
	last = lines[len(lines)-1]
	// A pre-provider failure names the op, not the provider method (REQ-003).
	if last.Op != "set-meta" || last.Method != "set-meta" || last.Outcome != "error" || last.ExitCode == nil || *last.ExitCode != exitFail {
		t.Errorf("set-meta line: %+v, want set-meta/set-meta/error/%d", last, exitFail)
	}

	// Redaction: a planted start-config secret in the environment never
	// appears in the trace.
	t.Setenv("GC_RUNTIME_START_SECRET", "TRACE-SECRET-SENTINEL")
	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"list-running"}, strings.NewReader(""), &stdout, &stderr, d); code != exitOK {
		t.Fatalf("list-running: exit %d, stderr %q", code, stderr.String())
	}
	if raw, err := os.ReadFile(tracePath); err != nil {
		t.Fatalf("read trace: %v", err)
	} else if strings.Contains(string(raw), "TRACE-SECRET-SENTINEL") {
		t.Errorf("trace contains the planted environment secret")
	}

	// Unknown ops never reach a handler and write no line.
	before := len(readTrace(t, tracePath))
	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"nonsense-op"}, strings.NewReader(""), &stdout, &stderr, d); code != exitUnknown {
		t.Fatalf("unknown op: exit %d, want %d", code, exitUnknown)
	}
	if after := len(readTrace(t, tracePath)); after != before {
		t.Errorf("unknown op wrote a line: %d -> %d", before, after)
	}

	_ = fmt.Sprint() // keep fmt if the assertions above change
}
