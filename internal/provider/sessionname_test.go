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
		{"pin names the town tab", town, nil, townTabName("my-city"), "[city] my-city"},
		{"no pin falls back", town, nil, townTabName(""), "[city] Gas City"},
		{"pin default is not special", town, nil, townTabName("default"), "[city] default"},
		{"rig from GC_RIG", rig, map[string]string{"GC_RIG": "Other"}, townTabName("my-city"), "[rig] Other"},
		{"rig from name split", rig, nil, townTabName("my-city"), "[rig] Indigo"},
		{"GC_RIG on a town name groups by rig", town, map[string]string{"GC_RIG": "Other"}, townTabName("my-city"), "[rig] Other"},
		{"over-long pin clamps", town, nil, townTabName(long), string([]rune("[city] " + long)[:maxRexLabel-1]) + "…"},
		{"over-long rig keeps prefix", rig, map[string]string{"GC_RIG": longRig}, "", "[rig] " + string([]rune(longRig)[:maxRexLabel-7]) + "…"},
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
	if got := rexSessionLabel("paris"); got != "[city] paris" {
		t.Errorf("rexSessionLabel = %q, want %q", got, "[city] paris")
	}
	if got := legacySessionLabel("paris"); got != "gc:paris" {
		t.Errorf("legacySessionLabel = %q, want %q", got, "gc:paris")
	}
}

// TestSessionLabelsKeepsLegacyNamespace pins the accepted set and its order:
// the canonical "[city] X" label comes first so a new session is created under
// it, and "gc:X" stays accepted so a session an older provider wrote is still
// found. The set is deduplicated in order.
func TestSessionLabelsKeepsLegacyNamespace(t *testing.T) {
	labels := SessionLabels([]string{"Shady_Sands", "Shady_Sands", "other"})
	want := []string{"[city] Shady_Sands", "[city] other", "gc:Shady_Sands", "gc:other"}
	if len(labels) != len(want) {
		t.Fatalf("SessionLabels = %v, want %v", labels, want)
	}
	for i := range want {
		if labels[i] != want[i] {
			t.Fatalf("SessionLabels = %v, want %v", labels, want)
		}
	}
	if got := SessionLabels([]string{"a"})[0]; got != "[city] a" {
		t.Errorf("canonical label = %q, want %q", got, "[city] a")
	}
}
