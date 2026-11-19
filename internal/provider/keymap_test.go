package provider

import (
	"errors"
	"slices"
	"testing"
)

func TestTranslateKeysGrammar(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		// gc v1.5.1's whole send-keys vocabulary must map unchanged (AC3).
		{"gc vocabulary", []string{"Enter", "Up", "Down", "Escape", "C-u"},
			[]string{"enter", "up", "down", "esc", "ctrl+u"}},
		{"named table", []string{"BTab", "PPage", "NPage", "F12", "IC", "DC", "Space"},
			[]string{"shift+tab", "page_up", "page_down", "f12", "insert", "delete", "space"}},
		{"modifier shorthands", []string{"C-c", "M-x", "S-Tab"},
			[]string{"ctrl+c", "alt+x", "shift+tab"}},
		{"modifiers keep typed order", []string{"C-M-a", "M-C-S-x"},
			[]string{"ctrl+alt+a", "alt+ctrl+shift+x"}},
		{"modifier on a named key", []string{"C-Up", "C-Space"},
			[]string{"ctrl+up", "ctrl+space"}},
		{"upper-case modifier base lowercased", []string{"C-A"},
			[]string{"ctrl+a"}},
		// Literal text is split one key per character (AC2).
		{"literal text splits", []string{"ok go"},
			[]string{"o", "k", "space", "g", "o"}},
		{"lower-case name is literal", []string{"hello", "tab"},
			[]string{"h", "e", "l", "l", "o", "t", "a", "b"}},
		{"upper-case literal is shift", []string{"R"},
			[]string{"shift+r"}},
		{"mis-cased table name is literal", []string{"Return"},
			[]string{"shift+r", "e", "t", "u", "r", "n"}},
		{"out-of-table function name is literal", []string{"F13"},
			[]string{"shift+f", "1", "3"}},
		// A length-2 C- is not a modifier prefix: it is literal text.
		{"short modifier prefix is literal", []string{"C-"},
			[]string{"shift+c", "-"}},
		{"punctuation is literal", []string{"a-b"},
			[]string{"a", "-", "b"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := translateKeys(tt.in)
			if err != nil {
				t.Fatalf("translateKeys(%v): %v", tt.in, err)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("translateKeys(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestTranslateKeysRejectsUntypableKeys(t *testing.T) {
	for _, key := range []string{"", "!", "C-!", "é", "C- "} {
		t.Run(key, func(t *testing.T) {
			got, err := translateKeys([]string{key})
			if err == nil {
				t.Fatalf("translateKeys(%q) = %v, want a rejection", key, got)
			}
			if !errors.Is(err, ErrKeyRejected) {
				t.Errorf("translateKeys(%q) err = %v, want ErrKeyRejected", key, err)
			}
			var rejected *KeyRejectedError
			if !errors.As(err, &rejected) || rejected.Key != key {
				t.Errorf("translateKeys(%q) error = %#v, want KeyRejectedError{Key: %q}", key, err, key)
			}
		})
	}
}

// TestTranslateKeysRejectsBeforeReturning pins the all-or-nothing rule: one bad
// key rejects the whole list, and nothing is returned for it.
func TestTranslateKeysRejectsWholeList(t *testing.T) {
	got, err := translateKeys([]string{"Enter", "!", "Up"})
	if err == nil {
		t.Fatalf("translateKeys = %v, want a rejection", got)
	}
	if got != nil {
		t.Errorf("translateKeys returned %v alongside the rejection, want nil", got)
	}
}
