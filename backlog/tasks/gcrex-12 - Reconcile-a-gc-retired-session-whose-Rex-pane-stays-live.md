---
id: GCREX-12
title: Reconcile a gc-retired session whose Rex pane stays live
status: To Do
assignee: []
created_date: '2026-11-17T22:35:00Z'
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
- [ ] #1 A doc records the comparison: how runtime-herdr-core handles stop/teardown, drain/cancel, and reconciliation of drained or retired sessions (process-tree kill, descendant handling, records), and what the Rex provider does differently that leaves the pane alive, citing the specific herdr-core files
- [ ] #2 The retire path is decided and documented: the doc states whether gc is expected to call stop on retire, whether the provider must close the pane and descendants on drain or retire, and who owns the control-dispatcher process
- [ ] #3 The chosen behavior removes the inconsistency: a drained or retired session does not leave a live Rex pane and process that gc neither tracks nor can stop, either by closing the pane (and is-running then returning false) or by tracking the process as a managed, visible service
- [ ] #4 Tests using the fake rex.Client cover the chosen retire/drain path, including a foreground process that acknowledges or ignores the drain when the fix is provider-side
- [ ] #5 A live check on ~/my-city after the fix shows `gc session list` and the Rex layout consistent, exactly one control dispatcher, no orphaned panes, and gc doctor clean apart from unrelated checks
- [ ] #6 go test ./... and golangci-lint run ./... pass
<!-- AC:END -->
