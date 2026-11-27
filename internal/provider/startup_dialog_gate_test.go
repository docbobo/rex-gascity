package provider_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/docbobo/rex-gascity/internal/provider"
	"github.com/docbobo/rex-gascity/internal/rex"
	"github.com/docbobo/rex-gascity/internal/rex/faketest"
)

// gateScript serves the trust dialog until the clear window's deadline,
// measured from the first read it served, then hands each later read to next
// with its 1-based index. It logs what each read served.
type gateScript struct {
	mu    sync.Mutex
	first time.Time
	after int
	log   screenLog
}

func (s *gateScript) hook(next func(ctx context.Context, n int) (string, error)) func(context.Context, rex.BlockID) (string, error) {
	return func(ctx context.Context, _ rex.BlockID) (string, error) {
		s.mu.Lock()
		now := time.Now()
		if s.first.IsZero() {
			s.first = now
		}
		if now.Before(s.first.Add(clearWindow)) {
			s.mu.Unlock()
			s.log.served(true)
			return trustDialogScreen, nil
		}
		s.after++
		n := s.after
		s.mu.Unlock()
		screen, err := next(ctx, n)
		if err != nil {
			// A failed read is never a dialog: assertNoInputIntoDialog must
			// not blame a later input on this read.
			s.log.served(false)
		} else {
			s.log.served(screen == trustDialogScreen)
		}
		return screen, err
	}
}

func (s *gateScript) firstServed() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.first
}

func (s *gateScript) readsAfterDeadline() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.after
}

// pause waits d or until ctx ends, as a slow Rex read under exec.CommandContext
// would.
func pause(ctx context.Context, d time.Duration) {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}

// gateRun is one Start against a gateScript.
type gateRun struct {
	fake     *faketest.Fake
	script   *gateScript
	p        *provider.Provider
	stateDir string
	err      error
}

func runGate(t *testing.T, prompt bool, next func(context.Context, int) (string, error)) gateRun {
	t.Helper()
	r := gateRun{fake: faketest.New(), script: &gateScript{}, stateDir: t.TempDir()}
	cfg := seedConfig
	if prompt {
		cfg.ReadyPromptPrefix = "❯ "
	} else {
		turnStarts(r.fake)
	}
	r.fake.FormatCtxFunc = r.script.hook(next)
	r.p = clearProvider(r.fake, r.stateDir)
	// No deadline on the parent: only the hard stop can end a stalled read.
	r.err = r.p.Start(context.Background(), dogName, cfg)
	return r
}

