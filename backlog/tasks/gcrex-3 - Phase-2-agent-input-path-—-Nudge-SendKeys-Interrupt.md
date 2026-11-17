---
id: GCREX-3
title: 'Phase 2: agent input path — Nudge, SendKeys, Interrupt'
status: Done
assignee:
  - '@boris'
created_date: '2026-11-17T16:25:00Z'
updated_date: '2026-11-17T18:55:00Z'
labels: []
dependencies:
  - GCREX-2
references:
  - docs/plans/native-rex-provider.md
  - docs/REX-CONTRACT.md
ordinal: 3000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Agents only make progress when the controller can deliver input. This phase adds the write path: typed text, submit confirmation, key sequences, and interrupt. Rex has no signal API, so interrupt is a ctrl+c keystroke against the pane, and the provider must handle a busy agent and report honestly when a keystroke could not be delivered.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 Nudge writes text and submits it, confirming the submit keystroke reached the pane
- [x] #2 SendKeys delivers arbitrary key sequences to the agent pane
- [x] #3 Interrupt sends ctrl+c and reports ErrSessionNotFound when no live agent is present
- [x] #4 Input to a closed or exited pane fails with a clear error rather than silently succeeding
- [x] #5 go test ./... covers the input path against a fake rex.Client
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. Add SendKeys(sessionID, blockID, keys...) to rex.Client; implement it in the CLI adapter as 'rex send-key -s -b KEY...'; record calls and support error injection in faketest.
2. Add provider.Nudge/SendKeys/Interrupt behind a livePane resolver: a missing pane or an exited program yields ErrSessionNotFound (wrapped with detail). Nudge types text then submits with the enter key and fails if the submit is not delivered; SendKeys translates gc/tmux key names to Rex syntax; Interrupt sends ctrl+c.
3. Add keymap.go: tmux names (C-c, M-x, Enter, Escape, BSpace, Up/Down/Left/Right, F1..F24, PageUp/PageDown, Delete/Insert, Tab/Space, Home/End) to Rex (ctrl+c, alt+x, enter, esc, backspace, up, f11, page_up, ...), normalizing modifier order.
4. Tests at the provider-core seam (fake rex.Client): Nudge writes then submits and surfaces submit failure; empty nudge no-op; SendKeys translation; Interrupt ctrl+c; missing/exited pane -> ErrSessionNotFound. Keymap unit tests. CLI adapter test for send-key argv. Extend the fake-rex harness for send-key.
5. Opt-in live e2e (REX_E2E=1): start a shell pane, nudge a command through a turn, peek the output, then interrupt.
6. Run go test ./..., go vet ./..., gofmt -l .; record evidence per AC and finalize.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Input path implemented. rex.Client gained SendKeys(sessionID, blockID, keys...), implemented in the CLI adapter as 'rex send-key -s <sid> -b <bid> KEY...' (the pinned CLI op; block.event needs a full physical-key payload, so send-key is the honest surface). faketest records SentKeys and can inject WriteErr/SendKeysErr. provider.Nudge/SendKeys/Interrupt run behind livePane: Resolve + BlockProcess, with a missing pane or an exited program yielding ErrSessionNotFound (wrapped with detail); a transport/observation error is passed through as is. Nudge types text then submits with the enter key and fails if the submit is not delivered; a typing failure skips the submit. SendKeys translates gc/tmux key names to Rex syntax (keymap.go); Interrupt sends ctrl+c.

Key names verified against the live CLI parser with a bogus target (exit 3 = key parsed, 2 = rejected): ctrl+c, alt+x, enter, esc, up/down/left/right, backspace, delete, insert, page_up, page_down, home, end, tab, space, f11, shift+tab, ctrl+left, ctrl+alt+x are all accepted. 'C-c'/'M-x'/'BSpace'/'arrow_up' are rejected by Rex, so the translation is required, not cosmetic.

Verification: go test ./..., go vet ./..., gofmt -l . clean. New tests: internal/provider/input_test.go (write-then-submit order, submit-failure surfaces, typing failure skips submit, empty nudge no-op, SendKeys translation + empty no-op, Interrupt ctrl+c, ErrSessionNotFound for missing/closed/exited panes), internal/provider/keymap_test.go, and CLI adapter send-key argv + no-op. Opt-in live e2e TestE2EInputPath (REX_E2E=1): starts /bin/sh, nudges 'echo E2E-NUDGE-OK' until it prints, runs 'sleep 300', interrupts, then nudges 'echo E2E-AFTER-INTERRUPT' and requires the output — so a failed interrupt would hang the test. Passed against the live server in 2.5s; TestE2EAgentLifecycle still passes; no scratch sessions left behind.

