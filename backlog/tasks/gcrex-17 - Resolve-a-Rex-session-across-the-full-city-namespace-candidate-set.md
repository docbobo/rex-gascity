---
id: GCREX-17
title: Resolve a Rex session across the full city-namespace candidate set
status: Done
assignee:
  - '@mc-wisp-xeepb'
created_date: '2026-11-17T23:28:00Z'
updated_date: '2026-11-18T10:02:00Z'
labels:
  - provider
  - state-dir
  - rpp
dependencies: []
references:
  - internal/provider/statedir.go
  - internal/provider/binding.go
  - internal/provider/provider.go
  - docs/REX-CONTRACT.md
  - docs/rex-provider.md
documentation:
  - docs/rex-provider.md
  - docs/REX-CONTRACT.md
priority: high
type: bug
ordinal: 17000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
The Rex provider derives its city namespace from the ambient process env (`CityFromEnv` in `internal/provider/statedir.go`): `GC_REX_CITY`, else basename(`GC_CITY_PATH`), else `GC_CITY`, else "default". RPP v0 ops carry no city, and every op runs `gc-runtime-rex` with the caller env unchanged (the exec runtime provider never sets `cmd.Env`), so the namespace depends on WHO invoked the provider.

That is not stable across gc callers on one city:
- The launchd supervisor (`com.gascity.supervisor`) plist sets no `GC_CITY*`/`GC_REX_CITY`, so the controller resolves "default".
- Session and nudge-poller processes carry `GC_CITY_PATH` (injected by `internal/citylayout`), so they resolve the city basename (for example "my-city").

Observed on ~/my-city: `start` wrote the mayor binding under `~/.local/state/rex-gascity/default`, while the nudge poller looked under `.../my-city`, found nothing, so `is-running`/`get-last-activity` returned false and the queued nudge never delivered (`gc nudge status mayor` stayed pending with `last_attempt_at` zero). Live capture confirmed every controller-invoked provider op had no `GC_CITY_PATH`/`GC_CITY`/`GC_REX_CITY`, while the poller process had `GC_CITY_PATH=/Users/boris/my-city`.

This is the documented "no city in RPP v0" gap (docs/REX-CONTRACT.md section 12-13, docs/rex-pilot-phase5.md). Pinning `GC_REX_CITY` at the supervisor is a per-city stopgap, not a fix: the provider must not disagree with itself depending on the caller.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 A binding stored under any candidate namespace for a city is found by is-running, get-last-activity, peek, list-running, stop, get-meta and set-meta invoked under any other candidate namespace for the same city, or city resolution is made caller-independent.
- [x] #2 For one city, start and the lookup ops agree on a single store regardless of whether the caller has GC_REX_CITY, basename(GC_CITY_PATH), GC_CITY, or none of them.
- [x] #3 A queued nudge whose target is running delivers even when the controller and the nudge poller resolve different candidate names from the env (regression test with the fake rex.Client over a shared state dir).
- [x] #4 Two cities pinned to different GC_REX_CITY values sharing one supervisor never cross-resolve each other sessions.
- [x] #5 go test ./... and golangci-lint run ./... pass.
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. Make CityFromEnv caller-independent: GC_REX_CITY when pinned, else "default"; CityCandidates returns the canonical namespace first, then basename(GC_CITY_PATH) and GC_CITY as read-only legacy namespaces.
2. Add StateDirs and SessionLabels helpers so the dispatcher can resolve the canonical state dir and the accepted gc:<city> labels.
3. Add composite binding/activity/meta stores: read every namespace in order, write only the canonical one, delete from every namespace.
4. Add WithStateDirs/WithSessionLabels/WithCityNamespaces; let the binder prefer and create under the canonical label while revalidating any candidate label.
5. Wire the namespace read set through cmd/gc-runtime-rex.
6. Tests: caller independence, cross-namespace lookup for every lookup op, nudge delivery over a shared state dir, pinned-city isolation; then go test ./... and golangci-lint run ./... .
7. Update docs/rex-provider.md and docs/REX-CONTRACT.md city-namespace sections.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Implementation: canonical city namespace is now GC_REX_CITY or "default"; basename(GC_CITY_PATH)/GC_CITY are ordered read-only fallbacks. Added StateDirs/SessionLabels, composite binding/activity/meta stores (read all, write canonical, delete all), multi-label Binder, and WithStateDirs/WithSessionLabels/WithCityNamespaces. The dispatcher wires the candidate set through, so start, is-running, get-last-activity, peek, list-running, stop, get-meta and set-meta share one store across callers.

Verification: named tests in internal/provider/citynamespace_test.go: TestCityFromEnvIsCallerIndependent (AC2), TestLookupSpansCandidateNamespaces (AC1: IsRunning, Peek, ListRunning, GetLastActivity, GetMeta, SetMeta, Stop), TestNudgeDeliversAcrossCandidateNamespaces (AC3), TestPinnedNamespacesDoNotCrossResolve (AC4). go test ./... passes; golangci-lint run ./... reports 0 issues (AC5).
<!-- SECTION:NOTES:END -->

## Comments

<!-- COMMENTS:BEGIN -->
author: @mc-wisp-xeepb
created: 2026-11-18T10:02:00Z

---

Resumed by Gas City worker `mc-wisp-xeepb` (session `pi-mc-wisp-xeepb`); previous seat `mc-wisp-thpy3` was orphaned mid-task. Workflow bead `mc-er4`, step bead `mc-p6l`.

---
<!-- COMMENTS:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
Made the Rex city namespace caller-independent. CityFromEnv now returns GC_REX_CITY when pinned, else "default", so the launchd controller, session processes and the nudge poller no longer disagree. CityCandidates exposes canonical-first read fallbacks for legacy basename(GC_CITY_PATH)/GC_CITY records; composite stores read across them while writes and the state lock stay canonical, and deletes cover every namespace. The Binder accepts every candidate gc:<city> label but creates and records under the canonical one. Pinned cities resolve to exactly their pin, so they never cross-read. Verified with go test ./... and golangci-lint run ./...; docs/rex-provider.md and docs/REX-CONTRACT.md updated. Risk: unpinned cities sharing a supervisor still share gc:default by design; pin GC_REX_CITY to isolate.
<!-- SECTION:FINAL_SUMMARY:END -->
