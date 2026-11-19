---
id: GCREX-63
title: 'idle: answer gc''s pane-idle snapshot from the provider'
status: In Progress
assignee:
  - '@mc-wisp-hd1cwp'
created_date: '2026-11-19T07:47:00Z'
updated_date: '2026-11-19T12:08:00Z'
labels:
  - provider
  - herdr
  - tdd
  - for-gascity
dependencies: []
references:
  - docs/provider-herdr-parity.md
  - docs/herdr-extensions.md
  - cmd/gc-runtime-rex/main.go
  - internal/provider/activity.go
priority: medium
type: feature
ordinal: 119000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
On 2026-11-19 gc drained a Rex worker (mc-wisp-qnenkn) with 'execution-claim-nudge: ... unexecuted after 0/3 nudge attempts; draining (execution-stalled)' while the agent was still working. gc's execution backstop has two guards against draining a live seat: the activity clock (sessionQuietHold, cmd/gc/execution_backstop.go) and a point-in-time pane snapshot that vetoes that clock (snapshotHoldFor). The snapshot guard is only available when the backend implements runtime.IdleSnapshotProvider. tmux does (internal/runtime/tmux/adapter.go:760, tri-state: (false,nil) means the capture contains a busy indicator, runtime.ErrIdleIndeterminate means neither busy nor ready prompt). gc's exec backend does NOT, so for a Rex city snapshotHoldFor returns "" and the activity clock decides alone. We already hold both halves of the answer: Peek returns pane content, and the provider already reads Rex program_status records for get-last-activity and nudge gating (internal/provider/activity.go, internal/provider/nudge.go). This task designs the gc-facing reading and implements the provider half behind a capability gate so gc ignores it until it consumes it. Relationship to docs/herdr-extensions.md: the idle-boundary token is recorded there as 'defer' because gc 1.5.1 has no consumer; a SnapshotIdle-equivalent read is a separate concern from that token, but revisit the verdict in the same change if the token is what gates the reading.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 docs/provider-herdr-parity.md (or docs/herdr-extensions.md) records how gc obtains a point-in-time idle reading from a pack runtime, and whether an RPP op or a capability token is the right seam, with the gc file:line that would consume it.
- [ ] #2 The tri-state contract is explicit: idle, busy, and indeterminate are distinguished, and a failed read is never reported as idle (mirroring tmux's ErrIdleIndeterminate).
- [ ] #3 The reading is derived from data the provider already has (program_status records and/or Peek content), with a named test per state.
- [ ] #4 Any new capability token is declared only in the change that implements the behaviour, and the baseline gc 1.5.1 handshake is unchanged.
- [ ] #5 go test ./... and golangci-lint run ./... pass.
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. Provider reading: add internal/provider/idle.go with IdleState (idle|busy|indeterminate) and Provider.IdleSnapshot(name). Resolve the pane fresh; a missing session is an error, never idle. Prefer the newest program_status record: idle -> idle, working/blocked -> busy, any other state -> indeterminate. With no record, fall back to the pane screen (BlockFormat): a busy-indicator line -> busy, a startup dialog -> indeterminate, a bare ready-prompt line -> idle, anything else -> indeterminate. A read failure returns an error; the op never prints idle on failure. 2. Named tests per state in internal/provider/idle_test.go (idle, busy, indeterminate from program_status; idle, busy, indeterminate from the screen fallback; gone and transport errors). 3. Dispatcher: register an 'idle <name>' op in cmd/gc-runtime-rex/main.go that prints the state word on success and exits 1 on a failed read; declare the idle-boundary capability in the handshake, in the same change that implements the op (gc 1.5.1 ignores unknown tokens). Add cmd/gc-runtime-rex/idle_test.go for the op's tri-state and failure exit; update the handshake assertion in dispatch_test.go only for the added token. 4. Docs: record the seam in docs/provider-herdr-parity.md (an RPP op, gated by the idle-boundary token, consumed by a future gc SnapshotIdle on the exec backend, read at cmd/gc/execution_backstop.go snapshotHoldFor) and move the idle-boundary verdict to implement in docs/herdr-extensions.md; note the contract in docs/REX-CONTRACT.md §9. 5. Verify go test ./... and golangci-lint run ./... pass.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Implemented in work step (uncommitted). Change: (1) internal/provider/idle.go: IdleState (idle|busy|indeterminate) and Provider.IdleSnapshot(name), preferring the newest program_status record (idle->idle, working/blocked->busy, any other state->indeterminate) and falling back to the rendered pane (busy marker->busy, startup dialog->indeterminate, bare ready prompt->idle, else indeterminate). A missing session is ErrSessionNotFound and a failed program-status or pane read is returned as-is, so a failed read is never idle. (2) internal/provider/idle_test.go: named tests per state for both sources plus the three failed-read cases. (3) cmd/gc-runtime-rex/main.go: a new 'idle <name>' op printing the state word on exit 0 and exiting 1 on a failed read, and the idle-boundary capability added to the handshake in the same change. (4) cmd/gc-runtime-rex/idle_test.go: the op's tri-state and failure exit; dispatch_test.go's TestProtocolHandshake updated for the added token only. (5) Docs: docs/provider-herdr-parity.md gains a 'Rex-specific extension: the idle snapshot' section; docs/herdr-extensions.md moves idle-boundary from defer to implement and adds the idle design section; docs/REX-CONTRACT.md §9 notes the op and token. Verification: go test ./... all packages ok; golangci-lint run ./... 0 issues. Red->green: the provider tests failed to compile before idle.go; TestIdleSnapshotFromProgramStatus/working and the dispatch idle-op tests pin busy vs indeterminate. gc side (working tree 890a58a, ahead of the v1.5.1 pin): runtime.IdleSnapshotProvider internal/runtime/runtime.go:757; exec backend lacks it (internal/runtime/exec/cutover.go:46-48); snapshotHoldFor cmd/gc/execution_backstop.go:493; idle-boundary token internal/runtime/protocol.go:60; WaitForIdle gate internal/runtime/exec/idle.go:36; consumers internal/runtime/program_status_shadow_fresh.go:101 and cmd/gc/idle_nudge.go:673. gc 1.5.1 (tag) declares only the six baseline capabilities, so the added token is inert there. Staging caveat for the finish step: main.go carries the pre-existing GCREX-44 opInterrupt hunk; dispatch_test.go carries GCREX-44 hunks. Stage only this task's hunks (main.go import/const/handshake/ops/opIdle; the two TestProtocolHandshake lines).

