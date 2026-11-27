# 0016. Report the wrapped gc as the agent process instead of tracking a service

- Status: Accepted
- Date: 2026-11-17 (backfilled from GCREX-12)

## Context

gc closed a pool-managed control-dispatcher session as `drained` while its Rex
pane and its `gc convoy control --serve` process stayed alive. gc asks
`process-alive` with the hint `gc`. But the pane's foreground process is the
wrapped binary (`.gc-wrapped`), so the provider answered `false`. gc then took
the drained branch and never called `stop`. The pane was orphaned.

## Decision

The provider captures `GC_BIN` from the start config `env` and persists its
basename on the binding. It expands an incoming `gc` hint with that basename
(`expandProcessNames`, `internal/provider/liveness.go:41`). This mirrors
herdr-core's `expand_names`. gc then sees the dispatcher alive, enters
stop-pending, and calls `stop`. `stop` closes the pane, which ends the
dispatcher, its foreground process.

`retire_test.go` covers the path: a live wrapped gc reads alive, an exited one
does not, and `stop` makes `is-running` false.

## Consequences

- gc's controller keeps owning the dispatcher's lifetime, and the provider only
  makes its observation correct.
- A descendant that `setsid`s out of the pane is beyond what a pane close can
  reach. That is a known limit, not a silent success.

## Alternatives rejected

- **Change gc's drain path.** gc's logic is right once the observation is
  right, and the bug was the provider's answer.
- **A service ledger that tracks the dispatcher independently of gc.** The
  dispatcher is a supervisor-initiated, pool-managed session, and its lifetime
  already belongs to gc's reconciler. A second owner would be a second source of
  truth.
- **herdr-core's token-based descendant sweep.** Rex exposes no process-table
  scan to a per-op provider.
