---
schema: gc.build.implementation-summary.v1
workflow: {id: gcrex-4e9fj, formula: do-work}
methodology: {pack: gascity, name: build-basic}
producer: {formula: do-work, stage: implement, attempt: 1}
status: approved
trace:
  upstream:
    - path: beads/gcrex-sn2i
      hash: bead:gcrex-sn2i
      ids: [REQ-001, REQ-002, REQ-003, REQ-004, REQ-005]
  coverage:
    - {id: REQ-001, status: covered}
    - {id: REQ-002, status: covered}
    - {id: REQ-003, status: covered}
    - {id: REQ-004, status: covered}
    - {id: REQ-005, status: covered}
---

# Implementation Summary: GCREX-68 — remove the Pi hook overlay from the pack

## Summary

The rex pack no longer ships
`pack/overlay/per-provider/pi/.pi/extensions/gc-hooks.js`. The core pack's
managed Pi hook is the only hook; the runtime pack keeps `stagePackOverlays` so
a city-configured `pack_overlay_dirs` override still lands, now pinned by a
synthetic fixture. The behavior that motivated the overlay (GCREX-33: a live
claim seat being re-prompted and abandoning its work) is still prevented by the
provider's `Nudge` refusal, and the residual risk of that refusal is recorded
for GCREX-67.


## Intended Behavior

- The pack carries no provider hook override and no `pack/overlay/` tree.
- `stagePackOverlays` is unchanged in behavior: it copies each
  `pack_overlay_dirs` entry's `per-provider/<GC_PROVIDER>` subtree into the
  work directory after `pre_start`, so a configured override still replaces a
  managed file gc's preserve predicate would keep. The staging test now builds
  its own synthetic overlay instead of reading the pack copy.
- A live-claim seat does not receive gc's re-armed fallback pool-claim nudge:
  `Nudge` refuses it (`ErrNudgeClaimHeld`, exit 1) using the existing
  `probeLiveClaim` liveness predicate.
- Residual risk until GCREX-67: the refusal spends a gc delivery attempt
  (gc v1.5.1 `cmd/gc/nudge_backstop.go` reserves before `sp.Nudge` and does not
  refund it). At the cap (`idleClaimNudgeMaxAttempts = 3`, gc v1.5.1
  `cmd/gc/idle_nudge.go`) v1.5.1 only gives up and logs
  (`poolClaimBackstop.exhausted` is a no-op at the pin), but gc edge drains and
  recycles the slot, so three backstop ticks on one live claim can drain a
  working seat on gc edge.
- Already-staged work directories keep the old override: gc's
  `hooks.PreserveManagedFile` reads the marker already on the destination
  (gc v1.5.1 `internal/hooks/preserve.go`), so a checkout or worktree staged by
  the removed overlay keeps its `GC_PI_HOOK_VERSION = 100` file and its old
  claim guard until an operator deletes `.pi/extensions/gc-hooks.js` by hand.

Coverage:

| ID | Status |
| --- | --- |
| REQ-001 | covered |
| REQ-002 | covered |
| REQ-003 | covered |
| REQ-004 | covered |
| REQ-005 | covered |

REQ-001 = AC1 (overlay removed), REQ-002 = AC2 (`stagePackOverlays` still
stages `pack_overlay_dirs`, test uses a synthetic fixture), REQ-003 = AC3
(overlay test and `docs/rex-provider.md` section removed), REQ-004 = AC4
(residual risk recorded), REQ-005 = AC5 (already-staged work directories
called out).

## Changed Files

- `pack/overlay/per-provider/pi/.pi/extensions/gc-hooks.js` — deleted.
- `internal/provider/pi_hook_overlay_test.go` — deleted
  (`TestPiHookOverlayDefersNudgesWhileClaimIsLive`), it exercised the deleted
  file.
- `internal/provider/pack_overlay_staging_test.go` — `stagePackOverlays` test
  rebuilt on a synthetic overlay (`TestStagePackOverlaysStagesConfiguredSubtree`)
  asserting the `per-provider/pi` subtree is staged and other subtrees are not.
- `internal/provider/overlay.go`, `internal/provider/provider.go` — comments no
  longer name the rex override or its v100 marker; staging behavior unchanged.
- `internal/provider/nudge.go` — `ErrNudgeClaimHeld` doc records the 3/3
  residual risk and points at the provider docs.
- `docs/rex-provider.md` — "Pi hook overlay (claim guard)" replaced by
  "Pool-claim nudge and live claims (GCREX-67 residual risk)": provider-side
  refusal, the spent-attempt consequence, and the manual cleanup for
  already-staged work directories.
- `AGENTS.md` — the overlay rule keeps its wording but no longer cites the
  deleted file as the pinned-marker example.
- `plans/GCREX-68/implementation-summary-gcrex-zerzm.md` — this summary.

## Verification

- First verification command:
  `go vet ./internal/provider/ && go test ./internal/provider/ -run 'Overlay|Start' -count=1`
  — PASS (`ok github.com/docbobo/rex-gascity/internal/provider 2.012s`).
- Lint: `golangci-lint run ./internal/provider/...` — PASS (0 issues).
- Final proof command: `make test` — PASS (`cmd/gc-runtime-rex`,
  `internal/launch`, `internal/provider`, `internal/rex/cli` all ok).
- No test was written red-first for the deletion itself (the change removes
  code); the retained behavior is pinned by
  `TestStagePackOverlaysStagesConfiguredSubtree` and the existing
  `internal/provider/nudge_test.go` claim-hold case.

## Remaining Risks

- Until GCREX-67 lands, a live-claim seat can be drained on gc edge after three
  refused pool-claim nudges (gc v1.5.1 only gives up and logs at the cap).
  Recorded in `docs/rex-provider.md` and on `ErrNudgeClaimHeld`.
- Checkouts and worktrees already staged by the removed overlay keep the v100
  hook and the old claim guard until `.pi/extensions/gc-hooks.js` is deleted by
  hand; gc's preserve predicate will not revert it.
- Overlay approval: the deletion is authorized by task bead `gcrex-sn2i`
  (GCREX-68, owner boris@pruessmann.org) and the commit carries
  `Overlay-Approved-By: GCREX-68 task bead gcrex-sn2i (owner
  boris@pruessmann.org), 2026-11-20`. No separate approval message was
  available in this session.
- The summary itself is committed with the change; no publish/PR step ran in
  this stage.
