package launch

import (
	"slices"
	"testing"
)

func TestCommandArgv(t *testing.T) {
	sh := func(c string) []string { return []string{"/bin/sh", "-lc", c} }
	for _, c := range []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"   ", nil},
		{"sleep 300", []string{"sleep", "300"}},
		{`claude  --flag='a b' "c \"d\"" e\ f`, []string{"claude", "--flag=a b", `c "d"`, "e f"}},
		{`echo ''`, []string{"echo", ""}},
		{`echo "unterminated`, sh(`echo "unterminated`)},
		{`echo 'unterminated`, sh(`echo 'unterminated`)},
		{`echo trailing\`, sh(`echo trailing\`)},
		{"a && b", sh("a && b")},
		{"echo $HOME", sh("echo $HOME")},
		{`echo "$HOME"`, sh(`echo "$HOME"`)},
		{`echo '$HOME'`, []string{"echo", "$HOME"}},
		{"FOO=bar claude", sh("FOO=bar claude")},
		{"ls *.go", sh("ls *.go")},
	} {
		if got := CommandArgv(c.in); !slices.Equal(got, c.want) {
			t.Errorf("CommandArgv(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// NewPlan is the one launch translation: it must route the pool env and carry
// the Rex block layout, so the transport never has to translate a launch.
func TestNewPlanOwnsBlockLayout(t *testing.T) {
	plan := NewPlan(Config{
		Argv:    []string{"pi"},
		Env:     map[string]string{"GC_ALIAS": "", "GC_TEMPLATE": "r/pi", "GC_AGENT": "bead1", "GC_SESSION_ID": "bead1", "BEADS_ACTOR": "bead1"},
		WorkDir: "/work/agent",
		Label:   "dog-1",
	})
	if plan.Env["BEADS_ACTOR"] != "r/pi" {
		t.Errorf("plan env BEADS_ACTOR = %q, want r/pi", plan.Env["BEADS_ACTOR"])
	}
	if got, want := plan.Spec.Cwd, "/work/agent"; got != want {
		t.Errorf("Cwd = %q, want %q", got, want)
	}
	if plan.Spec.Layout.Block == nil {
		t.Fatal("plan has no block layout")
	}
	block := plan.Spec.Layout.Block
	if block.Flavor != terminalFlavor || block.Label != "dog-1" {
		t.Errorf("block = %+v", block)
	}
	opts := block.Options
	if opts["cwd"] != "/work/agent" {
		t.Errorf("layout cwd = %v", opts["cwd"])
	}
	cmd, _ := opts["command"].([]string)
	if len(cmd) == 0 || cmd[len(cmd)-1] != "pi" {
		t.Errorf("layout command = %v, want argv ending in pi", cmd)
	}
}
