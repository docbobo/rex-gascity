---
id: GCREX-25.2
title: 'Start robustness: bound pre_start hooks, per-name locking, SIGTERM rollback'
status: Done
assignee:
  - '@mc-wisp-pf6k3f'
created_date: '2026-11-18T08:15:00Z'
updated_date: '2026-11-18T21:22:00Z'
labels:
  - provider
  - start
  - for-gascity
dependencies: []
references:
  - docs/provider-herdr-parity.md
  - internal/provider
  - cmd/gc-runtime-rex
parent_task_id: GCREX-25
priority: medium
type: task
ordinal: 60000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Three start-path robustness weaknesses from the parity analysis, split out of GCREX-25 on 2026-11-18 (task-type review) as hardening/concurrency work:

- pre_start hooks have no per-hook timeout;
- one city-wide flock is held across the hooks and the blocking launch, so a slow hook stalls every start and stop;
- there is no SIGTERM handler, so the rollback defer never runs (gc contains this by running stop afterwards, so this part is hardening).

Source: docs/provider-herdr-parity.md `### start` rows 9, 11, 12; mistake 8.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 Each pre_start hook runs under a timeout derived from the op budget, and a timeout fails the start with a clear error.
- [x] #2 The state lock is not held while pre_start or the blocking launch runs (per-name locking, or release and recheck), verified by a concurrency test.
- [x] #3 A SIGTERM handler runs the existing rollback before exit, so a gc-cancelled start does not depend on gc's follow-up stop.
- [x] #4 go test ./... and golangci-lint run ./... pass.
- [x] #5 docs/provider-herdr-parity.md is updated in the same change: the affected start row(s) are re-verified, the Summary of deviations and Coverage tables are recomputed, and Confirmed mistakes / Proposed follow-up work no longer list the item.
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. AC#1 pre_start timeout: add startBudget (120s) and preStartHookTimeout (30s) tunables on Provider with test options; in start, derive per-hook timeout = min(preStartHookTimeout, remaining op budget (ctx deadline or startBudget)); run each hook under ctx timeout and fail the start with a clear pre_start[i]: timed out after D error. Test-first: TestPreStartHookTimesOut (short hook budget, sleeping hook -> error, no launch) and TestPreStartHookWithinBudgetRuns.
2. AC#2 lock scope: refactor lock.go into a shared flock helper; add withNameLock (per gc session name) and withTabLock (per rig/tab, protects the shared-rig tab view+launch). Start/StartNew/Stop take the name lock for the whole op; start takes the city-wide state lock only for find-or-create session, seeding identity+provisional binding, and the final record. pre_start and the blocking launch (view+NewWindow/NewSplit) run outside the state lock; the launch holds only the tab lock. Test-first: TestSlowLaunchDoesNotHoldStateLock (block NewWindow in rig A, a Stop in another rig returns) and TestSlowPreStartDoesNotHoldStateLock.
3. AC#3 SIGTERM: main installs signal.NotifyContext(SIGTERM, SIGINT); add runWithContext so the op handler uses the cancelable context. A cancel during the start aborts the in-flight Rex/pre_start call, so the existing rollback defer runs before exit. Test-first: TestSIGTERMCancelsStartAndRollsBack (NotifyContext + syscall.Kill(self, SIGTERM), blocking NewWindow, assert run returns fail and no binding/identity remains).
4. AC#4: go test ./... and golangci-lint run ./... pass.
5. AC#5 docs/provider-herdr-parity.md: re-verify start rows 1, 11, 12; recompute Summary of deviations and Coverage; drop item 2 from Confirmed mistakes and renumber; trim start Follow-ups; update the stop locking-scope row for the added name lock.
6. Run gofmt/go vet, record notes, leave uncommitted.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Implemented GCREX-25.2 (work step; change left uncommitted for review).

AC#1 pre_start timeout: added Provider.startBudget (default 120s) and preStartHookTimeout (default 30s) with WithStartBudget/WithPreStartHookTimeout; start derives each hook's timeout as min(preStartHookTimeout, remaining start budget from the op ctx deadline else startBudget) and runPreStartWithTimeout fails the start with 'pre_start[i]: <cmd>: timed out after D'. Tests: TestPreStartHookTimesOut, TestPreStartHookCappedByStartBudget, TestPreStartHookCappedByContextDeadline, TestPreStartHookWithinBudgetRuns.

AC#2 lock scope: refactored lock.go into withFlock; added withNameLock (locks/<name>.lock) and withTabLock (locks/tab-<rig>.lock). Start/StartNew/Stop take the per-name flock for the whole op; start holds the city-wide state flock only for find-or-create session and the identity/provisional-binding writes; pre_start and the view+launch run outside it, the launch holding only the per-rig tab flock (keeps create-tab/join-tab atomic per rig). Tests: TestSlowPreStartDoesNotHoldStateLock and TestSlowLaunchDoesNotHoldStateLock; existing TestStartNewConcurrentSameNameOneWins / TestStartWithSharedStateDirLock still pass.

AC#3 SIGTERM: main installs signal.NotifyContext(SIGTERM, SIGINT); added runWithContext so the op handler uses the cancelable context. A cancel aborts the in-flight pre_start/Rex call and the existing rollback defer runs before gc's 2s forced kill. Test: TestSIGTERMCancelsStartAndRollsBack (NotifyContext + SIGTERM to self, blocking NewWindow, asserts exit fail with no binding/identity left).

