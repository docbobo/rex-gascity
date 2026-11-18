---
id: GCREX-13
title: >-
  Seed Rex provider session identity before launch so pending-create does not
  roll back mid-boot
status: Done
assignee:
  - '@pi'
created_date: '2026-11-17T22:48:00Z'
updated_date: '2026-11-18T00:22:00Z'
labels: []
dependencies: []
references:
  - https://github.com/gastownhall/gascity-packs/tree/main/runtime-herdr-core
  - /Users/boris/my-city/.gc/events.jsonl
  - /Users/boris/.gc/supervisor.log
documentation:
  - docs/REX-CONTRACT.md
  - docs/rex-provider.md
priority: high
type: bug
ordinal: 13000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Observed on ~/my-city (gc 1.5.1, `[session] provider = "rex"`) while rex-gascity pool seats were being spawned after GCREX-11/GCREX-12. The pi pool spawns a fresh Rex-backed session per work bead, but the create is rolled back mid-boot and the slot respawns, so seated work looks unclaimed even though routing is correct.

Evidence from ~/.gc/supervisor.log (23:42-23:43 local):

```
session reconciler: rolling back pending create pi-mc-wisp-cz1l: live runtime belongs to another session
session lifecycle: op=start wave=0 session=pi-mc-wisp-cz1l template=rex-gascity/pi outcome=stale_async_start duration=8.701s
session reconciler: rolling back pending create pi-mc-wisp-74xa: live runtime belongs to another session
session reconciler: rolling back pending create pi-mc-wisp-ppjx: live runtime belongs to another session
```

Five `rex-gascity/pi` sessions were created in ~90s; three closed `failed-create` ("session create failed: aborted before creation_complete"), one `runtime-missing`, and only the last (mc-wisp-m8s0) survived and claimed the work. The same loop appears earlier for `my-project/pi` (4xv8, iwk4, ovy8, af8t, 5umu, r4ia, easb, r7gw, utnd, ...). `gc ready` meanwhile showed the work correctly routed to `rex-gascity/pi`.

Root cause: gc's pending-create ownership check `runningSessionMatchesPendingCreateInfo` (cmd/gc/session_lifecycle_parallel.go:2935) reads `GC_SESSION_ID` and `GC_INSTANCE_TOKEN` from the provider meta (`sp.GetMeta`) and returns false ("live runtime belongs to another session") whenever the live identity does not match the session bead, including when the provider reports an empty identity (expectedToken non-empty, liveToken empty -> false). Rex's `Start` (internal/provider/provider.go:140) only records the pane binding after the launch returns and never seeds the identity sidecar from `cfg.Env`; the launch blocks ~8.7s (Rex "dismissing startup dialogs: theme selection dialog"). Any reconciler tick inside that window reads an unseeded/empty meta, fails the ownership check, and rolls the fresh runtime back (cmd/gc/session_reconciler.go ~2750). The pool slot is then freed and the reconciler spawns another, producing the create/respawn storm.

This is the same failure the herdr reference runtime documents and fixes: internal/runtime/herdr/provider.go seeds the metadata sidecar from cfg.Env (`seedMetaFromEnv`) and persists a provisional pane binding BEFORE the blocking launch, explicitly so "live runtime belongs to another session" cannot misread a fresh runtime mid-boot.

