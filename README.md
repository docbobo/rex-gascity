# rex-gascity

A Gas City runtime pack. Runtime `rex` runs agent sessions as native Rex panes instead of tmux panes. The pack ships one executable, `gc-runtime-rex`. It speaks the Runtime Provider Protocol (RPP v0) and drives the `rex` CLI. Gas City runs it once per operation.

## Status: release target, v2026.11.26

Version `2026.11.26` (tag `v2026.11.26`) is the target version. The tag is not published yet. The provider is self-installing. The pack's `bin/gc-runtime-rex` entry point fetches the matching release binary into a version-keyed cache on first use. An import needs no `PATH` edit, no Go toolchain and no repository checkout (PR #39).

The installer never replaces a working provider with an asset that cannot answer `inventory`. A test fixture or a broken release cannot break a city (PR #38).

CI runs `gc runtime check` and `gc runtime conformance` with no failed requirement, in a scratch city and state root. The conformance report also records `v2_ready`. That value is evidence and not a gate. The pack is not in a pack registry. Import it from this repository by tag, once that tag is published.

## Layout

```text
rex-gascity/
  cmd/gc-runtime-rex/               main: argv dispatch, handshake, version, trace
  internal/provider/                runtime semantics: sessions, panes, state, meta
  internal/rex/                     the rex CLI client (argv, wire, paging, faults)
  internal/rex/fakerex/             the fake `rex` executable the e2e suites drive
  internal/launch/                  first-turn launch handling
  pack/
    pack.toml                       [runtimes.rex] -> bin/gc-runtime-rex and [pack] version
    bin/gc-runtime-rex              the self-installing entry point (POSIX sh)
    bin/_lib.sh                     cache-path helper shared with the doctor fix
    install.sh                      release installer, and --from-source
    doctor/rex-binary/              gc doctor check: run.sh, fix.sh, doctor.toml
    doctor/rex-ui/                  gc doctor check for the Gas City UI
    rex-min-version                 the oldest rex CLI the pack drives
    release.sh                      the only publisher
    tests/                          check-*.sh (pack checks), conformance.sh
  ui/                               the Gas City UI, a Rex Lua extension, and its installer
  docs/                             operator and reference docs
  testdata/city/                    a scratch city for gc runtime check and conformance
  Makefile                          build, test, verify, check, conformance
```

## Install and commands

### Import by tag

The pack's binding is `rex`. Import it under that name. The pack runs agent
sessions as native Rex panes instead of tmux panes. It ships one executable,
`bin/gc-runtime-rex`, which gc runs from the installed pack. It installs the
matching release binary. The tag-based commands in this section are usable
after `v2026.11.26` is published.

```sh
gc import add https://github.com/docbobo/rex-gascity//pack --name rex --version 2026.11.26
```

`pack/pack.toml` declares `version = "2026.11.26"`, and the release tag is
`v2026.11.26`. `--version` takes the bare version, so the tag must be pushed
first. The provider binary installed for that version is the one a session
runs. A provider that reports another version fails the doctor check, which
names the mismatch.

### Install and first use

```sh
gc import install
gc doctor --check rex-gascity-runtime:rex-binary --fix
gc import check
```

- **`gc import install`** writes the pack's files into the city and makes the
  `rex` runtime available.
- **`gc doctor --fix`** downloads the pinned release binary into the cache now,
  so the first session starts with no download.
- **`gc import check`** verifies that the installed pack matches its import.
  It is read-only.

The first session needs nothing else. `bin/gc-runtime-rex` installs release
`v<version>` into

```text
${GC_RUNTIME_CACHE_DIR:-${XDG_CACHE_HOME:-$HOME/.cache}/rex-gascity}/<version>/<os>-<arch>
```

and execs it. On a cache hit it runs the cached binary with no network I/O.
All install output goes to stderr, so stdout stays clean. A plain
`gc doctor --check rex-gascity-runtime:rex-binary` is read-only: it never
downloads, and a cold cache reports "not yet installed, run `--fix` or start a
session". A green check means the provider handshakes, its version matches
`pack.toml`, and the `rex` CLI resolves.

### Gas City UI

The pack also ships an optional Rex extension, the Gas City UI, in `ui/`. A
second doctor check installs and verifies it:

```sh
gc doctor --check rex-gascity-runtime:rex-ui --fix
gc doctor --check rex-gascity-runtime:rex-ui
```

The check is read-only without `--fix`. It warns (exit 1) when the UI is not
installed or the Rex config does not load it, and errors (exit 2) when the
config fails to load or `rex` is missing. The UI also installs alone, without
the provider. See [`docs/rex-gascity-ui.md`](docs/rex-gascity-ui.md) and
[`ui/README.md`](ui/README.md).

### Upgrade and roll back

`gc import upgrade rex` moves the import to the newest tag that matches its
constraint. The constraint must allow more than one tag: `--version '^2026.11.26'`
matches later 2026 releases and not a 2027 release.

The install example pins a bare version, so convert it once. Remove the import
and add it again with a caret constraint.

```sh
gc import remove rex
gc import add https://github.com/docbobo/rex-gascity//pack --name rex --version '^2026.11.26'
```

Then run the upgrade, install and verify.

```sh
gc import upgrade rex
gc import install
gc import check
gc doctor --check rex-gascity-runtime:rex-binary --fix
```

`gc import check` is read-only. The doctor command downloads and runs the new
release binary now.

A bare version pins one tag, so `gc import upgrade rex` leaves it in place. To move to one explicit version, or to roll back to an older one,
remove the import and add it again with the bare version and the `//pack` URL
form.

```sh
gc import remove rex
gc import add https://github.com/docbobo/rex-gascity//pack --name rex --version 2026.11.26
gc import install
gc import check
gc doctor --check rex-gascity-runtime:rex-binary --fix
```

The cache keeps one directory per version and has no eviction. Old
`<version>/<os>-<arch>` directories stay until you remove them by hand.
`GC_RUNTIME_CACHE_DIR` only redirects new lookups.

A running supervisor or controller picks up the new provider only after a
restart. [City configuration](#city-configuration) covers the mechanism.
Session panes that the old provider started keep running on the old binary
until they restart.

### City configuration

```toml
[session]
provider = "rex"
```

`GC_SESSION=rex gc start` selects the provider for the process that constructs it. In a city with a running supervisor or controller, that process reads its own environment. Set `[session] provider` in `city.toml` and restart the supervisor instead.

### Details

- gc matches the version constraint against git tags with `git ls-remote`, and
  `packs.lock` pins the tag commit, so the import is reproducible.
- `--version sha:<commit>` pins one commit instead. It installs only when that
  commit's `[pack] version` has a published release. Otherwise the install
  fails and names the tag.
- A private repository needs git credentials for `git ls-remote` and the
  fetch. A private release needs `gh` with `GITHUB_TOKEN` or `GH_TOKEN`.
- Release assets exist for `darwin` and `linux` on `amd64` and `arm64`.
  `checksums.txt` ships beside them, and the installer verifies sha256 with
  `sha256sum` or `shasum -a 256` before it writes anything. It stages the file
  and renames it into place, so a running provider keeps its inode.
- `sh pack/install.sh` puts the provider on `PATH` by hand. It writes
  `$HOME/.local/bin`, or the directory of the `gc-runtime-rex` already on
  `PATH`. That copy does not warm the version-keyed cache. The doctor check
  still reports the cache as cold.
- `--from-source` builds from a checkout with Go. Such a build reports the
  module pseudo-version, and the doctor check warns that it cannot confirm the
  pack version.

## Doctor

The provider check is `rex-gascity-runtime:rex-binary`. The UI check, `rex-gascity-runtime:rex-ui`, is described in [`docs/rex-gascity-ui.md`](docs/rex-gascity-ui.md). Every run is read-only. It never starts a session, writes state or downloads. Exit 0 is OK, 1 is a warning and 2 is an error.

| Subject | Error | Warning |
|---|---|---|
| Provider | The cache is cold. The binary is not executable. The handshake is not RPP v0. A determined version differs from `[pack] version`. | The version cannot be determined (a source build, or an older binary with no `version` op). |
| Shim | `[pack] version`, `bin/_lib.sh` or the cache path is missing. No release asset exists for this platform. | |
| rex CLI | It is not on `PATH` (or in `GC_REX_BIN`). It is not executable. It is older than `pack/rex-min-version`. | The version cannot be determined. |

The provider version compares equal after a leading `v` and any `+build-metadata` suffix are stripped. A tagged `v2026.11.26` and a `-ldflags`-stamped `2026.11.26` match. `make build` stamps `[pack] version`.

The check compares versions only. It cannot tell two builds of one version apart.

The rex CLI pin is `pack/rex-min-version` (`0.1.0-preview.21`), the newest released Rex app. The CI conformance job reads the same file and installs the archive for that version, so the two cannot drift. The pin compares as a minimum, so a newer CLI passes. The version comes from `rex version --json`'s `rex_version` field. When `--json` is unsupported, it comes from the first line of `rex version`. An older CLI is an error.

## How sessions run

### The mapping

| Gas City | Rex | Label |
|---|---|---|
| City | Session | `gc:<city>` |
| Rig | Tab (window) | `Rig: <rig>`, or the city pin (else `Gas City`) for town-level agents |
| Agent | Pane (block) | `<agent>`, or the full session name for town-level agents |

The first agent in a rig creates a `Rig: <rig>` tab. A town-level agent uses the city pin, or `Gas City` when no pin is set. Later agents in the same rig split panes into that tab. Each new pane splits the largest-area pane along its longer axis at 50%, so panes stay balanced.

`stop` closes only the agent's block. When the last pane in a tab closes, Rex removes the tab. Teardown leaves the `gc:<city>` session in place, even when empty. It never stops the shared Rex server. A later `start` reuses the session.

A launched command that exits closes its pane. The provider sets no keep-open. The provider reports the agent as stopped rather than erroring.

### Identity, state and environment

RPP v0 calls carry no city. The provider resolves it from the environment.

| Variable | Default | Purpose |
|---|---|---|
| `GC_REX_BIN` | `rex` (via `PATH`) | Path or name of the `rex` CLI the provider drives |
| `GC_EXEC_CITY_NAME` | unset | gc's per-op city name, highest precedence |
| `GC_EXEC_CITY_PATH` | unset | gc's per-op city path. Its last component names the city when `GC_EXEC_CITY_NAME` is unset. |
| `GC_REX_CITY` | `basename($GC_CITY_PATH)`, then `$GC_CITY`, else `"default"` | City namespace for `gc:<city>` and sidecar state |
| `GC_REX_STATE_DIR` | `$XDG_STATE_HOME/rex-gascity` or `~/.local/state/rex-gascity` | Base directory for per-city sidecar state |
| `GC_SESSION` | unset | Selects the provider for a foreground controller (`rex`) |
| `GC_REX_TRACE` | unset (off) | Provider op trace, see tracing below |

Bindings, activity observations and metadata are stored per city under `<state dir>/<encoded-city>/`. The directory holds `bindings/`, `activity/`, `meta/` and a `lock` file. The city name is percent-encoded, so distinct names map to distinct directories. This holds on case-insensitive filesystems too.

### Provider ops

```json
{"version":0,"capabilities":["report-attachment","report-activity","idle-boundary","carrier.ops","input-outcome","observe.v1","city-scoped"]}
```

Implemented operations: `protocol`, `start`, `stop`, `attach`, `is-running`, `interrupt`, `nudge`, `send-keys`, `clear-scrollback`, `process-alive`, `peek`, `list-running`, `get-last-activity`, `idle`, `program-status`, `is-attached`, `set-meta`, `get-meta`, `remove-meta`, `observe`, `inventory`, `kill-object`. `proc.exec`, `proc.provision` and `watch-startup` are not implemented. They exit 2 without reading stdin.

### Provider tracing

`GC_REX_TRACE` appends one JSONL line per provider op. It is off by default and costs nothing when off.

| Value | Destination |
|---|---|
| unset or empty | off |
| `1` or `on` | `<resolved city state dir>/trace/provider-<YYYY-MM-DD>.jsonl` (UTC day) |
| any other value | that literal path, used unchanged |

gc forwards the caller's environment unchanged. Export the variable in the controller or session environment that launches `gc-runtime-rex`. Do not set it in a Rex or `city.toml` config. Set `GC_REX_CITY` in the same environment to keep the default path under your city's state directory. Without a city pin, the resolved city is `default`.

Each line holds the op name and the provider method the op calls. It also holds the duration in microseconds, the outcome and the exit code, and the resolved city. It holds the session and agent when the op has a name. No environment map and no start-config value is written.

The replies `protocol` and `version` write no line. An unknown op writes no line. Tracing never changes an op's own outcome or exit code. The cap is a best-effort 128 MiB per file. A line that would pass it is skipped with one stderr warning. The file is named by day but never rotated or deleted.

```sh
scripts/provider-trace-stats.sh <file.jsonl> [<file.jsonl> ...]
```

The reader needs `jq`. It prints one row per method, sorted by total time. The columns are `method count total_us p50_us p95_us max_us`.

## Tests

### Unit and e2e

```sh
make test            # go test ./..., no rex needed
make vet             # go vet ./...
make test-e2e        # fake-rex e2e suites plus pack/tests/check-import.sh
```

The e2e suites select the in-repo fake `rex` (`internal/rex/fakerex`) when `REX_E2E` is unset or `fake`. They need no Rex install. The real `cli.Client` runs unmodified against the fake. Argv, wire shapes, paging and child processes are exercised.

The fake emulates the CLI surface the provider uses. It also emulates the API methods those calls reach and runs real child processes for panes. It applies Rex's 983040-byte format page cap with `blob_read` paging. It returns exit codes 2, 3, 4 and 1. The fake's contract test reads `docs/REX-CONTRACT.md` and fails when the fake and the pinned contract drift.

`REX_E2E=1` or `REX_E2E=live` selects a real Rex server. That mode skips when no rex is on `PATH`. It mutates the configured server.

```sh
REX_E2E=1 go test ./internal/provider -run TestE2E -v
REX_E2E=1 go test ./cmd/gc-runtime-rex -run TestE2E -v
```

Any other `REX_E2E` value fails the test, so a typo never silently means the fake. See `docs/fake-rex.md` for what the fake covers and what it does not.

### Conformance

```sh
make check           # build, then gc runtime check rex --city testdata/city
make conformance     # pack/tests/conformance.sh --binary bin/gc-runtime-rex
```

`pack/tests/conformance.sh` never builds and needs no repository checkout. It runs in a scratch city and a scratch `GC_EXEC_STATE_DIR`. It drops inherited `REX_SERVER`, `REX_SESSION`, `REX_BLOCK` and `GC_REX_CITY`, so the run tests its own city.

It exits non-zero unless gc reported `ok: true` with `failed: 0`. `v2_ready` and `v2_failed` are recorded as evidence, never gated. It needs `gc` on `PATH` (or `GC_BIN`) and `python3` to read the JSON report.

### Pack checks

```sh
make verify                       # bash pack/tests/verify.sh
sh pack/tests/check-docs.sh       # runs the sh fences of the operator docs
```

`verify.sh` runs every `pack/tests/check-*.sh` in sorted order. It stops at the first failure. The checks need no network and no GitHub auth. They serve fake releases from local fixtures and a local bare repository.

| Check | Proves |
|---|---|
| `check-import.sh` | Tag import and release install work end to end. The `sha:` case fails and names the tag. |
| `check-docs.sh` | Every `sh` fence in `docs/rex-runtime-pack.md`, `docs/rex-binary-install.md` and `docs/rex-gascity-ui.md` runs against a scratch city. The version-keyed cache and the manual `PATH` install both reach the documented version. |
| `check-installer.sh` | `pack/install.sh` works over fake `curl`, `gh` and `uname`, including checksum, platform and destination failures. |
| `check-runtime-shim.sh` | The self-installing shim handles a cache hit, a cache miss, the deadline, a platform rejection and clean stdout. |
| `check-doctor-rex-version.sh` | The doctor check fails on an older rex CLI and passes on an equal or newer one. |
| `check-trace-stats.sh` | `scripts/provider-trace-stats.sh` prints the golden table. It skips with no `jq`. |
| `check-ci.sh` | `scripts/docs-only.sh` accepts only documentation paths, and `scripts/ci-timings.sh` computes p50 and p90 from job durations. Neither needs the network. |
| `check-conformance.sh` | Conformance exits 0 on a real provider and non-zero on a failing one. |
| `check-release.sh`, `check-release-version.sh` | The release gate accepts only `v<[pack] version>` and the workflow runs it. |

CI runs `make test`, the fake-backed e2e suites, `make verify` and the conformance job. The conformance job installs the pinned real rex first, then runs `pack/tests/conformance.sh`. The pin and its checksum are in `.github/workflows/ci.yml`. A newer preview never changes what CI tests.

Both jobs cache the pinned `gc` binary and skip the Conformance steps on a docs-only pull request. Every job prints its own wall time, and `scripts/ci-timings.sh` reports p50 and p90 per job for the last 30 days. `docs/ci.md` has the job map, the measured cost and the budgets.

`pack/release.sh` reruns `pack/tests/verify.sh` and `pack/tests/conformance.sh` against the downloaded release asset. It attaches `conformance.txt` to the draft. A tag pushed by hand builds nothing. The build runs on workflow dispatch.

`--dry-run` prints the resolved commit and asset names. `--resume` continues a draft. Publishing needs approval.

## Known limits

- No city in RPP v0 calls. Under gc the city is the name gc serves, because gc sets `GC_EXEC_CITY_NAME` and `GC_EXEC_CITY_PATH` on every op. Without a pin (an older gc, or the CLI fallback provider) the city falls back to `basename($GC_CITY_PATH)`, `$GC_CITY`, then `"default"`. Under `launchd` gc passes neither variable. Unpinned cities then share one `gc:default` session and state directory.
- No per-tab pane cap. Panes stay balanced by area, but a rig with many agents still divides the tab indefinitely.
- A command that exits closes its pane, as described above.
- No cache eviction. Old provider versions stay under the cache root.
- The doctor check compares versions only. It cannot detect two builds of one version, and a source build only warns.
- Conformance needs `gc` and `python3`. The pack itself needs neither at runtime.

## Development

```sh
make help            # list targets
make build           # bin/gc-runtime-rex, stamped with the pack version
make vet             # go vet ./...
make test            # go test ./...
make test-e2e        # fake-rex e2e suites, no rex install needed
make verify          # every pack/tests/check-*.sh
make check           # gc runtime check against testdata/city
make conformance     # gc runtime conformance against testdata/city
make hooks           # enable the versioned git hooks in .githooks
```

Go 1.27 or newer builds the provider. `make hooks` sets `core.hooksPath=.githooks`. The pre-commit hook runs `golangci-lint` on the staged Go files and blocks the commit on findings.

The linter set is pinned in `.golangci.yml` (`errcheck`, `govet`, `ineffassign`, `staticcheck`, `unused`). Run the whole repository by hand with `golangci-lint run ./...`.

## Documentation

- `docs/rex-runtime-pack.md` - operator quickstart: import, install, auth, upgrades, the release flow.
- `docs/rex-binary-install.md` - installer and shim internals.
- `docs/rex-provider.md` - provider reference: configuration, identity mapping, state and environment.
- `docs/provider-trace.md` - the op trace schema and the stats reader.
- `docs/fake-rex.md` - the test fake: coverage, limits and the live-server escape hatch.
- `docs/REX-CONTRACT.md` - the pinned Rex CLI and API surface, and exit codes.
- `docs/provider-herdr-parity.md` - the method-by-method parity record against the herdr-core provider.