Independent review (openrouter/deepseek/deepseek-v4.1-flash:high) of GCREX-63's uncommitted change: internal/provider/idle.go and internal/provider/idle_test.go (new), the main.go handshake/ops/opIdle hunks, cmd/gc-runtime-rex/idle_test.go (new), the two TestProtocolHandshake lines in dispatch_test.go, and the GCREX-63 doc hunks (parity §"Rex-specific extension: the idle snapshot", herdr-extensions idle row/design, REX-CONTRACT §9 idle notes). Excluded as other work: the GCREX-44/decision-1 interrupt bestEffort hunks (main.go opInterrupt, dispatch_test.go interrupt rows, REX-CONTRACT/parity interrupt prose, provider-method-audit/interrupt.md, gcrex-44 task md, decision-1), AGENTS.md, the GCREX-37/39 doc corpus, and .pi/.gc runtime state.

ACs: #1 the seam (an `idle <name>` RPP op gated by the `idle-boundary` token) and the would-be gc consumer (snapshotHoldFor, cmd/gc/execution_backstop.go:493; ShadowIdleBoundaryNow, internal/runtime/program_status_shadow_fresh.go:101; turnHoldFor, cmd/gc/idle_nudge.go:673) are recorded in the parity extension section and herdr-extensions; every gc cite re-checked at the working tree 890a58a (protocol.go:60, runtime.go:757, tmux/adapter.go:760 with the tmux.go:5868-5883 tri-state, exec/cutover.go:46-48 omitting IdleSnapshotProvider, exec/idle.go:36, execution_backstop.go:493, program_status_shadow_fresh.go:101, idle_nudge.go:673, paneContainsBusyIndicator at tmux.go:6240-6250). #2 tri-state explicit and a failed read is an error, never idle (idle.go; opIdle exits 1; TestIdleSnapshotFailedReadsAreErrors, TestIdleOpFailedReadIsAnError). #3 derived from the program_status records and the pane read the peek path already uses, with named per-state tests, and the real client returns empty records with nil error so the fallback is reachable. #4 the token is declared in the same change as the op; gc v1.5.1 declares only the six baseline strings and ignores unknown ones, and on the edge tree idleBoundaryDeclared still requires carrier.ops or proc.exec (exec/idle.go:36), so it is inert. #5 verified by this review: go test -count=1 ./... all packages ok; golangci-lint run ./... 0 issues; gofmt clean.

