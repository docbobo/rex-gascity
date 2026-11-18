---
id: GCREX-40
title: Read GC_EXEC_CITY_PATH and GC_EXEC_STATE_DIR alongside the GC_REX_* fallbacks
status: Done
assignee:
  - '@mc-wisp-ptfbu7'
created_date: '2026-11-18T22:05:00Z'
updated_date: '2026-11-18T23:35:00Z'
labels:
  - provider
  - herdr
  - for-gascity
  - tdd
dependencies: []
references:
  - docs/provider-method-audit/is-running.md
documentation:
  - docs/provider-herdr-method-io.md
priority: medium
type: task
ordinal: 96000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Develop test-first (TDD): write the failing test that reproduces this first, watch it go red, then the smallest change that makes it green (repo rule in AGENTS.md; see also the tdd skill).

Avoids: the dispatcher ignores gc's own env contract.

Every op resolves its state root through provider.NamespaceForCaller(), which reads GC_REX_CITY and GC_REX_STATE_DIR/XDG_STATE_HOME. The document (## Shared RPP contract § Dispatch #8) has gc passing GC_EXEC_CITY_PATH and GC_EXEC_STATE_DIR, so a caller that sets only gc's variables silently falls back to $XDG_STATE_HOME/rex-gascity. The audit hits this in is-running and remove-meta (docs/provider-method-audit/is-running.md, remove-meta.md); the fix is one resolution path, so one task covers both.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 GC_EXEC_CITY_PATH selects the city and GC_EXEC_STATE_DIR (absolute) sets the state root, with the existing GC_REX_* variables kept as fallbacks.
- [x] #2 A test covers the precedence order between the GC_EXEC_* and GC_REX_* variables.
- [x] #3 Behaviour with neither variable set is unchanged.
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. Add failing tests: StateDir prefers GC_EXEC_STATE_DIR over GC_REX_STATE_DIR (and rejects a relative GC_EXEC_STATE_DIR), falling back to GC_REX_STATE_DIR; CityFromEnv/CityCandidates prefer GC_EXEC_CITY_PATH over GC_REX_CITY; NamespaceForCaller treats GC_EXEC_CITY_PATH as a pin (no sibling discovery).
2. Add one env-pin helper (cityPin) used by CityFromEnv, CityCandidates and NamespaceForCaller; read GC_EXEC_STATE_DIR first in stateBase, keeping GC_REX_STATE_DIR as the fallback.
3. Run go test ./... and golangci-lint run ./...; leave the change uncommitted for the review step.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Implemented one env resolution path: cityPin() reads GC_EXEC_CITY_PATH then GC_REX_CITY (CityFromEnv, CityCandidates, NamespaceForCaller all use it); stateBase() reads GC_EXEC_STATE_DIR (absolute, else error) then GC_REX_STATE_DIR then XDG, so is-running and remove-meta both scope by gc's variables. New tests: TestStateDirExecStateDirOverride, TestStateDirExecFallsBackToRex, TestStateDirExecRelativeRejected, TestCityFromEnvPrefersExecCityPath, TestCityCandidatesExecCityPathIsPin, TestNamespaceForCallerExecCityPathIsPin. go test ./... and golangci-lint run ./... pass. Left uncommitted for review.

Independent review (openrouter/deepseek/deepseek-v4.1-flash:high).

Scope: working-tree diff vs HEAD — internal/provider/statedir.go, internal/provider/namespace.go, the three test files, and this task's markdown. Untracked runtime dirs (.pi/, .pi-agent/, .gc/) and other tasks' files ignored.

Verification:
- Re-ran go test ./... (pass) and golangci-lint run ./... (0 issues).
- AC#1: cityPin() (statedir.go:86) reads GC_EXEC_CITY_PATH then GC_REX_CITY; CityFromEnv (:108), CityCandidates (:218) and NamespaceForCaller (namespace.go:53) all use it. stateBase() (:18) reads GC_EXEC_STATE_DIR (absolute, else error) then GC_REX_STATE_DIR then XDG/HOME. main.go:164 turns the state-dir error into exit 1 + stderr, matching the documented state-dir failure (relative GC_EXEC_STATE_DIR -> exit 1; docs/provider-herdr-method-io.md failure row, citing rpp/statedir.py:187-190).
- AC#2: precedence tests TestStateDirExecStateDirOverride, TestStateDirExecFallsBackToRex, TestStateDirExecRelativeRejected, TestCityFromEnvPrefersExecCityPath, TestCityCandidatesExecCityPathIsPin, TestNamespaceForCallerExecCityPathIsPin.
- AC#3: XDG/home fallback tests set GC_EXEC_STATE_DIR="" and keep prior results; all pre-existing tests pass.

Source check (AGENTS.md): docs/provider-herdr-parity.md:469 is silent on GC_EXEC_*; pinned gc v1.5.1 does not set GC_EXEC_CITY_PATH (docs/provider-herdr-method-io.md:191 records that) and only documents GC_EXEC_STATE_DIR (v1.5.1 docs/reference/exec-session-provider.md:402, "if set"). Current gc edge sets GC_EXEC_CITY_NAME/GC_EXEC_CITY_PATH on every op (gascity edge internal/runtime/exec/exec.go:108-125). The change is therefore forward-compatible with the documented RPP contract and a no-op under v1.5.1 (verified). Note the task wording says the document "has gc passing" both vars; the document itself attributes them to the pack's OpContext derivation, not to gc v1.5.1.

Findings — minor/nit only, no blocking or major:
- minor: README.md:115-116 and docs/rex-provider.md:303-308 still document only GC_REX_CITY/GC_REX_STATE_DIR; the canonical env contract is stale. Follow-up: record GC_EXEC_* precedence there.
- minor: docs/provider-herdr-parity.md:469 (is-running) calls a caller unpinned when it has "no GC_REX_CITY/GC_CITY_PATH/GC_CITY"; unpinned now also requires GC_EXEC_CITY_PATH unset.
- minor (cross-task): docs/provider-method-audit/is-running.md:9, remove-meta.md:9 and docs/provider-herdr-method-deviations.md:388 still list this GCREX-40 deviation as open; update when the change commits.
- nit: pre-existing env-dependent tests do not clear the new variables (e.g., citynamespace_test.go TestLookupSpansCandidateNamespaces, TestUnpinnedLookupDiscoversPinnedCity). Reproduced read-only: GC_EXEC_CITY_PATH=/x GC_EXEC_STATE_DIR=/tmp/xrextest go test ./internal/provider/... -> 11 FAIL. Normal runs are green; clearing the vars in those tests would keep the suite hermetic.
- nit/unverified: a path-shaped pin makes the Rex session label gc:<absolute path>; docs/REX-CONTRACT.md:70 caps label setters at 128 chars, and the cap for session.create is unverified. A very long city path could exceed it.

Verdict: pass.

Finalization verification (this step): go test -count=1 ./... passes; golangci-lint run ./... reports 0 issues; named precedence tests (TestStateDirExecStateDirOverride, TestStateDirExecFallsBackToRex, TestStateDirExecRelativeRejected, TestCityFromEnvPrefersExecCityPath, TestCityCandidatesExecCityPathIsPin, TestNamespaceForCallerExecCityPathIsPin) all PASS. Review verdict pass on the uncommitted diff.
<!-- SECTION:NOTES:END -->

## Comments

<!-- COMMENTS:BEGIN -->
author: @mc-wisp-ptfbu7
created: 2026-11-18T23:18:00Z

---

Claimed by Gas City worker mc-wisp-ptfbu7 (seat rex-gascity/pi-1, session pi-mc-wisp-ptfbu7, template rex-gascity/pi); workflow bead mc-x20p, step bead mc-4glq.

---
<!-- COMMENTS:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
Change: one env-resolution path for gc's per-op contract. CityFromEnv, CityCandidates and NamespaceForCaller now share cityPin() (GC_EXEC_CITY_PATH, then GC_REX_CITY fallback); stateBase() reads GC_EXEC_STATE_DIR (must be absolute, else state-dir error -> exit 1) then GC_REX_STATE_DIR then XDG/HOME. Why: the shared RPP dispatch contract passes GC_EXEC_CITY_PATH/GC_EXEC_STATE_DIR, but the dispatcher only read the GC_REX_* fallbacks, so a gc caller silently resolved to $XDG_STATE_HOME/rex-gascity (audit: is-running, remove-meta). Verification: go test -count=1 ./... pass; golangci-lint run ./... 0 issues; six new precedence tests pass; independent review verdict pass. Risks/follow-ups (minor, from review): README.md and docs/rex-provider.md still document only GC_REX_*; docs/provider-herdr-parity.md is-running 'unpinned' wording; docs/provider-method-audit/{is-running,remove-meta}.md and docs/provider-herdr-method-deviations.md still list this deviation as open; some pre-existing env-dependent tests are not hermetic under GC_EXEC_*; path-shaped pin yields a long Rex session label (unverified 128-char cap).
<!-- SECTION:FINAL_SUMMARY:END -->
