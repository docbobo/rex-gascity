package provider_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"

	"github.com/docbobo/rex-gascity/internal/provider"
	"github.com/docbobo/rex-gascity/internal/rex"
	"github.com/docbobo/rex-gascity/internal/rex/faketest"
)

func TestPeekLines(t *testing.T) {
	fake, p := teardownStarted(t, teardownDog)
	fake.Formats = map[rex.BlockID]string{"block:dog-1": "a\nb\nc\nd\n"}
	ctx := context.Background()
	for _, tc := range []struct {
		lines int
		want  string
	}{
		{2, "c\nd\n"},
		{0, "a\nb\nc\nd\n"},
		{-1, "a\nb\nc\nd\n"},
		{10, "a\nb\nc\nd\n"},
	} {
		got, err := p.PeekLines(ctx, teardownDog, tc.lines)
		if err != nil || got != tc.want {
			t.Errorf("PeekLines(%d) = %q, %v; want %q", tc.lines, got, err, tc.want)
		}
	}
	if got, _ := p.Peek(ctx, teardownDog); got != "a\nb\nc\nd\n" {
		t.Errorf("Peek = %q", got)
	}
}

func TestPeekLinesVanishedPane(t *testing.T) {
	fake, p := teardownStarted(t, teardownDog)
	fake.FormatErr = fmt.Errorf("rex block.method: %w", rex.ErrNotFound)
	got, err := p.PeekLines(context.Background(), teardownDog, 2)
	if err != nil || got != "" {
		t.Errorf("PeekLines after pane vanished = %q, %v; want empty, nil", got, err)
	}
}

func TestListRunning(t *testing.T) {
	fake := faketest.New()
	p := provider.New("acme", fake)
	for _, n := range []string{"Indigo--dog-2", "Indigo--dog-1", "Indigo--dead", "Other--cat"} {
		teardownStart(t, p, n)
	}
	teardownExit(fake, "block:dead")

	got, err := p.ListRunning(context.Background(), "")
	if want := []string{"Indigo--dog-1", "Indigo--dog-2", "Other--cat"}; err != nil || !slices.Equal(got, want) {
		t.Errorf("all = %v, %v; want %v", got, err, want)
	}
	got, err = p.ListRunning(context.Background(), "Indigo--")
	if want := []string{"Indigo--dog-1", "Indigo--dog-2"}; err != nil || !slices.Equal(got, want) {
		t.Errorf("prefix = %v, %v; want %v", got, err, want)
	}
}

func TestClearScrollback(t *testing.T) {
	ctx := context.Background()
	fake, p := teardownStarted(t, teardownDog)
	if err := p.ClearScrollback(ctx, "nobody"); err != nil {
		t.Errorf("missing: %v", err)
	}
	if err := p.ClearScrollback(ctx, teardownDog); err != nil {
		t.Fatalf("ClearScrollback: %v", err)
	}
	found := false
	for _, c := range fake.BlockCalls {
		if c.Op == "clear" && c.BlockID == "block:dog-1" {
			found = true
		}
	}
	if !found {
		t.Error("BlockClear not called")
	}
}

func TestFileMetaStoreTwoInstancesDifferentKeys(t *testing.T) {
	dir := t.TempDir()
	a, b := provider.NewFileMetaStore(dir), provider.NewFileMetaStore(dir)
	if err := a.Set("s", "k1", "v1"); err != nil {
		t.Fatal(err)
	}
	if err := b.Set("s", "k2", "v2"); err != nil {
		t.Fatal(err)
	}
	for k, want := range map[string]string{"k1": "v1", "k2": "v2"} {
		if got, ok, err := a.Get("s", k); !ok || err != nil || got != want {
			t.Errorf("%s = %q, %v, %v", k, got, ok, err)
		}
	}
}

