---
schema: gc.build.implementation-summary.v1
workflow: {id: gcrex-jp3p, formula: do-work}
methodology: {pack: gascity, name: build-basic}
producer: {formula: do-work, stage: implement, attempt: 1}
status: approved
trace:
  upstream:
    - path: plans/GCREX-5qel/requirements.md
      hash: sha256:487ea8073044d3dffdc9ce91de72bfeaf3bae85f66f7ac8de2aa5669319bb32e
    - path: plans/GCREX-5qel/implementation-plan.md
      hash: sha256:f7f0447905cb06f86920191e571536bd06c1a6fcd80ca3769a3692cda6693b1a
    - path: beads/gcrex-194n
      hash: bead:gcrex-194n
      ids: [REQ-001, REQ-003, REQ-005, REQ-009]
  coverage:
    - {id: REQ-001, status: covered}
    - {id: REQ-003, status: covered}
    - {id: REQ-005, status: covered}
    - {id: REQ-009, status: covered}
---

# Implementation Summary: GCREX-5qel implement item 2 — route pane-label producers through paneLabelFor

## Summary

Plan section 2. Every derived pane-label producer/resolver in
`internal/provider` now calls the env-aware `paneLabelFor(n, cfg.Env)` added by
the dependency item (`gcrex-ac24`, base commit `fbc28c8`):

- `internal/provider/provider.go:439` — `launch.Config.Label`
- `internal/provider/provider.go:458` — discovery `hint.PaneLabel`
- `internal/provider/provider.go:698` — `record()` → `Binding.PaneLabel`
- `internal/provider/identity.go:72` — provisional binding `PaneLabel`

`Binding.paneLabel()` stays the stored-label-then-name legacy fallback; only
its comment and the `Binding.PaneLabel` field comment were reworded. No
scan/stillBound change: they already compare against the hint label captured
at `Resolve` (`internal/provider/binding.go:440-460` originally; the
`scan`/`stillBound` bodies are untouched). No tab-label, gascity, `pack/`, or
overlay change.

The worktree was fast-forwarded from `861c1c5` to the dependency's delivered
commit `fbc28c8` before this change so the call sites compile against
`paneLabelFor`; this item's own commit only contains the rewiring, the test,
and this summary.

## Intended Behavior

- A rig-scoped start whose env carries `GC_SESSION_NAME` and a resolvable
  agent (`GC_ALIAS` → `GC_AGENT` → `BEADS_ACTOR` suffix) now opens its Rex
  block under `<agent> (<GC_SESSION_NAME>)`, stores that label in the final
  binding, and seeds the same label into the start hint. A lost sidecar record
  is therefore rediscovered under the same env-derived label instead of
  launching a second pane.
- The provisional binding written during the launch window carries the same
  derived label, so `stillBound` accepts it once the final record replaces it.
- Unchanged: starts without `GC_SESSION_NAME` (all existing alias tests), the
  town-level short-circuit, `SessionName.PaneLabel()` as the name-only legacy
  fallback, tab labels, and the Rex wire/RPP contract.

Observed end-to-end effect for the wisp fixture env
(`GC_RIG=my-project`, `GC_AGENT=mc-wisp-ybq1`,
`GC_SESSION_NAME=gc__implementation-worker-mc-wisp-fvwrce`): the launched
block label and the stored `Binding.PaneLabel` are both
`mc-wisp-ybq1 (gc__implementation-worker-mc-wisp-fvwrce)` instead of the raw
start name `pi-mc-wisp-ybq1`.

## Changed Files

- `internal/provider/provider.go` — three label sites now call
  `paneLabelFor(n, cfg.Env)` (`:439`, `:458`, `:698`; the last shifted from
  `:679` after the dependency commit).
- `internal/provider/identity.go` — provisional binding label (`:72`).
- `internal/provider/binding.go` — `Binding.PaneLabel` field comment and
  `Binding.paneLabel()` comment describe the env-derived form and the
  name-only legacy fallback; behavior unchanged.
- `internal/provider/provider_alias_test.go` — new
  `TestStartDerivesPaneLabelFromEnv`: launch label, stored binding label, and
  lost-record rediscovery through the env-derived hint.
- `plans/GCREX-5qel/implementation-summary-gcrex-w0lc.md` — this artifact.

## Verification

- Red evidence (before the rewiring):
  `go test ./internal/provider -run TestStartDerivesPaneLabelFromEnv -count=1`
  failed with `pane label = "pi-mc-wisp-ybq1", want "mc-wisp-ybq1 (gc__implementation-worker-mc-wisp-fvwrce)"`,
  the same mismatch on the stored binding, and
  `relaunched a live pane: windows=0 splits=1` (the hint still matched the
  name-only label, so the live block was missed and a split was launched).
- First verification command: `go test ./internal/provider -run TestStartDerivesPaneLabelFromEnv -count=1` — PASS after the change.
- `go test ./internal/provider -count=1` — PASS.
- Final proof command: `make test` — PASS (all packages ok, including
  `internal/provider` and `internal/launch`;
  `internal/provider/sessionname_test.go` still asserts the name-only
  `PaneLabel()` and stays green).
- `golangci-lint run ./internal/provider/...` — `0 issues.`
- `gofmt -l internal/provider` — clean.
- Scope check: this item's commit touches only `internal/provider/` and
  `plans/GCREX-5qel/`; no `pack/`, no overlay, no gascity edit, no RPP or
  `internal/rex` protocol change.
- Not in this item: docs (REQ-010) and the live city-restart check (REQ-011)
  remain with their own plan items.

| ID | Status |
| --- | --- |
| REQ-001 | covered |
| REQ-003 | covered |
| REQ-005 | covered |
| REQ-009 | covered |

## Remaining Risks

- The launch label is derived from the start config env before
  `launch.NewPlan` projects pool routing (`cfg.Env = plan.Env` happens after
  `NewPlan`), while the hint and final record derive from the projected env.
  Projection only rewrites `BEADS_ACTOR` (to `GC_TEMPLATE`) when `GC_ALIAS` is
  empty and `GC_AGENT` is empty, so the two can disagree only for a seat with
  no alias and no agent — then the launch label would use the pre-projection
  `BEADS_ACTOR` and `stillBound` would miss the stored record, falling back to
  the hint scan. This edge case is not exercised by the requirements' env
  examples; a follow-up would derive the launch label from `plan.Env` by
  projecting before building the plan.
- Live pans are not relabeled; the new form appears only for panes started
  after this lands (documented behavior, REQ-011 restart).
- The worktree's base is the dependency commit `fbc28c8`, not
  `origin/main` `861c1c5`; integration must keep that commit before this one.
