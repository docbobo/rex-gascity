package provider

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/docbobo/rex-gascity/internal/rex"
	"github.com/docbobo/rex-gascity/internal/rex/faketest"
)

// clearCityPin gives the provider no city pin, so its town tab is
// townTabFallback whatever the test process inherited: a gc agent's own
// environment sets GC_REX_CITY (cityPin, statedir.go).
func clearCityPin(t *testing.T) {
	t.Helper()
	for _, k := range []string{"GC_EXEC_CITY_NAME", "GC_EXEC_CITY_PATH", "GC_REX_CITY"} {
		t.Setenv(k, "")
	}
}

// townStartCfg is a town-level start: no GC_RIG.
func townStartCfg() Config { return Config{Argv: []string{"pi"}} }

// rigStartCfg is a start in rig's tab.
func rigStartCfg(rig string) Config {
	return Config{Argv: []string{"pi"}, Env: map[string]string{"GC_RIG": rig}}
}

// tabOrder is the fake's window labels in session order. Call it only while
// no fake method is in flight.
func tabOrder(f *faketest.Fake) []string {
	labels := make([]string, 0, len(f.View.Windows))
	for _, w := range f.View.Windows {
		labels = append(labels, w.Label)
	}
	return labels
}

func mustStart(t *testing.T, p *Provider, name string, cfg Config) {
	t.Helper()
	if err := p.Start(context.Background(), name, cfg); err != nil {
		t.Fatalf("Start %s: %v", name, err)
	}
}

// TestStartPlacesNewTownTabFirst is #47 AC-1: a town tab created in the
// canonical session behind an existing rig tab moves in front of it with
// exactly one MoveWindow call. The rig tab is seeded in the canonical session
// (its view Label is the canonical label, as the requirements' Testing
// expectations ask), so the town start is the only start in the test.
// TestE2ETownTabFirst and TestStartLongPinTownTabMovesFirst start the rig
// through the provider.
func TestStartPlacesNewTownTabFirst(t *testing.T) {
	clearCityPin(t)
	fake := faketest.New()
	fake.Sessions = []rex.Session{{ID: "session:gc:acme", Label: "gc:acme"}}
	fake.View.Label = "gc:acme"
	fake.View.Windows = []rex.Window{{
		ID:     "window:Rig: rig-a",
		Label:  "Rig: rig-a",
		Blocks: []rex.Block{{ID: "block:polecat", Label: "polecat"}},
	}}
	p := New("acme", fake)

	mustStart(t, p, "mayor", townStartCfg())

	if len(fake.CreatedSessions) != 0 {
		t.Fatalf("created sessions = %v, want the seeded canonical session reused", fake.CreatedSessions)
	}
	if got, want := tabOrder(fake), []string{townTabFallback, "Rig: rig-a"}; !slices.Equal(got, want) {
		t.Errorf("tab order = %q, want %q", got, want)
	}
	want := []faketest.MoveWindowCall{{
		SessionID: "session:gc:acme",
		WindowID:  rex.WindowID("window:" + townTabFallback),
		Before:    "window:Rig: rig-a",
	}}
	if !slices.Equal(fake.MoveWindowCalls, want) {
		t.Errorf("MoveWindowCalls = %+v, want %+v", fake.MoveWindowCalls, want)
	}
}

// TestStartTownSplitKeepsOperatorOrder is #47 AC-4 and US-2: a second
// town-level agent splits into the existing town tab and leaves the order the
// operator chose alone. Its setup needs the town tab moved first, so it is red
// until slice 1's move exists.
func TestStartTownSplitKeepsOperatorOrder(t *testing.T) {
	clearCityPin(t)
	fake := faketest.New()
	p := New("acme", fake)
	mustStart(t, p, "rig-a--polecat", rigStartCfg("rig-a"))
	mustStart(t, p, "mayor", townStartCfg())
	if got, want := tabOrder(fake), []string{townTabFallback, "Rig: rig-a"}; !slices.Equal(got, want) {
		t.Fatalf("setup: tab order after mayor = %q, want %q", got, want)
	}
	// The operator drags the town tab back behind the rig tab.
	slices.Reverse(fake.View.Windows)
	fake.MoveWindowCalls = nil

	mustStart(t, p, "deacon", townStartCfg())

	if len(fake.CreatedSplits) != 1 {
		t.Fatalf("splits = %d, want deacon split into the town tab", len(fake.CreatedSplits))
	}
	if len(fake.MoveWindowCalls) != 0 {
		t.Errorf("MoveWindowCalls = %+v, want none for a split", fake.MoveWindowCalls)
	}
	if got, want := tabOrder(fake), []string{"Rig: rig-a", townTabFallback}; !slices.Equal(got, want) {
		t.Errorf("tab order = %q, want the operator's %q", got, want)
	}
}

