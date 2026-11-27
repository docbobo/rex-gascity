package provider_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/docbobo/rex-gascity/internal/provider"
	"github.com/docbobo/rex-gascity/internal/rex"
	"github.com/docbobo/rex-gascity/internal/rex/faketest"
)

// TestStartupDialogMatchRecorded pins #205 AC5: a withheld seed's marker names
// the dialog by its stable name, and the timing diagnostic keeps the name and
// the screen the decision read, capped at 4096 bytes from the end.
func TestStartupDialogMatchRecorded(t *testing.T) {
	stateDir := t.TempDir()
	fake := faketest.New()
	fake.ProgramStatuses = idleProgramStatuses()
	filler := strings.Repeat(strings.Repeat("y", 99)+"\n", 150)
	fake.Formats = map[rex.BlockID]string{dogBlock: filler + trustDialogScreen}
	p := startupProvider(fake, provider.WithNamespace(provider.NewNamespaceForDirs(stateDir)), provider.WithStartupDialogClear(50*time.Millisecond))

	if err := p.Start(context.Background(), dogName, provider.Config{Argv: []string{"pi"}, Nudge: "hello agent"}); err != nil {
		t.Fatalf("Start: %v", err)
	}

	want := "withheld: the pane shows a startup dialog (" + trustName + ")"
	if got, _ := p.GetMeta(context.Background(), dogName, provider.StartupDeliveryUnconfirmedMetaKey); got != want {
		t.Errorf("marker = %q, want %q", got, want)
	}
	timing := readStartupTiming(t, stateDir, dogName)
	if timing["readiness_outcome"] != "dialog" {
		t.Errorf("readiness_outcome = %v, want dialog", timing["readiness_outcome"])
	}
	if timing["dialog"] != trustName {
		t.Errorf("dialog = %v, want %q", timing["dialog"], trustName)
	}
	screen, _ := timing["dialog_screen"].(string)
	if len(screen) != 4096 || !strings.HasSuffix(screen, trustDialogScreen) {
		t.Errorf("dialog_screen: %d bytes, suffix ok %v; want the last 4096 bytes ending in the dialog",
			len(screen), strings.HasSuffix(screen, trustDialogScreen))
	}
	if _, ok := timing["dialog_gate"]; ok {
		t.Errorf("dialog_gate = %v, want absent for a readiness sighting", timing["dialog_gate"])
	}
}

var seedConfig = provider.Config{Argv: []string{"pi"}, Nudge: "hello agent"}

// TestStartupSeedDeliveredOnceAfterDialogClears is #205's regression (AC1): a
// dialog shows at readiness and clears inside the clear window. The seed is
// written once and submitted once, after the dialog is gone, the marker is
// cleared, and the wait ends dialog-cleared.
func TestStartupSeedDeliveredOnceAfterDialogClears(t *testing.T) {
	t.Run("status mode", func(t *testing.T) {
		stateDir := t.TempDir()
		fake := faketest.New()
		turnStarts(fake)
		log := &screenLog{}
		reads := 0
		fake.FormatFunc = func(rex.BlockID) string {
			reads++
			if reads <= 2 {
				log.served(true)
				return trustDialogScreen
			}
			log.served(false)
			return plainEditor
		}
		p := clearProvider(fake, stateDir)
		if err := p.Start(context.Background(), dogName, seedConfig); err != nil {
			t.Fatalf("Start: %v", err)
		}
		assertDeliveredOnceAfterClear(t, fake, log, p, stateDir)
	})
	t.Run("prompt mode", func(t *testing.T) {
		stateDir := t.TempDir()
		fake := faketest.New()
		written := watchWrites(fake)
		log := &screenLog{}
		reads := 0
		fake.FormatFunc = func(rex.BlockID) string {
			reads++
			switch {
			case reads == 1:
				log.served(true)
				return trustDialogScreen
			case written.Load():
				// The typed turn changes the screen, which confirms it.
				log.served(false)
				return plainEditor + "hello agent\nworking…"
			default:
				log.served(false)
				return plainEditor
			}
		}
		p := clearProvider(fake, stateDir)
		cfg := seedConfig
		cfg.ReadyPromptPrefix = "❯ "
		if err := p.Start(context.Background(), dogName, cfg); err != nil {
			t.Fatalf("Start: %v", err)
		}
		assertDeliveredOnceAfterClear(t, fake, log, p, stateDir)
	})
}

func assertDeliveredOnceAfterClear(t *testing.T, fake *faketest.Fake, log *screenLog, p *provider.Provider, stateDir string) {
	t.Helper()
	assertNoInputIntoDialog(t, fake, log)
	if n := writesOf(fake, "hello agent"); n != 1 {
		t.Errorf("seed written %d times, want 1", n)
	}
	if n := keyCount(fake, "enter"); n != 1 {
		t.Errorf("enter pressed %d times, want 1", n)
	}
	if got := startupMarker(t, p, dogName); got != "" {
		t.Errorf("marker = %q after a confirmed delivery, want cleared", got)
	}
	timing := readStartupTiming(t, stateDir, dogName)
	if timing["readiness_outcome"] != "dialog-cleared" {
		t.Errorf("readiness_outcome = %v, want dialog-cleared", timing["readiness_outcome"])
	}
	if timing["confirmed"] != true {
		t.Errorf("confirmed = %v, want true", timing["confirmed"])
	}
}

