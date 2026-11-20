package provider

import (
	"strings"
	"testing"
	"unicode/utf8"
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
