# 500. Keep `start` welded

- Status: Accepted
- Date: 2026-11-26

## Context

Parity row 120 ([parity doc:120](https://github.com/docbobo/rex-gascity/blob/09f11b8/docs/provider-herdr-parity.md#L120)) reads: `provision` is not
implemented. So is `exec`. gc's `provisionBox` runs the `provision` op only when
the handshake declares both `proc.provision` and `proc.exec`
(`/Volumes/Sources/gascity/internal/runtime/exec/exec.go:570-580`). Without
both, the welded `start` op provisions and launches in one shot
(`exec-session-provider.md:168-176`).

The un-weld, at gc edge `890a58a3e`:

- `provision` is `start` minus the agent launch (`exec-session-provider.md:168-173`).
- The "launch" half is not an RPP op. There is no `proc.launch` token in
  `internal/runtime/protocol.go` and no `launch` op in `internal/runtime/exec`.
  gc's `launchAgent` runs `tmux new-session` or `tmux respawn-pane -k` over
  `exec` into the box's tmux session `main` (`exec/exec.go:604-650`).
- A launch-only config change then `Relaunch`es in the warm box. That is
  `launchAgent` plus a host `session_setup`, with no reprovision
  (`exec/exec.go:650-672`).

So a pack un-welds by giving the controller a tmux box to type into. That is
the whole second half (`exec-session-provider.md:231,833`). The first half is
`provision` plus `exec`.

## Decision

We keep `start` welded. We do not implement `provision` or `exec` now, and we
keep `proc.provision` and `proc.exec` undeclared (`cmd/gc-runtime-rex/main.go:34-59`,
`unimplemented` at `:161-165`).

The bead asks whether the un-weld is worth it. It is not, until the launch half
exists for a non-tmux carrier:

1. **The launch half is in-box tmux, and nothing else exists.** gc runs the
   tmux command it appends to `exec`. The only successor to the welded `start`
   is a tmux session inside the Rex pane. A Rex agent is already its own process
   in a pane. It has a nudge and a readiness gate. A tmux layer under it adds
   nothing on a config change that `Stop`+`Start` or a relaunch cannot do.
2. **Passing on the launch-op reference means declaring `proc.exec` anyway.**
   The pin reads the two tokens independently. `provisionBox` runs `provision`
   only when the handshake declares both `proc.provision` and `proc.exec`
   (`exec/exec.go:277-296`). Declaring `proc.provision` alone withholds nothing
   from `exec`. It only leaves the un-weld inert. So the un-weld declares
   `proc.exec` too, and that token states intent to run it. The gates are in
   `REX-CONTRACT.md` section 9.
3. **Each declared capability must be exercised.** gc tries `exec` before the
   driving op and falls back only on exit 2 (`REX-CONTRACT.md` section 9,
   `exec/exec.go:894-904`). `exec` returns the command exit code and its output.
   The behavioral probe cannot check it. Agreeing to the wire behavior without
   a caller repeats the row 120 mistake.

One common set of both `Start`/`Stop` and the six welded ops is what a config
change preserves. So the accepted plan: do not implement the ops, do not declare
the capabilities, and record the decision here and in row 120.

No behavior changes. Row 120's verdict `not-implemented` stands. It now points
at this ADR as the decision, not only the omission.

A Rex `provision` would not be dead code on the pin. gc `v1.5.1` reads both
tokens and gates the separable launch on them. `supportsSeparableLaunch`
returns true only when the handshake declares `proc.provision` and `proc.exec`.
`provisionBox` then runs the `provision` op instead of the welded `start`
(`internal/runtime/exec/exec.go:277-296` at tag `v1.5.1`). The parity doc
records the same gate at [parity doc:432](https://github.com/docbobo/rex-gascity/blob/09f11b8/docs/provider-herdr-parity.md#L432). The un-weld is
inert on the pin whenever either capability is absent. Declaring both makes the
pin call `provision`, so the pin is not a reason to keep it welded. The
missing launch half is the reason to keep it welded.

Before any future landing, re-read the `provisionBox` gate against `v1.5.1` and
edge and run `make conformance`. ADR-0011 retired the parity docs this ADR first
cited; its links below are pinned to the last commit that held them.

## Consequences

- A launch-only config change costs a full `Start` (a new pane, or the welded
  relaunch path). There is no warm-box relaunch that keeps the pane.
- Box-without-agent is still unavailable. gc falls back to welded `start`, which
  conformance allows. The `exec`/`provision` rows are optional SKIP
  (`REX-CONTRACT.md:467-468`).
- The handshake keeps its seven tokens. A future un-weld must declare
  `proc.provision` with the `provision` op. It must declare `proc.exec` only
  with a tested `exec`.
- The identity keys (GC_RUNTIME_EPOCH, GC_CONTINUATION_EPOCH) are
  `set-meta`/`get-meta`/`remove-meta` work if something else needs them. They
  need no un-weld. The bead names them as a prerequisite of the un-weld. They
  are not a reason for it.

## Alternatives rejected

- **Implement `provision` and `exec` now, and relaunch over tmux.** The launch
  half is a tmux session in the pane. Rex already owns the process, the nudge
  and the readiness gate. A tmux shim under an agent in its own pane buys a
  warm box at the cost of a second multiplexer and a new carrier rule.
  `carrier.ops` disables the un-weld outright. The pin gc does not ignore the
  tokens: with both declared it runs `provision`.
- **Implement `provision` alone.** gc never calls it without `proc.exec` too
  (`exec/exec.go:570-580`). An undeclared `proc.provision` means `provision` is
  unknown by contract, so the conformance probe SKIPs it. Dead op.
- **Declare `proc.provision` and let gc fall back.** gc calls `provision` only
  when the handshake declares both `proc.provision` and `proc.exec`
  (`exec/exec.go:277-296` at `v1.5.1`). Declared alone, `proc.provision` leaves
  the un-weld inert and gc falls back to welded `start`. Declared together with
  `proc.exec`, the `provision` op may never answer exit 2
  (`REX-CONTRACT.md:222`), so a not-implemented `provision` fails every call and
  the un-weld becomes worse than welded `start`.
- **Add `proc.launch`, the token the bead's title implies.** There is no such
  token at edge `890a58a3e` (`internal/runtime/protocol.go`). The launch half is
  `exec`. A token that no gc reads is an unimplemented capability with a new
  name.

## Sources and status

- gc edge `890a58a3e`: `internal/runtime/exec/exec.go:267-296` (collision
  phrases), `:570-580` (`supportsSeparableLaunch`), `:604-672` (`launchAgent`,
  `Relaunch`), `internal/runtime/seam_adapter.go:60-85`,
  `internal/runtime/worker.go:83-93`,
  `docs/reference/exec-session-provider.md:133-176`, `:228-231`.
- Repo: [parity doc:120](https://github.com/docbobo/rex-gascity/blob/09f11b8/docs/provider-herdr-parity.md#L120), [`:420-446`](https://github.com/docbobo/rex-gascity/blob/09f11b8/docs/provider-herdr-parity.md#L420-L446),
  `cmd/gc-runtime-rex/main.go:34-59`, `:161-165`.
- `v1.5.1` pin: re-read here. `internal/runtime/exec/exec.go:277-296` gates the
  separable launch on both `proc.provision` and `proc.exec` and runs `provision`.
  `internal/runtime/protocol.go:18-55` declares the six capability tokens,
  including `proc.provision`. The parity doc records the same gate at
  [parity doc:432](https://github.com/docbobo/rex-gascity/blob/09f11b8/docs/provider-herdr-parity.md#L432). That the pin has no `proc.launch` is
  `unverified` at the pin. The edge checkout has none.
