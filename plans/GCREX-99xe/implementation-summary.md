---
schema: gc.build.implementation-summary.v1
workflow: {id: gcrex-471gp, formula: do-work}
methodology: {pack: gascity, name: build-basic}
producer: {formula: do-work, stage: implement, attempt: 1}
status: approved
trace:
  upstream:
    - path: beads/gcrex-99xe
      hash: bead:gcrex-99xe
      ids: [REQ-001, REQ-002, REQ-003, REQ-004]
    - path: internal/provider/binding.go
      hash: sha256:3c53beaf7593b358bb1336e41c79833dbce07710d8e2f2f2b28bfa0fdb7f829f
    - path: internal/provider/pane_label_test.go
      hash: sha256:487b1da5fe500f7ba588385c8a178428c99d6fde7721e288bc376022e56fe20b
  coverage:
    - {id: REQ-001, status: covered}
    - {id: REQ-002, status: covered}
    - {id: REQ-003, status: covered}
    - {id: REQ-004, status: covered}
---

# Implementation Summary: GCREX-99xe — Same-tab pane precedence

## Summary

`Binder.scan` (`internal/provider/binding.go`) tab-scoped path accepted both a
bare pane label (the hint/record `pane`, or the name-derived label) and a
`"<agent> (<GC_SESSION_NAME>)"` suffix-labelled pane from the same tab, so the
pane Rex listed first won. The tab scan now runs the suffix match first and
only then falls back to the bare-label match, so the unambiguous,
session-naming suffix always wins regardless of scan order. The bare-only case
still resolves. The scan comment now also names the town-level bare-label
fallback in `paneLabelFor` (a town-level start with no `GC_RIG` is labelled
with the bare session name), matching the code's actual acceptance.

## Intended Behavior

- Inside one candidate tab, a `"<agent> (<GC_SESSION_NAME>)"`-labelled pane is
  adopted before any bare-label pane; the result no longer depends on block
  order.
- A bare-label pane still resolves when it is the only pane shape in the tab:
  the hint/record pane label, or the name-derived pane label for a recordless
  resolve.
- The global (any-tab) fallback is unchanged: suffix matches only.
- The scan comment explains both bare-label shapes it accepts: the truncated
  suffix fallback and the town-level `paneLabelFor` fallback.

Coverage:

| ID | Status |
| --- | --- |
| REQ-001 | covered |
| REQ-002 | covered |
| REQ-003 | covered |
| REQ-004 | covered |

## Changed Files

- `internal/provider/binding.go` — tab-scoped loop now scans blocks twice per
  candidate tab: `suffixMatch` first, then the existing `paneMatches`
  (hint/record pane label, bare session name). Comment above the loop records
  the precedence; the `identity` comment now mentions the town-level
  bare-label fallback.
- `internal/provider/pane_label_test.go` — `TestTabScopedScanPrefersSuffixLabel`
  puts a bare-label pane and a suffix-labelled pane in one tab, asserts Stop
  adopts the suffix-labelled pane in both list orders, and asserts a bare-only
  tab still resolves.
- `plans/GCREX-99xe/implementation-summary.md` — this artifact.

## Verification

First verification command — the red proof, run against the pre-fix
`binding.go` (`git stash push -- internal/provider/binding.go`):

```sh
go test ./internal/provider/ -run TestTabScopedScanPrefersSuffixLabel
```

Observed: FAIL — `pane_label_test.go:481: Stop closed the bare-label pane
block:worker; the suffix-labelled pane names the session`. With the fix
restored the same test passes.

Final proof command — full suite, vet, and lint:

```sh
go test ./... && go vet ./... && golangci-lint run ./...
```

Observed: PASS — `go test ./...` all packages ok (`internal/provider` 7.939s);
`go vet ./...` exit 0; `golangci-lint run ./...` reports `0 issues.`

## Remaining Risks

- The fix is committed on a worktree based on `origin/main` `c864041`; the
  originating GCREX-p1zx branch (`origin/gcrex-p1zx-bare-label-resolve`,
  unmerged) edits the same `scan` comment and loop, so merging both will need
  a small conflict resolution. The precedence rule here (suffix first) is
  independent of that branch's bare-label tightening.
- "Required" lane checks (parity/review) do not apply: the change is confined
  to `Binder.scan` label precedence and adds no provider method surface.
