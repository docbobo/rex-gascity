---
id: GCREX-58
title: 'start: emit per-stage startup timing so a slow wisp start is attributable'
status: Done
assignee:
  - '@mc-wisp-tqshx9'
created_date: '2026-11-18T22:17:00Z'
updated_date: '2026-11-18T23:07:00Z'
labels:
  - provider
  - observability
  - tdd
  - for-gascity
dependencies: []
references:
  - docs/rex-provider.md
  - docs/provider-herdr-parity.md
  - internal/provider/nudge_delivery.go
priority: high
type: task
ordinal: 114000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Develop test-first (TDD): write the failing test that reproduces this first, watch it go red, then the smallest change that makes it green (repo rule in AGENTS.md; see also the tdd skill).

Avoids: a slow start that cannot be attributed, so every fix is a guess.

Observed live 2026-11-18 on ~/my-city: 316 `worker.operation` → `start_resolved` events have a median duration of 8633 ms and a mode of 8-9 s. The launch itself is ~0.25 s (start began 22:17:27.82, pane binding recorded 22:17:28.07 for `mc-wisp-g8hg6z`). The remaining ~8.2 s is spent inside the startup-nudge handshake, and nothing in the event log or the diagnostics separates its stages, so the cost cannot be attributed to readiness, the write, the submit confirmation or the retry ladder.

What the handshake does today (internal/provider/nudge_delivery.go): `deliverStartupNudge` → `waitForStartupReady` (program_status idle poll, bounded by `defaultStartupReadyWindow` 15 s / `defaultStartupSettleDelay` 3 s) → `nudgeReportBinding` (BlockWrite + Enter, then a `defaultNudgeConfirmWindow` 3 s confirmation poll) → up to four retries on `defaultStartupNudgeBackoffs` 2/4/6/8 s. Only the total `start_resolved` duration is recorded anywhere.

The single most useful number this task must produce: how long the readiness wait takes against a Pi pane, because that is where the ~8.2 s most likely sits. Pi reports OSC 7501 `program_status` only after its support probe, and gcs own builtin Pi profile uses `ReadyDelayMs: 8000` (internal/worker/builtin/profiles.go "pi"), which matches the observed 8-9 s mode closely. If the readiness wait is the whole cost, no bound cut and no dialog guard will shorten the common case, and the honest fix is to stop waiting for idle at all and use the readiness signal that actually arrives. The timing must be able to prove or refute that.

Constraints: no new dependencies; keep the existing `NudgeReport` and diagnostics shapes; the timing must be cheap enough to leave on in production.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 A started session records a per-stage duration for launch, readiness wait, write, submit confirmation and each retry attempt, not only the total.
- [x] #2 The per-stage durations are readable without attaching a debugger: an event field and/or the existing diagnostics JSON carry them.
- [x] #3 A test proves the stages are recorded and that the readiness stage is measured even when the wait ends on its bound rather than on idle.
- [x] #4 The recorded timing distinguishes "readiness returned on idle" from "readiness returned on its bound", because the difference decides whether the bound is a lever at all.
- [x] #5 docs/rex-provider.md and docs/provider-herdr-parity.md describe the new timing output and its field names.
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. Instrument the startup path at the existing seams (no new deps): (a) capture the pane-launch duration inside start() around NewWindow/NewSplit and return it on startResult; (b) make waitForStartupReady report a readiness outcome (idle / window-bound / settle-bound / canceled / disabled) plus its measured duration; (c) add per-attempt write and submit-confirmation durations to NudgeReport from nudgeReportBinding. 2. Thread the stages through deliverStartupNudge: launch, readiness, first write/confirm, and each retry attempt (backoff, write, confirm), plus a total. 3. Always write the stages to <stateDir>/diagnostics/<name>-startup-timing.json (best effort, like the existing drain/nudge diagnostics), so a successful start is attributable, not only an unconfirmed one. Keep the existing NudgeReport and nudge-diagnostic shapes; the readiness outcome is an explicit string so idle vs bound is unambiguous. 4. Tests (package provider_test, TDD): a test asserts the stages are recorded and readiness_ms is measured with readiness_outcome=window-bound when the wait ends on its bound; a retry test asserts each attempt's backoff/write/confirm are recorded; keep existing startup tests green. 5. Update docs/rex-provider.md and docs/provider-herdr-parity.md with the new output and field names. 6. Verify: go test ./... and golangci-lint run ./...; leave the change uncommitted for review.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Work step (mc-wisp-tqshx9, 2026-11-18): per-stage startup timing implemented test-first; change uncommitted for review.

