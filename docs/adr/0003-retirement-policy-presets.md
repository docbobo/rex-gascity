# 0003. Retirement is configured by named presets, not by policy fields

- Status: Accepted
- Date: 2026-11-26 (backfilled; the decision predates it)

## Context

`retire()` runs one fixed order: diagnostic sample, pre-close snapshot, close,
reap, clear, destroy, forget (`internal/provider/retire.go:222`). Callers differ
in which steps run and in their scope. Five call paths needed five shapes:
a plain stop, a stop whose pane was already gone, a stale-record drop, a
launch-record discard, and a start whose pane died.

The order itself is stated and fixed in `retire()`
(`internal/provider/retire.go:206-222`), and ADR-0016 records why `stop` closes
the pane on the retire path. This record is about the shape of the knob, not the
order of the steps.

## Decision

`retirePolicy` is unexported (`internal/provider/retire.go:59`). Its fields can
only switch a step off or pick that step's scope; nothing in it reorders the
steps. Policies are built only by the named constructors, never as literals at a
call site (`internal/provider/retire.go:55-58`):

- `stopPolicy()` — `internal/provider/retire.go:113`
- `stopNoPanePolicy(hadStored bool)` — `internal/provider/retire.go:140`
- `stalePolicy()` — `internal/provider/retire.go:161`
- `discardLaunchPolicy()` — `internal/provider/retire.go:179`
- `startDeadPanePolicy()` — `internal/provider/retire.go:198`

`TestRetirePolicyTable` pins every named constructor's fields to the expected
table (`internal/provider/retire_internal_test.go:226`), so changing what a
preset does is a visible change in a test, not a silent one at a call site.

## Consequences

A caller picks a shape by name; it cannot compose a retirement order the fixed
sequence was not written for. The variation stays at five audited shapes.

Adding a shape means adding a constructor and a row to the policy table. That is
the intended friction: a sixth shape should be a deliberate choice, because each
one is a different set of steps that can leave a pane or a record behind.

## Alternatives rejected

- **Export the fields and let call sites set them.** Then every field is part of
  the interface, and a call site composes a new sequence the fixed order was
  never checked against.
- **Functional options at the call site.** Same problem with more machinery: the
  policy becomes an argument list instead of a name.
- **One policy for all callers.** The five paths exist because the steps that are
  safe to run differ. A stop whose pane is already gone must not read a process
  tree; a launch-record discard must not destroy the city session.
- **Expose a flag per step as parameters on `retire()`.** The nine fields of
  `retirePolicy` spelled out at each call site is what the named constructors
  replaced.
