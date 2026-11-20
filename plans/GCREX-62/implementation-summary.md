---
schema: gc.build.implementation-summary.v1
workflow: {id: gcrex-sh6ol, formula: do-work}
methodology: {pack: gascity, name: build-basic}
producer: {formula: do-work, stage: implement, attempt: 1}
status: approved
trace:
  upstream:
    - path: beads/gcrex-v6a4
      hash: bead:gcrex-v6a4
      ids: [REQ-001, REQ-002, REQ-003, REQ-004, REQ-005, REQ-006]
  coverage:
    - {id: REQ-001, status: covered}
    - {id: REQ-002, status: covered}
    - {id: REQ-003, status: covered}
    - {id: REQ-004, status: covered}
    - {id: REQ-005, status: covered}
    - {id: REQ-006, status: covered}
---

# Implementation Summary: GCREX-62 — compile the provider when gc installs the pack

## Summary

The rex pack now installs its own runtime provider. `pack/doctor/rex-binary/fix.sh`
is the remediation behind the existing `rex:rex-binary` doctor check: gc runs it
with `gc doctor --fix`, builds `./cmd/gc-runtime-rex` from the module root one
level above the pack, stamps `main.buildVersion` with the `[pack] version` from
`pack.toml`, and installs the binary into `GOBIN` (or `GOPATH/bin`). The design
note `docs/rex-binary-install.md` records the seam choice, where the binary
lands, the rejected alternatives, and the gc citations. The proving test is
`TestRexBinaryDoctorFix` in `cmd/gc-runtime-rex/doctor_fix_test.go`.

gc has no pack build/install hook at the pinned `v1.5.1`: the pack spec defines
only doctor checks (§ 1.2.11) and commands (§ 1.3.4), `internal/packman/install.go`
resolves/clones/locks without building (no build function in the file), and a
declared runtime's bare `command` is resolved through `exec.LookPath`
(`internal/runtime/exec/exec.go:50-53`). A pack `fix.sh` is the only
remediation surface gc offers (`internal/doctor/pack_checks.go:67-91`,
`PackScriptCheck.Fix`). The spec promises a `pack-runtimes` doctor check
(§ 1.2.8, line 518) that does not exist in `v1.5.1`'s `internal/doctor/`; the
note reports that conflict rather than relying on it.

Rejected: a `warmup = true` check that builds (warmup still runs on demand, not
before the runtime is resolved, and hides a heavy build inside `gc start`); a
pack command (`commands/...` needs the user to remember a second command — the
same manual act); a `pre_start`/`session_setup` hook (does not exist for a
runtime declaration and would run per session, not per install); a prebuilt
binary in the pack (the pack cache is refreshed with `git clean -fd`,
`internal/config/pack_fetch.go:76-83`, so a committed binary would be a
large, per-platform artifact anyway); writing into the pack cache (deleted on
refresh, and not where gc looks).

## Intended Behavior

- `gc doctor --fix` on a city using the rex pack builds and installs
  `gc-runtime-rex`, stamped with `pack.toml`'s `[pack] version`; the
  `rex:rex-binary` check then passes because the version comparison matches
  (`normalize_version` strips a leading `v` and `+build-metadata`).
- The build works with the pack composed from a local include and from a remote
  import, because the module root is always the directory above the pack.
- The build needs no network: the module is stdlib-only (no `go.sum`, no
  `replace`), so `GOPROXY=off` holds; `GOTOOLCHAIN=local` makes a too-old
  toolchain fail with the real error instead of downloading one.
- The binary survives `gc pack fetch` / `gc import install`: it lands in
  `GOBIN`/`GOPATH/bin`, outside the cache directory that `updatePack` cleans.
  The runtime command stays the bare PATH name `gc-runtime-rex`, so the
  directory must be on the city process's PATH; the check reports the resolved
  path and the fix warns when the directory is not on PATH.
- Missing Go, missing `go.mod` beside the pack, or a missing `[pack] version`
  each exit 1 with the reason on stderr, which gc doctor prints as the fix
  error.
- `pack/doctor/rex-binary/run.sh`'s remediation text now says "builds the
  pack's module" (was "pack's commit") and suggests `go install ...@v<version>`
  (was `@latest`), matching what the fix does and the version the check wants.

