---
schema: gc.build.final-report.v1
workflow: {id: gcrex-140, formula: build-basic}
methodology: {pack: gascity, name: build-basic}
producer: {formula: build-basic, stage: finalize, attempt: 3}
status: approved
trace:
  upstream:
    - path: plans/GCREX-60/requirements.md
      hash: sha256:2b86050135ee63041421b3a6d61a78601d389c1ef3200c6fe7cdc609dadc4130
      ids: [REQ-001, REQ-002, REQ-003, REQ-004]
    - path: plans/GCREX-60/plan-review.md
      hash: sha256:85ac0c9e5c20d7bce5e2ff5f61d56574e5cea236150b6cda7d4628524271a627
    - path: plans/GCREX-60/implementation-plan.md
      hash: sha256:325c9d29870f46d9567a8215a9df62cc5d941b741ea847aa635c8451db15a20c
    - path: plans/GCREX-60/decomposition.md
      hash: sha256:dfd81bb6bbd7b6a7a32755ad8e2dfd341681d52aa69143c877ced4ec04a6c7a0
    - path: beads/gcrex-0sa1
      hash: bead:gcrex-0sa1
    - path: plans/GCREX-60/implementation-summary.md
      hash: sha256:8fbf2f92e863e01eeb10dfe511ce88dcdcffa7abbd30304f542fe09890be4aa1
    - path: plans/GCREX-60/review-report.md
      hash: sha256:87a8680e63fbbf3dde0819c2ea0fccb64ce979c232912b490d842ca23a185f96
      ids: [gcrex-hvlf, gcrex-72t8]
  coverage:
    - {id: REQ-001, status: covered}
    - {id: REQ-002, status: covered}
    - {id: REQ-003, status: covered}
    - {id: REQ-004, status: covered}
    - {id: gcrex-hvlf, status: covered}
    - {id: gcrex-72t8, status: covered}
---

# Factory Run: GCREX-60 — rex-backlog order fired for a day on a blocked task

Methodology: build-basic starter factory (`gascity/build-basic`).

## Summary

The rex-backlog order fired 46 times against a labeled `To Do` task it could
never claim (GCREX-59, blocked by GCREX-56): the gate matched any `for-gascity`
`To Do` task while the picker selected only `--ready` ones, so every run booted
pi plus the implementation worker/reviewer, closed "No backlog task claimed", and
left the board unchanged.

The fix (in `/Users/boris/my-city` at `4c1318c`, plus the review-fix worktree
changes) makes the gate ask the picker's `--ready` predicate and fail closed,
adds a one-hour no-op backoff with a single deduplicated mail, and adds a
`rex-backlog-blocked` order that alerts the human when labeled work exists but
none is ready and none is in progress. `make test` runs the suites.

| Requirement | Result |
| --- | --- |
| REQ-001 gate uses the picker's ready predicate, fails closed | covered |
| REQ-002 no-op dispatch backs off 1h and mails once per episode | covered |
| REQ-003 blocked board is reported to the human, once per episode | covered |
| REQ-004 `make test` runs every suite | covered |

## Outcome

**approved** at the implementation source anchor/worktree `/Users/boris/my-city`
(HEAD `4c1318c86d80aadd96dcb4e85f25c0d85ee9c178` plus the uncommitted RF-1/RF-2
review fixes). The final review (`plans/GCREX-60/review-report.md`) is
`status: approved`, verdict approved: iteration-1 findings RF-1 (failed mail send
looked like success), RF-2 (one mail per blocked episode), and ME-1 (production
send args untested) were all fixed and re-verified.

The launcher rig root `/Volumes/Sources/rex-gas-city` still holds the original
fixture and no implementation files; that is a publish-stage concern, not a
partial build.

Review lanes that ran: acceptance, simplicity, test evidence — recorded in
`plans/GCREX-60/acceptance-review.md`, `plans/GCREX-60/simplicity-review.md`,
`plans/GCREX-60/test-evidence-review.md`, synthesized in
`plans/GCREX-60/starter-review-synthesis.md` and fixed in
`plans/GCREX-60/review-fix-summary.md`.

Implementation convoy: `gcrex-0sa1` (items `gcrex-hvlf`, `gcrex-72t8`). Its
convoy item summaries were never written (the convoy stayed open and an upstream
drain step closed with `predecessor_failed`), so verification was performed
directly in the build stages against the `~/my-city` worktree.

Publish: no-op. `open_pr=false` and `push=false` in the workflow vars; this step
does not publish. The fix lives in `/Users/boris/my-city` as commit `4c1318c`
plus uncommitted review-fix changes.

## Artifacts

- Requirements: `plans/GCREX-60/requirements.md`
- Plan review: `plans/GCREX-60/plan-review.md`
- Implementation plan: `plans/GCREX-60/implementation-plan.md`
- Decomposition: `plans/GCREX-60/decomposition.md`
- Implementation summary: `plans/GCREX-60/implementation-summary.md`
- Review context: `plans/GCREX-60/build-basic-review-context.md`
- Review report: `plans/GCREX-60/review-report.md`
- Review lanes: `plans/GCREX-60/acceptance-review.md`,
  `plans/GCREX-60/simplicity-review.md`,
  `plans/GCREX-60/test-evidence-review.md`,
  `plans/GCREX-60/starter-review-synthesis.md`,
  `plans/GCREX-60/review-fix-summary.md`
- Implementation convoy: `gcrex-0sa1`
  (`gc.build.implementation_convoy_id` on the workflow root)
- Proof commands: `python3 plans/GCREX-60/acceptance-probes.py` → exit 0
  (mail-failure visibility, episode reset, same-episode archive); old-gate replay
  8 passed / 2 failed vs current gate 10/10.
  `bash plans/GCREX-60/simplicity-mail-failure.check.sh` → exit 0.
  `make test` in `/Users/boris/my-city` → 40 checks, 0 failed (blocked 14/14,
  no-op 16/16, ready 10/10).

## Remaining Risks

- `orders/rex-backlog.toml` still has the operator's uncommitted
  `enabled = false`; the fix is inert until the order is re-enabled. Operator
  decision.
- GCREX-59 stays blocked by GCREX-56 until GCREX-56 is labeled `for-gascity` or
  the dependency is re-scoped; the new alert reports it (intended behavior).
- The 1h no-op backoff and blocked-alert cooldown are fixed defaults
  (env-overridable only in tests); a different interval is an operator call.
- RR-1: the `REX_BACKLOG_MAIL_LOG` test seam remains; RR-2: deployed gc dedup
  behavior verified only against pinned gc `v1.5.1`. Both non-blocking.

## Next Human Action

1. Review the fix in `/Users/boris/my-city` (commit `4c1318c` plus the
   uncommitted review-fix diff) and commit it.
2. Decide whether to re-enable `orders/rex-backlog.toml`.
3. Label GCREX-56 `for-gascity` or re-scope GCREX-59 so the backlog can advance.

## Coverage

| ID | Status |
| --- | --- |
| REQ-001 | covered |
| REQ-002 | covered |
| REQ-003 | covered |
| REQ-004 | covered |
| gcrex-hvlf | covered |
| gcrex-72t8 | covered |
