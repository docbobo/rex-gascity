# Requirements: move stop/start policy out of provider.go (#92)

This spec turns the approved design (`brainstorming/design.md`, approach B,
approved by `gcrex-62r0q9`) into requirements for the plan. Code facts are
stated at base commit `d97988f`. They still hold on `main` at `c9894e9`:
`git diff --stat d97988f c9894e9 -- internal/` is empty. The `REQ-NNN` ids in
this spec are local to it.
The parity-doc stop-lock conflict owned by #53 is called "the #53 stop-lock
conflict" here. That avoids confusing it with any id in this list.

## Problem Statement

`internal/provider/provider.go` (1173 lines at `d97988f`) mixes orchestration
with policy:

- `start` (`provider.go:465-516`) calls nine helpers in a row. Two of them are
  name-lifecycle policy (`reapStartSurvivors`, `resolveStartTarget`). Four are
  work-dir preparation (`stageStartFiles`, `runPreStartPhase`,
  `requireWorkDir`, `stagePackOverlayPhase`). All of these helpers are defined
  in `provider.go`, along with `startPlan`.
- `stop` (`provider.go:738-779`) and `stopUnderPrimaryLock` (`:719`) hold
  Stop's decision tree on top of `retire`.
- Three steps of `retire` and the reap (`destroyEmptyCitySession` `:788`,
  `reapSurvivors` `:825`, `clearIncarnations` `:854`) and the hook helper
  `preStartCwd` (`:1166`) are defined in `provider.go`. Their owners
  (`retire.go`, `reap.go`, `hooks.go`) do not hold them.

The issue was written at `e6ebf9f`, and part of it is out of date. The
pre_start loop moved to `hooks.go` (ADR 0005). #143 removed the legacy-label
hints. The design's "issue is partly outdated" table records both. What is
left is the file layout. The behavior does not change.

## W6H

- **Who:** maintainers and agents who read or change the provider's Start/Stop
  path. Reviewers use the result to check lock order and step order.
- **What:** a no-behavior-change refactor. It moves 13 functions out of
  `provider.go` into files named for what they own. It adds two thin entry
  points (`recycleName`, `prepareWorkDir`). It records the layout decision in
  a new ADR (REQ-014). It re-points the `provider.go:N` citations in the living
  docs.
- **When:** now. Blocker #90 closed on 2026-11-25 (`c4be65e`). #136 also edits
  `provider.go`, and whichever PR lands second rebases.
- **Where:** `internal/provider/` (`provider.go`, new `lifecycle.go`, new
  `startprep.go`, appends to `retire.go`, `reap.go`, `hooks.go`),
  the new ADR at the path `make adr` prints (REQ-014), and the living docs
  listed in REQ-015.
- **Why:** `provider.go` should read as orchestration. The policy should live
  in modules with one caller-facing entry each, following the `retire.go`
  model (ADR 0003).
- **Which:** approach B of the design. Approaches A (fold into
  `retire.go`/`launch_pane.go`) and C (separate types or a sub-package) are
  rejected, and the new ADR (REQ-014) records why.
- **How:** verbatim moves, shown as moves by `git diff --color-moved`. The
  existing tests are the proof. Characterization tests are added first for any
  `prepareWorkDir` order constraint that no existing test pins.

## User Stories

- As a maintainer reading `start`, I see five named steps in order:
  `startPlan`, `recycleName`, `findOrCreateCitySession`, `prepareWorkDir`,
  `launchPane`. I do not have to read nine helper calls.
- As a maintainer changing how Stop retires a name, I find the whole decision
  tree in `lifecycle.go`. `retire.go` holds only the step executor and its
  steps.
- As a reviewer of this PR, I can confirm that no body changed by reading
  `git diff --color-moved=zebra d97988f`. I can confirm that no behavior
  changed because the named tests stay green.
- As a future reviewer who wants to fold the policy into `retire.go` or split it
  into types, I find the new ADR's (REQ-014) `## Alternatives rejected` section
  and its reasons.
- As a reader of the parity and method docs, each `provider.go:N` citation
  still lands on the code its sentence names.

