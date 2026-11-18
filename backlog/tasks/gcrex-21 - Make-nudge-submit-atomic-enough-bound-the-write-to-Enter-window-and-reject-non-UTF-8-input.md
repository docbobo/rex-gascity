---
id: GCREX-21
title: >-
  Make nudge submit atomic-enough: bound the write-to-Enter window and reject
  non-UTF-8 input
status: Done
assignee:
  - '@mc-wisp-ga8ni'
created_date: '2026-11-18T06:13:00Z'
updated_date: '2026-11-18T13:08:00Z'
labels:
  - provider
  - input
dependencies: []
references:
  - docs/provider-herdr-parity.md
  - internal/provider
  - cmd/gc-runtime-rex
priority: high
type: bug
ordinal: 46000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
The nudge op writes text and then sends Enter as two Rex calls with no internal deadline (internal/provider/provider.go, opNudge/Nudge). gc retries a failed nudge from its queue (15 s, 5 attempts) or the idle backstop (3 attempts), re-typing the whole text, so an exit 1 after the write reached the pane duplicates the prompt; a gc cancel between the two calls leaves text typed but unsubmitted. Non-UTF-8 stdin is silently rewritten to U+FFFD by json.Marshal and then typed (parity doc ### nudge rows 5, 8, 9, mistake 2). Multi-line behavior is untested.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 nudge gives the write and the Enter a bounded total budget derived from the op context, so a gc cancel or an unbounded stall cannot split the two steps.
- [x] #2 Non-UTF-8 stdin is rejected with exit 1 and a stderr message before anything is written to the pane.
- [x] #3 A write that succeeds but whose Enter fails is reported so it cannot cause silent duplication (either the op fails before the write, or the failure is distinguishable in stderr and recorded in meta).
- [x] #4 Unit tests cover non-UTF-8 rejection, Enter failure after a successful write, and a cancelled context.
- [x] #5 A live multi-line nudge into a Pi pane is exercised and its behavior recorded in docs/REX-CONTRACT.md section 6 or in the parity doc.
- [x] #6 go test ./... and golangci-lint run ./... pass.
- [x] #7 docs/provider-herdr-parity.md is updated in the same change: the affected section row(s) are re-verified after the fix (verdict changed, or a note added saying the deviation is now resolved), the 'Summary of deviations' row(s) and the 'Coverage' table are recomputed, and 'Confirmed mistakes' / 'Proposed follow-up work' no longer list the item.
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. Add a bounded submit deadline shared by BlockWrite and SendKeys, derived from the op context deadline (fallback bounded default). Once the write succeeds, attempt Enter on a context detached from parent cancellation but still bounded by the same deadline, so a cancel/stall cannot leave text typed but unsubmitted.
2. Reject non-UTF-8 stdin in opNudge before the provider runs: exit 1 with a stderr message and no pane write.
3. Distinguish a post-write Enter failure: error text says the text was typed, and record a GC_NUDGE_SUBMIT_FAILED meta key (cleared on the next successful submit).
4. Tests first: cancelled context writes nothing; non-UTF-8 stdin exits 1 with no write; Enter failure after a write is distinguishable in stderr and meta; the submit budget is bounded.
5. Live multi-line nudge into a scratch Pi pane and record the observed behavior in docs/REX-CONTRACT.md §6 and the parity doc.
6. Re-verify parity doc nudge rows 6/8/9 and the Summary/Coverage tables, drop the resolved mistakes and follow-ups.
7. go test ./... and golangci-lint run ./...; finalize and commit.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Verification: make test passes (cmd/gc-runtime-rex, internal/provider, internal/rex/cli); go vet ./... clean; golangci-lint run ./... reports 0 issues. Named tests: TestNudgeWriteIsBoundedByTheSubmitBudget, TestNudgeCanceledContextWritesNothing, TestNudgeRejectsInvalidUTF8, TestNudgeEnterFailureAfterWriteIsReported, TestNudgeWritesTextThenSubmits, TestNudgeFailsWhenSubmitIsNotDelivered.

Completed the partial GCREX-21 implementation found in the working tree, on top of the then-uncommitted GCREX-16 program_status work. Code: internal/provider/nudge.go (one submit deadline for write+Enter derived from the op context, default 10s; Enter attempted on a context detached from parent cancellation but still bounded by that deadline; GC_NUDGE_SUBMIT_FAILED meta on a post-write Enter failure, cleared on the next successful submit), cmd/gc-runtime-rex/main.go (invalid UTF-8 rejected with exit 1 before any Rex call), plus tests and parity/contract docs. Did not discard the prior partial work.

Live multi-line probe re-run: created a scratch Rex session gcrex21-probe running interactive Pi 1.5.2 (Rex revision 14) in /tmp; sent block write {"data":"line one\nline two"} then a separate enter key. Capture showed both lines in one composer submission and a single "OK" reply, confirming the newline is inserted without submitting and the separate Enter submits the whole text. Recorded in docs/REX-CONTRACT.md §6 and docs/provider-herdr-parity.md nudge row 6. Scratch session killed afterwards.
<!-- SECTION:NOTES:END -->

## Comments

<!-- COMMENTS:BEGIN -->
author: @mc-wisp-hw7ip
created: 2026-11-18T10:22:00Z

---

Claimed by Gas City worker mc-wisp-hw7ip (seat pi-mc-wisp-hw7ip, session mc-wisp-hw7ip, template rex-gascity/pi); workflow bead mc-kgb, root bead mc-fyq.

---

author: @mc-wisp-ga8ni
created: 2026-11-18T13:07:00Z

---

Claimed by Gas City worker mc-wisp-ga8ni (seat pi-mc-wisp-ga8ni, session mc-wisp-ga8ni, template rex-gascity/pi); workflow bead mc-p9m, root bead mc-9cb. Continuing the partial implementation left in the working tree (nudge.go, UTF-8 rejection, bounded submit, docs); did not discard it.

---
<!-- COMMENTS:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
Made nudge submit atomic-enough and rejected non-UTF-8 input. The RPP nudge op now rejects invalid UTF-8 with exit 1 and a stderr message before touching Rex. Provider nudge uses one submit budget (10 s or the op context deadline, whichever is earlier) for write+Enter; after a successful write it still attempts Enter on a context detached from parent cancellation but bounded by that deadline, so a gc cancel or stall cannot leave text typed and unsubmitted. A post-write Enter failure exits non-zero with "text was typed but submit key failed" and records GC_NUDGE_SUBMIT_FAILED in session meta; the next successful submit clears it. A live two-line nudge into an idle Pi 1.5.2 pane arrived as one submitted turn. GCREX-16 program_status gating/confirmation, which the working tree contained uncommitted, is landed with this change. Verification: make test passes, go vet clean, golangci-lint run ./... reports 0 issues; named tests cover bounded write, cancelled context, invalid UTF-8, and post-write Enter failure. Residual risk: the remaining split window is the transport call latency inside the shared budget; control bytes other than newline were not probed.
<!-- SECTION:FINAL_SUMMARY:END -->
