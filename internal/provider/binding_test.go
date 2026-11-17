package provider

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/docbobo/rex-gas-city/internal/rex"
	"github.com/docbobo/rex-gas-city/internal/rex/faketest"
)

const testLabel = "gc:acme"

func newTestBinder(t *testing.T, windows ...rex.Window) (*Binder, *MemoryStore, *faketest.Fake) {
	t.Helper()
	fake := faketest.New()
	fake.Sessions = []rex.Session{{ID: "session:acme", Label: testLabel}}
	fake.View = rex.SessionView{Label: testLabel, Windows: windows}
	store := NewMemoryStore()
	return NewBinder(fake, store, testLabel), store, fake
}

func indigoWindow() rex.Window {
	return rex.Window{ID: "window:Indigo", Label: "Indigo",
		Blocks: []rex.Block{{ID: "block:dog-1", Label: "dog-1"}}}
}

func indigoBinding() Binding {
	return Binding{Name: "Indigo--dog-1", SessionID: "session:acme", SessionLabel: testLabel,
		WindowID: "window:Indigo", BlockID: "block:dog-1"}
}

func TestResolveCacheHit(t *testing.T) {
	b, store, fake := newTestBinder(t, indigoWindow())
	want := indigoBinding()
	store.Save(want)

	got, ok, err := b.Resolve(context.Background(), want.Name)
	if err != nil || !ok || !reflect.DeepEqual(got, want) {
		t.Fatalf("Resolve = %+v, %v, %v", got, ok, err)
	}
	if fake.ListSessionsCalls != 0 || fake.SessionViewCalls != 1 {
		t.Errorf("calls: list=%d view=%d", fake.ListSessionsCalls, fake.SessionViewCalls)
	}
}

func TestResolveScanFallback(t *testing.T) {
	b, store, _ := newTestBinder(t, indigoWindow())

	got, ok, err := b.Resolve(context.Background(), "Indigo--dog-1")
	if err != nil || !ok || !reflect.DeepEqual(got, indigoBinding()) {
		t.Fatalf("Resolve = %+v, %v, %v", got, ok, err)
	}
	if saved, ok := store.Load("Indigo--dog-1"); !ok || !reflect.DeepEqual(saved, got) {
		t.Errorf("saved = %+v, %v", saved, ok)
	}
}

func TestResolveNotFound(t *testing.T) {
	b, _, _ := newTestBinder(t, indigoWindow())
	_, ok, err := b.Resolve(context.Background(), "Indigo--cat")
	if ok || err != nil {
		t.Fatalf("Resolve = %v, %v", ok, err)
	}
}

func TestResolveStaleBinding(t *testing.T) {
	b, store, _ := newTestBinder(t, rex.Window{ID: "window:Indigo", Label: "Indigo",
		Blocks: []rex.Block{{ID: "block:dog-1b", Label: "dog-1"}}})
	store.Save(indigoBinding())

	got, ok, err := b.Resolve(context.Background(), "Indigo--dog-1")
	if err != nil || !ok || got.BlockID != "block:dog-1b" {
		t.Fatalf("Resolve = %+v, %v, %v", got, ok, err)
	}
	if saved, _ := store.Load("Indigo--dog-1"); saved.BlockID != "block:dog-1b" {
		t.Errorf("store not refreshed: %+v", saved)
	}
}

func TestResolveStaleBindingGone(t *testing.T) {
	b, store, _ := newTestBinder(t)
	store.Save(indigoBinding())

	_, ok, err := b.Resolve(context.Background(), "Indigo--dog-1")
	if ok || err != nil {
		t.Fatalf("Resolve = %v, %v", ok, err)
	}
	if _, ok := store.Load("Indigo--dog-1"); ok {
		t.Error("stale binding not deleted")
	}
}

func TestResolveTransportErrorKeepsBinding(t *testing.T) {
	b, store, fake := newTestBinder(t, indigoWindow())
	store.Save(indigoBinding())
	boom := errors.New("boom")
	fake.SessionViewErr = boom

	_, ok, err := b.Resolve(context.Background(), "Indigo--dog-1")
	if ok || !errors.Is(err, boom) {
		t.Fatalf("Resolve = %v, %v", ok, err)
	}
	if _, ok := store.Load("Indigo--dog-1"); !ok {
		t.Error("binding was cleared on transport error")
	}
}

func TestResolveStaleSessionIDRescansOnNotFound(t *testing.T) {
	b, store, fake := newTestBinder(t, indigoWindow())
	fake.Sessions = []rex.Session{{ID: "session:new", Label: testLabel}}
	fake.NotFoundSessions = map[rex.SessionID]bool{"session:old": true}
	old := indigoBinding()
	old.SessionID = "session:old"
	store.Save(old)

	got, ok, err := b.Resolve(context.Background(), "Indigo--dog-1")
	if err != nil || !ok || got.BlockID != "block:dog-1" || got.SessionID != "session:new" {
		t.Fatalf("Resolve = %+v, %v, %v", got, ok, err)
	}
	if saved, _ := store.Load("Indigo--dog-1"); saved.SessionID != "session:new" {
		t.Errorf("store not refreshed: %+v", saved)
	}
}

func TestResolveForeignSessionLabelIgnored(t *testing.T) {
	b, store, fake := newTestBinder(t, indigoWindow())
	foreign := indigoBinding()
	foreign.SessionID = "session:other"
	foreign.SessionLabel = "gc:other"
	store.Save(foreign)

	got, ok, err := b.Resolve(context.Background(), "Indigo--dog-1")
	if err != nil || !ok || got.SessionID != "session:acme" || got.SessionLabel != testLabel {
		t.Fatalf("Resolve = %+v, %v, %v", got, ok, err)
	}
	if fake.ListSessionsCalls != 1 {
		t.Errorf("ListSessions calls = %d, want a rescan", fake.ListSessionsCalls)
	}
	if saved, _ := store.Load("Indigo--dog-1"); saved.SessionLabel != testLabel {
		t.Errorf("store not refreshed: %+v", saved)
	}
}

func TestResolveTownAgent(t *testing.T) {
	b, _, _ := newTestBinder(t, rex.Window{ID: "window:__town", Label: "__town",
		Blocks: []rex.Block{{ID: "block:mayor", Label: "mayor"}}})

	got, ok, err := b.Resolve(context.Background(), "mayor")
	if err != nil || !ok || got.WindowID != "window:__town" || got.BlockID != "block:mayor" {
		t.Fatalf("Resolve = %+v, %v, %v", got, ok, err)
	}
}
