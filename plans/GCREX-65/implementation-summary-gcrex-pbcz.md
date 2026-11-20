---
schema: gc.build.implementation-summary.v1
workflow: {id: gcrex-fr6r, formula: do-work}
methodology: {pack: gascity, name: build-basic}
producer: {formula: do-work, stage: implement, attempt: 1}
status: approved
trace:
  upstream:
    - path: plans/GCREX-65/implementation-plan.md
      hash: sha256:234c8df5445b178963757973d2fbb2beaab25ba080e4c961b707ceab05dc26dc
    - path: beads/gcrex-pbcz
      hash: bead:gcrex-pbcz
      ids: [REQ-002, REQ-008]
  coverage:
    - {id: REQ-002, status: covered}
    - {id: REQ-008, status: covered}
---

# Implementation Summary: GCREX-65 W4 — docs, e2e assertion, GCREX-30 note

> **Historical, superseded:** The guarded destroy described below was removed
> in GCREX-65 attempt 3 (`f1bd04c`). `Stop` now leaves the city session in
> place even when empty, regardless of attached clients, and a later `Start`
> reuses it. See `docs/REX-CONTRACT.md` §12 and `TestStopCallsNoDestroy`.
> The behavior, tests and verification below record W4, not the current state.

## Summary

Plan step 7. D3 is restated as an ownership rule in `docs/REX-CONTRACT.md`
(`session.destroy` row, §10 probe wording, D3 row, §12 destroy bullet, GCREX-4
empty-tab bullet), `docs/rex-provider.md` and `docs/provider-herdr-parity.md`,
plus `README.md`, `docs/plans/native-rex-provider.md` and a historical note in
`docs/rex-pilot-phase5.md` (all carried the "never calls `session.destroy`"
claim). The e2e tests assert the new teardown. A GCREX-30 note records the
interim. W2 (`c5f8b1b`), W3 (`66132e0`, here `5ffe1b1`) and W1 (`917d73e`, here
`7ad55b1`) were cherry-picked into this worktree because it was cut from
`origin/main` and they are not on it; the docs cite code that exists only with
them.

## Intended Behavior at W4 (superseded)

- The docs state: never destroy what we did not create (shared server, other or
  legacy-labelled sessions, human panes, `server.stop`); do destroy our panes and
  the canonical `gc:<city>` session (created at
  `internal/provider/provider.go:488`) once it has zero windows and no
  `kind == "app"` client.
- Citation: the plan says `:487`; after W3 the `CreateSession` call is at `:488`
  in this tree, so the docs cite `:488`.
- `TestE2EAgentExitLiveness`: self-exit with no `Stop` still leaves the session
  listed (Decision 4); a later `Stop` destroys it unless an app client is
  attached (that branch is logged, not skipped).
- `TestE2EIdentityActivityAttachment` and `TestE2EParallelStarts` asserted
  "session survives Stop", which the behavior change falsifies. Both now use the
  new helper `e2eExpectSessionAfterStop`.
- Parity doc: cross-reference line only; the `server-stop` verdict
  (`intentional`) is unchanged and no gc claim was added.
- Follow-up (Decision 4, out of scope): a self-exited agent with no later `Stop`
  leaves the empty session until the next `Stop` of any name in that city.

## Changed Files

- `docs/REX-CONTRACT.md`, `docs/rex-provider.md`, `docs/provider-herdr-parity.md`,
  `README.md`, `docs/plans/native-rex-provider.md`, `docs/rex-pilot-phase5.md`
- `internal/provider/e2e_test.go` — `e2eExpectSessionAfterStop`; three e2e tests updated.
- `backlog/tasks/gcrex-30 - Investigate-...md` — GCREX-65 note.

## W4 Verification (historical)

- First verification command: `REX_E2E=1 go test ./internal/provider -run TestE2EAgentExitLiveness`
  with `provider.go` reverted to `2de7a6e` (no destroy): FAIL ("session
  gc:e2e-exit-... still listed after Stop; the empty owned session must be
  destroyed"). With the real `provider.go`: PASS. Before the helper refactor,
  `TestE2EIdentityActivityAttachment` and `TestE2EParallelStarts` failed on the old
  survive assertion; now PASS.
- `REX_E2E=1 go test ./internal/provider -run TestE2E -v` (live server): all 6 PASS;
  no scratch session left in `rex session ls`.
- `grep -rn "never calls \`session.destroy\`\|never \`session.destroy\`" docs/`
  returns nothing (exit 1).
- Final proof: `make test` PASS (all packages ok); `golangci-lint run ./...`: 0 issues.

## W4 Remaining Risks (historical)

- The e2e tests run only with `REX_E2E=1` against a live server. With a Rex app
  attached the destroy is skipped and the tests only log that branch.
- This branch carries cherry-picked W1/W2/W3 commits; merging it alongside the W1
  and W3 branches will see identical patches.
- `docs/rex-pilot-phase5.md` disposition text ("asserts the session survives a
  provider Stop") is historical and left as written.

| ID | Status |
| --- | --- |
| REQ-002 | covered |
| REQ-008 | covered |
