package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/docbobo/rex-gascity/internal/rex"
	"github.com/docbobo/rex-gascity/internal/rex/faketest"
)

// paneSecret stands in for what a pane shows at a password prompt. A withheld
// op's outcome line and stderr must never carry pane content (#48 REQ-009).
const paneSecret = "[sudo] password for boris: PANE-CONTENT-MARKER"

// secretReadHarness starts dog and puts its pane in a secret read: a cooked
// read with echo off.
func secretReadHarness(t *testing.T) (*harness, *faketest.Fake) {
	t.Helper()
	h, fake := newHarness(t)
	h.start(dog)
	fake.InputModes = map[rex.BlockID]rex.InputMode{"block:dog-1": {Echo: false, Canonical: true, Signals: true}}
	fake.Formats = map[rex.BlockID]string{"block:dog-1": paneSecret}
	return h, fake
}

// needWireOutcome asserts exit 80 with one JSON outcome line carrying word,
// the same word on stderr, the secret-read reason, and no pane content.
func needWireOutcome(t *testing.T, r result, word string) {
	t.Helper()
	if r.code != exitInputOutcome {
		t.Fatalf("exit = %d, want %d (stderr %q)", r.code, exitInputOutcome, r.stderr)
	}
	var line struct {
		Outcome string `json:"outcome"`
		Detail  string `json:"detail"`
	}
	if err := json.Unmarshal([]byte(r.stdout), &line); err != nil {
		t.Fatalf("stdout = %q, want one JSON line: %v", r.stdout, err)
	}
	if line.Outcome != word {
		t.Errorf("outcome = %q, want %q", line.Outcome, word)
	}
	if !strings.Contains(r.stderr, word) || !strings.Contains(r.stderr, "pane is reading a secret") {
		t.Errorf("stderr = %q, want the %q word and the secret-read reason", r.stderr, word)
	}
	if strings.Contains(r.stdout+r.stderr, "PANE-CONTENT-MARKER") {
		t.Errorf("output carries pane content: stdout %q stderr %q", r.stdout, r.stderr)
	}
}

// TestNudgeSecretReadIsPendingInteraction drives REQ-009 through opNudge: a
// nudge into a secret read exits 80 pending_interaction and types nothing.
func TestNudgeSecretReadIsPendingInteraction(t *testing.T) {
	h, fake := secretReadHarness(t)

	needWireOutcome(t, h.run("hello", "nudge", dog), "pending_interaction")
	if len(fake.SentKeys) != 0 {
		t.Errorf("keys sent = %+v, want none", fake.SentKeys)
	}
	for _, c := range fake.BlockCalls {
		if c.Op == "write" {
			t.Errorf("text written into a secret read: %+v", c)
		}
	}
}

// TestSendKeysSecretReadIsNotSent drives REQ-009 through the send-keys op:
// Enter into a secret read exits 80 not_sent; a ctrl+c-only list still
// exits 0 and is sent.
func TestSendKeysSecretReadIsNotSent(t *testing.T) {
	h, fake := secretReadHarness(t)

	needWireOutcome(t, h.run("", "send-keys", dog, "Enter"), "not_sent")
	if len(fake.SentKeys) != 0 {
		t.Fatalf("keys sent = %+v, want none", fake.SentKeys)
	}
	if r := h.run("", "send-keys", dog, "C-c"); r.code != exitOK {
		t.Fatalf("send-keys C-c: exit %d, stderr %q", r.code, r.stderr)
	}
	if len(fake.SentKeys) != 1 || len(fake.SentKeys[0].Keys) != 1 || fake.SentKeys[0].Keys[0] != "ctrl+c" {
		t.Errorf("keys sent = %+v, want one ctrl+c", fake.SentKeys)
	}
}
