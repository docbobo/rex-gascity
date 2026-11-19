---
schema: gc.build.plan.v1
workflow: {id: gcrex-140, formula: build-basic}
methodology: {pack: gascity, name: build-basic}
producer: {formula: build-basic, stage: plan, attempt: 3}
status: approved
trace:
  upstream:
    - path: plans/GCREX-60/requirements.md
      hash: sha256:2b86050135ee63041421b3a6d61a78601d389c1ef3200c6fe7cdc609dadc4130
      ids: [REQ-001, REQ-002, REQ-003, REQ-004]
  coverage:
    - {id: REQ-001, status: covered}
    - {id: REQ-002, status: covered}
    - {id: REQ-003, status: covered}
    - {id: REQ-004, status: covered}
---

# Implementation Plan: GCREX-60 — rex-backlog order fired for a day on a blocked task

Requirements: `plans/GCREX-60/requirements.md` (bead `gcrex-3au`, convoy `gcrex-hrv`).
All code lives in `~/my-city`, not in this checkout.

## Summary

The fix already exists in `~/my-city` at commit `4c1318c` ("Stop the rex-backlog order looping on blocked work", 10 files, +659/−61). This plan does not re-implement it. It verifies the commit against REQ-001..REQ-004, fixes one comment typo, and leaves the order's `enabled` flag to the operator.

Checked on 2026-11-19:

- `make test` in `~/my-city` passes: three suites, 27 checks (blocked 7, noop 10, ready 10).
- The regression test is red on the pre-fix gate. Running `tests/rex-backlog-ready.test.sh` against `git show HEAD~1:scripts/rex-backlog-ready.sh` reports `FAIL - blocked labeled To Do (todo=1, ready=0) does not fire (want exit 1, got 0)` and `6 passed, 4 failed`.

## Current System

At `~/my-city` HEAD `4c1318c`:

- `scripts/rex-backlog-ready.sh` is the condition gate. It checks the 300s claim stamp (`.gc/state/rex-backlog.last`) and the 3600s no-op stamp (`.gc/state/rex-backlog.noop`); a malformed stamp counts as absent. It then requires `backlog task list --status "To Do" --labels for-gascity --ready --count` > 0 and an `In Progress` `for-gascity` count of 0. It fails closed, and its paths and intervals can be overridden with `REX_BACKLOG_*` env vars.
- `formulas/rex-backlog-work.toml`: the picker uses `--ready` (bugs first, then any type). On a no-op it calls `bash /Users/boris/my-city/assets/scripts/rex-backlog-noop.sh`.
- `assets/scripts/rex-backlog-noop.sh` clears the claimed-task and verdict state, writes the no-op stamp, runs `gc mail send human --dedup rex-backlog-noop`, and closes the bead as pass/no-op when `GC_BEAD_ID` is set.
- `orders/rex-backlog-blocked.toml` is a cooldown exec order (`interval = "1h"`) that runs `assets/scripts/rex-backlog-blocked.sh`. The script mails `human` with `--dedup rex-backlog-blocked`, naming the blocked tasks, when labeled `To Do` work exists, none is ready and none is in progress.
- `Makefile` target `test` runs every `tests/*.test.sh`.
- Working tree: `orders/rex-backlog.toml` has an uncommitted change from `enabled = true` to `enabled = false`. Its header comment says "no**on** backoff stamp" where it means "no-op backoff stamp".

## Proposed Implementation

1. **REQ-001 (shared predicate): verify only.** The gate and the picker both use `--ready`. The proof is `test_blocked_todo_does_not_fire` in `tests/rex-backlog-ready.test.sh`, which is red on the `HEAD~1` gate and green on HEAD (checked above). No code change.
2. **REQ-002 (no-op backoff): verify only.** `tests/rex-backlog-noop.test.sh` covers the stamp, the state clear, the single dedup mail, and the no-`GC_BEAD_ID` guard. The ready suite covers both backoff cases: "fresh no-op stamp suppresses the next poll" and "no-op stamp older than NOOP_INTERVAL fires again". No code change.
3. **REQ-003 (blocked-board alert): verify only.** `tests/rex-backlog-blocked.test.sh` covers three cases: a blocked board sends one mail naming the task, a ready board sends none, and a busy board sends none. No code change.
4. **Comment fix.** In the header of `orders/rex-backlog.toml`, change "noon backoff stamp" to "no-op backoff stamp". This is a one-word edit that changes no behavior and needs no test.
5. **`enabled` flag: do not touch.** Leave the uncommitted `enabled = false` in `orders/rex-backlog.toml` as the operator left it. Do not commit or revert it as part of this task (requirements, Open Questions). If step 4 is committed, stage only the comment hunk.
6. **REQ-004 (verification).** Run `make test` in `~/my-city` and record the per-suite counts in the implementation summary.

There are no overlay files in this change, so the overlay approval rule in `AGENTS.md` does not apply.

## Non-Goals

- Changing gc's order-suppression behavior. Per the requirements it only logs (`orderOpenWorkSuppressionAlertAfter = 20`, gascity v1.5.1 `cmd/gc/order_dispatch.go:2851`). This plan does not re-check that citation; it is taken from the requirements.
- Labeling GCREX-56 `for-gascity` or re-scoping the GCREX-56 → GCREX-59 dependency.
- Re-enabling `orders/rex-backlog.toml`, or changing the 300s poll, the 1h no-op backoff, or the 1h blocked-alert cooldown.
- Any code change in `rex-gas-city` beyond the plan artifacts.

## Verification

- `cd ~/my-city && make test` passes, with `blocked 7`, `noop 10`, `ready 10` (27 checks).
- Red/green proof for REQ-001: copy `tests/rex-backlog-ready.test.sh` next to `git show HEAD~1:scripts/rex-backlog-ready.sh` in a temp tree and run it. `blocked labeled To Do (todo=1, ready=0) does not fire` must FAIL. On HEAD it must pass.
- `grep -n noon ~/my-city/orders/rex-backlog.toml` returns nothing after step 4.
- `git -C ~/my-city diff orders/rex-backlog.toml` still shows the operator's `enabled = false` hunk unless the operator has changed it.

### Unresolved

- The validator gate for the requirements stage failed three times with `error: PyYAML is required to parse build artifacts` (`gc.attempt_log` on `gcrex-0az`). That is an environment problem, not an artifact defect. Run by hand from a worker shell, `validate_build_artifact.py` returns `{"ok": true}` for both `requirements.md` and this plan. PyYAML 6.0.3 is installed only in the user site-packages (`~/Library/Python/3.9/lib/python/site-packages`) for `/usr/bin/python3`. The cause is `unverified`, but the controller's check environment probably cannot see that user site-packages, so this plan's gate may fail the same way until PyYAML is installed system-wide or for that environment.
- Plan attempt 1 failed its gate the same way (`gc.attempt_log` on `gcrex-dhn`: `error: PyYAML is required to parse build artifacts`). Attempt 2 changed only this note and `producer.attempt`; the artifact content was already valid (`validate_build_artifact.py --schema gc.build.plan.v1` returns `{"ok": true}` from a worker shell). If the gate fails again, the fix is in the controller's check environment, not in this file.
- Plan attempt 2 failed its gate with the same PyYAML error. Attempt 3 changed only `producer.attempt` and this note.

## Coverage

| ID | Status |
| --- | --- |
| REQ-001 | covered |
| REQ-002 | covered |
| REQ-003 | covered |
| REQ-004 | covered |
