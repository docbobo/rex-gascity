---
id: GCREX-28.1
title: Record verdicts and designs for the herdr-only runtime extensions
status: Done
assignee:
  - '@mc-wisp-slf7w4'
created_date: '2026-11-18T08:15:00Z'
updated_date: '2026-11-18T22:53:00Z'
labels:
  - research
  - provider
  - extensions
  - herdr
  - for-gascity
dependencies: []
references:
  - docs/provider-herdr-parity.md
  - internal/provider
  - cmd/gc-runtime-rex
  - https://github.com/gastownhall/gascity-packs/tree/main/runtime-herdr-core
parent_task_id: GCREX-28
priority: medium
type: spike
ordinal: 61000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Decide, per herdr-only extension, whether the Rex provider should implement, defer or reject it, and design the accepted ones. Split out of GCREX-28 on 2026-11-18 (task-type review): this is the research/decision part, delivering a document, not code.

Extensions to judge from docs/provider-herdr-parity.md: observe, inventory, kill-object, copy-to, server-stop, streaming watch-startup, tty attach, and the handshake tokens `city-scoped`, `conformance-faults`, `idle-boundary`, `input-outcome`, `fs.host`.

Inputs: GCREX-29 supplies the Rex-only candidates; GCREX-30 settles whether a city-owned server makes server-stop and watch-startup worthwhile; GCREX-31 supplies the Rex CLI facts (clear/format semantics, exit codes) that shape copy-to and watch-startup.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 docs/provider-herdr-parity.md (or a new docs/herdr-extensions.md that it links) records, for every herdr-only extension, a verdict of implement, defer or reject with the reason and the gc-side trigger that would make it matter.
- [x] #2 Each accepted extension has a design section: op contract, capability token, deadline class, stdin, stdout shape, exit codes, state and cleanup.
- [x] #3 No production code changes are required by this task; follow-up implementation tasks (or GCREX-28.2) are filed or linked for each accepted extension.
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. Read the parity doc sections for every herdr-only extension (observe, inventory, kill-object, copy-to, server-stop, watch-startup, attach) and the protocol token set.
2. Re-confirm against the verified gc 1.5.1 findings already in docs/provider-herdr-parity.md (gw never sends these ops; gc ignores unknown capability tokens).
3. Write docs/herdr-extensions.md: a verdict (implement/defer/reject) with reason and gc-side trigger for each extension and each non-baseline handshake token.
4. Add a full design section (op contract, capability token, deadline class, stdin, stdout shape, exit codes, state/cleanup) for each accepted extension.
5. Link the new doc from docs/provider-herdr-parity.md.
6. No production code changes (this is the decision half; implementation is GCREX-28.2). Verify with go test ./... and golangci-lint run ./... as health, keep the change uncommitted.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Work step (mc-wisp-slf7w4, 2026-11-18): decision doc implemented; no production code.

- New docs/herdr-extensions.md records, for every herdr-only extension (observe, inventory, kill-object, copy-to, server-stop, streaming watch-startup, tty attach) and every non-baseline handshake token (observe.v1, carrier.ops, server-lifecycle, city-scoped, fs.host, input-outcome, idle-boundary, conformance-faults), a verdict of implement/defer/reject with the reason and the gc-side trigger.
- Accepted set (implemented by GCREX-28.2): observe (capability observe.v1) and inventory (carrier.ops); each has a design section (op contract, capability token, deadline class, stdin, stdout shape, exit codes, state/cleanup). Everything else is defer or reject; server-stop/server-lifecycle, city-scoped, fs.host are reject.
- Rationale is pinned to the existing evidence: docs/provider-herdr-parity.md 'Gaps that are deliberate, not mistakes' (gc 1.5.1 never sends observe/inventory/kill-object/server-stop/copy-to; watch-startup exit 2 falls back to peek polling) and the protocol section (gc 1.5.1 ignores unknown capability tokens; REX-CONTRACT.md §9 forbids declaring non-baseline tokens unless implemented). The doc states that an implement verdict is also the §9 contract change GCREX-28.2 must carry.
- Linked from docs/provider-herdr-parity.md (intro, protocol-surface bullet).
- Verification: go test -count=1 ./... all packages ok; golangci-lint run ./... 0 issues. Change is docs-only and stays uncommitted for review. Only docs/herdr-extensions.md (new) and docs/provider-herdr-parity.md (link) are touched; agent-state dirs untouched.
- Handoff: review the decision reasoning and the accepted set; finish commits only after the verdict passes. GCREX-28.2 implements the two accepted ops behind their tokens.

