package main

import (
	"context"
	"io"
	"strings"
	"testing"
)

// opFixture is the arguments and stdin an op handler needs to reach its
// provider call. The fixture map must cover ops exactly, both directions.
type opFixture struct {
	args  []string
	stdin string
}

var opFixtures = map[string]opFixture{
	"start":             {[]string{"s"}, `{"command":"sleep 300"}`},
	"stop":              {[]string{"s"}, ""},
	"attach":            {[]string{"s"}, ""},
	"is-running":        {[]string{"s"}, ""},
	"interrupt":         {[]string{"s"}, ""},
	"nudge":             {[]string{"s"}, "hello"},
	"send-keys":         {[]string{"s", "Enter"}, ""},
	"clear-scrollback":  {[]string{"s"}, ""},
	"process-alive":     {[]string{"s"}, "claude\n"},
	"peek":              {[]string{"s"}, ""},
	"list-running":      {nil, ""},
	"get-last-activity": {[]string{"s"}, ""},
	"idle":              {[]string{"s"}, ""},
	"program-status":    {[]string{"s"}, ""},
	"is-attached":       {[]string{"s"}, ""},
	"set-meta":          {[]string{"s", "k"}, "v"},
	"get-meta":          {[]string{"s", "k"}, ""},
	"remove-meta":       {[]string{"s", "k"}, ""},
	"observe":           {[]string{"s"}, "claude\n"},
	"inventory":         {nil, ""},
	"kill-object":       {[]string{"s"}, "{}"},
}

// TestOpSpecMethodMatchesHandler walks the ops table and proves each row's
// method names the provider call its handler makes. A row with no method
// (attach) must record no provider call. It fails when a row is reverted to a
// method the handler does not invoke.
func TestOpSpecMethodMatchesHandler(t *testing.T) {
	for name := range ops {
		if _, ok := opFixtures[name]; !ok {
			t.Errorf("op %q has no fixture in opFixtures", name)
		}
	}
	for name := range opFixtures {
		if _, ok := ops[name]; !ok {
			t.Errorf("opFixtures has %q, which is not an op", name)
		}
	}

	for name, spec := range ops {
		fixture, ok := opFixtures[name]
		if !ok {
			continue // reported above
		}
		t.Run(name, func(t *testing.T) {
			rec := &recordingProvider{}
			_ = spec.handle(context.Background(), rec, fixture.args, strings.NewReader(fixture.stdin), io.Discard)

			if spec.method == "" {
				if len(rec.calls) != 0 {
					t.Errorf("recorded calls = %q, want none for a row with no method", rec.methods())
				}
				return
			}
			if len(rec.calls) == 0 {
				t.Fatalf("recorded no provider call, want %s", spec.method)
			}
			if got := rec.calls[len(rec.calls)-1].method; got != spec.method {
				t.Errorf("recorded %q, row says %q", got, spec.method)
			}
		})
	}
}
