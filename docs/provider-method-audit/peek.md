# Method audit: `peek`

Verdict: `deviations`

## Deviations

| Doc claim | Current implementation | Behavioral impact | Severity |
|---|---|---|---|
| `## peek` § Success output, § Failure cases: missing record, hc `not_found`, or unreachable core with parked empty city exits `66` (`no screen for '<name>'`), empty `stdout`. | `internal/provider/provider.go:609-619` returns `"", nil` when `Resolve` finds no binding or `BlockFormat` returns `rex.ErrNotFound`, so `cmd/gc-runtime-rex/main.go:307-311` exits `0` with empty `stdout` and `stderr` (`dispatch_test.go:224-226,533-535`). Unreachable Rex always exits `1` (`main.go:171-174`). | Missing/vanished pane succeeds silently instead of failing with `66`; under gc v1.5.1, exit `0` empty avoids an error (`## peek` § Conflicts). | medium |
| `## peek` § Failure cases: `lines` not an int (including `""`, `"abc"`, `"1.5"`) exits `1`, empty `stdout`. | `cmd/gc-runtime-rex/main.go:300-306` checks `if len(args) > 1 && args[1] != ""`, so `""` is treated as `lines = 0` (success) instead of failing. (`strconv.Atoi` also rejects `" 5"`, which Python `int()` accepts.) | `peek <name> ""` returns the full output instead of exit `1`. gc v1.5.1 always sends `strconv.Itoa(lines)` (`## peek` § Conflicts). | low |
| `## peek` § Success output: splits raw text on `\n`, keeps any trailing empty row in `[-lines:]`, and always appends `\n` (empty screen -> `"\n"`). | `internal/provider/provider.go:612-638` returns `BlockFormat` output verbatim when `lines <= 0` (empty screen -> `""`, no `\n` added); `lastLines` strips one trailing `\n` before splitting and preserves trailing `\n` only if present. | Empty screen emits `""` not `"\n"`; `"a\nb\n"` with `lines=1` emits `"b\n"` not `"\n"`. Invisible to gc v1.5.1, which trims trailing `\n` (`## peek` § Conflicts). | low |
| `## peek` § Registration, § Failure cases; `## Shared RPP contract` § Stdout caps: discards output and exits `1` above 1 MiB (`OBSERVE_STDOUT_CAP`). | `cmd/gc-runtime-rex/main.go:175` writes full `out.Bytes()` with no 1 MiB cap check (`BlockFormat` pages until EOF at `internal/rex/cli/cli.go:327-350`). | Outputs over 1 MiB succeed instead of failing; matches gc v1.5.1's uncapped buffering (`## peek` § Conflicts). | low |
| `## peek` § Registration, § Failure cases; `## Shared RPP contract` § Deadlines and cancellation: 30 s `default` deadline and `GC_EXEC_DEADLINE_MS` override (pre-handler `=0` -> exit `1`). | `cmd/gc-runtime-rex/main.go:46,163-179` wires `SIGTERM`/`SIGINT` to `ctx` but has no `GC_EXEC_DEADLINE_MS` parsing and no 30 s deadline timer. | `GC_EXEC_DEADLINE_MS` is ignored; long calls rely on gc's own timeout. | low |
| `## Shared RPP contract` § Failure envelope: `stdout` write failure returns exit `1` with no `stderr`. | `cmd/gc-runtime-rex/main.go:175-178` writes `gc-runtime-rex: peek: write output: <err>` to `stderr` before returning `1`. | Extra `stderr` line on `stdout` write failure; exit code (`1`) matches. | low |

## Matches

- **Registration & stdin (`## peek` § Registration):** Registered in `ops` as `"peek": {1, "peek <name> [lines]", opPeek}` (`cmd/gc-runtime-rex/main.go:111`); `opPeek` (`main.go:298`) ignores `stdin`.
- **Argument validation (`## peek` § Inputs, § Failure cases):** Missing or empty `<name>` exits `1` with empty `stdout` (`main.go:154-157`); non-integer `lines` such as `"many"` exits `1` (`main.go:301-304`, `dispatch_test.go:223`); extra args beyond `lines` are ignored (`main.go:298-306`).
- **Line selection (`## peek` § Inputs, § Success output):** `lines <= 0` (including omitted `lines` -> `0`) returns all output; `lines > 0` returns the last `N` lines (`provider.go:621-624`, `phase4_test.go:16-36`, `dispatch_test.go:213-221`).
- **Buffered failure & error propagation (`## peek` § Failure cases, `## Shared RPP contract` § Failure envelope):** `opPeek` buffers into `out` and writes nothing to `stdout` on error (`main.go:170-175`); Rex/state-dir errors and `SIGTERM`/`SIGINT` cancellation exit `1` with prefixed `stderr` (`main.go:46,163-174`).

## Unverified

- Whether live Rex `block.format` (`internal/rex/cli/cli.go:334`) returns only the visible viewport or includes scrollback, and whether its returned text ends in `\n`.
- Behavior when `BlockFormat` returns >1 MiB from a live Rex server (no test exercises >1 MiB output).
- Exact gc v1.5.1 call-site gating (`is-running` before `peek`) and post-v1.5.1 `gc` expectations for exit `66` or `GC_EXEC_DEADLINE_MS`.
