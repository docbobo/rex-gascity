package provider_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/docbobo/rex-gascity/internal/provider"
	"github.com/docbobo/rex-gascity/internal/rex"
	"github.com/docbobo/rex-gascity/internal/rex/faketest"
)

// started returns a provider with one agent pane already started, so the input
// ops have a binding to resolve. It returns the fake for assertions.
func started(t *testing.T) (*faketest.Fake, *provider.Provider) {
	t.Helper()
	fake := faketest.New()
	p := provider.New("acme", fake)
	if err := p.Start(context.Background(), "Indigo--dog-1", provider.Config{Argv: []string{"claude"}}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	return fake, p
}

// ops returns the recorded block-method ops in order.
func ops(fake *faketest.Fake) []string {
	got := make([]string, len(fake.BlockCalls))
	for i, c := range fake.BlockCalls {
		got[i] = c.Op
	}
	return got
}

// hasOpsInOrder reports whether want appears in got as a subsequence.
func hasOpsInOrder(got, want []string) bool {
	i := 0
	for _, op := range got {
		if i < len(want) && op == want[i] {
			i++
		}
	}
	return i == len(want)
}

func TestNudgeWritesTextThenSubmits(t *testing.T) {
	fake := faketest.New()
	p := nudgeStarted(t, fake)

	if err := p.Nudge(context.Background(), "Indigo--dog-1", "hello agent"); err != nil {
		t.Fatalf("Nudge: %v", err)
	}

	// The liveness probe and program-status read run first, then the text,
	// then the submit.
	if got, want := ops(fake), []string{"process", "write", "send-key"}; !hasOpsInOrder(got, want) {
		t.Errorf("block ops = %v, want %v in order", got, want)
	}
	if len(fake.SentKeys) != 1 {
		t.Fatalf("send-keys calls = %d, want 1", len(fake.SentKeys))
	}
	sent := fake.SentKeys[0]
	if sent.SessionID != "session:gc:acme" || sent.BlockID != "block:dog-1" {
		t.Errorf("send-keys scope = %s/%s, want session:gc:acme/block:dog-1", sent.SessionID, sent.BlockID)
	}
	if len(sent.Keys) != 1 || sent.Keys[0] != "enter" {
		t.Errorf("submit keys = %v, want [enter]", sent.Keys)
	}
}

func TestNudgeFailsWhenSubmitIsNotDelivered(t *testing.T) {
	fake := faketest.New()
	p := nudgeStarted(t, fake)
	fake.SendKeysErr = errors.New("pane vanished")

	err := p.Nudge(context.Background(), "Indigo--dog-1", "hello")
	if err == nil {
		t.Fatal("Nudge = nil, want an error when the submit key is not delivered")
	}
	if !strings.Contains(err.Error(), "submit") {
		t.Errorf("error = %q, want it to name the failed submit", err)
	}
	// The text was typed; only the submit failed.
	if got, want := ops(fake), []string{"process", "write", "send-key"}; !hasOpsInOrder(got, want) {
		t.Errorf("block ops = %v, want %v in order", got, want)
	}
}

func TestNudgeTypingFailureSkipsSubmit(t *testing.T) {
	fake := faketest.New()
	p := nudgeStarted(t, fake)
	fake.WriteErr = errors.New("write failed")

	err := p.Nudge(context.Background(), "Indigo--dog-1", "hello")
	if err == nil {
		t.Fatal("Nudge = nil, want an error when typing fails")
	}
	// Enter must not be sent when the text never landed: the pane would hold a
	// half-typed line.
	if got := ops(fake); !slices.Contains(got, "write") || slices.Contains(got, "send-key") {
		t.Errorf("block ops = %v, want write and no send-key", got)
	}
}

func TestNudgeProcessErrorPassesThrough(t *testing.T) {
	fake, p := started(t)
	fake.ProcessErr = errors.New("rex unreachable")

	err := p.Nudge(context.Background(), "Indigo--dog-1", "hello")
	if err == nil {
		t.Fatal("Nudge = nil, want the observation error")
	}
	// A transport or observation error is not a missing session: masking it
	// would tell the caller the agent is gone when Rex is merely unreachable.
	if errors.Is(err, provider.ErrSessionNotFound) {
		t.Errorf("error = %v, want the transport error, not ErrSessionNotFound", err)
	}
	if !strings.Contains(err.Error(), "rex unreachable") {
		t.Errorf("error = %q, want it to carry the transport error", err)
	}
}

func TestNudgeEmptyTextIsANoOp(t *testing.T) {
	fake, p := started(t)

	if err := p.Nudge(context.Background(), "Indigo--dog-1", ""); err != nil {
		t.Fatalf("Nudge: %v", err)
	}
	// Nothing to deliver: no liveness probe, no text, no submit.
	if len(fake.BlockCalls) != 0 {
		t.Errorf("block ops = %v, want none", ops(fake))
	}
}

func TestNudgeExitedPaneFails(t *testing.T) {
	fake, p := started(t)
	fake.Processes = map[rex.BlockID]rex.ProcessInfo{
		"block:dog-1": {LastExit: &rex.ExitStatus{ExitCode: 0}},
	}

	err := p.Nudge(context.Background(), "Indigo--dog-1", "hello")
	if !errors.Is(err, provider.ErrSessionNotFound) {
		t.Fatalf("Nudge error = %v, want ErrSessionNotFound", err)
	}
	if got, want := ops(fake), []string{"process"}; !slices.Equal(got, want) {
		t.Errorf("block ops = %v, want %v (no input to a dead pane)", got, want)
	}
}

func TestNudgeUnknownSessionFails(t *testing.T) {
	fake := faketest.New()
	p := provider.New("acme", fake)

	err := p.Nudge(context.Background(), "Indigo--ghost", "hello")
	if !errors.Is(err, provider.ErrSessionNotFound) {
		t.Fatalf("Nudge error = %v, want ErrSessionNotFound", err)
	}
}

func TestNudgeClosedPaneFails(t *testing.T) {
	_, p := started(t)
	if err := p.Stop(context.Background(), "Indigo--dog-1"); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	err := p.Nudge(context.Background(), "Indigo--dog-1", "hello")
	if !errors.Is(err, provider.ErrSessionNotFound) {
		t.Fatalf("Nudge error = %v, want ErrSessionNotFound after the pane closed", err)
	}
}

func TestSendKeysRecordsInputFloor(t *testing.T) {
	fake := faketest.New()
	store := provider.NewMemoryActivityStore()
	p := provider.New("acme", fake, provider.WithActivityStore(store))
	if err := p.Start(context.Background(), activityName, provider.Config{Argv: []string{"claude"}}); err != nil {
		t.Fatalf("Start: %v", err)
	}

	if err := p.SendKeys(context.Background(), activityName, "Enter"); err != nil {
		t.Fatalf("SendKeys: %v", err)
	}
	rec, ok := store.Load(activityName)
	if !ok || rec.InputAt.IsZero() {
		t.Fatalf("activity record = %+v ok=%v, want a non-zero input floor", rec, ok)
	}
}

func TestSendKeysTranslatesKeyNames(t *testing.T) {
	fake, p := started(t)

	if err := p.SendKeys(context.Background(), "Indigo--dog-1", "C-c", "Enter", "Up"); err != nil {
		t.Fatalf("SendKeys: %v", err)
	}
	if len(fake.SentKeys) != 1 {
		t.Fatalf("send-keys calls = %d, want 1", len(fake.SentKeys))
	}
	got := fake.SentKeys[0].Keys
	want := []string{"ctrl+c", "enter", "up"}
	if !slices.Equal(got, want) {
		t.Errorf("keys = %v, want %v", got, want)
	}
}

func TestSendKeysRejectsBadKeyBeforeLookup(t *testing.T) {
	fake, p := started(t)

	err := p.SendKeys(context.Background(), "Indigo--dog-1", "!")
	if !errors.Is(err, provider.ErrKeyRejected) {
		t.Fatalf("SendKeys error = %v, want ErrKeyRejected", err)
	}
	// The rejection precedes the pane lookup and any Rex call.
	if len(fake.BlockCalls) != 0 {
		t.Errorf("block ops = %v, want none for a rejected key", ops(fake))
	}
	if len(fake.SentKeys) != 0 {
		t.Errorf("keys sent = %v, want none", fake.SentKeys)
	}
}

func TestSendKeysSplitsLiteralText(t *testing.T) {
	fake, p := started(t)

	if err := p.SendKeys(context.Background(), "Indigo--dog-1", "ok go"); err != nil {
		t.Fatalf("SendKeys: %v", err)
	}
	if len(fake.SentKeys) != 1 {
		t.Fatalf("send-keys calls = %d, want 1", len(fake.SentKeys))
	}
	got := fake.SentKeys[0].Keys
	want := []string{"o", "k", "space", "g", "o"}
	if !slices.Equal(got, want) {
		t.Errorf("keys = %v, want %v", got, want)
	}
}

func TestSendKeysEmptyIsANoOp(t *testing.T) {
	fake, p := started(t)

	if err := p.SendKeys(context.Background(), "Indigo--dog-1"); err != nil {
		t.Fatalf("SendKeys: %v", err)
	}
	if len(fake.BlockCalls) != 0 {
		t.Errorf("block ops = %v, want none for an empty key list", ops(fake))
	}
}

func TestSendKeysExitedPaneFails(t *testing.T) {
	fake, p := started(t)
	fake.Processes = map[rex.BlockID]rex.ProcessInfo{
		"block:dog-1": {LastExit: &rex.ExitStatus{ExitCode: 1}},
	}

	err := p.SendKeys(context.Background(), "Indigo--dog-1", "Enter")
	if !errors.Is(err, provider.ErrSessionNotFound) {
		t.Fatalf("SendKeys error = %v, want ErrSessionNotFound", err)
	}
	if len(fake.SentKeys) != 0 {
		t.Errorf("send-keys calls = %v, want none", fake.SentKeys)
	}
}

func TestInterruptSendsCtrlC(t *testing.T) {
	fake, p := started(t)

	if err := p.Interrupt(context.Background(), "Indigo--dog-1"); err != nil {
		t.Fatalf("Interrupt: %v", err)
	}
	if len(fake.SentKeys) != 1 {
		t.Fatalf("send-keys calls = %d, want 1", len(fake.SentKeys))
	}
	sent := fake.SentKeys[0]
	if sent.SessionID != "session:gc:acme" || sent.BlockID != "block:dog-1" {
		t.Errorf("interrupt scope = %s/%s, want session:gc:acme/block:dog-1", sent.SessionID, sent.BlockID)
	}
	if len(sent.Keys) != 1 || sent.Keys[0] != "ctrl+c" {
		t.Errorf("interrupt keys = %v, want [ctrl+c]", sent.Keys)
	}
}

func TestInterruptUnknownSessionFails(t *testing.T) {
	fake := faketest.New()
	p := provider.New("acme", fake)

	err := p.Interrupt(context.Background(), "Indigo--ghost")
	if !errors.Is(err, provider.ErrSessionNotFound) {
		t.Fatalf("Interrupt error = %v, want ErrSessionNotFound", err)
	}
	if len(fake.SentKeys) != 0 {
		t.Errorf("send-keys calls = %v, want none", fake.SentKeys)
	}
}

func TestInterruptExitedPaneFails(t *testing.T) {
	fake, p := started(t)
	fake.Processes = map[rex.BlockID]rex.ProcessInfo{
		"block:dog-1": {LastExit: &rex.ExitStatus{ExitCode: 0}},
	}

	err := p.Interrupt(context.Background(), "Indigo--dog-1")
	if !errors.Is(err, provider.ErrSessionNotFound) {
		t.Fatalf("Interrupt error = %v, want ErrSessionNotFound", err)
	}
}
