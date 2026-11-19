# Test Evidence Review: GCREX-60 rex-backlog blocked-task loop

Lane: `review.test-evidence-review` (bead `gcrex-jve`)
Date: 2026-11-19

## Verdict

**approve** — the accepted tasks recorded the required evidence and every proof
command actually covers the acceptance criteria it claims. No missing proof; no
product defects surfaced by the evidence.

## Scope and authority

Implementation worktree (per the review context's `## Implementation Worktrees`
section): `/Users/boris/my-city` (HEAD `4c1318c86d80aadd96dcb4e85f25c0d85ee9c178`).
All proof commands below were run from that worktree (`pwd -P` =
`/Users/boris/my-city`). The launcher root `/Volumes/Sources/rex-gas-city` was
not used to run evidence commands.

## Evidence recorded per accepted task

| Field | Recorded in | Present |
| --- | --- | --- |
| Intended behavior | implementation-summary.md "Intended Behavior" | yes (REQ-001 gate, REQ-002 no-op, REQ-003 blocked alert, REQ-004 verify) |
| First verification command | implementation-summary.md "Verification" → `ls ~/my-city/tests/*.test.sh` | yes |
| Proof command | implementation-summary.md "Verification" → `make -C ~/my-city test`, red/green gate, typo grep/diff | yes |
| Changed files | implementation-summary.md "Changed Files" (10 committed + 1 working-tree) | yes |
| Remaining risks | implementation-summary.md "Remaining Risks" | yes (4 risks) |

The one verification item (`gcrex-hvlf`) is marked `deferred` in the summary's
coverage because its convoy item stayed open with no per-item summary; the
evidence was instead recorded directly in the implementation summary. The
evidence itself exists and is reproducible, so this is a coverage-bookkeeping
gap, not missing proof. The optional typo item (`gcrex-72t8`) is `covered`.

## Acceptance-criteria coverage (commands actually cover the claims)

- **AC-1 (shared predicate, red/green):** `tests/rex-backlog-ready.test.sh`
  hardcodes `GATE="$SCRIPT_DIR/../scripts/rex-backlog-ready.sh"`. Re-running it
  against the pre-fix gate (`git show 4c1318c~1:scripts/rex-backlog-ready.sh`,
  staged in a temp `scripts/` + `tests/` tree) is RED: the named case fails
  (`FAIL - blocked labeled To Do (todo=1, ready=0) does not fire (want exit 1, got 0)`),
  and the run ends `6 passed, 4 failed`. Against HEAD's gate the same suite is
  GREEN (`10 passed, 0 failed`). The acceptance criterion is satisfied.
- **AC-2 (no-op backoff + one mail):** `tests/rex-backlog-noop.test.sh` covers the
  stamp, the epoch shape, the state clear, the dedup mail, the bead close, and the
  no-`GC_BEAD_ID` guard (10 checks). The ready suite covers both backoff cases
  ("fresh no-op stamp suppresses the next poll", "no-op stamp older than
  NOOP_INTERVAL fires again").
- **AC-3 (blocked-board alert; ready/busy mails nothing):** `tests/rex-backlog-blocked.test.sh`
  covers blocked-board mail, mail naming the task, ready board → no mail, no
  labeled work → no mail, in-progress → no mail, and missing rig → fails closed
  (7 checks).
- **AC-4 (`make test`):** reproduced below, 27 checks / 0 failed.

## Proof reproduced (this review)

`make -C /Users/boris/my-city test`:

- `tests/rex-backlog-blocked.test.sh`: 7 passed, 0 failed
- `tests/rex-backlog-noop.test.sh`: 10 passed, 0 failed
- `tests/rex-backlog-ready.test.sh`: 10 passed, 0 failed

Total 27 checks, 0 failed.

Red/green (REQ-001): pre-fix gate → `6 passed, 4 failed` (regression case fails);
HEAD gate → `10 passed, 0 failed`.

Typo (gcrex-72t8): `grep -n noon orders/rex-backlog.toml` exits 1 (no matches).
`git diff orders/rex-backlog.toml` shows the comment hunk ("noon" → "no-op") plus
the operator's pre-existing `enabled = true` → `enabled = false` hunk, left
untouched as instructed.

## Findings

- `info` — coverage bookkeeping: convoy item `gcrex-hvlf` is `deferred` in the
  summary because it stayed open with no per-item summary, but its REQ-001..004
  evidence was performed and recorded directly in the implementation summary.
  The evidence is present and reproducible; no fix-lane action required. Not a
  product defect.
- No missing proof. No product defect surfaced.

## Distinction: missing proof vs product defect

None of the acceptance criteria lack a working proof command. There is no
evidence of a product defect — all 27 checks pass and the regression test is
red-on-old / green-on-new as required.
