# Method Audit: `inventory`

**Verdict:** `not implemented`

## Deviations

| Doc claim | Current implementation | Behavioral impact | Severity |
|---|---|---|---|
| Registered in ops table with `reads_stdin=False`, `default` 30 s deadline, 8 MiB `INVENTORY_STDOUT_CAP` (`## inventory` § Registration) | Absent from `ops` (`cmd/gc-runtime-rex/main.go:97-114`) and `unimplemented` (`cmd/gc-runtime-rex/main.go:83-87`); no `Inventory` method in `internal/provider/*.go`. `runWithContext` returns exit 2 (`cmd/gc-runtime-rex/main.go:30,136-144`) with `gc-runtime-rex: unknown operation "inventory"`. | Calling `inventory` fails with exit 2 instead of returning an inventory object. No impact on gc v1.5.1, which never calls `inventory`. | low |
| Exit 0 single-line ASCII JSON `{"complete":bool,"sessions":[...]}` with sorted entries (`name`, `state`, `object`, `agent`, `attached`, `last_activity`, `restart_requested`, `identity`), plus screen-read activity refresh and >600 s meta GC (`## inventory` § Success output) | Not implemented; `inventory` exits 2 with empty stdout (`cmd/gc-runtime-rex/main.go:136-144`). | Callers cannot fetch a city-wide session inventory or trigger its side effects in one op; gc v1.5.1 uses `list-running` instead. | low |
| Graceful exit 0 `{"complete":false,"sessions":[]}` on `HcError` (kicking unparked core) or `{"complete":true,"sessions":[]}` when parked; exit 1 on state-dir/path/cap/signal errors (`## inventory` § Failure cases) | Exits 2 before `NamespaceForCaller()` (`cmd/gc-runtime-rex/main.go:154`) or any provider call. | State-dir, unreachable-core, and parked-city conditions are never evaluated for `inventory`. | low |
| `GC_EXEC_DEADLINE_MS` / 30 s deadline enforcement and 8 MiB stdout cap (`## inventory` § Registration; `## Shared RPP contract` § Deadlines and cancellation, § Stdout caps) | `cmd/gc-runtime-rex/main.go` neither reads `GC_EXEC_DEADLINE_MS` nor caps `out.Bytes()` (`cmd/gc-runtime-rex/main.go:165`). | Inert while `inventory` exits 2; would matter if `inventory` is implemented later. | low |

## Matches

- **Unknown-op contract (`## Shared RPP contract` § Failure envelope):** `runWithContext` (`cmd/gc-runtime-rex/main.go:136-144`) returns exit 2 (`exitUnknown`, `cmd/gc-runtime-rex/main.go:30`) with empty stdout, a prefixed stderr message (`gc-runtime-rex: unknown operation "inventory"`), and without reading `stdin` (`stdin` is only passed to `spec.handle` at `cmd/gc-runtime-rex/main.go:161`).
- **Handshake consistency (`## Shared RPP contract` § Success envelope, `## inventory` § Conflicts):** `handshake` (`cmd/gc-runtime-rex/main.go:35`) advertises only `["report-attachment","report-activity"]` and omits `observe.v1`, consistent with `inventory` not being implemented.
- **Signal handling (`## Shared RPP contract` § Deadlines and cancellation):** `main` (`cmd/gc-runtime-rex/main.go:45`) wires `SIGTERM`/`SIGINT` to `signal.NotifyContext`, though `inventory` exits 2 before any cancellable work.

## Unverified

- How a post-v1.5.1 `gc` behaves if it invokes `inventory` and receives exit 2 when `observe.v1` is not advertised in `protocol` (no `inventory` caller or decoder exists in the pinned gc v1.5.1 tree).
- Whether future `gc` versions paired with this provider will require `observe.v1`, `GC_EXEC_DEADLINE_MS`, `GC_EXEC_CITY_PATH`, or `GC_RPP_FAULT`.
