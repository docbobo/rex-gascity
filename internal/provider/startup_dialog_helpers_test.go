package provider_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/docbobo/rex-gascity/internal/provider"
	"github.com/docbobo/rex-gascity/internal/rex"
	"github.com/docbobo/rex-gascity/internal/rex/faketest"
)

// dogName and dogBlock are the session startupProvider's tests launch and the
// block the fake gives it.
const (
	dogName               = "Indigo--dog-1"
	dogBlock  rex.BlockID = "block:dog-1"
	trustName             = "Trust this folder?"
)

// idleProgramStatuses is a program that reported idle once and never changes,
// so a submit is never confirmed by program_status.
func idleProgramStatuses() map[rex.BlockID]rex.ProgramStatus {
	return map[rex.BlockID]rex.ProgramStatus{
		dogBlock: {Records: []rex.ProgramStatusRecord{{State: "idle", UpdatedAt: time.Now()}}},
	}
}

// Timing for the clear-window tests. clearPoll is the nudge poll interval and
// the hard stop's margin; clearSlack absorbs scheduling and stays below one
// poll (AC3).
const (
	clearPoll   = 40 * time.Millisecond
	clearWindow = 300 * time.Millisecond
	clearSlack  = 30 * time.Millisecond
)

// plainEditor is a live composer with no dialog on screen.
const plainEditor = "pi v1\n❯ "

// clearProvider is startupProvider with a state dir, the clear-window poll and
// a short clear window.
func clearProvider(fake rex.Client, stateDir string, opts ...provider.Option) *provider.Provider {
	base := []provider.Option{
		provider.WithNamespace(provider.NewNamespaceForDirs(stateDir)),
		provider.WithNudgeConfirmation(20*time.Millisecond, clearPoll),
		provider.WithStartupDialogClear(clearWindow),
	}
	return startupProvider(fake, append(base, opts...)...)
}

// screenLog records, in call order, whether each screen read served a dialog.
type screenLog struct {
	mu     sync.Mutex
	dialog []bool
}

func (l *screenLog) served(dialog bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.dialog = append(l.dialog, dialog)
}

// assertNoInputIntoDialog walks the fake's block calls in order and fails when
// a write or a key reaches the pane while the latest screen read served a
// dialog (#205 AC2).
func assertNoInputIntoDialog(t *testing.T, fake *faketest.Fake, log *screenLog) {
	t.Helper()
	log.mu.Lock()
	defer log.mu.Unlock()
	reads, showing := 0, false
	for _, c := range fake.BlockCalls {
		switch c.Op {
		case "format":
			if reads < len(log.dialog) {
				showing = log.dialog[reads]
			}
			reads++
		case "write", "send-key":
			if showing {
				t.Errorf("%s reached the pane while a dialog showed: %+v", c.Op, c)
			}
		}
	}
}

// writeCount counts writes; writesOf counts writes of text.
func writeCount(fake *faketest.Fake) int { return writesOf(fake, "") }

func writesOf(fake *faketest.Fake, text string) int {
	n := 0
	for _, c := range fake.BlockCalls {
		if c.Op == "write" && (text == "" || c.Text == text) {
			n++
		}
	}
	return n
}

// keyCount counts single-key presses of key ("enter", "ctrl+u").
func keyCount(fake *faketest.Fake, key string) int {
	n := 0
	for _, c := range fake.SentKeys {
		if len(c.Keys) == 1 && c.Keys[0] == key {
			n++
		}
	}
	return n
}

// watchWrites reports, through the returned flag, whether text was written.
func watchWrites(fake *faketest.Fake) *atomic.Bool {
	var written atomic.Bool
	fake.WriteFunc = func(rex.SessionID, rex.BlockID, string) { written.Store(true) }
	return &written
}

// turnStarts makes the program report idle until the seed is written, then a
// newer working record, so the submit is confirmed. It returns the flag.
func turnStarts(fake *faketest.Fake) *atomic.Bool {
	written := watchWrites(fake)
	at := time.Now()
	fake.ProgramStatusFunc = func(rex.BlockID) rex.ProgramStatus {
		if written.Load() {
			return rex.ProgramStatus{Records: []rex.ProgramStatusRecord{{State: "working", UpdatedAt: at.Add(time.Second)}}}
		}
		return rex.ProgramStatus{Records: []rex.ProgramStatusRecord{{State: "idle", UpdatedAt: at}}}
	}
	return written
}

// startupMarker is the session's GC_STARTUP_DELIVERY_UNCONFIRMED value.
func startupMarker(t *testing.T, p *provider.Provider, name string) string {
	t.Helper()
	got, _ := p.GetMeta(context.Background(), name, provider.StartupDeliveryUnconfirmedMetaKey)
	return got
}
