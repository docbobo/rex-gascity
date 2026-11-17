---
id: GCREX-6
title: 'Phase 5: pilot on a scratch city, then promote'
status: To Do
assignee: []
created_date: '2026-11-17T16:25:00Z'
updated_date: '2026-11-17T19:38:00Z'
labels: []
dependencies:
  - GCREX-5
references:
  - docs/plans/native-rex-provider.md
  - docs/REX-CONTRACT.md
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
- [ ] #6 The promotion record states the CLI/server version skew and whether the pilot requires a restarted server at the same API revision
- [ ] #7 The rig-tab split policy for a growing number of agents in one tab is decided and recorded
<!-- AC:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Verification notes (from the GCREX-8 plan review) for this phase to consume:

- Reuse internal/provider/e2e_test.go: it already holds opt-in live tests gated by REX_E2E (TestE2EAgentLifecycle, TestE2EInputPath). Extend them rather than writing new ones.
- Scenarios the review flagged as untested today: shell-vs-agent liveness (IsRunning would call a surviving shell running), concurrent topology creation from parallel Starts, empty-tab cleanup after the last pane closes, server restart recovery and whether block ids survive it, and version skew (CLI 0.1.0-preview.20/rev16 against server 0.1.0-preview.19/rev14 - decide whether promotion needs a restarted server).
- Rig-tab growth: repeated session.new_split into one tab has layout limits; this phase should fix the split policy for a rig with many agents.

Idle-detection decision to validate in the pilot. v1 GetLastActivity prefers program_status and otherwise fingerprints block format; it does not parse a ready prompt, and the provider does not emit OSC 7501. If the pilot shows format-poll is too coarse for the sleep tier, the follow-up is either to emit OSC 7501 from the provider or to scan block format for the ready prompt (GC_READY_PROMPT_PREFIX); the wiring lands on GCREX-5.
<!-- SECTION:NOTES:END -->
