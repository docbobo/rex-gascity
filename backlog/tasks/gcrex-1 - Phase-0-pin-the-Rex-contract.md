---
id: GCREX-1
title: 'Phase 0: pin the Rex contract'
status: Done
assignee:
  - '@boris'
created_date: '2026-11-17T16:25:00Z'
updated_date: '2026-11-17T17:56:00Z'
labels: []
dependencies: []
references:
  - docs/REX-CONTRACT.md
  - docs/plans/native-rex-provider.md
ordinal: 1000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
The provider talks to a preview-version product whose CLI/API surface moves between releases. Before writing transport code we need one pinned record of the exact ops, schemas, key syntax, and error codes we rely on, so later breakage is a diff against a known baseline instead of a mystery. The contract doc is written; it needs review and a re-verification recipe for each Rex upgrade.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 REX-CONTRACT.md records the pinned Rex version and API revision
- [x] #2 The doc covers session/window/block creation, block methods, events, key syntax, and error/exit codes
- [x] #3 The doc includes a command block to re-verify every fact after a Rex upgrade
- [x] #4 The RPP v0 op and handshake shape is recorded for the out-of-tree delivery form
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. Re-verify the pinned surface against the live Rex server; split the pin into a CLI row and a server row, because they differ (CLI 0.1.0-preview.20 / api revision 16, running server 0.1.0-preview.19 / api revision 14, effective min = 14).
2. Replace the §10 probe block: `rex --version` does not exist, so the version probe is `rex version --json` (CLI) plus `rex api call server.version` (server). Fill the coverage gaps — key syntax, event names, the 1048576-byte reply cap, block-method schemas, topology methods, RPP handshake/exit codes.
3. Add a Rex CLI exit/error-code table from live measurement (AC 2).
4. Anchor the RPP v0 section to gc 1.5.1 with the gc-repo paths and `gc runtime check` as the mechanical verifier (AC 4).
5. Confirm D0–D6 in the doc (plan line 501 gate).
6. Run the §10 block verbatim from the doc to prove it re-verifies the tables, then get one independent read-only review against the live server (plan gate: "contract doc reviewed").
7. Check AC 1–4 against that evidence, write the final summary, move to Done.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Rewrote docs/REX-CONTRACT.md against sources, not from memory.

Version pins are now split: Rex CLI 0.1.0-preview.20 / commit e33d248 / api revision 16 (from `rex version --json`), Rex server 0.1.0-preview.19 / commit ad5a836 / api revision 14 (from `rex api call server.version` and `rex whoami --json`), effective revision 14 = min(16, 14). The previous single pin (preview.19 / 14) was the server only, and the probe meant to catch the drift (`rex --version`) exits 2 with "unknown flag". The corrected probe is `rex version --json` plus `server.version`.

gc side pinned to gascity 1.5.1 (the installed gc), source docs/reference/exec-session-provider.md, internal/runtime/protocol.go, internal/runtime/REQUIREMENTS.md. What the old section claimed did not match 1.5.1: RPP exit codes are only 0/1/2 (not 66/75/80), the op table has 17 rows (send-keys, clear-scrollback, watch-startup are driving ops gc calls today but absent from that table), the handshake is {version, capabilities} with no `locality` field, and the real capability set is report-attachment, report-activity, proc.exec, proc.provision, proc.stream, tty.attach. The names carrier.ops, fs.host, idle-boundary, observe.v1, server-lifecycle, input-outcome and the ops observe, inventory, kill-object, server-stop appear only in this repo docs (research read gascity main without recording the revision); they now live in a section that says they are not in the pinned baseline and must not be declared.

Other corrections: format on the pinned server returns {content,id,total} and omits eof even when more remains (observed {"id":2,"total":25332,"len":814}); blob_read returns eof correctly, so detect more output by id. Added the D0-D6 confirmation table the Phase 0 gate asks for, a Rex CLI exit-code table (verified 0 success, 2 usage, 3 not-found, 1 op-level refusal), and note that the CLI exit 2 is a usage error, not RPP exit 2.

Evidence: the section 9 bash block was extracted from the document and run verbatim; everything succeeded, and the exit-code probes printed ok=0 usage=2 no-method=3 no-target=3 no-block-method=1.

Rewrote docs/REX-CONTRACT.md (uncommitted) after re-verifying every fact against the live server. Corrections found:

