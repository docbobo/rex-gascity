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
	p := nudgeStarted(t, fake, provider.WithStateDir(stateDir))

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
