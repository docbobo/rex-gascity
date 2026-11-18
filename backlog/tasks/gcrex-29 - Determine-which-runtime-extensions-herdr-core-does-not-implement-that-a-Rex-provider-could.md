---
id: GCREX-29
title: >-
  Determine which runtime extensions herdr-core does not implement that a Rex
  provider could
status: To Do
assignee: []
created_date: '2026-11-18T06:14:00Z'
labels:
  - research
  - provider
  - extensions
dependencies: []
references:
  - docs/provider-herdr-parity.md
  - docs/REX-CONTRACT.md
  - docs/plans/native-rex-provider.md
priority: medium
type: task
ordinal: 54000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
herdr-core is one carrier with one box model; Rex exposes things it cannot: a server-side client registry, named sessions/windows/blocks with alias sets, split/new-window layout operations, paged format/blob_read capture, multi-client attach to the same pane, port forwarding, block plugins and plugin reload, and a foreground server with --state-dir, --exit-empty and session save/restore. The parity work so far only asked 'where do we differ from herdr', never 'what could a Rex carrier do that no carrier does today'. This task answers that second question systematically, judged against what gc actually consumes (gc 1.5.1 source is the arbiter), so that we do not build extensions nobody reads, and so that the genuinely Rex-only ideas become tasks or feed the herdr-extension design and the D3 server-lifecycle reconsideration.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 A research document (docs/research/rex-provider-extensions.md) lists every Rex-only capability with evidence (Rex CLI help, REX-CONTRACT, or a probe) and, for each, the gc-side consumer or an explicit 'no consumer in gc 1.5.1'.
- [ ] #2 Each candidate carries a verdict (implement, defer, reject) with effort, risk and the concrete gc behaviour it would improve.
- [ ] #3 Candidates that survive are filed as tasks or folded into the herdr-extension design task, with links in both directions.
- [ ] #4 docs/provider-herdr-parity.md links the research document from the sections it touches (attach, is-attached, peek, list-running, start).
- [ ] #5 No code changes are required by this task; if a cheap probe is needed to establish a fact, its command and output are recorded.
<!-- AC:END -->
