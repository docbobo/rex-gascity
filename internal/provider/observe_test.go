package provider_test

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/docbobo/rex-gascity/internal/provider"
	"github.com/docbobo/rex-gascity/internal/rex"
	"github.com/docbobo/rex-gascity/internal/rex/faketest"
)

// TestObserveAbsentForUnknownName proves RPP-OBSERVE-002: a name no record and
// no live pane holds answers absent, with no object.
func TestObserveAbsentForUnknownName(t *testing.T) {
	ctx := context.Background()
	fake := faketest.New()
	p := provider.New("acme", fake)

	ans, err := p.Observe(ctx, "Indigo--dog-1", nil)
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if ans.State != provider.ObserveAbsent {
		t.Errorf("state = %q, want %q", ans.State, provider.ObserveAbsent)
	}
	if ans.Object != nil {
		t.Errorf("object = %+v, want nil for an absent name", ans.Object)
	}
}

// TestObserveRunningCarriesObjectAndIdentity proves RPP-OBSERVE-003: a started
// box answers running with its object and the start env's identity, read from
// the meta sidecar seedStartIdentity wrote before the launch.
func TestObserveRunningCarriesObjectAndIdentity(t *testing.T) {
	ctx := context.Background()
	fake := faketest.New()
	p := provider.New("acme", fake)
	name := "Indigo--dog-1"
	env := map[string]string{
		"GC_SESSION_ID":     "sid-1",
		"GC_INSTANCE_TOKEN": "tok-1",
	}
	if err := p.Start(ctx, name, provider.Config{Argv: []string{"sleep", "300"}, Env: env}); err != nil {
		t.Fatalf("Start: %v", err)
	}

	ans, err := p.Observe(ctx, name, nil)
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if ans.State != provider.ObserveRunning {
		t.Fatalf("state = %q, want %q", ans.State, provider.ObserveRunning)
	}
	if ans.Object == nil || ans.Object.ID == "" || ans.Object.Created == "" {
		t.Errorf("object = %+v, want a non-empty id and created", ans.Object)
	}
	if ans.Identity["GC_SESSION_ID"] != "sid-1" || ans.Identity["GC_INSTANCE_TOKEN"] != "tok-1" {
		t.Errorf("identity = %v, want the start env's", ans.Identity)
	}
}

// TestKillObjectGoneForUnknownObject proves the "never kill by name" rule: an
// object that does not match the stored binding answers gone and kills nothing.
func TestKillObjectGoneForUnknownObject(t *testing.T) {
	ctx := context.Background()
	fake := faketest.New()
	p := provider.New("acme", fake)
	name := "Indigo--dog-1"
	if err := p.Start(ctx, name, provider.Config{Argv: []string{"sleep", "300"}}); err != nil {
		t.Fatalf("Start: %v", err)
	}

	res, err := p.KillObject(ctx, name, provider.KillObjectRequest{
		Object: provider.ObserveObject{ID: "not-the-object", Created: "1970-01-01T00:00:00Z"},
		Mode:   "corpse",
	})
	if err != nil {
		t.Fatalf("KillObject: %v", err)
	}
	if res != "gone" {
		t.Errorf("result = %q, want gone", res)
	}
	// The box survives a mismatch.
	if running, err := p.IsRunning(ctx, name); err != nil || !running {
		t.Errorf("IsRunning after a mismatched kill = %v, %v; want true, nil", running, err)
	}
}

