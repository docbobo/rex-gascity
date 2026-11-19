---
schema: gc.build.review.v1
workflow: {id: gcrex-140, formula: build-basic}
methodology: {pack: gascity, name: build-basic}
producer: {formula: build-basic-review, stage: review, attempt: 1}
status: approved
trace:
  upstream:
    - path: plans/GCREX-60/requirements.md
      hash: sha256:2b86050135ee63041421b3a6d61a78601d389c1ef3200c6fe7cdc609dadc4130
      ids: [REQ-001, REQ-002, REQ-003, REQ-004]
    - path: plans/GCREX-60/build-basic-review-context.md
      hash: sha256:2908bb20faf7742635044a947df9ef499c0bb0c58710f0555af1d573d76ed036
    - path: plans/GCREX-60/implementation-summary.md
      hash: sha256:8fbf2f92e863e01eeb10dfe511ce88dcdcffa7abbd30304f542fe09890be4aa1
    - path: beads/gcrex-hvlf
      hash: bead:gcrex-hvlf
    - path: beads/gcrex-72t8
      hash: bead:gcrex-72t8
  coverage:
    - {id: REQ-001, status: covered}
    - {id: REQ-002, status: covered}
    - {id: REQ-003, status: covered}
    - {id: REQ-004, status: covered}
    - {id: gcrex-hvlf, status: covered}
    - {id: gcrex-72t8, status: covered}
---

# Build Review: GCREX-60 rex-backlog blocked-task loop

Producer: `build-basic.review` attempt 1 (bead `gcrex-bkw4`), finalizing the
review loop `gcrex-tyev`.

## Verdict

**approved** at the implementation source anchor and worktree.

The iteration-1 synthesis reported `iterate` (RF-1, RF-2, ME-1). The fix lane
(`gcrex-73j`) applied all three to `/Users/boris/my-city` and recorded them in
`plans/GCREX-60/review-fix-summary.md`. This stage re-ran every proof command the
synthesis named against that worktree; all pass. The only remaining delta is
that `/Volumes/Sources/rex-gas-city` (launcher rig root) has no implementation
files, which the review context and the bead both classify as a publish-stage
concern, not a review blocker. No launcher root mutation was made.

## Findings

- RF-1 (failed send looked like success) — resolved. `|| true` removed from the
  `gc mail send` in both helpers
  (`/Users/boris/my-city/assets/scripts/rex-backlog-blocked.sh`,
  `/Users/boris/my-city/assets/scripts/rex-backlog-noop.sh`); `set -e`
  propagates the failure. No-op keeps its backoff stamp and does not reach the
  pass/no-op close on failure.
- RF-2 (one mail per blocked episode) — resolved. Episode marker
  `$STATE_DIR/rex-backlog.blocked` is written only after a successful send,
  cleared on every not-blocked exit, and the dedup key carries an episode token
  (`rex-backlog-blocked-<epoch>-<pid>`). Marker exists before send, so a
  still-blocked rerun after a prior delivery sends nothing.
- ME-1 (production send args untested) — resolved. Both suites gained
  fake-`gc`-on-`PATH` sections driving the real send branch: recipient `human`,
  dedup key shape, no resend in the same episode, new key in a new episode,
  failed send exits nonzero and is retried, and no-op's pass/no-op close still
  happens only on a successful send.
- Info — RR-1 (`REX_BACKLOG_MAIL_LOG` test seam still present) and RR-2
  (deployed gc dedup behavior unverified beyond pinned `v1.5.1`) remain
  non-blocking residual risks, as recorded in the fix summary.
- Info — RR-3 (`gcrex-hvlf` was `deferred` in the implementation summary
  because its convoy item stayed open) is bookkeeping only; this report covers
  it as `covered` because its evidence is reproduced below.

## Coverage

| ID | Status |
| --- | --- |
| REQ-001 | covered |
| REQ-002 | covered |
| REQ-003 | covered |
| REQ-004 | covered |
| gcrex-hvlf | covered |
| gcrex-72t8 | covered |

## Verification

Run from `/Users/boris/my-city` with `pwd -P` equal to that path (worktree HEAD
`4c1318c86d80aadd96dcb4e85f25c0d85ee9c178` plus the uncommitted RF-1/RF-2 fixes):

- `python3 /Volumes/Sources/rex-gas-city/plans/GCREX-60/acceptance-probes.py`
  → exit 0. `MAIL_FAILURE_noop: exit=1, closed_pass=False`,
  `MAIL_FAILURE_blocked: exit=1, closed_pass=False`,
  `EPISODE_RESET: deliveries=2, expected=2`,
  `ARCHIVE_SAME_EPISODE: deliveries=1` (expected 1). Old-gate red replay
  8 passed / 2 failed; current gate green 10/10.
- `bash /Volumes/Sources/rex-gas-city/plans/GCREX-60/simplicity-mail-failure.check.sh`
  → exit 0 (`injected mail transport failure`; `Blocked detector exit after
  injected send failure: 73`; `PASS: blocked alert delivery failure remains
  visible`).
- `make test` → blocked 14/14, no-op 16/16, ready 10/10; 40 checks, 0 failed.
- `grep -n noon orders/rex-backlog.toml` → no matches (typo fix `gcrex-72t8`);
  the operator's uncommitted `enabled = false` hunk is untouched.

Reviewed artifact paths: acceptance `plans/GCREX-60/acceptance-review.md`,
simplicity `plans/GCREX-60/simplicity-review.md`, test evidence
`plans/GCREX-60/test-evidence-review.md`, synthesis
`plans/GCREX-60/starter-review-synthesis.md`, fix summary
`plans/GCREX-60/review-fix-summary.md`.
