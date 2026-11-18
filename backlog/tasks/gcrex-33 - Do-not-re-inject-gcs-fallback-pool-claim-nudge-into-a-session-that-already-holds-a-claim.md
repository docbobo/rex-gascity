---
id: GCREX-33
title: >-
  Do not re-inject gc's fallback pool-claim nudge into a session that already
  holds a claim
status: Done
assignee:
  - '@mc-wisp-o1dbew'
created_date: '2026-11-18T07:48:00Z'
updated_date: '2026-11-18T18:27:00Z'
labels:
  - workflow
dependencies: []
references:
  - .pi/extensions/gc-hooks.js
  - >-
    /Users/boris/my-city/.pi-agent/sessions/--Volumes-Sources-rex-gas-city--/2026-11-18T06-45-52-621Z_01a1ed67-9b6d-7132-a53e-aa98c5127999.jsonl
priority: medium
type: bug
ordinal: 58000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
gc sends defaultPoolClaimNudge — the literal 'Run gc hook --claim --drain-ack --json now; if it returns work, execute it immediately.' — after the 90s stalled-claim grace period (upstream cmd/gc/idle_nudge.go:76; docs/reference/config.md:95). The Pi extension piped into gc seats injects every drained nudge on before_agent_start: .pi/extensions/gc-hooks.js:183-188 -> run(['nudge','drain','--inject'], ctx.cwd). On 2026-11-18 seat mc-wisp-gwble received that text at 06:47:41, 06:50:15 and 06:52:39 after it had already claimed GCREX-16 at 06:47:58. Each re-injection sent it back through 'gc hook --claim'; it abandoned GCREX-16 and no-op'ed bead mc-ln0 at 06:53:52. The nudge re-armed because the seat stayed awake while the stalled-claim trigger bead changed (mc-9ub -> mc-29w -> mc-b9q -> mc-4qi; events seq 9008/9084/9179/9213). Evidence: /Users/boris/my-city/.pi-agent/sessions/--Volumes-Sources-rex-gas-city--/2026-11-18T06-45-52-621Z_01a1ed67-9b6d-7132-a53e-aa98c5127999.jsonl.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 gc-hooks.js does not inject a drained pool-claim nudge while the session has an outstanding claim (non-empty 'gc hook current' / GC_BEAD_ID).
- [ ] #2 gc does not re-arm the fallback claim nudge for a seat whose last claim is still live.
- [x] #3 Replaying the 2026-11-18 scenario no longer injects the nudge mid-claim, or the extension explicitly defers it until the claim closes.
- [x] #4 A deferred nudge is not lost: it is delivered once the claim is released or closed.
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. Verify both sources: the managed Pi hook (gc 1.5.1 internal/bootstrap/packs/core/overlay/per-provider/pi/.pi/extensions/gc-hooks.js, v9) injects every drained nudge unconditionally (gc-hooks.js:187), and gc's stalled-claim backstop re-sends defaultPoolClaimNudge after a 90s grace (cmd/gc/idle_nudge.go:76, docs/reference/config.md:95). The rex pack has no overlay, so it cannot influence the hook. 2. Ship a rex-pack per-provider Pi overlay that supersedes the managed hook. gascity's pack overlay mechanism stages overlay/per-provider/pi/... last-writer-wins (internal/runtime/staging.go StageProviderOverlayDir + WithPreserve; cmd/gc/build_desired_state_overlay_preserve_test.go TestPrepareTemplateResolution_LaterOverlayLayerOverridesManagedHook). 3. In the overlay, defer the queued-nudge drain while this session holds a live gc claim (gc hook current --id-only / GC_BEAD_ID non-empty), so the pool-claim nudge is never injected mid-claim and stays queued (not acked/lost). 4. Add a named behavioral Go test that runs the overlay under node with a fake gc: claim live -> drain skipped and no pool-claim text injected; claim absent -> drain runs and text injected. 5. Document the overlay in docs/rex-provider.md. 6. go test ./... and golangci-lint run ./... ; leave uncommitted.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Second occurrence, 2026-11-18 ~08:49-08:52 CET: the GCREX-16 worker pjj6r received the fallback nudge at 08:49:39 and again at 08:52:25. This time the worker refused it ('I already hold an active claim ... Re-running the hook now would be the exact re-prompt loop the task notes warn about'), so the nudge itself is not always fatal now - but it is still injected mid-claim, which is the defect.

