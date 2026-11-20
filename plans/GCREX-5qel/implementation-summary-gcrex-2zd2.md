---
schema: gc.build.implementation-summary.v1
workflow: {id: gcrex-wgy5, formula: do-work}
methodology: {pack: gascity, name: build-basic}
producer: {formula: do-work, stage: implement, attempt: 1}
status: approved
trace:
  upstream:
    - path: beads/gcrex-2zd2
      hash: bead:gcrex-2zd2
      ids: [CR-1, CR-2, CR-3]
    - path: beads/gcrex-5qel
      hash: bead:gcrex-5qel
  coverage:
    - {id: CR-1, status: covered}
    - {id: CR-2, status: covered}
    - {id: CR-3, status: covered}
---

# Implementation Summary: GCREX-5qel review fixes — CodeRabbit CHANGES_REQUESTED on PR #5

| ID | Status |
| --- | --- |
| CR-1 | covered |
| CR-2 | covered |
| CR-3 | covered |

## Summary

The three CodeRabbit review findings on PR #5 are fixed on
`gcrex-5qel-pane-label`, on top of `852dbbe`, with one commit (`a0feb91`)
pushed to origin:

1. **Record-less discovery** (`internal/provider/binding.go`). `Binder.scan`
   now also matches a block label ending in `" (<GC_SESSION_NAME>)"` and records
   the live label, so `IsRunning` and `Stop` find a live `<agent> (<session>)`
   pane when there is no sidecar record and no start env.
2. **Launch label from the projected env** (`internal/launch/launch.go`,
   `internal/provider/provider.go`). `launch.Project(env)` exposes the
   pool-routing projection `NewPlan` already applied; `start` derives the
   launch label from that projected env, so launch label, discovery hint and
   recorded label always agree.
3. **Clamp must not collide** (`internal/provider/sessionname.go`). The
   over-long branch truncates only the agent prefix; when the parenthesised
   session name alone cannot fit it falls back to the bare clamped session name
   instead of cutting the discovery identity.

Declined as instructed: retrying the legacy name-derived label during
rediscovery (pre-change pane adoption). Re-attach compatibility with existing
pane labels stays out of scope for gcrex-5qel; GCREX-72 legacy handling is
unchanged.

## Intended Behavior

- A rig-scoped pane is labelled `<agent> (<GC_SESSION_NAME>)`, agent resolved
  `GC_ALIAS` → `GC_AGENT` → the `BEADS_ACTOR` segment after the first `/`.
- The launch label, the discovery hint and the recorded binding are all derived
  from the same pool-routing-projected env, so `BEADS_ACTOR` projection rewrites
  cannot make them disagree and a restart with the record lost does not launch
  a duplicate pane.
- With no sidecar record and no start env, `is-running` and `stop` resolve the
  pane from the parenthesised session name, independently of the display
  prefix.
- Clamping to 128 runes drops display-prefix characters, never the
  `(<session>)` suffix; when the suffix alone cannot fit, the label falls back
  to the bare session name.

## Changed Files

- `internal/provider/binding.go` — `scan` matches the session-identity suffix
  and records the live pane label.
- `internal/provider/provider.go` — `start` derives the launch label from
  `launch.Project(cfg.Env)`.
- `internal/launch/launch.go` — new `Project(env)`; `NewPlan` uses it.
- `internal/provider/sessionname.go` — `paneLabelFor` clamp keeps the suffix,
  falls back to the bare session name when the suffix cannot fit.
- `internal/provider/pane_label_test.go` — `TestRecordlessDiscoveryBySessionSuffix`,
  `TestStartPaneLabelFromProjectedEnv`, and two new `TestPaneLabelForClamp`
  subtests.
- `docs/rex-provider.md`, `docs/REX-CONTRACT.md` §13.3 — projected-env label
  and record-less suffix discovery.
- `plans/GCREX-5qel/implementation-summary.md` — "Review fixes" section;
  this file.

## Verification

- First (red) run on the pre-fix sources (`852dbbe` code with the new tests):
  `go test ./internal/provider/ -run 'PaneLabel|Recordless|StartPaneLabel'` —
  failed: `IsRunning(rex-gascity--worker) = false, <nil>; want true, nil`,
  `Stop did not close block:worker (rex-gascity--worker)`,
  `launched pane label = "legacy (rex-gascity--worker)"`, and the
  suffix-too-long clamp case kept a truncated session.
- Final proof: `make test` (`go test ./...`) — `ok` for
  `cmd/gc-runtime-rex`, `internal/launch`, `internal/provider`,
  `internal/rex/cli`; `go vet ./...` — clean; `golangci-lint run ./...` —
  `0 issues`.
- PR handoff: commit `a0feb91` pushed to `origin/gcrex-5qel-pane-label`;
  PR #5 (https://github.com/docbobo/rex-gascity/pull/5) open, ready for
  review, no configured checks.

## Remaining Risks

- Identity past 128 runes cannot be represented in a Rex label: the fallback
  clamps the bare session name, so two sessions differing only beyond the limit
  still collide. No 128-rune label can avoid that.
- Suffix discovery matches any block whose label ends in ` (<name>)` inside an
  accepted tab, so a name that is a strict suffix of another session name in
  the same tab is resolved to the first match; gc session names include a
  unique wisp/session id, so this is not reachable in practice.
- The live pool-seat/mayor pane check after a city restart (REQ-011 of the
  parent plan) is still deferred to the operator; it restarts the running city,
  including this session.
