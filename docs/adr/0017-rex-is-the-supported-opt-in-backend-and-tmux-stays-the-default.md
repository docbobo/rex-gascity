# 0017. Rex is the supported opt-in backend and tmux stays the default

- Status: Accepted
- Date: 2026-11-17 (backfilled from the GCREX-6 pilot)

## Context

The GCREX-6 pilot ran a worker city on `rex` and a mayor city on tmux under the
machine-wide launchd supervisor, both on gc 1.5.1. Three kill and restart cycles
showed no wedged session. Teardown left no orphan panes.

The pilot found two defects, unbounded rig-tab splitting and an error on a
vanished pane. Both were fixed and covered by live tests. `make check` passed 14
of 14. `make conformance` passed 6, failed none and skipped 2 (the optional
`exec` and `provision`). A CLI-to-server version skew (api 16 against 14) was
shown to be inside the supported range.

## Decision

`[session] provider = "rex"` is a supported, opt-in backend for a city. tmux
stays gc's default and the fallback. A city may run its mayor on tmux and its
workers on Rex.

## Consequences

- Nothing changes for a city that does not opt in.
- The pack must keep working beside tmux cities on the same supervisor.
- Live-only gaps found in the pilot are tracked as issues, not here. One is the
  namespace under a supervisor, now pinned with `GC_REX_CITY`.

## Alternatives rejected

- **Make Rex the default backend.** Rex is a preview (`0.1.0-preview.*`), and a
  server restart is never exercised by the provider (ADR-0014). Replacing tmux
  was a non-goal from the start.
- **Keep Rex experimental until every start-config hook is applied.** The pilot
  showed the core lifecycle is sound. The hooks landed later (GCREX-10,
  GCREX-15, GCREX-56) without changing this decision.
