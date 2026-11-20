package provider_test

import (
	"context"
	"slices"
	"testing"

	"github.com/docbobo/rex-gascity/internal/provider"
	"github.com/docbobo/rex-gascity/internal/rex"
	"github.com/docbobo/rex-gascity/internal/rex/faketest"
)

const (
	aliasRig      = "my-project"
	aliasTown     = aliasRig
	rigTab        = "Rig: " + aliasRig
	wispStart     = "pi-mc-wisp-ybq1"
	wispInstance  = "my-project--pi-mc-wisp-ybq1"
	wispTemplate  = "my-project--pi"
	cdStart       = "core__control-dispatcher-mc-wisp-6znw"
	cdCanonical   = "my-project--core__control-dispatcher"
	cdInstance    = "my-project--core__control-dispatcher-mc-wisp-6znw"
	wispPaneBlock = rex.BlockID("block:" + wispStart)
	cdPaneBlock   = rex.BlockID("block:" + cdStart)
	wispWindowID  = rex.WindowID("window:" + aliasRig)
)

// pinTown clears the higher-precedence gc caller variables and sets the city
// pin, so a town-level name derives aliasTown regardless of the harness env.
func pinTown(t *testing.T) {
	t.Helper()
	t.Setenv("GC_EXEC_CITY_NAME", "")
	t.Setenv("GC_EXEC_CITY_PATH", "")
	t.Setenv("GC_REX_CITY", aliasTown)
}

func wispEnv() map[string]string {
	return map[string]string{
		"GC_RIG":      aliasRig,
		"GC_TEMPLATE": aliasRig + "/pi",
		"GC_ALIAS":    "",
		"GC_AGENT":    "mc-wisp-ybq1",
	}
}

func cdEnv() map[string]string {
	return map[string]string{
		"GC_RIG":      aliasRig,
		"GC_TEMPLATE": aliasRig + "/core.control-dispatcher",
		"GC_ALIAS":    aliasRig + "/core.control-dispatcher",
		"GC_AGENT":    aliasRig + "/core.control-dispatcher",
	}
}

