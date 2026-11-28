package provider

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/docbobo/rex-gascity/internal/rex"
	"github.com/docbobo/rex-gascity/internal/rex/faketest"
)

// fakePID is above Linux pid_max so no real host process matches it; a
// low pid like 42 can be a kernel thread and make kill return EPERM.
const fakePID = 1 << 22

func TestPaneLabelFor(t *testing.T) {
	tests := []struct {
		name string
		sess string
		env  map[string]string
		want string
	}{
		{
			name: "alias wins over agent and beads actor",
			sess: "gc__implementation-worker-mc-wisp-fvwrce",
			env: map[string]string{
				"GC_RIG":          "rex-gascity",
				"GC_ALIAS":        "gc.implementation-worker",
				"GC_AGENT":        "mc-wisp-fvwrce",
				"BEADS_ACTOR":     "rex-gascity/gc.implementation-worker",
				"GC_SESSION_NAME": "gc__implementation-worker-mc-wisp-fvwrce",
			},
			want: "[gc.implementation-worker] (gc__implementation-worker-mc-wisp-fvwrce)",
		},
		{
			name: "empty alias falls to agent",
			sess: "gc__requirements-planner-mc-wisp-mf93ir",
			env: map[string]string{
				"GC_RIG":          "rex-gascity",
				"GC_ALIAS":        "  ",
				"GC_AGENT":        "mc-wisp-mf93ir",
				"BEADS_ACTOR":     "rex-gascity/gc.requirements-planner",
				"GC_SESSION_NAME": "gc__requirements-planner-mc-wisp-mf93ir",
			},
			want: "[mc-wisp-mf93ir] (gc__requirements-planner-mc-wisp-mf93ir)",
		},
		{
			name: "beads actor suffix fallback",
			sess: "gc__task-decomposer-mc-wisp-aaaa",
			env: map[string]string{
				"GC_RIG":          "rex-gascity",
				"BEADS_ACTOR":     "rex-gascity/gc.task-decomposer",
				"GC_SESSION_NAME": "gc__task-decomposer-mc-wisp-aaaa",
			},
			want: "[gc.task-decomposer] (gc__task-decomposer-mc-wisp-aaaa)",
		},
		{
			name: "beads actor without slash used whole",
			sess: "gc__mayor-mc-wisp-bbbb",
			env: map[string]string{
				"GC_RIG":          "rex-gascity",
				"BEADS_ACTOR":     "mayor",
				"GC_SESSION_NAME": "gc__mayor-mc-wisp-bbbb",
			},
			want: "[mayor] (gc__mayor-mc-wisp-bbbb)",
		},
		{
			// No identity in the env: nothing is guessed, the label falls back
			// to the name-derived pane label.
			name: "no agent identity keeps name-derived label",
			sess: "gc__implementation-worker-mc-wisp-fvwrce",
			env: map[string]string{
				"GC_RIG":          "rex-gascity",
				"GC_SESSION_NAME": "gc__implementation-worker-mc-wisp-fvwrce",
			},
			want: "gc__implementation-worker-mc-wisp-fvwrce",
		},
		{
			name: "no agent identity on rig name keeps agent part",
			sess: "rex-gascity--dog-1",
			env: map[string]string{
				"GC_RIG":          "rex-gascity",
				"GC_SESSION_NAME": "rex-gascity--dog-1",
			},
			want: "dog-1",
		},
		{
			name: "empty session name keeps name-derived label",
			sess: "gc__implementation-worker-mc-wisp-fvwrce",
			env: map[string]string{
				"GC_RIG":      "rex-gascity",
				"GC_AGENT":    "mc-wisp-fvwrce",
				"GC_ALIAS":    "gc.implementation-worker",
				"BEADS_ACTOR": "rex-gascity/gc.implementation-worker",
			},
			want: "gc__implementation-worker-mc-wisp-fvwrce",
		},
		{
			name: "town-level unchanged without parentheses",
			sess: "mayor",
			env: map[string]string{
				"GC_ALIAS":        "mayor",
				"GC_AGENT":        "mayor",
				"GC_SESSION_NAME": "mayor",
			},
			want: "[mayor] (mayor)",
		},
		{
			name: "rigless pool name with rig set gets new label",
			sess: "gc__implementation-worker-mc-wisp-fvwrce",
			env: map[string]string{
				"GC_RIG":          "rex-gascity",
				"GC_AGENT":        "mc-wisp-fvwrce",
				"GC_SESSION_NAME": "gc__implementation-worker-mc-wisp-fvwrce",
			},
			want: "[mc-wisp-fvwrce] (gc__implementation-worker-mc-wisp-fvwrce)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := paneLabelFor(SessionName(tt.sess), tt.env)
			if got != tt.want {
				t.Fatalf("paneLabelFor(%q, %v) = %q, want %q", tt.sess, tt.env, got, tt.want)
			}
			if strings.HasPrefix(got, " (") || strings.HasSuffix(got, " ()") {
				t.Fatalf("paneLabelFor(%q) produced empty-parentheses label %q", tt.sess, got)
			}
		})
	}
}

func TestPaneLabelForClamp(t *testing.T) {
	longAgent := strings.Repeat("a", 200)
	longSession := "gc__" + strings.Repeat("é", 200)
	shortSession := "gc__short-mc-wisp-cccc"

	t.Run("long agent shortened, suffix intact", func(t *testing.T) {
		env := map[string]string{
			"GC_RIG":          "rex-gascity",
			"GC_ALIAS":        longAgent,
			"GC_SESSION_NAME": shortSession,
		}
		got := paneLabelFor("gc__short-mc-wisp-cccc", env)
		if n := utf8.RuneCountInString(got); n != maxPaneLabelRunes {
			t.Fatalf("label has %d runes, want %d: %q", n, maxPaneLabelRunes, got)
		}
		if !strings.HasSuffix(got, " ("+shortSession+")") {
			t.Fatalf("label %q lost the session suffix", got)
		}
	})

	t.Run("suffix alone too long falls back to the bare session name", func(t *testing.T) {
		env := map[string]string{
			"GC_RIG":          "rex-gascity",
			"GC_AGENT":        "agent",
			"GC_SESSION_NAME": longSession,
		}
		got := paneLabelFor("gc__long", env)
		if want := clampRexLabel(longSession); got != want {
			t.Fatalf("label = %q, want the bare clamped session name %q", got, want)
		}
		if !utf8.ValidString(got) {
			t.Fatalf("label %q is not valid UTF-8", got)
		}
		if again := paneLabelFor("gc__long", env); again != got {
			t.Fatalf("label is not deterministic: %q != %q", got, again)
		}
	})

	// The parenthesised session name is the discovery identity, so two long
	// sessions that differ only at the tail must still yield distinct labels.
	t.Run("long sessions differing at the tail stay distinct", func(t *testing.T) {
		base := "gc__" + strings.Repeat("s", 100)
		label := func(session string) string {
			return paneLabelFor(SessionName(session), map[string]string{
				"GC_RIG":          "rex-gascity",
				"GC_ALIAS":        strings.Repeat("a", 60),
				"GC_SESSION_NAME": session,
			})
		}
		la, lb := label(base+"-aaaa"), label(base+"-bbbb")
		if la == lb {
			t.Fatalf("sessions differing only at the tail share label %q", la)
		}
		for _, got := range []string{la, lb} {
			if n := utf8.RuneCountInString(got); n > maxPaneLabelRunes {
				t.Fatalf("label has %d runes, want <= %d: %q", n, maxPaneLabelRunes, got)
			}
		}
	})

	// The pool form's "#<id>" tail is display-only. An over-long pool form
	// drops that tail before it touches the agent, so the label still names
	// the session through the bracketed pool identity.
	t.Run("verified short form over the limit drops the display id", func(t *testing.T) {
		agent := strings.Repeat("a", 30)
		bead := strings.Repeat("9", 70)
		session := agent + "-" + bead
		id := "gcg-session-" + strings.Repeat("7", 30)
		env := map[string]string{
			"GC_RIG":          "rex-gascity",
			"GC_TEMPLATE":     "rex-gascity/" + agent,
			"GC_AGENT":        bead,
			"GC_SESSION_ID":   id,
			"GC_SESSION_NAME": session,
		}
		if full := "[" + agent + "] (#" + bead + ") #" + id; utf8.RuneCountInString(full) <= maxPaneLabelRunes {
			t.Fatalf("fixture pool form is %d runes, want over %d", utf8.RuneCountInString(full), maxPaneLabelRunes)
		}
		got := paneLabelFor(SessionName(session), env)
		want := "[" + agent + "] (#" + bead + ")"
		if got != want {
			t.Fatalf("paneLabelFor = %q, want %q", got, want)
		}
		if n := utf8.RuneCountInString(got); n > maxPaneLabelRunes {
			t.Fatalf("label has %d runes, want <= %d: %q", n, maxPaneLabelRunes, got)
		}
		if again := paneLabelFor(SessionName(session), env); again != got {
			t.Fatalf("label is not deterministic: %q != %q", got, again)
		}
		if !identityMatch(SessionName(session), got) {
			t.Fatalf("identityMatch(%q, %q) = false; dropping the display id must not cost the identity", session, got)
		}
		if sibling := SessionName(agent + "-" + strings.Repeat("8", len(bead))); identityMatch(sibling, got) {
			t.Fatalf("label %q was adopted by a sibling seat %q", got, sibling)
		}
	})

	t.Run("verified short form with a suffix that cannot fit falls back", func(t *testing.T) {
		agent := strings.Repeat("a", 60)
		bead := strings.Repeat("9", 200)
		session := agent + "-" + bead
		env := map[string]string{
			"GC_RIG":          "rex-gascity",
			"GC_TEMPLATE":     "rex-gascity/" + agent,
			"GC_AGENT":        bead,
			"GC_SESSION_NAME": session,
		}
		got := paneLabelFor(SessionName(session), env)
		if want := clampRexLabel(session); got != want {
			t.Fatalf("label = %q, want the bare clamped session name %q", got, want)
		}
	})

	t.Run("short label unchanged", func(t *testing.T) {
		env := map[string]string{
			"GC_RIG":          "rex-gascity",
			"GC_AGENT":        "agent",
			"GC_SESSION_NAME": shortSession,
		}
		want := "[agent] (" + shortSession + ")"
		if got := paneLabelFor("gc__short", env); got != want {
			t.Fatalf("paneLabelFor = %q, want %q", got, want)
		}
	})
}

