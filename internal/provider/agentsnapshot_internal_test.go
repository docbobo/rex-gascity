package provider

import (
	"testing"
)

// snapRow formats one row of gc's wide liveness snapshot (ps -ww -axo
// pid=,ppid=,args=): pid, parent pid, and the full command line, which -ww
// keeps from being cut at COLUMNS.
func snapRow(pid, ppid, args string) string {
	return pid + " " + ppid + " " + args + "\n"
}

// commRow formats one row of the narrow snapshot (ps -axo pid=,comm=) that
// carries the command name.
func commRow(pid, comm string) string {
	return pid + " " + comm + "\n"
}

// TestSnapshotAgent is the seam for RPP-OBSERVE-006: one gc-shaped snapshot,
// walked for every descendant of the box root, matched the way gc's table does.
func TestSnapshotAgent(t *testing.T) {
	cases := []struct {
		name      string
		ps        string
		comms     string
		root      int
		names     []string
		wantState string
		wantPID   int
	}{
		{
			// A Rex pane's foreground process may be the agent itself, so the
			// root is matched as well as its descendants.
			name:      "root is the agent",
			ps:        snapRow("100", "1", "sleep 1800"),
			comms:     commRow("100", "sleep"),
			root:      100,
			names:     []string{"sleep"},
			wantState: agentAlive,
			wantPID:   100,
		},
		{
			name: "interpreter argument names the agent",
			ps: snapRow("100", "1", "sh -c claude") +
				snapRow("101", "100", "node /usr/local/bin/claude --flag"),
			root:      100,
			names:     []string{"claude"},
			wantState: agentAlive,
			wantPID:   101,
		},
		{
			// Node 25 names its main thread, so the command line does not say
			// "node"; the command name of the narrow snapshot carries it.
			name: "program name from the comm snapshot",
			ps: snapRow("100", "1", "sh -c agent") +
				snapRow("101", "100", "node-MainThread /usr/local/bin/agent"),
			comms:     commRow("101", "node-MainThread"),
			root:      100,
			names:     []string{"node-MainThread"},
			wantState: agentAlive,
			wantPID:   101,
		},
		{
			// A command name cut to 16 bytes cannot match; the interpreter
			// argument, extension and all, still can.
			name: "truncated command name covered by the interpreter argument",
			ps: snapRow("100", "1", "sh -c claude") +
				snapRow("101", "100", "node /opt/agents/claude.js --serve"),
			comms:     commRow("101", "node-MainThrea"),
			root:      100,
			names:     []string{"claude"},
			wantState: agentAlive,
			wantPID:   101,
		},
		{
			name: "wrapper shell around the agent",
			ps: snapRow("100", "1", "sh -c claude") +
				snapRow("101", "100", "claude --mcp"),
			root:      100,
			names:     []string{"claude"},
			wantState: agentAlive,
			wantPID:   101,
		},
		{
			name: "every descendant is walked, not only children",
			ps: snapRow("100", "1", "sh -c run") +
				snapRow("101", "100", "bash run.sh") +
				snapRow("102", "101", "sleep 1800"),
			root:      100,
			names:     []string{"sleep"},
			wantState: agentAlive,
			wantPID:   102,
		},
		{
			name:      "alias kimi to kimi-code",
			ps:        snapRow("100", "1", "/opt/bin/kimi-code"),
			root:      100,
			names:     expandProcessNames([]string{"kimi"}, ""),
			wantState: agentAlive,
			wantPID:   100,
		},
		{
			// -ww keeps the whole command line, so a deep path still basenames.
			name: "long command line matched by argv0 basename",
			ps: snapRow("100", "1", "sh -c go") +
				snapRow("101", "100", "/usr/local/libexec/verylongagentname --serve --config /etc/x"),
			root:      100,
			names:     []string{"verylongagentname"},
			wantState: agentAlive,
			wantPID:   101,
		},
		{
			name: "readable subtree with no match is dead",
			ps: snapRow("100", "1", "sh -c run") +
				snapRow("101", "100", "bash run.sh"),
			root:      100,
			names:     []string{"sleep"},
			wantState: agentDead,
		},
		{
			name:      "no names on stdin is unknown, never dead",
			ps:        snapRow("100", "1", "sleep 1800"),
			root:      100,
			wantState: agentUnknown,
		},
		{
			name:      "no root pid is unknown",
			ps:        snapRow("100", "1", "sleep 1800"),
			root:      0,
			names:     []string{"sleep"},
			wantState: agentUnknown,
		},
		{
			name:      "unreadable snapshot is unknown, never dead",
			ps:        "",
			root:      100,
			names:     []string{"sleep"},
			wantState: agentUnknown,
		},
		{
			// The box root is gone from the table: its subtree is not visible,
			// so a missing match proves nothing.
			name:      "root absent from the snapshot is unknown",
			ps:        snapRow("200", "1", "sleep 1800"),
			root:      100,
			names:     []string{"sleep"},
			wantState: agentUnknown,
		},
		{
			name: "opaque descendant hides a possible match",
			ps: snapRow("100", "1", "sh -c run") +
				snapRow("101", "100", ""),
			root:      100,
			names:     []string{"sleep"},
			wantState: agentUnknown,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			state, pid := snapshotAgent(c.ps, c.comms, c.root, c.names)
			if state != c.wantState || pid != c.wantPID {
				t.Fatalf("snapshotAgent() = %q, %d; want %q, %d", state, pid, c.wantState, c.wantPID)
			}
		})
	}
}