Major findings (must be fixed before commit):
1. docs/provider-herdr-parity.md §protocol is not updated and now contradicts the shipped handshake. Line 294 still quotes the old body `{"version":0,"capabilities":["report-attachment","report-activity"]}` and "Both tokens are in the six-token gc 1.5.1 baseline"; row 1 at line 302 says "2 tokens, both in the gc 1.5.1 baseline" and "We implement none of the matching features"; the verdict at line 306 says "Our handshake declares only implemented baseline tokens". All false after main.go:46 prints three tokens, one of them non-baseline and gating the implemented `idle` op. Fix the implementation paragraph, its literal JSON, deviation row 1 and its rationale, and the verdict sentence (and refresh the stale main.go:35/108 cites: the constant is main.go:46, the protocol branch is main.go:206).
2. docs/REX-CONTRACT.md §9 is internally contradictory after this change: the heading at line 314 says "Not in the pinned baseline — do not declare", the body at line 320 says "our provider declares it for the `idle` snapshot op (GCREX-63)", and the paragraph closes at lines 325-326 with "idle state is reported through `report-activity` plus the block's `program_status`, not through a handshake token". docs/herdr-extensions.md:40 (Fact 2) still repeats the blanket rule. The rule needs the exception the task relies on (declared only in the change implementing the gated behavior, inert at the pin), and the closing sentence must separate state reporting from op gating. The §9 example JSON at line 293 also still shows two tokens.
3. The staging caveat is incomplete and can corrupt the record. It names only main.go and dispatch_test.go as carrying pre-existing GCREX-44 hunks, but docs/REX-CONTRACT.md and docs/provider-herdr-parity.md also carry decision-1 (interrupt bestEffort) hunks. If the finisher stages whole doc files while holding the opInterrupt hunk back, the commit's docs will claim best-effort exit 0 against exit-80 code. List the GCREX-44 doc hunks (plus provider-method-audit/interrupt.md, the gcrex-44 task md, decision-1, AGENTS.md and the GCREX-37 corpus) as excluded from this commit.

Minor (follow-ups):
4. internal/provider/idle.go's busyMarkers mirror only the three literal clauses of gc's paneContainsBusyIndicator; the claudeBusySpinnerRe and claudeBareGerundSpinnerRe clauses are not ported, so a spinner-only pane with no program_status record reads indeterminate where tmux reads busy. That is the safe reading (the clock decides, as with no snapshot) and program_status is primary, so not blocking; either port the regexes or narrow the "classifies like the tmux pane gc reads" comment.
5. Doc line-cite drift to refresh while fixing 1 and 2 (the parity §protocol main.go cites and the REX-CONTRACT cites it quotes).

Verdict: fail.

Addressed review findings (iteration 1; docs only, no code change this pass).

1. docs/provider-herdr-parity.md §protocol: rewrote the implementation paragraph for runWithContext, the three-token handshake JSON (report-attachment, report-activity, idle-boundary), and the main.go cites (constant main.go:46, protocol branch main.go:206, write main.go:207, write-failure main.go:208-211, comment main.go:42-45); corrected deviation row 1 (3 tokens; idle-boundary is the only non-baseline token and gates the implemented idle op), row 3's main.go cite, and the verdict sentence; refreshed the REX-CONTRACT cites it quotes (baseline table :302-312, §9 example :293, exception :314-321). Also fixed the other now-false 'handshake declares only report-attachment and report-activity' statements in the same doc (exec, is-attached, get-last-activity, attach, server-stop) and their main.go:35 cites.

2. docs/REX-CONTRACT.md §9: the example JSON now lists idle-boundary; the heading/rule is now 'Not in the pinned baseline — declare only alongside the behavior it gates', which carries the idle-boundary exception and separates idle state reporting (report-activity + program_status) from op gating. docs/herdr-extensions.md Fact 2 carries the same exception and the §9 quote at line 55 is updated.

3. Corrected staging caveat (supersedes the earlier one): the finish commit must exclude the GCREX-44/decision-1 hunks in both code and docs — main.go opInterrupt, dispatch_test.go interrupt rows, the interrupt bestEffort prose in docs/REX-CONTRACT.md and docs/provider-herdr-parity.md, docs/provider-method-audit/interrupt.md, the gcrex-44 task md, backlog/decisions/, AGENTS.md, the GCREX-37/39 doc corpus, and .pi/.gc runtime state. GCREX-63's own hunks are internal/provider/idle.go, internal/provider/idle_test.go, cmd/gc-runtime-rex/idle_test.go, the main.go handshake/const/import/ops/opIdle hunks, the two TestProtocolHandshake lines in dispatch_test.go, the REX-CONTRACT §9 idle notes, the herdr-extensions idle row/design plus Fact 2, and the parity §protocol fixes above plus 'Rex-specific extension: the idle snapshot'.

Verification: go test ./... all packages ok; golangci-lint run ./... 0 issues.

Independent review (openrouter/deepseek/deepseek-v4.1-flash:high), re-review after the addressed findings. Scope: GCREX-63's uncommitted change only (internal/provider/idle.go, internal/provider/idle_test.go, cmd/gc-runtime-rex/idle_test.go, the main.go handshake/const/import/ops/opIdle hunks, the two TestProtocolHandshake lines in dispatch_test.go, and the GCREX-63 doc hunks). Excluded as other work: GCREX-44/decision-1 interrupt bestEffort hunks (main.go opInterrupt, dispatch_test.go interrupt rows, REX-CONTRACT/parity interrupt prose, provider-method-audit/interrupt.md, gcrex-44 task md, decision-1), AGENTS.md, the GCREX-37/39 doc corpus, and .pi/.gc runtime state.

Verification: go test -count=1 ./... all packages ok; golangci-lint run ./... 0 issues.

