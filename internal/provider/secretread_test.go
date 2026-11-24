package provider_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/docbobo/rex-gascity/internal/provider"
	"github.com/docbobo/rex-gascity/internal/rex"
	"github.com/docbobo/rex-gascity/internal/rex/faketest"
)

// Issue #48: the provider must not type a nudge or send-keys input into a
// pane whose program is reading a secret. The predicate is canonical && !echo
// and is UNVERIFIED by the parity doc and gc v1.5.1; its basis is
// docs/REX-CONTRACT.md §4 input_mode and the issue #48 live probe.

// secretReadMode is a cooked read with echo off: sudo, ssh, read -s, getpass.
var secretReadMode = rex.InputMode{Echo: false, Canonical: true, Signals: true}

// readlineMode is a line editor (bash, a python REPL): echo off, not canonical.
var readlineMode = rex.InputMode{Echo: false, Canonical: false, Signals: true}

// wroteText reports whether the fake recorded any text write.
func wroteText(fake *faketest.Fake) bool {
	return slices.Contains(ops(fake), "write")
}

// TestNudgeWithholdsDuringSecretRead is the #48 proof test (REQ-002, REQ-004,
// E1): a Wait-mode nudge to an idle pane in a secret read types nothing, sends
// no submit, stamps no input floor, and answers pending_interaction wrapping
// ErrSecretRead with a detail that names the secret read.
func TestNudgeWithholdsDuringSecretRead(t *testing.T) {
	fake := faketest.New()
	fake.InputModes = map[rex.BlockID]rex.InputMode{activityBlock: secretReadMode}
	store := provider.NewMemoryActivityStore()
	p := nudgeStarted(t, fake, provider.WithActivityStore(store))

	_, err := p.NudgeReport(context.Background(), activityName, "hello", provider.NudgeModeWait)
	outcome := needOutcome(t, err, "pending_interaction")
	if !errors.Is(err, provider.ErrSecretRead) {
		t.Errorf("error = %v, want it to wrap ErrSecretRead", err)
	}
	if want := "provider: pane is reading a secret: " + activityName + " (input_mode canonical, echo off)"; outcome.Detail != want {
		t.Errorf("detail = %q, want %q", outcome.Detail, want)
	}
	if wroteText(fake) {
		t.Errorf("block ops = %v, want no write into a secret read", ops(fake))
	}
	if len(fake.SentKeys) != 0 {
		t.Errorf("send-keys calls = %+v, want none", fake.SentKeys)
	}
	if rec, ok := store.Load(activityName); ok && !rec.InputAt.IsZero() {
		t.Errorf("input floor = %v, want none for a withheld nudge", rec.InputAt)
	}
}

// TestNudgeTypesAtReadlinePrompt pins the false-positive guard (REQ-002, E7):
// echo off without a canonical read is readline, and the nudge is delivered.
// It also pins that the read happened, so the pin is about the predicate, not
// about a missing guard.
func TestNudgeTypesAtReadlinePrompt(t *testing.T) {
	fake := faketest.New()
	fake.InputModes = map[rex.BlockID]rex.InputMode{activityBlock: readlineMode}
	p := nudgeStarted(t, fake)

	if err := p.Nudge(context.Background(), activityName, "hello"); err != nil {
		t.Fatalf("Nudge at a readline prompt: %v", err)
	}
	if !wroteText(fake) {
		t.Errorf("block ops = %v, want the text written", ops(fake))
	}
	if len(fake.InputModeReads) != 1 {
		t.Errorf("input_mode reads = %d, want 1", len(fake.InputModeReads))
	}
}

// TestNudgeImmediateWithholdsDuringSecretRead covers REQ-004 in Immediate
// mode (E2, E11): the override skips the busy gate but never the secret read.
func TestNudgeImmediateWithholdsDuringSecretRead(t *testing.T) {
	fake := faketest.New()
	fake.ProgramStatuses = map[rex.BlockID]rex.ProgramStatus{
		activityBlock: {Records: []rex.ProgramStatusRecord{{ID: "a", State: "working"}}},
	}
	fake.InputModes = map[rex.BlockID]rex.InputMode{activityBlock: secretReadMode}
	p := nudgeStarted(t, fake)

	report, err := p.NudgeReport(context.Background(), activityName, "hello", provider.NudgeModeImmediate)
	needOutcome(t, err, "pending_interaction")
	if !errors.Is(err, provider.ErrSecretRead) {
		t.Errorf("error = %v, want it to wrap ErrSecretRead", err)
	}
	if report.State != "working" || report.Delivered {
		t.Errorf("report = %+v, want the baseline state and no delivery", report)
	}
	if wroteText(fake) || len(fake.SentKeys) != 0 {
		t.Errorf("ops = %v, keys = %+v; want nothing typed", ops(fake), fake.SentKeys)
	}
}

