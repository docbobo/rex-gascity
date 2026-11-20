---
id: decision-2
title: peek answers exit 66 for a session with no screen despite gc v1.5.1
date: '2026-11-19T23:33:00Z'
status: accepted
---
## Context

`peek` today answers a session with no pane as exit 0 with empty stdout
(`internal/provider/provider.go:458-461`, `TestPeekEmptyWhenNoPane`;
`docs/provider-herdr-parity.md` `### peek`, row 1, verdict intentional). The
herdr-core reference answers the same case as exit 66 (`NO_SCREEN`) with
`no screen for '<name>'` on stderr and empty stdout
(`runtime-herdr-core/assets/gc_runtime_herdr_core/herdrcore/reads.py:364-371`
in gastownhall/gascity-packs; parity doc `### peek`, herdr-core paragraph). GCREX-48
moves our `peek` to the herdr-core answer.

## Conflict

gc v1.5.1 (tag, `6924c2c`) has no handling for exit 66. The exec provider maps
every non-zero exit except 2 to an ordinary error:

- `internal/runtime/exec/exec.go:153-170` (`runError`): exit 2 on an op other
  than `start`/`provision` is swallowed as success (`:159-160`); every other
  non-zero exit becomes `fmt.Errorf("exec provider %s %s: %s", ...)` carrying
  stderr (`:162-169`). Exit 66 therefore reaches callers as a plain error.
- `exec.go:211`, `:223`, `:227-228` (`Start`): after the `start` op, startup
  dialog handling runs, and its error tears the session down through
  `cleanupAfterStartFailure`.
- `exec.go:343` (`dismissStartupDialogs`) reaches `Peek` at `:370` and `:378`
  (poll fallback; the stream path at `:358` uses `watch-startup`, not
  `peek`). `exec.go:573-575` (`DismissKnownDialogs`) does the same.
- `internal/runtime/dialog.go:558-560`, and the sibling loops at `:414`,
  `:481`, `:677`, `:951`, `:1142`, `:1211`, `:1282`, `:1344`, `:1404`: a `peek`
  error is returned immediately (`return err`); an empty capture instead goes
  through the loop's content checks and polls again (loop body not re-read for
  this record; unverified).
- `internal/worker/runtime_handle.go:187-190`: `RuntimeHandle.Peek` calls
  `IsRunning` first and returns `ErrSessionInactive` without calling `peek`
  when the session is not running, so a missing session rarely reaches the op.
- `cmd/gc/status_provider.go:162`: `statusProvider.Peek` passes the base
  provider's error through `boundedStatusCall` unchanged.

So with exit 66, a `peek` that races a vanished pane surfaces as an error to
gc where exit 0 surfaced as an empty capture. The parity doc already records
this (`### peek` row 1: "gc has no exit-66 handling ... 66 would be an
ordinary error (`exec/exec.go:153-170`)"). Not verified here: a live gc run
that hits the dialog loop with a vanished pane; the reading above comes from
source only.

## Decision

`peek` answers exit 66 (`no screen for '<name>'` on stderr, empty stdout) for:

1. a name with no binding (`Resolve` returns `ok=false`);
2. a mid-call `rex.ErrNotFound` (pane vanished between resolve and `format`);
3. `rex.ErrUnavailable`.

This is a deviation from herdr-core in case 3: herdr-core answers an
unreachable core as exit 1 `cannot read the city's core`, and uses 66 only for
a parked city or nothing running (`reads.py:272-278`, with `:275-276`). We
answer 66 for every "no screen to read" cause, including an unreachable Rex.
Exit 1 stays for the other failures (bad `lines`, unexpected Rex errors).

Rationale: the three causes mean the same thing to a caller, that there is no
screen to return, and 66 keeps that distinct from a genuine failure on the wire.
gc ignores the distinction today (it reads 66 as an ordinary error, see
Conflict), so the cost is a failed peek instead of an empty one on a race that
`runtime_handle.go:187-190` already mostly guards.

## Revisit when

- gc handles exit 66 for `peek` (upstream fix in gastownhall/gascity), or the
  RPP version that defines it lands (`exitcodes.py:22` calls 66 v2-ready).
  The fix belongs upstream; no overlay (see `AGENTS.md`, overlay rule).
- A live run shows the dialog loop tearing down a healthy start because
  `peek` returned 66 during startup.
