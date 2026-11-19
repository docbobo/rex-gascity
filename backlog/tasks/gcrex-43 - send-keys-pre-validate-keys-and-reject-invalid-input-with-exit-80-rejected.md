---
id: GCREX-43
title: 'send-keys: pre-validate keys and reject invalid input with exit 80 rejected'
status: Done
assignee:
  - '@mc-wisp-u621sa'
created_date: '2026-11-18T22:05:00Z'
updated_date: '2026-11-19T00:19:00Z'
labels:
  - provider
  - herdr
  - for-gascity
  - tdd
dependencies: []
references:
  - docs/provider-method-audit/send-keys.md
documentation:
  - docs/provider-herdr-method-io.md
priority: medium
type: task
ordinal: 99000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Develop test-first (TDD): write the failing test that reproduces this first, watch it go red, then the smallest change that makes it green (repo rule in AGENTS.md; see also the tdd skill).

Avoids: invalid keys reach rex send-key and fail with exit 1 after the session lookup.

The document (## send-keys § Inputs Key grammar, § Failure cases) requires all keys validated before the lookup, case-sensitive TMUX_KEYS (BTab, PPage, NPage, F1-F12), the C-/M-/S- len>2 rule, splitting literal text per character, and exit 80 rejected for unknown keys. The implementation validates nothing up front and splits no text.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 Unknown or malformed keys are rejected before any Rex lookup with exit 80 rejected.
- [x] #2 Multi-character literal text is split into one key per character.
- [x] #3 The keys gc v1.5.1 sends (Enter, Up, Down, Escape, C-u) still map unchanged, covered by a test.
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. Rewrite internal/provider/keymap.go to mirror herdr-core's translate_key grammar (docs/provider-herdr-method-io.md §send-keys Key grammar): exact-case tmux key table (incl. BTab→shift+tab, PPage/NPage, F1–F12), C-/M-/S- prefixes only while len(rest)>2 and only on a table name or a single literal char (lowercased), and literal text split per character (a-z0-9 and the punctuation set as themselves, space→space, A-Z→shift+<lower>). A key with no Rex spelling returns a KeyRejectedError (sentinel ErrKeyRejected). translateKeys translates all keys before any lookup.
2. Provider.SendKeys calls translateKeys before livePane, so an invalid key rejects before any Rex call (currently it calls livePane first, then translates).
3. Dispatcher: add exit 80 (exitInputOutcome) and map *provider.KeyRejectedError to one {"outcome":"rejected","detail":...} line on stdout plus 'rejected: <detail>' on stderr, exit 80.
4. TDD: keymap grammar unit tests (named table, modifiers, literal split, rejection, gc vocabulary Enter/Up/Down/Escape/C-u); provider test that a bad key rejects with ErrKeyRejected and probes Rex zero times; provider test that literal text splits; dispatcher test that send-keys with a bad key exits 80 rejected with JSON stdout and no Rex calls.
5. Docs: parity doc send-keys summary + detailed rows 1–3,7 and verdict (GCREX-43); audit send-keys.md key-grammar row moved from Deviations to Matches; REX-CONTRACT §9 record exit 80 rejected for send-keys.
6. go test ./... and golangci-lint run ./... must pass. Change stays uncommitted for the review step.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Implemented (uncommitted, for review).

Key grammar (TDD): rewrote internal/provider/keymap.go to mirror herdr-core's translate_key (docs/provider-herdr-method-io.md §send-keys Key grammar). translateKeys translates every key before any Rex call via translateKey: an exact-case tmuxKeys table incl. BTab->shift+tab, PPage/NPage and F1-F12; C-/M-/S- prefixes only while len(rest)>2, on a table name or a single lower-cased literal char, kept in typed order (peelMods); and literal text split char-by-char (a-z0-9 and the punctuation set as themselves, space->space, A-Z->shift+<lower>). A key with no Rex spelling returns KeyRejectedError (sentinel ErrKeyRejected).

Provider.SendKeys now calls translateKeys before livePane (internal/provider/provider.go), so a bad key rejects before any Rex call. The dispatcher maps *provider.KeyRejectedError to exit 80 with one {"outcome":"rejected","detail":...} line on stdout and 'rejected: <detail>' on stderr (cmd/gc-runtime-rex/main.go: exitInputOutcome, writeInputOutcome).

TDD red->green: TestTranslateKeysGrammar (incl. gc vocabulary Enter/Up/Down/Escape/C-u, AC3), TestTranslateKeysRejectsUntypableKeys, TestTranslateKeysRejectsWholeList, TestSendKeysRejectsBadKeyBeforeLookup (probes Rex zero times, AC1), TestSendKeysSplitsLiteralText (AC2), TestSendKeysRejectsBadKeyWithOutcome (dispatcher exit 80, stdout JSON).

Docs: docs/provider-herdr-parity.md send-keys summary + detailed rows 1-3,7 and the verdict updated (GCREX-43); docs/provider-method-audit/send-keys.md key-grammar row moved from Deviations to Matches and the exit-80 envelope row notes the new rejected path; docs/REX-CONTRACT.md §9 records the extended exit 80 input-outcome code.

Verification: go test -count=1 ./... all packages ok; golangci-lint run ./... 0 issues. Note: the pack source for herdr-core is not vendored in this repo; the grammar follows the recorded docs/provider-herdr-method-io.md derivation (the punctuation set is taken from that doc). Change left uncommitted for review.

Independent review (openrouter/deepseek/deepseek-v4.1-flash:high) — GCREX-43, uncommitted change.

Scope: internal/provider/keymap.go, keymap_test.go, input_test.go, provider.go (SendKeys order), cmd/gc-runtime-rex/main.go + dispatch_test.go, docs/REX-CONTRACT.md, docs/provider-herdr-parity.md §send-keys, docs/provider-method-audit/send-keys.md. Unrelated working-tree churn (gcrex-27/28 md, other untracked tasks/docs) ignored.

Verification: go test -count=1 ./... all ok; golangci-lint run ./... 0 issues; gofmt clean. AC3 verified against gc source at tag v1.5.1: the runtime SendKeys call sites are dialog.go Enter/Down/Up, submit.go:266 Escape, submit.go:485 C-u, and exec.go:804-809 falls back to the send-keys op; all five map (TestTranslateKeysGrammar 'gc vocabulary'). AC1: translateKeys runs before livePane (provider.go:710-720); TestSendKeysRejectsBadKeyBeforeLookup proves zero Rex calls and TestSendKeysRejectsBadKeyWithOutcome proves exit 80 with {"outcome":"rejected",...} on stdout. AC2: TestSendKeysSplitsLiteralText pins 'ok go' -> o k space g o. The exit-80-is-inert-at-baseline claim checks out at exec.go:153-171 (only exit 2 is special; 80 wraps stderr as a plain error).

Minor finding (follow-up): functionKeyNumber (internal/provider/keymap.go:112-131) accepts leading zeros: F01 -> f1, F012 -> f12, C-F01 -> ctrl+f1. The documented grammar (docs/provider-herdr-method-io.md §send-keys Key grammar step 1) is an exact TMUX_KEYS match limited to F1-F12; herdr-core types F01 as literal text (shift+f, 0, 1) and rejects C-F01 (base is neither a table name nor a single character). Unreachable from gc. Fix by requiring canonical spelling (e.g. reject n==0 or require strconv.Itoa(n)==key[1:]) and add a grammar test.

Nits: docs/provider-herdr-parity.md:246-249 still calls BTab/PPage/NPage 'missing' in the withdrawn-candidates paragraph though this change added them; the handshake still omits the input-outcome capability (already recorded in audit send-keys.md; GCREX-28/28.2 scope, fine at the gc 1.5.1 baseline).

Verdict: pass (minor/nit only, listed as follow-ups).

Verification (finalize step): make test -> go test ./... all packages ok; golangci-lint run ./... -> 0 issues. AC1: TestSendKeysRejectsBadKeyBeforeLookup (zero Rex calls) + TestSendKeysRejectsBadKeyWithOutcome (exit 80, {"outcome":"rejected"} stdout, no lookup). AC2: TestSendKeysSplitsLiteralText pins 'ok go' -> o k space g o. AC3: TestTranslateKeysGrammar/gc_vocabulary pins Enter/Up/Down/Escape/C-u -> enter/up/down/esc/ctrl+u.
<!-- SECTION:NOTES:END -->

## Comments

<!-- COMMENTS:BEGIN -->
author: @mc-wisp-u621sa
created: 2026-11-19T00:01:00Z

---

Claimed by Gas City worker mc-wisp-u621sa (seat rex-gascity/pi-1, session pi-mc-wisp-u621sa, template rex-gascity/pi); workflow bead mc-kelx, step bead mc-bifj.

---
<!-- COMMENTS:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
send-keys now validates the whole key list before any Rex lookup and rejects unknown or untypable keys with RPP exit 80 ({"outcome":"rejected","detail":...}). The keymap mirrors herdr-core's translate_key grammar: exact-case tmux table (BTab, PPage/NPage, F1-F12), C-/M-/S- modifiers only while len(rest)>2, and literal text split per character. Provider.SendKeys translates before livePane, and the dispatcher maps KeyRejectedError to exit 80. Docs updated: parity doc send-keys rows/verdict, audit send-keys key grammar now Matches, REX-CONTRACT §9 records the extended exit-80 input outcome. Verification: make test and golangci-lint run ./... pass. Follow-ups (non-blocking, from review): functionKeyNumber accepts leading zeros (F01); parity doc lines 246-249 still call BTab/PPage/NPage 'missing' in the withdrawn-candidates paragraph; handshake omits the input-outcome capability (GCREX-28/28.2).
<!-- SECTION:FINAL_SUMMARY:END -->
