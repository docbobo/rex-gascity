---
id: GCREX-7
title: Extract SessionName and Binding modules (architecture A+B)
status: Done
assignee:
  - '@boris'
created_date: '2026-11-17T17:18:00Z'
updated_date: '2026-11-17T17:27:00Z'
labels: []
dependencies: []
references:
  - docs/plans/native-rex-provider.md
  - docs/REX-CONTRACT.md
priority: high
type: task
ordinal: 7000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
The architecture review found the provider core resolving a gc session name to a Rex session/window/block four times (Start, Stop, IsRunning, Peek), with the planned `sessionname.go` and `binding.go` still absent. Phase 2 (GCREX-3) adds three more ops that will copy the same resolution. This task pulls the two deep modules forward: a `SessionName` type that owns the name grammar and label derivation, and a `Binding` module that owns name-to-address resolution, records bindings on Start, and forgets them on Stop. The file-backed sidecar is deferred until the RPP dispatcher chooses a state directory; the binding module exposes a store seam for it.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 A SessionName type parses `<rig>--<agent>` and the town fallback, exposes rig, agent, and town status, and derives the three Rex labels; table tests cover the grammar.
- [x] #2 The provider uses SessionName instead of the free parseName function and inline session-label concatenation.
- [x] #3 A Binding module resolves a name to session/window/block, records a binding on Start, forgets it on Stop, and falls back to the label scan when no binding is recorded.
- [x] #4 Stop, IsRunning, and Peek resolve through the Binding module and no longer duplicate the session/view scan.
- [x] #5 Tests cover binding cache hit, scan fallback, stale-binding removal, and transport error preserving the binding.
- [x] #6 go test ./..., go vet ./..., and gofmt -l . are clean.
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. B first: add `internal/provider/sessionname.go` with a SessionName type that owns the `<rig>--<agent>` grammar, the town fallback, and the three Rex label derivations; add table tests; update `provider.go` to use it.
2. Verify B in isolation: `gofmt`, `go vet`, `go test ./...`; review the diff before A starts.
3. A: add `internal/provider/binding.go` with a Binding value, a BindingStore seam with an in-memory implementation, and a Binder that resolves, records, and forgets. Resolution order: stored binding, live probe, label-scan fallback; transport errors keep the stored binding.
4. Wire the Provider to a Binder: Start records the binding; Stop/IsRunning/Peek resolve through it; remove the duplicated find helpers.
5. Extend `faketest` only as needed for call counts and error injection in binder tests.
6. Run `gofmt -l .`, `go vet ./...`, and `go test ./...`; review the full diff. Leave changes uncommitted for parent review.
7. Sidecar persistence is out of scope: the store seam is the follow-up.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
B landed: internal/provider/sessionname.go defines SessionName (rig/agent/town, tab/pane labels), sessionname_test.go covers the grammar and labels, and provider.go uses the type. parseName and the inline session label are gone. gofmt, go vet, go test are clean. A follows sequentially because it keys bindings on SessionName.

B landed first (sessionname.go + table tests), then A on top of it (binding.go, provider wiring, fake topology and call counters). Verified: gofmt -l . clean, go vet ./... clean, go test -count=1 ./... passes. AC #4 evidence: rg finds no parseName, findAgentBlock, or sessionLabel( in internal/provider; provider calls binder.FindSession/Resolve/Record/Forget. Committed as 6b17d38. The file-backed sidecar is deferred; the BindingStore seam is the follow-up.
<!-- SECTION:NOTES:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
Extracted the two deep modules the architecture review flagged. internal/provider/sessionname.go now owns the gc name grammar: SessionName parses <rig>--<agent> with the __town fallback, exposes rig/agent/town, and derives the tab and pane labels; a package-level rexSessionLabel(city) derives the session label. internal/provider/binding.go adds Binding, a BindingStore seam with an in-memory MemoryStore, and a Binder that resolves a name (stored binding, live probe via SessionView, then label-scan fallback), records on Start, and forgets on Stop. Stop, IsRunning and Peek resolve through the binder; parseName, sessionLabel, findSession and findAgentBlock are gone. The fake now models topology and counts ListSessions/SessionView calls, so Start -> Stop is testable without a live server. Verification: gofmt -l . clean, go vet ./... clean, go test -count=1 ./... passes. Committed as 6b17d38. The file-backed sidecar is deferred; the store seam is the follow-up.
<!-- SECTION:FINAL_SUMMARY:END -->
