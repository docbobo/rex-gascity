package provider_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/docbobo/rex-gascity/internal/prochost"
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

// TestObserveVisiblePaneDuringStart covers the window in RPP-OBSERVE-003
// where Rex has created the pane but Start has yet to record its block ID.
func TestObserveVisiblePaneDuringStart(t *testing.T) {
	ctx := context.Background()
	fake := faketest.New()
	bs := provider.NewMemoryStore()
	p := provider.New("acme", fake, provider.WithBindingStore(bs))
	const name = "Indigo--dog-1"
	cfg := provider.Config{
		Argv: []string{"sleep", "300"},
		Env:  map[string]string{"GC_SESSION_ID": "sid-1"},
	}
	if err := p.Start(ctx, name, cfg); err != nil {
		t.Fatal(err)
	}
	final, ok := p.Binding(name)
	if !ok || final.BlockID == "" {
		t.Fatal("missing final binding")
	}
	provisional := final
	provisional.BlockID = ""
	provisional.WindowID = ""
	bs.Save(provisional)
	// Rex rejects a block.process request without a block ID.
	fake.VanishedBlocks = map[rex.BlockID]bool{"": true}
	ans, err := p.Observe(ctx, name, nil)
	if err != nil || ans.State != provider.ObserveUnknown || ans.Object != nil || ans.Identity != nil {
		t.Fatalf("Observe during Start = %+v, %v; want unknown while the visible pane's object is unconfirmed", ans, err)
	}

	bs.Save(final)
	ans, err = p.Observe(ctx, name, nil)
	if err != nil || ans.State != provider.ObserveRunning || ans.Identity["GC_SESSION_ID"] != "sid-1" {
		t.Fatalf("Observe after Start = %+v, %v; want the running pane with its identity", ans, err)
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
		blockOf(t, p, name): {LastExit: &rex.ExitStatus{ExitCode: 0}},
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
	fake.VanishedBlocks = map[rex.BlockID]bool{blockOf(t, p, name): true}
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

// TestInventoryCompleteIgnoresUnlistedPane proves a pane whose label names a
// session no binding lists does not make the answer incomplete. Another store of
// the same city, or a stop still closing, leaves such a pane, and neither can be
// told apart from here, so it is no evidence about this store. gc reads a false
// "incomplete" as a reason to distrust every name the listing omits.
func TestInventoryCompleteIgnoresUnlistedPane(t *testing.T) {
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
	if !inv.Complete {
		t.Errorf("complete = false, want true: an unlisted pane proves nothing about the store")
	}
	if len(inv.Sessions) != 1 || inv.Sessions[0].Name != name {
		t.Errorf("sessions = %+v, want only %s", inv.Sessions, name)
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
		blockOf(t, p, name): {LastExit: &rex.ExitStatus{ExitCode: 0}},
	}
	fake.Clients = []rex.ClientInfo{{ID: "c1", Kind: "app", Name: "Rex"}}
	fake.View.Windows[0].Blocks[0].Owner = "c1"
	fake.View.ActiveWindowID = "window:[rig] Indigo"

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
	fake.VanishedBlocks = map[rex.BlockID]bool{blockOf(t, p, name): true}
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
	recorded := blockOf(t, p, name)
	fake.VanishedBlocks = map[rex.BlockID]bool{recorded: true}
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
	if bd, ok := p.Binding(name); !ok || bd.BlockID != recorded {
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
	recorded := blockOf(t, p, name)
	fake.VanishedBlocks = map[rex.BlockID]bool{recorded: true}
	fake.View.Windows[0].Blocks = []rex.Block{{ID: "block:dog-1-restored", Label: "dog-1"}}

	inv, err := p.Inventory(context.Background())
	if err != nil {
		t.Fatalf("Inventory: %v", err)
	}
	if len(inv.Sessions) != 1 || inv.Sessions[0].Object == nil || inv.Sessions[0].Object.ID != "block:dog-1-restored" {
		t.Fatalf("sessions = %+v, want the restored pane running", inv.Sessions)
	}
	if bd, ok := p.Binding(name); !ok || bd.BlockID != recorded {
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
	fake.VanishedBlocks = map[rex.BlockID]bool{blockOf(t, p, name): true}
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
	fake.VanishedBlocks = map[rex.BlockID]bool{blockOf(t, p, name): true}
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

// blockOf returns the Rex block the provider recorded for name. Object.ID is
// opaque (the block id plus the incarnation), so fake state keyed by block
// takes the binding instead.
func blockOf(t *testing.T, p *provider.Provider, name string) rex.BlockID {
	t.Helper()
	bd, ok := p.Binding(name)
	if !ok {
		t.Fatalf("Binding(%s) missing", name)
	}
	return bd.BlockID
}

// TestObserveObjectStableAndNewPerBox proves RPP-OBSERVE-005: two observes of
// one box return the same object, and a new box under the same name returns a
// different id. The fake reuses a block id for a relaunched pane under the same
// labels (internal/rex/faketest/fake.go, CloseBlock), so the id must not be the
// block id alone.
func TestObserveObjectStableAndNewPerBox(t *testing.T) {
	ctx := context.Background()
	fake := faketest.New()
	p := provider.New("acme", fake)
	name := "Indigo--dog-1"
	start := func() {
		t.Helper()
		if err := p.Start(ctx, name, provider.Config{Argv: []string{"sleep", "300"}}); err != nil {
			t.Fatalf("Start: %v", err)
		}
	}
	start()

	first, err := p.Observe(ctx, name, nil)
	if err != nil || first.State != provider.ObserveRunning || first.Object == nil {
		t.Fatalf("Observe = %+v, %v; want a running box with an object", first, err)
	}
	second, err := p.Observe(ctx, name, nil)
	if err != nil || second.Object == nil {
		t.Fatalf("Observe again = %+v, %v; want the same box", second, err)
	}
	if second.Object.ID != first.Object.ID || second.Object.Created != first.Object.Created {
		t.Errorf("object = %+v, want %+v byte-identical", second.Object, first.Object)
	}

	if err := p.Stop(ctx, name); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	start()
	again, err := p.Observe(ctx, name, nil)
	if err != nil || again.State != provider.ObserveRunning || again.Object == nil {
		t.Fatalf("Observe after the restart = %+v, %v; want a running box", again, err)
	}
	if again.Object.ID == first.Object.ID {
		t.Errorf("object id = %q after a new start, want a new one (the old box was %+v)", again.Object.ID, first.Object)
	}
}

// observeStalled answers BlockProcess only when the call's own deadline ends,
// as a slow backend does, so a test can measure observe's bound on its calls.
type observeStalled struct {
	*faketest.Fake
	stall bool
}

func (c *observeStalled) BlockProcess(ctx context.Context, sid rex.SessionID, b rex.BlockID) (rex.ProcessInfo, error) {
	if !c.stall {
		return c.Fake.BlockProcess(ctx, sid, b)
	}
	select {
	case <-ctx.Done():
		return rex.ProcessInfo{}, ctx.Err()
	case <-time.After(5 * time.Second):
		return rex.ProcessInfo{}, errors.New("observe: the Rex call carried no deadline")
	}
}

// TestObserveAnswersUnknownWithinBound proves RPP-OBSERVE-013 (rule 11): the
// answer lands inside gc's 4 s probe bound, and a backend that outlives the
// op's own deadline (GC_EXEC_DEADLINE_MS, set by gc on every op) reads unknown
// rather than an error or an overrun.
func TestObserveAnswersUnknownWithinBound(t *testing.T) {
	t.Setenv("GC_EXEC_DEADLINE_MS", "600")
	ctx := context.Background()
	client := &observeStalled{Fake: faketest.New()}
	p := provider.New("acme", client)
	name := "Indigo--dog-1"
	if err := p.Start(ctx, name, provider.Config{Argv: []string{"sleep", "300"}}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	client.stall = true

	begin := time.Now()
	ans, err := p.Observe(ctx, name, nil)
	elapsed := time.Since(begin)
	if err != nil {
		t.Fatalf("Observe of a stalled backend: %v; want unknown, not an error", err)
	}
	if ans.State != provider.ObserveUnknown {
		t.Errorf("state = %q, want %q", ans.State, provider.ObserveUnknown)
	}
	if elapsed >= 4*time.Second {
		t.Errorf("Observe took %v, want under the 4 s probe bound", elapsed)
	}
}

// TestObserveEmptyWideIsUnknown drives Observe through the real PSHost over a
// stub runner: the identity read answers a matching subtree, and the wide read
// is empty. The agent stays omitted, never dead, and the adapter keeps its two
// reads with -ww on the args read only.
func TestObserveEmptyWideIsUnknown(t *testing.T) {
	ctx := context.Background()
	fake := faketest.New()
	runner := &provider.FakeRunner{}
	runner.Handle = func(_ context.Context, c provider.RunCall) (string, error) {
		if strings.Contains(strings.Join(c.Args, " "), "args=") {
			return "", nil // the wide read is empty
		}
		return "  100     1 S    Fri Nov 20 19:44:50 2026 sleep\n  101   100 S    Fri Nov 20 19:44:50 2026 sh\n", nil
	}
	p := provider.New("acme", fake, provider.WithProcessHost(prochost.NewPSHost(runner)))
	name := "Indigo--dog-1"
	if err := p.Start(ctx, name, provider.Config{Argv: []string{"sleep", "300"}}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	fake.Processes = map[rex.BlockID]rex.ProcessInfo{
		"block:dog-1": {Foreground: &rex.Process{PID: 100, Name: "sleep"}},
	}

	before := len(runner.Calls())
	ans, err := p.Observe(ctx, name, []string{"sleep"})
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if ans.State != provider.ObserveRunning {
		t.Fatalf("state = %q, want %q", ans.State, provider.ObserveRunning)
	}
	if ans.Agent != "" || ans.AgentPID != "" {
		t.Errorf("agent = %q pid = %q, want an omitted agent, never dead, for an empty wide table", ans.Agent, ans.AgentPID)
	}
	calls := runner.Calls()[before:]
	if len(calls) != 2 {
		t.Fatalf("snapshot reads = %d, want 2 (identity then wide)", len(calls))
	}
	for i, c := range calls {
		if got, want := strings.Contains(strings.Join(c.Args, " "), "-ww"), i == 1; got != want {
			t.Errorf("read %d -ww = %v, want %v: %v", i, got, want, c.Args)
		}
	}
}

// TestObserveAgentFromProcessSnapshot proves RPP-OBSERVE-006's wire contract: a
// running box reports its agent when one of gc's names is in the box's process
// subtree, and omits it, never calling it dead, when the snapshot cannot be
// read or holds no match.
func TestObserveAgentFromProcessSnapshot(t *testing.T) {
	ctx := context.Background()
	fake := faketest.New()
	host := &prochost.Fake{}
	host.OnTable = func(call int, _ bool) (prochost.Table, error) {
		switch call {
		case 1:
			return prochost.Table{
				100: {PID: 100, PPID: 1, Comm: "sleep", Args: "sleep 300", Reliable: true},
				101: {PID: 101, PPID: 100, Comm: "sh", Args: "sh -c run", Reliable: true},
			}, nil
		case 2:
			return prochost.Table{
				100: {PID: 100, PPID: 1, Comm: "sleep", Args: "sleep 300", Reliable: true},
				101: {PID: 101, PPID: 100, Comm: "sh", Args: "sh -c run", Reliable: true},
			}, nil
		default:
			return nil, prochost.ErrUnreadableSnapshot
		}
	}
	p := provider.New("acme", fake, provider.WithProcessHost(host))
	name := "Indigo--dog-1"
	if err := p.Start(ctx, name, provider.Config{Argv: []string{"sleep", "300"}}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	fake.Processes = map[rex.BlockID]rex.ProcessInfo{
		"block:dog-1": {Foreground: &rex.Process{PID: 100, Name: "sleep"}},
	}

	ans, err := p.Observe(ctx, name, []string{"sleep"})
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if ans.Agent != "alive" || ans.AgentPID != "100" {
		t.Errorf("agent = %q pid = %q, want alive and 100", ans.Agent, ans.AgentPID)
	}

	none, err := p.Observe(ctx, name, []string{"gc-rpp-no-such-process"})
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if none.Agent != "dead" || none.AgentPID != "" {
		t.Errorf("agent = %q pid = %q, want dead and no pid", none.Agent, none.AgentPID)
	}

	unreadable, err := p.Observe(ctx, name, []string{"sleep"})
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if unreadable.Agent != "" || unreadable.AgentPID != "" {
		t.Errorf("agent = %q pid = %q, want an omitted agent for an unreadable snapshot", unreadable.Agent, unreadable.AgentPID)
	}
}

// TestObserveIdentityIsGetMetaIdentity proves RPP-OBSERVE-008: observe reports
// the one identity get-meta serves, including the process names observe.v1's
// liveness reads, and a set-meta of an identity key lands in the next observe.
func TestObserveIdentityIsGetMetaIdentity(t *testing.T) {
	t.Setenv("GCREX_OBSERVE_V1", "1")
	ctx := context.Background()
	p := provider.New("acme", faketest.New())
	const name = "Indigo--dog-1"
	env := map[string]string{
		"GC_SESSION_ID":     "sid-1",
		"GC_RUNTIME_EPOCH":  "7",
		"GC_INSTANCE_TOKEN": "tok-1",
		"GT_PROCESS_NAMES":  "sleep",
	}
	if err := p.Start(ctx, name, provider.Config{Argv: []string{"sleep", "300"}, Env: env}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	ans, err := p.Observe(ctx, name, nil)
	if err != nil || ans.State != provider.ObserveRunning {
		t.Fatalf("Observe = %+v, %v; want a running box", ans, err)
	}
	for _, key := range []string{"GC_SESSION_ID", "GC_RUNTIME_EPOCH", "GC_INSTANCE_TOKEN", "GT_PROCESS_NAMES"} {
		got, err := p.GetMeta(ctx, name, key)
		if err != nil {
			t.Fatalf("GetMeta(%s): %v", key, err)
		}
		if got != ans.Identity[key] {
			t.Errorf("GetMeta(%s) = %q, observe identity says %q", key, got, ans.Identity[key])
		}
	}

	if err := p.SetMeta(ctx, name, "GC_RUNTIME_EPOCH", "8"); err != nil {
		t.Fatalf("SetMeta: %v", err)
	}
	after, err := p.Observe(ctx, name, nil)
	if err != nil || after.State != provider.ObserveRunning {
		t.Fatalf("Observe after set-meta = %+v, %v; want a running box", after, err)
	}
	if got := after.Identity["GC_RUNTIME_EPOCH"]; got != "8" {
		t.Errorf("observe GC_RUNTIME_EPOCH after set-meta = %q, want %q", got, "8")
	}
}

// TestObserveAbsentAfterStop proves RPP-OBSERVE-004: stop retires the pane and
// its seeded identity, so observe answers absent and is-running false.
func TestObserveAbsentAfterStop(t *testing.T) {
	ctx := context.Background()
	p := provider.New("acme", faketest.New())
	const name = "Indigo--dog-1"
	env := map[string]string{"GC_SESSION_ID": "sid-1", "GC_INSTANCE_TOKEN": "tok-1"}
	if err := p.Start(ctx, name, provider.Config{Argv: []string{"sleep", "300"}, Env: env}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := p.Stop(ctx, name); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	ans, err := p.Observe(ctx, name, nil)
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if ans.State != provider.ObserveAbsent || ans.Object != nil {
		t.Errorf("Observe after stop = %+v, want absent with no object", ans)
	}
	if running, err := p.IsRunning(ctx, name); err != nil || running {
		t.Errorf("IsRunning after stop = %v, %v; want false, nil", running, err)
	}
}

// TestInventoryLastActivityFromStore proves RPP-OBSERVE-015's wire contract:
// every running entry carries a last_activity, the activity store's stamp for
// a polled box and the start-time floor for one gc has not polled yet, without
// the listing polling any box itself.
func TestInventoryLastActivityFromStore(t *testing.T) {
	ctx := context.Background()
	fake := faketest.New()
	p := provider.New("acme", fake)
	polled, fresh := "Indigo--dog-1", "Indigo--fox-2"
	for _, n := range []string{polled, fresh} {
		if err := p.Start(ctx, n, provider.Config{Argv: []string{"sleep", "300"}}); err != nil {
			t.Fatalf("Start %s: %v", n, err)
		}
	}
	freshBinding, ok := p.Binding(fresh)
	if !ok {
		t.Fatalf("no binding for %s", fresh)
	}
	activity, err := p.GetLastActivity(ctx, polled)
	if err != nil {
		t.Fatalf("GetLastActivity: %v", err)
	}
	fake.ListSessionsCalls, fake.SessionViewCalls = 0, 0
	fake.BlockCalls = nil

	inv, err := p.Inventory(ctx)
	if err != nil {
		t.Fatalf("Inventory: %v", err)
	}
	if !inv.Complete {
		t.Errorf("inventory = %+v, want complete", inv)
	}
	seen := map[string]bool{}
	for _, e := range inv.Sessions {
		if seen[e.Name] {
			t.Errorf("%s listed twice, want one entry per binding", e.Name)
		}
		seen[e.Name] = true
		at, err := time.Parse(time.RFC3339, e.LastActivity)
		if e.LastActivity == "" || err != nil || at.Unix() <= 0 {
			t.Errorf("%s last_activity = %q, want an RFC3339 time", e.Name, e.LastActivity)
			continue
		}
		switch e.Name {
		case polled:
			if !at.Equal(activity.At) {
				t.Errorf("last_activity = %v, want the activity store's %v", at, activity.At)
			}
		case fresh:
			if !at.Equal(freshBinding.CreatedAt) {
				t.Errorf("last_activity = %v, want the start time %v", at, freshBinding.CreatedAt)
			}
		}
	}
	if !seen[polled] || !seen[fresh] {
		t.Errorf("inventory = %+v, want both running sessions", inv)
	}
	if fake.ListSessionsCalls != 0 || fake.SessionViewCalls != 0 {
		t.Errorf("listing read sessions: list=%d view=%d, want none", fake.ListSessionsCalls, fake.SessionViewCalls)
	}
	for _, c := range fake.BlockCalls {
		// The liveness read and the program_status read are the two calls the
		// inventory budget allows; a screen capture (format) is a poll.
		if c.Op != "process" && c.Op != "program_status" {
			t.Errorf("inventory called %q per box, want only the liveness and status reads", c.Op)
		}
	}
}

// TestInventoryFailsWhenLivenessIsUnreadable proves a listing whose liveness
// read faults fails the op, so gc sees a non-zero exit instead of an answer
// that calls a running name absent.
func TestInventoryFailsWhenLivenessIsUnreadable(t *testing.T) {
	fake, p := started(t)
	fake.ProcessErr = errors.New("rex: protocol error")
	if inv, err := p.Inventory(context.Background()); err == nil {
		t.Fatalf("Inventory = %+v, want an error so the op exits non-zero", inv)
	}
}

// TestInventoryOmitsAProvisionalRecord proves a record written before its pane
// exists (a start in flight) is not read as a box: Rex rejects an empty block
// id, and the name is not running yet.
func TestInventoryOmitsAProvisionalRecord(t *testing.T) {
	fake := faketest.New()
	bs := provider.NewMemoryStore()
	p := provider.New("acme", fake, provider.WithBindingStore(bs))
	bs.Save(provider.Binding{Name: "Indigo--fox-2", SessionID: "session:pending", Attempt: "pending"})

	inv, err := p.Inventory(context.Background())
	if err != nil {
		t.Fatalf("Inventory: %v", err)
	}
	if !inv.Complete || len(inv.Sessions) != 0 {
		t.Errorf("inventory = %+v, want complete with no sessions", inv)
	}
	for _, c := range fake.BlockCalls {
		t.Errorf("inventory read %q for a record with no block yet", c.Op)
	}
}

// sequencedProcess answers the first BlockProcess call with first and every
// later call with later. It is a faketest ProcessFunc, called with the fake's
// lock held, so it touches only its own counter.
func sequencedProcess(first, later rex.ProcessInfo) func(rex.BlockID) rex.ProcessInfo {
	calls := 0
	return func(rex.BlockID) rex.ProcessInfo {
		calls++
		if calls == 1 {
			return first
		}
		return later
	}
}

// processReads counts the BlockProcess calls the fake recorded.
func processReads(fake *faketest.Fake) int {
	n := 0
	for _, c := range fake.BlockCalls {
		if c.Op == "process" {
			n++
		}
	}
	return n
}

// TestObserveAgentRootFromFreshRead guards RPP-OBSERVE-006's read order: the
// agent root comes from a fresh foreground read after boxState, and the one
// host snapshot is walked from that root. The first read names root 100
// (sleep); every later read names root 200, the agent. Taking the root from
// boxState's reading would make one read and answer dead.
func TestObserveAgentRootFromFreshRead(t *testing.T) {
	ctx := context.Background()
	fake := faketest.New()
	host := &prochost.Fake{}
	snapshots := 0
	host.OnTable = func(int, bool) (prochost.Table, error) {
		snapshots++
		return prochost.Table{
			100: {PID: 100, PPID: 1, Comm: "sleep", Args: "sleep 300", Reliable: true},
			200: {PID: 200, PPID: 1, Comm: "gc-rpp-no-such-process", Args: "gc-rpp-no-such-process", Reliable: true},
		}, nil
	}
	p := provider.New("acme", fake, provider.WithProcessHost(host))
	name := "Indigo--dog-1"
	if err := p.Start(ctx, name, provider.Config{Argv: []string{"sleep", "300"}}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	fake.ProcessFunc = sequencedProcess(
		rex.ProcessInfo{Foreground: &rex.Process{PID: 100, Name: "sleep"}},
		rex.ProcessInfo{Foreground: &rex.Process{PID: 200}},
	)

	before := processReads(fake)
	ans, err := p.Observe(ctx, name, []string{"gc-rpp-no-such-process"})
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if ans.Agent != "alive" || ans.AgentPID != "200" {
		t.Errorf("agent = %q pid = %q, want alive and 200 from the fresh read", ans.Agent, ans.AgentPID)
	}
	if got := processReads(fake) - before; got != 2 {
		t.Errorf("Observe made %d process reads, want 2 (boxState, then the fresh foreground read)", got)
	}
	if snapshots != 1 {
		t.Errorf("Observe took %d host snapshots, want 1", snapshots)
	}
}
