---
schema: gc.build.implementation-summary.v1
workflow: {id: gcrex-iwab, formula: do-work}
methodology: {pack: gascity, name: build-basic}
producer: {formula: do-work, stage: implement, attempt: 1}
status: approved
trace:
  upstream:
    - path: plans/GCREX-5qel/requirements.md
      hash: sha256:487ea8073044d3dffdc9ce91de72bfeaf3bae85f66f7ac8de2aa5669319bb32e
    - path: plans/GCREX-5qel/implementation-plan.md
      hash: sha256:f7f0447905cb06f86920191e571536bd06c1a6fcd80ca3769a3692cda6693b1a
    - path: beads/gcrex-ac24
      hash: bead:gcrex-ac24
      ids: [REQ-001, REQ-002, REQ-003, REQ-004, REQ-006, REQ-007, REQ-008]
  coverage:
    - {id: REQ-001, status: covered}
    - {id: REQ-002, status: covered}
    - {id: REQ-003, status: covered}
    - {id: REQ-004, status: covered}
    - {id: REQ-006, status: covered}
    - {id: REQ-007, status: covered}
    - {id: REQ-008, status: covered}
---

# Implementation Summary: GCREX-5qel implement item 1 — pane label derivation + clamp

## Summary

Plan sections 1 and 3. Added `paneLabelFor(SessionName, map[string]string)`,
`paneAgent(map[string]string)` and `maxPaneLabelRunes = 128` to
`internal/provider/sessionname.go`, plus the table-driven tests
`TestPaneLabelFor` / `TestPaneLabelForClamp` in
`internal/provider/pane_label_test.go`. REQ-007 was re-verified before the
change; the label really does come from `Config.Label` through
`launch.NewPlan` into the Rex block layout. The five producer/resolver call
sites (`provider.go:439`, `provider.go:458`, `provider.go:698`,
`identity.go:72`, `binding.go:64-70`) are NOT rewired here — work item 2 owns
them — so runtime pane labels are unchanged until that item lands. Docs
(REQ-010) and the live restart check (REQ-011) are also out of this item.

## Intended Behavior

- `paneLabelFor(n, env)` returns `<agent> (<GC_SESSION_NAME>)` when the
  start-config env carries both a resolvable agent and a trimmed, non-empty
  `GC_SESSION_NAME`; otherwise it returns `n.PaneLabel()` (today's
  name-derived label, unchanged).
- Town-level short-circuit first: `env["GC_RIG"] == "" && n.IsTown()` →
  `n.PaneLabel()`. `IsTown()` alone never decides, so gc 1.5.1 pool names
  without `--` are not misclassified.
- `paneAgent` resolution order, each step requiring a non-empty trimmed value:
  `GC_ALIAS`, then `GC_AGENT`, then `BEADS_ACTOR` after the first `/` (whole
  value when there is no `/`). No other variable, no sanitization.
- Clamp: unchanged when the composed label is ≤128 runes; otherwise
  `suffix = " (" + session + ")"`; if the suffix alone is ≥128 runes the
  first 128 runes of the composed label are returned; else the agent is cut to
  `128 - runes(suffix)` and the exact suffix is appended. All truncation is
  rune-based, so multi-byte sequences never split.
- `SessionName.PaneLabel()` is untouched and stays the name-only fallback.
- Tab labels (`TabLabel()`/`tabLabelFor`), session names, `gc:<city>`, RPP
  traffic, `pack/` and overlays are untouched.

REQ-007 trace finding (re-verified in this worktree at `861c1c5`):

- `internal/provider/provider.go:439` — `LaunchSpec`/`launch.Config` label is
  fed from `n.PaneLabel()`.
- `internal/launch/launch.go:40-52` — `NewPlan` sets `Spec.Label: cfg.Label`
  (line 47) and passes `cfg.Label` to `blockLayout` (line 48).
- `internal/launch/launch.go:258-274` — `blockLayout` writes
  `node.Label = label` (line 271) into `rex.LayoutBlock.Label`.
