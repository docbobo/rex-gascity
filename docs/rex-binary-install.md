# Installing the rex provider binary

GCREX-120. How the pack installs `gc-runtime-rex`: where the binary comes
from, where it goes, and what `gc doctor --fix` runs.

## The gap

Nothing in gc installs a pack's runtime provider binary for the city. gc
`v1.5.1` defines no build or install hook in
`docs/reference/specs/pack-spec.md`. Pack scripts are doctor checks and
commands. `gc import install` and `gc pack unpack` only write files into the
pack root and compose. gc resolves a declared runtime's bare command name
through `exec.LookPath` (`internal/runtime/exec/exec.go:50-53`). A fresh city
has the pack but no `gc-runtime-rex` on PATH, so every rex session fails to
start.

gc executes a pack check's fix as `exec.Command(FixScript)` with
`cmd.Dir = c.PackDir` and `GC_PACK_DIR` in the environment. A non-zero
exit surfaces the captured output (`internal/doctor/pack_checks.go:67-91`).
That is the only remediation path gc gives a pack.

## What the fix does

`pack/doctor/rex-binary/fix.sh` is a thin wrapper. It resolves the pack
directory from its own path and runs `pack/install.sh` from there. The install
needs no Go toolchain and no source checkout.

`pack/install.sh`:

1. Reads `[pack] version` from `pack.toml`. The release is `v<version>`.
2. Maps `uname -s` and `uname -m` to a release asset,
   `gc-runtime-rex_<os>_<arch>` (`darwin` or `linux`, `amd64` or
   `arm64`). Other pairs fail and name the value.
3. Downloads `checksums.txt` and the asset from
   `https://github.com/docbobo/rex-gascity/releases/download/v<version>/`
   with `curl`. It checks the HTTP status, because a 404 body is not a
   checksum file. On a non-2xx answer, or when curl is absent, it retries with
   `gh release download v<version>`. No operator flag selects the method.
4. Verifies the sha256 with `sha256sum` (Linux) or `shasum -a 256` (macOS)
   before it installs anything. A mismatch aborts and prints both hashes.
5. Installs to the directory of the `gc-runtime-rex` already on PATH, else
   `$HOME/.local/bin` (see below). It copies through a temp file, then
   applies `chmod 0755` and `mv`. A running process keeps its inode. A
   repeat install of the same release leaves the file alone and reports
   success.
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
text.

## Where the binary goes

One destination: the directory of the provider already on PATH, else
`$HOME/.local/bin`. The script never chooses between `GOBIN` and
`~/.local/bin`. It also never writes inside the pack, for two reasons. gc
only looks on PATH. Also, `updatePack` in
`internal/config/pack_fetch.go:76-83` discards local files with
`git checkout --force .` and `git clean -fd`.

A supervisor starts the city with its own environment. The directory must be
on that PATH, not only on an interactive shell's PATH. The check's
remediation says so, and the success output names the path it found the
provider at.

## `sha:` imports and versions

The release is chosen by `[pack] version`, not by the imported commit. A
`sha:` import works only when that commit's `[pack] version` has a published
release. Otherwise the install fails and names the tag. The doctor check
compares versions only. It cannot detect a same-version build from a different
commit.

## Failure contract

Each failure exits non-zero with one reason on stderr, which gc doctor shows.
The cases are an unsupported OS or machine, no `[pack] version`, and no
`curl` or `gh`. They also include no release for the tag (404, with
the private repository rule), a network failure, a missing `checksums.txt`
entry, and a sha256 mismatch.

`cmd/gc-runtime-rex/doctor_fix_test.go` drives the real `fix.sh` the way gc
does, with fake `curl` and `gh` scripts on PATH. It covers the install, the
gh fallback after a curl 404, and the checksum abort.
`pack/tests/check-installer.sh` covers the installer seam end to end. It
includes the arch mapping, the install directory, the PATH shadow warning,
the private repository fallback, and the `gc doctor --fix` path.

## Scope

Not covered: a build for a platform other than the fix's host, and a refresh
of the binary on a pack version bump. Both are install re-runs. A pack update
does not call the fix automatically, so the check catches the stale binary and
the remediation installs again.
