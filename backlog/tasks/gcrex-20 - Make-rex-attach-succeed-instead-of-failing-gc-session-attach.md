---
id: GCREX-20
title: Make rex attach succeed instead of failing gc session attach
status: In Progress
assignee:
  - '@mc-wisp-kb9kh'
created_date: '2026-11-18T06:13:00Z'
updated_date: '2026-11-18T10:09:00Z'
labels:
  - provider
  - attach
dependencies: []
references:
  - docs/provider-herdr-parity.md
  - internal/provider
  - cmd/gc-runtime-rex
priority: high
type: bug
ordinal: 45000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
gc 1.5.1 calls the attach op from gc session attach and gc session new and runs it through runWithTTY, which does not apply the RPP exit-2-is-success rule: our exit 2 surfaces as 'gc session attach: exit status 2' with exit 1. This is the only op where 'not implemented' is user-visible (parity doc ### attach, mistake 1). A Rex user can attach by opening the Rex app, but the gc CLI attach must not fail.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 attach exits 0 on a live Rex pane, either by printing a one-line hint on how to reach the pane in Rex or by focusing the Rex session/tab/block through the Rex API.
- [ ] #2 attach exits 0 with a hint (not 1 or 2) when the session is unknown, so gc session attach never fails on a Rex-backed session.
- [ ] #3 No tty.attach capability is declared; the op is a hint/focus path, not a pty passthrough, and the handshake is unchanged.
- [ ] #4 Unit tests cover the live-pane, unknown-session and Rex-unreachable cases with the expected exit codes and stderr.
- [ ] #5 docs/REX-CONTRACT.md section 9 no longer states that exit 2 is treated as success for attach; it records attach and exec as the exceptions.
- [ ] #6 go test ./... and golangci-lint run ./... pass.
- [ ] #7 docs/provider-herdr-parity.md is updated in the same change: the affected section row(s) are re-verified after the fix (verdict changed, or a note added saying the deviation is now resolved), the 'Summary of deviations' row(s) and the 'Coverage' table are recomputed, and 'Confirmed mistakes' / 'Proposed follow-up work' no longer list the item.
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. Implement attach in the dispatcher as a hint path: remove 'attach' from the unimplemented map, register opAttach in the ops table, and print a one-line hint (open Rex and select the session) to stdout. Exit 0 always, including unknown session and unreachable Rex. No Rex call, so attach cannot fail; no tty.attach declared, handshake unchanged. (AC #1 option A, #2, #3.)
2. Tests in cmd/gc-runtime-rex/dispatch_test.go: live pane, unknown session, and Rex-unreachable (client errors on SessionView/ListClients) each exit 0 with the hint on stdout and empty stderr. Remove attach from the exit-2 unimplemented-op test. (AC #4.)
3. Docs: REX-CONTRACT.md §9 exit-2 row records attach and exec as exceptions; rex-provider.md moves attach to implemented and removes it from the exit-2 list; provider-herdr-parity.md re-verifies the attach section, coverage/summary rows, removes the attach entry from 'Confirmed mistakes' and the 'Make attach succeed' follow-up (renumbering and updating cross-references). (AC #5, #7.)
4. Verify: go test ./... and golangci-lint run ./... pass. (AC #6.)
<!-- SECTION:PLAN:END -->

## Comments

<!-- COMMENTS:BEGIN -->
author: @mc-wisp-0b30f
created: 2026-11-18T10:09:00Z

---

Claimed by Gas City worker mc-wisp-0b30f (seat rex-gascity/pi-2, session pi-mc-wisp-0b30f, template rex-gascity/pi); workflow bead unknown, step bead unknown.

---

author: @mc-wisp-kb9kh
created: 2026-11-18T10:09:00Z

---

Claimed by Gas City worker mc-wisp-kb9kh (seat mc-wisp-kb9kh, session pi-mc-wisp-kb9kh, template rex-gascity/pi); workflow bead mc-fyq, step bead mc-kgb.

---
<!-- COMMENTS:END -->
