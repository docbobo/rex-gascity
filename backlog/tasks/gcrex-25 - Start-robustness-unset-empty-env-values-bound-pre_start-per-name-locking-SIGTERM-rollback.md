---
id: GCREX-25
title: >-
  Start robustness: unset empty env values, bound pre_start, per-name locking,
  SIGTERM rollback
status: To Do
assignee: []
created_date: '2026-11-18T06:14:00Z'
labels:
  - provider
  - start
dependencies: []
references:
  - docs/provider-herdr-parity.md
  - internal/provider
  - cmd/gc-runtime-rex
priority: medium
type: task
ordinal: 50000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Four start-path weaknesses from the parity analysis (parity doc ### start rows 1, 9, 11, 12; mistakes 7 and 8): empty start-config env values are applied as env KEY= although RPP defines empty as unset and gc's tmux adapter uses env -u; pre_start hooks have no per-hook timeout; one city-wide flock is held across hooks and the launch so a slow hook stalls every start and stop; and there is no SIGTERM handler, so the rollback defer never runs (gc contains this by running stop afterwards, so it is hardening).
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 Empty start-config env values are emitted as env -u KEY in the launch command, covered by a unit test.
- [ ] #2 Each pre_start hook runs under a timeout derived from the op budget, and a timeout fails the start with a clear error.
- [ ] #3 The state lock is not held while pre_start or the blocking launch runs (per-name locking, or release and recheck), verified by a concurrency test.
- [ ] #4 A SIGTERM handler runs the existing rollback before exit, so a gc-cancelled start does not depend on gc's follow-up stop.
- [ ] #5 go test ./... and golangci-lint run ./... pass.
- [ ] #6 docs/provider-herdr-parity.md is updated in the same change: the affected section row(s) are re-verified after the fix (verdict changed, or a note added saying the deviation is now resolved), the 'Summary of deviations' row(s) and the 'Coverage' table are recomputed, and 'Confirmed mistakes' / 'Proposed follow-up work' no longer list the item.
<!-- AC:END -->