- `grep -rn "Label" internal/launch internal/provider` (non-test) shows no
  other writer on the start path: the remaining `Label` hits are this same
  chain, `tabLabelFor`, stored binding labels, and session labels.
- Observed current label on this city:
  `GC_SESSION_NAME=gc__requirements-planner-mc-wisp-mf93ir`,
  `GC_AGENT=mc-wisp-mf93ir`, `GC_ALIAS` unset,
  `BEADS_ACTOR=rex-gascity/gc.requirements-planner`, `GC_RIG=rex-gascity`;
  `Agent()` returns the whole name because it has no `--`, i.e. the pane label
  is `gc__requirements-planner-mc-wisp-mf93ir`. Under the new derivation the
  same env yields `mc-wisp-mf93ir (gc__requirements-planner-mc-wisp-mf93ir)`
  once item 2 routes the call sites.

## Changed Files

- `internal/provider/sessionname.go` — `maxPaneLabelRunes`, `paneLabelFor`,
  `paneAgent`; added `unicode/utf8` import.
- `internal/provider/pane_label_test.go` — new: `TestPaneLabelFor`
  (alias wins, empty alias → agent, `BEADS_ACTOR` suffix, bare
  `BEADS_ACTOR`, no-agent, no-session, town-level unchanged, rigless pool name
  with `GC_RIG` set) and `TestPaneLabelForClamp` (long agent keeps suffix,
  long session rune-safe + deterministic, short label unchanged).
- `plans/GCREX-5qel/implementation-summary-gcrex-uro7.md` — this artifact.

## Verification

- Red evidence (behavioral, current code): with `paneLabelFor` stubbed to
  `return n.PaneLabel()` the new tests fail, e.g.
  `paneLabelFor("gc__implementation-worker-mc-wisp-fvwrce", …) = "gc__implementation-worker-mc-wisp-fvwrce", want "gc.implementation-worker (gc__implementation-worker-mc-wisp-fvwrce)"`
  and `label has 22 runes, want 128`. Before the implementation the package
  did not even build (`undefined: paneLabelFor`). The stub was reverted.
- First verification command:
  `go test ./internal/provider -run 'TestPaneLabel(For|ForClamp)'` — PASS
  (12 subtests).
- `make test` — PASS (all packages ok, including `internal/provider` and
  `internal/launch`).
- `golangci-lint run ./...` — `0 issues.`
- `gofmt -l internal/provider/` — clean.
- Diff scope: only `internal/provider/sessionname.go`,
  `internal/provider/pane_label_test.go` and
  `plans/GCREX-5qel/implementation-summary-gcrex-uro7.md`; no `pack/`,
  no `internal/rex` protocol change, no gascity edit.
- Final proof command: `make test` — PASS.

| ID | Status |
| --- | --- |
| REQ-001 | covered |
| REQ-002 | covered |
| REQ-003 | covered |
| REQ-004 | covered |
| REQ-006 | covered |
| REQ-007 | covered |
| REQ-008 | covered |

## Remaining Risks

- The new derivation is not wired to any call site in this item (by design),
  so live pane labels stay name-only until work item 2 routes
  `provider.go:439/458/698` and `identity.go:72`. A restart in between shows
  no visible change.
- `Binding.paneLabel()` (`internal/provider/binding.go:64-70`) still falls
  back to the name-only `PaneLabel()` for legacy records; item 2 updates the
  comments. Rediscovery agreement (`TestPaneLabelRediscovery` in the plan) is
  deferred to that item because the hint/stored-label pair is only meaningful
  once call sites use the derivation.
- `BEADS_ACTOR` without `/` is used whole (`mayor`), per the amended
  requirements; the human readability concern in the requirements Open
  Questions (order yields the wisp id rather than the role) remains open and
  is not decided here.
- The GCREX-72 tab work may land its own 128-rune clamp; if it does, the two
  helpers should be consolidated (Open Question carried from the plan).
- The plan and requirements files live only in the launcher checkout
  (`plans/GCREX-5qel/`, untracked there); only this summary is committed in the
  worktree.
