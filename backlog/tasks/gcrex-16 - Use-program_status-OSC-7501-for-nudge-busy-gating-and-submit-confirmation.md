---
id: GCREX-16
title: Use program_status (OSC 7501) for nudge busy-gating and submit confirmation
status: Done
assignee:
  - '@mc-wisp-hts1u'
created_date: '2026-11-17T23:08:00Z'
updated_date: '2026-11-18T09:46:00Z'
labels: []
dependencies:
  - GCREX-5
references:
  - /Volumes/Sources/rex-gas-city/internal/provider/activity.go
  - /Volumes/Sources/rex-gas-city/internal/provider/provider.go
documentation:
  - docs/REX-CONTRACT.md
  - docs/rex-provider.md
modified_files:
  - internal/provider/provider.go
priority: high
type: feature
ordinal: 16000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Use the pane's OSC 7501 `program_status` for nudge busy-gating and submit confirmation, instead of fire-and-forget writes.

**Background (this is a consumer gap, not an emitter gap).** The emitter already exists: Pi implements the Program Status Protocol, Rex parses the frames, and gc-managed Pi panes report under Rex. Live-verified on ~/my-city via `rex block call <block> program_status`:
- `pi-mc-wisp-1nmn` -> `{"records":[{"state":"idle","app":"pi",...}]}`
- `mayor` -> same
- a running Pi pane -> `{"state":"working","app":"pi",...}`

`internal/provider/activity.go` already reads it: `GetLastActivity` returns `ActivityProgramStatus` when records exist, else falls back to `format_poll`. What is missing is the consumer. v1 `Nudge` writes text + Enter without looking at state (`internal/provider/provider.go:508`; GCREX-4 deferred it), which is why gc logs `submit delivered but not confirmed (no busy-state indicator for this provider family)` (see `.gc/sessions/*/nudge-unconfirmed.log` on ~/my-city).

**Fix direction.**
- Busy-gating: before writing, read `program_status`. If the pane is `working`, do not inject mid-turn unless the caller asks for it (immediate/override); queue or retry instead. If `blocked` (kind `permission`/`question`/`auth`), surface that rather than typing over a dialog.
- Submit confirmation: after write + Enter, poll `program_status` for an `idle` -> `working` transition (or a new `updated_at`) within a bounded window, and report confirmed/unconfirmed accordingly, so gc stops falling back to its timed backstop.
- Keep it provider-side and behind `rex.Client`; keep the existing `format_poll` fallback for panes/programs that report nothing.

**Scope note.** gc's own message says "no busy-state indicator for this provider family"; if gc needs the provider to declare/expose the capability (handshake flag or a new RPP op such as program-status/last-activity that carries state, not just a timestamp), capture that here too. `get-last-activity` returns a timestamp and source, not the state itself.

**Non-goals.** Emitting OSC 7501 (the program does; Pi already does). Changing `GetLastActivity`'s source preference.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 Nudge reads program_status and does not inject while the pane is working unless an immediate/override mode is requested
- [x] #2 After write+Enter the provider confirms an idle->working transition (or new updated_at) within a bounded window and reports confirmed/unconfirmed
- [x] #3 blocked state (kind permission/question/auth) is surfaced distinctly rather than typed over
- [x] #4 Panes/programs that report no program_status keep working via the format_poll fallback (no regression)
- [x] #5 gc stops logging 'submit delivered but not confirmed (no busy-state indicator for this provider family)' for rex panes
- [x] #6 Tests using the fake rex.Client cover working/idle/blocked/empty states, confirmation timeout, and immediate override
- [x] #7 go test ./... and golangci-lint run ./... pass
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. Extend the Rex program_status model to parse `kind` (contract docs/REX-CONTRACT.md:95) and let the fake script per-call status/format responses. 2. Add provider nudge policy: NudgeModeWait (default) refuses a working pane with ErrNudgeBusy and a blocked pane with ErrNudgeBlocked (kind surfaced); NudgeModeImmediate overrides. No text is written on a refusal, so gc's retry does not duplicate input. 3. Confirm delivery provider-side: poll program_status for an idle->working transition or a newer updated_at within a bounded window; fall back to a block-format fingerprint when the program reports no status. Delivered-but-unconfirmed is reported in NudgeReport and a best-effort diagnostics file, never as an error (an error would make gc retype). 4. Wire GC_REX_NUDGE_IMMEDIATE as the RPP-level override; keep gc 1.5.1 exit semantics. 5. Test-first at the provider seam + dispatcher, then update docs/provider-herdr-parity.md and docs/rex-provider.md with the verified gc 1.5.1 claim (the 'no busy-state indicator' log is tmux-only). 6. make test + golangci-lint run ./... .
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Reset to To Do by coordinator 2026-11-18 08:3x UTC. The prior In Progress claim (assignee @rex-agent, set 2026-11-18T06:47Z) was made by headless pool-worker wisp mc-wisp-gwble (session 01a1ed67-9b6d-7132-a53e-aa98c5127999, ~/my-city/.pi-agent/sessions/--Volumes-Sources-rex-gas-city--). It claimed GCREX-16 at 06:47:58, began implementation research (activity.go / provider.go Nudge / gc tmux program_status), then was repeatedly re-prompted with 'Run gc hook --claim --drain-ack --json now; if it returns work, execute it immediately.' and re-entered the hook loop, no-op'ing mc-ln0 at 06:53:52 without releasing GCREX-16. No code was written. Claim was orphaned and blocked the rex-backlog gate.

