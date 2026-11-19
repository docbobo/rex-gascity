# Method Audit: `watch-startup`

**Verdict:** `not implemented`

## Deviations

| Doc claim | Current implementation | Behavioral impact | Severity |
|---|---|---|---|
| `watch-startup` registered (`## watch-startup > Registration`, `## The op table` #20) | Listed in `unimplemented` (`cmd/gc-runtime-rex/main.go:87-91`, line 90) and absent from `ops` (`cmd/gc-runtime-rex/main.go:99-116`); no `WatchStartup` method in `internal/provider/*.go` | `gc-runtime-rex watch-startup <name>` exits `2` (`exitUnknown`) with `gc-runtime-rex: operation "watch-startup" not implemented` on stderr and empty stdout (`cmd/gc-runtime-rex/main.go:141-149`). gc v1.5.1 treats exit `2` with no event as unsupported and falls back to `peek` polling + `send-keys` (`## watch-startup > Conflicts`). | low (gc v1.5.1 falls back to `peek`) |
| Accepts `watch-startup <name>`, validates non-empty `<name>` (missing/empty exits `1` `watch-startup: missing session name`), requires existing record (`no session 'X'` exits `1`), ignores extra args, runs `sweep_spawn_files` housekeeping (`## watch-startup > Inputs`, `## watch-startup > Failure cases`) | Returns `exitUnknown` (`2`) before arg validation (`cmd/gc-runtime-rex/main.go:150-153`), `NamespaceForCaller` state-dir lookup (`cmd/gc-runtime-rex/main.go:160`), or any Rex call | `gc-runtime-rex watch-startup` (missing `<name>`) and `gc-runtime-rex watch-startup ""` also exit `2` instead of `1`; no state dir or Rex server is touched. | low |
| Streams one compact ASCII NDJSON line `{"content":"<screen>"}\n` via `emit` on first read and on each screen-only (`lines:0`) change every 250 ms (`WATCH_POLL_S`), ending ~1 s before deadline with exit `0` (`## watch-startup > Success output`, `## Shared RPP contract > Success envelope`) | No streaming hook in `runWithContext` (handlers write to a buffered `bytes.Buffer` at `cmd/gc-runtime-rex/main.go:169-174`); no handler emits `{"content":...}` lines | No NDJSON screen stream is emitted; gc v1.5.1 loses the streaming optimization and polls `peek` when `ShouldAcceptStartupDialogs` is true (`## watch-startup > Conflicts`). | low (gc v1.5.1) |
| Enforces 120 s `start` deadline (or `GC_EXEC_DEADLINE_MS`), 1 s `budget` margin, `SIGTERM` cancellation (`exit 1` `canceled (terminated)`), and `BrokenPipeError` exit `1` on closed stdout (`## watch-startup > Registration`, `## watch-startup > Failure cases`, `## Shared RPP contract > Deadlines and cancellation`) | `cmd/gc-runtime-rex/main.go:46` wires `SIGTERM`/`SIGINT` via `signal.NotifyContext`, but has no `GC_EXEC_DEADLINE_MS` parsing, no 120 s deadline timer, and no streaming `emit` path | None for `watch-startup` (exits `2` immediately); gc v1.5.1 SIGKILLs `watch-startup` outright without `SIGTERM` and sets no `GC_EXEC_DEADLINE_MS` (`## watch-startup > Conflicts`). | low |
| Core unreachable on record read exits `1` and kicks core; mid-stream `HcError` exits `1` with `cannot read the screen: …` keeping prior emitted lines (`## watch-startup > Failure cases`) | No core-kick logic or `watch-startup` screen-read loop exists in `cmd/gc-runtime-rex/main.go` or `internal/provider/*.go` | None for `watch-startup` (always exits `2`). | low |

## Matches

- **Unknown/unimplemented op exit `2` without reading `stdin` (`## Shared RPP contract > Failure envelope > Unknown op`, `## watch-startup > Conflicts`):** `cmd/gc-runtime-rex/main.go:90,141-149` returns `exitUnknown` (`2`) with empty `stdout` and `gc-runtime-rex: operation "watch-startup" not implemented` on `stderr` before reading `stdin` or resolving `NamespaceForCaller`. `TestUnknownAndUnimplementedOps` (`cmd/gc-runtime-rex/dispatch_test.go:107-121`, line 113) pins this behavior for `watch-startup`.
- **gc v1.5.1 compatibility (`## watch-startup > Conflicts`):** Exiting `2` with empty `stdout` matches gc v1.5.1's `watch-startup` unsupported-op fallback to `peek` + `send-keys`, and `cmd/gc-runtime-rex/main.go:36` omits any streaming capability token from `handshake`. (`peek` and `send-keys` are registered in `ops` at `cmd/gc-runtime-rex/main.go:106,109`.)

## Unverified

- Runtime behavior of `hc` `terminal.read` on a finished or dead terminal, and whether a real screen's escaped JSON line can exceed gc v1.5.1's 64 KiB `bufio.NewScanner` default token limit (`## watch-startup > Unverified`).
- Whether any external wrapper or post-v1.5.1 gc release sets `GC_EXEC_DEADLINE_MS` or relies on streaming `watch-startup` instead of falling back to `peek` (`## watch-startup > Unverified`).
- Closed-pipe `emit` runtime behavior (`BrokenPipeError` in `rpp/perop.py`) and exact real-time end-of-stream behavior near the 120 s deadline margin (`## watch-startup > Unverified`).
