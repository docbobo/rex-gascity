---
id: GCREX-19.2
title: 'Compare ''start'': rex provider vs runtime-herdr-core'
status: Done
assignee:
  - '@rex-agent'
created_date: '2026-11-18T05:51:00Z'
updated_date: '2026-11-18T06:08:00Z'
labels: []
dependencies: []
references:
  - internal/provider
  - https://github.com/gastownhall/gascity-packs/tree/main/runtime-herdr-core
parent_task_id: GCREX-19
priority: medium
type: task
ordinal: 21000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Method-by-method parity comparison for the start provider interface method.

Our implementation: main.go opStart -> provider.StartNew (cmd/gc-runtime-rex/main.go, internal/provider).
Runtime-herdr-core: lifecycle.py op_start (runtime-herdr-core/assets/gc_runtime_herdr_core/herdrcore).

Compare behavior: stdin/input, stdout/stderr format, exit codes, capability gating, deadlines/timeouts, error mapping, state handling, cleanup/rollback, identity and env seeding, and concurrency. State whether each deviation is intentional (e.g. hierarchy mapping: gc City to Rex session, Rig to tab, Agent to pane vs the herdr box model; or RPP v0 vs observe.v1/carrier.ops) or a mistake, and whether we implement the method at all. Add the start section to docs/provider-herdr-parity.md and to the summary deviation table.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 docs/provider-herdr-parity.md has a 'start' section with our behavior, herdr-core behavior, deviations, and an intentional/mistake/not-implemented verdict.
- [x] #2 Any deviation or gap for 'start' is listed in the summary deviation table.
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. Read our dispatch/handler and the herdr-core handler. 2. Compare on the RPP axes. 3. Write the 'start' section and its summary rows into docs/provider-herdr-parity.md. 4. Classify each deviation intentional/mistake/not-implemented.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Verified: docs/provider-herdr-parity.md has exactly one 'start' section (`### start`) with our behavior, herdr-core behavior, a deviation table and a verdict; 18 summary row(s) under the same op in 'Summary of deviations'. Cross-checked against cmd/gc-runtime-rex/main.go and /tmp/gascity-packs/runtime-herdr-core.

gc 1.5.1 verification (commit 5c8656d): start: row 1 downgraded to intentional (with a gap) because gc runs 'stop <name>' after a failed start; row 9 upgraded to mistake because the protocol defines an empty env value as unset (gc's tmux adapter uses env -u).
<!-- SECTION:NOTES:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
Added the 'start' parity section to docs/provider-herdr-parity.md with our behavior, herdr-core behavior, per-deviation verdicts and 18 summary-table row(s). Analysis only; no behavior changed. Verification: section and summary rows present and counted; findings cross-referenced to existing tasks.
<!-- SECTION:FINAL_SUMMARY:END -->
