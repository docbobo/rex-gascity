# 0021. Recover a dialog-withheld seed inside Start

- Status: Accepted
- Date: 2026-11-27

## Context

Issue #205: a Pi mayor's startup seed was withheld because the readiness
check matched a startup dialog (`deliverStartupNudge`,
`internal/provider/nudge_delivery.go`). Nothing delivered it afterwards. gc's
exec `Start` never re-sends the seed (gc v1.5.1
`internal/runtime/exec/exec.go:211-231`). It stamps `primed_at` from its plan
and does not read `GC_STARTUP_DELIVERY_UNCONFIRMED`. Its queued-nudge
backstop covers pool slots only (gc edge `cmd/gc/idle_nudge.go:88-136`). Under
RPP v0 the exec script owns the seed, and `start` stays welded with one process
per op (ADR-0009). The check also read the whole scrollback, while gc peeks 120
lines (gc v1.5.1 `internal/runtime/dialog.go:18`).

## Decision

We recover inside `start`. After the first dialog sighting, Start keeps polling
and never types. It polls for a bounded clear window (15 s,
`WithStartupDialogClear`) measured from that sighting. Its Rex calls stop at a
hard deadline plus one poll. It delivers through the existing ladder only after
a poll with a clear screen tail and a ready verdict. A startup-only dialog gate
reads the tail before the first write, every submit key and each resend.
Otherwise the seed stays undelivered, the marker names the dialog, the timing
file keeps the screen tail, and `start` exits 0. The startup path reads only
the last 120 lines.

## Consequences

A dialog that clears by itself, or a stale match in scrollback, no longer
strands the seed. A persistent dialog caps the clear-and-ready phase at the
window plus one poll. That cap covers the phase's Rex calls only. The marker
and timing-file writes run after it, so they add their own delay.
gc's 120 s `startTimeout` is the separate outer limit for the whole script.

A dialog that only gc answers after `start` returns still leaves the seed
undelivered while gc reports primed. That needs an upstream gc change. A seed
that quotes a dialog marker can be left typed but unsubmitted with a
suspected-dialog marker. Ordinary nudges are unchanged.

## Alternatives rejected

- **B. A detached deliverer that outlives `start`.** It would also cover
  dialogs gc answers later, but it needs process lifecycle, locking and
  identity-race handling with no precedent here. It races gc's own dialog
  pass, works against ADR-0009's welded `start` and the process-per-op model,
  and is hard to test against the fake.
- **C. Fail `start` when the seed is withheld.** gc's
  `cleanupAfterStartFailure` stops and respawns a healthy pane (gc v1.5.1
  `exec.go:223-228,262-270`). The same dialog can return on every try, which
  causes a respawn storm.
- **D. Trust an `idle` program status over the screen.** This reopens
  GCREX-59, where a TUI reported idle while a trust dialog owned input and
  Enter picked an option. It weakens the safety rule for every
  status-reporting app.
- **Deliver on the readiness bound after a sighting.** That types without a
  ready signal into a pane that just showed a dialog. After a sighting only a
  clear read plus a ready verdict delivers.
- **Wait past the clear window's deadline.** It would make `start`'s duration
  unbounded by our own clock, and only gc's `startTimeout` would stop it. That
  kills the script and triggers cleanup. The deadline poll is the last poll.
