package provider

import (
	"fmt"
	"strings"
	"testing"
)

// Micro benchmarks for the pure string paths the provider runs on every send,
// start and observe pass. Run with `make bench`; compare runs with benchstat.

var (
	benchKeysSink   []string
	benchStringSink string
	benchBoolSink   bool
)

func BenchmarkTranslateKeys(b *testing.B) {
	cases := []struct {
		name string
		keys []string
	}{
		{"named", []string{"Enter"}},
		{"gc-vocabulary", []string{"Enter", "Up", "Down", "Escape", "C-u"}},
		{"modified", []string{"C-c", "M-x", "S-Tab", "C-M-a"}},
		{"function-keys", []string{"F1", "F6", "F12"}},
		{"literal-short", []string{"hello"}},
		{"literal-mixed", []string{"Hello, World 42"}},
		{"literal-long", []string{strings.Repeat("go test ./... ", 16)}},
	}
	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				out, err := translateKeys(tc.keys)
				if err != nil {
					b.Fatal(err)
				}
				benchKeysSink = out
			}
		})
	}
	b.Run("rejected", func(b *testing.B) {
		keys := []string{"Enter", "Up", "é"}
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if _, err := translateKeys(keys); err == nil {
				b.Fatal("want a rejection")
			}
		}
	})
}

func BenchmarkTabLabel(b *testing.B) {
	name := SessionName("rig--agent")
	town := SessionName("mayor")
	rigEnv := map[string]string{"GC_RIG": "rig"}
	b.Run("env-rig", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			benchStringSink = tabLabel(name, rigEnv, "city")
		}
	})
	b.Run("name-rig", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			benchStringSink = tabLabel(name, nil, "city")
		}
	})
	b.Run("town", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			benchStringSink = tabLabel(town, nil, "city")
		}
	})
	b.Run("clamped", func(b *testing.B) {
		long := map[string]string{"GC_RIG": strings.Repeat("r", 300)}
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			benchStringSink = tabLabel(name, long, "city")
		}
	})
}

func BenchmarkPaneLabelFor(b *testing.B) {
	longAgent := strings.Repeat("agent", 40)
	cases := []struct {
		name string
		n    SessionName
		env  map[string]string
	}{
		{"town", "mayor", nil},
		{"long-form", "rig--worker", map[string]string{
			"GC_RIG": "rig", "GC_AGENT": "worker", "GC_SESSION_NAME": "rig--worker",
		}},
		{"pool-short-form", "worker-bd-42", map[string]string{
			"GC_RIG": "rig", "GC_TEMPLATE": "rig/worker", "GC_AGENT": "bd-42",
			"GC_SESSION_NAME": "worker-bd-42",
		}},
		{"over-limit", SessionName("rig--" + longAgent), map[string]string{
			"GC_RIG": "rig", "GC_AGENT": longAgent, "GC_SESSION_NAME": "rig--" + longAgent,
		}},
	}
	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				benchStringSink = paneLabelFor(tc.n, tc.env)
			}
		})
	}
	b.Run("legacy", func(b *testing.B) {
		env := map[string]string{"GC_RIG": "rig", "GC_AGENT": "worker", "GC_SESSION_NAME": "rig--worker"}
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			benchStringSink = legacyPaneLabelFor("rig--worker", env)
		}
	})
}

func BenchmarkIdentityMatch(b *testing.B) {
	cases := []struct {
		name  string
		n     SessionName
		label string
	}{
		{"long-form-hit", "rig--worker", "worker (rig--worker)"},
		{"long-form-miss", "rig--other", "worker (rig--worker)"},
		{"short-form-hit", "worker-bd-42", "worker (#bd-42)"},
		{"no-parens", "rig--worker", "worker"},
	}
	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				benchBoolSink = identityMatch(tc.n, tc.label)
			}
		})
	}
	// Scan is the shape Binder.scan runs: one name against every pane label.
	for _, panes := range []int{10, 100, 1000} {
		labels := make([]string, panes)
		for i := range labels {
			labels[i] = fmt.Sprintf("agent%d (rig--agent%d)", i, i)
		}
		target := SessionName(fmt.Sprintf("rig--agent%d", panes-1))
		b.Run(fmt.Sprintf("scan-%d", panes), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				for _, l := range labels {
					if identityMatch(target, l) {
						benchBoolSink = true
					}
				}
			}
		})
	}
}
