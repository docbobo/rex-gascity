package provider_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/docbobo/rex-gascity/internal/provider"
	"github.com/docbobo/rex-gascity/internal/rex"
	"github.com/docbobo/rex-gascity/internal/rex/faketest"
)

// nudgeStarted starts one pane with a short confirmation window so the
// timeout tests do not sleep for the production default.
func nudgeStarted(t *testing.T, fake rex.Client, opts ...provider.Option) *provider.Provider {
	t.Helper()
	opts = append(opts, provider.WithNudgeConfirmation(20*time.Millisecond, time.Millisecond))
	p := provider.New("acme", fake, opts...)
	if err := p.Start(context.Background(), activityName, provider.Config{Argv: []string{"claude"}}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	// Start now performs a post-launch liveness read; these tests measure the
	// Nudge op's own Rex calls, so the setup's are cleared.
	if f, ok := fake.(*faketest.Fake); ok {
		f.BlockCalls = nil
	}
	return p
}

// statusFunc returns a scripted ProgramStatusFunc: call n (1-based) returns
// records[n-1], and every later call returns the last record.
func statusFunc(records ...rex.ProgramStatusRecord) func(rex.BlockID) rex.ProgramStatus {
	var calls int
	return func(rex.BlockID) rex.ProgramStatus {
		calls++
		i := calls - 1
		if i >= len(records) {
			i = len(records) - 1
		}
		return rex.ProgramStatus{Records: []rex.ProgramStatusRecord{records[i]}}
	}
}

// TestNudgePoolClaimGuardFollowsClaimLiveness proves the provider's delivery
// guard follows gc's live-claim predicate, not just the stamp. gc's
// stalled-claim backstop can re-arm the fallback pool-claim nudge from a stale
// snapshot; while the target session still holds the stamped bead the provider
// refuses to type it, and nothing is typed so a retry cannot clobber the
// in-flight task. Once the stamped bead is closed, released, or owned by
// another session the claim is not held and the nudge is delivered, so a stale
// stamp cannot suppress it forever (GCREX-33 AC #4).
func TestNudgePoolClaimGuardFollowsClaimLiveness(t *testing.T) {
	const poolClaim = "Run gc hook --claim --drain-ack --json now; if it returns work, execute it immediately."
	workDir := t.TempDir()
	const gcBin = "/fake/bin/gc"
	t.Setenv("GC_BIN", gcBin)

	cases := []struct {
		name     string
		claim    string
		beadJSON string
		held     bool
	}{
		{
			name:     "live claim is refused",
			claim:    "mc-claim-live",
			beadJSON: `[{"status":"in_progress","assignee":"seat-a","metadata":{"gc.session_id":"mc-session-live"}}]`,
			held:     true,
		},
		{
			name:     "closed claim is delivered",
			claim:    "mc-claim-live",
			beadJSON: `[{"status":"closed","assignee":"seat-a","metadata":{"gc.session_id":"mc-session-live"}}]`,
		},
		{
			name:     "released claim is delivered",
			claim:    "mc-claim-live",
			beadJSON: `[{"status":"open","assignee":"","metadata":{}}]`,
		},
		{
			name:     "foreign claim is delivered",
			claim:    "mc-claim-live",
			beadJSON: `[{"status":"in_progress","assignee":"seat-b","metadata":{"gc.session_id":"mc-other-session"}}]`,
		},
		{
			name:     "missing claim bead is delivered",
			claim:    "mc-claim-live",
			beadJSON: `[]`,
		},
		{
			name:     "no claim stamp is delivered",
			claim:    "",
			beadJSON: `[]`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := faketest.New()
			gc := &provider.FakeRunner{Handle: func(_ context.Context, c provider.RunCall) (string, error) {
				switch strings.Join(c.Args[:2], " ") {
				case "hook current":
					if tc.claim != "" {
						return tc.claim + "\n", nil
					}
					return "", errors.New("exit status 1")
				case "bd list":
					return tc.beadJSON, nil
				}
				return "", errors.New("exit status 1")
			}}

			p := provider.New("acme", fake,
				provider.WithNudgeConfirmation(20*time.Millisecond, time.Millisecond),
				provider.WithRunner(gc))
			if err := p.Start(context.Background(), activityName, provider.Config{
				Argv:    []string{"claude"},
				WorkDir: workDir,
				Env:     map[string]string{"GC_SESSION_ID": "mc-session-live"},
			}); err != nil {
				t.Fatalf("Start: %v", err)
			}

			err := p.Nudge(context.Background(), activityName, poolClaim)
			// Every probe runs gc from the seat's work dir, as the seat's session.
			for _, c := range gc.Calls() {
				if c.Name != gcBin || c.Dir != workDir || !slices.Contains(c.Env, "GC_SESSION_ID=mc-session-live") {
					t.Errorf("gc probe = %+v, want %s in %s as session mc-session-live", c, gcBin, workDir)
				}
			}
			got := ops(fake)
			typed := slices.Contains(got, "write") || slices.Contains(got, "send-key")
			if tc.held {
				if !errors.Is(err, provider.ErrNudgeClaimHeld) {
					t.Fatalf("Nudge error = %v, want ErrNudgeClaimHeld", err)
				}
				if typed {
					t.Errorf("block ops = %v, want no input while a claim is held", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Nudge error = %v, want delivery", err)
			}
			if !typed {
				t.Errorf("block ops = %v, want the nudge typed", got)
			}
		})
	}
}

func TestNudgeRecordsInputFloor(t *testing.T) {
	fake := faketest.New()
	store := provider.NewMemoryActivityStore()
	p := nudgeStarted(t, fake, provider.WithActivityStore(store))

	if err := p.Nudge(context.Background(), activityName, "hello"); err != nil {
		t.Fatalf("Nudge: %v", err)
	}
	rec, ok := store.Load(activityName)
	if !ok || rec.InputAt.IsZero() {
		t.Fatalf("activity record = %+v ok=%v, want a non-zero input floor", rec, ok)
	}
	if rec.Identity != string(activityBlock) {
		t.Errorf("identity = %q, want %q", rec.Identity, activityBlock)
	}
}

func TestNudgeRefusesWorkingPane(t *testing.T) {
	fake := faketest.New()
	fake.ProgramStatuses = map[rex.BlockID]rex.ProgramStatus{
		activityBlock: {Records: []rex.ProgramStatusRecord{{ID: "a", State: "working"}}},
	}
	p := nudgeStarted(t, fake)

	err := p.Nudge(context.Background(), activityName, "hello")
	if !errors.Is(err, provider.ErrNudgeBusy) {
		t.Fatalf("Nudge error = %v, want ErrNudgeBusy", err)
	}
	// A refused nudge types nothing, so gc's retry cannot duplicate input.
	if got, want := ops(fake), []string{"process", "program_status"}; !equal(got, want) {
		t.Errorf("block ops = %v, want %v (no input into a working pane)", got, want)
	}
	if len(fake.SentKeys) != 0 {
		t.Errorf("send-keys calls = %d, want 0", len(fake.SentKeys))
	}
}

func TestNudgeBlockedSurfacesKind(t *testing.T) {
	fake := faketest.New()
	fake.ProgramStatuses = map[rex.BlockID]rex.ProgramStatus{
		activityBlock: {Records: []rex.ProgramStatusRecord{{ID: "a", State: "blocked", Kind: "permission"}}},
	}
	p := nudgeStarted(t, fake)

	err := p.Nudge(context.Background(), activityName, "hello")
	if !errors.Is(err, provider.ErrNudgeBlocked) {
		t.Fatalf("Nudge error = %v, want ErrNudgeBlocked", err)
	}
	if !strings.Contains(err.Error(), "permission") {
		t.Errorf("error = %q, want it to name the blocked kind", err)
	}
	if got, want := ops(fake), []string{"process", "program_status"}; !equal(got, want) {
		t.Errorf("block ops = %v, want %v (no input over a dialog)", got, want)
	}
}

func TestNudgeImmediateOverridesWorkingGate(t *testing.T) {
	fake := faketest.New()
	early := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	fake.ProgramStatusFunc = statusFunc(
		rex.ProgramStatusRecord{ID: "a", State: "working", UpdatedAt: early},
		rex.ProgramStatusRecord{ID: "a", State: "working", UpdatedAt: early.Add(time.Second)},
	)
	p := nudgeStarted(t, fake)

	report, err := p.NudgeReport(context.Background(), activityName, "hello", provider.NudgeModeImmediate)
	if err != nil {
		t.Fatalf("NudgeReport: %v", err)
	}
	if !report.Delivered || !report.Confirmed {
		t.Errorf("report = %+v, want delivered and confirmed by the newer updated_at", report)
	}
	if got, want := ops(fake)[:3], []string{"process", "program_status", "write"}; !equal(got, want) {
		t.Errorf("first ops = %v, want %v (override types immediately)", got, want)
	}
}

func TestNudgeConfirmsIdleToWorking(t *testing.T) {
	fake := faketest.New()
	early := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	fake.ProgramStatusFunc = statusFunc(
		rex.ProgramStatusRecord{ID: "a", State: "idle", UpdatedAt: early},
		rex.ProgramStatusRecord{ID: "a", State: "working", UpdatedAt: early.Add(time.Second)},
	)
	p := nudgeStarted(t, fake)

	report, err := p.NudgeReport(context.Background(), activityName, "hello", provider.NudgeModeWait)
	if err != nil {
		t.Fatalf("NudgeReport: %v", err)
	}
	if !report.Delivered || !report.Confirmed {
		t.Errorf("report = %+v, want delivered and confirmed by idle->working", report)
	}
	if report.Source != provider.NudgeSourceProgramStatus {
		t.Errorf("Source = %q, want program_status", report.Source)
	}
	if report.State != "working" {
		t.Errorf("State = %q, want working", report.State)
	}
}

func TestNudgeConfirmationTimesOutWithoutError(t *testing.T) {
	fake := faketest.New()
	early := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	fake.ProgramStatuses = map[rex.BlockID]rex.ProgramStatus{
		activityBlock: {Records: []rex.ProgramStatusRecord{{ID: "a", State: "idle", UpdatedAt: early}}},
	}
	p := nudgeStarted(t, fake)

	report, err := p.NudgeReport(context.Background(), activityName, "hello", provider.NudgeModeWait)
	if err != nil {
		t.Fatalf("NudgeReport = %v, want nil after a delivered nudge", err)
	}
	if !report.Delivered || report.Confirmed {
		t.Errorf("report = %+v, want delivered and unconfirmed", report)
	}
	// Unconfirmed is not a delivery failure: an error here would make gc retype
	// the message on its next attempt.
	if len(fake.SentKeys) != 1 {
		t.Errorf("send-keys calls = %d, want 1", len(fake.SentKeys))
	}
}

func TestNudgeWithoutProgramStatusFallsBackToFormatPoll(t *testing.T) {
	fake := faketest.New()
	var calls int
	fake.FormatFunc = func(rex.BlockID) string {
		calls++
		if calls == 1 {
			return "prompt>"
		}
		return "prompt> hello"
	}
	p := nudgeStarted(t, fake)

	report, err := p.NudgeReport(context.Background(), activityName, "hello", provider.NudgeModeWait)
	if err != nil {
		t.Fatalf("NudgeReport: %v", err)
	}
	if !report.Delivered || !report.Confirmed {
		t.Errorf("report = %+v, want delivered and confirmed by the format change", report)
	}
	if report.Source != provider.NudgeSourceFormatPoll {
		t.Errorf("Source = %q, want format_poll", report.Source)
	}
}

func TestNudgeUnconfirmedWritesDiagnostic(t *testing.T) {
	fake := faketest.New()
	stateDir := t.TempDir()
	p := nudgeStarted(t, fake, provider.WithNamespace(provider.NewNamespaceForDirs(stateDir)))

	report, err := p.NudgeReport(context.Background(), activityName, "hello", provider.NudgeModeWait)
	if err != nil {
		t.Fatalf("NudgeReport: %v", err)
	}
	if !report.Delivered || report.Confirmed {
		t.Fatalf("report = %+v, want delivered and unconfirmed", report)
	}
	path := filepath.Join(stateDir, "diagnostics", "Indigo--dog-1-nudge-unconfirmed.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read diagnostic: %v", err)
	}
	if !strings.Contains(string(data), `"session":"Indigo--dog-1"`) {
		t.Errorf("diagnostic = %s, want the session name", data)
	}
}

// equal reports whether two string slices hold the same values in order.
func equal(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// blockingWriteFake models a Rex write that never completes: it holds the
// call until the provider's submit budget expires, then returns the context
// error. It proves the write is bounded rather than waiting for gc's kill.
type blockingWriteFake struct {
	*faketest.Fake
	started chan struct{}
}

func (f *blockingWriteFake) BlockWrite(ctx context.Context, _ rex.SessionID, _ rex.BlockID, _ string) error {
	close(f.started)
	<-ctx.Done()
	return ctx.Err()
}

func TestNudgeCanceledContextWritesNothing(t *testing.T) {
	fake := faketest.New()
	p := nudgeStarted(t, fake)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := p.Nudge(ctx, activityName, "hello")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Nudge error = %v, want context.Canceled", err)
	}
	if got := ops(fake); slices.Contains(got, "write") || slices.Contains(got, "send-key") {
		t.Errorf("block ops = %v, want no input on a cancelled context", got)
	}
}

func TestNudgeEnterFailureAfterWriteIsReported(t *testing.T) {
	fake := faketest.New()
	p := nudgeStarted(t, fake)
	fake.SendKeysErr = errors.New("pane vanished")

	err := p.Nudge(context.Background(), activityName, "hello")
	if err == nil {
		t.Fatal("Nudge = nil, want the failed submit reported")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "typed") {
		t.Errorf("error = %q, want it to say the text was typed but not submitted", err)
	}
	if !slices.Contains(ops(fake), "write") {
		t.Errorf("block ops = %v, want the text write before the failed submit", ops(fake))
	}
	got, err := p.GetMeta(context.Background(), activityName, provider.NudgeSubmitFailedMetaKey)
	if err != nil {
		t.Fatalf("GetMeta: %v", err)
	}
	if got == "" {
		t.Errorf("meta %s is empty, want the failed submit recorded", provider.NudgeSubmitFailedMetaKey)
	}

	// A later successful submit clears the stale failure marker.
	fake.SendKeysErr = nil
	fake.ProgramStatuses = map[rex.BlockID]rex.ProgramStatus{
		activityBlock: {Records: []rex.ProgramStatusRecord{{ID: "a", State: "idle"}}},
	}
	if err := p.Nudge(context.Background(), activityName, "hello"); err != nil {
		t.Fatalf("second Nudge: %v", err)
	}
	if got, _ := p.GetMeta(context.Background(), activityName, provider.NudgeSubmitFailedMetaKey); got != "" {
		t.Errorf("meta %s = %q after a successful submit, want cleared", provider.NudgeSubmitFailedMetaKey, got)
	}
}

func TestNudgeWriteIsBoundedByTheSubmitBudget(t *testing.T) {
	base := faketest.New()
	fake := &blockingWriteFake{Fake: base, started: make(chan struct{})}
	p := nudgeStarted(t, fake, provider.WithNudgeSubmitBudget(50*time.Millisecond))

	start := time.Now()
	err := p.Nudge(context.Background(), activityName, "hello")
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("Nudge = nil, want the expired write budget reported")
	}
	if elapsed > 2*time.Second {
		t.Errorf("Nudge took %v, want it bounded by the 50ms submit budget", elapsed)
	}
	if len(base.SentKeys) != 0 {
		t.Errorf("send-keys calls = %d, want 0 after a failed write", len(base.SentKeys))
	}
}