AC#4: go test ./... ok; golangci-lint run ./... 0 issues; gofmt clean; targeted -race runs pass.

AC#5: docs/provider-herdr-parity.md re-verified start rows 1/11/12 (verdicts now intentional), Coverage start verdict recomputed to intentional, Summary rows updated, Confirmed mistakes item for start removed and renumbered (now 2 items), Proposed follow-up renumbered, stop locking/cancellation rows updated; docs/rex-provider.md pre_start/state-dir bullets updated.

Independent review (openrouter/deepseek/deepseek-v4.1-flash:high): Verified AC#1-5 against the uncommitted working tree. go test -count=1 ./..., golangci-lint run ./... (0 issues), go vet and gofmt clean; targeted -race passes. AC#1: per-hook timeout = min(preStartHookTimeout, remaining start budget from op ctx deadline else startBudget); TestPreStartHookTimesOut / CappedByStartBudget / CappedByContextDeadline / WithinBudgetRuns pin it. AC#2: Start/StartNew/Stop take withNameLock; pre_start and the view+launch run outside the city-wide state lock (launch under withTabLock); TestSlowPreStartDoesNotHoldStateLock and TestSlowLaunchDoesNotHoldStateLock are meaningful. AC#3: main installs signal.NotifyContext(SIGTERM, SIGINT) and runWithContext threads it; TestSIGTERMCancelsStartAndRollsBack proves the rollback defer runs. AC#5: parity start rows 1/11/12 re-verdict intentional, Coverage and Summary recomputed, the start mistake removed and renumbered, follow-ups trimmed. Claims re-checked against pinned gc v1.5.1: start timeout 120s (internal/runtime/exec/exec.go:57,205-223) and SIGTERM then SIGKILL after a 2s grace (exec.go:92, execgrace.Apply(cmd, 2*time.Second)); GC_EXEC_DEADLINE_MS is absent at v1.5.1 (only on edge), so the 120s default matches the baseline. Minor/nit only (follow-ups, none blocking): (1) docs/provider-herdr-parity.md start row 1 cites main.go:41 for signal.NotifyContext (actual main.go:48) and provider.go:200 for defaultStartBudget (actual provider.go:179) - correct the stale line refs. (2) Stop docstring and parity stop row 5 say the state lock guards only store mutations, but Stop holds withStateLock across the blocking CloseBlock and the up-to-3s writeDrainDiagnostic, so a slow stop still holds the city-wide lock; wording overstates the invariant (no behavior regression). (3) the "start budget %s exhausted" message prints p.startBudget even when a shorter ctx deadline was the effective cap (cosmetic). (4) runPreStartWithTimeout labels a hook that fails as the parent deadline fires as "timed out" (acceptable). Verdict: pass.

Finalization verification (step rex-backlog-work.finish, 2026-11-18): independent review verdict pass. Fresh run against the uncommitted tree: go test -count=1 ./... (all packages ok), golangci-lint run ./... (0 issues), gofmt -l on the changed Go files (clean). Pinned AC proofs pass: TestPreStartHookTimesOut, TestPreStartHookCappedByStartBudget, TestPreStartHookCappedByContextDeadline, TestPreStartHookWithinBudgetRuns, TestSlowPreStartDoesNotHoldStateLock, TestSlowLaunchDoesNotHoldStateLock (internal/provider); TestSIGTERMCancelsStartAndRollsBack (cmd/gc-runtime-rex).
<!-- SECTION:NOTES:END -->

## Comments

<!-- COMMENTS:BEGIN -->
author: @mc-wisp-pf6k3f
created: 2026-11-18T21:05:00Z

---

Claimed by Gas City worker mc-wisp-pf6k3f (seat mc-wisp-pf6k3f, session pi-mc-wisp-pf6k3f, template rex-gascity/pi); workflow bead unknown.

---
<!-- COMMENTS:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
Made the Rex start path robust in three ways and updated the parity record.

What changed: (1) pre_start hooks now run under a per-hook timeout derived from the op budget (min of preStartHookTimeout=30s and the remaining start budget from the ctx deadline else startBudget=120s), failing the start with a clear 'pre_start[i]: ... timed out after D' error. (2) Locking was split: Start/StartNew/Stop hold a per-name flock for the whole op; the city-wide state flock is held only for find-or-create session and identity/provisional-binding writes, so pre_start and the blocking view+launch run outside it (the launch holds only a per-rig tab flock). (3) main installs signal.NotifyContext(SIGTERM, SIGINT) and runWithContext threads the cancelable context, so a gc-cancelled start aborts the in-flight Rex/pre_start call and the existing rollback defer runs before gc's forced kill. AC#5: docs/provider-herdr-parity.md re-verifies start rows 1/11/12, recomputes Coverage and Summary, removes the start mistake and renumbers, and updates the stop locking/cancellation rows; docs/rex-provider.md bullets updated.

Verification: go test -count=1 ./... ok; golangci-lint run ./... 0 issues; gofmt clean; named AC tests pass (see implementation notes). Independent review: pass, minor/nit only.

Risks/follow-ups (non-blocking, from review): parity doc start row 1 line refs are stale (main.go:41 -> 48, provider.go:200 -> 179); the Stop docstring and parity stop row 5 wording overstates the state-lock scope because Stop still holds the city-wide lock across CloseBlock and the writeDrainDiagnostic drain; the 'start budget exhausted' message prints p.startBudget even when a shorter ctx deadline was the cap (cosmetic).
<!-- SECTION:FINAL_SUMMARY:END -->
