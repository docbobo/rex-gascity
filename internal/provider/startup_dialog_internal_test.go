package provider

import (
	"fmt"
	"strings"
	"testing"
)

// TestMatchedStartupDialogNames pins REQ-005: one stable name per dialog (the
// first marker of its first group), checked in withholdsFirstTurn's order,
// with withholdsFirstTurn as its ok.
func TestMatchedStartupDialogNames(t *testing.T) {
	cases := []struct {
		name, screen, prefix, want string
		ok                         bool
	}{
		{"trust alternative keeps the group's first marker", "Do you trust the files in this folder?\n❯ 1. Yes, proceed\n  2. No, exit\n", "", "Trust this folder?", true},
		{"lower-case trust alternative", "please trust this folder", "", "Trust this folder?", true},
		{"theme dialog", "Choose the text style\n/theme\n", "", "Choose the text style", true},
		{"hooks dialog with lower-case alternatives", "hooks need review\ntrust all\nenter to review hooks\n", "", "Hooks need review", true},
		{"named dialog before a rate limit", "Trust this folder?\nUsage limit reached\n", "", "Trust this folder?", true},
		{"rate limit", "Usage limit reached\n", "", "rate-limit", true},
		{"rate-limit menu", "rate limit\nKeep trying\nStop\n", "", "rate-limit-menu", true},
		{"plain editor", "pi v1\n❯ ", "", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := matchedStartupDialog(c.screen, c.prefix)
			if got != c.want || ok != c.ok {
				t.Fatalf("matchedStartupDialog = %q, %v; want %q, %v", got, ok, c.want, c.ok)
			}
			if withholdsFirstTurn(c.screen, c.prefix) != c.ok {
				t.Fatalf("withholdsFirstTurn disagrees with matchedStartupDialog's ok %v", c.ok)
			}
		})
	}
}

// TestStartupScreenTail pins REQ-007's cut: the last startupDialogTailLines
// lines, a final newline kept and not counted, a shorter screen whole.
func TestStartupScreenTail(t *testing.T) {
	numbered := func(n int) string {
		var b strings.Builder
		for i := 1; i <= n; i++ {
			fmt.Fprintf(&b, "line %d\n", i)
		}
		return b.String()
	}
	short := numbered(startupDialogTailLines)
	if got := startupScreenTail(short); got != short {
		t.Fatalf("a %d-line screen was cut", startupDialogTailLines)
	}
	long := numbered(startupDialogTailLines + 5)
	got := startupScreenTail(long)
	if !strings.HasPrefix(got, "line 6\n") || strings.Count(got, "\n") != startupDialogTailLines {
		t.Fatalf("tail starts %q with %d lines, want line 6 and %d lines", got[:12], strings.Count(got, "\n"), startupDialogTailLines)
	}
	if got := startupScreenTail(strings.TrimSuffix(long, "\n")); !strings.HasPrefix(got, "line 6\n") {
		t.Fatalf("without a final newline the tail starts %q, want line 6", got[:12])
	}
}
