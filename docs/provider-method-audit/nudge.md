# Method Audit: `nudge`

Verdict: `deviations`

Audited against `docs/provider-herdr-method-io.md` (`## nudge` and `## Shared RPP contract`).

## Deviations

| Doc claim | Current implementation | Behavioral impact | Severity |
|---|---|---|---|
| Missing or dead box exits 80 with `{"outcome":"gone",...}` (`## nudge` §Inputs, §Failure cases, §Conflicts). | `NudgeReport` resolves the pane with `resolveInputPane`, which calls `livePane` (`internal/provider/provider.go:788-804`). `classifyInput` (`internal/provider/input.go:134`) turns `ErrSessionNotFound` into a typed `gone` outcome, and `handlerExitCode` (`cmd/gc-runtime-rex/main.go:318`) renders exit 80. gc reads it because the handshake declares `input-outcome` and `carrier.ops` (GCREX-70). | Matches the pack; gc projects `gone` to `runtime.ErrSessionNotFound`. | low |
| Empty `<name>` exits 80 `gone` (`## nudge` §Inputs, §Failure cases). | `runWithContext` (`cmd/gc-runtime-rex/main.go:135,159`) rejects a missing or empty `<name>` with exit 1 and empty stdout. | Callers get exit 1 usage error instead of exit 80 `gone`. | low |
| `GC_EXEC_DEADLINE_MS` sets op deadline; send reserves 5 s before deadline (`## nudge` §Inputs, §Failure cases). | `GC_EXEC_DEADLINE_MS` is not read (`main.go:135-174`). `nudgeSubmitContext` (`internal/provider/nudge.go:474-489`) caps write+submit to `defaultNudgeSubmitBudget` (10 s) or `ctx.Deadline()`; confirmation uses `defaultNudgeConfirmWindow` (3 s, `nudge.go:92,418`). | Custom deadline env var is ignored; write+submit and confirmation use fixed budgets. | low |
| Empty stdin reaches handler as empty text and still checks target box (`## nudge` §Inputs). | `NudgeReport` (`internal/provider/nudge.go:214-216`) returns `nil` immediately when `text == ""` before resolving the pane. | Empty stdin exits 0 without checking whether the pane exists. | low |
| No pool-claim precondition in doc (`## nudge` §Inputs). | `nudgeReportBinding` (`internal/provider/nudge.go:354-360`) refuses `defaultPoolClaimNudge` / `poolClaimNudgeSignature` with `ErrNudgeClaimHeld` (exit 80 `busy`, GCREX-69) when `claimProbe` reports a live claim. | Prevents re-prompting a busy pool seat with `gc hook --claim`; the `busy` word makes gc's stalled-claim backstop refund the reservation instead of re-dispatching the held claim. Intentional Rex extension (GCREX-33). | info |

## Matches

- **Registration & stdin:** `nudge` is registered in `ops` (`cmd/gc-runtime-rex/main.go:109`) and reads full stdin (`main.go:290-293`).
- **Pre-send UTF-8 validation:** `NudgeReport` calls `checkNudgeText` (`internal/provider/nudge.go:233`), which answers a `rejected` input outcome before any Rex call; `handlerExitCode` renders exit 80 (`main.go:318`; `dispatch_test.go` `TestNudgeRejectsInvalidUTF8`; GCREX-45).
- **Busy and blocked outcomes:** a `working` pane answers exit 80 `busy` and a `blocked` pane exit 80 `pending_interaction` (kind named), typing nothing (`internal/provider/nudge.go:356-372`; GCREX-45).
- **Failed-write and failed-submit outcomes:** a write or submit that cannot land answers exit 80 `unavailable` when the server is unreachable and `not_sent` otherwise (`nudge.go:322-341,394-413`; GCREX-45).
- **Delivered success envelope:** Confirmed delivery exits 0 with empty stdout and empty stderr (`main.go:313-317`).
- **`delivered_unobserved` success line:** an unconfirmed delivery exits 0 with one `{"outcome":"delivered_unobserved","detail":...}` line and keeps the diagnostic file (`main.go:318-320,322-333`; GCREX-45).
- **Busy/blocked refusal before typing:** `NudgeModeWait` refuses `"working"` and `"blocked"` panes before `BlockWrite` or `SendKeys` (`internal/provider/nudge.go:356-372`), preventing duplicate input on retry.
- **Unconfirmed delivery is non-fatal:** Confirmation timeout or `ctx.Done()` after submit returns `nil` error so gc does not retype (`internal/provider/nudge.go:418-467`).
- **Signal cancellation:** `main` catches `SIGTERM`/`SIGINT` (`cmd/gc-runtime-rex/main.go:44`); pre-typing cancellation fails with exit 1 (`internal/provider/nudge.go:213-215,389-393`), while an in-flight submit still attempts `submitKey` via `context.WithoutCancel(ctx)` (`internal/provider/nudge.go:406-408`).

## Unverified

- Live Rex `BlockWrite` / `SendKeys` typing and paste semantics (e.g. control bytes) cannot be verified from Go provider code alone without a live Rex server probe.
- Whether any non-gc consumer relies on `delivered_unobserved` stdout or `GC_EXEC_DEADLINE_MS` in this environment.