func TestStartDerivesTabFromGCRig(t *testing.T) {
	ctx := context.Background()
	pinTown(t)
	fake := faketest.New()
	p := provider.New("acme", fake)

	if err := p.Start(ctx, wispStart, provider.Config{Argv: []string{"pi"}, Env: wispEnv()}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if len(fake.CreatedWindows) != 1 {
		t.Fatalf("created windows = %d, want 1", len(fake.CreatedWindows))
	}
	w := fake.CreatedWindows[0]
	if w.Label != rigTab {
		t.Errorf("tab label = %q, want %q (GC_RIG groups a no-- name into its rig tab)", w.Label, rigTab)
	}
	if w.Spec.Label != wispStart {
		t.Errorf("pane label = %q, want %q (from the start name)", w.Spec.Label, wispStart)
	}

	b, ok := p.Binding(wispStart)
	if !ok {
		t.Fatal("Binding(start name) = false")
	}
	if b.TabLabel != rigTab || b.PaneLabel != wispStart {
		t.Errorf("binding labels = %q/%q, want %q/%q", b.TabLabel, b.PaneLabel, rigTab, wispStart)
	}
	for _, want := range []provider.SessionName{wispTemplate, wispInstance} {
		if !slices.Contains(b.Aliases, want) {
			t.Errorf("aliases = %v, want %q", b.Aliases, want)
		}
	}
}

// withSessionName adds the runtime session name to a start env.
func withSessionName(env map[string]string, name string) map[string]string {
	env["GC_SESSION_NAME"] = name
	return env
}

// TestStartDerivesPaneLabelFromEnv is the Start-threaded pane-label proof
// (GCREX-5qel item 2, gcrex-7n06r): one table over the four start-env shapes
// runs the same body — launched label, recorded binding, liveness, and
// record-lost rediscovery — so the label derivation is proven where Start
// wires it, not only at paneLabelFor. The pool row fails on main: main has no
// short form, so it launches "pi (<whole session name>)" there.
func TestStartDerivesPaneLabelFromEnv(t *testing.T) {
	const (
		sessName = "gc__implementation-worker-mc-wisp-fvwrce"
		projName = "rex-gascity--worker"
	)
	tests := []struct {
		name  string
		start string
		env   map[string]string
		want  string
	}{
		{
			name:  "verified pool seat short form",
			start: wispStart,
			env:   withSessionName(wispEnv(), wispStart),
			want:  "pi (mc-wisp-ybq1)",
		},
		{
			name:  "aliased named session keeps the whole name",
			start: cdStart,
			env:   withSessionName(cdEnv(), cdStart),
			want:  "core.control-dispatcher (" + cdStart + ")",
		},
		{
			// Pool routing rewrites BEADS_ACTOR to GC_TEMPLATE before the
			// label is derived; the env proves no short form here.
			name:  "pool-routed projected env",
			start: projName,
			env: map[string]string{
				"GC_RIG":          "rex-gascity",
				"GC_TEMPLATE":     "rex-gascity/worker",
				"GC_ALIAS":        "",
				"GC_AGENT":        "",
				"GC_SESSION_ID":   "mc-wisp-projected",
				"BEADS_ACTOR":     "legacy",
				"GC_SESSION_NAME": projName,
			},
			want: "worker (" + projName + ")",
		},
		{
			name:  "no agent identity keeps the name-derived label",
			start: sessName,
			env:   map[string]string{"GC_RIG": aliasRig, "GC_SESSION_NAME": sessName},
			want:  sessName,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			fake := faketest.New()
			cfg := provider.Config{Argv: []string{"pi"}, Env: tt.env}
			p := provider.New("acme", fake)
			if err := p.Start(ctx, tt.start, cfg); err != nil {
				t.Fatalf("Start: %v", err)
			}
			if len(fake.CreatedWindows) != 1 {
				t.Fatalf("created windows = %d, want 1", len(fake.CreatedWindows))
			}
			if got := fake.CreatedWindows[0].Spec.Label; got != tt.want {
				t.Fatalf("pane label = %q, want %q", got, tt.want)
			}
			b, ok := p.Binding(tt.start)
			if !ok {
				t.Fatal("Binding(start) = false after Start")
			}
			if b.PaneLabel != tt.want {
				t.Fatalf("binding pane label = %q, want %q", b.PaneLabel, tt.want)
			}
			if running, err := p.IsRunning(ctx, tt.start); err != nil || !running {
				t.Fatalf("IsRunning(%s) = %v, %v; want true, nil", tt.start, running, err)
			}

			// The record is lost and the same start runs again: the env-derived
			// hint finds the live pane under the same label instead of launching
			// a second one.
			rebuilt := provider.New("acme", fake)
			if err := rebuilt.Start(ctx, tt.start, cfg); err != nil {
				t.Fatalf("Start with record lost: %v", err)
			}
			if len(fake.CreatedWindows) != 1 || len(fake.CreatedSplits) != 0 {
				t.Fatalf("lost-record resume relaunched a pane: windows=%d splits=%d",
					len(fake.CreatedWindows), len(fake.CreatedSplits))
			}
		})
	}
}

