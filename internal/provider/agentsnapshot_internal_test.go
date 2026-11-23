package provider

import (
	"testing"

	"github.com/docbobo/rex-gascity/internal/prochost"
)

// snapRow builds one row of gc's host process table: pid, parent pid, and the
// full command line, which the module's -ww read keeps from being cut at
// COLUMNS. Reliable is true unless a case marks the row opaque.
func snapRow(pid, ppid int, args string) prochost.Row {
	return prochost.Row{PID: pid, PPID: ppid, Args: args, Reliable: true}
}

// snapComm adds the command name from the identity read.
func snapComm(r prochost.Row, comm string) prochost.Row {
	r.Comm = comm
	return r
}

// snapUnreliable marks a row the host could not prove, such as bracketed args
// or an interpreter option that lost its value.
func snapUnreliable(r prochost.Row) prochost.Row {
	r.Reliable = false
	return r
}

func snapTable(rows ...prochost.Row) prochost.Table {
	table := make(prochost.Table, len(rows))
	for _, r := range rows {
		table[r.PID] = r
	}
	return table
}

// TestSnapshotAgent is the seam for RPP-OBSERVE-006: one gc-shaped snapshot,
// walked for every descendant of the box root, matched the way gc's table does.
func TestSnapshotAgent(t *testing.T) {
	cases := []struct {
		name      string
		table     prochost.Table
		root      int
		names     []string
		wantState string
		wantPID   int
	}{
		{
			// A Rex pane's foreground process may be the agent itself, so the
			// root is matched as well as its descendants.
			name:      "root is the agent",
			table:     snapTable(snapComm(snapRow(100, 1, "sleep 1800"), "sleep")),
			root:      100,
			names:     []string{"sleep"},
			wantState: agentAlive,
			wantPID:   100,
		},
		{
			name: "interpreter argument names the agent",
			table: snapTable(
				snapRow(100, 1, "sh -c claude"),
				snapRow(101, 100, "node /usr/local/bin/claude --flag"),
			),
			root:      100,
			names:     []string{"claude"},
			wantState: agentAlive,
			wantPID:   101,
		},
		{
			// Node 25 names its main thread, so the command line does not say
			// "node"; the command name carries it.
			name: "program name from the comm column",
			table: snapTable(
				snapRow(100, 1, "sh -c agent"),
				snapComm(snapRow(101, 100, "node-MainThread /usr/local/bin/agent"), "node-MainThread"),
			),
			root:      100,
			names:     []string{"node-MainThread"},
			wantState: agentAlive,
			wantPID:   101,
		},
		{
			// ps brackets an argument list it cannot read. The row proves
			// nothing about the agent, so the answer is unknown, never dead.
			name: "bracketed argument list is opaque",
			table: snapTable(
				snapComm(snapRow(100, 1, "sh -c agent"), "sh"),
				snapComm(snapUnreliable(snapRow(101, 100, "[node]")), "node"),
			),
			root:      100,
			names:     []string{"agent"},
			wantState: agentUnknown,
		},
		{
			// Node 25's main-thread name carries the interpreter name, so the
			// script operand behind it still names the process.
			name: "main-thread interpreter name",
			table: snapTable(
				snapComm(snapRow(100, 1, "sh -c agent"), "sh"),
				snapComm(snapRow(101, 100, "node-MainThread /usr/local/bin/agent"), "node-MainThread"),
			),
			root:      100,
			names:     []string{"agent"},
			wantState: agentAlive,
			wantPID:   101,
		},
		{
			// A command name cut to 16 bytes cannot match; the interpreter
			// argument, extension and all, still can.
			name: "truncated command name covered by the interpreter argument",
			table: snapTable(
				snapRow(100, 1, "sh -c claude"),
				snapComm(snapRow(101, 100, "node /opt/agents/claude.js --serve"), "node-MainThrea"),
			),
			root:      100,
			names:     []string{"claude"},
			wantState: agentAlive,
			wantPID:   101,
		},
		{
			name: "wrapper shell around the agent",
			table: snapTable(
				snapRow(100, 1, "sh -c claude"),
				snapRow(101, 100, "claude --mcp"),
			),
			root:      100,
			names:     []string{"claude"},
			wantState: agentAlive,
			wantPID:   101,
		},
		{
			name: "every descendant is walked, not only children",
			table: snapTable(
				snapRow(100, 1, "sh -c run"),
				snapRow(101, 100, "bash run.sh"),
				snapRow(102, 101, "sleep 1800"),
			),
			root:      100,
			names:     []string{"sleep"},
			wantState: agentAlive,
			wantPID:   102,
		},
		{
			name:      "alias kimi to kimi-code",
			table:     snapTable(snapRow(100, 1, "/opt/bin/kimi-code")),
			root:      100,
			names:     expandProcessNames([]string{"kimi"}, ""),
			wantState: agentAlive,
			wantPID:   100,
		},
		{
			// -ww keeps the whole command line, so a deep path still basenames.
			name: "long command line matched by argv0 basename",
			table: snapTable(
				snapRow(100, 1, "sh -c go"),
				snapRow(101, 100, "/usr/local/libexec/verylongagentname --serve --config /etc/x"),
			),
			root:      100,
			names:     []string{"verylongagentname"},
			wantState: agentAlive,
			wantPID:   101,
		},
		{
			name: "readable subtree with no match is dead",
			table: snapTable(
				snapRow(100, 1, "sh -c run"),
				snapRow(101, 100, "bash run.sh"),
			),
			root:      100,
			names:     []string{"sleep"},
			wantState: agentDead,
		},
		{
			name:      "no names on stdin is unknown, never dead",
			table:     snapTable(snapRow(100, 1, "sleep 1800")),
			root:      100,
			wantState: agentUnknown,
		},
		{
			name:      "no root pid is unknown",
			table:     snapTable(snapRow(100, 1, "sleep 1800")),
			root:      0,
			names:     []string{"sleep"},
			wantState: agentUnknown,
		},
		{
			name:      "unreadable snapshot is unknown, never dead",
			table:     prochost.Table{},
			root:      100,
			names:     []string{"sleep"},
			wantState: agentUnknown,
		},
		{
			// The box root is gone from the table: its subtree is not visible,
			// so a missing match proves nothing.
			name:      "root absent from the snapshot is unknown",
			table:     snapTable(snapRow(200, 1, "sleep 1800")),
			root:      100,
			names:     []string{"sleep"},
			wantState: agentUnknown,
		},
		{
			// --require takes the next argument, so the script is the operand
			// after it. Reading the preload as the script would read a live
			// agent dead and authorize its kill.
			name: "value-taking option before the interpreter script",
			table: snapTable(
				snapRow(100, 1, "sh -c agent"),
				snapRow(101, 100, "node --require /opt/preload.js /opt/agent.js"),
			),
			root:      100,
			names:     []string{"agent"},
			wantState: agentAlive,
			wantPID:   101,
		},
		{
			// A value option with no value leaves the script unidentifiable:
			// the row could be the agent, so the answer is unknown, never dead.
			name: "value-taking option without a value is unknown",
			table: snapTable(
				snapRow(100, 1, "sh -c agent"),
				snapUnreliable(snapRow(101, 100, "node --require")),
			),
			root:      100,
			names:     []string{"agent"},
			wantState: agentUnknown,
		},
		{
			name: "opaque descendant hides a possible match",
			table: snapTable(
				snapRow(100, 1, "sh -c run"),
				snapRow(101, 100, ""),
			),
			root:      100,
			names:     []string{"sleep"},
			wantState: agentUnknown,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			state, pid := snapshotAgent(c.table, c.root, c.names)
			if state != c.wantState || pid != c.wantPID {
				t.Fatalf("snapshotAgent() = %q, %d; want %q, %d", state, pid, c.wantState, c.wantPID)
			}
		})
	}
}
