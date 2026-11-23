# rex-gascity

A [Gas City](https://docs.gascityhall.com) runtime provider for [Rex](https://rex.dev). It runs Gas City agents as native Rex terminal panes instead of tmux panes, using the Runtime Provider Protocol v0 (RPP v0).

The binary is `gc-runtime-rex`. Gas City launches it as a short-lived process per operation; it drives the `rex` CLI to create sessions, tabs and panes.

## How it maps onto Rex

| Gas City | Rex | Label |
|---|---|---|
| City | Session | `gc:<city>` |
| Rig | Tab (window) | `Rig: <rig>`, or the city pin (else `Gas City`) for town-level agents |
| Agent | Pane (block) | `<agent>`, or the full session name for town-level agents |

The first agent in a rig creates a `Rig: <rig>` tab; a town-level agent uses the city pin, or `Gas City` when no pin is set. Later agents in the same rig split panes into that tab: each new pane splits the largest-area pane along its longer axis at 50%, so panes stay balanced. `stop` closes only the agent's block; when the last pane in a tab closes, Rex removes the tab. Teardown leaves the `gc:<city>` session in place even when empty and never stops the shared Rex server. A later `start` reuses the session.

## Prerequisites

- `rex` CLI on `PATH`, or set `GC_REX_BIN` to its path. The provider is tested against `rex` 0.1.0-preview.20, whose CLI supports API revisions 0–16; the pilot ran against a server at revision 14.
- A reachable Rex server. Check with `rex session ls`.
- Gas City (`gc`) with RPP v0 pack runtime support. Tested with gc 1.5.1.
- Go 1.27 or newer to build from source.

## Install

```sh
go install github.com/docbobo/rex-gascity/cmd/gc-runtime-rex@latest
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
gc import add /path/to/rex-gascity/pack --name rex --city /path/to/my-city
```

which writes an `[imports.rex]` entry.

### Import from GitHub by tag

The pack is the `pack/` subdirectory, so the source is a tree URL:

```sh
gc import add https://github.com/docbobo/rex-gascity/tree/main/pack --name rex \
  --version 2026.11.02 --city /path/to/my-city
```

- `--version` takes the bare version (`2026.11.02`), not the tag name. gc
  lists the remote tags with `git ls-remote` and takes the highest tag that
  matches the constraint. `--version v2026.11.02` matches no tag and fails.
- Push the tag before the import. A version import needs a pushed tag,
  `v<version>`. Without one, gc reports that no tag matches the constraint.
- `packs.lock` pins the tag commit, so the import is reproducible. A private
  repository needs git credentials for `git ls-remote` and the fetch.
- `sha:<commit>` pins one commit instead. It works only when that commit's
  `[pack] version` has a published release. The install picks the release
  `v<[pack] version>`, so a `sha:` import of an unreleased commit fails at
  install and names the tag. See [`docs/rex-binary-install.md`](docs/rex-binary-install.md).

Then install the pack and the provider binary:

```sh
gc import install --city /path/to/my-city
gc doctor --check rex-gascity-runtime:rex-binary --city /path/to/my-city --fix
```

To release:

1. Bump `[pack].version` in `pack/pack.toml` and commit it.
2. Run `sh pack/release.sh v<version>`. It creates the draft and its tag,
   dispatches the build, waits for the four binaries and `checksums.txt`,
   downloads the asset for this host, verifies its sha256, runs conformance on
   that downloaded binary, and attaches `conformance.txt`.
3. Approve the publish prompt, or pass `--yes` after a human approved. Only
   then is the release published.

`--dry-run` prints the commit and asset names and changes no remote state.
`--resume` continues an existing draft. A failed conformance leaves the draft
in place and prints the cleanup command. Never push the tag by hand: a
hand-pushed tag builds nothing, because the build runs on workflow dispatch.

Only the tag matters to `gc import add`. The release assets matter at install
time, when `pack/install.sh` fetches the asset for `v<[pack] version>`.

A simple city without its own `pack.toml` can use a workspace include instead, with the path relative to `city.toml`:

```toml
[workspace]
name = "my-city"
includes = ["path/to/rex-gascity/pack"]
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

CI runs `pack/tests/conformance.sh --binary bin/gc-runtime-rex` against a
pinned gc commit. It installs the pinned preview rex CLI first: the provider's
session path shells out to `rex`, and the suite leaves no session behind. The
archive is pinned by commit and checksum, so a newer preview never changes what
CI tests.

The pack also ships a `gc doctor` check named `rex-gascity-runtime:rex-binary`. It reports a
missing provider binary or `rex` CLI with a remediation instead of leaving the
city's agents in an opaque `unknown` state:

```sh
gc doctor --check rex-gascity-runtime:rex-binary --city /path/to/city
```

The check resolves the runtime command from `pack/pack.toml` (a bare name on
`PATH` or a path relative to the pack directory), probes the provider's RPP v0
handshake, resolves the `rex` CLI, honoring `GC_REX_BIN`, and compares the
provider's reported version (`gc-runtime-rex version`) against the `version`
`pack.toml` declares. A mismatch exits `2` with a remediation; a version it
cannot determine warns (`1`) rather than failing, because the binary still
handshakes. `make build` stamps the pack's version into `bin/gc-runtime-rex`,
so a build from this repository matches.

## Configuration

### Environment variables

| Variable | Default | Purpose |
|---|---|---|
| `GC_REX_BIN` | `rex` (via `PATH`) | Path or name of the `rex` CLI the provider drives |
| `GC_EXEC_CITY_NAME` | unset | gc's per-op city name; pins the city namespace (highest precedence) |
| `GC_EXEC_CITY_PATH` | unset | gc's per-op city path; names the city by its last path component when `GC_EXEC_CITY_NAME` is unset |
| `GC_REX_CITY` | `basename($GC_CITY_PATH)`, then `$GC_CITY`, else `"default"` | City namespace for the `gc:<city>` session label and sidecar state |
| `GC_REX_STATE_DIR` | `$XDG_STATE_HOME/rex-gascity` or `~/.local/state/rex-gascity` | Base directory for per-city sidecar state |
| `GC_SESSION` | unset | Selects the provider for a foreground controller (`rex`) |

RPP v0 calls carry no city identifier, so the provider resolves the city from the environment. gc sets `GC_EXEC_CITY_NAME` and `GC_EXEC_CITY_PATH` on every op, so under gc the city is the name gc serves, never a path. Set `GC_REX_CITY` explicitly when several cities share one Rex server and you run the provider outside gc; otherwise they all fall back to the same `gc:default` session and state directory.

### Sidecar state

Bindings, activity observations and metadata are stored per city under `<state dir>/<encoded-city>/`, with `bindings/`, `activity/`, `meta/` and a `lock` file. The city name is percent-encoded so distinct names map to distinct directories, including on case-insensitive filesystems.

### RPP capabilities

```json
{"version":0,"capabilities":["report-attachment","report-activity","idle-boundary","carrier.ops","input-outcome"]}
```

Implemented operations: `protocol`, `start`, `stop`, `is-running`, `interrupt`, `nudge`, `send-keys`, `clear-scrollback`, `process-alive`, `peek`, `list-running`, `get-last-activity`, `idle`, `is-attached`, `set-meta`, `get-meta`, `remove-meta`.

`proc.exec`, `proc.provision`, `proc.stream` and `tty.attach` are not implemented and are not declared.

## Known limits

- **No city in RPP v0 invocations.** gc edge sets `GC_EXEC_CITY_NAME`/`GC_EXEC_CITY_PATH` on every op, so the city resolves without any provider-side pin. Without a pin (an older gc, or the CLI fallback provider) cities fall back to `basename($GC_CITY_PATH)`, `$GC_CITY`, then `"default"`. Under `launchd`, `gc` does not pass `GC_CITY` or `GC_CITY_PATH`, so cities without a pin share one `gc:default` session and state directory.
- **A launched command that exits closes its pane.** The provider sets no keep-open, so an agent that exits leaves no pane behind. The provider reports the agent as stopped rather than erroring.
- **No per-tab pane cap.** Panes stay balanced by area, but a rig with many agents still divides the tab indefinitely.

## Development

```sh
make help            # list targets
make build           # build bin/gc-runtime-rex
make vet             # go vet ./...
make test            # go test ./...
make test-e2e        # fake-rex e2e suites; no rex install needed
make check           # gc runtime check against testdata/city
make conformance     # gc runtime conformance against testdata/city
make hooks           # enable the versioned git hooks in .githooks
```

The provider e2e tests run against the in-repo fake `rex` by default, so they
need no install. `REX_E2E=1` selects a real Rex server instead:

```sh
REX_E2E=1 go test ./internal/provider -run TestE2E -v
REX_E2E=1 go test ./cmd/gc-runtime-rex -run TestE2E -v
```

Live mode creates and closes real sessions and panes on the configured server.
Point it at a server you are willing to modify. See
[`docs/fake-rex.md`](docs/fake-rex.md) for what the fake covers.

## Documentation

- [`docs/rex-runtime-pack.md`](docs/rex-runtime-pack.md) — operator quickstart for the runtime pack: import, install, auth, upgrades, release flow.
- [`docs/rex-provider.md`](docs/rex-provider.md) — provider reference: install, pack and city configuration, identity mapping, state and environment.
- [`docs/fake-rex.md`](docs/fake-rex.md) — the test fake: coverage, limits, scripted faults, and the live-server escape hatch.
- [`docs/REX-CONTRACT.md`](docs/REX-CONTRACT.md) — the pinned Rex CLI/API surface and the RPP v0 baseline.
- [`docs/plans/native-rex-provider.md`](docs/plans/native-rex-provider.md) — design and phase plan.
- [`docs/rex-pilot-phase5.md`](docs/rex-pilot-phase5.md) — pilot results and the promotion decision.
