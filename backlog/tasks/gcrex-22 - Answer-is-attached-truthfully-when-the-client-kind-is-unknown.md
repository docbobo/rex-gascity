---
id: GCREX-22
title: Answer is-attached truthfully when the client kind is unknown
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
ordinal: 47000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
IsAttached returns ErrAttachmentUnknown (exit 1) when the Rex client list contains a kind it cannot classify. gc 1.5.1 reads any is-attached error as false, which is fail-open: config-drift restart, progress-stall recycle and the drain-ack escalation kill then proceed against a pane a human may be watching (parity doc ### is-attached row 3, mistake 4). The safe direction is to answer true when we cannot tell.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 An unclassifiable Rex client kind answers true with exit 0 instead of failing.
- [ ] #2 The Rex-unreachable case still fails with exit 1, and the doc states the resulting fail-open behavior explicitly.
- [ ] #3 Unit tests cover app client active (true), cli/peer only (false), unknown kind (true), and Rex transport error (error).
- [ ] #4 go test ./... and golangci-lint run ./... pass.
- [ ] #5 docs/provider-herdr-parity.md is updated in the same change: the affected section row(s) are re-verified after the fix (verdict changed, or a note added saying the deviation is now resolved), the 'Summary of deviations' row(s) and the 'Coverage' table are recomputed, and 'Confirmed mistakes' / 'Proposed follow-up work' no longer list the item.
<!-- AC:END -->
