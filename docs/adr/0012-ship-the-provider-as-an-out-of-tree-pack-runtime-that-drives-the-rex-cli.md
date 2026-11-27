# 0012. Ship the provider as an out-of-tree pack runtime that drives the rex CLI

- Status: Accepted
- Date: 2026-11-17 (backfilled from plan decisions D0 and D2)

## Context

gc can select a session provider in three ways. It can use a pack-declared
runtime, which is an executable speaking RPP v0. It can use an in-tree Go
provider registered in `cmd/gc/runtime_registry.go`. Or it can use a raw
`exec:<path>` script. RPP v0 already carries the whole `Provider` contract as
wire ops (`docs/REX-CONTRACT.md` §9).

The Rex CLI is versioned and documented. Its socket wire protocol is
revision-numbered but undocumented (`REX-CONTRACT.md` §7).

## Decision

We ship `gc-runtime-rex` as a standalone Go module with no gascity imports.
`pack/pack.toml` declares it as `[runtimes.rex]` with `protocol = 0`, and the
pack installs it. A city selects it with `[session] provider = "rex"`.

The provider reaches Rex only by shelling out to the `rex` CLI, behind the
`rex.Client` interface (`internal/rex/client.go:165`). A socket client can
replace the CLI client later without touching provider logic.

## Consequences

- The provider releases on its own schedule and is checked with
  `gc runtime check` and `gc runtime conformance`.
- It sees only what RPP v0 and its capability tokens carry. Go-only interfaces
  such as `runtime.SessionEventProvider` are out of reach.
- Each op pays a process spawn per Rex call.

## Alternatives rejected

- **An in-tree Go provider in gc.** It would edit gc's registry and ship in the
  gc binary. Its only unique gain is push session events, and gc's controller
  does not consume any provider's event stream yet. Revisit only once it does.
- **A raw `exec:<path>` script.** It has the same surface, but the selector is
  a path, not a clean `rex` name, and the pack cannot install it.
- **Talk to the Rex socket directly.** That protocol is undocumented and moves
  with each revision. The CLI is the stable contract.
