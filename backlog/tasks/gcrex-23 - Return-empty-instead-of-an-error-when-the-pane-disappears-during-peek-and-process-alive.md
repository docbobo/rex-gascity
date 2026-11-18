---
id: GCREX-23
title: >-
  Return empty instead of an error when the pane disappears during peek and
  process-alive
status: To Do
assignee: []
created_date: '2026-11-18T06:13:00Z'
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
- [ ] #1 PeekLines returns empty output and exit 0 when the pane disappears mid-call, matching the already-missing case and ClearScrollback.
- [ ] #2 ProcessAlive returns false and exit 0 when BlockProcess reports ErrNotFound, matching paneAlive.
- [ ] #3 Unit tests cover the vanished-pane race for both ops.
- [ ] #4 go test ./... and golangci-lint run ./... pass.
- [ ] #5 docs/provider-herdr-parity.md is updated in the same change: the affected section row(s) are re-verified after the fix (verdict changed, or a note added saying the deviation is now resolved), the 'Summary of deviations' row(s) and the 'Coverage' table are recomputed, and 'Confirmed mistakes' / 'Proposed follow-up work' no longer list the item.
<!-- AC:END -->
