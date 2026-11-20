package provider

import (
	"context"
	"testing"

	"github.com/docbobo/rex-gascity/internal/rex/faketest"
)

// pinCity sets the process city pin the provider reads in New, clearing the
// higher-precedence gc caller variables so the test controls the pin.
func pinCity(t *testing.T, pin string) {
	t.Helper()
	t.Setenv("GC_EXEC_CITY_NAME", "")
	t.Setenv("GC_EXEC_CITY_PATH", "")
	t.Setenv("GC_REX_CITY", pin)
}

// TestStartTownPaneUsesCityPinLabel starts a town-level name with the pin set
// to my-city while New was given a different city: the tab is the pin, not the
// city argument and not a bare agent name.
func TestStartTownPaneUsesCityPinLabel(t *testing.T) {
	pinCity(t, "my-city")
	fake := faketest.New()
	p := New("acme", fake)

	if err := p.Start(context.Background(), "mayor", Config{Argv: []string{"pi"}}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if len(fake.CreatedWindows) != 1 {
		t.Fatalf("created windows = %d, want 1", len(fake.CreatedWindows))
	}
	if got := fake.CreatedWindows[0].Label; got != "my-city" {
		t.Errorf("town tab label = %q, want %q", got, "my-city")
	}
	b, ok := p.Binding("mayor")
	if !ok {
		t.Fatal("Binding(mayor) = false")
	}
	if b.TabLabel != "my-city" {
		t.Errorf("binding tab label = %q, want %q", b.TabLabel, "my-city")
	}
}

// TestStartRigPaneUsesRigPrefix starts a rig-qualified name without GC_RIG: the
// tab is "Rig: " plus the name split.
func TestStartRigPaneUsesRigPrefix(t *testing.T) {
	pinCity(t, "")
	fake := faketest.New()
	p := New("acme", fake)

	if err := p.Start(context.Background(), "Indigo--dog-1", Config{Argv: []string{"claude"}}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if got := fake.CreatedWindows[0].Label; got != "Rig: Indigo" {
		t.Errorf("rig tab label = %q, want %q", got, "Rig: Indigo")
	}
}

// TestResolveFreshRediscoversPaneUnderPinLabel drops the store record after a
// town start and checks normal discovery finds the same pane under the pin
// label, not under an old name-derived one.
func TestResolveFreshRediscoversPaneUnderPinLabel(t *testing.T) {
	pinCity(t, "my-city")
	fake := faketest.New()
	store := NewMemoryStore()
	p := New("acme", fake, WithBindingStore(store))
	ctx := context.Background()

	if err := p.Start(ctx, "mayor", Config{Argv: []string{"pi"}}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := store.Drop("mayor"); err != nil {
		t.Fatalf("Drop: %v", err)
	}

	got, ok, err := p.binder.Resolve(ctx, ResolveRequest{Name: "mayor", Mode: ResolveFresh})
	if err != nil || !ok {
		t.Fatalf("ResolveFresh = %v, %v; want found", ok, err)
	}
	if got.TabLabel != "my-city" || got.BlockID != "block:mayor" {
		t.Errorf("rediscovered binding = %+v, want tab my-city and block:mayor", got)
	}
}

// TestResolveFreshRediscoversPaneUnderRigLabel is the rig half of the same
// check: the rediscovered pane is matched under "Rig: <rig>".
func TestResolveFreshRediscoversPaneUnderRigLabel(t *testing.T) {
	pinCity(t, "my-city")
	fake := faketest.New()
	store := NewMemoryStore()
	p := New("acme", fake, WithBindingStore(store))
	ctx := context.Background()

	if err := p.Start(ctx, "Indigo--dog-1", Config{Argv: []string{"claude"}}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := store.Drop("Indigo--dog-1"); err != nil {
		t.Fatalf("Drop: %v", err)
	}

	got, ok, err := p.binder.Resolve(ctx, ResolveRequest{Name: "Indigo--dog-1", Mode: ResolveFresh})
	if err != nil || !ok {
		t.Fatalf("ResolveFresh = %v, %v; want found", ok, err)
	}
	if got.TabLabel != "Rig: Indigo" || got.BlockID != "block:dog-1" {
		t.Errorf("rediscovered binding = %+v, want tab \"Rig: Indigo\" and block:dog-1", got)
	}
}

// TestLegacyTabLabelFor pins the pre-GCREX-72 precedence: GC_RIG beats the
// town fallback, so a short pool name is looked for in its rig tab.
func TestLegacyTabLabelFor(t *testing.T) {
	for _, tc := range []struct {
		name SessionName
		rig  string
		want string
	}{
		{"dog-1", "Indigo", "Indigo"},
		{"dog-1", "", legacyTownTab},
		{"mayor", "", legacyTownTab},
		{"Indigo--dog-1", "", "Indigo"},
		{"Indigo--dog-1", "other-rig", "other-rig"},
	} {
		if got := legacyTabLabelFor(tc.name, map[string]string{"GC_RIG": tc.rig}); got != tc.want {
			t.Errorf("legacyTabLabelFor(%q, GC_RIG=%q) = %q, want %q", tc.name, tc.rig, got, tc.want)
		}
	}
}
