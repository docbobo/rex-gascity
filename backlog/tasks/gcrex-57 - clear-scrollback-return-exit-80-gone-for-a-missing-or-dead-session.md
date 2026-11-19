---
id: GCREX-57
title: 'clear-scrollback: return exit 80 gone for a missing or dead session'
status: Done
assignee:
  - '@mc-wisp-730ydu'
created_date: '2026-11-18T22:06:00Z'
updated_date: '2026-11-19T04:19:00Z'
labels:
  - provider
  - herdr
  - for-gascity
  - tdd
dependencies: []
references:
  - docs/provider-method-audit/clear-scrollback.md
documentation:
  - docs/provider-herdr-method-io.md
priority: medium
type: task
ordinal: 113000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Develop test-first (TDD): write the failing test that reproduces this first, watch it go red, then the smallest change that makes it green (repo rule in AGENTS.md; see also the tdd skill).

Avoids: a caller cannot tell cleared scrollback from a vanished pane.

The document (## clear-scrollback § Failure cases) specifies exit 80 with {"outcome":"gone",...} and no mutation when no box holds the name or the program ended. The audit rates this medium; gc discards ClearScrollback errors at its reconciler call sites, so gc is unaffected.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 A missing box or dead program exits 80 with a JSON outcome on stdout and performs no mutation.
- [x] #2 The live-session path keeps its current behaviour, covered by a test.
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. Red tests: an empty outcome-gone test in internal/provider/phase4_test.go (missing binding and exited program) and TestClearScrollbackReturnsGoneForMissingOrDeadSession in cmd/gc-runtime-rex/dispatch_test.go; move clear-scrollback out of the best-effort missing-session test; watch both go red. 2. Code: ClearScrollback resolves via livePane and classifies lookup failures with the shared paneLookupOutcome (renamed from interruptLookupOutcome), maps a mid-call rex.ErrNotFound to gone and ErrUnavailable to unavailable, and opClearScrollback returns the provider error directly instead of wrapping bestEffort. 3. Docs: move missing/dead and liveness rows from Deviations to Matches in docs/provider-method-audit/clear-scrollback.md; refresh the clear-scrollback summary rows and detailed section in docs/provider-herdr-parity.md. 4. Verify go test ./... and golangci-lint run ./.... Leave uncommitted.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Implemented (TDD). Red: TestClearScrollbackMissingOrDeadIsGone failed (error = nil, want InputOutcomeError gone) and TestClearScrollbackReturnsGoneForMissingOrDeadSession failed (exit 0, want 80) before the change. Green: ClearScrollback (internal/provider/provider.go:688-703) now resolves through livePane (:834-850) and classifies lookup failures with paneLookupOutcome (:823-832, renamed from interruptLookupOutcome and shared with Interrupt); a mid-call rex.ErrNotFound is gone and ErrUnavailable is unavailable, any other mid-call error stays exit 1. opClearScrollback (cmd/gc-runtime-rex/main.go:348-350) returns the provider error directly, so the typed outcome reaches writeInputOutcome and exit 80; bestEffort is no longer applied. Tests: TestClearScrollbackMissingOrDeadIsGone, updated TestClearScrollback (internal/provider/phase4_test.go); TestClearScrollbackReturnsGoneForMissingOrDeadSession, TestClearScrollbackDeliveredExitsZero, and clear-scrollback removed from TestInputOpsOnMissingSessionSucceed (cmd/gc-runtime-rex/dispatch_test.go). Verified: go test -count=1 ./... all packages ok; golangci-lint run ./... 0 issues; gofmt -l cmd internal empty; git diff --check clean. Docs: moved the missing/dead and liveness rows from Deviations to Matches in docs/provider-method-audit/clear-scrollback.md and refreshed the unreachable/input-outcome row; updated the clear-scrollback summary rows and the section narrative in docs/provider-herdr-parity.md, verdict intentional (rows 2-5 now match herdr-core; row 1, the real Rex clear, remains the intentional deviation). Sources: pack clear-scrollback doc ## clear-scrollback Failure cases (missing box or ended program exits 80 gone, no mutation) and herdr-core inputs.py:320-336; gc v1.5.1 discards ClearScrollback errors at its reconciler call sites (session_reconciler.go:3739, :3894), so gc is unaffected by the typed failure. Left uncommitted for review.

Finalization verification (mc-wisp-y7ux10, bead mc-psgu): review verdict pass (/Users/boris/my-city/.gc/state/rex-backlog.review). Re-ran go test -count=1 ./... — all packages ok (cmd/gc-runtime-rex 6.652s, internal/provider 7.278s, internal/launch, internal/rex/cli). Staging only the GCREX-57 files (provider.go, phase4_test.go, gc-runtime-rex/main.go, dispatch_test.go, provider-herdr-parity.md, provider-method-audit/clear-scrollback.md, task md). Excluded unrelated working-tree churn (gcrex-27/gcrex-28 task md, untracked gcrex-37 audit docs, .pi/.pi-agent/.gc). Open review nits carried as follow-ups, not blocking: parity row 1 still cites provider.go:489; GCREX-3 cross-ref still says clear-scrollback shares bestEffort; idempotency sentence lost its test citation.
<!-- SECTION:NOTES:END -->

## Comments

<!-- COMMENTS:BEGIN -->
author: @mc-wisp-730ydu
created: 2026-11-19T03:58:00Z

---

Claimed by Gas City worker mc-wisp-730ydu (seat unknown, session pi-mc-wisp-730ydu, template rex-gascity/pi); workflow bead mc-eoqd, step bead mc-jkp2.

---

author: @mc-wisp-n7cl1u
created: 2026-11-19T04:04:00Z

---

Review (GCREX-57, working tree): code meets AC #1/#2. Red/green confirmed: TestClearScrollbackMissingOrDeadIsGone and TestClearScrollbackReturnsGoneForMissingOrDeadSession both fail against HEAD (nil error / exit 0) and pass with the change. go test -count=1 ./..., golangci-lint run ./..., go vet ./..., gofmt -l cmd internal, and git diff --check all pass. gc v1.5.1 discards ClearScrollback at session_reconciler.go:3739 and :3894, so the typed failure is safe.

Minor doc fixes before commit:
1. docs/provider-herdr-parity.md: the GCREX-3 cross-reference still says nudge/send-keys/clear-scrollback share bestEffort; clear-scrollback no longer does. Update it.
2. docs/provider-herdr-parity.md row 1 still cites provider.go:489 for the clear; ClearScrollback is now at provider.go:688-703. Update the line reference.
3. docs/provider-herdr-parity.md still calls the clear idempotent ("a second clear ... is not an error") without a test; mark unverified or add a second-clear assertion.
4. docs/provider-method-audit/clear-scrollback.md "No retry" row says a dropped connection exits 1; ClearScrollback now maps mid-call rex.ErrUnavailable to exit 80 unavailable. A lost reply after write still exits 1. Reword.
5. Unrelated churn: gcrex-27 and gcrex-28 task markdowns are modified in the working tree; keep them out of the GCREX-57 commit.

Note: the claimed bead mc-5kvk is not present in the live bd store (bd show/history not found), so this review is recorded against GCREX-57.

---

author: @mc-wisp-99z8u6
created: 2026-11-19T04:14:00Z

---

Independent review, second pass (openrouter/deepseek/deepseek-v4.1-flash:high). The first pass (comment #2) recorded findings but left no verdict file and its review bead open; this pass completes the step and independently re-verifies. Scope: the GCREX-57 working-tree change — `internal/provider/provider.go`, `cmd/gc-runtime-rex/main.go`, the two test files, the clear-scrollback section of `docs/provider-herdr-parity.md`, `docs/provider-method-audit/clear-scrollback.md`, and this task file.

Verified:
- AC #1 (missing box or dead program exits 80, one JSON outcome line on stdout, no mutation). Red/green re-run in a throwaway HEAD archive: `TestClearScrollbackMissingOrDeadIsGone` and `TestClearScrollbackReturnsGoneForMissingOrDeadSession` fail against HEAD (error = nil / exit 0) and pass with the change; both assert no `clear` BlockCall.
- AC #2 (live path unchanged): `TestClearScrollback` (one BlockClear) and `TestClearScrollbackDeliveredExitsZero` (exit 0, empty stdout/stderr).
- gc side, re-checked per repo rule against tag v1.5.1: `_ = sp.ClearScrollback(name)` at `cmd/gc/session_reconciler.go:3739` (max-age) and `:3894` (idle), both in the branch where the kill already succeeded, error discarded — the typed exit 80 cannot change gc behavior. No parity verdict changes in this diff, and the gc check was re-run anyway.
- Reference behavior: `runtime-herdr-core` herdrcore/inputs.py — no name → gone (:213), no box → gone (:225-226), dead program → gone (:227-228, :333-334), unreachable → unavailable (:220-224); `clear_scrollback` (:320-335) clears nothing; `ops_table.py:86` registers it. Matches the AC wording.
- `go test -count=1 ./...` all green; `golangci-lint run ./...` 0 issues; `gofmt -l cmd internal` clean.

Blocking: none. Major: none.

Minor/nit (follow-ups, docs only):
1. `docs/provider-herdr-parity.md` GCREX-3 cross-reference still says nudge/send-keys/clear-scrollback share `bestEffort` missing-session handling; clear-scrollback no longer wraps bestEffort (`main.go:348-350`). Reword or date it.
2. Stale line references: parity row 1 cites `provider.go:489`; the clear is now `provider.go:693` (function 688-704). The audit doc's first deviation row and housekeeping row cite `provider.go:621-630` (that range is `clearIncarnations`). `livePane` is cited 834-850 but spans 834-853. Refresh while the section is being touched.
3. The idempotency sentence ("a second clear on an already-empty buffer is not an error") lost its test citation and no test pins it; mark unverified or add a second-clear assertion.
4. Audit doc "No retry" row says "a dropped connection or lost reply exits 1"; with GCREX-57 a mid-call `rex.ErrUnavailable` (rex client exit 4, `internal/rex/cli/cli.go:44-54`) maps to exit 80 `unavailable` (`provider.go:697-698`) — only non-transport mid-call errors stay exit 1. Reword, and check parity row 4 ("lost reply exit 1") for the same contradiction.
5. Unrelated churn: the `gcrex-27 …` and `gcrex-28 …` task markdowns are modified in the working tree; keep them out of the GCREX-57 commit.

Verdict: pass.

---
<!-- COMMENTS:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
clear-scrollback now answers exit 80 with a single JSON outcome on stdout and performs no mutation when the box is missing or its program has ended, matching the document's ## clear-scrollback Failure cases and herdr-core. ClearScrollback resolves the name via livePane and classifies lookup failures with the shared paneLookupOutcome (renamed from interruptLookupOutcome); a mid-call rex.ErrNotFound maps to gone and ErrUnavailable to unavailable, other mid-call errors stay exit 1. opClearScrollback returns the provider error directly, so the typed outcome reaches writeInputOutcome and exit 80; the bestEffort wrapper is gone. The live-session path is unchanged (one Rex clear, exit 0 with empty stdout/stderr). Verification: go test -count=1 ./... all packages ok; golangci-lint run ./... 0 issues; gofmt -l cmd internal clean; red/green proven by TestClearScrollbackMissingOrDeadIsGone and TestClearScrollbackReturnsGoneForMissingOrDeadSession (fail at HEAD, pass with the change). gc v1.5.1 discards ClearScrollback errors at session_reconciler.go:3739 and :3894, so gc is unaffected. Risks/follow-ups: docs-only nits from review (stale parity row-1 line ref, GCREX-3 bestEffort cross-ref, un-cited idempotency claim); Rex clear semantics on peek/attached clients remain unverified (parity row 6).
<!-- SECTION:FINAL_SUMMARY:END -->