Implemented in work step (uncommitted; review/finish follow). Change: (1) new pack/overlay/per-provider/pi/.pi/extensions/gc-hooks.js - the rex pack's per-provider Pi hook override. gascity stages overlay/per-provider/<provider>/ last-writer-wins (internal/runtime/staging.go StageProviderOverlayDir + WithPreserve; cmd/gc/build_desired_state_overlay_preserve_test.go TestPrepareTemplateResolution_LaterOverlayLayerOverridesManagedHook), so this supersedes core's managed .pi/extensions/gc-hooks.js on the workdir path. Its GC_PI_HOOK_VERSION=10 is above gc 1.5.1's managed 9, so the managed-hook preserve predicate also defers to it. (2) internal/provider/pi_hook_overlay_test.go - named behavioral test. (3) docs/rex-provider.md - 'Pi hook overlay (claim guard)' subsection. Behavior: before_agent_start now reads the live claim with 'gc hook current --id-only' (falling back to GC_BEAD_ID) and, while a claim is outstanding, leaves the nudge queue undrained; the fallback pool-claim nudge is never injected mid-claim and nothing is acked/lost, so the next turn after the claim closes delivers what was queued. Verified: 'go test ./...' passes; 'golangci-lint run ./...' reports 0 issues. Red-without-change: running the bundled managed v10 hook under node with a fake gc and FAKE_CLAIM set drains and injects the pool-claim text; the overlay does neither (TestPiHookOverlayDefersNudgesWhileClaimIsLive claim-live subtest fails against the managed hook). NOTE: the repo's .pi/ directory was NOT modified; the override lives under pack/. AC #2 (gc does not re-arm) is addressed at the delivery boundary - the injection is what clobbered the claim; gc-side re-arming of the queue is upstream and GCREX-32's per-seat claim already makes the routed step read in_progress.

Independent review (openrouter/~anthropic/claude-sonnet-latest) of the uncommitted GCREX-33 change (pack/overlay/per-provider/pi/.pi/extensions/gc-hooks.js, internal/provider/pi_hook_overlay_test.go, docs/rex-provider.md).

Verified clean: `go build ./...`, `go vet ./internal/provider`, `go test ./...`, `golangci-lint run ./...` (0 issues); the node red/green of the guard was reproduced (unguarded overlay drains+injects the pool-claim text; shipped overlay does neither).

BLOCKING
B1. The override is not guaranteed to reach the session workdir. The controller staging path that writes the managed Pi hook passes `hooks.PreserveManagedFile` via `runtime.WithPreserve` (v1.5.1 cmd/gc/build_desired_state.go:6584-6591). That predicate decides from the destination bytes alone: `PreserveManagedFile` calls `overlayManagedNeedsUpgrade(provider, rel, nil)` with desired=nil and returns `!needsUpgrade(existing)` (v1.5.1 internal/hooks/preserve.go:29-45); `stageProviderOverlayDir` skips any pre-existing file for which preserve returns true and only marks the pass-write flag when it actually writes (v1.5.1 internal/runtime/staging.go:227-243). An existing current managed hook (GC_PI_HOOK_VERSION=9; managedPiHookVersion=9 at v1.5.1, internal/hooks/hooks.go:36,296-301) is therefore preserved on every tick, and the rex overlay is never written on that path. The overlay's marker 10 does not enter the predicate at all, so "the preserve predicate also defers to it" is false. gc's own tests state this: `TestPrepareTemplateResolution_LaterOverlayLayerOverridesManagedHook` requires "The work directory starts without the managed file, so nothing here pre-exists the staging pass" (v1.5.1 build_desired_state_overlay_preserve_test.go:131-134), and its sibling `TestPrepareTemplateResolution_PreservesCurrentManagedHook:81` asserts a pre-existing current hook survives staging. The reported incident ran in the shared checkout, which already holds the untracked managed v9 hook (/Volumes/Sources/rex-gas-city/.pi/extensions/gc-hooks.js), so the guard can be inert exactly there. Only fresh worktrees (no tracked .pi) and the newer non-preserve `gc hook --worktree` path (edge cmd_hook_worktree.go:153,204; absent at v1.5.1) stage the overlay, so coverage is path-dependent and untested. Required: land the override through a writer that actually writes it, or prove/target the worktree path and add a test that stages the real pack overlay over a workdir that already holds the managed v9 hook.

