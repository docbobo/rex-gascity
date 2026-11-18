---
id: GCREX-20
title: Make rex attach succeed instead of failing gc session attach
status: To Do
assignee: []
created_date: '2026-11-18T06:13:00Z'
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
