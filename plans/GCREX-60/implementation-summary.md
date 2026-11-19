---
schema: gc.build.implementation-summary.v1
workflow: {id: gcrex-140, formula: build-basic}
methodology: {pack: gascity, name: build-basic}
producer: {formula: build-basic, stage: summarize-implementation, attempt: 3}
status: approved
trace:
  upstream:
    - path: plans/GCREX-60/requirements.md
      hash: sha256:2b86050135ee63041421b3a6d61a78601d389c1ef3200c6fe7cdc609dadc4130
      ids: [REQ-001, REQ-002, REQ-003, REQ-004]
    - path: plans/GCREX-60/decomposition.md
      hash: sha256:dfd81bb6bbd7b6a7a32755ad8e2dfd341681d52aa69143c877ced4ec04a6c7a0
    - path: plans/GCREX-60/implementation-plan.md
      hash: sha256:325c9d29870f46d9567a8215a9df62cc5d941b741ea847aa635c8451db15a20c
    - path: plans/GCREX-60/implementation-plan.md#implementation-convoy
      hash: sha256:325c9d29870f46d9567a8215a9df62cc5d941b741ea847aa635c8451db15a20c
      ids: [gcrex-hvlf, gcrex-72t8]
    - path: beads/gcrex-0sa1
      hash: bead:gcrex-0sa1
  coverage:
    - {id: REQ-001, status: covered}
    - {id: REQ-002, status: covered}
    - {id: REQ-003, status: covered}
    - {id: REQ-004, status: covered}
    - {id: gcrex-hvlf, status: deferred, rationale: "Convoy item remained open with no summary; its verification evidence (REQ-001..REQ-004) was performed directly in this stage against the ~/my-city working tree."}
    - {id: gcrex-72t8, status: covered}
---

# Implementation Summary: GCREX-60 — rex-backlog order fired for a day on a blocked task

## Summary

The rex-backlog order fired 46 times from 2026-11-18T00:36 to 2026-11-19T06:37
against a labeled `To Do` task it could never claim: the gate
(`scripts/rex-backlog-ready.sh`) matched any `for-gascity` `To Do` task, while
the picker (`formulas/rex-backlog-work.toml`) selected only `--ready` ones, and
GCREX-59 was blocked by GCREX-56. Each run booted pi plus
`gc.implementation-worker` and `gc.implementation-reviewer`, closed "No backlog
task claimed", and left the board unchanged; the 300s stamp was the only brake.