MAJOR
M1. AC #2 ("gc does not re-arm the fallback claim nudge for a seat whose last claim is still live") is not implemented or tested by this change; only delivery is deferred. The mitigation rests on GCREX-32 (Done, outside this diff) making the routed step read in_progress, which is unverified for this replay. Re-scope AC #2 with cited evidence or implement it.
M2. `currentClaimID` trusts `$GC_BEAD_ID` ahead of the live claim, but gc documents the session claim is read only via `gc hook current --id-only` and that GC_BEAD_ID "exists only in the controller's dispatch condition environment, never in a session shell" (v1.5.1 cmd/gc/cmd_hook_current.go:24-37). If it ever reaches a session shell it names a foreign/stale bead and the guard would suppress every queued nudge indefinitely. AC #1's wording permits it, but it contradicts gc's contract; consult `gc hook current` unconditionally.

MINOR / NITS
- docs/rex-provider.md: "The only behavioral difference is a claim guard" is inaccurate - the diff also drops PATH_PREFIX and switches to `GC_BIN` (overlay ~lines 36-54) and changes the module export shape. The PATH/GC_BIN change matches gc's newer managed hook but is an undocumented delta against the v1.5.1 baseline (GC_BIN is set for sessions via cmd/gc/template_resolve.go:387, so low risk).
- Marker 10 equals gc HEAD's managed 10 (edge internal/hooks/hooks.go:37); it only exceeds v1.5.1's 9. A routine gc update to a v10-managed build silently disables the override. Use a marker well above any managed version, or document the re-bump.
- Overlay comment says the marker "lets the overlay test assert the version", but the test does not; the test also never exercises the `$GC_BEAD_ID` branch and silently skips when node is absent, so `make test` verification of the guard is node-dependent.
- Operational: the shared checkout also carries an untracked gc-generated `.pi/extensions/gc-hooks.js` (v9) and other workers' task edits; the finalize/commit step must not sweep them in. Committing `.pi/extensions/gc-hooks.js` would additionally make `WithSkipTrackedOverlays` stop staging the overlay into worktrees.

Verdict: fail.

Addressed review findings (second pass). B1: the provider now stages each start-config pack_overlay_dirs entry's per-provider/<GC_PROVIDER> subtree into the work dir after pre_start and the work_dir check (internal/provider/overlay.go stagePackOverlays/copyTree; internal/provider/provider.go:418; cmd/gc-runtime-rex/main.go:61,199). This bypasses gc's PreserveManagedFile skip, so the v100 override replaces an existing managed v9 hook even in a shared checkout; TestStagePackOverlaysReplacesManagedHook proves the replacement. M2: currentClaimID now runs gc hook current --id-only unconditionally and no longer trusts GC_BEAD_ID; the provider-side probe uses the binding's ClaimSessionID and GC_BIN. Minors: marker bumped to 100 (above v9 and HEAD v10); docs/rex-provider.md and the overlay comment now state the marker does not enter the preserve predicate; GC_BIN replaces PATH_PREFIX and is documented as matching gc's newer managed hook. Added TestNudgeRefusesPoolClaimWhileClaimIsLive, which proves the provider refuses a pool-claim nudge while a live claim is held and types nothing. M1: AC #2 is gc-side and has no change in this repo; gc v1.5.1 revalidate re-reads the trigger live and clears when the trigger reads claimed (cmd/gc/idle_nudge.go:583-605), but the incident's trigger bead changed while the seat still held a different claim. The in-scope fix is the delivery boundary guard; AC #2 remains an upstream/gc limitation and is recorded here rather than silently claimed. Verified: go test ./... passes; golangci-lint run ./... reports 0 issues. Left uncommitted and In Progress for review.

