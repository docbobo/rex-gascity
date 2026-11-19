# Method audit: `peek`

Verdict: `deviations`

## Deviations

| Doc claim | Current implementation | Behavioral impact | Severity |
|---|---|---|---|
| `## peek` § Failure cases: unreachable core exits `1` (`cannot read the city's core`); exit `66` only for a parked or empty city. | `internal/provider/provider.go:775-784` and `cmd/gc-runtime-rex/main.go:508-510` answer exit `66` (`no screen for '<name>'`) for `rex.ErrUnavailable` from `Resolve` or `BlockFormat`; other Rex errors exit `1` (`main.go:269-270`). `TestPeekNoScreenWhenRexUnavailable` (`dispatch_test.go:249-262`). | Unreachable Rex exits `66`, not `1`. Deliberate (`backlog/decisions/decision-2`): every no-screen cause answers `66`. gc v1.5.1 reads `66` as an ordinary error (`exec/exec.go:153-170`), same as `1`. | low |
| `## peek` § Failure cases: `lines` not an int (including `""`, `"abc"`, `"1.5"`) exits `1`, empty `stdout`. | `cmd/gc-runtime-rex/main.go:500` checks `if len(args) > 1 && args[1] != ""`, so `""` is treated as `lines = 0` (success) instead of failing. (`strconv.Atoi` also rejects `" 5"`, which Python `int()` accepts.) | `peek <name> ""` returns the full output instead of exit `1`. gc v1.5.1 always sends `strconv.Itoa(lines)` (`## peek` § Conflicts). | low |
| `## peek` § Success output: splits raw text on `\n`, keeps any trailing empty row in `[-lines:]`, and always appends `\n` (empty screen -> `"\n"`). | `internal/provider/provider.go:786-789` returns `BlockFormat` output verbatim when `lines <= 0` (empty screen -> `""`, no `\n` added); `lastLines` (`provider.go:793-804`) strips one trailing `\n` before splitting and preserves trailing `\n` only if present. | Empty screen emits `""` not `"\n"`; `"a\nb\n"` with `lines=1` emits `"b\n"` not `"\n"`. Invisible to gc v1.5.1, which trims trailing `\n` (`## peek` § Conflicts). | low |
| `## peek` § Registration, § Failure cases; `## Shared RPP contract` § Stdout caps: discards output and exits `1` above 1 MiB (`OBSERVE_STDOUT_CAP`). | `cmd/gc-runtime-rex/main.go:273` writes full `out.Bytes()` with no 1 MiB cap check (`BlockFormat` pages until EOF at `internal/rex/cli/cli.go:334-363`). | Outputs over 1 MiB succeed instead of failing; matches gc v1.5.1's uncapped buffering (`## peek` § Conflicts). | low |
| `## peek` § Registration, § Failure cases; `## Shared RPP contract` § Deadlines and cancellation: 30 s `default` deadline and `GC_EXEC_DEADLINE_MS` override (pre-handler `=0` -> exit `1`). | `cmd/gc-runtime-rex/main.go:108,256` wires `SIGTERM`/`SIGINT` to `ctx` but has no `GC_EXEC_DEADLINE_MS` parsing and no 30 s deadline timer. | `GC_EXEC_DEADLINE_MS` is ignored; long calls rely on gc's own timeout. | low |
| `## Shared RPP contract` § Failure envelope: `stdout` write failure returns exit `1` with no `stderr`. | `cmd/gc-runtime-rex/main.go:273-276` writes `gc-runtime-rex: peek: write output: <err>` to `stderr` before returning `1`. | Extra `stderr` line on `stdout` write failure; exit code (`1`) matches. | low |

## Matches

- **Registration & stdin (`## peek` § Registration):** Registered in `ops` as `"peek": {1, "peek <name> [lines]", opPeek}` (`cmd/gc-runtime-rex/main.go:185`); `opPeek` (`main.go:498`) ignores `stdin`.
- **Argument validation (`## peek` § Inputs, § Failure cases):** Missing or empty `<name>` exits `1` with empty `stdout` (`main.go:238-241`); non-integer `lines` such as `"many"` exits `1` (`main.go:501-505`, `dispatch_test.go:223`); extra args beyond `lines` are ignored (`main.go:498-506`).
- **Line selection (`## peek` § Inputs, § Success output):** `lines <= 0` (including omitted `lines` -> `0`) returns all output; `lines > 0` returns the last `N` lines (`provider.go:786-789`, `phase4_test.go:16-36`, `dispatch_test.go:213-223`).
- **Buffered failure & error propagation (`## peek` § Failure cases, `## Shared RPP contract` § Failure envelope):** `opPeek` buffers into `out` and writes nothing to `stdout` on error (`main.go:255-273`); Rex/state-dir errors and `SIGTERM`/`SIGINT` cancellation exit `1` with prefixed `stderr` (`main.go:108,269-271`).

- **Missing session and vanished pane (`## peek` § Success output, § Failure cases):** a name with no binding, a mid-call `rex.ErrNotFound`, or an unreachable Rex exits `66` with `no screen for '<name>'` on `stderr` and empty `stdout` (`provider.go:775-784`, `main.go:296,508-510`; `TestPeekNoScreenWhenNoRecord`, `TestPeekNoScreenWhenPaneVanished`, `TestPeekNoScreenWhenRexUnavailable`, `dispatch_test.go:236-262`). gc v1.5.1 has no exit-66 handling, so it reads this as an ordinary error (`exec/exec.go:153-170`); `RuntimeHandle.Peek` checks `IsRunning` first (`runtime_handle.go:187-190`), so the case rarely reaches the op (`backlog/decisions/decision-2`).

## Unverified

- Whether live Rex `block.format` (`internal/rex/cli/cli.go:334`) returns only the visible viewport or includes scrollback, and whether its returned text ends in `\n`.
- Behavior when `BlockFormat` returns >1 MiB from a live Rex server (no test exercises >1 MiB output).
- A live gc run in which the startup-dialog loop (`runtime/dialog.go:558-560`) peeks a vanished pane and now receives exit `66`; the reading comes from gc v1.5.1 source only. Post-v1.5.1 `gc` expectations for exit `66` or `GC_EXEC_DEADLINE_MS`.