func TestStartWithSharedStateDirLock(t *testing.T) {
	dir := t.TempDir()
	fake := faketest.New()
	p1 := provider.New("acme", fake, provider.WithNamespace(provider.NewNamespaceForDirs(dir)))
	p2 := provider.New("acme", fake, provider.WithNamespace(provider.NewNamespaceForDirs(dir)))
	var wg sync.WaitGroup
	for i, pr := range []*provider.Provider{p1, p2} {
		name := []string{"Indigo--a", "Indigo--b"}[i]
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := pr.Start(context.Background(), name, teardownCfg); err != nil {
				t.Errorf("Start %s: %v", name, err)
			}
		}()
	}
	wg.Wait()
	for _, n := range []string{"Indigo--a", "Indigo--b"} {
		if _, ok := p1.Binding(n); !ok {
			t.Errorf("no binding for %s", n)
		}
	}
	if len(fake.CreatedSessions) != 1 {
		t.Errorf("sessions created = %d, want 1", len(fake.CreatedSessions))
	}
}

// startNewRace runs StartNew for names[i] on its own Provider, all sharing
// one state dir and one fake, the way concurrent dispatcher processes share
// one Rex server. It returns the errors in name order.
func startNewRace(t *testing.T, names ...string) (*faketest.Fake, *provider.Provider, []error) {
	t.Helper()
	dir := t.TempDir()
	fake := faketest.New()
	type outcome struct {
		i   int
		err error
	}
	done := make(chan outcome, len(names))
	release := make(chan struct{})
	for i, name := range names {
		pr := provider.New("acme", fake, provider.WithNamespace(provider.NewNamespaceForDirs(dir)))
		go func() {
			<-release
			done <- outcome{i, pr.StartNew(context.Background(), name, teardownCfg)}
		}()
	}
	close(release)
	errs := make([]error, len(names))
	for range names {
		o := <-done
		errs[o.i] = o.err
	}
	return fake, provider.New("acme", fake, provider.WithNamespace(provider.NewNamespaceForDirs(dir))), errs
}

func TestStartNewConcurrentSameNameOneWins(t *testing.T) {
	for range 20 {
		fake, p, errs := startNewRace(t, "Indigo--a", "Indigo--a")
		ok, already := 0, 0
		for _, err := range errs {
			switch {
			case err == nil:
				ok++
			case errors.Is(err, provider.ErrAlreadyRunning):
				already++
			default:
				t.Fatalf("StartNew: unexpected error %v", err)
			}
		}
		if ok != 1 || already != 1 {
			t.Fatalf("succeeded=%d already-running=%d, want 1 and 1", ok, already)
		}
		if n := len(fake.CreatedWindows) + len(fake.CreatedSplits); n != 1 {
			t.Fatalf("panes launched = %d, want 1", n)
		}
		err := p.StartNew(context.Background(), "Indigo--a", teardownCfg)
		if !errors.Is(err, provider.ErrAlreadyRunning) {
			t.Fatalf("sequential StartNew = %v, want ErrAlreadyRunning", err)
		}
	}
}

func TestStartNewConcurrentDistinctNamesBothStart(t *testing.T) {
	fake, p, errs := startNewRace(t, "Indigo--a", "Indigo--b")
	for i, err := range errs {
		if err != nil {
			t.Errorf("StartNew #%d: %v", i, err)
		}
	}
	for _, n := range []string{"Indigo--a", "Indigo--b"} {
		if _, ok := p.Binding(n); !ok {
			t.Errorf("no binding for %s", n)
		}
	}
	if len(fake.CreatedSessions) != 1 {
		t.Errorf("sessions created = %d, want 1", len(fake.CreatedSessions))
	}
}

func TestStartNewRelaunchesDeadPane(t *testing.T) {
	ctx := context.Background()
	fake := faketest.New()
	p := provider.New("acme", fake)
	if err := p.StartNew(ctx, teardownDog, teardownCfg); err != nil {
		t.Fatal(err)
	}
	teardownExit(fake, "block:dog-1")
	if err := p.StartNew(ctx, teardownDog, teardownCfg); err != nil {
		t.Errorf("StartNew after exit = %v, want relaunch", err)
	}
}