## Technical Stories

- **TS-1 Baseline:** on `d97988f`, run every test named in REQ-011 and record
  them green. Record the baseline `provider.go` line count. Do this before any
  move.
- **TS-2 Order coverage first:** for each `prepareWorkDir` order constraint
  in REQ-012, find an existing test that fails when the order is broken. If
  none exists, add a characterization test through `Provider.Start` with the
  fakes, on the branch, before the move. Each proof test, existing or added,
  is green on unchanged code. Show it red by temporarily swapping the two
  calls, record the red run, then revert the swap.
- **TS-3 Lifecycle module:** create `lifecycle.go`. Move `stopUnderPrimaryLock`,
  `stop`, `reapStartSurvivors` and `resolveStartTarget` verbatim. Add
  `recycleName`.
- **TS-4 Start-preparation module:** create `startprep.go`. Move `startPlan`,
  `stageStartFiles`, `runPreStartPhase`, `requireWorkDir` and
  `stagePackOverlayPhase` verbatim. Add `prepareWorkDir`.
- **TS-5 Helper homes:** append `destroyEmptyCitySession` and
  `clearIncarnations` to `retire.go`, `reapSurvivors` to `reap.go`, and
  `preStartCwd` to `hooks.go`.
- **TS-6 Rewire `start`** to the five-step shape in REQ-006. Prune the imports
  `provider.go` no longer uses.
- **TS-7 ADR:** on the implementation branch, run
  `make adr TITLE="Provider policy modules are files of Provider methods with one entry point"`.
  Record the path it prints in the task notes, and fill in that file. Do not
  rename it to a fixed number (REQ-014).
- **TS-8 Citation re-point:** update the `provider.go:N` citations in the living
  docs. Start from the docs on the implementation branch's base, not from
  `d97988f`. Since `d97988f`, #176 and #177 edited
  `docs/provider-herdr-parity.md`, replacing five lines one for one, and
  added one row at line 8 of `docs/provider-method-audit/start.md`, which
  shifts that file's later lines by one.
- **TS-9 Spec mirror:** commit this spec without front matter at
  `docs/superpowers/specs/2026-11-26-provider-policy-modules-design.md` on the
  implementation branch. This lane does not commit.
- **TS-10 Gates and delivery:** run `make test`, `golangci-lint run ./...` and
  `pack/tests/verify.sh`. Re-run the duplicate-PR check for #92. Push the
  branch and open the PR.

## Behavior Requirements

- **REQ-001 Functions leave `provider.go`.** After the change, `provider.go`
  defines none of these 13 functions. Each one is defined in the file listed:

  | Function | Destination |
  | --- | --- |
  | `stopUnderPrimaryLock`, `stop`, `reapStartSurvivors`, `resolveStartTarget` | `lifecycle.go` (new) |
  | `startPlan`, `stageStartFiles`, `runPreStartPhase`, `requireWorkDir`, `stagePackOverlayPhase` | `startprep.go` (new) |
  | `destroyEmptyCitySession`, `clearIncarnations` | `retire.go` (appended after the existing code) |
  | `reapSurvivors` | `reap.go` (appended) |
  | `preStartCwd` | `hooks.go` (appended) |

  All of them stay unexported methods or functions in package `provider`. No
  call site changes, with one exception: the rewire of `start` in REQ-006. It
  replaces the direct calls at `provider.go:473-502` with one call to
  `recycleName` and one to `prepareWorkDir`. The moved signatures and bodies
  do not change (REQ-007).

- **REQ-002 `lifecycle.go` holds the name lifecycle.** It holds the four moved
  functions and one new method, `recycleName`. It holds no other new
  declaration. Stop's four branches stay in `stop` in today's order:
  1. Rex is unavailable and no binding is stored: clear the incarnations.
  2. Any other `Resolve` error: return it.
  3. No pane: write the drain diagnostic when a binding was stored, then
     `retire(stopNoPanePolicy(hadStored))`.
  4. A pane: `retire(stopPolicy())`.

  No classifier function is extracted. The public `Stop` method stays in
  `provider.go`. It is unchanged and still wraps `stopUnderPrimaryLock` in
  `withNameLock`.

