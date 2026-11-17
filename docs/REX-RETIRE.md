# Retiring a gc session whose Rex pane stays live

This records the comparison behind GCREX-12 and the retire/drain decision that
follows from it: why gc closed a pool-managed control-dispatcher session as
`drained` while its Rex pane and `gc convoy control --serve` process stayed
alive, how `runtime-herdr-core` avoids the same gap, and what the Rex provider
now does differently.

## The observed failure

On `~/my-city` (gc 1.5.1, `[session] provider = "rex"`), gc starts the control
dispatcher as a pool wisp (`core__control-dispatcher-mc-wisp-*`), the dispatcher
registers, and the reconciler then drains and closes the session bead with
`close_reason = "session drained: pool slot retired by reconciler"`. The Rex
pane and the `gc convoy control --serve --follow` process stay live, but the
dispatcher disappears from `gc session list` (active/suspended only).

The bead never carries `drain_finalize=deadline`, so this is the ordinary
drain-ack finalize, not the 30-minute `poolSlotDrainRetireDeadline` bound.

## Why gc closed the bead without stopping the runtime

gc's reconciler observes each desired session with
`observeRuntimeProviderLiveness` (`cmd/gc/session_reconciler.go`), which returns
`running` (provider `is-running`) and `alive` (provider `process-alive`). The
exec adapter for a pack runtime maps the two RPP ops:

- `IsRunning` asks `is-running` (`internal/runtime/exec/exec.go`); the Rex
  provider answers `true` while the pane exists.
- `ProcessAlive` asks `process-alive` (`internal/runtime/exec/exec.go`); only a
  literal `false` reports a dead agent, an empty/unknown answer reads as alive.

The control dispatcher's configured hint is `process_names = ["gc"]` (core pack
`agents/control-dispatcher/agent.toml`). But gc runs from a wrapped binary, so
`$GC_BIN` is `/nix/store/.../bin/.gc-wrapped` and Rex reports the pane's
foreground process as:

```
name = ".gc-wrapped"
path = "/nix/store/.../bin/.gc-wrapped"
argv0 = "/nix/store/.../bin/.gc-wrapped"
```

The Rex provider's `process-alive` matched process names literally
(`internal/provider/liveness.go`, `processMatches`), so `.gc-wrapped` did not
match the `gc` hint and the op answered `false`. gc therefore saw
`running = true, alive = false` and classified the seat as a zombie.

The reconciler's drain-ack branch keys on `alive`, not `running`
(`cmd/gc/session_reconciler.go`, the `if alive { markDrainAckStopPending ... }
else { finalizeDrainAckStoppedSession ... }` arm). On the `!alive` arm it closes
the bead with state `drained` **without ever calling provider `stop`**, so
nothing closes the pane. gc's own drain-ack escalation notes this class of wedge
(`cmd/gc/drain_ack_escalation.go`: "the runtime never dies"), but escalation is
only reached from the `alive` arm, which the false-zombie result never enters.

## How runtime-herdr-core handles it

`runtime-herdr-core` (github.com/gastownhall/gascity-packs) is the closest
reference: same RPP lifecycle, same `process-alive` op, same gc reconciler
consumer.

- `assets/gc_runtime_herdr_core/herdrcore/procs.py`
  - `expand_names(names, gc_binary)` adds `os.path.basename(gc_binary)` to the
    match set whenever a hint is `gc`, and also adds `NAME_ALIASES`
    (`kimi -> kimi-code`). This is the whole difference in the liveness
    observation: the wrapped gc is part of the match set.
  - `matches(proc, names)` then compares the process `comm` basename, the
    `argv[0]` basename, and (through interceptors) the script argument basename
    against the expanded set; a zombie matches nothing.
- `assets/gc_runtime_herdr_core/herdrcore/reads.py`
  - `agent_state(snapshot, record, names, meta)` calls
    `expand_names(names, meta.gc_bin)` before `find_agent`, returning
    `alive`/`dead`/`unknown` (unknown when the agent may be an unreadable
    process).
  - `op_process_alive` and `op_observe` use `agent_state`; `process-alive`
    answers `false` only on a definite `dead`, and an empty answer (cannot tell)
    reads as alive in gc.
