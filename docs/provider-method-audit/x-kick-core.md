# Method Audit: `x-kick-core`

Verdict: `not implemented`

## Deviations

| doc claim | current implementation | behavioral impact | severity |
|---|---|---|---|
| Registered in op table with `reads_stdin=False`, `housekeeping=False`, `default` 30 s deadline, 64 KiB stdout cap (`## x-kick-core` §Registration) | Absent from `ops` (`cmd/gc-runtime-rex/main.go:92-109`) and `unimplemented` (`cmd/gc-runtime-rex/main.go:78-82`); no kick handler or `kick.json` logic in `internal/provider/*.go`. `runWithContext` exits `2` (`exitUnknown`) with `gc-runtime-rex: unknown operation "x-kick-core"` on stderr (`cmd/gc-runtime-rex/main.go:32,132-140`). | Calling `x-kick-core` exits `2` instead of running a core kick. Inert for gc v1.5.1 (never calls `x-kick-core`; `## x-kick-core` §Conflicts) and Rex has no internal `x-kick-core` spawner. | low |
| Invoked as `<bin> x-kick-core` with no further args, spawned by `kick()` on `HcUnreachable` with `GC_EXEC_DEADLINE_MS` stripped (`## x-kick-core` §Inputs) | `runWithContext` rejects `x-kick-core` at `cmd/gc-runtime-rex/main.go:132-140` before arg checks or `NamespaceForCaller`; no `kick()` or `x-kick-core` spawn site exists in `internal/provider/*.go`. | Read ops never spawn a background `x-kick-core` process when Rex is unreachable; Rex CLI calls fail directly in the op. | low |
| Exit `0` with empty stdout/stderr across 4 paths (`started`, `already running`, kick not due, lock held), writing `kick.json` in `pack_state` on `started`/`already running` (`## x-kick-core` §Success output) | Returns exit `2` with empty stdout and `unknown operation` on stderr (`cmd/gc-runtime-rex/main.go:136-140`); no `kick.json` is ever written. | No durable `kick.json` status file or background core start occurs. | low |
| Failure cases exit `1` (`ConfigError`, `check_core_lock`, `hc server start` failure, hello timeout, `HcError`/`OSError`, `LockFileError`, expired deadline, start budget <= 0, `Cancelled`) with `kick.json` `ok:false` on post-lock failures (`## x-kick-core` §Failure cases) | Always exits `2` before `NamespaceForCaller()` (`cmd/gc-runtime-rex/main.go:150`) or any lock/backend call. | No exit `1` kick failure paths or `kick.json` failure records exist. | low |
| `GC_EXEC_DEADLINE_MS` / 30 s default deadline with 1 s `deadline.budget` margin (`## x-kick-core` §Registration, §Failure cases; `## Shared RPP contract` §Deadlines and cancellation) | `cmd/gc-runtime-rex/main.go` does not read `GC_EXEC_DEADLINE_MS` (`internal/provider/provider.go:178-179` notes this explicitly) and enforces no 30 s runner timer. | Inert while `x-kick-core` exits `2`; would matter only if implemented. | low |

## Matches

- **Unknown-op envelope (`## Shared RPP contract` §Failure envelope):** `runWithContext` (`cmd/gc-runtime-rex/main.go:132-140`) returns exit `2` (`exitUnknown`, `cmd/gc-runtime-rex/main.go:32`) with empty stdout, a prefixed stderr message (`gc-runtime-rex: unknown operation "x-kick-core"`), and without reading `stdin` (`stdin` is only passed to `spec.handle` at `cmd/gc-runtime-rex/main.go:158`).
- **Handshake consistency (`## x-kick-core` §Conflicts; `## Shared RPP contract` §Success envelope):** `handshake` (`cmd/gc-runtime-rex/main.go:38`) advertises only `["report-attachment","report-activity"]` and does not advertise any internal or `server-lifecycle` capability.
- **Signal cancellation wiring (`## Shared RPP contract` §Deadlines and cancellation):** `main` (`cmd/gc-runtime-rex/main.go:46`) wires `SIGTERM`/`SIGINT` to `signal.NotifyContext`, though `x-kick-core` exits `2` before any cancellable work.

## Unverified

- Whether any gc version newer than `v1.5.1` ever invokes `x-kick-core` (cannot be settled from this repo; gc `v1.5.1` has no `x-kick-core` path per `## x-kick-core` §Conflicts).
- Whether `GC_EXEC_DEADLINE_MS` or `GC_RPP_FAULT` is sent by any external caller in practice (neither is read in `cmd/gc-runtime-rex/main.go` or `internal/provider/*.go`; `## x-kick-core` §Unverified, `## Shared RPP contract` §Unverified).
- Pack-internal `herdr-core` questions (why `housekeeping=False` was chosen, Python-side `Popen` reaping of `x-kick-core`, and `resolve_paths`/`OpContext.__init__` exceptions beyond `_wrap`) are outside the Go provider codebase (`## x-kick-core` §Unverified).
