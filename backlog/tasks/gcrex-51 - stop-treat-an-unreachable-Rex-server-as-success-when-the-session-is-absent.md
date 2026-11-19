---
id: GCREX-51
title: "stop: treat an unreachable Rex server as success when the session is absent"
status: In Progress
assignee: []
created_date: "2026-11-18T22:05:00Z"
updated_date: '2026-11-19T20:53:00Z'
labels:
  - provider
  - herdr
  - tdd
milestone: m-0
dependencies: []
references:
  - docs/provider-method-audit/stop.md
documentation:
  - docs/provider-herdr-method-io.md
priority: medium
type: task
ordinal: 107000
---
## Description

<!-- SECTION:DESCRIPTION:BEGIN -->

Develop test-first (TDD): write the failing test that reproduces this first, watch it go red, then the smallest change that makes it green (repo rule in AGENTS.md; see also the tdd skill).

Avoids: stop fails with exit 1 for a session that is already gone.

The document (## stop § Success output, § Failure cases) exits 0 when the core is down and nothing of it runs. The implementation fails whenever the Rex server is unreachable, even for an absent or already-dead session.

<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria

<!-- AC:BEGIN -->

- [ ] #1 With Rex unreachable and no binding for the name, stop exits 0.
- [ ] #2 With Rex unreachable and a binding present, the current exit 1 is kept and the reason is recorded in the task notes.
- [ ] #3 Tests cover both branches.
- [x] #4 `make test` and `golangci-lint run ./...` pass.
- [x] #5 `docs/provider-herdr-parity.md` and `docs/provider-method-audit/stop.md` reflect the new verdict for the unreachable-server row, checked against the parity doc and gc `v1.5.1`.
<!-- AC:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->

Why a binding present keeps exit 1: with Rex down, `stop` cannot close the pane, and a stored binding is the only evidence a pane may still be live. Exit 0 there would hide a possible leak, and gc would not retry. herdr-core covers this with a pid/token sweep; Rex has no equivalent, so the error is returned and the binding, meta and activity stay for a retry. "Binding present" is `Binder.lookup` (`internal/provider/binding.go:197`: exact name or alias, local stores only), read before `Resolve` so a record `resolveFresh` would drop still counts.

Proving tests (slices gcrex-3nz3 and gcrex-d7uh):

- No binding, exit 0: `TestStopUnreachableWithoutBindingSucceeds` (`internal/provider/teardown_test.go`), `TestStopUnreachableWithoutBindingClearsMeta` (`internal/provider/stop_unreachable_test.go`, meta and activity cleared), `TestStopUnreachableRexUnknownNameExitsZero` (`cmd/gc-runtime-rex/dispatch_test.go`, empty stdout and stderr).
- Binding present, exit 1: `TestStopUnreachableWithBindingKeepsError` (`teardown_test.go`), `TestStopUnreachableAliasBindingKeepsError` (`stop_unreachable_test.go`). Both inject `SessionViewErr`, not `ListSessionsErr` alone: `revalidate` uses `SessionView`, and a successful empty view drops the record before `scan`.

Accepted residual: a lost binding record with a still-live pane reports success, because the label scan that could find the pane needs Rex.

Docs (slice 3, gcrex-ckts): `docs/provider-method-audit/stop.md` "Core/server unreachable" row (severity medium to low) and `docs/provider-herdr-parity.md` `### stop` deviation row 8 plus the summary row. Both sources re-read: herdr-core `lifecycle.py:518-522,528` (`op_stop` falls back to `_stop_without_core` only when `HcUnreachable` and `nothing_runs`), gc `v1.5.1` `internal/runtime/exec/exec.go:262-270` (backstop `p.Stop` at `:266`, a stop error is joined into the start error) and `:581-583` (`Stop` returns the op error). No conflict between them.

AC #1 to #3 are ticked when slices 1 and 2 land; the docs rows above already describe their behavior.

<!-- SECTION:NOTES:END -->
