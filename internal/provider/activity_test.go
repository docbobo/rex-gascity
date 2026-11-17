package provider_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/docbobo/rex-gascity/internal/provider"
	"github.com/docbobo/rex-gascity/internal/rex"
	"github.com/docbobo/rex-gascity/internal/rex/faketest"
)

const (
	activityName  = "Indigo--dog-1"
	activityBlock = rex.BlockID("block:dog-1")
)

func TestGetLastActivityPrefersProgramStatus(t *testing.T) {
	fake, p := started(t)
	early := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	late := early.Add(time.Hour)
	fake.ProgramStatuses = map[rex.BlockID]rex.ProgramStatus{
		activityBlock: {Records: []rex.ProgramStatusRecord{
			{ID: "a", State: "idle", UpdatedAt: late},
			{ID: "b", State: "busy", UpdatedAt: early},
		}},
	}
	fake.Formats = map[rex.BlockID]string{activityBlock: "output"}

	got, err := p.GetLastActivity(context.Background(), activityName)
	if err != nil {
		t.Fatalf("GetLastActivity: %v", err)
	}
	if !got.At.Equal(late) || got.Source != provider.ActivityProgramStatus {
		t.Errorf("got %+v, want At=%v Source=program_status", got, late)
	}
}

func TestGetLastActivityFallsBackToFormatPoll(t *testing.T) {
	fake, p := started(t)
	fake.Formats = map[rex.BlockID]string{activityBlock: "output"}

	got, err := p.GetLastActivity(context.Background(), activityName)
	if err != nil {
		t.Fatalf("GetLastActivity: %v", err)
	}
	if got.Source != provider.ActivityFormatPoll {
		t.Errorf("Source = %q, want format_poll", got.Source)
	}
	if got.At.IsZero() {
		t.Error("At is zero, want a timestamp")
	}
}

func TestGetLastActivityUnchangedFormatKeepsTimeChangedAdvances(t *testing.T) {
	fake, p := started(t)
	ctx := context.Background()
	fake.Formats = map[rex.BlockID]string{activityBlock: "one"}

	first, err := p.GetLastActivity(ctx, activityName)
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	time.Sleep(5 * time.Millisecond)
	second, err := p.GetLastActivity(ctx, activityName)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if !second.At.Equal(first.At) {
		t.Errorf("unchanged output: At = %v, want %v", second.At, first.At)
	}

	fake.Formats[activityBlock] = "two"
	third, err := p.GetLastActivity(ctx, activityName)
	if err != nil {
		t.Fatalf("third: %v", err)
	}
	if !third.At.After(first.At) {
		t.Errorf("changed output: At = %v, want after %v", third.At, first.At)
	}
}

func TestGetLastActivitySurvivesNewProviderOverSameStateDir(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	fake := faketest.New()
	p1 := provider.New("acme", fake, provider.WithStateDir(dir))
	if err := p1.Start(ctx, activityName, provider.Config{Argv: []string{"claude"}}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	fake.Formats = map[rex.BlockID]string{activityBlock: "steady"}

	first, err := p1.GetLastActivity(ctx, activityName)
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	time.Sleep(5 * time.Millisecond)
	p2 := provider.New("acme", fake, provider.WithStateDir(dir))
	second, err := p2.GetLastActivity(ctx, activityName)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if !second.At.Equal(first.At) {
		t.Errorf("fresh provider: At = %v, want %v", second.At, first.At)
	}
}

func TestGetLastActivityNoPane(t *testing.T) {
	p := provider.New("acme", faketest.New())

	got, err := p.GetLastActivity(context.Background(), activityName)
	if err != nil {
		t.Fatalf("GetLastActivity: %v", err)
	}
	if got.Source != provider.ActivityNone || !got.At.IsZero() {
		t.Errorf("got %+v, want zero time and source none", got)
	}
}

func TestGetLastActivityProgramStatusErrorPassesThrough(t *testing.T) {
	fake, p := started(t)
	boom := errors.New("transport down")
	fake.ProgramStatusErr = boom

	if _, err := p.GetLastActivity(context.Background(), activityName); !errors.Is(err, boom) {
		t.Errorf("err = %v, want %v", err, boom)
	}
}

func TestGetLastActivityZeroUpdatedAtDoesNotBeatPoll(t *testing.T) {
	fake, p := started(t)
	fake.ProgramStatuses = map[rex.BlockID]rex.ProgramStatus{
		activityBlock: {Records: []rex.ProgramStatusRecord{{ID: "a", State: "idle"}}},
	}
	fake.Formats = map[rex.BlockID]string{activityBlock: "output"}

	got, err := p.GetLastActivity(context.Background(), activityName)
	if err != nil {
		t.Fatalf("GetLastActivity: %v", err)
	}
	if got.Source != provider.ActivityFormatPoll || got.At.IsZero() {
		t.Errorf("got %+v, want format_poll with a timestamp", got)
	}
}

func TestActivityPolicyAndSources(t *testing.T) {
	policy := strings.ToLower(provider.ActivityPolicy)
	for _, want := range []string{"fallback", "osc 7501"} {
		if !strings.Contains(policy, want) {
			t.Errorf("ActivityPolicy %q does not mention %q", provider.ActivityPolicy, want)
		}
	}
	for got, want := range map[provider.ActivitySource]string{
		provider.ActivityProgramStatus: "program_status",
		provider.ActivityFormatPoll:    "format_poll",
		provider.ActivityNone:          "none",
	} {
		if string(got) != want {
			t.Errorf("source = %q, want %q", got, want)
		}
	}
}

func TestGetLastActivityIgnoresRecordFromOtherIdentity(t *testing.T) {
	ctx := context.Background()
	fake := faketest.New()
	store := provider.NewMemoryActivityStore()
	p := provider.New("acme", fake, provider.WithActivityStore(store))
	if err := p.Start(ctx, activityName, provider.Config{Argv: []string{"claude"}}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	fake.Formats = map[rex.BlockID]string{activityBlock: "same"}
	first, err := p.GetLastActivity(ctx, activityName)
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	// Replace the stored record with one from another pane, same fingerprint.
	old := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	rec, _ := store.Load(activityName)
	rec.Identity = "block:old-pane"
	rec.At = old
	store.Save(activityName, rec)

	got, err := p.GetLastActivity(ctx, activityName)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if got.At.Equal(old) || got.At.Before(first.At) {
		t.Errorf("At = %v: inherited the other pane's time", got.At)
	}
	if again, _ := store.Load(activityName); again.Identity != string(activityBlock) {
		t.Errorf("stored identity = %q, want %q", again.Identity, activityBlock)
	}
}