- **REQ-003 `recycleName` contract.** Signature:
  `func (p *Provider) recycleName(ctx context.Context, n SessionName, cfg Config) (relaunch, done bool, err error)`.
  It calls `p.reapStartSurvivors(ctx, n)`. If that returns an error, it returns
  `false, false, err` with the error unwrapped. Otherwise it returns
  `p.resolveStartTarget(ctx, n, cfg)` unchanged. Its doc comment carries the
  reason now given at `provider.go:469-472`. A kept binding is the only name for
  a survivor once its pane is gone. `ResolveFresh` drops that record, so the reap
  must finish first or the pid runs on with no record.

- **REQ-004 `startprep.go` holds start preparation.** It holds the five moved
  functions and one new method, `prepareWorkDir`. It holds no other new
  declaration.

- **REQ-005 `prepareWorkDir` contract.** Signature:
  `func (p *Provider) prepareWorkDir(ctx context.Context, plan launch.Plan, cfg Config) error`.
  It runs these steps in order and stops at the first error, which it returns
  unwrapped:
  1. `stageStartFiles(cfg)`
  2. `runPreStartPhase(ctx, plan, cfg)`
  3. `requireWorkDir(cfg.WorkDir)`
  4. `stagePackOverlayPhase(cfg)`

  It takes no new lock. It runs under the per-name lock that its caller
  holds: both `Start` and `StartNew` call `start` inside `withNameLock`
  (`provider.go:364-370`, `:386-410`). It runs outside the city-wide state
  lock (`runPreStartPhase` comment, `provider.go:589-590`). Parity doc
  `### start` row 12 (intentional) agrees. It says the state flock "never"
  covers pre_start or the launch.

  Its doc comment states the three order constraints, each with the reason
  given in the existing helper comments:
  - Staging runs before pre_start, so a hook sees the staged work_dir
    (`stageStartFiles` comment).
  - The work_dir check runs after pre_start, because pre_start may create the
    work_dir (`requireWorkDir` comment).
  - Pack overlays are staged after pre_start, because the per-provider subtree
    overwrites a managed file the hook may have rewritten (`stageStartFiles` and
    `stagePackOverlayPhase` comments).

  The doc comment also says that the city-wide state lock is not held, so a
  slow hook does not stall the city (`runPreStartPhase` comment). It does not
  say that no lock is held: the caller's per-name lock is held.

- **REQ-006 `start` shape.** `start` keeps its signature and does these steps
  in order:
  1. `n := SessionName(name)`
  2. `plan, attempt := p.startPlan(name, &cfg)`
  3. `relaunch, done, err := p.recycleName(ctx, n, cfg)`: return
     `startResult{}, err` on error, and `startResult{}, nil` when `done`
  4. `sid, err := p.findOrCreateCitySession(ctx)`: return on error
  5. `p.prepareWorkDir(ctx, plan, cfg)`: return on error
  6. return `p.launchPane(ctx, launchRequest{...})`, with the same
     `launchRequest` field values as today, including
     `tab: tabLabel(n, cfg.Env, p.townTab)`

  `start` calls no other method. `tabLabel` inside the `launchRequest` literal
  is an argument expression, not a step. The inline comment above the reap
  (`:469-472`) moves into `recycleName`'s doc comment (REQ-003). The
  "Find or create the city session" and "Seed identity, launch the pane"
  comments stay in `start`.

- **REQ-007 Verbatim moves and minimal edits to existing files.**
  - For each moved function, the bytes from the first line of its doc comment
    through its closing brace are identical to `d97988f`.
  - `git diff --color-moved=zebra d97988f` shows those blocks as moved.
  - In `retire.go`, `reap.go` and `hooks.go`, the only changes are the appended
    functions and the import lines those functions need. No other existing line
    changes. For example, `reapSurvivors` uses `errors.Join`, and `reap.go` does
    not import `errors` at `d97988f`, so that import is added.
  - The existing `retire.go:N` citations in ADR 0003 and the specs stay valid.
  - The new files contain only a package clause, imports, an optional file
    comment of at most one paragraph naming the module's single entry point, the
    moved functions, and the new entry point.
  - `provider.go` loses only the moved functions and the imports it no longer
    uses, and `start` changes as REQ-006 says.

