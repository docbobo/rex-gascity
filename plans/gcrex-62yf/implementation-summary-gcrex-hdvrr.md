---
schema: gc.build.implementation-summary.v1
workflow: {id: gcrex-crffw, formula: do-work}
methodology: {pack: gascity, name: build-basic}
producer: {formula: do-work, stage: implement, attempt: 1}
status: approved
trace:
  upstream:
    - path: beads/gcrex-62yf
      hash: bead:gcrex-62yf
    - path: plans/gcrex-62yf/review.md
      hash: sha256:0692cc10b7a9a4b25d893f5a4d731fa058761a445ba947d4747b6580c3f76c75
    - path: internal/provider/provider.go
      hash: sha256:bec9b3edd74d38922694e118fcdcc79c1944df535dc4de928ee505e393bdce2d
    - path: internal/provider/reap.go
      hash: sha256:86fc8925bff35981289304f3299985a09f3155b40d117d4f7b4655c04ff91070
    - path: internal/provider/reap_internal_test.go
      hash: sha256:816e0f2d8ee260cf37031c9bb1883a43e9e3b03e3ce3adcfe52ff5f17e5731e7
    - path: docs/provider-herdr-parity.md
      hash: sha256:38b00f90973486b46c923643fb2acc3b3752a96b69b26161b2de17e5833f48c1
    - path: docs/provider-herdr-method-deviations.md
      hash: sha256:9e048bdc321c607eaf72837868bc7369544841ad82b16ccf8fd53a40e72ef070
    - path: docs/provider-method-audit/stop.md
      hash: sha256:deff32d129daf5dcf1fddf7bc0fde56d68b73e47465ab439899107a05877226d
  coverage:
    - {id: GCREX-50-AC1, status: covered}
    - {id: GCREX-50-AC2, status: covered}
    - {id: GCREX-50-AC3, status: covered}
    - {id: R3-D1, status: covered}
    - {id: R3-D2, status: covered}
    - {id: R3-D3, status: covered}
    - {id: R3-D4, status: covered}
---

# Implementation summary: GCREX-50 review round 3

## Summary

Applied the owner decision on the round-3 review in the worktree
`/Volumes/Sources/rex-gas-city/worktrees/gcrex-62yf`, commit `3ef3385`
("GCREX-50: re-read the pane after close, tighten reap scope docs") on top of
`baefd74`.

D1 (option a) is implemented: `Provider.stop` keeps the pre-close snapshot,
then reads the pane's process tree a second time after `block.close` and unions
what the pane still reports into the reap set (`unionProcs`), so a process the
pane started between the snapshot and the close is signalled and fails the stop
instead of passing as success. `rex.ErrNotFound` (the normal no-pane case after
a close) and any other re-read error are tolerated: the pre-close snapshot
stays authoritative and a failed re-read never fails the stop.

D2 rewrote the locking-scope claims with the real bound (reap runs under both
locks, externally bounded by the op's 30 s SIGTERM, `reap` honours `ctx` only
in `waitGone`), the upgrade path (snapshot under the locks, release the state
lock before reaping) and the upstream comparison. D3 deleted the redundant
`TestStopReportsTheSignalError`. D4 counted the second `BlockProcess` read in
the stop deadline row. U1 (`Name` vs `ps comm`) remains recorded as Unverified
in `docs/provider-method-audit/stop.md`; no code change is justified without a
live run.

## Intended Behavior

- `stop` performs two pane reads on a live-pane path: one before
  `block.close` (so a detached descendant is still attributable) and one
  after it. The second read only ever adds pids; the union is by pid.
- A process the pane reports only after the close gets TERM, then KILL, and
  the stop exits 1 naming it while keeping binding, meta and activity.
- A closed pane answering `ErrNotFound`, or any failed second read, leaves the
  pre-close snapshot in charge; the stop proceeds as before.
- The reap's locking scope and deadline accounting are documented as they are,
  including that Rex is the only provider that re-checks after the close.

## Changed Files

- `internal/provider/provider.go` — second `paneSubtree` read after
  `CloseBlock`, unioned into the reap tree; comment for the discarded signal
  error when nothing survives.
- `internal/provider/reap.go` — `unionProcs` helper.
- `internal/provider/reap_internal_test.go` — new
  `TestStopReapsAProcessThePaneStartedAfterTheSnapshot`; deleted
  `TestStopReportsTheSignalError`.
- `docs/provider-herdr-parity.md` — stop rows "wait for exit / tree kill",
  "survivor handling" and "locking scope", numbered deviation 5.
- `docs/provider-herdr-method-deviations.md` — stop deadline row (two
  `BlockProcess` round-trips) and descendant-reaping row (post-close re-read).
- `docs/provider-method-audit/stop.md` — descendant-reaping bullet records the
  post-close re-read and its test.

## Verification

First (red) verification, before the fix, at `baefd74` + new test:

```sh
go test ./internal/provider/ -run TestStopReapsAProcessThePaneStartedAfterTheSnapshot -count=1
```

Observed: `--- FAIL: TestStopReapsAProcessThePaneStartedAfterTheSnapshot`
(`Stop = nil, want a survivor failure for the process started after the
snapshot`). Fail.

Final proof after the fix, in the worktree at `3ef3385`:

```sh
go build ./... && go vet ./internal/provider/ && go test ./... && golangci-lint run ./internal/provider/
```

Observed: build and vet clean; `go test ./...` all packages `ok` (provider
6.6 s); `golangci-lint` `0 issues.` Pass. The full suite and `golangci-lint`
were also run before the commit and the pre-commit hook passed.

## Remaining Risks

- The second read is one more Rex round-trip on the stop critical path; it is
  now counted in the deadline row (D4).
- A process that leaves the pane's reported tree between the snapshot and the
  close, and is reparented away before the re-read, is still outside the
  guarantee (documented in the parity row as a snapshot, not a token sweep).
- U1: `living` still treats a `Name`/`ps comm` mismatch as gone; unverified
  against a live Rex, recorded as Unverified in `stop.md`.
- `ps -axo pid=,ppid=,stat=,comm=` portability across macOS and Linux remains
  untested by a live run (standing note from the review).

## Coverage

| ID | Status |
| --- | --- |
| GCREX-50-AC1 | covered |
| GCREX-50-AC2 | covered |
| GCREX-50-AC3 | covered |
| R3-D1 | covered |
| R3-D2 | covered |
| R3-D3 | covered |
| R3-D4 | covered |
