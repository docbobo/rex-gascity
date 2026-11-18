---
id: GCREX-15
title: >-
  Run start-config pre_start hooks in the Rex runtime provider so rig agents
  boot in a git worktree
status: Done
assignee:
  - '@pi'
created_date: '2026-11-17T23:03:00Z'
updated_date: '2026-11-18T00:22:00Z'
labels: []
dependencies:
  - GCREX-5
references:
  - >-
    https://github.com/gastownhall/gascity/tree/main/examples/gastown/packs/gastown
documentation:
  - docs/rex-provider.md
  - docs/REX-CONTRACT.md
modified_files:
  - cmd/gc-runtime-rex/main.go
  - internal/provider/provider.go
priority: high
type: feature
ordinal: 15000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Run the start-config `pre_start` hooks in the Rex runtime provider so rig agents boot in their own git worktree.

**Recommendation (the general guarantee).** The native gc way to make every agent in a rig use a worktree is a per-agent `agent.toml`:

```
work_dir = ".gc/worktrees/{{.Rig}}/{{.AgentBase}}"
pre_start = ["{{.ConfigDir}}/assets/scripts/worktree-setup.sh {{.RigRoot}} {{.WorkDir}} {{.AgentBase}} --sync"]
```

`work_dir` is the session cwd; `pre_start` runs the idempotent `git worktree add` script (shipped in the gastown pack: examples/gastown/packs/gastown/assets/scripts/worktree-setup.sh) before the session is created, so the agent starts INSIDE the worktree. Reference: examples/gastown/packs/gastown/agents/polecat/agent.toml and the gastown pack.

**Why it does not work today.** The Rex dispatcher honors `work_dir` (passed to Rex `NewWindow`/`NewSplit`) but ignores `pre_start` (and the rest of the start hooks: `session_setup`, `session_setup_script`, `session_live`, `copy_files`, `overlay_dir`, `pack_overlay_dirs`). So the worktree is never created and the session cannot start in it. This is the same open GCREX-5 item; documented in docs/rex-provider.md and docs/REX-CONTRACT.md section 12. gc already expects `.gc/worktrees` (gc doctor checks `worktrees`, `worktree-disk-size`, `nested-worktree-prune`).

**Fix direction.** Decode a `pre_start` array (ordered) from the start config in `gc-runtime-rex start`, resolve it against the launch cwd/`work_dir`, and run each command before the blocking `NewWindow`/`NewSplit`. Fail the start (do not launch) if a pre_start command exits non-zero, mirroring the tmux/herdr runtimes. Keep commands environment-equivalent to the agent launch (the start-config `env` map). Then the gastown worktree recipe works verbatim for every rig agent.

**Non-goals.** Provider-independent worktree isolation via formulas (mol-polecat-work / mol-scoped-work / mol-polecat-commit) already works and stays as the per-bead option; this item is about the per-agent `work_dir` + `pre_start` path. The remaining hooks (session_setup, session_live, copy_files, overlays) can follow separately under GCREX-5.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 startConfig decodes the pre_start array and gc-runtime-rex start passes it to the provider with the launch context (work_dir + start-config env)
- [x] #2 Each pre_start command runs, in order, before NewWindow/NewSplit; a non-zero exit fails the start and no pane is left behind
- [x] #3 With a per-agent work_dir + pre_start set to the gastown worktree-setup.sh, a fresh rig agent starts inside .gc/worktrees/<rig>/<agent> on the correct branch
- [x] #4 The already-implemented work_dir is still honored, and a spec with no pre_start behaves exactly as today
- [x] #5 Tests using the fake rex.Client cover: pre_start order, failure aborts launch, no pre_start is a no-op, and pre_start sees the start-config env
- [x] #6 go test ./... and golangci-lint run ./... pass
- [x] #7 Live check on ~/my-city: configure a rig agent with the worktree recipe and confirm it starts in its worktree (gc doctor worktrees + session pwd)
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. Decode ordered pre_start []string in startConfig; add PreStart to provider.Config; pass through opStart. 2. Run each command in order before NewWindow/NewSplit, in work_dir, with the start-config env; non-zero exit fails the start with no pane. 3. Fake-rex + temp-dir tests for order, abort-on-failure, no-op, env visibility. 4. Update docs/rex-provider.md. 5. go test ./... + golangci-lint run ./...; parent merges and runs the live check.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Implemented in worktree gcrex-15-prestart (DeepSeek v4 Pro), merged to main as 8f88781 (cherry-pick of 8b159c5). startConfig decodes ordered pre_start []string -> provider.Config.PreStart. Provider.start runs each entry via /bin/sh -c in order, cwd=WorkDir, env=os.Environ() merged with cfg.Env, strictly before identity seeding/provisional binding/launch; a non-zero exit aborts with 'pre_start[N]: ...' and leaves no pane, seed, or binding. Empty/nil is a no-op. Tests: internal/provider/prestart_test.go (8 tests: order, abort, no-op, empty slice, env passthrough, cwd, abort-with-identity-env, second-command-fails). go test ./... ok; golangci-lint run ./... 0 issues. Live check pending.

Correction: after a GPG re-sign rewrite the merged main commit is 1d60311 (not 8f88781); same tree and content.

Live check found and fixed a real defect: pre_start ran with cwd=work_dir, but the gastown recipe's work_dir is what pre_start creates, so it died with 'chdir ... no such file or directory'. Fixed in 1f0e16a to mirror runtime-herdr-core's lifecycle._setup_cwd: cwd=work_dir when it exists else '/', and a configured work_dir must exist after the hooks. Post-fix live probe: pre_start created a git worktree with 'git worktree add', ran before the pane (prestart-ts < start-ts), saw GC_SESSION_ID from the start-config env, and the pane started with cwd and git toplevel = the worktree; a missing work_dir without a creating hook fails the start with a clear error and leaves no pane. gc doctor worktrees checks pass (105 passed, 7 warnings, 1 pre-existing unrelated failure).
<!-- SECTION:NOTES:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
The start-config pre_start hooks now run in the Rex provider before the pane launches. startConfig decodes an ordered pre_start array and passes it with WorkDir and Env; each entry runs via /bin/sh -c in order before identity seeding and the launch, with cwd set to work_dir when it already exists and '/' otherwise (mirroring runtime-herdr-core's lifecycle._setup_cwd), and the process environment merged with the start-config env. A non-zero exit aborts the start and leaves no pane, seeded identity, or binding; after the hooks a configured work_dir must be an existing directory or the start fails (mirroring herdr). The already-implemented work_dir is honored; an absent pre_start is a no-op. A live check caught that the original implementation ran pre_start with cwd=work_dir, which made the gastown worktree recipe impossible because that recipe's work_dir is created by pre_start; fixed in 1f0e16a. Verified: commits 1d60311 and 1f0e16a; go test ./..., go test -race ./internal/provider, golangci-lint run ./... (0 issues), go vet clean; live probe: pre_start created a git worktree, ran before the pane, saw GC_SESSION_ID, and the pane started inside the worktree (cwd and git toplevel equal the worktree); a missing work_dir fails with a clear error and no pane; gc doctor worktrees checks pass. Risk: the post-hook work_dir requirement is stricter than before (a spec relying on Rex to create work_dir would now fail earlier), matching the herdr reference.
<!-- SECTION:FINAL_SUMMARY:END -->
