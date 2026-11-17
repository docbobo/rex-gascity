---
id: GCREX-6
title: 'Phase 5: pilot on a scratch city, then promote'
status: Done
assignee:
  - '@pi'
created_date: '2026-11-17T16:25:00Z'
updated_date: '2026-11-17T20:37:00Z'
labels: []
dependencies:
  - GCREX-5
references:
  - docs/plans/native-rex-provider.md
  - docs/REX-CONTRACT.md
ordinal: 6000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Before making Rex the default, run it on a scratch city with the mayor still on tmux so a provider failure cannot take down the controller. The pilot exists to surface liveness, wedge detection, and orphan behaviour over several real work cycles, and to collect the errors that only appear under sustained use.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 A scratch city runs with GC_SESSION=rex and the mayor pinned to tmux
- [x] #2 Liveness and wedge detection behave correctly across several work cycles
- [x] #3 No orphan panes or sessions remain after teardown
- [x] #4 Observed errors are reviewed and either fixed or accepted with a documented reason
- [x] #5 Promotion to [session] provider = "rex" is recorded with the evidence behind it
- [x] #6 The promotion record states the CLI/server version skew and whether the pilot requires a restarted server at the same API revision
- [x] #7 The rig-tab split policy for a growing number of agents in one tab is decided and recorded
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. Fix the rig-tab split policy: parse session.view layer geometry into rex.Block rects and split the largest-area pane along its longer axis at 50% (balanced grid), replacing the always-split-first-block/horizontal rule; fall back to the first block when geometry is missing. Cover with fake-client unit tests and a live REX_E2E grid test.
2. Harden liveness: treat a vanished pane (rex.ErrNotFound from BlockProcess) as not running rather than an error; add live tests for agent exit (block closes -> IsRunning=false) and a live shell (IsRunning=true).
3. Extend the opt-in live suite (internal/provider/e2e_test.go): parallel Starts into one rig, empty-tab removal after the last pane closes, and version-skew assertions (CLI api range vs server api_revision, handshake v0). Server-restart recovery stays out of the live suite because the Rex server is shared (D3); its stale-id path is already covered by TestE2EIdentityActivityAttachment.
4. Run the pilot: REX_E2E=1 live suite, make check, make conformance, and a scratch gatecity run with GC_SESSION=rex; capture results.
5. Record the pilot in docs/rex-pilot-phase5.md: run config, version skew, observed errors and dispositions, promotion to [session] provider = "rex", rig-tab split policy.
6. Update docs/rex-provider.md, docs/plans/native-rex-provider.md (Phase 5 status) and docs/REX-CONTRACT.md open items.
7. Verify with go vet, go test ./..., golangci-lint run ./..., make check, make conformance; then finalize the task.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Verification notes (from the GCREX-8 plan review) for this phase to consume:

- Reuse internal/provider/e2e_test.go: it already holds opt-in live tests gated by REX_E2E (TestE2EAgentLifecycle, TestE2EInputPath). Extend them rather than writing new ones.
- Scenarios the review flagged as untested today: shell-vs-agent liveness (IsRunning would call a surviving shell running), concurrent topology creation from parallel Starts, empty-tab cleanup after the last pane closes, server restart recovery and whether block ids survive it, and version skew (CLI 0.1.0-preview.20/rev16 against server 0.1.0-preview.19/rev14 - decide whether promotion needs a restarted server).
- Rig-tab growth: repeated session.new_split into one tab has layout limits; this phase should fix the split policy for a rig with many agents.

Idle-detection decision to validate in the pilot. v1 GetLastActivity prefers program_status and otherwise fingerprints block format; it does not parse a ready prompt, and the provider does not emit OSC 7501. If the pilot shows format-poll is too coarse for the sleep tier, the follow-up is either to emit OSC 7501 from the provider or to scan block format for the ready prompt (GC_READY_PROMPT_PREFIX); the wiring lands on GCREX-5.

Pilot run (2026-11-17, gc 1.5.1, rex CLI 0.1.0-preview.20/rev16, server 0.1.0-preview.19/rev14).

