package provider

import "strings"

// rexKey translates a gc/tmux key name into Rex's key-binding syntax. gc
// passes tmux names such as "C-c", "Enter" and "Up"; Rex expects "ctrl+c",
// "enter" and "up". A name that is already Rex syntax, or a bare character,
// passes through unchanged so Rex reports an unknown key honestly.
func rexKey(key string) string {
	if key == "" {
		return ""
	}
	mods, base := splitMods(key)
	base = rexBaseKey(base)
	if len(mods) == 0 {
		return base
	}
	return strings.Join(mods, "+") + "+" + base
}

// splitMods peels tmux's modifier prefixes (C-, M-, S-) off key and returns
// them in Rex's canonical order (shift, ctrl, alt). Rex accepts any order, but
// emitting one canonical form keeps tests and logs stable.
func splitMods(key string) (mods []string, base string) {
	seen := map[string]bool{}
	rest := key
loop:
	for {
		switch {
		case strings.HasPrefix(rest, "C-"):
			seen["ctrl"] = true
			rest = rest[2:]
		case strings.HasPrefix(rest, "M-"):
			seen["alt"] = true
			rest = rest[2:]
		case strings.HasPrefix(rest, "S-"):
			seen["shift"] = true
			rest = rest[2:]
		default:
			break loop
		}
	}
	for _, m := range []string{"shift", "ctrl", "alt"} {
		if seen[m] {
			mods = append(mods, m)
		}
	}
	return mods, rest
}

// rexBaseKey maps tmux's named keys onto Rex's names. Named keys are matched
// case-insensitively; anything else (a single character, an already-Rex name,
// or an unknown name) is returned as is.
func rexBaseKey(key string) string {
	if key == "" {
		return key
	}
	lower := strings.ToLower(key)
	switch lower {
	case "enter", "return":
		return "enter"
	case "escape", "esc":
		return "esc"
	case "bspace", "backspace":
		return "backspace"
	case "delete", "dc":
		return "delete"
	case "insert", "ic":
		return "insert"
	case "pageup", "pgup":
		return "page_up"
	case "pagedown", "pgdn":
		return "page_down"
	case "tab", "space", "home", "end", "up", "down", "left", "right":
		return lower
	}
	if isFunctionKey(lower) {
		return lower
	}
	return key
}

// isFunctionKey reports whether k names a function key, f1 through f24.
func isFunctionKey(k string) bool {
	if len(k) < 2 || k[0] != 'f' {
		return false
	}
	n := 0
	for _, r := range k[1:] {
		if r < '0' || r > '9' {
			return false
		}
		n = n*10 + int(r-'0')
	}
	return n >= 1 && n <= 24
}
