package provider

import (
	"context"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/docbobo/rex-gascity/internal/rex/faketest"
)

func TestPaneLabelFor(t *testing.T) {
	tests := []struct {
		name string
		sess string
		env  map[string]string
		want string
	}{
		{
			name: "alias wins over agent and beads actor",
			sess: "gc__implementation-worker-mc-wisp-fvwrce",
			env: map[string]string{
				"GC_RIG":          "rex-gascity",
				"GC_ALIAS":        "gc.implementation-worker",
				"GC_AGENT":        "mc-wisp-fvwrce",
				"BEADS_ACTOR":     "rex-gascity/gc.implementation-worker",
				"GC_SESSION_NAME": "gc__implementation-worker-mc-wisp-fvwrce",
			},
			want: "gc.implementation-worker (gc__implementation-worker-mc-wisp-fvwrce)",
		},
		{
			name: "empty alias falls to agent",
			sess: "gc__requirements-planner-mc-wisp-mf93ir",
			env: map[string]string{
				"GC_RIG":          "rex-gascity",
				"GC_ALIAS":        "  ",
				"GC_AGENT":        "mc-wisp-mf93ir",
				"BEADS_ACTOR":     "rex-gascity/gc.requirements-planner",
				"GC_SESSION_NAME": "gc__requirements-planner-mc-wisp-mf93ir",
			},
			want: "mc-wisp-mf93ir (gc__requirements-planner-mc-wisp-mf93ir)",
		},
		{
			name: "beads actor suffix fallback",
			sess: "gc__task-decomposer-mc-wisp-aaaa",
			env: map[string]string{
				"GC_RIG":          "rex-gascity",
				"BEADS_ACTOR":     "rex-gascity/gc.task-decomposer",
				"GC_SESSION_NAME": "gc__task-decomposer-mc-wisp-aaaa",
			},
			want: "gc.task-decomposer (gc__task-decomposer-mc-wisp-aaaa)",
		},
		{
			name: "beads actor without slash used whole",
			sess: "gc__mayor-mc-wisp-bbbb",
			env: map[string]string{
				"GC_RIG":          "rex-gascity",
				"BEADS_ACTOR":     "mayor",
				"GC_SESSION_NAME": "gc__mayor-mc-wisp-bbbb",
			},
			want: "mayor (gc__mayor-mc-wisp-bbbb)",
		},
		{
			name: "no agent identity keeps name-derived label",
			sess: "gc__implementation-worker-mc-wisp-fvwrce",
			env: map[string]string{
				"GC_RIG":          "rex-gascity",
				"GC_SESSION_NAME": "gc__implementation-worker-mc-wisp-fvwrce",
			},
			want: "gc__implementation-worker-mc-wisp-fvwrce",
		},
		{
			name: "no agent identity on rig name keeps agent part",
			sess: "rex-gascity--dog-1",
			env: map[string]string{
				"GC_RIG":          "rex-gascity",
				"GC_SESSION_NAME": "rex-gascity--dog-1",
			},
			want: "dog-1",
		},
		{
			name: "empty session name keeps name-derived label",
			sess: "gc__implementation-worker-mc-wisp-fvwrce",
			env: map[string]string{
				"GC_RIG":      "rex-gascity",
				"GC_AGENT":    "mc-wisp-fvwrce",
				"GC_ALIAS":    "gc.implementation-worker",
				"BEADS_ACTOR": "rex-gascity/gc.implementation-worker",
			},
			want: "gc__implementation-worker-mc-wisp-fvwrce",
		},
		{
			name: "town-level unchanged without parentheses",
			sess: "mayor",
			env: map[string]string{
				"GC_ALIAS":        "mayor",
				"GC_AGENT":        "mayor",
				"GC_SESSION_NAME": "mayor",
			},
			want: "mayor",
		},
		{
			name: "rigless pool name with rig set gets new label",
			sess: "gc__implementation-worker-mc-wisp-fvwrce",
			env: map[string]string{
				"GC_RIG":          "rex-gascity",
				"GC_AGENT":        "mc-wisp-fvwrce",
				"GC_SESSION_NAME": "gc__implementation-worker-mc-wisp-fvwrce",
			},
			want: "mc-wisp-fvwrce (gc__implementation-worker-mc-wisp-fvwrce)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := paneLabelFor(SessionName(tt.sess), tt.env)
			if got != tt.want {
				t.Fatalf("paneLabelFor(%q, %v) = %q, want %q", tt.sess, tt.env, got, tt.want)
			}
			if strings.HasPrefix(got, " (") || strings.HasSuffix(got, " ()") {
				t.Fatalf("paneLabelFor(%q) produced empty-parentheses label %q", tt.sess, got)
			}
		})
	}
}