- **REQ-008 No behavior change.** These all stay as they are:
  - every step order in Start and Stop
  - every lock: which locks are taken, their order and their scope
  - every error string and wrap, including
    `start %s: reap the pending survivors and retry: %w`
  - every `errors.Is` target
  - the binding each path passes

  `retire`, `retirePolicy`, the five policy constructors and
  `TestRetirePolicyTable` are unchanged (ADR 0003). The constraints the
  retirement spec put out of scope still hold:
  - `reapStartSurvivors` is not folded into `startDeadPanePolicy`.
  - Stop's Rex-unavailable branch does not move into `retire`.

  Any behavior change found on the way is filed as a separate issue and is not
  made here.

- **REQ-009 What stays in `provider.go`.** These stay where they are:
  - `newBinding`, `nextSplit` and `rectArea` (#136 is about to edit
    `newBinding`)
  - the public `Stop` and `Start`, `StartNew` and its fresh_only and
    already-running guard
  - `runSessionSetup`, `failIfAgentEnded` and `deliverStartupNudge`
  - the `Provider` type and its options

- **REQ-010 The #53 stop-lock conflict is carried, not resolved.**
  `destroyEmptyCitySession`'s doc comment moves verbatim, including "The caller
  holds the state lock" (`provider.go:782`). No lock changes. The PR body
  repeats both readings:
  - Code: `Stop` does not hold the city-wide state lock
    (`provider.go:702-705`). It takes the per-name locks (`provider.go:706-718`,
    `lock.go`). On the destroy path it also tries the city session's launch
    lock (`retire.go:299-302`, `provider.go:792-803`). Do not describe this as
    "only name locks".
  - Parity doc: the `### stop` row 5 (intentional) says the state-dir-wide
    flock is held for the store mutations, including the reap.

  The PR body names #53 as the owner.

- **REQ-011 Existing proof tests stay green before and after.** These tests
  pass on `d97988f` and after the change. Locations are at `d97988f`.

  | Branch or step | Tests |
  | --- | --- |
  | Stop, Rex unavailable, no binding | `TestStopUnreachableWithoutBindingClearsMeta` (`stop_unreachable_test.go`), `TestStopUnreachableWithoutBindingSucceeds` (`teardown_test.go`) |
  | Stop, Rex unavailable, alias binding answers | `TestStopUnreachableAliasBindingKeepsError` (`stop_unreachable_binding_test.go`) |
  | Stop, no pane, stored binding | `TestStopWritesDiagnosticWhenPaneIsAlreadyGone` (`drain_diag_test.go`), `TestStopWithoutPaneClearsAllMetaAndActivity` (`incarnation_test.go`), `TestStopNoPaneKeepsBindingWhenDestroyFails` (`teardown_test.go`) |
  | Stop, unknown name | `TestStopUnknownSessionIsANoOp`, `TestStopIsIdempotentForUnknownSession` (`provider_test.go`) |
  | Stop, pane | the `Provider.Stop` tests in `reap_internal_test.go`, `TestStopToleratesVanishedPane`, `TestStopPropagatesOtherCloseErrors` (`teardown_test.go`), `TestStopRemovesEmptyCitySession` (`teardown_spy_test.go`) |
  | Start, survivors swept before resolve | `TestStartFailsWhilePendingSurvivorsSurvive` (`review_round4_internal_test.go`) |
  | Start, dead pane recycled | `TestStartAfterExitRelaunchesInNewTab`, `TestStartAfterExitRelaunchesAsSplitWhenRigTabRemains` (`teardown_test.go`), `TestStartNewRelaunchesDeadPane` (`phase4_test.go`), `TestStartDropsTheRetiredIdentityWhenTheRelaunchFails` (`identity_test.go`) |
  | Start, idempotent live pane | `TestStartTwiceCreatesNoDuplicatePane` (`teardown_test.go`) |
  | Start, pre_start before launch | `TestPreStartCommandsRunBeforeLaunch` (`prestart_test.go`), a smoke test only |

  `TestPreStartCommandsRunBeforeLaunch` checks files and launch records only
  after `Start` returns (`prestart_test.go:16-45`). It is not proof that
  preparation runs before the launch. That proof is the call-order check in
  AC-3.

  If the plan finds a moved branch that no test pins, it records the branch in
  the task notes and the PR body as a follow-up. It does not add a test for
  that branch in this change. The three REQ-012 order checks are the only new
  tests this change may add.

