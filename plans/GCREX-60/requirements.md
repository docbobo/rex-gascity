---
schema: gc.build.requirements.v1
workflow: {id: gcrex-140, formula: build-basic}
methodology: {pack: gascity, name: build-basic}
producer: {formula: build-basic, stage: requirements, attempt: 3}
status: approved
trace:
  upstream:
    - path: beads/gcrex-3au
      hash: bead:gcrex-3au
    - path: beads/gcrex-hrv
      hash: bead:gcrex-hrv
  coverage:
    - {id: REQ-001, status: covered}
    - {id: REQ-002, status: covered}
    - {id: REQ-003, status: covered}
    - {id: REQ-004, status: covered}
---

# Requirements: GCREX-60 — rex-backlog order fired for a day on a blocked task

Input target: bead `gcrex-3au` (backlog item GCREX-60, P1 bug), via input convoy `gcrex-hrv`.
The change lives in `~/my-city` (`orders/`, `formulas/`, `scripts/`, `assets/scripts/`, `tests/`), not in this checkout.

## Problem Statement

On 2026-11-18/19 the `rex-backlog` order fired 46 times (about every 11-15 minutes) against a task it could never claim. Each run booted a pi session plus `gc.implementation-worker` and `gc.implementation-reviewer`, closed with "No backlog task claimed", and left the board unchanged.

Cause: the gate (`scripts/rex-backlog-ready.sh`) matched any `To Do` task labeled `for-gascity` (GCREX-59). The picker (`formulas/rex-backlog-work.toml`) selects only `--ready` tasks, and GCREX-59 is blocked by GCREX-56. The no-op close changed nothing, so the 300s stamp was the only brake. gc's own suppression only logs (`orderOpenWorkSuppressionAlertAfter = 20`, gascity v1.5.1 `cmd/gc/order_dispatch.go:2851`) and never stops a dispatch.

Goal: the gate and picker agree on readiness, an empty run backs off, and a blocked board is reported to the human.

## W6H

- **Who:** the human operator (Boris), who receives mail; the `rex-backlog` order and its implementation worker/reviewer agents.
- **What:** align the gate with the picker, back off no-op runs, alert on a blocked board.
- **When:** observed 2026-11-18T00:36:20 to 2026-11-19T06:37:03; the checks run on every orchestrator tick.
- **Where:** `~/my-city`: `scripts/rex-backlog-ready.sh`, `orders/rex-backlog.toml`, `orders/rex-backlog-blocked.toml`, `assets/scripts/rex-backlog-blocked.sh`, `assets/scripts/rex-backlog-noop.sh`, `formulas/rex-backlog-work.toml`, `tests/`.
- **Why:** the loop wasted compute and agent sessions and was silent for a day.
- **How:** shell scripts and orders with a shell test harness run by `make test`.
- **How much:** 46 fires; 23 worker and 23 reviewer sessions on 2026-11-19 alone.

## User Stories

- As the operator, I want the order to fire only when a task is claimable, so that no agent sessions boot for empty runs.
- As the operator, I want an empty run to back off for an hour and tell me once, so that a no-op does not repeat every five minutes.
- As the operator, I want to be told when labeled work exists but is all blocked, so that a stuck board is loud instead of silently idle.

## Technical Stories

- The gate counts `To Do` tasks with `--labels for-gascity --ready --count`, the same predicate as the picker.
- A no-op run writes `.gc/state/rex-backlog.noop`. The gate treats it as a one-hour backoff (`NOOP_INTERVAL=3600`), and a malformed stamp counts as absent.
- The no-op close lives in `assets/scripts/rex-backlog-noop.sh` and the formula's no-op block only calls it, so the close and the gate's backoff check cannot drift apart.
- The exec order `orders/rex-backlog-blocked.toml` (cooldown 1h) runs `assets/scripts/rex-backlog-blocked.sh`. It mails the human, naming the blocked tasks, when labeled work exists but none is ready and none is in progress. It uses `gc mail --dedup`, so there is one mail per blocked episode.
- Gate paths and intervals are overridable by `REX_BACKLOG_*` env vars so tests can drive a fake board.
- The gate fails closed: any error exits non-zero and the order does not fire.

