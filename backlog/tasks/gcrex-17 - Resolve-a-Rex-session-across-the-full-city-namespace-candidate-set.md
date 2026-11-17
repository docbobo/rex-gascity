---
id: GCREX-17
title: Resolve a Rex session across the full city-namespace candidate set
status: To Do
assignee: []
created_date: '2026-11-17T23:28:00Z'
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
- [ ] #1 A binding stored under any candidate namespace for a city is found by is-running, get-last-activity, peek, list-running, stop, get-meta and set-meta invoked under any other candidate namespace for the same city, or city resolution is made caller-independent.
- [ ] #2 For one city, start and the lookup ops agree on a single store regardless of whether the caller has GC_REX_CITY, basename(GC_CITY_PATH), GC_CITY, or none of them.
- [ ] #3 A queued nudge whose target is running delivers even when the controller and the nudge poller resolve different candidate names from the env (regression test with the fake rex.Client over a shared state dir).
- [ ] #4 Two cities pinned to different GC_REX_CITY values sharing one supervisor never cross-resolve each other sessions.
- [ ] #5 go test ./... and golangci-lint run ./... pass.
<!-- AC:END -->
