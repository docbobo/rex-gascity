package provider

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// clampRexLabel counts runes without a []rune copy. This pins it against the
// []rune round trip it replaced, including invalid UTF-8 and multibyte labels
// at the limit. The win is measured by BenchmarkTabLabel/clamped and
// BenchmarkPaneLabelFor/over-limit.

func refClampRexLabel(s string) string {
	r := []rune(s)
	if len(r) <= maxRexLabel {
		return s
	}
	return string(r[:maxRexLabel-1]) + "…"
}

func TestClampRexLabelMatchesReference(t *testing.T) {
	cases := map[string]string{
		"empty":             "",
		"short":             "Rig: gc",
		"at limit":          strings.Repeat("a", maxRexLabel),
		"one over":          strings.Repeat("a", maxRexLabel+1),
		"long ascii":        strings.Repeat("a", 500),
		"multibyte at lim":  strings.Repeat("é", maxRexLabel),
		"multibyte over":    strings.Repeat("é", maxRexLabel+1),
		"bytes over runes":  strings.Repeat("é", 100), // 200 bytes, 100 runes
		"cjk over":          strings.Repeat("世", 300),
		"mixed over":        strings.Repeat("aé世", 100),
		"emoji over":        strings.Repeat("😀", 200),
		"invalid at limit":  strings.Repeat("\xff", maxRexLabel),
		"invalid over":      strings.Repeat("\xff", maxRexLabel+5),
		"invalid in prefix": "a\xffb" + strings.Repeat("c", 200),
		"truncated rune":    strings.Repeat("a", 200) + "\xe4\xb8",
	}
	for name, in := range cases {
		if got, want := clampRexLabel(in), refClampRexLabel(in); got != want {
			t.Errorf("%s: clampRexLabel = %q, want %q", name, got, want)
		}
	}
}

// The pane ladder reference below is written from the grammar, not from the
// helpers: render a candidate and count runes. It pins the order in which a
// label gives up the display tail, the agent and finally the suffix.

func refPane(agent, suffix, id string) string {
	prefix := agent
	if agent != "" && !strings.Contains(agent, "] ") {
		prefix = "[" + agent + "]"
	}
	label := prefix + " (" + suffix + ")"
	if id != "" {
		label += " #" + id
	}
	return label
}

func refCutAgent(agent string, budget int) (string, bool) {
	r := []rune(agent)
	if len(r) <= budget {
		return agent, true
	}
	if budget < 2 {
		return "", false
	}
	return string(r[:budget-1]) + "…", true
}

func refFitPool(agent, pool, id string) (string, bool) {
	suffix := paneLabelShortForm + pool
	for _, tail := range []string{id, ""} {
		if tail != "" || id == "" {
			if label := refPane(agent, suffix, tail); utf8.RuneCountInString(label) <= maxPaneLabelRunes {
				return label, true
			}
		}
		if label := refPane(agent, suffix, ""); utf8.RuneCountInString(label) <= maxPaneLabelRunes {
			return label, true
		}
	}
	return "", false
}

func refFitLong(agent, name, id string) (string, bool) {
	tails := []string{id}
	if id != "" {
		tails = append(tails, "")
	}
	for _, tail := range tails {
		if label := refPane(agent, name, tail); utf8.RuneCountInString(label) <= maxPaneLabelRunes {
			return label, true
		}
		overhead := len("[] ()") + utf8.RuneCountInString(name)
		if agent == "" || strings.Contains(agent, "] ") {
			overhead = len(" ()") + utf8.RuneCountInString(name)
		}
		if tail != "" {
			overhead += len(" #") + utf8.RuneCountInString(tail)
		}
		cut, ok := refCutAgent(agent, maxPaneLabelRunes-overhead)
		if !ok {
			continue
		}
		if label := refPane(cut, name, tail); utf8.RuneCountInString(label) <= maxPaneLabelRunes {
			return label, true
		}
	}
	return "", false
}

func TestPaneLadderMatchesReference(t *testing.T) {
	cases := []struct{ name, agent, suffix, id string }{
		{"empty id short", "worker", "gc__worker-bd1", ""},
		{"exact limit with id", "worker", "gc__worker-bd1", "bd1"},
		{"just over, id dropped", "worker", strings.Repeat("s", 110), "bd1"},
		{"agent cut keeps suffix", strings.Repeat("a", 120), "gc__agent-bd1", "bd1"},
		{"multibyte agent cut", strings.Repeat("é", 100), "gc__agent-bd1", "bd1"},
		{"suffix too long", "worker", strings.Repeat("s", 130), "bd1"},
		{"empty agent", "", "gc__worker-bd1", "bd1"},
		{"long cut to marker", strings.Repeat("a", 200), strings.Repeat("s", 100), ""},
	}
	for _, tc := range cases {
		gotL, gotOK := fitLongPaneLabel(tc.agent, tc.suffix, tc.id)
		wantL, wantOK := refFitLong(tc.agent, tc.suffix, tc.id)
		if gotL != wantL || gotOK != wantOK {
			t.Errorf("%s: long ladder = (%q,%v), want (%q,%v)", tc.name, gotL, gotOK, wantL, wantOK)
		}
		gotL, gotOK = fitPoolPaneLabel(tc.agent, tc.suffix, tc.id)
		wantL, wantOK = refFitPool(tc.agent, tc.suffix, tc.id)
		if gotL != wantL || gotOK != wantOK {
			t.Errorf("%s: pool ladder = (%q,%v), want (%q,%v)", tc.name, gotL, gotOK, wantL, wantOK)
		}
		if gotOK && utf8.RuneCountInString(gotL) > maxPaneLabelRunes {
			t.Errorf("%s: %q exceeds the limit", tc.name, gotL)
		}
	}
}

func TestTaggedLabelMatchesReference(t *testing.T) {
	cases := []struct{ kind, name string }{
		{"city", "Shady_Sands"},
		{"rig", "Indigo"},
		{"city", "Gas City"},
		{"city", ""},
		{"rig", strings.Repeat("r", 200)},
	}
	for _, tc := range cases {
		want := "[" + tc.kind + "] " + tc.name
		if got := taggedLabel(tc.kind, tc.name); got != want {
			t.Errorf("taggedLabel(%q, %q) = %q, want %q", tc.kind, tc.name, got, want)
		}
	}
}
