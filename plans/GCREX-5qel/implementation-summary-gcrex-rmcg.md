---
schema: gc.build.implementation-summary.v1
workflow: {id: gcrex-egc5, formula: do-work}
methodology: {pack: gascity, name: build-basic}
producer: {formula: do-work, stage: implement, attempt: 1}
status: approved
trace:
  upstream:
    - path: plans/GCREX-5qel/requirements.md
      hash: sha256:487ea8073044d3dffdc9ce91de72bfeaf3bae85f66f7ac8de2aa5669319bb32e
    - path: plans/GCREX-5qel/implementation-plan.md
      hash: sha256:f7f0447905cb06f86920191e571536bd06c1a6fcd80ca3769a3692cda6693b1a
    - path: plans/GCREX-5qel/implementation-summary-gcrex-w0lc.md
      hash: sha256:dbdb10bacc60f2f18d637702724ff27e3c7ca0b6bf7bb465dbff4b863f9a08fb
    - path: beads/gcrex-fs3o
      hash: bead:gcrex-fs3o
      ids: [REQ-005, REQ-008]
  coverage:
    - {id: REQ-005, status: covered}
    - {id: REQ-008, status: covered}
---

# Implementation Summary: GCREX-5qel implement item 3 — rediscovery agreement test

## Summary

Plan section 3 (`TestPaneLabelRediscovery`). Added one test to
`internal/provider/pane_label_test.go` that pins the rediscovery agreement for
a single start-config env:

- the hint `paneLabelFor(SessionName(start), env)` is the label `record()`
  stored on the binding (`internal/provider/provider.go:698`), and the label
  the launch used (`fake.CreatedWindows[0].Spec.Label`);
- a resumed start with the sidecar record present keeps the live block
  (`stillBound` over the stored tab/pane labels);
- a fresh provider with the record lost rediscovers the same block through the
  env-derived hint at `internal/provider/provider.go:458` instead of launching
  a second pane.

No production code changed. The base of this worktree is the dependency chain
from item 1 (`fbc28c8`) and item 2 (`09cf7ec`) — the worktree was
fast-forwarded from `origin/main` `861c1c5` to `09cf7ec` so the test compiles
against `paneLabelFor` and the routed call sites, matching item 2's own base
handling. Item 2's `TestStartDerivesPaneLabelFromEnv` covers the lost-record
path end to end; this test adds the hint-equals-stored agreement assertion and
the record-present resume in one place under the plan's name.

## Intended Behavior

- For env `GC_RIG=rex-gascity`, `GC_AGENT=mc-wisp-fvwrce`,
  `GC_SESSION_NAME=gc__implementation-worker-mc-wisp-fvwrce`, the pane label is
  `mc-wisp-fvwrce (gc__implementation-worker-mc-wisp-fvwrce)` at launch, in the
  start hint, and in the stored binding — one value, no divergence.
- Resume with the binding present is idempotent: no new window, no new split.
- Resume without the binding (sidecar lost) finds the live block under the same
  env-derived label; it must not launch a second pane. The provider needs no
  production change to express this: `Resolve` already accepts the env hint and
  `scan` compares block labels (`internal/provider/binding.go:440-460`).

## Changed Files

- `internal/provider/pane_label_test.go` — new `TestPaneLabelRediscovery`
  (package `provider`, reuses `internal/rex/faketest`; no new dependency).
- `plans/GCREX-5qel/implementation-summary-gcrex-rmcg.md` — this artifact.

## Verification

- Red evidence, hint regression probe: with the `Resolve` hint temporarily
  reverted to `n.PaneLabel()` (`internal/provider/provider.go:458`), the test
  fails at the lost-record path:
  `lost-record resume relaunched a pane: windows=0 splits=1`. Probe reverted.
- Red evidence, derivation probe: with `paneLabelFor` temporarily stubbed to
  `return n.PaneLabel()`, the test fails with
  `launched pane label = "pi-mc-wisp-fvwrce", want "mc-wisp-fvwrce (gc__implementation-worker-mc-wisp-fvwrce)"`.
  Probe reverted.
- First verification command:
  `go test ./internal/provider -run TestPaneLabelRediscovery -count=1` — PASS.
- `go test ./internal/provider` — PASS.
- `golangci-lint run ./internal/provider/...` — `0 issues.`
- `gofmt -l internal/` — clean.
- Final proof command: `make test` — PASS (all packages ok).
- Scope check: the item's commit touches only `internal/provider/pane_label_test.go`
  and `plans/GCREX-5qel/`; no `pack/`, no overlay, no `internal/rex` protocol
  change, no gascity edit.

| ID | Status |
| --- | --- |
| REQ-005 | covered |
| REQ-008 | covered |

## Remaining Risks

- The lost-record case is modelled with the fake client's session view; a real
  Rex restart that renumbers blocks would take the same `scan` path but is not
  exercised here (no bespoke production machinery was added, per the item's
  fallback).
- This worktree's base is the unmerged item 1 + item 2 commits (`fbc28c8`,
  `09cf7ec`) on top of `origin/main` `861c1c5`; integration must keep that
  order before this commit.
