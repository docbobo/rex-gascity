---
id: GCREX-56
title: 'start: honour or explicitly reject the documented start config fields'
status: Done
assignee:
  - '@mc-wisp-qnenkn'
created_date: '2026-11-18T22:06:00Z'
updated_date: '2026-11-19T08:36:00Z'
labels:
  - provider
  - herdr
  - tdd
  - for-gascity
milestone: m-0
dependencies: []
references:
  - docs/provider-method-audit/start.md
documentation:
  - docs/provider-herdr-method-io.md
priority: medium
type: task
ordinal: 112000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Develop test-first (TDD): write the failing test that reproduces this first, watch it go red, then the smallest change that makes it green (repo rule in AGENTS.md; see also the tdd skill).

Avoids: config the caller sends is silently dropped.

The document (## start § Inputs) lists 17 startConfig fields. session_setup, session_setup_script, session_live, overlay_dir, copy_files, fresh_only, ready_prompt_prefix and ready_delay_ms never run, never stage, never warn and never influence readiness. This is one decision per field over one config struct.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 Each of the eight unhandled fields is implemented or documented as unsupported with a reason.
- [x] #2 session_setup runs, overlay_dir and copy_files stage, session_live warns, fresh_only exits 73, and readiness uses ready_prompt_prefix and ready_delay_ms for the fields this task accepts.
- [x] #3 Every field the task accepts has a test.
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. Parse the eight unhandled startConfig fields (session_setup, session_setup_script, session_live, overlay_dir, copy_files, fresh_only, ready_prompt_prefix, ready_delay_ms) and carry them into provider.Config.
2. Stage overlay_dir and copy_files into work_dir before pre_start; warn (do not abort) on a copy_files destination that leaves work_dir.
3. fresh_only: a held name is refused with exit 73 instead of recycled.
4. session_setup and session_setup_script run after launch with warnings only; session_live emits SESSION_LIVE_SKIPPED once.
5. Readiness waits honour ready_delay_ms (a minimum delay, capped 60s) and ready_prompt_prefix (a screen check) when set.
6. Add a named test per accepted field; update the parity record and the start audit.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Implemented all eight fields test-first (red proven by disabling each path and watching the named test fail).

- Parsing: startConfig (cmd/gc-runtime-rex/main.go:74-91) now reads session_setup, session_setup_script, session_live, overlay_dir, copy_files ([]provider.CopyFile), fresh_only, ready_prompt_prefix and ready_delay_ms, and carries them into provider.Config. process_names and work_dir_exclude remain ignored and are documented.
- Staging: overlay_dir (universal subtree minus per-provider/.gc, plus per-provider/<GC_PROVIDER>) and copy_files stage into work_dir before pre_start; a destination that leaves work_dir is a warning, a missing source is skipped. pack_overlay_dirs staging stays after pre_start by design. internal/provider/overlay.go:59-159.
- session_setup/session_setup_script run after launch, warnings only; a non-empty session_live warns SESSION_LIVE_SKIPPED once. internal/provider/provider.go:1006-1035.
- fresh_only refuses a held name (live or dead) with ErrFreshCollision; the dispatcher maps it to exit 73 session "N" already exists (fresh_only). provider.go:346-358, main.go:223-225,292-294.
- Readiness: ready_delay_ms floors the startup wait (capped 60s) and ready_prompt_prefix makes the pane's rendered prompt the readiness signal; the readiness window/settle bound widen to cover the delay. nudge_delivery.go:294-392.

Tests: internal/provider/startconfig_test.go (TestStartStagesOverlayDirAndCopyFiles, TestStartCopyFilesRefusesEscapingDestination, TestStartSessionSetupRunsAsWarningsOnly, TestStartWarnsSessionLiveSkipped, TestStartNewFreshOnlyRefusesAHeldName, TestStartReadinessUsesReadyDelayMs, TestStartReadinessUsesReadyPromptPrefix) and cmd/gc-runtime-rex/startconfig_test.go (TestStartFreshOnlyExits73, TestStartRunsSessionSetupAndWarnsSessionLive, TestStartStagesOverlayDirThroughDispatcher). Red->green verified through the real dispatcher and provider.

Verification: go test ./... all packages ok; go vet ./... ok; golangci-lint run ./... 0 issues; gofmt -l cmd internal empty. Docs updated: docs/provider-herdr-parity.md (rows 2-5, 15, 16 and the start prose), docs/provider-method-audit/start.md, docs/rex-provider.md, docs/REX-CONTRACT.md, docs/herdr-extensions.md. Process names and startup-dialog withholding remain the documented gaps.

Review round (GCREX-56 finish; two read-only subagent reviews, Standards axis and Spec axis, against gc v1.5.1 and runtime-herdr-core 0f07496). Every finding was re-checked against the source before it was accepted.

- Staging containment (High, both reviews): staging was lexical only, so a symlinked destination inside work_dir could be written through. internal/provider/overlay.go now stages through an os.Root of the work dir plus a per-component Lstat walk (stageDest, newStageDest, rejectSymlink, copyTreeInto); nothing can land outside work_dir. Tests: TestStartStagingRefusesDestinationSymlink.
- Self-copy (High, spec): a copy_files source already at its destination is skipped (sameFile, gc v1.5.1 internal/runtime/staging.go:331-341), so staging a work dir onto itself is a no-op. Tests: TestStartCopyFilesSkipsASourceAlreadyAtItsDestination.
- rel_dst merge (Medium, standards): a directory source with no rel_dst merges into work_dir; a file whose rel_dst names an existing directory lands inside it. Test: TestStartCopyFilesOmittedRelDstStagesIntoWorkDir.
- Setup-hook env and bound (Medium, both): session_setup hooks now receive exactly the launched pane’s projected env (startResult.env carries plan.Env, so a pool routing override such as BEADS_ACTOR applies) and each hook is bounded by min(30s, remaining start budget). Test: TestStartSessionSetupSeesProjectedLaunchEnv.
- Readiness (Medium, both): ready_delay_ms is now a bound on the prompt wait rather than a competing signal (gc sends one or the other), the millisecond value is clamped before the Duration conversion, and screenHasPromptPrefix matches gc’s paneShowsReadyPrompt faithfully: bottom-up scan, NBSP normalised, one border skipped, and a numbered menu’s cursor row excluded (gc v1.5.1 internal/runtime/tmux/tmux.go:4199-4231; herdrcore/readiness.py:78-86). New outcome ready_prompt_bound distinguishes a prompt-prefix window timeout. Tests: TestScreenHasPromptPrefixMatchesGcPaneShowsReadyPrompt (13 cases), TestStartReadinessUsesReadyPromptPrefix.
- fresh_only (Medium, spec): the check now goes through Binder.lookup, so a record holding the name only as an alias also collides.
- New phase-order proofs: TestStartStagesBeforePreStart (a fatal pre_start hook cannot run before staging), TestStartSessionSetupOnlyAfterALaunch (a failed launch runs no setup). A non-directory overlay_dir source now fails the start (TestStartOverlayDirNotADirectoryFails), and TestStartReadinessUsesReadyPromptPrefix’s positive half uses program_status working so it can only pass through the prompt path.
- Doc/claim corrections: Config.OverlayDir and ReadyDelayMS comments, main.go’s startConfig comment (process_names omitted, fresh_only pack-only, work_dir_exclude pack-only), docs/provider-herdr-parity.md readiness row, docs/provider-method-audit/start.md (all anchors re-pointed; the undocumented no-stderr-warning for an unconfirmed nudge and the per-hook-not-shared setup budget are now recorded as low deviations), docs/rex-provider.md (“narrowed reading” replaced with the faithful match, os.Root boundary noted).

Final verification: go test ./... all packages ok (cmd/gc-runtime-rex 6.6s, internal/provider 7.2s); go vet ./... clean; golangci-lint run ./... 0 issues; gofmt -l cmd internal empty.
<!-- SECTION:NOTES:END -->

## Comments

<!-- COMMENTS:BEGIN -->
author: @mc-wisp-qnenkn
created: 2026-11-19T06:05:00Z

---

Claimed by Gas City worker mc-wisp-qnenkn (seat rex-gascity/pi-2, session pi-mc-wisp-qnenkn, template rex-gascity/pi); workflow bead mc-eokx, step bead mc-fq0r.

---
<!-- COMMENTS:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
All eight previously dropped start-config fields are now handled, and the two that stay ignored are documented with a reason.

What changed: session_setup/session_setup_script run after the launch with warnings only; overlay_dir and copy_files stage into work_dir before pre_start (an escaping destination is a warning, a non-directory overlay source fails); session_live warns SESSION_LIVE_SKIPPED once; fresh_only refuses a held name with exit 73; ready_prompt_prefix makes the pane prompt the readiness signal and ready_delay_ms bounds that wait (60 s cap). process_names (no Rex process-appeared signal) and work_dir_exclude (not a gc field) remain ignored and are documented.

Why: the caller sent configuration that was silently dropped.

Verification: go test ./... all packages ok; go vet ./... clean; golangci-lint run ./... 0 issues; gofmt -l cmd internal empty. The 15 named provider tests plus 3 dispatcher tests pass, including the new phase-order, self-copy, os.Root containment, alias-collision and menu-cursor cases.

Risks and follow-ups: the startup-readiness replacement is not tied to program_status when a prefix is set, so a status-only agent could wait out the window (bounded). Staging is not part of a transactional rollback, so a file staged before a later phase fails stays behind. Withholding the first turn on a startup dialog is GCREX-59.
<!-- SECTION:FINAL_SUMMARY:END -->