// TestPaneLabelOverflowIdentity is the boundary proof for the pool form: at
// exactly the 128-rune limit the display-only "#<id>" tail is kept, one rune
// over it is dropped, and neither shortening costs the label its identity. A
// shortened label fits a whole family of names, so accepting one would let a
// recordless IsRunning/Stop pick the wrong pane.
func TestPaneLabelOverflowIdentity(t *testing.T) {
	const agent = "worker"
	id := "gcg-session-" + strings.Repeat("7", 20)
	base := len("[worker] (#)")
	atLimit := maxPaneLabelRunes - base - len(" #"+id)
	tests := []struct {
		name  string
		pool  int
		tail  bool
		names bool
	}{
		{"exact limit", atLimit, true, true},
		{"one over the limit", atLimit + 1, false, true},
		// A pool identity that cannot fit even without the tail falls back to
		// the clamped session name and names nobody.
		{"far over the limit", atLimit + 100, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool := strings.Repeat("9", tt.pool)
			session := agent + "-" + pool
			env := map[string]string{
				"GC_RIG":          "rex-gascity",
				"GC_TEMPLATE":     "rex-gascity/" + agent,
				"GC_AGENT":        pool,
				"GC_SESSION_ID":   id,
				"GC_SESSION_NAME": session,
			}
			got := paneLabelFor(SessionName(session), env)
			if n := utf8.RuneCountInString(got); n > maxPaneLabelRunes {
				t.Fatalf("label has %d runes, want <= %d: %q", n, maxPaneLabelRunes, got)
			}
			if hasTail := strings.Contains(got, " #"); hasTail != tt.tail {
				t.Fatalf("label %q tail = %v, want %v", got, hasTail, tt.tail)
			}
			if names := identityMatch(SessionName(session), got); names != tt.names {
				t.Fatalf("identityMatch(%q, %q) = %v, want %v", session, got, names, tt.names)
			}
			if sibling := SessionName(agent + "-" + strings.Repeat("8", tt.pool)); identityMatch(sibling, got) {
				t.Fatalf("label %q was adopted by a sibling seat %q", got, sibling)
			}
		})
	}
}

// TestLegacyShortenedShortFormIsNotAnIdentity covers a pane written before
// the bracketed tags with an agent too long for the label: the agent was cut
// (and marked) but the parenthesised bead id stayed, so the label fits two
// names. The scan must not adopt it: this session's own pane, and a sibling
// whose name shares the visible prefix and the bead id, are both refused,
// because a label that fits two sessions is no evidence for either. Leaving
// the pane behind beats stopping the wrong one.
func TestLegacyShortenedShortFormIsNotAnIdentity(t *testing.T) {
	ctx := context.Background()
	agent := strings.Repeat("a", 100)
	bead := strings.Repeat("9", 40)
	session := agent + "-" + bead
	short := agent[:60] + paneLabelShortened
	label := short + " (#" + bead + ")"
	fake := faketest.New()
	fake.Sessions = []rex.Session{{ID: "session:gc:acme", Label: "gc:acme"}}
	block := rex.BlockID("block:" + label)
	fake.View = rex.SessionView{Windows: []rex.Window{{
		ID:     "window:rig",
		Label:  "Rig: rex-gascity",
		Blocks: []rex.Block{{ID: block, Label: label}},
	}}}
	fake.Processes = map[rex.BlockID]rex.ProcessInfo{
		block: {Foreground: &rex.Process{PID: fakePID, Name: "pi"}},
	}
	if !strings.HasSuffix(label, " (#"+bead+")") || !strings.Contains(label, paneLabelShortened) {
		t.Fatalf("label %q does not mark its shortened agent", label)
	}
	// Names built from the visible agent text are exactly what must not match.
	names := []string{
		session,
		agent + "-extra-" + bead,
		short[:len(short)-len(paneLabelShortened)] + "-" + bead,
		short + "-" + bead,
	}
	for _, name := range names {
		running, err := New("acme", fake).IsRunning(ctx, name)
		if err != nil || running {
			t.Fatalf("IsRunning(%s) = %v, %v; want false, nil", name, running, err)
		}
		if err := New("acme", fake).Stop(ctx, name); err != nil {
			t.Fatalf("Stop(%s): %v", name, err)
		}
	}
	if slices.Contains(fake.ClosedBlocks, block) {
		t.Fatalf("Stop closed %s from a shortened label; closed %v", block, fake.ClosedBlocks)
	}
}

