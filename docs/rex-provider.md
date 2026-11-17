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

Implemented RPP operations: `protocol`, `start`, `stop`, `is-running`, `interrupt`, `nudge`, `send-keys`, `clear-scrollback`, `process-alive`, `peek`, `list-running`, `get-last-activity`, `is-attached`, `set-meta`, `get-meta`, `remove-meta`.

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

## Sidecar State and Environment Variables

Because RPP invokes `gc-runtime-rex` as a short-lived process per operation, pane bindings, activity observations, and session metadata are stored on disk per city.

### State Directory Resolution

Base directory precedence:

1. `$GC_REX_STATE_DIR` (if non-empty)
2. `$XDG_STATE_HOME/rex-gascity` (if `XDG_STATE_HOME` is set)
3. `~/.local/state/rex-gascity`

Within that base directory, each city uses `<base>/<encoded-city>/` (percent-encoded so distinct city names always map to distinct directories, including on case-insensitive filesystems such as macOS APFS: only `a-z`, `0-9` and `-` stay literal, and every other byte, including uppercase letters, becomes `%XX`), containing `bindings/`, `activity/`, `meta/`, and an `flock` file `lock`.

### City Namespace Resolution

RPP v0 passes no city identifier on argv or stdin, so `gc-runtime-rex` resolves `<city>` from the environment in order:

1. `$GC_REX_CITY`
2. `basename($GC_CITY_PATH)`
3. `$GC_CITY`
4. `"default"`

### Environment Knobs

| Variable | Default | Purpose |
|---|---|---|
| `GC_REX_BIN` | `rex` (resolved via `PATH`) | Path or name of the `rex` CLI binary used by the provider |
| `GC_REX_CITY` | `basename($GC_CITY_PATH)`, `$GC_CITY`, else `"default"` | Overrides the city namespace used for the `gc:<city>` Rex session label and sidecar state subdirectory |
| `GC_REX_STATE_DIR` | `$XDG_STATE_HOME/rex-gascity` or `~/.local/state/rex-gascity` | Overrides the base directory for per-city sidecar state |

## Known v1 Limits

- **No city in RPP v0 invocations**: An RPP call carries only the operation and agent session name. Without `GC_REX_CITY`, cities fall back to `GC_CITY_PATH` / `GC_CITY` and then `"default"`. Because the `gc` controller does not pass `GC_CITY` or `GC_CITY_PATH` when running under `launchd`, cities without `GC_REX_CITY` share one `gc:default` Rex session and state directory. Set `GC_REX_CITY` explicitly when isolating cities on the same Rex server.
- **Launched command exit closes its pane**: The provider sets no keep_open, so an agent command that exits closes its Rex block, and closing the last block removes the tab. `is-running` reports such a vanished pane as not running, not as an error.
- **Start config `env` map is not applied**: Rex terminal launch options have no per-pane environment map. The `env` field in the `start` JSON payload is ignored, and agent panes inherit the Rex server's environment.
- **Setup and live hooks are not run**: `pre_start`, `session_setup`, `session_setup_script`, and `session_live` from start config are ignored.
- **Startup `nudge` is not sent**: The `nudge` field in the `start` JSON payload is ignored, and `gc` does not resend it later. An agent's first prompt must come from its command line or from a later explicit `gc session nudge`.
- **File staging is not done**: `copy_files`, `overlay_dir`, and `pack_overlay_dirs` from start config are ignored; nothing is copied into the agent's work dir.
- **`list-running` only sees known panes**: It is derived from the sidecar bindings plus a liveness check, not from a scan of Rex. A live pane with no cached binding (for example after the state dir was removed) is not listed.
- **Unimplemented operations exit 2**: `exec`, `provision`, `attach`, and `watch-startup` are not implemented and exit with code `2` per RPP v0.
- **No OSC 7501 or busy-gated nudge**: OSC 7501 agent status frames are not emitted, and nudge busy-gating is deferred. `nudge` writes the message text and sends `enter` without waiting for an idle prompt.
- **Teardown leaves the server and session shell intact**: Stopping agents closes only city-owned blocks (`rex block close`); it never stops the shared Rex server and never calls `session.destroy`.

See `docs/rex-pilot-phase5.md` for the pilot and promotion record (GCREX-6), including the observed `gc:default` namespace under a launchd supervisor.
