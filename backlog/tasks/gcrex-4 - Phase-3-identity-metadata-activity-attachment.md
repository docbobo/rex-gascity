---
id: GCREX-4
title: 'Phase 3: identity, metadata, activity, attachment'
status: Done
assignee:
  - '@boris'
created_date: '2026-11-17T16:25:00Z'
updated_date: '2026-11-17T19:36:00Z'
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
- [x] #1 A sidecar maps gc session name to session/tab/block ids and survives pane re-creation
- [x] #2 Stale bindings are detected and re-resolved without leaking orphan panes
- [x] #3 GetLastActivity reports agent activity from program_status events, falling back to output polling
- [x] #4 IsAttached reports per agent using the active tab or focused block, and reports honestly when attachment cannot be determined
- [x] #5 Teardown removes only city-owned panes and never stops the shared Rex server
- [x] #6 The activity path accounts for gc 1.5.1 shipping no OSC 7501 emitter: either the provider emits OSC 7501 itself or the format-poll fallback is the documented v1 path, and GetLastActivity states which is in force
- [x] #7 Bindings re-resolve after a Rex server restart and leave no orphan panes
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
Phase 3 closes the gaps GCREX-8 recorded. Decisions (recorded here as the plan of record):

D-A State dir. Persistent state lives under <stateDir> = $GC_REX_STATE_DIR when set, else $XDG_STATE_HOME/rex-gas-city/<city>, else ~/.local/state/rex-gas-city/<city>. Files are per-name: bindings/<sha256(gcName)>.json, activity/<sha256(gcName)>.json, meta/<sha256(gcName)>.json. Writes are atomic (temp + rename). Provider default stores stay in-memory so the existing tests stay hermetic; WithStateDir(dir) opts into the file sidecar. GCREX-5 wires WithStateDir(StateDir(city)) in the dispatcher.

D-B Binding authority. The store stays a cache; session.view plus the label scan stays the source of truth (Binder.Resolve unchanged). The sidecar adds the plan's fields (launch_mode, created_at, ready_prompt) and process_names.

D-C OSC 7501. gc 1.5.1 ships no emitter, so the provider does NOT emit OSC 7501 in v1. GetLastActivity prefers program_status when Rex reports records and falls back to polling block format, returning the source in force (program_status | format_poll | none). The format-poll fallback is the documented v1 path; document it in the plan §6 and REX-CONTRACT §12. The fallback fingerprints output in the activity sidecar so it survives the one-process-per-op dispatcher.

