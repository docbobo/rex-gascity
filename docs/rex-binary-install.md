# Installing the rex provider binary: gc doctor --fix

`gcrex-v6a4` — GCREX-62. How the `rex:rex-binary` doctor check's fix works, and
why the binary goes where it goes.

## The gap

Nothing in gc builds or installs a pack's runtime provider binary for the city.
At gc `v1.5.1`, `docs/reference/specs/pack-spec.md` defines no build or install
hook: pack scripts are doctor checks (§ 1.2.11, line 568) and commands
(§ 1.3.4, line 770), and `gc import install` / `gc pack unpack` write files into
the pack root and compose; neither builds. gc resolves a declared runtime's
bare command name through `exec.LookPath`
(`internal/runtime/exec/exec.go:50-53`, `NewProvider`: "The script path may be
absolute, relative, or a bare name resolved via exec.LookPath"). So a fresh
city has the pack but no `gc-runtime-rex` on PATH, and every rex session fails
to start.

Conflict, reported as such: § 1.2.8 of the same spec (line 518) says a
`pack-runtimes` doctor check verifies each declared executable is installed and
handshakes, but `v1.5.1` has no such check in `internal/doctor/` (no file or
identifier matches `pack-runtimes` there; only `internal/runtime/REQUIREMENTS.md`
mentions the name). Treat the check as unverified/not present in the pinned
baseline, not as a fix path that exists.

gc executes a pack check's fix as `exec.Command(FixScript)` with
`cmd.Dir = c.PackDir` and `GC_PACK_DIR` in the environment, a non-zero exit
surfacing the captured output (`internal/doctor/pack_checks.go:67-91`); the
spec documents `fix.sh` as the opt-in (§ 1.2.11, line 581). That is the only
remediation path gc gives a pack, and it is the contract `fix.sh` is written
against.

The doctor check already existed (`pack/doctor/rex-binary/`) and already told
the user to run `gc doctor --fix`; there was no fix for it to run. That fix is
the install path.

## What the fix does

`pack/doctor/rex-binary/fix.sh`, from the pack directory:

1. Resolves the module root as the directory above the pack. The pack is a
   subdirectory of the module whether the city composes it through a local
   include or a remote import, so the same script works in both.
2. Requires `go.mod` there. Absent, the pack was copied without its module and
   nothing can be built — it fails and says so.
3. Reads `[pack] version` from `pack.toml` and stamps it into the binary with
   `-ldflags "-X main.buildVersion=<version>"`, which is the same variable the
   check compares against, and the same mechanism `make build` uses. Stamping
   the declared version rather than the commit's is deliberate: the check's
   contract is "provider matches the pack version", and a tagged install and a
   `-ldflags`-stamped build are meant to compare equal (`normalize_version` in
   `run.sh`).
4. Builds with `GOTOOLCHAIN=local GOPROXY=off`. The module has no
   dependencies (no `go.sum`, no `replace`), so the build needs no network;
   `GOTOOLCHAIN=local` keeps a too-old toolchain from silently trying to
   download one, so the failure names the real problem. `-X main.buildVersion`
   is the variable `make build` sets from `PACK_VERSION` (`Makefile:10`).
5. `go install ./cmd/gc-runtime-rex` into `GOBIN` (or `GOPATH/bin`), then warns
   if that directory is not on PATH.

## Why GOBIN and not the pack directory

Writing the binary inside the pack would sit where gc never looks: gc resolves
the command from PATH (`internal/runtime/exec/exec.go:50-53`), not from the
pack. It would also not survive a pack refresh — `updatePack` in
`internal/config/pack_fetch.go:76-83` discards local modifications with
`git checkout --force .` and `git clean -fd` before checking out, which deletes
build products. GOBIN is outside the cache, so a later `gc pack fetch` does not
undo the fix.

The trade-off is real and is why the check reports it: the directory must be on
the PATH of the process that runs the city. A supervisor starts the city with
its own environment, so adding GOBIN to an interactive shell's PATH is not
enough; the check's remediation says so, and its success output names the path
it found the provider at.

## Failure contract

The fix is one script with three exit-1 cases, each printing the reason gc
doctor shows: no `go.mod` beside the pack, no `[pack] version` to stamp, no Go
toolchain on PATH (with the `go install ...@v<version>` fallback). `go install`
failing exits non-zero with its own toolchain error above the `fix.sh` line.
`TestRexBinaryDoctorFix` in `cmd/gc-runtime-rex/doctor_fix_test.go` executes
`fix.sh` the way gc does (the script itself, cwd at the pack, module one level
up) and pins the success path — a version-stamped binary run with `version`
reports the pack's version — plus the no-toolchain and no-`go.mod` failures.

## Scope

Not covered: building for a platform other than the fix's host, and refreshing
the binary on a pack version bump. Both are `gc doctor --fix` re-runs; a pack
update does not call the fix automatically, so the check catches the stale
binary and the remediation builds again.
