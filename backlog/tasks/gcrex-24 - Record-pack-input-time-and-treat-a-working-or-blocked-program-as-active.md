---
id: GCREX-24
title: Record pack input time and treat a working or blocked program as active
status: To Do
assignee: []
created_date: '2026-11-18T06:13:00Z'
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
- [ ] #1 A session whose newest program_status state is working or blocked reports the current time.
- [ ] #2 A successful nudge or send-keys records an input time that acts as a floor for the reported activity, so typing into a pane is not invisible until the screen changes.
- [ ] #3 An unreadable or unknown source still reports nothing (exit 0, empty) rather than a stale stamp, matching the current unknown handling.
- [ ] #4 Unit tests cover an old working record yielding a fresh time, an input floor dominating a stale screen hash, and an unknown source yielding empty.
- [ ] #5 go test ./... and golangci-lint run ./... pass.
- [ ] #6 docs/provider-herdr-parity.md is updated in the same change: the affected section row(s) are re-verified after the fix (verdict changed, or a note added saying the deviation is now resolved), the 'Summary of deviations' row(s) and the 'Coverage' table are recomputed, and 'Confirmed mistakes' / 'Proposed follow-up work' no longer list the item.
<!-- AC:END -->