- `assets/gc_runtime_herdr_core/herdrcore/lifecycle.py`
  - Module doc: `stop` and `kill-object` "end an incarnation's whole process
    tree" - `_capture` collects the agent subtree plus every process carrying
    `GC_INSTANCE_TOKEN`, TERM then KILL, `rescan` re-scans the token until
    nothing is found, and the record/metadata stay for a retry while anything
    may survive (`kill_incarnation`, `op_stop`, `_recycle`).
  - `expand_names` is also used for readiness (`cfg.env.get("GC_BIN", "")`),
    so the same wrapped-gc recognition drives both liveness and start.
- `assets/gc_runtime_herdr_core/herdrcore/metastore.py` persists
  `process_names` and `gc_bin` per incarnation, so the expansion survives the
  short-lived per-op process.

Net: the Rex provider's only material liveness difference was the missing
`GC_BIN` expansion. herdr-core's stop is additionally a process-tree kill, which
matters for detached descendants; the Rex provider's `stop` is a pane close.

## Decision

1. **gc is expected to call `stop` on the drain-ack/retire path, and it already
   does** whenever it observes the seat as alive. The gap was that a false
   `process-alive` made the seat look dead, so gc took the zombie/finalize arm
   and closed the bead without stopping. We do not change gc here; we make its
   observation correct.
2. **The Rex provider must report the wrapped gc as the agent process.** It now
   captures `GC_BIN` from the start config `env`, persists its basename on the
   binding, and expands incoming `process-alive` names with it when a hint is
   `gc` - mirroring herdr-core's `expand_names`. gc then sees `alive = true`,
   enters the stop-pending branch, and calls provider `stop`.
3. **The control dispatcher is not a separately owned service; it is a
   supervisor-initiated, pool-managed ephemeral session.** Its lifetime belongs
   to gc's controller/reconciler, which already owns the drain and the stop. We
   therefore do not add a service ledger or make the pane independently
   tracked; we make the existing stop path reachable instead.
4. **`stop` closes the pane, and `is-running` then returns false.** The
   dispatcher runs as the pane's foreground process (`exec gc convoy control`),
   so closing the block ends it. Rex exposes no process-table scan to a
   per-op provider, so the provider cannot perform herdr-core's token-based
   descendant sweep; a descendant that deliberately `setsid`s out of the pane is
   outside what a pane close can reach, and is a known limit rather than a
   silent success.

## Where the fix lives

- `internal/provider/liveness.go`: `expandProcessNames` and `procNameAliases`
  (herdr-core parity), used by `ProcessAlive`, with a fallback to the provider's
  own `GC_BIN` for bindings written before the basename was persisted.
- `internal/provider/binding.go`, `internal/provider/filestore.go`: the `GCBin`
  field and its on-disk record.
- `internal/provider/provider.go`: `Start` records `GC_BIN`'s basename.
- `internal/provider/retire_test.go`: fake-`rex.Client` coverage of the retire
  path - a live wrapped gc is reported alive, an exited program is not, and
  `stop` closes the pane so `is-running` is false.

## Live check on ~/my-city

With the fixed binary installed, against the live control-dispatcher pane
(`core__control-dispatcher-mc-wisp-791k`, foreground `.gc-wrapped`):

- `process-alive` with the `gc` hint: `false` on the old binary, `true` on the
  fixed one; `is-running` is `true`.
- `gc session list` lists the dispatcher as a tracked session and `rex block ls`
  shows exactly one dispatcher pane - no orphan.
- Under the old binary the same dispatcher was closed `drained` ~20s after start
  with the pane left behind; under the fixed binary the seat stayed tracked.
- `gc doctor`: 105 passed, 7 warnings, 1 failed - the one failure is the
  unrelated `beads-role` (global git config).
