package provider

import "testing"

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
