package provider

import "strings"

// Startup-dialog detection: whether a pane's rendered screen shows a dialog the
// first turn must not land in. It ports runtime-herdr-core's
// herdrcore/readiness.py (pinned 0f07496), which in turn narrows gc's own
// startup-dialog detection (gc v1.5.1 internal/runtime/dialog.go:1706-1917,
// ContainsRateLimitDialog and containsPromptIndicator).
//
// The first turn Start types must never land in a dialog: a dialog marks its
// selected option with the prompt glyph (for example "❯ 1. No, exit"), and the
// turn's Enter would pick it. gc answers the dialog only once Start returns, so
// the provider withholds the nudge instead of pressing the selected option.

// startupDialogGroups lists the named startup dialogs, mirroring readiness.py's
// DIALOGS: each dialog is a group of alternative markers, and the screen shows
// that dialog when it shows one marker from every group. Keep this list and the
// rate-limit markers below in step with herdrcore/readiness.py.
var startupDialogGroups = [][][]string{
	{{"Choose the text style"}, {"/theme"}},
	{{"Resume from summary"}, {"Resume full session as-is"}, {"Enter to confirm"}},
	{{"Update available!"}, {"Skip until next version"}, {"Press enter to continue"}},
	{{
		"Trust this folder?",
		"trust this folder",
		"Quick safety check",
		"Do you trust the contents of this directory?",
		"Do you trust the files in this folder?",
		"Trust project folder?",
	}},
	{{"Allow external CLAUDE.md"}, {"allow external imports"}},
	{{"New MCP server found"}, {"Use this and all future MCP servers"}},
	{
		{"Hooks need review", "hooks need review"},
		{"Trust all and continue", "trust all"},
		{"Continue without trusting", "enter to review hooks"},
	},
	{{"Bypass Permissions mode"}},
	{{"Detected a custom API key in your environment", "Do you want to use this API key?"}},
}

// rateLimitDialogMarkers is readiness.py's RATE_LIMIT_DIALOG: the
// high-confidence provider rate-limit screen markers gc reads as a dialog.
var rateLimitDialogMarkers = []string{"Usage limit reached", "You've hit your limit", "/rate-limit-options"}

// bareRateLimitMarkers is readiness.py's BARE_RATE_LIMIT: the low-confidence
// mention that only counts beside a menu cursor row (or, for the gc-reference
// showsDialog, as the RATE_LIMIT screen marker).
var bareRateLimitMarkers = []string{"rate limit", "Rate limit"}

// allRateLimitMarkers is readiness.py's RATE_LIMIT = RATE_LIMIT_DIALOG +
// BARE_RATE_LIMIT, the permissive screen matcher showsDialog uses.
var allRateLimitMarkers = append(append([]string{}, rateLimitDialogMarkers...), bareRateLimitMarkers...)

// rateLimitMenuOptions is readiness.py's RATE_LIMIT_MENU: the options gc's
// ContainsProviderRateLimitScreen reads beside a "rate limit" mention.
var rateLimitMenuOptions = []string{"Keep trying", "Stop"}

// codexBanner and codexStatusSeparator are readiness.py's CODEX_BANNER and
// CODEX_STATUS_SEPARATOR: once Codex draws its status line under its prompt, a
// stale "rate limit" line in scrollback no longer counts.
const (
	codexBanner          = "OpenAI Codex"
	codexStatusSeparator = " · "
)

// startupPromptGlyphs and startupPromptEndings are readiness.py's PROMPT_GLYPHS
// and PROMPT_ENDINGS: the glyphs that mark a shell or agent prompt, standalone
// or before text, and the suffixes a bare prompt line can end in.
var (
	startupPromptGlyphs  = []string{"❯", "›", ">"}
	startupPromptEndings = []string{">", "$", "%", "#", "❯", "›"}
)

// showsDialog reports whether the screen shows a startup dialog gc knows. It is
// readiness.py's shows_dialog, the gc reference (gc's ContainsRateLimitDialog);
// Start uses withholdsFirstTurn, which narrows it.
func showsDialog(screen string) bool {
	return namedDialog(screen) || rateLimited(screen, allRateLimitMarkers)
}