Independent review (openrouter/deepseek/deepseek-v4.1-flash:high): fail.

Blocking
1. `observe` design has the wrong op contract (docs/herdr-extensions.md:77-95). The reference takes the session name as the single positional arg (`runtime-herdr-core/assets/gc_runtime_herdr_core/herdrcore/reads.py:57-59,131`), reads **agent process names** from stdin only to judge agent liveness (`reads.py:63-65,136-156`), and emits exactly one `ObserveAnswer` per invocation (`rpp/wire.py:162-198`). Empty stdin still answers for the named session. The doc instead says it reads agent names from stdin and prints "one JSON object per input name", with empty stdin yielding "no answers". That contradicts the parity doc's own observe section (`docs/provider-herdr-parity.md:966`: "stdin is a list of agent process names... It prints one JSON ObserveAnswer") and the reference source; the "cardinality unverified" note at :93-95 is false. AC2's op contract/stdin/stdout are therefore wrong. Fix the section to `observe <name>`, stdin = agent process names, one `ObserveAnswer`.

Major
2. `carrier.ops` is not an inventory marker (docs/herdr-extensions.md:47,62,107-109). The reference defines it as the carrier selector: "gc drives the box only through the pack's own nudge, peek, send-keys, interrupt and clear-scrollback" (`runtime-herdr-core/assets/gc_runtime_herdr_core/handshake.py:6-8`). gc's working tree implements exactly that (`internal/runtime/protocol.go:35-41`): all five ops required, exit 2 from one becomes failure, input fencing none, start welded. So "It supersedes nothing" and "Additive under carrier.ops" are wrong. At the v1.5.1 pin the token is ignored, so the change is inert today, but GCREX-28.2 must scope the claim to the pin and state the consequence/re-check for a gc that consumes it (the five driving ops and their exit-2 paths).

Minor / nits
3. docs/herdr-extensions.md:12: `attach` is one of the 17 baseline ops, not an exception; the beyond-table calls are `clear-scrollback`, `send-keys` and `watch-startup` (docs/REX-CONTRACT.md §9). Reword.
4. docs/herdr-extensions.md:49: v1.5.1 has no production caller of `Place.Stage`, but the op is wired: `execPlace.Stage` -> `Provider.CopyTo` -> `copy-to` (`internal/runtime/exec/seams.go:118-126`, `exec.go:793-797`, test `seams_test.go:232-266`). Say "no production caller" rather than "never sends"; the trigger is otherwise right.
5. AC1 asks for a gc-side trigger per item; the token table has no trigger column and `fs.host`/`server-lifecycle` state no model-change trigger. Add them or a sentence covering rejections.
6. Inputs GCREX-29, GCREX-30 and GCREX-31 are still To Do. The doc handles GCREX-30 via the server-stop trigger, but it should say the decisions are provisional until those inputs land.

Notes (out of scope, not a doc defect)
- `git status` also shows unrelated tracked edits (gcrex-27 frontmatter/label churn) and many untracked task files from other work; the worker note says only the two docs were touched. Stage only `docs/herdr-extensions.md`, `docs/provider-herdr-parity.md` and the GCREX-28.1 task markdown when committing.
- Scope checked: docs only; no production code changed. Did not re-run go test/golangci-lint because the diff has no code.

Verdict: fail.

Addressed review findings (fail verdict, 2026-11-18):

Blocking 1 — observe op contract (docs/herdr-extensions.md 'observe'): now documents `observe <name>` with the session name as the single positional argument; stdin is agent process names used only to judge agent liveness (empty stdin still answers for the named session); stdout is exactly one JSON ObserveAnswer per invocation. Removed the false 'one JSON object per input name' / 'cardinality unverified' wording. Matches reads.py:57-59,131 and rpp/wire.py:162-198 and the parity doc's observe section.

