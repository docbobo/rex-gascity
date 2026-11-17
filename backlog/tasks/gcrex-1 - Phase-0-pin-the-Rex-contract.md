---
id: GCREX-1
title: 'Phase 0: pin the Rex contract'
status: In Progress
assignee: []
created_date: '2026-11-17T16:25:00Z'
labels: []
dependencies: []
references:
  - docs/REX-CONTRACT.md
  - docs/plans/native-rex-provider.md
ordinal: 1000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
The provider talks to a preview-version product whose CLI/API surface moves between releases. Before writing transport code we need one pinned record of the exact ops, schemas, key syntax, and error codes we rely on, so later breakage is a diff against a known baseline instead of a mystery. The contract doc is written; it needs review and a re-verification recipe for each Rex upgrade.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 REX-CONTRACT.md records the pinned Rex version and API revision
- [ ] #2 The doc covers session/window/block creation, block methods, events, key syntax, and error/exit codes
- [ ] #3 The doc includes a command block to re-verify every fact after a Rex upgrade
- [ ] #4 The RPP v0 op and handshake shape is recorded for the out-of-tree delivery form
<!-- AC:END -->
