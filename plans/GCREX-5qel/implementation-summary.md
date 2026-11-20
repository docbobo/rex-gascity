---
schema: gc.build.implementation-summary.v1
workflow: {id: gcrex-d0jb, formula: build-basic}
methodology: {pack: gascity, name: build-basic}
producer: {formula: build-basic, stage: finalize, attempt: 1}
status: approved
trace:
  upstream:
    - path: plans/GCREX-5qel/requirements.md
      hash: sha256:487ea8073044d3dffdc9ce91de72bfeaf3bae85f66f7ac8de2aa5669319bb32e
    - path: plans/GCREX-5qel/implementation-plan.md
      hash: sha256:f7f0447905cb06f86920191e571536bd06c1a6fcd80ca3769a3692cda6693b1a
    - path: beads/gcrex-5qel
      hash: bead:gcrex-5qel
      ids: [REQ-001, REQ-002, REQ-003, REQ-004, REQ-005, REQ-006, REQ-007, REQ-008, REQ-009, REQ-010, REQ-011]
  coverage:
    - {id: REQ-001, status: covered}
    - {id: REQ-002, status: covered}
    - {id: REQ-003, status: covered}
    - {id: REQ-004, status: covered}
    - {id: REQ-005, status: covered}
    - {id: REQ-006, status: covered}
    - {id: REQ-007, status: covered}
    - {id: REQ-008, status: covered}
    - {id: REQ-009, status: covered}
    - {id: REQ-010, status: covered}
    - {id: REQ-011, status: deferred, rationale: "Needs a full city stop/start with the newly installed gc-runtime-rex; deferred to the operator because it restarts the running city (including this session). Everything observable from tests and the built binary is green."}
---

# Implementation Summary: GCREX-5qel — pane label from agent + session name

## What shipped

A rig-scoped pane now opens as `<agent> (<GC_SESSION_NAME>)` instead of the raw
gc session name. The agent resolves `GC_ALIAS` → `GC_AGENT` → the `BEADS_ACTOR`
segment after the first `/`; a blank session name or agent keeps the
name-derived label, town-level panes are unchanged, and the label is clamped to
128 runes without cutting the parenthesised session name.

## Changes

- `internal/provider/sessionname.go` — `paneLabelFor(n, env)` derives the label
  once; `paneAgent(env)` implements the resolution order; `maxPaneLabelRunes`
  clamp keeps the session suffix intact.
- `internal/provider/provider.go` — `start` uses `paneLabelFor` for the launch
  label, the binding hint (`provider.go:445`, `:464`), and the recorded binding
  (`:709`); discovery re-derives the same label, so resolution never depends on
  the human-readable prefix.
- `internal/provider/binding.go`, `internal/provider/identity.go` — binding
  fallback/comment and identity mapping follow the derived label.
- Tests: `internal/provider/pane_label_test.go` (`TestPaneLabelFor`,
  `TestPaneLabelForClamp`, `TestPaneLabelRediscovery`) and
  `TestStartDerivesPaneLabelFromEnv` in `provider_alias_test.go`.
- Docs: `docs/rex-provider.md` identity table + start-config derivation,
  `docs/REX-CONTRACT.md` §13.3 pane-label paragraph and the
  `GC_ALIAS`/`GC_AGENT`/`GC_SESSION_NAME` rows. A pane label stays
  display-only: it is not part of the durable four-variable contract.

## Proof

- `go test ./...` — all packages pass.
- `go vet ./...` — clean; `golangci-lint run ./...` — 0 issues.
- Two per-item summaries carry the red/green detail:
  `implementation-summary-gcrex-uro7.md` (derivation + clamp),
  `implementation-summary-gcrex-w0lc.md` (call sites),
  `implementation-summary-gcrex-rmcg.md` (rediscovery).
- REQ-007 trace: the only start-path block-label writers are
  `launch.NewPlan`/`blockLayout` (`internal/launch/launch.go:40-52`,
  `:258-274`), fed by `Config.Label` from the provider.

## Review fixes

CodeRabbit's CHANGES_REQUESTED review on PR #5 (`gcrex-2zd2`, commit on top of
`852dbbe`):

1. **Record-less discovery** — `Binder.scan` also matches a block label ending
   in `" (<GC_SESSION_NAME>)"` and records the live label, so `IsRunning` and
   `Stop` find a `<agent> (<session>)` pane with no sidecar record and no start
   env (`internal/provider/binding.go`).
2. **Launch label from the projected env** — `launch.Project(env)` exposes the
   pool-routing projection; `start` derives the launch label from it, so the
   launch label, the discovery hint and the recorded label always agree
   (`internal/launch/launch.go`, `internal/provider/provider.go`).
3. **Clamp must not collide** — the over-long branch truncates only the agent
   prefix and falls back to the bare clamped session name when the suffix alone
   cannot fit, instead of cutting the session identity
   (`internal/provider/sessionname.go`).

Declined as instructed: retrying the legacy name-derived label during
rediscovery (pre-change pane adoption). Re-attach compatibility with existing
pane labels stays out of scope; GCREX-72 legacy handling is unchanged.

Tests: `TestRecordlessDiscoveryBySessionSuffix`,
`TestStartPaneLabelFromProjectedEnv`, and the new `TestPaneLabelForClamp`
subtests; each fails on `852dbbe`. Detail:
`implementation-summary-gcrex-2zd2.md`.

Identity past 128 runes still cannot be represented, so the fallback clamps the
bare session name; two sessions differing only beyond the limit collide. 128-rune
labels cannot avoid that.

## Not done here

- REQ-011 live check (pool seat and mayor pane after a city restart) is
  deferred: it requires stopping and starting the running city. Nothing in the
  code path depends on it; the label is asserted end-to-end in
  `TestStartDerivesPaneLabelFromEnv`.
