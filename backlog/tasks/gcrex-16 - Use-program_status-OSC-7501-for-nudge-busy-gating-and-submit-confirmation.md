---
id: GCREX-16
title: Use program_status (OSC 7501) for nudge busy-gating and submit confirmation
status: To Do
assignee: []
created_date: '2026-11-17T23:08:00Z'
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
priority: medium
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
- [ ] #1 Nudge reads program_status and does not inject while the pane is working unless an immediate/override mode is requested
- [ ] #2 After write+Enter the provider confirms an idle->working transition (or new updated_at) within a bounded window and reports confirmed/unconfirmed
- [ ] #3 blocked state (kind permission/question/auth) is surfaced distinctly rather than typed over
- [ ] #4 Panes/programs that report no program_status keep working via the format_poll fallback (no regression)
- [ ] #5 gc stops logging 'submit delivered but not confirmed (no busy-state indicator for this provider family)' for rex panes
- [ ] #6 Tests using the fake rex.Client cover working/idle/blocked/empty states, confirmation timeout, and immediate override
- [ ] #7 go test ./... and golangci-lint run ./... pass
<!-- AC:END -->