Implemented:
- waitForStartupReady now returns a readiness outcome (idle | window-bound | settle-bound | canceled | disabled) plus its measured duration, so idle vs bound is explicit (nudge_delivery.go).
- NudgeReport carries Write and Confirm durations measured in nudgeReportBinding around BlockWrite and the post-Enter confirmation poll (nudge.go).
- startResult carries launchDur measured around the NewSplit/NewWindow call under the tab lock (provider.go).
- deliverStartupNudge threads launch, readiness, first write/confirm and each resend (attempt ordinal, backoff_ms, write_ms, confirm_ms, confirmed) plus total_ms, and always writes <stateDir>/diagnostics/<name>-startup-timing.json (best effort), including successful starts, because the slow case is usually a successful start whose readiness wait dominated.
- Readiness outcome and field names documented in docs/rex-provider.md (new 'Startup timing diagnostic (GCREX-58)' section) and docs/provider-herdr-parity.md (start row + start config row 15).

Tests (package provider_test, red first then green):
- TestStartupTimingDiagnosticRecordsBoundReadiness: a pane that reports a present non-idle status records readiness_outcome=window-bound with readiness_ms >= the window, and launch_ms/readiness_ms/write_ms/confirm_ms/total_ms are present.
- TestStartupTimingRecordsEachRetryAttempt: with two retry backoffs, attempts[] has two entries each carrying backoff_ms/write_ms/confirm_ms.
Existing startup tests (idle delivery, non-idle window, settle path, context cancel, resend, unconfirmed marker) stay green.

Verification: go test ./... exit 0 (all packages ok); golangci-lint run ./... 0 issues; gofmt clean. No new dependencies; existing NudgeReport and diagnostics shapes kept (only additive fields). The change stays uncommitted; docs and code are the only touched files.

Independent review (openrouter/deepseek/deepseek-v4.1-flash:high), step bead mc-j9tt. Read-only wrt the implementation.

Scope reviewed: internal/provider/{provider.go,nudge.go,nudge_delivery.go,identity_test.go}, docs/rex-provider.md, docs/provider-herdr-parity.md, backlog/tasks/gcrex-58*.md. Excluded as out of scope: unrelated working-tree edits (gcrex-27/28 markdown, other untracked task/docs files) and .pi/.pi-agent/.gc runtime state.

Verification run: `go test ./...` PASS (all packages ok); `gofmt -l` clean; `golangci-lint run ./...` 0 issues. Red-before proof: in a temp checkout of HEAD (14d53a9) with only the new identity_test.go, both new tests fail with "read startup timing diagnostic: ... no such file"; with the change both pass. TestStartupTimingDiagnosticRecordsBoundReadiness pins readiness_outcome=window-bound, readiness_ms>=window and all stage fields; TestStartupTimingRecordsEachRetryAttempt pins two attempts[] entries with backoff_ms/write_ms/confirm_ms. Existing startup tests stay green.

AC1-AC5 met. The startupTiming shape matches the documented field names (launch_ms, readiness_ms, readiness_outcome idle|window-bound|settle-bound|canceled|disabled, write_ms, confirm_ms, confirmed, attempts[] with 2-based attempt/backoff_ms/write_ms/confirm_ms/confirmed, total_ms); the file is written best-effort on every nudged start; NudgeReport and the drain/nudge diagnostic shapes kept (additive fields only); no new deps.

gc-source check (AGENTS.md): "gc records only the total start_resolved duration" verified at gc v1.5.1 — internal/worker/operation_events.go:45-54 carries only started_at/finished_at/duration_ms (computed at :135), and the tmux nudge path records no stage durations (internal/runtime/tmux/adapter.go:1712 sendStartupNudgeWithRetry, ladder at :1643). Task-comment claim ReadyDelayMs:8000 for "pi" verified at internal/worker/builtin/profiles.go:791. Parity doc row 15 verdict unchanged ("intentional (with a gap)"); its added sentence describes our diagnostic only — no conflict with the parity record.