Major finding (must be fixed before commit):
1. docs/provider-herdr-parity.md "Summary of deviations" protocol row still says "2 baseline tokens only" and "Contract forbids non-baseline tokens". Both are false after this change: the handshake now declares three tokens (report-attachment, report-activity, idle-boundary), and §9 permits a non-baseline token in the same change that implements the behavior it gates. Update the summary row to three tokens and the gated-declaration rule. The detailed §protocol section and the extension section are now correct; the summary table is the remaining contradiction.

Minor (follow-ups):
2. docs/provider-herdr-parity.md is-attached still says "The handshake declares report-attachment (and report-activity)" and get-last-activity still says "Capability report-activity is declared"; both omit idle-boundary. Update for completeness.
3. internal/provider/idle.go's busyMarkers comment still claims the pane "classifies like the tmux pane gc reads" while omitting the claudeBusySpinnerRe and claudeBareGerundSpinnerRe clauses; either port the regexes or narrow the comment. This was minor #4 in the first review and remains open.
4. docs/herdr-extensions.md "Deadline class" says the idle read is capped at gc's probe deadline rather than the 30 s default, but opIdle does not set a per-op deadline; align the doc with the code or set the deadline.

Verdict: fail.

Addressed review findings (iteration 2; docs/comment only, no behavior change this pass). (1) docs/provider-herdr-parity.md 'Summary of deviations' protocol row (was the remaining contradiction): it now records 3 handshake tokens (report-attachment, report-activity baseline, plus idle-boundary) and replaces 'Contract forbids non-baseline tokens' with the gated-declaration rule ('a non-baseline token only in the change that implements the behavior it gates'). (2) The is-attached and get-last-activity implementation paragraphs now list idle-boundary alongside the other declared tokens instead of omitting it. (3) Narrowed the busyMarkers comment in internal/provider/idle.go: it no longer claims a Rex pane classifies like the tmux pane; it states the three literal clauses are carried, gc's claudeBusySpinnerRe/claudeBareGerundSpinnerRe are not ported, and a spinner-only pane with no program_status record reads indeterminate (the safe direction). (4) Aligned docs/herdr-extensions.md idle 'Deadline class' with the code: opIdle sets no per-op deadline, so gc's 30 s exec default applies (a probe class would fit, and a consuming gc applies its own deadline when it wires SnapshotIdle). Verification: gofmt clean; go test ./... all packages ok; golangci-lint run ./... 0 issues.

Independent review (openrouter/deepseek/deepseek-v4.1-flash:high), re-review after iteration 2's addressed findings. Scope: GCREX-63's uncommitted change only: internal/provider/idle.go, internal/provider/idle_test.go, cmd/gc-runtime-rex/idle_test.go, the main.go import/const/handshake/ops/opIdle hunks, the two TestProtocolHandshake lines in dispatch_test.go, and the GCREX-63 doc hunks. Excluded as other work: the GCREX-44/decision-1 interrupt bestEffort hunks (main.go opInterrupt, dispatch_test.go interrupt rows, REX-CONTRACT/parity interrupt prose, provider-method-audit/interrupt.md, gcrex-44 task md, decision-1), AGENTS.md, the GCREX-37/39 doc corpus, and .pi/.gc runtime state.

Iteration-2 findings verified fixed: (1) parity "Summary of deviations" protocol row now reads 3 tokens and the gated-declaration rule; (2) is-attached, get-last-activity, exec, attach and server-stop list idle-boundary; (3) the idle.go busyMarkers comment no longer overclaims: it states the three literal clauses carried, the two unported gc spinner regexes, and the indeterminate reading for a spinner-only pane; (4) herdr-extensions "Deadline class" records the 30 s default and that opIdle sets no per-op deadline. gc-side cites re-checked against gc working tree 890a58a and tag v1.5.1: protocol.go:60, exec/idle.go:36-40 (idleBoundaryDeclared needs report-activity, report-attachment, carrier.ops|proc.exec and idle-boundary), runtime.go:757, exec/cutover.go:46-48, execution_backstop.go:493, program_status_shadow_fresh.go:100-101, idle_nudge.go:670-673, tmux/adapter.go:760, tmux.go:5868-5883, tmux.go:6240-6246. v1.5.1 declares exactly the six baseline tokens and ignores unknown strings (protocol.go, handshake_test.go:91-102); it also already has runtime.IdleSnapshotProvider (runtime.go:304) and tmux SnapshotIdle (adapter.go:473) but no snapshotHoldFor/exec backstop, so the docs' working-tree labels for the backstop cites are correct.

Verification: go test -count=1 ./... all packages ok; golangci-lint run ./... 0 issues; gofmt -l clean.