Independent cross-family review (Claude Sonnet, read-only) found one real defect and two hardening items; all addressed. (1) Blocker: the first e2e marker was a substring of the typed command ('echo E2E-NUDGE-OK' contains 'E2E-NUDGE-OK'), so a screen echo would satisfy it even if Enter never ran. Fixed: the nudged commands now compute their output ('echo E2E-$((20+22))' -> 'E2E-42', 'echo AFTER-$((1+1))' -> 'AFTER-2'), and the test waits for the pane's foreground process to actually become 'sleep' before interrupting. Verified by mutation: with the Interrupt call removed, the test fails after 64s with only the echoed command lines; with it, the test passes in ~0.6s, stable over 3 runs. (2) Hardened the adapter argv with '--' before the keys so a key can never be parsed as a flag ('rex send-key -s -b -- KEY...'); verified '--' works with the live CLI. (3) Added TestNudgeProcessErrorPassesThrough to pin that a transport/observation error from BlockProcess is surfaced as-is, not masked as ErrSessionNotFound (a block-not-found race stays a clear error, not a silent success). Reviewer verdict: all 5 ACs met. Residual, documented: submit confirmation is transport-level (Rex returns from send-key once the block received the key); agent-level confirmation needs program_status/idle detection and is Phase 3. No test asserts live BlockWrite/BlockEvent beyond the shell e2e; the shell exercises write + send-key.
<!-- SECTION:NOTES:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
Phase 2 adds the Rex input path behind the existing rex.Client boundary. rex.Client gained SendKeys(sessionID, blockID, keys...), implemented in the CLI adapter as 'rex send-key -s <sid> -b <bid> -- KEY...' (the pinned CLI key op; block.event would need a full physical-key payload). provider.Nudge types text and submits with the enter key, provider.SendKeys presses arbitrary keys, and provider.Interrupt sends ctrl+c. All three run behind a livePane guard: a missing pane or an exited program yields ErrSessionNotFound (wrapped with detail), while a transport/observation error is surfaced as-is rather than masked. Nudge skips the submit when typing fails and reports a failed submit instead of claiming delivery. keymap.go translates gc/tmux key names (C-c, M-x, Enter, Escape, BSpace, Up/Down/Left/Right, F1..F24, PageUp/PageDown, Delete/Insert, Tab/Space, Home/End) to Rex syntax, normalizing modifier order.

Verification. go test ./..., go vet ./..., gofmt -l . are clean. New tests at the provider-core seam (fake rex.Client): write-then-submit order and scope, submit-failure surfaces, typing failure skips submit, empty nudge/SendKeys no-ops, SendKeys translation, Interrupt ctrl+c, and ErrSessionNotFound for missing, stopped, and exited panes; a transport error passes through unmasked. keymap unit tests cover the mapping; the CLI adapter test pins the send-key argv. Every emitted key name was checked against the live rex send-key parser (exit 3 = parsed, 2 = rejected). Opt-in live TestE2EInputPath (REX_E2E=1) starts /bin/sh, nudges 'echo E2E-$((20+22))' until 'E2E-42' prints, waits until 'sleep' really holds the foreground, interrupts, then requires 'echo AFTER-$((1+1))' to print 'AFTER-2'; it passes in ~0.6s and is stable over 3 runs. Mutation check: removing the Interrupt call makes the test fail after 64s with only the echoed command lines, so it genuinely proves the submit and the interrupt. No scratch sessions were left behind; TestE2EAgentLifecycle still passes.

Risks and follow-ups. Submit confirmation is transport-level: Rex returns from send-key once the block received the key, so a nil error confirms the keystroke reached the pane, not that the agent acted on it. Agent-level confirmation and the plan's busy-agent handling (skip a nudge while a turn runs unless immediate) need program_status or idle detection, which is Phase 3. The live path exercises write + send-key only; BlockEvent and BlockClear still have no live coverage. The livePane liveness probe uses the same last_exit signal as IsRunning, so a pane whose shell outlives the agent reads as live.
<!-- SECTION:FINAL_SUMMARY:END -->
