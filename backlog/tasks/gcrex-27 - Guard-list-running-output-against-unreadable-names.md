---
id: GCREX-27
title: Guard list-running output against unreadable names
status: Done
assignee:
  - '@mc-wisp-y7ux10'
created_date: '2026-11-18T06:14:00Z'
updated_date: '2026-11-19T04:45:00Z'
labels:
  - provider
  - for-gascity
dependencies: []
references:
  - docs/provider-herdr-parity.md
  - internal/provider
  - cmd/gc-runtime-rex
priority: low
type: task
ordinal: 52000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
ListRunning prints every binding name without validation. gc parses the output line by line, so a name that is empty, padded or contains a control character would corrupt that parse; herdr-core drops such names with a readable_name check (parity doc ### list-running row 3, mistake 10). Low risk because names come from gc, but the guard is one function.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 ListRunning skips any name that is empty, has leading or trailing whitespace, or contains a control character.
- [x] #2 A unit test pins the filter for all three cases and confirms valid alias names still appear.
- [x] #3 go test ./... and golangci-lint run ./... pass.
- [x] #4 docs/provider-herdr-parity.md is updated in the same change: the affected section row(s) are re-verified after the fix (verdict changed, or a note added saying the deviation is now resolved), the 'Summary of deviations' row(s) and the 'Coverage' table are recomputed, and 'Confirmed mistakes' / 'Proposed follow-up work' no longer list the item.
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. Red test: TestListRunningSkipsUnreadableNames in internal/provider/phase4_test.go saves bindings with empty, leading-space, trailing-space, and control-char names plus one valid name and one valid alias of an unreadable binding; it asserts the empty-prefix listing keeps only the valid name and a prefix probe still lists the valid alias. Watch it go red. 2. Code: add readableName(name string) bool (non-empty, name == strings.TrimSpace(name), no unicode.IsControl rune) to internal/provider/provider.go and apply it to the primary name and each alias before ListRunning appends them. 3. Docs: update docs/provider-herdr-parity.md (coverage verdict for list-running, the summary-of-deviations name-filter row, the Confirmed mistakes entry, the detailed Our implementation paragraph, row 3, the section verdict, cross-references, and follow-ups) and move the readable_name deviation row to Matches in docs/provider-method-audit/list-running.md. 4. Verify go test ./... and golangci-lint run ./...; leave uncommitted for the review step.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Implemented (TDD). Red: TestListRunningSkipsUnreadableNames failed with ListRunning(all) = ["" " Indigo--lead" "Indigo--alias-ctl\x02" "Indigo--ctl\x01" "Indigo--good" "Indigo--trail "] and the alias-prefix listing returned the unreadable primary too. Green: added readableName (non-empty, name == strings.TrimSpace(name), no unicode.IsControl rune) in internal/provider/provider.go and applied it to the primary name and each alias before ListRunning appends them; the filter mirrors herdr-core readable_name (rpp/wire.py:194). Verified: go test -count=1 ./... all packages ok; golangci-lint run ./... 0 issues; gofmt -l cmd internal empty. Docs: docs/provider-herdr-parity.md coverage verdict list-running now intentional, summary name-filter row rewritten, the Confirmed mistakes table emptied with a GCREX-27 note, the detailed Our implementation paragraph and row 3 now describe readableName, the section verdict is intentional, cross-references add GCREX-27, and the name-filter follow-up is removed; docs/provider-method-audit/list-running.md moved the readable_name row from Deviations to Matches. Left uncommitted for review.

Independent review (openrouter/deepseek/deepseek-v4.1-flash:high).

**Blocking:** `docs/provider-herdr-parity.md` § "Proposed follow-up work" still lists item 1 "**List output guard.** Skip names that are empty, padded or contain control characters. Closes 1." (lines 286-287). AC #4 requires that section to no longer list the item; the "Confirmed mistakes" table it refers to is now empty, so "Closes 1" dangles. Remove item 1 and re-read the section intro after removal.

**Minor/nit:** the rationale "a name with a newline or padding would corrupt gc's line parse" (summary row line 132, deviation row 3 line 545) is loose for padding. gc splits stdout on "\n" (`internal/runtime/exec/exec.go:763-772` at v1.5.1, after `TrimRight(..., "\n")` at `exec.go:104`), so only a newline splits one session into two lines; a padded name yields a name gc cannot match rather than a split. Wording is inherited from the old mistake row; tighten it or leave it.

Verified independently: `readableName` mirrors herdr-core `readable_name` at the pinned reference `gastownhall/gascity-packs@0f07496` (`rpp/wire.py:194`: non-empty, `name == name.strip()`, no `Cc`; applied in `op_list_running`, `reads.py:319`). Go's `strings.TrimSpace` + `unicode.IsControl` is equivalent: Python's extra C0 separators (\x1c-\x1f) are `Cc` and rejected by both. `TestListRunningSkipsUnreadableNames` goes red at HEAD `provider.go` (all unreadable names listed, alias probe leaks the unreadable primary) and green with the fix. `go test -count=1 ./...` all ok; `golangci-lint run ./...` 0 issues; `gofmt -l cmd internal` empty. Parity doc rows/section verdict/coverage/cross-references and the audit-doc move to Matches are otherwise accurate.

Out of scope (other tasks' working-tree state, not findings here): the untracked `docs/provider-herdr-method-deviations.md` still carries the old `readable_name` deviation row (line 177) and must be updated where it lands, or it will contradict the per-op audit; `backlog/tasks/gcrex-28 ...md` has unrelated formatting-only edits.

Verdict: fail.

Addressed review findings: removed the now-resolved 'List output guard' entry (and its dangling 'Closes 1' reference) from the 'Proposed follow-up work' section of docs/provider-herdr-parity.md and dropped the now-false claim that the numbering refers to the mistake table; tightened the name-filter rationale in the summary row and list-running deviation row 3 to say a newline would split one session across two output lines and padding would leave a name gc cannot match (verified against gc v1.5.1 internal/runtime/exec/exec.go:104 and :771-772), rather than the loose 'newline or padding would corrupt the parse'. go test ./... ok; golangci-lint run ./... 0 issues.

Independent review (openrouter/deepseek/deepseek-v4.1-flash:high), iteration 2.

**Blocking: none.** The iteration-1 blocking item is resolved: `docs/provider-herdr-parity.md` § "Proposed follow-up work" no longer lists the list-output guard and no longer points at the (now empty) mistake-table numbering. Both tightened rationales read correctly: a newline would split one session across two output lines, and padding would leave a name gc cannot match.

**Minor/nit:**
- The line citations in the touched "Our implementation" paragraph were already stale before this change and remain so: `provider.go:919-...` is beyond EOF (HEAD's file is 903 lines; this change brings it to 921), and `provider.go:506-509,551-562`, `:520-536`, `:538`, `:652-665` now point at unrelated code (`nextSplit`/`rectArea`, `PeekLines`). The same drift shows in the audit-doc rows (`provider.go:658-671`, `:653-659`, `:635-657`). Not introduced here; recommend a separate line-reference refresh pass rather than folding it into this change. The new `readableName` claim cites the file only, so it adds no new drift.
- Out of scope, as in iteration 1: the untracked `docs/provider-herdr-method-deviations.md` still carries the stale `readable_name` deviation row (line 177, "perform no ... filter"), which will contradict the per-op audit when it lands; the task that owns it must refresh it. Other untracked files were not authored by GCREX-27 and were ignored.

**Verified independently:** `readableName` mirrors herdr-core `readable_name` at the pinned `gastownhall/gascity-packs@0f07496` (`rpp/wire.py:194-196`: non-empty, `name == name.strip()`, no `Cc`; used in `op_list_running`, `reads.py:319`); Go's `strings.TrimSpace` + `unicode.IsControl` rejects the same names. gc v1.5.1 side: `internal/runtime/exec/exec.go:104` strips trailing newlines and `ListRunning` splits the rest on `"\n"` (`exec.go:761-771`), so the newline/padding rationale is accurate. The test is genuinely red→green: with HEAD's `provider.go` plus the new test, `TestListRunningSkipsUnreadableNames` fails exactly as recorded (all unreadable names listed; the alias probe also leaks the unreadable primary); in this working tree it passes. `go test -count=1 ./...` and `make test` pass, `golangci-lint run ./...` reports 0 issues, `gofmt -l cmd internal` is empty. The coverage table, summary row, emptied "Confirmed mistakes" section, section verdict, cross-references and follow-up removal are coherent; no stale `mistake` reference to the name filter remains in the parity doc, and the audit report moved the row to Matches.

Verdict: pass.

Finalization verification (2026-11-19): go test -count=1 ./... all packages ok (cmd/gc-runtime-rex 6.7s, internal/launch 0.6s, internal/provider 7.3s, internal/rex/cli 6.6s); golangci-lint run ./... 0 issues; gofmt -l cmd internal empty. TestListRunningSkipsUnreadableNames pins AC #1/#2 and passes; the readableName guard is applied to the primary name and each alias in ListRunning (internal/provider/provider.go). Review-loop verdict pass (iteration 2).
<!-- SECTION:NOTES:END -->

## Comments

<!-- COMMENTS:BEGIN -->
author: @mc-wisp-y7ux10
created: 2026-11-19T04:26:00Z

---

Claimed by Gas City worker mc-wisp-y7ux10 (seat rex-gascity/pi-1, session pi-mc-wisp-y7ux10, template rex-gascity/pi); workflow bead mc-t76v, step bead mc-lyn6.

---
<!-- COMMENTS:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
Added readableName(name) to internal/provider/provider.go (non-empty, name == strings.TrimSpace(name), no unicode.IsControl rune) and applied it to the primary binding name and every alias before ListRunning appends them, so a name gc cannot parse back is never printed. Pinned by TestListRunningSkipsUnreadableNames in internal/provider/phase4_test.go, which covers empty, leading-space, trailing-space, and control-char names plus a readable alias of an unreadable primary. Docs updated in the same change: docs/provider-herdr-parity.md (coverage verdict, summary-of-deviations name-filter row, emptied Confirmed mistakes table, detailed Our implementation paragraph, deviation row 3, section verdict, cross-references, follow-ups) and docs/provider-method-audit/list-running.md (readable_name row moved to Matches). Verification: go test -count=1 ./... all ok; golangci-lint run ./... 0 issues; gofmt -l cmd internal empty. Review-loop reached a passing verdict (independent review iteration 2: blocking none). Risks/follow-ups: pre-existing stale line citations in the list-running 'Our implementation' prose were noted by review but left for a separate line-reference refresh pass; docs/provider-herdr-method-deviations.md still carries the stale readable_name deviation row and must be refreshed by the task that owns it.
<!-- SECTION:FINAL_SUMMARY:END -->