## Behavior Requirements

- **REQ-001 (shared predicate):** the gate and the formula picker use the same readiness predicate. A labeled `To Do` task with an open dependency must not fire the order.
- **REQ-002 (no-op backoff):** a no-op run writes the backoff stamp so the gate stays closed for one hour, clears stale state, mails the human once, and closes the bead `pass`/no-op. It does nothing bead-side without `GC_BEAD_ID`.
- **REQ-003 (blocked-board alert):** when labeled `To Do` work exists, none is ready and none is in progress, an order mails the human naming the blocked tasks. An unblocked board or a busy board (a `for-gascity` task in progress) mails nothing.
- **REQ-004 (verification):** `make test` passes in `~/my-city` with every suite under `tests/*.test.sh`.

## Example Mapping

| Rule | Example | Expected |
| --- | --- | --- |
| Gate asks what the picker asks | GCREX-59 labeled `for-gascity`, `To Do`, blocked by GCREX-56 (ready count 0) | gate exits 1, order does not fire |
| Gate asks what the picker asks | A labeled `To Do` task with no open dependency, nothing in progress | gate exits 0 |
| Noop backoff | Last no-op stamp 10 minutes old | gate exits 1 |
| Noop backoff | Last no-op stamp older than 1h | gate may exit 0 |
| Blocked alert | Labeled `To Do` blocked, none ready, none in progress | one mail naming the blocked task |
| Blocked alert | A ready task exists, or one is `In Progress` | no mail |

## Acceptance Criteria

1. The rex-backlog gate and the formula picker use the same readiness predicate, with a named test that fails on the old gate and passes on the new one. Evidence: the blocked-todo case in `tests/rex-backlog-ready.test.sh` fails on the pre-fix gate at git HEAD and passes after.
2. A no-op run backs the gate off for an hour and mails the human once. A test covers the backoff stamp and the mail (`tests/rex-backlog-noop.test.sh`, plus the gate's backoff case in `tests/rex-backlog-ready.test.sh`).
3. A blocked board (labeled To Do, none ready, none in progress) is reported to the human by an order, with a test. An unblocked or busy board mails nothing (`tests/rex-backlog-blocked.test.sh`).
4. `make test` passes in `~/my-city`. Recorded evidence: three suites, 27 checks (ready 10, blocked 7, noop 10).

## Out Of Scope

- Changing gc's own order-suppression behavior (it only logs; that is upstream gascity).
- Labeling GCREX-56 `for-gascity` or re-scoping the GCREX-56 → GCREX-59 dependency. This is a recorded follow-up, not part of this task.
- Making changes in this checkout (`rex-gas-city`) beyond this board record and plan artifacts.
- Reworking the 300s poll stamp or the open-work single-flight gate.

## Open Questions

- Who labels GCREX-56 `for-gascity`, or should the GCREX-59 dependency be re-scoped? Until then GCREX-59 stays blocked, and the new alert will correctly report it.
- `orders/rex-backlog.toml` currently has `enabled = false`. Should it be re-enabled once this lands? The default assumption is that this is the operator's call and outside this task.
- The one-hour no-op backoff and the one-hour blocked-alert cooldown are fixed defaults. Is a different interval wanted? The assumption is no, since they are env-overridable for tests only.
- The implementation appears to already exist in `~/my-city` (commit "Stop the rex-backlog order looping on blocked work" plus working-tree edits to `orders/rex-backlog.toml`). Downstream stages should verify against it rather than re-implement it.

## Coverage

| ID | Status |
| --- | --- |
| REQ-001 | covered |
| REQ-002 | covered |
| REQ-003 | covered |
| REQ-004 | covered |
