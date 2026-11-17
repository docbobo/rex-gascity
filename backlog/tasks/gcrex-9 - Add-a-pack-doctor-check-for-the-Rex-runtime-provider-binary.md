---
id: GCREX-9
title: Add a pack doctor check for the Rex runtime provider binary
status: Done
assignee: []
created_date: '2026-11-17T21:03:00Z'
updated_date: '2026-11-17T21:43:00Z'
labels: []
dependencies: []
ordinal: 9000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
A city configured with `[session] provider = "rex"` fails silently when `gc-runtime-rex` is not resolvable. The supervisor logs `exec: "gc-runtime-rex": executable file not found in $PATH` on every reconciler tick, but `gc status` only shows agents as `unknown (partial status)` and no `gc:<city>` Rex session ever appears, so the cause is not obvious. Other runtime packs ship a `doctor/` directory so `gc doctor` surfaces missing prerequisites; this pack has none. Add a check so the missing provider binary is reported with a remediation instead of surfacing as opaque agent state.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 The pack registers a doctor check named `rex:<check>` that gc reports in `gc doctor` output
- [x] #2 The check exits 2 and prints the missing binary plus a remediation when the runtime command cannot be resolved
- [x] #3 The check exits 0 when the runtime command resolves
- [x] #4 The check also covers the `rex` CLI and honors `GC_REX_BIN`
- [x] #5 The check resolves the runtime command from the pack `pack.toml`, handling both a bare name from PATH and a path relative to the pack directory
- [x] #6 The check follows the pack doctor convention: executable `run.sh`, exit 0/1/2, first stdout line is the message and later lines are details
- [x] #7 `gc doctor` against `testdata/city` runs the check and it passes with the provider installed
- [x] #8 README and the provider reference document the new check
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
Add pack/doctor/rex-binary/{doctor.toml,run.sh}. The check resolves [runtimes.rex].command from pack.toml (bare name via PATH or path relative to the pack dir), probes the provider's protocol op for an RPP v0 handshake, and resolves the rex CLI honoring GC_REX_BIN. It follows the pack doctor protocol (exit 0/1/2, first stdout line is the message, later lines details) and stays read-only. Document it in README.md and docs/rex-provider.md, and verify with gc doctor --check rex:rex-binary --city testdata/city.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Implemented pack/doctor/rex-binary/doctor.toml and run.sh (executable). Registered as rex:rex-binary in gc doctor. Verified: full sweep lists rex:rex-binary; --check rex:rex-binary against testdata/city passes (exit 0) with the provider installed; missing provider on PATH exits 2 and names the binary plus remediation; GC_REX_BIN=nope exits 2 and names the rex CLI; GC_REX_BIN override resolves. README and docs/rex-provider.md document the check and the supervisor PATH caveat.
<!-- SECTION:NOTES:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
Added the pack's first doctor check: pack/doctor/rex-binary/{doctor.toml,run.sh}. gc registers it as rex:rex-binary. The check resolves [runtimes.rex].command from pack/pack.toml (bare name via PATH or path relative to the pack directory), probes the provider's protocol op for an RPP v0 handshake, and resolves the rex CLI honoring GC_REX_BIN. It follows the pack doctor protocol (exit 0/1/2, first stdout line is the message, later lines details) and is read-only. Verified: gc doctor --check rex:rex-binary --city testdata/city passes (exit 0); a provider missing from PATH exits 2 and names the binary plus remediation; GC_REX_BIN=nope exits 2 and names the rex CLI; GC_REX_BIN override resolves. go vet ./... and go test ./... pass. README.md and docs/rex-provider.md document the check and the supervisor PATH caveat.
<!-- SECTION:FINAL_SUMMARY:END -->
