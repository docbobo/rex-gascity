---
id: GCREX-60
title: >-
  rex-backlog order fired for a day on a blocked task: align the gate with the
  picker, back off no-ops, alert when the board is blocked
status: Done
assignee: []
created_date: '2026-11-19T05:51:00Z'
updated_date: '2026-11-19T23:14:00Z'
labels:
  - workflow
milestone: m-0
dependencies: []
priority: high
type: bug
ordinal: 116000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->

Observed 2026-11-18/19 on ~/my-city. The rex-backlog order (orders/rex-backlog.toml, formula rex-backlog-work) fired 46 times between 2026-11-18T00:36:20 and 2026-11-19T06:37:03, roughly every 11-15 minutes, and booted a pi session plus gc.implementation-worker and gc.implementation-reviewer each time (23 of each template on 2026-11-19 alone). Every run closed with 'No backlog task claimed; nothing to review/address/finalize.' and changed nothing on the board. The change lives in ~/my-city (a separate repo: orders/, formulas/, scripts/, tests/), not in this checkout; this task is the board-side record so the defect is not lost.

Cause: the order's gate and the formula's picker evaluated different predicates. scripts/rex-backlog-ready.sh:47 matched any 'To Do' task labeled for-gascity (count 1: GCREX-59), while formulas/rex-backlog-work.toml:113,117 picked only --ready tasks (count 0: GCREX-59 is blocked by GCREX-56). The gate therefore fired forever against work the picker could never claim; the no-op close left the board unchanged, so the 300s stamp was the only brake and every poll repeated the empty run. gc's own suppression only logs (orderOpenWorkSuppressionAlertAfter = 20, gascity v1.5.1 cmd/gc/order_dispatch.go:2851) and never stops a dispatch.

Prevention, implemented in ~/my-city as three changes with a shell test harness (make test):

1. scripts/rex-backlog-ready.sh now counts with --ready, so the gate asks exactly what the picker asks; a blocked labeled task no longer fires the order.
2. A no-op run writes .gc/state/rex-backlog.noop, which the gate honours as a one-hour backoff, and mails the human once (gc mail --dedup); an empty run no longer retries every five minutes.
3. New exec order orders/rex-backlog-blocked.toml (cooldown 1h) runs assets/scripts/rex-backlog-blocked.sh, which mails the human when labeled work exists but none is ready, naming the blocked tasks.

Verification: tests/rex-backlog-ready.test.sh (10 cases; the blocked-todo case fails on the pre-fix gate at git HEAD and passes after) and tests/rex-backlog-blocked.test.sh (7 cases) pass under `make test`. Live: the gate now exits 1 against the real board and `gc order check` reports 'rex-backlog condition no condition: not met (exit 1)'.

Follow-up not done here: GCREX-56 is not labeled for-gascity, so no order can ever drain it and GCREX-59 stays blocked. Someone should label GCREX-56 for-gascity or re-scope that dependency.

<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria

<!-- AC:BEGIN -->

- [ ] #1 The rex-backlog gate and the formula picker use the same readiness predicate, with a named test that fails on the old gate and passes on the new one.
- [ ] #2 A no-op run backs the gate off for an hour and mails the human once; a test covers the backoff stamp and the mail.
- [ ] #3 A blocked board (labeled To Do, none ready, none in progress) is reported to the human by an order, with a test; an unblocked or busy board mails nothing.
- [ ] #4 make test passes in ~/my-city.
<!-- AC:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->

Change 2's no-op close was extracted from the formula into assets/scripts/rex-backlog-noop.sh so it is testable (tests/rex-backlog-noop.test.sh, 10 cases: writes the one-hour backoff stamp, clears stale state, mails the human once, closes the bead pass/no-op, and does nothing bead-side without GC_BEAD_ID). The formula's no-op block now just calls that script, so the close and the gate's backoff check can no longer drift apart — that drift is what let the loop run.

Final evidence in ~/my-city: `make test` runs three suites, 27 checks, all passing — tests/rex-backlog-ready.test.sh (10), tests/rex-backlog-blocked.test.sh (7), tests/rex-backlog-noop.test.sh (10). The ready suite is the red-to-green proof for the gate fix: its blocked-todo case fails on the pre-fix gate at git HEAD and passes on the new one.

<!-- SECTION:NOTES:END -->
