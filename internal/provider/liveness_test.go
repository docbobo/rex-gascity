package provider

import (
	"context"
	"fmt"
	"testing"

	"github.com/docbobo/rex-gascity/internal/rex"
	"github.com/docbobo/rex-gascity/internal/rex/faketest"
)

func TestProcessAlive(t *testing.T) {
	exited := &rex.ExitStatus{}
	shell := &rex.Process{Name: "zsh", Path: "/bin/zsh", Argv0: "-zsh"}
	agent := rex.Process{Name: "node", Path: "/usr/bin/node", Argv0: "/opt/bin/claude"}

	tests := []struct {
		name  string
		info  rex.ProcessInfo
		names []string
		want  Liveness
	}{
		{"last exit beats a matching foreground", rex.ProcessInfo{LastExit: exited, Foreground: &agent}, []string{"claude"}, LivenessDead},
		{"last exit with no names", rex.ProcessInfo{LastExit: exited}, nil, LivenessDead},
		{"last exit beats a matching ancestor", rex.ProcessInfo{LastExit: exited, Ancestors: []rex.Process{agent}}, []string{"claude"}, LivenessDead},
		{"empty names and no exit is alive", rex.ProcessInfo{Foreground: shell}, nil, LivenessAlive},
		{"foreground matches by argv0 basename", rex.ProcessInfo{Foreground: &agent}, []string{"claude"}, LivenessAlive},
		{"foreground matches by name", rex.ProcessInfo{Foreground: &agent}, []string{"node"}, LivenessAlive},
		{"ancestor matches", rex.ProcessInfo{Foreground: shell, Ancestors: []rex.Process{agent}}, []string{"claude"}, LivenessAlive},
		// Rex reports the script an interpreter runs, so node /path/to/claude
		// matches the agent name that gc configures.
		{"foreground matches an interpreter script", rex.ProcessInfo{Foreground: &rex.Process{Name: "node", Path: "/usr/bin/node", Script: "/path/to/claude"}}, []string{"claude"}, LivenessAlive},
		{"ancestor matches an interpreter script", rex.ProcessInfo{Foreground: shell, Ancestors: []rex.Process{{Name: "bun", Script: "/opt/bin/claude"}}}, []string{"claude"}, LivenessAlive},
		// The pack matches a script argument with or without its extension.
		{"script matches without its extension", rex.ProcessInfo{Foreground: &rex.Process{Name: "deno", Script: "/work/agent.js"}}, []string{"agent"}, LivenessAlive},
		{"script matches with its extension", rex.ProcessInfo{Foreground: &rex.Process{Name: "npx", Script: "/work/agent.js"}}, []string{"agent.js"}, LivenessAlive},
		{"readable script with no match is dead", rex.ProcessInfo{Foreground: &rex.Process{Name: "node", Script: "/path/to/other"}}, []string{"claude"}, LivenessDead},
		// A fully readable table with no match is a definite dead.
		{"shell only, agent missing", rex.ProcessInfo{Foreground: shell, Ancestors: []rex.Process{*shell}}, []string{"claude"}, LivenessDead},
		// An empty table (the launcher is not at the agent yet) cannot be
		// resolved into dead: gc reads the empty answer as alive.
		{"empty table is unknown", rex.ProcessInfo{}, []string{"claude"}, LivenessUnknown},
		// An opaque process with no readable identity could be the agent.
		{"opaque process is unknown", rex.ProcessInfo{Foreground: &rex.Process{PID: 7}}, []string{"claude"}, LivenessUnknown},
		// Readable non-match plus an opaque process is still unknown.
		{"opaque ancestor with a readable foreground is unknown", rex.ProcessInfo{Foreground: shell, Ancestors: []rex.Process{{PID: 7}}}, []string{"claude"}, LivenessUnknown},
		// Empty names never match a readable non-matching process: dead, not
		// unknown, because the table was readable.
		{"empty names never match", rex.ProcessInfo{Foreground: shell}, []string{""}, LivenessDead},
		// The one name rule is exact equality: no suffix match.
		{"sh does not match bash", rex.ProcessInfo{Foreground: &rex.Process{Name: "bash", Argv0: "bash"}}, []string{"sh"}, LivenessDead},
		{"a suffix does not match", rex.ProcessInfo{Foreground: &rex.Process{Name: "claude-code", Argv0: "claude-code"}}, []string{"claude"}, LivenessDead},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := processAlive(tt.info, tt.names); got != tt.want {
				t.Errorf("processAlive = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsRunningTreatsVanishedPaneAsStopped(t *testing.T) {
	fake := faketest.New()
	fake.Sessions = []rex.Session{{ID: "session:gc:acme", Label: "gc:acme"}}
	fake.View = rex.SessionView{
		Windows: []rex.Window{{
			ID:     "window:Indigo",
			Label:  "Rig: Indigo",
			Blocks: []rex.Block{{ID: "block:dog-1", Label: "dog-1"}},
		}},
	}
	fake.ProcessErr = fmt.Errorf("rex block.method: %w", rex.ErrNotFound)
	p := New("acme", fake)

	running, err := p.IsRunning(context.Background(), "Indigo--dog-1")
	if err != nil || running {
		t.Errorf("IsRunning = %v, %v; want false, nil", running, err)
	}
}
