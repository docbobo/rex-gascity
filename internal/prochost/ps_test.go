package prochost

import (
	"reflect"
	"testing"
)

// TestParseRowsFixture pins the identity parser against literal
// `ps -axo pid=,ppid=,stat=,comm=,lstart=` output. The command name may
// contain spaces, lstart is a fixed 24-byte column and may be absent, a
// malformed pid is skipped, and empty output is an empty table.
func TestParseRowsFixture(t *testing.T) {
	const fixture = `  501     1 Ss   Google Chrome Helper (Renderer) Fri Nov 20 19:44:50 2026
  777     1 S    /usr/libexec/some helper
  abc     1 S    bogus row Fri Nov 20 19:44:52 2026
  900     1 Z    <defunct> Fri Nov 20 19:44:53 2026
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