// TestNudgeBusyGateAnswersBeforeSecretRead pins the check order (REQ-004,
// E10): a Wait-mode nudge to a working pane keeps its busy answer and makes no
// input_mode read.
func TestNudgeBusyGateAnswersBeforeSecretRead(t *testing.T) {
	fake := faketest.New()
	fake.ProgramStatuses = map[rex.BlockID]rex.ProgramStatus{
		activityBlock: {Records: []rex.ProgramStatusRecord{{ID: "a", State: "working"}}},
	}
	fake.InputModes = map[rex.BlockID]rex.InputMode{activityBlock: secretReadMode}
	p := nudgeStarted(t, fake)

	_, err := p.NudgeReport(context.Background(), activityName, "hello", provider.NudgeModeWait)
	needOutcome(t, err, "busy")
	if !errors.Is(err, provider.ErrNudgeBusy) || errors.Is(err, provider.ErrSecretRead) {
		t.Errorf("error = %v, want ErrNudgeBusy and not ErrSecretRead", err)
	}
	if len(fake.InputModeReads) != 0 {
		t.Errorf("input_mode reads = %d, want 0 behind the busy gate", len(fake.InputModeReads))
	}
}

// TestNudgeSecretReadProbeErrorFailsOpen pins REQ-003 (E12, E13): a failed
// input_mode read, here an old Rex without the method, delivers as today even
// though the mode map says secret read.
func TestNudgeSecretReadProbeErrorFailsOpen(t *testing.T) {
	fake := faketest.New()
	fake.InputModes = map[rex.BlockID]rex.InputMode{activityBlock: secretReadMode}
	fake.InputModeErr = errors.New(`rex: no such block method "input_mode"`)
	p := nudgeStarted(t, fake)

	if err := p.Nudge(context.Background(), activityName, "hello"); err != nil {
		t.Fatalf("Nudge with a failed input_mode read: %v", err)
	}
	if !wroteText(fake) {
		t.Errorf("block ops = %v, want the text written (fail open)", ops(fake))
	}
}

// stallInputModeFake models an input_mode read that never answers and tells
// the test when the read has started, so the test can cancel the parent
// during the read.
type stallInputModeFake struct {
	*faketest.Fake
	entered chan struct{}
}

func (f *stallInputModeFake) BlockInputMode(ctx context.Context, _ rex.SessionID, _ rex.BlockID) (rex.InputMode, error) {
	close(f.entered)
	<-ctx.Done()
	return rex.InputMode{}, ctx.Err()
}

