---
id: GCREX-38
title: >-
  Second-pass deepening of internal/provider interfaces (architecture review
  candidates 1-5)
status: Done
assignee:
  - '@mc-wisp-kwkyxm'
created_date: '2026-11-18T20:48:00Z'
updated_date: '2026-11-18T21:47:00Z'
labels:
  - architecture
  - refactor
  - for-gascity
dependencies: []
references:
  - docs/provider-herdr-parity.md
priority: medium
type: chore
ordinal: 94000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
The first pass of architecture-review candidates 1-5 (commits `5f2fb3e..6e74ac3`) landed as behavior-preserving refactors that kept every exported Go interface, option constructor and behavior. A two-axis review found several of the spec's headline outcomes did not land, because the pass traded interface shrinkage for safety:

- Candidate 2: the three store interfaces (`BindingStore`, `ActivityStore`, `MetaStore`) still exist; only the fan-out was centralized into one generic helper. The spec asked for one seam.
- Candidate 3: `WithStateDir`, `WithStateDirs`, `WithSessionLabels` and `WithCityNamespaces` all remain, and `WithNamespace` is a fifth; the spec asked to delete the overlapping options.
- Candidate 5: `internal/launch` exposes five functions rather than one entry point, and `blockLayout` still lives in `internal/rex/cli`.
- Candidate 4: the six nudge/startup tunables moved into `nudgeConfig` but are embedded back into `Provider`, so they are still reachable.
- Candidate 1: resolution now has three `Binder` surfaces (`Resolve`, `Cached`, `Live`) rather than one entry point.

This task is the deeper pass. It may break the provider package's exported Go API (it is `internal/` and only used in this module), but the RPP v0 op surface and every observable behavior must stay unchanged. Per AGENTS.md, verify any nudge/launch parity claim against `docs/provider-herdr-parity.md` and the pinned gc v1.5.1 source before changing it.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 The three sidecar store interfaces collapse to one seam, with the memory and file adapters and a single composite fan-out retained, and the on-disk JSON layout unchanged.
- [x] #2 The overlapping namespace options are removed and their callers and tests migrate to the single namespace constructor; WithCityNamespaces no longer swallows a state-dir error silently.
- [x] #3 internal/launch exposes one entry point and owns the Rex block layout; internal/rex/cli no longer translates a launch.
- [x] #4 The nudge and startup tunables are no longer reachable on Provider through an embedded config.
- [x] #5 make test passes, and every behavior change lands with the named test that proves it.
- [x] #6 The RPP v0 op surface (the CLI ops and their outputs) and the provider observable behavior are unchanged.
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. Collapse the three sidecar store interfaces into one generic store seam with memory/file adapters and one composite fan-out, preserving on-disk JSON. 2. Replace the overlapping namespace options with one namespace constructor and surface state-dir errors at the dispatcher. 3. Collapse Binder resolution to one entry point with cached/live/rediscover modes. 4. Make internal/launch own the one launch plan and Rex block layout; internal/rex/cli only serializes it. 5. Stop embedding nudgeConfig in Provider. 6. Migrate callers and tests, run go test ./... and golangci-lint run ./..., and re-check the RPP v0 surface against the parity doc.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Implemented the deeper pass. Candidate 2: the three sidecar store interfaces are gone; a single generic Store[K,V] seam (Read/Write/Drop) now backs bindings, activity and metadata, with one mem adapter, one file adapter (encode/decode + presence predicate) and one composite fan-out; on-disk JSON is byte-identical (TestFileStoreOnDiskBytes). Candidate 3: WithStateDir/WithStateDirs/WithSessionLabels/WithCityNamespaces are removed; WithNamespace is the only namespace option, NewNamespace reports state-dir errors, and the dispatcher surfaces them (TestGetMetaUnreadableStoreFails); NewNamespaceForDirs covers already-resolved dirs. Candidate 4: Provider holds nudgeConfig in a named field, so the tunables are no longer promoted. Candidate 5: internal/launch.NewPlan is the one launch translation and owns the Rex block layout; internal/rex/cli only serializes spec.Layout (TestNewPlanOwnsBlockLayout, cli tests). Candidate 1: Binder has one Resolve(ctx, ResolveRequest) entry point with ResolveCached/ResolveLive/ResolveFresh modes; Cached/Live/ResolveHint are gone. Verification: go test ./... and go vet ./... pass; golangci-lint run ./... reports 0 issues. No RPP v0 op or observable behavior changed; the CLI dispatch tests pass unchanged.

Working tree note for review/finish: the untracked backlog/tasks/gcrex-* and docs/provider-herdr-method-io.md, docs/provider-method-audit/ files predate this run; this change touches only cmd/gc-runtime-rex/signal_test.go and internal/{launch,provider,rex}. Leave the pre-existing untracked files out of the finish commit unless they are explicitly in scope.

Independent review (openrouter/deepseek/deepseek-v4.1-flash:high) of the uncommitted GCREX-38 deeper pass.

Scope reviewed: tracked edits (cmd/gc-runtime-rex/signal_test.go, internal/{launch,provider,rex}) plus the untracked backlog task markdown. Ignored .pi/, .pi-agent/, .gc/ and the pre-existing untracked gcrex-28*/32/36/37*/docs files.

