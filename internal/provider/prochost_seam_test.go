package provider

import (
	"context"
	"strings"
	"testing"

	"github.com/docbobo/rex-gascity/internal/prochost"
	"github.com/docbobo/rex-gascity/internal/rex/faketest"
)

// TestPSHostWithStubRunner proves the real adapter over a stub runner keeps the
// merge, puts -ww on the args read only, and lets waitGone exit on the first
// scan that proves the process gone.
func TestPSHostWithStubRunner(t *testing.T) {
	ctx := context.Background()
	var calls []string
	outputs := []string{
		"  501     1 Ss   Fri Nov 20 19:44:50 2026 node\n",
		"501 1 node server.js\n",
	}
	runner := &FakeRunner{}
	call := 0
	runner.Handle = func(_ context.Context, c RunCall) (string, error) {
		calls = append(calls, strings.Join(append([]string{c.Name}, c.Args...), " "))
		out := ""
		if call < len(outputs) {
			out = outputs[call]
		}
		call++
		return out, nil
	}
	p := New("acme", faketest.New(), WithProcessHost(prochost.NewPSHost(runner)))

	table, err := p.processHost.Table(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 {
		t.Fatalf("needArgs reads = %d, want 2", len(calls))
	}
	if strings.Contains(calls[0], "-ww") {
		t.Fatalf("identity read carries -ww: %v", calls[0])
	}
	if !strings.Contains(calls[1], "-ww") {
		t.Fatalf("args read lacks -ww: %v", calls[1])
	}
	if got := table[501]; got.Comm != "node" || got.Args != "node server.js" {
		t.Fatalf("merged row 501 = %#v", got)
	}

	reapShortWaits(t)
	scans := 0
	runner.Handle = func(_ context.Context, _ RunCall) (string, error) {
		scans++
		if scans == 1 {
			return outputs[0], nil
		}
		return "", nil
	}
	left, err := p.waitGone(ctx, []prochost.Ident{{PID: 501, Name: "node"}}, reapTermWait)
	if err != nil {
		t.Fatal(err)
	}
	if len(left) != 0 {
		t.Fatalf("waitGone left = %v, want none", left)
	}
	if scans != 2 {
		t.Fatalf("waitGone scans = %d, want 2 (the proving scan ends the poll)", scans)
	}
}
