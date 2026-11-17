---
id: GCREX-11
title: >-
  Map gc sessions to Rex tabs from the start-config env, with a GC_* env
  inventory
status: In Progress
assignee:
  - '@boris'
created_date: '2026-11-17T22:04:00Z'
updated_date: '2026-11-17T22:17:00Z'
labels: []
dependencies: []
references:
  - docs/REX-CONTRACT.md
  - internal/provider/sessionname.go
  - internal/provider/provider.go
  - internal/provider/binding.go
  - cmd/gc-runtime-rex/main.go
priority: high
type: bug
ordinal: 11000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
With `[session] provider = "rex"` in ~/my-city, every gc agent lands in the `__town` Rex tab, multiple control-dispatcher panes pile up, and wisp panes open and disappear in a loop.

Root cause (reproduced by wrapping `gc-runtime-rex` and capturing its argv/stdin for a few minutes; capture kept at /tmp/rpp-capture.log on this machine):

The provider derives the Rex tab from the gc session name by splitting on `--` (`SessionName.Rig` in internal/provider/sessionname.go). gc 1.5.1 no longer puts the rig in the name it passes to `start` for pool/wisp sessions. Observed:
- `start pi-mc-wisp-ybq1` — a `my-project/pi` wisp, name has no `--`.
- `start core__control-dispatcher-mc-wisp-6znw` — a `my-project/core.control-dispatcher` wisp.
- The start config carries the rig out-of-band: `"GC_RIG":"my-project"`, `"GC_TEMPLATE":"my-project/pi"`, `"GC_AGENT":"mc-wisp-ybq1"`, `"GC_ALIAS":""`.

Because the name has no `--`, `SessionName.Rig()` returns `__town` and the pane opens in the town tab instead of the rig tab. All rig agents (pi wisps, control dispatchers, mayor) then share one tab.

Second half of the bug: gc probes pools with the rig-qualified name, but the provider stored the binding under the short instance name. Observed probes:
- `is-running my-project--core__control-dispatcher`
- `list-running my-project--pi-`
- `list-running my-project--gc__task-decomposer-` (and the other `my-project--gc.*` agents)
The provider returns false/empty for these, so gc concludes the pool has no live instance and starts another. That is the repeated start/stop churn and the "multiple control dispatchers" pile-up, and it leaves stale bindings plus orphaned `gc convoy control --serve --follow` processes (gc doctor reports orphaned sessions).

This went unnoticed because the GCREX-6 pilot only ran a town-level agent (`worker`, no rig), and the rig-tab mapping is otherwise covered only by unit tests that hand-craft `Indigo--dog-1`-style names.

Research requirement (part of this task): produce a holistic inventory of the GC_* variables gc ships in the RPP start-config env, so the provider can rely on the right ones. At least these were observed: GC_AGENT, GC_ALIAS, GC_BEADS, GC_BEADS_SCOPE_ROOT, GC_BIN, GC_CITY, GC_CITY_PATH, GC_CITY_RUNTIME_DIR, GC_CONTINUATION_EPOCH, GC_CONTROL_DISPATCHER_TRACE_DEFAULT, GC_CONTROLLER_TOKEN, GC_DIR, GC_DISABLE_USAGE_METRICS, GC_DOLT_HOST, GC_DOLT_PORT, GC_HOME, GC_INSTANCE_TOKEN, GC_PROVIDER, GC_RIG, GC_RIG_ROOT, GC_RUNTIME_EPOCH, GC_SESSION_ID, GC_SESSION_NAME, GC_SESSION_ORIGIN, GC_SPAWN_ORIGIN, GC_STARTUP_PROMPT_DELIVERED, GC_SUPERVISOR_PRESERVE_SESSIONS_ON_SIGNAL, GC_TEMPLATE, GC_TRIGGER_BEAD_ID, GC_TRIGGER_BEAD_STORE_REF, GC_TRIGGER_WORK_BEAD_ID, GC_TRIGGER_WORK_STORE_REF. The inventory should say what each means, when it is set (always vs conditional), and which are the durable provider contract versus incidental.

The fix and the research land together: the research establishes the contract, the fix uses it.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 A doc (docs/REX-CONTRACT.md and/or docs/rex-provider.md) lists every GC_* variable observed in the RPP start-config env with its meaning, when it is set, and whether it is durable contract or incidental
- [ ] #2 The doc records the observed RPP session-name forms for named/pool agents, pool instances, and wisp/session-bead sessions, and which ops (start, is-running, list-running, get-meta) use which form
- [ ] #3 Start derives the Rex tab from GC_RIG (falling back to the name split), so a rig agent pane opens in its rig tab rather than __town
- [ ] #4 is-running with a rig-qualified agent name (for example my-project--core__control-dispatcher) reports true while any instance of that agent is alive
- [ ] #5 list-running with a rig-qualified prefix (for example my-project--pi-) returns that agent live instances
- [ ] #6 Live check against ~/my-city with provider=rex shows a single control dispatcher, wisp panes in the my-project tab, and no repeated start/stop churn
- [ ] #7 Tests using the fake rex.Client cover the rig-from-env tab and the rig-qualified is-running/list-running lookups; go test ./... and golangci-lint run ./... pass
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. Research: extract the GC_* start-config env inventory from /tmp/rpp-capture.log and cross-check against gc v1.5.1 source (github.com/gastownhall/gascity, /tmp/gascity-src): internal/session/lifecycle.go (env), internal/agent/session_name.go (identity encoding / -> --, . -> __), cmd/gc/pool_session_name.go (PoolSessionName, poolIdentitySessionName), internal/agentutil/pool.go + cmd/gc/pool.go (list-running prefix probes), internal/runtime/envsecret.go (secret GC_* set).
2. Code: (a) Binding gains TabLabel/PaneLabel (actual Rex labels) and Aliases []SessionName; (b) start derives the tab from GC_RIG (falling back to name split) and records aliases Sanitize(GC_TEMPLATE), Sanitize(GC_ALIAS), qualified Sanitize(GC_AGENT), and Sanitize(GC_RIG/<town-start-name>); (c) Binder.Resolve/Binding/IsRunning/ListRunning/Stop become alias-aware; stillBound/scan use stored labels with name-derived fallback; FileStore serializes the new fields backward-compatibly.
3. Tests: fake rex.Client covers rig-from-env tab and rig-qualified is-running/list-running.
4. Docs: docs/rex-provider.md GC_* inventory + RPP name forms; docs/REX-CONTRACT.md identity/naming contract.
5. Verify: go test ./..., golangci-lint run ./..., plus live check on ~/my-city (single control dispatcher, wisp panes in the my-project tab, no churn).
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Integrated the provider fix and docs on main (uncommitted): Binding gains TabLabel/PaneLabel/Aliases; start derives the Rex tab from GC_RIG (falling back to the name split); rig-qualified aliases (Sanitize(GC_TEMPLATE/GC_ALIAS/qualified GC_AGENT/GC_RIG+town-name)) are recorded and resolved by is-running/list-running/stop/peek/binding; FileStore serializes the new fields backward-compatibly. Added provider_alias_test.go plus a stale-binding rescan test I fixed during integration. go test ./... and golangci-lint run ./... pass. Docs: REX-CONTRACT.md 13 (GC_* inventory + session-name forms), rex-provider.md identity/env sections.
<!-- SECTION:NOTES:END -->
