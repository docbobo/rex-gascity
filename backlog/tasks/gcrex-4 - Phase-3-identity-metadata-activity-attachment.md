---
id: GCREX-4
title: 'Phase 3: identity, metadata, activity, attachment'
status: To Do
assignee: []
created_date: '2026-11-17T16:25:00Z'
updated_date: '2026-11-17T19:17:00Z'
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

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Verification notes (from the GCREX-8 plan review). Gaps the verification found between the code that exists today and the plan that describes this phase:

- Binding persistence and authority. internal/provider/binding.go has only the in-memory MemoryStore, and provider.New hardcodes it. The plan's file sidecar at <stateDir>/bindings/<sha256(name)>.json (with launch_mode, created_at, ready_prompt) does not exist. This phase must choose the state directory and the persistence shape, and state an explicit authority/recovery policy: today the store is a cache and session.view is the source of truth (Binder validates both ids and labels, then falls back to a label scan). Keep the SessionName/Binding module split from GCREX-7; the BindingStore seam is the extension point.
- Attachment. No ListClients method exists on rex.Client. session.list_clients is the only attachment source (session.view has no clients field). SessionView.Clients in internal/rex/client.go is an unused field the CLI never populates - remove it or mark it dead.
- Activity and events. rex.Client.Events exists but internal/rex/cli returns "Events is not implemented" and the fake mirrors it; no provider code consumes events. Decide the OSC 7501 policy here: gc 1.5.1 ships no emitter, so the provider must either emit OSC 7501 itself or scan format for the ready prompt.
- Liveness. IsRunning returns info.LastExit == nil only and never inspects Foreground/Ancestors, so a terminal whose agent died but whose shell survives reads as running. Strengthen the semantics (ProcessAlive does not exist - see the note on GCREX-2).
- Input gating. Busy gating (ImmediateNudge, "type nothing while a turn runs") and agent/program-state submit confirmation were deferred here by GCREX-3; today's confirmation is only that the Enter keystroke reached the pane.
- Error classification and teardown. Stop closes the block and forgets the binding; it does not classify a close-time disappearance as success, and it does not close a tab left empty. REX-CONTRACT.md section 12 fixes the rule: teardown closes city-owned blocks with block.close, never session.destroy, never server.stop.
<!-- SECTION:NOTES:END -->
