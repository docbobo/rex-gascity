# 500. Keep `start` welded

- Status: Accepted
- Date: 2026-11-26

## Context

Parity row 120 (`docs/provider-herdr-parity.md:120`) reads: `provision` is not
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
   A handshake that declares `proc.provision` withholds `proc.exec` from gc.
   So gc tests `exec` as unsupported unless the token is declared. The gates
   are in `REX-CONTRACT.md` section 9. Declaring it states intent to run it.
3. **Each declared capability must be exercised.** gc runs no `exec` for a pack
   that never declares it. `exec` returns the command exit code and its output.
   The behavioral probe cannot check it. Agreeing to the wire behavior without
   a caller repeats the row 120 mistake.

One common set of both `Start`/`Stop` and the six welded ops is what a config
change preserves. So the accepted plan: do not implement the ops, do not declare
the capabilities, and record the decision here and in row 120.

No behavior changes. Row 120's verdict `not-implemented` stands. It now points
at this ADR as the decision, not only the omission.

A Rex `provision` would be dead code on the pin. It would live only on an edge
gc. That is a second reason to keep it welded.

The pin has no consumer of `proc.provision` today. gc `v1.5.1` parses exactly
six tokens and ignores the rest. On the pin the un-weld is inert. The pin gives
no parity gain at all.

The sources are `docs/provider-herdr-method-io.md` and
`docs/provider-herdr-parity.md`. Before any future landing, re-run the parity
check against `v1.5.1` and edge. The `provisionBox` gate at the pin is recorded
at `docs/provider-herdr-parity.md:430`.

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
  `carrier.ops` disables the un-weld outright. The pin gc ignores the tokens
  anyway.
- **Implement `provision` alone.** gc never calls it without `proc.exec` too
  (`exec/exec.go:570-580`). An undeclared `proc.provision` means `provision` is
  unknown by contract, so the conformance probe SKIPs it. Dead op.
- **Declare `proc.provision` and let gc fall back.** A declared op may never
  answer exit 2 (`REX-CONTRACT.md:222`, `exec/exec.go:425-429`). gc would then
  fail every `provision` call. The un-weld becomes worse than welded `start`.
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
- Repo: `docs/provider-herdr-parity.md:120`, `:420-446`,
  `cmd/gc-runtime-rex/main.go:34-59`, `:161-165`.
- `v1.5.1` pin: the parity doc records the `provisionBox` gate at the pin
  (`docs/provider-herdr-parity.md:430`). We did not re-read the pin tree here.
  That the pin has no `proc.launch` is `unverified` at the pin. The edge
  checkout has none.
