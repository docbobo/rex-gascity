---
id: GCREX-61
title: >-
  Doctor: verify the installed rex-gascity provider matches the version the pack
  declares
status: Done
assignee:
  - '@mc-wisp-b3qn2c'
created_date: '2026-11-19T05:56:00Z'
updated_date: '2026-11-19T09:46:00Z'
labels:
  - for-gascity
dependencies: []
references:
  - docs/provider-herdr-parity.md
  - docs/rex-provider.md
  - pack/doctor/rex-binary/run.sh
priority: medium
type: enhancement
ordinal: 117000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
A pack ships a runtime provider as an external binary: `pack.toml` declares `[runtimes.rex] command = "gc-runtime-rex"` plus `version = "0.1.0"`. gc resolves and handshakes that binary (the `pack-runtimes` doctor check and the session-exec readiness latch confirm the RPP handshake), but it never compares the binary that is actually installed against the version the pack declares. Nothing ties the pack's `version` to a version the binary reports.

That gap bites after an upgrade. The pack directory can move to a new version while the `gc-runtime-rex` on the supervisor's `PATH` stays the old one, and every failure then shows up as an opaque runtime error — a missing op, a different exit code, a changed output shape — with no message that the installed provider and the pack disagree. The provider already carries a build identity (`go version -m` reports the module pseudo-version and the VCS revision), so the two can be compared; the missing piece is a way to read that identity through the executable and a place to compare it.

The comparison belongs in the pack's existing `gc doctor` check (`rex:rex-binary`, `pack/doctor/rex-binary/run.sh`), which today only checks that the provider is invocable and answers the RPP v0 handshake. This task adds the version comparison there.

Constraints and open questions a worker must settle by reading both sources before coding:

- `docs/provider-herdr-parity.md` records the provider surface method by method; the pinned gc baseline is `v1.5.1` (tag), with the working tree possibly ahead. gc's relevant paths are `internal/runtime/exec/`, `internal/runtime/protocol.go`, and `docs/reference/exec-session-provider.md`.
- gc does not currently read the pack's `version` for comparison. Verify in the pinned gc tree where `[pack] version` is parsed (`internal/config/pack.go`, `PackMeta`) and that no comparison exists, before assuming one does.
- Decide what the provider can report (for example a `version` op or a `--version` flag on `gc-runtime-rex`, printing the module version and/or VCS revision) and how a bare `go install` binary or a `-ldflags` override is identified. Reading `debug.ReadBuildInfo` is the likely path for a Go provider; a pack that ships a non-Go binary needs a documented minimum contract.
- Choose the comparison semantics explicitly and write them down: exact match, or tolerate a build whose revision the pack cannot know. An unknown installed version must not read as a match.
- Define the pack-side declaration the check compares against (the existing `pack.toml` `version`, or a new field if `version` is not the right unit) and note that gc does not expose the pack version to the check today, so the check likely reads `pack.toml` itself, as it already does for the command.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 gc-runtime-rex can report its own version (module version and, when available, VCS revision) through a documented invocation, with a test that proves the output
- [x] #2 The rex:rex-binary doctor check compares the installed provider's reported version against the version the pack declares and exits 2 with a remediation when they disagree
- [x] #3 The check exits 0 when the provider matches the pack version, and warns (exit 1) rather than errors when the provider version cannot be determined but the binary otherwise works
- [x] #4 The comparison semantics are documented: what counts as a match, and what a local build, a +dirty tree, or an unset module version does
- [x] #5 Tests cover matching, mismatched, and undeterminable installed versions at the chosen seam, and make test passes
- [x] #6 README.md and docs/rex-provider.md document the version check and its remediation
- [x] #7 gc doctor --check rex:rex-binary --city testdata/city still passes with a matching provider installed
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. Provider version reporting: add a `version` op to cmd/gc-runtime-rex that prints one JSON line (module, version, revision, modified) from runtime/debug.ReadBuildInfo, with a buildVersion ldflags override; handled before provider construction so it needs no city/state. Document the invocation.
2. Doctor check: extend pack/doctor/rex-binary/run.sh to read the declared [pack] version from pack.toml, run `<provider> version`, normalize (strip leading v / +metadata), and compare: match -> exit 0, mismatch -> exit 2 with a remediation, undeterminable/unsupported -> warn exit 1.
3. Makefile: derive PACK_VERSION from pack/pack.toml and build/install with -ldflags "-X main.buildVersion=$(PACK_VERSION)" so a repo build matches the pack (AC7).
4. Tests (Go, make test): version op output + versionReportFrom resolution; the real run.sh with fake provider/rex + a temp pack.toml covering match, v-prefix match, mismatch, and undeterminable.
5. Docs: README.md and docs/rex-provider.md document the version check, its remediation, and the comparison semantics (local/+dirty/pseudo-version).
6. Verify: go test ./..., golangci-lint run ./..., make build then gc doctor --check rex:rex-binary --city testdata/city with bin/ on PATH (AC7).
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Implemented in work step (uncommitted; review/finish follow). Change:

