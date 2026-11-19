---
id: GCREX-55
title: 'start: pass an empty pre_start env value as unset, not as an empty string'
status: Done
assignee:
  - '@mc-wisp-s673g0'
created_date: '2026-11-18T22:06:00Z'
updated_date: '2026-11-19T03:54:00Z'
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
ordinal: 111000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Develop test-first (TDD): write the failing test that reproduces this first, watch it go red, then the smallest change that makes it green (repo rule in AGENTS.md; see also the tdd skill).

Avoids: hooks see K= empty where the contract says unset, changing shell behaviour.

MergeEnv sets K= for an empty value; the document (## start § Inputs) treats "" as unset.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 An empty value in the pre_start env unsets the variable for the hook and the launched command.
- [x] #2 A non-empty value is passed through unchanged, with a test for both.
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. Red tests (internal/launch/mergeenv_test.go): add TestMergeEnvEmptyValueUnsets (overlay {"A":"","C":""} pops A from base and omits C; non-empty {"A":"new"} still replaces) and update TestMergeEnvOverlayReplacesAndSorts to drop the C= expectation; watch red. 2. Code: in internal/launch/launch.go mergeEnv, an overlay entry with an empty value deletes the key from the merged map instead of writing K= (matching launch.command's env -u K and herdr-core envplan.agent_env/_setup_env). 3. Verify go test ./... and golangci-lint run ./... plus gofmt. 4. Docs: remove the now-resolved 'pre_start env treats "" as unset ... sets K=' deviation from docs/provider-method-audit/start.md and record it under the implementation/matches section; refresh the docs/provider-herdr-parity.md start row that carries the empty-env rule if it still marks it a gap. Leave uncommitted.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Implemented (TDD). Red: TestMergeEnvEmptyValueUnsets failed (got [A= B=1 C=], want [B=1]) and TestPreStartEnvEmptyValueIsUnset failed (hook saw `MY_EMPTY_VAR` `set`), both captured with internal/launch/launch.go reverted to HEAD via git stash. Green: mergeEnv (internal/launch/launch.go:233-249) now deletes an overlay key whose value is empty instead of writing K=, so Plan.MergedEnv (internal/provider/provider.go:409) passes an empty pre_start env value to the hook unset; the launched command already emitted `env -u KEY` (TestLaunchCommandEmptyEnvIsUnset). Tests: TestMergeEnvEmptyValueUnsets, TestMergeEnvNonEmptyValuePassedThrough (internal/launch/mergeenv_test.go), TestPreStartEnvEmptyValueIsUnset (internal/provider/prestart_test.go); updated TestMergeEnvOverlayReplacesAndSorts. Verified: go test -count=1 ./... all packages ok; golangci-lint run ./... 0 issues; gofmt -l cmd internal empty. Sources: gc v1.5.1 docs/reference/exec-session-provider.md:269 ('An empty value means the variable must be unset'); herdr-core envplan.py:67-71 (agent_env) and lifecycle.py:599-609 (_setup_env) pop empty session keys. Docs: removed the now-resolved 'pre_start env treats "" as unset ... sets K=' deviation row from docs/provider-method-audit/start.md and added a Matches bullet; refreshed docs/provider-herdr-parity.md row 11 and the pre_start summary row. Left uncommitted for review.

Independent review (openrouter/deepseek/deepseek-v4.1-flash:high); scope: the uncommitted GCREX-55 change (mergeEnv, the two test files, the audit and parity docs; unrelated gcrex-27/28 formatting and other untracked files excluded). Verified: mergeEnv (internal/launch/launch.go:233-252) deletes an empty overlay key, so a pre_start hook sees it unset (TestPreStartEnvEmptyValueIsUnset) and the launched command still emits env -u (TestLaunchCommandEmptyEnvIsUnset); non-empty values pass through (TestMergeEnvNonEmptyValuePassedThrough, TestPreStartEnv). Red evidence confirmed by construction from HEAD: the old m[k]=v produced K= and both new tests fail against it. Sources re-verified: gc v1.5.1 docs/reference/exec-session-provider.md:269 (empty value means the variable must be unset); herdr-core @ 0f07496 herdrcore/envplan.py:67-71 and herdrcore/lifecycle.py:599-609 (both pop empty session keys). Gates re-run: go test -count=1 ./... pass; gofmt -l cmd internal clean; golangci-lint run ./... 0 issues; git diff --check clean. Findings: none blocking or major. Minor follow-ups: (1) the audit bullet cites internal/launch/launch.go:233-249 while mergeEnv spans 233-252; (2) untracked compiled artifact docs/provider-herdr-method-deviations.md:93 still lists the pre_start empty-env deviation as open - the known-stale GCREX-39 artifact already carried by the GCREX-40 and GCREX-47 reviews. Verdict: pass.

Finalization (mc-f4n1): re-ran the gates on the reviewed change: go test -count=1 ./... (make test) all packages ok; golangci-lint run ./... 0 issues; gofmt -l cmd internal empty; git diff --check clean. Confirmed the gc v1.5.1 source for the cited rule: docs/reference/exec-session-provider.md (v1.5.1) states 'An empty value means the variable must be unset.' Fixed the review's minor follow-up by correcting the mergeEnv line citation in docs/provider-method-audit/start.md to internal/launch/launch.go:233-252 (the second follow-up, the stale untracked docs/provider-herdr-method-deviations.md:93 artifact, belongs to GCREX-39 and is not part of this change).
<!-- SECTION:NOTES:END -->

## Comments

<!-- COMMENTS:BEGIN -->
author: @mc-wisp-s673g0
created: 2026-11-19T03:43:00Z

---

Claimed by Gas City worker mc-wisp-s673g0 (seat rex-gascity/pi, session pi-mc-wisp-s673g0, template rex-gascity/pi); workflow bead mc-agt9, step bead mc-qza4.

---

author: @mc-wisp-b37s5x
created: 2026-11-19T03:48:00Z

---

Independent review verdict: pass. No blocking or major findings; two minor follow-ups are recorded in the implementation notes. External claims re-checked against gc v1.5.1 and herdr-core @ 0f07496; go test ./..., gofmt and golangci-lint all clean.

---
<!-- COMMENTS:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
mergeEnv (internal/launch/launch.go:233-252) now deletes an overlay key whose value is empty instead of writing K=, so a pre_start hook (Plan.MergedEnv via internal/provider/provider.go:409) and the merged launch env see the variable unset, matching gc v1.5.1 (docs/reference/exec-session-provider.md: 'An empty value means the variable must be unset') and herdr-core envplan/setup-hook handling. Non-empty values pass through unchanged; the launched command already emitted env -u KEY for empty values. Tests: TestMergeEnvEmptyValueUnsets, TestMergeEnvNonEmptyValuePassedThrough (internal/launch/mergeenv_test.go) and TestPreStartEnvEmptyValueIsUnset (internal/provider/prestart_test.go), plus the updated TestMergeEnvOverlayReplacesAndSorts. Docs: removed the resolved deviation from docs/provider-method-audit/start.md and refreshed the two start rows in docs/provider-herdr-parity.md. Verification: make test all packages ok; golangci-lint run ./... 0 issues; gofmt/git diff --check clean. Risk: none observed; the stale GCREX-39 deviations artifact is tracked there as a follow-up.
<!-- SECTION:FINAL_SUMMARY:END -->