// needOutcome asserts err carries an RPP input outcome with the given word.
func needOutcome(t *testing.T, err error, word string) *provider.InputOutcomeError {
	t.Helper()
	var outcome *provider.InputOutcomeError
	if !errors.As(err, &outcome) {
		t.Fatalf("error = %v, want an *InputOutcomeError", err)
	}
	if outcome.Outcome != word {
		t.Fatalf("outcome = %q, want %q", outcome.Outcome, word)
	}
	return outcome
}

// TestNudgeBusyIsBusyOutcome pins the typed word for a working pane: the pane
// is refused before any input, and the error still wraps ErrNudgeBusy for
// callers that classify it structurally.
func TestNudgeBusyIsBusyOutcome(t *testing.T) {
	fake := faketest.New()
	fake.ProgramStatuses = map[rex.BlockID]rex.ProgramStatus{
		activityBlock: {Records: []rex.ProgramStatusRecord{{ID: "a", State: "working"}}},
	}
	p := nudgeStarted(t, fake)

	_, err := p.NudgeReport(context.Background(), activityName, "hello", provider.NudgeModeWait)
	needOutcome(t, err, "busy")
	if !errors.Is(err, provider.ErrNudgeBusy) {
		t.Errorf("error = %v, want it to still wrap ErrNudgeBusy", err)
	}
	if len(fake.SentKeys) != 0 {
		t.Errorf("send-keys calls = %d, want 0 into a working pane", len(fake.SentKeys))
	}
}