Major 2 — carrier.ops: removed 'supersedes nothing' / 'additive under carrier.ops'. The decision table and the handshake-token section now describe carrier.ops as the carrier selector (nudge, peek, send-keys, interrupt, clear-scrollback), note gc's working tree implements it (internal/runtime/protocol.go:35-41: all five driving ops required, exit 2 from any one becomes a failure, input fencing none, start welded), and scope the claim to the v1.5.1 pin with a re-check requirement for a consuming gc.

Minor 3 — attach wording: §9 line now lists clear-scrollback, send-keys and watch-startup as the beyond-table calls and attach as one of the 17 baseline ops.

Minor 4 — copy-to: changed 'never sends' to 'no production caller', keeping the seam-wired note (execPlace.Stage -> Provider.CopyTo -> copy-to).

Minor 5 — gc-side triggers: the token table has a trigger column; server-lifecycle and fs.host now carry explicit model-change triggers.

Minor 6 — provisional status: added a sentence that the decisions are provisional until GCREX-29/30/31 land.

Verification: go test ./... all packages ok; golangci-lint run ./... 0 issues. Docs-only change; no production code touched; change remains uncommitted for the next review iteration.

Independent review (openrouter/deepseek/deepseek-v4.1-flash:high): fail.

Iteration 2. Previous findings 1-6 re-checked and addressed: observe now documents `observe <name>`, stdin names for liveness only, one ObserveAnswer, empty-stdin answers (reads.py:57-59,131,136-156; wire.py:162-198); carrier.ops is described as the carrier selector with the gc working-tree consequence (handshake.py:6-8; gc HEAD internal/runtime/protocol.go:35-41); attach is listed as one of the 17 baseline ops; copy-to says "no production caller" with the seam cited (v1.5.1 exec.go:793-797, seams.go:119-126, no production Stage caller); the token table has a gc-side trigger column and the provisional note is in place.

Blocking
1. The observe.v1 decision is scoped as inert/additive, but at the gc that consumes the token it obliges kill-object — the one extension this doc defers. gc HEAD internal/runtime/protocol.go:84-89: observe.v1 "declares the exec observe contract reconciler v2 reads (PAR-EXEC): the observe, inventory and kill-object ops, fresh_only honored on start and provision, and exit 2 from a declared op read as a failure... With report-attachment it makes an exec runtime v2-ready." Our handshake already declares report-attachment (cmd/gc-runtime-rex/main.go:39), so declaring observe.v1 makes Rex v2-ready on the handshake alone (gc HEAD internal/runtime/exec/readiness.go:10-18,62-71; docs/reference/exec-session-provider.md "observe.v1": "gc reads it through observe.v1 on the handshake alone", obligations 1-13 including fresh_only and strict exits). gc's conformance then requires kill-object: internal/runtime/rppcheck/rppcheck.go:244 maps observe.v1 to "observe, inventory, kill-object", :602-608 appends killObjectCheck, :700-710 fails with "observe.v1 declared but kill-object not implemented (exit 2)". Yet herdr-extensions.md:55 defers kill-object, :68 marks observe.v1 "none needed — accepted now", :79-82 argues from the v1.5.1 pin only, and :170-171 puts kill-object out of GCREX-28.2's scope — contradicting the doc's own Fact 2 (:41-42) and the consuming-gc treatment it gives carrier.ops (:69, :154-161). Fix: either accept kill-object with observe.v1 (with its design section) or state the consuming-gc consequence and scope the declaration so GCREX-28.2 cannot declare observe.v1 without the profile (fresh_only start, strict exits; the profile also includes peek of a dead box, obligation 13).

Major
2. The inventory cap provenance is wrong (docs/herdr-extensions.md:124-126). "the reference's cap is a herdr-internal detail, not an RPP rule" is false for the gc that consumes observe.v1: docs/reference/exec-session-provider.md "inventory" says "Stdout is one JSON object of at most 8 MiB", gc HEAD enforces internal/runtime/exec/inventory.go:24-27 (inventoryOutputCap = 8 << 20) and observe.go:31 (1 MiB), and the reference itself says gc fails an observe answer over 1 MiB and an inventory answer over 8 MiB per gc's contract (rpp/ops.py:32-37). Require our cap to stay within gc's (<=8 MiB inventory, <=1 MiB observe) instead; the observe design (:86-111) omits its 1 MiB cap.

