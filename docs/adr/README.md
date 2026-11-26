# Architecture decision records

One file per decision a future reader would otherwise re-open: what we chose, why,
and — in `## Alternatives rejected` — what we turned down. That last section is
the point. It is what stops the next architecture review, human or agent, from
proposing the same refactor again.

Not every change needs one. Write an ADR when a decision has a rejected
alternative someone would plausibly propose again. Skip the ones that would only
restate what the code already says.

## Writing one

```sh
make adr TITLE="the pane lock is per session"
```

That writes `docs/adr/NNNN-the-pane-lock-is-per-session.md` from `template.md`
and prints its path. Fill it in, then add its row below.

- Statuses: `Proposed`, `Accepted`, `Superseded by ADR-NNNN`, `Rejected`.
- Date the decision, not the backfill. Mark `(backfilled)` when the two differ.
- Keep it to about a page. Link the longer standing documents
  (`docs/REX-RETIRE.md`, `docs/provider-herdr-parity.md`) rather than copying
  them.

An ADR records a decision, not an instruction. The rules an agent must follow
live in the `AGENTS.md` files.

## Index

| # | decision | status |
| --- | --- | --- |
| [0001](0001-ui-status-row-framing.md) | The UI keeps a second row format for the status probe | Accepted |
| [0002](0002-liveness-unknown-is-not-dead.md) | A liveness read that fails answers `unknown`, never `dead` | Accepted |
| [0003](0003-retirement-policy-presets.md) | Retirement is configured by named presets, not by policy fields | Accepted |
| [0004](0004-binder-start-env.md) | `Binder` derives its labels from the projected start env | Accepted |
| [0005](0005-route-provider-hooks-through-the-runner-seam.md) | Provider hooks run through the `Runner` seam | Accepted |
