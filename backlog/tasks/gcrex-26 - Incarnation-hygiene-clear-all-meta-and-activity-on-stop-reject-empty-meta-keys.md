---
id: GCREX-26
title: >-
  Incarnation hygiene: clear all meta and activity on stop, reject empty meta
  keys
status: To Do
assignee: []
created_date: '2026-11-18T06:14:00Z'
labels:
  - provider
  - state
dependencies: []
references:
  - docs/provider-herdr-parity.md
  - internal/provider
  - cmd/gc-runtime-rex
priority: medium
type: bug
ordinal: 51000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Stop clears only the identity keys, so suspended, GC_DRAIN_ACK, GC_READY_PROMPT_PREFIX and the activity entry survive the incarnation and reappear for the next agent of the same name; herdr-core deletes the whole meta file with the incarnation (parity doc ### stop row 4 and ### set-meta row 2, mistake 9). Separately, set-meta/get-meta/remove-meta accept an empty key where herdr-core rejects it, and Binding.ReadyPrompt is written and never read (mistake 11, ### watch-startup follow-up).
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 Stop (and the no-binding path) clears every meta key for the name plus its activity entry, covered by a unit test that sets suspended/GC_DRAIN_ACK before stopping and reads nothing after.
- [ ] #2 A fresh start of the same name observes no meta or activity from the previous incarnation.
- [ ] #3 The three meta ops reject an empty key with exit 1 and a stderr message, covered by unit tests.
- [ ] #4 A decision is recorded for Binding.ReadyPrompt: either it gates something or it is removed, with a note in the parity doc ### watch-startup section.
- [ ] #5 go test ./... and golangci-lint run ./... pass.
- [ ] #6 docs/provider-herdr-parity.md is updated in the same change: the affected section row(s) are re-verified after the fix (verdict changed, or a note added saying the deviation is now resolved), the 'Summary of deviations' row(s) and the 'Coverage' table are recomputed, and 'Confirmed mistakes' / 'Proposed follow-up work' no longer list the item.
<!-- AC:END -->