Independent review (openrouter/~anthropic/claude-sonnet-latest), iteration 2, of the uncommitted GCREX-33 change (pack/overlay/per-provider/pi/.pi/extensions/gc-hooks.js, internal/provider/overlay.go, internal/provider/pi_hook_overlay_test.go, internal/provider/pack_overlay_staging_test.go, internal/provider/{provider,nudge,binding,identity,filestore}.go, cmd/gc-runtime-rex/main.go, docs/rex-provider.md).

VERIFIED CLEAN
- go build ./..., go vet ./internal/provider ./cmd/gc-runtime-rex, go test ./... all pass; golangci-lint run ./... reports 0 issues. Node red/green of the guard reproduced (TestPiHookOverlayDefersNudgesWhileClaimIsLive claim-live subtest fails against an unguarded hook; shipped overlay passes).
- B1 (resolved): the provider stages each pack_overlay_dirs entry's per-provider/<GC_PROVIDER> subtree at start after pre_start (internal/provider/overlay.go stagePackOverlays/copyTree; internal/provider/provider.go:411-421), bypassing gc's PreserveManagedFile skip. gc v1.5.1 does send pack_overlay_dirs to the exec script (internal/runtime/exec/json.go:39,63; docs/reference/exec-session-provider.md:234) and puts GC_PROVIDER into the session env (cmd/gc/session_lifecycle_parallel.go:1269), so the wiring is real. TestStagePackOverlaysReplacesManagedHook proves a managed v9 hook is replaced by the v100 override; gc's predicate (v1.5.1 internal/hooks/preserve.go:29-45) then preserves v100 on later ticks.
- M2 (resolved): the overlay reads the live claim only via `gc hook current --id-only` (no GC_BEAD_ID trust), and the provider probe uses the binding's ClaimSessionID with GC_SESSION_ID set explicitly. Go's os/exec dedups the appended GC_SESSION_ID, so the override is effective (verified empirically). Matches gc's contract at v1.5.1 cmd/gc/cmd_hook_current.go:24-37.
- The new `/.pi` ignore (.gitignore:14) keeps the provider-staged hook from dirtying git status.

MAJOR (open)
M1. AC #2 is not implemented, and the re-scope justification is factually wrong about the pinned source. The task notes say "gc v1.5.1 revalidate re-reads the trigger live and clears when the trigger reads claimed (cmd/gc/idle_nudge.go:583-605)". At v1.5.1 `poolClaimBackstop.revalidate` is idle_nudge.go:441-443 and returns `backstopResolutionOutstanding` unconditionally; lines 583-605 at v1.5.1 are `writeIdleClaimMarker`/`clearIdleClaimMarker`. The described behavior is gc HEAD (edge), where revalidate is indeed at idle_nudge.go:583 and re-reads the trigger. Per AGENTS.md the pinned baseline is v1.5.1; the note cites v1.5.1 but describes HEAD. On either version the backstop never consults the seat's live claim on a different bead: `poolClaimBackstop.resolve` keys on the slot's trigger bead alone (v1.5.1 idle_nudge.go:420-430; HEAD idle_nudge.go:532-542), so when the trigger bead changes while the seat still holds its prior claim, the nudge is re-armed. AC #2 therefore remains unmet in this repo. Required: correct the citation to the version actually being read and either (a) close AC #2 explicitly as an upstream/gc limitation with the v1.5.1 mechanism recorded, or (b) state the provider/extension delivery guard as the accepted interpretation of AC #2 (it does stop the re-armed nudge reaching the seat, but gc still reserves and retries the attempt).

MINOR / NITS
- Three near-identical provider tests cover the same path: TestNudgeRefusesPoolClaimWhileClaimIsLive, TestNudgeRefusesPoolClaimNudgeWhileClaimIsLive, and TestNudgeRefusesPoolClaimWhileClaimLive. Consolidate to one.
- The overlay comment says the marker "lets the overlay test assert the version," but TestPiHookOverlayDefersNudgesWhileClaimIsLive never reads gcPiHookVersion.
- The overlay defers the whole nudge queue (not only the pool-claim nudge) while a claim is live; acceptable, but state it.
- The provider writes the rex v100 override over any existing managed hook with no version comparison, so a future managed hook above v100 would be clobbered on start. Document the re-bump, or gate the write on the marker.

