# Installing the rex provider binary

GCREX-120. How the pack installs `gc-runtime-rex`: where the binary comes
from, where it goes, and what `gc doctor --fix` runs. The pack's
[`rex-runtime-pack.md`](rex-runtime-pack.md) is the operator quickstart. This file has
the mechanism.

## The gap

Nothing in gc installs a pack's runtime provider binary for the city. gc
`v1.5.1` defines no build or install hook in
`docs/reference/specs/pack-spec.md`. Pack scripts are doctor checks and
commands. `gc import install` and `gc pack unpack` only write files into the
pack root and compose. gc anchors a pack-declared runtime `command` at the
pack directory when it holds a path separator (`internal/config/pack_runtimes.go:94-102`).
A bare name stays as-is and is resolved through `exec.LookPath`
(`internal/runtime/exec/exec.go:50-53,83`).

This pack points `[runtimes.rex] command` at the shipped shim
`bin/gc-runtime-rex`, so the provider needs no PATH entry. Before the shim
existed, a fresh city had the pack but no `gc-runtime-rex` on PATH, so every rex
session failed to start.

gc executes a pack check's fix as `exec.Command(FixScript)`. It sets
`cmd.Dir = c.PackDir` and puts `GC_PACK_DIR` in the environment. A non-zero
exit surfaces the captured output (`internal/doctor/pack_checks.go:67-91`).
That is the only remediation path gc gives a pack.

## Operator flow

No pack registry is involved. The import clones the git repository and uses
the `pack/` subdirectory as the pack root:

```sh
gc import add https://github.com/docbobo/rex-gascity/tree/main/pack --name rex --version 2026.11.06
gc import install
gc doctor --check rex-gascity-runtime:rex-binary --fix
gc doctor --check rex-gascity-runtime:rex-binary
gc import check
```

