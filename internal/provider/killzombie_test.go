package provider_test

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"syscall"
	"testing"

	"github.com/docbobo/rex-gascity/internal/prochost"
	"github.com/docbobo/rex-gascity/internal/provider"
	"github.com/docbobo/rex-gascity/internal/rex"
	"github.com/docbobo/rex-gascity/internal/rex/faketest"
)

// psSnapshotHost is the host process seam the zombie tests read. The observe
// read (needArgs) answers the wide and identity snapshot. The reap read answers
// no rows: a zombie kill closes the pane, so its processes are gone.
type psSnapshotHost struct {
	wide, comms string
	err         error
	table       prochost.Table // when set, answers the observe read instead
}

func psSnapshot(wide, comms string, err error) *psSnapshotHost {
	return &psSnapshotHost{wide: wide, comms: comms, err: err}
}

func (h *psSnapshotHost) Table(_ context.Context, needArgs bool) (prochost.Table, error) {
	if h.err != nil {
		return nil, h.err
	}
	if !needArgs {
		return prochost.Table{}, nil
	}
	if h.table != nil {
		return h.table, nil
	}
	return snapshotTable(h.wide, h.comms), nil
}

func (h *psSnapshotHost) Signal(int, syscall.Signal) error { return nil }

// snapshotTable merges the wide and identity reads into one table.
func snapshotTable(wide, comms string) prochost.Table {
	table := prochost.Table{}
	for _, line := range strings.Split(comms, "\n") {
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		pid, _ := strconv.Atoi(f[0])
		table[pid] = prochost.Row{PID: pid, Comm: f[1], Reliable: true}
	}
	for _, line := range strings.Split(wide, "\n") {
		f := strings.Fields(line)
		if len(f) < 3 {
			continue
		}
		pid, _ := strconv.Atoi(f[0])
		ppid, _ := strconv.Atoi(f[1])
		row := table[pid]
		row.PID, row.PPID, row.Args, row.Reliable = pid, ppid, strings.Join(f[2:], " "), true
		table[pid] = row
	}
	return table
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
	_, p := zombieStarted(t, psSnapshot("100 1 sleep 300\n", "100 sleep\n", nil), env)
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
	_, p := zombieStarted(t, psSnapshot("100 1 gc-rpp-no-such-process\n", "100 gc-rpp-no-such-process\n", nil), env)
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
	_, p := zombieStarted(t, psSnapshot("", "", errors.New("ps: no snapshot")), env)
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
	_, p := zombieStarted(t, psSnapshot("100 1 sleep 300\n999 1 other-agent\n", "100 sleep\n999 other-agent\n", nil), env)
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
	_, p := zombieStarted(t, psSnapshot("100 1 sleep 300\n", "100 sleep\n", nil), env)
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
	_, p := zombieStarted(t, psSnapshot("100 1 sleep 300\n", "100 sleep\n", nil), nil)
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
	fake, p := zombieStarted(t, psSnapshot("100 1 sleep 300\n", "100 sleep\n", nil), env)
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
	alive := "100 1 sleep 300\n105 100 gc-rpp-no-such-process\n"
	host := psSnapshot(alive, "100 sleep\n105 gc-rpp-no-such-process\n", nil)
	_, p := zombieStarted(t, host, env)
	name := "Indigo--dog-1"
	ans, err := p.Observe(ctx, name, []string{"gc-rpp-no-such-process"})
	if err != nil || ans.Object == nil || ans.AgentPID != "105" {
		t.Fatalf("Observe = %+v, %v; want the matched descendant pid 105", ans, err)
	}
	// The agent exits; the root and the host stay readable.
	host.table = snapshotTable("100 1 sleep 300\n", "100 sleep\n")

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
	_, p := zombieStarted(t, psSnapshot("100 1 sleep 300\n999 1 other-agent\n", "100 sleep\n999 other-agent\n", nil), env)
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
		provider.WithProcessHost(psSnapshot("100 1 sleep 300\n", "100 sleep\n", nil)))
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
