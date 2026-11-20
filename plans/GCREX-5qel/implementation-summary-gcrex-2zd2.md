---
schema: gc.build.implementation-summary.v1
workflow: {id: gcrex-2zd2, formula: do-work}
methodology: {pack: gascity, name: build-basic}
producer: {formula: do-work, stage: implement, attempt: 1}
status: approved
trace:
  upstream:
    - path: plans/GCREX-5qel/implementation-summary.md
    - path: beads/gcrex-2zd2
      hash: bead:gcrex-2zd2
    - path: beads/gcrex-5qel
      hash: bead:gcrex-5qel
---

# Implementation Summary: GCREX-5qel review fixes — CodeRabbit CHANGES_REQUESTED on PR #5

## Summary

Three review findings fixed on `gcrex-5qel-pane-label`, each with a test that
fails on the previous head (`852dbbe`):

1. **Record-less discovery** (`internal/provider/binding.go`, `scan`). The
   fallback scan now matches a block whose label ends in `" (<GC_SESSION_NAME>)"`
   as well as the exact name-derived label, and records the live label
   (`PaneLabel: blk.Label`) so `stillBound` keeps working. `IsRunning` and
   `Stop` therefore find a live `<agent> (<session>)` pane with no sidecar
   record and no start env.
2. **Launch label from the projected env** (`internal/launch/launch.go`,
   `internal/provider/provider.go`). `launch.Project(env)` exposes the pool
   routing projection `NewPlan` already applied; `start` derives the launch
   label from it, so launch label, discovery hint and recorded label all agree.
   Before this, `BEADS_ACTOR=legacy` + `GC_TEMPLATE=rig/worker` launched
   `legacy (seat)` while the hint said `worker (seat)`, and a restart with the
   record lost launched a duplicate pane.
3. **Clamp must not collide** (`internal/provider/sessionname.go`). The
   over-long branch truncates only the agent prefix; when the parenthesised
   session name alone cannot fit, it falls back to the bare clamped session
   name instead of silently cutting the session identity.

DECLINED as instructed: CodeRabbit's request to retry the legacy name-derived
label during rediscovery. Re-attach compatibility with pre-change pane labels is
out of scope for gcrex-5qel; existing GCREX-72 legacy handling is untouched.

## Proof (red on 852dbbe, green with the fix)

- `TestRecordlessDiscoveryBySessionSuffix` (`IsRunning`, `Stop`) —
  red: `IsRunning(rex-gascity--worker) = false`; `Stop` closed no block.
- `TestStartPaneLabelFromProjectedEnv` — red: launched label
  `legacy (rex-gascity--worker)`, want `worker (rex-gascity--worker)`.
- `TestPaneLabelForClamp/suffix_alone_too_long_falls_back_to_the_bare_session_name`
  — red: the 128-rune cut kept `agent (` + a truncated session.
- Guards: `TestPaneLabelForClamp/long_sessions_differing_at_the_tail_stay_distinct`,
  `TestPaneLabelRediscovery`, `TestStartDerivesPaneLabelFromEnv` unchanged.

## Verification

- `make test` (`go test ./...`) — all packages pass.
- `go vet ./...` — clean; `golangci-lint run ./...` — 0 issues.

## Docs

- `docs/rex-provider.md` — projected-env label paragraph and record-less
  suffix discovery.
- `docs/REX-CONTRACT.md` §13.3 — same in the pane-label paragraph.
- `plans/GCREX-5qel/implementation-summary.md` — "Review fixes" section.

## Limit

Identity past 128 runes cannot be represented in a Rex label: the fallback
clamps the bare session name, so two sessions differing only beyond the limit
still collide. Nothing shorter than that is achievable with a 128-rune label.
