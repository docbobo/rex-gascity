package provider

import (
	"slices"
	"testing"
)

func TestNormalizeStartEnv(t *testing.T) {
	tests := []struct {
		name      string
		env       map[string]string
		wantAlias string
		wantSet   bool
	}{
		{"alias preserved", map[string]string{"GC_ALIAS": "a/b", "GC_TEMPLATE": "r/pi", "GC_AGENT": "sid"}, "a/b", true},
		{"empty alias derived", map[string]string{"GC_ALIAS": "", "GC_TEMPLATE": "r/pi", "GC_AGENT": "sid"}, "r/pi", true},
		{"absent alias derived", map[string]string{"GC_TEMPLATE": "r/pi", "GC_AGENT": "sid"}, "r/pi", true},
		{"unqualified template gets rig", map[string]string{"GC_TEMPLATE": "pi", "GC_RIG": "r", "GC_AGENT": "sid"}, "r/pi", true},
		{"unqualified template no rig", map[string]string{"GC_TEMPLATE": "pi", "GC_AGENT": "sid"}, "pi", true},
		{"template equals agent", map[string]string{"GC_ALIAS": "", "GC_TEMPLATE": "r/pi", "GC_AGENT": "r/pi"}, "", false},
		{"no template", map[string]string{"GC_ALIAS": "", "GC_AGENT": "sid"}, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := map[string]string{}
			for k, v := range tt.env {
				in[k] = v
			}
			got := normalizeStartEnv(in)
			if len(in) != len(tt.env) {
				t.Fatalf("input map mutated: %v", in)
			}
			for k, v := range tt.env {
				if in[k] != v {
					t.Fatalf("input %s mutated", k)
				}
			}
			a, ok := got["GC_ALIAS"]
			if tt.wantSet && a != tt.wantAlias {
				t.Errorf("GC_ALIAS = %q, want %q", a, tt.wantAlias)
			}
			if !tt.wantSet && a != tt.env["GC_ALIAS"] {
				t.Errorf("GC_ALIAS changed to %q (present=%v)", a, ok)
			}
			if !tt.wantSet && len(got) != len(tt.env) {
				t.Errorf("env changed: %v", got)
			}
		})
	}
}

func TestNormalizeStartEnvKeepsIdentityKeys(t *testing.T) {
	in := map[string]string{"GC_TEMPLATE": "r/pi", "GC_AGENT": "sid", "GC_SESSION_ID": "s1", "GC_INSTANCE_TOKEN": "tok"}
	got := normalizeStartEnv(in)
	if got["GC_SESSION_ID"] != "s1" || got["GC_INSTANCE_TOKEN"] != "tok" || got["GC_AGENT"] != "sid" {
		t.Errorf("identity keys changed: %v", got)
	}
}

func TestLaunchCommandEmptyEnvIsUnset(t *testing.T) {
	got := launchCommand([]string{"pi"}, map[string]string{"B": "1", "GC_ALIAS": "", "A": ""})
	want := []string{"env", "-u", "A", "-u", "GC_ALIAS", "B=1", "pi"}
	if !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}