D-D Attachment. IsAttached reads session.list_clients, counts only app-kind clients (the provider's own CLI connection is canned in the same list), then refines to the agent via session.view active_window_id / focused_block_id. No app clients => false. Transport failure or an unidentifiable client kind => an error (honest: cannot determine), never a false yes.

D-E Teardown/liveness. Start becomes idempotent: an existing live pane for the name is reused; an exited pane is closed and relaunched in the same rig tab, so re-Start cannot leak a duplicate pane. Stop closes the pane with block.close, treats rex.ErrNotFound as success, and closes the rig tab when it is left empty (session.close_window). Never session.destroy, never server.stop. IsRunning and livePane only treat the pane as live when the agent process is still present in the foreground/ancestor tree (matched by the recorded process_names).

Slices (independent files, one writer each):
1. transport: rex.Client gains ListClients, BlockProgramStatus, CloseWindow; rex.ErrNotFound mapped from exit 3; cli + faketest + tests; drop the dead SessionView.Clients.
2. sidecar: statedir.go, filestore.go (BindingStore file impl + JSON helpers), binding fields, Provider options/fields, memory ActivityStore/MetaStore + file impls, tests.
3. teardown/liveness/start: provider.go Start/Stop/IsRunning/livePane changes + teardown_test.go.
4. attach+meta: attach.go (IsAttached) + meta.go (SetMeta/GetMeta/RemoveMeta) + tests.
5. activity: activity.go (GetLastActivity + source) + activity_test.go.
6. docs: plan §6/§11 and REX-CONTRACT §12 record the OSC 7501 policy and the state dir.
Then: gofmt -l . clean, go vet ./... clean, go test -count=1 ./... green; independent read-only review of the ACs; final signed commit.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Verification notes (from the GCREX-8 plan review). Gaps the verification found between the code that exists today and the plan that describes this phase:

- Binding persistence and authority. internal/provider/binding.go has only the in-memory MemoryStore, and provider.New hardcodes it. The plan's file sidecar at <stateDir>/bindings/<sha256(name)>.json (with launch_mode, created_at, ready_prompt) does not exist. This phase must choose the state directory and the persistence shape, and state an explicit authority/recovery policy: today the store is a cache and session.view is the source of truth (Binder validates both ids and labels, then falls back to a label scan). Keep the SessionName/Binding module split from GCREX-7; the BindingStore seam is the extension point.
- Attachment. No ListClients method exists on rex.Client. session.list_clients is the only attachment source (session.view has no clients field). SessionView.Clients in internal/rex/client.go is an unused field the CLI never populates - remove it or mark it dead.
- Activity and events. rex.Client.Events exists but internal/rex/cli returns "Events is not implemented" and the fake mirrors it; no provider code consumes events. Decide the OSC 7501 policy here: gc 1.5.1 ships no emitter, so the provider must either emit OSC 7501 itself or scan format for the ready prompt.
- Liveness. IsRunning returns info.LastExit == nil only and never inspects Foreground/Ancestors, so a terminal whose agent died but whose shell survives reads as running. Strengthen the semantics (ProcessAlive does not exist - see the note on GCREX-2).
- Input gating. Busy gating (ImmediateNudge, "type nothing while a turn runs") and agent/program-state submit confirmation were deferred here by GCREX-3; today's confirmation is only that the Enter keystroke reached the pane.
- Error classification and teardown. Stop closes the block and forgets the binding; it does not classify a close-time disappearance as success, and it does not close a tab left empty. REX-CONTRACT.md section 12 fixes the rule: teardown closes city-owned blocks with block.close, never session.destroy, never server.stop.

Integration pass. The four slices land green (gofmt, vet, go test -race: 84 provider tests). One live E2E regression was found and is being fixed: TestE2EInputPath failed with 'agent program exited' on a live pane because the argv[0]-derived process-name liveness misclassified it. Live probe of a pane launched as command ["/bin/sh"]: Rex reports name=bash, path=/bin/bash, argv0=/bin/sh, so argv0/name heuristics cannot decide death. Decision: IsRunning and the input ops keep last_exit as the v1 liveness truth, and a strict ProcessAlive(names) op (empty => alive) backs the RPP process-alive op for GCREX-5. ProcessNames is removed from the binding sidecar. Also removed the dead CloseWindow transport op: a live probe confirmed block.close of a tab's last block already removes the tab (session.view then reports zero windows), so teardown is block-only as REX-CONTRACT section 12 requires.

Final verification (GCREX-4):
- gofmt -l . clean; go vet ./... clean; go test -race -count=1 ./... green (96 provider tests, plus rex/cli).
- Live Rex gate: REX_E2E=1 go test ./internal/provider -run TestE2E -v -> all three pass (TestE2EAgentLifecycle, TestE2EInputPath, TestE2EIdentityActivityAttachment; the last logs activity source=format_poll advanced=true, IsAttached=true appClient=true).
- Two independent read-only reviews (different model family) were run. Review 1 found two blockers; both were fixed and re-verified by review 2: (a) a restored city under a new session id now clears the cached binding on rex.ErrNotFound and re-resolves by label; (b) attachment is now true only when an app client exists AND the agent's rig tab is the session's active window containing the pane, and Start launches with focus=false so a background rig cannot steal focus.
- Review-driven hardening also landed: injective percent-encoded state-dir names (no city collision or '.'/'..' escape), stored bindings validated against the requested name and configured city, activity fingerprints tied to the pane's block id, and MetaStore write errors propagated through SetMeta/RemoveMeta.
- Deliberately deferred to GCREX-5 (recorded there): cross-process Start serialization, best-effort BindingStore/ActivityStore writes, and cross-process metadata read-modify-write.
<!-- SECTION:NOTES:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
Phase 3 (identity, metadata, activity, attachment) is implemented and verified.

What changed. internal/rex gains ListClients (session.list_clients, the only attachment source), BlockProgramStatus (OSC 7501 records), rex.ErrNotFound mapped from CLI exit 3, and LaunchSpec.Focus; the dead SessionView.Clients field and an unused CloseWindow op were removed. internal/provider gains a file sidecar behind the existing BindingStore seam: StateDir (GC_REX_STATE_DIR, else XDG_STATE_HOME/~/.local/state), an injective percent-encoded per-city directory, atomic per-name JSON for bindings, activity fingerprints and metadata, and functional options (WithStateDir/WithBindingStore/WithActivityStore/WithMetaStore) so the default New stays in-memory for hermetic tests. Start is now idempotent: a live pane is reused, an exited pane is closed and relaunched in the same rig tab so re-Start cannot leak a duplicate. Stop closes only the resolved block, treats rex.ErrNotFound as success, and never calls session.destroy or server.stop (a live probe confirmed Rex removes a tab when its last block closes, so teardown is block-only). GetLastActivity prefers program_status and falls back to polling block format with a persisted, pane-tied fingerprint, returning the source in force (program_status | format_poll | none); ActivityPolicy records the decision. IsAttached counts only app-kind clients and reports attached only when the agent's rig tab is the session's active window containing the pane; unknown client kinds return ErrAttachmentUnknown rather than a false yes. A strict ProcessAlive(names) op backs the future RPP process-alive op, while IsRunning and the input ops keep last_exit as the v1 liveness truth. Metadata is a per-session KV that round-trips values byte-for-byte.

Why. GCREX-8's verification notes listed the gaps this phase closes: no file sidecar, no ListClients, no activity source, no OSC 7501 policy, coarse attachment, dead SessionView.Clients. The phase title also covers metadata, so the sidecar carries the meta KV.

Verification. gofmt -l . clean; go vet ./... clean; go test -race -count=1 ./... green (96 provider tests plus rex/cli). Live Rex: REX_E2E=1 go test ./internal/provider -run TestE2E -v passes all three tests; TestE2EIdentityActivityAttachment exercises identity persistence across provider instances, stale-id re-resolution, activity (source=format_poll, advancing), the attachment invariant and teardown that leaves the shared session. Two independent read-only reviews by a different model family were run; review 1's two blockers (restored city under a new session id; attachment over-reporting) were fixed and cleared by review 2, whose remaining medium findings (city-dir collision, doc/test precision) were also fixed.

Risks and follow-ups. Liveness is deliberately last_exit-only: a live probe showed Rex runs a launched command as the default shell with argv0 set to the command, so process-name heuristics misclassify live panes. Attachment is session-coarse: Rex lists the server-wide app client on every session and the surface cannot say which tab a client views; a single-rig city therefore reads attached. AC #7 is evidenced by injected new session/window/block ids and a real not-found probe, not an actual Rex server restart (D3 forbids stopping the shared server). Deferred to GCREX-5: cross-process Start serialization, best-effort BindingStore/ActivityStore writes, and cross-process metadata read-modify-write.
<!-- SECTION:FINAL_SUMMARY:END -->