(1) Provider version reporting (cmd/gc-runtime-rex/main.go): a new `version` op, handled before the provider is built (no city/state/Rex needed), prints one JSON line {"module","version","revision","modified"} from runtime/debug.ReadBuildInfo via versionReportFrom. A package var buildVersion (set with -ldflags "-X main.buildVersion=") overrides the embedded module version; the Go placeholder "(devel)" reads as empty (unknown). New tests: cmd/gc-runtime-rex/version_test.go (versionReportFrom resolution + the op's JSON output).
(2) Doctor check (pack/doctor/rex-binary/run.sh): reads the declared [pack] version from pack.toml, runs `<provider> version`, normalizes (strip a leading v and any +build-metadata), and compares. Match -> exit 0; determined mismatch -> exit 2 with a remediation; undeterminable (no version, or an older binary with no version op) -> warn exit 1. doctor.toml description updated. New test: cmd/gc-runtime-rex/doctor_check_test.go drives the real run.sh in a temp pack layout against a fake provider, covering match, v-prefix match, dirty-build match, mismatch, undeterminable, and missing-op.
(3) Makefile: PACK_VERSION is read from pack/pack.toml and build/install stamp it with -ldflags, so a repo build matches the pack.
(4) Docs: README.md and docs/rex-provider.md document the version check, its remediation and the comparison semantics (match / mismatch / undeterminable / local + dirty + pseudo-version).

Verification: go test ./... all packages ok (cmd/gc-runtime-rex 7.5s); golangci-lint run ./... 0 issues; gofmt -l cmd internal empty. AC1 red: `gc-runtime-rex version` exited 2 unknown op before the change; now bin/gc-runtime-rex version -> {"module":"github.com/docbobo/rex-gascity","version":"0.1.0","revision":"e6f743b8...","modified":true}. AC7: make build (stamps 0.1.0), then PATH=bin:$PATH gc doctor --check rex:rex-binary --city testdata/city -> 1 passed, exit 0. Doctor-check red->green: against a provider reporting 9.9.9 with pack 0.1.0, HEAD's run.sh exits 0; the new run.sh exits 2 with a remediation (pinned by TestRexBinaryDoctorCheckVersionComparison/mismatch_errors).

Caveats for the finish step: cmd/gc-runtime-rex/main.go carries an unrelated pre-existing hunk (opInterrupt -> bestEffort, another in-flight task); stage only the GCREX-61 hunks (import, const block, runWithContext version op) and exclude the opInterrupt hunk. cmd/gc-runtime-rex/dispatch_test.go is also another task's in-flight change and must not be staged. GCREX-61 files: cmd/gc-runtime-rex/{main.go,version_test.go,doctor_check_test.go}, Makefile, README.md, docs/rex-provider.md, pack/doctor/rex-binary/{run.sh,doctor.toml}.

Independent review (openrouter/deepseek/deepseek-v4.1-flash:high). Scope: GCREX-61 hunks only (cmd/gc-runtime-rex/main.go version hunks — NOT the GCREX-44 opInterrupt hunk; version_test.go; doctor_check_test.go; Makefile; README.md; docs/rex-provider.md; pack/doctor/rex-binary/{run.sh,doctor.toml}). Verdict: fail.

MAJOR
1. docs/rex-provider.md "Version match" contradicts itself about a bare local install, and the "Undeterminable" bullet is false on the pinned toolchain. It says "a bare local `go install` reports the placeholder `(devel)`" (which would warn, exit 1). On go1.27.3 (go.mod: go 1.27) a bare `go install ./cmd/gc-runtime-rex` reports a VCS pseudo-version: measured `{"module":"github.com/docbobo/rex-gascity","version":"v0.0.0-20261119090541-e6f743b8b2a0+dirty",...}`, so the determined-mismatch path applies and exits 2 — exactly what the same section's "Local and dirty builds" bullet says. Running the new run.sh against that binary with pack version 0.1.0 exits 2 "does not match the pack". The same wrong parenthetical appears in run.sh's undeterminable remediation text. Fix: `(devel)` is the no-VCS/no-module-info case; state that a bare local install is a determined mismatch (exit 2), consistent with the other bullet.

MINOR
2. docs/rex-provider.md's example JSON does not match the op's real output (verified with bin/gc-runtime-rex): module is the main module path `github.com/docbobo/rex-gascity`, not `.../cmd/gc-runtime-rex`, and `"modified":false` never appears because the field is `omitempty` (a clean build omits it). version_test.go's fixture uses the same package-suffixed module path; consider aligning it.

NITS
3. run.sh silently skips the comparison when pack.toml has no `[pack] version` (gc's validatePackMeta requires only name and schema — gc v1.5.1 internal/config/pack.go:2434), while doctor.toml now advertises version matching. A warn, or a line in the docs, would close the gap.
4. Makefile's PACK_VERSION awk matches the first `version =` line anywhere in pack.toml, unlike run.sh's `[pack]`-scoped awk. Fine for the current file; flagging the asymmetry.

VERIFICATION (all green)
- go test -count=1 ./cmd/gc-runtime-rex/ — version tests plus all 6 doctor table cases pass; make test passes all packages; golangci-lint run ./cmd/gc-runtime-rex/ 0 issues; gofmt clean.
- make -n build stamps `-X main.buildVersion=0.1.0` from pack/pack.toml; bin/gc-runtime-rex version prints the stamped JSON, exit 0.
- AC7: PATH=bin:$PATH gc doctor --check rex:rex-binary --city testdata/city -> passes, exit 0.
- Red premise confirmed: HEAD's run.sh exits 0 against a provider reporting 9.9.9 with pack 0.1.0; the new script exits 2 (TestRexBinaryDoctorCheckVersionComparison/mismatch_errors is red without the change).
- gc claims checked against pinned v1.5.1: `[pack] version` is parsed (internal/config/config.go:848-853 PackMeta, pack.go:42) and never exposed to pack checks — PackScriptCheck passes cmd.Environ() plus PackRuntimeEnv (city/runtime vars and GC_PACK_STATE_DIR only, internal/citylayout/runtime.go:216-222) plus GC_PACK_DIR (internal/doctor/pack_checks.go). Exit mapping 0/1/2 confirmed there. The parity doc does not cover the doctor check.

Staging caveat from the implementation notes holds: main.go's opInterrupt hunk and cmd/gc-runtime-rex/dispatch_test.go are GCREX-44 work; stage only the version hunks/files above.

Addressed review findings (review verdict: fail).

MAJOR 1 (docs/rex-provider.md "Version match" + run.sh undeterminable remediation): corrected the false claim that a bare local `go install` reports "(devel)". Verified on go1.27.3: a bare `go build`/`go install` from this Git checkout reports a module pseudo-version (`v0.0.0-20261119090541-e6f743b8b2a0+dirty`), so it takes the determined-mismatch path (exit 2), not the warn path. Docs now state "(devel)" is the no-VCS/no-module-info placeholder and is consistent with the existing "Local and dirty builds" bullet; run.sh's remediation text matches. Behavior unchanged (the mismatch path was already correct).

MINOR 2 (docs/rex-provider.md example JSON + version_test.go fixture): example now uses the real main-module path `github.com/docbobo/rex-gascity` and omits the never-printed `"modified":false` (the field is omitempty; a clean build omits it), noting the omission; version_test.go's fixture/assertion aligned to the same module path.

NIT 3: documented that a pack.toml with no `[pack] version` skips the comparison (new "No declared version" bullet).
NIT 4: Makefile PACK_VERSION awk now scopes to `[pack]` like run.sh, instead of matching the first `version =` anywhere.

Verification: go test ./... all packages ok (cmd/gc-runtime-rex 7.2s); golangci-lint run ./... 0 issues; gofmt -l cmd/gc-runtime-rex empty. AC7: make build (stamps 0.1.0) then PATH=bin:$PATH gc doctor --check rex:rex-binary --city testdata/city -> 1 passed, exit 0, with bin/gc-runtime-rex reporting version 0.1.0.

Independent review (openrouter/deepseek/deepseek-v4.1-flash:high), iteration 2. Scope: the GCREX-61 hunks only — cmd/gc-runtime-rex/main.go (import, buildVersion/versionReport/versionReportFrom, the `version` op), version_test.go, doctor_check_test.go, Makefile, README.md, docs/rex-provider.md, pack/doctor/rex-binary/{run.sh,doctor.toml}. The opInterrupt bestEffort hunk in main.go and cmd/gc-runtime-rex/dispatch_test.go are GCREX-44 work and were excluded, per the worker's staging caveat.

Verdict: pass. No blocking or major issue remains; the iteration-1 findings are resolved. Verified: a bare local build reports the module pseudo-version (measured: unstamped build -> v0.0.0-20261119090541-e6f743b8b2a0+dirty -> determined mismatch, exit 2), so the docs claim is now right; the docs example uses the main module path and no longer prints "modified":false; the "No declared version" skip is documented; the Makefile awk is [pack]-scoped like run.sh. Remaining minor/nit follow-ups:

MINOR 1 (docs/rex-provider.md "Undeterminable" bullet + run.sh undeterminable remediation): "(devel)" is still attributed to the no-VCS/no-module-info case, but on the pinned toolchain (go1.27.3; go.mod says go 1.27) that case reports an empty version directly, not the placeholder. Measured: `go build -buildvcs=false` -> {"module":"github.com/docbobo/rex-gascity","version":""}; a build from a non-git copy of the source -> the same empty version. The provider's "(devel)" -> empty normalization stays useful defensively (older toolchains, test binaries) and the exit path is unchanged (empty -> warn, exit 1), so this is text accuracy only.

NIT 2: docs/rex-provider.md's example JSON shortens the revision ("e6f743b8b2a0") where the op prints the full vcs.revision (measured e6f743b8b2a0af3d7d8dcf757b692ee147bab41a); label the example illustrative or use the full value.

NIT 3: the Makefile's `LDFLAGS ?=` lets an environment/packaging LDFLAGS replace `-X main.buildVersion=...` instead of appending it; such a build silently reports its pseudo-version and the doctor check then errors. A dedicated variable that appends the stamp would avoid the footgun.

NIT 4: doctor_check_test.go has no case for the documented "no declared version skips the comparison" branch.

VERIFICATION (all green)
- go test -count=1 ./... passes every package; make test passes; golangci-lint run ./... 0 issues; gofmt -l cmd internal empty.
- Focused tests pass: TestVersionReportFrom (4 subtests), TestVersionOp, TestRexBinaryDoctorCheckVersionComparison (match / v-prefix / dirty / mismatch / undeterminable / missing-op).
- Red premise: HEAD's run.sh against a provider reporting 9.9.9 with pack version 0.1.0 exits 0 (verified by running `git show HEAD:pack/doctor/rex-binary/run.sh` in a temp pack layout); the new script exits 2, pinned by TestRexBinaryDoctorCheckVersionComparison/mismatch_errors.
- End-to-end against the real pack: stamped build -> exit 0; unstamped -> exit 2 with a remediation; -buildvcs=false -> exit 1 warn; the first stdout line is the message in each case.
- AC7: make build (stamps 0.1.0) then PATH=bin:$PATH gc doctor --check rex:rex-binary --city testdata/city -> "1 passed", exit 0.
- gc-side claims re-checked against the pinned tree /Volumes/Sources/gascity, tag v1.5.1 (6924c2cfeb27cb6f3f3667533f8d0ce46cc3e849): PackMeta.Version is parsed (internal/config/config.go:849-853) and nothing compares it against an installed provider binary (internal/runtime/exec only reads the protocol handshake version); PackScriptCheck runs with cmd.Environ() + PackRuntimeEnv (city vars + GC_PACK_STATE_DIR, internal/citylayout/runtime.go:216) + GC_PACK_DIR only (internal/doctor/pack_checks.go:100-103), so pack.toml is the sole declared-version source; the exit contract is 0=OK / 1=Warning / default=Error (pack_checks.go:126-131). docs/provider-herdr-parity.md does not cover the doctor check, so gc source is the authority here.
- AC1-AC7 are met on this evidence; the task-finalization step owns checking the boxes. Runtime trace: the AC7 gc doctor run created the gitignored testdata/city/.gc/ state dir; nothing tracked or staged changed.
- Staging caveat unchanged: stage only the GCREX-61 files; main.go's opInterrupt hunk and dispatch_test.go belong to GCREX-44.

Finalization verification (2026-11-19): make test -> all packages ok; go test -count=1 ./cmd/gc-runtime-rex/ -> ok 7.6s; golangci-lint run ./... -> 0 issues. AC7: make build stamps -X main.buildVersion=0.1.0; bin/gc-runtime-rex version -> {"module":"github.com/docbobo/rex-gascity","version":"0.1.0",...}; PATH=bin:$PATH gc doctor --check rex:rex-binary --city testdata/city -> 1 passed, exit 0.
<!-- SECTION:NOTES:END -->

## Comments

<!-- COMMENTS:BEGIN -->
author: @mc-wisp-b3qn2c
created: 2026-11-19T09:15:00Z

---

Claimed by Gas City worker mc-wisp-b3qn2c (seat rex-gascity/pi, session pi-mc-wisp-b3qn2c, template rex-gascity/pi); workflow bead mc-wgn5, step bead mc-jska.

---
<!-- COMMENTS:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
The rex:rex-binary doctor check now compares the installed provider's reported build version against the version pack.toml declares, closing the pack-upgrade drift that previously surfaced only as an opaque runtime error. gc-runtime-rex gained a local, city-free `version` op printing one JSON line (module/version/revision/modified) from runtime/debug.ReadBuildInfo, overridable with -ldflags; run.sh reads [pack] version, normalizes (strips leading v and +metadata), and exits 0 on match, 2 with a remediation on a determined mismatch, 1 when undeterminable (binary still handshakes); the Makefile stamps the pack version so repo builds match. Verified: make test and golangci-lint clean; AC7 gc doctor check passes end to end; the version comparison test is red against HEAD's run.sh and green now. Risks/follow-ups: a local `go install` from an untagged tree reports a pseudo-version and errors (documented); remaining review nits (docs wording on the no-VCS case, LDFLAGS override footgun, missing no-declared-version test case) are non-blocking.
<!-- SECTION:FINAL_SUMMARY:END -->
