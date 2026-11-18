---
id: GCREX-27
title: Guard list-running output against unreadable names
status: To Do
assignee: []
created_date: '2026-11-18T06:14:00Z'
labels:
  - provider
dependencies: []
references:
  - docs/provider-herdr-parity.md
  - internal/provider
  - cmd/gc-runtime-rex
priority: low
type: task
ordinal: 52000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
ListRunning prints every binding name without validation. gc parses the output line by line, so a name that is empty, padded or contains a control character would corrupt that parse; herdr-core drops such names with a readable_name check (parity doc ### list-running row 3, mistake 10). Low risk because names come from gc, but the guard is one function.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 ListRunning skips any name that is empty, has leading or trailing whitespace, or contains a control character.
- [ ] #2 A unit test pins the filter for all three cases and confirms valid alias names still appear.
- [ ] #3 go test ./... and golangci-lint run ./... pass.
- [ ] #4 docs/provider-herdr-parity.md is updated in the same change: the affected section row(s) are re-verified after the fix (verdict changed, or a note added saying the deviation is now resolved), the 'Summary of deviations' row(s) and the 'Coverage' table are recomputed, and 'Confirmed mistakes' / 'Proposed follow-up work' no longer list the item.
<!-- AC:END -->