Code landed: balanced rig-tab split (largest-area pane, longer axis, 50%; rects parsed from session.view layers[].layout); paneAlive treats a vanished pane as not running; live tests extended with TestE2EAgentExitLiveness, TestE2EParallelStarts, TestE2ERigSplitGrid and TestE2EVersionSkew.

Pilot evidence: scratch city /tmp/gcrex-pilot ran the worker on rex (session gc:default, tab __town) while a companion /tmp/gcrex-pilot-mayor ran its mayor on tmux. Three kill/restart cycles went active -> asleep/runtime-missing -> active within ~15s with no wedges. Teardown left no orphan panes or sessions. Live suite all PASS; make check 14/14; make conformance 6/8 with exec/provision as optional skips.

Open items recorded: worker namespace fell back to gc:default because the launchd supervisor carries no GC_REX_CITY (GCREX-5 limit, pilot-confirmed); gc stop removed the city Rex session although the provider never calls session.destroy; server-restart recovery not exercised live because the server is shared (D3).

Final verification (2026-11-17): live REX_E2E provider suite PASS (incl. -race for the new tests), dispatcher suite PASS, make check 14/14, make conformance 6/8 with only the optional exec/provision skips, go test ./... ok, go vet clean, gofmt clean. Pilot evidence and decisions recorded in docs/rex-pilot-phase5.md; split policy also mirrored in docs/rex-provider.md and the plan/contract. Machine state restored: no registered cities, supervisor stopped and service uninstalled, rex session list at baseline. Follow-ups: supervisor GC_REX_CITY pinning (open), gc stop session removal and server-restart recovery (accepted with reasons).
<!-- SECTION:NOTES:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
Phase 5 pilot complete; the Rex backend is promoted as the supported opt-in session provider.

What changed
- Rig-tab split policy fixed: `nextSplit` splits the largest-area pane along its longer axis at 50%, with rects parsed from `session.view` `layers[].layout` into `rex.Block.Rect`; falls back to the first block, horizontal, when geometry is missing. The old rule halved the first pane (14 splits -> widths 1/16384).
- Liveness hardened: `paneAlive` treats a vanished pane (`rex.ErrNotFound`) as not running, so `is-running` returns false instead of an error after an agent command exits and its block closes.
- Live suite extended: `TestE2EAgentExitLiveness`, `TestE2EParallelStarts`, `TestE2ERigSplitGrid`, and `TestE2EVersionSkew` (all opt-in on REX_E2E).
- Docs: new `docs/rex-pilot-phase5.md` (run config, version skew, work cycles, error dispositions, promotion decision, split-policy decision); updated `docs/rex-provider.md`, `docs/plans/native-rex-provider.md`, `docs/REX-CONTRACT.md`.

Why
- Phase 5 must surface liveness, wedge and orphan behaviour over real work cycles before promotion, and the review flagged the rig-tab layout limit and several untested scenarios.

Verification
- Live: REX_E2E provider suite all PASS (also -race for the parallel/split/exit tests), dispatcher suite PASS; TestE2EVersionSkew logged CLI 0.1.0-preview.20/rev16 supports 0..16 vs server 0.1.0-preview.19/rev14.
- Pilot: scratch worker city on rex and a mayor companion city on tmux; three kill/restart cycles went active -> asleep/runtime-missing -> active within ~15s with no wedge; teardown left no orphan panes or sessions; a foreground GC_SESSION=rex supervisor run created rex session gc:gcrex-envcheck for a city with no [session] config.
- make check 14/14; make conformance 6/8 (exec/provision optional skips); go test ./... ok; go vet and gofmt clean; golangci-lint reports only 2 pre-existing errcheck findings in internal/provider/filestore.go.
- Machine state restored: supervisor stopped and launchd service uninstalled, no registered cities, rex session list back to the user's baseline.

Risks / follow-ups (recorded in docs/rex-pilot-phase5.md)
- Open: the launchd supervisor carries no GC_REX_CITY, so supervised cities share gc:default; pin it at the supervisor level.
- Accepted: gc stop removed the city Rex session once (provider teardown stays block-only); server-restart recovery not exercised live because the server is shared (D3); start-config hooks still unapplied.
<!-- SECTION:FINAL_SUMMARY:END -->