Verification run:
- `go test -count=1 ./...` -> all packages ok.
- `golangci-lint run ./...` -> 0 issues.
- On-disk JSON layout: `TestFileStoreOnDiskBytes` unchanged and passing; enc/dec pairs match the old byte shapes.
- Launch wire shape: `launch.NewPlan` builds `rex.Layout{Block}` with the same flavor/options/label tags the removed `cli.blockLayout` emitted; for every real caller (provider builds spec via NewPlan) the JSON is identical. `TestNewPlanOwnsBlockLayout`, `TestProjectPoolRoutingLaunchcommand`, `TestLaunchCommandEmptyEnvIsUnset`.
- Env projection/command unchanged: the projection runs once in NewPlan and feeds the hint, seed, hooks and command exactly as before.
- Candidate 1: single `Binder.Resolve(ctx, ResolveRequest{Name,Hint,Mode})`; Cached/Live/ResolveHint gone; modes map 1:1 to the old surfaces (metaName/Binding use the zero (cached) mode).
- Candidate 2: single `Store[K,V]` seam; `memStore`, `fileStore`, generic `compositeStore`; the three per-kind interfaces and the per-kind composites are gone.
- Candidate 3: `WithStateDir(s)`, `WithSessionLabels`, `WithCityNamespaces` removed; `WithNamespace` is the one option and `NewNamespace` reports state-dir errors.
- Candidate 4: `nudgeConfig` is a named (non-embedded) `nudge` field; no tunable is promoted on Provider.
- Candidate 5: `internal/rex/cli` no longer builds a layout; it serializes `spec.Layout`.
- No RPP v0 op, CLI arg/exit/stdio behavior, or provider observable behavior changed.

Findings by severity

Blocking: none.

Major: none.

Minor / nit (follow-ups, none block the commit):
1. Dead transport fields. `rex.LaunchSpec.Shell`, `.Login`, `.Interactive` (internal/rex/client.go:45-50) are no longer serialized: `internal/rex/cli` sends `spec.Layout` verbatim (cli.go:143,165) and `launch.NewPlan` never sets them. They stay exported with doc comments implying support, so a future caller would silently lose them (previously they reached the layout). No current caller sets them, so no observable change today. Delete them or document that Layout supersedes them.
2. Metadata seam is weaker than the others. `WithMetaStore` takes `Store[metaKey,string]` with the unexported metaKey, and `NewMemoryMetaStore`/`NewFileMetaStore` return unexported concrete types, unlike `WithBindingStore`/`NewMemoryStore`. External callers cannot implement the metadata seam, and `Clear` became an optional runtime `clearer` assertion: `Provider.metaClear` silently no-ops for a store without it, where the old `MetaStore` interface required Clear. In-tree adapters all implement Clear, so no regression today, but the retire path lost its compile-time guarantee.
3. Acceptance #3 wording: "internal/launch exposes one entry point". It still exports two functions, `NewPlan` and `CommandArgv` (launch.go). `CommandArgv` is the start-config command parser the dispatcher calls, so this is defensible, but the literal criterion is not met.
4. Doc drift: parity-record file:line citations into the refactored files are now stale (docs/provider-herdr-parity.md cites stores.go:238, binding.go:435, provider.go:778, provider.go:596-628, provider.go:418, etc.). Behavior is unchanged; the citations should be refreshed in a follow-up.
5. Nit: accidental test rename `TestProjectPoolRoutingLaunchCommand` -> `TestProjectPoolRoutingLaunchcommand` (internal/launch/routing_test.go).
6. Nit: `launch.NewPlan` calls `command(argv, env)` twice (once for Spec.Command, once inside blockLayout); deriving both from one call avoids drift (launch.go:44-49,251-255).
7. Nit: stale comment in provider.Config: "(see launch.Command)" refers to a function that no longer exists (provider.go:44).

Verdict: pass (only minor/nit issues remain).

Finalization verification (2026-11-18): make test passes; the named behavior tests TestFileStoreOnDiskBytes, TestGetMetaUnreadableStoreFails and TestNewPlanOwnsBlockLayout pass with -count=1. Independent review reported pass with only minor/nit follow-ups (dead rex.LaunchSpec.Shell/Login/Interactive transport fields, metadata-seam exposure and Clear compile-time guarantee, stale docs/provider-herdr-parity.md line citations, one test-name casing nit). No RPP v0 op or observable provider behavior changed.
<!-- SECTION:NOTES:END -->

## Comments

<!-- COMMENTS:BEGIN -->
author: @mc-wisp-kwkyxm
created: 2026-11-18T21:25:00Z

---

Claimed by Gas City worker mc-wisp-kwkyxm (seat rex-gascity/pi-1, session pi-mc-wisp-kwkyxm, template rex-gascity/pi); workflow bead unknown, step bead unknown.

---
<!-- COMMENTS:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
What changed: deeper architecture-review pass over internal/provider, internal/launch and internal/rex/cli. Collapsed BindingStore/ActivityStore/MetaStore into one generic Store[K,V] seam with memory/file adapters and one compositeStore fan-out (on-disk JSON byte-identical). Removed WithStateDir(s)/WithSessionLabels/WithCityNamespaces; WithNamespace is the sole namespace option and NewNamespace reports state-dir errors. internal/launch.NewPlan is the single launch translation and owns the Rex block layout; internal/rex/cli only serializes spec.Layout. Binder resolves through one Resolve(ctx, ResolveRequest) with cached/live/fresh modes, and nudgeConfig is a named non-embedded field. Why: the first pass preserved exported surfaces instead of shrinking them; this pass removes the redundant seams while keeping RPP v0 and observable behavior fixed. Verification: make test passes; TestFileStoreOnDiskBytes, TestGetMetaUnreadableStoreFails and TestNewPlanOwnsBlockLayout pass with -count=1; independent review passed with only minor/nit follow-ups. Risks/follow-ups: dead rex.LaunchSpec.Shell/Login/Interactive fields, metadata-seam exposure/Clear compile-time guarantee, stale parity-doc citations, and a test-name casing nit remain as non-blocking cleanups.
<!-- SECTION:FINAL_SUMMARY:END -->
