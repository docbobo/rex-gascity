# Installing the rex provider binary

GCREX-120. How the pack installs `gc-runtime-rex`: where the binary comes
from, where it goes, and what `gc doctor --fix` runs. The pack's
[`README.md`](../pack/README.md) is the operator quickstart. This file has
the mechanism.

## The gap

Nothing in gc installs a pack's runtime provider binary for the city. gc
`v1.5.1` defines no build or install hook in
`docs/reference/specs/pack-spec.md`. Pack scripts are doctor checks and
commands. `gc import install` and `gc pack unpack` only write files into the
pack root and compose. gc resolves a declared runtime's bare command name
through `exec.LookPath` (`internal/runtime/exec/exec.go:50-53`). A fresh city
has the pack but no `gc-runtime-rex` on PATH, so every rex session fails to
start.

gc executes a pack check's fix as `exec.Command(FixScript)`. It sets
`cmd.Dir = c.PackDir` and puts `GC_PACK_DIR` in the environment. A non-zero
exit surfaces the captured output (`internal/doctor/pack_checks.go:67-91`).
That is the only remediation path gc gives a pack.

## Operator flow

No pack registry is involved. The import clones the git repository and uses
the `pack/` subdirectory as the pack root:

```sh
gc import add https://github.com/docbobo/rex-gascity/tree/main/pack --name rex --version 0.2.0
export PATH="$HOME/.local/bin:$PATH"
gc import install
gc doctor --check rex:rex-binary --fix
gc doctor --check rex:rex-binary
gc-runtime-rex version
gc import check
```

The tag `v0.2.0` must exist before the import, and it comes from
`pack/release.sh v0.2.0` (see [Cut a release](#cut-a-release)). `--version`
takes the bare semver. A public repository needs no git credentials. A private
repository needs them for `git ls-remote` and the fetch.

`gc import install` only materializes the pack. gc does not run the pack
install step by itself. `gc doctor --fix` runs it. An operator who prefers
to run it by hand does so from the repository root:

```sh
sh pack/install.sh
```

From the pack directory itself, the same script is `sh ./install.sh`.

## What the fix does

`pack/doctor/rex-binary/fix.sh` is a thin wrapper. It resolves the pack
directory from its own path and runs `pack/install.sh` from there. The
install needs no Go toolchain and no source checkout.

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
5. Installs to the directory of the `gc-runtime-rex` already on PATH, else
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

One destination: the directory of the provider already on PATH, else
`$HOME/.local/bin`. The script never chooses between `GOBIN` and
`~/.local/bin`. It also never writes inside the pack, for two reasons. gc
only looks on PATH. Also, `updatePack` in
`internal/config/pack_fetch.go:76-83` discards local files with
`git checkout --force .` and `git clean -fd`.

## PATH and the rex CLI

`pack.toml` declares `[runtimes.rex] command = "gc-runtime-rex"`, a bare
name, so gc resolves the provider through PATH. The install directory must be
on that PATH. A supervisor or controller starts the city with its own
environment. The directory must be on its PATH too, not only on an
interactive shell's PATH.

The `rex` CLI is a prerequisite. The provider drives it for every session
operation. `GC_REX_BIN` overrides the PATH lookup. This differs from the
`cloudflare` runtime, which has no external binary. The doctor check reports
a missing CLI with its own remediation.

## `sha:` imports and versions

The release is chosen by `[pack] version`, not by the imported commit. A
`sha:` import installs only when that commit's `[pack] version` has a
published release. Otherwise the install fails and names the tag. The doctor
check compares versions only. It cannot detect a same-version build from a
different commit.

To upgrade, re-pin the import and reinstall:

```sh
gc import remove rex
gc import add https://github.com/docbobo/rex-gascity/tree/main/pack --name rex --version 0.3.0
gc import install
gc doctor --check rex:rex-binary --fix
gc doctor --check rex:rex-binary
```

To roll back, import the previous tag and rerun the same commands.

## Cut a release

`pack/release.sh` cuts the release in one order: create the draft and its tag,
dispatch the build, wait for the assets, verify the downloaded asset, attach
`conformance.txt`, then publish. CI only builds into the draft.

Run `sh pack/release.sh v0.2.0 --dry-run` to print the resolved commit and
asset names without changing remote state. Run `sh pack/release.sh v0.2.0` to
cut the release.

The release branch is `main`, and `[pack].version` must be `0.2.0` in a
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

## Tests

- `cmd/gc-runtime-rex/doctor_fix_test.go` drives the real `fix.sh` the way
  gc does. It puts fake `curl` and `gh` scripts on PATH. It covers the
  install, the gh fallback after a curl 404, and the checksum abort.
- `pack/tests/check-installer.sh` covers the installer seam end to end. It
  includes the arch mapping and the install directory. It also includes the
  PATH shadow warning and the private repository fallback. The last part is
  the `gc doctor --fix` path.
- `pack/tests/check-import.sh` covers the import flow against a local bare
  repository and a fake `curl`. It imports by tag. It also imports the
  released commit as a `sha` version. One more assertion covers a commit
  without a release. The install must fail and name the tag.
- `pack/tests/check-docs.sh` runs every command in a `sh` fence. It covers
  this file and `pack/README.md`, against a scratch city.

## Scope

Not covered: a build for a platform other than the fix's host. Provenance is
also out of scope. So is a refresh of the binary on a pack version bump. A
pack update does not call the fix automatically. The check catches a stale
binary. The remediation installs again.
