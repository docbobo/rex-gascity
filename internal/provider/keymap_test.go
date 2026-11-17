package provider

import "testing"

func TestRexKey(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"ctrl shorthand", "C-c", "ctrl+c"},
		{"meta shorthand", "M-x", "alt+x"},
		{"shift shorthand", "S-Tab", "shift+tab"},
		{"combined mods in Rex order", "C-M-x", "ctrl+alt+x"},
		{"mods normalized to shift,ctrl,alt", "M-C-S-x", "shift+ctrl+alt+x"},
		{"enter", "Enter", "enter"},
		{"return alias", "Return", "enter"},
		{"escape", "Escape", "esc"},
		{"esc passthrough", "Esc", "esc"},
		{"backspace", "BSpace", "backspace"},
		{"delete", "Delete", "delete"},
		{"insert", "Insert", "insert"},
		{"page up", "PageUp", "page_up"},
		{"page down", "PageDown", "page_down"},
		{"arrows", "Up", "up"},
		{"arrow down", "Down", "down"},
		{"tab", "Tab", "tab"},
		{"space", "Space", "space"},
		{"home", "Home", "home"},
		{"end", "End", "end"},
		{"function key lowercased", "F11", "f11"},
		{"function key 24", "f24", "f24"},
		{"bare character", "a", "a"},
		{"already Rex", "ctrl+c", "ctrl+c"},
		{"unknown name passes through", "NUL", "NUL"},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := rexKey(tt.in); got != tt.want {
				t.Errorf("rexKey(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
