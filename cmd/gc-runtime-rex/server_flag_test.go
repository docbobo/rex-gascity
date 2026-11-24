package main

import (
	"testing"

	"github.com/docbobo/rex-gascity/internal/rex/cli"
)

// GC_REX_SERVER is the opt-in for a city-owned server. Set, the client
// addresses it; unset, the client keeps the ambient server.
func TestLiveDepsServerFromEnv(t *testing.T) {
	t.Setenv("GC_REX_BIN", "/opt/rex")
	t.Setenv("GC_REX_SERVER", "unix:///run/gc/acme/rex.sock")

	c, ok := liveDeps().newClient().(*cli.Client)
	if !ok {
		t.Fatalf("liveDeps client = %T, want *cli.Client", liveDeps().newClient())
	}
	if c.Server != "unix:///run/gc/acme/rex.sock" {
		t.Errorf("Server = %q, want the GC_REX_SERVER value", c.Server)
	}
	if c.Bin != "/opt/rex" {
		t.Errorf("Bin = %q, want /opt/rex", c.Bin)
	}
}

func TestLiveDepsNoServerByDefault(t *testing.T) {
	t.Setenv("GC_REX_SERVER", "")

	c, ok := liveDeps().newClient().(*cli.Client)
	if !ok {
		t.Fatalf("liveDeps client = %T, want *cli.Client", liveDeps().newClient())
	}
	if c.Server != "" {
		t.Errorf("Server = %q, want empty with GC_REX_SERVER unset", c.Server)
	}
}
