# Rex-only capabilities a gc provider could offer

herdr-core is one carrier with one box model. Rex exposes things herdr-core
cannot. The list covers a server-side client registry and named sessions,
windows and blocks with aliases. It also covers layout operations, paged screen
capture, and several clients on one pane. The last group is TCP forwarding,
block plugins, and a foreground server with session save and restore.

This document asks one question for each such capability. Does gc 1.5.1 read
it? Could a change to this provider make gc do something it cannot do today?

The arbiter is gc tag `v1.5.1`, the baseline the parity record pins. This
document does not build a capability that has no gc-side consumer, however
good it looks from the Rex side. The companion question "what does herdr-core
have that we lack" is
[`docs/provider-herdr-parity.md`](../provider-herdr-parity.md). The decision
record for those extensions is
[`docs/herdr-extensions.md`](../herdr-extensions.md).

## Method

Three sources, in this order:

1. **Rex behaviour.** The output of `rex <command> --help` and
   `rex block types`, recorded below with the command. The contract record
   [`docs/REX-CONTRACT.md`](../REX-CONTRACT.md) §3-§5 is the second source for
   the wire shape.
2. **The gc consumer.** `git grep` and `git show` against tag `v1.5.1` in the
   pinned gc checkout. File and line are cited for every claim. Where the
   working tree is ahead of the tag, the claim is marked `unverified` against
   the pin.
3. **Our side.** `internal/provider` and `internal/rex/cli`. This keeps
   "already used" apart from "could be used".

Verdicts:

| Verdict | Meaning |
|---|---|
| `implement` | gc or this provider gains observable behaviour now. No row below needs new code, so the only `implement` row is already implemented. |
| `defer` | Worth having when a named gc-side or Rex-side trigger appears. The trigger is the entry point. |
| `reject` | Wrong for a shared, user-scoped Rex server or for gc's model. Do not revisit unless the model changes. |

Effort is `S` (a day or less), `M` (a week), or `L` (more than a week).
"Improvement" answers what gc would do better, not what Rex can do.

`unverified` marks a claim that the two sources do not settle.

## 1. Server-side client registry

**Rex surface.** `rex client ls` and `rex client inspect` list every program
connected to the server, with label and ownership. The wire source is
`session.list_clients` (`REX-CONTRACT.md` §3). That call returns
`{client_id, principal, info, connected_at, connection_state}`. The session
view has no `clients` field. The registry is the only source.

**Consumer.** Our `is-attached` already reads it. `ListClients` at
`internal/rex/cli/cli.go:122` maps the reply to `rex.ClientInfo`. gc consumes
one boolean per session. `is-attached` prints `true` or `false`
(`docs/reference/exec-session-provider.md:160` at v1.5.1). The provider
contract has no client identity, focus, or per-client input
(`docs/reference/exec-session-provider.md:144-166`). Verified: no gc v1.5.1
symbol names a client.

**Verdict.** `defer` for client-identity-aware behaviour. The boolean read is
already `implement`. A per-client op would let gc tell a person's client from
an agent's client. It would also let `is-attached` distinguish "a human watches
this pane" from "another agent's client holds a connection".

| Effort | Risk | Improvement |
|---|---|---|
| S to expose, M to consume (gc change) | Low. The registry is read-only. | gc could gate idle sleep on a human watcher rather than on any attached client. No such gc rule exists at v1.5.1. |

