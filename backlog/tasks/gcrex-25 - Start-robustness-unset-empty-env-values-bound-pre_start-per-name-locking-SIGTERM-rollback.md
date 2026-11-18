---
id: GCREX-25
title: 'Harden the Rex start path (split: RPP env conformance + start robustness)'
status: To Do
assignee: []
created_date: '2026-11-18T06:14:00Z'
updated_date: '2026-11-18T08:15:00Z'
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
Umbrella for the start-path work that was GCREX-25, split on 2026-11-18 during a task-type review.

The parity analysis found four start-path weaknesses (docs/provider-herdr-parity.md `### start` rows 1, 9, 11, 12; mistakes 7 and 8). They split by kind:

- **RPP-conformance defect** — empty start-config env values are applied as `env KEY=` although RPP v0 defines empty as unset and gc's tmux adapter uses `env -u` → **GCREX-25.1** (bug).
- **Hardening and concurrency** — pre_start hook timeout, per-name locking, SIGTERM rollback → **GCREX-25.2** (task).

The two subtasks are independent and can land separately.
<!-- SECTION:DESCRIPTION:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Split into GCREX-25.1 (bug: empty env values as env -u) and GCREX-25.2 (task: pre_start timeout, per-name locking, SIGTERM rollback) on 2026-11-18 during a task-type review. Original description and acceptance criteria moved to the subtasks; this task is now the umbrella.
<!-- SECTION:NOTES:END -->
