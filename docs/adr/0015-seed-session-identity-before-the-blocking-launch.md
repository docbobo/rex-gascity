# 0015. Seed session identity before the blocking launch

- Status: Accepted
- Date: 2026-11-18 (backfilled from GCREX-13)

## Context

A Rex pane launch can block for seconds while Rex dismisses startup dialogs.
During that window, gc's pending-create check
(`runningSessionMatchesPendingCreateInfo`,
`cmd/gc/session_lifecycle_parallel.go`) reads `GC_SESSION_ID` and
`GC_INSTANCE_TOKEN` through `get-meta`.

The provider used to record the binding only after the launch returned, so the
check saw an empty identity. The reconciler then rolled the create back ("live
runtime belongs to another session") and freed the pool slot, and the next spawn
repeated the loop. A pilot city logged `stale_async_start` after about 8.7 s and
a run of `failed-create` sessions.

## Decision

`Start` seeds the identity keys from the start config `env` before
`NewWindow`/`NewSplit` (`internal/provider/identity.go`). The keys are
`GC_SESSION_ID`, `GC_INSTANCE_TOKEN`, `GC_RUNTIME_EPOCH`,
`GC_CONTINUATION_EPOCH`, `GC_AGENT`, `GC_TEMPLATE`, `GC_ALIAS` and `GC_PROVIDER`.

The seed first clears those keys, so a prior life cannot survive. It then
persists a provisional binding with no block id. The final binding replaces the
provisional one after the launch.

A failed or canceled start clears the seed and drops the binding. `Stop` clears
them after the pane closes. `Stop` takes the same state lock as `Start`, so a
rollback during the launch closes the real pane. `identity_test.go` probes
`get-meta` while the launch is blocked.

## Consequences

- `get-meta` answers the current incarnation's identity for the whole launch
  window.
- Every rollback path has to clean up both the seed and the binding. Tests pin
  each path.

## Alternatives rejected

- **Record the binding only after the launch returns.** This was the original
  order, and it is the race.
- **Change gc's pending-create check to tolerate an empty identity.** That fix
  is not ours to land, and an empty identity is ambiguous: it would also accept a
  runtime that really belongs to another session. The herdr provider solves the
  same race provider-side, with `seedMetaFromEnv` and `bindPlacement`.
- **Make the launch non-blocking and bind first.** Rex returns the block id only
  from the launch call, so there is nothing to bind until it returns.
