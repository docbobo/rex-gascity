---
schema: gc.build.implementation-summary.v1
workflow: {id: gcrex-xzp1b, formula: do-work}
methodology: {pack: gascity, name: build-basic}
producer: {formula: do-work, stage: implement, attempt: 1}
status: approved
trace:
  upstream:
    - path: beads/gcrex-p1sd
      hash: bead:gcrex-p1sd
      ids: [REQ-001, REQ-002, REQ-003, REQ-004]
  coverage:
    - {id: REQ-001, status: covered}
    - {id: REQ-002, status: covered}
    - {id: REQ-003, status: covered}
    - {id: REQ-004, status: covered}
---

# Implementation Summary: GCREX-70 — declare `input-outcome`

## Summary

The provider handshake now declares `carrier.ops` and `input-outcome` alongside
`report-attachment`, `report-activity` and `idle-boundary`:

`{"version":0,"capabilities":["report-attachment","report-activity","idle-boundary","carrier.ops","input-outcome"]}`

(`cmd/gc-runtime-rex/main.go:50`). That is the full precondition gc reads before
it treats a driving op as an `input-outcome` op: `carrier.ops` makes gc run
`nudge`, `peek`, `send-keys`, `interrupt` and `clear-scrollback` as provider
ops instead of exec, and `input-outcome` makes it read their exit 80 and its
single outcome line (`internal/runtime/exec/outcome.go`, `outcomesRead` and
`inputOutcomeOp`). gc projects the words to `runtime.ErrSessionBusy` /
`runtime.ErrNudgeRefusedPendingInteraction` (`internal/runtime/protocol.go`,
`InputOutcome.Sentinel`), which is the classification the nudge backstop needs.

The decision and its evidence are in `docs/REX-CONTRACT.md` §Outcome
vocabulary and `docs/provider-herdr-parity.md` §Failures: the capability path is
read each time, not just the sentinel projection. The install-time caveat is
restated against the pinned builds: gc 1.5.1 ignores both tokens and reads exit
80 as a plain error (the pre-GCREX-70 behaviour); the graph.v2 pin
`e7d4e2354` also predates `internal/runtime/exec/outcome.go`; the installed
`edge-20261118` build has the carrier machinery.

With the declaration in place the best-effort wrapper is gone:
`bestEffort` and the `inputOutcomeDeclared` seam were deleted, `opInterrupt`
returns `p.Interrupt` directly, `opNudge` returns the typed outcome, and
`opSendKeys` returns the provider error. A missing or dead pane is now a typed
`gone` for all four driving ops through the provider's `ErrSessionNotFound`
classification in `runWithContext`.

## Intended Behavior

- The handshake declares five tokens; nothing else changes in the reply shape.
- `nudge`, `send-keys`, `interrupt` and `clear-scrollback` answer a missing or
  dead pane with exit 80 and one `{"outcome":"gone","detail":...}` line; the
  existing words (`busy`, `blocked`, `rejected`, `not_sent`, `unavailable`,
  `unconfirmed`, `delivered_unobserved`) keep their exit codes and payloads.
- `send-keys` with zero keys remains a no-op (exit 0, no lookup), since zero
  keys is no input.
- `clear-scrollback` keeps its provider-typed `gone` (`ClearScrollback` already
  returned an `*InputOutcomeError`); its other failure words remain exit 1.
- gc 1.5.1 (no carrier) sees the same exit 80 as an ordinary failure, so the
  declaration does not change behavior on that baseline.

## Changed Files

- `cmd/gc-runtime-rex/main.go` — handshake tokens, `inputOutcomeOps` map and
  `ErrSessionNotFound` → `gone` classification in `runWithContext`; removed
  `bestEffort` and `inputOutcomeDeclared`; `opInterrupt`/`opNudge`/`opSendKeys`
  return provider errors directly.
