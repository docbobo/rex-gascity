package prochost

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// TestParseRowsFixture pins the identity parser against literal
// `ps -axo pid=,ppid=,stat=,lstart=,comm=` output. The command name may
// contain spaces, lstart is a fixed 24-byte column and may be absent, a
// malformed pid is skipped, and empty output is an empty table.
func TestParseRowsFixture(t *testing.T) {
	const fixture = `  501     1 Ss   Fri Nov 20 19:44:50 2026 Google Chrome Helper (Renderer)
  777     1 S    /usr/libexec/some helper
  abc     1 S    Fri Nov 20 19:44:52 2026 bogus row
  900     1 Z    Fri Nov 20 19:44:53 2026 <defunct>
`
	want := Table{
		501: {PID: 501, PPID: 1, Comm: "Google Chrome Helper (Renderer)",
			Start: "Fri Nov 20 19:44:50 2026", Reliable: true},
		777: {PID: 777, PPID: 1, Comm: "/usr/libexec/some helper", Reliable: true},
		900: {PID: 900, PPID: 1, Comm: "<defunct>",
			Start: "Fri Nov 20 19:44:53 2026", Zombie: true, Reliable: true},
	}

	if got := parseRows(fixture); !reflect.DeepEqual(got, want) {
		t.Fatalf("parseRows fixture mismatch:\n got %#v\nwant %#v", got, want)
	}
	if got := parseRows(""); len(got) != 0 {
		t.Fatalf("parseRows(\"\") = %#v, want an empty table", got)
	}
}

// TestParseRowsCaptured pins the parser against output captured from macOS BSD
// ps (macOS 26.6.2, 2026-11-23). See testdata/README.md for the exact commands.
func TestParseRowsCaptured(t *testing.T) {
	want := Table{
		1: {PID: 1, PPID: 0, Comm: "(launchd)",
			Start: "Sat Oct 17 10:40:12 2026", Reliable: true},
		452: {PID: 452, PPID: 1, Comm: "(logd)",
			Start: "Sat Oct 17 10:42:14 2026", Reliable: true},
		456: {PID: 456, PPID: 1, Comm: "(fseventsd)",
			Start: "Sat Oct 17 10:42:14 2026", Reliable: true},
		462: {PID: 462, PPID: 1, Comm: "(accessoryupdater)",
			Start: "Sat Oct 17 10:42:14 2026", Reliable: true},
	}
	got := parseRows(readFixture(t, "captured-identity.txt"))
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseRows captured mismatch:\n got %#v\nwant %#v", got, want)
	}
	// REQ-005 regression: the reviewed column order cut row 462 to
	// "(accessoryupdate", which then matched that name as exact. The cut
	// string must never be a candidate.
	cut := RowCandidates(got[462])
	if Match(cut, []string{"(accessoryupdate"}) {
		t.Fatalf("truncated comm matched: %v", cut)
	}
	if !Match(cut, []string{"(accessoryupdater)"}) {
		t.Fatalf("full comm did not match: %v", cut)
	}
}

// TestMergeCaptured pins the merge of the two captured reads through the one
// row parser: wide membership and PPIDs, identity comm, start and zombie.
func TestMergeCaptured(t *testing.T) {
	ident := parseRows(readFixture(t, "captured-identity.txt"))
	got, err := mergeRows(ident, readFixture(t, "captured-wide.txt"))
	if err != nil {
		t.Fatal(err)
	}
	want := Table{
		1: {PID: 1, PPID: 0, Comm: "(launchd)", Args: "(launchd)",
			Start: "Sat Oct 17 10:40:12 2026", Reliable: true},
		452: {PID: 452, PPID: 1, Comm: "(logd)", Args: "(logd)",
			Start: "Sat Oct 17 10:42:14 2026", Reliable: true},
		456: {PID: 456, PPID: 1, Comm: "(fseventsd)", Args: "(fseventsd)",
			Start: "Sat Oct 17 10:42:14 2026", Reliable: true},
		462: {PID: 462, PPID: 1, Comm: "(accessoryupdater)", Args: "(accessoryupdater)",
			Start: "Sat Oct 17 10:42:14 2026", Reliable: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("mergeRows captured mismatch:\n got %#v\nwant %#v", got, want)
	}
}

// TestReliableArgs pins the two unreliable markers the wide parser sets.
func TestReliableArgs(t *testing.T) {
	for _, tc := range []struct {
		args string
		want bool
	}{
		{"[kernel_task]", false},
		{"node -r", false},
		{"node --eval", false},
		{"node server.js", true},
		{"node -r preload.js app.js", true},
		{"sleep 30", true},
	} {
		if got := reliableArgs(tc.args); got != tc.want {
			t.Errorf("reliableArgs(%q) = %v, want %v", tc.args, got, tc.want)
		}
	}
}

func readFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// TestPSHostMergeSets pins the named merge sets: wide rows supply membership
// and PPIDs, and identity rows only decorate. A wide-only row stays, an
// identity-only row drops, and a wide PPID wins over the identity PPID.
func TestPSHostMergeSets(t *testing.T) {
	wide := `501 1 node server.js
502 999 worker --flag
600 502 orphan
`
	r := &stubRunner{outputs: []string{identityFixture, wide}}
	table, err := NewPSHost(r).Table(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := table[600]; !ok {
		t.Fatal("wide-only row dropped")
	}
	if _, ok := table[503]; ok {
		t.Fatal("identity-only row kept")
	}
	if got := table[502].PPID; got != 999 {
		t.Fatalf("row 502 PPID = %d, want the wide PPID 999", got)
	}
	if got := table[501]; got.Comm != "node" || got.Args != "node server.js" {
		t.Fatalf("merged row 501 = %#v", got)
	}
}
