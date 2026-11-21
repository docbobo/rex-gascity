# The rex runtime pack

A Gas City runtime pack for [Rex](https://rex.dev). It registers one runtime,
`rex`, whose provider binary `gc-runtime-rex` speaks RPP v0 and drives the
`rex` CLI.

The provider is distributed as a prebuilt binary on the pack's GitHub
releases. A normal install needs no Go toolchain and no repository checkout.

This file is enough for a new operator to reach a green
`gc doctor --check rex:rex-binary`. The repository's
[`docs/rex-binary-install.md`](../docs/rex-binary-install.md) has the
internals.

## Prerequisites

- `gc` 1.5.1 or newer.
- The `rex` CLI on `PATH`. Set `GC_REX_BIN` to its path to override the
  lookup. This pack drives an external binary. A pure-Go runtime such as
  `cloudflare` does not.
- macOS or Linux on amd64 or arm64. Windows has no release asset.
- Write access to the install directory: the directory of the
  `gc-runtime-rex` already on `PATH`, else `$HOME/.local/bin`.

```sh
command -v gc
command -v rex
```

## Import the pack

No pack registry is involved. gc clones the git repository and uses the
`pack/` subdirectory as the pack root. The version constraint is matched
against git tags with `git ls-remote`.

```sh
gc import add https://github.com/docbobo/rex-gascity/tree/main/pack --name rex --version 0.2.0
```

- `--version` takes the bare semver `0.2.0`. The tag is `v0.2.0`, so
  `--version v0.2.0` matches no tag.
- The tag must be pushed before the import.
- A public repository needs no credentials. A private repository needs git
  credentials for `git ls-remote` and the fetch.
- `--version sha:<commit>` pins one commit instead of a tag. It installs
  only when that commit's `[pack] version` has a published release. See
  [Versions and upgrades](#versions-and-upgrades).

## Install

Put the install directory on `PATH` first, in the shell and in the
environment of the supervisor or controller that runs the city:

```sh
export PATH="$HOME/.local/bin:$PATH"
gc import install
gc doctor --check rex:rex-binary --fix
gc doctor --check rex:rex-binary
gc-runtime-rex protocol
gc import check
```

`gc import install` writes the pack files. It does not install the provider
binary, because gc has no pack install hook. `gc doctor --fix` runs the
pack's `doctor/rex-binary/fix.sh`, which runs `pack/install.sh`. Run
`sh pack/install.sh` yourself from the pack directory if you prefer.

A green `gc doctor --check rex:rex-binary` means the provider handshakes,
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

One destination: the directory `command -v gc-runtime-rex` reports, else
`$HOME/.local/bin`. The script never installs into `GOBIN` or inside the pack,
because gc rewrites the pack directory on install. A copy of
`gc-runtime-rex` earlier on `PATH` produces a warning that names it.

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

## PATH

`pack.toml` declares `[runtimes.rex] command = "gc-runtime-rex"`, a bare
name. gc resolves it through `PATH`. The install directory must be on the
`PATH` of the process that runs the city. A supervisor or controller starts
with its own environment, so set `PATH` there as well, not only in an
interactive shell.

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
gc import add https://github.com/docbobo/rex-gascity/tree/main/pack --name rex --version 0.3.0
gc import install
gc doctor --check rex:rex-binary --fix
gc doctor --check rex:rex-binary
```

To roll back, use the previous version in the same commands. A caret
constraint, `--version '^0.2.0'`, tracks a major line, and
`gc import upgrade rex` then moves to the newest matching tag.

## Source checkout

From a git checkout of this repository, with a Go toolchain:

```text
sh pack/install.sh --from-source
```

Such a build reports the module pseudo-version, so the doctor check warns
that it cannot confirm the pack version. The release asset is the default
path.

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
| Doctor reports a version mismatch | Re-run `gc doctor --check rex:rex-binary --fix`. The release for `pack.toml`'s version is the only accepted build. |
