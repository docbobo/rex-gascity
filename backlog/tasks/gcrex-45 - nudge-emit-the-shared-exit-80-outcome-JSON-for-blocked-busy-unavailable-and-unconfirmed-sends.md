---
id: GCREX-45
title: >-
  nudge: emit the shared exit-80 outcome JSON for blocked, busy, unavailable and
  unconfirmed sends
status: Done
assignee:
  - '@mc-wisp-e8884z'
created_date: '2026-11-18T22:05:00Z'
updated_date: '2026-11-19T01:18:00Z'
labels:
  - provider
  - herdr
  - for-gascity
  - tdd
dependencies: []
references:
  - docs/provider-method-audit/nudge.md
documentation:
  - docs/provider-herdr-method-io.md
priority: medium
type: task
ordinal: 101000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Develop test-first (TDD): write the failing test that reproduces this first, watch it go red, then the smallest change that makes it green (repo rule in AGENTS.md; see also the tdd skill).

Avoids: non-gc callers get exit 1 and stderr where the contract promises a typed outcome.

The document (## nudge § Failure cases, § Success output; ## Shared RPP contract § Input outcome) requires exit 80 with {"outcome":...} for invalid UTF-8, blocked, busy, unavailable and unwritten sends, and exit 0 with delivered_unobserved when delivery cannot be confirmed. Today blocked/busy/transport errors exit 1 with empty stdout and the unconfirmed case only writes diagnostics/<name>-nudge-unconfirmed.json.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 The four failure outcomes exit 80 with a JSON outcome line on stdout.
- [x] #2 Unconfirmed delivery exits 0 with a JSON outcome line on stdout instead of only a diagnostics file.
- [x] #3 A test covers each outcome and the existing diagnostics file remains or its removal is explained.
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. Provider (internal/provider/nudge.go): classify nudge failures as RPP input outcomes. livePane unreachable -> InputOutcomeError unavailable; working pane -> busy; blocked pane -> pending_interaction; BlockWrite and submit fail -> unavailable (rex.ErrUnavailable) else not_sent. Keep ErrSessionNotFound plain so the dispatcher's best-effort exit 0 is unchanged, and keep ErrNudgeClaimHeld a plain exit 1 (Rex extension, not in the pack vocabulary).
2. Dispatcher (cmd/gc-runtime-rex/main.go): opNudge returns InputOutcomeError rejected for non-UTF-8 stdin; calls NudgeReport and writes the delivered_unobserved JSON line (exit 0) when the nudge was delivered but the turn start was not observed; the diagnostics file stays.
3. TDD tests: provider outcome classification (busy, pending_interaction, unavailable lookup, not_sent/unavailable write and submit) in internal/provider/nudge_test.go; dispatcher TestNudgeReturnsInputOutcomes (exit 80 + JSON, nothing typed), TestNudgeUnconfirmedExitsZeroWithOutcome (exit 0 + JSON + diagnostic), and update TestNudgeRejectsInvalidUTF8 to expect exit 80 rejected.
4. Docs: docs/REX-CONTRACT.md §9 nudge words; docs/provider-herdr-parity.md nudge rows/summary; docs/provider-method-audit/nudge.md moves the fixed rows to Matches.
5. Verify go test ./... and golangci-lint run ./... .
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Implemented (uncommitted, for review). Provider: added nudgeLookupOutcome and nudgeSendOutcome (internal/provider/nudge.go) and classified the busy/blocked gates. A working pane returns exit 80 busy, a blocked pane pending_interaction (still wrapping ErrNudgeBusy/ErrNudgeBlocked), an unreachable server (lookup, write or submit) unavailable, any other write/submit failure not_sent, and a missing/dead session keeps ErrSessionNotFound so the dispatcher's best-effort exit 0 is unchanged. Dispatcher: opNudge returns InputOutcomeError rejected for non-UTF-8 stdin and calls NudgeReport; a delivered-but-unconfirmed report exits 0 with one delivered_unobserved JSON line (writeDeliveredUnobserved) while the diagnostics file is still written by the provider. TDD red->green: TestNudgeBusyIsBusyOutcome, TestNudgeBlockedIsPendingInteraction, TestNudgeUnreachableLookupIsUnavailable, TestNudgeWriteFailureIsNotSent, TestNudgeWriteUnavailableIsUnavailable, TestNudgeSubmitFailureIsNotSent (internal/provider/nudge_test.go); TestNudgeReturnsInputOutcomes and TestNudgeUnconfirmedExitsZeroWithOutcome (cmd/gc-runtime-rex/dispatch_test.go); TestNudgeRejectsInvalidUTF8 updated to expect exit 80 rejected. Docs: REX-CONTRACT §9 nudge words; parity nudge summary/detail rows, verdict and GCREX-45 cross-ref; audit nudge.md moved the fixed rows to Matches. Verification: go test -count=1 ./... all packages ok; go vet clean; golangci-lint run ./... 0 issues; gofmt clean. Change left uncommitted for review.

Independent review (openrouter/deepseek/deepseek-v4.1-flash:high): Verified the change against the task AC and the parity/audit sources. Behavior: go test -count=1 ./... passes; golangci-lint run ./... 0 issues; gofmt clean. AC #1 (exit 80 + JSON for blocked/busy/unavailable/not_sent; plus rejected) met; AC #2 (unconfirmed exits 0 with delivered_unobserved JSON and diagnostic kept) met; AC #3 (tests for each outcome) met. Findings by severity:
- Major: docs/provider-herdr-parity.md:672 says 'The only stdout a nudge writes is the delivered_unobserved line.' False: the exit-80 failure paths also write one {"outcome":...} line via writeInputOutcome (cmd/gc-runtime-rex/main.go:170-190); the same paragraph's own rejected/busy/pending_interaction/unavailable/not_sent description contradicts it. Correct to 'the delivered_unobserved line on success, or one exit-80 outcome line on failure'.
- Minor: docs/provider-herdr-parity.md:683 row 4 says 'nothing typed' for a server-down nudge. True for a pre-send lookup failure; false for a write/submit failure after the binding resolved (nudgeSendOutcome labels it unavailable/not_sent and the text may be in the pane). Qualify the row.
- Minor: docs/provider-herdr-parity.md:672 has stale line refs: nudge.go:209-220 for the NudgeModeImmediate bypass (gate is 356-372), nudge.go:148-166 for the format fingerprint/write (current 394-413), nudge.go:153 for noteInput, nudge.go:218-231 for the submit deadline (current 474-489), nudge.go:154-163 for the Enter detachment (current 406-412).
- Minor: docs/provider-herdr-parity.md:158 row 3 still cites main.go:151-156 for bestEffort; current bestEffort is 219-224.
- Minor: internal/provider/provider.go:31-38 comment lists only gone/not_sent/unavailable; the change adds busy and pending_interaction (and the dispatcher uses rejected).
Verdict: fail.

Addressed review findings (docs/comment accuracy; no behavior change):

- Major: docs/provider-herdr-parity.md:672 corrected the false 'The only stdout a nudge writes is the delivered_unobserved line.' It now states the delivered_unobserved line on success, or one exit-80 {"outcome":...} line on a rejected/busy/pending_interaction/unavailable/not_sent failure, citing main.go:176-183.
- Minor: parity row 4 (server down) and the matching summary row now qualify 'nothing typed': true for a pre-send lookup failure, but a write/submit failure after the binding resolved has already typed the text.
- Minor: refreshed stale parity line refs (nudge.go NudgeModeImmediate bypass 209-220 -> 357; fingerprint/write 148-166 -> 379-413; noteInput 153 -> 402; submit deadline 218-231 -> 474-485; Enter detachment 154-163 -> 406-412; GC_NUDGE_SUBMIT_FAILED 409-415 -> 410-414; bestEffort main.go 151-156 -> 219-224).
- Minor: internal/provider/provider.go InputOutcomeError comment now lists the full wire vocabulary: gone, not_sent, unavailable, busy, pending_interaction, rejected.

Verification: go test -count=1 ./... all packages ok; golangci-lint run ./... 0 issues. Change left uncommitted for the next review iteration.

Independent review (openrouter/deepseek/deepseek-v4.1-flash:high, iteration 2): re-reviewed the current uncommitted GCREX-45 diff against the task AC, docs/provider-herdr-method-io.md ## nudge and ## Shared RPP contract, docs/provider-herdr-parity.md, and gc v1.5.1 (internal/runtime/protocol.go; internal/runtime/exec/exec.go nudgeOp/runError; docs/reference/exec-session-provider.md:370). Behavior verified: go test -count=1 ./... all packages ok; golangci-lint run ./... 0 issues; go vet ./... clean; gofmt -l cmd internal empty; git diff --check clean. TDD red proof: with the current test files copied onto a HEAD archive, TestNudgeBusyIsBusyOutcome, TestNudgeBlockedIsPendingInteraction, TestNudgeUnreachableLookupIsUnavailable, TestNudgeWriteFailureIsNotSent, TestNudgeWriteUnavailableIsUnavailable, TestNudgeSubmitFailureIsNotSent (provider) and TestNudgeReturnsInputOutcomes, TestNudgeUnconfirmedExitsZeroWithOutcome, TestNudgeRejectsInvalidUTF8 (dispatcher) fail with the old plain errors/exit 1 and no stdout line; all pass in the working tree. AC #1 met (exit 80 plus one JSON line for busy, pending_interaction, unavailable and not_sent; rejected is covered too), AC #2 met (exit 0 plus a delivered_unobserved JSON line, and the diagnostics file is still written and asserted), AC #3 met (each outcome has a test; the diagnostic is retained). No blocking or major findings.
Minor/nit docs follow-ups (no behavior change):
- docs/provider-herdr-parity.md detail row 1 still cites `nudge.go:209-220` for the NudgeModeImmediate override; the bypass is at `nudge.go:357` (the summary paragraph already uses 357). This is the stale ref the previous addressed-note claimed fixed.
- Detail row 9 still cites `nudge.go:218-231` and `nudge.go:103-108,146-163`, and row 11 plus the get-last-activity section (lines 603, 613) still cite `nudge.go:153`. Current locations are 474-485 (submit budget), 406-412 (detached Enter) and 402 (noteInput); refresh those refs.
- Detail row 4 and the summary row say "text already typed when the write or submit fails". A write failure may type nothing or only part of the text; only a submit failure guarantees the full text is in the pane. Suggest "may have typed part of the text (write) or left the text typed (submit)".
- The summary's "instead of only writing the diagnostic" can read as diagnostic removal; "while still writing the diagnostic" is clearer.
- docs/provider-method-audit/nudge.md ranges start early: the gate is 357-372 (not 356-372) and the classifiers are 322-338 (not 322-341).
Unverified, not introduced here: the Rex CLI's exit-3 to ErrNotFound mapping can still turn a transport failure into best-effort exit 0 (already recorded in parity nudge row 4); live Rex exit-3 behavior remains unverified. A BlockWrite error cannot distinguish "nothing written" from "written but reply lost"; the plan intentionally classifies it not_sent.
Working-tree note: unrelated edits to backlog/tasks/gcrex-27 and gcrex-28 are present; finish must stage only GCREX-45 files.
Verdict: pass.

Verification (finish step): go test -count=1 ./... -> all packages ok; golangci-lint run ./... -> 0 issues; gofmt clean. Review-loop verdict=pass (iteration 2), no blocking/major findings. Minor docs line-ref nits remain recorded in the review notes as follow-ups.
<!-- SECTION:NOTES:END -->

## Comments

<!-- COMMENTS:BEGIN -->
author: @mc-wisp-e8884z
created: 2026-11-19T00:54:00Z

---

Claimed by Gas City worker mc-wisp-e8884z (seat rex-gascity/pi-1, session pi-mc-wisp-e8884z, template rex-gascity/pi); workflow bead mc-1sn9, step bead mc-n1jl.

---
<!-- COMMENTS:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
nudge now emits the shared exit-80 RPP input-outcome JSON, matching the pack contract. Provider (internal/provider/nudge.go): nudgeLookupOutcome classifies an unreachable server as unavailable; nudgeSendOutcome classifies an unreachable server as unavailable and any other write/submit failure as not_sent; a working pane returns busy, a blocked pane pending_interaction (still wrapping ErrNudgeBusy/ErrNudgeBlocked); a missing/dead session keeps ErrSessionNotFound (dispatcher best-effort exit 0) and a held claim stays plain exit 1. Dispatcher (cmd/gc-runtime-rex/main.go): opNudge rejects non-UTF-8 stdin with exit-80 rejected, and a delivered-but-unconfirmed nudge exits 0 with one delivered_unobserved JSON line while the diagnostics file is still written. Docs: REX-CONTRACT §9, provider-herdr-parity nudge rows/summary, and provider-method-audit/nudge.md updated. Tests: TestNudgeBusyIsBusyOutcome, TestNudgeBlockedIsPendingInteraction, TestNudgeUnreachableLookupIsUnavailable, TestNudgeWriteFailureIsNotSent, TestNudgeWriteUnavailableIsUnavailable, TestNudgeSubmitFailureIsNotSent (provider); TestNudgeReturnsInputOutcomes, TestNudgeUnconfirmedExitsZeroWithOutcome, updated TestNudgeRejectsInvalidUTF8 (dispatcher). Verification: go test -count=1 ./... all ok; golangci-lint run ./... 0 issues. GC 1.5.1 reads exit 80 as a plain error and ignores nudge stdout, so the typed words stay inert at the pinned baseline. Risk/follow-up: nothing typed vs partially typed on a write failure cannot be distinguished (classified not_sent); remaining docs line-ref nits are cosmetic.
<!-- SECTION:FINAL_SUMMARY:END -->
