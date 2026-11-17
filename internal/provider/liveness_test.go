package provider

import (
	"testing"

	"github.com/docbobo/rex-gas-city/internal/rex"
)

func TestProcessAlive(t *testing.T) {
	exited := &rex.ExitStatus{}
	shell := &rex.Process{Name: "zsh", Path: "/bin/zsh", Argv0: "-zsh"}
	agent := rex.Process{Name: "node", Path: "/usr/bin/node", Argv0: "/opt/bin/claude"}

	tests := []struct {
		name  string
		info  rex.ProcessInfo
		names []string
		want  bool
	}{
		{"last exit beats a matching foreground", rex.ProcessInfo{LastExit: exited, Foreground: &agent}, []string{"claude"}, false},
		{"last exit with no names", rex.ProcessInfo{LastExit: exited}, nil, false},
		{"last exit beats a matching ancestor", rex.ProcessInfo{LastExit: exited, Ancestors: []rex.Process{agent}}, []string{"claude"}, false},
		{"empty names and no exit is alive", rex.ProcessInfo{Foreground: shell}, nil, true},
		{"foreground matches by argv0 basename", rex.ProcessInfo{Foreground: &agent}, []string{"claude"}, true},
		{"foreground matches by name", rex.ProcessInfo{Foreground: &agent}, []string{"node"}, true},
		{"ancestor matches", rex.ProcessInfo{Foreground: shell, Ancestors: []rex.Process{agent}}, []string{"claude"}, true},
		{"shell only, agent missing", rex.ProcessInfo{Foreground: shell, Ancestors: []rex.Process{*shell}}, []string{"claude"}, false},
		{"empty names never match", rex.ProcessInfo{Foreground: &rex.Process{}}, []string{""}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := processAlive(tt.info, tt.names); got != tt.want {
				t.Errorf("processAlive = %v, want %v", got, tt.want)
			}
		})
	}
}
