# Seeding Rex session identity before launch

This records the comparison behind GCREX-13: gc's pending-create reconciler can
read a fresh Rex runtime as "live runtime belongs to another session" while the
pane launch is still blocking. The fix is provider-side: seed the identity
metadata and a provisional binding before the launch call, and clear them on
every rollback and on Stop.

## The observed failure

On `~/my-city` (gc 1.5.1, `[session] provider = "rex"`), a pool spawn creates a
fresh Rex-backed session, but the launch blocks for several seconds while Rex
dismisses its startup dialogs. During that window gc's pending-create ownership
check `runningSessionMatchesPendingCreateInfo`
(`cmd/gc/session_lifecycle_parallel.go`) reads `GC_SESSION_ID` and
`GC_INSTANCE_TOKEN` through the provider's `get-meta` op. The Rex provider only
recorded the pane binding after the launch returned, so the check saw an empty
identity and returned false. The reconciler then rolled the pending create back
(`cmd/gc/session_reconciler.go`, "live runtime belongs to another session"),
freeing the pool slot; the next spawn repeated the loop. The supervisor log
showed `stale_async_start` after ~8.7 s and several `failed-create` sessions in
a row while only the last survived.

## How the herdr reference handles it

The herdr runtime provider
(`github.com/gastownhall/gascity-packs`, `internal/runtime/herdr/provider.go`)
solves the same race by ordering the writes before the blocking launch:

- `seedMetaFromEnv` writes the start config's `env` identity keys into the
  provider's metadata sidecar for the session name first. The short-lived
  per-op provider therefore answers `get-meta` with the current
  `GC_SESSION_ID`/`GC_INSTANCE_TOKEN` while the pane is still booting.
- `bindPlacement` then persists a provisional pane binding before the launch
  call. The binding gives the metadata resolver the session's primary name and
  its aliases even though the launched pane is not yet available.
- Only after those writes does the provider run the blocking launch and record
  the final placement. On a failed launch the provisional record and seeded
  metadata are cleaned up; on stop the incarnation's metadata is removed so a
  prior life cannot be mistaken for the current one.

`runtime-herdr-core`'s metastore
(`assets/gc_runtime_herdr_core/herdrcore/metastore.py`) persists the same
per-incarnation metadata across the short-lived op processes. The durable
provider contract remains the four start-config keys `GC_RIG`, `GC_TEMPLATE`,
`GC_ALIAS`, and `GC_AGENT`; the identity keys are incidental but must be visible
during the launch window.

## What the Rex provider does now

`internal/provider/identity.go` seeds these keys from the start config `env`
under the runtime session name before `NewWindow`/`NewSplit`:

`GC_SESSION_ID`, `GC_INSTANCE_TOKEN`, `GC_RUNTIME_EPOCH`,
`GC_CONTINUATION_EPOCH`, `GC_AGENT`, `GC_TEMPLATE`, `GC_ALIAS`, `GC_PROVIDER`.

The seed first clears the same keys so a previous incarnation's identity cannot
survive a start that carries no replacement. It then persists a provisional
binding with the session id, tab label, pane label, and aliases but no block id
(the block id does not exist until Rex returns from the launch). The final
binding replaces the provisional one after the launch returns.

Teardown invariants:

- A failed or canceled `Start` clears the seeded keys and drops the provisional
  binding.
- `Stop` clears the seeded keys after the pane is closed (or was already gone)
  and then drops the binding. `Stop` takes the same state lock as `Start`, so a
  rollback arriving during the launch waits for the launch to finish and closes
  the real pane instead of racing the provisional record.

The tests in `internal/provider/identity_test.go` use the fake `rex.Client` to
probe `get-meta` while `NewWindow` is blocked, to verify a stale prior-life
identity is cleared, and to verify a failed launch leaves no seeded metadata or
binding behind.