// TestStartupDialogGateBlocksAtDeadline pins AC3a: REQ-001 (a), REQ-002 and
// REQ-004 (b)/(c) at the clear window's deadline.
func TestStartupDialogGateBlocksAtDeadline(t *testing.T) {
	shownMarker := "withheld: the pane shows a startup dialog (" + trustName + ")"
	unresolvedMarker := "withheld: a startup dialog was seen (" + trustName + "); the pane was not clear and ready when the clear window ended"

	t.Run("the pre-write gate sees the dialog again after the deadline poll", func(t *testing.T) {
		r := runGate(t, false, func(ctx context.Context, n int) (string, error) {
			if n == 1 {
				return plainEditor, nil // the deadline poll: clear, and idle
			}
			pause(ctx, 2*time.Millisecond) // the gate read returns after the deadline
			return trustDialogScreen, nil
		})
		if r.err != nil {
			t.Fatalf("Start: %v", r.err)
		}
		if n, k := writeCount(r.fake), keyCount(r.fake, "enter"); n != 0 || k != 0 {
			t.Errorf("writes %d, enters %d; want none", n, k)
		}
		if n := r.script.readsAfterDeadline(); n != 2 {
			t.Errorf("%d reads after the deadline, want 2 (deadline poll, gate) and no poll after the gate", n)
		}
		timing := readStartupTiming(t, r.stateDir, dogName)
		if timing["readiness_outcome"] != "dialog" || timing["dialog_gate"] != "pre-write" {
			t.Errorf("readiness_outcome %v, dialog_gate %v; want dialog, pre-write", timing["readiness_outcome"], timing["dialog_gate"])
		}
		if got := startupMarker(t, r.p, dogName); got != shownMarker {
			t.Errorf("marker = %q, want %q", got, shownMarker)
		}
	})

	t.Run("the pre-write gate read fails after the deadline poll", func(t *testing.T) {
		r := runGate(t, false, func(ctx context.Context, n int) (string, error) {
			if n == 1 {
				return plainEditor, nil
			}
			pause(ctx, 2*time.Millisecond)
			return "", errors.New("rex: format failed")
		})
		if r.err != nil {
			t.Fatalf("Start: %v", r.err)
		}
		if n := writeCount(r.fake); n != 0 {
			t.Errorf("writes %d, want 0", n)
		}
		timing := readStartupTiming(t, r.stateDir, dogName)
		if timing["readiness_outcome"] != "dialog-unresolved" {
			t.Errorf("readiness_outcome = %v, want dialog-unresolved", timing["readiness_outcome"])
		}
		if timing["dialog"] != trustName || timing["dialog_screen"] != trustDialogScreen {
			t.Errorf("evidence %v / %q, want the last sighting", timing["dialog"], timing["dialog_screen"])
		}
		if _, ok := timing["dialog_gate"]; ok {
			t.Errorf("dialog_gate = %v, want absent: the last sighting was the readiness wait's", timing["dialog_gate"])
		}
		if got := startupMarker(t, r.p, dogName); got != unresolvedMarker {
			t.Errorf("marker = %q, want %q", got, unresolvedMarker)
		}
	})

	// The two stall cases check the stalled call's context end on the REQ-002
	// clock: first dialog served + window + one poll, plus the named slack.
	stallCase := func(t *testing.T, prompt bool, stallAt int) {
		t.Helper()
		var stallEnd time.Time
		r := runGate(t, prompt, func(ctx context.Context, n int) (string, error) {
			switch {
			case n < stallAt:
				return plainEditor, nil
			case n == stallAt:
				<-ctx.Done()
				stallEnd = time.Now()
				return "", ctx.Err()
			default:
				if err := ctx.Err(); err != nil {
					return "", err
				}
				return plainEditor, nil
			}
		})
		if r.err != nil {
			t.Fatalf("Start: %v (an unresolved dialog must not fail the start)", r.err)
		}
		if stallEnd.IsZero() {
			t.Fatal("the scripted read never stalled")
		}
		if limit := r.script.firstServed().Add(clearWindow + clearPoll + clearSlack); stallEnd.After(limit) {
			t.Errorf("stalled read ended %v after the first sighting, want <= %v",
				stallEnd.Sub(r.script.firstServed()), clearWindow+clearPoll+clearSlack)
		}
		if n := writeCount(r.fake); n != 0 {
			t.Errorf("writes %d, want 0", n)
		}
		if k := keyCount(r.fake, "enter"); k != 0 {
			t.Errorf("enters %d, want 0", k)
		}
		if got := readStartupTiming(t, r.stateDir, dogName)["readiness_outcome"]; got != "dialog-unresolved" {
			t.Errorf("readiness_outcome = %v, want dialog-unresolved", got)
		}
		if got := startupMarker(t, r.p, dogName); got != unresolvedMarker {
			t.Errorf("marker = %q, want %q", got, unresolvedMarker)
		}
	}
	t.Run("the deadline poll's screen read stalls", func(t *testing.T) {
		stallCase(t, false, 1)
	})
	// The ladder read stalled here is prompt mode's baseline screen read: it
	// runs between the clear-and-ready poll and the pre-write gate. The
	// secret-mode read cannot model a stall until its context ends:
	// readingSecret caps its own probe at half the time left
	// (inputmode.go:68-80), so it returns before the hard stop.
	t.Run("a ladder read before the pre-write gate stalls", func(t *testing.T) {
		stallCase(t, true, 2)
	})

	t.Run("a clear pre-write gate read returns after the deadline", func(t *testing.T) {
		r := runGate(t, false, func(ctx context.Context, n int) (string, error) {
			if n == 2 {
				pause(ctx, clearPoll/2) // after the deadline, before the hard stop
			}
			return plainEditor, nil
		})
		if r.err != nil {
			t.Fatalf("Start: %v", r.err)
		}
		assertNoInputIntoDialog(t, r.fake, &r.script.log)
		if n, k := writesOf(r.fake, "hello agent"), keyCount(r.fake, "enter"); n != 1 || k != 1 {
			t.Errorf("writes %d, enters %d; want 1, 1", n, k)
		}
		timing := readStartupTiming(t, r.stateDir, dogName)
		if timing["readiness_outcome"] != "dialog-cleared" || timing["confirmed"] != true {
			t.Errorf("readiness_outcome %v, confirmed %v; want dialog-cleared, true", timing["readiness_outcome"], timing["confirmed"])
		}
		if got := startupMarker(t, r.p, dogName); got != "" {
			t.Errorf("marker = %q, want none: the clear gate read delivered", got)
		}
	})

	// A clear reply that returns at the hard stop is stale: the read was cut
	// off there, so it must not let the seed through (REQ-002, REQ-004 (c)).
	t.Run("a clear pre-write gate read returns at the hard stop", func(t *testing.T) {
		r := runGate(t, false, func(ctx context.Context, n int) (string, error) {
			if n == 2 { // the pre-write gate: the deadline poll came first, the status read skipped the baseline
				<-ctx.Done()
			}
			return plainEditor, nil
		})
		if r.err != nil {
			t.Fatalf("Start: %v", r.err)
		}
		if n, k := writeCount(r.fake), keyCount(r.fake, "enter"); n != 0 || k != 0 {
			t.Errorf("writes %d, enters %d; want none", n, k)
		}
		if got := readStartupTiming(t, r.stateDir, dogName)["readiness_outcome"]; got != "dialog-unresolved" {
			t.Errorf("readiness_outcome = %v, want dialog-unresolved", got)
		}
		if got := startupMarker(t, r.p, dogName); got != unresolvedMarker {
			t.Errorf("marker = %q, want %q", got, unresolvedMarker)
		}
	})
}