// TestStartupSeedWithheldWhenDialogPersists pins AC3 / REQ-004 (a): a dialog
// that never clears leaves the seed untyped with a named withhold marker, and
// the wait's Rex calls end within the hard stop: first sighting + window + one
// poll, on the REQ-002 clock (first dialog served to last screen read served).
func TestStartupSeedWithheldWhenDialogPersists(t *testing.T) {
	for _, busy := range []bool{false, true} {
		name := "status idle"
		if busy {
			name = "status busy after the sighting"
		}
		t.Run(name, func(t *testing.T) {
			stateDir := t.TempDir()
			fake := faketest.New()
			at := time.Now()
			var first, last time.Time
			sighted := false
			fake.ProgramStatusFunc = func(rex.BlockID) rex.ProgramStatus {
				state := "idle"
				if busy && sighted {
					state = "working"
				}
				return rex.ProgramStatus{Records: []rex.ProgramStatusRecord{{State: state, UpdatedAt: at}}}
			}
			fake.FormatFunc = func(rex.BlockID) string {
				now := time.Now()
				if first.IsZero() {
					first = now
				}
				last, sighted = now, true
				return trustDialogScreen
			}
			p := clearProvider(fake, stateDir)
			if err := p.Start(context.Background(), dogName, seedConfig); err != nil {
				t.Fatalf("Start: %v (a withheld seed must not fail the start)", err)
			}
			if n, k := writeCount(fake), keyCount(fake, "enter"); n != 0 || k != 0 {
				t.Errorf("writes %d, enters %d; want none into a dialog", n, k)
			}
			want := "withheld: the pane shows a startup dialog (" + trustName + ")"
			if got := startupMarker(t, p, dogName); got != want {
				t.Errorf("marker = %q, want %q", got, want)
			}
			if got := readStartupTiming(t, stateDir, dogName)["readiness_outcome"]; got != "dialog" {
				t.Errorf("readiness_outcome = %v, want dialog", got)
			}
			gap := last.Sub(first)
			if gap < clearWindow {
				t.Errorf("last screen read %v after the first sighting; the wait must poll until the deadline (%v)", gap, clearWindow)
			}
			if limit := clearWindow + clearPoll + clearSlack; gap > limit {
				t.Errorf("last screen read %v after the first sighting, want <= %v", gap, limit)
			}
		})
	}
}

// TestStartupSeedUndeliveredWhenClearButNotReady pins AC4 / REQ-004 (b): after
// a sighting, a clear screen that never shows the prompt, or a read that
// fails, ends dialog-unresolved at the deadline with no write, even though the
// prompt bound passed long before. The status-mode case pins the guard that
// keeps a failed read from failing open to idle (panestate.go startupVerdict)
// and entering delivery.
func TestStartupSeedUndeliveredWhenClearButNotReady(t *testing.T) {
	want := "withheld: a startup dialog was seen (" + trustName + "); the pane was not clear and ready when the clear window ended"
	promptCfg := seedConfig
	promptCfg.ReadyPromptPrefix = "❯ "
	run := func(t *testing.T, setup func(*faketest.Fake), cfg provider.Config) {
		t.Helper()
		stateDir := t.TempDir()
		fake := faketest.New()
		setup(fake)
		p := clearProvider(fake, stateDir, provider.WithStartupReadiness(50*time.Millisecond, 10*time.Millisecond))
		if err := p.Start(context.Background(), dogName, cfg); err != nil {
			t.Fatalf("Start: %v", err)
		}
		if n, k := writeCount(fake), keyCount(fake, "enter"); n != 0 || k != 0 {
			t.Errorf("writes %d, enters %d; want none into a dialog", n, k)
		}
		if got := startupMarker(t, p, dogName); got != want {
			t.Errorf("marker = %q, want %q", got, want)
		}
		timing := readStartupTiming(t, stateDir, dogName)
		if timing["readiness_outcome"] != "dialog-unresolved" {
			t.Errorf("readiness_outcome = %v, want dialog-unresolved", timing["readiness_outcome"])
		}
		if timing["dialog"] != trustName || timing["dialog_screen"] != trustDialogScreen {
			t.Errorf("evidence = %v / %q, want the last sighting", timing["dialog"], timing["dialog_screen"])
		}
	}
	t.Run("clear but the prompt never shows", func(t *testing.T) {
		run(t, func(fake *faketest.Fake) {
			reads := 0
			fake.FormatFunc = func(rex.BlockID) string {
				reads++
				if reads == 1 {
					return trustDialogScreen
				}
				return "loading workspace…\n"
			}
		}, promptCfg)
	})
	t.Run("every read after the sighting fails", func(t *testing.T) {
		run(t, func(fake *faketest.Fake) {
			var reads atomic.Int32
			fake.FormatCtxFunc = func(context.Context, rex.BlockID) (string, error) {
				if reads.Add(1) == 1 {
					return trustDialogScreen, nil
				}
				return "", errors.New("rex: format failed")
			}
		}, promptCfg)
	})
	t.Run("status mode, every read after the sighting fails", func(t *testing.T) {
		run(t, func(fake *faketest.Fake) {
			fake.ProgramStatusFunc = idleStatus
			var reads atomic.Int32
			fake.FormatCtxFunc = func(context.Context, rex.BlockID) (string, error) {
				if reads.Add(1) == 1 {
					return trustDialogScreen, nil
				}
				return "", errors.New("rex: format failed")
			}
		}, seedConfig)
	})
}

