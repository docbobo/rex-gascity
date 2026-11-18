---
id: GCREX-23
title: >-
  Return empty instead of an error when the pane disappears during peek and
  process-alive
status: Done
assignee:
  - '@mc-wisp-bblcfz'
created_date: '2026-11-18T06:13:00Z'
updated_date: '2026-11-18T15:17:00Z'
labels:
  - provider
  - liveness
dependencies: []
references:
  - docs/provider-herdr-parity.md
  - internal/provider
  - cmd/gc-runtime-rex
priority: medium
type: bug
ordinal: 48000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Two ops answer differently for the same condition: a session that is already missing returns empty/exit 0, but a pane that vanishes between resolve and the Rex call fails with exit 1. PeekLines does not catch ErrNotFound from BlockFormat, and ProcessAlive does not catch ErrNotFound from BlockProcess, although paneAlive and ClearScrollback already treat it as gone (parity doc ### peek row 2 and ### process-alive row 3, mistakes 5 and 6).
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 PeekLines returns empty output and exit 0 when the pane disappears mid-call, matching the already-missing case and ClearScrollback.
- [x] #2 ProcessAlive returns false and exit 0 when BlockProcess reports ErrNotFound, matching paneAlive.
- [x] #3 Unit tests cover the vanished-pane race for both ops.
- [x] #4 go test ./... and golangci-lint run ./... pass.
- [x] #5 docs/provider-herdr-parity.md is updated in the same change: the affected section row(s) are re-verified after the fix (verdict changed, or a note added saying the deviation is now resolved), the 'Summary of deviations' row(s) and the 'Coverage' table are recomputed, and 'Confirmed mistakes' / 'Proposed follow-up work' no longer list the item.
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. Add a fake Rex BlockFormat error hook so a format call can fail with ErrNotFound. 2. Catch rex.ErrNotFound in Provider.PeekLines (empty output, nil) and Provider.ProcessAlive (false, nil), matching the already-missing path and ClearScrollback/paneAlive. 3. Add provider and dispatcher tests for the vanished-pane race on both ops. 4. Re-verify the process-alive and peek rows in docs/provider-herdr-parity.md; update their verdicts, the summary rows, the coverage table, and remove the resolved items from Confirmed mistakes and Proposed follow-up work. 5. Run go test ./... and golangci-lint run ./... and append verification notes.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Implementation complete in the working tree (uncommitted by design for this worker step).

- Added faketest.Fake.FormatErr so BlockFormat can return an error and reproduce the vanished-pane race.
- Provider.PeekLines now maps rex.ErrNotFound from BlockFormat to empty output, nil; Provider.ProcessAlive maps rex.ErrNotFound from BlockProcess to false, nil. Both match the already-missing paths and ClearScrollback/paneAlive.
- Tests: internal/provider/phase4_test.go TestPeekLinesVanishedPane; internal/provider/teardown_test.go TestProcessAliveVanishedPane; cmd/gc-runtime-rex/dispatch_test.go TestPeekLines and TestProcessAlive cover the same race through the dispatcher.
- docs/provider-herdr-parity.md: process-alive missing-pane and peek vanished-mid-call rows now verdict intentional; summary/coverage rows recomputed; resolved items removed from Confirmed mistakes and Proposed follow-up work; follow-up numbering updated.
- Verification: go test ./... passes; golangci-lint run ./... reports 0 issues.
- No commit, no Done transition, task-id file retained per the worker step instructions.

Verification: go test -count=1 ./internal/provider ./cmd/gc-runtime-rex passes; go test ./... passes; golangci-lint run ./... reports 0 issues. New tests TestPeekLinesVanishedPane and TestProcessAliveVanishedPane exercise ErrNotFound from BlockFormat/BlockProcess; dispatcher tests cover the same race.
<!-- SECTION:NOTES:END -->

## Comments

<!-- COMMENTS:BEGIN -->
author: @mc-wisp-bblcfz
created: 2026-11-18T14:58:00Z

---

Claimed by Gas City worker mc-wisp-bblcfz (seat rex-gascity/pi-1, session pi-mc-wisp-bblcfz, template rex-gascity/pi); workflow bead unknown, step bead unknown.

---
<!-- COMMENTS:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
PeekLines and ProcessAlive now treat rex.ErrNotFound as an empty/false answer instead of an error when a pane vanishes between resolve and the Rex call. Added FormatErr to the fake Rex client and unit/dispatcher tests for both ops. Updated docs/provider-herdr-parity.md: the peek and process-alive missing-pane rows are now intentional, summary/coverage rows recomputed, and the resolved items removed from Confirmed mistakes and follow-up work. Verification: go test ./... and golangci-lint run ./... pass. Risk: the review-loop verdict file was lost to a state-file race; this finalization performed a direct review of the diff before committing.
<!-- SECTION:FINAL_SUMMARY:END -->
