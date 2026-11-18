---
id: GCREX-19
title: >-
  Compare the rex provider with runtime-herdr-core method by method (parity
  analysis)
status: Done
assignee:
  - '@rex-agent'
created_date: '2026-11-18T05:51:00Z'
updated_date: '2026-11-18T06:14:00Z'
labels:
  - provider
  - parity
  - herdr
dependencies: []
references:
  - cmd/gc-runtime-rex/main.go
  - internal/provider
  - https://github.com/gastownhall/gascity-packs/tree/main/runtime-herdr-core
  - https://docs.gascity.com/reference/exec-session-provider.md
priority: high
type: task
ordinal: 19000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Our rex runtime provider (`gc-runtime-rex` + `internal/provider`) was written against the Runtime Provider Protocol v0 and a vendored reference (`runtime-herdr-core`), but the two implementations have drifted. We have been debugging behavior differences (e.g. `GC_AGENT`/`GC_ALIAS` identity, claim/liveness) one symptom at a time; this task replaces that with a systematic, method-by-method comparison.

## Goal

For every provider interface method (RPP v0 op), compare **our** implementation with **runtime-herdr-core**'s and produce a single analysis document that states, per method:

- what our provider does (entry points in `cmd/gc-runtime-rex/main.go` and `internal/provider`),
- what herdr-core does (the handler in `runtime-herdr-core/assets/gc_runtime_herdr_core/herdrcore`),
- the deviation, if any, and
- whether the deviation is **intentional** (for example because we map the hierarchy differently — gc City → Rex session, Rig → tab, Agent → pane, vs herdr's box model — or because we follow RPP v0 while herdr implements `observe.v1`/`carrier.ops`) or a **mistake**.

Methods we do not implement at all must appear too, marked as not implemented, with a note on what herdr-core does and whether the op is optional (exit 2) under the contract.

## Method

- Use the RPP v0 "Operations" table in `docs.gascity.com/reference/exec-session-provider.md` plus `watch-startup` as the authoritative interface. Treat herdr-core as the reference implementation, not as ground truth: a difference is not automatically a bug.
- Compare behavior, not cosmetics: input/stdin, output format, exit codes, capability gating, deadlines/timeouts, error mapping, state handling, cleanup/rollback, identity/env seeding, and concurrency.
- One subtask per interface method. Do not batch methods.

## Deliverable

A single analysis document (propose `docs/provider-herdr-parity.md`) with a section per method and a summary table of deviations and gaps. Cross-reference any task that already touches a method (e.g. GCREX-18, the `GC_ALIAS`/`GC_AGENT` claim identity).

## Out of scope

Fixing the deviations — this task only analyzes and classifies them. Follow-up implementation tasks are created from the findings.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 A single analysis document (docs/provider-herdr-parity.md) exists with one section per RPP v0 op plus watch-startup, and a summary deviation table.
- [x] #2 Each section states our behavior, herdr-core behavior, any deviation, and a verdict of intentional, mistake, or not-implemented, with rationale for intentional differences (e.g. hierarchy mapping or RPP v0 vs observe.v1/carrier.ops).
- [x] #3 Every op we do not implement (e.g. exec, provision, observe, inventory, kill-object, server-stop, attach, watch-startup, copy-to) is present and marked not-implemented with what herdr-core does instead and whether the op is optional under the contract.
- [x] #4 Findings are classified and cross-referenced to existing tasks (e.g. GCREX-18); proposed follow-up fixes are listed but not implemented here.
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. Split GCREX-19 into one subtask per provider interface method (25). 2. For each method, compare our dispatch/handler against runtime-herdr-core on the RPP axes and record intentional/mistake/not-implemented. 3. Assemble the per-method sections and the summary deviation table into docs/provider-herdr-parity.md. 4. Deduplicate and rank the confirmed mistakes, and list (not implement) proposed follow-ups.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
All 25 subtasks Done. docs/provider-herdr-parity.md: 1087 lines, 25 sections, 121 summary rows, 25 verdicts, 25 cross-reference blocks, 25 follow-up blocks. Unimplemented ops present and marked: exec, provision, observe, inventory, kill-object, server-stop, attach, watch-startup, copy-to. Consolidated sections added: 'Confirmed mistakes' (11 ranked, deduplicated), 'Gaps that are deliberate, not mistakes', 'Proposed follow-up work' (8 items, none implemented). Evidence per section cites cmd/gc-runtime-rex/main.go and internal/provider, against gastownhall/gascity-packs@0f07496 runtime-herdr-core.

Verification pass (commit 5c8656d): all gc-behavior claims re-checked against gc tag v1.5.1 source (internal/runtime/exec, protocol.go, docs/reference/exec-session-provider.md). Verdict changes: attach -> mistake (gc calls it, exit 2 is a failure, 'gc session attach' breaks); is-running -> intentional (gc reads exit 1 and false alike as dead); is-attached unknown-client-kind -> mistake (fail-open on kill paths); send-keys BTab/partial-failure -> intentional (gc emits only Enter/Up/Down/Escape/C-u); start SIGTERM gap -> downgraded (gc runs stop after a failed start), start empty-env -> mistake (protocol says unset; use env -u). observe/inventory/kill-object/server-stop/copy-to confirmed never called by gc 1.5.1. Added 'Operational risks' section (Rex outage reads as all-dead; rex exit 3 mapped to ErrNotFound by our client; seam probe fan-out).

Follow-up work filed from 'Proposed follow-up work' and the extension analysis: GCREX-20 attach exits 2 and fails gc session attach (mistake 1); GCREX-21 nudge write-to-Enter window and non-UTF-8 (mistake 2); GCREX-22 is-attached unknown client kind is fail-open (mistake 4); GCREX-23 peek/process-alive ErrNotFound inconsistency (mistakes 5, 6); GCREX-24 activity truthfulness for working/blocked plus input floor (mistake 3); GCREX-25 start robustness (mistakes 7, 8); GCREX-26 incarnation hygiene and empty meta keys (mistakes 9, 11); GCREX-27 list-running name guard (mistake 10); GCREX-16 remains the nudge busy-gating task. Extension and investigation tracks: GCREX-28 design and implement the herdr-only extensions; GCREX-29 find Rex-only extensions herdr cannot do; GCREX-30 investigate rex server run as an owned city-scoped server (revisits D3, feeds server-stop and watch-startup); GCREX-31 probe the Rex CLI facts still marked unverified. Each implementation task carries an acceptance criterion that keeps docs/provider-herdr-parity.md in sync.
<!-- SECTION:NOTES:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
Produced docs/provider-herdr-parity.md: a method-by-method parity analysis of gc-runtime-rex against runtime-herdr-core for all 17 RPP v0 ops plus watch-startup, attach, observe, inventory, kill-object, server-stop and copy-to. 25 sections, 121 classified deviations, a summary table, 11 ranked confirmed mistakes (liveness answers that error instead of answering false; non-atomic nudge submit; start rollback skipped on timeout; meta surviving stop; activity not treating working/blocked as active), and 8 proposed follow-up items. Analysis only: no provider behavior changed. Verification: every section has our behavior, herdr-core behavior, deviation rows and a verdict; section and summary-row counts match; unimplemented ops marked not-implemented with herdr behavior and contract optionality. Risks: several verdicts are unverified pending gc 1.5.1 source or a live Rex/hc probe, as noted per section.
<!-- SECTION:FINAL_SUMMARY:END -->
