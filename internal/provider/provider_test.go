package provider_test

import (
	"context"
	"slices"
	"testing"

	"github.com/docbobo/rex-gas-city/internal/provider"
	"github.com/docbobo/rex-gas-city/internal/rex/faketest"
)

func TestStartCreatesCitySessionRigTabAndAgentPane(t *testing.T) {
	fake := faketest.New()
	p := provider.New("acme", fake)

	err := p.Start(context.Background(), "Indigo--dog-1", provider.Config{
		Argv:    []string{"claude", "--flag"},
		WorkDir: "/work/indigo",
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	if got, want := fake.CreatedSessions, []string{"gc:acme"}; !slices.Equal(got, want) {
		t.Errorf("created sessions = %v, want %v", got, want)
	}
	if len(fake.CreatedWindows) != 1 {
		t.Fatalf("created windows = %d, want 1", len(fake.CreatedWindows))
	}
	w := fake.CreatedWindows[0]
	if w.Label != "Indigo" {
		t.Errorf("tab label = %q, want %q", w.Label, "Indigo")
	}
	if w.Spec.Label != "dog-1" {
		t.Errorf("pane label = %q, want %q", w.Spec.Label, "dog-1")
	}
	if !slices.Equal(w.Spec.Command, []string{"claude", "--flag"}) {
		t.Errorf("pane command = %v, want [claude --flag]", w.Spec.Command)
	}
	if w.Spec.Cwd != "/work/indigo" {
		t.Errorf("pane cwd = %q, want %q", w.Spec.Cwd, "/work/indigo")
	}
}
