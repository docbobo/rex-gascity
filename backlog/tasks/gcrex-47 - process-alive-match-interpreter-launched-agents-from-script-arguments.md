---
id: GCREX-47
title: 'process-alive: match interpreter-launched agents from script arguments'
status: Done
assignee:
  - '@mc-wisp-45jqvl'
created_date: '2026-11-18T22:05:00Z'
updated_date: '2026-11-19T02:09:00Z'
labels:
  - provider
  - herdr
  - for-gascity
  - tdd
dependencies: []
references:
  - docs/provider-method-audit/process-alive.md
documentation:
  - docs/provider-herdr-method-io.md
priority: medium
type: task
ordinal: 103000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Develop test-first (TDD): write the failing test that reproduces this first, watch it go red, then the smallest change that makes it green (repo rule in AGENTS.md; see also the tdd skill).

Avoids: node/bun/npx/deno running an agent reads as dead.

processMatches checks Name, Argv0 and the basenames of Path/Argv0, so node /path/to/claude does not match. The document (## process-alive § Success output, § Conflicts) expects the full box subtree searched with node/bun/npx/deno script arguments matched. The audit rates this high; it is included here rather than left untracked.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 A process named node, bun, npx or deno matches when the agent name appears in its script arguments.
- [x] #2 The existing name and basename matches still work, covered by a test.
- [x] #3 A test covers node /path/to/claude reading as alive.
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. Model Rex's process `script` field on rex.Process (the live block process schema sets it for interpreters); parse it through the existing BlockProcess reply. 2. Extend processMatches to match the basename of Script and that basename without its extension (pack matches node/bun/npx/deno script args with and without extension), and treat Script as readable identity in processOpaque. 3. TDD: add failing liveness table cases for node + /path/to/claude, an ancestor, and agent.js with the agent hint; add a BlockProcess wire-parse test for script. 4. Docs: update REX-CONTRACT process schema, the parity process-alive interpreter row/verdict, and move the interpreter half of the audit's high row to Matches. 5. Verify go test ./... and golangci-lint run ./...; leave uncommitted for review.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Implemented (uncommitted, for review). Rex's live block-process schema reports an interpreter's `script` path; added `Script` to `rex.Process` (parsed by the existing BlockProcess reply). `processMatches` now also matches `basename(script)` with and without its extension, and `processOpaque` treats a process with only a script as readable. TDD red->green: the four new liveness cases (node + /path/to/claude foreground/ancestor, agent.js with hint agent, agent.js literal) failed first with `processAlive = 2 (dead), want 1 (alive)`; they pass after the match. Existing name/argv0/path cases and TestBlockProcess still pass, and the CLI parse test now asserts `Foreground.Script`. Docs: REX-CONTRACT process schema lists `script`; parity process-alive interpreter row is now intentional, verdict `intentional`, residual-mistake table/follow-up updated; audit process-alive high row narrowed to the remaining subtree scope (Low) and the interpreter rule moved to Matches. Verification: go test -count=1 ./... all packages ok; gofmt -l cmd internal empty; golangci-lint run ./... 0 issues. Follow-up: docs/provider-herdr-method-deviations.md is a stale GCREX-39 artifact and still carries the old high/medium rows.

Independent review (step pinned openrouter/deepseek/deepseek-v4.1-flash:high; session mc-wisp-9sa6nu reports model deepseek/deepseek-v4.1-flash): findings by severity. Blocking: none. Major: none. Verdict: pass.

Minor:
1. Line-citation drift after this change shifted liveness.go by +10 lines: the parity doc still cites ProcessAlive at liveness.go:155-182 (actual 165-188), paneAlive at 145-148 (actual 155-158), and the ErrNotFound branch at 161-164 (actual 170-174, row 3); the audit file cites liveness.go:155-159 (actual Resolve path 166-169) and the unreadable branch 172-176 (actual 175-179).
2. REX-CONTRACT section 0 still pins the running server at 0.1.0-preview.19 / rev 14 and says every table was verified at revision 14, while the new `script?` field was verified against the live server 0.1.0-preview.20 / rev 16 (`rex api call server.version`). Parsing is additive, so a server without the field degrades to the previous behavior; record the provenance or refresh section 0.
3. Rex's interpreter scope is a superset of the pack's rule: herdr-core/gc gate script matching to node/bun/npx/deno (herdrcore/procs.py:63; gc internal/runtime/tmux/state_cache.go knownInterpreters), but Rex also reports `script` for other interpreters (live probe: python3 /tmp/gc-probe-script/agent.py -> script set). Parity row 2 and the audit Matches bullet read as if the rule were identical; a clause noting the Rex match is broader (and can only add matches) would make them exact.

Follow-up carried from the worker: docs/provider-herdr-method-deviations.md (untracked GCREX-39 artifact) still lists GCREX-47 as an open high deviation.

Verified: go test ./... ok (all packages), golangci-lint run ./... 0 issues, gofmt -l empty. AC1/AC2/AC3 are covered by the new TestProcessAlive table cases (node/bun/npx/deno; node + /path/to/claude; script with and without extension) and TestBlockProcess asserts Foreground.Script; the readable-script-with-no-match case pins the processOpaque change. Cross-checked the declassified parity claims: gc v1.5.1 internal/runtime/tmux/state_cache.go (knownInterpreters; first non-flag, non-run/exec/x arg; basename with and without extension), gc internal/runtime/exec/exec.go:703-706,709-718 (empty output reads alive), herdr-core procs.py:63,564-583. Live Rex probes on the running server: `node <file>` returns script = the file path, a bin-wrapper node (argv0 pi) omits the key, python3 reports it too.

Scope note: the working tree also carries unrelated uncommitted churn (backlog/tasks/gcrex-27, gcrex-28 markdown normalization/label edits) and other tasks' untracked files; finish must stage only internal/rex/client.go, internal/provider/liveness.go, internal/provider/liveness_test.go, internal/rex/cli/cli_test.go, docs/REX-CONTRACT.md, docs/provider-herdr-parity.md, docs/provider-method-audit/process-alive.md and this task's markdown.

Recovery note: the re-dispatched work step (mc-tyvj) saw GCREX-47 still In Progress from the stalled session mc-wisp-45jqvl, correctly no-oped, and its cleanup deleted the task-id state file; the reviewer restored /Users/boris/my-city/.gc/state/rex-backlog.task = GCREX-47 so address/finish can complete this run. The formula's no-op gate should distinguish a live claim from a stalled one (related: GCREX-36).

Finalization verification (finish step mc-ejz0, 2026-11-19T02:09:10Z): review verdict /Users/boris/my-city/.gc/state/rex-backlog.review = pass; no blocking/major findings. Ran go test -count=1 ./... (all packages ok), gofmt -l cmd internal (empty), golangci-lint run ./... (0 issues). AC1 covered by 'foreground matches an interpreter script', 'ancestor matches an interpreter script', 'script matches without its extension', 'script matches with its extension' in TestProcessAlive; AC2 by the pre-existing name/argv0 basename cases that still pass; AC3 by 'foreground matches an interpreter script' (node + /path/to/claude). TestBlockProcess still asserts Foreground.Script. Staged only the task's files.
<!-- SECTION:NOTES:END -->

## Comments

<!-- COMMENTS:BEGIN -->
author: @mc-wisp-45jqvl
created: 2026-11-19T01:42:00Z

---

Claimed by Gas City worker mc-wisp-45jqvl (seat rex-gascity/pi-1, session pi-mc-wisp-45jqvl, template rex-gascity/pi); workflow bead mc-5zqo, step bead mc-tyvj.

---
<!-- COMMENTS:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
Added interpreter-script matching to provider process liveness. rex.Process gained a Script field parsed from the existing BlockProcess reply; processMatches now also matches basename(script) with and without its extension, and processOpaque treats a script-only process as readable. This fixes node/bun/npx/deno running an agent reading as dead (gc configures the agent name the interpreter runs as a script argument). TDD: four new TestProcessAlive liveness cases and a BlockProcess script assertion failed first, then pass. Docs updated: REX-CONTRACT process schema, parity process-alive interpreter row/verdict, and the audit row moved to Matches. Verification: go test ./... ok, gofmt clean, golangci-lint 0 issues. Review: pass, no blocking/major. Risks/follow-ups (minor, from review): line citations in the parity/audit docs drifted by +10 lines after this change; REX-CONTRACT section 0 still pins the older server rev while script? was verified against 0.1.0-preview.20/rev16; Rex's interpreter scope is broader than the pack's node/bun/npx/deno gate and the docs could say so. docs/provider-herdr-method-deviations.md (untracked GCREX-39 artifact) still lists GCREX-47 as an open high deviation.
<!-- SECTION:FINAL_SUMMARY:END -->
