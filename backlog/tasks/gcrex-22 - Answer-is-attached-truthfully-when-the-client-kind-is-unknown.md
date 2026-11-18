---
id: GCREX-22
title: Answer is-attached truthfully when the client kind is unknown
status: Done
assignee:
  - '@mc-wisp-9qusha'
created_date: '2026-11-18T06:13:00Z'
updated_date: '2026-11-18T14:54:00Z'
labels:
  - provider
  - liveness
dependencies: []
references:
  - docs/provider-herdr-parity.md
  - internal/provider
  - cmd/gc-runtime-rex
priority: medium
type: bug
ordinal: 47000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
IsAttached returns ErrAttachmentUnknown (exit 1) when the Rex client list contains a kind it cannot classify. gc 1.5.1 reads any is-attached error as false, which is fail-open: config-drift restart, progress-stall recycle and the drain-ack escalation kill then proceed against a pane a human may be watching (parity doc ### is-attached row 3, mistake 4). The safe direction is to answer true when we cannot tell.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 An unclassifiable Rex client kind answers true with exit 0 instead of failing.
- [x] #2 The Rex-unreachable case still fails with exit 1, and the doc states the resulting fail-open behavior explicitly.
- [x] #3 Unit tests cover app client active (true), cli/peer only (false), unknown kind (true), and Rex transport error (error).
- [x] #4 go test ./... and golangci-lint run ./... pass.
- [x] #5 docs/provider-herdr-parity.md is updated in the same change: the affected section row(s) are re-verified after the fix (verdict changed, or a note added saying the deviation is now resolved), the 'Summary of deviations' row(s) and the 'Coverage' table are recomputed, and 'Confirmed mistakes' / 'Proposed follow-up work' no longer list the item.
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. Change TestIsAttachedUnknownKind to expect (true, nil) for an unclassifiable client kind and add a peer-only false case; run to watch it fail (red). 2. Change Provider.IsAttached (internal/provider/attach.go): an unknown client kind answers true, nil immediately (safe direction), instead of returning ErrAttachmentUnknown/exit 1; remove the now-dead ErrAttachmentUnknown. 3. Keep the Rex transport error path returning the error (exit 1) and the cli/peer-only path false. 4. Verify go test ./... and golangci-lint run ./.... 5. Update docs/provider-herdr-parity.md: §is-attached row 3 verdict to intentional, the Summary-of-deviations row and Coverage-table verdict, drop the is-attached item from Confirmed mistakes and Proposed follow-up work, and refresh file:line citations for the new attach.go. Re-verify the Rex-unreachable fail-open note (AC #2).
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Implemented: an unclassifiable Rex client kind now answers true with exit 0 instead of returning ErrAttachmentUnknown/exit 1. internal/provider/attach.go removes the dead exported ErrAttachmentUnknown and its errors/fmt imports; the cli/peer-only path still answers false and a Rex transport error still propagates as exit 1 (gc reads that as not attached, stated explicitly in the IsAttached doc comment and the parity doc). Tests: TestIsAttachedUnknownKind now asserts (true, nil) for "" and "mystery"; added TestIsAttachedPeerOnly; cmd/gc-runtime-rex TestIsAttached now asserts true for kind "martian". Docs: parity doc §is-attached row 3 verdict mistake→intentional, Coverage and Summary-of-deviations verdicts recomputed, the item removed from Confirmed mistakes (rows renumbered 1-7) and Proposed follow-up work (Closes renumbered), attach.go citations refreshed to :18-53/:36-43/:64-73, and the Rex-unreachable fail-open note made explicit (AC #2). Also updated the stale ErrAttachmentUnknown references in docs/plans/native-rex-provider.md since the symbol no longer exists.

TDD proof: with the old attach.go, TestIsAttachedUnknownKind fails (IsAttached returned ErrAttachmentUnknown); with the fix it passes. Verification: go test -count=1 ./... passes (cmd/gc-runtime-rex, internal/provider, internal/rex/cli); golangci-lint run ./... reports 0 issues.

Independent review (openrouter/~anthropic/claude-sonnet-latest): Blocking: none. Major: none. Minor/nit: docs/provider-herdr-parity.md:856 (the `### attach` deviation table) still cites `internal/provider/attach.go:22-52` for IsAttached; after ErrAttachmentUnknown was removed the function body moved to :18-53, so that citation is stale (the is-attached section and GCREX-4 paragraph were refreshed correctly). Follow-up only.

Verified against source: AC#1 unknown/empty client kind answers true, exit 0 (internal/provider/attach.go:36-43; TestIsAttachedUnknownKind with kinds "" and "mystery"; cmd/gc-runtime-rex TestIsAttached kind "martian"). AC#2 Rex transport error still returns exit 1 (attach.go:26-29; TestIsAttachedListClientsError) and the fail-open reading is stated explicitly in the IsAttached doc comment and docs/provider-herdr-parity.md row 3/verdict. AC#3 tests cover app-active true (TestIsAttachedAgentTabActive), cli-only false (TestIsAttachedCLIOnly), peer-only false (TestIsAttachedPeerOnly), unknown true, transport error. AC#4 `go test ./...` passes and `golangci-lint run ./...` reports 0 issues. AC#5 docs recomputed consistently: coverage row mistake->intentional, summary-of-deviations row 3 verdict, Confirmed mistakes renumbered 1-7 with is-attached removed, Proposed follow-up Closes renumbered (1,2 / 3,4 / 5,7 / 6); line citations :18-53, :36-43, :64-73 verified correct by line count. Verdict: pass.

Finalization verification: review verdict pass; go test -count=1 ./internal/provider ./cmd/gc-runtime-rex passes; go test ./... passes; golangci-lint run ./... reports 0 issues. Fixed the stale attach.go citation in docs/provider-herdr-parity.md:856 (:22-52 -> :18-53) noted by review.
<!-- SECTION:NOTES:END -->

## Comments

<!-- COMMENTS:BEGIN -->
author: @mc-wisp-9qusha
created: 2026-11-18T14:43:00Z

---

Claimed by Gas City worker mc-wisp-9qusha (seat rex-gascity/pi-1, session pi-mc-wisp-9qusha, template rex-gascity/pi); workflow bead mc-02c, step bead mc-0eh.

---
<!-- COMMENTS:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
IsAttached now answers true for unclassifiable Rex client kinds instead of ErrAttachmentUnknown/exit 1; Rex transport errors still exit 1. Removed the dead error type and updated unit/CLI tests plus the parity and plan docs. Verification: go test ./... passes and golangci-lint run ./... reports 0 issues. No blocking risks; the documented optional Stop-side attachment guard remains a follow-up.
<!-- SECTION:FINAL_SUMMARY:END -->
