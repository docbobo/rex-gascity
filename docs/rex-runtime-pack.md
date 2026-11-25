# The rex runtime pack

A Gas City runtime pack for [Rex](https://rex.dev). It registers one runtime,
`rex`. Its provider binary `gc-runtime-rex` speaks RPP v0 and drives the
`rex` CLI.

The provider is distributed as a prebuilt binary on the pack's GitHub
releases. A normal install needs no Go toolchain and no repository checkout.

This file is enough for a new operator to reach a green
`gc doctor --check rex-gascity-runtime:rex-binary`. The repository's
[`rex-binary-install.md`](rex-binary-install.md) has the
internals.

## Prerequisites

- `gc` 1.5.1 or newer.
- The `rex` CLI on `PATH`. Set `GC_REX_BIN` to its path to override the
  lookup. This pack drives an external binary. A pure-Go runtime such as
  `cloudflare` does not.
- macOS or Linux on amd64 or arm64. Windows has no release asset.
- Write access to the runtime cache directory:
  `${GC_RUNTIME_CACHE_DIR:-${XDG_CACHE_HOME:-$HOME/.cache}/rex-gascity}`.
  The first session (or `gc doctor --fix`) installs the provider there.

```sh
command -v gc
command -v rex
```

## Import the pack

No pack registry is involved. gc clones the git repository and uses the
`pack/` subdirectory as the pack root. The version constraint is matched
against git tags with `git ls-remote`.

```sh
gc import add https://github.com/docbobo/rex-gascity/tree/main/pack --name rex --version 2026.11.05
```

- `--version` takes the bare version `2026.11.05`. The tag is `v2026.11.05`,
  so `--version v2026.11.05` matches no tag.
- The tag must be pushed before the import.
- A public repository needs no credentials. A private repository needs git
  credentials for `git ls-remote` and the fetch.
- `--version sha:<commit>` pins one commit instead of a tag. It installs
  only when that commit's `[pack] version` has a published release. See
  [Versions and upgrades](#versions-and-upgrades).

## Install

Import is enough. `gc import install` writes the pack files. The pack's
`[runtimes.rex] command` points at the shipped shim `bin/gc-runtime-rex`, which
gc runs relative to the pack directory. On the first rex session the shim
installs the matching release into the version-keyed cache and execs it. No
`gc doctor --fix` and no `PATH` edit are needed.

```sh
gc import install
gc doctor --check rex-gascity-runtime:rex-binary --fix
gc doctor --check rex-gascity-runtime:rex-binary
gc import check
```

`gc doctor --check rex-gascity-runtime:rex-binary` verifies the provider
read-only. It probes the cached binary and never downloads. On a cold cache it
reports "not yet installed, run `--fix` or start a session". `gc doctor --fix`
runs the pack's `doctor/rex-binary/fix.sh`, which warms that same cache without
waiting for a session. Only that cache counts as a repair. A manual
`sh pack/install.sh` writes a PATH copy instead of warming the cache, so the
next check still reports the provider as missing. Use the manual script only to
put the provider on `PATH`.

A green `gc doctor --check rex-gascity-runtime:rex-binary` means the provider handshakes,
its version matches `pack.toml`, and the `rex` CLI resolves. A full
`gc doctor` also checks the other components of the city.

## What install downloads

Release `v<version>` of `docbobo/rex-gascity` holds:

| Asset | Platform |
|---|---|
| `gc-runtime-rex_darwin_amd64` | macOS on Intel |
| `gc-runtime-rex_darwin_arm64` | macOS on Apple silicon |
| `gc-runtime-rex_linux_amd64` | Linux on amd64 |
| `gc-runtime-rex_linux_arm64` | Linux on arm64 |
| `checksums.txt` | sha256 of each asset |

`pack/install.sh` maps `uname -s` and `uname -m` to the asset for the
host. It fetches the asset and `checksums.txt` from
`https://github.com/docbobo/rex-gascity/releases/download/v<version>/`, and
verifies sha256 with `sha256sum` on Linux or `shasum -a 256` on macOS
before it installs. An unsupported OS or machine fails and names the value.

The install is atomic. The script stages the file in the destination
directory, applies `chmod 0755`, then renames it. A running provider keeps
its inode. A repeat install of the same version reports success and replaces
nothing in use.

`pack/install.sh` picks one destination. When the shim or `gc doctor --fix`
runs it, `GC_RUNTIME_INSTALL_DIR` names the version-keyed cache. Run by hand
with no such variable, it installs to the directory `command -v
gc-runtime-rex` reports, else `$HOME/.local/bin`.

It never installs into `GOBIN` or inside the pack, because gc rewrites the
pack directory on install. A copy of `gc-runtime-rex` earlier on `PATH`
produces a warning that names it.

## Auth

`pack/install.sh` tries the public URL with `curl` first. If that fails,
or `curl` is absent, it runs
`gh release download v<version> -R docbobo/rex-gascity` automatically.

- Public repository: `curl` alone is enough. No token and no `gh`.
- Private repository: `gh` with `GITHUB_TOKEN` or `GH_TOKEN`. Plain
  `curl` with a token cannot fetch `releases/download/...`.

The fallback is automatic, so making the repository public needs no script
change. The failure text names the case. A 401 or 404 with no `gh` is an
auth or visibility problem. A transport error is a network problem.

## PATH and the cache

`pack.toml` declares `[runtimes.rex] command = "bin/gc-runtime-rex"`. The
path separator makes gc resolve it relative to the pack directory, not through
`PATH`. The provider needs no `PATH` entry.

The shim caches the release binary at
`${GC_RUNTIME_CACHE_DIR:-${XDG_CACHE_HOME:-$HOME/.cache}/rex-gascity}/<version>/<os>-<arch>`
and execs it on a hit with no network I/O.

There is no cache eviction in v1. Stale `<version>/<os>-<arch>` directories
stay in place after an upgrade. Remove them by hand.

Changing `GC_RUNTIME_CACHE_DIR` to a fresh root only redirects new lookups and
installs. It does not remove the old directories. Delete them from the old
root to reclaim that space.

Only the provider binary is PATH-independent. The `rex` CLI still needs
`PATH` or `GC_REX_BIN`.

## Versions and upgrades

The release is `v<[pack] version>`, not the imported commit. Two
consequences:

- A `sha:<commit>` import installs only when that commit's `[pack] version`
  has a published release. Otherwise the install fails and names the tag to
  use.
- The doctor check compares versions only. It cannot tell two builds of the
  same version apart.

To upgrade, re-pin the import and reinstall:

```sh
gc import remove rex
gc import add https://github.com/docbobo/rex-gascity/tree/main/pack --name rex --version 2026.11.05
gc import install
gc doctor --check rex-gascity-runtime:rex-binary --fix
gc doctor --check rex-gascity-runtime:rex-binary
```

To roll back, use the previous version in the same commands. A caret
constraint tracks the major line: `--version '^2026.11.05'` matches later
2026 releases and not a 2027 release. `gc import upgrade rex` then moves to
the newest matching tag.

## Gas City UI

A second check, `rex-gascity-runtime:rex-ui`, covers the Gas City UI, a Rex
extension that is not part of the provider. It is optional: a missing UI is a
warning (exit 1), and a config that does not load or a missing `rex` is an
error (exit 2). The check is read-only. `gc doctor --check rex-gascity-runtime:rex-ui --fix`
installs the UI for the pack's version and wires the Rex config. Install,
doctor rows, the unwritable `init.lua` case, upgrade and uninstall are in
[`rex-gascity-ui.md`](rex-gascity-ui.md).

## Source checkout

From a git checkout of this repository, with a Go toolchain:

```text
sh pack/install.sh --from-source
```

Such a build reports the module pseudo-version, so the doctor check warns
that it cannot confirm the pack version. The release asset is the default
path.

## Build and release

`pack/release.sh` is the only publisher. CI builds into the draft that the
script creates and never publishes.

Run `sh pack/release.sh v2026.11.05 --dry-run` to print the resolved commit and
asset names without changing remote state. Run `sh pack/release.sh v2026.11.05`
to create the draft, build, verify, ask, and publish. Run
`sh pack/release.sh v2026.11.05 --resume` to continue an existing draft.

It refuses a dirty tracked tree, a branch other than `main`, a tag that
disagrees with `[pack] version`, a tag that points at another commit, and a
release that is already published. An existing draft needs `--resume`, and
`--resume` refuses a tag that points at another commit.

A run creates the draft, pushes the tag at the current commit, dispatches
`release.yml`, waits a
bounded time for the four binaries and `checksums.txt`, downloads the asset
for this host, checks its sha256, runs `pack/tests/verify.sh` and
`pack/tests/conformance.sh` against that downloaded asset, and writes
`conformance.txt`. The file records PASS or FAIL, the conformance output, the
rex version, the gc version and commit, the asset name and sha256, the host,
the UTC time and the tag commit.

A failing conformance attaches the evidence, leaves the draft in place, prints
`gh release delete <tag> --cleanup-tag`, and exits non-zero. There is no skip
case. Publish needs approval: the prompt is interactive, and `--yes` is for a
run where a human already approved.

Bump `[pack].version` and commit before the release run. The build gate refuses
a tag that disagrees with it.

## Trust model

`checksums.txt` ships in the same release as the binary. It catches a
corrupt or truncated download. It does not prove provenance. Whoever can
replace the asset can replace the checksum. A compromised release is out of
scope for GCREX-120. GitHub artifact attestations are a later option.

## Troubleshooting

| Symptom | Cause and fix |
|---|---|
| `404` or `401` from the download | The repo is private, or the tag has no release. Use `gh` with a token, or check the tag. |
| `sha256 mismatch` | The download is corrupt. Run the install again. |
| `unsupported operating system` or `unsupported machine` | No asset exists for the host. Use macOS or Linux on amd64 or arm64. |
| `... is not on PATH` warning | Add the named directory to `PATH`, including the supervisor's environment. |
| Doctor reports a version mismatch | Re-run `gc doctor --check rex-gascity-runtime:rex-binary --fix`. The release for `pack.toml`'s version is the only accepted build. |
