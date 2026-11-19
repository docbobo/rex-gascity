---
id: GCREX-44
title: 'interrupt: return exit 80 outcomes instead of exit 0 or exit 1'
status: Done
assignee:
  - '@mc-wisp-izlztl'
created_date: '2026-11-18T22:05:00Z'
updated_date: '2026-11-19T00:50:00Z'
labels:
  - provider
  - herdr
  - for-gascity
  - tdd
dependencies: []
references:
  - docs/provider-method-audit/interrupt.md
documentation:
  - docs/provider-herdr-method-io.md
priority: medium
type: task
ordinal: 100000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Develop test-first (TDD): write the failing test that reproduces this first, watch it go red, then the smallest change that makes it green (repo rule in AGENTS.md; see also the tdd skill).

Avoids: a delivered interrupt, a vanished pane and a dead server are indistinguishable.

Provider.Interrupt runs inside bestEffort (missing session to exit 0) and runWithContext maps every other error to exit 1. The document (## interrupt § Failure cases) specifies exit 80 with gone, not_sent or unavailable. gc v1.5.1 maps both 1 and 80 to failure, so gc is unaffected; the gain is for callers that read input-outcome.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 A missing or dead session returns exit 80 gone rather than exit 0.
- [x] #2 Unreachable-server and pre-send transport failures return exit 80 not_sent or unavailable instead of exit 1.
- [x] #3 A delivered interrupt keeps its current exit code, covered by a test.
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. Add provider.InputOutcomeError (outcome, detail, wrapped cause). Provider.Interrupt classifies: livePane ErrSessionNotFound -> gone; livePane rex.ErrUnavailable -> unavailable; any other livePane/pre-send error -> not_sent; SendKeys rex.ErrUnavailable -> unavailable; any other SendKeys error stays exit 1 (outcome unknown after a possible write). 2. Dispatcher: opInterrupt drops bestEffort and returns p.Interrupt directly; runWithContext maps *provider.InputOutcomeError through writeInputOutcome (exit 80, one JSON stdout line, '<outcome>: <detail>' stderr). 3. TDD tests: provider classification (gone for missing/dead, unavailable for server-down, not_sent for pre-send, exit-1-ish for post-write unknown, delivered nil); dispatcher exit 80 JSON for gone/unavailable/not_sent and exit 0 for delivered; update TestInputOpsOnMissingSessionSucceed (interrupt no longer best-effort). 4. Docs: REX-CONTRACT §9 adds interrupt's exit-80 words; parity doc interrupt rows 3-4 and summary reason; audit interrupt.md moves the gone/failure rows to Matches. 5. Verify go test ./... and golangci-lint run ./... .
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Implemented (uncommitted, for review). Provider: added InputOutcomeError (Outcome/Detail/Err) and interruptLookupOutcome in internal/provider/provider.go. Provider.Interrupt now classifies a livePane failure as gone (ErrSessionNotFound), unavailable (rex.ErrUnavailable) or not_sent (anything else); a SendKeys failure that is rex.ErrUnavailable is unavailable, and any other SendKeys failure stays a plain exit-1 error (outcome unknown after a possible write). Dispatcher: opInterrupt no longer wraps in bestEffort; runWithContext maps *provider.InputOutcomeError through writeInputOutcome, so interrupt now exits 80 with one {"outcome":...,"detail":...} stdout line and '<outcome>: <detail>' stderr.

TDD red->green: provider tests TestInterruptMissingPaneIsGone, TestInterruptDeadPaneIsGone, TestInterruptUnreachableIsUnavailable, TestInterruptPreSendFailureIsNotSent, TestInterruptSendUnavailableIsUnavailable, TestInterruptSendFailureStaysUnknown (input_test.go); dispatcher TestInterruptReturnsInputOutcomes (missing/dead gone, server unreachable unavailable, pre-send not_sent; exit 80 + JSON stdout, no keys sent) and TestInterruptDeliveredExitsZero (dispatch_test.go). TestInputOpsOnMissingSessionSucceed no longer lists interrupt, since it is no longer best-effort.

Docs: docs/REX-CONTRACT.md §9 records interrupt's gone/unavailable/not_sent words and the exit-1 unknown-after-write case; docs/provider-herdr-parity.md interrupt summary/implementation/rows 3-4 and verdict updated (GCREX-44), plus the exit-3 and cross-reference notes; docs/provider-method-audit/interrupt.md moved missing/dead, unreachable and pre-send rows from Deviations to Matches and added the unverified written/unwritten note.

Verification: make test -> go test ./... all packages ok; golangci-lint run ./... -> 0 issues; gofmt clean. Change left uncommitted for review.

Independent review (openrouter/deepseek/deepseek-v4.1-flash:high) of the uncommitted GCREX-44 diff vs HEAD plus the task-relevant untracked files (docs/provider-method-audit/interrupt.md, task markdown). gcrex-27/28 edits and .pi/.gc runtime state are other work, not this change.

ACs: #1 missing/dead -> exit 80 gone; #2 unreachable/pre-send -> exit 80 unavailable/not_sent; #3 delivered stays exit 0. TDD proof in a /tmp copy with only provider.go and main.go reverted to HEAD: TestInterruptReturnsInputOutcomes red (missing/dead exit 0, unreachable/pre-send exit 1) and provider tests red (undefined InputOutcomeError); green with the change. make test passes; golangci-lint 0 issues; gofmt and go vet clean; fresh -count=1 run green.

Major
- docs/provider-herdr-parity.md "Summary of deviations" interrupt rows (lines 176-177) still state the old behavior ("no live program: exit 0, silent"; "failure mapping: non-NotFound errors exit 1"), contradicting the updated detailed rows 3-4 (lines 757-758) and the code. Prior op tasks updated their summary rows in the same commit (GCREX-41 3da2ab8, GCREX-43 7b40a58), so the parity record is left self-contradictory. Update both rows and their rationale.

Minor
- docs/provider-method-audit/interrupt.md "Missing session" Matches bullet claims stderr "interrupt: gone: ..."; the provider emits "gone: session not found: <name> has no pane" (writeInputOutcome, main.go:199-214; probe-confirmed). The pack shape is "<op>: <word>[: detail]" (method-io.md Failure envelope) and REX-CONTRACT section 9 documents "<word>[: detail]". State the actual shape / record the missing prefix, or add the prefix.
- Line cites rewritten by this change are wrong: parity interrupt section "interruptKey provider.go:35" is now the InputOutcomeError type (const is :54); Interrupt is 753-769 not 753-772; interruptLookupOutcome is 774-783 not 774-786. Audit: not_sent default is provider.go:781 not :786; dead-pane branch 796-802 not 798-803; send-failure block 758-766 not 766-771; lookup is 776-779 and 763-764, not 776-785/761-767.
- REX-CONTRACT section 9 "so this code is inert at the pinned baseline" and parity row 3 "its effective best-effort handling is unchanged" overstate gc insensitivity: for gone the exit flips 0->80 versus gc v1.5.1's best-effort line (exec-session-provider.md:370), and RuntimeHandle.Interrupt propagates the error unguarded (internal/worker/runtime_handle.go:243-248); the managed SessionHandle path pre-checks IsRunning (handle_lifecycle.go:322-330, submit.go:261-275), so the common path is unaffected. method-io interrupt Conflicts #1 already records the deviation; align the wording.
- Nit: non-unavailable lookup failures map to not_sent while the pack maps a list-call non-RPC HcError to unavailable (method-io interrupt failure table). AC#2 permits either word; the audit's "Matches" could record the choice.

gc refs re-checked at tag v1.5.1 (4f698fe): exit table 0/1/2; exec runError (exec.go:159-181) reads exit 80 as a plain error; no input-outcome/carrier.ops hit; golden conformance defers Interrupt_MissingSession (runtimecontract/lockstep_test.go:75-76), so make conformance is unaffected.

Verdict: fail.

Addressed review findings (docs only; code and tests from the first pass unchanged). Parity summary rows 176-177 now match the detailed rows 3-4 and the code: interrupt gone -> exit 80 (was exit 0 silent), and failure mapping -> pre-send 80 unavailable/not_sent + unknown-after-write 1 (was non-NotFound errors exit 1). Fixed stale interrupt line cites: parity prose (Interrupt 753-769, interruptKey 54, interruptLookupOutcome 774-783), parity rows 1/3/4 (SendKeys 729, no-signal comment 750-752, gone classifier 774-783, failure mapping 753-783), and docs/provider-method-audit/interrupt.md (main.go 158-161/43/52/185/188-192 and provider.go 54,758/774-783/796-802/776-779,763-764/781/758-766/744-746/753-769/758). Stated the actual exit-80 stderr shape (gone: session not found: <name> has no pane) and added a Deviations row for the missing herdr-core <op>: prefix. REX-CONTRACT section 9 no longer claims the exit-80 code is inert: it records that a gone interrupt flips gc's best-effort exit 0 to an error on the unguarded RuntimeHandle path, while the managed SessionHandle path pre-checks IsRunning and never reaches the case (gc exec-session-provider.md:370, internal/worker/runtime_handle.go:243-248, internal/worker/handle_lifecycle.go:322-330, internal/session/submit.go:261-275). Recorded the deliberate not_sent choice for a non-unavailable lookup failure (AC#2 permits either word; the pack maps a list-call non-RPC HcError to unavailable). Checks: go test -count=1 ./... all packages ok; golangci-lint run ./... 0 issues; gofmt clean.

Second independent review (openrouter/deepseek/deepseek-v4.1-flash:high) of the uncommitted GCREX-44 diff vs HEAD plus the task's untracked files (docs/provider-method-audit/interrupt.md, task markdown). Scope unchanged: gcrex-27/28 edits, the GCREX-37/39 doc corpus (provider-herdr-method-io.md, provider-herdr-method-deviations.md), and .pi/.gc state are other work, not this change.

Round-1 findings verified fixed. Major: parity summary rows 176-177 now match detailed rows 757-758 and the code (gone -> exit 80; pre-send -> 80 unavailable/not_sent; unknown-after-write -> exit 1). Minors: the audit's Missing session bullet states the real stderr shape (gone: session not found: <name> has no pane) and a Deviations row records the missing herdr-core "<op>: " prefix; the rewritten interrupt cites are corrected; REX-CONTRACT section 9 no longer calls exit 80 inert and records the gc best-effort conflict plus both gc handle paths; the deliberate not_sent choice is recorded.

ACs against the current tree: #1 missing/dead -> interruptLookupOutcome (provider.go:774-783) + runWithContext map (main.go:181-184) -> exit 80 gone; TestInterruptMissingPaneIsGone, TestInterruptDeadPaneIsGone, TestInterruptReturnsInputOutcomes{missing,dead} assert exit 80, JSON outcome, no keys. #2 unreachable (lookup and send, provider.go:763-764) -> unavailable, other pre-send -> not_sent; TestInterruptUnreachableIsUnavailable, TestInterruptSendUnavailableIsUnavailable, TestInterruptPreSendFailureIsNotSent, dispatcher server_unreachable/pre-send_failure. Post-write unknown stays exit 1 (TestInterruptSendFailureStaysUnknown). #3 delivered -> exit 0 empty output (TestInterruptDeliveredExitsZero). Non-vacuous vs HEAD: the dispatcher test reads exit 80 + the word where HEAD answers exit 0 (missing/dead via bestEffort) or exit 1, and the provider tests name InputOutcomeError which HEAD lacks.

Verification (this review): go test -count=1 ./... ok in every package; the TestInterrupt* suite runs and passes in both packages; golangci-lint run ./... 0 issues; gofmt and go vet clean. gc claims re-checked at v1.5.1 (4f698fe): exec-session-provider.md:370, runtime_handle.go:243-248, handle_lifecycle.go:322-330, submit.go:261-275, exec.go:153-181, no input-outcome hits, lockstep_test.go:75-76. Every changed-doc cite re-resolves to the intended code/text.

Minor (follow-ups, non-blocking): (1) the +4-line insertion in runWithContext shifted main.go cites in the parity send-keys section that were correct at HEAD: line 713 uses 299, 299-305, 300-303, 195-210, 215-220 (now 303, 303-310, 304-307, 199-214, 219-224) and row 4 line 724 uses 215-220 (now 219-224); row 5 line 725 was already approximate. Docs-only. (2) docs/provider-herdr-method-deviations.md still lists the interrupt deviations as open and GCREX-44 as pending follow-up work - the known compiled-summary lag (same as recorded by the GCREX-40/41 reviews); regenerate with the doc batch.

Verdict: pass.

Verification (finalize): go test -count=1 ./... -> all packages ok; golangci-lint run ./... -> 0 issues; gofmt clean. Focused: TestInterrupt* in internal/provider and cmd/gc-runtime-rex pass (missing/dead -> gone, unreachable -> unavailable, pre-send -> not_sent, post-write -> exit 1, delivered -> exit 0); TestInterruptReturnsInputOutcomes and TestInterruptDeliveredExitsZero assert the dispatcher exit codes/JSON.
<!-- SECTION:NOTES:END -->

## Comments

<!-- COMMENTS:BEGIN -->
author: @mc-wisp-izlztl
created: 2026-11-19T00:22:00Z

---

Claimed by Gas City worker mc-wisp-izlztl (seat rex-gascity/pi-1, session pi-mc-wisp-izlztl, template rex-gascity/pi); workflow bead mc-10xe, step bead mc-10xe.

---
<!-- COMMENTS:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
Provider.Interrupt now returns typed input outcomes instead of collapsing to exit 0/exit 1: a missing or dead pane -> exit 80 gone, an unreachable Rex server -> exit 80 unavailable, another pre-send failure -> exit 80 not_sent, and a send failure that may have written the request stays exit 1 (outcome unknown); a delivered interrupt stays exit 0. Implemented via provider.InputOutcomeError, interruptLookupOutcome in internal/provider/provider.go, and runWithContext mapping it through writeInputOutcome; opInterrupt no longer wraps Interrupt in bestEffort. Docs updated: REX-CONTRACT section 9, the parity interrupt rows/summary, and the interrupt method audit. Verified with go test -count=1 ./..., golangci-lint run ./... (0 issues) and gofmt; TDD red->green shown by the new TestInterrupt* suite against HEAD. Risk: for the gone case gc v1.5.1's unguarded RuntimeHandle.Interrupt path now sees an error where its contract expects a best-effort exit 0 (documented in REX-CONTRACT section 9 and method-io interrupt Conflicts #1); the managed SessionHandle path pre-checks IsRunning and is unaffected. Docs-only follow-ups remain: stale main.go line cites in the parity send-keys section and the compiled deviation summary lag (GCREX-37/39 doc batch).
<!-- SECTION:FINAL_SUMMARY:END -->