Verdict: fail.

Addressed review findings (third pass). M1: corrected the citation. At v1.5.1 poolClaimBackstop.revalidate is cmd/gc/idle_nudge.go:441-443 and returns backstopResolutionOutstanding unconditionally; the live re-read described in the prior note is gc HEAD (edge) at idle_nudge.go:583-605. v1.5.1 poolClaimBackstop.resolve keys on the slot's trigger bead alone (idle_nudge.go:420-430), so a trigger-bead change while the seat holds a different claim re-arms the nudge. AC #2 is therefore closed as an upstream/gc limitation; the accepted interpretation is the provider/extension delivery guard (it stops the re-armed nudge reaching the seat, though gc still reserves and retries the attempt). Minors: consolidated the three duplicate provider tests into TestNudgeRefusesPoolClaimNudgeWhileClaimIsLive; corrected the overlay marker comment; documented that the guard defers the whole nudge queue; documented the v100 re-bump when a future managed hook exceeds it. Verified: go test ./... passes; golangci-lint run ./... reports 0 issues.

Independent review (openrouter/~anthropic/claude-sonnet-latest), iteration 3, of the uncommitted GCREX-33 change (pack/overlay/per-provider/pi/.pi/extensions/gc-hooks.js, internal/provider/{overlay,provider,nudge,binding,identity,filestore}.go, internal/provider/{pi_hook_overlay,pack_overlay_staging,nudge}_test.go, cmd/gc-runtime-rex/main.go, docs/rex-provider.md, docs/REX-CONTRACT.md, docs/provider-herdr-parity.md).

VERIFIED CLEAN
- go build ./..., go vet ./internal/provider ./cmd/gc-runtime-rex, go test ./..., golangci-lint run ./... all pass (0 lint issues).
- Red/green reproduced by hand: with a fake gc and FAKE_CLAIM set, the bundled managed v9 hook drains and injects the pool-claim text, while the shipped overlay does neither (managed hook ignores GC_BIN and uses PATH; the overlay honors GC_BIN). The named tests pass.
- Wiring verified against v1.5.1: gc sends pack_overlay_dirs (internal/runtime/exec/json.go:38-39,63) and GC_PROVIDER in the start env (cmd/gc/session_lifecycle_parallel.go:1269); cfg.PackOverlayDirs holds <pack>/overlay (internal/config/pack.go:924-929), so the overlay at pack/overlay/per-provider/pi/... is the right source. GC_SESSION_ID is in the start env (internal/session/lifecycle.go:34-40,67), so Binding.ClaimSessionID is populated.
- Preserve path: v1.5.1 PreserveManagedFile -> piHookNeedsUpgrade reads the existing file's GC_PI_HOOK_VERSION (internal/hooks/preserve.go:33-45; hooks.go:296-310); a staged v100 (>= managed 9) is preserved on later ticks. The provider's own staging (provider.go:418, overlay.go) is what replaces an existing v9.
- M1 citation correction from iteration 2 is accurate: at v1.5.1 poolClaimBackstop.revalidate is cmd/gc/idle_nudge.go:441-443 and returns backstopResolutionOutstanding unconditionally; the live re-read is gc HEAD idle_nudge.go:583-605; v1.5.1 resolve (idle_nudge.go:420-430) keys on the trigger bead alone.

