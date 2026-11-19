# Method audit: `get-last-activity`

Verdict: `deviations`

## Deviations

| Doc claim | Current implementation | Behavioral impact | Severity |
|---|---|---|---|
| `## get-last-activity` § Success output: outputs bare RFC 3339 UTC with milliseconds (`%Y-%m-%dT%H:%M:%S.mmmZ`). | `cmd/gc-runtime-rex/main.go:329` formats with `time.RFC3339` (whole seconds, e.g. `2026-11-17T18:00:00Z\n`; pinned in `cmd/gc-runtime-rex/dispatch_test.go:288`). | Sub-second precision is truncated. gc v1.5.1 parses `time.RFC3339`, which accepts both whole-second and fractional formats. | low |
| `## get-last-activity` § Inputs, § Failure cases: requires an intact/live record; a finished or dead box exits `0` with empty stdout. | `internal/provider/activity.go:54` uses `p.binder.Resolve` (`internal/provider/binding.go:330-357`), which checks `SessionView` membership (`stillBound`) without checking `BlockProcess` / `LastExit != nil` (unlike `Binder.Live` at `binding.go:258-296`). | If a pane's process exited (`LastExit != nil`) while its Rex block remains in `SessionView`, `GetLastActivity` still queries status/screen and returns a timestamp instead of empty stdout. | low |
| `## get-last-activity` § Success output: reported non-`working`/`blocked` pane uses `max(status_ms or created_ms, input_ms)` with no screen read. | `internal/provider/activity.go:71-73` requires `!latest.IsZero()`; a `program_status` record with zero `UpdatedAt` falls through to `BlockFormat` screen polling (`internal/provider/activity_test.go:132-146`) rather than using `binding.CreatedAt`. | Extra `BlockFormat` call and `"now"`/`format_poll` stamp instead of `CreatedAt` when `UpdatedAt` is zero. | low |
| `## get-last-activity` § Failure cases: unreachable core in a parked city (`server-stop`) exits `0` empty; in a non-parked city exits `1` and kicks a start. | `cmd/gc-runtime-rex/main.go:102-119` has no `server-stop`; `internal/provider/activity.go:54-57` returns any `Resolve` error and `cmd/gc-runtime-rex/main.go:171-174` exits `1` without kicking a start. | Unreachable Rex always exits `1` with empty stdout. Under gc v1.5.1, `Observe` discards `GetLastActivity` errors. | low |
| `## get-last-activity` § Registration & `## Shared RPP contract`: enforces `GC_EXEC_DEADLINE_MS` / 30 s deadline, 64 KiB stdout cap, stderr redaction, and silent stderr on stdout write failure. | `cmd/gc-runtime-rex/main.go:47-48,162-179` does not read `GC_EXEC_DEADLINE_MS`, set a 30 s `ctx` deadline, check a 64 KiB cap, or redact stderr; stdout write failure writes to stderr before exiting `1` (`main.go:175-178`). | `GC_EXEC_DEADLINE_MS=0` is ignored (gc v1.5.1 does not send it and enforces its own 30 s timeout); 64 KiB cap is unreachable for a one-line timestamp. | low |

## Matches

- **Registration & capability (`## get-last-activity` § Registration):** Registered in `cmd/gc-runtime-rex/main.go:113`; `handshake` declares `"report-activity"` (`cmd/gc-runtime-rex/main.go:39`).
- **Inputs & stdin (`## get-last-activity` § Inputs):** Accepts `get-last-activity <name>`, ignores `stdin` and extra `args[1:]` (`cmd/gc-runtime-rex/main.go:323-324`), and exits `1` with empty stdout on missing or empty `<name>` (`cmd/gc-runtime-rex/main.go:154-157`, `cmd/gc-runtime-rex/dispatch_test.go:128`).
- **Missing pane (`## get-last-activity` § Success output, § Failure cases):** Unbound session returns `ActivityNone` and zero `At` (`internal/provider/activity.go:58-60`), producing exit `0` and empty stdout (`cmd/gc-runtime-rex/main.go:328-331`, `cmd/gc-runtime-rex/dispatch_test.go:291-293`).
- **Activity precedence & input floor (`## get-last-activity` § Success output):** Prefers `program_status`, maps `working`/`blocked` to `now` (`internal/provider/activity.go:68-70`, `internal/provider/activity_test.go:148-172`), falls back to `BlockFormat` sha256 fingerprinting (`activity.go:34-37,75-90`, `activity_test.go:41-83`), applies same-incarnation `noteInput` floor (`activity.go:67,70,73,81,89,112-135`, `activity_test.go:174-200`), and isolates records by `BlockID` (`activity_test.go:240-269`).
- **Backend & store error handling (`## get-last-activity` § Failure cases):** Errors from `BlockProgramStatus` or `BlockFormat` (including mid-call `ErrNotFound`) exit `1` with empty stdout (`activity.go:62-65,75-78`, `main.go:171-174`, `dispatch_test.go:294-295`); `fileActivityStore` load/save errors are swallowed and fall back to unpersisted timestamps (`internal/provider/stores.go:144-154`); `SIGTERM`/`SIGINT` cancels `ctx` (`main.go:47-48`).

## Unverified

- Whether any caller sends extra argv beyond `<name>` (ignored by `opGetLastActivity`, `cmd/gc-runtime-rex/main.go:324`).
- Concurrent `get-last-activity` calls for one session name across separate `gc-runtime-rex` processes (`fileTable` uses an in-process `sync.Mutex` and atomic rename at `internal/provider/filestore.go:24-48,79-99`, with no cross-process `flock`).
- Live Rex server behavior when a pane exits (`LastExit != nil`) vs block removal from `SessionView`, and live `BlockProgramStatus`/`BlockFormat` error behavior when a block vanishes mid-call.