func TestStartDerivesTabFromNameWithoutRig(t *testing.T) {
	ctx := context.Background()
	fake := faketest.New()
	p := provider.New("acme", fake)

	if err := p.Start(ctx, "Indigo--dog-1", provider.Config{Argv: []string{"claude"}}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if got := fake.CreatedWindows[0].Label; got != "Rig: Indigo" {
		t.Errorf("tab label = %q, want %q", got, "Rig: Indigo")
	}
	b, _ := p.Binding("Indigo--dog-1")
	if len(b.Aliases) != 0 {
		t.Errorf("aliases = %v, want none without env", b.Aliases)
	}
}

func TestIsRunningCanonicalAlias(t *testing.T) {
	ctx := context.Background()
	pinTown(t)
	fake := faketest.New()
	p := provider.New("acme", fake)

	if err := p.Start(ctx, cdStart, provider.Config{Argv: []string{"gc"}, Env: cdEnv()}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if got := fake.CreatedWindows[0].Label; got != rigTab {
		t.Errorf("tab label = %q, want %q", got, rigTab)
	}

	running, err := p.IsRunning(ctx, cdCanonical)
	if err != nil || !running {
		t.Fatalf("IsRunning(%s) = %v, %v; want true, nil", cdCanonical, running, err)
	}

	// The pane stays in the view after the program exits; liveness must flip.
	fake.Processes = map[rex.BlockID]rex.ProcessInfo{
		cdPaneBlock: {LastExit: &rex.ExitStatus{ExitCode: 0}},
	}
	running, err = p.IsRunning(ctx, cdCanonical)
	if err != nil || running {
		t.Fatalf("IsRunning(%s) after exit = %v, %v; want false, nil", cdCanonical, running, err)
	}
}

func TestListRunningAliases(t *testing.T) {
	ctx := context.Background()
	fake := faketest.New()
	p := provider.New("acme", fake)
	if err := p.Start(ctx, wispStart, provider.Config{Argv: []string{"pi"}, Env: wispEnv()}); err != nil {
		t.Fatalf("Start wisp: %v", err)
	}
	if err := p.Start(ctx, cdStart, provider.Config{Argv: []string{"gc"}, Env: cdEnv()}); err != nil {
		t.Fatalf("Start dispatcher: %v", err)
	}

	got, err := p.ListRunning(ctx, "my-project--pi-")
	if err != nil {
		t.Fatalf("ListRunning(prefix): %v", err)
	}
	if want := []string{wispInstance}; !slices.Equal(got, want) {
		t.Errorf("ListRunning(%q) = %v, want %v", "my-project--pi-", got, want)
	}

	got, err = p.ListRunning(ctx, "my-project--core__control-dispatcher-")
	if err != nil {
		t.Fatalf("ListRunning(cd prefix): %v", err)
	}
	if want := []string{cdInstance}; !slices.Equal(got, want) {
		t.Errorf("ListRunning(cd prefix) = %v, want %v", got, want)
	}

	// The empty prefix must list only primary names: gc's adoption barrier and
	// dead-session reconciler treat every returned name as a real session, so an
	// alias there would mint a phantom orphan bead.
	got, err = p.ListRunning(ctx, "")
	if err != nil {
		t.Fatalf("ListRunning(all): %v", err)
	}
	want := []string{cdStart, wispStart}
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("ListRunning(all) = %v, want %v", got, want)
	}
}

func TestIsRunningAnyInstanceAlive(t *testing.T) {
	ctx := context.Background()
	fake := faketest.New()
	p := provider.New("acme", fake)
	second := "core__control-dispatcher-mc-wisp-zzzz"
	if err := p.Start(ctx, cdStart, provider.Config{Argv: []string{"gc"}, Env: cdEnv()}); err != nil {
		t.Fatalf("Start first: %v", err)
	}
	if err := p.Start(ctx, second, provider.Config{Argv: []string{"gc"}, Env: cdEnv()}); err != nil {
		t.Fatalf("Start second: %v", err)
	}

	// The first instance exited; the second is still alive under the canonical
	// identity, so is-running must report true.
	fake.Processes = map[rex.BlockID]rex.ProcessInfo{
		cdPaneBlock:                    {LastExit: &rex.ExitStatus{ExitCode: 0}},
		rex.BlockID("block:" + second): {},
	}
	running, err := p.IsRunning(ctx, cdCanonical)
	if err != nil || !running {
		t.Fatalf("IsRunning(%s) = %v, %v; want true with a live second instance", cdCanonical, running, err)
	}
}

func TestIsRunningUnknownAliasFalse(t *testing.T) {
	fake := faketest.New()
	p := provider.New("acme", fake)
	running, err := p.IsRunning(context.Background(), "my-project--nobody")
	if err != nil || running {
		t.Fatalf("IsRunning(unknown) = %v, %v; want false, nil", running, err)
	}
}

func TestStartRigQualifiedNameKeepsTab(t *testing.T) {
	ctx := context.Background()
	fake := faketest.New()
	p := provider.New("acme", fake)
	env := map[string]string{
		"GC_RIG":      aliasRig,
		"GC_TEMPLATE": aliasRig + "/dog-1",
		"GC_ALIAS":    aliasRig + "/dog-1",
	}
	if err := p.Start(ctx, "my-project--dog-1", provider.Config{Argv: []string{"claude"}, Env: env}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if got := fake.CreatedWindows[0].Label; got != rigTab {
		t.Errorf("tab label = %q, want %q", got, rigTab)
	}
	b, _ := p.Binding("my-project--dog-1")
	if len(b.Aliases) != 0 {
		t.Errorf("aliases = %v, want none for a rig-qualified name", b.Aliases)
	}
}

func TestStopAndPeekThroughAlias(t *testing.T) {
	ctx := context.Background()
	fake := faketest.New()
	p := provider.New("acme", fake)
	if err := p.Start(ctx, cdStart, provider.Config{Argv: []string{"gc"}, Env: cdEnv()}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	fake.Formats = map[rex.BlockID]string{cdPaneBlock: "hello\n"}

	out, err := p.Peek(ctx, cdCanonical)
	if err != nil || out != "hello\n" {
		t.Fatalf("Peek(alias) = %q, %v; want %q", out, err, "hello\n")
	}
	if err := p.Stop(ctx, cdCanonical); err != nil {
		t.Fatalf("Stop(alias): %v", err)
	}
	if !slices.Contains(fake.ClosedBlocks, cdPaneBlock) {
		t.Errorf("closed = %v, want %s", fake.ClosedBlocks, cdPaneBlock)
	}
	if _, ok := p.Binding(cdCanonical); ok {
		t.Error("Binding still present after Stop(alias)")
	}
}

func TestStartRecoversLostBinding(t *testing.T) {
	ctx := context.Background()
	fake := faketest.New()
	writer := provider.New("acme", fake)
	if err := writer.Start(ctx, wispStart, provider.Config{Argv: []string{"pi"}, Env: wispEnv()}); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// A second provider has no sidecar record for the pane but the pane lives.
	// Start with the same start config must reuse it, not open a duplicate.
	reader := provider.New("acme", fake)
	if err := reader.Start(ctx, wispStart, provider.Config{Argv: []string{"pi"}, Env: wispEnv()}); err != nil {
		t.Fatalf("Start after lost binding: %v", err)
	}
	if len(fake.CreatedWindows) != 1 || len(fake.CreatedSplits) != 0 {
		t.Errorf("windows=%d splits=%d, want 1/0 (no duplicate pane)", len(fake.CreatedWindows), len(fake.CreatedSplits))
	}
}

func TestMetaAliasResolution(t *testing.T) {
	ctx := context.Background()
	fake := faketest.New()
	p := provider.New("acme", fake)
	if err := p.Start(ctx, cdStart, provider.Config{Argv: []string{"gc"}, Env: cdEnv()}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := p.SetMeta(ctx, cdStart, "GC_SESSION_ID", "mc-wisp-6znw"); err != nil {
		t.Fatalf("SetMeta: %v", err)
	}
	v, err := p.GetMeta(ctx, cdCanonical, "GC_SESSION_ID")
	if err != nil || v != "mc-wisp-6znw" {
		t.Fatalf("GetMeta(alias) = %q, %v; want mc-wisp-6znw", v, err)
	}
}

func TestIsRunningRecoversFromStaleBinding(t *testing.T) {
	ctx := context.Background()
	pinTown(t)
	fake := faketest.New()
	p := provider.New("acme", fake)
	if err := p.Start(ctx, wispStart, provider.Config{Argv: []string{"pi"}, Env: wispEnv()}); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Rex recreated the pane under the same env-derived labels but a new block
	// id; liveness must rediscover it rather than report the stale address dead.
	fake.View = rex.SessionView{Windows: []rex.Window{{
		ID:     wispWindowID,
		Label:  rigTab,
		Blocks: []rex.Block{{ID: "block:recreated", Label: wispStart}},
	}}}

	running, err := p.IsRunning(ctx, wispInstance)
	if err != nil || !running {
		t.Fatalf("IsRunning(%s) after pane recreation = %v, %v; want true, nil", wispInstance, running, err)
	}
}

// rebuiltWispFake returns a fake whose session view still holds the wisp pane
// recorded by a first provider, as a restarted process would find it.
func rebuiltWispFake() *faketest.Fake {
	fake := faketest.New()
	fake.Sessions = []rex.Session{{ID: "session:gc:acme", Label: "gc:acme"}}
	fake.View = rex.SessionView{Windows: []rex.Window{{
		ID:     wispWindowID,
		Label:  rigTab,
		Blocks: []rex.Block{{ID: wispPaneBlock, Label: wispStart}},
	}}}
	return fake
}

func TestAliasResolutionThroughFileStore(t *testing.T) {
	ctx := context.Background()
	pinTown(t)
	dir := t.TempDir()

	writer := provider.New("acme", faketest.New(), provider.WithBindingStore(provider.NewFileStore(dir)))
	if err := writer.Start(ctx, wispStart, provider.Config{Argv: []string{"pi"}, Env: wispEnv()}); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// A second provider reads the aliases only from disk.
	reader := provider.New("acme", rebuiltWispFake(), provider.WithBindingStore(provider.NewFileStore(dir)))

	b, ok := reader.Binding(wispInstance)
	if !ok {
		t.Fatal("Binding(instance alias) = false after reload")
	}
	if b.Name != wispStart || b.TabLabel != rigTab || b.PaneLabel != wispStart {
		t.Errorf("reloaded binding = %+v", b)
	}
	if want := []provider.SessionName{wispTemplate, wispInstance}; !slices.Equal(b.Aliases, want) {
		t.Errorf("reloaded aliases = %v, want %v", b.Aliases, want)
	}

	running, err := reader.IsRunning(ctx, wispInstance)
	if err != nil || !running {
		t.Fatalf("IsRunning(instance alias) after reload = %v, %v; want true, nil", running, err)
	}
}

// TestStartRoutesUnaliasedPoolToTemplate is the GCREX-32 regression: a pool
// seat must launch with GC_ALIAS empty (so gc records the claim under the
// per-seat session bead id, not a shared template string) while routing its
// work query as the pool template through BEADS_ACTOR.
func TestStartRoutesUnaliasedPoolToTemplate(t *testing.T) {
	ctx := context.Background()
	fake := faketest.New()
	p := provider.New("acme", fake)
	env := wispEnv()
	env["GC_SESSION_ID"] = "mc-wisp-ybq1"
	env["GC_INSTANCE_TOKEN"] = "tok-1"
	if err := p.Start(ctx, wispStart, provider.Config{Argv: []string{"pi"}, Env: env}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	cmd := fake.CreatedWindows[0].Spec.Command
	want := []string{"env", "-u", "GC_ALIAS", "BEADS_ACTOR=" + aliasRig + "/pi", "GC_AGENT=mc-wisp-ybq1",
		"GC_INSTANCE_TOKEN=tok-1", "GC_RIG=" + aliasRig, "GC_SESSION_ID=mc-wisp-ybq1",
		"GC_TEMPLATE=" + aliasRig + "/pi", "pi"}
	if !slices.Equal(cmd, want) {
		t.Errorf("command = %v, want %v", cmd, want)
	}
	for key, w := range map[string]string{
		"GC_ALIAS": "", "GC_AGENT": "mc-wisp-ybq1", "GC_SESSION_ID": "mc-wisp-ybq1", "GC_INSTANCE_TOKEN": "tok-1",
	} {
		if got, _ := p.GetMeta(ctx, wispStart, key); got != w {
			t.Errorf("meta %s = %q, want %q", key, got, w)
		}
	}
	if env["GC_ALIAS"] != "" {
		t.Errorf("caller env mutated: %v", env)
	}
}
