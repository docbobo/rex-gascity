---
id: GCREX-24
title: Record pack input time and treat a working or blocked program as active
status: Done
assignee:
  - '@mc-wisp-ibl4x'
created_date: '2026-11-18T06:13:00Z'
updated_date: '2026-11-18T13:17:00Z'
labels:
  - provider
  - activity
dependencies: []
references:
  - docs/provider-herdr-parity.md
  - internal/provider
  - cmd/gc-runtime-rex
priority: high
type: bug
ordinal: 49000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
get-last-activity reports the newest program_status UpdatedAt or a screen-hash change, so a silent but busy agent (long tool run) looks idle to gc, which idle-stops, sleeps, recycles on progress-stall, escalates a drain kill and nudges on the same value (parity doc ### get-last-activity rows 2 and 3, mistake 3). herdr-core returns 'now' for working/blocked and keeps a pack-input floor.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 A session whose newest program_status state is working or blocked reports the current time.
- [x] #2 A successful nudge or send-keys records an input time that acts as a floor for the reported activity, so typing into a pane is not invisible until the screen changes.
- [x] #3 An unreadable or unknown source still reports nothing (exit 0, empty) rather than a stale stamp, matching the current unknown handling.
- [x] #4 Unit tests cover an old working record yielding a fresh time, an input floor dominating a stale screen hash, and an unknown source yielding empty.
- [x] #5 go test ./... and golangci-lint run ./... pass.
- [x] #6 docs/provider-herdr-parity.md is updated in the same change: the affected section row(s) are re-verified after the fix (verdict changed, or a note added saying the deviation is now resolved), the 'Summary of deviations' row(s) and the 'Coverage' table are recomputed, and 'Confirmed mistakes' / 'Proposed follow-up work' no longer list the item.
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. Extend ActivityRecord with an InputAt floor and persist it (input_updated_at) in the file store; old records lack it and read as no floor.
2. Add Provider.noteInput(binding): load the name's record, keep its fingerprint/time, stamp InputAt=now. Call it after a successful nudge write and after a successful send-keys.
3. GetLastActivity: pick the newest program_status record; if its state is working or blocked, report now. Otherwise apply max(base, input floor) for both the program_status and format-poll paths. Floor applies only when the record's identity is the resolved block; the no-binding path stays ActivityNone/zero.
4. Tests: old working record yields ~now; input floor dominates an unchanged screen hash; unknown source with an input record stays empty; nudge and send-keys record the floor.
5. Run go test ./... and golangci-lint run ./....
6. Update docs/provider-herdr-parity.md: re-verify the get-last-activity rows, recompute Summary/Coverage, drop mistake #1 and follow-up #4.
7. Commit only this task's files and push.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Implemented in internal/provider/activity.go: GetLastActivity now takes the newest program_status record (newestProgramStatus) and reports now when its state is working or blocked; every returned time is raised to the ActivityRecord.InputAt floor for the resolved block. ActivityRecord gained InputAt (persisted as input_updated_at in stores.go), and noteInput stamps it after a successful nudge write (nudge.go) and a successful send-keys (provider.go). The floor is ignored when the record's identity is not the resolved block, and the no-binding path still returns ActivityNone/zero.
Tests: TestGetLastActivityWorkingOrBlockedRecordYieldsNow, TestGetLastActivityInputFloorDominatesStaleScreenHash, TestGetLastActivityUnknownSourceIgnoresInputFloor, TestNudgeRecordsInputFloor, TestSendKeysRecordsInputFloor. Verification: go test ./... (ok github.com/docbobo/rex-gascity/internal/provider, cmd/gc-runtime-rex, internal/rex/cli); golangci-lint run ./... -> 0 issues. The three activity tests are red before the fix (InputAt did not exist and the working record returned the stale stamp).
Parity doc: docs/provider-herdr-parity.md get-last-activity rows 2/3 and the nudge/send-keys activity rows moved from mistake/not-implemented to intentional, the section verdict is now intentional, Coverage and Summary recomputed, Confirmed mistakes no longer lists get-last-activity (renumbered), and Proposed follow-up 4 removed.

Verification evidence: go test ./... all packages ok; golangci-lint run ./... -> 0 issues. Named tests: TestGetLastActivityWorkingOrBlockedRecordYieldsNow (AC1), TestGetLastActivityInputFloorDominatesStaleScreenHash + TestNudgeRecordsInputFloor + TestSendKeysRecordsInputFloor (AC2), TestGetLastActivityUnknownSourceIgnoresInputFloor (AC3).
<!-- SECTION:NOTES:END -->

## Comments

<!-- COMMENTS:BEGIN -->
author: @mc-wisp-ibl4x
created: 2026-11-18T13:13:00Z

---

Claimed by Gas City worker mc-wisp-ibl4x (seat mc-wisp-ibl4x, session pi-mc-wisp-ibl4x, template rex-gascity/pi); workflow bead mc-vdv, step bead mc-3uf.

---
<!-- COMMENTS:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
GetLastActivity now returns the current time when the newest program_status state is working or blocked, and raises every reported time to a per-incarnation pack-input floor recorded by nudge and send-keys. This closes parity mistake 1: gc idle-stops, sleeps, recycles on progress-stall and escalation-kills on the stale stamp, so a silent but busy Rex agent used to read as idle. ActivityRecord gained InputAt (persisted as input_updated_at); noteInput is called after a successful nudge write and a successful send-keys. The floor is ignored for a record from another incarnation and the no-binding path still reports nothing. Verification: go test ./... and golangci-lint run ./... pass. Docs: provider-herdr-parity.md get-last-activity rows and the nudge/send-keys activity rows are now intentional, Coverage/Summary recomputed, Confirmed mistakes and Proposed follow-up no longer list the item. Risks: the input floor lives in the same sidecar and is only dropped when a new pane identity is observed, so an unclean stop could leave a recent floor for a name until the next resolve; low impact and bounded by keyboard-interaction timing.
<!-- SECTION:FINAL_SUMMARY:END -->
