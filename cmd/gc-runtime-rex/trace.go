package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/docbobo/rex-gascity/internal/provider"
)

// traceEnv is the on-switch. Unset or empty means off; "1" or "on" selects the
// default daily path under the city state dir; any other value is a literal
// path. Read once at process start (REQ-002).
const traceEnv = "GC_REX_TRACE"

// traceCap is the best-effort per-file size cap. Before appending, the writer
// checks size + the full next line against it and skips the line with one
// stderr warning; it never truncates, rotates or waits on another writer
// (REQ-006).
const traceCap = 128 << 20

// traceVersion is the line schema version (REQ-004).
const traceVersion = 1

// traceLine is one JSONL line. The field set is the whitelist from REQ-004:
// no environment map, no rig, no parent_op_id.
type traceLine struct {
	TS         string `json:"ts"`
	Op         string `json:"op"`
	Method     string `json:"method"`
	DurationUS int64  `json:"duration_us"`
	Outcome    string `json:"outcome"`
	ExitCode   int    `json:"exit_code"`
	City       string `json:"city"`
	Session    string `json:"session,omitempty"`
	Agent      string `json:"agent,omitempty"`
	V          int    `json:"v"`
}

// tracer appends one line per eligible op. A nil tracer means tracing is off.
type tracer struct {
	path string
	city string
}

// newTracer reads GC_REX_TRACE once. It returns nil when tracing is off. A
// destination it cannot use as a regular file (a pipe, a directory) is
// rejected with one stderr warning and tracing stays off; the op runs
// untraced (REQ-006).
func newTracer(city string, stderr io.Writer) *tracer {
	path := os.Getenv(traceEnv)
	if path == "" {
		return nil
	}
	if path == "1" || path == "on" {
		dir, err := provider.StateDir(city)
		if err != nil {
			fmt.Fprintf(stderr, "gc-runtime-rex: trace: %v\n", err)
			return nil
		}
		path = filepath.Join(dir, "trace", "provider-"+time.Now().UTC().Format("2006-01-02")+".jsonl")
	}
	if !regularDestination(path) {
		fmt.Fprintf(stderr, "gc-runtime-rex: trace: %s is not a regular file; tracing disabled\n", path)
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		fmt.Fprintf(stderr, "gc-runtime-rex: trace: %v\n", err)
		return nil
	}
	return &tracer{path: path, city: city}
}

// regularDestination reports whether path can be appended to as a regular
// file. A missing file is fine; an existing non-regular file is not.
func regularDestination(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return os.IsNotExist(err)
	}
	return info.Mode().IsRegular()
}

// record appends one line for an eligible op and swallows every failure with
// one stderr warning, so tracing never changes the op's outcome (REQ-007).
// The caller times the span; a nil tracer records nothing.
func (t *tracer) record(stderr io.Writer, l traceLine) {
	if t == nil {
		return
	}
	l.TS = time.Now().UTC().Format(time.RFC3339Nano)
	l.City = t.city
	l.V = traceVersion
	data, err := json.Marshal(l)
	if err != nil {
		fmt.Fprintf(stderr, "gc-runtime-rex: trace: encode: %v\n", err)
		return
	}
	data = append(data, '\n')
	f, err := os.OpenFile(t.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		fmt.Fprintf(stderr, "gc-runtime-rex: trace: open: %v\n", err)
		return
	}
	defer func() { _ = f.Close() }()
	// The size check is unlocked: two writers can both pass and overshoot by
	// one line each. That is the accepted best-effort bound (REQ-006).
	if info, err := f.Stat(); err == nil && info.Size()+int64(len(data)) > traceCap {
		fmt.Fprintf(stderr, "gc-runtime-rex: trace: %s is at its %d MiB cap; line skipped\n", t.path, traceCap>>20)
		return
	}
	if _, err := f.Write(data); err != nil {
		fmt.Fprintf(stderr, "gc-runtime-rex: trace: write: %v\n", err)
	}
}

// traceSession returns the op's session and agent fields. The session is the
// op's name argument. An op that takes no name (list-running's prefix) and an
// op whose name is not a session name (inventory's "objects" sentinel) record
// neither field (REQ-004).
func traceSession(spec opSpec, args []string) (session, agent string) {
	if spec.required < 1 || len(args) == 0 || args[0] == "" || args[0] == "objects" {
		return "", ""
	}
	name := provider.SessionName(args[0])
	return args[0], name.Agent()
}