Major finding (must be fixed before commit):
1. README.md:130-136 and docs/rex-provider.md:207-214 still document the pre-change handshake and op set, and are now false. README "RPP capabilities" prints `{"version":0,"capabilities":["report-attachment","report-activity"]}` and an implemented-operations list without `idle`; rex-provider.md "Protocol and Capabilities" repeats the same JSON, "Declared capabilities: report-attachment (is-attached), report-activity (get-last-activity)", and the same op list. main.go:46 now declares `idle-boundary` and main.go:175-193 registers `idle <name>`. This is the same defect class the first two reviews failed on in the parity doc; fix both files in this change: add `idle-boundary` to both JSON literals, add it to rex-provider.md's declared-capabilities bullet, add `idle` to both op lists, and extend the staging caveat's GCREX-63 hunk list with README.md and docs/rex-provider.md. (The untracked GCREX-37/39 corpus - docs/provider-method-audit/protocol.md and docs/provider-herdr-method-deviations.md - also still says 2 capabilities, but it is excluded from this commit and belongs to that task.)

Minor (follow-ups):
2. Line-cite drift in the very sections this change edits (pre-existing; refresh while there): parity exec section `main.go:74-79` (unimplemented is now 161-165), `main.go:81-97` (ops is now 175-193), `main.go:115-123` (unknown-op branch is now ~230-235); server-stop `main.go:112-122` (same branch); is-attached `main.go:93` (now 189) and `main.go:286-293` (`opIsAttached` now 540); get-last-activity `main.go:303-312` (`opGetLastActivity` now 517; 303-312 is the outcome encoder); attach `main.go:205-213` (`opAttach` now 379) and `main.go:322` (`opIsAttached` now 540); and the parity cites `docs/rex-provider.md:119-123`, `:123`, `:125`, `:334` (those lines hold GC_SESSION prose; the capability bullets are at 207-214).
3. An untracked 5.5 MB `gc-runtime-rex` Mach-O build artifact sits in the checkout root, is not gitignored, and is not in the staging caveat. The finish step must not stage it; delete it or list it as excluded.
4. Cosmetic: four consecutive blank lines before "## Rex-specific extension" (docs/provider-herdr-parity.md:1085-1090).

Verdict: fail.

Addressed review findings (iteration 3; docs + cleanup, no behavior change this pass).

Major #1 (README.md and docs/rex-provider.md documented the pre-change handshake and op set). Both now list the third token and the new op: the JSON literal is {"version":0,"capabilities":["report-attachment","report-activity","idle-boundary"]} (README.md:133, docs/rex-provider.md:209); rex-provider.md's declared-capabilities bullet now adds `idle-boundary` (`idle`) (docs/rex-provider.md:212); both op lists add `idle` (README.md:136, docs/rex-provider.md:215).

Minor #2 (line-cite drift in the sections this change edits). docs/provider-herdr-parity.md: the exec and provision paragraphs now cite main.go:161-165 (unimplemented), main.go:175-193 (ops) and main.go:228-235 (unknown-op branch); server-stop cites main.go:175-193/161-165/228-235; is-attached cites main.go:189 and opIsAttached at main.go:540; get-last-activity cites main.go:517; attach cites opAttach at main.go:379 and opIsAttached at main.go:540. The stale docs/rex-provider.md:119-123, :123, :125, :334 cites are corrected to :212-213 (the capability bullets).

Minor #3 (untracked 5.5 MB Mach-O artifact in the checkout root). Deleted the root-level `gc-runtime-rex` (an accidental `go build ./cmd/gc-runtime-rex` output; the Makefile target writes bin/gc-runtime-rex, which is untouched). git status now has no untracked gc-runtime-rex.

Minor #4 (cosmetic). docs/provider-herdr-parity.md: four consecutive blank lines before "## Rex-specific extension" reduced to one.

Staging caveat (supersedes the earlier one): the finish commit must exclude the GCREX-44/decision-1 hunks in both code and docs - main.go opInterrupt, dispatch_test.go interrupt rows, the interrupt bestEffort prose in docs/REX-CONTRACT.md and docs/provider-herdr-parity.md, docs/provider-method-audit/interrupt.md, the gcrex-44 task md, backlog/decisions/, AGENTS.md, the GCREX-37/39 doc corpus, and .pi/.gc runtime state. GCREX-63's own hunks are internal/provider/idle.go, internal/provider/idle_test.go, cmd/gc-runtime-rex/idle_test.go, the main.go handshake/const/import/ops/opIdle hunks, the two TestProtocolHandshake lines in dispatch_test.go, README.md, docs/rex-provider.md, the REX-CONTRACT section 9 idle notes, the herdr-extensions idle row/design plus Fact 2, and the parity protocol fixes plus the "Rex-specific extension: the idle snapshot" section.

Verification: gofmt -l clean; go test -count=1 ./... all packages ok; golangci-lint run ./... 0 issues.

Independent review (openrouter/deepseek/deepseek-v4.1-flash:high), re-review after iteration 3's addressed findings. Scope: GCREX-63's uncommitted change only — internal/provider/idle.go and idle_test.go (new), cmd/gc-runtime-rex/idle_test.go (new), the main.go handshake/const/comment/ops/opIdle hunks, the two TestProtocolHandshake lines in dispatch_test.go, README.md, docs/rex-provider.md, the REX-CONTRACT §9 idle notes, the herdr-extensions idle row/design plus Fact 2, and the parity protocol fixes plus the "Rex-specific extension: the idle snapshot" section. Excluded as other work: the GCREX-44/decision-1 interrupt bestEffort hunks (main.go opInterrupt, dispatch_test.go interrupt rows, the REX-CONTRACT/parity interrupt prose, provider-method-audit/interrupt.md, the gcrex-44 task md, decision-1), AGENTS.md, the GCREX-37/39 doc corpus, and .pi/.gc runtime state.

