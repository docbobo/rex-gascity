# Provider method audit: `interrupt`

Verdict: deviations

## Deviations

| Doc claim | Current implementation | Behavioral impact | Severity |
|---|---|---|---|
| Nonexistent or dead session exits 80 with `{"outcome":"gone","detail":...}` on stdout (`§interrupt` Success output, Failure cases) | `opInterrupt` wraps `p.Interrupt` in `bestEffort` (`cmd/gc-runtime-rex/main.go:326-328,246-252`), mapping `ErrSessionNotFound` from `livePane` (missing pane or `paneAlive == false`) to exit 0 with empty stdout/stderr (`internal/provider/provider.go:926-928,946-955,960-976`; `internal/provider/liveness.go:155-158`; `TestInterruptReturnsInputOutcomes/missing_pane`, `TestInputOpsOnMissingSessionSucceed`) | Matches gc v1.5.1's best-effort contract, which the handshake asks for by not declaring `input-outcome` (gc `docs/reference/exec-session-provider.md:370`); callers expecting `input-outcome` cannot distinguish a missing/dead session from a delivered interrupt. Declaring the capability and answering typed `gone` is GCREX-70 | medium |
| Empty `<name>` (`""`) yields exit 80 `gone` (`§interrupt` Inputs, Failure cases) | `runWithContext` rejects `rest[0] == ""` with exit 1 (`exitFail`) and usage on stderr (`cmd/gc-runtime-rex/main.go:165-167`) | Empty session name fails with exit 1 instead of exit 80 `gone` | low |
| Handshake declares `carrier.ops` and `input-outcome` (`§interrupt` Registration) | `handshake` declares only `["report-attachment","report-activity"]` (`cmd/gc-runtime-rex/main.go:43`) | gc does not treat the provider as supporting typed `input-outcome` | low |
| `default` 30 s deadline, `GC_EXEC_DEADLINE_MS` override, 1 s margin, and `min(5 s, remaining - 1 s)` call timeout (`§interrupt` Registration, Inputs; `§Shared RPP contract` Deadlines) | `main` uses `signal.NotifyContext(context.Background(), ...)` with no timeout; `GC_EXEC_DEADLINE_MS` is not read; `p.Interrupt` sets no 5 s call timeout (`cmd/gc-runtime-rex/main.go:52`; `internal/provider/provider.go:925-941`) | Hung `rex` calls rely on gc's external 30 s process timeout rather than self-enforcing a 30 s / 5 s budget | low |
| Calls `hc` `terminal.interrupt` with a 3 s settle wait (`§interrupt` Inputs) | Sends `ctrl+c` keystroke via `rex send-key` with no settle wait (`internal/provider/provider.go:71,930`; `internal/rex/cli/cli.go:370-380`) | Relies on terminal line discipline / app handling `ctrl+c` rather than a backend interrupt call and 3 s wait | low |
| Redacts secrets/env values from stderr (`§Shared RPP contract` Failure envelope) | `runWithContext` writes `err` to stderr verbatim without redaction (`cmd/gc-runtime-rex/main.go:197`) | Sensitive tokens present in an underlying error message would leak to stderr | low |
| Exit-80 stderr carries the op name, `<op>: <word>[: detail]` (`§Shared RPP contract` Failure envelope; `§interrupt` Failure cases) | `writeInputOutcome` writes only `<word>[: detail]`, so `interrupt` emits `gone: …`, not `interrupt: gone: …` (`cmd/gc-runtime-rex/main.go:226-242`) | stderr omits herdr-core's op prefix; the stdout outcome line is unaffected | low |

## Matches

