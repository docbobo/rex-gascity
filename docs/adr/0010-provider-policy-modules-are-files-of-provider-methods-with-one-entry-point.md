# 0010. Provider policy modules are files of Provider methods with one entry point

- Status: Accepted
- Date: 2026-11-27

## Context

At `d97988f`, `internal/provider/provider.go` was 1173 lines. It mixed
orchestration with policy:

- `start` called nine helpers in a row. Two were name-lifecycle policy:
  `reapStartSurvivors` and `resolveStartTarget`. Four were work-dir
  preparation: `stageStartFiles`, `runPreStartPhase`, `requireWorkDir` and
  `stagePackOverlayPhase`.
- `stopUnderPrimaryLock` and `stop` held Stop's lock order and its decision
  tree, on top of `retire`.
- Two steps of `retire` (`destroyEmptyCitySession`, `clearIncarnations`), the
  reap mechanism (`reapSurvivors`) and the hook helper (`preStartCwd`) lived in
  `provider.go`, not in the files that own those concerns.

ADR 0003 set the model for policy in this package. `retire.go` exposes one
entry point, `retire`, with named presets. Its callers keep the decision that
picks a preset.

## Decision

We split the provider's stop/start policy into files of `Provider` methods in
package `provider`. Each file has one entry point per caller:

- `lifecycle.go` holds the name lifecycle. `start` enters through
  `recycleName`, which reaps an earlier incarnation's survivors before
  `ResolveFresh` and then resolves the record. `Stop` enters through
  `stopUnderPrimaryLock`, which takes the primary names' locks. `stop` keeps
  its four branches in their old order. `reapStartSurvivors` and
  `resolveStartTarget` live here too.
- `startprep.go` holds start preparation. `startPlan` builds the launch plan.
  `prepareWorkDir` runs four steps in a fixed order: staging, pre_start, the
  work_dir check, then pack-overlay staging. Its doc comment states why each
  step comes where it does.
- `destroyEmptyCitySession` and `clearIncarnations` sit at the end of
  `retire.go`, `reapSurvivors` at the end of `reap.go`, and `preStartCwd` at
  the end of `hooks.go`.

`start` now reads as five steps: `startPlan`, `recycleName`,
`findOrCreateCitySession`, `prepareWorkDir`, `launchPane`. We moved every
function verbatim. No behavior, lock, error string or test changed.

## Consequences

- `provider.go` holds the `Provider` type, its options, the public methods
  and the orchestration. It went from 1173 to 863 lines.
- A change to how Stop retires a name, or how a start recycles one, goes in
  `lifecycle.go`. A change to the order of work-dir preparation goes in
  `startprep.go`. Three tests pin that order:
  - `TestStartStagesBeforePreStart`
  - `TestPreStartMayCreateWorkDir`
  - `TestStartStagesPackOverlaysAfterPreStart`
- The moved functions stay unexported methods on `Provider`. The internal
  tests compile unchanged. `New` plus the fakes stay the test seam.
- The package has two more files. A reader looking for a policy function
  starts at the entry point that `start` or `Stop` calls.
- Doc citations of the moved functions now name `lifecycle.go`,
  `startprep.go`, `retire.go`, `reap.go` or `hooks.go`.

## Alternatives rejected

- **A: fold the policy into `retire.go` and `launch_pane.go`.** This puts the
  executor and its callers in one file. ADR 0003 keeps them apart: `retire.go`
  runs a preset and its callers choose one. Start preparation in
  `launch_pane.go` would blur that file's lock regime. `launchPane` owns the
  identity seed, the launch-lock order and the rollback, and preparation runs
  before all of them, outside the state lock.
- **C: separate types or a sub-package.** A `lifecycle` or `startprep` type
  would need most of `Provider`: the binder, the Rex client, the stores, the
  name and launch locks, and the warnings writer. The diff would be larger for
  no new seam. `New` plus the fakes already give tests one.
