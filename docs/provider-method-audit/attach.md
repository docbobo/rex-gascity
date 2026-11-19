# Method Audit: `attach`

**Verdict:** `deviations`

## Deviations

| Doc claim | Current implementation | Behavioral impact | Severity |
|---|---|---|---|
| `## attach` § Success output: `os.execve(hc_bin, [hc_bin, "attach", record.address], env)` replaces the process and blocks as an interactive TTY viewer (`hc attach`); writes nothing itself and exits with `hc`'s exit code. | `cmd/gc-runtime-rex/main.go:233-241` (`opAttach`, `attachHint`) writes a one-line hint (`Rex has no in-terminal attach; open the Rex app and select the session "<name>" to attach\n`) to `stdout` and returns `nil` (exit `0` at `main.go:180`). | Deliberate divergence from the herdr-core pack: Rex has no in-terminal attach. Matches gc v1.5.1's `Attach` contract ("tty passthrough", `docs/provider-herdr-parity.md:851,856`) better than exiting `2`, because gc v1.5.1 runs `attach` via `runWithTTY` without mapping exit `2` to success (`gc session attach` would fail with `exit status 2`). | info |
| `## attach` § Inputs & § Failure cases: requires a live, intact record for `<name>`, an executable pinned `hc`, and a responding core (`hello()`), exiting `1` on unknown/dead session or core down. | `cmd/gc-runtime-rex/main.go:233-236` never calls `Provider` or Rex (`p` is `_`); `TestAttachSucceedsWithHint` (`cmd/gc-runtime-rex/dispatch_test.go:328-353`) pins exit `0` with the hint for a live pane, an unknown session, and an unreachable Rex client (`downRex`). | An unknown session or unreachable Rex server exits `0` with the hint instead of failing `gc session attach`. Note `runWithContext` still calls `provider.NamespaceForCaller()` (`main.go:164-168`) before `opAttach`, so an invalid state dir / missing city path exits `1` (`state dir: ...`) even though `p` is unused. | low |
| `## attach` § Success output: holds a shared `flock` on the incarnation's attach lock across `execve` so `is-attached` can detect attached viewers. | `cmd/gc-runtime-rex/main.go:233-236` takes no lock. `internal/provider/attach.go:18-73` (`IsAttached`, `watchesAgent`) probes viewer attachment server-side via `ListClients` and `SessionView` instead of a lock file. | None: intentional per `docs/provider-herdr-parity.md:853` because Rex tracks connected app clients server-side. | info |
| `## attach` § Registration & `## Shared RPP contract` § Deadlines and cancellation: `start` deadline (120 s) overridden by `GC_EXEC_DEADLINE_MS` (fails pre-handler when expired), `housekeeping=True` spawn-file sweep, and `SMALL_STDOUT_CAP` (64 KiB) check. | `cmd/gc-runtime-rex/main.go:131-180` (`runWithContext`) does not read `GC_EXEC_DEADLINE_MS`, applies no 120 s timer, runs no spawn-file sweep, and enforces no 64 KiB stdout cap (signal cancellation only via `signal.NotifyContext` at `main.go:46`). | None under gc v1.5.1: gc never sets `GC_EXEC_DEADLINE_MS`, `runWithTTY` applies no deadline to `attach` (`docs/provider-herdr-parity.md:854`), and `attachHint` is ~90–120 bytes (well under 64 KiB). | info |
| `## Shared RPP contract` § Failure envelope: stdout write failure returns exit `1` immediately with no stderr. | `cmd/gc-runtime-rex/main.go:176-179` writes `gc-runtime-rex: attach: write output: <err>` to `stderr` before returning `exitFail` (`1`). | Extra diagnostic line on `stderr` when `stdout` write fails; exit code (`1`) matches. | low |

## Matches

- **Op registration & arity (`## The op table` #21, `## attach` § Registration, § Inputs):** `attach` is registered in `ops` at `cmd/gc-runtime-rex/main.go:104` with `required: 1` (`"attach <name>"`). Missing `<name>` or empty `""` exits `1` with `missing argument; usage: gc-runtime-rex attach <name>` on `stderr` and empty `stdout` (`main.go:154-157`).
- **Stdin untouched (`## attach` § Registration, § Inputs):** `opAttach` (`main.go:233`) ignores `stdin`, matching `reads_stdin=False` in the op table.
- **Capability omitted (`## attach` § Registration, § Conflicts):** `handshake` (`cmd/gc-runtime-rex/main.go:39`) omits `tty.attach`, matching both the herdr-core pack and gc v1.5.1 (where `tty.attach` is reserved and `Attach` never checks it, `docs/provider-herdr-parity.md:852`).
- **Dispatcher-only implementation (`internal/provider/attach.go`):** `internal/provider/` has no `Attach` method; `internal/provider/attach.go:18-73` implements only `IsAttached` and `watchesAgent` for `is-attached`.
- **Tests (`cmd/gc-runtime-rex/dispatch_test.go:328-353`):** `TestAttachSucceedsWithHint` verifies exit `0`, hint on `stdout`, and empty `stderr` across live, unknown-session, and unreachable-Rex cases. (`TestMissingNameIsFailure` at `dispatch_test.go:123-137` omits `{"attach"}` from its table, though the shared check at `main.go:154-157` applies.)

## Unverified

- Whether any gc release newer than `v1.5.1` checks `tty.attach` before calling `attach`, sends `GC_EXEC_DEADLINE_MS`, or treats a non-TTY hint exit `0` differently.
- Behavior of `hc attach` itself (no-TTY handling, raw mode, `SIGWINCH`, detach key, exit codes), as `hc` source is outside this repository (`## attach` § Unverified).