Iteration-3 findings verified fixed: (1) README.md:133/136 and docs/rex-provider.md:209/212/215 now list the three-token handshake (idle-boundary) and the idle op; (2) the parity cite drift in the sections this change edits is refreshed — spot-checked against the file at main.go:46 (const), :206 (protocol branch), :207 (write), :208-211 (write failure), :42-45 (comment), :161-165 (unimplemented), :175-193 (ops), :228-235 (unknown-op), :189 (is-attached row), :379 (opAttach), :517 (opGetLastActivity), :540 (opIsAttached); (3) the 5.5 MB root gc-runtime-rex artifact is deleted; (4) the four blank lines before "## Rex-specific extension" are one.

Verification this pass: gofmt -l clean; go test -count=1 ./... all packages ok; golangci-lint run ./... 0 issues. Named tests green: TestIdleSnapshotFromProgramStatus (idle/working/blocked/done/error), TestIdleSnapshotFallsBackToThePane (ready/alt-ready/busy/cancel/startup-dialog/unrecognised/empty), TestIdleSnapshotFailedReadsAreErrors (missing session, program-status failure, pane failure), TestIdleOpTriState, TestIdleOpFailedReadIsAnError, TestProtocolHandshake. AC #1 holds (the op+token seam and the would-be gc consumer are recorded in both docs). AC #2/#3 hold: the tri-state is explicit on the wire and in IdleState, every state and every failed-read path has a named subtest, and no path prints a state on failure (the provider returns the error; opIdle exits 1). AC #4 holds: idle-boundary is declared in the same change as the op, no other capability moved, and at the pin v1.5.1 internal/runtime/protocol.go declares only the six baseline strings while TestProtocol_UnknownCapabilitiesIgnored pins ignoring unknown strings. AC #5 verified above. No code defects found in idle.go, idle_test.go, the dispatch idle_test.go or opIdle: fresh resolve, a missing session is ErrSessionNotFound, program-status read then BlockFormat fallback, errors never idle, no per-op deadline, no stdin. gc working-tree (890a58a) cites re-checked: protocol.go:60, exec/idle.go:36, runtime.go:757, exec/cutover.go:46-48 (seamBackedProvider has no IdleSnapshotProvider), execution_backstop.go:493 + :498, program_status_shadow_fresh.go:101, idle_nudge.go:673, tmux/adapter.go:760, tmux.go:5868, tmux.go:6240-6251 (idle.go's three literal busy clauses are exact and its two unported claude spinner regexes are declared).

Major finding (must be fixed before commit):
1. docs/REX-CONTRACT.md:530, §12 "Delivery and handshake (resolved, GCREX-5)", still says "The handshake declares version 0, report-attachment and report-activity, and no locality field" (lines 535-536). That is false after main.go:46, and it is the same defect class iterations 1-3 failed on (parity §protocol, then the parity summary row, then README/rex-provider). Update the sentence to the three tokens (idle-boundary declared alongside the idle op it gates) and extend the staging caveat with the §12 hunk.

Minor (follow-ups):
2. The parity sections this change does not touch still carry their pre-existing cite drift (it predates this change: ops was already at main.go:173 at HEAD while they cite :81-97): observe :962 (main.go:81-97, :74-79, :116-123, :83-95), inventory :987/:995/:996, kill-object :1015/:1023, copy-to :1067/:1075, watch-startup :807/:815, and the start/is-running/process-alive/list-running/peek/nudge/send-keys/clear-scrollback rows. Refresh in a doc pass or record as a follow-up.
3. The consumer cite cmd/gc/idle_nudge.go:673 is the turnHoldFor fallback call; the SnapshotIdle read is execution_backstop.go:498 (inside :493). Keep as the consumer path or cite :498.
4. internal/provider/idle.go's readyPromptPrefixes comment says the glyphs are what "gc lists for agent prompts": ❯ is gc's verified default (tmux.go:5738) and the ›/> pair comes from this repo's readiness.go (readiness.py's PROMPT_GLYPHS, :70-79), not from the gc checkout. Cite readiness.go or label the provenance.

Verdict: fail.

