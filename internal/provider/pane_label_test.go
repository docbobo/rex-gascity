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
			want: "gc.implementation-worker (gc__implementation-worker-mc-wisp-fvwrce)",
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
			want: "mc-wisp-mf93ir (gc__requirements-planner-mc-wisp-mf93ir)",
		},
		{
			name: "beads actor suffix fallback",
			sess: "gc__task-decomposer-mc-wisp-aaaa",
			env: map[string]string{
				"GC_RIG":          "rex-gascity",
				"BEADS_ACTOR":     "rex-gascity/gc.task-decomposer",
				"GC_SESSION_NAME": "gc__task-decomposer-mc-wisp-aaaa",
			},
			want: "gc.task-decomposer (gc__task-decomposer-mc-wisp-aaaa)",
		},
		{
			name: "beads actor without slash used whole",
			sess: "gc__mayor-mc-wisp-bbbb",
			env: map[string]string{
				"GC_RIG":          "rex-gascity",
				"BEADS_ACTOR":     "mayor",
				"GC_SESSION_NAME": "gc__mayor-mc-wisp-bbbb",
			},
			want: "mayor (gc__mayor-mc-wisp-bbbb)",
		},
		{
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
			want: "mayor",
		},
		{
			name: "rigless pool name with rig set gets new label",
			sess: "gc__implementation-worker-mc-wisp-fvwrce",
			env: map[string]string{
				"GC_RIG":          "rex-gascity",
				"GC_AGENT":        "mc-wisp-fvwrce",
				"GC_SESSION_NAME": "gc__implementation-worker-mc-wisp-fvwrce",
			},
			want: "mc-wisp-fvwrce (gc__implementation-worker-mc-wisp-fvwrce)",
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

	// An over-long short form keeps its parenthesised bead id for display
	// instead of falling through to the long form (which cannot fit either).
	// Cutting the agent's tail costs the label its identity: the bead id alone
	// fits two names, so identityMatch must refuse it rather than guess.
	t.Run("verified short form over the limit keeps the bead id but names nobody", func(t *testing.T) {
		agent := strings.Repeat("a", 60)
		bead := strings.Repeat("9", 78)
		session := agent + "-" + bead
		env := map[string]string{
			"GC_RIG":          "rex-gascity",
			"GC_TEMPLATE":     "rex-gascity/" + agent,
			"GC_AGENT":        bead,
			"GC_SESSION_NAME": session,
		}
		if short := agent + " (" + bead + ")"; utf8.RuneCountInString(short) <= maxPaneLabelRunes {
			t.Fatalf("fixture short form is %d runes, want over %d", utf8.RuneCountInString(short), maxPaneLabelRunes)
		}
		got := paneLabelFor(SessionName(session), env)
		if n := utf8.RuneCountInString(got); n > maxPaneLabelRunes {
			t.Fatalf("label has %d runes, want <= %d: %q", n, maxPaneLabelRunes, got)
		}
		if !strings.HasSuffix(got, " ("+bead+")") {
			t.Fatalf("label %q lost the parenthesised bead id %q", got, bead)
		}
		if again := paneLabelFor(SessionName(session), env); again != got {
			t.Fatalf("label is not deterministic: %q != %q", got, again)
		}
		if identityMatch(SessionName(session), got) {
			t.Fatalf("identityMatch(%q, %q) = true; a shortened agent names nobody", session, got)
		}
		// The name that shares the visible prefix and the bead id differs only
		// where the cut happened, so the label cannot tell the two apart.
		if sibling := SessionName(agent + "-extra-" + bead); identityMatch(sibling, got) {
			t.Fatalf("label %q was adopted by %q", got, sibling)
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
		want := "agent (" + shortSession + ")"
		if got := paneLabelFor("gc__short", env); got != want {
			t.Fatalf("paneLabelFor = %q, want %q", got, want)
		}
	})
}

// TestPaneLabelOverflowIdentity is the boundary proof for R2-F1: a verified
// short form at exactly the 128-rune limit is not shortened and still names
// the session through identityMatch, while one rune over it must shorten the
// agent and then names nobody. A shortened label fits a whole family of names,
// so accepting it would let a recordless IsRunning/Stop pick the wrong pane.
func TestPaneLabelOverflowIdentity(t *testing.T) {
	tests := []struct {
		name     string
		agent    int
		bead     int
		wantRune int
		names    bool
	}{
		{"exact limit", 60, 65, 128, true},
		{"one over the limit", 60, 66, 129, false},
		{"far over the limit", 100, 40, 143, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agent := strings.Repeat("a", tt.agent)
			bead := strings.Repeat("9", tt.bead)
			session := agent + "-" + bead
			if n := utf8.RuneCountInString(agent + " (" + bead + ")"); n != tt.wantRune {
				t.Fatalf("fixture short form is %d runes, want %d", n, tt.wantRune)
			}
			env := map[string]string{
				"GC_RIG":          "rex-gascity",
				"GC_TEMPLATE":     "rex-gascity/" + agent,
				"GC_AGENT":        bead,
				"GC_SESSION_NAME": session,
			}
			got := paneLabelFor(SessionName(session), env)
			if n := utf8.RuneCountInString(got); n > maxPaneLabelRunes {
				t.Fatalf("label has %d runes, want <= %d: %q", n, maxPaneLabelRunes, got)
			}
			if !strings.HasSuffix(got, " ("+bead+")") {
				t.Fatalf("label %q lost the parenthesised bead id %q", got, bead)
			}
			if names := identityMatch(SessionName(session), got); names != tt.names {
				t.Fatalf("identityMatch row %q = %v, want %v", tt.name, names, tt.names)
			}
			if sibling := SessionName(agent + "-" + strings.Repeat("8", tt.bead)); identityMatch(sibling, got) {
				t.Fatalf("label %q was adopted by a sibling seat %q", got, sibling)
			}
		})
	}
}

