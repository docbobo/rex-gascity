# GCREX-72 — implementation summary

Title: Name the town-level tab after the city instead of `__town`
Bead: `gcrex-lve2` · Branch: `gcrex-72-town-tab`

## What shipped

`const townRig = "__town"` is gone. The town-level Rex tab label now derives
from the city pin instead of the synthetic rig name, and rig-level tabs are
unambiguous about being rig tabs.

| Session name | Before | After |
|---|---|---|
| town-level (`mayor`) | `__town` | the city pin (e.g. `my-city`), `Gas City` when no pin is set |
| rig agent (`rex-gas-city--foo`) | `<rig>` from name split | `Rig: <rig>`, `GC_RIG` preferred over the name split |

Everything else is unchanged: session names (`<rig>--<agent>`), session labels
(`gc:<city>`), pane/block labels, and the `IsTown` / `Rig` / `Agent` split on
`--`. Generated labels are clamped to Rex's 128-character limit
(`internal/provider/sessionname.go`).

## Changes

- `internal/provider/sessionname.go` — `townTabLabel(pin)` replaces
  `townRig`, `tabLabel(name, env, town)` derives town vs `Rig: <rig>` and
  clamps to `maxRexLabel`; `SessionName.IsTown` added.
- `internal/provider/provider.go` — `Provider` resolves the town tab once in
  `New` from `townTabLabel(cityPin())` and passes it to every start hint, so a
  stray `GC_RIG` on a town-level name cannot override the city label.
- `internal/provider/binding.go` — the binder carries the same `townTab`, so a
  legacy record fallback and a discovery scan agree; `tabLabelFor` deleted.
- `internal/provider/tablabel_test.go` (new, 109 lines) — the label matrix:
  town with/without pin, town with stray `GC_RIG`, rig with/without `GC_RIG`,
  name-split fallback, and the 128-char clamp.
- Docs: `README.md` identity table, `docs/REX-CONTRACT.md` §13.3 `GC_RIG` row,
  `docs/rex-provider.md` identity mapping + start-config derivation.

## Proof

- `tablabel_test.go` cases were red against the `__town` code and green after
  the provider/binder change.
- `go build ./...`, `go vet ./...`, `go test ./...` — pass.
- `golangci-lint run ./internal/provider/...` — 0 issues.

## Note on publishing

The workflow's publish step (`gcrex-aj09`) was closed as skipped by gc's
done-workflow retirement: build-basic repoints `gc.input_convoy_id` to the
implementation convoy, and gc retires the root when that convoy is terminal.
The branch was therefore assembled and pushed by hand from the implementation
worktrees. See the bead note on `gcrex-lve2`.