Addressed review findings (iteration 4; docs + comment only, no behavior change this pass). Major #1: docs/REX-CONTRACT.md section 12 "Delivery and handshake" still described the pre-change two-token handshake; the sentence now reads three tokens - report-attachment, report-activity and idle-boundary (the last outside the 1.5.1 baseline, declared only in the same change as the idle op it gates, GCREX-63) - with no locality field. Minor #3: the gc consumer path in the herdr-extensions idle design and the parity "Rex-specific extension" section now names the SnapshotIdle read at cmd/gc/execution_backstop.go:498 in addition to snapshotHoldFor at :493. Minor #4: internal/provider/idle.go's readyPromptPrefixes comment now cites provenance - the first glyph is gc's default tmux idle prefix (tmux.go:5738, defaultReadyPromptPrefix) and the second/third pair is this provider's startupPromptGlyphs (readiness.go:68-73, readiness.py's PROMPT_GLYPHS) - instead of claiming gc lists them for agent prompts. Minor #2 (pre-existing cite drift in the parity sections this change does not touch) is recorded as a follow-up, not fixed here. Verification: gofmt -l clean; go test ./... all packages ok; golangci-lint run ./... 0 issues.

Staging caveat (supersedes the iteration-3 one): the finish commit must exclude the GCREX-44/decision-1 hunks in both code and docs - main.go opInterrupt, dispatch_test.go interrupt rows, the interrupt bestEffort prose in docs/REX-CONTRACT.md and docs/provider-herdr-parity.md, docs/provider-method-audit/interrupt.md, the gcrex-44 task md, backlog/decisions/, AGENTS.md, the GCREX-37/39 doc corpus, and .pi/.gc runtime state. GCREX-63's own hunks are internal/provider/idle.go, internal/provider/idle_test.go, cmd/gc-runtime-rex/idle_test.go, the main.go handshake/const/import/ops/opIdle hunks, the two TestProtocolHandshake lines in dispatch_test.go, README.md, docs/rex-provider.md, the REX-CONTRACT section 9 and section 12 idle notes, the herdr-extensions idle row/design plus Fact 2, and the parity protocol fixes plus the "Rex-specific extension: the idle snapshot" section.

work (re-run 2026-11-19T12:02:05Z): continued the existing uncommitted GCREX-63 change from the orphaned run rather than redoing it; no code change this pass. Verified: go build ./... clean; go test -count=1 ./... all packages ok; golangci-lint run ./... 0 issues. Named tests green: TestIdleSnapshotFromProgramStatus, TestIdleSnapshotFallsBackToThePane, TestIdleSnapshotFailedReadsAreErrors (internal/provider), TestIdleOpTriState, TestIdleOpFailedReadIsAnError, TestProtocolHandshake (cmd/gc-runtime-rex). Change left uncommitted for review-loop/finish.

Independent review (openrouter/deepseek/deepseek-v4.1-flash:high), confirmation pass after iteration 4's fix. Scope: GCREX-63's uncommitted change only — internal/provider/idle.go and idle_test.go (new), cmd/gc-runtime-rex/idle_test.go (new), the main.go handshake/const/comment/ops/opIdle hunks, the two TestProtocolHandshake lines in dispatch_test.go, README.md, docs/rex-provider.md, the REX-CONTRACT §9 and §12 idle notes, the herdr-extensions idle row/design plus Fact 2, and the parity protocol fixes plus the 'Rex-specific extension: the idle snapshot' section. Excluded as other work: the GCREX-44/decision-1 interrupt bestEffort hunks (main.go opInterrupt, dispatch_test.go interrupt rows, the REX-CONTRACT/parity interrupt prose, provider-method-audit/interrupt.md, the gcrex-44 task md, decision-1), AGENTS.md, the GCREX-37/39 doc corpus, and .pi/.gc runtime state.

Iteration 4's major finding is fixed: docs/REX-CONTRACT.md §12 'Delivery and handshake' now records the three-token handshake (report-attachment, report-activity, idle-boundary — the last outside the 1.5.1 baseline and declared only in the same change as the idle op it gates, GCREX-63) with no locality field. Re-checked the other prior findings: README.md:133/136 and docs/rex-provider.md:209/212/215 list the third token and the idle op; the parity summary protocol row (line 89) records 3 tokens and the gated-declaration rule; herdr-extensions Fact 2 carries the exception; the REX-CONTRACT §9 heading/rule and its closing sentence separate idle state reporting from op gating; the idle design names the SnapshotIdle read at cmd/gc/execution_backstop.go:498.

Verification this pass: go build ./... clean; go test -count=1 ./... all packages ok; golangci-lint run ./... 0 issues; gofmt -l clean. Named tests green: TestIdleSnapshotFromProgramStatus (idle/working/blocked/done/error), TestIdleSnapshotFallsBackToThePane (ready/alt-ready/busy/cancel/startup-dialog/unrecognised/empty), TestIdleSnapshotFailedReadsAreErrors (missing session, program-status failure, pane failure), TestIdleOpTriState (five states), TestIdleOpFailedReadIsAnError (missing session, program-status failure), TestProtocolHandshake. gc-side claims re-checked against the pinned tag v1.5.1 (protocol.go declares exactly the six baseline tokens; unknown strings ignored) and the working tree (protocol.go:60 idle-boundary; exec/cutover.go:46-48 seamBackedProvider has no IdleSnapshotProvider; exec/idle.go:36 idleBoundaryDeclared needs report-activity, report-attachment, carrier.ops|proc.exec and idle-boundary; execution_backstop.go:493/:498 snapshotHoldFor).