// TestStartupDialogClearWaitCanceledOrEnded pins REQ-002's last examples: a
// cancel during the clear window ends canceled and types nothing; a pane whose
// program ends during it ends ended and fails a non-one_shot start.
func TestStartupDialogClearWaitCanceledOrEnded(t *testing.T) {
	t.Run("canceled", func(t *testing.T) {
		stateDir := t.TempDir()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		fake := faketest.New()
		fake.ProgramStatusFunc = idleStatus
		reads := 0
		fake.FormatFunc = func(rex.BlockID) string {
			reads++
			if reads == 3 {
				cancel()
			}
			return trustDialogScreen
		}
		p := clearProvider(fake, stateDir)
		_ = p.Start(ctx, dogName, seedConfig) // the cancel's own error is not under test
		if n := writeCount(fake); n != 0 {
			t.Errorf("writes %d after a cancel, want 0", n)
		}
		if got := readStartupTiming(t, stateDir, dogName)["readiness_outcome"]; got != "canceled" {
			t.Errorf("readiness_outcome = %v, want canceled", got)
		}
	})
	t.Run("ended", func(t *testing.T) {
		stateDir := t.TempDir()
		fake := faketest.New()
		fake.ProgramStatusFunc = idleStatus
		sighted := false
		fake.FormatFunc = func(rex.BlockID) string {
			sighted = true
			return trustDialogScreen
		}
		fake.ProcessFunc = func(rex.BlockID) rex.ProcessInfo {
			if sighted {
				return rex.ProcessInfo{LastExit: &rex.ExitStatus{ExitCode: 1}}
			}
			return rex.ProcessInfo{}
		}
		p := clearProvider(fake, stateDir)
		err := p.Start(context.Background(), dogName, seedConfig)
		if err == nil || !strings.Contains(err.Error(), "before its first turn") {
			t.Fatalf("Start error = %v, want before-its-first-turn", err)
		}
		if got := readStartupTiming(t, stateDir, dogName)["readiness_outcome"]; got != "ended" {
			t.Errorf("readiness_outcome = %v, want ended", got)
		}
	})
}

// TestStartupDialogIgnoredAboveScreenTail pins AC6 / REQ-007: a dialog marker
// that scrolled above the last 120 lines no longer withholds the seed; one
// inside them still does.
func TestStartupDialogIgnoredAboveScreenTail(t *testing.T) {
	screen := func(linesBelow int) string {
		var b strings.Builder
		b.WriteString("Do you trust the files in this folder?\n")
		for i := range linesBelow {
			fmt.Fprintf(&b, "log line %d\n", i)
		}
		b.WriteString(plainEditor)
		return b.String()
	}
	t.Run("marker 200 lines up", func(t *testing.T) {
		fake := faketest.New()
		turnStarts(fake)
		fake.Formats = map[rex.BlockID]string{dogBlock: screen(199)}
		p := clearProvider(fake, t.TempDir())
		if err := p.Start(context.Background(), dogName, seedConfig); err != nil {
			t.Fatalf("Start: %v", err)
		}
		if n := writesOf(fake, "hello agent"); n != 1 {
			t.Errorf("seed written %d times, want 1", n)
		}
		if got := startupMarker(t, p, dogName); strings.HasPrefix(got, "withheld:") {
			t.Errorf("marker = %q, want no withhold for scrollback", got)
		}
	})
	t.Run("marker inside the last 120 lines", func(t *testing.T) {
		fake := faketest.New()
		fake.ProgramStatuses = idleProgramStatuses()
		fake.Formats = map[rex.BlockID]string{dogBlock: screen(100)}
		p := clearProvider(fake, t.TempDir())
		if err := p.Start(context.Background(), dogName, seedConfig); err != nil {
			t.Fatalf("Start: %v", err)
		}
		if n := writeCount(fake); n != 0 {
			t.Errorf("writes %d, want 0", n)
		}
		if got := startupMarker(t, p, dogName); !strings.HasPrefix(got, "withheld:") {
			t.Errorf("marker = %q, want a withhold", got)
		}
	})
}
