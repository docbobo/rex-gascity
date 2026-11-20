# Method audit: `protocol`

Verdict: `deviations`

## Deviations

| Doc claim | Current implementation | Behavioral impact | Severity |
|---|---|---|---|
| `## protocol` § Success output: declares 10 capabilities (`observe.v1`, `report-attachment`, `report-activity`, `city-scoped`, `conformance-faults`, `server-lifecycle`, `carrier.ops`, `idle-boundary`, `input-outcome`, `fs.host`). | `cmd/gc-runtime-rex/main.go:50` hardcodes 5 capabilities: `["report-attachment","report-activity","idle-boundary","carrier.ops","input-outcome"]`. | gc 1.5.1 ignores the non-baseline tokens; the pinned edge build consumes `idle-boundary`, `carrier.ops` and `input-outcome`. The remaining 5 tokens (`observe.v1`, `city-scoped`, `conformance-faults`, `server-lifecycle`, `fs.host`) stay undeclared because their behavior is not implemented. | low |
| `## protocol` § Success output: includes `"locality":"local"` in handshake JSON. | `cmd/gc-runtime-rex/main.go:39` omits `locality` (pinned by `cmd/gc-runtime-rex/dispatch_test.go:93-95`). | None under gc v1.5.1 (`ProtocolInfo` drops unknown fields per `## protocol` § Conflicts 2). A newer gc reading `locality` (e.g. for `fs.host`) will not see `"local"`. | low |
| `## protocol` § Failure cases & `## Shared RPP contract` § Deadlines and cancellation: `GC_EXEC_DEADLINE_MS` overrides deadline (`=0` fails before handler) and pre-handler `SIGTERM` fails with exit 1. | `cmd/gc-runtime-rex/main.go:138-144` does not read `GC_EXEC_DEADLINE_MS` and does not check `ctx.Err()` before writing `handshake`, so it still writes stdout and exits `0`. | `GC_EXEC_DEADLINE_MS=0` or a `SIGTERM` delivered before dispatch does not fail `protocol`. gc v1.5.1 does not send `GC_EXEC_DEADLINE_MS` and treats canceled calls as canceled regardless of exit code. | low |
| `## Shared RPP contract` § Failure envelope: stdout write failure returns exit 1 immediately with no stderr. | `cmd/gc-runtime-rex/main.go:139-142` writes `gc-runtime-rex: protocol: write output: <err>` to `stderr` before returning `exitFail` (`1`) (tested in `cmd/gc-runtime-rex/dispatch_test.go:420-426`). | Extra diagnostic line on `stderr` when `stdout` write fails; exit code (`1`) matches. | low |

## Matches

- **No backend/state initialization (`## protocol` § Registration, § Inputs):** `cmd/gc-runtime-rex/main.go:138-144` short-circuits `op == "protocol"` before `NamespaceForCaller()` (`main.go:163`) and `provider.New(...)` (`main.go:168`), so no state dir, city lookup, or Rex client is touched.
- **CLI arguments & stdin (`## protocol` § Inputs):** Requires no session name, ignores extra positional args (`main.go:136,138`), and never reads `stdin` (`main.go:138-144`). Missing op (`len(args) == 0`) exits `2` (`main.go:132-135`); unknown ops exit `2` without reading `stdin` (`main.go:145-153`, `dispatch_test.go:107-120`).
- **Success envelope & JSON shape (`## protocol` § Success output):** Exits `0` (`main.go:143`), writes one compact ASCII JSON line + `\n` (`main.go:39,139`), leaves `stderr` empty, sets `"version":0`, and omits `proc.exec`, `proc.provision`, `input-fence`, `tty.attach`, and `proc.stream` (`dispatch_test.go:83-105`).
- **Pre-encoded static response (`## protocol` § Registration):** `handshake` is a compile-time constant (`main.go:39`), not constructed per call, and is 72 bytes with `\n` (well below the 64 KiB cap).

## Unverified

- Whether any gc release newer than `v1.5.1` consumes `locality` or the 8 omitted capability tokens (`observe.v1`, `city-scoped`, `conformance-faults`, `server-lifecycle`, `carrier.ops`, `idle-boundary`, `input-outcome`, `fs.host`), or sends `GC_EXEC_DEADLINE_MS`.
- Behavior when `gc-runtime-rex` fails to start as an OS process (e.g. missing binary or dynamic-link failure) before `main()` runs.
