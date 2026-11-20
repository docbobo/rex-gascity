---
id: GCREX-65
title: >-
  Destroy the city Rex session we created when its last pane closes (ownership
  rule for a shared server)
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

Closing the last pane leaves the city's Rex session alive in `rex session ls`. Observed: after the last agent pane in the city's session closes, `rex session ls` still lists `gc:<city>` with zero windows (a live `rex session inspect gc:my-city` on this host shows the same class of shell). The panes and the tab go away; the session stays.

The ownership rule for a shared Rex server:

- **Never destroy what we did not create.** The Rex server is shared and not ours: `server.stop` stays uncalled (D3, docs/REX-CONTRACT.md section 11), and neither is any other city's session, window, block, or a human's pane.
- **We may destroy what we did create.** The city session `gc:<city>` is ours: the provider mints it itself with `p.client.CreateSession(ctx, p.binder.canonicalLabel())` (`internal/provider/provider.go:474`, inside the `FindSession`-miss branch at :469). The panes and tabs in it are ours too.

Under that rule the empty session is **our own leftover, not an invariant to accept**: we created it, its last block was closed by us, and nothing destroys it. That needs fixing, not documenting.

Today's behaviour:

- `Provider.stop` closes only the resolved agent block, `p.client.CloseBlock(ctx, binding.SessionID, binding.BlockID)` (`internal/provider/provider.go:694`), then `clearIncarnations` + `binder.Forget`; a vanished pane is tolerated (`!errors.Is(err, rex.ErrNotFound)`, `internal/provider/teardown_test.go` `TestStopToleratesVanishedPane`).
- Rex removes the tab when its last block closes; the session shell survives, and the next `Start` reuses it through `binder.FindSession`.
- D3 as written says "the provider never calls it [session.destroy]" (docs/REX-CONTRACT.md section 11 D3 and section 12; docs/rex-provider.md:188 and :441; docs/provider-herdr-parity.md:1054 "GCREX-4 — teardown is block-only (`block.close`); never `session.destroy` or `server.stop`"). That prohibition was written for sessions we did not create and for the shared server; applying it to our own session is what leaves the empty shell. **D3 must be restated as an ownership rule**, not an absolute: never destroy a session we did not create; do destroy the one we did.
- The empty-tab/session-survival probe was resolved in GCREX-4 (docs/REX-CONTRACT.md:514-517) and `TestE2EAgentExitLiveness` asserts the session survives after an agent's command exits (`internal/provider/e2e_test.go:468-541`, assertion at :522). That assertion encodes the old absolute and will need to change with this work.
- The pilot's different observation is a separate case, not a counterexample: during Phase 5 `gc stop` removed the `gc:default` session entirely and the disposition was "**Accepted.** gc owns the lifecycle of its own sessions. Provider teardown stays block-only" (docs/rex-pilot-phase5.md, "Errors observed and dispositions" item 2), also noting "The gc-side path that removes the session is not visible in the supervisor log". A city stop cleaned up; a pane-only teardown (every agent exits, a rig drains, a pool slot retires) does not.

## What this task must settle

1. **Reproduce it live and record the exact sequence.** Close/kill the last pane in a city's session (agent command exits, or `block.close`), then `rex session ls` and `rex session inspect gc:<city>` before and after. Confirm the session ends up with zero windows and stays listed.
2. **Define the ownership predicate precisely.** The provider only ever touches sessions whose label is in its accepted set (`Binder.FindSession` scans `sessionLabels`, canonical first, `internal/provider/binding.go:176-192`); it creates only under `canonicalLabel()`. Decide whether the destroy targets (a) the session id the provider created and holds in its own bindings, (b) any session carrying the canonical label, or (c) any session in the accepted label set, including a legacy adopted label. State which, and why a legacy labelled session is or is not ours.
3. **Decide the trigger.** Empty wins over "last close": "the last city-owned block closed" is necessary but not sufficient. Only destroy when the session has **no active content** — after our close, re-read `session.view` and proceed only when it reports zero windows and zero blocks; any window, any block, ours or not, live program or not, means leave the session alone. Also do not destroy on the mere absence of panes while a `Start` is in flight or a rig is about to relaunch. `session.destroy` closes every window and block it finds, so the emptiness check must sit as close to the call as possible; state the remaining TOCTOU window (a human opening a pane between the `session.view` read and the destroy) and how it is bounded or accepted. Name what re-creates the session afterwards and confirm that is cheap and intended (`CreateSession` under the canonical label).
4. **Verify the destroy contract before designing around it.** `session.destroy` requires an attached connection or fails `conflict`, rejects an unknown id with `session_not_found`, closes every window and block, and is destructive (docs/REX-CONTRACT.md:68, :464). Section 12 records that a fresh CLI call on a scratch session returned exit 0, so the CLI may satisfy the attachment itself — confirm this for the provider's own invocation and record the exit codes, rather than assuming.
5. **Guard the irreversible cases, since destroy is not reversible:**
   - **any active content at all** — a window or a block, ours or not, with a live program or not → do not destroy; this is the primary guard and it must be checked immediately before the destroy, not inferred from the last close;
   - a human attached (is-attached / `session.list_clients`, docs/REX-CONTRACT.md section 12) → destroying hangs up their view;
   - a concurrent `Start` between the last close and the destroy (the state lock already serialises session creation, `provider.go:466-476`; make the destroy use it);
   - an unreachable server → treat an absent session as success (GCREX-51);
   - idempotency and retry: an already-gone session must be success, exactly as `CloseBlock`+`ErrNotFound` is today.
6. **Reconcile D3 and the affected docs and tests.** Restate D3 as ownership (docs/REX-CONTRACT.md section 11 and section 12; docs/rex-provider.md:188, :441; docs/provider-herdr-parity.md:1054), update the GCREX-4 empty-tab verdict to say the session is destroyed when it is ours and empty, and update `TestE2EAgentExitLiveness`'s "the city session must survive" assertion (`internal/provider/e2e_test.go:522`) to the new rule.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria

<!-- AC:BEGIN -->

- [ ] #1 The empty-session behaviour is reproduced live and recorded with commands and both rex session ls / rex session inspect outputs, before and after the last pane closes
- [ ] #2 The ownership rule is stated in the repo as a rule, not an absolute: never destroy what we did not create (the shared server, other sessions, a human pane); destroy what we did (the canonical gc:<city> session, our panes), citing where the provider creates the session
- [ ] #3 The ownership predicate is decided and documented: which session (created id / canonical label / accepted legacy label) is destroyable, and why
- [ ] #4 The trigger is emptiness, not the close: after the last city-owned block closes, the destroy runs only when session.view reports zero windows and zero blocks, checked immediately before the call; any active content leaves the session alone; the TOCTOU window between the view read and the destroy is bounded or explicitly accepted; the next Start re-creates the session
- [ ] #5 session.destroy attached-connection requirement, conflict/session_not_found behaviour, and CLI exit codes are verified against the pinned contract before any use, not assumed
- [ ] #6 The guard conditions are covered in order: any active content at all, a human attached, a concurrent Start, an unreachable server, and idempotent retry on an already-gone session
- [ ] #7 Implemented with a named proving test that fails without the change and make test passing
- [ ] #8 D3 is restated as an ownership rule in docs/REX-CONTRACT.md (sections 11 and 12) and corrected in docs/rex-provider.md:188,:441 and docs/provider-herdr-parity.md:1054; the GCREX-4 empty-tab verdict and TestE2EAgentExitLiveness session-survival assertion are updated; GCREX-30 references the outcome
<!-- AC:END -->