- **REQ-012 Order coverage for `prepareWorkDir`.** Each of these constraints
  has a named test that fails when the two steps swap:
  - (a) staging before pre_start
  - (b) the work_dir check after pre_start
  - (c) pack overlay staging after pre_start

  An existing test counts if a swap makes it fail. For each constraint that has
  no such test, a characterization test is added through `Provider.Start` with
  the fakes, before the move. Every proof test, existing or added, is green on
  unchanged code. For each of the three constraints, the task notes record a
  red run with the two calls swapped (AC-7). The swap is reverted before
  commit. No test is added that cannot fail.

  Candidates to check first, each with the swap, before adding a test:
  - (a) `TestStartStagesBeforePreStart` (`startconfig_test.go:145-173`)
  - (b) `TestPreStartMayCreateWorkDir` (`prestart_test.go:369-397`)
  - (c) none known. `TestStagePackOverlaysStagesConfiguredSubtree`
    (`pack_overlay_staging_test.go:14-46`) calls the staging helper directly.
    It does not check when staging runs relative to a hook, so it does not
    count.

- **REQ-013 No test is deleted or rewritten.** `retire_internal_test.go`,
  `reap_internal_test.go` and `launch_pane_internal_test.go` compile and pass
  unchanged. The issue's suggestion to turn most internal tests into
  `Provider.Stop` tests is not adopted, for the reasons in design D5.
  - Of the 33 tests in `retire_internal_test.go`, 27 pin retire presets that
    stop never uses.
  - 9 of the 12 tests in `reap_internal_test.go` already drive `Provider.Stop`.

  The only test changes allowed are:
  - the additions from REQ-012
  - citation-only edits to test-file comments under REQ-015, such as
    `pane_label_test.go:359-362`, where the new target is accurate. No code
    line in a test file changes.

- **REQ-014 Layout ADR.** The ADR is created on the implementation branch with
  `make adr TITLE="Provider policy modules are files of Provider methods with one entry point"`
  and lands in the same PR. Its path is the one `make adr` prints there. Do not
  fix the number in advance. `scripts/new-adr.sh:32-45` takes one more than
  the highest `NNNN-*.md` number in `docs/adr/`, and other PRs keep taking
  numbers. The approved design calls this ADR "ADR 0008" (`design.md:343`,
  `:400`). 0008 was free at `d97988f`, but on `main` at `c9894e9` 0008 and
  0009 record other decisions (`docs/adr/README.md:41-42`), so `make adr`
  assigns 0010 or higher. `d97988f` stays the reference for the verbatim
  moves (REQ-007). The ADR records approach B. Its `## Alternatives rejected`
  section names two alternatives:
  - A: fold the policy into `retire.go` and `launch_pane.go`. Rejected because
    it mixes the executor with its callers (ADR 0003) and blurs
    `launch_pane.go`'s lock regime.
  - C: separate types or a sub-package. Rejected because the types would need
    most of `Provider`, it is a larger diff, and `New` plus the fakes already
    give a seam.

