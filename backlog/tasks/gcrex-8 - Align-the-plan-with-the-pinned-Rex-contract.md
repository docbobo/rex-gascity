---
id: GCREX-8
title: Align the plan with the pinned Rex contract
status: Done
assignee:
  - '@boris'
created_date: '2026-11-17T18:44:00Z'
updated_date: '2026-11-17T19:18:00Z'
labels: []
dependencies: []
references:
  - docs/REX-CONTRACT.md
priority: high
type: docs
ordinal: 8000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
docs/plans/native-rex-provider.md was written before the Rex contract was pinned, from gascity main, so its RPP v0 claims do not match gc 1.5.1. REX-CONTRACT.md §9 now records the real surface, but every phase task (GCREX-3 … GCREX-6) references this plan, and the wrong vocabulary was already copied into GCREX-5's handshake acceptance criterion. Update the plan so it agrees with the pinned baseline and stops describing D1–D6 as open.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 The plan's RPP capability vocabulary matches gc 1.5.1: only report-attachment, report-activity, proc.exec, proc.provision, proc.stream and tty.attach are presented as real, and fs.host, carrier.ops, observe.v1, input-outcome, idle-boundary and server-lifecycle are removed or marked as not in the pinned baseline
- [x] #2 The handshake example is {"version":0,"capabilities":[...]} with no locality field, matching REX-CONTRACT.md §9
- [x] #3 The plan records D0–D6 as confirmed and no longer says D1–D6 are open or that there is no code
- [x] #4 fs.host wording is gone from the scope, launch and copy rows, and the Phase 0 gate text matches what was delivered (the contract doc plus its re-verification probe)
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. Record the pinned baseline: REX-CONTRACT.md §9 fixes gc 1.5.1's six capability strings and the handshake shape; §11 confirms D0–D6; §12 lists the open items the plan must stop presenting as open.
2. Rewrite the plan header/status and §0 so D0–D6 are confirmed and the 'No code yet / D1–D6 open' claim is gone.
3. Align capability vocabulary across the plan: keep only report-attachment, report-activity, proc.exec, proc.provision, proc.stream, tty.attach; remove fs.host, carrier.ops, observe.v1, input-outcome, idle-boundary, server-lifecycle or mark them explicitly not in the pinned baseline.
4. Replace the handshake example with {"version":0,"capabilities":[...]} and drop the locality field.
5. Drop fs.host wording from the D4 scope row, the Start/launch row and the CopyTo row; make the Phase 0 gate text match what was delivered (the contract doc plus its §10 re-verification probe).
6. Self-check the file with rg for stale vocabulary and review the diff.
7. Independently verify the updated plan against the code (internal/provider, internal/rex, REX-CONTRACT) and report mismatches.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Aligned docs/plans/native-rex-provider.md with the pinned Rex/gc contract and with the code that exists. Two writer passes, both verified against the artifact (not the writer's self-report).

Pass 1 - pinned-baseline alignment (AC 1-4):
Header/§0: status is IN PROGRESS with D0-D6 confirmed (REX-CONTRACT §11); the decision tables keep the alternatives as history only.
Capabilities: §3 handshake is {"version":0,"capabilities":["report-attachment","report-activity"]} with no locality field; §5 lists exactly the six gc 1.5.1 strings (report-attachment, report-activity, proc.exec, proc.provision, proc.stream, tty.attach) and states earlier-draft names are not in the pinned baseline and must not be declared.
fs.host: removed everywhere, including the D4 scope row, the Start/launch row and the CopyTo row.
Phase gates: Phase 0 records the delivered contract doc plus its §10 re-verification probe; Phases 1-2 marked implemented; Phase 4 points at GCREX-5.
Also corrected: attachment source is session.list_clients (session.view has no clients field), peek pages blob_read at max_bytes 983040, gc drives input over exec (proc.exec) with the dedicated ops as fallback, and the §3 layout/interface sketch matches internal/rex and internal/provider (no nested runtime-rex/ module; cmd/gc-runtime-rex still planned).

Verification of pass 1: an independent read-only reviewer (different model family, openrouter/~openai/gpt-astra-latest) read the plan, contract and all Go code and reported all four ACs PASS with line-level evidence. Direct greps: no fs.host/carrier.ops/observe.v1/input-outcome/idle-boundary/server-lifecycle/locality in the plan; handshake at line 232; D0-D6 confirmed at lines 3, 28, 69; Phase 0 gate names the contract doc plus its §10 probe; no fs.host in the D4/Start/CopyTo rows. No Go code changed and no tests run (docs-only task).

Pass 2 - plan-vs-code corrections the review raised, all fixed (items 1-8):
1. §4 now names the real API: type SessionName (owns the <rig>--<agent> grammar and __town fallback; Rig/Agent/IsTown/TabLabel/PaneLabel) plus package-level rexSessionLabel(city). The GCREX-7 module split is kept as the deliberate design.
2. §10 Phase 2 states what GCREX-3 delivered (text written + submit keystroke sent; confirmation is transport-level) and marks busy gating and agent/program-state confirmation as deferred to Phase 3; the §5 Nudge row keeps the target spec with Phase-3 markers.
3. §3 layout marks the REX_E2E-gated live tests done and states rex.LaunchSpec already lives in internal/rex; no separate provider-side launch-policy module is planned.
4. §5 lead-in and table header now say provider behavior, and name the mapping onto the REX-CONTRACT §9 wire ops (which have no CopyTo/RunLive rows and add protocol/exec/provision/watch-startup).
5. D3 and §11 authorize block-only teardown (block.close), leave the shared city session, and forbid session.destroy and server.stop; the §2 primitive row is annotated that the provider never calls session.destroy.
6. §5/§6 capability wording is now "Planned: declared by the gc-runtime-rex handshake (GCREX-5)".
7. §11 program-status bullet drops the literal `unknown` state, treats an empty/missing status as indeterminate, and records that gc 1.5.1 ships no OSC 7501 emitter so v1 must emit it or scan format.
8. §4 Binding persistence is explicitly deferred behind the GCREX-7 BindingStore seam (in-memory MemoryStore today, file sidecar planned); the §11 ListRunning phrasing matches.

Residual code gaps were recorded as notes on the owning tasks, not as plan defects: GCREX-4 (binding persistence/authority, ListClients, events + OSC 7501 policy, foreground liveness, busy gating, teardown classification), GCREX-5 (wire-op mapping, per-op payload formats, exec scope, capabilities, cross-process Start coordination + focus=false/login options, unimplemented provider methods), GCREX-6 (reuse the live tests; shell-vs-agent liveness, concurrent Starts, empty-tab cleanup, restart/id survival, version skew, rig-tab growth), GCREX-2 (AC #4 names ProcessAlive, which was never implemented).
<!-- SECTION:NOTES:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
Aligned docs/plans/native-rex-provider.md with the pinned Rex/gc contract and with the code that exists (three verified passes: one writer rewrite, one writer correction pass, and small parent consistency edits).

Why: the plan predated docs/REX-CONTRACT.md and carried RPP vocabulary from gascity main that gc 1.5.1 ignores, plus a handshake example with a locality field that RPP v0 does not have. Every phase task references this plan, so the errors would have propagated into GCREX-4/5/6.

What changed in the plan: status and §0 record D0-D6 as confirmed; the capability vocabulary is only gc 1.5.1's six strings, with earlier-draft names stated as not in the pinned baseline and not to be declared; the handshake example is {"version":0,"capabilities":[...]} with no locality; fs.host is gone from the scope, launch and CopyTo rows; the Phase 0 gate records the delivered contract doc plus its §10 re-verification probe. Then, from the verification findings: §4 names the real SessionName/rexSessionLabel API, §10 states what Phase 2 actually delivered and defers busy gating to Phase 3, §3 marks the REX_E2E live tests and rex.LaunchSpec as present, §5's table is labelled as provider behaviors and points at the REX-CONTRACT §9 wire ops, D3/§11 authorize block-only teardown and forbid session.destroy, the capability rows say the handshake is planned with GCREX-5, §11 drops the non-existent program_status `unknown` state, and §4 marks the file sidecar as deferred behind the GCREX-7 BindingStore seam.

Verification: an independent read-only review by a different model family (openrouter/~openai/gpt-astra-latest) confirmed all four acceptance criteria PASS with line-level evidence, and direct greps confirm the stale vocabulary and the corrected phrasings. Docs-only change: no Go code touched, no tests run.

Residual work belongs to the phase tasks and is recorded as notes there: GCREX-4 (binding persistence and authority, ListClients attachment, events + OSC 7501 policy, foreground liveness, busy gating, teardown classification), GCREX-5 (wire-op mapping, per-op payload formats, exec scope, capabilities, cross-process Start coordination and launch options, unimplemented provider methods), GCREX-6 (live-test reuse and the untested scenarios), GCREX-2 (AC #4 names a ProcessAlive that was never implemented).
<!-- SECTION:FINAL_SUMMARY:END -->