- **Registration & args**: Registered as `"interrupt"` requiring `<name>` (`cmd/gc-runtime-rex/main.go:124`); extra args after `<name>` are ignored (`main.go:165,326-328`).
- **Stdin**: Never read (`cmd/gc-runtime-rex/main.go:326-328`, `_ io.Reader`).
- **Success output**: A delivered interrupt exits 0 with empty stdout and empty stderr (`cmd/gc-runtime-rex/main.go:200-204,326-328`; `TestInterruptDeliveredExitsZero`).
- **Missing session (gc contract)**: A name no pane holds exits 0 with empty stdout and stderr, matching gc's "Best-effort interrupt/nudge" line (`docs/reference/exec-session-provider.md:370`), which is the contract the undeclared `input-outcome` handshake selects (`TestInterruptReturnsInputOutcomes/missing_pane`, `TestInputOpsOnMissingSessionSucceed`). The provider layer still classifies it as typed `gone` (`internal/provider/provider.go:946-955`); `bestEffort` consumes that at the dispatcher. herdr-core's typed `gone` is recorded under Deviations and is the GCREX-70 target.
- **Dead/finished program (gc contract)**: A pane whose program exited also exits 0 best-effort (`internal/provider/provider.go:960-976`; `TestInterruptReturnsInputOutcomes/dead_pane`). The typed `gone` classification stays at the provider layer for GCREX-70.
- **Core unreachable**: A Rex server that cannot be reached before sending exits 80 `unavailable`, from either the lookup or the send (`internal/provider/provider.go:950-951,935-936`; `TestInterruptUnreachableIsUnavailable`, `TestInterruptSendUnavailableIsUnavailable`, `TestInterruptReturnsInputOutcomes/server_unreachable`). GCREX-44 closed this deviation.
- **Other pre-send failure**: A lookup that fails for any other reason before the key is sent exits 80 `not_sent` (`internal/provider/provider.go:952-953`; `TestInterruptPreSendFailureIsNotSent`, `TestInterruptReturnsInputOutcomes/pre-send_failure`). GCREX-44 closed this deviation. This word is a deliberate choice: AC#2 allows `not_sent` or `unavailable`; the pack maps a list-call non-RPC `HcError` to `unavailable` (`§interrupt` Failure cases), while we answer `not_sent` because a lookup failure means no key was sent.
- **Unknown after write**: A send failure that may have written the request stays exit 1 with `interrupt <name>: …` on stderr and no stdout (`internal/provider/provider.go:930-938`; `TestInterruptSendFailureStaysUnknown`). Matches `§interrupt` Failure cases ("outcome unknown").
- **No activity note**: `p.Interrupt` does not call `p.noteInput` (unlike `SendKeys` at `internal/provider/provider.go:916-918`), matching `§interrupt` Success output (`internal/provider/provider.go:925-941`).
- **No retry**: `p.Interrupt` invokes `SendKeys` once with no retry (`internal/provider/provider.go:930`).
- **SIGTERM**: `SIGTERM`/`SIGINT` cancels `ctx` via `signal.NotifyContext` (`cmd/gc-runtime-rex/main.go:52`).
- **Tests**: Covered by `TestInterruptSendsCtrlC`, `TestInterruptUnknownSessionFails`, `TestInterruptExitedPaneFails`, `TestInterruptMissingPaneIsGone`, `TestInterruptDeadPaneIsGone`, `TestInterruptUnreachableIsUnavailable`, `TestInterruptPreSendFailureIsNotSent`, `TestInterruptSendUnavailableIsUnavailable`, `TestInterruptSendFailureStaysUnknown` (`internal/provider/input_test.go`), `TestInterruptReturnsInputOutcomes`, `TestInterruptDeliveredExitsZero`, `TestInputOpsReachPane` (`cmd/gc-runtime-rex/dispatch_test.go`).

## Unverified

- Whether any gc caller passes extra argv after `<name>` (`§interrupt` Unverified).
- Whether `rex send-key ... ctrl+c` delivers `SIGINT` via PTY or raw `\x03` / key event inside Rex itself (cannot be settled from this repo's Go code).
- Which gc release (beyond v1.5.1) first consumes exit 80 / `input-outcome` (`§interrupt` Unverified).
- Whether the Rex CLI can distinguish a request that was written from one that was not; the adapter treats every non-`ErrUnavailable` send failure as outcome unknown.