The tag `v2026.11.06` must exist before the import, and it comes from
`pack/release.sh v2026.11.06` (see [Cut a release](#cut-a-release)). `--version`
takes the bare version. A public repository needs no git credentials. A private
repository needs them for `git ls-remote` and the fetch.

Import is enough. The provider runs on the first session with no `--fix` and no
`PATH` change. `gc doctor` verifies read-only: a cold cache reports "not yet
installed, run `--fix` or start a session" and downloads nothing. The `--fix`
above warms the cache now, off the session path.

`gc import install` only materializes the pack. gc does not run the pack
install step by itself. `gc doctor --fix` runs it into the version-keyed
cache that the shim reads. That is the path that makes a first session fast,
and the one the operator flow above uses.

A hand-run `sh pack/install.sh` is a different, optional path. From the
repository root:

```sh
sh pack/install.sh
```

From the pack directory itself, the same script is `sh ./install.sh`. With no
`GC_RUNTIME_INSTALL_DIR`, it installs onto PATH, not into the shim's cache
(see [Where the binary goes](#where-the-binary-goes)). It also waits for the
fetch with no deadline, as it always did.

A later session still installs into the cache, because the shim reads the
cache and never execs the PATH copy. Use the hand run only to give another
tool a provider on PATH.

## What the fix does

`pack/doctor/rex-binary/fix.sh` is a thin wrapper. It resolves the pack
directory from its own path and runs `pack/install.sh` from there. The
install needs no Go toolchain and no source checkout.

`--fix` refuses with a non-zero exit before any download when the installed
rex CLI is older than `pack/rex-min-version`. It cannot update the Rex app,
which bundles the CLI.

`pack/install.sh`:

1. Reads `[pack] version` from `pack.toml`. The release is `v<version>`.
2. Maps `uname -s` and `uname -m` to a release asset,
   `gc-runtime-rex_<os>_<arch>` (`darwin` or `linux`, `amd64` or
   `arm64`). Other pairs, including Windows, fail and name the value.
3. Downloads `checksums.txt` and the asset from
   `https://github.com/docbobo/rex-gascity/releases/download/v<version>/`
   with `curl`. It checks the HTTP status, because a 404 body is not a
   checksum file. On a non-2xx answer, or when curl is absent, it retries
   with `gh release download v<version> -R docbobo/rex-gascity`. The
   download goes into a temporary directory. No operator flag selects the
   method.
4. Verifies the sha256 with `sha256sum` (Linux) or `shasum -a 256` (macOS)
   before it installs anything. A mismatch aborts and prints both hashes.
5. Installs into `GC_RUNTIME_INSTALL_DIR` when it is set. That is the
   version-keyed cache the shim and `gc doctor --fix` use. Otherwise it
   installs to the directory of the `gc-runtime-rex` already on PATH, else
   `$HOME/.local/bin` (see below). It copies through a temp file in the same
   directory, then applies `chmod 0755` and `mv`. A running process keeps
   its inode. A repeat install of the same release leaves the file alone and
   reports success.
6. Warns when another `gc-runtime-rex` comes earlier on PATH, and names that
   path.

`--from-source` keeps `go install ./cmd/gc-runtime-rex` as an explicit
opt-in for a checkout. Such a build reports the module pseudo-version, so the
doctor check may warn that it cannot confirm the pack version.

## Private repository rule

The public repository serves release assets to plain `curl` with no token.
For a private repository, a bearer token does not work on
`releases/download/...`. The script then needs `gh` with `GITHUB_TOKEN`
or `GH_TOKEN`. The same rule is in the script footer and in the failure
text. The fallback is automatic, so making the repository public needs no
script change.

## Where the binary goes

`GC_RUNTIME_INSTALL_DIR` chooses the destination. When it is set, the script
installs there. The shim and `gc doctor --fix` set it to the version-keyed
cache. Otherwise the script installs to the directory of the provider already
on PATH, else `$HOME/.local/bin`. It never chooses `GOBIN`.

The script never writes the provider binary inside the pack. `updatePack` in
`internal/config/pack_fetch.go:76-83` discards local files with
`git checkout --force .` and `git clean -fd`. The script also excludes the
shipped `pack/bin/gc-runtime-rex` from default destination selection, so a
PATH that holds `pack/bin` cannot overwrite the tracked file.

## PATH, the cache, and the rex CLI

`pack.toml` declares `[runtimes.rex] command = "bin/gc-runtime-rex"`. gc
anchors that path at the pack directory, so the provider needs no PATH entry.
The shim execs the cached release binary on a hit. On a miss it installs into
the version-keyed cache first.

The cache root is
`${GC_RUNTIME_CACHE_DIR:-${XDG_CACHE_HOME:-$HOME/.cache}/rex-gascity}`. The
binary lands at `<root>/<version>/<os>-<arch>/gc-runtime-rex`. There is no
cache eviction in v1. Remove stale version directories by hand.

Changing `GC_RUNTIME_CACHE_DIR` to a fresh root only redirects new lookups and
installs. It does not remove the old directories. Delete them from the old
root to reclaim that space.

The `rex` CLI is a prerequisite. The provider drives it for every session
operation. `GC_REX_BIN` overrides the PATH lookup. This differs from the
`cloudflare` runtime, which has no external binary. The doctor check reports
a missing CLI with its own remediation.

## First use and recovery

A cold first use installs the release on the session's start path. It
succeeds only when a published release matches `[pack] version`, its assets
are reachable, and the cache root is writable.

The shim bounds that fetch to keep it under gc's 30 s `protocol` and 120 s
`start` deadlines. It reads `GC_RUNTIME_INSTALL_DEADLINE` as an integer in
[1, 25] seconds and defaults to 25. A non-numeric or out-of-range value falls
back to 25. A slow or stalled fetch fails with a cause-naming message and the
`gc doctor --fix` remedy, not an unexplained gc timeout.

`gc doctor --fix` fetches off gc's deadline with a generous 300 s budget. A
release too slow for the shim's budget still warms. It honors
`GC_RUNTIME_INSTALL_DEADLINE` too: the fix hook uses that value when the
environment sets it, and 300 otherwise. gc passes the environment through to
the hook (`internal/doctor/pack_checks.go:67-79`).

If even 300 s is not enough, set the variable and run the fix hook by hand:

```sh
GC_RUNTIME_INSTALL_DEADLINE=600 sh pack/doctor/rex-binary/fix.sh
```

That hook computes the version-keyed cache dir from `bin/_lib.sh` and installs
there, so the next session cache-hits with no fetch. A bare
`GC_RUNTIME_INSTALL_DEADLINE=<secs> sh pack/install.sh` is not the recovery
path. It installs onto PATH, not into the cache, and leaves the shim cold.

## `sha:` imports and versions

The release is chosen by `[pack] version`, not by the imported commit. A
`sha:` import installs only when that commit's `[pack] version` has a
published release. Otherwise the install fails and names the tag. The doctor
check compares versions only. It cannot detect a same-version build from a
different commit.

To upgrade, re-pin the import and reinstall:

```sh
gc import remove rex
gc import add https://github.com/docbobo/rex-gascity/tree/main/pack --name rex --version 2026.11.06
gc import install
gc doctor --check rex-gascity-runtime:rex-binary --fix
gc doctor --check rex-gascity-runtime:rex-binary
```

To roll back, import the previous tag and rerun the same commands.

## Cut a release

`pack/release.sh` cuts the release in one order: create the draft and its tag,
dispatch the build, wait for the assets, verify the downloaded asset, attach
`conformance.txt`, then publish. CI only builds into the draft.

Run `sh pack/release.sh v2026.11.06 --dry-run` to print the resolved commit and
asset names without changing remote state. Run `sh pack/release.sh v2026.11.06`
to cut the release.

The release branch is `main`, and `[pack].version` must be `2026.11.06` in a
committed tree. `--dry-run` prints the resolved commit and asset names and
changes no remote state. `--resume` continues an existing draft.

Before the draft is created, the script runs `pack/tests/verify.sh`. After the
build, it downloads `gc-runtime-rex_<os>_<arch>` and `checksums.txt`, verifies
sha256, and runs `pack/tests/conformance.sh` on that downloaded binary. The
published release therefore carries an artifact produced by CI, not by the
developer's checkout.

Publishing needs human approval. A failed conformance leaves the draft in
place, attaches the failure output, and prints
`gh release delete <tag> --cleanup-tag`.

## Trust model

`checksums.txt` comes from the same release as the binary. It catches
corruption in transit and a truncated download. It does not prove
provenance. Whoever can replace the release asset can replace the checksum
too. A compromised release is out of scope for GCREX-120. GitHub artifact
attestations are a later option.

## Failure contract

Each failure exits non-zero with one reason on stderr. gc doctor shows that
reason. One case is an unsupported OS or machine. Others are no
`[pack] version` and no `curl` or `gh`. The list also has no release for
the tag (404, with the private repository rule) and a network failure. A
missing `checksums.txt` entry and a sha256 mismatch complete the list.

The installer also refuses to replace a working provider with an asset that
cannot answer `inventory`. A stub asset, or a release that only names its
version, turns every supervisor cycle into a failed call. The fix reports
that conflict and names the installed file to remove first.

## Tests

- `cmd/gc-runtime-rex/doctor_fix_test.go` drives the real `fix.sh` the way
  gc does. It puts fake `curl` and `gh` scripts on PATH. It covers the
  install, the gh fallback after a curl 404, and the checksum abort. The
  release assets delegate every other op to a provider built from source,
  so the test proves the installed binary answers `inventory` and
  `observe`. One more case proves a stub asset does not replace a working
  provider.
- `pack/tests/check-installer.sh` covers the installer seam end to end. It
  includes the arch mapping and the install directory. It also includes the
  PATH shadow warning and the private repository fallback. The last part is
  the `gc doctor --fix` path. That path uses a fake `rex`, so it needs no
  rex install; only `gc` must be on PATH.
- `pack/tests/check-import.sh` covers the import flow against a local bare
  repository and a fake `curl`. It imports by tag. It also imports the
  released commit as a `sha` version. One more assertion covers a commit
  without a release. The install must fail and name the tag.
- `pack/tests/check-docs.sh` runs every command in a `sh` fence. It covers
  this file and `docs/rex-runtime-pack.md`, against a scratch city.

## Scope

Not covered: a build for a platform other than the fix's host. Provenance is
also out of scope. So is a refresh of the binary on a pack version bump. A
pack update does not call the fix automatically. The check catches a stale
binary. The remediation installs again.
