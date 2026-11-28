# 0023. One pane reading per Observe

- Status: Accepted
- Date: 2026-11-28

## Context

`Observe` (`internal/provider/observe.go`) answered `observe.v1` from two Rex
`BlockProcess` reads of the same pane. `boxState` made the first read and
classified it in `readPaneLife` (`internal/provider/panelive.go`). Then, for a
running box with gc process names, `Observe` called `p.foregroundPID`, which
made a second read of the same block just to learn the agent root PID.

Two readings of one pane can disagree. An agent that exits between the reads
yields `state=running` paired with a root from a different reading (or none),
which is a self-contradictory liveness answer. The second read also spends the
4 s observe probe budget (`observeProbeBound`) on a fact the first read already
held: `paneLife.foregroundPID()` is in the same `ProcessInfo` the state was
classified from.

The previous test, `TestObserveAgentRootFromFreshRead`, pinned the opposite
contract: it sequenced foreground 100 then 200 and asserted two `process` reads
with `agent_pid=200`. That test is why this decision needs a record; without
it the next reader re-proposes the fresh read.

## Decision

We make `boxState` return the `paneLife` reading it classified, carried with
the state and the binding in a `boxRead` struct, and we take the running
agent's root PID from that same reading (`read.life.foregroundPID()`). A bound
running box now costs exactly one `BlockProcess` call per `Observe`, and the
box state and agent root cannot come from different readings.

On the gone-resolve path `boxState` still reads twice: it probes the recorded
block and, when that block is gone, reads the live replacement pane found under
the record's labels. It returns the replacement pane's reading, because that is
the reading for the pane whose state and root are answered. The one-read rule
is about one pane, not about the stale-record probe plus the replacement pane.

The classification and error policy stay in `readPaneLife` alone. `paneUnknown`
still answers `unknown`, never `dead` (ADR 0002).

## Consequences

- Observe makes one Rex `process` read for a running box, so it spends less of
  its probe budget and adds less avoidable load to the shared Rex server.
- The reported `agent`/`agent_pid` are at most one read older than the `state`,
  never from a later, different reading. gc consumes `agent` as a liveness
  verdict for the state it reports, so a root newer than that state bought
  nothing.
- `boxState`'s signature changed to `(boxRead, error)`. The compiler surfaced
  every caller; `Inventory` and `killObjectLocked` take `.state`/`.holder` and
  ignore `.life`, with no change to their read counts or behavior.
- `p.foregroundPID` stays because `deadAgent` (kill-object's zombie walk) still
  uses it; the fresh read there is intentional, since kill-object re-checks the
  live root after a stop.
- RPP-OBSERVE-006 permitting one reading for both state and root is
  **unverified** against gc source: the pinned gc checkout (`$GASCITY_SRC`,
  default `/Volumes/Sources/gascity`) is absent and `GASCITY_SRC` is unset on
  this host. The in-repo conformance run is the executable check available
  here; on 2026-11-28 it passed (`ok: true`, 51 passed, 0 failed, 24 skipped,
  `v2_ready: true`) against gc `edge-20261124`.

## Alternatives rejected

- **Keep the second fresh read so the root is newest.** Rejected: two readings
  of one pane can disagree (an agent exiting between reads yields `running`
  plus a root from a different reading); the second read spends the observe
  budget; gc consumes `agent` as a liveness verdict for the state it reports,
  so a newer root than the state buys nothing. This is the alternative
  `TestObserveAgentRootFromFreshRead` used to justify, so it is recorded here.
- **Read the root first, then the state.** Rejected: it is the same duplicate
  read with the same disagreement window, only with the roles reversed.
- **Return a four-value tuple `(state, holder, life, error)`.** Rejected: a
  caller can silently drop the reading, which is exactly the invariant this
  change protects. A struct keeps state and its reading inseparable.
