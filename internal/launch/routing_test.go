package launch

import (
	"slices"
	"testing"
)

func copyEnv(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// An unaliased pool seat must route its work query as the pool template (so
// gc's hook reads the store the routed step lives in) while keeping GC_ALIAS
// empty (so the claim is recorded under the per-seat session bead id, not a
// shared template string). See ProjectPoolRouting and GCREX-32.
func TestProjectPoolRouting(t *testing.T) {
	tests := []struct {
		name      string
		env       map[string]string
		wantActor string
		changed   bool
	}{
		{
			name:      "unaliased pool seat routes the actor to the template",
			env:       map[string]string{"GC_ALIAS": "", "GC_TEMPLATE": "r/pi", "GC_AGENT": "bead1", "GC_SESSION_ID": "bead1", "BEADS_ACTOR": "bead1", "GC_INSTANCE_TOKEN": "tok"},
			wantActor: "r/pi",
			changed:   true,
		},
		{
			name:      "absent alias routes too",
			env:       map[string]string{"GC_TEMPLATE": "r/pi", "GC_AGENT": "bead1", "GC_SESSION_ID": "bead1", "BEADS_ACTOR": "bead1"},
			wantActor: "r/pi",
			changed:   true,
		},
		{
			name:      "non-empty alias preserved verbatim",
			env:       map[string]string{"GC_ALIAS": "a/b", "GC_TEMPLATE": "r/pi", "GC_AGENT": "bead1", "GC_SESSION_ID": "bead1", "BEADS_ACTOR": "bead1"},
			wantActor: "bead1",
		},
		{
			name:      "template equals agent unchanged",
			env:       map[string]string{"GC_ALIAS": "", "GC_TEMPLATE": "r/pi", "GC_AGENT": "r/pi", "GC_SESSION_ID": "bead1", "BEADS_ACTOR": "bead1"},
			wantActor: "bead1",
		},
		{
			name:      "no template unchanged",
			env:       map[string]string{"GC_ALIAS": "", "GC_AGENT": "bead1", "GC_SESSION_ID": "bead1", "BEADS_ACTOR": "bead1"},
			wantActor: "bead1",
		},
		{
			name:      "no session id unchanged",
			env:       map[string]string{"GC_ALIAS": "", "GC_TEMPLATE": "r/pi", "GC_AGENT": "bead1", "BEADS_ACTOR": "bead1"},
			wantActor: "bead1",
		},
		{
			name:      "already routed unchanged",
			env:       map[string]string{"GC_ALIAS": "", "GC_TEMPLATE": "r/pi", "GC_AGENT": "bead1", "GC_SESSION_ID": "bead1", "BEADS_ACTOR": "r/pi"},
			wantActor: "r/pi",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := copyEnv(tt.env)
			got := projectPoolRouting(in)
			for k, v := range tt.env {
				if in[k] != v {
					t.Fatalf("input %s mutated: %q", k, in[k])
				}
			}
			if got["BEADS_ACTOR"] != tt.wantActor {
				t.Errorf("BEADS_ACTOR = %q, want %q", got["BEADS_ACTOR"], tt.wantActor)
			}
			if got["GC_ALIAS"] != tt.env["GC_ALIAS"] {
				t.Errorf("GC_ALIAS = %q, want %q", got["GC_ALIAS"], tt.env["GC_ALIAS"])
			}
			if tt.changed {
				for _, k := range []string{"GC_AGENT", "GC_SESSION_ID", "GC_INSTANCE_TOKEN"} {
					if v, ok := tt.env[k]; ok && got[k] != v {
						t.Errorf("%s = %q, want %q", k, got[k], v)
					}
				}
			} else if len(got) != len(tt.env) {
				t.Errorf("env changed: %v", got)
			}
		})
	}
}

// The launched pane must never carry a GC_ALIAS value (gc would record it as
// the claim assignee) and must carry the template as its routing actor.
func TestProjectPoolRoutingLaunchcommand(t *testing.T) {
	in := map[string]string{"GC_ALIAS": "", "GC_TEMPLATE": "r/pi", "GC_AGENT": "bead1", "GC_SESSION_ID": "bead1", "BEADS_ACTOR": "bead1"}
	got := command([]string{"pi"}, projectPoolRouting(in))
	want := []string{"env", "-u", "GC_ALIAS", "BEADS_ACTOR=r/pi", "GC_AGENT=bead1", "GC_SESSION_ID=bead1", "GC_TEMPLATE=r/pi", "pi"}
	if !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestLaunchCommandEmptyEnvIsUnset(t *testing.T) {
	got := command([]string{"pi"}, map[string]string{"B": "1", "GC_ALIAS": "", "A": ""})
	want := []string{"env", "-u", "A", "-u", "GC_ALIAS", "B=1", "pi"}
	if !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}