MAJOR (open)
M1. The claim-liveness predicate is stale after the seat completes its bead, so the guard can defer the fallback nudge indefinitely and AC #4 is not met. Both layers treat a non-empty current_claim_bead_id as "busy": the overlay's currentClaimID runs `gc hook current --id-only`, and the provider's probeLiveClaim (internal/provider/nudge.go) does the same. But gc does not clear that stamp when the claimed WORK bead closes. CurrentClaimBeadID returns the raw metadata with no status check (v1.5.1 internal/session/store.go:356-362); every clear path is session-close/retire or an undelivered-claim unwind (cmd/gc/session_affinity_metadata.go:37-60; callers cmd/gc/session_beads.go:1158,1307,1361,1422,3824; cmd/gc/cmd_hook_claim.go:1237). gc's own liveness notion differs: sessionOwnsLiveClaim / claimedWorkStillHeldBy treat a closed (or released) work bead as not held (cmd/gc/session_ownership.go:78,123-133). The fallback backstop fires on the trigger bead's open state alone (isUnclaimedTrigger, cmd/gc/idle_nudge.go:528), independent of the stamp. So a pool seat that finished its claim while its routed trigger bead is still open has the re-armed nudge deferred until the session ends or it claims again - exactly the "deferred nudge is lost" failure AC #4 forbids. The overlay comment and docs/rex-provider.md ("the next turn after the claim closes delivers what was queued") are false for the completed-claim case. Required: gate on whether the stamped bead is still held (mirror sessionOwnsLiveClaim) or bound the deferral, or re-scope AC #4 and document the stale window with the v1.5.1 mechanism.

MINOR / NITS
- Both guards share the same stale predicate, so they fail together; fixing only one still leaves the other suppressing.
- docs/rex-provider.md and the overlay comment say the marker "does not enter the preserve predicate". Once the override is staged, the existing file's marker (100 >= managed 9) is exactly what makes piHookNeedsUpgrade return false and gc preserve it (v1.5.1 internal/hooks/hooks.go:296-310). The desired marker is not consulted for the initial replacement, but the wording is misleading.
- Integration: the pack overlay reaches the running city only after the pack is committed and /Users/boris/my-city/packs.lock is re-pinned (currently sha:e46e03f...), and the gc-runtime-rex on PATH (go/bin, 17:37) must be rebuilt. Verify both at finalize and keep the unrelated backlog task edits out of the commit.

Verdict: fail.

