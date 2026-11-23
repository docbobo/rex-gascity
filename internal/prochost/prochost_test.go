package prochost

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

// TestSubtreeAndAncestor pins the reported-process walk: a root and every
// descendant, and an ancestor search that stops at init or a cycle.
func TestSubtreeAndAncestor(t *testing.T) {
	table := Table{
		10: {PID: 10, PPID: 1},
		11: {PID: 11, PPID: 10},
		12: {PID: 12, PPID: 11},
		20: {PID: 20, PPID: 1},
		21: {PID: 21, PPID: 20},
	}
	if got := Subtree(table, []int{10}); !reflect.DeepEqual(got, []int{10, 11, 12}) {
		t.Fatalf("Subtree = %v, want [10 11 12]", got)
	}
	if !AncestorIn(table, 12, map[int]bool{10: true}) {
		t.Fatal("12 did not report ancestor 10")
	}
	if AncestorIn(table, 21, map[int]bool{10: true}) {
		t.Fatal("21 reported a non-ancestor")
	}
	if AncestorIn(table, 10, map[int]bool{10: true}) {
		t.Fatal("root counted as its own ancestor")
	}
}

type stubRunner struct {
	calls   [][]string
	outputs []string
	errs    []error
}

func (s *stubRunner) Run(ctx context.Context, dir string, env []string, name string, args ...string) (string, error) {
	s.calls = append(s.calls, append([]string{name}, args...))
	i := len(s.calls) - 1
	if i < len(s.errs) && s.errs[i] != nil {
		return "", s.errs[i]
	}
	if i < len(s.outputs) {
		return s.outputs[i], nil
	}
	return "", nil
}

const identityFixture = `  501     1 Ss   node Fri Nov 20 19:44:50 2026
  502   501 S    worker Fri Nov 20 19:44:51 2026
  503     1 Z    gone Fri Nov 20 19:44:52 2026
`

// TestPSHostReadCountsAndMerge pins the read contract: identity-only is one
// ps, needArgs is two, wide rows supply membership and PPIDs, identity rows
// add comm, start and zombie, a wide-only row stays, and an identity-only row
// drops.
func TestPSHostReadCountsAndMerge(t *testing.T) {
	wide := `501 1 node server.js
502 999 worker --flag
600 502 orphan
`
	r := &stubRunner{outputs: []string{identityFixture, wide}}
	h := NewPSHost(r)

	table, err := h.Table(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.calls) != 1 {
		t.Fatalf("identity-only reads = %d, want 1", len(r.calls))
	}
	if got := table[501]; got.Comm != "node" || got.Start == "" || !got.Reliable {
		t.Fatalf("row 501 = %#v", got)
	}

	r = &stubRunner{outputs: []string{identityFixture, wide}}
	h = NewPSHost(r)
	table, err = h.Table(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.calls) != 2 {
		t.Fatalf("needArgs reads = %d, want 2", len(r.calls))
	}
	if !strings.Contains(strings.Join(r.calls[1], " "), "-ww") {
		t.Fatalf("args read lacks -ww: %v", r.calls[1])
	}
	if got := table[501]; got.Comm != "node" || got.Args != "node server.js" || got.PPID != 1 {
		t.Fatalf("merged row 501 = %#v", got)
	}
	if got := table[502]; got.PPID != 999 {
		t.Fatalf("wide PPID lost: %#v", got)
	}
	if _, ok := table[600]; !ok {
		t.Fatal("wide-only row dropped")
	}
	if _, ok := table[503]; ok {
		t.Fatal("identity-only row kept")
	}
}

// TestPSHostEmptyWideUnreadable pins the empty-wide contract: with needArgs an
// empty or failed wide read is ErrUnreadableSnapshot, never an identity-only
// table.
func TestPSHostEmptyWideUnreadable(t *testing.T) {
	for _, tc := range []struct {
		name   string
		output string
		err    error
	}{
		{"empty", "  \n", nil},
		{"failed", "", errors.New("boom")},
	} {
		r := &stubRunner{outputs: []string{identityFixture, tc.output}, errs: []error{nil, tc.err}}
		if _, err := NewPSHost(r).Table(context.Background(), true); !errors.Is(err, ErrUnreadableSnapshot) {
			t.Fatalf("%s: error = %v, want ErrUnreadableSnapshot", tc.name, err)
		}
	}
}