The fix already existed in `~/my-city` at commit `4c1318c` ("🐛 Stop the
rex-backlog order looping on blocked work"). This stage verified it against
REQ-001..REQ-004 rather than re-implementing it: the gate now asks the picker's
`--ready` predicate, a no-op run closes through `rex-backlog-noop.sh` with a
one-hour backoff stamp and one mail, and a new `rex-backlog-blocked` order mails
the human when labeled work exists but none is ready and none is in progress.

Implementation convoy `gcrex-0sa1` (items `gcrex-hvlf` verify
REQ-001..REQ-004, `gcrex-72t8` fix the "noon" comment typo) was still `open`
with no item summary when this stage ran; an upstream drain step closed
`gc.outcome=fail` (`gc.failure_reason=predecessor_failed`,
`gc.failure_subject=gcrex-9zh`), so no `gc.implementation.summary_path`,
`gc.build.implementation_summary_path`, or `gc.var.summary_path` exists on any
convoy item. With no item summaries to merge, the verification below was
performed here directly against the `~/my-city` working tree so the root
summary carries real evidence; `gcrex-hvlf` is therefore marked `deferred` in
coverage. The optional typo item (`gcrex-72t8`) was also completed here:
`orders/rex-backlog.toml` line 6 "no noon backoff stamp" now reads "no no-op
backoff stamp", with the operator's uncommitted `enabled = false` left
untouched.

## Intended Behavior

- **Gate (REQ-001):** `scripts/rex-backlog-ready.sh` counts `To Do` tasks with
  `--labels for-gascity --ready --count` — the same predicate the picker uses —
  and refuses to fire when a labeled task is `In Progress`, the last poll stamp
  is younger than `MIN_INTERVAL` (300s), or the no-op stamp is younger than
  `NOOP_INTERVAL` (3600s). It fails closed: any error exits non-zero.
- **No-op backoff (REQ-002):** a dispatch that claimed nothing calls
  `assets/scripts/rex-backlog-noop.sh`, which writes `.gc/state/rex-backlog.noop`
  (epoch), mails the human once per episode via `gc mail --dedup`, clears the
  claimed-task and review-verdict files, and closes the bead `pass`/`no-op` when
  `GC_BEAD_ID` is set. Without `GC_BEAD_ID` it does no bead work. A malformed
  stamp counts as absent.
- **Blocked alert (REQ-003):** `orders/rex-backlog-blocked.toml` (cooldown 1h)
  runs `assets/scripts/rex-backlog-blocked.sh`, which mails the human naming the
  blocked tasks when labeled `To Do` work exists, none is ready, and none is in
  progress. A ready or busy board mails nothing. Dedup is owned by the script.
- **Verification (REQ-004):** `make test` runs every suite under `tests/*.test.sh`.

## Changed Files

Committed in `~/my-city` at `4c1318c` (10 files, +659/−61):

- `scripts/rex-backlog-ready.sh` — gate now uses `--ready`, honours the no-op
  stamp (`NOOP_INTERVAL=3600`), fails closed.
- `formulas/rex-backlog-work.toml` — picker/no-op block calls the no-op script.
- `orders/rex-backlog.toml` — order wiring for the no-op close and stamp.
- `orders/rex-backlog-blocked.toml` — new 1h cooldown exec order.
- `assets/scripts/rex-backlog-blocked.sh` — blocked-board detector + mail.
- `assets/scripts/rex-backlog-noop.sh` — no-op close, stamp, mail, state clear.
- `Makefile` — `make test` runs the suites.
- `tests/rex-backlog-ready.test.sh`, `tests/rex-backlog-noop.test.sh`,
  `tests/rex-backlog-blocked.test.sh` — 27 checks.

Working-tree edit made in this stage (uncommitted, as instructed):

- `orders/rex-backlog.toml` line 6 — "no noon backoff stamp" → "no no-op
  backoff stamp". `grep -n noon orders/rex-backlog.toml` is empty; the diff
  still shows the operator's `enabled = false`.

## Verification

Implementation convoy: `gcrex-0sa1`. Source anchors: `gcrex-hvlf`, `gcrex-72t8`.
Per-item summary paths: none recorded (items still open, no summaries written).

- First check: suites discovered via
  `ls ~/my-city/tests/*.test.sh` (ready, noop, blocked).
- `make -C ~/my-city test` → all suites pass: blocked 7/7, noop 10/10,
  ready 10/10 (27 checks, 0 failed).
- Red/green proof for REQ-001: running `tests/rex-backlog-ready.test.sh` with
  the pre-fix gate from `git show HEAD~1:scripts/rex-backlog-ready.sh` fails
  `blocked labeled To Do (todo=1, ready=0) does not fire (want exit 1, got 0)`
  and 6 passed / 4 failed; with the fixed gate all 10 pass. The working tree
  was restored afterwards (`git diff --stat scripts/rex-backlog-ready.sh` empty).
- Typo proof: `grep -n noon orders/rex-backlog.toml` exits 1 (no matches);
  `git diff orders/rex-backlog.toml` shows the comment hunk plus the operator's
  `enabled = false`.

## Remaining Risks

- `orders/rex-backlog.toml` still has `enabled = false` (operator's uncommitted
  change). The fix is inert until the operator re-enables the order — an
  operator decision, recorded in requirements Open Questions.
- GCREX-59 remains blocked by GCREX-56 until someone labels GCREX-56
  `for-gascity` or re-scopes the dependency; the new alert will report it, which
  is the intended behavior. Recorded follow-up, out of scope here.
- The one-hour no-op backoff and blocked-alert cooldown are fixed defaults,
  env-overridable only for tests; whether a different interval is wanted is an
  operator call.
- gc's own order-suppression only logs (`orderOpenWorkSuppressionAlertAfter = 20`,
  gascity v1.5.1 `cmd/gc/order_dispatch.go:2851`); that behavior is upstream and
  out of scope.

## Coverage

| ID | Status |
| --- | --- |
| REQ-001 | covered |
| REQ-002 | covered |
| REQ-003 | covered |
| REQ-004 | covered |
| gcrex-hvlf | deferred |
| gcrex-72t8 | covered |