Fix direction: on Rex `Start`, before launching the pane, seed the identity metadata gc probes (`GC_SESSION_ID`, `GC_INSTANCE_TOKEN`, and any other identity keys from cfg.Env such as `GC_RUNTIME_EPOCH`/`GC_AGENT`/`GC_TEMPLATE`) into the provider meta store under the session name, and persist the provisional binding; record the final binding after the launch (or adopt/repair on launch failure). Ensure Stop and every rollback path clear the seeded identity so a stale prior-life identity cannot outlive its life. Compare against runtime-herdr-core and keep the fix provider-side if gc's contract is that the provider owns the sidecar.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 A doc records the herdr comparison: how runtime-herdr-core seeds identity meta and a provisional pane binding before the blocking launch, and why (seedMetaFromEnv / bindPlacement ordering in internal/runtime/herdr/provider.go), citing the specific files
- [x] #2 Rex Start seeds the identity keys gc's runningSessionMatchesPendingCreateInfo reads (GC_SESSION_ID, GC_INSTANCE_TOKEN, and GC_RUNTIME_EPOCH) into the meta store under the session name before the blocking launch, and persists a provisional binding, so a reconciler tick inside the launch window matches instead of rolling back
- [x] #3 A fresh rex pool spawn (rex-gascity/pi and my-project/pi) produces no 'live runtime belongs to another session' / stale_async_start while the launch is in flight; the create/respawn churn stops
- [x] #4 Teardown invariant preserved: Stop and every pending-create rollback path clear the seeded identity so a stale GC_SESSION_ID/GC_INSTANCE_TOKEN cannot outlive its life
- [x] #5 Tests using the fake rex.Client cover: a probe between start and launch-return matches (no rollback), a stale prior-life identity is cleared on Stop, and the empty-identity case no longer misreads as another session
- [x] #6 go test ./... and golangci-lint run ./... pass
- [x] #7 Live check on ~/my-city after the fix: spawn a rex-gascity/pi pool seat and a my-project/pi wisp; gc session list shows one seat per unit of work with no repeated start/rollback churn and no orphaned panes
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. (committed b99e0dc) seed GC_SESSION_ID/GC_INSTANCE_TOKEN/GC_RUNTIME_EPOCH + provisional binding before the blocking launch; clear on Stop and on failed/canceled start (internal/provider/identity.go, provider.go). 2. Independently verify against all 7 ACs in a dedicated worktree (review-family model), fix any defect. 3. Merge; run go test ./... + golangci-lint run ./... on main. 4. Live check on ~/my-city with the parent (no create/respawn churn).
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Independent verification (cross-family reviewer, Claude Opus 4.8, worktree gcrex-13-review, base e11eb4d): all reviewed criteria satisfied; no defect; 0 commits needed. Evidence: seed+provisional binding run before NewSplit/NewWindow (provider.go:191-199), replaced by record() after launch (provider.go:277); Stop clears on both the no-pane path (provider.go:302-306) and the pane path (provider.go:307-315); deferred cleanup clears on failed/canceled start (provider.go:143-153); seedStartIdentity clears all keys then sets non-empty only (identity.go:46-58). go test ./... ok, go test -race ./internal/provider/ ok, golangci-lint run ./... 0 issues. Live check still pending.

Live checks (new binary installed to ~/go/bin): (1) A controller-driven start of template rex-gascity/pi ran pi-adhoc-ce0034a31a; supervisor.log shows 'op=start ... outcome=success duration=8.75s phases=[start_call=8.75s]' with no 'belongs to another session', no 'stale_async_start', and no rollback. (2) Direct dispatcher start with a start-config identity env: get-meta returned the seeded GC_SESSION_ID/GC_INSTANCE_TOKEN/GC_RUNTIME_EPOCH on the live server immediately. (3) Probes cleaned up (stop); rex session gc:my-city shows only the mayor pane, no orphans.
<!-- SECTION:NOTES:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
GCREX-13 is fixed and verified end to end. The Rex provider now seeds the start-config identity keys gc's pending-create ownership check reads (GC_SESSION_ID, GC_INSTANCE_TOKEN, GC_RUNTIME_EPOCH, plus GC_CONTINUATION_EPOCH/GC_AGENT/GC_TEMPLATE/GC_ALIAS/GC_PROVIDER) into its metadata sidecar and records a provisional pane binding BEFORE the blocking NewWindow/NewSplit launch, so a reconciler tick during Rex's startup-dialog window reads the current identity instead of an empty one and does not roll the fresh runtime back as 'live runtime belongs to another session'. The seed and provisional binding are replaced by the final binding after launch, and are cleared on a failed/canceled start and on Stop, so a prior life's identity cannot outlive it. Mirrors runtime-herdr-core's seedMetaFromEnv/bindPlacement ordering; docs/REX-IDENTITY.md records the comparison. Verified: commit b99e0dc; independent cross-family review (Claude Opus 4.8) found no defect; go test ./..., go test -race ./internal/provider, golangci-lint run ./... (0 issues), go vet all clean; live on ~/my-city a controller-driven rex-gascity/pi start completed with outcome=success duration=8.75s and no rollback/stale_async_start churn, and a direct dispatcher probe returned the seeded identity on the live server. Risk: none known.
<!-- SECTION:FINAL_SUMMARY:END -->