// TestStartupDialogGateSubmitBudget pins REQ-002: the gate read does not
// consume the submit budget. The gate returns a clear screen well after a
// short budget, before the hard stop; the write and the Enter still land.
func TestStartupDialogGateSubmitBudget(t *testing.T) {
	reads := 0
	fake, stateDir := faketest.New(), t.TempDir()
	fake.FormatCtxFunc = func(ctx context.Context, _ rex.BlockID) (string, error) {
		// With no program_status: read 1 is the settle-bound poll, read 2 the
		// ladder's baseline read, read 3 the pre-write gate.
		reads++
		if reads == 3 {
			pause(ctx, 50*time.Millisecond)
		}
		return plainEditor, nil
	}
	p := clearProvider(fake, stateDir, provider.WithNudgeSubmitBudget(10*time.Millisecond))
	if err := p.Start(context.Background(), dogName, seedConfig); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if n, k := writeCount(fake), keyCount(fake, "enter"); n != 1 || k != 1 {
		t.Errorf("writes %d, enters %d; want 1, 1: the gate read must not use the submit budget", n, k)
	}
}

// TestStartupDialogGateParentCancel pins REQ-002: a parent cancel during the
// gate's read is canceled, not the hard stop's dialog-unresolved.
func TestStartupDialogGateParentCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	fake, stateDir := faketest.New(), t.TempDir()
	turnStarts(fake)
	script := &gateScript{}
	fake.FormatCtxFunc = script.hook(func(readCtx context.Context, n int) (string, error) {
		if n == 1 {
			return plainEditor, nil // the deadline poll: clear, and idle
		}
		pause(readCtx, clearPoll/2) // after the deadline, before the hard stop
		cancel()
		return trustDialogScreen, nil
	})
	p := clearProvider(fake, stateDir)
	if err := p.Start(ctx, dogName, seedConfig); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if n, k := writeCount(fake), keyCount(fake, "enter"); n != 0 || k != 0 {
		t.Errorf("writes %d, enters %d; want none", n, k)
	}
	timing := readStartupTiming(t, stateDir, dogName)
	if timing["readiness_outcome"] != "canceled" {
		t.Errorf("readiness_outcome = %v, want canceled", timing["readiness_outcome"])
	}
	if got := startupMarker(t, p, dogName); got != "" && got[:len("withheld:")] == "withheld:" {
		t.Errorf("marker = %q, want no withheld marker", got)
	}
}

// TestStartupDialogCancelAfterWritePreservesReadiness pins REQ-002: a parent
// cancel after the write is not canceled readiness. The ladder delivered, so
// the diagnostic keeps the readiness outcome it reached.
func TestStartupDialogCancelAfterWritePreservesReadiness(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	fake, stateDir := faketest.New(), t.TempDir()
	fake.ProgramStatuses = idleProgramStatuses()
	fake.FormatFunc = func(rex.BlockID) string { return plainEditor }
	fake.WriteFunc = func(rex.SessionID, rex.BlockID, string) { cancel() }
	p := clearProvider(fake, stateDir)
	if err := p.Start(ctx, dogName, seedConfig); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if n, k := writeCount(fake), keyCount(fake, "enter"); n != 1 || k != 1 {
		t.Errorf("writes %d, enters %d; want 1, 1: the write had already landed", n, k)
	}
	if got := readStartupTiming(t, stateDir, dogName)["readiness_outcome"]; got != "idle" {
		t.Errorf("readiness_outcome = %v, want idle: the cancel came after the write", got)
	}
}

