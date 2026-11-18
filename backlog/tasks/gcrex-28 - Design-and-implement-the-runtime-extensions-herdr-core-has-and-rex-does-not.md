---
id: GCREX-28
title: Design and implement the runtime extensions herdr-core has and rex does not
status: To Do
assignee: []
created_date: '2026-11-18T06:14:00Z'
updated_date: '2026-11-18T06:14:00Z'
labels:
  - provider
  - extensions
  - herdr
dependencies: []
references:
  - docs/provider-herdr-parity.md
  - internal/provider
  - cmd/gc-runtime-rex
  - https://github.com/gastownhall/gascity-packs/tree/main/runtime-herdr-core
priority: medium
type: task
ordinal: 53000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
runtime-herdr-core implements a set of ops and behaviours beyond RPP v0 that our provider lacks: observe (JSON state answer, 4 s probe deadline, observe.v1), inventory ({complete, sessions[]}, carrier.ops), kill-object (stdin object, corpse/zombie modes), copy-to (stage a file or tree into the work dir), server-stop (per-city core teardown, server-lifecycle), a streaming watch-startup (NDJSON screen lines every 250 ms) and a tty attach path. Their handshake also declares city-scoped, conformance-faults, idle-boundary, input-outcome and fs.host. Our parity analysis (docs/provider-herdr-parity.md) shows gc 1.5.1 ignores unknown capability tokens and never calls observe, inventory, kill-object, server-stop or copy-to, while it does call attach (which fails today, see GCREX-20). This task turns that analysis into decisions and implementations: which extensions are worth having for Rex, in what order, each gated behind a declared capability so gc ignores it until it is real, with no regression to the baseline handshake. Whether a city-owned Rex server makes server-stop and watch-startup worthwhile is investigated in the `rex server run` task, so sequence accordingly.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 docs/provider-herdr-parity.md (or a new docs/herdr-extensions.md that it links) records, for every herdr-only extension, a verdict of implement, defer or reject with the reason and the gc-side trigger that would make it matter.
- [ ] #2 Each accepted extension has a design section: op contract, capability token, deadline class, stdin, stdout shape, exit codes, state and cleanup.
- [ ] #3 Each accepted extension is implemented with unit tests and declares its capability only in the same change that implements it.
- [ ] #4 The baseline handshake for a gc 1.5.1 city is unchanged unless the corresponding op is implemented and tested against `gc runtime check`.
- [ ] #5 go test ./... and golangci-lint run ./... pass.
- [ ] #6 docs/provider-herdr-parity.md is updated: the sections for implemented ops drop their not-implemented verdicts and the coverage and summary tables are recomputed.
<!-- AC:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Sequencing: the server-stop and streaming watch-startup parts depend on the outcome of GCREX-30 (rex server run as an owned, city-scoped server). Start with the extensions that need no server change: observe, inventory, kill-object, copy-to, and the NDJSON watch-startup shape. GCREX-31 supplies the Rex-side facts (clear/format semantics, exit codes) that shape copy-to and watch-startup.
<!-- SECTION:NOTES:END -->