// TestNudgeBlockedIsPendingInteraction pins the word for a pane waiting on a
// dialog; the detail still names the blocked kind.
func TestNudgeBlockedIsPendingInteraction(t *testing.T) {
	fake := faketest.New()
	fake.ProgramStatuses = map[rex.BlockID]rex.ProgramStatus{
		activityBlock: {Records: []rex.ProgramStatusRecord{{ID: "a", State: "blocked", Kind: "permission"}}},
	}
	p := nudgeStarted(t, fake)

	_, err := p.NudgeReport(context.Background(), activityName, "hello", provider.NudgeModeWait)
	outcome := needOutcome(t, err, "pending_interaction")
	if !errors.Is(err, provider.ErrNudgeBlocked) {
		t.Errorf("error = %v, want it to still wrap ErrNudgeBlocked", err)
	}
	if !strings.Contains(outcome.Detail, "permission") {
		t.Errorf("detail = %q, want it to name the blocked kind", outcome.Detail)
	}
}

// TestNudgeUnreachableLookupIsUnavailable: a server that cannot be listed is
// unavailable, not a plain exit 1.
func TestNudgeUnreachableLookupIsUnavailable(t *testing.T) {
	fake := faketest.New()
	// Do not start a pane: a live binding resolves from disk, so the list call
	// (and thus the unreachable server) is only exercised for an unknown name.
	p := provider.New("acme", fake, provider.WithNudgeConfirmation(20*time.Millisecond, time.Millisecond))
	fake.ListSessionsErr = rex.ErrUnavailable

	_, err := p.NudgeReport(context.Background(), activityName, "hello", provider.NudgeModeWait)
	needOutcome(t, err, "unavailable")
}

