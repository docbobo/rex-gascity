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

## Not done here

- REQ-011 live check (pool seat and mayor pane after a city restart) is
  deferred: it requires stopping and starting the running city. Nothing in the
  code path depends on it; the label is asserted end-to-end in
  `TestStartDerivesPaneLabelFromEnv`.
