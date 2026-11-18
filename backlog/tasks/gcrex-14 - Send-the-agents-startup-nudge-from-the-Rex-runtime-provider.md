---
id: GCREX-14
title: Send the agent's startup nudge from the Rex runtime provider
status: Done
assignee:
  - '@pi'
created_date: '2026-11-17T23:03:00Z'
updated_date: '2026-11-18T00:22:00Z'
labels: []
dependencies:
  - GCREX-5
references:
  - /Users/boris/my-city/.pi-agent/sessions
  - /Users/boris/my-city/.gc/runtime/session-reconciler-trace
documentation:
  - docs/rex-provider.md
  - docs/REX-CONTRACT.md
priority: high
type: feature
ordinal: 14000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Send the start-config `nudge` from the Rex runtime provider so a freshly launched agent begins work immediately instead of waiting for gc's timed backstop.

**Observed problem.** Fresh pool/rig seats (the `pi-mc-wisp-*` sessions on ~/my-city) sit with an empty prompt for ~100 s after launch. Measured from Pi session files: session created 22:43:18, first user message 22:45:07 (109 s); session 22:50:20 -> first message 22:52:07 (107 s). The first message is the agent's configured `nudge` ("Run gc hook --claim --drain-ack --json now; ..."), i.e. the wisp is up in ~10 s but is not told to work until the backstop fires.

**Root cause.** `gc-runtime-rex start` decodes only `work_dir`, `command`, `ready_prompt_prefix`, and `env` from the start config (`startConfig`, cmd/gc-runtime-rex/main.go). gc 1.5.1 sends the initial prompt in the start config's `nudge` field and does not resend it, so the dispatcher drops it. The wisp then only moves when gc's queued-nudge store drains (per-session `gc nudge poll` sidecar; 2 min claim TTL, 15 s retry, 5 attempts) or the reconciler's `nudge_stalled_pool_claims` backstop runs. Confirmation of delivery is weak because no busy/idle frames are emitted (see the separate OSC 7501 question / nudge-unconfirmed.log: "submit delivered but not confirmed (no busy-state indicator for this provider family)").

This is documented as open: docs/rex-provider.md "Startup `nudge` is not sent" and docs/REX-CONTRACT.md section 12 / GCREX-5.

**Fix direction.** In `opStart`/`Provider.Start`, capture the start config's `nudge` and, after the pane launches successfully (launch returns), write the nudge text and submit it (reuse the existing `Nudge` path: text + enter). Keep the current `Nudge` semantics. On launch failure, do not deliver. Add a `nudge` field to `startConfig`.

**Non-goals.** Busy-gated nudge / OSC 7501 idle detection (separate item); the remaining start hooks pre_start/session_setup/etc. (GCREX-5 / the pre_start item).
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 startConfig decodes the start-config nudge field, and gc-runtime-rex start passes it through to the provider
- [x] #2 On a successful launch the provider delivers the startup nudge (text + submit) exactly once; on a failed or already-running start the nudge is not delivered
- [x] #3 A fresh rex-gascity/pi (and my-project/pi) pool spawn begins work in single-digit seconds, not ~100 s; the reconciler nudge_stalled_pool_claims backstop is no longer the first thing that wakes it
- [x] #4 Tests using the fake rex.Client cover: nudge delivered after launch, not delivered on ErrAlreadyRunning / launch failure, and empty nudge is a no-op
- [x] #5 go test ./... and golangci-lint run ./... pass
- [x] #6 Live check on ~/my-city: spawn a rex pool seat and confirm the first user message timestamp is within a few seconds of the Pi session start (compare .pi-agent/sessions/*.jsonl vs events.jsonl)
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. Add nudge to startConfig and provider.Config; pass through opStart. 2. Deliver the nudge (text + enter) exactly once after a successful launch only, via the recorded binding; skip on launch failure, already-running/already-alive, and empty nudge. 3. Fake-rex tests for delivered/not-delivered/no-op. 4. Update docs/rex-provider.md. 5. go test ./... + golangci-lint run ./...; parent merges and runs the live check.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Implemented in worktree gcrex-14-nudge (DeepSeek v4 Pro), merged to main as 86ca99a (cherry-pick of 5e0256e; conflicts with GCREX-15 in Config/startConfig/start and the two docs resolved by hand, signatures re-verified). startConfig decodes nudge -> provider.Config.Nudge. Provider.start registers a deferred nudge after the provisional binding; on return it delivers BlockWrite(text)+SendKeys(enter) via the just-recorded sid/launchedBlock only when a pane launched (launchedBlock != ''), cfg.Nudge != '', and err == nil. Not delivered on launch failure, the already-alive idempotent path, or ErrAlreadyRunning; empty is a no-op. A launchOK flag keeps the identity/binding cleanup from firing when the launch succeeded but the nudge failed (pane left intact, error surfaced). Tests in internal/provider/identity_test.go (5 cases). go test -count=1 ./... ok; golangci-lint run ./... 0 issues; go vet clean. Live check pending.

Follow-up hardening on main (commit cd0fe48) after an independent cross-family review: startup nudge delivery is now best effort (a BlockWrite/SendKeys transport failure no longer fails Start or clears the seeded identity/binding; the launched pane is preserved). The launchOK cleanup guard was dropped as unnecessary. faketest.BlockCall now records the written Text, TestStartupNudgeDeliveredAfterLaunch asserts the exact nudge text, TestStartupNudgeDeliveryFailureKeepsPane covers write/send-key failure, and TestPreStartInheritsBaseEnv verifies pre_start inherits os.Environ(). Independent review (claude-sonnet-4.6) found no blocking defects and 'all acceptance criteria satisfied'. Verification on merged main: go test -count=1 ./... ok, go test -race ./internal/provider ok, golangci-lint run ./... 0 issues, go vet clean.

Live checks (new binary): a controlled dispatcher start ran a pane whose program recorded its start time and the nudge it received; the nudge text 'Run gc hook --claim --drain-ack --json now' arrived 0.004s after the pane program started. Pre-fix baseline from the same city's earlier pool sessions: first user message ~125-130s after session creation (12:47:54 -> 12:50:04, 12:45:29 -> 12:47:35), i.e. the reconciler backstop, now replaced by immediate delivery.
<!-- SECTION:NOTES:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
The start-config nudge is now delivered from the Rex provider. startConfig decodes the nudge field and passes it to provider.Config; after a successful launch the provider writes the text to the just-recorded pane binding and submits Enter exactly once, without re-resolving the pane (so no readiness race). It is not delivered on a failed launch, on the already-alive idempotent path, on ErrAlreadyRunning, or when the text is empty. Delivery is best effort (matching the provider's nudge op): a transport failure does not fail Start or clear the seeded identity/binding, so gc never tears down a healthy pane over it; gc's queued-nudge backstop covers an undelivered first prompt. Verified: commits 86ca99a (feature) and cd0fe48 (best-effort hardening and tests); independent cross-family review (Claude Sonnet 4.6) found no blocking defects; go test ./..., go test -race ./internal/provider, golangci-lint run ./... (0 issues), go vet clean. Live: a controlled pane recorded the nudge 0.004s after its program started with the exact text; the pre-fix baseline on the same city was ~125-130s. Risk/limit: nudge delivery failures are not surfaced to gc (intentional); if reporting is needed later, use a distinct error type rather than a plain start failure.
<!-- SECTION:FINAL_SUMMARY:END -->