// withholdsFirstTurn reports whether the screen shows a dialog the first turn
// must not land in: a named dialog gc knows, a high-confidence rate-limit
// screen, or a bare "rate limit" mention beside a menu cursor row. It is
// readiness.py's withholds_first_turn, what the startup readiness wait uses.
func withholdsFirstTurn(screen, prefix string) bool {
	return namedDialog(screen) || rateLimited(screen, rateLimitDialogMarkers) || rateLimitMenu(screen, prefix)
}

// namedDialog is readiness.py's _named_dialog: every group of one dialog has a
// marker on the screen.
func namedDialog(screen string) bool {
	for _, groups := range startupDialogGroups {
		if dialogGroupsMatch(screen, groups) {
			return true
		}
	}
	return false
}

// dialogGroupsMatch reports whether one marker from each group is on screen.
func dialogGroupsMatch(screen string, groups [][]string) bool {
	for _, alternatives := range groups {
		matched := false
		for _, marker := range alternatives {
			if strings.Contains(screen, marker) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

// rateLimited is readiness.py's _rate_limited: a marker shows, unless Codex has
// finished starting and the marker is only stale scrollback.
func rateLimited(screen string, markers []string) bool {
	if strings.Contains(screen, codexBanner) && codexSettled(screen) {
		return false
	}
	for _, marker := range markers {
		if strings.Contains(screen, marker) {
			return true
		}
	}
	return false
}

// rateLimitMenu is readiness.py's _rate_limit_menu: gc's provider rate-limit
// screen (the mention with its options), or a bare mention beside a numbered
// menu's cursor row. A numbered list with no cursor row (a tip's) is no menu.
func rateLimitMenu(screen, prefix string) bool {
	if strings.Contains(screen, codexBanner) && codexSettled(screen) {
		return false
	}
	if strings.Contains(strings.ToLower(screen), "rate limit") && containsAll(screen, rateLimitMenuOptions) {
		return true
	}
	return rateLimited(screen, bareRateLimitMarkers) && showsMenuCursor(screen, prefix)
}

// showsMenuCursor is readiness.py's _shows_menu_cursor: some line is a numbered
// menu's cursor row, marked by the configured prefix or by a prompt glyph and a
// space.
func showsMenuCursor(screen, prefix string) bool {
	cursors := make([]string, 0, len(startupPromptGlyphs)+1)
	for _, glyph := range startupPromptGlyphs {
		cursors = append(cursors, glyph+" ")
	}
	if prefix != "" {
		cursors = append(cursors, prefix)
	}
	lines := strings.Split(screen, "\n")
	for _, cursor := range cursors {
		for index := range lines {
			if menuCursorRow(lines, index, cursor) {
				return true
			}
		}
	}
	return false
}

// codexSettled is readiness.py's _codex_settled: Codex drew its status line
// ("<model> · <dir>") under its bottom-most prompt line, which it does once
// startup is over.
func codexSettled(screen string) bool {
	lines := strings.Split(screen, "\n")
	for index := len(lines) - 1; index >= 0; index-- {
		if !promptIndicator(lines[index]) {
			continue
		}
		for _, below := range lines[index+1:] {
			if strings.Contains(below, codexStatusSeparator) {
				return true
			}
		}
		return false
	}
	return false
}

// promptIndicator is readiness.py's _prompt_indicator for one line: a shell or
// agent prompt, its glyph alone or before text that is not a numbered menu row
// ("› 1."), or a line ending in a prompt ending.
func promptIndicator(line string) bool {
	text := stripBorder(strings.ReplaceAll(line, "\u00a0", " "))
	text = strings.TrimRight(text, " \t")
	if text == "" {
		return false
	}
	for _, glyph := range startupPromptGlyphs {
		if text == glyph {
			return true
		}
		if rest, ok := strings.CutPrefix(text, glyph+" "); ok && !isNumbered(rest) {
			return true
		}
	}
	for _, ending := range startupPromptEndings {
		if strings.HasSuffix(text, ending) {
			return true
		}
	}
	return false
}

// isNumbered is readiness.py's _numbered: digits, then a dot.
func isNumbered(text string) bool {
	digits := 0
	for digits < len(text) && text[digits] >= '0' && text[digits] <= '9' {
		digits++
	}
	return digits > 0 && digits < len(text) && text[digits] == '.'
}

// containsAll reports whether every substring is on screen.
func containsAll(screen string, subs []string) bool {
	for _, sub := range subs {
		if !strings.Contains(screen, sub) {
			return false
		}
	}
	return true
}
