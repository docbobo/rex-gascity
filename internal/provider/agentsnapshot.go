package provider

import (
	"context"
	"strings"

	"github.com/docbobo/rex-gascity/internal/prochost"
)

// gc's agent verdicts on the wire (docs/reference/exec-session-provider.md,
// "Agent liveness"). Only alive and dead are certain; unknown is omitted from
// the answer, because gc reads an absent agent as "cannot tell", never dead.
const (
	agentAlive   = "alive"
	agentDead    = "dead"
	agentUnknown = "unknown"
)

// snapshotAgent answers gc's agent state for the box whose root process is
// root, from a host snapshot the caller already took. It walks the root and
// every descendant (a Rex pane's foreground process can be the agent itself)
// and matches each one with the module's name rule. It answers unknown, never
// dead, whenever it cannot be sure: no names, an empty snapshot, a root
// missing from the snapshot (its subtree is not observable), or an opaque
// descendant that could be the agent. names must already be expanded.
func snapshotAgent(table prochost.Table, root int, names []string) (string, int) {
	if root <= 0 || len(names) == 0 {
		return agentUnknown, 0
	}
	if _, ok := table[root]; !ok {
		return agentUnknown, 0
	}
	children := make(map[int][]int, len(table))
	for pid, row := range table {
		children[row.PPID] = append(children[row.PPID], pid)
	}
	opaque := false
	queue := []int{root}
	for i := 0; i < len(queue); i++ {
		pid := queue[i]
		row := table[pid]
		if prochost.Match(prochost.RowCandidates(row), names) {
			return agentAlive, pid
		}
		if !row.Reliable || snapshotOpaque(row) {
			opaque = true
		}
		queue = append(queue, children[pid]...)
	}
	if opaque {
		return agentUnknown, 0
	}
	return agentDead, 0
}

// snapshotOpaque reports whether a row carries no readable identity: no command
// name and no command line. Such a row can neither match a name nor be ruled
// out as the agent.
func snapshotOpaque(row prochost.Row) bool {
	return row.Comm == "" && strings.TrimSpace(row.Args) == ""
}

// pidOutsideSubtree reports whether pid is a live row of the snapshot that is
// not root and not one of root's descendants. An absent pid is not outside: it
// may have exited, and an exited agent is exactly what the zombie check looks
// for. An empty snapshot is not outside either, so the caller's own proof
// requirement still decides.
func pidOutsideSubtree(table prochost.Table, root, pid int) bool {
	if len(table) == 0 || root <= 0 || pid <= 0 {
		return false
	}
	row, ok := table[pid]
	if !ok {
		return false
	}
	for i := 0; i <= len(table); i++ {
		if pid == root {
			return false
		}
		if pid <= 1 {
			return true
		}
		parent, ok := table[row.PPID]
		if !ok {
			return true
		}
		pid, row = row.PPID, parent
	}
	return false
}

// agentState takes gc's snapshot and answers the box's agent state. An
// unreadable snapshot is unknown, never dead.
func (p *Provider) agentState(ctx context.Context, root int, names []string) (string, int) {
	if root <= 0 || len(names) == 0 {
		return agentUnknown, 0
	}
	table, err := p.processHost.Table(ctx, true)
	if err != nil {
		return agentUnknown, 0
	}
	return snapshotAgent(table, root, names)
}
