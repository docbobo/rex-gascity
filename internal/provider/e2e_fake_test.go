package provider_test

import (
	"context"
	"os"
	"os/exec"
	"testing"

	"github.com/docbobo/rex-gascity/internal/rex/cli"
	"github.com/docbobo/rex-gascity/internal/rex/fakerex"
)

// e2eServer returns the client the E2E suite drives and whether it is a live
// Rex server.
//
//	REX_E2E unset, "fake":  the process-level fake (internal/rex/fakerex), so
//	                        the suite runs with no Rex app and no rex on PATH.
//	REX_E2E "1", "live":    the live Rex server; the test skips without rex.
//
// Any other value fails the test, so REX_E2E=1 never silently means the fake.
func e2eServer(t *testing.T) (client *cli.Client, live bool) {
	t.Helper()
	v := os.Getenv("REX_E2E")
	live, ok := e2eMode(v)
	if !ok {
		t.Fatalf("REX_E2E=%q: want unset, \"fake\", \"1\", or \"live\"", v)
	}
	if !live {
		return &cli.Client{Bin: fakerex.New(t).Bin}, false
	}
	if _, err := exec.LookPath("rex"); err != nil {
		t.Skipf("REX_E2E=%s selects the live Rex server, but rex is not on PATH", v)
	}
	return cli.New(), true
}

// e2eMode reports whether the REX_E2E value selects the live server, and
// whether it is a value the suite accepts. A value the suite does not know
// never falls back to the fake, so REX_E2E=1 can never silently run it.
func e2eMode(v string) (live, ok bool) {
	switch v {
	case "", "fake":
		return false, true
	case "1", "live":
		return true, true
	}
	return false, false
}

func TestE2EMode(t *testing.T) {
	for v, want := range map[string]struct{ live, ok bool }{
		"":         {false, true},
		"fake":     {false, true},
		"1":        {true, true},
		"live":     {true, true},
		"0":        {false, false},
		"true":     {false, false},
		"nonsense": {false, false},
	} {
		live, ok := e2eMode(v)
		if live != want.live || ok != want.ok {
			t.Errorf("e2eMode(%q) = %v, %v; want %v, %v", v, live, ok, want.live, want.ok)
		}
	}
}

// e2eKillCity removes the scratch session labelled gc:<city> if a test left it
// behind. It goes through client's own binary, so cleanup never needs a PATH
// rex.
func e2eKillCity(ctx context.Context, client *cli.Client, city string) {
	sessions, err := client.ListSessions(ctx)
	if err != nil {
		return
	}
	for _, s := range sessions {
		if s.Label == "gc:"+city {
			_ = client.DestroySession(ctx, s.ID)
		}
	}
}
