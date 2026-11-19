---
id: GCREX-53
title: 'start: fail when the agent exits during startup or before its first turn'
status: Done
assignee:
  - '@mc-wisp-edev0l'
created_date: '2026-11-18T22:05:00Z'
updated_date: '2026-11-19T03:39:00Z'
labels:
  - provider
  - herdr
  - for-gascity
  - tdd
dependencies: []
references:
  - docs/provider-method-audit/start.md
documentation:
  - docs/provider-herdr-method-io.md
priority: medium
type: task
ordinal: 109000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Develop test-first (TDD): write the failing test that reproduces this first, watch it go red, then the smallest change that makes it green (repo rule in AGENTS.md; see also the tdd skill).

Avoids: start reports success for an agent that was never usable.

The document (## start § Failure cases, § Conflicts #5) fails the op when the agent exits during startup or before its first turn, unless lifecycle=one_shot. The implementation treats an instant exit and a missed first turn as success and does not distinguish one_shot.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 An instant agent exit or a missed first turn exits non-zero.
- [x] #2 Lifecycle one_shot is distinguished from the default and does not fail on that check.
- [x] #3 Both cases have tests.
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. Red tests (cmd/gc-runtime-rex/dispatch_test.go): TestStartInstantExitFails (dead pane right after launch -> exit 1, stderr names 'exited during startup', pane left in place); TestStartOneShotInstantExitSucceeds ('lifecycle':'one_shot' -> exit 0); TestStartMissedFirstTurnFails (pane alive at the post-launch probe, dead during the startup-nudge readiness wait -> exit 1, 'before its first turn'); TestStartOneShotMissedFirstTurnSucceeds (same, one_shot -> exit 0). Add a ProcessFunc to internal/rex/faketest to model liveness changing between calls. 2. Code: add Config.Lifecycle and parse 'lifecycle' in startConfig (cmd/gc-runtime-rex/main.go). In internal/provider: const lifecycleOneShot = "one_shot"; failIfAgentEnded (single paneAlive probe after launch, mirrors herdr-core _fail_if_ended) wired into Start/StartNew; waitForStartupReady detects a dead pane and returns an 'ended' outcome; deliverStartupNudge returns the before-first-turn error unless one_shot. 3. Verify go test ./... and golangci-lint run ./.... 4. Docs: move the post-launch liveness row from Deviations to Matches in docs/provider-method-audit/start.md; update docs/provider-herdr-parity.md row 14 and the start Our-implementation paragraph. Known limitation to record: Rex exposes no 'agent process appeared' signal, so the LAUNCH_WAIT_S/AGENT_WAIT_S wait is not replicated; an instant exit is caught by the post-launch probe and the readiness wait (nudge) only. Leave uncommitted.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
TDD. Red: TestStartInstantExitFails and TestStartMissedFirstTurnFails failed against HEAD (exit 0; the missed-first-turn case spent 38s in the full nudge ladder). Green: all four new tests pass (0.18s). Code: Config.Lifecycle + startConfig.lifecycle (main.go:71-79,236-256); lifecycleOneShot and failIfAgentEnded, a single post-launch pane liveness probe (provider.go:295-318) wired into Start and StartNew; waitForStartupReady returns an 'ended' outcome when paneAlive is false (nudge_delivery.go:286-333) and deliverStartupNudge returns the before-first-turn error unless one_shot (one_shot skips the failure and never types into a dead pane). Test seam: added ProcessFunc to faketest and made CloseBlock drop the closed block's process state so a relaunched pane is live; started/nudgeStarted clear BlockCalls after Start because the post-launch probe is now part of Start. Docs: moved the liveness row from Deviations to Matches in docs/provider-method-audit/start.md; parity rows 2 and 14 updated and the start paragraph/verdict/gap list adjusted. Verification: go test -count=1 ./... all packages ok; golangci-lint run ./... 0 issues. Known gap recorded: Rex exposes no 'agent process appeared' signal, so the 5s/30s _await_agent wait is not replicated; process_names still ignored.

Independent review (openrouter/deepseek/deepseek-v4.1-flash:high), step bead mc-fenv. Read-only: nothing edited, staged, committed, or pushed.

Scope: cmd/gc-runtime-rex/{main.go,dispatch_test.go}; internal/provider/{provider.go,nudge_delivery.go,input_test.go,nudge_test.go}; internal/rex/faketest/fake.go; docs/provider-herdr-parity.md (start row 2, row 14, row 15 context, Our-implementation paragraph); docs/provider-method-audit/start.md; the GCREX-53 task file. Excluded: the unrelated gcrex-27/gcrex-28 markdown rewrites, the untracked GCREX-37 audit/task files, and .pi/.pi-agent/.gc runtime state.

Verified: `go test -count=1 ./...` all packages ok (cmd/gc-runtime-rex 6.8s, internal/provider 7.0s); `golangci-lint run ./...` 0 issues; `gofmt -l cmd internal` empty. Red proof reproduced in a /tmp copy (tests + faketest at the working tree, main.go/provider.go/nudge_delivery.go reverted to HEAD 12da43b): TestStartInstantExitFails fails (exit 0, empty stderr, 0.01s) and TestStartMissedFirstTurnFails fails (exit 0 after the 38.03s nudge ladder), matching the notes; all four new tests pass green. AC#1 met: instant exit and missed first turn exit 1 with `the agent exited during startup` / `...before its first turn` on stderr and empty stdout (TestStartInstantExitFails, TestStartMissedFirstTurnFails; the first also pins that the dead pane stays for gc's stop). AC#2 met: `lifecycle=one_shot` skips both checks and no nudge is typed into a dead pane (TestStartOneShotInstantExitSucceeds, TestStartOneShotMissedFirstTurnSucceeds). AC#3 met: four dispatcher tests plus the faketest ProcessFunc/CloseBlock seam. gc v1.5.1 re-checked: `internal/runtime/exec/json.go:29` carries Lifecycle and `internal/runtime/runtime.go:673-678` defines only `LifecycleOneShot = "one_shot"`, so the `one_shot`-only distinction is right; `internal/runtime/exec/exec.go:262-270` cleanupAfterStartFailure runs `Stop` after a failed start, so leaving the dead pane in place is the correct shape. herdr-core @0f07496 re-checked: `ONE_SHOT` lifecycle.py:161; `_fail_if_ended` :1027-1036; first_turn gated by `if cfg.nudge:` :819, raise at :1125, one_shot warning at :1124. Message, exit code and one_shot gating match the spec doc's start failure table.

Findings by severity.

Major 1 - the docs that describe the nudge wait contradict the new behavior. The change adds a sixth readiness outcome (`ended`, internal/provider/nudge_delivery.go:53) and makes the wait fail the start when the agent has ended before its first turn, but docs/rex-provider.md:430 still says "The wait and delivery never fail the start" (the wait now can), and its diagnostic table at :377 enumerates only idle/window-bound/settle-bound/canceled/disabled; docs/provider-herdr-parity.md:347 row 15 still says "failures swallowed" and lists `readiness_outcome` without `ended`. Row 14 and the audit doc do document the new failure correctly, so these two now contradict the change (and each other). Fix: qualify the rex-provider.md sentence (delivery never fails; the wait fails only on the ended case) and add `ended` to both outcome lists.

Minor 1 - line refs in the touched cells are stale. docs/provider-herdr-parity.md:325 start paragraph: Provider.Start/StartNew cited at :248/:267 (actual 251/272), `start` at :295 (actual 332), and the refs after the inserted code (close-dead-pane :318, state lock :346, pre_start :358/:776, work_dir :389, overlay :396, provisional binding :400-410, env :418, launch :422, binding :444/:452, nudge :455-572) are stale by the inserted lines; opStart is still cited main.go:180 (actual 231) while the same sentence now cites main.go:236-256. Row 15 still cites main.go:55-61 for startConfig (actual 71-79) although row 2 was refreshed. Audit start.md cites nudge_delivery.go:94-169 (actual 94-173) and :286-333 (286-336). Row 2's gc ref exec/json.go:23-39 is 26-42 at v1.5.1.

Minor 2 - the "an instant exit is caught by the post-launch probe" claim (parity row 14, audit Matches) is proven only against the dispatcher fake; whether a real instant Rex exit is visible to the single immediate BlockProcess, rather than the pack's 5s/30s _await_agent wait, remains unverified, and the pre-change row's unverified marker was dropped rather than resolved. Suggest wording it as "an exit visible to the post-launch probe" or keeping an `unverified` note.

Minor 3 - docs/provider-herdr-method-deviations.md:95 still lists the post-launch liveness deviation (medium, old implementation text); the compiled rollup still needs the workstream regeneration pass (known staleness flagged in GCREX-49's review).

Nit 1 - failIfAgentEnded's stderr omits the pack's `(exit code N / signal S)` detail that rex.ProcessInfo.LastExit carries (herdrcore/lifecycle.py:1035); gc does not parse the phrase, so cosmetic.
Nit 2 - no test pins the new readiness_outcome=ended diagnostic value; the failure behavior itself is covered.

Verdict: fail - implementation, tests and ACs are met; Major 1 is a docs contradiction that should be fixed before commit.

Addressed review findings (Minor 1-3 + Nit 2; Major 1 was the docs contradiction). Docs: docs/rex-provider.md now lists the 'ended' readiness outcome in the diagnostic table and its explanation, and rewrote 'The wait and delivery never fail the start' to 'Delivery never fails the start' plus the one wait failure (agent ended before its first turn, non-one_shot). docs/provider-herdr-parity.md row 15 drops 'failures swallowed' and lists ended; row 14 now says 'an exit visible to the post-launch probe' and marks real-Rex instant-exit visibility unverified; the start paragraph's provider.go/main.go refs were refreshed (opStart 231, Start/StartNew 251/272, start 332, close-dead-pane 375, FindSession 388, pre_start 407/848, work_dir 433, overlay 441, provisional 447-458, tab lock 469, final binding 486/494, launch.go 154-175 applied 462, rollback 344-353, defaultStartBudget 181, signal 52, runWithContext 136, dispatch 107, already-running 255, nudge_delivery.go 94-173/286-336); row 2 exec/json.go 23-39 -> 26-42. docs/provider-method-audit/start.md refs refreshed and an Unverified bullet added for the real-Rex probe. docs/provider-herdr-method-deviations.md dropped the now-false post-launch liveness deviation row (full rollup regeneration still pending, GCREX-49). Test: added TestStartupTimingRecordsEndedReadiness (identity_test.go) pinning readiness_outcome=ended. go test -count=1 ./... ok; golangci-lint run ./... 0 issues; gofmt clean. Left uncommitted.

Independent review (openrouter/deepseek/deepseek-v4.1-flash:high), step bead mc-dprk, iteration 2; read-only: nothing edited, staged, committed or pushed. Re-reviewed the tracked GCREX-53 diff and the address-step edits (cmd/gc-runtime-rex/{main.go,dispatch_test.go}; internal/provider/{provider.go,nudge_delivery.go,identity_test.go,input_test.go,nudge_test.go}; internal/rex/faketest/fake.go; docs/provider-herdr-parity.md; docs/provider-method-audit/start.md; docs/rex-provider.md; the task file).

Verified: `go test -count=1 ./...` all packages ok (cmd/gc-runtime-rex 6.7s, internal/provider 7.2s); `golangci-lint run ./...` 0 issues; `gofmt -l cmd internal` empty. Red proof re-run in a /tmp copy (working-tree tests, provider.go/nudge_delivery.go/main.go reverted to HEAD 12da43b): TestStartInstantExitFails fails (exit 0, empty stderr, 0.01s), TestStartMissedFirstTurnFails fails (exit 0 after the 38.03s nudge ladder), TestStartupTimingRecordsEndedReadiness fails (Start error nil). All four dispatcher tests plus the new identity test pass green. AC#1-#3 met. gc v1.5.1 re-checked: `exec/json.go:26-42` carries `lifecycle` at :29, `runtime.go:672-677` defines only `LifecycleOneShot`, `exec.go:262-270` runs Stop after a failed start. herdr-core @0f07496 re-checked: ONE_SHOT `lifecycle.py:161`, `_fail_if_ended` :1027-1036, `first_turn` :1113-1125 (raise :1125), `if cfg.nudge:` :819, `_await_agent` :1004. The refreshed line refs in row 14, row 15, the start paragraph and the audit all match the working tree. Iteration-1 Major 1 (rex-provider.md sentence + row 15) is fixed; Minors 1-3 and Nit 2 are addressed.

Findings.

Major 1 (new) - docs/provider-herdr-parity.md:106, the "Summary of deviations" table, still says `post-launch liveness check ... not-implemented ... dying agent surfaces later via is-running`. The change implements the check and updated detailed row 14 (:346) to intentional (with a gap), so the summary row now contradicts the code, the same document's detail row and the audit. Fix: update its Rex column and verdict to match row 14 (it remains an intentional-with-a-gap deviation because the process-appeared wait is not replicated and process_names is still ignored).

Minor 1 - the same summary table at :93 still says "6 of the 15 gc sends; rest ignored" while detailed row 2 now says 7 (lifecycle as the one_shot gate).

Minor 2 - the untracked docs/provider-herdr-method-io.md:902-904 (this task's documentation reference) still says "Rex does not (parity row 14, not-implemented)"; update it in the GCREX-37/39 regeneration pass, or now.

Minor 3 - the partially edited untracked docs/provider-herdr-method-deviations.md is internally inconsistent: the start per-op table dropped the liveness row but the glance at :30 still counts 10 deviations / 4 medium (now 9 / 3), and the GCREX-52 empty-stdin row still lists a fixed deviation; the full rollup regeneration remains pending.

Nit - carried over from iteration 1: the stderr message omits the pack's "(exit code N / signal S)" detail (cosmetic; gc does not parse it).

Verdict: fail - implementation, tests and ACs are met; Major 1 is a parity-doc contradiction that should be fixed before commit.

Addressed review findings (iteration 2). Major 1 fixed: docs/provider-herdr-parity.md summary-of-deviations row for start/post-launch liveness check now matches the detailed row 14 - Rex cell states the single failIfAgentEnded probe plus the nudge readiness wait fail 'the agent exited during startup[ before its first turn]' and lifecycle=one_shot skips both; verdict changed not-implemented -> intentional (with a gap); rationale records GCREX-53 plus the unverified real-Rex instant-exit visibility and the un-replicated _await_agent wait. Minor 1 fixed in the same table: start/config-fields-consumed Rex cell 6 -> 7 of the 15 gc sends, matching detailed row 2. Minors 2-3 (untracked docs/provider-herdr-method-io.md:902-904 and docs/provider-herdr-method-deviations.md glance counts) left to the GCREX-37/49 rollup regeneration, which those files already await; they are wholesale-stale, not part of this diff. Docs-only, no behavior change. go test -count=1 ./... all packages ok; golangci-lint run ./... 0 issues; gofmt -l cmd internal empty. Left uncommitted.

Independent review (openrouter/deepseek/deepseek-v4.1-flash:high), step bead mc-li91, iteration 3; read-only: nothing edited, staged, committed or pushed. Re-reviewed the tracked GCREX-53 diff and the iteration-3 address edits (docs/provider-herdr-parity.md summary-of-deviations row).

Verified: iteration-2 Major 1 is fixed - the Summary of deviations row for start/post-launch liveness check (docs/provider-herdr-parity.md:106) now matches detailed row 14 and the code: the single `failIfAgentEnded` probe plus the nudge readiness wait, `lifecycle=one_shot` skips both, verdict not-implemented -> intentional (with a gap), with the un-replicated `_await_agent` wait and the unverified real-Rex instant-exit visibility recorded. Iteration-2 Minor 1 fixed: summary table start/config-fields 6 -> 7 of the 15 gc sends, matching detailed row 2. Both sources re-checked: gc v1.5.1 `internal/runtime/exec/json.go:26-42` carries the 15 fields with `lifecycle` at :29 and `internal/runtime/runtime.go:672-677` defines only `LifecycleOneShot = "one_shot"`; `internal/runtime/exec/exec.go:262-270` runs Stop after a failed start, so leaving the dead pane for gc's post-failure stop is the correct shape. herdr-core @0f07496: `ONE_SHOT` lifecycle.py:161, `_await_agent` :1004-1024 (LAUNCH_WAIT_S 5.0 :169, AGENT_WAIT_S 30.0 :170), `_fail_if_ended` :1027-1036, `if cfg.nudge:` :819, `first_turn` :1113-1125 (one_shot warning :1124, raise :1125). Red proof independently reproduced in a /tmp copy (working-tree tests + faketest over HEAD 12da43b sources): TestStartInstantExitFails fails (exit 0, empty stderr), TestStartMissedFirstTurnFails fails (exit 0 after the 38.04s ladder), TestStartupTimingRecordsEndedReadiness fails (Start error nil). Green in tree: `go test -count=1 ./...` all packages ok (cmd/gc-runtime-rex 6.6s, internal/provider 6.9s); `golangci-lint run ./...` 0 issues; `gofmt -l cmd internal` empty. AC#1-#3 met with the four dispatcher tests plus the identity test. The tracked docs are now internally consistent (parity summary vs detail vs audit; rex-provider.md readiness table and narrative).

Findings. Minor 1 (carried, out of the task's commit scope): the untracked docs/provider-herdr-method-io.md and docs/provider-herdr-method-deviations.md are still the pre-change snapshot - the deviations glance counts start 10/4 and its start table still lists the empty-stdin deviation fixed at HEAD 12da43b, and the io doc still calls the liveness check not-implemented; both await the GCREX-37/49 regeneration pass. Minor 2: non-touched rows in the tracked docs/provider-method-audit/start.md still cite pre-change line refs (staging row `provider.go:334-365`, pre_start-fatal row `provider.go:349-351`; the code is at :403-441 and :417-424); they were already stale at HEAD and belong to the same regeneration pass, noted because the file is in this diff. Nit (carried): the failIfAgentEnded stderr omits herdr-core's `(exit code N / signal S)` detail that rex.ProcessInfo.LastExit carries; cosmetic, gc does not parse the phrase. Nit: a one_shot start that ends before its first turn returns silently where herdr-core emits a warning; subsumed by the documented no-warnings-channel gap (parity row 16).

Verdict: pass - no blocking or major issues; only minor/nit follow-ups remain.

Finalization verification (2026-11-19): make test (go test ./...) all packages ok; golangci-lint run ./... 0 issues; gofmt -l cmd internal empty; the five GCREX-53 tests pass (TestStartInstantExitFails, TestStartOneShotInstantExitSucceeds, TestStartMissedFirstTurnFails, TestStartOneShotMissedFirstTurnSucceeds, TestStartupTimingRecordsEndedReadiness). AC#1-#3 proven by these named tests. Review-loop verdict: pass (iteration 3, step mc-li91).
<!-- SECTION:NOTES:END -->

## Comments

<!-- COMMENTS:BEGIN -->
author: @mc-wisp-edev0l
created: 2026-11-19T02:50:00Z

---

Claimed by Gas City worker mc-wisp-edev0l (seat rex-gascity/pi-1, session pi-mc-wisp-edev0l, template rex-gascity/pi); workflow bead mc-a5ky, step bead mc-jb0d.

---
<!-- COMMENTS:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
start now fails when the agent exits during startup or before its first turn, unless lifecycle=one_shot. Added Config.Lifecycle (parsed from the 'lifecycle' start field), lifecycleOneShot, failIfAgentEnded (single post-launch pane-liveness probe wired into Start/StartNew), an 'ended' outcome from waitForStartupReady, and a before-first-turn failure from deliverStartupNudge unless one_shot. Test seam: faketest ProcessFunc + CloseBlock process-state drop. Docs updated: provider-herdr-parity.md (start rows 2/14/15, summary-of-deviations row, start paragraph), provider-method-audit/start.md, rex-provider.md readiness table/narrative. Verified: make test all ok; golangci-lint 0 issues; gofmt clean; red proof reproduced in a /tmp copy over HEAD 12da43b. Known gap (documented): Rex exposes no 'agent process appeared' signal, so herdr-core's _await_agent 5s/30s wait is not replicated; real-Rex instant-exit visibility to the probe is unverified. Review-loop verdict: pass.
<!-- SECTION:FINAL_SUMMARY:END -->