// TestStartupDialogFirstSeenAtPreWriteGate pins REQ-001 (a): readiness ends
// on a bound with no sighting, and the pre-write gate sees the first dialog.
// Expected call order with no program_status: read 1 is the settle-bound
// poll, read 2 the ladder's baseline read, read 3 the pre-write gate.
func TestStartupDialogFirstSeenAtPreWriteGate(t *testing.T) {
	type script struct {
		reads   int
		sighted bool
		written bool
	}
	setup := func(fake *faketest.Fake, log *screenLog, dialogFrom, dialogTo int, statusAfterSighting bool) {
		s := &script{}
		fake.WriteFunc = func(rex.SessionID, rex.BlockID, string) { s.written = true }
		fake.FormatFunc = func(rex.BlockID) string {
			s.reads++
			if s.reads >= dialogFrom && (dialogTo == 0 || s.reads <= dialogTo) {
				s.sighted = true
				log.served(true)
				return trustDialogScreen
			}
			log.served(false)
			return plainEditor
		}
		at := time.Now()
		fake.ProgramStatusFunc = func(rex.BlockID) rex.ProgramStatus {
			switch {
			case !statusAfterSighting || !s.sighted:
				return rex.ProgramStatus{}
			case s.written:
				return rex.ProgramStatus{Records: []rex.ProgramStatusRecord{{State: "working", UpdatedAt: at.Add(time.Second)}}}
			default:
				return rex.ProgramStatus{Records: []rex.ProgramStatusRecord{{State: "idle", UpdatedAt: at}}}
			}
		}
	}

	t.Run("the bound no longer delivers after the gate's sighting", func(t *testing.T) {
		stateDir := t.TempDir()
		fake, log := faketest.New(), &screenLog{}
		setup(fake, log, 3, 3, false)
		p := clearProvider(fake, stateDir)
		if err := p.Start(context.Background(), dogName, seedConfig); err != nil {
			t.Fatalf("Start: %v", err)
		}
		if n := writeCount(fake); n != 0 {
			t.Errorf("writes %d, want 0: no clear read plus idle ever came", n)
		}
		timing := readStartupTiming(t, stateDir, dogName)
		if timing["readiness_outcome"] != "dialog-unresolved" || timing["dialog_gate"] != "pre-write" {
			t.Errorf("readiness_outcome %v, dialog_gate %v; want dialog-unresolved, pre-write", timing["readiness_outcome"], timing["dialog_gate"])
		}
	})
	t.Run("a clear read plus idle delivers after the gate's sighting", func(t *testing.T) {
		stateDir := t.TempDir()
		fake, log := faketest.New(), &screenLog{}
		setup(fake, log, 3, 3, true)
		p := clearProvider(fake, stateDir)
		if err := p.Start(context.Background(), dogName, seedConfig); err != nil {
			t.Fatalf("Start: %v", err)
		}
		assertNoInputIntoDialog(t, fake, log)
		if n := writesOf(fake, "hello agent"); n != 1 {
			t.Errorf("seed written %d times, want 1", n)
		}
		timing := readStartupTiming(t, stateDir, dogName)
		if timing["readiness_outcome"] != "dialog-cleared" || timing["dialog_gate"] != "pre-write" {
			t.Errorf("readiness_outcome %v, dialog_gate %v; want dialog-cleared, pre-write", timing["readiness_outcome"], timing["dialog_gate"])
		}
	})
	t.Run("clear wait disabled", func(t *testing.T) {
		stateDir := t.TempDir()
		fake, log := faketest.New(), &screenLog{}
		setup(fake, log, 3, 0, false)
		p := clearProvider(fake, stateDir, provider.WithStartupDialogClear(0))
		if err := p.Start(context.Background(), dogName, seedConfig); err != nil {
			t.Fatalf("Start: %v", err)
		}
		if n := writeCount(fake); n != 0 {
			t.Errorf("writes %d, want 0", n)
		}
		timing := readStartupTiming(t, stateDir, dogName)
		if timing["readiness_outcome"] != "dialog" || timing["dialog_gate"] != "pre-write" {
			t.Errorf("readiness_outcome %v, dialog_gate %v; want dialog (not settle-bound), pre-write", timing["readiness_outcome"], timing["dialog_gate"])
		}
		want := "withheld: the pane shows a startup dialog (" + trustName + ")"
		if got := startupMarker(t, p, dogName); got != want {
			t.Errorf("marker = %q, want %q", got, want)
		}
	})
}

// TestStartupDialogDuringBackoffStopsResend pins REQ-001 before a resend: a
// dialog that appears after the first seed stops the ladder before the clear
// key, so no ctrl+u and no second seed reach it. The first submit never
// confirms, because the program stays idle, so the ladder does back off and
// resend if nothing stops it.
func TestStartupDialogDuringBackoffStopsResend(t *testing.T) {
	fake := faketest.New()
	fake.ProgramStatuses = idleProgramStatuses()
	written := watchWrites(fake)
	afterWrite := 0
	fake.FormatCtxFunc = func(_ context.Context, _ rex.BlockID) (string, error) {
		if written.Load() {
			// The pre-submit read stays clear; the dialog appears on the
			// pre-resend read after the backoff.
			afterWrite++
			if afterWrite > 1 {
				return trustDialogScreen, nil
			}
		}
		return plainEditor, nil
	}
	p := clearProvider(fake, t.TempDir(), provider.WithStartupNudgeRetries([]time.Duration{time.Millisecond}))
	if err := p.Start(context.Background(), dogName, seedConfig); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if n := writesOf(fake, "hello agent"); n != 1 {
		t.Errorf("seed written %d times, want 1", n)
	}
	if n := keyCount(fake, "enter"); n != 1 {
		t.Errorf("enter pressed %d times, want 1", n)
	}
	if n := keyCount(fake, "ctrl+u"); n != 0 {
		t.Errorf("draft cleared %d times after a dialog showed, want 0", n)
	}
	want := "delivered but not confirmed: a startup dialog appeared (" + trustName + ") before a resend"
	if got := startupMarker(t, p, dogName); got != want {
		t.Errorf("marker = %q, want %q", got, want)
	}
}

// TestStartupDialogClearBeforeSeedKeepsResend pins the lost-Enter rescue when a
// dialog is seen and cleared before the first seed. The resend gate runs under
// ctx, not the pre-write hard stop: a backoff that outlasts the clear window
// and its poll must still clear and retype the seed, not withhold it on a read
// the expired hard stop cut off.
func TestStartupDialogClearBeforeSeedKeepsResend(t *testing.T) {
	fake := faketest.New()
	fake.ProgramStatuses = idleProgramStatuses()
	script := &gateScript{}
	fake.FormatCtxFunc = script.hook(func(ctx context.Context, _ int) (string, error) {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		return plainEditor, nil
	})
	p := clearProvider(fake, t.TempDir(),
		provider.WithStartupNudgeRetries([]time.Duration{clearWindow + clearPoll + 100*time.Millisecond}))
	if err := p.Start(context.Background(), dogName, seedConfig); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if n := writesOf(fake, "hello agent"); n != 2 {
		t.Errorf("seed written %d times, want 2", n)
	}
	if n := keyCount(fake, "ctrl+u"); n != 1 {
		t.Errorf("draft cleared %d times, want 1", n)
	}
}

