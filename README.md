# rex-gas-city

A [Gas City](https://docs.gascityhall.com) runtime provider for [Rex](https://rex.dev). It runs Gas City agents as native Rex terminal panes instead of tmux panes, using the Runtime Provider Protocol v0 (RPP v0).

The binary is `gc-runtime-rex`. Gas City launches it as a short-lived process per operation; it drives the `rex` CLI to create sessions, tabs and panes.

## How it maps onto Rex

| Gas City | Rex | Label |
|---|---|---|
| City | Session | `gc:<city>` |
| Rig | Tab (window) | `<rig>`, or `__town` for town-level agents |
| Agent | Pane (block) | `<agent>`, or the full session name for town-level agents |

The first agent in a rig creates a tab. Later agents in the same rig split panes into that tab: each new pane splits the largest-area pane along its longer axis at 50%, so panes stay balanced. `stop` closes only the agent's block; when the last pane in a tab closes, Rex removes the tab. Teardown never stops the shared Rex server and never calls `session.destroy`.

## Prerequisites

- `rex` CLI on `PATH`, or set `GC_REX_BIN` to its path. The provider is tested against `rex` 0.1.0-preview.20, whose CLI supports API revisions 0–16; the pilot ran against a server at revision 14.
- A reachable Rex server. Check with `rex session ls`.
- Gas City (`gc`) with RPP v0 pack runtime support. Tested with gc 1.5.1.
- Go 1.27 or newer to build from source.

## Install

```sh
go install github.com/docbobo/rex-gas-city/cmd/gc-runtime-rex@latest
```

Make sure `$(go env GOBIN)` (or `$(go env GOPATH)/bin` when `GOBIN` is unset) is on `PATH`.

From a checkout:

```sh
make build            # writes bin/gc-runtime-rex
make install          # go install ./cmd/gc-runtime-rex
```

## Use it in a city

Add the pack to the city. A schema-2 city uses a pack import:

```sh
gc import add /path/to/rex-gas-city/pack --name rex --city /path/to/my-city
```

which writes an `[imports.rex]` entry. A simple city without its own `pack.toml` can use a workspace include instead, with the path relative to `city.toml`:

```toml
[workspace]
name = "my-city"
includes = ["path/to/rex-gas-city/pack"]
```

Then select the provider for the whole city:

```toml
[session]
provider = "rex"
```

Or select it for one invocation without editing `city.toml`:

```sh
GC_SESSION=rex gc start
```

`GC_SESSION` only applies to the process that constructs the session provider. In a city managed by an already-running supervisor or controller, that process reads its own environment, so `GC_SESSION=rex gc start` has no effect. For a managed city, set `[session] provider = "rex"` in `city.toml` and restart the supervisor so it picks up the change.

Start the city as usual:

```sh
gc start
```

Agents then appear as Rex tabs and panes in the `gc:<city>` session. Attach from the Rex app or with `rex`.

## Verify the installation

Check that the pack registers and the provider speaks RPP v0:

```sh
GC_REX_CITY=rex-verify gc runtime check rex --city /path/to/city
GC_REX_CITY=rex-verify gc runtime conformance rex --city /path/to/city
```

Both commands print `resolved runtime "rex" from pack "rex"` when the pack is picked up. If `city.toml` fails to load, `gc` falls back to any bare `rex` executable on `PATH` instead of running `gc-runtime-rex`; check that line to be sure which binary answered.

From this repository, the same checks run against `testdata/city`:

```sh
make check
make conformance
```

## Configuration

### Environment variables

| Variable | Default | Purpose |
|---|---|---|
| `GC_REX_BIN` | `rex` (via `PATH`) | Path or name of the `rex` CLI the provider drives |
| `GC_REX_CITY` | `basename($GC_CITY_PATH)`, then `$GC_CITY`, else `"default"` | City namespace for the `gc:<city>` session label and sidecar state |
| `GC_REX_STATE_DIR` | `$XDG_STATE_HOME/rex-gas-city` or `~/.local/state/rex-gas-city` | Base directory for per-city sidecar state |
| `GC_SESSION` | unset | Selects the provider for a foreground controller (`rex`) |

RPP v0 calls carry no city identifier, so the provider resolves the city from the environment. Set `GC_REX_CITY` explicitly when several cities share one Rex server; otherwise they all fall back to the same `gc:default` session and state directory.

### Sidecar state

Bindings, activity observations and metadata are stored per city under `<state dir>/<encoded-city>/`, with `bindings/`, `activity/`, `meta/` and a `lock` file. The city name is percent-encoded so distinct names map to distinct directories, including on case-insensitive filesystems.

### RPP capabilities

```json
{"version":0,"capabilities":["report-attachment","report-activity"]}
```

Implemented operations: `protocol`, `start`, `stop`, `is-running`, `interrupt`, `nudge`, `send-keys`, `clear-scrollback`, `process-alive`, `peek`, `list-running`, `get-last-activity`, `is-attached`, `set-meta`, `get-meta`, `remove-meta`.

`proc.exec`, `proc.provision`, `proc.stream` and `tty.attach` are not implemented and are not declared.

## Known limits

- **No city in RPP v0 invocations.** Without `GC_REX_CITY`, cities fall back to `$GC_CITY_PATH`, `$GC_CITY`, then `"default"`. Under `launchd`, `gc` does not pass `GC_CITY` or `GC_CITY_PATH`, so cities without `GC_REX_CITY` share one `gc:default` session and state directory.
- **A launched command that exits closes its pane.** The provider sets no keep-open, so an agent that exits leaves no pane behind. The provider reports the agent as stopped rather than erroring.
- **No per-tab pane cap.** Panes stay balanced by area, but a rig with many agents still divides the tab indefinitely.

## Development

```sh
make help            # list targets
make build           # build bin/gc-runtime-rex
make vet             # go vet ./...
make test            # go test ./...
make check           # gc runtime check against testdata/city
make conformance     # gc runtime conformance against testdata/city
make hooks           # enable the versioned git hooks in .githooks
```

Live tests talk to a real Rex server and are gated behind `REX_E2E`:

```sh
REX_E2E=1 go test ./internal/provider -run TestE2E -v
REX_E2E=1 go test ./cmd/gc-runtime-rex -run TestE2E -v
```

They create and close real sessions and panes on the configured server. Point them at a server you are willing to modify.

## Documentation

- [`docs/rex-provider.md`](docs/rex-provider.md) — provider reference: install, pack and city configuration, identity mapping, state and environment.
- [`docs/REX-CONTRACT.md`](docs/REX-CONTRACT.md) — the pinned Rex CLI/API surface and the RPP v0 baseline.
- [`docs/plans/native-rex-provider.md`](docs/plans/native-rex-provider.md) — design and phase plan.
- [`docs/rex-pilot-phase5.md`](docs/rex-pilot-phase5.md) — pilot results and the promotion decision.
