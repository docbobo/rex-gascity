---
id: GCREX-21
title: >-
  Make nudge submit atomic-enough: bound the write-to-Enter window and reject
  non-UTF-8 input
status: To Do
assignee: []
created_date: '2026-11-18T06:13:00Z'
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
- [ ] #1 nudge gives the write and the Enter a bounded total budget derived from the op context, so a gc cancel or an unbounded stall cannot split the two steps.
- [ ] #2 Non-UTF-8 stdin is rejected with exit 1 and a stderr message before anything is written to the pane.
- [ ] #3 A write that succeeds but whose Enter fails is reported so it cannot cause silent duplication (either the op fails before the write, or the failure is distinguishable in stderr and recorded in meta).
- [ ] #4 Unit tests cover non-UTF-8 rejection, Enter failure after a successful write, and a cancelled context.
- [ ] #5 A live multi-line nudge into a Pi pane is exercised and its behavior recorded in docs/REX-CONTRACT.md section 6 or in the parity doc.
- [ ] #6 go test ./... and golangci-lint run ./... pass.
- [ ] #7 docs/provider-herdr-parity.md is updated in the same change: the affected section row(s) are re-verified after the fix (verdict changed, or a note added saying the deviation is now resolved), the 'Summary of deviations' row(s) and the 'Coverage' table are recomputed, and 'Confirmed mistakes' / 'Proposed follow-up work' no longer list the item.
<!-- AC:END -->