// TestStartupDialogPoolClaimProbeUnderHardStop pins REQ-002 for the pool-claim
// probe: after a sighting the first attempt's probe runs under the hard stop,
// so a stalled probe is cut off there and reports no claim. The gate read after
// it then fails under the expired context and the wait ends dialog-unresolved.
func TestStartupDialogPoolClaimProbeUnderHardStop(t *testing.T) {
	const poolClaim = "Run gc hook --claim --drain-ack --json now; if it returns work, execute it immediately."
	const heldJSON = `[{"status":"in_progress","assignee":"seat-a","metadata":{"gc.session_id":"mc-session-live"}}]`
	unresolvedMarker := "withheld: a startup dialog was seen (" + trustName + "); the pane was not clear and ready when the clear window ended"
	claimConfig := func() provider.Config {
		cfg := seedConfig
		cfg.Nudge = poolClaim
		cfg.Env = map[string]string{"GC_SESSION_ID": "mc-session-live"}
		return cfg
	}

	for _, tc := range []struct {
		name  string
		args  []string
		reply string
	}{
		{name: "the claim read stalls", args: []string{"hook"}, reply: ""},
		{name: "the bead read stalls", args: []string{"bd"}, reply: "mc-claim-live\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var probeEnd time.Time
			gc := &provider.FakeRunner{Handle: func(ctx context.Context, c provider.RunCall) (string, error) {
				if len(c.Args) > 0 && c.Args[0] == tc.args[0] {
					<-ctx.Done()
					probeEnd = time.Now()
					return "", ctx.Err()
				}
				return tc.reply, nil
			}}
			fake := faketest.New()
			fake.ProgramStatuses = idleProgramStatuses()
			script := &gateScript{}
			fake.FormatCtxFunc = script.hook(func(ctx context.Context, n int) (string, error) {
				if n == 1 {
					return plainEditor, nil // the deadline poll: clear, and idle
				}
				if err := ctx.Err(); err != nil {
					return "", err
				}
				return trustDialogScreen, nil
			})
			stateDir := t.TempDir()
			p := clearProvider(fake, stateDir, provider.WithRunner(gc))
			if err := p.Start(context.Background(), dogName, claimConfig()); err != nil {
				t.Fatalf("Start: %v", err)
			}
			if probeEnd.IsZero() {
				t.Fatal("the claim probe never stalled: the ladder did not reach the pool-claim block")
			}
			if limit := script.firstServed().Add(clearWindow + clearPoll + clearSlack); probeEnd.After(limit) {
				t.Errorf("stalled probe ended %v after the first sighting, want <= %v",
					probeEnd.Sub(script.firstServed()), clearWindow+clearPoll+clearSlack)
			}
			if n, k, u := writeCount(fake), keyCount(fake, "enter"), keyCount(fake, "ctrl+u"); n != 0 || k != 0 || u != 0 {
				t.Errorf("writes %d, enters %d, clears %d; want none", n, k, u)
			}
			if got := readStartupTiming(t, stateDir, dogName)["readiness_outcome"]; got != "dialog-unresolved" {
				t.Errorf("readiness_outcome = %v, want dialog-unresolved", got)
			}
			if got := startupMarker(t, p, dogName); got != unresolvedMarker {
				t.Errorf("marker = %q, want %q", got, unresolvedMarker)
			}
		})
	}

	// Guard, green on e19f548 too: before any sighting capped returns the
	// parent, so the probe runs as before and a live claim refuses the seed.
	t.Run("no sighting: a live claim still refuses the seed", func(t *testing.T) {
		gc := &provider.FakeRunner{Handle: func(_ context.Context, c provider.RunCall) (string, error) {
			if len(c.Args) > 0 && c.Args[0] == "hook" {
				return "mc-claim-live\n", nil
			}
			if len(c.Args) > 0 && c.Args[0] == "bd" {
				return heldJSON, nil
			}
			return "", errors.New("exit status 1")
		}}
		fake, stateDir := faketest.New(), t.TempDir()
		turnStarts(fake)
		fake.FormatFunc = func(rex.BlockID) string { return plainEditor }
		p := clearProvider(fake, stateDir, provider.WithRunner(gc))
		if err := p.Start(context.Background(), dogName, claimConfig()); err != nil {
			t.Fatalf("Start: %v", err)
		}
		if len(gc.Calls()) == 0 {
			t.Fatal("the claim probe never ran")
		}
		if n := writeCount(fake); n != 0 {
			t.Errorf("writes %d, want 0: a live claim refuses the pool-claim seed", n)
		}
		if _, ok := readStartupTiming(t, stateDir, dogName)["dialog"]; ok {
			t.Error("dialog recorded, want none: this start saw no dialog")
		}
	})
}

