package provider

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestSessionName(t *testing.T) {
	tests := []struct {
		name  string
		rig   string
		agent string
		town  bool
	}{
		{"Indigo--dog-1", "Indigo", "dog-1", false},
		{"mayor", "", "mayor", true},
		{"a--b--c", "a", "b--c", false},
		{"__town--mayor", "__town", "mayor", false},
		// Empty is town-level with an empty agent; callers must not pass it.
		{"", "", "", true},
	}
	for _, tt := range tests {
		n := SessionName(tt.name)
		if n.Rig() != tt.rig || n.Agent() != tt.agent || n.IsTown() != tt.town {
			t.Errorf("%q: got rig=%q agent=%q town=%v, want %q %q %v",
				tt.name, n.Rig(), n.Agent(), n.IsTown(), tt.rig, tt.agent, tt.town)
		}
		if n.String() != tt.name {
			t.Errorf("%q: String() = %q", tt.name, n.String())
		}
		if n.PaneLabel() != tt.agent {
			t.Errorf("%q: pane=%q, want %q", tt.name, n.PaneLabel(), tt.agent)
		}
	}
}

func TestTabLabel(t *testing.T) {
	town := SessionName("mayor")
	rig := SessionName("Indigo--dog-1")
	long := strings.Repeat("é", maxRexLabel+10)
	longRig := strings.Repeat("é", maxRexLabel+10)
	tests := []struct {
		desc string
		name SessionName
		env  map[string]string
		town string
		want string
	}{
		{"pin names the town tab", town, nil, townTabLabel("my-city"), "my-city"},
		{"no pin falls back", town, nil, townTabLabel(""), "Gas City"},
		{"pin default is not special", town, nil, townTabLabel("default"), "default"},
		{"rig from GC_RIG", rig, map[string]string{"GC_RIG": "Other"}, townTabLabel("my-city"), "Rig: Other"},
		{"rig from name split", rig, nil, townTabLabel("my-city"), "Rig: Indigo"},
		{"stray GC_RIG on a town name", town, map[string]string{"GC_RIG": "Other"}, townTabLabel("my-city"), "my-city"},
		{"over-long pin clamps", town, nil, townTabLabel(long), string([]rune(long)[:maxRexLabel-1]) + "…"},
		{"over-long rig keeps prefix", rig, map[string]string{"GC_RIG": longRig}, "", "Rig: " + string([]rune(longRig)[:maxRexLabel-6]) + "…"},
	}
	for _, tt := range tests {
		got := tabLabel(tt.name, tt.env, tt.town)
		if got != tt.want {
			t.Errorf("%s: tabLabel = %q, want %q", tt.desc, got, tt.want)
		}
		if n := utf8.RuneCountInString(got); n > maxRexLabel {
			t.Errorf("%s: tabLabel has %d runes, want <= %d", tt.desc, n, maxRexLabel)
		}
		if !utf8.ValidString(got) {
			t.Errorf("%s: tabLabel %q is not valid UTF-8", tt.desc, got)
		}
	}
}

func TestClampRexLabel(t *testing.T) {
	short := strings.Repeat("a", maxRexLabel)
	if got := clampRexLabel(short); got != short {
		t.Errorf("clampRexLabel(128) changed a label at the limit")
	}
	long := short + "bc"
	got := clampRexLabel(long)
	if n := utf8.RuneCountInString(got); n != maxRexLabel {
		t.Errorf("clampRexLabel over-long = %d runes, want %d", n, maxRexLabel)
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("clampRexLabel over-long = %q, want a trailing ellipsis", got)
	}
	multi := strings.Repeat("é", maxRexLabel) + "é"
	got = clampRexLabel(multi)
	if !utf8.ValidString(got) {
		t.Errorf("clampRexLabel split a UTF-8 sequence: %q", got)
	}
	if n := utf8.RuneCountInString(got); n != maxRexLabel {
		t.Errorf("clampRexLabel multi-byte = %d runes, want %d", n, maxRexLabel)
	}
}

func TestRexSessionLabel(t *testing.T) {
	if got := rexSessionLabel("paris"); got != "gc:paris" {
		t.Errorf("rexSessionLabel = %q", got)
	}
}
