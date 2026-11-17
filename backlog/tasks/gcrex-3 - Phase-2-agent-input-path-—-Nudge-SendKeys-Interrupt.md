---
id: GCREX-3
title: 'Phase 2: agent input path — Nudge, SendKeys, Interrupt'
status: To Do
assignee: []
created_date: '2026-11-17T16:25:00Z'
labels: []
dependencies:
  - GCREX-2
references:
  - docs/plans/native-rex-provider.md
  - docs/REX-CONTRACT.md
ordinal: 3000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Agents only make progress when the controller can deliver input. This phase adds the write path: typed text, submit confirmation, key sequences, and interrupt. Rex has no signal API, so interrupt is a ctrl+c keystroke against the pane, and the provider must handle a busy agent and report honestly when a keystroke could not be delivered.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 Nudge writes text and submits it, confirming the submit keystroke reached the pane
- [ ] #2 SendKeys delivers arbitrary key sequences to the agent pane
- [ ] #3 Interrupt sends ctrl+c and reports ErrSessionNotFound when no live agent is present
- [ ] #4 Input to a closed or exited pane fails with a clear error rather than silently succeeding
- [ ] #5 go test ./... covers the input path against a fake rex.Client
<!-- AC:END -->
