# herdr-only runtime extensions — decisions and designs

Decisions for the runtime extensions and handshake tokens that
`runtime-herdr-core` has and our Rex provider does not. This is the design
half of GCREX-28; implementation lives in GCREX-28.2 and is scoped by the
**accepted** rows below.

- Evidence base: [`docs/provider-herdr-parity.md`](provider-herdr-parity.md),
  which compares us against `gastownhall/gascity-packs` `runtime-herdr-core`
  @ `0f07496` and verifies gc behavior against tag `v1.5.1`.
- Contract: RPP v0 as pinned in [`REX-CONTRACT.md` §9](REX-CONTRACT.md)
  (gc 1.5.1) — 17 baseline ops plus the beyond-table `clear-scrollback`,
  `send-keys` and `watch-startup` calls. `attach` is one of the 17.
- Our side: `cmd/gc-runtime-rex/main.go` (dispatch) and `internal/provider`.

## How to read this document

Every herdr-only extension and every non-baseline handshake token gets one
verdict:

| Verdict | Meaning |
|---|---|
| **implement** | Worth having in Rex now. GCREX-28.2 implements it, gated behind the named capability token when one exists. It has a design section here. |
| **defer** | Not worth implementing until a named gc-side (or Rex-side) trigger appears. No design section; the trigger is the entry point for a future task. |
| **reject** | Wrong for Rex's model or deliberately excluded. Not revisited unless the model changes. |

The decisions are driven by three facts. The first two pin the baseline the
parity doc verifies; the third is what a *consuming* gc — the working tree,
ahead of the `v1.5.1` tag — does with the tokens the pin ignores.

