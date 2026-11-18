---
id: GCREX-25.2
title: 'Start robustness: bound pre_start hooks, per-name locking, SIGTERM rollback'
status: To Do
assignee: []
created_date: '2026-11-18T08:15:00Z'
labels:
  - provider
  - start
dependencies: []
references:
  - docs/provider-herdr-parity.md
  - internal/provider
  - cmd/gc-runtime-rex
parent_task_id: GCREX-25
priority: medium
type: task
ordinal: 60000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Three start-path robustness weaknesses from the parity analysis, split out of GCREX-25 on 2026-11-18 (task-type review) as hardening/concurrency work:

- pre_start hooks have no per-hook timeout;
- one city-wide flock is held across the hooks and the blocking launch, so a slow hook stalls every start and stop;
- there is no SIGTERM handler, so the rollback defer never runs (gc contains this by running stop afterwards, so this part is hardening).

Source: docs/provider-herdr-parity.md `### start` rows 9, 11, 12; mistake 8.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 Each pre_start hook runs under a timeout derived from the op budget, and a timeout fails the start with a clear error.
- [ ] #2 The state lock is not held while pre_start or the blocking launch runs (per-name locking, or release and recheck), verified by a concurrency test.
- [ ] #3 A SIGTERM handler runs the existing rollback before exit, so a gc-cancelled start does not depend on gc's follow-up stop.
- [ ] #4 go test ./... and golangci-lint run ./... pass.
- [ ] #5 docs/provider-herdr-parity.md is updated in the same change: the affected start row(s) are re-verified, the Summary of deviations and Coverage tables are recomputed, and Confirmed mistakes / Proposed follow-up work no longer list the item.
<!-- AC:END -->