// TestStartupDialogBetweenWriteAndSubmit pins REQ-001 (b): the pre-submit gate.
func TestStartupDialogBetweenWriteAndSubmit(t *testing.T) {
	t.Run("a dialog appears after the write", func(t *testing.T) {
		stateDir := t.TempDir()
		fake := faketest.New()
		fake.ProgramStatuses = idleProgramStatuses()
		written := watchWrites(fake)
		fake.FormatFunc = func(rex.BlockID) string {
			if written.Load() {
				return trustDialogScreen
			}
			return plainEditor
		}
		p := clearProvider(fake, stateDir, provider.WithStartupNudgeRetries([]time.Duration{time.Millisecond}))
		if err := p.Start(context.Background(), dogName, seedConfig); err != nil {
			t.Fatalf("Start: %v", err)
		}
		if n, k, u := writeCount(fake), keyCount(fake, "enter"), keyCount(fake, "ctrl+u"); n != 1 || k != 0 || u != 0 {
			t.Errorf("writes %d, enters %d, ctrl+u %d; want 1, 0, 0 (no submit, no resend)", n, k, u)
		}
		want := "delivery failed: the seed was typed but not submitted, a startup dialog appeared (" + trustName + ")"
		if got := startupMarker(t, p, dogName); got != want {
			t.Errorf("marker = %q, want %q", got, want)
		}
		timing := readStartupTiming(t, stateDir, dogName)
		if timing["dialog_gate"] != "pre-submit" || timing["readiness_outcome"] != "idle" {
			t.Errorf("dialog_gate %v, readiness_outcome %v; want pre-submit, idle", timing["dialog_gate"], timing["readiness_outcome"])
		}
		if _, ok := timing["dialog_suspect"]; ok {
			t.Errorf("dialog_suspect = %v, want absent: the seed holds no marker text", timing["dialog_suspect"])
		}
	})
	t.Run("a dialog appears after a resend's write", func(t *testing.T) {
		stateDir := t.TempDir()
		fake := faketest.New()
		fake.ProgramStatuses = idleProgramStatuses()
		var writes atomic.Int32
		fake.WriteFunc = func(rex.SessionID, rex.BlockID, string) { writes.Add(1) }
		fake.FormatFunc = func(rex.BlockID) string {
			if writes.Load() >= 2 {
				return trustDialogScreen
			}
			return plainEditor
		}
		p := clearProvider(fake, stateDir, provider.WithStartupNudgeRetries([]time.Duration{time.Millisecond}))
		if err := p.Start(context.Background(), dogName, seedConfig); err != nil {
			t.Fatalf("Start: %v", err)
		}
		if n, k, u := writeCount(fake), keyCount(fake, "enter"), keyCount(fake, "ctrl+u"); n != 2 || k != 1 || u != 1 {
			t.Errorf("writes %d, enters %d, ctrl+u %d; want 2, 1, 1 (the resend is written, one submit, one clear)", n, k, u)
		}
		want := "delivery failed: the seed was typed but not submitted, a startup dialog appeared (" + trustName + ")"
		if got := startupMarker(t, p, dogName); got != want {
			t.Errorf("marker = %q, want %q", got, want)
		}
		timing := readStartupTiming(t, stateDir, dogName)
		if timing["dialog_gate"] != "pre-submit" || timing["readiness_outcome"] != "idle" {
			t.Errorf("dialog_gate %v, readiness_outcome %v; want pre-submit, idle", timing["dialog_gate"], timing["readiness_outcome"])
		}
	})
	t.Run("a parent cancel after the write keeps the readiness outcome", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		stateDir := t.TempDir()
		fake := faketest.New()
		fake.ProgramStatuses = idleProgramStatuses()
		written := watchWrites(fake)
		fake.WriteFunc = func(rex.SessionID, rex.BlockID, string) {
			written.Store(true)
			cancel()
		}
		fake.FormatFunc = func(rex.BlockID) string {
			if written.Load() {
				return trustDialogScreen
			}
			return plainEditor
		}
		p := clearProvider(fake, stateDir, provider.WithStartupNudgeRetries([]time.Duration{time.Millisecond}))
		if err := p.Start(ctx, dogName, seedConfig); err != nil {
			t.Fatalf("Start: %v", err)
		}
		if n, k, u := writeCount(fake), keyCount(fake, "enter"), keyCount(fake, "ctrl+u"); n != 1 || k != 0 || u != 0 {
			t.Errorf("writes %d, enters %d, ctrl+u %d; want 1, 0, 0 (no submit, no clear, no resend)", n, k, u)
		}
		want := "delivery failed: the seed was typed but not submitted, a startup dialog appeared (" + trustName + ")"
		if got := startupMarker(t, p, dogName); got != want {
			t.Errorf("marker = %q, want %q", got, want)
		}
		timing := readStartupTiming(t, stateDir, dogName)
		if timing["dialog_gate"] != "pre-submit" || timing["readiness_outcome"] != "idle" {
			t.Errorf("dialog_gate %v, readiness_outcome %v; want pre-submit, idle", timing["dialog_gate"], timing["readiness_outcome"])
		}
	})
	t.Run("the read fails after a sighting", func(t *testing.T) {
		stateDir := t.TempDir()
		fake := faketest.New()
		fake.ProgramStatuses = idleProgramStatuses()
		written := watchWrites(fake)
		var reads atomic.Int32
		fake.FormatCtxFunc = func(context.Context, rex.BlockID) (string, error) {
			if written.Load() {
				return "", errors.New("rex: format failed")
			}
			if reads.Add(1) == 1 {
				return trustDialogScreen, nil
			}
			return plainEditor, nil
		}
		p := clearProvider(fake, stateDir)
		if err := p.Start(context.Background(), dogName, seedConfig); err != nil {
			t.Fatalf("Start: %v", err)
		}
		if n, k := writeCount(fake), keyCount(fake, "enter"); n != 1 || k != 0 {
			t.Errorf("writes %d, enters %d; want 1, 0", n, k)
		}
		want := "delivery failed: the seed was typed but not submitted, the screen could not be read after a startup dialog was seen (" + trustName + ")"
		if got := startupMarker(t, p, dogName); got != want {
			t.Errorf("marker = %q, want %q", got, want)
		}
		timing := readStartupTiming(t, stateDir, dogName)
		if timing["dialog"] != trustName || timing["readiness_outcome"] != "dialog-cleared" {
			t.Errorf("dialog %v, readiness_outcome %v; want the last sighting and dialog-cleared", timing["dialog"], timing["readiness_outcome"])
		}
		if _, ok := timing["dialog_gate"]; ok {
			t.Errorf("dialog_gate = %v, want absent: a failed read keeps the readiness sighting", timing["dialog_gate"])
		}
	})
	t.Run("a parent cancel during the write does not block the submit", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		fake := faketest.New()
		fake.ProgramStatuses = idleProgramStatuses()
		fake.WriteFunc = func(rex.SessionID, rex.BlockID, string) { cancel() }
		var reads atomic.Int32
		fake.FormatCtxFunc = func(ctx context.Context, _ rex.BlockID) (string, error) {
			if err := ctx.Err(); err != nil {
				return "", err
			}
			if reads.Add(1) == 1 {
				return trustDialogScreen, nil // a sighting, so a failed gate read would stop
			}
			return plainEditor, nil
		}
		p := clearProvider(fake, t.TempDir())
		_ = p.Start(ctx, dogName, seedConfig) // what follows the submit is not under test
		if k := keyCount(fake, "enter"); k != 1 {
			t.Errorf("enters %d, want 1: the pre-submit gate reads under the submit key's context", k)
		}
	})
}

