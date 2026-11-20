---
id: GCREX-65
title: >-
  Retain the city Rex session when its last pane closes (shared server teardown)
status: In Progress
assignee: []
created_date: "2026-11-19T07:51:00Z"
updated_date: '2026-11-19T20:53:00Z'
labels: []
milestone: m-0
dependencies: []
references:
  - internal/provider/provider.go
  - docs/REX-CONTRACT.md
  - docs/rex-pilot-phase5.md
  - internal/provider/e2e_test.go
priority: medium
type: bug
ordinal: 121000
---
## Description

<!-- SECTION:DESCRIPTION:BEGIN -->

<!-- coderabbit-ai-generated:v1 -->

<!-- coderabbit-ai-generated-section:v1:cb1bec0ff9e8c56333584ce51a7231727c235c4e3d67cff94a85b32419069c9d -->

Closing the last pane removes its tab but leaves the empty canonical `gc:<city>` session listed. The live scratch-session probe and its before/after commands and outputs are recorded in `docs/REX-CONTRACT.md` §12, “Empty city session”.

**Final decision:** retain the canonical city session, even when empty. The probe found a server-wide Rex app client listed on the unattended scratch session, so the approved skip branch applies. `Stop` closes the resolved agent block and does not call `session.destroy` or `server.stop`; a later `Start` reuses the session when present (`docs/REX-CONTRACT.md` §11 D3 and §12; `docs/provider-herdr-parity.md`, server-stop cross-references). The provider creates the canonical session on a `FindSession` miss (`internal/provider/provider.go`, `CreateSession`); it also retains adopted legacy-labelled sessions.

**Superseded proposal:** this task originally proposed destroying an empty owned session after guarded checks, changing D3 and the empty-tab verdict, and replacing the session-survival assertion in `TestE2EAgentExitLiveness`. That proposal and the proposed change to the assertion are superseded by the live-probe decision. The survival assertion remains valid: the test checks self-exit and then checks that `Stop` leaves the original session in place. `TestStopCallsNoDestroy` covers both pane-present and no-pane stop paths (`internal/provider/teardown_spy_test.go`).

The Phase 5 pilot separately observed `gc stop` removing a session; its cause was not visible in the supervisor log (`docs/rex-pilot-phase5.md`, “Errors observed and dispositions”, item 2). GCREX-30 records the outcome for any future city-owned server design.

<!-- /coderabbit-ai-generated:v1 -->
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria

<!-- AC:BEGIN -->

<!-- coderabbit-ai-generated:v1 -->

<!-- coderabbit-ai-generated-section:v1:0fa9730227ba4e09f4d638cae87bc4727d60ec019dab4cdaeafa978e25e53f53 -->

- [ ] #1 The live empty-session probe is recorded with the before/after `rex session ls` and `rex session inspect` outputs (`docs/REX-CONTRACT.md` §12).
- [ ] #2 D3 records that the provider closes its own panes, retains the canonical session even when empty, and does not stop the shared server or destroy other sessions or human panes (`docs/REX-CONTRACT.md` §11 D3 and §12).
- [ ] #3 The decision records that adopted legacy-labelled sessions are also retained; the earlier proposal to choose a destroyable session is superseded.
- [ ] #4 The empty-session decision records that a later `Start` reuses the session when present; the proposed guarded destroy trigger and recreation path are superseded.
- [ ] #5 The scratch probe and `session.destroy` contract are recorded in `docs/REX-CONTRACT.md` §12; the provider does not call it.
- [ ] #6 Provider teardown remains block-only, including when an app client is listed or the pane has already vanished; the proposed destroy guards are superseded.
- [ ] #7 The retained-session behavior is covered by `TestStopCallsNoDestroy` and the session-survival checks in `TestE2EAgentExitLiveness` and `TestE2EIdentityActivityAttachment`; `make test` passes.
- [ ] #8 The GCREX-4 empty-tab verdict and affected docs reflect retention, the existing session-survival assertion remains in place, and GCREX-30 references the outcome.

<!-- /coderabbit-ai-generated:v1 -->
<!-- AC:END -->
