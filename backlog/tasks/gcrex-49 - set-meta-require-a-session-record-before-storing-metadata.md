---
id: GCREX-49
title: 'set-meta: require a session record before storing metadata'
status: Done
assignee:
  - '@mc-wisp-mlhp9z'
created_date: '2026-11-18T22:05:00Z'
updated_date: '2026-11-19T02:29:00Z'
labels:
  - provider
  - herdr
  - for-gascity
  - tdd
dependencies: []
references:
  - docs/provider-method-audit/set-meta.md
documentation:
  - docs/provider-herdr-method-io.md
priority: medium
type: task
ordinal: 105000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Develop test-first (TDD): write the failing test that reproduces this first, watch it go red, then the smallest change that makes it green (repo rule in AGENTS.md; see also the tdd skill).

Avoids: metadata written for sessions that do not exist, contradicting the contract.

Provider.SetMeta never contacts Rex and never checks the session, so an unknown, stopped or unstarted session stores a key and exits 0. The document (## set-meta § Inputs, § Failure cases) requires a record with gc.obj and exits 1 otherwise.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 An unknown, stopped or unstarted session exits 1 and stores nothing.
- [x] #2 The precondition matches the document rule that a session record with gc.obj must exist.
- [x] #3 The check is decided against gc real call sites so normal gc flows are not broken, with a test for both sides.
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. Change Provider.SetMeta (internal/provider/meta.go) to require a session record: resolve the name through the cached binder (no Rex contact) and return an error when no stored binding answers to it, so an unknown, stopped or unstarted session exits 1 and stores nothing. GetMeta/RemoveMeta keep herdr-core's asymmetry (remove-meta succeeds for an unknown session).
2. TDD, red first: provider-level tests for both sides (a recorded/started session stores; an unknown name fails and stores nothing; a stopped session fails after Stop), plus a dispatcher-level test that set-meta on an unknown name exits 1 with empty stdout and that a started session round-trips. Adapt existing meta/identity tests that set meta before a session exists to record a binding (simulating a crashed prior incarnation) first.
3. Decide the check against gc v1.5.1 call sites so normal flows are not broken: cmd/gc/cmd_runtime_drain.go:64,122-124,186,209 and cmd/gc/session_wake.go:317-328 act on sessions the reconciler lists, and internal/session/lifecycle.go SyncRuntimeAlias runs only when IsRunning is true; each implies a stored binding.
4. Docs: parity process set-meta Our implementation + residual-mistake row; docs/provider-method-audit/set-meta.md move the unknown-session row from Deviations to Matches.
5. Verify go test -count=1 ./... and golangci-lint run ./...; leave the change uncommitted for the review step.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Implemented (uncommitted, for review). Provider.SetMeta (internal/provider/meta.go:56-62) now requires a session record: metaName (meta.go:14-21) resolves the name through the cached binder and reports whether a stored binding answers to it; with no record, SetMeta returns 'no session <name>' (dispatcher exit 1, empty stdout) and writes nothing. GetMeta/RemoveMeta keep herdr-core's asymmetry: remove-meta still succeeds for an unknown session. TDD red->green: TestSetMetaRequiresSessionRecord and TestSetMetaRejectedAfterStop failed first ('SetMeta without a session record = nil, want error'), and dispatcher TestSetMetaUnknownSessionFailsExitOne failed with 'exit = 0, want 1'; all pass after the change. Tests updated: meta_test.go records a session before set-meta (sessionProvider helper); TestStartClearsStaleIdentityWhenEnvHasEmptyValues and TestStartupNudgeConfirmedClearsMarker record a crashed prior-life binding before preloading stale meta; TestMetaRoundTripAcrossCalls starts the pane first. Decided against gc v1.5.1 call sites: cmd/gc/cmd_runtime_drain.go:64,122-124,186,209 and cmd/gc/session_wake.go:317-328 act on reconciler-listed sessions, and internal/session/lifecycle.go SyncRuntimeAlias runs only when IsRunning (internal/session/manager.go:1705, cmd/gc/session_beads.go:2176); each keeps a binding, so normal gc flows are unaffected. Docs: parity set-meta Our-implementation paragraph + deviations row 1 + residual row, and the audit set-meta precondition moved to Matches. Verification: go test -count=1 ./... all packages ok; gofmt -l cmd internal empty; golangci-lint run ./... 0 issues. Follow-up: broader line-citation drift remains in the parity/audit docs (pre-existing); only the set-meta refs were refreshed.

Independent review (openrouter/deepseek/deepseek-v4.1-flash:high): Blocking: none. Major: none. Minor/nit follow-ups: (1) docs/provider-herdr-parity.md set-meta paragraph keeps pre-existing stale line refs (opSetMeta main.go:333-339 -> actual 422-428; fileMetaStore.Set stores.go:222 -> 318; Stop provider.go:741-771 -> 537-573; clearIncarnations provider.go:778-794 -> 580-596; identity.go:20-56 -> 18-27); the change refreshed only the meta.go refs it shifted, as noted. (2) docs/provider-herdr-method-deviations.md rollup still lists the set-meta unknown-session medium deviation and 5/1/4 counts, and is stale for Done tasks GCREX-43..47 too, so it needs the workstream regeneration pass. (3) Nit: stderr text no session %q quotes with double quotes where herdr-core uses single quotes; exit 1, empty stdout and stores-nothing match, and gc v1.5.1 does not parse the phrase (IsSessionGone matches neither). Verified: provider precondition internal/provider/meta.go:14-21,56-62 with GetMeta/RemoveMeta asymmetry preserved; red proof independently re-run via go test -overlay against HEAD meta.go (TestSetMetaRequiresSessionRecord, TestSetMetaRejectedAfterStop, dispatcher TestSetMetaUnknownSessionFailsExitOne all failed as recorded), green go test -count=1 ./... ok, gofmt -l cmd internal empty, golangci-lint run ./... 0 issues; gc v1.5.1 call sites rechecked (cmd_runtime_drain.go:64,122-124,186,209; session_wake.go:313-328; SyncRuntimeAlias gated on IsRunning manager.go:1705 / state active session_beads.go:2176, second site 2380-2387 only logs). Verdict: pass.

Finalize verification: go test -count=1 ./... all packages ok (cmd/gc-runtime-rex 6.871s, internal/provider 7.263s, internal/rex/cli 6.534s, internal/launch 0.335s); golangci-lint run ./... 0 issues. Review verdict: pass.
<!-- SECTION:NOTES:END -->

## Comments

<!-- COMMENTS:BEGIN -->
author: @mc-wisp-mlhp9z
created: 2026-11-19T02:14:00Z

---

Claimed by Gas City worker mc-wisp-mlhp9z (seat pi-mc-wisp-mlhp9z, session pi-mc-wisp-mlhp9z, template rex-gascity/pi); workflow bead mc-1f19, step bead mc-s6nv.

---
<!-- COMMENTS:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
Provider.SetMeta now requires a stored session record before storing: metaName resolves the name through the cached binder and SetMeta returns 'no session <name>' (dispatcher exit 1, empty stdout, nothing written) for an unknown, stopped or unstarted session, matching herdr-core's set-meta precondition. GetMeta/RemoveMeta keep herdr-core's asymmetry (remove-meta still succeeds for an unknown session). Verification: go test -count=1 ./... all packages ok; golangci-lint run ./... 0 issues. Risk/follow-ups: pre-existing line-citation drift remains in the parity/audit docs (only the set-meta refs were refreshed); docs/provider-herdr-method-deviations.md rollup still needs the workstream regeneration pass.
<!-- SECTION:FINAL_SUMMARY:END -->
