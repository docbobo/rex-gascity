package provider

import (
	"errors"
	"slices"
	"testing"
	"time"
)

// paneQuestion names the pane-state entry point a TestPaneStateVerdict row asks.
type paneQuestion string

const (
	askIdle     paneQuestion = "idle"      // idleVerdict
	askStartup  paneQuestion = "startup"   // startupVerdict
	askTurn     paneQuestion = "turn"      // turnBaseline + turnVerdict
	askLateTurn paneQuestion = "late-turn" // lateTurnVerdict
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

var (
	turnT0 = time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)
	turnT1 = turnT0.Add(time.Second)
)

// noRecord is an observation with no program_status record.
var noRecord = programStatusSnapshot{}

// rec is a program_status record with state and a zero timestamp.
func rec(state string) programStatusSnapshot {
	return programStatusSnapshot{Present: true, State: state}
}

// recAt is a program_status record with state, updated at at.
func recAt(state string, at time.Time) programStatusSnapshot {
	return programStatusSnapshot{Present: true, State: state, Latest: at}
}

// verdictRow is one TestPaneStateVerdict row: one observation, one question,
// the wanted verdict and the wanted number of screen reads.
type verdictRow struct {
	name   string
	ask    paneQuestion
	status programStatusSnapshot // the observation's (turn: the poll's) status
	screen screenFixture         // the observation's (turn: the poll's) screen

	// startup inputs
	prefix    string
	pastFloor bool
	atBound   bool

	// turn inputs: the pre-send baseline observation
	baseStatus programStatusSnapshot
	baseScreen screenFixture

	want          paneVerdict
	wantErr       bool        // idle: the screen read error is returned
	wantSource    NudgeSource // turn
	wantReads     int         // screen reads of the observation (turn: of the poll)
	wantBaseReads int         // turn: screen reads of the baseline
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

// startupRows are REQ-009: startupVerdict, today's readinessNote minus bound naming.
var startupRows = []verdictRow{
	{name: "startup prompt: dialog", ask: askStartup, prefix: "❯ ", status: noRecord, screen: screenText(trustDialog), want: verdictDialog, wantReads: 1},
	{name: "startup prompt: dialog over a matching prompt", ask: askStartup, prefix: "READY>", status: noRecord, screen: screenText("READY>\n" + trustDialog), want: verdictDialog, wantReads: 1},
	{name: "startup prompt: ready prompt", ask: askStartup, prefix: "❯ ", status: noRecord, screen: screenText("booting\n❯ "), want: verdictPromptReady, wantReads: 1},
	{name: "startup prompt: no prompt yet", ask: askStartup, prefix: "❯ ", status: noRecord, screen: screenText("booting\n"), want: verdictWaiting, wantReads: 1},
	{name: "startup prompt: read fails at the bound", ask: askStartup, prefix: "❯ ", atBound: true, status: noRecord, screen: screenFails(), want: verdictWaiting, wantReads: 1},
	{name: "startup prompt: ignores the floor and an idle record", ask: askStartup, prefix: "❯ ", pastFloor: true, status: rec("idle"), screen: screenText("booting\n"), want: verdictWaiting, wantReads: 1},
	{name: "startup status: idle past the floor", ask: askStartup, pastFloor: true, status: rec("idle"), screen: screenText("❯ "), want: verdictIdle, wantReads: 1},
	{name: "startup status: GCREX-59 dialog over an idle record", ask: askStartup, pastFloor: true, status: rec("idle"), screen: screenText(trustDialog), want: verdictDialog, wantReads: 1},
	{name: "startup status: idle past the floor, read fails", ask: askStartup, pastFloor: true, status: rec("idle"), screen: screenFails(), want: verdictIdle, wantReads: 1},
	{name: "startup status: dialog at the bound", ask: askStartup, pastFloor: true, atBound: true, status: noRecord, screen: screenText(trustDialog), want: verdictDialog, wantReads: 1},
	{name: "startup status: no dialog at the bound", ask: askStartup, pastFloor: true, atBound: true, status: rec("working"), screen: screenText("booting\n"), want: verdictWaiting, wantReads: 1},
	{name: "startup status: read fails at the bound", ask: askStartup, pastFloor: true, atBound: true, status: noRecord, screen: screenFails(), want: verdictWaiting, wantReads: 1},
	{name: "startup status: working before the bound", ask: askStartup, pastFloor: true, status: rec("working"), screen: screenText(trustDialog), want: verdictWaiting, wantReads: 0},
	// D2: prompt mode checks menu cursors with the configured prefix, status mode with none.
	{name: "D2 startup prompt: prefix cursor beside a bare mention", ask: askStartup, prefix: "READY>", status: noRecord, screen: screenText(prefixCursorMenu), want: verdictDialog, wantReads: 1},
	{name: "D2 startup status: same screen at the bound", ask: askStartup, pastFloor: true, atBound: true, status: noRecord, screen: screenText(prefixCursorMenu), want: verdictWaiting, wantReads: 1},
	// D3: startup does not consult busy markers; at the settle bound it delivers.
	{name: "D3 startup status: busy marker at the bound", ask: askStartup, pastFloor: true, atBound: true, status: noRecord, screen: screenText(busyScreen), want: verdictWaiting, wantReads: 1},
	// D4: prompt mode accepts only the configured prefix.
	{name: "D4 startup prompt: another ready glyph", ask: askStartup, prefix: "❯ ", status: noRecord, screen: screenText("booting\n› "), want: verdictWaiting, wantReads: 1},
	// D5: startup does not trim the status state.
	{name: "D5 startup status: padded idle record", ask: askStartup, pastFloor: true, status: rec(" idle "), screen: screenText("❯ "), want: verdictWaiting, wantReads: 0},
	// D6: status mode reads no screen before the floor and the bound.
	{name: "D6 startup status: before the floor and the bound", ask: askStartup, status: rec("idle"), screen: screenText(trustDialog), want: verdictWaiting, wantReads: 0},
}

// turnRows are REQ-010: turnVerdict (with its baseline) and lateTurnVerdict.
var turnRows = []verdictRow{
	{name: "turn: idle to working", ask: askTurn, baseStatus: rec("idle"), baseScreen: screenText("a"), status: rec("working"), screen: screenText("b"), want: verdictBusy, wantSource: NudgeSourceProgramStatus, wantBaseReads: 0, wantReads: 0},
	{name: "turn: newer timestamp on working", ask: askTurn, baseStatus: recAt("working", turnT0), baseScreen: screenText("a"), status: recAt("working", turnT1), screen: screenText("a"), want: verdictBusy, wantSource: NudgeSourceProgramStatus},
	{name: "turn: newer timestamp on idle", ask: askTurn, baseStatus: recAt("idle", turnT0), baseScreen: screenText("a"), status: recAt("idle", turnT1), screen: screenText("a"), want: verdictBusy, wantSource: NudgeSourceProgramStatus},
	{name: "turn: working, same timestamp", ask: askTurn, baseStatus: recAt("working", turnT0), baseScreen: screenText("a"), status: recAt("working", turnT0), screen: screenText("b"), want: verdictWaiting},
	{name: "turn: baseline record, screen changed", ask: askTurn, baseStatus: recAt("idle", turnT0), baseScreen: screenText("a"), status: recAt("idle", turnT0), screen: screenText("b"), want: verdictWaiting},
	{name: "turn: baseline record, poll has none", ask: askTurn, baseStatus: rec("idle"), baseScreen: screenText("a"), status: noRecord, screen: screenText("b"), want: verdictWaiting},
	{name: "turn: no record, screen changed", ask: askTurn, baseStatus: noRecord, baseScreen: screenText("prompt>"), status: noRecord, screen: screenText("prompt> hello"), want: verdictBusy, wantSource: NudgeSourceFormatPoll, wantBaseReads: 1, wantReads: 1},
	{name: "turn: no record, screen unchanged", ask: askTurn, baseStatus: noRecord, baseScreen: screenText("prompt>"), status: noRecord, screen: screenText("prompt>"), want: verdictWaiting, wantBaseReads: 1, wantReads: 1},
	{name: "turn: no baseline record, poll record working", ask: askTurn, baseStatus: noRecord, baseScreen: screenText("prompt>"), status: rec("working"), screen: screenText("prompt>"), want: verdictBusy, wantSource: NudgeSourceProgramStatus, wantBaseReads: 1, wantReads: 0},
	{name: "turn: no baseline record, poll record idle, screen changed", ask: askTurn, baseStatus: noRecord, baseScreen: screenText("prompt>"), status: rec("idle"), screen: screenText("prompt> hello"), want: verdictBusy, wantSource: NudgeSourceFormatPoll, wantBaseReads: 1, wantReads: 1},
	{name: "turn: baseline read failed", ask: askTurn, baseStatus: noRecord, baseScreen: screenFails(), status: noRecord, screen: screenText("prompt> hello"), want: verdictWaiting, wantBaseReads: 1, wantReads: 0},
	{name: "turn: poll read fails", ask: askTurn, baseStatus: noRecord, baseScreen: screenText("prompt>"), status: noRecord, screen: screenFails(), want: verdictWaiting, wantBaseReads: 1, wantReads: 1},
	{name: "late turn: working", ask: askLateTurn, status: rec("working"), screen: screenText("a"), want: verdictBusy},
	{name: "late turn: blocked", ask: askLateTurn, status: rec("blocked"), screen: screenText("a"), want: verdictBusy},
	{name: "late turn: idle", ask: askLateTurn, status: rec("idle"), screen: screenText("a"), want: verdictWaiting},
	{name: "late turn: no record", ask: askLateTurn, status: noRecord, screen: screenText("a"), want: verdictWaiting},
	// D5: the resend ladder does not trim the status state.
	{name: "D5 late turn: padded working record", ask: askLateTurn, status: rec(" working "), screen: screenText("a"), want: verdictWaiting},
}

// parityRows are REQ-013: the 43 cases of the deleted per-predicate parity
// tests, each asked on a question where the pinned predicate decides the
// verdict. A row's trailing comment names its source case by file:line at
// 296b74f.
var parityRows = []verdictRow{
	// From TestShowsDialogMatchesHerdrCoreReadiness: runtime-herdr-core
	// readiness.py (pinned 0f07496) test_herdrcore_readiness.py. shows_dialog is
	// named-dialog OR any rate-limit marker (gc's ContainsRateLimitDialog), and
	// a Codex screen that has finished starting shows none. Each case is asked
	// (a) on startup in status mode at the bound with no record, and (b) on the
	// idle snapshot with no record.
	{name: "trust dialog (shows_dialog, startup)", ask: askStartup, pastFloor: true, atBound: true, status: noRecord, screen: screenText("Do you trust the files in this folder?\n❯ 1. Yes, proceed\n  2. No, exit\n"), want: verdictDialog, wantReads: 1},                         // readiness_internal_test.go:17
	{name: "trust dialog (shows_dialog, idle)", ask: askIdle, status: noRecord, screen: screenText("Do you trust the files in this folder?\n❯ 1. Yes, proceed\n  2. No, exit\n"), want: verdictDialog, wantReads: 1},                                                               // readiness_internal_test.go:17
	{name: "bypass permissions (shows_dialog, startup)", ask: askStartup, pastFloor: true, atBound: true, status: noRecord, screen: screenText("WARNING: Claude Code running in Bypass Permissions mode\n❯ 1. No, exit\n  2. Yes, I accept\n"), want: verdictDialog, wantReads: 1}, // readiness_internal_test.go:18
	{name: "bypass permissions (shows_dialog, idle)", ask: askIdle, status: noRecord, screen: screenText("WARNING: Claude Code running in Bypass Permissions mode\n❯ 1. No, exit\n  2. Yes, I accept\n"), want: verdictDialog, wantReads: 1},                                       // readiness_internal_test.go:18
	{name: "custom api key (shows_dialog, startup)", ask: askStartup, pastFloor: true, atBound: true, status: noRecord, screen: screenText("Detected a custom API key in your environment\n"), want: verdictDialog, wantReads: 1},                                                  // readiness_internal_test.go:19
	{name: "custom api key (shows_dialog, idle)", ask: askIdle, status: noRecord, screen: screenText("Detected a custom API key in your environment\n"), want: verdictDialog, wantReads: 1},                                                                                        // readiness_internal_test.go:19
	{name: "theme picker with /theme (shows_dialog, startup)", ask: askStartup, pastFloor: true, atBound: true, status: noRecord, screen: screenText("Choose the text style\nTo change this later, run /theme\n"), want: verdictDialog, wantReads: 1},                              // readiness_internal_test.go:20
	{name: "theme picker with /theme (shows_dialog, idle)", ask: askIdle, status: noRecord, screen: screenText("Choose the text style\nTo change this later, run /theme\n"), want: verdictDialog, wantReads: 1},                                                                    // readiness_internal_test.go:20
	{name: "theme picker without every marker (shows_dialog, startup)", ask: askStartup, pastFloor: true, atBound: true, status: noRecord, screen: screenText("Choose the text style\n"), want: verdictWaiting, wantReads: 1},                                                      // readiness_internal_test.go:21
	{name: "theme picker without every marker (shows_dialog, idle)", ask: askIdle, status: noRecord, screen: screenText("Choose the text style\n"), want: verdictWaiting, wantReads: 1},                                                                                            // readiness_internal_test.go:21
	{name: "hook review with every marker (shows_dialog, startup)", ask: askStartup, pastFloor: true, atBound: true, status: noRecord, screen: screenText("Hooks need review\nTrust all and continue\nContinue without trusting\n"), want: verdictDialog, wantReads: 1},            // readiness_internal_test.go:22
	{name: "hook review with every marker (shows_dialog, idle)", ask: askIdle, status: noRecord, screen: screenText("Hooks need review\nTrust all and continue\nContinue without trusting\n"), want: verdictDialog, wantReads: 1},                                                  // readiness_internal_test.go:22
	{name: "hook review missing a marker (shows_dialog, startup)", ask: askStartup, pastFloor: true, atBound: true, status: noRecord, screen: screenText("Hooks need review\nTrust all and continue\n"), want: verdictWaiting, wantReads: 1},                                       // readiness_internal_test.go:23
	{name: "hook review missing a marker (shows_dialog, idle)", ask: askIdle, status: noRecord, screen: screenText("Hooks need review\nTrust all and continue\n"), want: verdictWaiting, wantReads: 1},                                                                             // readiness_internal_test.go:23
	{name: "usage limit (shows_dialog, startup)", ask: askStartup, pastFloor: true, atBound: true, status: noRecord, screen: screenText("You've hit your limit\n"), want: verdictDialog, wantReads: 1},                                                                             // readiness_internal_test.go:24
	{name: "usage limit (shows_dialog, idle)", ask: askIdle, status: noRecord, screen: screenText("You've hit your limit\n"), want: verdictDialog, wantReads: 1},                                                                                                                   // readiness_internal_test.go:24
	// Old false. The idle row is then decided by screenShowsReadyPrompt, which sees "❯ ".
	{name: "a live composer is no dialog (shows_dialog, startup)", ask: askStartup, pastFloor: true, atBound: true, status: noRecord, screen: screenText("❯ Try it\n"), want: verdictWaiting, wantReads: 1}, // readiness_internal_test.go:25
	{name: "a live composer is no dialog (shows_dialog, idle)", ask: askIdle, status: noRecord, screen: screenText("❯ Try it\n"), want: verdictIdle, wantReads: 1},                                          // readiness_internal_test.go:25
	// Old false. The idle row is then decided by screenShowsReadyPrompt, which sees "› ".
	{name: "codex settled hides the bare mention (shows_dialog, startup)", ask: askStartup, pastFloor: true, atBound: true, status: noRecord, screen: screenText("OpenAI Codex\n› ask about the rate limit\n  gpt-5 · ~/repo\n"), want: verdictWaiting, wantReads: 1}, // readiness_internal_test.go:26
	{name: "codex settled hides the bare mention (shows_dialog, idle)", ask: askIdle, status: noRecord, screen: screenText("OpenAI Codex\n› ask about the rate limit\n  gpt-5 · ~/repo\n"), want: verdictIdle, wantReads: 1},                                          // readiness_internal_test.go:26
	// herdr-core shows_dialog says dialog; withholds_first_turn narrows it; dead
	// matcher deleted in #44. The rows pin today's live verdicts. The idle row is
	// decided by screenShowsReadyPrompt, which sees "› ".
	{name: "codex not settled shows the bare mention (shows_dialog, startup)", ask: askStartup, pastFloor: true, atBound: true, status: noRecord, screen: screenText("OpenAI Codex\n› ask about the rate limit\n"), want: verdictWaiting, wantReads: 1}, // readiness_internal_test.go:27
	{name: "codex not settled shows the bare mention (shows_dialog, idle)", ask: askIdle, status: noRecord, screen: screenText("OpenAI Codex\n› ask about the rate limit\n"), want: verdictIdle, wantReads: 1},                                          // readiness_internal_test.go:27
	// herdr-core shows_dialog says dialog; withholds_first_turn narrows it; dead
	// matcher deleted in #44. The rows pin today's live verdicts: a one-option
	// menu has no numbered neighbour, so menuCursorRow reads "› 1. Retry" as a
	// composer and the idle row is decided by screenShowsReadyPrompt (follow-up 4
	// in the #44 PR body).
	{name: "codex rate-limit menu is a dialog (shows_dialog, startup)", ask: askStartup, pastFloor: true, atBound: true, status: noRecord, screen: screenText("OpenAI Codex\nrate limit\n› 1. Retry\n  enter confirm · esc skip\n"), want: verdictWaiting, wantReads: 1}, // readiness_internal_test.go:28
	{name: "codex rate-limit menu is a dialog (shows_dialog, idle)", ask: askIdle, status: noRecord, screen: screenText("OpenAI Codex\nrate limit\n› 1. Retry\n  enter confirm · esc skip\n"), want: verdictIdle, wantReads: 1},                                          // readiness_internal_test.go:28

	// From TestWithholdsFirstTurnMatchesHerdrCoreReadiness: runtime-herdr-core
	// readiness.py (pinned 0f07496) test_herdrcore_readiness.py. A named dialog,
	// a high-confidence rate-limit screen, or a bare "rate limit" mention beside
	// a numbered menu's cursor row withholds; a bare mention with no menu does
	// not. A case with a prefix is asked on startup in prompt mode with that
	// prefix; a case with no prefix on startup in status mode at the bound. An
	// old false in prompt mode is then decided by screenHasPromptPrefix:
	// prompt-ready when the screen shows the prefix, else waiting.
	{name: "trust dialog (withholds_first_turn, startup)", ask: askStartup, prefix: "❯ ", status: noRecord, screen: screenText("Do you trust the files in this folder?\n❯ 1. Yes, proceed\n  2. No, exit\n"), want: verdictDialog, wantReads: 1}, // readiness_internal_test.go:53
	{name: "usage limit with a prompt (withholds_first_turn, startup)", ask: askStartup, prefix: "READY>", status: noRecord, screen: screenText("You've hit your limit\nREADY> \n"), want: verdictDialog, wantReads: 1},                          // readiness_internal_test.go:54
	// Old false. The row is then decided by screenHasPromptPrefix, which sees READY>.
	{name: "bare mention under the prompt (withholds_first_turn, startup)", ask: askStartup, prefix: "READY>", status: noRecord, screen: screenText("READY> \n  rate limit: 40% used\n"), want: verdictPromptReady, wantReads: 1},                                                                 // readiness_internal_test.go:55
	{name: "bare mention with no prompt (withholds_first_turn, startup)", ask: askStartup, prefix: "READY>", status: noRecord, screen: screenText("booting\n  rate limit: 40% used\n"), want: verdictWaiting, wantReads: 1},                                                                       // readiness_internal_test.go:56
	{name: "bare mention beside a glyph with no prefix, status line (withholds_first_turn, startup)", ask: askStartup, pastFloor: true, atBound: true, status: noRecord, screen: screenText("❯ \n  Rate limit resets at 5pm\n"), want: verdictWaiting, wantReads: 1},                              // readiness_internal_test.go:57
	{name: "bare mention on its own line (withholds_first_turn, startup)", ask: askStartup, pastFloor: true, atBound: true, status: noRecord, screen: screenText("Rate limit reached\nretrying...\n"), want: verdictWaiting, wantReads: 1},                                                        // readiness_internal_test.go:58
	{name: "bare mention on the marker's own line (withholds_first_turn, startup)", ask: askStartup, pastFloor: true, atBound: true, status: noRecord, screen: screenText("Rate limit reached: 100%\n"), want: verdictWaiting, wantReads: 1},                                                      // readiness_internal_test.go:59
	{name: "bare mention in a tip (withholds_first_turn, startup)", ask: askStartup, prefix: "❯ ", status: noRecord, screen: screenText("Welcome\nTip: avoid hitting the rate limit by...\n"), want: verdictWaiting, wantReads: 1},                                                                // readiness_internal_test.go:60
	{name: "numbered list with no cursor row (withholds_first_turn, startup)", ask: askStartup, prefix: "❯ ", status: noRecord, screen: screenText("Tips:\n1. mind the rate limit\n2. use /compact\n"), want: verdictWaiting, wantReads: 1},                                                       // readiness_internal_test.go:61
	{name: "provider rate-limit screen (withholds_first_turn, startup)", ask: askStartup, pastFloor: true, atBound: true, status: noRecord, screen: screenText("Rate limit reached\n1. Keep trying\n2. Stop\n"), want: verdictDialog, wantReads: 1},                                               // readiness_internal_test.go:62
	{name: "provider rate-limit screen with a cursor (withholds_first_turn, startup)", ask: askStartup, prefix: "❯ ", status: noRecord, screen: screenText("Rate limit reached\n❯ 1. Keep trying\n  2. Stop\n"), want: verdictDialog, wantReads: 1},                                               // readiness_internal_test.go:63
	{name: "uppercase provider rate-limit screen (withholds_first_turn, startup)", ask: askStartup, pastFloor: true, atBound: true, status: noRecord, screen: screenText("RATE LIMIT REACHED\n1. Keep trying\n2. Stop\n"), want: verdictDialog, wantReads: 1},                                     // readiness_internal_test.go:64
	{name: "usage-limit menu with a cursor (withholds_first_turn, startup)", ask: askStartup, pastFloor: true, atBound: true, status: noRecord, screen: screenText("Usage limit reached for gemini-3-flash-preview.\n1. Keep trying\n2. Stop\n"), want: verdictDialog, wantReads: 1},              // readiness_internal_test.go:65
	{name: "bare mention beside a cursor and prefix (withholds_first_turn, startup)", ask: askStartup, prefix: "❯ ", status: noRecord, screen: screenText("rate limit\n❯ 1. Retry\n  2. Wait\n"), want: verdictDialog, wantReads: 1},                                                              // readiness_internal_test.go:66
	{name: "bare mention beside a glyph with no prefix, menu cursor (withholds_first_turn, startup)", ask: askStartup, pastFloor: true, atBound: true, status: noRecord, screen: screenText("rate limit\n❯ 1. Retry\n  2. Wait\n"), want: verdictDialog, wantReads: 1},                            // readiness_internal_test.go:67
	{name: "bare mention beside a prefix cursor (withholds_first_turn, startup)", ask: askStartup, prefix: "READY>", status: noRecord, screen: screenText("rate limit\nREADY> 1. Retry\n  2. Wait\n"), want: verdictDialog, wantReads: 1},                                                         // readiness_internal_test.go:68
	{name: "codex settled hides the menu (withholds_first_turn, startup)", ask: askStartup, pastFloor: true, atBound: true, status: noRecord, screen: screenText("OpenAI Codex\n› why did Rate limit reached show Keep trying and Stop\n  gpt-5 · ~/repo\n"), want: verdictWaiting, wantReads: 1}, // readiness_internal_test.go:69
	// Old false. The row is then decided by screenHasPromptPrefix, which sees READY>.
	{name: "a plain prompt (withholds_first_turn, startup)", ask: askStartup, prefix: "READY>", status: noRecord, screen: screenText("READY> \n"), want: verdictPromptReady, wantReads: 1}, // readiness_internal_test.go:70

	// From TestScreenHasPromptPrefixMatchesGcPaneShowsReadyPrompt: gc's
	// paneShowsReadyPrompt. The deleted test cited gc v1.5.1
	// internal/runtime/tmux/tmux.go:4199-4231, which holds matchesPromptPrefix
	// and stripLeadingBoxBorder; paneShowsReadyPrompt and isMenuCursorRow are at
	// gc edge 890a58a3e internal/runtime/tmux/tmux.go:5554 and :5583.
	// The bottom-most prompt line decides, NBSP reads as a space, one leading
	// box border is skipped, the bare glyph counts, and a numbered menu's cursor
	// row is no prompt. Each case is asked on startup in prompt mode with its
	// prefix.
	{name: "bare glyph on its own line (paneShowsReadyPrompt, startup)", ask: askStartup, prefix: "❯ ", status: noRecord, screen: screenText("booting\n❯ "), want: verdictPromptReady, wantReads: 1},         // nudge_prompt_internal_test.go:18
	{name: "glyph then draft text (paneShowsReadyPrompt, startup)", ask: askStartup, prefix: "❯ ", status: noRecord, screen: screenText("booting\n❯ hello"), want: verdictPromptReady, wantReads: 1},         // nudge_prompt_internal_test.go:19
	{name: "bare glyph with no trailing space (paneShowsReadyPrompt, startup)", ask: askStartup, prefix: "❯ ", status: noRecord, screen: screenText("booting\n❯"), want: verdictPromptReady, wantReads: 1},   // nudge_prompt_internal_test.go:20
	{name: "nbsp between glyph and text (paneShowsReadyPrompt, startup)", ask: askStartup, prefix: "❯ ", status: noRecord, screen: screenText("booting\n❯ hello"), want: verdictPromptReady, wantReads: 1},   // nudge_prompt_internal_test.go:21
	{name: "box border before the glyph (paneShowsReadyPrompt, startup)", ask: askStartup, prefix: "❯ ", status: noRecord, screen: screenText("booting\n│ ❯ hello"), want: verdictPromptReady, wantReads: 1}, // nudge_prompt_internal_test.go:22
	{name: "thick box border before the glyph (paneShowsReadyPrompt, startup)", ask: askStartup, prefix: "❯ ", status: noRecord, screen: screenText("booting\n┃❯ "), want: verdictPromptReady, wantReads: 1}, // nudge_prompt_internal_test.go:23
	{name: "glyph only in past output (paneShowsReadyPrompt, startup)", ask: askStartup, prefix: "❯ ", status: noRecord, screen: screenText("❯ old turn\ndone"), want: verdictPromptReady, wantReads: 1},     // nudge_prompt_internal_test.go:24
	// Masked: the screen matches the trust dialog's "trust this folder" marker
	// (startupDialogGroups in readiness.go), so prompt mode reads dialog before
	// the prompt rule runs. predicateRows keeps the old false.
	{name: "selected dialog option is no prompt (paneShowsReadyPrompt, startup)", ask: askStartup, prefix: "❯ ", status: noRecord, screen: screenText("Do you trust this folder?\n❯ 1. Yes\n  2. No"), want: verdictDialog, wantReads: 1}, // nudge_prompt_internal_test.go:25
	{name: "second dialog option below the cursor (paneShowsReadyPrompt, startup)", ask: askStartup, prefix: "❯ ", status: noRecord, screen: screenText("Menu\n❯ 2. Keep trying\n  1. Stop"), want: verdictWaiting, wantReads: 1},         // nudge_prompt_internal_test.go:26
	{name: "a draft starting 1. is still a prompt (paneShowsReadyPrompt, startup)", ask: askStartup, prefix: "❯ ", status: noRecord, screen: screenText("booting\n❯ 1. fix the test\n"), want: verdictPromptReady, wantReads: 1},          // nudge_prompt_internal_test.go:27
	{name: "no glyph anywhere (paneShowsReadyPrompt, startup)", ask: askStartup, prefix: "❯ ", status: noRecord, screen: screenText("booting\nworking"), want: verdictWaiting, wantReads: 1},                                              // nudge_prompt_internal_test.go:28
	// Masked by mode selection: an empty prefix is status mode, which never asks
	// screenHasPromptPrefix. predicateRows keeps the old false.
	{name: "empty prefix matches nothing (paneShowsReadyPrompt, startup)", ask: askStartup, pastFloor: true, atBound: true, status: noRecord, screen: screenText("booting\n❯ "), want: verdictWaiting, wantReads: 1}, // nudge_prompt_internal_test.go:29
	{name: "border glyph alone is not the prefix (paneShowsReadyPrompt, startup)", ask: askStartup, prefix: "❯ ", status: noRecord, screen: screenText("booting\n│"), want: verdictWaiting, wantReads: 1},            // nudge_prompt_internal_test.go:30
}

// predicateRows hold the parity cases no verdict row can observe (masked
// cases), asserted directly on the predicate. Nothing else belongs here.
var predicateRows = []struct {
	name   string
	screen string
	prefix string
	want   bool
}{
	// nudge_prompt_internal_test.go:25: prompt mode reads this screen as a
	// dialog before it asks screenHasPromptPrefix.
	{"selected dialog option is no prompt", "Do you trust this folder?\n❯ 1. Yes\n  2. No", "❯ ", false},
	// nudge_prompt_internal_test.go:29: an empty prefix selects status mode. The
	// guard mirrors gc v1.5.1 matchesPromptPrefix
	// (internal/runtime/tmux/tmux.go:4204-4206).
	{"empty prefix matches nothing", "booting\n❯ ", "", false},
}

// TestPaneStateVerdict maps one observation to one verdict for every question
// the pane-state module answers, and pins how often each decision reads the
// screen. Rows named D1-D6 pin the divergences issue #44 keeps.
func TestPaneStateVerdict(t *testing.T) {
	rows := slices.Concat(idleRows, startupRows, turnRows, parityRows)
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			o, reads := row.screen.observe(row.status)
			var (
				got     paneVerdict
				gotErr  error
				source  NudgeSource
				baseRds *int
			)
			switch row.ask {
			case askIdle:
				got, gotErr = idleVerdict(o)
			case askStartup:
				got = startupVerdict(o, row.prefix, row.pastFloor, row.atBound)
			case askTurn:
				var base paneObservation
				base, baseRds = row.baseScreen.observe(row.baseStatus)
				base = turnBaseline(base)
				got, source = turnVerdict(base, o)
			case askLateTurn:
				got = lateTurnVerdict(o)
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
			if source != row.wantSource {
				t.Errorf("source = %q, want %q", source, row.wantSource)
			}
			if *reads != row.wantReads {
				t.Errorf("screen reads = %d, want %d", *reads, row.wantReads)
			}
			if baseRds != nil && *baseRds != row.wantBaseReads {
				t.Errorf("baseline screen reads = %d, want %d", *baseRds, row.wantBaseReads)
			}
		})
	}
	for _, row := range predicateRows {
		t.Run("predicates/"+row.name, func(t *testing.T) {
			if got := screenHasPromptPrefix(row.screen, row.prefix); got != row.want {
				t.Errorf("screenHasPromptPrefix(%q, %q) = %v, want %v", row.screen, row.prefix, got, row.want)
			}
		})
	}
}
