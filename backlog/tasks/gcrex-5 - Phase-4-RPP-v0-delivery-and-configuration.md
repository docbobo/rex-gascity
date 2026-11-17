---
id: GCREX-5
title: 'Phase 4: RPP v0 delivery and configuration'
status: To Do
assignee: []
created_date: '2026-11-17T16:25:00Z'
labels: []
dependencies:
  - GCREX-4
references:
  - docs/plans/native-rex-provider.md
  - docs/REX-CONTRACT.md
ordinal: 5000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
The provider must ship as an out-of-tree executable so Rex support reaches users without a gc release. This phase adds the RPP v0 op dispatcher and protocol handshake, the pack declaration that registers the runtime name rex, and the docs that tell a user how to select it. Gas City validates pack runtimes with gc runtime check and conformance runs.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 gc-runtime-rex answers the protocol handshake with its capability set and locality
- [ ] #2 Every RPP v0 op used by the provider is dispatched and returns protocol-correct exit codes
- [ ] #3 A pack declares [runtimes.rex] and the runtime resolves by name
- [ ] #4 gc runtime check rex and gc runtime conformance rex pass
- [ ] #5 Docs explain selecting the provider with [session] provider = "rex" or GC_SESSION=rex
<!-- AC:END -->
