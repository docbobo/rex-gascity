package provider

import "testing"

// TestShowsDialogMatchesHerdrCoreReadiness pins the gc-reference screen matcher
// against runtime-herdr-core's readiness.py (pinned 0f07496)
// test_herdrcore_readiness.py: shows_dialog is named-dialog OR any rate-limit
// marker (gc's ContainsRateLimitDialog), and a Codex screen that has finished
// starting (its status line under the composer) shows none.
func TestShowsDialogMatchesHerdrCoreReadiness(t *testing.T) {
	const glyph = "❯"
	tests := []struct {
		name   string
		screen string
		want   bool
	}{
		{"trust dialog", "Do you trust the files in this folder?\n" + glyph + " 1. Yes, proceed\n  2. No, exit\n", true},
		{"bypass permissions", "WARNING: Claude Code running in Bypass Permissions mode\n" + glyph + " 1. No, exit\n  2. Yes, I accept\n", true},
		{"custom api key", "Detected a custom API key in your environment\n", true},
		{"theme picker with /theme", "Choose the text style\nTo change this later, run /theme\n", true},
		{"theme picker without every marker", "Choose the text style\n", false},
		{"hook review with every marker", "Hooks need review\nTrust all and continue\nContinue without trusting\n", true},
		{"hook review missing a marker", "Hooks need review\nTrust all and continue\n", false},
		{"usage limit", "You've hit your limit\n", true},
		{"a live composer is no dialog", glyph + " Try it\n", false},
		{"codex settled hides the bare mention", "OpenAI Codex\n› ask about the rate limit\n  gpt-5 · ~/repo\n", false},
		{"codex not settled shows the bare mention", "OpenAI Codex\n› ask about the rate limit\n", true},
		{"codex rate-limit menu is a dialog", "OpenAI Codex\nrate limit\n› 1. Retry\n  enter confirm · esc skip\n", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := showsDialog(tt.screen); got != tt.want {
				t.Errorf("showsDialog(%q) = %v, want %v", tt.screen, got, tt.want)
			}
		})
	}
}

// TestWithholdsFirstTurnMatchesHerdrCoreReadiness pins the withhold decision
// against runtime-herdr-core's readiness.py (pinned 0f07496)
// test_herdrcore_readiness.py: a named dialog, a high-confidence rate-limit
// screen, or a bare "rate limit" mention beside a numbered menu's cursor row
// withholds; a bare mention with no menu (a status line, a tip, a numbered list
// with no cursor row) does not.
func TestWithholdsFirstTurnMatchesHerdrCoreReadiness(t *testing.T) {
	const glyph = "❯"
	tests := []struct {
		name   string
		screen string
		prefix string
		want   bool
	}{
		{"trust dialog", "Do you trust the files in this folder?\n" + glyph + " 1. Yes, proceed\n  2. No, exit\n", glyph + " ", true},
		{"usage limit with a prompt", "You've hit your limit\nREADY> \n", "READY>", true},
		{"bare mention under the prompt", "READY> \n  rate limit: 40% used\n", "READY>", false},
		{"bare mention with no prompt", "booting\n  rate limit: 40% used\n", "READY>", false},
		{"bare mention beside a glyph with no prefix", glyph + " \n  Rate limit resets at 5pm\n", "", false},
		{"bare mention on its own line", "Rate limit reached\nretrying...\n", "", false},
		{"bare mention on the marker's own line", "Rate limit reached: 100%\n", "", false},
		{"bare mention in a tip", "Welcome\nTip: avoid hitting the rate limit by...\n", glyph + " ", false},
		{"numbered list with no cursor row", "Tips:\n1. mind the rate limit\n2. use /compact\n", glyph + " ", false},
		{"provider rate-limit screen", "Rate limit reached\n1. Keep trying\n2. Stop\n", "", true},
		{"provider rate-limit screen with a cursor", "Rate limit reached\n" + glyph + " 1. Keep trying\n  2. Stop\n", glyph + " ", true},
		{"uppercase provider rate-limit screen", "RATE LIMIT REACHED\n1. Keep trying\n2. Stop\n", "", true},
		{"usage-limit menu with a cursor", "Usage limit reached for gemini-3-flash-preview.\n1. Keep trying\n2. Stop\n", "", true},
		{"bare mention beside a cursor and prefix", "rate limit\n" + glyph + " 1. Retry\n  2. Wait\n", glyph + " ", true},
		{"bare mention beside a glyph with no prefix", "rate limit\n" + glyph + " 1. Retry\n  2. Wait\n", "", true},
		{"bare mention beside a prefix cursor", "rate limit\nREADY> 1. Retry\n  2. Wait\n", "READY>", true},
		{"codex settled hides the menu", "OpenAI Codex\n› why did Rate limit reached show Keep trying and Stop\n  gpt-5 · ~/repo\n", "", false},
		{"a plain prompt", "READY> \n", "READY>", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := withholdsFirstTurn(tt.screen, tt.prefix); got != tt.want {
				t.Errorf("withholdsFirstTurn(%q, %q) = %v, want %v", tt.screen, tt.prefix, got, tt.want)
			}
		})
	}
}