// TestRecordlessClampedShortFormIsNotAnIdentity starts an overflowing pool
// seat and drops the sidecar record, so only the shortened label is left. The
// scan must not adopt it: this session's own pane, and a sibling whose name
// shares the visible prefix and the bead id, are both refused, because a label
// that fits two sessions is no evidence for either. Leaving the pane behind
// beats stopping the wrong one.
func TestRecordlessClampedShortFormIsNotAnIdentity(t *testing.T) {
	ctx := context.Background()
	agent := strings.Repeat("a", 100)
	bead := strings.Repeat("9", 40)
	session := agent + "-" + bead
	env := map[string]string{
		"GC_RIG":          "rex-gascity",
		"GC_TEMPLATE":     "rex-gascity/" + agent,
		"GC_AGENT":        bead,
		"GC_SESSION_NAME": session,
	}
	fake := faketest.New()
	if err := New("acme", fake).Start(ctx, session, Config{Argv: []string{"pi"}, Env: env}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	label := paneLabelFor(SessionName(session), env)
	block := rex.BlockID("block:" + label)
	// The shortened agent is marked, so the label itself shows that it lost
	// the tail instead of reading like a short agent.
	if !strings.HasSuffix(label, " ("+bead+")") || !strings.Contains(label, paneLabelShortened) {
		t.Fatalf("label %q does not mark its shortened agent", label)
	}
	// Names built from the visible agent text are exactly what must not match.
	short := label[:strings.LastIndex(label, " (")]
	short = strings.TrimSuffix(short, paneLabelShortened)
	names := []string{
		session,
		agent + "-extra-" + bead,
		short + "-" + bead,
		short + paneLabelShortened + "-" + bead,
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
		want    = "mc-wisp-fvwrce (" + session + ")"
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
// launched before GC_TEMPLATE drove the agent prefix carries
// "<GC_AGENT> (<GC_SESSION_NAME>)". With GC_TEMPLATE set, the new hint differs
// by the agent (and the start name differs from GC_SESSION_NAME), so a
// recordless start must still find the old pane by its session suffix instead
// of launching a duplicate.
func TestPaneLabelRediscoveryAfterTemplateChange(t *testing.T) {
	ctx := context.Background()
	const (
		start    = "pi-mc-wisp-fvwrce"
		session  = "gc__implementation-worker-mc-wisp-fvwrce"
		oldLabel = "mc-wisp-fvwrce (" + session + ")"
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
			block: {Foreground: &rex.Process{PID: 42, Name: "pi"}},
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
				block: {Foreground: &rex.Process{PID: 42, Name: "pi"}},
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
			block: {Foreground: &rex.Process{PID: 42, Name: "pi"}},
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
// "<GC_TEMPLATE basename> (<session bead id>)", verifiable against the runtime
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
			want: "gc.implementation-worker (mc-wisp-kvqib)",
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
			want: "gc.implementation-worker (mc-wisp-1e4r8)",
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
			want: "gc.run-operator (mc-wisp-jwlvf)",
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
			want: "core.control-dispatcher (core__control-dispatcher-mc-wisp-vmecx)",
		},
		{
			name: "town-level unchanged",
			sess: "mayor",
			env: map[string]string{
				"GC_TEMPLATE":     "mayor",
				"GC_SESSION_NAME": "mayor",
			},
			want: "mayor",
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
	const want = "worker (gc__other-mc-wisp-aaa)"
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
		{"named form", "core__control-dispatcher-mc-wisp-vmecx", "core.control-dispatcher (core__control-dispatcher-mc-wisp-vmecx)", true},
		{"pool short form", "gc__implementation-worker-mc-wisp-kvqib", "gc.implementation-worker (mc-wisp-kvqib)", true},
		// The second branch is the concatenation, not the literal suffix: a
		// long-form-shaped label names a session only when Sanitize(agent)+"-"+
		// suffix is the whole name.
		{"long-form-shaped label matched by concatenation", "legacy-agent-a--b-1", "legacy-agent (a--b-1)", true},
		{"concat branch does not accept a different agent", "other-agent-a--b-1", "legacy-agent (a--b-1)", false},
		{"another session's bead id same tab", "gc__implementation-worker-mc-wisp-1e4r8", "gc.implementation-worker (mc-wisp-kvqib)", false},
		{"bare label", "rex-gascity--worker", "worker", false},
		{"suffix satisfies neither branch", "gc__implementation-worker-mc-wisp-1e4r8", "other.agent (mc-wisp-1e4r8)", false},
		{"empty suffix", "rex-gascity--worker", "worker ()", false},
		{"prefix only", "rex-gascity--worker", "worker (nonsense)", false},
		// fitPaneLabel cuts the agent to fit 128 runes. That label names nobody:
		// a prefix plus the bead id fits other names too, so it is not evidence.
		{"clamped short form", strings.Repeat("a", 100) + "-" + strings.Repeat("9", 40), strings.Repeat("a", 85) + " (" + strings.Repeat("9", 40) + ")", false},
		{"clamped short form, sibling bead id", strings.Repeat("a", 100) + "-" + strings.Repeat("9", 40), strings.Repeat("a", 85) + " (" + strings.Repeat("8", 40) + ")", false},
		// A label at exactly the limit was not cut, so only the whole name
		// counts: a name sharing its ends but not its middle is another
		// session.
		{"128-rune label, exact name", strings.Repeat("a", 123) + "-id", strings.Repeat("a", 123) + " (id)", true},
		{"128-rune label, longer name with the same ends", strings.Repeat("a", 123) + "-extra-id", strings.Repeat("a", 123) + " (id)", false},
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
// "<agent> (<bead id>)" is verified against the runtime name alone.
func TestRecordlessDiscoveryByPoolIdentity(t *testing.T) {
	ctx := context.Background()
	const (
		name  = "gc__implementation-worker-mc-wisp-kvqib"
		label = "gc.implementation-worker (mc-wisp-kvqib)"
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
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := legacyPaneLabelFor(tc.sess, tc.env); got != tc.want {
				t.Errorf("legacyPaneLabelFor = %q, want %q", got, tc.want)
			}
		})
	}
}