Minor / nits
3. inventory stdout shape (:128-132) says each session carries `box`, but the wire key is `object` (rpp/wire.py:228-240; gc's contract example uses "object"), and the reference allows a state-less entry carrying only `name` (:215-231). Name the JSON keys (or say these are the reference's Python field names) and the state-unknown entry.
4. observe exit codes (:107-109): "Reserve 2 for 'unknown op' (only during rollout)" suggests a window with the token declared and the op absent, which Fact 2 forbids and gc HEAD reads as a failure (protocol.go:87-88); say exit 2 is only for consumers not yet told the token.

Verdict: fail.

Addressed review findings (iteration-2 fail, 2026-11-18):

Blocking 1 — observe.v1 is the reconciler-v2 profile, not an additive one-op token; the doc now says so. Accepted the whole observe.v1 profile as one bundle: kill-object moves from defer to implement (decision table, 'Implement'), observe.v1's token row is retitled 'reconciler-v2 profile (observe, inventory, kill-object)', and Facts 1-3 separate the v1.5.1 pin (inert: its protocol.go defines none of the herdr-only tokens) from the consuming gc (observe.v1 obliges observe/inventory/kill-object, fresh_only starts, strict exits, identity consistency, peek of a dead box; with report-attachment it makes Rex v2-ready — internal/runtime/exec/readiness.go). GCREX-28.2 consequences now carry the profile obligations, not just three ops. This matches the reviewer's first offered fix; kill-object gets a full design section (op contract, capability token observe.v1, default deadline, stdin {object,mode,agent_pid}, stdout killed/gone/renamed/live/changed, exit codes, per-name lock + kill-tree/forget cleanup).

Major 2 — inventory cap provenance corrected: gc's own bound is 8 MiB (internal/runtime/exec/inventory.go:27) and observe's is 1 MiB (observe.go:31); the reference matches (rpp/ops.py:33-34). The inventory deadline bullet now requires our caps to stay within gc's (<=8 MiB inventory, <=1 MiB observe), and the observe stdout-shape bullet adds its 1 MiB cap.

Minor 3 — inventory stdout shape names the wire keys and the state-less entry: a session with no readable state carries only name; otherwise state, object (not box), agent, attached, last_activity, restart_requested, identity (rpp/wire.py:228-240).

Minor 4 — observe exit codes: exit 2 is only for a consumer not yet told the token; once observe.v1 is declared, exit 2 from a declared op is a failure (protocol.go; exec-session-provider.md obligation 3). Removed the 'only during rollout' window.

Deferred list no longer includes kill-object. No production code changed.

Verification: go test ./... exit 0 (no failures); golangci-lint run ./... 0 issues. Docs-only change; change stays uncommitted for the next review iteration; agent-state dirs untouched.

Independent review (openrouter/deepseek/deepseek-v4.1-flash:high): pass.

Scope: uncommitted docs-only change in /Volumes/Sources/rex-gas-city — new docs/herdr-extensions.md (256 lines) and the 2-line link in docs/provider-herdr-parity.md; GCREX-28.1 task markdown. No production code changed; untracked .pi/.pi-agent/.gc ignored.

Verification: the decision table covers every herdr-only extension and non-baseline token named in the task; each accepted extension (observe, inventory, kill-object) has the AC2 design bullets (op contract, capability token, deadline class, stdin, stdout shape, exit codes, state and cleanup); AC3 is met by linking GCREX-28.2. Claims re-checked against the parity doc, gc v1.5.1/HEAD, and the reference pack @0f07496: observe.v1 as the reconciler-v2 profile (internal/runtime/protocol.go:84-89; internal/runtime/exec/readiness.go v2ProfileCapabilities; docs/reference/exec-session-provider.md §observe.v1 obligations 1-13); rppcheck's observe.v1 -> observe, inventory, kill-object map (:244, :602-608, :700-710); carrier.ops as the five-driving-op selector (protocol.go:35-41); inventory 8 MiB / observe 1 MiB caps (inventory.go:27, observe.go:31, rpp/ops.py:33-34); kill-object semantics (herdrcore/lifecycle.py:542-593; exec-session-provider.md:1243); copy-to seam wiring; watch-startup 500 ms fallback; and the §9 17-row baseline plus beyond-table calls. All prior blocking/major findings from iterations 1-2 are addressed.

