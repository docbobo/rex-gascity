---
schema: gc.build.implementation-summary.v1
workflow: {id: gcrex-pl1zq, formula: do-work}
methodology: {pack: gascity, name: build-basic}
producer: {formula: do-work, stage: implement, attempt: 1}
status: approved
trace:
  upstream:
    - path: beads/gcrex-62yf
      hash: bead:gcrex-62yf
    - path: plans/gcrex-62yf/review.md
      hash: sha256:792d36204231c8aac1f110d5b156cc286b1f258681dbe1ea3b4876e17be8c455
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
  coverage:
    - {id: GCREX-50-AC1, status: covered}
    - {id: GCREX-50-AC2, status: covered}
    - {id: GCREX-50-AC3, status: covered}
    - {id: RR2-R1, status: covered}
    - {id: RR2-R2, status: covered}
    - {id: RR2-R3, status: covered}
    - {id: RR2-R4, status: covered}
---

# Implementation summary — GCREX-50, review round 2

Source anchor `gcrex-62yf`. Worktree
`/Volumes/Sources/rex-gas-city/worktrees/gcrex-62yf`, branch
`gcrex-62yf-gcrex-50-stop-verify-descendant-reaping`. Commit `baefd74`
("GCREX-50: address review round 2 on the descendant reap"), parent `a51dcdf`,
against `origin/main` `a7d96f0`. Review input: `plans/gcrex-62yf/review.md`
(`changes_required`, R1–R4). No overlay file is touched, so the overlay-approval
rule does not apply.

## Summary

The round-2 review confirmed every round-1 finding fixed and left one blocking
correctness bug plus three lower-severity items. `baefd74` fixes them:
`signalAll` no longer abandons the rest of the tree at the first signal error, a
pane that vanishes between `Resolve` and `BlockProcess` is treated as the
no-pane case instead of a failed snapshot, the leaf-foreground scope is pinned
by a test, and `stop.md` records the unverified `Name` vs `ps comm` matching
claim. R2's fix also removes a needless `ps` fork when there is nothing to reap.

## Intended Behavior

- A reap pass delivers TERM (and later KILL) to every captured process even
  when one signal fails; the first failure is still reported, so the survivor
  error names the cause as well as the survivors.
- `stop` of a pane that disappears between resolve and the snapshot behaves
  like the no-pane case: it closes (tolerating `rex.ErrNotFound`), clears the
  incarnation, and succeeds; an unreadable table still fails before the close.
- The snapshot scope is the foreground and its descendants. A foreground that
  is a leaf therefore captures only that process; the agent's other children
  are outside the guarantee, which the parity row now states.
- No behavioral change for a healthy stop.

## Changed Files

- `internal/provider/reap.go`: `errors` import and `rex`; `paneSubtree`
  returns an empty tree on `rex.ErrNotFound` (`reap.go:126-135`); `signalAll`
  keeps looping and returns the first error (`reap.go:211-224`); `reap`
  short-circuits an empty tree so no `ps` fork is issued
  (`reap.go:271-277`).
- `internal/provider/reap_internal_test.go`: adds
  `TestStopKeepsSignallingAfterASignalFailure` (R1),
  `TestStopTreatsAVanishedPaneAsAnEmptyTree` (R2) and
  `TestPaneSubtreePinsAForegroundLeaf` (R3).
- `docs/provider-method-audit/stop.md`: match entry updated (line cites
  `reap.go:126,211,271`, `splitProcessRow` `reap.go:101`), notes the
  non-aborting signal pass, the vanished-pane case and the leaf scope; new
  Unverified bullet for `Name` vs `ps comm` matching.
- `docs/provider-herdr-parity.md`: survivor-handling row gains the
  non-aborting signal pass and the leaf-foreground sentence with its tests.

## Verification

First verification command (red proof, run against the pre-change tip
`a51dcdf`):

```
go test ./internal/provider/ -run 'TestStopKeepsSignallingAfterASignalFailure|TestStopTreatsAVanishedPaneAsAnEmptyTree|TestPaneSubtreePinsAForegroundLeaf' -count=1
```

Observed before the fix: FAIL —
`signals = [], want [terminated:400 killed:400] despite the 300 failure` and
`paneSubtree = [], rex: not found, want an empty tree`.
`TestPaneSubtreePinsAForegroundLeaf` passed before and after (it pins existing
behavior, per the review's R3 wording).

Final proof command (after the fix, at `baefd74`):

```
go build ./... && go vet ./internal/provider/ && go test -count=1 ./... && golangci-lint run ./...
```

Observed: build and vet clean; all packages pass
(`cmd/gc-runtime-rex`, `internal/launch`, `internal/provider`,
`internal/rex/cli`); `golangci-lint` 0 issues.

## Remaining Risks

- R3 is accepted, not fixed: a leaf foreground narrows the snapshot, so the
  agent's other children are outside the guarantee. Documented and pinned.
- R4 is unverified: whether Rex's `Name` matches `ps -o comm=` on the host is
  not established without a live Rex run; a mismatch turns a live process into
  a false success. Recorded in `docs/provider-method-audit/stop.md` Unverified.
- The reap still runs under the city-wide state lock (accepted in round 1,
  bounded at ~4 s plus `ps` calls).

## Coverage

| ID | Status |
| --- | --- |
| GCREX-50-AC1 | covered |
| GCREX-50-AC2 | covered |
| GCREX-50-AC3 | covered |
| RR2-R1 | covered |
| RR2-R2 | covered |
| RR2-R3 | covered |
| RR2-R4 | covered |
