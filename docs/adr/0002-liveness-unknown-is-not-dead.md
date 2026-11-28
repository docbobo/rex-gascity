# 0002. A liveness read that fails answers `unknown`, never `dead`

- Status: Accepted
- Date: 2026-11-26 (backfilled; the decision predates it)

## Context

`readPaneLife` is the provider's one `BlockProcess` call
(`internal/provider/panelive.go:39`, `:49`). The Binder, the Provider, reap,
observe and the liveness ops all read a pane's program through it:
`internal/provider/provider.go:900`, `internal/provider/binding.go:345`,
`internal/provider/reap.go:86`, `internal/provider/observe.go:230`, `:247`,
`:259`, `internal/provider/liveness.go:131`, `:150`.

Rex answers `ErrNotFound` for a block it no longer knows, and fails with
`ErrUnavailable`, a lost reply, or the op's deadline for everything else. gc
reads a provider error as a negative answer.

## Decision

`readPaneLife` classifies one read into four states: `paneUnknown`, `paneGone`,
`paneExited`, `paneRunning` (`internal/provider/panelive.go:10-25`).

- `rex.ErrNotFound` is `paneGone`.
- Any other error is `paneUnknown`, and keeps its cause so `errors.Is` still
  matches `rex.ErrUnavailable` or `context.DeadlineExceeded`
  (`internal/provider/panelive.go:50-54`).
- A successful read is `paneExited` when `LastExit` is set and `paneRunning`
  otherwise (`internal/provider/panelive.go:55-58`).

An error never becomes `paneExited`. `paneUnknown` is the zero value, so a
reading nobody made is never mistaken for an answer.

The error policy lives in that one function and nowhere else
(`internal/provider/panelive.go:40-41`). Callers restate that they reuse it
rather than re-deriving it: `internal/provider/liveness.go:129` and
`internal/provider/binding.go:343`.

## Consequences

A transient Rex failure cannot be reported as a dead agent, which is what stops
the reconciler from reaping a live one. `internal/provider/AGENTS.md:32` states
the rule: "The provider answers `unknown`, never `dead`, when it cannot be sure
of agent liveness (`agentsnapshot.go`)."

The cost is that every caller has three states to handle, not two. `alive()`
returns `(false, err)` for `paneUnknown` rather than `(false, nil)`
(`internal/provider/panelive.go:71-80`), so the distinction reaches the caller.

The second source is the parity doc, which records the same policy for its
sibling ops: a missing pane answers `false, nil` from `ErrNotFound` for
`process-alive` ([parity doc:129](https://github.com/docbobo/rex-gascity/blob/09f11b8/docs/provider-herdr-parity.md#L129)), and an unclassifiable
client kind answers `true` for `is-attached` because "we cannot tell"
([parity doc:142](https://github.com/docbobo/rex-gascity/blob/09f11b8/docs/provider-herdr-parity.md#L142)).

The parity document is retired by
[ADR-0011](0011-verify-the-provider-against-gc-and-conformance-not-herdr-core-parity.md);
the links above are pinned to the pre-deletion commit `09f11b8` and are
historical evidence only.

## Alternatives rejected

- **Fold `unknown` into not alive** — a two-valued `alive() (bool, error)` that
  answers `false, nil` for a failed read. The reconciler then treats an
  unreachable Rex as a dead pane and reaps a live agent.
- **Answer `dead` for `ErrNotFound` as well as for other errors.** `ErrNotFound`
  is Rex saying the block is gone, which is a fact. A lost reply is not.
- **Read liveness per op site.** That is where the error policies drifted before
  the single reader; four call sites classify errors four ways.
- **Merge the read with `panestate.go`.** The two answer different questions:
  `paneLife` is whether the program still runs, `panestate` is what the screen
  shows (`internal/provider/panelive.go:27-30`).
