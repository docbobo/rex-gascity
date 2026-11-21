package provider_test

import (
	"context"
	"errors"
	"testing"

	"github.com/docbobo/rex-gascity/internal/provider"
	"github.com/docbobo/rex-gascity/internal/rex"
	"github.com/docbobo/rex-gascity/internal/rex/faketest"
)

// psSnapshot is a Runner answering the two ps calls the agent snapshot reads: a
// wide pid, ppid, args table and a narrow pid, comm table.
func psSnapshot(wide, comms string, err error) *provider.FakeRunner {
	return &provider.FakeRunner{Handle: func(_ context.Context, c provider.RunCall) (string, error) {
		if c.Name != "ps" {
			return "", nil
		}
		if err != nil {
			return "", err
		}
		if len(c.Args) == 2 {
			return comms, nil
		}
		return wide, nil
	}}
}

// zombieStarted starts name under observe.v1 with the meta the zombie check
// reads and a readable foreground pid.
func zombieStarted(t *testing.T, runner provider.Runner, env map[string]string) (*faketest.Fake, *provider.Provider) {
	t.Helper()
	t.Setenv("GCREX_OBSERVE_V1", "1")
	fake := faketest.New()
	p := provider.New("acme", fake, provider.WithRunner(runner))
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
// re-checked against the box: a different pid is changed, never killed.
func TestKillObjectZombieChangedWhenAgentPIDDiffers(t *testing.T) {
	ctx := context.Background()
	env := map[string]string{"GT_PROCESS_NAMES": "gc-rpp-no-such-process"}
	_, p := zombieStarted(t, psSnapshot("100 1 sleep 300\n", "100 sleep\n", nil), env)
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
