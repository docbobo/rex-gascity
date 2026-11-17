---
id: GCREX-2
title: 'Phase 1: provider core — Start, Stop, IsRunning, Peek'
status: In Progress
assignee:
  - '@boris'
created_date: '2026-11-17T16:25:00Z'
updated_date: '2026-11-17T17:02:00Z'
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
- [x] #1 Start creates the city session, rig tab, and agent pane, and joins an existing rig tab instead of creating a second one
- [x] #2 Stop closes the agent pane and leaves the shared city session and other rigs intact
- [x] #3 Stop is idempotent for an unknown session
- [x] #4 IsRunning and ProcessAlive distinguish a live agent from a terminal whose program exited
- [ ] #5 Peek returns the agent output, following paged format results to completion
- [x] #6 go test ./... passes with the provider core tested against a fake rex.Client
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. Slice: Start joins an existing rig tab. Red test: when the city session already has a tab labeled Indigo, a second agent Start creates no new window and goes in via NewSplit beside the existing pane.
2. Implement tab lookup in Start from the session view, then split into the found tab.
3. Slice: Stop closes the agent pane, leaves the city session and other rigs intact, and is idempotent for an unknown session.
4. Slice: IsRunning/ProcessAlive read block process state and distinguish a live agent from a terminal whose program exited.
5. Slice: Peek reads format output and follows paged results to completion.
6. Verify go test ./... and record evidence per acceptance criterion.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
In place: go.mod (module github.com/docbobo/rex-gas-city), internal/rex/client.go (Client interface + types), internal/rex/faketest (recording fake), internal/provider/provider.go (Start + parseName), internal/provider/provider_test.go (first slice, green). Next slice: Start joins an existing rig tab via NewSplit.

Core slices green: Start (creates city session + rig tab, joins an existing rig tab via NewSplit), Stop (closes only the agent pane, idempotent when the session or pane is absent), IsRunning (block process LastExit), Peek (block format). go test ./... and go vet ./... pass. Remaining for Phase 1's smoke gate: the rex/cli adapter (S2) that maps Client onto the rex CLI, and paged format following (AC #5) inside that adapter. Provider core still needs no gascity imports.

CLI adapter (rex/cli) added: implements rex.Client by shelling out to the rex CLI. Topology via 'rex api call' (session.create/list/view/new_window/new_split, block.close), block methods via 'rex api call block.method' (process, format with blob_read paging, write, clear), events via block.event. Events() streaming is deferred to Phase 3. Tested against a fake rex binary on the adapter seam (S2): CreateSession, ListSessions, NewWindow (layout JSON), NewSplit, CloseBlock, SessionView (layer flattening), BlockProcess, BlockFormat paging and single-page, BlockWrite, BlockEvent. go test ./..., go vet ./..., gofmt clean.
<!-- SECTION:NOTES:END -->