- The old pin `Rex version 0.1.0-preview.19 / API revision 14` was the running server, not the CLI the provider drives (§7), and the doc never said which. Live: CLI `rex version --json` = 0.1.0-preview.20 / api revision 16; server `rex api call server.version` = 0.1.0-preview.19 / api revision 14; effective min = 14. §0 now pins both, with commits and build dates.
- The §10 version probe `rex --version` does not exist (exit 2, "unknown flag"); the whole block died on line 1 and so never caught the CLI/server skew.
- §3 claimed `session.destroy` requires the caller to be attached or fails `conflict`. It does not reproduce: a fresh CLI call returned exit 0 and closed the session. Caveat removed; §12 records it as resolved.
- 983040 was called the byte cap; the hard control-response cap is 1048576 bytes (format with no max_bytes over a 2 MB capture fails `too_large` 413, exit 1). 983040 is our request size. §4/§12 corrected.
- §4 `size`/`pwd` descriptions were loose; live output is size -> geometry + owning client, pwd -> cwd + owner.
- `target.resolve` needs `kinds[]`/`queries[]`, not bare id/label/prefix.

Added: §8 Rex CLI exit/error codes (0 success; 1 server failure or decode failure, e.g. too_large 413 / bad_request 400; 2 client usage/validation; 3 target not found), §9 RPP v0 provenance (gc 1.5.1, gc-repo paths, `gc runtime check`), §11 D0–D6 confirmations, and a rewritten §10 probe block that creates and destroys its own scratch session.

Evidence: the §10 block was extracted from the doc with awk and run verbatim — exit 0, all lines match the tables (exit codes 3/2/2, 15 topology methods present, 11 block methods, 13 events, scratch round-trip saw_probe=true, blob_read {} -> 3, ctrl+c and [Slash] ok, block.event paste ok, block.close {block_id,revision}, session.destroy ok, rex events --json -> 124, gc version 1.5.1). No scratch session leaked.

Safety correction after review feedback: the first §10 draft called `rex api call session.destroy` on a scratch session. Removed — a destroy in a paste-able recipe is a bad idea because a wrong or empty variable could name the wrong session. §10 now creates exactly one scratch session, aborts unless the id matches `session:*`, and kills only via a trap on that id; `session.destroy` is asserted from `rex api describe` output instead of executed.

Also corrected a wrong claim of mine: `rex api describe session.destroy` states the connection must be attached (`session.attach`/`bootstrap`/`attach_all`) or the call fails `conflict`, and an unknown id fails `session_not_found`. My earlier "the attachment caveat did not reproduce" was wrong — a fresh CLI call returned exit 0 because the CLI satisfies the attachment itself. §3 and §12 now keep the requirement and mark the op destructive and unused by the provider (teardown uses block.close).

Re-ran the §10 block verbatim after the edit: exit 0, scratch session cleaned up, guard negative-test aborts on a bad id. Note: the background reviewer left one scratch session (`gc-review-scratch`, 6ef429) alive; it will be killed after the review finishes. No user session was touched (Nix Config, nixpkgs, rex-gas-city all intact).

Independent review (gcrex1-review, Claude Sonnet, cross-family) found genuine defects; all fixed in a second rewrite of the doc, re-derived from the gc 1.5.1 source rather than from the plan.

Blocking finding, AC 4: §9 did not match gc 1.5.1. RPP has exactly three exit codes (0/1/2) — the 66/75/80 codes came from the plan, not the spec — and exit 2 means "unknown op, treated as success" except from `start`/`provision`. The spec op table has 17 rows; `observe`, `inventory`, `kill-object` and `server-stop` are not in it, while `clear-scrollback`, `send-keys` and `watch-startup` are called by gc but deliberately absent from the table (the driving ops are a documented fallback for a runtime without `exec`). The handshake has no `locality` field, and the pinned baseline has exactly six capabilities (report-attachment, report-activity, proc.exec, proc.provision, proc.stream, tty.attach, from internal/runtime/protocol.go). `fs.host`, `carrier.ops`, `observe.v1` and `input-outcome` appear nowhere in gc 1.5.1; `idle-boundary` is design-doc language and `server-lifecycle` only a release-gate filename. §9 now records all of that and has a "not in the pinned baseline — do not declare" list.

Other fixes: §4 blob_read input fields are optional, not required (id 0/omitted reads the newest result, max_bytes defaults to 49152 and clamps at 983040, results live ~5 minutes) and the output {id,data,total,eof} is what is required — the old "{} fails not_found proves the fields are mandatory" was wrong. §4 format: `content` is always the only required field; with max_bytes set, `total` is always present and the reply carries either `id` (more remains, `eof` absent) or `eof: true`. §5 process_changed also carries `ancestors`. §3 session.view `clients[]` appears only while clients are attached. §8 malformed ids land on 2 (bad JSON argument), 3 (bad positional target) or 1 (bad field reaching the server decoder) by path, not one code.

