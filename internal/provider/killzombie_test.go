package provider_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/docbobo/rex-gascity/internal/prochost"
	"github.com/docbobo/rex-gascity/internal/provider"
	"github.com/docbobo/rex-gascity/internal/rex"
	"github.com/docbobo/rex-gascity/internal/rex/faketest"
)

// zombieSnapshot builds the zombie tests' host: the observe read answers rows,
// the reap read answers none because a zombie kill closes the pane. err fails
// every observe read.
func zombieSnapshot(err error, rows ...prochost.Row) *prochost.Fake {
	host := &prochost.Fake{}
	host.OnTable = func(_ int, needArgs bool) (prochost.Table, error) {
		if err != nil {
			return nil, err
		}
		if !needArgs {
			return prochost.Table{}, nil
		}
		if host.TableField != nil {
			return host.TableField, nil
		}
		return reapTableOf(rows...), nil
	}
	return host
}

// zombieRow is one typed host row for the zombie tests.
func zombieRow(pid, ppid int, comm, args string) prochost.Row {
	return prochost.Row{PID: pid, PPID: ppid, Comm: comm, Args: args, Reliable: true}
}

// zombieStarted starts name under observe.v1 with the meta the zombie check
// reads and a readable foreground pid.
func zombieStarted(t *testing.T, host prochost.Host, env map[string]string) (*faketest.Fake, *provider.Provider) {
	t.Helper()
	t.Setenv("GCREX_OBSERVE_V1", "1")
	fake := faketest.New()
	p := provider.New("acme", fake, provider.WithProcessHost(host))
	name := "Indigo--dog-1"
	if err := p.Start(context.Background(), name, provider.Config{Argv: []string{"sleep", "300"}, Env: env}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	fake.Processes = map[rex.BlockID]rex.ProcessInfo{
		blockOf(t, p, name): {Foreground: &rex.Process{PID: 100, Name: "sleep"}},
	}
	return fake, p
}

// TestKillObjectZombieKillsDeadAgentBox proves RPP-OBSERVE-010's zombie kill: a
// box whose recorded agent name is provably gone from its process subtree is
// killed, and the name answers absent afterwards.
func TestKillObjectZombieKillsDeadAgentBox(t *testing.T) {
	ctx := context.Background()
	env := map[string]string{"GT_PROCESS_NAMES": "gc-rpp-no-such-process"}
	_, p := zombieStarted(t, zombieSnapshot(nil, zombieRow(100, 1, "sleep", "sleep 300")), env)
	name := "Indigo--dog-1"
	ans, err := p.Observe(ctx, name, []string{"gc-rpp-no-such-process"})
	if err != nil || ans.Object == nil || ans.State != provider.ObserveRunning || ans.Agent != "dead" {
		t.Fatalf("Observe = %+v, %v; want a running box whose agent is dead", ans, err)
	}

	res, err := p.KillObject(ctx, name, provider.KillObjectRequest{Object: *ans.Object, Mode: "zombie"})
	if err != nil {
		t.Fatalf("KillObject: %v", err)
	}
	if res != "killed" {
		t.Errorf("result = %q, want killed", res)
	}
	after, err := p.Observe(ctx, name, nil)
	if err != nil || after.State != provider.ObserveAbsent {
		t.Errorf("Observe after the kill = %+v, %v; want absent", after, err)
	}
}

// TestKillObjectZombieLeavesLiveAgentBox proves the zombie check never kills a
// box whose agent name is still in its process subtree.
func TestKillObjectZombieLeavesLiveAgentBox(t *testing.T) {
	ctx := context.Background()
	env := map[string]string{"GT_PROCESS_NAMES": "gc-rpp-no-such-process"}
	_, p := zombieStarted(t, zombieSnapshot(nil, zombieRow(100, 1, "gc-rpp-no-such-process", "gc-rpp-no-such-process")), env)
	name := "Indigo--dog-1"
	ans, err := p.Observe(ctx, name, nil)
	if err != nil || ans.Object == nil {
		t.Fatalf("Observe = %+v, %v; want a box with an object", ans, err)
	}

	res, err := p.KillObject(ctx, name, provider.KillObjectRequest{Object: *ans.Object, Mode: "zombie"})
	if err != nil {
		t.Fatalf("KillObject: %v", err)
	}
	if res != "changed" {
		t.Errorf("result = %q, want changed", res)
	}
	if after, err := p.Observe(ctx, name, []string{"gc-rpp-no-such-process"}); err != nil || after.State != provider.ObserveRunning || after.Agent != "alive" {
		t.Errorf("Observe after a refused zombie kill = %+v, %v; want the live-agent box still there", after, err)
	}
}

// TestKillObjectZombieChangedWhenSnapshotUnreadable proves an unreadable
// process table never reads as a dead agent: zombie answers changed.
func TestKillObjectZombieChangedWhenSnapshotUnreadable(t *testing.T) {
	ctx := context.Background()
	env := map[string]string{"GT_PROCESS_NAMES": "gc-rpp-no-such-process"}
	_, p := zombieStarted(t, zombieSnapshot(errors.New("ps: no snapshot")), env)
	name := "Indigo--dog-1"
	ans, err := p.Observe(ctx, name, nil)
	if err != nil || ans.Object == nil {
		t.Fatalf("Observe = %+v, %v; want a box with an object", ans, err)
	}
	res, err := p.KillObject(ctx, name, provider.KillObjectRequest{Object: *ans.Object, Mode: "zombie"})
	if err != nil {
		t.Fatalf("KillObject: %v", err)
	}
	if res != "changed" {
		t.Errorf("result = %q, want changed", res)
	}
}

// TestKillObjectZombieChangedWhenAgentPIDDiffers proves a given agent_pid is
// re-checked against the box: a live pid outside its subtree is changed, never
// killed. An absent pid is not outside: it may be the agent that exited.
func TestKillObjectZombieChangedWhenAgentPIDDiffers(t *testing.T) {
	ctx := context.Background()
	env := map[string]string{"GT_PROCESS_NAMES": "gc-rpp-no-such-process"}
	_, p := zombieStarted(t, zombieSnapshot(nil, zombieRow(100, 1, "sleep", "sleep 300"), zombieRow(999, 1, "other-agent", "other-agent")), env)
	name := "Indigo--dog-1"
	ans, err := p.Observe(ctx, name, nil)
	if err != nil || ans.Object == nil {
		t.Fatalf("Observe = %+v, %v; want a box with an object", ans, err)
	}
	res, err := p.KillObject(ctx, name, provider.KillObjectRequest{Object: *ans.Object, Mode: "zombie", AgentPID: "999"})
	if err != nil {
		t.Fatalf("KillObject: %v", err)
	}
	if res != "changed" {
		t.Errorf("result = %q, want changed", res)
	}
}

// TestKillObjectZombieKillsWhenAgentPIDMatches proves a matching agent_pid is
// no obstacle to the kill.
func TestKillObjectZombieKillsWhenAgentPIDMatches(t *testing.T) {
	ctx := context.Background()
	env := map[string]string{"GT_PROCESS_NAMES": "gc-rpp-no-such-process"}
	_, p := zombieStarted(t, zombieSnapshot(nil, zombieRow(100, 1, "sleep", "sleep 300")), env)
	name := "Indigo--dog-1"
	ans, err := p.Observe(ctx, name, nil)
	if err != nil || ans.Object == nil {
		t.Fatalf("Observe = %+v, %v; want a box with an object", ans, err)
	}
	res, err := p.KillObject(ctx, name, provider.KillObjectRequest{Object: *ans.Object, Mode: "zombie", AgentPID: "100"})
	if err != nil {
		t.Fatalf("KillObject: %v", err)
	}
	if res != "killed" {
		t.Errorf("result = %q, want killed", res)
	}
}

// TestKillObjectZombieChangedWithoutProcessNames proves a record with no
// GT_PROCESS_NAMES cannot prove its agent dead, so zombie answers changed.
func TestKillObjectZombieChangedWithoutProcessNames(t *testing.T) {
	ctx := context.Background()
	_, p := zombieStarted(t, zombieSnapshot(nil, zombieRow(100, 1, "sleep", "sleep 300")), nil)
	name := "Indigo--dog-1"
	ans, err := p.Observe(ctx, name, nil)
	if err != nil || ans.Object == nil {
		t.Fatalf("Observe = %+v, %v; want a box with an object", ans, err)
	}
	res, err := p.KillObject(ctx, name, provider.KillObjectRequest{Object: *ans.Object, Mode: "zombie"})
	if err != nil {
		t.Fatalf("KillObject: %v", err)
	}
	if res != "changed" {
		t.Errorf("result = %q, want changed", res)
	}
}

// TestKillObjectZombieChangedWhenAttached proves zombie never kills a box a
// human is watching, even with a dead agent.
func TestKillObjectZombieChangedWhenAttached(t *testing.T) {
	ctx := context.Background()
	env := map[string]string{"GT_PROCESS_NAMES": "gc-rpp-no-such-process"}
	fake, p := zombieStarted(t, zombieSnapshot(nil, zombieRow(100, 1, "sleep", "sleep 300")), env)
	name := "Indigo--dog-1"
	fake.Clients = []rex.ClientInfo{attachApp}
	for i := range fake.View.Windows {
		for j := range fake.View.Windows[i].Blocks {
			if fake.View.Windows[i].Blocks[j].ID == blockOf(t, p, name) {
				fake.View.Windows[i].Blocks[j].Owner = attachApp.ID
			}
		}
	}
	fake.View.Windows = append(fake.View.Windows, rex.Window{ID: "window:Other"})
	fake.View.ActiveWindowID = fake.View.Windows[0].ID

	ans, err := p.Observe(ctx, name, nil)
	if err != nil || ans.Object == nil {
		t.Fatalf("Observe = %+v, %v; want a box with an object", ans, err)
	}
	res, err := p.KillObject(ctx, name, provider.KillObjectRequest{Object: *ans.Object, Mode: "zombie"})
	if err != nil {
		t.Fatalf("KillObject: %v", err)
	}
	if res != "changed" {
		t.Errorf("result = %q, want changed", res)
	}
}

// TestKillObjectZombieKillsWhenAgentPIDIsADescendant proves agent_pid names the
// matched agent, which Observe reports even when the agent is a descendant of
// the foreground root: after that pid exits, zombie must still reap the box.
func TestKillObjectZombieKillsWhenAgentPIDIsADescendant(t *testing.T) {
	ctx := context.Background()
	env := map[string]string{"GT_PROCESS_NAMES": "gc-rpp-no-such-process"}
	host := zombieSnapshot(nil, zombieRow(100, 1, "sleep", "sleep 300"), zombieRow(105, 100, "gc-rpp-no-such-process", "gc-rpp-no-such-process"))
	_, p := zombieStarted(t, host, env)
	name := "Indigo--dog-1"
	ans, err := p.Observe(ctx, name, []string{"gc-rpp-no-such-process"})
	if err != nil || ans.Object == nil || ans.AgentPID != "105" {
		t.Fatalf("Observe = %+v, %v; want the matched descendant pid 105", ans, err)
	}
	// The agent exits; the root and the host stay readable.
	host.TableField = reapTableOf(zombieRow(100, 1, "sleep", "sleep 300"))

	res, err := p.KillObject(ctx, name, provider.KillObjectRequest{Object: *ans.Object, Mode: "zombie", AgentPID: ans.AgentPID})
	if err != nil {
		t.Fatalf("KillObject: %v", err)
	}
	if res != "killed" {
		t.Errorf("result = %q, want killed for the reported agent's pid", res)
	}
}

// TestKillObjectZombieChangedWhenAgentPIDIsAnotherBox proves a live pid outside
// the box's subtree still names another box, so zombie answers changed.
func TestKillObjectZombieChangedWhenAgentPIDIsAnotherBox(t *testing.T) {
	ctx := context.Background()
	env := map[string]string{"GT_PROCESS_NAMES": "gc-rpp-no-such-process"}
	_, p := zombieStarted(t, zombieSnapshot(nil, zombieRow(100, 1, "sleep", "sleep 300"), zombieRow(999, 1, "other-agent", "other-agent")), env)
	name := "Indigo--dog-1"
	ans, err := p.Observe(ctx, name, nil)
	if err != nil || ans.Object == nil {
		t.Fatalf("Observe = %+v, %v; want a box with an object", ans, err)
	}
	res, err := p.KillObject(ctx, name, provider.KillObjectRequest{Object: *ans.Object, Mode: "zombie", AgentPID: "999"})
	if err != nil {
		t.Fatalf("KillObject: %v", err)
	}
	if res != "changed" {
		t.Errorf("result = %q, want changed for another box's pid", res)
	}
}

// TestKillObjectZombieChangedForAdoptedPane proves an adopted record's stored
// process names cannot prove its pane's agent dead: they belong to an earlier
// incarnation, so zombie answers changed even when the snapshot walks the root
// without finding one of them.
func TestKillObjectZombieChangedForAdoptedPane(t *testing.T) {
	ctx := context.Background()
	t.Setenv("GCREX_OBSERVE_V1", "1")
	fake := faketest.New()
	store := provider.NewMemoryStore()
	p := provider.New("acme", fake,
		provider.WithBindingStore(store),
		provider.WithProcessHost(zombieSnapshot(nil, zombieRow(100, 1, "sleep", "sleep 300"))))
	name := "Indigo--dog-1"
	if err := p.Start(ctx, name, provider.Config{
		Argv: []string{"sleep", "300"},
		Env:  map[string]string{"GT_PROCESS_NAMES": "gc-rpp-no-such-process"},
	}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	bd, ok := p.Binding(name)
	if !ok {
		t.Fatalf("binding for %s not recorded", name)
	}
	fake.Processes = map[rex.BlockID]rex.ProcessInfo{bd.BlockID: {Foreground: &rex.Process{PID: 100, Name: "sleep"}}}
	bd.Adopted = true
	store.Save(bd)

	ans, err := p.Observe(ctx, name, nil)
	if err != nil || ans.Object == nil {
		t.Fatalf("Observe = %+v, %v; want a box with an object", ans, err)
	}
	res, err := p.KillObject(ctx, name, provider.KillObjectRequest{Object: *ans.Object, Mode: "zombie"})
	if err != nil {
		t.Fatalf("KillObject: %v", err)
	}
	if res != "changed" {
		t.Errorf("result = %q, want changed for an adopted pane", res)
	}
}

// TestKillObjectZombieRereadsForeground guards kill-object's fresh read: the
// zombie root comes from a read later than boxState's. boxState reads root 100
// (sleep); every later read names root 200, the agent, so the kill answers
// changed and closes nothing. Reusing boxState's reading would walk from 100,
// find no agent and reap a live one.
func TestKillObjectZombieRereadsForeground(t *testing.T) {
	ctx := context.Background()
	env := map[string]string{"GT_PROCESS_NAMES": "gc-rpp-no-such-process"}
	fake, p := zombieStarted(t, zombieSnapshot(nil,
		zombieRow(100, 1, "sleep", "sleep 300"),
		zombieRow(200, 1, "gc-rpp-no-such-process", "gc-rpp-no-such-process"),
	), env)
	name := "Indigo--dog-1"
	ans, err := p.Observe(ctx, name, nil)
	if err != nil || ans.Object == nil {
		t.Fatalf("Observe = %+v, %v; want a box with an object", ans, err)
	}
	fake.ProcessFunc = sequencedProcess(
		rex.ProcessInfo{Foreground: &rex.Process{PID: 100, Name: "sleep"}},
		rex.ProcessInfo{Foreground: &rex.Process{PID: 200}},
	)

	res, err := p.KillObject(ctx, name, provider.KillObjectRequest{Object: *ans.Object, Mode: "zombie"})
	if err != nil {
		t.Fatalf("KillObject: %v", err)
	}
	if res != "changed" {
		t.Errorf("result = %q, want changed: the fresh read names the live agent", res)
	}
	if len(fake.ClosedBlocks) != 0 {
		t.Errorf("ClosedBlocks = %v, want none", fake.ClosedBlocks)
	}
}

// TestKillObjectZombieExitedRereadKeepsForeground guards the fresh read's
// foreground on an exited reply. boxState reads the box running; the later
// read reports both an exit status and foreground 100. The walk still runs
// from 100, finds no agent and kills, as before the pane-liveness module. A
// read that dropped the foreground on an exited reply would answer changed.
func TestKillObjectZombieExitedRereadKeepsForeground(t *testing.T) {
	ctx := context.Background()
	env := map[string]string{"GT_PROCESS_NAMES": "gc-rpp-no-such-process"}
	fake, p := zombieStarted(t, zombieSnapshot(nil, zombieRow(100, 1, "sleep", "sleep 300")), env)
	name := "Indigo--dog-1"
	ans, err := p.Observe(ctx, name, nil)
	if err != nil || ans.Object == nil {
		t.Fatalf("Observe = %+v, %v; want a box with an object", ans, err)
	}
	block := blockOf(t, p, name)
	fake.ProcessFunc = sequencedProcess(
		rex.ProcessInfo{Foreground: &rex.Process{PID: 100, Name: "sleep"}},
		rex.ProcessInfo{LastExit: &rex.ExitStatus{}, Foreground: &rex.Process{PID: 100, Name: "sleep"}},
	)

	res, err := p.KillObject(ctx, name, provider.KillObjectRequest{Object: *ans.Object, Mode: "zombie"})
	if err != nil {
		t.Fatalf("KillObject: %v", err)
	}
	if res != "killed" {
		t.Errorf("result = %q, want killed: the exited reread still names root 100", res)
	}
	if !slices.Equal(fake.ClosedBlocks, []rex.BlockID{block}) {
		t.Errorf("ClosedBlocks = %v, want [%s]", fake.ClosedBlocks, block)
	}
}
