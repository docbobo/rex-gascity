---
id: GCREX-31
title: Probe the Rex CLI behaviours the parity analysis could not confirm
status: To Do
assignee: []
created_date: '2026-11-18T06:14:00Z'
labels:
  - research
  - rex
  - probes
dependencies: []
references:
  - docs/REX-CONTRACT.md
  - docs/provider-herdr-parity.md
  - internal/rex
priority: medium
type: task
ordinal: 56000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Several parity findings are blocked on Rex-side facts that no local source answers. The most important: our Rex client maps any `rex` exit code 3 to ErrSessionNotFound (internal/rex/cli/cli.go) and the driving ops treat that as success, so if Rex also uses exit 3 for a transport failure, a nudge or interrupt against a down server would silently report success. Also unconfirmed: what block.method clear does to peek output and attached clients; whether format returns scrollback or only the viewport and how it behaves on very large output; whether send-key is atomic across a multi-key list; and what block process reports as name/argv0 for a pane running `node agent.js` (needed before deciding on interpreter-script matching). Each answer either clears an 'unverified' cell in docs/provider-herdr-parity.md or produces a follow-up task.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 Each probe is recorded in docs/REX-CONTRACT.md with the exact command and the observed output: Rex exit codes for a down server and for a missing session; a transport-failure connection attempt; clear then peek; format on a pane with scrollback; format with a large capture; send-key with a good and a bad key in one call; block process on a node-script pane.
- [ ] #2 The 'Operational risks' entry about rex exit 3 in docs/provider-herdr-parity.md is resolved to a confirmed fact, and, if transport failures share exit 3, a follow-up task exists to stop mapping it to not-found.
- [ ] #3 The parity doc cells that name these probes as the settling step are updated (peek rows 4 and 6, send-keys row 7, process-alive row 2, clear-scrollback row 6), and the summary table verdicts follow.
- [ ] #4 No provider behaviour changes in this task beyond documentation and the follow-up tasks it files.
<!-- AC:END -->