Trigger: gc gains a client-identity consumer, or a `tty.attach`/`stream` op
lands (#5).

## 2. Named sessions, windows and blocks, and alias sets

**Rex surface.** Rex offers `rex session rename|move`, `rex window rename|move`,
and `rex block rename|move|swap`. It also offers `target.resolve` and
`target.complete`. Those calls resolve labels, short ids, window positions and
client labels (`REX-CONTRACT.md` §3). Labels are limited to 128 characters.
Resolution is the "alias set" surface: one object answers to several labels.

**Consumer.** The hierarchy mapping is already used. City maps to session, rig
maps to tab, and agent maps to pane (`REX-CONTRACT.md` §1). `start` creates the
tab and pane through `NewSplit`/`NewWindow` (`internal/provider/provider.go:666`,
`:686`). Our binder resolves names and aliases (`binding.go`). gc addresses a
session by one name. It expects `list-running <prefix>` to return primary names
only. An alias returned for an empty prefix would mint a phantom orphan bead
(`REX-CONTRACT.md:898-901`). Verified: gc v1.5.1 has no rename, move or
target-resolve op (`docs/reference/exec-session-provider.md:144-166`).

**Verdict.** `reject` for rename, move and resolve as provider behaviour.
These are operator actions in the Rex TUI, not carrier operations. The alias
set is already consumed where gc needs it (name resolution). Returning it on
`list-running` would break gc's name contract.

| Effort | Risk | Improvement |
|---|---|---|
| M | Medium: a rename races gc's stored binding and the alias map. | None for gc. A user can already rename in Rex. |

## 3. Split and window layout operations

**Rex surface.** `rex split` creates a block beside another with a direction,
side and ratio. `rex window new` creates a tab with a layout. `rex zoom`,
`move`, `swap` and `resize` mutate an existing layout
(`REX-CONTRACT.md` §3 `session.new_split`, `session.new_window`).

**Consumer.** Creation is already used by `start`. The provider chooses
`NewWindow` for a new tab, or `NewSplit` to join an existing one, under the
per-rig lock (`internal/provider/provider.go:646-686`). gc has no op that
mutates a layout after start (`docs/reference/exec-session-provider.md:144-166`
at v1.5.1). A city is laid out by its rigs and agents. gc never rearranges
panes.

**Verdict.** `reject` for the mutation ops. The creation half is already
`implement`.

| Effort | Risk | Improvement |
|---|---|---|
| M | Medium: a provider-side layout change fights the user's arrangement and gc's tab/pane identity. | None. No gc behaviour names a layout. |

## 4. Paged screen capture (`format` + `blob_read`)

**Rex surface.** `block.method format` takes `max_bytes`. It returns either
`id` (more remains) or `eof: true`. `blob_read` pages the remainder by
`offset`. The reply cap is 1 MiB. `blob_read` clamps to 983040 bytes
(`REX-CONTRACT.md` §4).

**Consumer.** Already used. `internal/rex/cli/cli.go:321-340` asks for
`max_bytes: 983040` and follows `id` until `eof` (`formatMaxBytes`). gc's
`peek` passes a line count only (`script peek <name> <lines>`,
`docs/reference/exec-session-provider.md:156`). So gc never asks for bytes and
never pages. The paging exists to keep a large capture from failing
`too_large`. It is not an extension that gc requests.

**Verdict.** `implement` (already in use). No work.

| Effort | Risk | Improvement |
|---|---|---|
| 0 | None. | `peek` returns a full capture instead of failing `too_large`. Verified in our probe record and the parity doc `peek` rows. |

## 5. Multi-client attach to one pane

**Rex surface.** `rex session list_clients` reports several clients on one
session. Each has a `connection_state` (`REX-CONTRACT.md` §3). A block is
rendered for every attached client. Input comes from all of them. The gc-side
`attach` op is a terminal passthrough
(`internal/runtime/exec/exec.go:697` at v1.5.1). Our `attach` answers exit 0
with a hint (GCREX-20).

**Consumer.** gc v1.5.1 reserves `tty.attach` and sets `CanAttachTTY`. No
consumer reads it. `internal/runtime/auto/auto_test.go:600` says "nothing
consumes them yet". The `attach` op has no PTY op behind the token
(`docs/reference/exec-session-provider.md:198` at v1.5.1). Multi-client
observation is used only through the `is-attached` boolean (#1).

**Verdict.** `defer`. This is the same decision as the existing `tty attach`
row in [`docs/herdr-extensions.md`](../herdr-extensions.md) ("defer": needs a
Rex primitive and gc conformance support).

| Effort | Risk | Improvement |
|---|---|---|
| L: a Rex PTY attach primitive plus a gc connection op | High: terminal ownership, input fencing and detach semantics. | `gc session attach` becomes a real interactive terminal instead of a hint. |

Trigger: a user request for true in-terminal attach, or a Rex attach primitive
landing. Revisit with `tty.attach`.

## 6. TCP port forwarding

**Rex surface.** `rex forward new` forwards TCP like `ssh -L` and `ssh -R`.
Each forward is a tunnel block. It reconnects and closes with defined
semantics. `rex forward ls|close` lists and closes forwards. `rex block types`
lists `com.superlogical.tunnel.dial` and `com.superlogical.tunnel.listen`.

**Consumer.** None in gc v1.5.1. Verified: no port-forward, `ssh -L` or
tunnel symbol in `internal/runtime` or `cmd/gc` at the pin. The only match is
the `ssh` provider's transport note (`internal/runtime/ssh/ssh.go:14`). The
`runtimecapability` probes speak of a "sandbox→host tunnel" as a transport
concern, not an RPP op.

**Verdict.** `reject` for RPP v0. A forward is a user-level Rex feature. A
provider that forwarded ports for gc would expose a service on a shared
server. That service would have no gc-side lifecycle to close it.

| Effort | Risk | Improvement |
|---|---|---|
| M | High: opens network listeners. The provider would own their lifetime. | None. gc has no service to forward. |

## 7. Block plugins and plugin reload

**Rex surface.** `rex server run --block-dir DIR` discovers executable block
plugins. `--block-plugin-reload` reloads them after source changes.
`rex block types` lists the installed flavors
(`com.superlogical.terminal.shell`, `onboarding`, `tunnel.dial`,
`tunnel.listen`).

**Consumer.** None in gc v1.5.1. Verified: `plugin` appears only in provider
capability prose about agent hooks (`docs/reference/config.md:730`). It is
never a runtime op (`docs/reference/exec-session-provider.md:144-166`). gc
starts one terminal block flavor and closes it.

**Verdict.** `reject`. Plugin loading is a server-owner feature. A carrier that
depended on a plugin flavor would make gc's start depend on the user's server
configuration.

| Effort | Risk | Improvement |
|---|---|---|
| L | Medium: a plugin load failure would surface as a start failure. | None. |

## 8. Foreground server, state dir, `--exit-empty`, session save/restore

**Rex surface.** `rex server run` runs in the foreground. It saves its sessions
on Ctrl+C, SIGTERM and `rex server stop`. It accepts `--state-dir DIR`,
`--restore` (default true) and `--exit-empty`. `--exit-empty` stops the server
when no sessions or control connections remain. It also accepts `--block-dir`,
`--block-plugin-reload`, `--max-sessions`, `--max-blocks`, `--max-windows` and
`--max-panes`. `rex server status|stop|logs` manage the persistent variant.

**Consumer.** At v1.5.1, none. Verified: `server-lifecycle` and `server-stop`
do not exist at the tag. `git grep` returns nothing in `internal/runtime` and
`docs/reference`. The `ProtocolCapabilityServerLifecycle` token is a working
tree addition. The working tree does run `server-stop` from `TeardownServer`
after the city's sessions stop (`internal/runtime/exec/cutover.go:161-164`,
`internal/runtime/exec/exec.go:328`). That happens only for a pack that
declares the token. `ConfigureServer` is a no-op by test
(`internal/runtime/exec/server_test.go:60-71`). Save/restore and `--exit-empty`
have no gc consumer at either revision. Rex owns its server lifetime. A shared
server must not exit while another city or the user works.

**Verdict.** `defer` to GCREX-30 (city-owned server). The
[`docs/herdr-extensions.md`](../herdr-extensions.md) decision already names
GCREX-30 as the trigger for `server-stop` and `server-lifecycle`. Session
save/restore stays a Rex-owner feature. gc gets idempotent start through stored
bindings and does not need restored sessions. `--state-dir` and the limits are
server configuration, not provider behaviour.

| Effort | Risk | Improvement |
|---|---|---|
| M for a city-scoped server, 0 for save/restore | High: stopping a shared server kills other cities and the user's panes. | A gc-owned server could be stopped at `gc stop`. Today gc must leave the shared server alone. |

Trigger: GCREX-30 changes the server model to city-owned. Then revisit
`server-stop` and `server-lifecycle` as a new op, not this one.

## Summary

| Capability | gc 1.5.1 consumer | Verdict | Effort | Risk |
|---|---|---|---|---|
| Client registry | `is-attached` boolean only (`cli.go:122`) | implement (read), defer (identity) | S / M | low |
| Named objects and alias sets | hierarchy and name resolution, no rename op | reject | M | medium |
| Split and layout operations | `start` uses `new_window`/`new_split`, no mutation op | implement (create), reject (mutate) | 0 / M | medium |
| Paged `format`/`blob_read` | `peek`, already paged in `cli.go:321-340` | implement (already) | 0 | none |
| Multi-client attach | `is-attached`, `tty.attach` reserved with no consumer | defer | L | high |
| Port forwarding | none | reject | M | high |
| Block plugins and reload | none | reject | L | medium |
| Foreground server, state dir, save/restore | none at v1.5.1, `server-stop` in the working tree only | defer to GCREX-30 | M / 0 | high |

No row needs implementation now. That is the finding, not a gap. Every
Rex-only capability that gc can consume is already consumed. The rest need a
gc-side change first.

## What survives, and where it goes

- **Client identity (#1)** and **multi-client attach (#5)** fold into the
  existing `tty attach` deferral in
  [`docs/herdr-extensions.md`](../herdr-extensions.md). Trigger: a
  `tty.attach`/`stream` op, or a gc rule that distinguishes a human watcher.
- **Server lifecycle (#8)** folds into GCREX-30. Trigger: a city-owned Rex
  server.
- The reject rows need no task. They are recorded here so the question is
  closed, with the reason, rather than reopened.

## Probes recorded

All commands were run on 2026-11-21 against the local `rex` on `PATH`. The
output is quoted in the sections above. No probe changed provider or server
state.

| Command | Fact established |
|---|---|
| `rex --help` | command surface: `forward`, `client`, `server`, `events`, `api`, `actions` |
| `rex client --help` | `ls`, `inspect` over the connected-client registry |
| `rex session --help`, `rex window --help`, `rex block --help` | rename, move, swap, focus, capture |
| `rex forward --help`, `rex forward new --help` | `-L`/`-R` forwarding, tunnel blocks, reconnect |
| `rex server run --help` | `--state-dir`, `--restore`, `--exit-empty`, `--block-dir`, `--block-plugin-reload`, limits |
| `rex block types` | installed flavors, including `tunnel.dial` and `tunnel.listen` |
| `rex capture --help` | screen plus scrollback, about 64 MiB, `--format text\|html\|vt` |

The gc claims were checked with `git show v1.5.1:internal/runtime/protocol.go`,
`git show v1.5.1:docs/reference/exec-session-provider.md`, and `git grep` at
`v1.5.1`. The searched terms were `server-lifecycle`, `server-stop`,
`tty.attach`, `proc.stream`, `port.?forward`, `plugin` and
`session.list_clients`.
