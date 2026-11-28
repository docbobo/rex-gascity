# Rex Session Provider (`gc-runtime-rex`)

`gc-runtime-rex` is a Gas City runtime provider for [Rex](https://rex.dev) that speaks Runtime Provider Protocol v0 (RPP v0). It runs Gas City agents as native Rex terminal panes rather than tmux panes.

## Prerequisites

- `rex` CLI on `PATH` (or set via `GC_REX_BIN`).
- A reachable Rex server (local socket or `REX_SERVER`). Verify connectivity with `rex session ls`.
- Gas City (`gc`) with RPP v0 pack runtime support.

## Install

Install `gc-runtime-rex` onto `PATH` with `go install`:

```sh
go install github.com/docbobo/rex-gascity/cmd/gc-runtime-rex@latest
```

Make sure `$(go env GOBIN)` (or `$(go env GOPATH)/bin` when `GOBIN` is unset) is on `PATH`.

From a local checkout, you can also build into `./bin`:

```sh
make build
```

This writes `bin/gc-runtime-rex`. Prepend `bin/` to `PATH` or copy the binary into a directory on `PATH`.

## Pack and City Configuration

The runtime pack lives at `pack/pack.toml`:

```toml
[pack]
name = "rex-gascity-runtime"
version = "2026.11.08"
schema = 2
description = "Gas City runtime pack for Rex: a native Rex session provider speaking RPP v0."

[runtimes.rex]
command = "bin/gc-runtime-rex"
protocol = 0
```

`command = "bin/gc-runtime-rex"` contains a path separator, so `gc` resolves it
against the pack directory, and the shipped shim installs the matching release
into the version-keyed cache.

### Pool-claim nudge and live claims (GCREX-67 residual risk)

The pack carries no Pi hook override. The core pack's managed
`.pi/extensions/gc-hooks.js` is the only hook, and the runtime pack stages only
the per-provider overlays a city configures through `pack_overlay_dirs`
(`internal/provider/overlay.go` `stagePackOverlays`, pinned by
`TestStagePackOverlaysStagesConfiguredSubtree`). The claim guard against
GCREX-33 now lives only in the provider's `Nudge`.

`gc`'s stalled-claim backstop re-queues the fallback pool-claim nudge (`Run gc
hook --claim --drain-ack --json now; …`) for a pool slot whose routed trigger
bead still reads open (gc v1.5.1 `cmd/gc/idle_nudge.go` `defaultPoolClaimNudge`;
90 s grace, `docs/reference/config.md`). `Nudge` refuses to type that prompt
while the session still holds a live claim (`internal/provider/nudge.go`
`ErrNudgeClaimHeld`, via the `probeLiveClaim` liveness predicate, which mirrors
gc v1.5.1 `cmd/gc/session_ownership.go` `claimedWorkStillHeldBy`: a closed,
released, or foreign-owned claim is not held). Nothing is typed, so a working
seat is not sent back through `gc hook --claim`. The refusal answers exit 80 as
the RPP `busy` input outcome. gc `v1.5.1` has no `input-outcome` vocabulary, so
80 is a plain error there and the delivery attempt is spent, the same cost as
the old exit 1. A gc at `890a58a3e` / `544beb8d6` (#9137 only, not in `v1.5.1`;
inert at the pin) maps the word to `runtime.ErrSessionBusy`, and the
stalled-claim backstop refunds the reservation instead of spending it
(GCREX-69).

Residual risk at the `v1.5.1` pin. gc reserves the delivery attempt before
calling the provider and does not refund a refused nudge whose word it cannot
classify (gc v1.5.1 `cmd/gc/nudge_backstop.go`: `reserve` precedes `sp.Nudge`;
only gc edge refunds `ErrSessionBusy`/pending-interaction). At the cap
(`idleClaimNudgeMaxAttempts = 3`, gc v1.5.1 `cmd/gc/idle_nudge.go:54`) v1.5.1
only gives up and logs (`poolClaimBackstop.exhausted` is a no-op at the pin),
but gc edge drains and recycles the slot (`cmd/gc/idle_nudge.go`
`poolClaimBackstop.exhausted`, working tree). A seat that stays on one live
claim across three backstop ticks can therefore be drained on gc edge. On a gc
edge at `544beb8d6` (#9137, post-`v1.5.1`) the `busy` refusal is refunded, so
the drain risk remains only for a gc edge that predates that commit, or for a
non-`busy` failure.

Already-staged work directories. gc's `hooks.PreserveManagedFile` predicate
decides from the marker already on the destination (gc v1.5.1
`internal/hooks/preserve.go:29`), so a worktree or shared checkout that the
removed overlay already staged keeps its `GC_PI_HOOK_VERSION = 100`
`.pi/extensions/gc-hooks.js` and its old claim guard. Remove that file by hand
from each such work directory to fall back to gc's managed hook.

Include the pack in your city's `city.toml` (paths in `includes` resolve relative to `city.toml`):

```toml
[workspace]
name = "my-city"
includes = ["path/to/rex-gascity/pack"]
```

Select the provider for the whole city in `city.toml`:

```toml
[session]
provider = "rex"
```

Or select it for a single invocation without editing `[session]`:

```sh
GC_SESSION=rex gc start
```

`GC_SESSION` only applies to the process that builds the session provider. For a supervised city, the supervisor or controller that is already running picks the provider from its own environment, so `GC_SESSION=rex gc start` has no effect there. It works only when `gc start` runs a foreground controller (or another process) that constructs the provider itself. For managed cities, set `[session] provider = "rex"` in `city.toml`.

### Validating the Pack Runtime

Validate the pack registration and RPP implementation with `gc`'s built-in checks:

```sh
GC_REX_CITY=rex-verify gc runtime check rex --city /path/to/city
GC_REX_CITY=rex-verify gc runtime conformance rex --city /path/to/city
```

From this repository, `make check` and `make conformance` build `bin/gc-runtime-rex` and run both suites against `testdata/city`.

Always verify that the output includes `resolved runtime "rex" from pack ...`. If `city.toml` fails to load, `gc runtime check rex` falls back to any bare `rex` executable on `PATH` instead of running `gc-runtime-rex`.

### Doctor Check

The pack registers a `gc doctor` check named `rex-gascity-runtime:rex-binary` (from
`pack/doctor/rex-binary/`). Run it directly, or as part of the full sweep:

```sh
gc doctor --check rex-gascity-runtime:rex-binary --city /path/to/city
gc doctor --city /path/to/city
```

The check:

- resolves the runtime command from `pack/pack.toml`, accepting a bare name on
  `PATH` or a path relative to the pack directory;
- confirms the provider answers the `protocol` op with an RPP v0 handshake;
- compares the provider's reported version against the `[pack] version` in
  `pack.toml` (see [Version match](#version-match));
- resolves the `rex` CLI, honoring `GC_REX_BIN`.

It exits `0` when the provider matches and both binaries are present, `1` for a
warning, and `2` when the provider binary or the `rex` CLI cannot be resolved,
or the provider's version disagrees with the pack. The first stdout line is the
message; later lines name the missing binary and give a remediation. A missing
provider otherwise surfaces only as opaque agent state (`unknown (partial
status)`) and a supervisor log line:

```text
exec: "gc-runtime-rex": executable file not found in $PATH
```

A supervisor or controller resolves the provider with its own `PATH`, so the
directory that holds `gc-runtime-rex` must be on that environment's `PATH`, not
just your interactive shell's.

#### Version match

gc resolves and handshakes the provider but never compares the binary it
installed against the version the pack declares, so a pack upgrade that leaves
an older `gc-runtime-rex` on `PATH` shows up only as an opaque runtime error.
The check closes that gap: the provider answers the `version` op with one JSON
line carrying the module version and VCS revision it was built from —

```json
{"module":"github.com/docbobo/rex-gascity","version":"0.1.0","revision":"e6f743b8b2a0"}
```

A clean build omits `modified`, and a build with no VCS revision omits
`revision`; the provider prints only the fields it resolved. The check
compares that `version` against the `[pack] version` in
`pack.toml`. The semantics are:

- **Match (exit 0)**: the reported version equals the declared version after a
  leading `v` and any `+build-metadata` suffix are stripped, so a tagged
  `v2026.11.08` and an `-ldflags`-stamped `2026.11.08` both match `version = "2026.11.08"`.
- **Mismatch (exit 2)**: a determined version that differs, with a remediation
  naming the pack's version.
- **Undeterminable (exit 1)**: the provider reports no version or has no
  `version` op. This is the no-VCS/no-module-info case: the toolchain embeds
  the placeholder `(devel)`, which the provider reports as an empty version. A
  bare local `go install` from a Git checkout is not this case — it reports a
  module pseudo-version and so is a determined mismatch (exit 2). The binary
  still handshakes, so this warns rather than errors.
- **Local and dirty builds**: `make build` and `make install` stamp the pack's
  version with `-ldflags "-X main.buildVersion=<version>"`, so a build from
  this repository matches. A `go install` from an untagged tree reports a
  module pseudo-version, which does not match the declared release and so
  errors; a build from a modified tree reports `"modified":true` and, when its
  version still matches, passes with a note.
- **No declared version**: when `pack.toml` has no `[pack] version`, the check
  has nothing to compare against, so it skips the comparison and reports only
  the handshake result.

## Protocol and Capabilities

`gc-runtime-rex protocol` returns the RPP v0 handshake:

```json
{"version":0,"locality":"local","capabilities":["report-attachment","report-activity","idle-boundary","carrier.ops","input-outcome","observe.v1","city-scoped"]}
```

- **Locality**: `"locality":"local"` states that Rex boxes run on the
  controller host under the same user. The server is a unix socket under the
  controller's `Application Support`, and state is a local spool. gc reads the
  field through `ProtocolInfo.DeclaresLocal` (`internal/runtime/protocol.go:213-233`).
  An absent or other value means remote, and `sessionRuntimeIsRemote`
  (`cmd/gc/storage_verb_exclusion.go:314`) then refuses a storage split without
  `--force=remote-runtime`. The declaration is a statement of deployment fact.
  A provider pointed at a remote Rex server through a tunnel would make it
  false.
- **Declared capabilities**: `report-attachment` (`is-attached`), `report-activity` (`get-last-activity`), `idle-boundary` (`idle`), `carrier.ops` (the five session ops gc drives instead of exec) and `input-outcome` (the typed exit-80 words on driving ops; gc 1.5.1 ignores it and reads a refusal as a plain error, a gc at `544beb8d6` (#9137, post-`v1.5.1`) reads the word and refunds a `busy` nudge), plus `observe.v1` (the reconciler-v2 profile: `observe`, `inventory`, `kill-object` and `fresh_only`) and `city-scoped` (real per-city state isolation).
- **Capabilities not declared**: `proc.exec`, `proc.provision`, `proc.stream`,
  `tty.attach`, and `fs.host`. `fs.host` adds work-dir staging rules the
  provider does not implement. Locality alone states the deployment fact.

Implemented RPP operations: `protocol`, `start`, `stop`, `attach`, `is-running`, `interrupt`, `nudge`, `send-keys`, `clear-scrollback`, `process-alive`, `peek`, `list-running`, `get-last-activity`, `idle`, `program-status`, `is-attached`, `set-meta`, `get-meta`, `remove-meta`, `observe`, `inventory`, `kill-object`.

Two of these are Rex-only, and `docs/REX-CONTRACT.md` §9 has their contract:

- `idle <name>` (GCREX-63) answers one word, `idle`, `busy` or `indeterminate`, from the pane's `program_status` and screen. It is gated by `idle-boundary`. A failed read exits 1 and is never `idle`.
- `program-status <name>` (GCREX-64) answers one JSON line with the judged OSC 7501 reading: `validity` (`valid`, `not_armed`, `exited`) and, when valid, `state` and `kind`. No token gates it. It stays inert until gc's exec backend reads a `ProgramStatusProvider`.

## Identity Mapping

Gas City sessions map onto the three-level Rex hierarchy. Every generated
label carries a bracketed kind tag; the name after it is the identity.

| Gas City | Rex | Label / Rule |
|---|---|---|
| City | Session | `[city] <city>` (one Rex session per city namespace); the pre-tag `gc:<city>` is still accepted |
| Rig | Tab (window) | `[rig] <GC_RIG>` whenever `GC_RIG` is non-empty; otherwise `[rig] <rig>` for `<rig>--<agent>`, or `[city] <pin>` for town-level names with no `--` (`[city] Gas City` when no pin is set). The pane label never decides the tab |
| Agent | Pane (block) | `[<agent>] (<session>)`; the agent is the agent part of `GC_TEMPLATE` (else `GC_ALIAS`, then `GC_AGENT`, then the `BEADS_ACTOR` segment after the first `/`). An unaliased pool seat whose session name verifies as `<agent>-<bead id>` gets `[<agent>] (#<bead id>) #<session id>`; every other session keeps the long `(<GC_SESSION_NAME>)` form, and a town-level start keeps `[<agent>] (<name>)`. A start with no resolvable agent keeps the bare session name |

- The bracketed text names the kind of identity that follows; only a pane with
  no agent identity stays bare (`mayor`, `shell`). One parenthesis rule serves
  all three levels: the contents of `(...)` are the identity.
- Starting the first agent in a rig creates a new Rex tab labeled
  `[rig] <rig>`; a start whose env carries `GC_RIG` joins `[rig] <GC_RIG>` even
  when its runtime name has no `--` (a wisp instance such as
  `gc__implementation-worker-mc-wisp-s5gax5`). A start with no `GC_RIG` and a
  town-level name opens a tab labeled with the city pin (`[city] my-city`), or
  `[city] Gas City` when the process has no pin.
- Session names are unchanged; the session *label* gains the tag. A pane label
  is display-only (see below).
- The pane label is `"[<agent>] (<suffix>)"` (GCREX-5qel, gcrex-7n06r):
  - R1 town-level start (no `GC_RIG`) is `[<agent>] (<name>)`, with the
    session name in the parentheses and no display tail;
  - R2 rig-scoped agent prefix is the agent part of `GC_TEMPLATE`
    (`path.Base`): `gc.implementation-worker`, `core.control-dispatcher`.
    When `GC_TEMPLATE` is absent it falls back to `GC_ALIAS`, then
    `GC_AGENT`, then the `BEADS_ACTOR` segment after the first `/`;
  - R3 suffix: an unaliased pool/ephemeral seat whose session name verifies as
    `Sanitize(agent)+"-"+bead id` reads
    `[<agent>] (#<bead id>) #<session id>` — the `#` inside the parentheses
    marks the pool identity, and the trailing `#<session id>` is display-only;
    a named/aliased session, or any name the env cannot verify, uses the whole
    `GC_SESSION_NAME` in the parentheses, escaped with one `\` when the name
    itself starts with `#` (see below). The suffix candidate is the bead id
    from `GC_AGENT` (a value containing `/` or `.` is a qualified agent, never
    a bead id), else `GC_SESSION_ID`; a seat with neither falls back to the
    template's agent part and the verification fails, so it gets the long form;
  - R4 a pane with no gc env (started by hand, `shell`) is never relabelled.

  A blank session name or agent keeps the name-derived label, and an
  over-long label is clamped to 128 runes. The two forms shorten differently.
  The pool form drops its display-only `#<session id>` tail first, keeping
  `[<agent>] (#<bead id>)`, and never cuts the agent: if the marked identity
  does not fit, the long form takes over. The long and town forms keep the
  identity and cut the agent with `…` to fit, first with the display tail and
  then without it; only a suffix that cannot fit at all falls back to the bare
  session name. A cut agent ends in `…` (`paneLabelShortened`), so the cut
  stays visible.
- How a short label is verified: gc's pool runtime name is
  `Sanitize(path.Base(GC_TEMPLATE)) + "-" + beadID` (`PoolSessionName`,
  `cmd/gc/pool_session_name.go` at v1.5.1), with `Sanitize` mapping `/` to
  `--` and `.` to `__` (`internal/agent/session_name.go`). Before emitting the
  pool form the provider asserts `Sanitize(agent)+"-"+suffix ==
  GC_SESSION_NAME`; when that fails (gc has changed its name shape before) it
  emits the long `"[<agent>] (<GC_SESSION_NAME>)"` form rather than a guess.
  The candidate suffix is only tried: without `GC_AGENT` and `GC_SESSION_ID`
  the fallback candidate is the template's agent part, so the check fails and
  the long form is emitted.
  Discovery applies the same rule (`identityMatch`): the marked short form
  (tagged `[agent] (#<bead id>)` or the legacy untagged `agent (#<bead id>)`)
  names session `n` when its identity is exactly
  `Sanitize(agent)+"-"+bead id`; an unmarked suffix names only the session
  whose complete name equals the parentheses. The marker is what keeps a
  label to one session: without it `[worker] (seat)` would name both the town
  session `seat` and the pool seat `worker-seat`. Shortening the agent keeps
  the identity — the parenthesis survives the cut — and the trailing `#` tail
  is display-only: it is never compared, so it alone never names a session.
  A complete session name that itself starts with `#` is escaped in the
  suffix: `[worker] (\#seat)` names the session `#seat`, while
  `[worker] (#seat)` keeps naming only the pool seat `worker-seat`. The
  writer puts one `\` in front of a name that starts with `#` or with `\`
  itself, and the reader strips exactly one and never treats an escaped
  suffix as a pool identity, so one label still decodes to one session. The
  escape is part of the suffix, so it survives shortening and the 128-rune
  clamp the same way the name does. This is what lets a rig-scoped `#seat`
  keep a name-bearing label: a record-less scan cannot derive the rig tab
  (`[rig] Indigo`) from the name, so the pane must still answer to it.
- The launch label, the discovery hint and the recorded label all come from the same pool-routing-projected env (`launch.Project`), the one the pane itself runs with. Deriving the launch label from the pre-projection env would disagree with the hint whenever routing rewrites `BEADS_ACTOR`, and a restart with the sidecar record lost would launch a duplicate pane.
- With no sidecar record and no start env (`is-running`, `stop`), the fallback scan accepts a pane label whose parenthesised suffix names the session — the long `(<GC_SESSION_NAME>)` form (its suffix escaped with one `\` when the name starts with `#`), or the verified pool short form `Sanitize(agent)+"-"+bead id == name` (written `#<bead id>`) — as well as the exact name-derived label, so a live pane is found without any start env. A shortened short form names nobody: such a pane is left alone rather than risking another session's stop.
- A pane started by a build before GCREX-72 still sits in a tab labelled `__town` or the bare rig name (`Indigo`). Resolution, discovery and `start` accept that label for a record with no stored tab label or a scan that finds no derived one, so an upgrade reuses the live pane instead of opening a second tab. A record that carries a stored tab label is scanned in that tab alone: the same pane label in another tab is a different binding, never an adoption. New tabs always use the labels above.
- Starting more agents in the same rig splits panes into that rig's existing tab. Each new pane splits the largest-area pane along its longer axis (horizontal when width >= height, else vertical) at 50%, so panes stay balanced. If the layout has no usable geometry, it falls back to splitting the first block horizontally.
- `stop` closes only the city-owned agent block (`rex block close`). Closing the last pane in a tab removes that tab automatically in Rex. Teardown never stops the shared Rex server and never destroys a session: the live probe hit the plan's approved skip branch because the Rex app is listed on every session of the shared server, so the empty canonical `[city] <city>` shell stays listed and the next `Start` reuses it (`docs/REX-CONTRACT.md` §12).

### Rig from the start config

gc 1.5.1 starts pool and wisp instances under a runtime name that carries no
`--` (`pi-mc-wisp-ybq1`, `core__control-dispatcher-mc-wisp-6znw`); the rig
arrives out of band in the start config `env` as `GC_RIG`. The provider
therefore derives the Rex tab label as:

- a non-empty `GC_RIG` wins regardless of the name shape: `[rig] <GC_RIG>`,
  including a town-level name with no `--`;
- with no `GC_RIG`, a town-level name uses the town label: `[city] <pin>`, or
  `[city] Gas City` when no pin is set;
- with no `GC_RIG`, a rig name (`<rig>--<agent>`) uses `[rig] ` plus
  `SessionName(name).Rig()`.

Every generated label is clamped to Rex's 128-character limit, except the
canonical session label: `[city] <city>` keeps the city identity whole, because
two long city names that shared a truncated label would fold into one Rex
session.

The pane label is `[<agent>] (<suffix>)` for a rig-scoped session, with the
agent resolved from `GC_TEMPLATE` (agent part) → `GC_ALIAS` → `GC_AGENT` → the
`BEADS_ACTOR` segment after the first `/`. An unaliased pool seat whose name
verifies as `Sanitize(agent)+"-"+bead id` gets the marked short
`[<agent>] (#<bead id>)` suffix; every other session keeps
`[<agent>] (<GC_SESSION_NAME>)`. A blank session name or agent keeps the
name-derived label, and an over-long label is clamped as described above. A
shortened agent ends in `…`, and that marked label names no session.
`GC_SESSION_NAME` is load-bearing for the long label.

At `start` the provider records the pane's binding under the runtime name it
was started with, plus rig-qualified aliases derived from the start config
(`Sanitize` maps `/` to `--` and `.` to `__`):

- `Sanitize(GC_TEMPLATE)` when `GC_TEMPLATE` is non-empty (for example
  `my-project--pi`, `my-project--core__control-dispatcher`);
- `Sanitize(GC_ALIAS)` when `GC_ALIAS` is non-empty (usually the same as the
  template alias);
- `Sanitize(GC_AGENT)` when `GC_AGENT` is qualified (contains `/` or `.`);
- `Sanitize(GC_RIG + "/" + name)` when `GC_RIG` is non-empty and the start
  name is town-level (no `--`, for example `my-project--pi-mc-wisp-ybq1`).

Empty names and duplicates are dropped; each alias points at the same pane as
the primary binding.

`IsRunning`, `Stop`, `ClearScrollback`, `Peek`, `Binding`, and the metadata ops
resolve through those aliases. `IsRunning` is true when the name is the primary
binding or an alias of any live primary binding that answers to it.
`ListRunning(prefix)` emits a live primary name when it matches the prefix,
plus each matching alias when the prefix is non-empty, sorted and deduped. This
is what lets gc's rig-qualified probes (`is-running
my-project--core__control-dispatcher`, `list-running my-project--pi-`) resolve
to live instances. The empty prefix deliberately lists primary names only:
gc's adoption barrier and dead-session reconciler treat every name returned by
`list-running` as a real session, so emitting an alias there would mint a
phantom orphan session bead.

Bindings written before this change (or without a start-config env) carry no
stored aliases and keep resolving by the derived tab label (the town label for
town-level names, `[rig] <rig>` or the legacy `Rig: <rig>` from the name split
otherwise) and the pane label.

## Sidecar State and Environment Variables

Because RPP invokes `gc-runtime-rex` as a short-lived process per operation, pane bindings, activity observations, and session metadata are stored on disk per city.

### State Directory Resolution

Base directory precedence:

1. `$GC_REX_STATE_DIR` (if non-empty)
2. `$XDG_STATE_HOME/rex-gascity` (if `XDG_STATE_HOME` is set)
3. `~/.local/state/rex-gascity`

Within that base directory, each city uses `<base>/<encoded-city>/` (percent-encoded so distinct city names always map to distinct directories, including on case-insensitive filesystems such as macOS APFS: only `a-z`, `0-9` and `-` stay literal, and every other byte, including uppercase letters, becomes `%XX`), containing `bindings/`, `activity/`, `meta/`, and an
`flock` file `lock`. Starts and stops also take a per-name flock under
`locks/<name>.lock`, and the launch takes a per-rig flock under
`locks/tab-<rig>.lock`; those serialize one name (or one shared tab) without
holding the city-wide `lock` across a hook or the blocking launch.

The dispatcher resolves one canonical namespace and, when needed, reads a small
ordered set of legacy namespaces:

- **Canonical:** gc's per-op city name `$GC_EXEC_CITY_NAME`, else
  `basename($GC_EXEC_CITY_PATH)` (GCREX-71), else `$GC_REX_CITY`, else
  `"default"`. New sessions, new bindings, metadata writes and the state lock
  use this namespace. gc's two variables outrank the provider's own pin,
  because gc sets them on every op it runs the provider for.
- **Read set:** the canonical namespace first, then `basename($GC_CITY_PATH)`
  and `$GC_CITY` when they differ. Reads fall back through the set in order;
  deletes cover every directory. A record written by an older build (or by a
  caller that carried `GC_CITY_PATH`) therefore still resolves.
- **Unpinned discovery (GCREX-35):** when no city pin is set, the read set
  also includes the sibling city state directories under the same base. Each
  directory name is decoded (the inverse of the percent encoding) and only a
  name that re-encodes to the same directory is trusted; duplicates of an
  existing candidate are skipped and the rest are sorted after the env-derived
  entries. This is what lets the gc CLI fallback provider — invoked with none
  of `GC_EXEC_CITY_NAME`, `GC_EXEC_CITY_PATH`, `GC_REX_CITY`, `GC_CITY_PATH` or
  `GC_CITY` at all — resolve a binding the pinned controller wrote. Discovery
  only widens reads: a live pane still has to revalidate against its
  `[city] <city>` Rex session, so a dead city's record is never reported as
  running. The relaxation is that an unpinned caller cannot
  name a city, so if two live cities hold the same session name, the name
  resolves in read-set order (canonical `default`, then env fallbacks, then
  sorted siblings). Pin a caller with `GC_REX_CITY` to keep it isolated.
- **Pinned:** when a city pin is set (`GC_EXEC_CITY_NAME`,
  `GC_EXEC_CITY_PATH` or `GC_REX_CITY`) the read set is exactly that one
  namespace, so cities pinned to different values never cross-resolve and
  pinned callers never read sibling directories.

### City Namespace Resolution

RPP v0 passes no city identifier on argv or stdin, and the exec runtime provider
passes the caller's environment through unchanged. The controller, session
processes and the nudge poller therefore see different incidental `GC_CITY*`
values for the same city. To keep the provider from disagreeing with itself,
the canonical namespace is caller-independent:

1. `$GC_EXEC_CITY_NAME` when set (gc's name for the city it serves)
2. `basename($GC_EXEC_CITY_PATH)` when the name is absent and that path names a
   directory (gc edge sets both on every op)
3. `$GC_REX_CITY` when set (the provider's own pin, for running outside gc)
4. `"default"` otherwise

The pin is a city **name**, never a path: gc's absolute
`GC_EXEC_CITY_PATH` names the city by its last path component, so the Rex
session label stays `[city] my-city` and a city's sidecar state stays in one
namespace. Using the raw path as the pin made the label `[city] /Users/…/my-city`
and split the state the moment gc started setting the variable (GCREX-71).

`basename($GC_CITY_PATH)` and `$GC_CITY` do **not** select the canonical store;
they are only additional read namespaces for records written by earlier
versions.

An unpinned caller additionally discovers the city pins that share its state
base: sibling directories are decoded back to city names and appended to the
read set (GCREX-35). Without this, a `GC_REX_CITY`-pinned controller writes its
bindings under e.g. `my-city/`, while a shell with no city env resolves only
`default/` and every lookup answers "not running" for a live pane. The fix is
provider-side because gc's CLI fallback provider invocation carries no city
environment at all; the provider has to find the pin, and revalidation keeps
the widened read honest.

### Environment Knobs

| Variable | Default | Purpose |
|---|---|---|
| `GC_REX_BIN` | `rex` (resolved via `PATH`) | Path or name of the `rex` CLI binary used by the provider |
| `GC_EXEC_CITY_NAME` | unset | gc's per-op city name; pins the canonical city namespace (highest precedence) |
| `GC_EXEC_CITY_PATH` | unset | gc's per-op city path; names the city by its last path component when `GC_EXEC_CITY_NAME` is unset |
| `GC_REX_CITY` | `"default"` | Pins the canonical city namespace for the `[city] <city>` Rex session label and sidecar state subdirectory; also removes all fallback read namespaces |
| `GC_REX_STATE_DIR` | `$XDG_STATE_HOME/rex-gascity` or `~/.local/state/rex-gascity` | Overrides the base directory for per-city sidecar state |

### Start-config `GC_*` environment

RPP `start` carries the launch config as JSON on stdin, including an `env` map.
gc 1.5.1 ships every managed-session variable there. The provider's durable
contract is exactly four of them: `GC_RIG`, `GC_TEMPLATE`, `GC_ALIAS`, and
`GC_AGENT` (used to derive the tab and the aliases above). The rest are
incidental to the provider and are applied to the launched process unchanged
(GCREX-10). Empty values in the start-config env are applied as unset
(`env -u KEY`) at the launch boundary, matching RPP and gc's tmux adapter. An
omitted `env` is the same as an empty one: the provider normalizes the projected
env to an empty map, so a fresh start resolves against the labels the name
implies instead of a stale record's labels and does not launch a duplicate.

For an unaliased pool or demand seat the provider keeps `GC_ALIAS` empty and
routes the seat's **work-query agent identity** back to `GC_TEMPLATE` by setting
`BEADS_ACTOR` to the template. `GC_AGENT`, `GC_SESSION_ID`, the tokens and the
caller's map are untouched, and the empty `GC_ALIAS` is emitted as
`env -u GC_ALIAS` at the launch boundary. The pane still answers to the template
name: the template is recorded as a binding alias independently of `GC_ALIAS`.

Two gc 1.5.1 behaviours make this the shape that works, and a live city run
proved both (see GCREX-32 notes):

- **A blank `GC_ALIAS` alone cannot discover the routed step.** gc's hook
  resolves the work-query agent from `firstNonEmpty(GC_ALIAS, BEADS_ACTOR,
  GC_AGENT, GC_SESSION_NAME)` (`hookSessionAgentForQuery`,
  `cmd/gc/cmd_hook.go:818`), and `hookWorkQueryStores`
  (`cmd/gc/hook_cross_store.go:107`) resolves the rig from that agent. gc ships
  `GC_ALIAS` empty and `GC_AGENT`/`BEADS_ACTOR` as the session bead id, so the
  query runs against an identity that resolves to no rig and never reads the
  store the routed step lives in (gastownhall/gascity#5716). With `GC_ALIAS`
  empty and `BEADS_ACTOR` set to the template the query reads the rig and city
  stores and finds the step.
- **`GC_ALIAS` must stay empty for the claim.** gc's claim writer
  `hookClaimAssigneeIdentity` (`cmd/gc/cmd_hook.go:866`) records a claim under
  `alias` first, so a synthesized `GC_ALIAS` makes every slot claim under one
  shared string that no open session bead answers to. A second seat then adopts
  the first's `in_progress` bead
  (`hookClaimExistingAssignment`/`certifyHookAdoption`,
  `cmd/gc/cmd_hook_claim.go:472-477`), and `releaseOrphanedPoolAssignments`
  (`cmd/gc/pool_session_name.go:211`) reopens the claim, respawning the pool so
  a duplicate seat can close the shared step and drain the real worker mid-edit
  (the GCREX-18 deviation). With `GC_ALIAS` empty the claim is recorded under
  `GC_SESSION_ID`, the concrete session bead id, which a live session answers to
  (`sessionBeadAssigneeIdentities`, `cmd/gc/pool_session_name.go:29`) and which
  every slot holds its own copy of, so seats single-flight.

On `stop`, if the pane's work dir is a git work tree with modified tracked
files, the provider writes `<state dir>/diagnostics/<name>.json` naming the
worker, its aliases, its `GC_SESSION_ID`, the changed files and the file edited
last. Untracked files are ignored. The check runs before the pane is closed (or
from the stored binding when the pane is already gone), and git runs with
`GIT_OPTIONAL_LOCKS=0` so it never takes the index lock. This leaves a trace
when gc drains a worker that still held edits. It is best effort: both git calls
share one 3 s budget, every error is swallowed, and a clean tree, a non-git dir,
or an unset state dir writes nothing and never affects `stop`.
The full inventory (meaning, when set, durable vs incidental) and the observed
RPP session-name forms are in `docs/REX-CONTRACT.md` §13.

### Startup timing diagnostic (GCREX-58)

Every `start` that delivers a nudge writes
`<state dir>/diagnostics/<name>-startup-timing.json`, so a slow start can be
attributed to a stage instead of guessed at. gc records only the total
`start_resolved` duration; this file separates the stages, in whole
milliseconds:

| Field | Meaning |
|---|---|
| `launch_ms` | the pane launch (`NewSplit`/`NewWindow`) |
| `readiness_ms` | the `program_status` readiness wait |
| `readiness_outcome` | how the wait ended: `idle`, `window-bound` (a status record was present but never idle before the 15 s window), `settle-bound` (no status record at all before the 3 s settle delay), `prompt`/`prompt-bound` (a configured `ready_prompt_prefix` appeared, or its window elapsed), `dialog` (the last read showed a startup dialog, so the seed was not typed, GCREX-59, #205), `dialog-cleared` (a dialog was seen, then a poll showed a clear screen and a ready verdict. That poll can be the deadline poll, the first one whose screen read returned at or after the deadline. Readiness is then resolved, and the seed goes to the delivery ladder. A gate there can still stop it before it types), `dialog-unresolved` (a dialog was seen, and the clear wait ended without a delivery: the deadline poll's read was clear but not ready or failed, or the `pre-write` gate at or after the deadline failed to read), `ended` (the pane's program had already exited, failing a non-`one_shot` start before its first turn), `canceled`, or `disabled` |
| `write_ms` | the first nudge text write |
| `confirm_ms` | the first post-Enter confirmation poll |
| `confirmed` | whether the first attempt's turn start was observed |
| `attempts[]` | one entry per resend: `attempt` (2-based), `backoff_ms`, `write_ms`, `confirm_ms`, `confirmed` |
| `dialog` | the startup dialog of the most recent sighting, by its stable name (the first marker of its first group, or `rate-limit`/`rate-limit-menu`). Only when this start saw a dialog |
| `dialog_screen` | the screen tail that sighting read, the last 4096 bytes. Raw pane text that can hold seed text after a write, so it stays in this private file |
| `dialog_gate` | `pre-write`, `pre-submit` or `pre-resend` when the startup dialog gate saw it. Absent when the readiness wait did |
| `dialog_suspect` | `seed` when the match may be the seed's own echo (the seed's text alone matches that dialog). Otherwise absent |
| `total_ms` | `launch_ms` plus everything from readiness on |

`readiness_outcome` is the load-bearing field: `idle` means the wait returned
as soon as the program reported ready, while `window-bound`/`settle-bound` mean
the wait was capped by the bound, so cutting the bound is (or is not) a lever.
A `dialog` or `dialog-unresolved` outcome means the seed was never typed.
`dialog-cleared` means a dialog was seen, then a poll had a clear read and a
ready verdict no later than the deadline poll. That poll can be the deadline
poll. Readiness is resolved, and the seed goes to the gated ladder. A gate
there can still stop it before it types (see "Startup seed and dialogs").
`readiness_ms` covers the readiness wait and any clear wait. `prompt`/`prompt-bound` are the `ready_prompt_prefix`
twin of `idle`/`window-bound`.
An `ended` outcome is not a bound: the pane's program was already gone, and a
non-`one_shot` start fails with `the agent exited during startup, before its
first turn`.
The file is best effort and never changes the start result; an unset state dir
writes nothing, and a start that delivers no nudge (empty `nudge`, no launch,
an already-alive pane) writes nothing.

gc also reads provider metadata over `get-meta` (not the start env):
`GC_PROVIDER`, `GC_SESSION_ID`, `GC_DRAIN_ACK`, `GC_ALIAS`, `GC_AGENT`, and
`suspended`. The provider serves these from its metadata sidecar. `set-meta`,
`get-meta`, and `remove-meta` resolve a rig-qualified alias to the pane's
primary name, because gc reads some keys (`suspended`, `GC_SESSION_ID`) under
the canonical identity even when the pane was started under a short pool name.

### Startup seed and dialogs (#205)

Under RPP v0 the exec script owns the startup seed (`nudge`). gc's exec
`Start` runs the script's `start`, then `dismissStartupDialogs`, then returns.
It never sends the seed again (gc v1.5.1
`internal/runtime/exec/exec.go:211-231`, edge 890a58a3e
`internal/runtime/exec/exec.go:462-514`). gc stamps `primed_at` from its
routing plan, not from delivery (edge
`cmd/gc/session_lifecycle_parallel.go:3311-3315`,
`cmd/gc/startup_prompt_delivery.go:63-70`). Neither v1.5.1 nor edge reads
`GC_STARTUP_DELIVERY_UNCONFIRMED` (`git grep` finds no hit). gc's queued-nudge
backstop covers pool slots only (edge `cmd/gc/idle_nudge.go:88-136`). So for a
named session such as the mayor, the marker is the only signal that the seed
did not land. The second source for this behavior is `make test` and
`make conformance` (ADR-0011).

- **Screen tail.** The startup dialog check, the dialog gate and the
  prompt-mode ready check read only the last 120 lines of the block text.
  gc's `startupDialogPeekLines` does the same (gc v1.5.1
  `internal/runtime/dialog.go:18`). `IdleSnapshot` and the nudge ladder still
  read the whole screen.
- **Clear window.** When the startup wait first sees a dialog, Start keeps
  polling for a clear window (15 s by default). It never types, and it
  measures the window from that first sighting. It delivers the seed through
  the normal ladder only after a poll with a successful screen read, no
  dialog, and a ready verdict. A ready verdict is an `idle` record past the
  `ready_delay_ms` floor, or the configured prompt (`dialog-cleared`). After a
  sighting, the bound fallbacks (`window-bound`, `settle-bound`,
  `prompt-bound`) no longer deliver.
- **Deadline.** The deadline poll is the first poll at or after the deadline.
  It can still deliver. Otherwise the seed is not typed and `start` exits 0.
  It never fails, because gc would stop a healthy pane
  (`cleanupAfterStartFailure`, gc v1.5.1 `exec.go:223-228,262-270`). A read
  showing the dialog gives `dialog` and
  `withheld: the pane shows a startup dialog (<name>)`. A clear-but-not-ready
  or failed read gives `dialog-unresolved` and
  `withheld: a startup dialog was seen (<name>); the pane was not clear and ready when the clear window ended`.
- **Hard stop.** After a sighting, every Rex call of the wait runs under a
  context that ends at the deadline plus one poll interval. This includes the
  pre-write gate read. A call cut off there is a failed read. The hard stop
  bounds those Rex calls only. Once the pre-write gate shows a clear screen,
  the ladder runs under its own contexts. The marker and timing-file writes
  run after the capped phase and are outside it.
- **Outer limit.** Nothing bounds the time from the start of the wait to the
  first sighting locally. The readiness wait plus the clear window take about
  30 s with the defaults. That is an estimate, not a bound. gc's 120 s
  `startTimeout` kills the whole script, launch and ladder included. It is a
  separate outer limit (gc v1.5.1 `exec.go:204-206`).
- **Dialog gate.** Startup delivery reads the screen tail before each input
  call. It reads at `pre-write` before the first write and at `pre-submit`
  before every submit key (under the submit key's own context). At
  `pre-resend` it reads before each resend's `ctrl+u` and again before its
  write. A dialog there, or a failed read after this start saw a dialog,
  sends no input. Before any sighting, a failed read fails open. At
  `pre-write` the wait resumes polling until the same
  deadline only when the gate read returned before the deadline and the clear
  wait is enabled. Otherwise the wait ends there with no input. A dialog gives
  `dialog`, and a failed read gives `dialog-unresolved`. At
  `pre-submit` the marker reads
  `delivery failed: the seed was typed but not submitted, <reason>`. At
  `pre-resend` it reads
  `delivered but not confirmed: <reason> before a resend`. The unreadable
  form is `the screen could not be read after a startup dialog was seen
  (<name>)`. Ordinary `nudge`
  calls run no gate. The gate is a snapshot check, not an atomic one. The read
  and the input call are separate Rex calls, so a dialog that draws between
  them is not caught. A stop leaves the readiness outcome the wait reached,
  for example `idle` or `dialog-cleared`. A start that typed the seed reads
  as delivered, not `canceled` (#208); a parent cancel before the first write
  still gives `canceled`.
- **Evidence.** The startup-timing file records `dialog`, `dialog_screen`
  (last 4096 bytes), `dialog_gate` and `dialog_suspect` for the most recent
  sighting. `dialog_screen` can hold seed text after a write. It stays in that
  private file and never goes into a marker or a log line.
- **Residual cases.** A dialog that only gc answers after `start` returns
  leaves the seed undelivered while gc reports the session primed. Making
  `primed_at` follow delivery is an upstream gc change. A seed that quotes a
  dialog marker (for example `trust this folder`) can match its own echo
  after the write. The gate then stops with
  `a startup dialog may have appeared (<name>); the seed contains its marker text`
  and `dialog_suspect=seed`. The seed stays typed but unsubmitted.
- **Unverified.** Whether the #205 incident's match was a real dialog or a
  scrollback false positive, and whether gc runs `dismissStartupDialogs` for
  `builtin:pi`. The new evidence fields answer the first one next time.

ADR-0021 records why the recovery runs inside `start`.

### Identity metadata before launch

`start` seeds the start-config identity keys into the metadata sidecar and
records a provisional binding **before** the blocking `NewWindow`/`NewSplit`
call. gc's pending-create ownership check reads `GC_SESSION_ID`,
`GC_INSTANCE_TOKEN`, and `GC_RUNTIME_EPOCH` through `get-meta`; without the seed,
a reconciler tick during Rex's startup-dialog dismissal sees an empty identity
and rolls the fresh pending create back as "live runtime belongs to another
session". The provisional binding carries no block ID yet, but lets `get-meta`
resolve the runtime name and its rig-qualified aliases. The final binding
replaces it when the launch returns. A failed start clears the seeded identity
and provisional binding; `stop` clears the same keys before forgetting the pane,
so a prior life's token cannot outlive it. ADR-0015 records the decision.

### Wrapped-gc process names

The provider captures `GC_BIN` from the start config and persists its basename
on the pane binding. `process-alive` expands the incoming gc hints with that
basename when a hint is `gc` (and with gc's `kimi` -> `kimi-code` alias), so a
control dispatcher launched as `$GC_BIN` (`/nix/store/.../bin/.gc-wrapped`, not
`gc`) is still observed as a live agent. Without the expansion gc reads the
live pane as a dead/zombie seat and closes the session bead without issuing
`stop`, leaving the pane and its process behind. This mirrors
`runtime-herdr-core`'s `expand_names`; ADR-0016 records the decision.

## Known v1 Limits

- **No city in RPP v0 invocations (GCREX-17, extended by GCREX-35)**: An RPP call carries only the operation and agent session name, and the caller's `GC_CITY*` values are incidental. The canonical namespace is `GC_REX_CITY` when set, else `"default"`; `basename(GC_CITY_PATH)` and `GC_CITY` are read-only fallbacks for legacy records. An unpinned caller also discovers the sibling city state directories under its base, so a binding written under a `GC_REX_CITY` pin still resolves when the caller carries no city env at all (GCREX-35). Start, `is-running`, `get-last-activity`, `peek`, `list-running`, `stop`, `get-meta` and `set-meta` therefore agree on one store regardless of which caller invokes them. Cities on one Rex server still share the `gc:default` session unless each is pinned with `GC_REX_CITY`; pinning also prevents cross-city fallback reads, and the discovered sibling set is ordered so a duplicate name resolves deterministically.
- **Launched command exit closes its pane**: The provider sets no keep_open, so an agent command that exits closes its Rex block, and closing the last block removes the tab. `is-running` reports such a vanished pane as not running, not as an error.
- **Start config `env` map is applied via the `env` argv wrapper**: Rex terminal launch options have no per-pane environment map, so the provider prefixes the launch command with `env K=V ...` (sorted keys). A bare shell (no command) cannot be seeded and still inherits the Rex server's environment.
- **Setup hooks run with warnings only (GCREX-56)**: `session_setup` entries and `session_setup_script` run after the pane launches, in order, with `/bin/sh -c` in the start config's `work_dir` (or `/`), with the launch environment plus `GC_SESSION=<name>`. A hook failure is written to stderr as a `start: session_setup[i] …` warning and never fails the start, mirroring runtime-herdr-core. A non-empty `session_live` is never applied (it styles a tmux server Rex does not run) and warns `SESSION_LIVE_SKIPPED` once. Warnings go to the provider's warning writer (stderr in production); gc 1.5.1 drops a successful start's stderr, so they are an operator diagnostic.
- **Startup `nudge` is delivered after a bounded readiness wait (GCREX-14, GCREX-34):** The `nudge` field in the `start` JSON payload is typed into the freshly launched pane and submitted with Enter, so the agent receives its initial prompt. Before typing, the provider polls the pane's `program_status` and delivers as soon as the newest record reports `idle`; the full window is 15 s (matching gc's `defaultReadyProbeTimeout`), while a program that has reported no status at all is given a shorter 3 s settle delay so a non-status agent is not held for the whole window. The wait honours the start context. Before typing, the provider reads the last 120 lines of the pane's rendered screen. It holds the nudge back when that screen shows a startup dialog gc knows: a workspace-trust, theme, resume, update, MCP/hook-trust, bypass-permissions, custom-key or rate-limit screen. The dialog marks its selected option with the prompt glyph, so the turn's Enter would pick it. gc answers the dialog only once `start` returns. The dialog list, the rate-limit markers and the Codex settled-status guard are ported from runtime-herdr-core's `herdrcore/readiness.py`; after a sighting it waits a bounded clear window and delivers only after a clear, ready poll. A seed left undelivered sets `GC_STARTUP_DELIVERY_UNCONFIRMED` to `withheld: …` naming the dialog. It records `readiness_outcome` `dialog` or `dialog-unresolved`, never a failed start (GCREX-59, #205, see "Startup seed and dialogs"). Otherwise, after the Enter, the provider observes the pane for a bounded confirmation window; a delivered but unconfirmed nudge leaves `GC_STARTUP_DELIVERY_UNCONFIRMED` in the pane's metadata (and a diagnostic file), and a confirmed delivery clears that key. A delivered but unconfirmed submit is resent down a bounded backoff ladder (2/4/6/8 s, mirroring gc's `startupNudgeRetryBackoffs`): each resend first clears the drafted line (`ctrl+u`) and re-types the nudge, and if the pane has since gone `working` or `blocked` the earlier submit is treated as landed and no resend happens. Readiness detection and the TUI accepting input are not the same moment, so this rescues the lost-Enter case the readiness wait alone does not. Each stage of the attempt is timed and written to the startup-timing diagnostic above (`readiness_outcome` distinguishes an idle return from a bound, a withheld dialog or an ended agent; GCREX-58, GCREX-59). `ready_delay_ms` floors the wait (capped at 60 s), and `ready_prompt_prefix`, when set, makes the pane's rendered prompt the readiness signal instead of `program_status` — read as gc's `paneShowsReadyPrompt` does (bottom-most prompt line wins, NBSP reads as a space, one box border is skipped, and a numbered menu's cursor row is not a prompt) — while `ready_delay_ms` then only bounds that wait; GCREX-56 added both. The nudge is not delivered on a failed launch, an already-running session (Start's idempotent path or StartNew's ErrAlreadyRunning), or when the nudge text is empty. Delivery never fails the start. Before any dialog sighting, a timeout or a canceled context still delivers the nudge best-effort when the context lets it. After a sighting, the bound fallbacks no longer deliver. A timeout only runs the clear wait to its deadline poll, and then `dialog` or `dialog-unresolved` returns without typing. A transport failure is not a start failure, so a healthy pane is never torn down over it. No gc backstop covers an undelivered first prompt of a named session. gc's queued-nudge backstop covers pool slots only (gc edge `cmd/gc/idle_nudge.go:88-136`), so `GC_STARTUP_DELIVERY_UNCONFIRMED` is the only signal. The readiness wait has one failure of its own: when the pane's program has already exited before its first turn, a non-`one_shot` start fails with `the agent exited during startup, before its first turn` (`readiness_outcome` `ended`), and `lifecycle=one_shot` skips it.
- **File staging (GCREX-56)**: before `pre_start`, the provider stages each `copy_files` entry into the work dir (a destination that leaves the work dir is refused with a warning; a missing source is skipped) and stages an `overlay_dir`'s universal subtree (minus the top-level `per-provider` and `.gc`) plus its `per-provider/<GC_PROVIDER>` subtree. Staging writes through an `os.Root` of the work dir, so a symlinked destination inside the work dir cannot escape it, and a `copy_files` source that already is its destination is skipped. After `pre_start` it stages each `pack_overlay_dirs` entry's `per-provider/<GC_PROVIDER>` subtree, so a pack's per-provider hook override replaces an existing managed file. Unlike gc, managed hook/settings files are overwritten rather than JSON-merged.
- **`fresh_only` refuses a held name (GCREX-56)**: a start with `fresh_only` fails with exit 73 `session "<name>" already exists (fresh_only)` when any binding record holds the name, live or dead, instead of recycling it. A binding read that fails is reported before that check, so an unreadable store never reads as a free name. gc 1.5.1 never sends `fresh_only`.
- **`pre_start` runs before the pane launches** (GCREX-15, GCREX-25.2): each entry is executed in order with `/bin/sh -c`, cwd set to the start config's `work_dir` when that directory already exists and to `/` otherwise (mirroring runtime-herdr-core's `_setup_cwd`, so a hook can create the work dir), and the process environment merged with the start-config `env` map. Each hook runs under a per-hook timeout (30 s by default, `WithPreStartHookTimeout`) capped by whatever remains of the start budget (the op context's deadline when it carries one, else 120 s, `WithStartBudget`); a hook that outlives it fails the start with `pre_start[i]: … timed out after D`. A non-zero exit aborts the launch and leaves no seeded identity or binding behind. After the hooks, a configured `work_dir` must be an existing directory or the start fails. An empty or absent `pre_start` is a no-op.
- **`list-running` only sees known panes**: It is derived from the sidecar bindings plus a liveness check, not from a scan of Rex. A live pane with no cached binding (for example after the state dir was removed) is not listed. It emits each live primary binding whose name matches the prefix, plus each matching rig-qualified alias when the prefix is non-empty; the empty prefix lists primary names only. If the binding store cannot be enumerated (a corrupt record, an unreadable directory), `list-running` returns an error instead of a truncated list; gc reads that as "could not enumerate" and keeps the city.
- **Unimplemented operations exit 2**: `exec`, `provision`, and `watch-startup` are not implemented and exit with code `2` per RPP v0. `server-stop` and `copy-to` are not registered, so they also exit 2. At `v1.5.1`, gc's exec `CopyTo` runs `copy-to` best-effort (`internal/runtime/exec/exec.go:793-797`). The retired parity doc said gc never sends it. Whether a 1.5.1 city reaches that call for an exec provider is `unverified`. When `watch-startup` is unsupported, gc falls back to `runtime.AcceptStartupDialogs` over `peek` and `send-keys` (`internal/runtime/exec/exec.go:376-380` at `v1.5.1`). ADR-0009 records why `start` stays welded.
- **`attach` is a hint, not a pty passthrough (GCREX-20)**: gc 1.5.1 calls `attach` from `gc session attach` and `gc session new` through `runWithTTY`, which does not apply the exit-2-is-success rule, so a Rex-backed session cannot answer exit 2 without failing the command. The provider registers `attach` and always exits 0, printing a one-line hint to open the Rex app and select the session. It reads no Rex state, so an unknown session and an unreachable server succeed too. No `tty.attach` capability is declared: this is a hint path, not an interactive connection.
- **`nudge` is busy-gated on the agent's OSC 7501 status (GCREX-16, GCREX-21)**: the pane program emits OSC 7501 status frames (Pi does; gc-managed Pi panes report `idle`/`working`/…), and Rex stores them as `program_status`. `nudge` reads the newest record before it types. A `working` pane answers the `busy` input outcome. A `blocked` pane answers `pending_interaction` and names the blocked kind. Both exit 80 and type nothing (`internal/provider/pane_delivery.go`, `input.go`; `TestNudgeBusyIsBusyOutcome`, `TestNudgeBlockedIsPendingInteraction`). A program that reports no status is not gated.
- **Teardown leaves the server and the session intact**: Stopping agents closes only city-owned blocks (`rex block close`) and never stops the shared Rex server. After the last pane closes, the empty canonical `[city] <city>` session is left in place — the live probe lists the server-wide Rex app client on every session, which is the approved skip branch — and the next `Start` reuses it (`docs/REX-CONTRACT.md` §12).

## Operational risks

These are risks in how gc reads the provider, not provider defects.

- **A Rex outage reads as "everything is dead".** At `v1.5.1`, gc's exec
  `IsRunning` reads an op error the same as `false`
  (`internal/runtime/exec/exec.go:652-674`, `boxOccupancy`). While the Rex
  server is unreachable, every session therefore looks dead, and the reconciler
  starts replacements. The provider's own liveness reads answer `unknown`, never
  `dead` (ADR-0002), but `is-running` has no third answer on the wire.
- **`rex` exit 3 is broad.** `internal/rex/cli/cli.go:59-60` maps every exit 3
  to `rex.ErrNotFound`, and the driving ops treat not-found as success. Exit 3
  is a 404 `not_found` answer and never a transport failure (`REX-CONTRACT.md`
  §10.1, GCREX-31). A down server exits 4, which the client reads as
  unavailable. The one other exit-3 cause is an expired `blob_read` result, and
  its five-minute hold outlives the 30 s op deadline.
- **Probe fan-out.** One gc `IsAttached`, `GetLastActivity` or `ProcessAlive`
  observation can cost up to four provider invocations, because two
  `is-running` probes come first. Each one is a Rex round trip. This is
  `unverified` against `v1.5.1`: the retired parity doc recorded it, and the gc
  seam-layer source was not re-read for this move.

ADR-0017 records the GCREX-6 pilot and the promotion to a supported opt-in backend.