Coverage:

| ID | Status |
| --- | --- |
| REQ-001 | covered |
| REQ-002 | covered |
| REQ-003 | covered |
| REQ-004 | covered |
| REQ-005 | covered |
| REQ-006 | covered |

REQ-001 = AC1 (design note with mechanism, destination, rejected alternatives);
REQ-002 = AC2 (no build/install hook, cited against `internal/packman/install.go`
and `docs/reference/specs/pack-spec.md` at `v1.5.1`, with the `pack-runtimes`
conflict reported); REQ-003 = AC3 (module root above `pack/`, same for local
include and remote import, and the test proves the fix works with cwd at the
pack); REQ-004 = AC4 (toolchain requirement, failure behavior, network-free
build from the stdlib-only module); REQ-005 = AC5 (survives cache refresh via
`GOBIN`; command stays a bare PATH name, with the PATH requirement stated);
REQ-006 = AC6 (implemented, not deferred: `TestRexBinaryDoctorFix` proves it,
`make test` passes).

## Changed Files

- `pack/doctor/rex-binary/fix.sh` — new. The `gc doctor --fix` remediation:
  module-root check, version stamp from `pack.toml`, `GOTOOLCHAIN=local
  GOPROXY=off go install -trimpath -ldflags "-X main.buildVersion=<version>"
  ./cmd/gc-runtime-rex`, PATH warning.
- `pack/doctor/rex-binary/run.sh` — remediation text aligned with the fix
  ("pack's module", `@v<declared version>`).
- `cmd/gc-runtime-rex/doctor_fix_test.go` — new. `TestRexBinaryDoctorFix`
  executes `fix.sh` as gc does (the script itself, cwd at the pack, module one
  level up): installs a version-stamped binary and asserts
  `{ "version": "0.1.0" }`; no Go on PATH fails naming the toolchain; no
  `go.mod` fails naming `go.mod`.
- `docs/rex-binary-install.md` — new design note: the gap, the chosen seam,
  `GOBIN` and why not the pack cache, the failure contract, scope.
- `plans/GCREX-62/implementation-summary.md` — this summary.

## Verification

- First verification command (red, then green):
  `go test ./cmd/gc-runtime-rex -run TestRexBinaryDoctorFix -v`
  — first run before `fix.sh` existed: FAIL
  (`read fix script: open ../../pack/doctor/rex-binary/fix.sh: no such file or
  directory`); after adding `fix.sh`: PASS, all three subtests.
- End-to-end manual run from the worktree:
  `GOBIN=/tmp/rexbin sh pack/doctor/rex-binary/fix.sh` — exit 0, then
  `/tmp/rexbin/gc-runtime-rex version` printed
  `{"module":"github.com/docbobo/rex-gascity","version":"0.1.0",...}`; the
  `rex:rex-binary` check run with `/tmp/rexbin` on PATH printed
  `provider: /tmp/rexbin/gc-runtime-rex (version 0.1.0)` and exit 0.
- Final proof command: `make test` — PASS (`cmd/gc-runtime-rex`,
  `internal/launch`, `internal/provider`, `internal/rex/cli` all ok).
- `gofmt -l cmd/gc-runtime-rex` (clean), `go vet ./cmd/gc-runtime-rex` (clean),
  `golangci-lint run ./...` — PASS (0 issues).

## Remaining Risks

- The fix builds the worktree's current tree, not the locked commit. In the
  pack cache those coincide (the cache is the checkout), but a local include
  that points at a dirty working tree builds the dirty tree — the check
  surfaces the revision and marks a modified tree rather than silently
  accepting it.
- A pack version bump does not trigger the fix; the stale binary fails the
  check and `gc doctor --fix` builds again. No automatic refresh.
- The binary is host-platform only; a city shared across platforms needs one
  install per platform.
- `GOBIN`/`GOPATH/bin` must be on the PATH of the process that runs the city
  (supervisor or controller), not just the operator's shell. The check reports
  the resolved path, and the fix warns, but neither edits the supervisor's
  environment.
- The spec's `pack-runtimes` doctor check is documented (§ 1.2.8) but absent in
  `v1.5.1`; if a later gc adds it with a build remediation, it would supersede
  this pack fix. Recorded as a conflict in `docs/rex-binary-install.md`.
