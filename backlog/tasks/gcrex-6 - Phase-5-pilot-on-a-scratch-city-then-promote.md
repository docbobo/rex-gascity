---
id: GCREX-6
title: 'Phase 5: pilot on a scratch city, then promote'
status: To Do
assignee: []
created_date: '2026-11-17T16:25:00Z'
labels: []
dependencies:
  - GCREX-5
references:
  - docs/plans/native-rex-provider.md
ordinal: 6000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Before making Rex the default, run it on a scratch city with the mayor still on tmux so a provider failure cannot take down the controller. The pilot exists to surface liveness, wedge detection, and orphan behaviour over several real work cycles, and to collect the errors that only appear under sustained use.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 A scratch city runs with GC_SESSION=rex and the mayor pinned to tmux
- [ ] #2 Liveness and wedge detection behave correctly across several work cycles
- [ ] #3 No orphan panes or sessions remain after teardown
- [ ] #4 Observed errors are reviewed and either fixed or accepted with a documented reason
- [ ] #5 Promotion to [session] provider = "rex" is recorded with the evidence behind it
<!-- AC:END -->