// TestStartTownTabAlreadyFirstMakesNoMoveCall is #47 AC-3: a mayor start into
// an empty canonical session makes no MoveWindow call at all. MoveWindowCalls
// records failed calls too, so a redundant before == window call fails here.
func TestStartTownTabAlreadyFirstMakesNoMoveCall(t *testing.T) {
	clearCityPin(t)
	fake := faketest.New()
	p := New("acme", fake)

	mustStart(t, p, "mayor", townStartCfg())

	if len(fake.MoveWindowCalls) != 0 {
		t.Errorf("MoveWindowCalls = %+v, want none", fake.MoveWindowCalls)
	}
	if got, want := tabOrder(fake), []string{townTabFallback}; !slices.Equal(got, want) {
		t.Errorf("tab order = %q, want %q", got, want)
	}
}

// TestStartLongPinTownTabMovesFirst is #47 AC-4: a town pin longer than
// maxRexLabel runes makes a clamped town tab label. The gate classifies the
// start, not the label, so the clamped tab still moves first. The canonical
// session label stays short ("gc:acme"), so only the window label is clamped.
func TestStartLongPinTownTabMovesFirst(t *testing.T) {
	clearCityPin(t)
	t.Setenv("GC_REX_CITY", strings.Repeat("T", 200))
	fake := faketest.New()
	p := New("acme", fake)
	if p.binder.canonicalLabel() != "gc:acme" {
		t.Fatalf("canonical label = %q, want gc:acme", p.binder.canonicalLabel())
	}
	mustStart(t, p, "rig-a--polecat", rigStartCfg("rig-a"))

	mustStart(t, p, "mayor", townStartCfg())

	clamped := clampRexLabel(strings.Repeat("T", 200))
	if clamped == p.townTab {
		t.Fatal("test setup: the town label is not clamped")
	}
	if len(fake.MoveWindowCalls) != 1 {
		t.Errorf("MoveWindowCalls = %+v, want exactly one", fake.MoveWindowCalls)
	}
	if got, want := tabOrder(fake), []string{clamped, "Rig: rig-a"}; !slices.Equal(got, want) {
		t.Errorf("tab order = %q, want %q", got, want)
	}
}

// TestStartRigTabAfterTownMakesNoMoveCall is #47 AC-4: a rig tab created after
// the town tab is not moved.
func TestStartRigTabAfterTownMakesNoMoveCall(t *testing.T) {
	clearCityPin(t)
	fake := faketest.New()
	p := New("acme", fake)

	mustStart(t, p, "mayor", townStartCfg())
	mustStart(t, p, "rig-a--polecat", rigStartCfg("rig-a"))

	if len(fake.MoveWindowCalls) != 0 {
		t.Errorf("MoveWindowCalls = %+v, want none", fake.MoveWindowCalls)
	}
	if got, want := tabOrder(fake), []string{townTabFallback, "Rig: rig-a"}; !slices.Equal(got, want) {
		t.Errorf("tab order = %q, want %q", got, want)
	}
}

// TestStartTownStyleNameWithRigIsNotMoved is #47 AC-4: GC_RIG wins over the
// name shape, so a town-style name started with GC_RIG gets a rig tab that is
// not moved.
func TestStartTownStyleNameWithRigIsNotMoved(t *testing.T) {
	clearCityPin(t)
	fake := faketest.New()
	p := New("acme", fake)
	mustStart(t, p, "rig-a--polecat", rigStartCfg("rig-a"))

	mustStart(t, p, "dog-1", rigStartCfg("rig-b"))

	if len(fake.MoveWindowCalls) != 0 {
		t.Errorf("MoveWindowCalls = %+v, want none", fake.MoveWindowCalls)
	}
	if got, want := tabOrder(fake), []string{"Rig: rig-a", "Rig: rig-b"}; !slices.Equal(got, want) {
		t.Errorf("tab order = %q, want %q", got, want)
	}
}

