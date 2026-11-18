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
name = "rex"
version = "0.1.0"
schema = 2
description = "Gas City runtime pack for Rex: a native Rex session provider speaking RPP v0."

[runtimes.rex]
command = "gc-runtime-rex"
protocol = 0
```

`command = "gc-runtime-rex"` has no path separator, so `gc` resolves it via `PATH` at run time and the pack carries no platform-specific binary.

### Pi hook overlay (claim guard)

The core pack installs `gc`'s managed Pi hook at
`.pi/extensions/gc-hooks.js`; the rex pack overrides it with
`pack/overlay/per-provider/pi/.pi/extensions/gc-hooks.js`. `gc` stages
`overlay/per-provider/<provider>/` trees into the session work directory and a
later pack's copy wins on a fresh workdir (gc `internal/runtime/staging.go`
`StageProviderOverlayDir`; `cmd/gc/build_desired_state_overlay_preserve_test.go`
`TestPrepareTemplateResolution_LaterOverlayLayerOverridesManagedHook`). That
path alone does not replace an existing current managed hook: the controller's
`hooks.PreserveManagedFile` predicate keeps it (gc 1.5.1
`internal/hooks/preserve.go`; the sibling test
`TestPrepareTemplateResolution_PreservesCurrentManagedHook`). The predicate
reads the marker of the file already on the destination, not the desired
overlay's marker, so gc's staging never selects the rex copy over a current
managed one. The provider therefore stages the start config's
`pack_overlay_dirs` per-provider subtree itself, after `pre_start`, so the rex
override replaces a managed v9 hook even in a shared checkout. The override
carries `GC_PI_HOOK_VERSION = 100`, well above gc 1.5.1's 9 and gc HEAD's 10;
once staged, that marker is what makes the preserve predicate keep the override
on later ticks. The provider's writer does not compare versions, so if a future
gc ships a managed hook above 100, re-bump the marker.

The behavioral difference this override adds is a claim guard in
`before_agent_start`. `gc`'s
stalled-claim backstop re-queues the fallback pool-claim nudge (`Run gc hook
--claim --drain-ack --json now; …`) for a pool slot whose routed trigger bead
still reads open (gc `cmd/gc/idle_nudge.go`, `defaultPoolClaimNudge`; 90 s grace,
`docs/reference/config.md`). The managed hook drains and injects every queued
nudge unconditionally, so a seat that already held a claim received that text
mid-task, ran the hook again with `--drain-ack`, handed the claim back and
abandoned the task (GCREX-33; my-city events 2026-11-18 06:47–06:53, seat
`mc-wisp-gwble`). The overlay reads the stamped claim with `gc hook current
--id-only` unconditionally; `$GC_BEAD_ID` belongs to the controller's dispatch
environment, not the session shell. A non-empty stamp alone is not liveness:
gc does not clear `current_claim_bead_id` when the claimed work bead closes
(gc 1.5.1 `internal/session/store.go` `CurrentClaimBeadID`), so the overlay
re-reads the stamped bead through `gc bd list --id <claim> --all --json` and
mirrors gc's `claimedWorkStillHeldBy` predicate (gc 1.5.1
`cmd/gc/session_ownership.go`): a closed, released (open and unassigned), or
foreign-owned (`gc.session_id` names another session) claim is not held. While
the claim reads held the overlay leaves the nudge queue undrained: nothing is
injected mid-claim and nothing is acked or lost, and the next turn after the
claim stops being held drains and delivers what was queued. This defers the
whole queue, not only the pool-claim prompt. The provider's `Nudge` refusal
uses the same liveness predicate (`internal/provider/nudge.go`
`probeLiveClaim`), so the fallback pool-claim text is delivered once the claim
is no longer held instead of being suppressed indefinitely.

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

The pack registers a `gc doctor` check named `rex:rex-binary` (from
`pack/doctor/rex-binary/`). Run it directly, or as part of the full sweep:

```sh
gc doctor --check rex:rex-binary --city /path/to/city
gc doctor --city /path/to/city
```

The check:

- resolves the runtime command from `pack/pack.toml`, accepting a bare name on
  `PATH` or a path relative to the pack directory;
- confirms the provider answers the `protocol` op with an RPP v0 handshake;
- resolves the `rex` CLI, honoring `GC_REX_BIN`.

It exits `0` when both are present, `1` for a warning, and `2` when the provider
binary or the `rex` CLI cannot be resolved. The first stdout line is the
message; later lines name the missing binary and give a remediation. A missing
provider otherwise surfaces only as opaque agent state (`unknown (partial
status)`) and a supervisor log line:

```text
exec: "gc-runtime-rex": executable file not found in $PATH
```

A supervisor or controller resolves the provider with its own `PATH`, so the
directory that holds `gc-runtime-rex` must be on that environment's `PATH`, not
just your interactive shell's.

## Protocol and Capabilities

`gc-runtime-rex protocol` returns the RPP v0 handshake:

```json
{"version":0,"capabilities":["report-attachment","report-activity"]}
```

- **Declared capabilities**: `report-attachment` (`is-attached`), `report-activity` (`get-last-activity`).
- **Capabilities not declared**: `proc.exec`, `proc.provision`, `proc.stream`, `tty.attach` (and no locality capability).

Implemented RPP operations: `protocol`, `start`, `stop`, `attach`, `is-running`, `interrupt`, `nudge`, `send-keys`, `clear-scrollback`, `process-alive`, `peek`, `list-running`, `get-last-activity`, `is-attached`, `set-meta`, `get-meta`, `remove-meta`.

## Identity Mapping

Gas City sessions map onto the three-level Rex hierarchy:

| Gas City | Rex | Label / Rule |
|---|---|---|
| City | Session | `gc:<city>` (one Rex session per city namespace) |
| Rig | Tab (window) | `<rig>` for `<rig>--<agent>`, or `__town` for town-level names with no `--` |
| Agent | Pane (block) | `<agent>` for `<rig>--<agent>`, or the full session name for town-level agents |

- Starting the first agent in a rig creates a new Rex tab labeled `<rig>` (or `__town`).
- Starting more agents in the same rig splits panes into that rig's existing tab. Each new pane splits the largest-area pane along its longer axis (horizontal when width >= height, else vertical) at 50%, so panes stay balanced. If the layout has no usable geometry, it falls back to splitting the first block horizontally.
- `stop` closes only the city-owned agent block (`rex block close`). Closing the last pane in a tab removes that tab automatically in Rex. Teardown never stops the shared Rex server and never calls `session.destroy`.

### Rig from the start config

gc 1.5.1 starts pool and wisp instances under a runtime name that carries no
`--` (`pi-mc-wisp-ybq1`, `core__control-dispatcher-mc-wisp-6znw`); the rig
arrives out of band in the start config `env` as `GC_RIG`. The provider
therefore derives the Rex tab label as:

- `GC_RIG` when it is non-empty; else
- `SessionName(name).Rig()` (the name split, for older names and town-level
  sessions).

The pane label stays `SessionName(name).PaneLabel()`.

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
stored aliases and keep resolving by the name-derived tab and pane labels.

## Sidecar State and Environment Variables

Because RPP invokes `gc-runtime-rex` as a short-lived process per operation, pane bindings, activity observations, and session metadata are stored on disk per city.

### State Directory Resolution

Base directory precedence:

1. `$GC_REX_STATE_DIR` (if non-empty)
2. `$XDG_STATE_HOME/rex-gascity` (if `XDG_STATE_HOME` is set)
3. `~/.local/state/rex-gascity`

Within that base directory, each city uses `<base>/<encoded-city>/` (percent-encoded so distinct city names always map to distinct directories, including on case-insensitive filesystems such as macOS APFS: only `a-z`, `0-9` and `-` stay literal, and every other byte, including uppercase letters, becomes `%XX`), containing `bindings/`, `activity/`, `meta/`, and an `flock` file `lock`.

The dispatcher resolves one canonical namespace and, when needed, reads a small
ordered set of legacy namespaces:

- **Canonical:** `$GC_REX_CITY` when set, else `"default"`. New sessions, new
  bindings, metadata writes and the state lock use this namespace.
- **Read set:** the canonical namespace first, then `basename($GC_CITY_PATH)`
  and `$GC_CITY` when they differ. Reads fall back through the set in order;
  deletes cover every directory. A record written by an older build (or by a
  caller that carried `GC_CITY_PATH`) therefore still resolves.
- **Pinned:** when `$GC_REX_CITY` is set the read set is exactly that one
  namespace, so cities pinned to different values never cross-resolve.

### City Namespace Resolution

RPP v0 passes no city identifier on argv or stdin, and the exec runtime provider
passes the caller's environment through unchanged. The controller, session
processes and the nudge poller therefore see different incidental `GC_CITY*`
values for the same city. To keep the provider from disagreeing with itself,
the canonical namespace is caller-independent:

1. `$GC_REX_CITY` when set (an explicit pin)
2. `"default"` otherwise

`basename($GC_CITY_PATH)` and `$GC_CITY` do **not** select the canonical store;
they are only additional read namespaces for records written by earlier
versions.

### Environment Knobs

| Variable | Default | Purpose |
|---|---|---|
| `GC_REX_BIN` | `rex` (resolved via `PATH`) | Path or name of the `rex` CLI binary used by the provider |
| `GC_REX_CITY` | `"default"` | Pins the canonical city namespace for the `gc:<city>` Rex session label and sidecar state subdirectory; also removes all fallback read namespaces |
| `GC_REX_STATE_DIR` | `$XDG_STATE_HOME/rex-gascity` or `~/.local/state/rex-gascity` | Overrides the base directory for per-city sidecar state |

### Start-config `GC_*` environment

RPP `start` carries the launch config as JSON on stdin, including an `env` map.
gc 1.5.1 ships every managed-session variable there. The provider's durable
contract is exactly four of them: `GC_RIG`, `GC_TEMPLATE`, `GC_ALIAS`, and
`GC_AGENT` (used to derive the tab and the aliases above). The rest are
incidental to the provider and are applied to the launched process unchanged
(GCREX-10). Empty values in the start-config env are applied as unset
(`env -u KEY`) at the launch boundary, matching RPP and gc's tmux adapter.

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

gc also reads provider metadata over `get-meta` (not the start env):
`GC_PROVIDER`, `GC_SESSION_ID`, `GC_DRAIN_ACK`, `GC_ALIAS`, `GC_AGENT`, and
`suspended`. The provider serves these from its metadata sidecar. `set-meta`,
`get-meta`, and `remove-meta` resolve a rig-qualified alias to the pane's
primary name, because gc reads some keys (`suspended`, `GC_SESSION_ID`) under
the canonical identity even when the pane was started under a short pool name.

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
so a prior life's token cannot outlive it. See `docs/REX-IDENTITY.md` for the
`runtime-herdr-core` comparison.

### Wrapped-gc process names

The provider captures `GC_BIN` from the start config and persists its basename
on the pane binding. `process-alive` expands the incoming gc hints with that
basename when a hint is `gc` (and with gc's `kimi` -> `kimi-code` alias), so a
control dispatcher launched as `$GC_BIN` (`/nix/store/.../bin/.gc-wrapped`, not
`gc`) is still observed as a live agent. Without the expansion gc reads the
live pane as a dead/zombie seat and closes the session bead without issuing
`stop`, leaving the pane and its process behind. This mirrors
`runtime-herdr-core`'s `expand_names`; see `docs/REX-RETIRE.md`.

## Known v1 Limits

- **No city in RPP v0 invocations (GCREX-17)**: An RPP call carries only the operation and agent session name, and the caller's `GC_CITY*` values are incidental. The canonical namespace is `GC_REX_CITY` when set, else `"default"`; `basename(GC_CITY_PATH)` and `GC_CITY` are read-only fallbacks for legacy records. Start, `is-running`, `get-last-activity`, `peek`, `list-running`, `stop`, `get-meta` and `set-meta` therefore agree on one store regardless of which caller invokes them. Cities on one Rex server still share the `gc:default` session unless each is pinned with `GC_REX_CITY`; pinning also prevents cross-city fallback reads.
- **Launched command exit closes its pane**: The provider sets no keep_open, so an agent command that exits closes its Rex block, and closing the last block removes the tab. `is-running` reports such a vanished pane as not running, not as an error.
- **Start config `env` map is applied via the `env` argv wrapper**: Rex terminal launch options have no per-pane environment map, so the provider prefixes the launch command with `env K=V ...` (sorted keys). A bare shell (no command) cannot be seeded and still inherits the Rex server's environment.
- **Setup hooks except `pre_start` are not run**: `session_setup`, `session_setup_script`, and `session_live` from start config are ignored.
- **Startup `nudge` is delivered after a bounded readiness wait (GCREX-14, GCREX-34):** The `nudge` field in the `start` JSON payload is typed into the freshly launched pane and submitted with Enter, so the agent receives its initial prompt. Before typing, the provider polls the pane's `program_status` and delivers as soon as the newest record reports `idle`; the full window is 15 s (matching gc's `defaultReadyProbeTimeout`), while a program that has reported no status at all is given a shorter 3 s settle delay so a non-status agent is not held for the whole window. The wait honours the start context. After the Enter, the provider observes the pane for a bounded confirmation window; a delivered but unconfirmed nudge leaves `GC_STARTUP_DELIVERY_UNCONFIRMED` in the pane's metadata (and a diagnostic file), and a confirmed delivery clears that key. A delivered but unconfirmed submit is resent down a bounded backoff ladder (2/4/6/8 s, mirroring gc's `startupNudgeRetryBackoffs`): each resend first clears the drafted line (`ctrl+u`) and re-types the nudge, and if the pane has since gone `working` or `blocked` the earlier submit is treated as landed and no resend happens. Readiness detection and the TUI accepting input are not the same moment, so this rescues the lost-Enter case the readiness wait alone does not. Delivery is still not withheld on a startup dialog, and `ready_prompt_prefix` is still not used to gate the nudge. The nudge is not delivered on a failed launch, an already-running session (Start's idempotent path or StartNew's ErrAlreadyRunning), or when the nudge text is empty. The wait and delivery never fail the start: on a timeout or a canceled context the nudge is still delivered best-effort when the context permits, and a transport failure is not reported as a start failure, so a healthy pane is never torn down over it; gc's queued-nudge backstop covers an undelivered first prompt.
- **Pack overlay staging is done; other file staging is not**: the provider stages each `pack_overlay_dirs` entry's `per-provider/<GC_PROVIDER>/` subtree into the work dir after `pre_start`, so a pack's per-provider hook override replaces an existing managed file. `copy_files` and `overlay_dir` are still ignored.
- **`pre_start` runs before the pane launches** (GCREX-15): each entry is executed in order with `/bin/sh -c`, cwd set to the start config's `work_dir` when that directory already exists and to `/` otherwise (mirroring runtime-herdr-core's `_setup_cwd`, so a hook can create the work dir), and the process environment merged with the start-config `env` map. A non-zero exit aborts the launch and leaves no seeded identity or binding behind. After the hooks, a configured `work_dir` must be an existing directory or the start fails. An empty or absent `pre_start` is a no-op.
- **`list-running` only sees known panes**: It is derived from the sidecar bindings plus a liveness check, not from a scan of Rex. A live pane with no cached binding (for example after the state dir was removed) is not listed. It emits each live primary binding whose name matches the prefix, plus each matching rig-qualified alias when the prefix is non-empty; the empty prefix lists primary names only.
- **Unimplemented operations exit 2**: `exec`, `provision`, and `watch-startup` are not implemented and exit with code `2` per RPP v0.
- **`attach` is a hint, not a pty passthrough (GCREX-20)**: gc 1.5.1 calls `attach` from `gc session attach` and `gc session new` through `runWithTTY`, which does not apply the exit-2-is-success rule, so a Rex-backed session cannot answer exit 2 without failing the command. The provider registers `attach` and always exits 0, printing a one-line hint to open the Rex app and select the session. It reads no Rex state, so an unknown session and an unreachable server succeed too. No `tty.attach` capability is declared: this is a hint path, not an interactive connection.
- **No busy-gated nudge (OSC 7501 is emitted by the agent)**: OSC 7501 agent status frames are emitted by the pane program, not the provider — Pi implements the protocol and gc-managed Pi panes report `idle`/`working`/… under Rex, so `program_status` is normally populated. The provider does not yet consume it: nudge busy-gating and submit confirmation are deferred, so `nudge` writes the message text and sends `enter` without waiting for an idle prompt.
- **Teardown leaves the server and session shell intact**: Stopping agents closes only city-owned blocks (`rex block close`); it never stops the shared Rex server and never calls `session.destroy`.

See `docs/rex-pilot-phase5.md` for the pilot and promotion record (GCREX-6), including the observed `gc:default` namespace under a launchd supervisor.
See `docs/REX-RETIRE.md` for the retire/drain comparison against
`runtime-herdr-core` and the wrapped-gc decision (GCREX-12).
