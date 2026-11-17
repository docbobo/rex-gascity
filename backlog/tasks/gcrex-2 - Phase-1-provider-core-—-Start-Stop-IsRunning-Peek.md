---
id: GCREX-2
title: 'Phase 1: provider core — Start, Stop, IsRunning, Peek'
status: In Progress
assignee: []
created_date: '2026-11-17T16:25:00Z'
labels: []
dependencies: []
references:
  - docs/plans/native-rex-provider.md
  - docs/REX-CONTRACT.md
ordinal: 2000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
First usable provider: map a gc session (city/rig/agent) onto a Rex city session, rig tab, and agent pane; start an agent, read its output, stop it, and answer whether it is running. Layout is Option B from the plan: one Rex session per city, one tab per rig, one pane per agent. The provider core depends only on the rex.Client interface so the CLI transport can be replaced by a wire client later without touching provider logic. Started: module scaffold, Client interface, fake client, and the first Start slice are in place.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 Start creates the city session, rig tab, and agent pane, and joins an existing rig tab instead of creating a second one
- [ ] #2 Stop closes the agent pane and leaves the shared city session and other rigs intact
- [ ] #3 Stop is idempotent for an unknown session
- [ ] #4 IsRunning and ProcessAlive distinguish a live agent from a terminal whose program exited
- [ ] #5 Peek returns the agent output, following paged format results to completion
- [ ] #6 go test ./... passes with the provider core tested against a fake rex.Client
<!-- AC:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
In place: go.mod (module github.com/docbobo/rex-gas-city), internal/rex/client.go (Client interface + types), internal/rex/faketest (recording fake), internal/provider/provider.go (Start + parseName), internal/provider/provider_test.go (first slice, green). Next slice: Start joins an existing rig tab via NewSplit.
<!-- SECTION:NOTES:END -->
