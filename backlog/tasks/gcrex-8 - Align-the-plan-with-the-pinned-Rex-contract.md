---
id: GCREX-8
title: Align the plan with the pinned Rex contract
status: To Do
assignee:
  - '@boris'
created_date: '2026-11-17T18:44:00Z'
labels: []
dependencies: []
references:
  - docs/REX-CONTRACT.md
priority: high
type: docs
ordinal: 8000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
docs/plans/native-rex-provider.md was written before the Rex contract was pinned, from gascity main, so its RPP v0 claims do not match gc 1.5.1. REX-CONTRACT.md §9 now records the real surface, but every phase task (GCREX-3 … GCREX-6) references this plan, and the wrong vocabulary was already copied into GCREX-5's handshake acceptance criterion. Update the plan so it agrees with the pinned baseline and stops describing D1–D6 as open.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 The plan's RPP capability vocabulary matches gc 1.5.1: only report-attachment, report-activity, proc.exec, proc.provision, proc.stream and tty.attach are presented as real, and fs.host, carrier.ops, observe.v1, input-outcome, idle-boundary and server-lifecycle are removed or marked as not in the pinned baseline
- [ ] #2 The handshake example is {"version":0,"capabilities":[...]} with no locality field, matching REX-CONTRACT.md §9
- [ ] #3 The plan records D0–D6 as confirmed and no longer says D1–D6 are open or that there is no code
- [ ] #4 fs.host wording is gone from the scope, launch and copy rows, and the Phase 0 gate text matches what was delivered (the contract doc plus its re-verification probe)
<!-- AC:END -->