// TestStartRigLabelEqualToTownPinIsNotMoved is #47 AC-4, the regression for
// a label-equality gate: with the pin "Rig: blue", a rig start with
// GC_RIG=blue creates a window whose label equals the town label. It is a rig
// tab and is not moved.
func TestStartRigLabelEqualToTownPinIsNotMoved(t *testing.T) {
	clearCityPin(t)
	t.Setenv("GC_REX_CITY", "Rig: blue")
	fake := faketest.New()
	p := New("acme", fake)
	if p.townTab != "Rig: blue" {
		t.Fatalf("townTab = %q, want the pin", p.townTab)
	}
	mustStart(t, p, "rig-a--polecat", rigStartCfg("rig-a"))

	mustStart(t, p, "blue--polecat", rigStartCfg("blue"))

	if len(fake.MoveWindowCalls) != 0 {
		t.Errorf("MoveWindowCalls = %+v, want none", fake.MoveWindowCalls)
	}
	if got, want := tabOrder(fake), []string{"Rig: rig-a", "Rig: blue"}; !slices.Equal(got, want) {
		t.Errorf("tab order = %q, want %q", got, want)
	}
}

// TestStartRelaunchRecreatingTownTabMovesItFirst is #47 AC-4 and REQ-008: a
// dead town pane whose relaunch recreates the town tab behind a rig tab gets
// the same one-shot move as a first creation.
func TestStartRelaunchRecreatingTownTabMovesItFirst(t *testing.T) {
	clearCityPin(t)
	fake := faketest.New()
	p := New("acme", fake)
	mustStart(t, p, "mayor", townStartCfg())
	mustStart(t, p, "rig-a--polecat", rigStartCfg("rig-a"))
	old, ok := p.Binding("mayor")
	if !ok {
		t.Fatal("no mayor binding")
	}
	fake.Processes = map[rex.BlockID]rex.ProcessInfo{old.BlockID: {LastExit: &rex.ExitStatus{ExitCode: 0}}}

	mustStart(t, p, "mayor", townStartCfg())

	if !slices.Contains(fake.ClosedBlocks, old.BlockID) || len(fake.CreatedWindows) != 3 {
		t.Fatalf("closed %v, windows created %d: want the dead town tab closed and recreated", fake.ClosedBlocks, len(fake.CreatedWindows))
	}
	if len(fake.MoveWindowCalls) != 1 {
		t.Errorf("MoveWindowCalls = %+v, want exactly one", fake.MoveWindowCalls)
	}
	if got, want := tabOrder(fake), []string{townTabFallback, "Rig: rig-a"}; !slices.Equal(got, want) {
		t.Errorf("tab order = %q, want %q", got, want)
	}
}

// TestStartTownTabMoveFailureOnlyWarns is #47 AC-5: a failed move leaves the
// start successful and emits one warning naming the session.
func TestStartTownTabMoveFailureOnlyWarns(t *testing.T) {
	clearCityPin(t)
	fake := faketest.New()
	var warnings bytes.Buffer
	p := New("acme", fake, WithWarnings(&warnings))
	mustStart(t, p, "rig-a--polecat", rigStartCfg("rig-a"))
	fake.MoveWindowErr = rex.ErrNotFound

	mustStart(t, p, "mayor", townStartCfg())

	if n := strings.Count(warnings.String(), "start mayor: place town tab first:"); n != 1 {
		t.Errorf("warnings = %q, want one place-town-tab-first warning for mayor", warnings.String())
	}
	if len(fake.MoveWindowCalls) != 1 {
		t.Errorf("MoveWindowCalls = %+v, want the one failed call", fake.MoveWindowCalls)
	}
	if got, want := tabOrder(fake), []string{"Rig: rig-a", townTabFallback}; !slices.Equal(got, want) {
		t.Errorf("tab order = %q, want %q (the failed move changes nothing)", got, want)
	}
	if _, ok := p.Binding("mayor"); !ok {
		t.Error("mayor has no binding; a failed move must not undo the start")
	}
}

