---
schema: gc.build.implementation-summary.v1
workflow: {id: gcrex-lnsaz, formula: do-work}
methodology: {pack: gascity, name: build-basic}
producer: {formula: do-work, stage: implement, attempt: 1}
status: approved
trace:
  upstream:
    - path: beads/gcrex-62yf
      hash: bead:gcrex-62yf
    - path: plans/gcrex-62yf/review.md
      hash: sha256:review-round-1
    - path: internal/provider/reap.go
      hash: sha256:f4f495e5a79c1313081eb0a97a409c94c478da1b3239af945fd166cb33c567c2
    - path: internal/provider/provider.go
      hash: sha256:fdc9eb965f9ef5828fac564a2a143d6db5133fc8caa0ed271654bd759d44b120
    - path: internal/provider/reap_internal_test.go
      hash: sha256:e13c2ba71f038c384dbe8acc896b24fad98ece97908d2d60f0a0de329428fe40
    - path: docs/provider-herdr-parity.md
      hash: sha256:0ab4165fe811fc1a04fe82d01225905e80b7cf74669e253519e8da9b7a187840
    - path: docs/provider-method-audit/stop.md
      hash: sha256:5f1f32d1b41ef45d75c6b778c2996403132af5c694cc451583ed9418d208e9c2
    - path: docs/provider-herdr-method-deviations.md
      hash: sha256:01673cb678c5da449a0186a1d703ee02def70a1447ce63d53a938370e885712c
  coverage:
    - {id: GCREX-50-AC1, status: covered}
    - {id: GCREX-50-AC2, status: covered}
    - {id: GCREX-50-AC3, status: covered}
    - {id: RF-F1, status: covered}
    - {id: RF-F2, status: covered}
    - {id: RF-F3, status: covered}
    - {id: RF-F4, status: covered}
    - {id: RF-F5, status: covered}
    - {id: RF-F6, status: covered}
    - {id: RF-F7, status: covered}
    - {id: RF-F8, status: covered}
---

# Implementation summary — GCREX-50, review round 1

Source anchor `gcrex-62yf`. Worktree
`/Volumes/Sources/rex-gas-city/worktrees/gcrex-62yf`, branch
`gcrex-62yf-gcrex-50-stop-verify-descendant-reaping`. Commit `a51dcdf`
("GCREX-50: address review round 1 on the descendant reap"), parent `c7d4f16`,
against `origin/main` `a7d96f0`. Review input: `plans/gcrex-62yf/review.md`
(`changes_required`, F1–F8). Fix narrative:
`plans/gcrex-62yf/review-fix-summary.md`.

## Summary

The round-1 review found one correctness gap in the process-table parser and
seven smaller issues in the reap's error reporting, snapshot ordering, subtree
attribution and documentation. All eight are addressed in `a51dcdf`: every
survivor is now parsed and reaped, a failed signal is named in the error, an
unreadable process tree fails the stop before the pane is closed, the subtree
walk is rooted at the foreground so an ancestor's sibling is not signalled, and
the parity/audit prose states exactly what the snapshot can promise.

## Intended Behavior

- `ps -axo pid=,ppid=,stat=,comm=` rows are parsed as three fixed columns plus
  the remainder of the line as `comm`; a row with an empty or space-containing
  command name is kept (RF-F1).
- A TERM/KILL signal failure is carried into `survivorError`, so the operator
  sees `operation not permitted` instead of only "descendants may survive"
  (RF-F3).
- A `BlockProcess` failure returns
  `could not inspect the pane's process tree, leaving it open: <err>` before
  `block.close`; the pane, binding, meta and activity stay for the retry
  (RF-F5).
- The subtree walk starts at the pane's reported foreground (falling back to
  the highest reported process when Rex reports none); reported ancestors are
  used for names only, so a sibling under the pane shell is not captured or
  signalled (RF-F4).
- The reap continues to run under the per-name and city-wide state flocks; the
  resulting few-second city-wide hold is accepted and documented with the
  release-early upgrade path (RF-F2).
- `stop` deviates from herdr-core only where `stop.md` says so; the deviations
  table now counts `stop` as 6 deviations, 0 medium, 6 low (RF-F6–F8).

## Changed Files

- `internal/provider/reap.go` — `splitProcessRow`; fixed-column row parsing;
  `signalAll` returns the first signal error; `reap` returns
  `([]paneProc, error)`; `paneSubtree` seeded from the foreground only.
- `internal/provider/provider.go` — `stop` fails before `block.close` when
  `paneSubtree` errors and passes the reap's signal error to `survivorError`.
- `internal/provider/reap_internal_test.go` — `TestProcessTableKeepsRowsWithAnEmptyOrSpacedComm`,
  `TestStopReportsTheSignalError`, `TestStopLeavesThePaneOpenWhenTheTreeCannotBeRead`,
  `TestPaneSubtreeSkipsAncestorsSiblings`.
- `docs/provider-herdr-parity.md` — stop rows and implementation paragraph:
  foreground-rooted snapshot, scoped success guarantee, accepted city-wide
  reap hold.
- `docs/provider-method-audit/stop.md` — reaping bullet cites `Provider.stop`
  and its call sites; Unverified regains the live-Rex row.
- `docs/provider-herdr-method-deviations.md` — `stop` row `6|0|6|0`, total
  `17` medium / `105` low, stop body rows updated, closed fixes removed from
  the tracked-task list.

No overlay path is touched, so no overlay approval is needed.

## Verification

First verification (fail-first, per TDD): each new test was run against the
pre-fix tree and failed for the stated reason —

- `TestProcessTableKeepsRowsWithAnEmptyOrSpacedComm`: survivor rows 300/400
  missing from the captured tree.
- `TestStopReportsTheSignalError`: `Stop` error lacked `operation not
  permitted`.
- `TestStopLeavesThePaneOpenWhenTheTreeCannotBeRead`: pane was closed and the
  error claimed survivors.
- `TestPaneSubtreeSkipsAncestorsSiblings`: captured pids included `200` and
  `500`.

Final proof, from `/Volumes/Sources/rex-gas-city/worktrees/gcrex-62yf`:

- `go build ./...` — pass.
- `go vet ./internal/provider/` — pass.
- `go test -count=1 ./internal/provider/` — `ok ... 6.769s`.
- `go test ./...` — every package `ok`.
- `golangci-lint run ./...` — `0 issues` (also run by the pre-commit hook).

Coverage:

| ID | Status |
| --- | --- |
| GCREX-50-AC1 | covered |
| GCREX-50-AC2 | covered |
| GCREX-50-AC3 | covered |
| RF-F1 | covered |
| RF-F2 | covered |
| RF-F3 | covered |
| RF-F4 | covered |
| RF-F5 | covered |
| RF-F6 | covered |
| RF-F7 | covered |
| RF-F8 | covered |

## Remaining Risks

- The fixes have not been re-reviewed; this is review iteration 1 of the rework.
- No live Rex run has exercised the reap: what `block.close` leaves behind and
  the unreadable-table path are verified from code and fakes only (recorded in
  `docs/provider-method-audit/stop.md` Unverified).
- `ps -axo` is verified on macOS only; Linux uses the same columns but was not
  run.
- A process that forks and is reparented between the snapshot and the walk is
  still outside the captured set; the docs now state that limit instead of
  claiming the whole tree.
