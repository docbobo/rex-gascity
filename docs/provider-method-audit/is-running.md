# Method Audit: `is-running`

**Verdict:** `deviations`

## Deviations

| Doc claim | Current implementation | Behavioral impact | Severity |
|---|---|---|---|
| `## is-running` §Inputs: `GC_EXEC_CITY_PATH` selects city; `GC_EXEC_STATE_DIR` (must be absolute) sets state root. | `cmd/gc-runtime-rex/main.go:149` calls `provider.NamespaceForCaller()` (`internal/provider/namespace.go:45-53`), which reads `GC_REX_CITY` (`internal/provider/statedir.go:79-84`) and `GC_REX_STATE_DIR` / `XDG_STATE_HOME` (`internal/provider/statedir.go:11-26`); no absolute-path check. | `GC_EXEC_CITY_PATH` and `GC_EXEC_STATE_DIR` are ignored; callers using only gc's `GC_EXEC_STATE_DIR` fall back to `$XDG_STATE_HOME/rex-gascity`. | medium |
| `## is-running` §Inputs & §Failure cases: does not need core up; when core is unreachable and nothing of it runs, returns exit 0 `false\n` (otherwise exit 1 and kicks core start). | `IsRunning` (`internal/provider/provider.go:595-598`) calls `Binder.Live` (`internal/provider/binding.go:282-313`), which always queries Rex (`Resolve`/`scan` or `revalidate`/`liveOnly`), never starts Rex, and returns exit 1 on any Rex error (`cmd/gc-runtime-rex/main.go:154-157`). | None for gc v1.5.1: `IsRunning` and `Start` cleanup treat exit 1 and exit 0 `false` identically (`boxOccupancy` returns `(false, false)` vs `(false, true)`, both `IsRunning() == false`). | low |
| `## is-running` §Registration & `## Shared RPP contract` §Deadlines: 30 s default deadline or `GC_EXEC_DEADLINE_MS`; 64 KiB stdout cap; `sweep_spawn_files` housekeeping. | `runWithContext` (`cmd/gc-runtime-rex/main.go:42-48,140-159`) uses `context.Background()` with signal cancellation only; no `GC_EXEC_DEADLINE_MS` or 30 s timer, no stdout cap, no spawn-file sweep. | Relies on gc's 30 s SIGTERM/SIGKILL. 64 KiB cap is unreachable for 5–6 byte `true\n`/`false\n` output. | low |
| `## is-running` §Inputs: `GC_RPP_FAULT=unavailable` forces `HcUnreachable`. | Not implemented in `cmd/gc-runtime-rex/main.go` or `internal/provider/`. | Fault-injection env var is ignored. | low |
| `## Shared RPP contract` §Failure envelope: stderr errors are redacted (`rpp/redact.py`) and follow `"<op>: <message>"` (e.g. `canceled (terminated)`). | `cmd/gc-runtime-rex/main.go:155` writes `gc-runtime-rex: is-running: <err>` to stderr without redaction; SIGTERM yields Go's `context canceled`. | Error text differs slightly; `is-running` takes no secrets in args/stdin, though `rex` CLI stderr is passed through unredacted. | low |

## Matches

- **Op registration & arity (`## is-running` §Registration, §Inputs):** Registered as `is-running <name>` requiring 1 arg (`cmd/gc-runtime-rex/main.go:108`); `args[1:]` ignored (`cmd/gc-runtime-rex/main.go:241`). Missing or empty `<name>` exits 1 with empty stdout (`cmd/gc-runtime-rex/main.go:142-145`).
- **Stdin ignored (`## is-running` §Registration, §Inputs):** `opIsRunning` ignores `stdin` (`cmd/gc-runtime-rex/main.go:240`).
- **Success output (`## is-running` §Success output):** `printBool` writes `true\n` or `false\n` to stdout and exits 0 (`cmd/gc-runtime-rex/main.go:175-177,240-247`).
- **Stopped/missing semantics (`## is-running` §Success output):** `Binder.Live` (`internal/provider/binding.go:282-327`) returns `false, nil` (exit 0 `false\n`) when no pane is found, when the pane vanished (`rex.ErrNotFound`), or when `info.LastExit != nil`.
- **Failure envelope (`## is-running` §Failure cases):** `runWithContext` buffers stdout and flushes it only on `err == nil`, so every exit 1 (state-dir error, Rex error, SIGTERM) leaves stdout empty (`cmd/gc-runtime-rex/main.go:153-160`).
- **Cancellation (`## Shared RPP contract` §Deadlines and cancellation):** `signal.NotifyContext` catches `SIGTERM`/`SIGINT` (`cmd/gc-runtime-rex/main.go:44`), cancelling the `rex` CLI subprocess (`internal/rex/cli/cli.go:37`) and exiting 1 with empty stdout.

## Unverified

- Whether any gc caller passes extra positional arguments beyond `<name>` to `is-running` (both herdr-core and `gc-runtime-rex` ignore `args[1:]` by inspection).
- Whether `rex` CLI stderr on an unreachable server can ever contain sensitive environment strings (cannot be settled from `rex-gas-city` Go code alone).
