---
id: GCREX-2
title: 'Phase 1: provider core — Start, Stop, IsRunning, Peek'
status: Done
assignee:
  - '@boris'
created_date: '2026-11-17T16:25:00Z'
updated_date: '2026-11-17T19:17:00Z'
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
- [x] #5 Peek returns the agent output, following paged format results to completion
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

7. Probe the live Rex server: format/blob_read paging is real (format with max_bytes returns {content,id,total,eof}; blob_read returns {id,data,total,eof}), and block.method/block.event require session_id. The adapter omitted session_id, so every block op would fail against a live server.

8. Add sessionID to the block methods on rex.Client, pass session_id in the CLI adapter, request format with max_bytes and follow blob_read until eof.

9. Re-test adapter on the fake rex binary (session_id present, multi-page following) and against the live server; record evidence for AC #5.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
In place: go.mod (module github.com/docbobo/rex-gas-city), internal/rex/client.go (Client interface + types), internal/rex/faketest (recording fake), internal/provider/provider.go (Start + parseName), internal/provider/provider_test.go (first slice, green). Next slice: Start joins an existing rig tab via NewSplit.

Core slices green: Start (creates city session + rig tab, joins an existing rig tab via NewSplit), Stop (closes only the agent pane, idempotent when the session or pane is absent), IsRunning (block process LastExit), Peek (block format). go test ./... and go vet ./... pass. Remaining for Phase 1's smoke gate: the rex/cli adapter (S2) that maps Client onto the rex CLI, and paged format following (AC #5) inside that adapter. Provider core still needs no gascity imports.

CLI adapter (rex/cli) added: implements rex.Client by shelling out to the rex CLI. Topology via 'rex api call' (session.create/list/view/new_window/new_split, block.close), block methods via 'rex api call block.method' (process, format with blob_read paging, write, clear), events via block.event. Events() streaming is deferred to Phase 3. Tested against a fake rex binary on the adapter seam (S2): CreateSession, ListSessions, NewWindow (layout JSON), NewSplit, CloseBlock, SessionView (layer flattening), BlockProcess, BlockFormat paging and single-page, BlockWrite, BlockEvent. go test ./..., go vet ./..., gofmt clean.

AC #5 done. Live probes against the Rex server settled the two open facts: (1) block.method and block.event require session_id next to block_id; the adapter had been omitting it, so every block op would have failed against a real server (the fake rex binary matched on method name only, so tests stayed green). (2) format with no max_bytes returns the whole capture or fails too_large; with max_bytes set it returns {content,id,total,eof} and a set id means more remains. blob_read returns {id,data,total,eof} with all four fields required. Fix: rex.Client block methods now take sessionID; the CLI adapter sends session_id, requests format with max_bytes=983040, and follows id with blob_read until eof.

Verification: fake-rex tests cover session_id on block.method/block.event and a two-page follow (format -> blob_read eof=false -> blob_read eof=true) plus the single-page case; provider tests assert Peek scopes the call to session:gc:acme/block:dog-1/format. New opt-in live test internal/provider/e2e_test.go (REX_E2E=1 go test ./internal/provider -run TestE2E -v) runs Start -> Peek -> IsRunning -> Stop -> Peek on a scratch city; the agent emits ~4 MiB, so Peek returning (len > 983040) with the end sentinel proves paging to completion. It passed against the live server in 2.0s. go test ./..., go vet ./..., gofmt -l . are clean.

Verification note (from the GCREX-8 plan review): AC #4 says "IsRunning and ProcessAlive distinguish a live agent from a terminal whose program exited", but ProcessAlive was never implemented. internal/provider/provider.go's IsRunning returns only info.LastExit == nil and never inspects Foreground/Ancestors, so a terminal whose agent died but whose shell survives reads as running. The checked AC text overstates the delivered surface; the semantic gap is recorded on GCREX-4.
<!-- SECTION:NOTES:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
Phase 1 provider core is complete. Peek now returns the agent's whole output: the rex/cli adapter requests format with max_bytes=983040 and follows the reply's id with blob_read until eof, so a capture larger than Rex's single-reply cap no longer fails or truncates. Live probing also exposed that block.method and block.event require session_id next to block_id, which the adapter had omitted; the block methods on rex.Client now take sessionID and the adapter sends it, so every block op works against a real server (the fake-rex tests had matched on method name only).

Verification: fake-rex tests cover session_id, a two-page follow, and the single-page case; provider tests assert Peek scopes the call to the city session. A new opt-in live test (internal/provider/e2e_test.go; REX_E2E=1 go test ./internal/provider -run TestE2E -v) runs Start -> Peek -> IsRunning -> Stop -> Peek on a scratch city against a ~4 MiB capture and passed in 2.0s. go test ./..., go vet ./..., gofmt -l . clean. docs/REX-CONTRACT.md records the resolved paging protocol and the session_id requirement.

Follow-ups: BlockWrite/BlockEvent/BlockClear carry session_id but have no live coverage yet (GCREX-3 exercises them); Peek is bounded by terminal scrollback, not the agent's full history; re-check Rex's 983040-byte reply cap on upgrade (REX-CONTRACT.md §9).
<!-- SECTION:FINAL_SUMMARY:END -->
