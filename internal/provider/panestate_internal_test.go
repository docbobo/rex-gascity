package provider

import (
	"errors"
	"slices"
	"testing"
)

// paneQuestion names the pane-state entry point a TestPaneStateVerdict row asks.
type paneQuestion string

const (
	askIdle paneQuestion = "idle" // idleVerdict
)

// errScreenRead is the read error a failing screen fixture returns.
var errScreenRead = errors.New("screen read failed")

// screenFixture is the screen a row's observation would read.
type screenFixture struct {
	text string
	err  error
}

// screenText is a screen that reads as text.
func screenText(text string) screenFixture { return screenFixture{text: text} }

// screenFails is a screen whose read fails.
func screenFails() screenFixture { return screenFixture{err: errScreenRead} }

// observe builds an observation over the fixture and a counter of its reads.
func (f screenFixture) observe(status programStatusSnapshot) (paneObservation, *int) {
	reads := new(int)
	return paneObservation{
		status: status,
		screen: newScreenRead(func() (string, error) {
			*reads++
			return f.text, f.err
		}),
	}, reads
}

// noRecord is an observation with no program_status record.
var noRecord = programStatusSnapshot{}

// rec is a program_status record with state and a zero timestamp.
func rec(state string) programStatusSnapshot {
	return programStatusSnapshot{Present: true, State: state}
}

// verdictRow is one TestPaneStateVerdict row: one observation, one question,
// the wanted verdict and the wanted number of screen reads.
type verdictRow struct {
	name   string
	ask    paneQuestion
	status programStatusSnapshot // the observation's status
	screen screenFixture         // the observation's screen

	want      paneVerdict
	wantErr   bool // idle: the screen read error is returned
	wantReads int  // screen reads of the observation
}

// Screens shared by several rows.
const (
	trustDialog      = "Do you trust the files in this folder?\n❯ 1. Yes, proceed\n  2. No, exit\n"
	busyScreen       = "working\nesc to interrupt\n"
	prefixCursorMenu = "rate limit\nREADY> 1. Retry\n  2. Wait\n"
)

// idleRows are REQ-008: idleVerdict, today's IdleSnapshot + screenIdleState.
var idleRows = []verdictRow{
	{name: "idle: record idle", ask: askIdle, status: rec("idle"), screen: screenText("booting\n"), want: verdictIdle, wantReads: 0},
	{name: "idle: record working", ask: askIdle, status: rec("working"), screen: screenText("❯ "), want: verdictBusy, wantReads: 0},
	{name: "idle: record blocked", ask: askIdle, status: rec("blocked"), screen: screenText("❯ "), want: verdictBusy, wantReads: 0},
	{name: "idle: record done is no idle boundary", ask: askIdle, status: rec("done"), screen: screenText("❯ "), want: verdictWaiting, wantReads: 0},
	{name: "idle: no record, read fails", ask: askIdle, status: noRecord, screen: screenFails(), wantErr: true, wantReads: 1},
	{name: "idle: no record, empty screen", ask: askIdle, status: noRecord, screen: screenText(""), want: verdictWaiting, wantReads: 1},
	{name: "idle: no record, startup dialog", ask: askIdle, status: noRecord, screen: screenText(trustDialog), want: verdictDialog, wantReads: 1},
	{name: "idle: no record, ready prompt", ask: askIdle, status: noRecord, screen: screenText("booting\n❯ "), want: verdictIdle, wantReads: 1},
	{name: "idle: no record, anything else", ask: askIdle, status: noRecord, screen: screenText("booting\n"), want: verdictWaiting, wantReads: 1},
	// D1: a status record decides alone, so a dialog under an idle record reads idle.
	{name: "D1 idle: record idle over a dialog", ask: askIdle, status: rec("idle"), screen: screenText(trustDialog), want: verdictIdle, wantReads: 0},
	// D2: the idle snapshot checks menu cursors with no prefix, so a prefix cursor is no menu.
	{name: "D2 idle: prefix cursor is no menu cursor", ask: askIdle, status: noRecord, screen: screenText(prefixCursorMenu), want: verdictWaiting, wantReads: 1},
	// D3: the idle snapshot consults busy markers.
	{name: "D3 idle: busy marker without a record", ask: askIdle, status: noRecord, screen: screenText(busyScreen), want: verdictBusy, wantReads: 1},
	// D4: the idle snapshot accepts any readyPromptPrefixes glyph.
	{name: "D4 idle: any ready glyph", ask: askIdle, status: noRecord, screen: screenText("booting\n› "), want: verdictIdle, wantReads: 1},
	// D5: the idle snapshot trims the status state.
	{name: "D5 idle: padded idle record", ask: askIdle, status: rec(" idle "), screen: screenText("booting\n"), want: verdictIdle, wantReads: 0},
}

// TestPaneStateVerdict maps one observation to one verdict for every question
// the pane-state module answers, and pins how often each decision reads the
// screen. Rows named D1-D6 pin the divergences issue #44 keeps.
func TestPaneStateVerdict(t *testing.T) {
	rows := slices.Concat(idleRows)
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			o, reads := row.screen.observe(row.status)
			var (
				got    paneVerdict
				gotErr error
			)
			switch row.ask {
			case askIdle:
				got, gotErr = idleVerdict(o)
			default:
				t.Fatalf("unknown question %q", row.ask)
			}
			if row.wantErr {
				if !errors.Is(gotErr, errScreenRead) {
					t.Errorf("error = %v, want the screen read error", gotErr)
				}
			} else {
				if gotErr != nil {
					t.Errorf("error = %v, want none", gotErr)
				}
				if got != row.want {
					t.Errorf("verdict = %q, want %q", got, row.want)
				}
			}
			if *reads != row.wantReads {
				t.Errorf("screen reads = %d, want %d", *reads, row.wantReads)
			}
		})
	}
}
