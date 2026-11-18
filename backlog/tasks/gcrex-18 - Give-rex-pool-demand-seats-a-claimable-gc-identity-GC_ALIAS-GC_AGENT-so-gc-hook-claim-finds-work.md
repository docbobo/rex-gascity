---
id: GCREX-18
title: >-
  Give rex pool/demand seats a claimable gc identity (GC_ALIAS/GC_AGENT) so gc
  hook --claim finds work
status: To Do
assignee: []
created_date: '2026-11-18T05:47:00Z'
updated_date: '2026-11-18T06:13:00Z'
labels:
  - provider
  - identity
dependencies: []
references:
  - internal/provider/provider.go
  - internal/provider/identity.go
  - internal/provider/binding.go
  - docs/rex-provider.md
  - https://github.com/gastownhall/gascity-packs/tree/main/runtime-herdr-core
  - docs/provider-herdr-parity.md
  - https://github.com/gastownhall/gascity/blob/v1.5.1/cmd/gc/cmd_hook.go
priority: high
type: bug
ordinal: 18000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
A `rex-gascity/pi` pool/demand seat launched by gc cannot claim its routed work: the agent runs `gc hook --claim --drain-ack --json` (the standard startup nudge) and gets `{"action":"drain","reason":"no_work"}`, so it drains without executing anything. Observed live on ~/my-city during the `rex-backlog` order: `session.demand_claim_divergence` with `trigger_status_at_drain: unreadable`, and the workflow beads (`mc-9ub` etc.) stay open.

## Root cause (captured live from a running seat's env)

- `GC_AGENT=mc-wisp-nslx7`  ← the session id, not the agent identity
- `GC_ALIAS=`               ← empty
- `GC_TEMPLATE=rex-gascity/pi`  ← the identity work is routed to

`gc hook --claim` resolves the claiming agent from `GC_ALIAS` (falling back to `GC_AGENT`). With `GC_ALIAS` empty it looks for work routed to the session id, finds none, and returns `no_work`.

## Reproduced deterministically

Same command, only the identity differs:

- `GC_AGENT=<sid> GC_ALIAS=""`             → `{"action":"drain","reason":"no_work"}`
- `GC_AGENT=<sid> GC_ALIAS=rex-gascity/pi` → `{"action":"work","reason":"claimed","bead_id":"mc-9ub"}`

`GC_BEADS_SCOPE_ROOT` (rig vs city store) makes no difference.

## Verification against runtime-herdr-core (the reference runtime provider)

Checked https://github.com/gastownhall/gascity-packs/tree/main/runtime-herdr-core. It does **not** synthesize identity either:

- `rpp/wire.py`: `IDENTITY_KEYS = ("GC_SESSION_ID", "GC_RUNTIME_EPOCH", "GC_INSTANCE_TOKEN", "GT_PROCESS_NAMES")` — `GC_ALIAS`/`GC_AGENT` are not seeded.
- `metastore.seed()` copies only those keys from the start-config env; the spawn env is `cfg.env` passed through (`spawnfile` / `envplan.agent_env`), with empty values dropped.
- The gastown packs document the contract: `GC_AGENT` is canonical and `GC_ALIAS` can legitimately be empty (`gastown/agents/boot/prompt.template.md`, `gastown/formulas/mol-refinery-patrol.toml`).

So provider-side synthesis is a **deviation from the reference runtime**; the durable fix belongs in gc (it should ship the pool/agent identity in `GC_AGENT` or `GC_ALIAS` for rig pool/demand seats, as it already does for town agents such as `bd.dog-1`). This task implements a contained provider-side fallback so the rex integration works in the meantime, with the divergence called out in code and linked upstream.

## Fix direction (contained, in the rex provider)

