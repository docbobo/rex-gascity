package provider

import "testing"

func TestTaggedLabel(t *testing.T) {
	tests := []struct {
		kind, name, want string
	}{
		{"city", "Shady_Sands", "[city] Shady_Sands"},
		{"rig", "rex-gascity", "[rig] rex-gascity"},
		{"city", "Gas City", "[city] Gas City"},
		{"rig", "a-b_c.d", "[rig] a-b_c.d"},
	}
	for _, tt := range tests {
		if got := taggedLabel(tt.kind, tt.name); got != tt.want {
			t.Errorf("taggedLabel(%q, %q) = %q, want %q", tt.kind, tt.name, got, tt.want)
		}
	}
}

func TestPaneTag(t *testing.T) {
	tests := []struct{ agent, want string }{
		{"gc.implementation-worker", "[gc.implementation-worker]"},
		{"mayor", "[mayor]"},
	}
	for _, tt := range tests {
		if got := paneTag(tt.agent); got != tt.want {
			t.Errorf("paneTag(%q) = %q, want %q", tt.agent, got, tt.want)
		}
	}
}

func TestSplitPaneLabel(t *testing.T) {
	tests := []struct {
		name   string
		label  string
		want   paneParts
		wantOK bool
	}{
		{
			name:  "new pool form with tail",
			label: "[gc.implementation-worker] (mc-wisp-fvwrce) #gcg-session-982da4d7e3004e6ca3c8a95f2c7956c4",
			want: paneParts{
				agent:  "gc.implementation-worker",
				suffix: "mc-wisp-fvwrce",
				id:     "gcg-session-982da4d7e3004e6ca3c8a95f2c7956c4",
				tagged: true,
			},
			wantOK: true,
		},
		{
			name:   "new pool form without tail",
			label:  "[gc.implementation-worker] (mc-wisp-fvwrce)",
			want:   paneParts{agent: "gc.implementation-worker", suffix: "mc-wisp-fvwrce", tagged: true},
			wantOK: true,
		},
		{
			name:  "new long form with tail",
			label: "[gc.implementation-worker] (gc__implementation-worker-mc-wisp-fvwrce) #gcg-session-1",
			want: paneParts{
				agent:  "gc.implementation-worker",
				suffix: "gc__implementation-worker-mc-wisp-fvwrce",
				id:     "gcg-session-1",
				tagged: true,
			},
			wantOK: true,
		},
		{
			name:   "town form has no tail",
			label:  "[mayor] (mayor)",
			want:   paneParts{agent: "mayor", suffix: "mayor", tagged: true},
			wantOK: true,
		},
		{
			name:   "legacy long form",
			label:  "gc.implementation-worker (gc__implementation-worker-mc-wisp-fvwrce)",
			want:   paneParts{agent: "gc.implementation-worker", suffix: "gc__implementation-worker-mc-wisp-fvwrce"},
			wantOK: true,
		},
		{
			name:   "legacy pool form keeps the mark in the suffix",
			label:  "gc.implementation-worker (#mc-wisp-kvqib)",
			want:   paneParts{agent: "gc.implementation-worker", suffix: "#mc-wisp-kvqib"},
			wantOK: true,
		},
		{
			name:   "empty label",
			label:  "",
			wantOK: false,
		},
		{
			name:   "empty parentheses",
			label:  "[mayor] ()",
			wantOK: false,
		},
		{
			name:   "empty tag",
			label:  "[] (mayor)",
			wantOK: false,
		},
		{
			name:   "no parentheses",
			label:  "[mayor]",
			wantOK: false,
		},
		{
			name:   "bare label",
			label:  "mayor",
			wantOK: false,
		},
		{
			name:   "unterminated tag",
			label:  "[mayor (mayor)",
			wantOK: false,
		},
		{
			name:   "text after the tag",
			label:  "[mayor] x (mayor)",
			wantOK: false,
		},
		{
			name:   "empty tail id",
			label:  "[mayor] (mayor) #",
			wantOK: false,
		},
		{
			name:   "tail id with a space",
			label:  "[mayor] (mayor) #a b",
			wantOK: false,
		},
		{
			name:   "trailing text after the tail",
			label:  "[mayor] (mayor) #id x",
			wantOK: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := splitPaneLabel(tt.label)
			if ok != tt.wantOK {
				t.Fatalf("splitPaneLabel(%q) ok = %v, want %v", tt.label, ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if got != tt.want {
				t.Errorf("splitPaneLabel(%q) = %+v, want %+v", tt.label, got, tt.want)
			}
			if round := got.String(); round != tt.label {
				t.Errorf("paneParts.String() = %q, want %q", round, tt.label)
			}
		})
	}
}

// TestSplitPaneLabelFirstTagEnd pins the grammar rule that the tag ends at the
// first "] ": a name carrying that separator cannot be tagged, and reading it
// as one would silently widen the tag.
func TestSplitPaneLabelFirstTagEnd(t *testing.T) {
	if _, ok := splitPaneLabel("[a] b] (name)"); ok {
		t.Fatal("splitPaneLabel accepted text after the first tag end")
	}
	if tagReadable("a] b") {
		t.Fatal("tagReadable accepted a name containing the tag end")
	}
	if !tagReadable("a]b") {
		t.Fatal("tagReadable rejected a name with a bare ]")
	}
}

func TestPanePartsNamesSession(t *testing.T) {
	tests := []struct {
		name  string
		parts paneParts
		sess  SessionName
		want  bool
	}{
		{
			name:  "new pool form names the pooled session",
			parts: paneParts{agent: "gc.implementation-worker", suffix: "mc-wisp-fvwrce", id: "gcg-session-1", tagged: true},
			sess:  "gc__implementation-worker-mc-wisp-fvwrce",
			want:  true,
		},
		{
			name:  "new pool form without tail still names it",
			parts: paneParts{agent: "gc.implementation-worker", suffix: "mc-wisp-fvwrce", tagged: true},
			sess:  "gc__implementation-worker-mc-wisp-fvwrce",
			want:  true,
		},
		{
			name:  "new long form names the session by its suffix",
			parts: paneParts{agent: "gc.implementation-worker", suffix: "gc__implementation-worker-mc-wisp-fvwrce", id: "gcg-session-1", tagged: true},
			sess:  "gc__implementation-worker-mc-wisp-fvwrce",
			want:  true,
		},
		{
			name:  "legacy long form still names it",
			parts: paneParts{agent: "gc.implementation-worker", suffix: "gc__implementation-worker-mc-wisp-fvwrce"},
			sess:  "gc__implementation-worker-mc-wisp-fvwrce",
			want:  true,
		},
		{
			name:  "legacy pool form still names it",
			parts: paneParts{agent: "gc.implementation-worker", suffix: "#mc-wisp-kvqib"},
			sess:  "gc__implementation-worker-mc-wisp-kvqib",
			want:  true,
		},
		{
			name:  "town form names the town session",
			parts: paneParts{agent: "mayor", suffix: "mayor", tagged: true},
			sess:  "mayor",
			want:  true,
		},
		{
			name:  "the tail id is never the identity",
			parts: paneParts{agent: "mayor", suffix: "mayor", id: "gcg-session-1", tagged: true},
			sess:  "gcg-session-1",
			want:  false,
		},
		{
			name:  "a shortened tag names nobody",
			parts: paneParts{agent: "gc.implem…", suffix: "mc-wisp-fvwrce", tagged: true},
			sess:  "gc__implementation-worker-mc-wisp-fvwrce",
			want:  false,
		},
		{
			name:  "a shortened legacy agent names nobody",
			parts: paneParts{agent: "gc.implem…", suffix: "#mc-wisp-fvwrce"},
			sess:  "gc__implementation-worker-mc-wisp-fvwrce",
			want:  false,
		},
		{
			name:  "another session's suffix names nothing",
			parts: paneParts{agent: "gc.implementation-worker", suffix: "mc-wisp-other", tagged: true},
			sess:  "gc__implementation-worker-mc-wisp-fvwrce",
			want:  false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.parts.namesSession(tt.sess); got != tt.want {
				t.Errorf("namesSession(%q) = %v, want %v", tt.sess, got, tt.want)
			}
		})
	}
}

// TestNewPanePartsTagReadability pins that a name the tag cannot carry is
// rendered in the legacy shape rather than as an unreadable tag.
func TestNewPanePartsTagReadability(t *testing.T) {
	readable := newPaneParts("mayor", "mayor", "")
	if !readable.tagged || readable.String() != "[mayor] (mayor)" {
		t.Errorf("readable parts = %+v (%q)", readable, readable.String())
	}
	unreadable := newPaneParts("mayor] x", "mayor-x", "id")
	if unreadable.tagged || unreadable.String() != "mayor] x (mayor-x) #id" {
		t.Errorf("unreadable parts = %+v (%q)", unreadable, unreadable.String())
	}
	blank := newPaneParts("", "mayor", "")
	if blank.tagged {
		t.Error("blank agent rendered a tag")
	}
}
