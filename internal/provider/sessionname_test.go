package provider

import "testing"

func TestSessionName(t *testing.T) {
	tests := []struct {
		name  string
		rig   string
		agent string
		town  bool
	}{
		{"Indigo--dog-1", "Indigo", "dog-1", false},
		{"mayor", "__town", "mayor", true},
		{"a--b--c", "a", "b--c", false},
		{"__town--mayor", "__town", "mayor", false},
		// Empty is town-level with an empty agent; callers must not pass it.
		{"", "__town", "", true},
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
		if n.TabLabel() != tt.rig || n.PaneLabel() != tt.agent {
			t.Errorf("%q: tab=%q pane=%q", tt.name, n.TabLabel(), n.PaneLabel())
		}
	}
}

func TestRexSessionLabel(t *testing.T) {
	if got := rexSessionLabel("paris"); got != "gc:paris" {
		t.Errorf("rexSessionLabel = %q", got)
	}
}