- **REQ-015 Citation re-point, no verdict change.** Each `provider.go:N`
  citation in these living docs lands on the code its sentence names after the
  change. A citation to a moved function points at its new file and line.
  - `docs/provider-herdr-parity.md`
  - `docs/provider-herdr-method-deviations.md`
  - `docs/provider-herdr-method-io.md`
  - `docs/provider-method-audit/*.md`
  - the Go comments under `internal/` that cite `provider.go:N`

  This includes citations that had already drifted at `d97988f`. Only
  locations change. No parity verdict changes, and neither does any
  method-I/O or deviation text.

  A citation is re-pointed only when the claim its sentence makes has a
  current target. Some unchanged sentences make a claim the current code no
  longer matches. For those, the citation is left as it is. Each one is
  recorded in the task notes and the PR body as a follow-up, with the
  discrepancy. Do not cite unrelated code to fill the gap. Do not change the
  sentence or the verdict. Example: `docs/provider-herdr-parity.md:350`
  (`### start` row 6, intentional) says recycle "closes the dead Rex block
  only" and cites `provider.go:186`. The current policy also clears the start
  identity and forgets the record (`provider.go:548-570`,
  `retire.go:187-204`). Historical records are not re-cited:
  `docs/superpowers/specs/*`, `docs/superpowers/plans/*`, and ADRs other than
  one whose cited lines this change shifts. `docs/REX-RETIRE.md` and
  `internal/provider/AGENTS.md` need no edit.

- **REQ-016 Gates and delivery.**
  - `make test`, `golangci-lint run ./...` (pre-commit hook enabled with
    `make hooks`) and `pack/tests/verify.sh` pass.
  - No file under any `overlay` directory is added, edited, moved or deleted.
    `stagePackOverlayPhase` is Go code that stages overlays. Moving it is not
    an overlay change.
  - The spec mirror from TS-9 is committed.
  - The duplicate-PR check for #92 is re-run before the PR opens.
  - The branch is pushed and a PR is opened. The PR body:
    - cites the proof tests (REQ-011, REQ-012)
    - gives the two readings of the #53 stop-lock conflict (REQ-010)
    - names #136 and #53 as related
    - lists the follow-ups: moved branches no test pins (REQ-011) and
      citations left unmatched, each with its discrepancy (REQ-015). If a
      list is empty, it says "none".

## Example Mapping

| Story | Rule | Example |
| --- | --- | --- |
| Stop, Rex down, no binding | The branch stays in `stop`, unchanged | `TestStopUnreachableWithoutBindingClearsMeta` passes. Stop returns nil and meta/activity are cleared, as on `d97988f` |
| Stop, pane gone, stored binding | Diagnostic, then `retire(stopNoPanePolicy(true))` | `TestStopWritesDiagnosticWhenPaneIsAlreadyGone` passes unchanged |
| Start with an unreaped survivor | `recycleName` reaps before it resolves and returns the error unwrapped | `TestStartFailsWhilePendingSurvivorsSurvive` sees the same `reap the pending survivors and retry` error |
| Start on a live pane | `done` returns before staging or launch | `TestStartTwiceCreatesNoDuplicatePane` passes. `prepareWorkDir` is not reached |
| Start, pre_start creates work_dir | The work_dir check runs after pre_start | A test where only pre_start creates the work_dir starts the pane. With `requireWorkDir` swapped before `runPreStartPhase`, it fails (REQ-012 b) |
| Start, staged file seen by hook | Staging before pre_start | A pre_start hook that reads a staged `copy_files` entry succeeds. With the two calls swapped, it fails (REQ-012 a) |
| Start, pack overlay vs hook rewrite | Overlay staging after pre_start | A hook rewrites a managed file, and the pack overlay version wins. With the two calls swapped, the hook's version wins and the test fails (REQ-012 c) |
| Reviewer checks the moves | Verbatim | `git diff --color-moved=zebra d97988f` shows each moved function as a moved block. `reap.go`'s diff is one import line plus the appended `reapSurvivors` |
| Parity citation | Location only | A parity row that cites `provider.go:738-779` for `stop` now cites the line range of `stop` in `lifecycle.go` after the move. The verdict cell is unchanged |
| Stale parity citation | Re-point only when the claim has a current target | `### start` row 6 cites `provider.go:186` for "closes the dead Rex block only". No current code makes that claim, so the citation is unchanged. The follow-up list records it with the discrepancy (identity clear and forget, `retire.go:187-204`) |
| Uncovered moved branch | Record, do not test | The plan finds a Stop branch no REQ-011 test pins. It goes in the follow-up list. No test is added for it |

