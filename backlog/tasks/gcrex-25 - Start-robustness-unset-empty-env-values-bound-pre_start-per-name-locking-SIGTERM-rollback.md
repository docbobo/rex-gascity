---
id: GCREX-25
title: 'Harden the Rex start path (split: RPP env conformance + start robustness)'
status: Done
assignee:
  - '@mc-wisp-dux7ns'
created_date: '2026-11-18T06:14:00Z'
updated_date: '2026-11-18T22:02:00Z'
labels:
  - provider
  - start
  - for-gascity
dependencies: []
references:
  - docs/provider-herdr-parity.md
  - internal/provider
  - cmd/gc-runtime-rex
priority: medium
type: task
ordinal: 50000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Umbrella for the start-path work that was GCREX-25, split on 2026-11-18 during a task-type review.

The parity analysis found four start-path weaknesses (docs/provider-herdr-parity.md `### start` rows 1, 9, 11, 12; mistakes 7 and 8). They split by kind:

- **RPP-conformance defect** — empty start-config env values are applied as `env KEY=` although RPP v0 defines empty as unset and gc's tmux adapter uses `env -u` → **GCREX-25.1** (bug).
- **Hardening and concurrency** — pre_start hook timeout, per-name locking, SIGTERM rollback → **GCREX-25.2** (task).

The two subtasks are independent and can land separately.
<!-- SECTION:DESCRIPTION:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. Confirm both children are complete: GCREX-25.1 (3/3 AC) and GCREX-25.2 (5/5 AC) are Done with independent review pass.
2. Confirm the children's changes are committed (5c0ee9b finalizes 25.1; 58f4535 'provider: harden the Rex start path' carries 25.2).
3. No code change is required: GCREX-25 is an umbrella with no acceptance criteria of its own; its scope is fully delivered by the two subtasks.
4. Run go test ./... and golangci-lint run ./... on the current tree as health evidence.
5. Leave the task In Progress for review/finish, which marks the umbrella Done with a final summary.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Split into GCREX-25.1 (bug: empty env values as env -u) and GCREX-25.2 (task: pre_start timeout, per-name locking, SIGTERM rollback) on 2026-11-18 during a task-type review. Original description and acceptance criteria moved to the subtasks; this task is now the umbrella.

Work step (mc-wisp-dux7ns, 2026-11-18): umbrella task, no implementation required.

- Both children complete: GCREX-25.1 (3/3 AC, Done, independent review pass, finalized in commit 5c0ee9b 'Finalize GCREX-25.1: empty start-config env values emit env -u') and GCREX-25.2 (5/5 AC, Done, independent review pass, code landed in commit 58f4535 'provider: harden the Rex start path').
- Parity doc docs/provider-herdr-parity.md confirms both resolved: start row 9 empty-env is intentional; start rows 1/11/12 (SIGTERM rollback, bounded pre_start, per-name/per-rig lock scope) are intentional with named tests.
- Verification on the current tree: go test ./... all packages ok; golangci-lint run ./... 0 issues.
- No code change made; the worktree diff for this step is limited to this task's Backlog.md markdown (claim comment + plan). Tooling/agent-state dirs untouched.
- Handoff: review the task-markdown change, then finish by marking this umbrella Done with a final summary; no acceptance criteria of its own remain.

Independent review (openrouter/deepseek/deepseek-v4.1-flash:high), uncommitted worktree on dd7c1d1.

Scope: this step's change is the GCREX-25 task markdown only (plan, notes, claim comment, status In Progress, for-gascity label, assignee). No code change is expected: the umbrella's scope is delivered by GCREX-25.1 and GCREX-25.2.

Blocking: none.
Major: none.

Minor/nit (follow-ups, not blocking):
1. Implementation Notes is a single line carrying literal \n escape sequences (task markdown line 51) instead of real line breaks; fix while finalizing.
2. The note "the worktree diff for this step is limited to this task's Backlog.md markdown" holds for this step's own edits but is misleading about the worktree: GCREX-27's markdown is modified, and GCREX-28's markdown plus the GCREX-28/32/36/37 task files and docs/provider-method-audit/ are untracked, all from other runs. Finalize must stage only this task's file.
3. Nit: the umbrella now carries for-gascity and an assignee, making it order-eligible; harmless because it is In Progress and finalize marks it Done.

Verified:
- GCREX-25.1: Done, 3/3 AC, independent review pass; commit 5c0ee9b exists and contains the task file.
- GCREX-25.2: Done, 5/5 AC, independent review pass; commit 58f4535 "provider: harden the Rex start path" contains the code, tests, parity doc and rex-provider doc.
- docs/provider-herdr-parity.md start rows 1 (:333), 9 (:341), 11 (:343) and 12 (:344) are verdict intentional with the named tests cited; summary row at :98, start verdict at :351.
- Named tests exist: TestPreStartHookTimesOut, TestPreStartHookCappedByStartBudget, TestPreStartHookCappedByContextDeadline (internal/provider/prestart_test.go), TestSlowPreStartDoesNotHoldStateLock, TestSlowLaunchDoesNotHoldStateLock (internal/provider/lock_scope_test.go), TestSIGTERMCancelsStartAndRollsBack (cmd/gc-runtime-rex/signal_test.go).
- Fresh evidence: go test -count=1 ./... all packages ok; golangci-lint run ./... 0 issues.

Verdict: pass.

Verification (finalize, 2026-11-18): no acceptance criteria or DoD items are defined on this umbrella, so none were checked. Both subtasks are Done and their autonomous-review verdicts are recorded: GCREX-25.1 (commit 5c0ee9b) and GCREX-25.2 (commit 58f4535). go test -count=1 ./... and golangci-lint run ./... pass on the current tree. Finalize step changed only this task's markdown; the literal \n artifact from the work step was normalized to real line breaks via the Backlog CLI. Unrelated worktree files (GCREX-27/28/32/36/37 and docs/) belong to other runs and were not staged.
<!-- SECTION:NOTES:END -->

## Comments

<!-- COMMENTS:BEGIN -->
author: @mc-wisp-dux7ns
created: 2026-11-18T21:53:00Z

---

Claimed by Gas City worker mc-wisp-dux7ns (seat mc-wisp-dux7ns, session pi-mc-wisp-dux7ns, template rex-gascity/pi); workflow bead mc-0tr4, step bead mc-z7uc.

---

author: @rex-reviewer
created: 2026-11-18T21:56:00Z

---

Independent review recorded: verdict pass, no blocking or major findings.

---
<!-- COMMENTS:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
Umbrella only, no code change: GCREX-25 was split on 2026-11-18 into GCREX-25.1 (RPP env conformance: empty start-config env values emit env -u; commit 5c0ee9b) and GCREX-25.2 (start robustness: bounded pre_start hooks, per-name locking, SIGTERM rollback; commit 58f4535). Both subtasks are Done with independent review pass, so the umbrella's scope is fully delivered. This step's change is the GCREX-25 task markdown (plan, notes, final summary, status); it has no acceptance criteria of its own. Verification: fresh go test -count=1 ./... all packages ok and golangci-lint run ./... 0 issues; the reviewed markdown diff is the only file staged. Risks/follow-ups: none beyond unrelated untracked worktree files from other runs, left untouched.
<!-- SECTION:FINAL_SUMMARY:END -->
