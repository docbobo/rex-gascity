---
id: GCREX-30
title: >-
  Investigate rex server run as an owned, city-scoped server for start/stop and
  attach
status: To Do
assignee: []
created_date: '2026-11-18T06:14:00Z'
updated_date: '2026-11-18T06:14:00Z'
labels:
  - research
  - server
  - provider
dependencies: []
references:
  - docs/REX-CONTRACT.md
  - docs/plans/native-rex-provider.md
  - docs/provider-herdr-parity.md
  - internal/rex
priority: medium
type: task
ordinal: 55000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Decision D3 (docs/plans/native-rex-provider.md:83, docs/REX-CONTRACT.md section 11) says the provider must never own the Rex server, because one shared server also serves the user's other work; that is why server-stop is deliberately not implemented. `rex server run` changes the picture: it runs a foreground server with --state-dir, --listen, --config, --restore and --exit-empty, saves sessions on Ctrl+C/SIGTERM/rex server stop, and can exit when no sessions remain. That opens the door to a per-city server that the provider starts as a supervised child (or that the city's own supervision starts), which would make start/stop of the server well-defined and server-stop implementable, and would isolate our panes from the user's interactive Rex work. Open questions this task must answer: can `rex server run --state-dir <city> --exit-empty` be run as a child of start/stop and be stopped safely; what --restore means for gc's reconciler (restored sessions vs dead-session cleanup, stale panes reappearing); how every `rex` invocation addresses that server (the Rex CLI ignores REX_SERVER for the app and offers -S/--server plus --autostart, so the provider must pass the address explicitly); how humans attach (Rex app or `rex -S <addr> attach`) and what that means for is-attached (per-server client registry) and for attach (GCREX-20); how multiple cities on one host, socket permissions (0600), tailnet listeners and cross-user access behave; and whether --exit-empty is a good lifecycle. Deliverable is a design note with a recommendation on revisiting D3, not an implementation.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 A design note (docs/plans or docs/research) documents the experiments run with `rex server run`, with commands and observed behaviour: foreground start, SIGTERM behaviour, session save/restore, --exit-empty, --state-dir isolation, and a second server alongside the user's existing one.
- [ ] #2 The note states how the provider and any human client would address a city-owned server (the exact flag/env and where it must be threaded through internal/rex and internal/provider).
- [ ] #3 The note covers attach: what a human runs to reach a pane on a city-owned server, and what is-attached and attach would have to do differently.
- [ ] #4 The note covers the failure modes: server crash with live panes, restored sessions after a city restart, two cities on one host, socket permissions and remote access.
- [ ] #5 A recommendation is recorded on whether to revise D3, with the affected tasks named (server-stop, watch-startup, GCREX-20 attach, GCREX-30 probes, the herdr-extension design).
- [ ] #6 Follow-up implementation tasks are filed if the recommendation is to adopt a city-owned server; none is implemented in this task.
- [ ] #7 docs/provider-herdr-parity.md `### server-stop` and `### attach` reference the note once it exists.
<!-- AC:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Relevant Rex CLI facts captured while filing: `rex server run` supports --listen, --extra-listen, --socket-mode (default 0600), --state-dir, --config, --restore (default true), --exit-empty, --max-sessions/-blocks/-windows/-panes, --tailscale* and --block-dir. Ctrl+C/SIGTERM/`rex server stop` save sessions and stop; SIGHUP saves only when the server owns a terminal. `rex server start|stop|status|logs|install` manage a persistent/detached server. The CLI child commands take -S/--server and --autostart (default true); the Rex skill documentation explicitly says not to set REX_SERVER for the app, so a city-owned server must be addressed explicitly per invocation.

GCREX-65 (2026-11-20): The earlier guarded session destroy was superseded. `Stop` leaves the canonical `gc:<city>` session in place even when empty, and the next `Start` reuses it. A city-owned server (this task) would need its own session lifecycle design, including how `--exit-empty` would work. See `docs/REX-CONTRACT.md` §11 D3 and §12.
<!-- SECTION:NOTES:END -->
