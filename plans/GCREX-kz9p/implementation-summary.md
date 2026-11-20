---
schema: gc.build.implementation-summary.v1
workflow: {id: gcrex-280m, formula: do-work}
methodology: {pack: gascity, name: build-basic}
producer: {formula: do-work, stage: implement, attempt: 1}
status: approved
trace:
  upstream:
    - path: beads/gcrex-kz9p
      hash: bead:gcrex-kz9p
      ids: [REQ-001, REQ-002, REQ-003, REQ-004, REQ-005]
  coverage:
    - {id: REQ-001, status: covered}
    - {id: REQ-002, status: covered}
    - {id: REQ-003, status: covered}
    - {id: REQ-004, status: covered}
    - {id: REQ-005, status: covered}
---

# Implementation Summary: GCREX-kz9p — wisp panes land in their rig tab

## Summary

`tabLabel` (`internal/provider/sessionname.go`) read the session-name shape
before the start env: a name with no `--` (a gc wisp instance such as
`gc__implementation-worker-mc-wisp-s5gax5`) hit the `name.IsTown()` branch and
returned the city tab label even when the start env carried `GC_RIG=rex-gascity`.
Rig panes therefore collapsed into the city tab. The function now reads
`GC_RIG` first and returns `Rig: <GC_RIG>` for any name shape; the town label is
used only when `GC_RIG` is empty.

## Intended Behavior

- `tabLabel(name, env, town)` returns `Rig: <GC_RIG>` whenever `env["GC_RIG"]`
  is non-empty, including town-style names with no `--`.
- With `GC_RIG` empty: a town-level name (no `--`) returns the town label (city
  pin, or `Gas City` when no pin is set); a rig name returns `Rig: <name.Rig()>`.
- Every result stays clamped to Rex's 128-rune label limit.
- The pane label is unchanged (`<agent> (<GC_SESSION_NAME>)`); the tab no longer
  depends on it.
- Existing panes started by the pre-fix branch build (a wisp in the city tab)
  are not migrated; each one moves to its rig tab when it is next started.

Coverage:

| ID | Status |
| --- | --- |
| REQ-001 | covered |
| REQ-002 | covered |
| REQ-003 | covered |
| REQ-004 | covered |
| REQ-005 | covered |

## Changed Files

- `internal/provider/sessionname.go` — `tabLabel` reads `GC_RIG` before the name
  shape; town label only when `GC_RIG` is empty.
- `internal/provider/sessionname_test.go` — the "stray GC_RIG on a town name"
  case is inverted to "GC_RIG on a town name groups by rig" and expects
  `Rig: Other` (regression test for the grouping decision).
- `internal/provider/provider_alias_test.go` — start-level expectations use
  `rigTab`: `TestStartDerivesTabFromGCRig`, `TestIsRunningCanonicalAlias`,
  `TestIsRunningRecoversFromStaleBinding`, `TestAliasResolutionThroughFileStore`;
  the misleading `aliasTab` constant is removed.
- `docs/rex-provider.md` — identity table, start-config derivation and the
  "starting the first agent" bullet state that `GC_RIG` wins for any name shape.
- `plans/GCREX-kz9p/implementation-summary.md` — this artifact.

## Verification

First (red, before the fix) — reproduces the bug through the tab decision:

```sh
go test ./internal/provider/ -run 'TestTabLabel|TestStartDerivesTabFromGCRig|TestIsRunningCanonicalAlias'
```

Observed: FAIL — `tabLabel = "my-city", want "Rig: Other"`, `tab label =
"my-project", want "Rig: my-project"` (both the town-name and the wisp-name
cases landed in the city tab).

Final proof (after the fix) — full package suite plus vet:

```sh
go vet ./... && go test ./...
```

Observed: PASS — vet clean, every package `ok` (`internal/provider` included).

## Remaining Risks

- A pane started before this fix holds the city tab label in its recorded
  binding; it is not re-tabbed, and a restart with the record lost opens a pane
  in the rig tab rather than the old city tab. The duplicate is bounded to one
  stale pane per wisp and clears when the wisp restarts.
- The bead notes also ask for "remove the name-split fallback and derive the tab
  from `GC_*` only"; the acceptance criteria contradict that by requiring the
  town label whenever `GC_RIG` is empty. The acceptance criteria won: a
  town-level name with no `GC_RIG` still lands in the city tab, and a rig name
  with no `GC_RIG` still uses the name split. Resolving that conflict needs an
  operator decision on how a GC_RIG-less start should be grouped.
- The live binary `~/go/bin/gc-runtime-rex` must be rebuilt (`make install`)
  after this branch lands for the operator to see the fix.