## Acceptance Criteria

- **AC-1 (REQ-001, REQ-002, REQ-004, REQ-009):**
  `grep -n '^func' internal/provider/provider.go` lists none of the 13 moved
  functions. Each is defined in its REQ-001 destination. `Stop`, `newBinding`,
  `nextSplit`, `rectArea` and the REQ-009 functions are still in
  `provider.go`. `lifecycle.go` and `startprep.go` each declare exactly one
  function that is not a moved one: `recycleName` and `prepareWorkDir`.
- **AC-2 (REQ-003, REQ-005):** `recycleName` and `prepareWorkDir` have the
  signatures, step order, unwrapped error returns and doc-comment content that
  REQ-003 and REQ-005 give. `prepareWorkDir` takes no new lock. It runs under
  the caller's per-name lock and outside the city-wide state lock. Its doc
  comment says the state lock is not held. It does not say that no lock is
  held.
- **AC-3 (REQ-006):** `start`'s body is the six steps of REQ-006, in that order,
  with no other method call. The `launchRequest` fields are unchanged.
- **AC-4 (REQ-007):** each moved function, from its doc comment through its
  closing brace, is byte-identical to `d97988f`. The check extracts each block
  from both trees and compares them, and the task notes record the result. In
  the `retire.go`, `reap.go` and `hooks.go` diffs, every line is either an added
  import or appended after the last line at `d97988f`.
- **AC-5 (REQ-008, REQ-010):** the diff adds, removes and reorders no
  `Lock`/`Unlock`/`with*Lock` call. `retirePolicy`, its five constructors,
  `retire` and `TestRetirePolicyTable` are unchanged. `destroyEmptyCitySession`'s
  doc comment still contains "The caller holds the state lock".
- **AC-6 (REQ-011, REQ-013):** every REQ-011 test passes on `d97988f` and on
  the branch, and the task notes record both runs. No existing test file loses
  or changes a code line. The only changed lines are the citation-only comment
  edits that REQ-013 allows.
- **AC-7 (REQ-012):** constraints (a), (b) and (c) are each pinned by a named
  test. For each of the three, whether an existing test or an added one gives
  the proof, the task notes record a green run on unchanged code, a red run
  with the swap, and the revert.
- **AC-8 (REQ-014):** the ADR file at the path `make adr` printed on the
  implementation branch exists, and the task notes record that path. It is the
  only ADR the PR adds, and its title is the REQ-014 title. Its
  `## Alternatives rejected` section names approaches A and C with their
  reasons.
- **AC-9 (REQ-015):** each `provider.go:N` citation in the REQ-015 docs lands on
  the code its sentence names, or it appears in the task notes and PR body's
  follow-up list with its discrepancy (REQ-015). `git diff` of the parity docs
  changes only location text, never a verdict cell.
- **AC-10 (REQ-016):** `make test`, `golangci-lint run ./...` and
  `pack/tests/verify.sh` pass. `git diff --name-only d97988f` lists no path
  under an `overlay` directory. The spec mirror is committed. The branch is
  pushed and a PR is open. Its body has the REQ-016 contents.

## Out Of Scope

- Any change to `retire`, `retirePolicy`, the policy constructors or
  `TestRetirePolicyTable`.
- Folding `reapStartSurvivors` into the dead-pane retire policy. Moving Stop's
  Rex-unavailable branch into `retire`. Changing any lock.
- Moving `newBinding`, `nextSplit`, `rectArea`, `StartNew`'s guard,
  `runSessionSetup`, `failIfAgentEnded` or `deliverStartupNudge`.
- Resolving the #53 stop-lock conflict. Refreshing the stale parity `### stop`
  rows 1-2 and the `### kill-object` text. #53 owns all three.
- Re-citing historical specs, plans and unaffected ADRs.
- Converting `retire_internal_test.go` to `Provider.Stop` tests.
- New tests other than the three REQ-012 order checks. Uncovered moved
  branches are follow-ups (REQ-011).