func TestPaneLabelForClamp(t *testing.T) {
	longAgent := strings.Repeat("a", 200)
	longSession := "gc__" + strings.Repeat("é", 200)
	shortSession := "gc__short-mc-wisp-cccc"

	t.Run("long agent shortened, suffix intact", func(t *testing.T) {
		env := map[string]string{
			"GC_RIG":          "rex-gascity",
			"GC_ALIAS":        longAgent,
			"GC_SESSION_NAME": shortSession,
		}
		got := paneLabelFor("gc__short-mc-wisp-cccc", env)
		if n := utf8.RuneCountInString(got); n != maxPaneLabelRunes {
			t.Fatalf("label has %d runes, want %d: %q", n, maxPaneLabelRunes, got)
		}
		if !strings.HasSuffix(got, " ("+shortSession+")") {
			t.Fatalf("label %q lost the session suffix", got)
		}
	})

	t.Run("long session truncated rune-safe and deterministic", func(t *testing.T) {
		env := map[string]string{
			"GC_RIG":          "rex-gascity",
			"GC_AGENT":        "agent",
			"GC_SESSION_NAME": longSession,
		}
		got := paneLabelFor("gc__long", env)
		if n := utf8.RuneCountInString(got); n != maxPaneLabelRunes {
			t.Fatalf("label has %d runes, want %d", n, maxPaneLabelRunes)
		}
		if !utf8.ValidString(got) {
			t.Fatalf("label %q is not valid UTF-8", got)
		}
		if again := paneLabelFor("gc__long", env); again != got {
			t.Fatalf("label is not deterministic: %q != %q", got, again)
		}
	})

	t.Run("short label unchanged", func(t *testing.T) {
		env := map[string]string{
			"GC_RIG":          "rex-gascity",
			"GC_AGENT":        "agent",
			"GC_SESSION_NAME": shortSession,
		}
		want := "agent (" + shortSession + ")"
		if got := paneLabelFor("gc__short", env); got != want {
			t.Fatalf("paneLabelFor = %q, want %q", got, want)
		}
	})
}

// TestPaneLabelRediscovery is the GCREX-5qel section 3 agreement proof: for one
// start-config env the hint paneLabelFor produces is the label record() stores,
// and a resumed start finds the live block under that label both from the
// stored sidecar record and, with the record lost, from the env-derived hint
// (provider.go:458).
func TestPaneLabelRediscovery(t *testing.T) {
	ctx := context.Background()
	const (
		start   = "pi-mc-wisp-fvwrce"
		session = "gc__implementation-worker-mc-wisp-fvwrce"
		want    = "mc-wisp-fvwrce (" + session + ")"
	)
	env := map[string]string{
		"GC_RIG":          "rex-gascity",
		"GC_AGENT":        "mc-wisp-fvwrce",
		"GC_SESSION_NAME": session,
	}

	fake := faketest.New()
	p := New("acme", fake)
	if err := p.Start(ctx, start, Config{Argv: []string{"pi"}, Env: env}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	stored, ok := p.Binding(start)
	if !ok {
		t.Fatal("Binding(start) = false after Start")
	}
	if hint := paneLabelFor(start, env); hint != stored.PaneLabel {
		t.Fatalf("hint label = %q, recorded label = %q", hint, stored.PaneLabel)
	}
	if got := fake.CreatedWindows[0].Spec.Label; got != want {
		t.Fatalf("launched pane label = %q, want %q", got, want)
	}

	// Sidecar record present: a resumed start keeps the live block.
	if err := p.Start(ctx, start, Config{Argv: []string{"pi"}, Env: env}); err != nil {
		t.Fatalf("Start with record present: %v", err)
	}
	if len(fake.CreatedWindows) != 1 || len(fake.CreatedSplits) != 0 {
		t.Fatalf("record-present resume relaunched a pane: windows=%d splits=%d",
			len(fake.CreatedWindows), len(fake.CreatedSplits))
	}

	// Sidecar record lost: a fresh provider sees only Rex, so the env-derived
	// hint must find the block under the same label instead of launching again.
	rebuilt := faketest.New()
	rebuilt.Sessions = fake.Sessions
	rebuilt.View = fake.View
	if err := New("acme", rebuilt).Start(ctx, start, Config{Argv: []string{"pi"}, Env: env}); err != nil {
		t.Fatalf("Start with record lost: %v", err)
	}
	if len(rebuilt.CreatedWindows) != 0 || len(rebuilt.CreatedSplits) != 0 {
		t.Fatalf("lost-record resume relaunched a pane: windows=%d splits=%d",
			len(rebuilt.CreatedWindows), len(rebuilt.CreatedSplits))
	}
}