1. At the `v1.5.1` pin gc has no production caller for `observe`, `inventory`,
   `kill-object`, `server-stop` or `copy-to`, and its
   `internal/runtime/protocol.go` defines none of the herdr-only tokens
   (`observe.v1`, `carrier.ops`, `server-lifecycle`, `idle-boundary`,
   `input-outcome`, `fs.host`, `city-scoped`, `conformance-faults`); it
   declares only `report-attachment`, `report-activity`, `proc.exec`,
   `proc.stream`, `tty.attach` and `proc.provision`. Those omissions are inert
   against the pin (`docs/provider-herdr-parity.md` §"Gaps that are
   deliberate, not mistakes").
2. `REX-CONTRACT.md` §9 says not to declare tokens outside the pinned
   baseline, and the pin ignores unknown capability strings by design
   (`internal/runtime/protocol.go`; pinned by `exec/handshake_test.go`).
3. A consuming gc gives those tokens meaning. `observe.v1` is not an
   `observe`-only gate: it is the *reconciler-v2 profile token*
   (`internal/runtime/protocol.go` `ProtocolCapabilityObserveV1`). Declaring
   it obliges `observe`, `inventory` and `kill-object`, `fresh_only` starts,
   strict exit-2 handling, identity consistency and `peek` of a dead box
   (`docs/reference/exec-session-provider.md` §"observe.v1", obligations
   1–13). Our handshake already declares `report-attachment`, so adding
   `observe.v1` makes Rex v2-ready on the handshake alone
   (`internal/runtime/exec/readiness.go` `v2ProfileCapabilities`), which
   switches a consuming gc's city to reconciler v2.

Fact 2 means an **implement** verdict is also a contract change: the accepted
extension's token moves from the §9 "not in the baseline — do not declare"
list to the declared set, in the same change that implements the op
(GCREX-28.2 AC 1). An op that is not implemented must not have its token
declared, and by Fact 3 a profile token must not be declared without the
whole profile. That is why the accepted set is the `observe.v1` profile as one
bundle — `observe`, `inventory` and `kill-object` — and nothing else.

These decisions are provisional: GCREX-29 (Rex-only candidates), GCREX-30
(whether a city-owned server makes `server-stop` and `watch-startup`
worthwhile) and GCREX-31 (Rex CLI facts shaping `copy-to` and `watch-startup`)
are still To Do. Revisit the affected rows when those inputs land.

## Decision summary

| Extension | Kind | Verdict | Reason | gc-side trigger that changes this |
|---|---|---|---|---|
| `observe` | read | **implement** | One round trip answers "what is this session doing"; collapses the per-session probe fan-out (`is-running` → `get-last-activity`/`is-attached`/`process-alive`). Accepted as part of the `observe.v1` profile (Fact 3): the token is the reconciler-v2 gate, not an additive one-op capability. | accepted now with the profile |
| `inventory` | read | **implement** | One call returns state, attachment, activity and identity for every session; the same fan-out reduction as `observe` at bulk scope, and a natural basis for a Rex status view. No dedicated token: `inventory` is an op. `carrier.ops` is a separate carrier-selector declaration, not an inventory marker. | accepted now with the profile |
| `kill-object` | lifecycle | **implement** | The `observe.v1` profile obliges it: a consuming gc maps the token to `observe, inventory, kill-object` (`internal/runtime/rppcheck/rppcheck.go:244`, `:602-608`) and fails conformance with "observe.v1 declared but kill-object not implemented" (`:700-710`). It re-checks and kills the exact object, which the `stop` survivor gap needs. | accepted now with the profile |
| `copy-to` | fs | defer | No production caller in v1.5.1; the seam is wired (`execPlace.Stage` → `Provider.CopyTo` → `copy-to`), so a future caller needs no new plumbing. exec-pack staging is `copy_files`/`overlay_dir` inside `start`/`provision`, which is the real gap. `copy-to` on its own stages nothing gc asks for. | a gc version with a `copy-to` caller, or `copy_files` staging being implemented and wanting a reusable staging primitive |
| streaming `watch-startup` | stream | defer | gc 1.5.1 already accepts exit 2 and falls back to `peek` polling every 500 ms plus `send-keys` dialog handling; Rex dismisses startup dialogs in-launch. A 250 ms NDJSON stream is a latency optimisation, not a correctness need. | a pilot showing startup dialogs that the `peek`+`send-keys` fallback handles slowly or unreliably |
| `tty attach` | tty | defer | `attach` already answers exit 0 with a hint (GCREX-20), so `gc session attach` works; a real pty passthrough needs a Rex primitive and `gc runtime conformance` support for the reserved token. | a user request for true in-terminal attach, or a Rex attach primitive landing |
| `server-stop` | lifecycle | reject | GCREX-1 §11 D3: the Rex server is shared and user-scoped, not per-city. Stopping it would kill the user's other work. | only if Rex gains an owned, city-scoped server (GCREX-30) — then revisit as a new op, not this one |

## Handshake token decisions

Tokens are grouped by the extension they gate or select. A token is declared
only when the behavior it declares is implemented (Fact 2).

| Token | Gated by / selects | Verdict | Reason | gc-side trigger that changes this |
|---|---|---|---|---|
| `observe.v1` | reconciler-v2 profile (`observe`, `inventory`, `kill-object`) | **implement** | The profile token, not an `observe`-only gate. Declaring it obliges `observe`, `inventory`, `kill-object`, `fresh_only` starts, strict exits, identity consistency and `peek` of a dead box, and (with `report-attachment`) makes Rex v2-ready. Move it off the §9 do-not-declare list only in the change that carries the whole profile. | accepted now as the profile |
| `carrier.ops` | carrier selector | **implement** | Declares that gc drives the box only through the pack's own `nudge`, `peek`, `send-keys`, `interrupt` and `clear-scrollback` (`handshake.py:6-8`). Not an inventory marker. At the v1.5.1 pin gc ignores it; gc's working tree implements it (`internal/runtime/protocol.go:35-41`): all five driving ops required, exit 2 from any one becomes a failure, input fencing none, start welded. | a gc that consumes `carrier.ops`; re-check the five driving ops and their exit-2 paths before relying on the declaration |
| `server-lifecycle` | `server-stop` | reject | Follows the `server-stop` rejection; D3 keeps the shared server alive. | only if Rex gains an owned, city-scoped server (GCREX-30) — a model change |
| `city-scoped` | server model | reject | False for Rex: one user-scoped server serves every city. Declaring it would misrepresent the model; revisit only with GCREX-30. | only if GCREX-30 changes the server model to city-owned |
| `fs.host` | host filesystem | reject | Rex exposes no host-fs op and the token needs a newer gc (`handshake.py` docstring cites gastownhall/gascity#9032/#9129). No Rex behavior to gate. | only if a newer gc consumes `fs.host` and Rex gains a host-fs op — a model change |
| `input-outcome` | input exit codes | defer | gc 1.5.1 has no `input-outcome` and treats exit 80 as failure, so tmux-style best-effort stays correct. | gc consumes outcome-bearing input responses |
| `idle-boundary` | activity | defer | No gc 1.5.1 consumer and no settled Rex meaning for an "idle boundary". | gc declares/consumes it |
| `conformance-faults` | diagnostics | defer | herdr-internal diagnostics with no gc 1.5.1 consumer. | gc declares/consumes it (e.g. inside `gc runtime check`) |

## Designs for accepted extensions

The accepted set is the `observe.v1` profile — `observe`, `inventory` and
`kill-object` — plus the `carrier.ops` declaration. `observe` and `inventory`
are reads; `kill-object` is a targeted kill. `observe` and `kill-object` are
gated by `observe.v1`; `inventory` needs no token. At the v1.5.1 pin gc
ignores the non-baseline tokens, so declaring `observe.v1` and `carrier.ops`
cannot change baseline behavior (GCREX-28.2 AC 2); a consuming gc reads
`observe.v1` as the reconciler-v2 profile (Fact 3), so GCREX-28.2 carries the
profile's remaining obligations, not just the three ops.

### `observe`

- **Op contract.** `observe <name>` takes the session name as its single
  positional argument. It reads agent process names from stdin only to judge
  agent liveness; it does not take session names from stdin. It answers once
  for the named session. It is a read: it must never start, stop, nudge or
  otherwise mutate a session.
- **Capability token.** `observe.v1`, declared in the `protocol` handshake in
  the same change that adds the handler. No other token implies it.
- **Deadline class.** *probe*. The reference uses a 4 s probe deadline
  (`rpp/ops.py`); our dispatcher should cap the op at that class rather than
  the 30 s default so a hung Rex server cannot stall a gc probe. The Rex call
  should be bounded by the op context so SIGTERM cancels it.
- **stdin.** A newline-separated list of agent process names (not session
  names), used only to judge agent liveness. Empty stdin is valid: the op
  still answers for the named session, using the box's own program as the
  agent when there are no names to judge by.
- **stdout shape.** Exactly one JSON `ObserveAnswer` per invocation
  (`rpp/wire.py:162-198`): `absent` when there is no record (or the server is
  stopped), `unknown` when the server cannot be reached or a lock times out,
  `dead` when a known record is not intact (with its box object and
  identity), or `running` with the box object, agent state, agent pid and
  identity. `absent` and `unknown` carry only the state. Stdout is capped at
  1 MiB — gc's own `observeOutputCap` (`internal/runtime/exec/observe.go:31`),
  which the reference matches (`rpp/ops.py:33`). A malformed or oversized
  answer reads `unknown`, never `absent` (`docs/reference/
  exec-session-provider.md` §"observe.v1").
- **Exit codes.** 0 for every answer, including `absent` and `unknown` — the
  state is the payload, not the exit code. Exit 2 stays the unknown-op answer
  only for a consumer not yet told the token; once `observe.v1` is declared it
  must not be used for anything else, because a consuming gc reads exit 2 from
  a declared op as a failure (`internal/runtime/protocol.go`
  `ProtocolCapabilityObserveV1`; `docs/reference/exec-session-provider.md`
  §"observe.v1" obligation 3). Exit 1 is an internal failure that prevents any
  answer.
- **State and cleanup.** None. Read-only; no locks held across the reply
  beyond the read. No meta is written, nothing is killed, no file is staged.

### `inventory`

- **Op contract.** No arguments, no stdin. Returns a snapshot of every
  session the provider tracks for the city, so a single call replaces a
  per-session fan-out.
- **Capability token.** None. `inventory` is an op, not a capability; the
  reference declares no inventory-specific token. The handshake's
  `carrier.ops` is a separate carrier-selector declaration (see "Handshake
  token decisions") and does not gate `inventory`. Declaring it would change
  the carrier contract, so it is not required to add this op. `list-running`
  stays for the name-only contract.
- **Deadline class.** *default* (30 s) as the reference uses. Stdout is
  capped at 8 MiB — gc's own `inventoryOutputCap`
  (`internal/runtime/exec/inventory.go:27`), which the reference matches
  (`rpp/ops.py:34`) and the contract states (`docs/reference/
  exec-session-provider.md` §"inventory"). Our cap must stay within gc's:
  8 MiB for `inventory` and 1 MiB for `observe`.
- **stdin.** Ignored (must not read).
- **stdout shape.** One JSON object: `{"complete": bool, "sessions": [ ... ]}`.
  A session whose state cannot be read carries only its `name`; otherwise it
  has `name`, `state` (running/dead), the box under the wire key `object`
  (`{"id","created"}`, not `box`), `agent`, `attached`, `last_activity`,
  `restart_requested` and `identity` (`rpp/wire.py:228-240`; gc's contract
  example uses `object`). `complete` is false whenever any record could not be
  read, so a caller can tell a partial answer from a full one.
- **Exit codes.** 0 with `complete:false` when the server is partially or
  wholly unreadable (a stopped or unreachable Rex server is `complete:false`,
  empty `sessions`), 1 only on an internal failure that prevents building the
  object. Never exit 2 once declared.
- **State and cleanup.** None. Read-only snapshot; no meta writes, no
  session changes, no staging.

### `kill-object`

- **Op contract.** `kill-object <name>` takes the session name as its single
  positional argument and a JSON object on stdin. It re-checks that the exact
  object still holds the name and its creation time, then kills that object;
  it never kills by name. `corpse` kills only while the object still holds the
  name, no client is attached and nothing runs, answering `live` otherwise;
  `zombie` kills only while the object still holds the name, no client is
  attached, the box runs and its agent is still dead (`agent_pid` unchanged
  when given), answering `changed` otherwise. It does not replace `stop`;
  `stop` stays the baseline teardown.
- **Capability token.** `observe.v1`, the profile token; there is no separate
  `kill-object` token. A consuming gc refuses an object kill from a pack that
  is not v2-ready (`internal/runtime/exec/observe.go` `KillCorpseObject`,
  `KillZombieObject`, `killObject`).
- **Deadline class.** *default* (30 s), as the reference registers it
  (`herdrcore/ops_table.py:71`). The kill is bounded by the op context so
  SIGTERM cancels it.
- **stdin.** One JSON object: `{"object":{"id","created"},
  "mode":"corpse"|"zombie","agent_pid":"..."}` (`agent_pid` optional, empty
  when the box does not know it; `rpp/wire.py:276-287`). Malformed input is an
  op error and kills nothing.
- **stdout shape.** Exactly one scalar: `killed`, `gone`, `renamed`, `live`
  or `changed` (`rpp/wire.py:138`; gc maps the same five in
  `internal/runtime/exec/observe.go` `killObjectResults`). `gone` covers an
  object that no longer exists or was created at another time; `renamed`
  covers a name the object no longer holds. Stdout stays under the 64 KiB
  small cap.
- **Exit codes.** 0 for every kill outcome — the outcome is the payload.
  Exit 2 is the unknown-op answer only for a consumer not yet told the token;
  once `observe.v1` is declared it is a failure (Fact 3). Exit 1 for malformed
  stdin, an internal failure, or a kill that might leave descendants alive
  (the box stays for a retry).
- **State and cleanup.** Takes the exclusive per-name lock for the
  re-check-and-kill. On success it kills the whole process tree (TERM, then
  KILL) and drops the record's meta file and activity entry, so no identity or
  meta survives the object; a second kill on the same object answers `gone`.
  On a partial kill the box and its meta stay and the op exits 1.

## Deferred extensions

A deferred extension needs no design now. When its trigger fires, open a
follow-up task that: accepts the extension, adds the design section here, and
scopes GCREX-28.2-style implementation with the same gate/token rule. The
deferred set is `copy-to`, streaming `watch-startup`, `tty attach`, and — for
tokens — `input-outcome`, `idle-boundary`, `conformance-faults`.

## Consequences for GCREX-28.2

- GCREX-28.2 implements the **`observe.v1` profile**: **`observe`**,
  **`inventory`** and **`kill-object`**, and declares **`observe.v1`** in the
  same change (AC 1). `inventory` needs no token of its own; it is an op.
- Declaring `observe.v1` is the v2 profile, not a one-op gate. GCREX-28.2 must
  also carry the profile's remaining obligations
  (`docs/reference/exec-session-provider.md` §"observe.v1"): `fresh_only` on
  `start` (and `provision` if declared); exit 2 from every implemented op as a
  failure; one identity for `GC_SESSION_ID`, `GC_RUNTIME_EPOCH`,
  `GC_INSTANCE_TOKEN` and `GT_PROCESS_NAMES` across `get-meta`, `observe` and
  `inventory`; `absent` only when the backend proves nothing holds the name;
  `peek` for a dead box; and serialized per-name ops. With `report-attachment`
  already declared, this makes Rex v2-ready on a consuming gc
  (`internal/runtime/exec/readiness.go`), which switches its city to
  reconciler v2 — one profile change, not a set of independent gates.
- It also declares **`carrier.ops`** as the carrier selector, not as an
  inventory marker. At the v1.5.1 pin gc ignores unknown tokens, so the
  declaration is inert today. A gc that consumes `carrier.ops` will drive the
  box only through the five dedicated ops (`nudge`, `peek`, `send-keys`,
  `interrupt`, `clear-scrollback`); all five must be present and exit 2 from
  any one is a failure, not "unknown op". It also sets input fencing to none
  and start welded. Re-check those five paths against the consuming gc before
  relying on the declaration.
- It must move `observe.v1` and `carrier.ops` off the `REX-CONTRACT.md` §9
  do-not-declare list and record them as declared tokens.
- The baseline handshake for a gc 1.5.1 city must be unchanged apart from
  those two tokens, and all three ops must be tested against `gc runtime
  check` (AC 2).
- It must update the parity doc coverage/summary tables: `observe`, `inventory`
  and `kill-object` move from `not implemented` to `implemented`, and the
  "deliberate gaps" paragraph drops them from the never-sent list (AC 4).
- Everything else in the decision summary stays deferred or rejected and is
  out of GCREX-28.2's scope.
