# Method Audit: `is-attached`

**Verdict:** `deviations`

## Deviations

| Doc claim | Current implementation | Behavioral impact | Severity |
|---|---|---|---|
| Attachment = local attach lockfile OR `/proc` viewer process (`hc attach` / `herdr-core attach`), never querying the server; finished records with a held lock/viewer return `true` (`## is-attached` § Success output) | Queries Rex server: `p.binder.Resolve` (`internal/provider/attach.go:19`) returns `false, nil` if missing (`:23-25`); `p.client.ListClients` (`:26`) checks client `Kind` (`"app"` sets `hasApp`, `"cli"`/`"peer"` ignored, unknown `Kind` returns `true` at `:36-42`); `p.client.SessionView` (`:48`) checks `ActiveWindowID == WindowID` and block presence (`:60-68`). | Server-backed check instead of local lock/`/proc`; gone/finished panes return `false` rather than `true`; unclassifiable client kinds fail-closed (`true`). Intentional for Rex; gc only reads `true`/`false`. | low |
| `deadline="probe"` (4.0 s) or `GC_EXEC_DEADLINE_MS` enforced from process start; expired budget exits 1 (`## is-attached` § Registration, § Failure cases; `## Shared RPP contract` § Deadlines and cancellation) | `main` wires `signal.NotifyContext(context.Background(), ...)` (`cmd/gc-runtime-rex/main.go:45`) with no timeout; neither `main.go` nor `internal/provider` reads `GC_EXEC_DEADLINE_MS` or sets a 4.0 s deadline. | Under gc v1.5.1 (30 s default, no `GC_EXEC_DEADLINE_MS`), relies on gc's 30 s timeout and Rex CLI timeouts rather than a 4 s self-deadline. | low |
| Unreachable core returns exit 0 `false\n` when `parked(ctx) and nothing_runs(ctx)`, otherwise exits 1 and kicks a core start (`## is-attached` § Inputs, § Failure cases) | No parked-city check or core auto-start. Transport errors in `Resolve`, `ListClients`, or `SessionView` (`internal/provider/attach.go:19-52`) propagate to `runWithContext` (`cmd/gc-runtime-rex/main.go:161-163`) and exit 1. | Unreachable Rex server always exits 1 (which gc v1.5.1 maps to `false`), even when parked, and never starts Rex. | low |
| Supports `GC_RPP_FAULT=unavailable`, `GC_EXEC_CITY_PATH`, `GC_HERDR_CORE_DEBUG=1`, stderr redaction, and 64 KiB stdout cap (`## is-attached` § Registration, § Failure cases; `## Shared RPP contract`) | None of these env vars, redaction, or stdout cap checks exist (`cmd/gc-runtime-rex/main.go:154-169`, `internal/provider/statedir.go:72-86`). | No impact on gc v1.5.1 (`true\n`/`false\n` is 5–6 bytes; gc v1.5.1 does not set these env vars). Fault-injection harnesses using `GC_RPP_FAULT` will not trigger. | low |

## Matches

- **Registration & capability (`## is-attached` § Registration):** Registered in `ops` as `"is-attached": {1, "is-attached <name>", opIsAttached}` (`cmd/gc-runtime-rex/main.go:114`). `handshake` declares `"report-attachment"` (`cmd/gc-runtime-rex/main.go:35`).
- **Inputs & stdin (`## is-attached` § Registration, § Inputs):** Requires non-empty `<name>` at `args[0]` (`cmd/gc-runtime-rex/main.go:114,145-148`); missing or empty arg exits 1 with empty stdout and prefixed stderr (`:145-148`). Extra args are ignored (`:342`); `stdin` is never read (`:341`).
- **Success output (`## is-attached` § Success output):** Attached writes `true\n` (exit 0); not attached or unknown/missing session writes `false\n` (exit 0) (`cmd/gc-runtime-rex/main.go:179-181,341-348`, `internal/provider/attach.go:23-25`). Covered by `TestIsAttached` (`cmd/gc-runtime-rex/dispatch_test.go:298-314`) and `internal/provider/attach_test.go:42-104`.
- **Failure envelope & signals (`## is-attached` § Failure cases, `## Shared RPP contract`):** Errors leave stdout empty, write `gc-runtime-rex: is-attached: <err>` to stderr, and exit 1 (`cmd/gc-runtime-rex/main.go:154-169`). `SIGTERM`/`SIGINT` cancels the op context (`cmd/gc-runtime-rex/main.go:45`).

## Unverified

- Whether any post-v1.5.1 `gc` version sends `GC_EXEC_DEADLINE_MS`, enforces a 4 s `probe` timeout, or uses `GC_RPP_FAULT` / `GC_EXEC_CITY_PATH` (absent from pinned gc v1.5.1).
- Whether `gc` v1.5.1's `stop` path ever calls `is-attached` (noted as unverified in `docs/provider-herdr-method-io.md` § `is-attached` § Unverified).
