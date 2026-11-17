---
id: GCREX-4
title: 'Phase 3: identity, metadata, activity, attachment'
status: To Do
assignee: []
created_date: '2026-11-17T16:25:00Z'
updated_date: '2026-11-17T18:44:00Z'
labels: []
dependencies:
  - GCREX-3
references:
  - docs/plans/native-rex-provider.md
  - docs/REX-CONTRACT.md
ordinal: 4000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Gas City needs to know which Rex objects belong to which gc session, when an agent last did anything, and whether a human is watching. Rex names are cosmetic and panes can be re-created, so stable ids live in a sidecar keyed by the gc session name. Activity comes from program_status and process events, with a format-poll fallback. Attachment is reported per agent, not per city session, because Option B puts every agent in one session.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 A sidecar maps gc session name to session/tab/block ids and survives pane re-creation
- [ ] #2 Stale bindings are detected and re-resolved without leaking orphan panes
- [ ] #3 GetLastActivity reports agent activity from program_status events, falling back to output polling
- [ ] #4 IsAttached reports per agent using the active tab or focused block, and reports honestly when attachment cannot be determined
- [ ] #5 Teardown removes only city-owned panes and never stops the shared Rex server
- [ ] #6 The activity path accounts for gc 1.5.1 shipping no OSC 7501 emitter: either the provider emits OSC 7501 itself or the format-poll fallback is the documented v1 path, and GetLastActivity states which is in force
- [ ] #7 Bindings re-resolve after a Rex server restart and leave no orphan panes
<!-- AC:END -->