// failPostCreateView fails exactly one SessionView: the first one after an
// armed NewWindow. launchOnce's pre-create read then succeeds and only
// placeTownTabFirst's post-create read fails. faketest.SessionViewErr would
// fail the pre-create read too and fail the start (pattern:
// viewErrAfterClose, teardown_test.go).
type failPostCreateView struct {
	*faketest.Fake
	err      error
	arm      bool // set by the test: the next NewWindow arms one failure
	failNext bool
	failed   int
}

func (c *failPostCreateView) NewWindow(ctx context.Context, sid rex.SessionID, label string, spec rex.LaunchSpec) (rex.WindowID, rex.BlockID, error) {
	wid, block, err := c.Fake.NewWindow(ctx, sid, label, spec)
	if c.arm {
		c.arm, c.failNext = false, true
	}
	return wid, block, err
}

func (c *failPostCreateView) SessionView(ctx context.Context, sid rex.SessionID) (rex.SessionView, error) {
	if c.failNext {
		c.failNext = false
		c.failed++
		return rex.SessionView{}, c.err
	}
	return c.Fake.SessionView(ctx, sid)
}

// TestStartTownTabViewFailureOnlyWarns is #47 AC-5: when only the post-create
// SessionView fails, the start succeeds, one warning is emitted, and no move
// is attempted.
func TestStartTownTabViewFailureOnlyWarns(t *testing.T) {
	clearCityPin(t)
	c := &failPostCreateView{Fake: faketest.New(), err: errors.New("view down")}
	var warnings bytes.Buffer
	p := New("acme", c, WithWarnings(&warnings))
	mustStart(t, p, "rig-a--polecat", rigStartCfg("rig-a"))
	c.arm = true

	mustStart(t, p, "mayor", townStartCfg())

	if c.failed != 1 {
		t.Fatalf("post-create view failures = %d, want 1", c.failed)
	}
	if n := strings.Count(warnings.String(), "start mayor: place town tab first: view down"); n != 1 {
		t.Errorf("warnings = %q, want one place-town-tab-first warning", warnings.String())
	}
	if len(c.MoveWindowCalls) != 0 {
		t.Errorf("MoveWindowCalls = %+v, want none after a failed view read", c.MoveWindowCalls)
	}
	if got, want := tabOrder(c.Fake), []string{"Rig: rig-a", townTabFallback}; !slices.Equal(got, want) {
		t.Errorf("tab order = %q, want %q", got, want)
	}
}

// TestStartTownTabInLegacySessionIsNotMoved is #47 AC-4: with no canonical
// session, FindSession falls back to an accepted legacy session. The town
// window is created there as before and stays where Rex put it.
func TestStartTownTabInLegacySessionIsNotMoved(t *testing.T) {
	clearCityPin(t)
	fake := faketest.New()
	fake.Sessions = []rex.Session{{ID: "session:gc:old", Label: "gc:old"}}
	fake.View.Windows = []rex.Window{{
		ID:     "window:Rig: rig-a",
		Label:  "Rig: rig-a",
		Blocks: []rex.Block{{ID: "block:polecat", Label: "polecat"}},
	}}
	p := New("acme", fake, WithNamespace(Namespace{labels: []string{"gc:acme", "gc:old"}}))

	mustStart(t, p, "mayor", townStartCfg())

	if bd, ok := p.Binding("mayor"); !ok || bd.SessionID != "session:gc:old" {
		t.Fatalf("mayor binding = %+v, %v; want it in the legacy session", bd, ok)
	}
	if len(fake.CreatedSessions) != 0 {
		t.Errorf("created sessions = %v, want none (session selection is unchanged)", fake.CreatedSessions)
	}
	if len(fake.MoveWindowCalls) != 0 {
		t.Errorf("MoveWindowCalls = %+v, want none in a legacy session", fake.MoveWindowCalls)
	}
	if got, want := tabOrder(fake), []string{"Rig: rig-a", townTabFallback}; !slices.Equal(got, want) {
		t.Errorf("tab order = %q, want %q", got, want)
	}
}