// TestStartupDialogBeforeResend pins REQ-001 (c): the pre-resend gate, before
// the clear key and again before the resend's write.
func TestStartupDialogBeforeResend(t *testing.T) {
	// After the write the screen reads are: 1 pre-submit, 2 pre-resend before
	// ctrl+u, 3 pre-resend before the resend's write. The static idle record
	// never confirms the submit, and the confirmation poll reads no screen.
	run := func(t *testing.T, dialogAt, wantClears int) {
		t.Helper()
		stateDir := t.TempDir()
		fake := faketest.New()
		fake.ProgramStatuses = idleProgramStatuses()
		written := watchWrites(fake)
		after := 0
		fake.FormatFunc = func(rex.BlockID) string {
			if !written.Load() {
				return plainEditor
			}
			after++
			if after == dialogAt {
				return trustDialogScreen
			}
			return plainEditor
		}
		p := clearProvider(fake, stateDir, provider.WithStartupNudgeRetries([]time.Duration{time.Millisecond}))
		if err := p.Start(context.Background(), dogName, seedConfig); err != nil {
			t.Fatalf("Start: %v", err)
		}
		if n, k, u := writeCount(fake), keyCount(fake, "enter"), keyCount(fake, "ctrl+u"); n != 1 || k != 1 || u != wantClears {
			t.Errorf("writes %d, enters %d, ctrl+u %d; want 1, 1, %d", n, k, u, wantClears)
		}
		want := "delivered but not confirmed: a startup dialog appeared (" + trustName + ") before a resend"
		if got := startupMarker(t, p, dogName); got != want {
			t.Errorf("marker = %q, want %q", got, want)
		}
		if got := readStartupTiming(t, stateDir, dogName)["dialog_gate"]; got != "pre-resend" {
			t.Errorf("dialog_gate = %v, want pre-resend", got)
		}
	}
	t.Run("before the clear key", func(t *testing.T) { run(t, 2, 0) })
	t.Run("before the resend's write", func(t *testing.T) { run(t, 3, 1) })
	t.Run("a parent cancel during the resend's gate read", func(t *testing.T) {
		// A dialog was seen during readiness and cleared. The resend's second
		// gate read then fails because the parent is canceled. The first seed
		// was typed and submitted, so the gate's stop must win: the marker
		// names the dialog and the readiness outcome stays dialog-cleared
		// (REQ-001 (c)).
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		stateDir := t.TempDir()
		fake := faketest.New()
		fake.ProgramStatuses = idleProgramStatuses()
		written := watchWrites(fake)
		var first time.Time
		after := 0
		fake.FormatCtxFunc = func(readCtx context.Context, _ rex.BlockID) (string, error) {
			if first.IsZero() {
				first = time.Now()
			}
			if !written.Load() {
				if time.Since(first) < clearWindow {
					return trustDialogScreen, nil
				}
				return plainEditor, nil
			}
			after++
			if after == 3 {
				cancel()
				return "", readCtx.Err()
			}
			return plainEditor, nil
		}
		p := clearProvider(fake, stateDir, provider.WithStartupNudgeRetries([]time.Duration{time.Millisecond}))
		if err := p.Start(ctx, dogName, seedConfig); err != nil {
			t.Fatalf("Start: %v", err)
		}
		if n, k, u := writeCount(fake), keyCount(fake, "enter"), keyCount(fake, "ctrl+u"); n != 1 || k != 1 || u != 1 {
			t.Errorf("writes %d, enters %d, ctrl+u %d; want 1, 1, 1 (no resend's write)", n, k, u)
		}
		want := "delivered but not confirmed: the screen could not be read after a startup dialog was seen (" + trustName + ") before a resend"
		if got := startupMarker(t, p, dogName); got != want {
			t.Errorf("marker = %q, want %q", got, want)
		}
		if got := readStartupTiming(t, stateDir, dogName)["readiness_outcome"]; got != "dialog-cleared" {
			t.Errorf("readiness_outcome = %v, want dialog-cleared", got)
		}
	})
	t.Run("a parent cancel with no sighting on the resend's gate read", func(t *testing.T) {
		// No dialog was seen, so the failed resend read is a plain cancel. The
		// first seed was typed and submitted: the marker must not say "before
		// typing", and the readiness outcome stays idle (REQ-001 (c)).
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		stateDir := t.TempDir()
		fake := faketest.New()
		fake.ProgramStatuses = idleProgramStatuses()
		written := watchWrites(fake)
		after := 0
		fake.FormatCtxFunc = func(readCtx context.Context, _ rex.BlockID) (string, error) {
			if !written.Load() {
				return plainEditor, nil
			}
			after++
			if after == 3 {
				cancel()
				return "", readCtx.Err()
			}
			return plainEditor, nil
		}
		p := clearProvider(fake, stateDir, provider.WithStartupNudgeRetries([]time.Duration{time.Millisecond}))
		if err := p.Start(ctx, dogName, seedConfig); err != nil {
			t.Fatalf("Start: %v", err)
		}
		if n, k, u := writeCount(fake), keyCount(fake, "enter"), keyCount(fake, "ctrl+u"); n != 1 || k != 1 || u != 1 {
			t.Errorf("writes %d, enters %d, ctrl+u %d; want 1, 1, 1 (no resend's write)", n, k, u)
		}
		want := "delivery failed: nudge " + string(dogName) + ": context canceled"
		if got := startupMarker(t, p, dogName); got != want {
			t.Errorf("marker = %q, want %q", got, want)
		}
		if got := readStartupTiming(t, stateDir, dogName)["readiness_outcome"]; got != "idle" {
			t.Errorf("readiness_outcome = %v, want idle", got)
		}
	})
}