Priority raised to High by coordinator 2026-11-18 ~08:45 CET so the rex-backlog worker (which picks with '--sort priority --limit 1') selects it ahead of GCREX-17/20/21/24. GCREX-17 partial work is parked on branch gcrex-17-wip; the board is otherwise clear of In Progress.

Reset to To Do by coordinator 2026-11-18 ~08:58 CET at the user's request; state preserved (priority high, notes intact). The 08:49 claim by pjj6r (session 01a1eda0-4316) was killed by GCREX-32: a duplicate seat qxi3v closed the shared step mc-hwl no-op at 08:53:06 and pjj6r was drained at 08:53:33 with no code written, only this task-status edit.

Implemented GCREX-16 (test-first). Code: internal/provider/nudge.go (NudgeReport, NudgeModeWait/NudgeModeImmediate, ErrNudgeBusy, ErrNudgeBlocked, bounded program_status confirmation with format-poll fallback, best-effort <stateDir>/diagnostics/<name>-nudge-unconfirmed.json); internal/provider/provider.go (window/interval defaults + WithNudgeConfirmation); internal/rex/{client.go,cli/cli.go} (parse program_status kind per REX-CONTRACT.md:95); internal/rex/faketest/fake.go (scripted status/format); cmd/gc-runtime-rex/main.go (GC_REX_NUDGE_IMMEDIATE override). Docs: rex-provider.md, REX-CONTRACT.md, plans/native-rex-provider.md, provider-herdr-parity.md.

Tests (all named, all pass): TestNudgeRefusesWorkingPane, TestNudgeBlockedSurfacesKind, TestNudgeImmediateOverridesWorkingGate, TestNudgeConfirmsIdleToWorking, TestNudgeConfirmationTimesOutWithoutError, TestNudgeWithoutProgramStatusFallsBackToFormatPoll, TestNudgeUnconfirmedWritesDiagnostic, TestNudgeBusyPaneIsRefusedWithoutTyping, TestNudgeImmediateEnvOverridesBusyGate. Existing GetLastActivity format-poll tests still pass (no regression). make test: pass. golangci-lint run ./...: 0 issues. go vet ./...: clean. Built binary handshake unchanged (report-attachment, report-activity).

AC #5 verification and conflict with the task premise: gc v1.5.1 never writes 'submit delivered but not confirmed (no busy-state indicator...)' for an exec/Rex pane. The string is written only by the tmux runtime (v1.5.1 internal/runtime/tmux/tmux.go:2893-2906, recordUnconfirmedSubmit called from the tmux NudgeSession fallback at :2879). The exec provider's nudgeOp returns only the op error (v1.5.1 internal/runtime/exec/exec.go:612-618) and Nudge falls back to it (:721-729); it has no confirmation channel and reads no outcome word. The nudge-unconfirmed.log found under ~/my-city/.gc/sessions/bd__dog-mc-wisp-fe3 is dated 2026-11-17 17:57, before the Rex sessions, and matches tmux's format. So the criterion is satisfied, and the task's premise that gc logs it for rex panes is incorrect for v1.5.1. Consequence: our provider-side Confirmed flag is observable at the provider seam but is not consumed by gc 1.5.1. gc edge (working tree) defines a future 'input-outcome' capability (internal/runtime/exec/outcome.go; docs/reference/exec-session-provider.md 'Typed input outcomes') with delivered_unobserved/busy/pending_interaction; the pinned baseline does not read it, so it was documented rather than emitted. No handshake capability was added.
<!-- SECTION:NOTES:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
Added a provider-side nudge consumer for OSC 7501 program_status. Nudge now refuses a working pane with ErrNudgeBusy and a blocked pane with ErrNudgeBlocked (kind named) without typing, so gc's queued retry cannot duplicate input; NudgeModeImmediate / GC_REX_NUDGE_IMMEDIATE overrides the gate. After write+Enter it confirms a turn start within a 3 s bounded window by an idle->working transition or a newer updated_at, falling back to a block-format fingerprint when no program_status is reported. A delivered-but-unconfirmed nudge is not an error (an error would make gc retype it); it is reported in NudgeReport and a best-effort diagnostics file under the provider state dir. program_status records now carry kind. Verified against gc v1.5.1: the 'no busy-state indicator' log is written only by the tmux runtime (tmux.go:2893-2906); the exec provider's nudgeOp returns only the exit error (exec/exec.go:612-618, :721-729), so Rex panes never produced it. The task's premise that gc logs it for rex panes is therefore corrected; gc 1.5.1 does not consume the confirmation, and the edge-only input-outcome channel is documented rather than emitted. Verification: make test passes; golangci-lint run ./... reports 0 issues; go vet clean. No live Rex probe was run.
<!-- SECTION:FINAL_SUMMARY:END -->
