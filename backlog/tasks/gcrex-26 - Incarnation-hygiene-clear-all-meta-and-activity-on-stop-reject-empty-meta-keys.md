---
id: GCREX-26
title: >-
  Incarnation hygiene: clear all meta and activity on stop, reject empty meta
  keys
status: Done
assignee:
  - '@mc-wisp-gk2sr5'
created_date: '2026-11-18T06:14:00Z'
updated_date: '2026-11-18T16:36:00Z'
labels:
  - provider
  - state
dependencies: []
references:
  - docs/provider-herdr-parity.md
  - internal/provider
  - cmd/gc-runtime-rex
priority: medium
type: bug
ordinal: 51000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Stop clears only the identity keys, so suspended, GC_DRAIN_ACK, GC_READY_PROMPT_PREFIX and the activity entry survive the incarnation and reappear for the next agent of the same name; herdr-core deletes the whole meta file with the incarnation (parity doc ### stop row 4 and ### set-meta row 2, mistake 9). Separately, set-meta/get-meta/remove-meta accept an empty key where herdr-core rejects it, and Binding.ReadyPrompt is written and never read (mistake 11, ### watch-startup follow-up).
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 Stop (and the no-binding path) clears every meta key for the name plus its activity entry, covered by a unit test that sets suspended/GC_DRAIN_ACK before stopping and reads nothing after.
- [x] #2 A fresh start of the same name observes no meta or activity from the previous incarnation.
- [x] #3 The three meta ops reject an empty key with exit 1 and a stderr message, covered by unit tests.
- [x] #4 A decision is recorded for Binding.ReadyPrompt: either it gates something or it is removed, with a note in the parity doc ### watch-startup section.
- [x] #5 go test ./... and golangci-lint run ./... pass.
- [x] #6 docs/provider-herdr-parity.md is updated in the same change: the affected section row(s) are re-verified after the fix (verdict changed, or a note added saying the deviation is now resolved), the 'Summary of deviations' row(s) and the 'Coverage' table are recomputed, and 'Confirmed mistakes' / 'Proposed follow-up work' no longer list the item.
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. TDD stop hygiene: provider tests that set suspended/GC_DRAIN_ACK/GC_READY_PROMPT_PREFIX and record activity, Stop, then assert no meta key or activity survives; a fresh start of the same name sees neither. Watch them fail.
2. Add Clear(name) to MetaStore and Delete(name) to ActivityStore (memory, file, composite) plus a Provider.clearIncarnation helper; make Stop and its no-binding branch call it.
3. TDD empty-key rejection: dispatcher tests that set-meta/get-meta/remove-meta reject an empty key with exit 1 and a stderr message; reject it in run().
4. Resolve Binding.ReadyPrompt by removal: written and never read, and herdr-core does not use the ready prompt inside watch-startup either. Drop the field, the ready_prompt record field, the GC_READY_PROMPT_PREFIX fallback read, and Config/startConfig.ReadyPromptPrefix; update tests.
5. Update docs/provider-herdr-parity.md: re-verify the stop state-cleaned row, set-meta lifetime row, get-meta/remove-meta empty-key rows and watch-startup; recompute Coverage counts and the Summary of deviations; drop mistakes 3 and 5 and the closed follow-ups; renumber.
6. Run go test ./... and golangci-lint run ./...; leave the change uncommitted for review.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Implemented (uncommitted, for review).

Incarnation hygiene: Stop and its no-binding branch now call Provider.clearIncarnations (internal/provider/provider.go), which clears the whole meta sidecar (MetaStore.Clear: fileMetaStore.Clear removes the name's meta dir; memoryMetaStore.Clear) and the activity entry (ActivityStore.Delete) for the caller name and the resolved primary name. Composite stores fan Clear/Delete across every namespace. New tests: TestStopClearsAllMetaAndActivity and TestStartAfterStopObservesNoMetaOrActivity (internal/provider/incarnation_test.go).

Empty meta keys: the dispatcher rejects an empty key for set-meta/get-meta/remove-meta with exit 1 and a stderr message (cmd/gc-runtime-rex/main.go, keyedOps); covered by TestMetaOpsRejectEmptyKey (cmd/gc-runtime-rex/dispatch_test.go).

Binding.ReadyPrompt removed as dead, write-only state: nothing read it, and herdr-core's watch-startup keeps prompt matching in gc. Dropped the field, the ready_prompt record field, the GC_READY_PROMPT_PREFIX fallback read, and Config/startConfig.ReadyPromptPrefix; the start config's ready_prompt_prefix is now an ignored field.

docs/provider-herdr-parity.md: re-verified the stop row 4 (state cleaned) and set-meta row 2 (lifetime across stop) after the fix, plus the get-meta/remove-meta empty-key rows and watch-startup; verdicts flipped to intentional/resolved, Coverage verdicts and Summary of deviations recomputed, Confirmed mistakes 3 and 5 dropped and the rest renumbered to 1-3, and the resolved follow-ups removed.

Verification: make test all ok; golangci-lint run ./... reports 0 issues; targeted go test ./internal/provider/ -run 'TestStopClearsAllMetaAndActivity|TestStartAfterStopObservesNoMetaOrActivity' and go test ./cmd/gc-runtime-rex/ -run TestMetaOpsRejectEmptyKey pass.

Independent review (openrouter/~anthropic/claude-sonnet-latest). Verdict: fail.

Method: read the uncommitted diff (`git diff HEAD`) against the AC/DoD; verified `make test` (pass), `go test ./...` (pass), `golangci-lint run ./...` (0 issues); cross-checked the parity-doc claims against the tree.

Blocking: none.

Major:
1. AC#6 doc re-verification is incomplete. (a) The Summary of deviations `start` row (docs/provider-herdr-parity.md:91) still reads "6 of the 15 gc sends" while the `### start` deviation row (:333) now reads "5 of the 15"; the count was not recomputed in the summary. (b) The `### start` "Our implementation" paragraph kept stale main.go citations that this change shifted: dispatch cited main.go:82 (actual 89), opStart cited main.go:162 (actual 180), "already running" cited main.go:182-185 (actual 199-201), context.Background() cited main.go:138 (actual 156). The `### stop` refs were updated (90/206/147) but start's were not. The parity doc values verifiable citations, so this is a real miss.
2. AC#1 covers "Stop (and the no-binding path)" but only the live-pane path is tested. clearIncarnations on the pane-already-gone branch (internal/provider/provider.go:759) has no test asserting meta/activity are cleared; incarnation_test.go stops while the pane is alive, and TestStopWritesDiagnosticWhenPaneIsAlreadyGone asserts the diagnostic only. The repo DoD requires a named test per behavior change.

Minor/nit:
3. The working tree also carries unrelated changes (backlog/config.yml global definition_of_done; gcrex-28/29/30/31/32/33/35 task metadata). Keep the eventual commit scoped to GCREX-26, or state the intent.
4. The failed-start deferred rollback (provider.go:335) still calls clearStartIdentity (identity keys only), so a failed start can leave user meta/activity; out of the AC but inconsistent with the new stop hygiene.
5. TestStartAfterStopObservesNoMetaOrActivity re-checks meta from the fresh provider but not activity (activity is only checked on disk pre-restart). Low value.

Positives: Clear/Delete added to all three meta/activity implementations with correct locking and error joins; clearIncarnations dedupes and clears both caller and primary name; the empty-key guard sits at the dispatcher with a stderr diagnostic and a test; ReadyPrompt removal is complete (no dangling non-test references); no accidental edits.

Addressed review findings (iteration 1). Major 1: docs/provider-herdr-parity.md ### start re-verified after the GCREX-26 main.go line shifts — Summary of deviations start row now reads '5 of the 15 gc sends' (was 6), and the ### start paragraph plus deviation rows 1/5/17 citations corrected: dispatch main.go:89 (was 82), opStart main.go:180 (was 162), 'already running' main.go:199-201 (was 182-185), context.Background() main.go:156 (was 138), empty-stdin main.go:186-190 (was 169-173). Major 2: added TestStopWithoutPaneClearsAllMetaAndActivity in internal/provider/incarnation_test.go covering the no-pane branch of Stop; it fails against the pre-fix clearStartIdentity path (meta suspended/GC_DRAIN_ACK/GC_READY_PROMPT_PREFIX/GC_USER_KEY and the activity record survive) and passes with clearIncarnations. Minor 5: TestStartAfterStopObservesNoMetaOrActivity now also asserts a fresh start sees no retained activity record. Verification: go test ./... pass; golangci-lint run ./... 0 issues; make test pass. Still uncommitted for the next review iteration.

Independent review (openrouter/~anthropic/claude-sonnet-latest) — iteration 2. Verdict: pass.

Method: read the uncommitted diff (git diff HEAD) against GCREX-26's AC/DoD; ran go test ./... -count=1 (pass) and golangci-lint run ./... (0 issues); re-checked the parity-doc claims and the corrected main.go/provider.go line citations against the tree; confirmed the no-pane test really takes the no-pane branch (fake.View.Windows=nil makes SessionView omit the block, so Resolve returns !ok).

Previous findings: both majors are addressed. (1) Summary of deviations start row now reads '5 of the 15 gc sends' matching the sand startsand deviation row, and the sand startsand paragraph citations are corrected (dispatch main.go:89, opStart main.go:180, already-running main.go:199-201, context.Background main.go:156, empty-stdin main.go:186-199). (2) TestStopWithoutPaneClearsAllMetaAndActivity now covers the no-pane branch of stop, and TestStartAfterStopObservesNoMetaOrActivity also asserts no retained activity on a fresh start. Coverage verdicts for stop/set-meta/remove-meta flipped to intentional, and Confirmed mistakes + follow-ups no longer list the GCREX-26 items.

ACs: #1 Clear/Delete added to memory, file and composite stores; Stop and its no-pane branch call clearIncarnations, with discriminating unit tests. #2 fresh start observes no meta/activity, tested. #3 empty key rejected (exit 1, stderr) for set/get/remove-meta, tested via TestMetaOpsRejectEmptyKey. #4 ReadyPrompt resolved by removal, no dangling non-test references, watch-startup section notes it. #5 go test ./... and golangci-lint run ./... pass. #6 parity doc re-verified and recomputed.

Blocking: none.
Major: none.

Minor/nit:
1. No-pane Stop resolves the primary name only from bindings.Load(n); that load is not alias-aware, so when Stop is called with an alias and the pane is already gone, stored.Name is empty and clearIncarnations clears only the alias, leaving the primary's meta/activity. This matches the old clearStartIdentity shape and gc normally stops under the start name, so low impact; using binder.lookup(n) in the no-pane branch would close it.
2. The working tree still carries unrelated changes (backlog/config.yml global definition_of_done; gcrex-28/29/30/31/32/33/35 task metadata). Keep the GCREX-26 commit scoped to this task's files.
3. The failed-start rollback at provider.go:335 still calls clearStartIdentity (identity keys only), so a failed start can leave user meta/activity; out of this AC but inconsistent with the new Stop hygiene.

Positives: Clear/Delete fan correctly across namespaces with locking and error joins; clearIncarnations dedupes and clears both the caller and resolved primary; the empty-key guard sits at dispatch with a stderr diagnostic; ReadyPrompt removal is complete; parity citations verified (binding.go:435 Forget, provider.go:759/763/769, stores.go:238/125).

Verification (finalization): go test ./... -count=1 pass; golangci-lint run ./... reports 0 issues; make test pass. Named tests pass: TestStopClearsAllMetaAndActivity, TestStopWithoutPaneClearsAllMetaAndActivity, TestStartAfterStopObservesNoMetaOrActivity (internal/provider/incarnation_test.go), TestMetaOpsRejectEmptyKey (cmd/gc-runtime-rex/dispatch_test.go). Independent review iteration 2: pass, no blocking or major findings.
<!-- SECTION:NOTES:END -->

## Comments

<!-- COMMENTS:BEGIN -->
author: @mc-wisp-gk2sr5
created: 2026-11-18T15:48:00Z

---

Claimed by Gas City worker mc-wisp-gk2sr5 (seat rex-gascity/pi-1, session pi-mc-wisp-gk2sr5, template rex-gascity/pi); workflow bead mc-4km, step bead mc-z1c.

---
<!-- COMMENTS:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
Incarnation hygiene is now complete: Stop (and its no-pane branch) call Provider.clearIncarnations, which clears every meta key for the caller and resolved primary name plus the activity entry, via new MetaStore.Clear / ActivityStore.Delete implementations in the memory, file and composite stores. The dispatcher rejects an empty key for set-meta/get-meta/remove-meta with exit 1 and a stderr diagnostic. Binding.ReadyPrompt, which was written and never read, was removed along with its record field, GC_READY_PROMPT_PREFIX fallback read and Config/startConfig.ReadyPromptPrefix. docs/provider-herdr-parity.md re-verifies the stop, set-meta, get-meta/remove-meta and watch-startup sections, recomputes Coverage and Summary of deviations, and drops the resolved mistakes and follow-ups.

Verification: go test ./... -count=1 pass; golangci-lint run ./... 0 issues; make test pass. Named tests: TestStopClearsAllMetaAndActivity, TestStopWithoutPaneClearsAllMetaAndActivity, TestStartAfterStopObservesNoMetaOrActivity, TestMetaOpsRejectEmptyKey.

Risks/follow-ups (out of scope, recorded by review): the failed-start rollback still calls clearStartIdentity (identity keys only), and the no-pane Stop branch resolves the primary name only from a non-alias-aware bindings.Load(n); neither is a parity regression.
<!-- SECTION:FINAL_SUMMARY:END -->
