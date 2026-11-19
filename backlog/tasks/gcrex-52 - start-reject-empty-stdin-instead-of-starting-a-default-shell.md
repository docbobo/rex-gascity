---
id: GCREX-52
title: 'start: reject empty stdin instead of starting a default shell'
status: Done
assignee:
  - '@mc-wisp-nck79h'
created_date: '2026-11-18T22:05:00Z'
updated_date: '2026-11-19T02:47:00Z'
labels:
  - provider
  - herdr
  - for-gascity
  - tdd
dependencies: []
references:
  - docs/provider-method-audit/start.md
documentation:
  - docs/provider-herdr-method-io.md
priority: medium
type: task
ordinal: 108000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Develop test-first (TDD): write the failing test that reproduces this first, watch it go red, then the smallest change that makes it green (repo rule in AGENTS.md; see also the tdd skill).

Avoids: a malformed request silently launches a shell.

opStart only unmarshals when stdin is non-empty after trimming, so an empty body starts a default shell and exits 0. The document (## start § Failure cases) requires exit 1.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 Empty or whitespace-only stdin exits 1 with a message on stderr.
- [x] #2 gc real start calls still succeed, covered by the existing dispatch tests.
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. TDD red first: add dispatcher tests that empty and whitespace-only stdin to start exit 1, write nothing to stdout, name the start config in stderr, and launch no pane (fake CreatedWindows/CreatedSplits stay empty). 2. Change opStart (cmd/gc-runtime-rex/main.go) to fail when the trimmed body is empty instead of treating it as {}; keep JSON parsing for non-empty bodies. 3. Verify gc real calls still succeed: gc v1.5.1 always marshals a start config object (internal/runtime/exec/hoststage.go:95-103, json.go:57-100), so its stdin is non-empty; existing dispatch start tests cover the success path. 4. Run go test ./... and golangci-lint run ./...; update docs/provider-method-audit/start.md and docs/provider-herdr-parity.md to move the empty-stdin row from Deviations to Matches. Leave uncommitted.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Implemented: opStart now rejects an empty or whitespace-only stdin body with 'start config: empty stdin' instead of treating it as {}; red test TestStartEmptyStdinFails (empty and whitespace subtests) failed against the old code with exit 0 and a launched pane, and passes after the fix. Docs: moved the empty-stdin row from Deviations to Matches in docs/provider-method-audit/start.md and updated docs/provider-herdr-parity.md row 17 and the start Our-implementation paragraph. Verification: go test -count=1 ./... all packages ok; golangci-lint run ./... 0 issues. gc v1.5.1 always marshals a start config object (internal/runtime/exec/hoststage.go:95-103, json.go:57-100), so real start calls send non-empty stdin and the existing dispatch start tests still pass. Change left uncommitted for review.

Independent review (openrouter/deepseek/deepseek-v4.1-flash:high). Verified: TestStartEmptyStdinFails is red against HEAD (exit 0, pane launched) and green with the change; go test -count=1 ./... passes; golangci-lint run ./cmd/gc-runtime-rex/... 0 issues. AC#1 met: empty/whitespace-only stdin exits 1 with `parse start config: empty stdin` on stderr, empty stdout, no pane. AC#2 met: gc v1.5.1 always marshals a start-config object (`exec/json.go:44-68` json.Marshal of a struct, so at least `{}`; `exec/exec.go:79-99` pipes non-nil data to stdin; `exec/exec.go:219-223` start call), so real stdin is never empty; existing start tests pass. Minor: plan/notes cite gc v1.5.1 as `internal/runtime/exec/hoststage.go:95-103, json.go:57-100`; hoststage.go does not exist at tag v1.5.1 (working-tree file) and json.go is 68 lines there (marshalStartConfig at :44-68). Correct baseline citations: `exec/json.go:44-68`, `exec/exec.go:79-99,219-223`; the claim itself is verified true. Minor follow-up: non-object JSON `null` still starts a default shell, exit 0 (verified: code=0, windows=1 in a scratch copy); herdr-core's start failure cases list non-object as exit 1; outside this task's ACs and not overclaimed in the docs. Nit: the parity-doc start paragraph keeps stale pre-existing refs (`opStart` at main.go:180, decode at 186-199, 218) while opStart sits at main.go:230 in both HEAD and the change; row 17's new `main.go:230-243` is correct. Out of scope: the tracked gcrex-27/gcrex-28 task-markdown edits are unrelated backlog CLI normalization. Verdict: pass.

Finalize verification: TestStartEmptyStdinFails is red against HEAD (empty/whitespace stdin exit 0, pane launched) and green with the change; go test -count=1 ./... all packages ok; golangci-lint run ./... 0 issues; git diff --cached will list only cmd/gc-runtime-rex/main.go, cmd/gc-runtime-rex/dispatch_test.go, docs/provider-herdr-parity.md, docs/provider-method-audit/start.md, and this task file.
<!-- SECTION:NOTES:END -->

## Comments

<!-- COMMENTS:BEGIN -->
author: @mc-wisp-nck79h
created: 2026-11-19T02:33:00Z

---

Claimed by Gas City worker mc-wisp-nck79h (seat rex-gascity/pi-1, session pi-mc-wisp-nck79h, template rex-gascity/pi); workflow bead mc-a5ky, step bead mc-e453.

---
<!-- COMMENTS:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
start now rejects an empty or whitespace-only stdin body with 'parse start config: empty stdin' (exit 1, empty stdout, nothing launched) instead of treating it as {} and starting a default shell, matching herdr-core's start failure case. The change touches opStart in cmd/gc-runtime-rex/main.go plus a dispatcher test (TestStartEmptyStdinFails) covering empty and whitespace bodies, and moves the empty-stdin row from Deviations to Matches in docs/provider-method-audit/start.md and docs/provider-herdr-parity.md. Verification: the new test fails without the change and passes with it; go test ./... and golangci-lint run ./... are clean. gc v1.5.1 always marshals a start-config object, so real start calls send non-empty stdin and are unaffected (existing dispatch start tests cover the success path). Follow-ups (minor, out of scope): non-object JSON null still starts a default shell; the parity-doc start paragraph keeps stale pre-existing line refs.
<!-- SECTION:FINAL_SUMMARY:END -->
