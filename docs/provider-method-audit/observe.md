# Method Audit: `observe`

Verdict: not implemented

## Deviations

| Doc claim | Current implementation | Behavioral impact | Severity |
|---|---|---|---|
| `observe` registered (`## observe > Registration`, `## The op table` #5) and `observe.v1` declared in handshake (`## observe > Registration`) | Absent from `ops` (`cmd/gc-runtime-rex/main.go:97-113`) and `unimplemented` (`cmd/gc-runtime-rex/main.go:82-86`); `handshake` declares only `report-attachment` and `report-activity` (`cmd/gc-runtime-rex/main.go:36`); no `Observe` method in `internal/provider/*.go` | `gc-runtime-rex observe <name>` exits 2 (`exitUnknown`) with `gc-runtime-rex: unknown operation "observe"` on stderr (`cmd/gc-runtime-rex/main.go:139-145`) | low (gc v1.5.1 has no `observe`) / medium (for newer gc expecting `observe.v1`) |
| Accepts `observe <name>`, reads stdin as newline-separated process names, runs under per-name lock (`## observe > Inputs`) | Returns `exitUnknown` (2) before arg validation (`cmd/gc-runtime-rex/main.go:147-150`), state dir lookup (`cmd/gc-runtime-rex/main.go:157`), or any stdin read | `stdin` is untouched; even `gc-runtime-rex observe` (missing `<name>`) exits 2 instead of 1 | low |
| Exits 0 with one compact ASCII JSON line (`absent`, `unknown`, `dead`, or `running` with `object`, `identity`, `agent`, `agent_pid`) (`## observe > Success output`) | No handler exists; stdout is always empty on exit 2 (`cmd/gc-runtime-rex/main.go:139-145,163-166`) | No JSON observation emitted; gc v1.5.1 instead folds `get-last-activity`, `process-alive`, and `is-attached` (`cmd/gc-runtime-rex/main.go:105,108,109`) | low (gc v1.5.1) / medium (newer gc) |
| Enforces 4 s `probe` deadline, `GC_EXEC_DEADLINE_MS`, 1 s stdin-open margin, and 1 MiB `OBSERVE_STDOUT_CAP` (`## observe > Registration`, `## Shared RPP contract > Deadlines and cancellation`, `## Shared RPP contract > Stdout caps`) | Not implemented in `cmd/gc-runtime-rex/main.go` (uses `signal.NotifyContext` for SIGTERM/SIGINT at `cmd/gc-runtime-rex/main.go:46`; no `GC_EXEC_DEADLINE_MS` or stdout cap) | None for `observe` (exits 2 immediately); broader dispatcher relies on gc SIGTERM/SIGKILL | low |
| `GC_RPP_FAULT=unavailable` / down core returns exit 0 `{"state":"unknown"}` and kicks core; parked core returns `{"state":"absent"}` (`## observe > Inputs`, `## observe > Failure cases`) | No `GC_RPP_FAULT` fault injection or daemon-kick logic in `cmd/gc-runtime-rex/main.go` or `internal/provider/*.go` | None for `observe` (always exits 2) | low |

## Matches

- **Unknown-op exit 2 without reading stdin** (`## Shared RPP contract > Failure envelope > Unknown op`): `cmd/gc-runtime-rex/main.go:139-145` returns `exitUnknown` (`2`) with empty stdout and `<op>: unknown operation` on stderr before reading stdin or resolving `NamespaceForCaller` (`cmd/gc-runtime-rex/main.go:157`). `TestUnknownAndUnimplementedOps` (`cmd/gc-runtime-rex/dispatch_test.go:108-120`) covers this path for other unknown/unimplemented ops.
- **RPP v0 handshake** (`## observe > Conflicts`, `## Shared RPP contract > Inherited from gc v1.5.1`): `cmd/gc-runtime-rex/main.go:36,133-138` omits `observe.v1`, matching gc v1.5.1's capability table, and exits 2 on `observe`, which gc v1.5.1 treats as unimplemented/success.
- **v1.5.1 `Observe` constituent ops**: The three ops gc v1.5.1 folds for `Observe` (`process-alive`, `get-last-activity`, `is-attached`) are registered in `ops` (`cmd/gc-runtime-rex/main.go:105,108,109`).

## Unverified

- Newer gc `observe.v1` contract details (request/response schema, 4 s probe timeout, 1 MiB cap, stdin trailing-newline handling, and gc's behavior on non-zero exit, empty stdout, or malformed JSON), because gc v1.5.1 has no `observe` op or `observe.v1` capability (`## observe > Unverified`).
- Whether any newer gc release sends `GC_EXEC_DEADLINE_MS` or how gc behaves on 4 s probe timeout kill timing (`## observe > Unverified`).