§10 now exercises the real paging path (format with max_bytes -> id -> blob_read), the >1 MiB too_large cap, and, with GC_SRC set, greps the six capability names out of the pinned gc source. Re-ran it verbatim: exit 0, paging has_id=true/eof=false then blob_read returns data, format over cap -> 1, gc 1.5.1, six capabilities printed, scratch session cleaned up.

Provenance note: the task already carried a note describing exactly these corrections, written before this pass; it appears to be orphaned output from a session that no longer exists (its log is gone and no other checkout or stash has the described doc). My first rewrite had reverted §9 to the plan text; this pass restores the source-derived version, so the note and the doc now agree.

Second review round (same reviewer, resumed, cross-family): all four acceptance criteria met. Two residual errors it named are now fixed: the doc claimed gc ships an OSC 7501 emitter at internal/runtime/runtimetest/osc7501emitter — no file in gc 1.5.1 mentions 7501, so that claim is removed and §4/D5 now say the emitter is ours to write; and `session.view.clients[]` does not exist in the schema — §3 now points attachment at `session.list_clients` (verified, returns clients[] with client_id/principal/info/connected_at/connection_state) and §12 records the D1 attachment consequence.

Also hardened §10: it now asserts the two session.destroy schema lines instead of printing them, bumps the cap-write sleep to 3 s for margin, checks session.list_clients, and (with GC_SRC) re-verifies §9 mechanically — the exit-code rows (0/1/2), the op-table row count (17), and the six capability names. Re-ran verbatim: exit 0, every line matches, scratch session cleaned up (client_count:3 on the scratch session, format over cap -> 1, 17 op rows).
<!-- SECTION:NOTES:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
Phase 0 pins the Rex contract so later breakage is a diff against a known baseline. Both the Rex and the RPP surfaces are now recorded from sources, not from the plan.

What docs/REX-CONTRACT.md now records. §0 pins two Rex versions because they differ: the CLI the provider drives (§7) is 0.1.0-preview.20 / api revision 16, the running server answers at 0.1.0-preview.19 / revision 14, so the effective surface is min = 14. The old single pin was the server alone, and the probe meant to catch the drift (`rex --version`) does not exist — it exits 2 with "unknown flag", so the block died on line 1. §8 records the Rex CLI error space (0 success; 1 server failure or decode failure, with the status/code in the message; 2 client usage or validation; 3 target not found; a malformed id lands on 2, 3 or 1 by path). §9 records RPP v0 as gc 1.5.1 actually defines it: exit codes 0/1/2 only, exit 2 meaning "unknown op, treated as success" except from `start`/`provision`; the spec's 17-row op table; `clear-scrollback`/`send-keys`/`watch-startup` called by gc but deliberately outside that table; the handshake `{"version":0,"capabilities":[...]}` with no `locality`; and exactly six capabilities. The plan's `fs.host`, `carrier.ops`, `observe.v1`, `input-outcome`, `idle-boundary` and `server-lifecycle` are listed as not in the pinned baseline. §4 corrects block-method I/O (blob_read inputs optional, its four output fields required; format's only required field is `content`, with `id` vs `eof` marking more-remains; the 1 048 576-byte reply cap, where 983040 is our request size), and notes gc ships no OSC 7501 emitter. §3 points attachment at `session.list_clients` because `session.view` has no `clients` field, §5 adds `process_changed.ancestors`, §10 is a runnable probe block, §11 confirms D0–D6, §12 lists the open items.

Verification. The §10 block is extracted from the document and run verbatim (with `GC_SRC` set to the gc 1.5.1 source): exit 0, every printed line matches the tables, and it re-verifies the RPP exit-code rows and the 17 op rows from the pinned source, the six capabilities, the block methods, the 13 events, real paging (`format` -> `id` -> `blob_read`), the `too_large` cap, key syntax, and the events stream; it creates one scratch session, kills only that id, and never calls `session.destroy`. Two independent read-only review rounds (Claude Sonnet, cross-family) checked the doc against the live server and the gc source. The first found §9 contradicted gc 1.5.1 — AC 4 failed — plus defects in §4/§5/§8; all were fixed. The second re-ran the probe and returned met for all four criteria.

Follow-ups. The plan's capability vocabulary has no basis in gc 1.5.1 and must be rewritten before Phase 4. Decide whether Phase 5 promotion requires a restarted server, given the CLI/server skew. Attachment detection goes through `session.list_clients`. An OSC 7501 emitter is ours to write if we want native idle state. Block-id persistence across a server restart and rig-tab split limits remain unverified because restarting the shared server is out of bounds.
<!-- SECTION:FINAL_SUMMARY:END -->