Minor/nit follow-ups (non-blocking):
1. docs/herdr-extensions.md:77 cites "GCREX-1 §11 D3"; the decision text is docs/REX-CONTRACT.md §11 D3 (line 416), pinned by GCREX-1. Reword.
2. docs/herdr-extensions.md:185 "agent_pid unchanged when given" matches the contract (exec-session-provider.md:1243), but the reference's _kill_object (herdrcore/lifecycle.py:560-593) does not compare agent_pid. Note the reference divergence so a future implementer does not assume the reference enforces it.
3. docs/herdr-extensions.md:169-172 says a stopped Rex server is complete:false; the reference's op_inventory returns complete:true for a stopped core (herdrcore/reads.py:201-205) because it proves nothing runs. The Rex choice is defensible; say why it differs.
4. docs/herdr-extensions.md:173-174 "no meta writes" — the reference's op_inventory runs _collect_garbage (herdrcore/reads.py:220-223), which can remove meta. Say our design forgoes housekeeping.
5. docs/herdr-extensions.md:120-122 "using the box's own program as the agent" — the reference sets agent_pid to the box's program but leaves agent None (herdrcore/reads.py:152-155). Say "agent pid" for precision.
6. docs/herdr-extensions.md:162-168 lists agent among the keys a session "has"; the reference omits agent when None (rpp/wire.py:227-240). Say "agent when known".
7. docs/herdr-extensions.md:167-168 "complete is false whenever any record could not be read" — the reference also sets complete:false when the whole core is unreachable (herdrcore/reads.py:201-205); "unreadable or unreachable" is more exact.

Verdict: pass.

Finalization (mc-wisp-tqshx9, 2026-11-18): independent review verdict pass (state file rex-backlog.review=pass). Re-ran health on the frozen docs-only change: go test ./... exit 0 (all packages ok); golangci-lint run ./... 0 issues. git diff --stat confirms no .go or production files touched; only docs/herdr-extensions.md (new), the 2-line link in docs/provider-herdr-parity.md, and the GCREX-28.1 task markdown are staged/committed.
<!-- SECTION:NOTES:END -->

## Comments

<!-- COMMENTS:BEGIN -->
author: @mc-wisp-slf7w4
created: 2026-11-18T22:06:00Z

---

Claimed by Gas City worker mc-wisp-slf7w4 (seat rex-gascity/pi-1, session pi-mc-wisp-slf7w4, template rex-gascity/pi); workflow bead mc-5ftu, step bead mc-uf2b. Note: the rex-backlog order selected the umbrella GCREX-28 (highest-priority ready), but GCREX-28 was split on 2026-11-18 into GCREX-28.1 (research/design, this task) and GCREX-28.2 (implementation, depends on this one). Working the actionable child rather than the umbrella.

---
<!-- COMMENTS:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
Decided and designed the herdr-only runtime extensions (research half of GCREX-28). Added docs/herdr-extensions.md: a verdict (implement/defer/reject) with reason and a gc-side trigger for every herdr-only extension (observe, inventory, kill-object, copy-to, server-stop, streaming watch-startup, tty attach) and every non-baseline handshake token (observe.v1, carrier.ops, server-lifecycle, city-scoped, fs.host, input-outcome, idle-boundary, conformance-faults). Accepted as one reconciler-v2 profile (observe.v1): observe, inventory, kill-object, each with a full design section (op contract, capability token, deadline class, stdin, stdout shape, exit codes, state/cleanup). Linked the doc from docs/provider-herdr-parity.md. No production code changed; GCREX-28.2 carries the implementation. Verification: go test ./... exit 0; golangci-lint run ./... 0 issues; change is docs-only. Risks/follow-ups (non-blocking, for GCREX-28.2): seven minor precision nits from the final review (reference divergence on kill-object agent_pid, stopped-server complete:false rationale, forgoing inventory housekeeping, agent/agent_pid wording, agent-when-known key, complete on unreachable core).
<!-- SECTION:FINAL_SUMMARY:END -->
