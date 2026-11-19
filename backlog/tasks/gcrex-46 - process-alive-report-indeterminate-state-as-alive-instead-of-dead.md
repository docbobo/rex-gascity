---
id: GCREX-46
title: 'process-alive: report indeterminate state as alive instead of dead'
status: Done
assignee:
  - '@mc-wisp-c2cd49'
created_date: '2026-11-18T22:05:00Z'
updated_date: '2026-11-19T01:39:00Z'
labels:
  - provider
  - herdr
  - for-gascity
  - tdd
dependencies: []
references:
  - docs/provider-method-audit/process-alive.md
documentation:
  - docs/provider-herdr-method-io.md
priority: medium
type: task
ordinal: 102000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Develop test-first (TDD): write the failing test that reproduces this first, watch it go red, then the smallest change that makes it green (repo rule in AGENTS.md; see also the tdd skill).

Avoids: gc killing or ignoring an agent whose state we cannot read.

processAlive in internal/provider/liveness.go is strict: any non-matching or empty Foreground/Ancestors returns false, and a non-ErrNotFound BlockProcess error exits 1. The document (## process-alive § Success output, § Failure cases) says indeterminate state returns exit 0 with empty stdout, which gc reads as alive. The audit rates this medium.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 An unreadable process table or an opaque process with no match returns exit 0 with empty stdout.
- [x] #2 A definite no-match with a fully readable process table still returns false.
- [x] #3 Transient BlockProcess errors are not reported to gc as dead.
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. Introduce a three-valued liveness result in internal/provider/liveness.go: LivenessUnknown/LivenessAlive/LivenessDead. processAlive returns Unknown when the process table is empty or holds an opaque (no name/path/argv0) process after no match, Alive on a match, Dead only from a fully readable table with no match; LastExit still forces Dead; empty names still Alive. 2. Provider.ProcessAlive returns the tri-state: BlockProcess ErrNotFound -> Dead (unchanged, GCREX-23); any other BlockProcess error -> Unknown (not exit 1); resolve/no-binding errors unchanged. 3. opProcessAlive (cmd/gc-runtime-rex/main.go) prints true/false for Alive/Dead and nothing (exit 0) for Unknown. 4. TDD tests: provider table cases for empty/opaque -> Unknown and readable no-match -> Dead; dispatcher exit-0-empty on a transient BlockProcess error. Update existing processAlive table expectations. 5. Docs: docs/provider-herdr-parity.md process-alive row 4 + summary/coverage; docs/provider-method-audit/process-alive.md move the indeterminate row to Matches; docs/REX-CONTRACT.md process-alive output vocabulary. 6. go test ./... and golangci-lint run ./... .
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Implemented (uncommitted, for review). Provider: introduced a three-valued Liveness (LivenessUnknown/Alive/Dead) in internal/provider/liveness.go. processAlive returns Unknown when the process list is empty or holds an opaque process (no name/path/argv0) after no match, Alive on a match or empty names, and Dead only from a fully readable table with no match or a non-nil LastExit. Provider.ProcessAlive returns the tri-state: BlockProcess ErrNotFound stays Dead (GCREX-23); any other BlockProcess error is now Unknown (not exit 1); binding/no-binding and resolve errors unchanged. Dispatcher: opProcessAlive prints true/false for Alive/Dead and nothing (exit 0) for Unknown, the pack cannot-tell answer gc reads as alive. TDD red->green: TestProcessAliveTransientErrorIsIndeterminate and TestProcessAliveEmptyTableIsIndeterminate (cmd/gc-runtime-rex/dispatch_test.go) failed first (exit 1 / false); TestProcessAliveTransientProcessErrorIsUnknown (internal/provider/teardown_test.go) and the new processAlive table cases (empty table, opaque process, opaque ancestor) pass. Docs: docs/provider-herdr-parity.md process-alive section row 4 removed and rows renumbered, coverage count 7->6, verdict and Confirmed-mistakes note updated; docs/provider-method-audit/process-alive.md moved the indeterminate row to Matches; docs/REX-CONTRACT.md process-alive output vocabulary now lists the empty cannot-tell answer. Verification: go test -count=1 ./... all packages ok; golangci-lint run ./... 0 issues; gofmt -l cmd internal empty. Change left uncommitted for review.

Independent review (openrouter/deepseek/deepseek-v4.1-flash:high): Blocking: none. Major: none. Minor follow-ups: (1) The compiled summary docs/provider-herdr-method-deviations.md is stale. Its process-alive section still lists the fixed medium indeterminate row (line 168) with pre-change line citations; the verdict table still counts 5 deviations / 1 medium / 3 low (line 38); the total row still says 136/18/104/7 (line 55); and Tracked fix work still says 18 medium. The per-op report now has 4 deviations (1 high, 0 medium, 3 low) with the cannot-tell row in Matches. The summary is a GCREX-39 artifact outside the docs listed for this task, so regenerate or update it as a follow-up. (2) Nit: the match-before-opaque ordering is not pinned by a test; a matching foreground plus an opaque ancestor returns Alive, but only the non-matching opaque case is tested. (3) Nit: the task acceptance checkboxes are still unchecked; tick them in the address step. Verified: AC1-3 met; go test -count=1 ./... all packages ok; gofmt -l cmd internal empty; golangci-lint run ./... 0 issues. The new dispatcher tests fail without the change (transient error -> exit 1; empty table -> false). The cannot-tell answer matches gc v1.5.1 exec/exec.go:709-718 (empty stdout read as alive) and herdr-core reads.py:301-302. The remaining Core unreachable Low deviation (resolve errors still exit 1) is intentionally out of scope; the interpreter-script High deviation remains GCREX-47. Verdict: pass.

Finalization verification: go test -count=1 ./... -> all packages ok; targeted TestProcessAlive (empty table/opaque -> unknown), TestProcessAliveMatchesByName (readable no-match -> dead), TestProcessAliveTransientProcessErrorIsUnknown, TestProcessAliveTransientErrorIsIndeterminate, and TestProcessAliveEmptyTableIsIndeterminate pass; gofmt -l cmd internal -> empty; golangci-lint run ./... -> 0 issues. AC1-3 checked against those tests. Note: the process-alive audit-doc update lives in the untracked docs/provider-method-audit/ artifact from GCREX-39 and is deliberately not staged, to avoid sweeping unrelated uncommitted work into this commit.
<!-- SECTION:NOTES:END -->

## Comments

<!-- COMMENTS:BEGIN -->
author: @mc-wisp-c2cd49
created: 2026-11-19T01:21:00Z

---

Claimed by Gas City worker mc-wisp-c2cd49 (seat unknown, session pi-mc-wisp-c2cd49, template unknown); workflow bead mc-vyjh, step bead mc-hb2v.

---
<!-- COMMENTS:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
process-alive now reports indeterminate liveness as alive instead of dead: a transient BlockProcess error, an empty process table, or an opaque process with no match returns exit 0 with empty stdout, which gc reads as alive. A definite no-match against a fully readable table still returns false, and LastExit still forces dead. Verified with the provider and dispatcher tests plus go test ./..., gofmt, and golangci-lint (0 issues). Follow-ups: interpreter-script matching remains GCREX-47; the GCREX-39 compiled summary docs/provider-herdr-method-deviations.md is stale and still lists the fixed deviation.
<!-- SECTION:FINAL_SUMMARY:END -->
