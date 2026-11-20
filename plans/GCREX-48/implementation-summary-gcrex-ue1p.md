---
schema: gc.build.implementation-summary.v1
workflow: {id: gcrex-m7p2, formula: do-work}
methodology: {pack: gascity, name: build-basic}
producer: {formula: do-work, stage: implement, attempt: 1}
status: approved
trace:
  upstream:
    - path: beads/gcrex-ue1p
      hash: bead:gcrex-ue1p
      ids: [REQ-004, REQ-005, REQ-006]
  coverage:
    - {id: REQ-004, status: covered}
    - {id: REQ-005, status: covered}
    - {id: REQ-006, status: covered}
---

# Implementation Summary: GCREX-48 WI-3 — parity/audit docs for peek exit 66

## Summary

Updated the peek parity and method-audit docs to match the exit-66 behavior
landed in WI-2. This worktree was cut from origin/main, which lacks WI-1 and
WI-2 (they landed in sibling worktrees as unpushed detached commits), so
`e7747c4` (decision-2) and `b640965` (code and tests) were cherry-picked first
(`68bff5f`, `04e6ce4`). The docs commit is `f23b4d9`.

## Intended Behavior

- `docs/provider-herdr-parity.md` `### peek`: "Our implementation" paragraph
  describes exit 66 for no binding, `ErrNotFound` and `ErrUnavailable`, cites
  decision-2 and the three named tests, and carries refreshed line cites.
- Rows 1-2 (missing session, vanished pane) match herdr-core now, so they left
  the deviation table; remaining rows renumbered 1-5. Row 5 (unreachable
  server) records exit 66 per decision-2 with the gc v1.5.1 caveat. Coverage
  table count 7 -> 5; summary table rows updated to match. Verdict paragraph
  rewritten. The `seam_adapter.go:189-194` is-running citation is dropped; only
  `runtime_handle.go:187-190` remains.
- `docs/provider-method-audit/peek.md`: the missing-record/vanished-pane
  deviation row became a Matches entry; the unreachable-Rex row is a low
  deviation; stale line cites refreshed; Unverified notes the live dialog-loop
  race.

| ID | Status |
| --- | --- |
| REQ-004 | covered |
| REQ-005 | covered |
| REQ-006 | covered |

## Changed Files

- `docs/provider-herdr-parity.md`
- `docs/provider-method-audit/peek.md`
- `plans/GCREX-48/implementation-summary-gcrex-ue1p.md`
- (cherry-picked, unchanged content) `backlog/decisions/decision-1*`, `decision-2*`, and WI-2's code/test files.
- No overlay files touched.

## Verification

- First command: re-checked claims against gc v1.5.1 (`git show v1.5.1:internal/runtime/exec/exec.go` lines 153-170,
  `internal/worker/runtime_handle.go` 183-192, `internal/runtime/dialog.go` 555-562): all confirmed.
- Final proof: `make test` — pass (all packages ok); `golangci-lint run ./...` — 0 issues.
- Test names cited: `TestPeekNoScreenWhenNoRecord`, `TestPeekNoScreenWhenPaneVanished`,
  `TestPeekNoScreenWhenRexUnavailable`.

## Remaining Risks

- gc v1.5.1 reads exit 66 as an ordinary error; a live run of the startup
  dialog loop against a vanished pane is unverified (source reading only).
- Doc line cites outside `### peek` were not refreshed.
- The branch is detached and unpushed; WI-1/WI-2 commits exist twice (originals
  in sibling worktrees, cherry-picks here).