// TestNudgeSecretReadProbeStallFailsOpen pins the probe's own bound (REQ-003,
// test 5a, E13b, E13c).
func TestNudgeSecretReadProbeStallFailsOpen(t *testing.T) {
	// (a) The parent has a deadline and stays live: the probe gives up on its
	// own budget and the nudge is delivered within the parent's deadline.
	// Without the bound the stalled read would use up the parent deadline and
	// the nudge would fail "before typing".
	t.Run("parent live", func(t *testing.T) {
		fake := faketest.New()
		fake.InputModeStall = true
		p := nudgeStarted(t, fake, provider.WithInputModeProbeTimeout(20*time.Millisecond))

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		report, err := p.NudgeReport(ctx, activityName, "hello", provider.NudgeModeWait)
		if err != nil {
			t.Fatalf("NudgeReport with a stalled input_mode read: %v", err)
		}
		if !report.Delivered || !wroteText(fake) {
			t.Errorf("report = %+v, ops = %v; want the text delivered", report, ops(fake))
		}
	})

	// (c) The probe budget is longer than the parent's deadline: the budget
	// is capped at half of the remaining time, so the read gives up early
	// enough for the nudge to be typed before the parent expires.
	t.Run("budget capped by parent deadline", func(t *testing.T) {
		fake := faketest.New()
		fake.InputModeStall = true
		p := nudgeStarted(t, fake, provider.WithInputModeProbeTimeout(10*time.Second))

		ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
		defer cancel()
		report, err := p.NudgeReport(ctx, activityName, "hello", provider.NudgeModeWait)
		if err != nil {
			t.Fatalf("NudgeReport with a stalled read under a short deadline: %v", err)
		}
		if !report.Delivered || !wroteText(fake) {
			t.Errorf("report = %+v, ops = %v; want the text delivered", report, ops(fake))
		}
	})

	// (b) The parent is cancelled while the read stalls: nothing is typed and
	// the answer is today's cancellation error, not a secret-read withhold.
	t.Run("parent cancelled", func(t *testing.T) {
		fake := &stallInputModeFake{Fake: faketest.New(), entered: make(chan struct{})}
		// The probe budget is far longer than the test allows, so only the
		// parent's cancellation can end the read in time.
		p := nudgeStarted(t, fake, provider.WithInputModeProbeTimeout(10*time.Second))

		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			<-fake.entered
			cancel()
		}()
		start := time.Now()
		_, err := p.NudgeReport(ctx, activityName, "hello", provider.NudgeModeWait)
		if elapsed := time.Since(start); elapsed > 5*time.Second {
			t.Errorf("NudgeReport took %v after the parent was cancelled, want the read to stop with the parent", elapsed)
		}
		if !errors.Is(err, context.Canceled) || errors.Is(err, provider.ErrSecretRead) {
			t.Fatalf("error = %v, want context.Canceled and not ErrSecretRead", err)
		}
		if !strings.Contains(err.Error(), "before typing") {
			t.Errorf("error = %q, want today's before-typing cancellation", err)
		}
		if wroteText(fake.Fake) || len(fake.SentKeys) != 0 {
			t.Errorf("ops = %v, keys = %+v; want nothing typed", ops(fake.Fake), fake.SentKeys)
		}
	})
}

// lateReplyInputModeFake models an input_mode read that answers a secret read
// after its context ended, as a CLI response decoded after the command
// completed can. It tells the test when the read has started.
type lateReplyInputModeFake struct {
	*faketest.Fake
	entered chan struct{}
}

func (f *lateReplyInputModeFake) BlockInputMode(ctx context.Context, _ rex.SessionID, _ rex.BlockID) (rex.InputMode, error) {
	close(f.entered)
	<-ctx.Done()
	return secretReadMode, nil
}

// TestNudgeDiscardsLateInputModeReply pins REQ-003 / AC-6: a reply that
// arrives after the probe context ended does not decide the guard. A probe
// timeout with a live parent fails open and delivers the nudge; a parent
// cancellation keeps the cancellation result and types nothing.
func TestNudgeDiscardsLateInputModeReply(t *testing.T) {
	// (a) The probe times out and the late reply says secret read: the parent
	// is live, so the guard fails open and the nudge is delivered.
	t.Run("probe timeout, parent live", func(t *testing.T) {
		fake := &lateReplyInputModeFake{Fake: faketest.New(), entered: make(chan struct{})}
		p := nudgeStarted(t, fake, provider.WithInputModeProbeTimeout(20*time.Millisecond))

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		report, err := p.NudgeReport(ctx, activityName, "hello", provider.NudgeModeWait)
		if err != nil {
			t.Fatalf("NudgeReport with a late secret-read reply: %v", err)
		}
		if !report.Delivered || !wroteText(fake.Fake) {
			t.Errorf("report = %+v, ops = %v; want the text delivered", report, ops(fake.Fake))
		}
	})

	// (b) The parent is cancelled during the read and the late reply says
	// secret read: the nudge keeps today's cancellation result and types
	// nothing, rather than answering a secret-read withhold.
	t.Run("parent cancelled", func(t *testing.T) {
		fake := &lateReplyInputModeFake{Fake: faketest.New(), entered: make(chan struct{})}
		// The probe budget is far longer than the test allows, so only the
		// parent's cancellation can end the read.
		p := nudgeStarted(t, fake, provider.WithInputModeProbeTimeout(10*time.Second))

		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			<-fake.entered
			cancel()
		}()
		_, err := p.NudgeReport(ctx, activityName, "hello", provider.NudgeModeWait)
		if !errors.Is(err, context.Canceled) || errors.Is(err, provider.ErrSecretRead) {
			t.Fatalf("error = %v, want context.Canceled and not ErrSecretRead", err)
		}
		if wroteText(fake.Fake) || len(fake.SentKeys) != 0 {
			t.Errorf("ops = %v, keys = %+v; want nothing typed", ops(fake.Fake), fake.SentKeys)
		}
	})
}