- Rewording a parity, method-I/O or deviation sentence whose claim no longer
  matches the code. Its citation is a follow-up (REQ-015).
- A new sub-package, a new type, or new exported API.
- Any behavior change found during the move. That is filed as a separate issue.
- Any overlay file.

## Open Questions

None blocking. The approval lane recorded zero unresolved questions. This
self-review settled these readings, so they are not open:

- **Import lines in "append-only" files.** `reapSurvivors` needs `errors`, and
  `reap.go` does not import it at `d97988f`. REQ-007 allows the import lines
  that appended code needs and nothing else.
- **"Nothing else" in `start`.** The step list does not count the
  `tabLabel(...)` argument in the `launchRequest` literal. The reap-ordering
  comment moves to `recycleName`. The other two comments stay (REQ-006).
- **New-file headers.** A package clause, imports and an optional one-paragraph
  file comment are allowed (REQ-007).
- **Lock wording.** "No lock" would be wrong: `start` runs under the
  per-name lock. The spec says "no new lock, outside the city-wide state
  lock" (REQ-005), and "does not hold the state lock" for Stop (REQ-010).
- **Test-file citations.** Citation-only comment edits in test files are
  allowed, rather than excluding test files from REQ-015. REQ-015 already
  covers Go comments under `internal/` (REQ-013, AC-6).
- **Uncovered branches vs. new tests.** The REQ-012 limit wins. Uncovered
  branches are follow-ups (REQ-011).
- **ADR number.** The spec does not fix one. The design's "ADR 0008" was the
  next free number at `d97988f`. The ADR takes the path `make adr` prints on
  the implementation branch (REQ-014, AC-8).
- **Spec mirror.** This lane does not write into the shared `main` working tree,
  which has untracked files. The mirror is committed on the implementation
  branch (TS-9).

Non-blocking sequencing note: #136 and this change both edit `provider.go`. The
two do not block each other, and whichever lands second rebases. Keeping
`newBinding` in place limits the overlap to `start`'s body.

### Claims relied on (two-source check)

| Claim | Source 1 (code) | Source 2 | Result |
| --- | --- | --- | --- |
| Stop of an absent name returns nil | `provider.go:753-761` | gc v1.5.1 `internal/runtime/runtime.go:163-164`, "Returns nil if the session does not exist (idempotent)". Re-read in this lane with `git show v1.5.1:` | verified, unchanged |
| A pre_start failure does not stop the start; only cancellation aborts | `provider.go:584-590` (`runPreStartPhase` comment) | gc v1.5.1 `docs/reference/exec-session-provider.md:328-333`, "Non-fatal: warn on stderr if a command fails, but don't abort start". Re-read in this lane | verified, unchanged |
| Stop does not hold the city-wide state lock. It takes the per-name locks, and the destroy path also tries the launch lock | `provider.go:702-718`, `lock.go`, `retire.go:299-302`, `provider.go:792-803` | parity doc `### stop` row 5 (intentional) says the state-dir-wide flock is held for the store mutations | conflict, carried to #53 (REQ-010) |
| Start preparation runs under the caller's per-name lock and outside the state lock | `provider.go:364-370`, `:386-410`, `:589-590` | parity doc `### start` row 12 (intentional): the state flock "never" covers pre_start or the launch | verified, unchanged |
| Retirement step order and presets | `retire.go`, `TestRetirePolicyTable` | ADR 0003 | verified, untouched |

## Coverage

| ID | Status |
| --- | --- |
| REQ-001 | covered |
| REQ-002 | covered |
| REQ-003 | covered |
| REQ-004 | covered |
| REQ-005 | covered |
| REQ-006 | covered |
| REQ-007 | covered |
| REQ-008 | covered |
| REQ-009 | covered |
| REQ-010 | covered |
| REQ-011 | covered |
| REQ-012 | covered |
| REQ-013 | covered |
| REQ-014 | covered |
| REQ-015 | covered |
| REQ-016 | covered |