// TestKillObjectCorpseLeavesRunningBox proves corpse mode never kills a box
// something still runs in: gc sends corpse believing the box is dead, and the
// box may have come back between its observe and the kill.
func TestKillObjectCorpseLeavesRunningBox(t *testing.T) {
	ctx := context.Background()
	fake := faketest.New()
	p := provider.New("acme", fake)
	name := "Indigo--dog-1"
	if err := p.Start(ctx, name, provider.Config{Argv: []string{"sleep", "300"}}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	ans, err := p.Observe(ctx, name, nil)
	if err != nil || ans.State != provider.ObserveRunning {
		t.Fatalf("Observe = %+v, %v; want a running box", ans, err)
	}

	res, err := p.KillObject(ctx, name, provider.KillObjectRequest{Object: *ans.Object, Mode: "corpse"})
	if err != nil {
		t.Fatalf("KillObject: %v", err)
	}
	if res != "live" {
		t.Errorf("result = %q, want live", res)
	}
	if running, err := p.IsRunning(ctx, name); err != nil || !running {
		t.Errorf("IsRunning after a corpse kill of a running box = %v, %v; want true, nil", running, err)
	}
}

// TestKillObjectCorpseRemovesDeadBox proves corpse mode kills the box it names
// when nothing runs in it, so gc's recycle loop can reap it.
func TestKillObjectCorpseRemovesDeadBox(t *testing.T) {
	ctx := context.Background()
	fake := faketest.New()
	p := provider.New("acme", fake)
	name := "Indigo--dog-1"
	if err := p.Start(ctx, name, provider.Config{Argv: []string{"sleep", "300"}}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	ans, err := p.Observe(ctx, name, nil)
	if err != nil || ans.State != provider.ObserveRunning {
		t.Fatalf("Observe = %+v, %v; want a running box", ans, err)
	}
	// The box's program exits; Rex keeps the pane.
	fake.Processes = map[rex.BlockID]rex.ProcessInfo{
		rex.BlockID(ans.Object.ID): {LastExit: &rex.ExitStatus{ExitCode: 0}},
	}
	dead, err := p.Observe(ctx, name, nil)
	if err != nil || dead.State != provider.ObserveDead {
		t.Fatalf("Observe = %+v, %v; want a dead box", dead, err)
	}

	res, err := p.KillObject(ctx, name, provider.KillObjectRequest{Object: *ans.Object, Mode: "corpse"})
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

// TestKillObjectZombieAnswersChanged proves zombie mode never kills while the
// agent check (RPP-OBSERVE-006) is unimplemented: the pack cannot prove the
// agent is still dead, so it answers changed and leaves the box running.
func TestKillObjectZombieAnswersChanged(t *testing.T) {
	ctx := context.Background()
	fake := faketest.New()
	p := provider.New("acme", fake)
	name := "Indigo--dog-1"
	if err := p.Start(ctx, name, provider.Config{Argv: []string{"sleep", "300"}}); err != nil {
		t.Fatalf("Start: %v", err)
	}
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
	if running, err := p.IsRunning(ctx, name); err != nil || !running {
		t.Errorf("IsRunning after a zombie kill = %v, %v; want true, nil", running, err)
	}
}

// TestKillObjectZombieGoneWhenBlockVanished proves zombie mode answers gone,
// not changed, when the object names no box: zombie is a verdict about a box
// that is still there, and a vanished one is not.
func TestKillObjectZombieGoneWhenBlockVanished(t *testing.T) {
	fake, p := started(t)
	name := "Indigo--dog-1"
	ans, err := p.Observe(context.Background(), name, nil)
	if err != nil || ans.Object == nil {
		t.Fatalf("Observe = %+v, %v; want a box with an object", ans, err)
	}
	fake.VanishedBlocks = map[rex.BlockID]bool{rex.BlockID(ans.Object.ID): true}
	fake.View.Windows[0].Blocks = nil

	res, err := p.KillObject(context.Background(), name, provider.KillObjectRequest{Object: *ans.Object, Mode: "zombie"})
	if err != nil {
		t.Fatalf("KillObject: %v", err)
	}
	if res != "gone" {
		t.Errorf("result = %q, want gone for a vanished object", res)
	}
}

// TestKillObjectRejectsUnknownMode proves gc's two modes are the only ones that
// act: any other value is an error, not a kill.
func TestKillObjectRejectsUnknownMode(t *testing.T) {
	ctx := context.Background()
	fake := faketest.New()
	p := provider.New("acme", fake)
	name := "Indigo--dog-1"
	if err := p.Start(ctx, name, provider.Config{Argv: []string{"sleep", "300"}}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	ans, err := p.Observe(ctx, name, nil)
	if err != nil || ans.Object == nil {
		t.Fatalf("Observe = %+v, %v; want a box with an object", ans, err)
	}

	for _, mode := range []string{"", "halt"} {
		res, err := p.KillObject(ctx, name, provider.KillObjectRequest{Object: *ans.Object, Mode: mode})
		if err == nil {
			t.Errorf("KillObject mode %q = %q, nil; want an error", mode, res)
		}
	}
	if running, err := p.IsRunning(ctx, name); err != nil || !running {
		t.Errorf("IsRunning after rejected kills = %v, %v; want true, nil", running, err)
	}
}

// TestInventoryReportsStateAndSkipsStopped proves inventory's entries carry
// the state observe reports, and that a stopped box is not listed.
func TestInventoryReportsStateAndSkipsStopped(t *testing.T) {
	ctx := context.Background()
	fake := faketest.New()
	p := provider.New("acme", fake)
	live, stopped := "Indigo--dog-1", "Indigo--cat-2"
	if err := p.Start(ctx, live, provider.Config{Argv: []string{"sleep", "300"}, Env: map[string]string{"GC_SESSION_ID": "sid-1"}}); err != nil {
		t.Fatalf("Start %s: %v", live, err)
	}
	if err := p.Start(ctx, stopped, provider.Config{Argv: []string{"sleep", "300"}}); err != nil {
		t.Fatalf("Start %s: %v", stopped, err)
	}
	if err := p.Stop(ctx, stopped); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	want, err := p.Observe(ctx, live, nil)
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}

	inv, err := p.Inventory(ctx)
	if err != nil {
		t.Fatalf("Inventory: %v", err)
	}
	if !inv.Complete {
		t.Errorf("complete = false, want true with the store and every box readable")
	}
	if len(inv.Sessions) != 1 || inv.Sessions[0].Name != live {
		t.Fatalf("sessions = %+v, want only %s", inv.Sessions, live)
	}
	got := inv.Sessions[0]
	if got.State != provider.ObserveRunning {
		t.Errorf("state = %q, want %q", got.State, provider.ObserveRunning)
	}
	if got.Object == nil || *got.Object != *want.Object {
		t.Errorf("object = %+v, want observe's %+v", got.Object, want.Object)
	}
	if got.Identity["GC_SESSION_ID"] != "sid-1" {
		t.Errorf("identity = %v, want the start env's", got.Identity)
	}
}

// TestInventoryIncompleteForUnlistedPane proves inventory does not claim
// completeness for a pane its binding store does not list, such as one started
// by an older script or one whose binding was lost.
func TestInventoryIncompleteForUnlistedPane(t *testing.T) {
	ctx := context.Background()
	fake := faketest.New()
	p := provider.New("acme", fake)
	name := "Indigo--dog-1"
	if err := p.Start(ctx, name, provider.Config{Argv: []string{"sleep", "300"}}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	// The pane's label names a session no binding lists.
	fake.View.Windows[0].Blocks[0].Label = "worker (Ghost--nobody-1)"

	inv, err := p.Inventory(ctx)
	if err != nil {
		t.Fatalf("Inventory: %v", err)
	}
	if inv.Complete {
		t.Errorf("complete = true, want false for a pane no binding lists")
	}
}

// TestKillObjectCorpseLiveWhenAttached proves corpse mode leaves a dead box
// open while a client is attached: gc's `live` answer keeps the operator's
// screen, which gc's edge doc requires and which this mode must re-check.
func TestKillObjectCorpseLiveWhenAttached(t *testing.T) {
	fake, p := started(t)
	name := "Indigo--dog-1"
	ans, err := p.Observe(context.Background(), name, nil)
	if err != nil || ans.Object == nil {
		t.Fatalf("Observe = %+v, %v; want a box with an object", ans, err)
	}
	// The box's program exited; Rex keeps the pane, and an operator has it open.
	fake.Processes = map[rex.BlockID]rex.ProcessInfo{
		rex.BlockID(ans.Object.ID): {LastExit: &rex.ExitStatus{ExitCode: 0}},
	}
	fake.Clients = []rex.ClientInfo{{ID: "c1", Kind: "app", Name: "Rex"}}
	fake.View.Windows[0].Blocks[0].Owner = "c1"
	fake.View.ActiveWindowID = "window:Rig: Indigo"

	res, err := p.KillObject(context.Background(), name, provider.KillObjectRequest{Object: *ans.Object, Mode: "corpse"})
	if err != nil {
		t.Fatalf("KillObject: %v", err)
	}
	if res != "live" {
		t.Errorf("result = %q, want live for an attached dead box", res)
	}
	if len(fake.ClosedBlocks) != 0 {
		t.Errorf("closed blocks = %v, want none while a client is attached", fake.ClosedBlocks)
	}
}

// TestKillObjectDoesNotCloseRecreatedPane proves kill-object never kills by
// name: when the recorded pane vanished and something else took the name's
// labels without Start, the object names no box, so the kill answers gone and
// leaves the new pane alone.
func TestKillObjectDoesNotCloseRecreatedPane(t *testing.T) {
	fake, p := started(t)
	name := "Indigo--dog-1"
	ans, err := p.Observe(context.Background(), name, nil)
	if err != nil || ans.Object == nil {
		t.Fatalf("Observe = %+v, %v; want a box with an object", ans, err)
	}
	// The recorded pane vanishes from Rex; a restore puts a different pane
	// under the same tab and pane labels, without going through Start.
	fake.VanishedBlocks = map[rex.BlockID]bool{rex.BlockID(ans.Object.ID): true}
	fake.View.Windows[0].Blocks = []rex.Block{{ID: "block:dog-1-restored", Label: "dog-1"}}

	res, err := p.KillObject(context.Background(), name, provider.KillObjectRequest{Object: *ans.Object, Mode: "corpse"})
	if err != nil {
		t.Fatalf("KillObject: %v", err)
	}
	if res != "gone" {
		t.Errorf("result = %q, want gone for the vanished object", res)
	}
	if len(fake.ClosedBlocks) != 0 {
		t.Errorf("closed blocks = %v, want none for the vanished object", fake.ClosedBlocks)
	}
	if running, err := p.IsRunning(context.Background(), name); err != nil || !running {
		t.Errorf("IsRunning after the kill = %v, %v; want the recreated pane still running", running, err)
	}
}

// TestObserveAbsentWhenPaneVanished proves what Rex proves when it no longer
// knows the recorded block: the name is absent, not dead, and Observe leaves
// the stale record for Start or Stop to replace.
func TestObserveAbsentWhenPaneVanished(t *testing.T) {
	fake, p := started(t)
	name := "Indigo--dog-1"
	if _, err := p.Observe(context.Background(), name, nil); err != nil {
		t.Fatalf("Observe: %v", err)
	}
	bd, ok := p.Binding(name)
	if !ok {
		t.Fatalf("Binding(%s) missing before the vanish", name)
	}
	fake.VanishedBlocks = map[rex.BlockID]bool{bd.BlockID: true}

	ans, err := p.Observe(context.Background(), name, nil)
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if ans.State != provider.ObserveAbsent || ans.Object != nil {
		t.Errorf("Observe = %+v, want absent with no object", ans)
	}
	if _, ok := p.Binding(name); !ok {
		t.Error("Observe forgot the stale binding; Start and Stop own that cleanup")
	}
}

// TestObserveAnswersForPaneRestoredUnderName proves rule 7's converse: Rex
// proving the recorded block gone does not prove the name is free. A restore
// or an operator can leave another pane under the same labels, and observe
// must answer for that box instead of absent.
func TestObserveAnswersForPaneRestoredUnderName(t *testing.T) {
	fake := faketest.New()
	p := provider.New("acme", fake)
	name := "Indigo--dog-1"
	if err := p.Start(context.Background(), name, provider.Config{
		Argv: []string{"sleep", "300"},
		Env:  map[string]string{"GC_SESSION_ID": "sid-1"},
	}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	first, err := p.Observe(context.Background(), name, nil)
	if err != nil || first.Object == nil {
		t.Fatalf("Observe = %+v, %v; want a box with an object", first, err)
	}
	if first.Identity["GC_SESSION_ID"] != "sid-1" {
		t.Fatalf("identity = %v, want the started incarnation's", first.Identity)
	}
	// A restore puts a different pane under the same tab and pane labels.
	fake.VanishedBlocks = map[rex.BlockID]bool{rex.BlockID(first.Object.ID): true}
	fake.View.Windows[0].Blocks = []rex.Block{{ID: "block:dog-1-restored", Label: "dog-1"}}

	ans, err := p.Observe(context.Background(), name, nil)
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if ans.State != provider.ObserveRunning {
		t.Errorf("state = %q, want running for the restored pane", ans.State)
	}
	if ans.Object == nil || ans.Object.ID != "block:dog-1-restored" {
		t.Errorf("object = %+v, want the restored pane's", ans.Object)
	}
	// Observe is a read: the restored pane answers, but the recorded binding is
	// left for Start or Stop to replace, and the retired box's created and
	// identity are not reported for a pane the provider did not start.
	if bd, ok := p.Binding(name); !ok || bd.BlockID != rex.BlockID(first.Object.ID) {
		t.Errorf("binding after observe = %+v, %v; want the recorded %s kept", bd, ok, first.Object.ID)
	}
	if ans.Object != nil && ans.Object.Created == first.Object.Created {
		t.Errorf("created = %q, want the retired box's %q not reported", ans.Object.Created, first.Object.Created)
	}
	// A box with no gc identity (one an operator made) is `{}` on the wire;
	// an absent or null identity is "not yet readable" to gc (rule 5), which
	// would keep an identity read of this pane pending forever.
	raw, err := json.Marshal(ans)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !bytes.Contains(raw, []byte(`"identity":{}`)) {
		t.Errorf("answer = %s, want \"identity\":{} for a pane the provider did not start", raw)
	}
}

// TestInventoryRestoredPaneLeavesBindingAlone proves inventory, like observe,
// answers for a restored pane without adopting it into the binding store.
func TestInventoryRestoredPaneLeavesBindingAlone(t *testing.T) {
	fake, p := started(t)
	name := "Indigo--dog-1"
	first, err := p.Observe(context.Background(), name, nil)
	if err != nil || first.Object == nil {
		t.Fatalf("Observe = %+v, %v; want a box with an object", first, err)
	}
	fake.VanishedBlocks = map[rex.BlockID]bool{rex.BlockID(first.Object.ID): true}
	fake.View.Windows[0].Blocks = []rex.Block{{ID: "block:dog-1-restored", Label: "dog-1"}}

	inv, err := p.Inventory(context.Background())
	if err != nil {
		t.Fatalf("Inventory: %v", err)
	}
	if len(inv.Sessions) != 1 || inv.Sessions[0].Object == nil || inv.Sessions[0].Object.ID != "block:dog-1-restored" {
		t.Fatalf("sessions = %+v, want the restored pane running", inv.Sessions)
	}
	if bd, ok := p.Binding(name); !ok || bd.BlockID != rex.BlockID(first.Object.ID) {
		t.Errorf("binding after inventory = %+v, %v; want the recorded %s kept", bd, ok, first.Object.ID)
	}
	// Like observe, the restored pane's identity is an empty set, not null: gc
	// reads null as "not yet readable" (rule 5), but this box has no gc
	// identity at all.
	raw, err := json.Marshal(inv.Sessions[0])
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !bytes.Contains(raw, []byte(`"identity":{}`)) {
		t.Errorf("entry = %s, want \"identity\":{} for a pane the provider did not start", raw)
	}
}

// TestKillObjectGoneForRestoredPane pins kill-object's answer for a pane that
// took a vanished record's name: the exact-object re-check only knows the
// recorded box, so the restored pane's object answers `gone` and no pane is
// closed. Killing by name would be the unsafe alternative (rule 7).
func TestKillObjectGoneForRestoredPane(t *testing.T) {
	ctx := context.Background()
	fake := faketest.New()
	p := provider.New("acme", fake)
	name := "Indigo--dog-1"
	if err := p.Start(ctx, name, provider.Config{Argv: []string{"sleep", "300"}}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	first, err := p.Observe(ctx, name, nil)
	if err != nil || first.Object == nil {
		t.Fatalf("Observe = %+v, %v; want a box with an object", first, err)
	}
	fake.VanishedBlocks = map[rex.BlockID]bool{rex.BlockID(first.Object.ID): true}
	fake.View.Windows[0].Blocks = []rex.Block{{ID: "block:dog-1-restored", Label: "dog-1"}}
	restored, err := p.Observe(ctx, name, nil)
	if err != nil || restored.Object == nil || restored.Object.ID != "block:dog-1-restored" {
		t.Fatalf("Observe = %+v, %v; want the restored pane", restored, err)
	}

	res, err := p.KillObject(ctx, name, provider.KillObjectRequest{Object: *restored.Object, Mode: "corpse"})
	if err != nil {
		t.Fatalf("KillObject: %v", err)
	}
	if res != "gone" {
		t.Errorf("result = %q, want gone: read ops do not adopt a restored pane", res)
	}
	if len(fake.ClosedBlocks) != 0 {
		t.Errorf("closed blocks = %v, want none: never close a pane the record does not name", fake.ClosedBlocks)
	}
}

// TestObserveAbsentLeavesIncarnationStateAlone proves Observe is a read: when
// the recorded block vanished it answers absent and leaves the binding and the
// meta sidecar for the next Start or Stop to clear.
func TestObserveAbsentLeavesIncarnationStateAlone(t *testing.T) {
	fake := faketest.New()
	p := provider.New("acme", fake)
	name := "Indigo--dog-1"
	if err := p.Start(context.Background(), name, provider.Config{
		Argv: []string{"sleep", "300"},
		Env:  map[string]string{"GC_SESSION_ID": "sid-1"},
	}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	bd, ok := p.Binding(name)
	if !ok {
		t.Fatalf("Binding(%s) missing before the vanish", name)
	}
	fake.VanishedBlocks = map[rex.BlockID]bool{bd.BlockID: true}
	fake.View.Windows[0].Blocks = nil

	ans, err := p.Observe(context.Background(), name, nil)
	if err != nil || ans.State != provider.ObserveAbsent {
		t.Fatalf("Observe = %+v, %v; want absent", ans, err)
	}
	if got, err := p.GetMeta(context.Background(), name, "GC_SESSION_ID"); err != nil || got != "sid-1" {
		t.Errorf("meta after absent observe = %q, %v; want the retired incarnation's sid-1", got, err)
	}
	if _, ok := p.Binding(name); !ok {
		t.Error("Observe dropped the binding; Start and Stop own that cleanup")
	}
}

// TestKillObjectGoneWhenBlockVanished proves kill-object names no box when the
// recorded block is gone: it answers gone and closes nothing, so gc is not
// told it killed a box it could not see.
func TestKillObjectGoneWhenBlockVanished(t *testing.T) {
	fake, p := started(t)
	name := "Indigo--dog-1"
	ans, err := p.Observe(context.Background(), name, nil)
	if err != nil || ans.Object == nil {
		t.Fatalf("Observe = %+v, %v; want a box with an object", ans, err)
	}
	fake.VanishedBlocks = map[rex.BlockID]bool{rex.BlockID(ans.Object.ID): true}
	fake.View.Windows[0].Blocks = nil

	res, err := p.KillObject(context.Background(), name, provider.KillObjectRequest{Object: *ans.Object, Mode: "corpse"})
	if err != nil {
		t.Fatalf("KillObject: %v", err)
	}
	if res != "gone" {
		t.Errorf("result = %q, want gone for a vanished object", res)
	}
	if len(fake.ClosedBlocks) != 0 {
		t.Errorf("closed blocks = %v, want none for a vanished object", fake.ClosedBlocks)
	}
}

// TestInventorySkipsVanishedBinding proves inventory agrees with observe about
// a name nothing holds: a binding whose block vanished is omitted from a
// complete listing rather than listed dead.
func TestInventorySkipsVanishedBinding(t *testing.T) {
	fake, p := started(t)
	bd, ok := p.Binding("Indigo--dog-1")
	if !ok {
		t.Fatalf("Binding missing before the vanish")
	}
	fake.VanishedBlocks = map[rex.BlockID]bool{bd.BlockID: true}
	fake.View.Windows[0].Blocks = nil

	inv, err := p.Inventory(context.Background())
	if err != nil {
		t.Fatalf("Inventory: %v", err)
	}
	if !inv.Complete {
		t.Errorf("complete = false, want true: Rex proved nothing holds the name")
	}
	if len(inv.Sessions) != 0 {
		t.Errorf("sessions = %+v, want none for the vanished binding", inv.Sessions)
	}
}

// TestInventoryIncompleteWhenLivenessUnavailable proves an unreadable box is
// listed with no state and the answer claims no completeness, because a
// backend fault proves nothing absent.
func TestInventoryIncompleteWhenLivenessUnavailable(t *testing.T) {
	fake, p := started(t)
	fake.ProcessErr = rex.ErrUnavailable

	inv, err := p.Inventory(context.Background())
	if err != nil {
		t.Fatalf("Inventory: %v", err)
	}
	if inv.Complete {
		t.Errorf("complete = true, want false when liveness cannot be read")
	}
	if len(inv.Sessions) != 1 || inv.Sessions[0].State != "" {
		t.Errorf("sessions = %+v, want one entry with no state", inv.Sessions)
	}
}
