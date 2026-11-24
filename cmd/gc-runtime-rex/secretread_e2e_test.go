package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/docbobo/rex-gascity/internal/rex/fakerex/fakestate"
)

// TestE2ESecretReadWithholdsInput drives the #48 guard end to end against the
// fake-rex binary: a pane whose block reports a secret read (set through
// fakerex.Fake.Update) gets no nudge (exit 80 pending_interaction) and no
// send-keys Enter (exit 80 not_sent); once the read ends, Enter is sent.
//
// It runs only against the fake. A live Rex cannot be put into a secret read
// from a test, and the predicate itself stays unverified against live Rex
// (#48 OQ-1).
func TestE2ESecretReadWithholdsInput(t *testing.T) {
	d, _, f := e2eEnv(t)
	if f == nil {
		t.Skip("REX_E2E selects the live server; this scenario needs the fake's per-block input_mode")
	}
	t.Setenv("GC_REX_STATE_DIR", t.TempDir())
	t.Setenv("GC_REX_CITY", fmt.Sprintf("e2e-secret-%d", time.Now().UnixNano()))

	call := func(stdin string, args ...string) result {
		t.Helper()
		var stdout, stderr bytes.Buffer
		code := run(args, strings.NewReader(stdin), &stdout, &stderr, d)
		return result{code: code, stdout: stdout.String(), stderr: stderr.String()}
	}
	const name = "Indigo--dog-1"
	defer call("", "stop", name)

	cfg := fmt.Sprintf(`{"work_dir":%q,"command":"sleep 300"}`, os.TempDir())
	if r := call(cfg, "start", name); r.code != exitOK {
		t.Fatalf("start: exit %d, stderr %q", r.code, r.stderr)
	}
	setMode := func(m *fakestate.InputMode) {
		t.Helper()
		f.Update(t, func(st *fakestate.State) {
			n := 0
			for _, s := range st.Sessions {
				for _, w := range s.Windows {
					for _, b := range w.Blocks {
						b.InputMode = m
						n++
					}
				}
			}
			if n != 1 {
				t.Fatalf("fake holds %d blocks, want the one started pane", n)
			}
		})
	}
	outcome := func(r result) string {
		t.Helper()
		if r.code != exitInputOutcome {
			t.Fatalf("exit = %d, want %d (stdout %q, stderr %q)", r.code, exitInputOutcome, r.stdout, r.stderr)
		}
		var line struct {
			Outcome string `json:"outcome"`
		}
		if err := json.Unmarshal([]byte(r.stdout), &line); err != nil {
			t.Fatalf("stdout = %q, want one JSON outcome line: %v", r.stdout, err)
		}
		return line.Outcome
	}

	setMode(&fakestate.InputMode{Echo: false, Canonical: true, Signals: true})
	if got := outcome(call("hello", "nudge", name)); got != "pending_interaction" {
		t.Errorf("nudge into a secret read: outcome %q, want pending_interaction", got)
	}
	if got := outcome(call("", "send-keys", name, "Enter")); got != "not_sent" {
		t.Errorf("send-keys Enter into a secret read: outcome %q, want not_sent", got)
	}

	setMode(nil)
	if r := call("", "send-keys", name, "Enter"); r.code != exitOK {
		t.Errorf("send-keys Enter after the secret read ended: exit %d, stderr %q", r.code, r.stderr)
	}
}
