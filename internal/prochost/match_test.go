package prochost

import "testing"

// TestMatchExactCandidates pins the one name-matching rule: exact candidate
// equality. There is no suffix match and "?" is not a wildcard.
func TestMatchExactCandidates(t *testing.T) {
	if !Match(Candidates("", "node", "/usr/local/bin/node", ""), []string{"node"}) {
		t.Fatal("argv0 base node did not match node")
	}
	if Match(Candidates("bash", "bash", "", ""), []string{"sh"}) {
		t.Fatal("bash matched sh")
	}
	if Match(Candidates("?", "?", "", ""), []string{"?"}) {
		t.Fatal(`"?" matched`)
	}
	if Match(Candidates("", "pi", "", ""), []string{""}) {
		t.Fatal("empty name matched")
	}
	for _, name := range []string{"main.js", "main"} {
		if !Match(Candidates("node", "node", "", "/app/main.js"), []string{name}) {
			t.Fatalf("interpreter script candidate did not match %q", name)
		}
	}
}

// TestSameIdentity pins the signalling identity: a different start time on the
// same pid is a different process, an equal start with a renamed comm is the
// same process, and a zombie or an opaque identity is never the same.
func TestSameIdentity(t *testing.T) {
	id := Ident{PID: 7, Name: "node", Start: "Fri Nov 20 19:44:50 2026"}
	if Same(id, Row{PID: 7, Comm: "node", Start: "Sat Nov 21 10:00:00 2026"}) {
		t.Fatal("different start matched")
	}
	if !Same(id, Row{PID: 7, Comm: "renamed", Start: id.Start}) {
		t.Fatal("equal start with renamed comm did not match")
	}
	if Same(id, Row{PID: 7, Comm: "node", Start: id.Start, Zombie: true}) {
		t.Fatal("zombie matched")
	}
	if Same(Ident{PID: 7}, Row{PID: 7}) {
		t.Fatal("opaque identity matched")
	}
}