Addressed review findings (iteration 3, M1). M1: the claim-liveness predicate was the raw `gc hook current --id-only` stamp, which gc never clears when the claimed work bead closes (gc v1.5.1 internal/session/store.go:356-362 CurrentClaimBeadID), so a seat that finished its bead kept suppressing the re-armed fallback nudge indefinitely (AC #4). Both layers now re-read the stamped bead and mirror gc's claimedWorkStillHeldBy predicate (gc v1.5.1 cmd/gc/session_ownership.go:123-133): closed, released (open + empty assignee), or foreign-owned (metadata gc.session_id names another session) is not held. Go: internal/provider/nudge.go probeLiveClaim now runs `gc bd list --id <claim> --all --json` after the stamp and returns the claim only while held (claimRecordHeld); a bd read error still reports held, mirroring sessionOwnsLiveClaim. Overlay: pack/overlay/per-provider/pi/.pi/extensions/gc-hooks.js claimIsLive does the same and the before_agent_start guard uses it for the whole queue. Red-without-change: forcing claimRecordHeld/claimIsLive to report held fails TestNudgePoolClaimGuardFollowsClaimLiveness/closed_claim_is_delivered and TestPiHookOverlayDefersNudgesWhileClaimIsLive/closed_claim_drains_the_nudge respectively; both pass with the fix. Minors: reworded the marker note in docs/rex-provider.md and the overlay comment (the preserve predicate reads the already-staged destination's marker, not the desired overlay's). Verified: make test passes (go test ./... green), go build ./... and go vet ./internal/provider ./cmd/gc-runtime-rex clean, golangci-lint run ./... reports 0 issues. Left uncommitted and In Progress for review. Integration at finalize: the overlay only reaches the running city after the pack is committed and /Users/boris/my-city/packs.lock is re-pinned, and gc-runtime-rex on PATH must be rebuilt; keep the unrelated backlog task edits out of the commit.

Independent review (openrouter/~anthropic/claude-sonnet-latest), iteration 4, of the uncommitted GCREX-33 change (pack/overlay/per-provider/pi/.pi/extensions/gc-hooks.js, internal/provider/{overlay,nudge,provider,binding,identity,filestore}.go, internal/provider/{nudge,pi_hook_overlay,pack_overlay_staging}_test.go, cmd/gc-runtime-rex/main.go, docs/rex-provider.md, docs/REX-CONTRACT.md, docs/provider-herdr-parity.md).

VERIFIED CLEAN
- go build ./..., go vet ./..., make test (go test -count=1 ./...) and golangci-lint run ./... all pass (0 lint issues).
- The iteration-3 M1 fix is real and matches the pinned source. probeLiveClaim (internal/provider/nudge.go) and the overlay's claimIsLive now re-read the stamped bead and mirror gc v1.5.1 claimedWorkStillHeldBy (cmd/gc/session_ownership.go:123-133): closed, released (open+empty assignee), or foreign-owned (metadata gc.session_id != this session) is not held. gc's stamp is indeed raw and never cleared on bead close (v1.5.1 internal/session/store.go:356-362 CurrentClaimBeadID), and the backstop keys on the trigger bead alone (v1.5.1 cmd/gc/idle_nudge.go:420-430 resolve; :441-443 revalidate returns outstanding unconditionally), so the stale-predicate bug was genuine.
- Probe commands verified against the live v1.5.1 CLI: `gc hook current --id-only` exists and exits 1 with no claim (help contract); `gc bd list --id <id> --all --json` returns a JSON array of records with status/assignee/metadata (default "gc bd: answering from the city store" banner is stderr, so JSON.parse/Unmarshal are safe); a missing id returns []. From the rig checkout it resolves the city store, where the mc-* claim beads live. Every metadata value in the city store is a JSON string, so map[string]string does not spuriously fail.
- The provider's appended GC_SESSION_ID is effective: Go's os/exec dedups Cmd.Env with last-wins (verified empirically with a /bin/sh child).
- Wiring against v1.5.1: RuntimeEnvWithSessionContext sets GC_SESSION_ID and GC_PROVIDER is merged into agentCfg.Env (cmd/gc/session_lifecycle_parallel.go:1261-1269); pack_overlay_dirs is sent to the exec provider (internal/runtime/exec/json.go:39,63) and populated from <pack>/overlay (internal/config/pack.go:924-929), so pack/overlay/per-provider/pi is the right source and cfg.Env["GC_PROVIDER"] ("pi", as in this live session) is the right selector.
- Red/green: TestNudgePoolClaimGuardFollowsClaimLiveness (closed/released/foreign/missing claim must deliver) and TestPiHookOverlayDefersNudgesWhileClaimIsLive (closed/released/no-claim must drain) both go red if the liveness check is forced to report "held", and pass as shipped.

MINOR / NITS
- Fail-open on a probe error. Both layers treat a failed/empty `gc hook current` answer as "no claim" and deliver (internal/provider/nudge.go probeLiveClaim `if err != nil || claim == ""`; overlay currentClaimID -> "" -> claimIsLive false). A transient gc failure therefore re-enables the exact mid-claim injection the guard exists to prevent. gc's own sessionOwnsLiveClaim is deliberately fail-closed ("a transient store failure must never be the reason a working session dies": v1.5.1 cmd/gc/session_ownership.go:74-76). The provider already fails closed on the second leg, but the overlay's claimIsLive treats an empty bd answer as not held while treating garbled JSON as held -- internally inconsistent with its own comment. Recommend deferring (refuse/drain-skip) on any probe failure, or at least matching the provider leg-2 policy.
- AC #2 ("gc does not re-arm the fallback claim nudge for a seat whose last claim is still live") is still unmet in this repo and is correctly recorded as an upstream/gc limitation: v1.5.1 revalidate returns outstanding and resolve keys on the trigger bead alone, so no provider-side change can stop the re-arm. The accepted interpretation is the delivery-boundary guard (it stops the re-armed nudge reaching the seat). Keep AC #2 visibly re-scoped in the task; do not mark it satisfied.
- The overlay defers the whole nudge queue, not only the pool-claim prompt (documented in code and docs/rex-provider.md); acceptable, but it means stop/drain-style queued nudges are also delayed while a claim is live.
- Integration is still pending and cannot be exercised from this checkout: the gc-runtime-rex on PATH (/Users/boris/go/bin, 17:37) has no pack_overlay_dirs string, so this very session's workdir still holds the managed v9 hook and never received the v100 overlay. The finalize step must rebuild gc-runtime-rex, commit the pack overlay, and re-pin /Users/boris/my-city/packs.lock before the guard is live. The freshly built binary does contain the field.
- `.gitignore` adds a repo-wide `/.pi`; fine for this purpose but it hides any future tracked .pi content.

Verdict: pass (only minor/nit issues remain; the deferred items are hardening and finalize-time integration, not correctness gaps under normal operation).

Test evidence: make test (go test -count=1 ./...), golangci-lint run ./... (0 issues). Named proof tests: TestNudgePoolClaimGuardFollowsClaimLiveness, TestPiHookOverlayDefersNudgesWhileClaimIsLive, TestStagePackOverlaysReplacesManagedHook.

Finalization (mc-wys, 2026-11-18). Review-loop reached pass (iteration 4). Verification on the rig checkout:
- `make test` (go test -count=1 ./...) green.
- `golangci-lint run ./...` -> 0 issues.
- Named proof tests, run with -v: TestStagePackOverlaysReplacesManagedHook PASS; TestNudgePoolClaimGuardFollowsClaimLiveness PASS (live refused; closed/released/foreign/missing/no-stamp delivered); TestPiHookOverlayDefersNudgesWhileClaimIsLive PASS (live defers; closed/released/no-claim drain).
AC #1, #3, #4 checked: the guard and liveness predicate are proven by the named tests above.
AC #2 deliberately NOT checked. It is a gc-side re-arm (v1.5.1 poolClaimBackstop.revalidate always returns outstanding, idle_nudge.go:441-443; resolve keys on the trigger bead alone, :420-430) and no rex-side change can stop it. The accepted interpretation is the delivery-boundary guard, which stops the re-armed nudge reaching the seat. Recorded as an upstream/gc limitation rather than satisfied.
Integration at finalize (not exercised from this checkout): rebuild gc-runtime-rex on PATH, commit the pack overlay, and re-pin /Users/boris/my-city/packs.lock before the guard is live.
<!-- SECTION:NOTES:END -->

## Comments

<!-- COMMENTS:BEGIN -->
author: @mc-wisp-o1dbew
created: 2026-11-18T16:40:00Z

---

Claimed by Gas City worker mc-wisp-o1dbew (seat rex-gascity/pi-2, session pi-mc-wisp-o1dbew, template rex-gascity/pi); workflow bead mc-5pn, step bead mc-hhu.

---
<!-- COMMENTS:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
Prevent gc's re-armed fallback pool-claim nudge from reaching a seat that already holds a claim. Two layers: a rex-pack per-provider Pi hook overlay (pack/overlay/per-provider/pi/.pi/extensions/gc-hooks.js, GC_PI_HOOK_VERSION=100) that leaves the nudge queue undrained while the session holds a live claim; and provider-side pack-overlay staging (internal/provider/overlay.go, provider.go:418) plus a Nudge claim probe (internal/provider/nudge.go) so the override lands over an existing managed hook and the nudge is refused mid-claim. Liveness mirrors gc's claimedWorkStillHeldBy (v1.5.1 cmd/gc/session_ownership.go:123-133): a closed/released/foreign-owned stamped claim is not held, so the deferred nudge is not lost (AC #4). Verified: make test green, golangci-lint 0 issues, and named tests TestStagePackOverlaysReplacesManagedHook, TestNudgePoolClaimGuardFollowsClaimLiveness, TestPiHookOverlayDefersNudgesWhileClaimIsLive. AC #2 is an upstream/gc limitation (v1.5.1 re-arm keys on the trigger bead alone) and stays unchecked; the guard only stops the re-armed nudge reaching the seat. Risk: integration requires a rebuilt gc-runtime-rex and a re-pinned packs.lock.
<!-- SECTION:FINAL_SUMMARY:END -->
