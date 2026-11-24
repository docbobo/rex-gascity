## Scope

`internal/provider/` is the runtime provider. It implements RPP v0 for gc —
sessions, panes, state, meta, identity, overlays — and drives the `rex` CLI
through `internal/rex`. The repo-wide rules in the root `AGENTS.md` apply here;
this file adds only local invariants.

## Claims need two sources

The root rule ("Verifying claims") is strictest in this package. Before a change
to provider behavior lands, check it against both:

- `docs/provider-herdr-parity.md` — the method-by-method verdicts.
- The pinned gc checkout (`$GASCITY_SRC`, default `/Volumes/Sources/gascity`,
  tag `v1.5.1`): `internal/runtime/exec/`, `internal/runtime/protocol.go`,
  `docs/reference/exec-session-provider.md`.

Adding or changing a provider method means re-running the parity comparison and
updating `docs/provider-herdr-parity.md` plus `docs/provider-herdr-method-io.md`
and `docs/provider-herdr-method-deviations.md` in the same change. Do not change
a parity verdict without re-running the check against both sources.

## Invariants

- `internal/provider/namespace.go` is the one place that states the
  canonical-first dir/label rule for city state and Rex session labels. New
  namespace behavior starts there.
- `internal/provider/overlay.go` stages pack overlays (`runtimeMirrorDir =
  ".gc"`, `perProviderDir = "per-provider"`). Overlay files are managed
  exceptions — see the root "Overlay changes need approval" rule. Do not add or
  edit one without Boris's written approval.
- The provider answers `unknown`, never `dead`, when it cannot be sure of agent
  liveness (`agentsnapshot.go`).

## Tests

- The provider e2e suites run against the in-repo fake `rex`
  (`internal/rex/fakerex`). Run them with `make test-e2e`; `make test` includes
  them because the fake is the default when `REX_E2E` is unset.
- `REX_E2E=1` (or `REX_E2E=live`) selects the live server and is the escape
  hatch, not the normal path.
- A behavior change lands with the named test that fails without it. See the
  root "Test-driven development" rule.
