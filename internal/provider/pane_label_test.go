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

// TestStartPaneLabelFromProjectedEnv is the GCREX-5qel review fix for the
// launch label: pool routing rewrites BEADS_ACTOR, so the launch label, the
// discovery hint and the recorded label must all come from the projected env.
// Before the fix the launch used the pre-projection agent and a restart with
// the record lost launched a duplicate pane.
func TestStartPaneLabelFromProjectedEnv(t *testing.T) {
	ctx := context.Background()
	const start = "rex-gascity--worker"
	want := "worker (" + start + ")"
	env := map[string]string{
		"GC_RIG":          "rex-gascity",
		"GC_TEMPLATE":     "rex-gascity/worker",
		"GC_ALIAS":        "",
		"GC_AGENT":        "",
		"GC_SESSION_ID":   "mc-wisp-projected",
		"BEADS_ACTOR":     "legacy",
		"GC_SESSION_NAME": start,
	}
	if pre := paneLabelFor(start, env); pre == want {
		t.Fatalf("test env does not exercise projection: pre-projection label is already %q", pre)
	}

	fake := faketest.New()
	p := New("acme", fake)
	if err := p.Start(ctx, start, Config{Argv: []string{"pi"}, Env: env}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if got := fake.CreatedWindows[0].Spec.Label; got != want {
		t.Fatalf("launched pane label = %q, want the projected-env label %q", got, want)
	}
	stored, ok := p.Binding(start)
	if !ok {
		t.Fatal("Binding(start) = false after Start")
	}
	if stored.PaneLabel != want {
		t.Fatalf("recorded pane label = %q, want %q", stored.PaneLabel, want)
	}

	// The record is lost and the same start runs again: the hint from the
	// projected env finds the live pane, so no second pane is launched.
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