// TestStartupSeedQuotingDialogMarker pins REQ-005's seed-ambiguous match: a
// seed that quotes a dialog marker stops at pre-submit with a suspected-dialog
// reason, and a real dialog at that gate is never bypassed.
func TestStartupSeedQuotingDialogMarker(t *testing.T) {
	const seed = "please trust this folder, then run gc prime"
	want := "delivery failed: the seed was typed but not submitted, a startup dialog may have appeared (" + trustName + "); the seed contains its marker text"
	for name, afterWrite := range map[string]string{
		"only the typed seed shows": plainEditor + seed,
		"a real dialog shows too":   trustDialogScreen + "❯ " + seed,
	} {
		t.Run(name, func(t *testing.T) {
			stateDir := t.TempDir()
			fake := faketest.New()
			fake.ProgramStatuses = idleProgramStatuses()
			written := watchWrites(fake)
			fake.FormatFunc = func(rex.BlockID) string {
				if written.Load() {
					return afterWrite
				}
				return plainEditor
			}
			p := clearProvider(fake, stateDir, provider.WithStartupNudgeRetries([]time.Duration{time.Millisecond}))
			if err := p.Start(context.Background(), dogName, provider.Config{Argv: []string{"pi"}, Nudge: seed}); err != nil {
				t.Fatalf("Start: %v", err)
			}
			if n, k, u := writeCount(fake), keyCount(fake, "enter"), keyCount(fake, "ctrl+u"); n != 1 || k != 0 || u != 0 {
				t.Errorf("writes %d, enters %d, ctrl+u %d; want 1, 0, 0 (no resend, no clear)", n, k, u)
			}
			if got := startupMarker(t, p, dogName); got != want {
				t.Errorf("marker = %q, want %q", got, want)
			}
			timing := readStartupTiming(t, stateDir, dogName)
			if timing["dialog_suspect"] != "seed" || timing["dialog_gate"] != "pre-submit" {
				t.Errorf("dialog_suspect %v, dialog_gate %v; want seed, pre-submit", timing["dialog_suspect"], timing["dialog_gate"])
			}
		})
	}
}
