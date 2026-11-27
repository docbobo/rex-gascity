package provider

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// ErrKeyRejected is the sentinel wrapped by KeyRejectedError. Callers classify
// a key with no Rex spelling with errors.Is.
var ErrKeyRejected = errors.New("rex: key rejected")

// KeyRejectedError reports a key the grammar cannot spell. herdr-core rejects
// the whole send-keys call as `rejected` (exit 80) before any lookup, and the
// dispatcher maps this error to that outcome.
type KeyRejectedError struct {
	Key string
}

func (e *KeyRejectedError) Error() string {
	return fmt.Sprintf("key %q has no Rex spelling", e.Key)
}

func (e *KeyRejectedError) Unwrap() error { return ErrKeyRejected }

// tmuxKeys is the exact-case tmux key table herdr-core matches first
// (TMUX_KEYS, herdrcore/inputs.py:82-107), mapped onto Rex's key names.
// Matched case-sensitively. keymap_test.go pins the table.
var tmuxKeys = map[string]string{
	"Enter":    "enter",
	"Escape":   "esc",
	"Esc":      "esc",
	"Tab":      "tab",
	"BTab":     "shift+tab",
	"BSpace":   "backspace",
	"Space":    "space",
	"Up":       "up",
	"Down":     "down",
	"Left":     "left",
	"Right":    "right",
	"Home":     "home",
	"End":      "end",
	"PageUp":   "page_up",
	"PgUp":     "page_up",
	"PPage":    "page_up",
	"PageDown": "page_down",
	"PgDn":     "page_down",
	"NPage":    "page_down",
	"IC":       "insert",
	"Insert":   "insert",
	"DC":       "delete",
	"Delete":   "delete",
}

// hcPunctuation is the punctuation herdr-core accepts as literal text (and as
// a modifiable single-character base), besides letters, digits and space.
const hcPunctuation = "`\\[],=-.';/"

// translateKeys maps gc/tmux key arguments to Rex key names, mirroring
// herdr-core's grammar (translate_key, herdrcore/inputs.py:126-148):
//  1. an exact-case table name or F1–F12 is one key;
//  2. C-/M-/S- prefixes, while more than two characters remain, modify a table
//     name or one literal character;
//  3. anything else is literal text, character by character.
//
// keymap_test.go and keymap_equiv_test.go pin the grammar. Every key is
// translated before any Rex call; the first key with no
// Rex spelling rejects the whole call, so a bad key never reaches Rex.
func translateKeys(keys []string) ([]string, error) {
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		expanded, ok := translateKey(key)
		if !ok {
			return nil, &KeyRejectedError{Key: key}
		}
		out = append(out, expanded...)
	}
	return out, nil
}

// translateKey spells one key as one or more Rex keys. A table name maps to one
// key; a C-/M-/S- modified table name or single character maps to one modified
// key; any other string is literal text typed character by character. ok is
// false when the key has no Rex spelling.
func translateKey(key string) ([]string, bool) {
	if key == "" {
		return nil, false
	}
	if rex, ok := namedKey(key); ok {
		return []string{rex}, true
	}
	mods, base := peelMods(key)
	if len(mods) > 0 {
		if rex, ok := namedKey(base); ok {
			return []string{joinMods(mods) + rex}, true
		}
		if r, ok := singleLiteralChar(base); ok {
			return []string{joinMods(mods) + string(r)}, true
		}
		return nil, false
	}
	return literalKeys(key)
}

// namedKey resolves an exact-case table name, including the F1–F12 function
// keys, to a Rex key name.
func namedKey(key string) (string, bool) {
	if rex, ok := tmuxKeys[key]; ok {
		return rex, true
	}
	if n, ok := functionKeyNumber(key); ok {
		return "f" + strconv.Itoa(n), true
	}
	return "", false
}

// functionKeyNumber reports whether key is an exact-case F1–F12 name and
// returns its number. herdr-core's table stops at F12; a higher number is
// literal text.
func functionKeyNumber(key string) (int, bool) {
	if len(key) < 2 || key[0] != 'F' {
		return 0, false
	}
	n := 0
	for i := 1; i < len(key); i++ {
		c := key[i]
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + int(c-'0')
	}
	if n < 1 || n > 12 {
		return 0, false
	}
	return n, true
}

// peelMods strips tmux's C-/M-/S- prefixes, in typed order, while more than one
// character remains. The length rule is what makes "C-" literal text rather
// than a modifier with an empty base (herdr-core: while len(rest) > 2).
func peelMods(key string) (mods []string, base string) {
	rest := key
	for len(rest) > 2 {
		switch {
		case strings.HasPrefix(rest, "C-"):
			mods = append(mods, "ctrl")
		case strings.HasPrefix(rest, "M-"):
			mods = append(mods, "alt")
		case strings.HasPrefix(rest, "S-"):
			mods = append(mods, "shift")
		default:
			return mods, rest
		}
		rest = rest[2:]
	}
	return mods, rest
}

// joinMods renders modifier names in typed order as a Rex prefix.
func joinMods(mods []string) string {
	return strings.Join(mods, "+") + "+"
}

// singleLiteralChar accepts a one-character modifier base, lower-cased the way
// herdr-core lower-cases it. Space is not a legal modifier base (use Space).
func singleLiteralChar(base string) (rune, bool) {
	r := []rune(base)
	if len(r) != 1 {
		return 0, false
	}
	lower := r[0]
	if lower >= 'A' && lower <= 'Z' {
		lower += 'a' - 'A'
	}
	if isLiteralRune(lower) {
		return lower, true
	}
	return 0, false
}

// literalKeys types a key as literal text, one Rex key per character.
func literalKeys(key string) ([]string, bool) {
	out := make([]string, 0, len(key))
	for _, r := range key {
		switch {
		case r == ' ':
			out = append(out, "space")
		case r >= 'A' && r <= 'Z':
			out = append(out, shiftedLetterKeys[r-'A'])
		case isLiteralRune(r):
			// isLiteralRune admits ASCII only, so the table covers every case.
			out = append(out, asciiKeys[r])
		default:
			return nil, false
		}
	}
	return out, true
}

// asciiKeys spells each ASCII character as its one-character Rex key, and
// shiftedLetterKeys spells each upper-case letter as shift+<lower>. Both are
// built once so literalKeys allocates no string per typed character.
var (
	asciiKeys         [utf8.RuneSelf]string
	shiftedLetterKeys [26]string
)

func init() {
	for r := range asciiKeys {
		asciiKeys[r] = string(rune(r))
	}
	for i := range shiftedLetterKeys {
		shiftedLetterKeys[i] = "shift+" + string(rune('a'+i))
	}
}

// isLiteralRune reports whether r is a character herdr-core accepts as literal
// text: a lower-case letter, a digit, or hcPunctuation. Upper-case letters are
// handled as shift+<lower>; space is handled separately.
func isLiteralRune(r rune) bool {
	if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
		return true
	}
	return strings.ContainsRune(hcPunctuation, r)
}
