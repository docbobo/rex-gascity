# 0007. Guard opSpec method

- Status: Accepted
- Date: 2026-11-26

## Context

The `ops` table (`cmd/gc-runtime-rex/main.go:196`) hand-keeps a `method` string
per row (`opSpec.method`, `cmd/gc-runtime-rex/main.go:176`). The dispatcher
copies that string into every trace line
(`cmd/gc-runtime-rex/main.go:270`). Nothing tied the string to the provider
call the handler makes.

The pair drifted. The `nudge` row said `"Nudge"` while `opNudge` calls
`NudgeReport`. Trace lines from `nudge` recorded a method no provider call used,
and no test failed. `TestE2ETraceStartAndPeek` used it as the red proof: at
`ad101aa` the line read `op/method "nudge"/"Nudge"`.

A second hand-kept copy has the same shape. `startConfig`
(`cmd/gc-runtime-rex/main.go:151`) mirrors `provider.Config` field by field, and
`opStart` copies the fields across again
(`cmd/gc-runtime-rex/main.go:400-440`). At `ad101aa` the pair was in sync and
nothing kept it that way. A dropped field took the zero value without a failure.

## Decision

We guard both copies with table tests and keep the production table literal.

- `TestOpSpecMethodMatchesHandler` (`cmd/gc-runtime-rex/opspec_test.go:45`)
  walks every `ops` row with a recording fake provider. It fails when a row
  names a method its handler does not call, and when a row with no method does
  call one. `opFixtures` must cover the table in both directions.
- `TestStartConfigParity` (`cmd/gc-runtime-rex/startconfig_test.go:144`) runs
  `opStart` with every `startConfig` field set and checks both directions over
  `reflect`. Every `provider.Config` field must be non-zero, so it has a source.
  Every `startConfig` field must reach a `provider.Config` field, apart from
  `process_names` and `command`, which feed `seedProcessNames` and `Argv`.
- `TestE2ETraceStartAndPeek` (`cmd/gc-runtime-rex/trace_e2e_test.go:52`) pins
  the end-to-end result for `nudge`: `op "nudge"` and `method "NudgeReport"`.

The dispatch path stays unreflected and keeps literal strings.

## Consequences

- A new op needs a fixture. A wrong method fails in CI with the row name.
- A new `startConfig` field needs a `provider.Config` source, or an explicit
  allowlist entry with a reason. Dropping a mapping fails by field name.
- The `nudge` trace line now reports `NudgeReport`, as REQ-003 asks.
- The tests run in process. They add no dependency and no allocation to the
  dispatch path. `go.mod` and `go.sum` stay untouched.
- The two structs share field names by construction, so the parity test compares
  names. A rename on both sides that keeps the mapping correct still passes,
  which is the intended result.

## Alternatives rejected

- Derive `method` from the handler with reflection in the dispatch path. It
  allocates on every traced op, returns an untyped string, and contradicts
  REQ-006, which forbids reflection in production. A 21-row literal table plus
  a test gives the same guarantee at no dispatch cost.
- Make each handler return its method name. It is invasive across 21 handlers
  for one string, and it still does not prove the handler invoked that provider
  call. The recording fake stays, so the churn buys nothing.
- Read the method from the provider client after the call. The client returns
  results, not method names. Adding it to the interface grows the seam for a
  value the table already holds.
