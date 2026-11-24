package provider

import (
	"slices"
	"strings"
	"testing"
)

// literalKeys spells characters from tables built once. This pins it against
// the straightforward per-character implementation it replaced, so the
// allocation trim cannot change a single key. The win is measured by
// BenchmarkTranslateKeys.

func refLiteralKeys(key string) ([]string, bool) {
	out := make([]string, 0, len(key))
	for _, r := range key {
		switch {
		case r == ' ':
			out = append(out, "space")
		case r >= 'A' && r <= 'Z':
			out = append(out, "shift+"+string(r+'a'-'A'))
		case isLiteralRune(r):
			out = append(out, string(r))
		default:
			return nil, false
		}
	}
	return out, true
}

func TestLiteralKeysMatchesReference(t *testing.T) {
	cases := []string{
		"", "a", "hello", "Hello, World 42", "A", "Z", "az09", " ", "a b",
		"`\\[],=-.';/", strings.Repeat("go test ./... ", 16),
		"é", "abé", "~", "!", "\x00", "世",
	}
	for _, in := range cases {
		got, gotOK := literalKeys(in)
		want, wantOK := refLiteralKeys(in)
		if gotOK != wantOK || !slices.Equal(got, want) {
			t.Errorf("literalKeys(%q) = %v,%v want %v,%v", in, got, gotOK, want, wantOK)
		}
	}
	// Every single rune in the ASCII range plus a few beyond it.
	for i := range 0x250 {
		r := rune(i)
		in := string(r)
		got, gotOK := literalKeys(in)
		want, wantOK := refLiteralKeys(in)
		if gotOK != wantOK || !slices.Equal(got, want) {
			t.Errorf("literalKeys(%U) = %v,%v want %v,%v", r, got, gotOK, want, wantOK)
		}
	}
}
