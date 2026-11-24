package provider

import (
	"strings"
	"testing"
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