In `internal/provider/provider.go` `StartNew`, normalize the launch env before `seedStartIdentity` / `launchCommand`: when `GC_ALIAS` is empty and `GC_TEMPLATE` is set (and `GC_TEMPLATE != GC_AGENT`), export a claimable agent identity derived from the qualified template/rig. Apply the same env to the pane command, `seedStartIdentity`, and `aliasesFor` so they agree. Never overwrite a non-empty `GC_ALIAS`.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 A rex rig pool/demand seat launched by gc can run gc hook --claim --drain-ack --json and claim its routed bead, with no no_work/no-op drain.
- [ ] #2 The root cause is established from a captured seat env: either the unset-vs-empty GC_ALIAS projection or the GC_TEMPLATE/session-context fallback is identified as the reason the claim finds no work, and recorded in the notes before any identity synthesis is added.
- [ ] #3 If identity synthesis is needed, the provider preserves a non-empty GC_ALIAS, else derives the qualified identity from GC_TEMPLATE (and GC_RIG), and applies it consistently to the pane env, seedStartIdentity and aliasesFor. Synthesis is skipped when gc explicitly sent an empty GC_ALIAS unless the notes prove the template fallback is broken.
- [ ] #4 The change does not silently diverge from runtime-herdr-core: a code comment states that herdr-core passes gc's identity through, links the upstream gc issue, and scopes the fallback to the broken case.
- [ ] #5 Unit tests in internal/provider cover: GC_ALIAS non-empty preserved; GC_ALIAS empty plus GC_TEMPLATE set; GC_TEMPLATE absent leaves the env unchanged; and the empty-value projection below.
- [ ] #6 go test ./... and golangci-lint run ./... pass.
- [ ] #7 Live check on ~/my-city: a rex-backlog order dispatch reaches a claimed bead and the selected backlog task moves to In Progress, with no session.demand_claim_divergence for the seat.
- [ ] #8 Empty start-config env values are applied to the pane as unset (env -u KEY), never as a literal empty variable, covered by a unit test.
- [ ] #9 Any derived identity leaves GC_SESSION_ID and GC_INSTANCE_TOKEN untouched, so a start-collision re-observation via get-meta still matches the pending create, covered by a unit test.
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. Reproduce on the current city with the seat's full pane env captured; confirm whether GC_SESSION_ID/GC_SESSION_NAME reach the pane and whether `gc hook --claim` takes the GC_TEMPLATE fallback (trace with `gc hook --claim --json` and a work-query echo). Confirm the gc version the city runs; the template fallback above is v1.5.1 behavior.
2. Fix the env projection first: apply empty start-config env values as unset (`env -u KEY`) when composing the launch argv, so gc's deliberate blank-GC_ALIAS pool semantics survive (parity doc `### start` row 9). Verify that the seat then claims.
3. Only if the template fallback still cannot claim, derive a claimable identity: keep a non-empty GC_ALIAS, else derive from a rig-qualified GC_TEMPLATE; apply it to the pane env, seedStartIdentity and aliasesFor. Never derive when gc explicitly sent `GC_ALIAS=""` without first proving the fallback path is broken, and never touch GC_SESSION_ID/GC_INSTANCE_TOKEN.
4. Document the divergence from runtime-herdr-core (it passes gc's env through) and link the upstream gc issue.
5. Unit tests for the env projection and the derivation rules; then the live order check.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
GCREX-19 parity analysis + gc 1.5.1 source re-check changed the diagnosis.

gc 1.5.1 already has a pool/template identity path that the earlier root cause did not account for (`cmd/gc/cmd_hook.go`):
- Agent resolution: `GC_ALIAS` then `GC_AGENT` (`:262-266`). With no positional argument, if `GC_TEMPLATE` is set AND session context is present (`GC_SESSION_NAME` or `GC_SESSION_ID` non-empty), gc uses the template and marks `sessionTemplateContext` (`:268-276`).
- If that lookup still fails, gc retries with `GC_TEMPLATE` when the unresolved name is one of `GC_ALIAS`/`GC_AGENT`/`GC_SESSION_NAME` (`:338-356`).
- `clearPoolTemplateRuntimeIdentity` deliberately blanks `GC_ALIAS` for pool/template seats, and the code comments treat a non-empty `GC_ALIAS` as "that did not run" (`:831-857`). Provider-side synthesis of `GC_ALIAS` therefore fights an intentional gc behavior.
- The claim identity writer order is alias -> session bead id -> agentForQuery -> resolved name -> session name (`hookClaimAssigneeIdentity`, `:850-868`), and every reader leads with the session bead id. The live capture's `GC_AGENT=<sid>` is therefore already the canonical owner string for readers, so `no_work` is more likely coming from the work-query/session-context path than from a missing alias.
- gc's start-config env carries `GC_SESSION_ID`, `GC_SESSION_NAME`, `GC_RUNTIME_EPOCH`, `GC_CONTINUATION_EPOCH`, `GC_INSTANCE_TOKEN`, `BEADS_HOLDER_TOKEN`, `GC_ALIAS` (always, possibly empty), and with session context also `GC_TEMPLATE`, `GC_SESSION_ORIGIN`, `GC_AGENT`/`BEADS_ACTOR` (`internal/session/lifecycle.go:33-89`). The template fallback hinges on `GC_SESSION_ID`/`GC_SESSION_NAME` reaching the pane.
- RPP defines an empty env value as "unset" and gc's tmux adapter emits `env -u KEY`; we emit `env KEY=`, so a literal empty `GC_ALIAS` is present instead of unset. See docs/provider-herdr-parity.md `### start` row 9.
- A start collision makes gc skip its post-failure `stop` and re-observe the session via `get-meta`, so `GC_SESSION_ID` and `GC_INSTANCE_TOKEN` must keep matching the pending create (parity doc `### start` row 5). Any identity synthesis must leave those two keys alone.
<!-- SECTION:NOTES:END -->
