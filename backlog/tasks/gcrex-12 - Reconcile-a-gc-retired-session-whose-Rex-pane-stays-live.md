---
id: GCREX-12
title: Reconcile a gc-retired session whose Rex pane stays live
status: Done
assignee: []
created_date: '2026-11-17T22:35:00Z'
updated_date: '2026-11-17T22:57:00Z'
labels: []
dependencies: []
references:
  - https://github.com/gastownhall/gascity-packs/tree/main/runtime-herdr-core
  - /Users/boris/my-city/.gc/events.jsonl
documentation:
  - docs/REX-CONTRACT.md
  - docs/rex-provider.md
priority: medium
type: bug
ordinal: 12000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Observed on ~/my-city (gc 1.5.1, [session] provider = "rex") right after GCREX-11: gc starts the control dispatcher as a pool wisp (for example core__control-dispatcher-mc-wisp-h43m), the dispatcher registers with the controller/supervisor, and the reconciler then drains and closes the session bead. It closes with close_reason "session drained: pool slot retired by reconciler" without ever issuing the provider stop op, so the Rex pane (in the my-project tab) and the `gc convoy control --serve --follow` process stay live while the dispatcher disappears from `gc session list` (which lists only active/suspended sessions).

Evidence, ~/my-city/.gc/events.jsonl around 23:27:40-23:28:05: bead.created, worker.operation start_resolved succeeded, bead.updated with sleep_reason runtime-missing, then awake, then bead.closed (state drained) and session.stopped "drain acknowledged by agent"; there is no worker.operation for stop. `rex block ls` still shows block core__control-dispatcher-mc-wisp-h43m alive and one `convoy control --serve` process; the provider correctly answers is-running true for the runtime name and both rig-qualified aliases. GCREX-11 fixed the tab mapping and the is-running/list-running lookups so only one dispatcher starts; this is the leftover after the retire.

The fix direction is open and must be decided by comparison, not assumed: either gc should stop the runtime when it retires a pool slot, the Rex provider must close the pane (and any descendant process) on drain/retire, or the control dispatcher is a supervisor-owned service that should be tracked and visible instead of left as a dangling session. The comparison target is the herdr-core runtime pack, which implements the same RPP lifecycle (stop, teardown, process-tree kill, drain/cancel handling, session records) and is the closest reference for what a correct provider does here.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 A doc records the comparison: how runtime-herdr-core handles stop/teardown, drain/cancel, and reconciliation of drained or retired sessions (process-tree kill, descendant handling, records), and what the Rex provider does differently that leaves the pane alive, citing the specific herdr-core files
- [x] #2 The retire path is decided and documented: the doc states whether gc is expected to call stop on retire, whether the provider must close the pane and descendants on drain or retire, and who owns the control-dispatcher process
- [x] #3 The chosen behavior removes the inconsistency: a drained or retired session does not leave a live Rex pane and process that gc neither tracks nor can stop, either by closing the pane (and is-running then returning false) or by tracking the process as a managed, visible service
- [x] #4 Tests using the fake rex.Client cover the chosen retire/drain path, including a foreground process that acknowledges or ignores the drain when the fix is provider-side
- [x] #5 A live check on ~/my-city after the fix shows `gc session list` and the Rex layout consistent, exactly one control dispatcher, no orphaned panes, and gc doctor clean apart from unrelated checks
- [x] #6 go test ./... and golangci-lint run ./... pass
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. Root cause (from gc 1.5.1 source): the control-dispatcher's configured process_names is ["gc"], but the launched binary is gc's wrapper .gc-wrapped (GC_BIN). The Rex provider's process-alive matches names literally, so it answers false while is-running answers true; gc's reconciler reads running&&!alive as a dead/zombie seat and, on drain-ack, finalizes (closes) the bead WITHOUT calling provider stop. Pane + dispatcher process stay live.
2. Reference: runtime-herdr-core expands the 'gc' hint with basename(GC_BIN) (herdrcore/procs.py expand_names + matches; herdrcore/reads.py agent_state/op_process_alive) so the wrapped gc is seen as the agent.
3. Fix: capture GC_BIN from the start config env, persist its basename in the Binding, and expand incoming process names with the gc alias in ProcessAlive (mirroring herdr-core). gc then observes alive=true, takes the stop-pending branch and calls provider stop, which closes the pane; is-running then false.
4. Tests (fake rex.Client): wrapped-gc foreground matches hint gc; exited pane stays dead; start->process-alive->stop->is-running is the covered retire path.
5. Doc docs/REX-RETIRE.md records the comparison and decision (gc is expected to call stop; the provider must report the wrapped gc as alive so the stop path is reached; the control dispatcher is a supervisor-initiated pool session retired by the reconciler).
6. Verify: go test ./..., golangci-lint run ./..., live check on ~/my-city.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Implemented and verified. Commit bb31168 on main.
Root cause: gc's control-dispatcher hint is process_names=["gc"] but gc runs from the wrapper $GC_BIN=.gc-wrapped. internal/provider/liveness.go matched names literally, so process-alive answered false while is-running answered true. gc's reconciler (cmd/gc/session_reconciler.go) reads running && !alive as a zombie and, on drain-ack, finalizes the bead WITHOUT calling provider stop, orphaning the pane.
Fix: capture GC_BIN from the start config, persist its basename on the Binding (binding.go/filestore.go/provider.go), and expand process-alive hints like runtime-herdr-core's expand_names (liveness.go), incl. the kimi->kimi-code alias and a provider-env GC_BIN fallback for legacy records.
Evidence: docs/REX-RETIRE.md (herdr-core comparison + decision). Tests: internal/provider/retire_test.go (fake rex.Client). Verification: go test ./... pass; golangci-lint run ./... 0 issues; staged snapshot built and tested in a temp checkout.
Live check on ~/my-city: old binary process-alive core__control-dispatcher-mc-wisp-1cvx -> false; fixed -> true (is-running true). Closed the stale orphan pane (is-running now false). New dispatcher mc-wisp-791k started and stayed tracked (gc session list shows it; rex block ls shows exactly one dispatcher pane, no orphan). gc doctor 105 passed / 7 warnings / 1 failed, the failure unrelated (beads-role global git config).
<!-- SECTION:NOTES:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
A drained/retired gc control-dispatcher no longer leaves a live Rex pane behind. Root cause: gc's configured process_names=["gc"] does not match the wrapped gc binary ($GC_BIN=.gc-wrapped), so the Rex provider's process-alive answered false while is-running answered true; gc's reconciler read running && !alive as a zombie and closed the session bead without issuing provider stop. The provider now captures GC_BIN, persists its basename on the binding, and expands process-alive hints with it (and the kimi->kimi-code alias), mirroring runtime-herdr-core's expand_names in herdrcore/procs.py. gc then observes the seat alive, enters its stop-pending branch, and calls stop, which closes the pane; is-running then returns false. docs/REX-RETIRE.md records the herdr-core comparison and the decision (gc is expected to call stop; the provider must report the wrapped gc as the agent; the control dispatcher is a supervisor-initiated pool session owned by gc's reconciler, not a separate service). Verified with fake-rex.Client tests, go test ./..., golangci-lint run ./..., and a ~/my-city live check (process-alive false->true; stale orphan closed; new dispatcher mc-wisp-791k tracked with exactly one Rex dispatcher pane and no orphan; gc doctor clean apart from the unrelated beads-role check). Risk/limit: a descendant that deliberately setsid's out of the pane is outside a pane-close stop and is documented as a known limit. Commit bb31168 on main.
<!-- SECTION:FINAL_SUMMARY:END -->