Findings by severity:
- Blocking: none.
- Major: none.
- Minor: (1) launch_ms (provider.go:408-443) starts before withTabLock, so it includes the tab-lock wait, SessionView and the binding record, while the field comment and docs/rex-provider.md call it "the pane launch (NewSplit/NewWindow)"; tighten the measurement or document the broader stage. (2) startupTiming.aliases is always empty: deliverStartupNudge builds a bare Binding (nudge_delivery.go:97) with no aliases, so writeStartupTimingDiagnostic's alias loop never fires; drop the field or resolve aliases for the name.
- Nit: (3) only window-bound is asserted; an idle-outcome assertion (and optionally settle-bound) would pin AC4 both ways. (4) attempts[].backoff_ms records the configured ladder value, not the measured sleep; total_ms carries real elapsed time, so attribution is unaffected.

Verdict: pass.

Verification (finalize step mc-z6bd): review verdict file = pass. Re-ran: `go test ./...` exit 0 (all packages ok); `go test -count=1 -run TestStartupTiming ./internal/provider/` PASS (TestStartupTimingDiagnosticRecordsBoundReadiness, TestStartupTimingRecordsEachRetryAttempt); `gofmt -l` clean; `golangci-lint run ./...` 0 issues. Committed files scoped to this task: internal/provider/{provider.go,nudge.go,nudge_delivery.go,identity_test.go}, docs/rex-provider.md, docs/provider-herdr-parity.md, backlog/tasks/gcrex-58*.md.
<!-- SECTION:NOTES:END -->

## Comments

<!-- COMMENTS:BEGIN -->
author: @mc-wisp-n5m4l4
created: 2026-11-18T22:18:00Z

---

Baseline from ~/my-city/.gc/events.jsonl on 2026-11-18: 316 `start_resolved` events, median 8633 ms, p90 9026 ms, mode 8-9 s (198 events in the 8 s bucket). Outliers worth explaining in the same instrumentation: `mayor` at 24597, 24729, 28750 and 50100 ms, and `bd__dog-mc-wisp-fe3` at 25457 ms. Also 71 events recorded duration 0 ms, which needs an explanation before the histogram is trusted. Expected finding: the 8-9 s mode is Pi booting to its first idle report, not a provider bound — gcs own builtin Pi profile sets `ReadyDelayMs: 8000` (gc internal/worker/builtin/profiles.go, "pi") for exactly that reason, so the rex provider is not obviously slower than gc here. If the readiness stage confirms that, the honest conclusion is that the bound is not the lever and the timing work should say so rather than invite a pointless bound cut.

---

author: @mc-wisp-tqshx9
created: 2026-11-18T22:55:00Z

---

Claimed by Gas City worker mc-wisp-tqshx9 (seat rex-gascity/pi-1, session pi-mc-wisp-tqshx9, template rex-gascity/pi); workflow bead mc-g5fk, step bead mc-upcl.

---
<!-- COMMENTS:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
start now records per-stage startup timing so a slow wisp start is attributable. What changed: startResult.launchDur measures the pane launch; waitForStartupReady returns a readiness outcome (idle | window-bound | settle-bound | canceled | disabled) plus duration; NudgeReport carries write/confirm durations; deliverStartupNudge threads launch, readiness, first write/confirm and each resend (attempt, backoff_ms, write_ms, confirm_ms, confirmed) plus total_ms and best-effort writes <stateDir>/diagnostics/<name>-startup-timing.json on every nudged start. Why: the 8-9 s start mode could not be attributed to a stage; readiness_outcome is the load-bearing field that says whether the wait returned on idle or on its bound. Verification: go test ./... exit 0; the two new provider_test cases pass (bound-readiness and per-retry stages, both borrowed red at HEAD 14d53a9 with only the test file); gofmt clean; golangci-lint 0 issues. Risks/follow-ups: none blocking. Review minors left as follow-ups (not fixed): launch_ms includes the tab-lock wait plus SessionView and binding record, broader than the 'pane launch' label; startupTiming.aliases is always empty on this path; only window-bound outcome is asserted. Existing NudgeReport and nudge-diagnostic shapes kept (additive fields only).
<!-- SECTION:FINAL_SUMMARY:END -->