// TestNudgeWriteFailureIsNotSent: a write that fails before any submit typed
// nothing usable, so it is not_sent.
func TestNudgeWriteFailureIsNotSent(t *testing.T) {
	fake := faketest.New()
	p := nudgeStarted(t, fake)
	fake.WriteErr = errors.New("rex: protocol error")

	_, err := p.NudgeReport(context.Background(), activityName, "hello", provider.NudgeModeWait)
	needOutcome(t, err, "not_sent")
}

// TestNudgeWriteUnavailableIsUnavailable keeps the unreachable-server word on
// the write path.
func TestNudgeWriteUnavailableIsUnavailable(t *testing.T) {
	fake := faketest.New()
	p := nudgeStarted(t, fake)
	fake.WriteErr = rex.ErrUnavailable

	_, err := p.NudgeReport(context.Background(), activityName, "hello", provider.NudgeModeWait)
	needOutcome(t, err, "unavailable")
}

// TestNudgeSubmitFailureIsNotSent: the text reached the pane but the submit
// did not, so the send was cut short.
func TestNudgeSubmitFailureIsNotSent(t *testing.T) {
	fake := faketest.New()
	p := nudgeStarted(t, fake)
	fake.SendKeysErr = errors.New("pane vanished")

	_, err := p.NudgeReport(context.Background(), activityName, "hello", provider.NudgeModeWait)
	outcome := needOutcome(t, err, "not_sent")
	if !strings.Contains(outcome.Detail, "typed") {
		t.Errorf("detail = %q, want it to say the text was typed", outcome.Detail)
	}
}