ACs: #1 the op+token seam and the would-be gc consumer are recorded in the parity extension section and herdr-extensions. #2 tri-state explicit; a failed read is an error, never idle (idle.go, opIdle exit 1, the named failed-read tests). #3 derived from program_status records and the pane read peek already uses, with named per-state tests. #4 idle-boundary is declared in the same change as the op; v1.5.1 declares only the six baseline strings and ignores unknown tokens, and on the edge tree idleBoundaryDeclared still requires carrier.ops or proc.exec, so it is inert. #5 verified above. No code defects found in idle.go, idle_test.go, the dispatch idle_test.go or opIdle.

Minor (follow-ups, not blocking): the pre-existing cite drift in the parity sections this change does not touch (observe, inventory, kill-object, copy-to, watch-startup and the start/is-running/process-alive/list-running/peek/nudge/send-keys/clear-scrollback rows) remains; refresh in a doc pass. internal/provider/idle.go's busyMarkers omit gc's claudeBusySpinnerRe/claudeBareGerundSpinnerRe, so a spinner-only pane with no program_status record reads indeterminate where tmux reads busy — the safe direction, and program_status is primary.

Verdict: pass.
<!-- SECTION:NOTES:END -->

## Comments

<!-- COMMENTS:BEGIN -->
author: @mc-wisp-wy713s
created: 2026-11-19T09:49:00Z

---

Claimed by Gas City worker mc-wisp-wy713s (seat rex-gascity/pi, session pi-mc-wisp-wy713s, template rex-gascity/pi); workflow bead mc-13vh, step bead mc-26bt.

---

author: @mc-wisp-wy713s
created: 2026-11-19T09:54:00Z

---

Scoped the design to an RPP op gated by the existing idle-boundary token rather than a new token, because gc's working tree already defines idle-boundary as the idle-observation gate (internal/runtime/protocol.go:60) and implements WaitForIdle under it. Declaring it is inert until a carrier op lands (GCREX-28.2), per idleBoundaryDeclared.

---

author: @boris
created: 2026-11-19T11:57:00Z

---

Reset from In Progress to To Do (operator decision, @boris): the run was orphaned, not abandoned work.

What happened: worker mc-wisp-wy713s claimed this at ~09:49 CET (run root mc-13vh). The work step finished (mc-26bt closed 10:56) and the review-loop ran all four iterations, but the formula's [steps.check] max_attempts = 4 was exhausted while the verdict file still read 'fail' (iteration 4's address fixed the last finding - REX-CONTRACT section 12 still described the two-token handshake - but no fifth review ran to confirm it). The body scope then aborted (gc.outcome=fail) and finish (the only committing step) was skipped (gc.outcome=skipped). The worker was killed and drained at 10:56:41, so no session owns this task and it sat In Progress for ~2h, which gated off the rex-backlog order and blocked GCREX-64.

The implementation is probably already done and only needs the review to be continued:
- Uncommitted in the rig checkout: internal/provider/idle.go + idle_test.go (new), cmd/gc-runtime-rex/idle_test.go (new), the main.go handshake/const/import/ops/opIdle hunks, the two TestProtocolHandshake lines in dispatch_test.go, and the GCREX-63 doc hunks (README.md, docs/rex-provider.md, REX-CONTRACT sections 9 and 12, herdr-extensions idle row/design + Fact 2, parity protocol fixes + the 'Rex-specific extension: the idle snapshot' section).
- Last review (openrouter/deepseek/deepseek-v4.1-flash:high, iteration 4) found no code defects in idle.go/opIdle, verified all five ACs, and left only the section 12 doc sentence as major; that fix is already applied in the working tree. go build ./... clean; idle/handshake tests pass.
- Next worker: inspect git status --short and git diff HEAD first and CONTINUE from the existing change rather than redoing it, then run one more review to confirm and let finish commit.

Staging caveat for whoever finishes this: the finish commit must exclude the GCREX-44/decision-1 interrupt bestEffort hunks - main.go opInterrupt (bestEffort(p.Interrupt(...))), the dispatch_test.go interrupt rows, the interrupt prose in REX-CONTRACT.md and docs/provider-herdr-parity.md, docs/provider-method-audit/interrupt.md, the gcrex-44 task md, backlog/decisions/, AGENTS.md, the GCREX-37/39 doc corpus, and .pi/.gc runtime state.

---

author: @mc-wisp-hd1cwp
created: 2026-11-19T12:02:00Z

---

Claimed by Gas City worker mc-wisp-hd1cwp (seat rex-gascity/pi-1, session pi-mc-wisp-hd1cwp, template rex-gascity/pi); workflow bead mc-r1ef, step bead mc-f2p1. Continuing the existing uncommitted GCREX-63 change from the orphaned run.

---
<!-- COMMENTS:END -->