// TestPaneLabelRediscovery is the GCREX-5qel section 3 agreement proof: for one
// start-config env the hint paneLabelFor produces is the label record() stores,
// and a resumed start finds the live block under that label both from the
// stored sidecar record and, with the record lost, from the env-derived hint
// (provider.go:458).
func TestPaneLabelRediscovery(t *testing.T) {
	ctx := context.Background()
	const (
		start   = "pi-mc-wisp-fvwrce"
		session = "gc__implementation-worker-mc-wisp-fvwrce"
		want    = "[mc-wisp-fvwrce] (" + session + ")"
	)
	env := map[string]string{
		"GC_RIG":          "rex-gascity",
		"GC_AGENT":        "mc-wisp-fvwrce",
		"GC_SESSION_NAME": session,
	}

	fake := faketest.New()
	p := New("acme", fake)
	if err := p.Start(ctx, start, Config{Argv: []string{"pi"}, Env: env}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	stored, ok := p.Binding(start)
	if !ok {
		t.Fatal("Binding(start) = false after Start")
	}
	if hint := paneLabelFor(start, env); hint != stored.PaneLabel {
		t.Fatalf("hint label = %q, recorded label = %q", hint, stored.PaneLabel)
	}
	if got := fake.CreatedWindows[0].Spec.Label; got != want {
		t.Fatalf("launched pane label = %q, want %q", got, want)
	}

	// Sidecar record present: a resumed start keeps the live block.
	if err := p.Start(ctx, start, Config{Argv: []string{"pi"}, Env: env}); err != nil {
		t.Fatalf("Start with record present: %v", err)
	}
	if len(fake.CreatedWindows) != 1 || len(fake.CreatedSplits) != 0 {
		t.Fatalf("record-present resume relaunched a pane: windows=%d splits=%d",
			len(fake.CreatedWindows), len(fake.CreatedSplits))
	}

	// Sidecar record lost: a fresh provider sees only Rex, so the env-derived
	// hint must find the block under the same label instead of launching again.
	rebuilt := faketest.New()
	rebuilt.Sessions = fake.Sessions
	rebuilt.View = fake.View
	if err := New("acme", rebuilt).Start(ctx, start, Config{Argv: []string{"pi"}, Env: env}); err != nil {
		t.Fatalf("Start with record lost: %v", err)
	}
	if len(rebuilt.CreatedWindows) != 0 || len(rebuilt.CreatedSplits) != 0 {
		t.Fatalf("lost-record resume relaunched a pane: windows=%d splits=%d",
			len(rebuilt.CreatedWindows), len(rebuilt.CreatedSplits))
	}
}

// TestPaneLabelRediscoveryAfterTemplateChange covers the upgrade path: a pane
// launched before GC_TEMPLATE drove the agent prefix carries the tagged long
// form with "<GC_AGENT>" as the agent. With GC_TEMPLATE set, the new hint differs
// by the agent (and the start name differs from GC_SESSION_NAME), so a
// recordless start must still find the old pane by its session suffix instead
// of launching a duplicate.
func TestPaneLabelRediscoveryAfterTemplateChange(t *testing.T) {
	ctx := context.Background()
	const (
		start    = "pi-mc-wisp-fvwrce"
		session  = "gc__implementation-worker-mc-wisp-fvwrce"
		oldLabel = "[mc-wisp-fvwrce] (" + session + ")"
	)
	oldEnv := map[string]string{
		"GC_RIG":          "rex-gascity",
		"GC_AGENT":        "mc-wisp-fvwrce",
		"GC_SESSION_NAME": session,
	}
	env := map[string]string{
		"GC_RIG":          "rex-gascity",
		"GC_TEMPLATE":     "rex-gascity/gc.implementation-worker",
		"GC_AGENT":        "mc-wisp-fvwrce",
		"GC_SESSION_NAME": session,
	}

	fake := faketest.New()
	if err := New("acme", fake).Start(ctx, start, Config{Argv: []string{"pi"}, Env: oldEnv}); err != nil {
		t.Fatalf("Start (old label): %v", err)
	}
	if len(fake.CreatedWindows) != 1 || fake.CreatedWindows[0].Spec.Label != oldLabel {
		t.Fatalf("launched labels = %v, want %q", fake.CreatedWindows, oldLabel)
	}

	// Sidecar record lost, start env now carries GC_TEMPLATE: the hint names a
	// different agent, so only the shared session suffix still identifies it.
	rebuilt := faketest.New()
	rebuilt.Sessions = fake.Sessions
	rebuilt.View = fake.View
	if err := New("acme", rebuilt).Start(ctx, start, Config{Argv: []string{"pi"}, Env: env}); err != nil {
		t.Fatalf("Start with record lost: %v", err)
	}
	if len(rebuilt.CreatedWindows) != 0 || len(rebuilt.CreatedSplits) != 0 {
		t.Fatalf("lost-record resume relaunched a pane: windows=%d splits=%d",
			len(rebuilt.CreatedWindows), len(rebuilt.CreatedSplits))
	}
}

// TestRecordlessDiscoveryBySessionSuffix is the GCREX-5qel review fix for
// environment-free discovery: with no sidecar record and no start env, a
// resolve must still find the live pane whose label carries the
// parenthesised session name, before any Start supplies a hint.
func TestRecordlessDiscoveryBySessionSuffix(t *testing.T) {
	ctx := context.Background()
	const (
		name  = "rex-gascity--worker"
		label = "worker (rex-gascity--worker)"
		block = rex.BlockID("block:" + label)
	)
	newFake := func() *faketest.Fake {
		f := faketest.New()
		f.Sessions = []rex.Session{{ID: "session:gc:acme", Label: "gc:acme"}}
		f.View = rex.SessionView{Windows: []rex.Window{{
			ID:     "window:rig",
			Label:  "Rig: rex-gascity",
			Blocks: []rex.Block{{ID: block, Label: label}},
		}}}
		return f
	}

	t.Run("IsRunning", func(t *testing.T) {
		running, err := New("acme", newFake()).IsRunning(ctx, name)
		if err != nil || !running {
			t.Fatalf("IsRunning(%s) = %v, %v; want true, nil", name, running, err)
		}
	})

	t.Run("Stop", func(t *testing.T) {
		fake := newFake()
		if err := New("acme", fake).Stop(ctx, name); err != nil {
			t.Fatalf("Stop(%s): %v", name, err)
		}
		if !slices.Contains(fake.ClosedBlocks, block) {
			t.Fatalf("Stop did not close %s; closed %v", block, fake.ClosedBlocks)
		}
	})
}

// TestRecordlessCrossTabDiscovery is the GCREX-5qel round-2 fix for a no-`--`
// session started with GC_RIG: it lives in a "Rig: <rig>" tab the name cannot
// derive, so a recordless resolve must find it by session identity across every
// tab instead of scanning the town tab only.
func TestRecordlessCrossTabDiscovery(t *testing.T) {
	ctx := context.Background()
	const (
		name  = "pi-mc-wisp-ybq1"
		label = "pi-agent (pi-mc-wisp-ybq1)"
		block = rex.BlockID("block:" + label)
	)
	newFake := func() *faketest.Fake {
		f := faketest.New()
		f.Sessions = []rex.Session{{ID: "session:gc:acme", Label: "gc:acme"}}
		f.View = rex.SessionView{Windows: []rex.Window{{
			ID:     "window:my-project",
			Label:  "Rig: my-project",
			Blocks: []rex.Block{{ID: block, Label: label}},
		}}}
		f.Processes = map[rex.BlockID]rex.ProcessInfo{
			block: {Foreground: &rex.Process{PID: fakePID, Name: "pi"}},
		}
		return f
	}

	t.Run("IsRunning", func(t *testing.T) {
		running, err := New("acme", newFake()).IsRunning(ctx, name)
		if err != nil || !running {
			t.Fatalf("IsRunning(%s) = %v, %v; want true, nil", name, running, err)
		}
	})

	t.Run("Stop", func(t *testing.T) {
		fake := newFake()
		if err := New("acme", fake).Stop(ctx, name); err != nil {
			t.Fatalf("Stop(%s): %v", name, err)
		}
		if !slices.Contains(fake.ClosedBlocks, block) {
			t.Fatalf("Stop did not close %s; closed %v", block, fake.ClosedBlocks)
		}
	})
}

// TestRecordlessRigPaneNotAttributedThroughTownTab guards the CodeRabbit
// finding on the recordless fallback: a legacy town tab holds the town pane
// labelled "mayor", so a recordless resolve for the rig session
// "Indigo--mayor" must not accept that bare label just because its agent part
// is "mayor". The town tab is a candidate only for a town-level name, so the
// actual town session still resolves.
func TestRecordlessRigPaneNotAttributedThroughTownTab(t *testing.T) {
	ctx := context.Background()
	clearCityPin(t)
	t.Setenv("GC_REX_CITY", "my-city")
	const (
		townName = "mayor"
		ownBlock = rex.BlockID("block:mayor")
	)
	newFake := func() *faketest.Fake {
		f := faketest.New()
		f.Sessions = []rex.Session{{ID: "session:gc:acme", Label: "gc:acme"}}
		f.View = rex.SessionView{Windows: []rex.Window{{
			ID:     "window:town",
			Label:  "my-city",
			Blocks: []rex.Block{{ID: ownBlock, Label: townName}},
		}}}
		f.Processes = map[rex.BlockID]rex.ProcessInfo{
			ownBlock: {Foreground: &rex.Process{PID: fakePID, Name: "pi"}},
		}
		return f
	}
	newProvider := func() *Provider { return New("acme", newFake()) }

	t.Run("rig name does not claim the town pane", func(t *testing.T) {
		running, err := newProvider().IsRunning(ctx, "Indigo--mayor")
		if err != nil {
			t.Fatalf("IsRunning: %v", err)
		}
		if running {
			t.Fatalf("IsRunning(Indigo--mayor) = true; the town pane of mayor must not answer for it")
		}
	})

	t.Run("town name still resolves", func(t *testing.T) {
		running, err := newProvider().IsRunning(ctx, townName)
		if err != nil {
			t.Fatalf("IsRunning: %v", err)
		}
		if !running {
			t.Fatalf("IsRunning(%s) = false; the town rung must keep finding the town pane", townName)
		}
	})
}

// TestRecordlessTownRigNameCollision guards the CodeRabbit finding on
// binding.go:574. When the city pin equals the rig name, the pre-GCREX-72 tab
// label for the rig (its bare rig name) is the same string as the town tab's
// bare name. A bare pane label in that tab cannot say which session owns the
// pane: the town pane labelled "mayor" sits in "Indigo" exactly where a legacy
// rig pane for "Indigo--mayor" would. The rig name must not adopt the town
// pane, while the town name still resolves it.
func TestRecordlessTownRigNameCollision(t *testing.T) {
	ctx := context.Background()
	clearCityPin(t)
	t.Setenv("GC_REX_CITY", "Indigo")
	const (
		pin       = "Indigo"
		rigName   = pin + "--mayor"
		townName  = "mayor"
		townBlock = rex.BlockID("block:mayor")
	)
	newFake := func() *faketest.Fake {
		f := faketest.New()
		f.Sessions = []rex.Session{{ID: "session:gc:acme", Label: "gc:acme"}}
		// The bare pin tab is the pre-GCREX-72 town tab; its pane carries the
		// bare agent label.
		f.View = rex.SessionView{Windows: []rex.Window{{
			ID:     "window:town",
			Label:  pin,
			Blocks: []rex.Block{{ID: townBlock, Label: townName}},
		}}}
		f.Processes = map[rex.BlockID]rex.ProcessInfo{
			townBlock: {Foreground: &rex.Process{PID: fakePID, Name: "pi"}},
		}
		return f
	}
	newProvider := func() *Provider { return New("acme", newFake()) }

	t.Run("rig name does not claim the town pane", func(t *testing.T) {
		running, err := newProvider().IsRunning(ctx, rigName)
		if err != nil {
			t.Fatalf("IsRunning(%s): %v", rigName, err)
		}
		if running {
			t.Fatalf("IsRunning(%s) = true; the town pane of mayor must not answer for it", rigName)
		}
		fake := newFake()
		p := New("acme", fake)
		if err := p.Stop(ctx, rigName); err != nil {
			t.Fatalf("Stop(%s): %v", rigName, err)
		}
		if slices.Contains(fake.ClosedBlocks, townBlock) {
			t.Fatalf("Stop(%s) closed the town pane %s", rigName, townBlock)
		}
	})

	t.Run("town name still resolves its pane", func(t *testing.T) {
		running, err := newProvider().IsRunning(ctx, townName)
		if err != nil {
			t.Fatalf("IsRunning(%s): %v", townName, err)
		}
		if !running {
			t.Fatalf("IsRunning(%s) = false; the town rung must keep finding the town pane", townName)
		}
		fake := newFake()
		p := New("acme", fake)
		if err := p.Stop(ctx, townName); err != nil {
			t.Fatalf("Stop(%s): %v", townName, err)
		}
		if !slices.Contains(fake.ClosedBlocks, townBlock) {
			t.Fatalf("Stop(%s) did not close the town pane %s; closed %v", townName, townBlock, fake.ClosedBlocks)
		}
	})
}

// TestHashLeadingNameIsNotThePoolForm proves a complete session name beginning
// with the pool mark never renders as the pool form, which discovery would read
// as the different session Sanitize(agent)+"-"+tail (CodeRabbit). The "#seat"
// pane must answer for "#seat" and must not answer for the pool seat
// "worker-seat".
func TestHashLeadingNameIsNotThePoolForm(t *testing.T) {
	ctx := context.Background()
	const name = SessionName("#seat")
	env := map[string]string{"GC_ALIAS": "worker", "GC_SESSION_NAME": string(name)}

	fake := faketest.New()
	p := New("acme", fake)
	if err := p.Start(ctx, string(name), Config{Argv: []string{"pi"}, Env: env}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if len(fake.CreatedWindows) != 1 {
		t.Fatalf("created windows = %d, want 1", len(fake.CreatedWindows))
	}
	label := fake.CreatedWindows[0].Spec.Label
	if !identityMatch(name, label) {
		t.Fatalf("pane label = %q, want a label that names %q", label, name)
	}
	if identityMatch(SessionName("worker-seat"), label) {
		t.Fatalf("pane label = %q names the pool seat worker-seat", label)
	}
	if want := `[worker] (\#seat)`; label != want {
		t.Fatalf("pane label = %q, want %q; the escape keeps the identity out of the pool form", label, want)
	}

	// Record lost: discovery must still find the pane for "#seat" and never
	// for the pool seat the parenthesised form would have named.
	rebuilt := faketest.New()
	rebuilt.Sessions = fake.Sessions
	rebuilt.View = fake.View
	if running, err := New("acme", rebuilt).IsRunning(ctx, string(name)); err != nil || !running {
		t.Fatalf("IsRunning(%q) = %v, %v; the pane must answer for its own name", name, running, err)
	}
	if running, err := New("acme", rebuilt).IsRunning(ctx, "worker-seat"); err != nil || running {
		t.Fatalf("IsRunning(worker-seat) = %v, %v; the #seat pane must not answer for the pool seat", running, err)
	}
}

// TestRecordlessHashLeadingRigSessionFindsItsPane pins the CodeRabbit finding
// on launch_identity.go: a hash-leading name started under GC_RIG lives in the
// "[rig] Indigo" tab, which a recordless scan cannot derive from the name. Its
// pane label therefore has to carry the identity itself, or IsRunning reports
// the live pane as gone after the binding record is lost.
func TestRecordlessHashLeadingRigSessionFindsItsPane(t *testing.T) {
	ctx := context.Background()
	clearCityPin(t)
	const name = SessionName("#seat")
	env := map[string]string{
		"GC_RIG":          "Indigo",
		"GC_ALIAS":        "worker",
		"GC_SESSION_NAME": string(name),
	}

	fake := faketest.New()
	p := New("acme", fake)
	if err := p.Start(ctx, string(name), Config{Argv: []string{"pi"}, Env: env}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if len(fake.CreatedWindows) != 1 {
		t.Fatalf("created windows = %d, want 1", len(fake.CreatedWindows))
	}
	if want := "[rig] Indigo"; fake.CreatedWindows[0].Label != want {
		t.Fatalf("tab label = %q, want %q", fake.CreatedWindows[0].Label, want)
	}
	label := fake.CreatedWindows[0].Spec.Label
	if !identityMatch(name, label) {
		t.Fatalf("pane label = %q, want a label that names %q", label, name)
	}
	if identityMatch(SessionName("worker-seat"), label) {
		t.Fatalf("pane label = %q names the pool seat worker-seat", label)
	}

	// Record lost and no start env survives: the cross-tab scan has only the
	// name, and the "[rig] Indigo" tab is not a name-derived candidate.
	rebuilt := faketest.New()
	rebuilt.Sessions = fake.Sessions
	rebuilt.View = fake.View
	if running, err := New("acme", rebuilt).IsRunning(ctx, string(name)); err != nil || !running {
		t.Fatalf("IsRunning(%q) = %v, %v; the pane must answer without its record", name, running, err)
	}
	if running, err := New("acme", rebuilt).IsRunning(ctx, "worker-seat"); err != nil || running {
		t.Fatalf("IsRunning(worker-seat) = %v, %v; the #seat pane must not answer for the pool seat", running, err)
	}
}

// TestRecordlessPoolAndNamedSiblingResolveTheirOwnPane is the recordless
// discovery proof CodeRabbit asked for on label.go: the marked pool label
// "[worker] (#seat)" and the unmarked named label "[worker] (seat)" are
// different sessions. With no record and no start env, each session must
// resolve only its own pane, so Stop acts on the right one.
func TestRecordlessPoolAndNamedSiblingResolveTheirOwnPane(t *testing.T) {
	ctx := context.Background()
	clearCityPin(t)
	const (
		poolName   = "worker-seat"
		namedName  = "seat"
		poolLabel  = "[worker] (#seat)"
		namedLabel = "[worker] (seat)"
		poolBlock  = rex.BlockID("block:pool")
		namedBlock = rex.BlockID("block:named")
	)
	newFake := func() *faketest.Fake {
		f := faketest.New()
		f.Sessions = []rex.Session{{ID: "session:gc:acme", Label: "gc:acme"}}
		f.View = rex.SessionView{Windows: []rex.Window{{
			ID:    "window:rig",
			Label: "Rig: rex-gascity",
			Blocks: []rex.Block{
				{ID: poolBlock, Label: poolLabel},
				{ID: namedBlock, Label: namedLabel},
			},
		}}}
		return f
	}

	t.Run("pool seat resolves only the marked pane", func(t *testing.T) {
		if running, err := New("acme", newFake()).IsRunning(ctx, poolName); err != nil || !running {
			t.Fatalf("IsRunning(%s) = %v, %v; want true, nil", poolName, running, err)
		}
		fake := newFake()
		if err := New("acme", fake).Stop(ctx, poolName); err != nil {
			t.Fatalf("Stop(%s): %v", poolName, err)
		}
		if !slices.Contains(fake.ClosedBlocks, poolBlock) {
			t.Fatalf("Stop(%s) did not close %s; closed %v", poolName, poolBlock, fake.ClosedBlocks)
		}
		if slices.Contains(fake.ClosedBlocks, namedBlock) {
			t.Fatalf("Stop(%s) closed the named sibling %s", poolName, namedBlock)
		}
	})

	t.Run("named seat resolves only the unmarked pane", func(t *testing.T) {
		if running, err := New("acme", newFake()).IsRunning(ctx, namedName); err != nil || !running {
			t.Fatalf("IsRunning(%s) = %v, %v; want true, nil", namedName, running, err)
		}
		fake := newFake()
		if err := New("acme", fake).Stop(ctx, namedName); err != nil {
			t.Fatalf("Stop(%s): %v", namedName, err)
		}
		if !slices.Contains(fake.ClosedBlocks, namedBlock) {
			t.Fatalf("Stop(%s) did not close %s; closed %v", namedName, namedBlock, fake.ClosedBlocks)
		}
		if slices.Contains(fake.ClosedBlocks, poolBlock) {
			t.Fatalf("Stop(%s) closed the pool sibling %s", namedName, poolBlock)
		}
	})
}

// TestTownPaneLabelClamp pins the 128-rune limit on the town-level pane label:
// an agent and session name that together overflow are shortened the same way
// the rig branch shortens, and the parenthesised identity survives the cut.
func TestTownPaneLabelClamp(t *testing.T) {
	t.Run("label over the limit shortens the agent", func(t *testing.T) {
		agent := strings.Repeat("a", 62)
		name := SessionName(strings.Repeat("b", 62))
		got := townPaneLabel(agent, name)
		if n := utf8.RuneCountInString(got); n > maxPaneLabelRunes {
			t.Fatalf("label has %d runes, want <= %d: %q", n, maxPaneLabelRunes, got)
		}
		if !strings.HasSuffix(got, "("+string(name)+")") {
			t.Fatalf("label %q lost the parenthesised identity", got)
		}
		if !strings.Contains(got, paneLabelShortened) {
			t.Fatalf("label %q does not mark the shortened agent", got)
		}
		if !identityMatch(name, got) {
			t.Fatalf("identityMatch(%q, %q) = false; the cut must keep the identity", name, got)
		}
	})

	t.Run("name that cannot fit falls back to the bare clamped name", func(t *testing.T) {
		name := SessionName(strings.Repeat("c", 200))
		if got, want := townPaneLabel("agent", name), clampRexLabel(string(name)); got != want {
			t.Fatalf("townPaneLabel = %q, want %q", got, want)
		}
	})
}

// TestRecordlessBareLabelNotAdoptedGlobally is the round-3 fix for the
// CodeRabbit Major thread on binding.go: a bare label (the fallback when
// "<agent> (<session>)" cannot fit) does not identify which session created
// the pane, so the global fallback must not bind it. Another session's pane
// can carry the same bare label (a rigless "other--worker" is labelled
// "worker"). Lengths 125-128 are the range where the suffix stops fitting but
// the name is still within Rex's limit.
func TestRecordlessBareLabelNotAdoptedGlobally(t *testing.T) {
	ctx := context.Background()
	for _, n := range []int{125, 126, 127, 128} {
		session := strings.Repeat("s", n)
		label := clampRexLabel(session)
		if label != session {
			t.Fatalf("session of %d runes clamps to %q, want itself", n, label)
		}
		block := rex.BlockID("block:" + label)
		newFake := func() *faketest.Fake {
			f := faketest.New()
			f.Sessions = []rex.Session{{ID: "session:gc:acme", Label: "gc:acme"}}
			f.View = rex.SessionView{Windows: []rex.Window{{
				ID:     "window:rig",
				Label:  "Rig: my-project",
				Blocks: []rex.Block{{ID: block, Label: label}},
			}}}
			f.Processes = map[rex.BlockID]rex.ProcessInfo{
				block: {Foreground: &rex.Process{PID: fakePID, Name: "pi"}},
			}
			return f
		}

		t.Run(fmt.Sprintf("len=%d", n), func(t *testing.T) {
			running, err := New("acme", newFake()).IsRunning(ctx, session)
			if err != nil {
				t.Fatalf("IsRunning: %v", err)
			}
			if running {
				t.Fatalf("IsRunning = true; bare label %q in another tab must not be adopted", label)
			}
			fake := newFake()
			if err := New("acme", fake).Stop(ctx, session); err != nil {
				t.Fatalf("Stop: %v", err)
			}
			if slices.Contains(fake.ClosedBlocks, block) {
				t.Fatalf("Stop closed %s; a bare label must not be adopted", block)
			}
		})
	}
}

// TestTabScopedBareLabelNotAdopted guards the CodeRabbit concern retained on
// binding.go: the tab-scoped scan also accepted a bare label equal to the
// session name. A legacy pane for "rig--rig--worker" is labelled "rig--worker"
// and sits in "Rig: rig" — exactly the tab a recordless resolve for
// "rig--worker" scans — so the scan adopted that other session's pane. Only the
// expected pane label (the hint/stored PaneLabel or the name-derived one) is
// accepted in the candidate tabs now.
func TestTabScopedBareLabelNotAdopted(t *testing.T) {
	ctx := context.Background()
	const (
		name  = "rig--worker"
		label = "rig--worker" // legacy pane of rig--rig--worker
		block = rex.BlockID("block:legacy")
	)
	newFake := func() *faketest.Fake {
		f := faketest.New()
		f.Sessions = []rex.Session{{ID: "session:gc:acme", Label: "gc:acme"}}
		f.View = rex.SessionView{Windows: []rex.Window{{
			ID:     "window:rig",
			Label:  "Rig: rig",
			Blocks: []rex.Block{{ID: block, Label: label}},
		}}}
		f.Processes = map[rex.BlockID]rex.ProcessInfo{
			block: {Foreground: &rex.Process{PID: fakePID, Name: "pi"}},
		}
		return f
	}

	running, err := New("acme", newFake()).IsRunning(ctx, name)
	if err != nil {
		t.Fatalf("IsRunning: %v", err)
	}
	if running {
		t.Fatalf("IsRunning = true; tab-scoped bare label %q of another session must not be adopted", label)
	}
	fake := newFake()
	if err := New("acme", fake).Stop(ctx, name); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if slices.Contains(fake.ClosedBlocks, block) {
		t.Fatalf("Stop closed %s; a tab-scoped bare label must not be adopted", block)
	}
}

// TestBareLabelPaneFoundWithAttribution is the counterpart proof: a 125-128
// rune town-level session name yields a bare session label, and Start still
// finds its own pane both through the hint it passes and through the stored
// record.
func TestBareLabelPaneFoundWithAttribution(t *testing.T) {
	ctx := context.Background()
	for _, n := range []int{125, 126, 127, 128} {
		session := strings.Repeat("s", n)
		env := map[string]string{"GC_SESSION_NAME": session}

		fake := faketest.New()
		p := New("acme", fake)
		if err := p.Start(ctx, session, Config{Argv: []string{"pi"}, Env: env}); err != nil {
			t.Fatalf("len=%d: Start: %v", n, err)
		}
		if got := fake.CreatedWindows[0].Spec.Label; got != session {
			t.Fatalf("len=%d: launched pane label = %q, want the bare session label", n, got)
		}
		if _, ok := p.Binding(session); !ok {
			t.Fatalf("len=%d: Binding = false after Start", n)
		}

		// Record present: a resumed start keeps the live block.
		if err := p.Start(ctx, session, Config{Argv: []string{"pi"}, Env: env}); err != nil {
			t.Fatalf("len=%d: Start with record present: %v", n, err)
		}
		if len(fake.CreatedWindows) != 1 || len(fake.CreatedSplits) != 0 {
			t.Fatalf("len=%d: record-present resume relaunched a pane: windows=%d splits=%d",
				n, len(fake.CreatedWindows), len(fake.CreatedSplits))
		}

		// Record lost: the env-derived hint carries the bare label, so a fresh
		// provider finds the live block instead of launching again.
		rebuilt := faketest.New()
		rebuilt.Sessions = fake.Sessions
		rebuilt.View = fake.View
		if err := New("acme", rebuilt).Start(ctx, session, Config{Argv: []string{"pi"}, Env: env}); err != nil {
			t.Fatalf("len=%d: Start with record lost: %v", n, err)
		}
		if len(rebuilt.CreatedWindows) != 0 || len(rebuilt.CreatedSplits) != 0 {
			t.Fatalf("len=%d: lost-record resume relaunched a pane: windows=%d splits=%d",
				n, len(rebuilt.CreatedWindows), len(rebuilt.CreatedSplits))
		}
	}
}

// TestTabScopedScanPrefersSuffixLabel pins the precedence inside one tab: a
// suffix-labelled pane names the session outright, while a bare label only
// belongs to it through the hint/record. The suffix-labelled pane must win
// whichever order Rex lists the panes in.
func TestTabScopedScanPrefersSuffixLabel(t *testing.T) {
	ctx := context.Background()
	const (
		name     = "rex-gascity--worker"
		bare     = rex.BlockID("block:worker")
		suffixed = rex.BlockID("block:agent (rex-gascity--worker)")
	)
	barePane := rex.Block{ID: bare, Label: "worker"}
	suffixPane := rex.Block{ID: suffixed, Label: "agent (rex-gascity--worker)"}
	newFake := func(blocks ...rex.Block) *faketest.Fake {
		f := faketest.New()
		f.Sessions = []rex.Session{{ID: "session:gc:acme", Label: "gc:acme"}}
		f.View = rex.SessionView{Windows: []rex.Window{{
			ID:     "window:rig",
			Label:  "Rig: rex-gascity",
			Blocks: blocks,
		}}}
		return f
	}

	for _, tt := range []struct {
		order  string
		blocks []rex.Block
	}{
		{"bare first", []rex.Block{barePane, suffixPane}},
		{"suffix first", []rex.Block{suffixPane, barePane}},
	} {
		t.Run(tt.order, func(t *testing.T) {
			fake := newFake(tt.blocks...)
			if err := New("acme", fake).Stop(ctx, name); err != nil {
				t.Fatalf("Stop(%s): %v", name, err)
			}
			if slices.Contains(fake.ClosedBlocks, bare) {
				t.Fatalf("Stop closed the bare-label pane %s; the suffix-labelled pane names the session", bare)
			}
			if !slices.Contains(fake.ClosedBlocks, suffixed) {
				t.Fatalf("Stop did not close %s; closed %v", suffixed, fake.ClosedBlocks)
			}
		})
	}

	t.Run("bare alone still resolves", func(t *testing.T) {
		fake := newFake(barePane)
		if err := New("acme", fake).Stop(ctx, name); err != nil {
			t.Fatalf("Stop(%s): %v", name, err)
		}
		if !slices.Contains(fake.ClosedBlocks, bare) {
			t.Fatalf("Stop did not close the bare-label pane %s; closed %v", bare, fake.ClosedBlocks)
		}
	})
}

// TestPaneLabelForPoolShortForm is the live-case table for GCREX-...
// (gcrex-7n06r): an unaliased pool seat's label is
// "<GC_TEMPLATE basename> (#<session bead id>)", verifiable against the runtime
// session name alone (gc cmd/gc/pool_session_name.go PoolSessionName).
func TestPaneLabelForPoolShortForm(t *testing.T) {
	tests := []struct {
		name string
		sess string
		env  map[string]string
		want string
	}{
		{
			name: "implementation worker pool seat",
			sess: "gc__implementation-worker-mc-wisp-kvqib",
			env: map[string]string{
				"GC_RIG":          "rex-gascity",
				"GC_TEMPLATE":     "rex-gascity/gc.implementation-worker",
				"GC_AGENT":        "mc-wisp-kvqib",
				"GC_ALIAS":        "",
				"BEADS_ACTOR":     "rex-gascity/gc.implementation-worker",
				"GC_SESSION_ID":   "mc-wisp-kvqib",
				"GC_SESSION_NAME": "gc__implementation-worker-mc-wisp-kvqib",
			},
			want: "[gc.implementation-worker] (#mc-wisp-kvqib) #mc-wisp-kvqib",
		},
		{
			name: "second seat from the same template",
			sess: "gc__implementation-worker-mc-wisp-1e4r8",
			env: map[string]string{
				"GC_RIG":          "rex-gascity",
				"GC_TEMPLATE":     "rex-gascity/gc.implementation-worker",
				"GC_AGENT":        "mc-wisp-1e4r8",
				"GC_ALIAS":        "",
				"BEADS_ACTOR":     "rex-gascity/gc.implementation-worker",
				"GC_SESSION_ID":   "mc-wisp-1e4r8",
				"GC_SESSION_NAME": "gc__implementation-worker-mc-wisp-1e4r8",
			},
			want: "[gc.implementation-worker] (#mc-wisp-1e4r8) #mc-wisp-1e4r8",
		},
		{
			name: "run operator seat",
			sess: "gc__run-operator-mc-wisp-jwlvf",
			env: map[string]string{
				"GC_RIG":          "rex-gascity",
				"GC_TEMPLATE":     "rex-gascity/gc.run-operator",
				"GC_AGENT":        "mc-wisp-jwlvf",
				"GC_ALIAS":        "",
				"BEADS_ACTOR":     "rex-gascity/gc.run-operator",
				"GC_SESSION_ID":   "mc-wisp-jwlvf",
				"GC_SESSION_NAME": "gc__run-operator-mc-wisp-jwlvf",
			},
			want: "[gc.run-operator] (#mc-wisp-jwlvf) #mc-wisp-jwlvf",
		},
		{
			name: "aliased control dispatcher keeps the whole session name",
			sess: "core__control-dispatcher-mc-wisp-vmecx",
			env: map[string]string{
				"GC_RIG":          "rex-gascity",
				"GC_TEMPLATE":     "rex-gascity/core.control-dispatcher",
				"GC_ALIAS":        "rex-gascity/core.control-dispatcher",
				"GC_AGENT":        "mc-wisp-vmecx",
				"BEADS_ACTOR":     "rex-gascity/core.control-dispatcher",
				"GC_SESSION_NAME": "core__control-dispatcher-mc-wisp-vmecx",
			},
			want: "[core.control-dispatcher] (core__control-dispatcher-mc-wisp-vmecx)",
		},
		{
			name: "town-level unchanged",
			sess: "mayor",
			env: map[string]string{
				"GC_TEMPLATE":     "mayor",
				"GC_SESSION_NAME": "mayor",
			},
			want: "[mayor] (mayor)",
		},
		{
			name: "no gc env leaves the name-derived label",
			sess: "gc__implementation-worker-mc-wisp-kvqib",
			env:  map[string]string{},
			want: "gc__implementation-worker-mc-wisp-kvqib",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := paneLabelFor(SessionName(tt.sess), tt.env); got != tt.want {
				t.Fatalf("paneLabelFor(%q, %v) = %q, want %q", tt.sess, tt.env, got, tt.want)
			}
		})
	}
}

// TestPaneLabelForInvariantFallback pins the safety guard: when the start env
// does not satisfy Sanitize(prefix)+"-"+suffix == GC_SESSION_NAME, the label
// degrades to the old "<agent> (<GC_SESSION_NAME>)" shape instead of guessing.
func TestPaneLabelForInvariantFallback(t *testing.T) {
	env := map[string]string{
		"GC_RIG":          "rex-gascity",
		"GC_TEMPLATE":     "rex-gascity/worker",
		"GC_AGENT":        "mc-wisp-aaa",
		"BEADS_ACTOR":     "rex-gascity/worker",
		"GC_SESSION_NAME": "gc__other-mc-wisp-aaa",
	}
	const want = "[worker] (gc__other-mc-wisp-aaa)"
	if got := paneLabelFor("gc__other-mc-wisp-aaa", env); got != want {
		t.Fatalf("paneLabelFor = %q, want the long fallback %q", got, want)
	}

	// The long fallback stays resolvable recordlessly: the suffix is the
	// session name, so Stop and IsRunning find the pane without any env.
	ctx := context.Background()
	const block = rex.BlockID("block:fallback")
	newFake := func() *faketest.Fake {
		f := faketest.New()
		f.Sessions = []rex.Session{{ID: "session:gc:acme", Label: "gc:acme"}}
		f.View = rex.SessionView{Windows: []rex.Window{{
			ID:     "window:rig",
			Label:  "Rig: rex-gascity",
			Blocks: []rex.Block{{ID: block, Label: want}},
		}}}
		return f
	}
	running, err := New("acme", newFake()).IsRunning(ctx, "gc__other-mc-wisp-aaa")
	if err != nil || !running {
		t.Fatalf("IsRunning(long fallback) = %v, %v; want true, nil", running, err)
	}
	fake := newFake()
	if err := New("acme", fake).Stop(ctx, "gc__other-mc-wisp-aaa"); err != nil {
		t.Fatalf("Stop(long fallback): %v", err)
	}
	if !slices.Contains(fake.ClosedBlocks, block) {
		t.Fatalf("Stop did not close the long-fallback pane %s; closed %v", block, fake.ClosedBlocks)
	}
}

// TestIdentityMatch pins the discovery rule that replaces the bare suffix
// match: a label names session n when its parenthesised suffix is n (legacy
// and named labels) or n is exactly Sanitize(agent)+"-"+suffix (pool short
// form). Prefix-only matching is never accepted.
func TestIdentityMatch(t *testing.T) {
	tests := []struct {
		name  string
		sess  string
		label string
		want  bool
	}{
		{"legacy long form", "rex-gascity--worker", "worker (rex-gascity--worker)", true},
		{"tagged long form", "rex-gascity--worker", "[worker] (rex-gascity--worker)", true},
		{"tagged pool form", "gc__implementation-worker-mc-wisp-kvqib", "[gc.implementation-worker] (#mc-wisp-kvqib) #mc-wisp-kvqib", true},
		{"tagged pool form without the id", "gc__implementation-worker-mc-wisp-kvqib", "[gc.implementation-worker] (#mc-wisp-kvqib)", true},
		{"tagged unmarked pool-looking suffix names nobody", "gc__implementation-worker-mc-wisp-kvqib", "[gc.implementation-worker] (mc-wisp-kvqib)", false},
		{"named form", "core__control-dispatcher-mc-wisp-vmecx", "core.control-dispatcher (core__control-dispatcher-mc-wisp-vmecx)", true},
		{"pool short form", "gc__implementation-worker-mc-wisp-kvqib", "gc.implementation-worker (#mc-wisp-kvqib)", true},
		// A complete session name in the long form names only that session:
		// "[worker] (seat)" is the named session seat, never the marked pool
		// seat worker-seat (CodeRabbit, label.go).
		{"named session is not the pool seat", "worker-seat", "[worker] (seat)", false},
		{"named session is itself", "seat", "[worker] (seat)", true},
		// A complete name that starts with the pool mark is escaped in the
		// suffix, so the label names that session and not the pool seat the
		// unescaped form would name.
		{"escaped hash-leading long form", "#seat", `[worker] (\#seat)`, true},
		{"escaped hash-leading long form is not the pool seat", "worker-seat", `[worker] (\#seat)`, false},
		{"hash-leading pool-looking suffix is the pool seat", "worker-seat", "[worker] (#seat)", true},
		{"hash-leading pool-looking suffix is not the named session", "#seat", "[worker] (#seat)", false},
		// The escape itself is escaped, so one suffix still decodes to one name.
		{"escaped escape", `\#seat`, `[worker] (\\#seat)`, true},
		{"escaped escape is not the unescaped name", "#seat", `[worker] (\\#seat)`, false},
		// A long form's suffix is the whole session name. A label whose suffix
		// is only part of the name names nobody.
		{"long-form label, suffix is not the whole name", "legacy-agent-a--b-1", "legacy-agent (a--b-1)", false},
		{"long-form label, different agent", "other-agent-a--b-1", "legacy-agent (a--b-1)", false},
		{"another session's bead id same tab", "gc__implementation-worker-mc-wisp-1e4r8", "gc.implementation-worker (#mc-wisp-kvqib)", false},
		// The "#" mark keeps the bead id from reading as a session name: a
		// named session whose name is the bead id is not named by the label.
		{"named session equal to the bead id", "mc-wisp-kvqib", "gc.implementation-worker (#mc-wisp-kvqib)", false},
		{"bare label", "rex-gascity--worker", "worker", false},
		{"suffix satisfies neither branch", "gc__implementation-worker-mc-wisp-1e4r8", "other.agent (mc-wisp-1e4r8)", false},
		{"empty suffix", "rex-gascity--worker", "worker ()", false},
		{"prefix only", "rex-gascity--worker", "worker (nonsense)", false},
		// fitPaneLabel cuts the agent to fit 128 runes. That label names nobody:
		// a prefix plus the bead id fits other names too, so it is not evidence.
		{"clamped short form", strings.Repeat("a", 100) + "-" + strings.Repeat("9", 40), strings.Repeat("a", 85) + " (" + strings.Repeat("9", 40) + ")", false},
		{"clamped short form, sibling bead id", strings.Repeat("a", 100) + "-" + strings.Repeat("9", 40), strings.Repeat("a", 85) + " (" + strings.Repeat("8", 40) + ")", false},
		// A short form at exactly the limit was not cut, so its bead id and
		// agent still name the session. A name sharing the ends but not the
		// middle is another session.
		{"128-rune short form, exact name", strings.Repeat("a", 123) + "-id", strings.Repeat("a", 123) + " (#id)", true},
		{"128-rune short form, longer name with the same ends", strings.Repeat("a", 123) + "-extra-id", strings.Repeat("a", 123) + " (#id)", false},
		// A shortened agent is marked, so neither the visible prefix nor the
		// marker text can be turned back into a name.
		{"shortened label, name from its agent text", strings.Repeat("a", 84) + "-bid", strings.Repeat("a", 84) + paneLabelShortened + " (bid)", false},
		{"shortened label, name including the marker", strings.Repeat("a", 84) + paneLabelShortened + "-bid", strings.Repeat("a", 84) + paneLabelShortened + " (bid)", false},
		// The long form still names its session when only the agent was cut.
		{"shortened long form", "gc__s", strings.Repeat("a", 50) + paneLabelShortened + " (gc__s)", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := identityMatch(SessionName(tt.sess), tt.label); got != tt.want {
				t.Fatalf("identityMatch(%q, %q) = %v, want %v", tt.sess, tt.label, got, tt.want)
			}
		})
	}
}

// TestSanitizeQualifiedContract pins gc's separator contract
// (internal/agent/session_name.go at v1.5.1: '/' -> '--', '.' -> '__'; the
// encoded forms never collapse back onto each other).
func TestSanitizeQualifiedContract(t *testing.T) {
	for _, tt := range []struct{ in, want string }{
		{"gc/implementation-worker", "gc--implementation-worker"},
		{"core.control-dispatcher", "core__control-dispatcher"},
		{"rex-gascity/core.control-dispatcher", "rex-gascity--core__control-dispatcher"},
		{"a--b", "a--b"},
		{"a__b", "a__b"},
		{"", ""},
	} {
		if got := sanitizeQualified(tt.in); got != tt.want {
			t.Fatalf("sanitizeQualified(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// TestRecordlessDiscoveryByPoolIdentity is the discovery half of the short
// label: with no sidecar record and no start env, the pool label
// "<agent> (#<bead id>)" is verified against the runtime name alone.
func TestRecordlessDiscoveryByPoolIdentity(t *testing.T) {
	ctx := context.Background()
	const (
		name  = "gc__implementation-worker-mc-wisp-kvqib"
		label = "gc.implementation-worker (#mc-wisp-kvqib)"
		block = rex.BlockID("block:pool")
	)
	newFake := func() *faketest.Fake {
		f := faketest.New()
		f.Sessions = []rex.Session{{ID: "session:gc:acme", Label: "gc:acme"}}
		f.View = rex.SessionView{Windows: []rex.Window{{
			ID:     "window:rig",
			Label:  "Rig: rex-gascity",
			Blocks: []rex.Block{{ID: block, Label: label}},
		}}}
		return f
	}

	running, err := New("acme", newFake()).IsRunning(ctx, name)
	if err != nil || !running {
		t.Fatalf("IsRunning(%s) = %v, %v; want true, nil", name, running, err)
	}
	fake := newFake()
	if err := New("acme", fake).Stop(ctx, name); err != nil {
		t.Fatalf("Stop(%s): %v", name, err)
	}
	if !slices.Contains(fake.ClosedBlocks, block) {
		t.Fatalf("Stop did not close %s; closed %v", block, fake.ClosedBlocks)
	}

	// A sibling seat's short label must not be adopted.
	sibling := "gc__implementation-worker-mc-wisp-1e4r8"
	running, err = New("acme", newFake()).IsRunning(ctx, sibling)
	if err != nil {
		t.Fatalf("IsRunning(%s): %v", sibling, err)
	}
	if running {
		t.Fatalf("IsRunning(%s) = true; adopted the sibling seat's pane", sibling)
	}
}

// TestPoolSeatAndNamedSessionRenderDistinctLabels ties the labels the writer
// emits to the collision CodeRabbit flagged (label.go): a pool seat renders
// "[worker] (#seat)" while a session literally named "seat" renders
// "[worker] (seat)", so the two must never be the same string. The recordless
// discovery tests above hardcode these labels; this pins that the writer
// produces them from the env each shape carries.
func TestPoolSeatAndNamedSessionRenderDistinctLabels(t *testing.T) {
	poolLabel := paneLabelForAgent("worker-seat",
		map[string]string{"GC_RIG": "rex-gascity", "GC_AGENT": "seat", "GC_SESSION_NAME": "worker-seat"}, "worker")
	if want := "[worker] (#seat)"; poolLabel != want {
		t.Fatalf("pool pane label = %q, want %q", poolLabel, want)
	}
	namedLabel := paneLabelForAgent("seat",
		map[string]string{"GC_RIG": "rex-gascity", "GC_ALIAS": "worker", "GC_SESSION_NAME": "seat"}, "worker")
	if want := "[worker] (seat)"; namedLabel != want {
		t.Fatalf("named pane label = %q, want %q", namedLabel, want)
	}
	if poolLabel == namedLabel {
		t.Fatalf("pool seat and named session render the same label %q", poolLabel)
	}
}

// TestLegacyPaneLabelForIsFullSession pins the pre-GC_TEMPLATE rule: the old
// label is always "<agent> (<GC_SESSION_NAME>)", never the pool short form the
// current rule may derive from the same env (CodeRabbit review, PR 19).
func TestLegacyPaneLabelForIsFullSession(t *testing.T) {
	tests := []struct {
		name string
		sess SessionName
		env  map[string]string
		want string
	}{
		{
			// Pool-short derivable now, but the pre-change provider wrote the
			// full session name.
			name: "short form not applied",
			sess: "pi-1",
			env: map[string]string{"GC_RIG": "rig", "GC_AGENT": "worker",
				"GC_SESSION_NAME": "worker-worker", "GC_TEMPLATE": "rig/new"},
			want: "worker (worker-worker)",
		},
		{
			name: "alias drives the agent",
			sess: "rig--worker",
			env: map[string]string{"GC_RIG": "rig", "GC_ALIAS": "w",
				"GC_AGENT": "worker", "GC_SESSION_NAME": "rig--worker"},
			want: "w (rig--worker)",
		},
		{
			name: "town session keeps the name-derived label",
			sess: "mayor",
			env:  map[string]string{"GC_SESSION_NAME": "mayor"},
			want: "mayor",
		},
		{
			// Pre-change truncation cut the agent without the shortened marker.
			// A live pane started before the upgrade carries that unmarked cut,
			// so the legacy label must reproduce it: 100-rune agent and 40-rune
			// session give an 85-rune agent plus the tail.
			name: "pre-change unmarked truncation",
			sess: SessionName(strings.Repeat("a", 100) + "-" + strings.Repeat("s", 40)),
			env: map[string]string{
				"GC_RIG":          "rig",
				"GC_ALIAS":        strings.Repeat("a", 100),
				"GC_SESSION_NAME": strings.Repeat("s", 40),
			},
			want: strings.Repeat("a", 85) + " (" + strings.Repeat("s", 40) + ")",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := legacyPaneLabelFor(tc.sess, tc.env); got != tc.want {
				t.Errorf("legacyPaneLabelFor = %q, want %q", got, tc.want)
			}
		})
	}
}
