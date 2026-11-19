package provider

import "testing"

// TestScreenHasPromptPrefixMatchesGcPaneShowsReadyPrompt pins the prompt-line
// reading shared with the startup readiness wait against gc's
// paneShowsReadyPrompt (gc v1.5.1 internal/runtime/tmux/tmux.go:4199-4231):
// the bottom-most prompt line decides, NBSP reads as a space, one leading box
// border is skipped, the bare glyph counts, and a numbered menu's cursor row is
// no prompt.
func TestScreenHasPromptPrefixMatchesGcPaneShowsReadyPrompt(t *testing.T) {
	tests := []struct {
		name   string
		screen string
		prefix string
		want   bool
	}{
		{"bare glyph on its own line", "booting\n❯ ", "❯ ", true},
		{"glyph then draft text", "booting\n❯ hello", "❯ ", true},
		{"bare glyph with no trailing space", "booting\n❯", "❯ ", true},
		{"nbsp between glyph and text", "booting\n❯\u00a0hello", "❯ ", true},
		{"box border before the glyph", "booting\n│ ❯ hello", "❯ ", true},
		{"thick box border before the glyph", "booting\n┃❯ ", "❯ ", true},
		{"glyph only in past output", "❯ old turn\ndone", "❯ ", true},
		{"selected dialog option is no prompt", "Do you trust this folder?\n❯ 1. Yes\n  2. No", "❯ ", false},
		{"second dialog option below the cursor", "Menu\n❯ 2. Keep trying\n  1. Stop", "❯ ", false},
		{"a draft starting 1. is still a prompt", "booting\n❯ 1. fix the test\n", "❯ ", true},
		{"no glyph anywhere", "booting\nworking", "❯ ", false},
		{"empty prefix matches nothing", "booting\n❯ ", "", false},
		{"border glyph alone is not the prefix", "booting\n│", "❯ ", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := screenHasPromptPrefix(tt.screen, tt.prefix); got != tt.want {
				t.Errorf("screenHasPromptPrefix(%q, %q) = %v, want %v", tt.screen, tt.prefix, got, tt.want)
			}
		})
	}
}