- `cmd/gc-runtime-rex/dispatch_test.go` — `TestProtocolHandshake` pins the five
  tokens; `TestInputOpsOnMissingSessionAnswerGone` and
  `TestSendKeysGoneExits80` pin exit 80 `gone` for the driving ops;
  `TestInterruptReturnsInputOutcomes` now expects exit 80 `gone` for missing and
  dead panes; the seam-based `TestSendKeysOutcomeDeclaredGoneExits80` is
  replaced by `TestSendKeysGoneExits80`.
- `docs/REX-CONTRACT.md` — handshake capabilities, the
  `GC_EXEC_DEADLINE_MS` caveat, and the outcome-vocabulary decision restated
  against the pinned builds.
- `docs/provider-herdr-parity.md` — protocol and failure verdicts updated.
- `docs/provider-herdr-method-deviations.md`,
  `docs/provider-method-audit/{interrupt,nudge,protocol,send-keys}.md` — stale
  best-effort and two-token claims corrected.

## Verification

First verification command (red before the declaration, green after):

```
go test -count=1 -run 'TestInputOpsOnMissingSessionAnswerGone|TestSendKeysGoneExits80|TestInterruptReturnsInputOutcomes' ./cmd/gc-runtime-rex
```

Observed: fail before the handshake declared the tokens (the ops still returned
exit 0 through the seam/wrapper), pass after.

Final proof commands, all run in the worktree:

```
make test
PATH="$PWD/bin:$PATH" GC_REX_CITY=rex-verify gc runtime check rex --city testdata/city
PATH="$PWD/bin:$PATH" GC_REX_CITY=rex-verify gc runtime conformance rex --city testdata/city
```

Observed: `go test ./...` all packages pass (uncached), `go vet ./...` clean;
`gc runtime check` 18/18 checks pass, including `capability carrier.ops:
interrupt: typed outcome gone` and `capability input-outcome: nudge to an
absent name: exit 80 gone`; conformance passes every input requirement it runs
(RPP-INPUT-001/002/003/004/008/010/012), with RPP-INPUT-005/006/007/009/011
skipped because we do not declare `conformance-faults`, and a pre-existing
unrelated RPP-OBSERVE-017 failure (missing `observe.v1`) that keeps the suite at
27/67.

gc-side classification proof: `TestInputOutcomeProjectionMatchesLaneAndQueue`
(`cmd/gc/input_outcome_projection_test.go:121` in the gc source tree) passes for
every word including `busy` and `pending_interaction`; the provider-side
`TestNudgeBusyIsBusyOutcome` (`internal/provider/nudge_test.go:438`) pins our
`busy` word. The test file does not exist at pin `e7d4e2354` (it was added
2026-11-13, commit `544beb8d6`), and the installed `edge-20261118` binary's
conformance run covers the exit-80/one-line contract above.

Coverage:

| ID | Status |
| --- | --- |
| REQ-001 | covered |
| REQ-002 | covered |
| REQ-003 | covered |
| REQ-004 | covered |

REQ-001 = AC #1 (decision recorded against the pinned gc and the capability
path), REQ-002 = AC #2 (handshake adds `input-outcome` with `carrier.ops`, and
the exit-80 ops keep their classification), REQ-003 = AC #3 (named tests prove
the refusal class), REQ-004 = AC #4 (contract and parity docs restated).

## Remaining Risks

- `busy`/`blocked` live probes are gated behind `conformance-faults`, which we
  do not declare; their classification is pinned by gc's lane/queue test and
  our provider test, not by an end-to-end conformance run.
- gc 1.5.1 reads exit 80 as a plain error, so on that baseline a refusal is
  still an untyped failure (unchanged, pre-existing).
- gc pin `e7d4e2354` predates the carrier machinery; the installed
  `edge-20261118` build is where the declaration takes effect.
- The pre-existing RPP-OBSERVE-017 conformance failure (missing `observe.v1`)
  is untouched by this change.
