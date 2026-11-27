# 0014. The provider never owns or stops the shared Rex server

- Status: Accepted
- Date: 2026-11-17 (backfilled from plan decisions D3 and D4)

## Context

A user runs one Rex server, and their own work runs on it beside the city.
Rex has `server.stop`. RPP has a `server-stop` op. The server's version also
moves only on restart, so stopping it changes what every client talks to
(`docs/REX-CONTRACT.md` §0).

## Decision

We never destroy what we did not create. That covers the shared server, other
sessions, legacy-labelled sessions and human panes. We do destroy what we
created: our panes, with `block.close`. We also destroy the canonical
`gc:<city>` session once its last pane is gone, behind the fail-closed empty
check (`destroyEmptyCitySession`, `REX-CONTRACT.md` §12, GCREX-31).

`server-stop` is not registered. The provider talks to a local server, either
the default socket or `-S <host>` for a server that shares the filesystem, and
agents run on gc's host.

`GC_REX_SERVER` (`-S`) can point a city at a server the operator runs for it.
The default stays the shared server.

## Consequences

- Teardown is block-only, and Rex removes a tab when its last block closes.
- A Rex upgrade that needs a server restart is the operator's job, never the
  provider's.
- A city-owned server with a provider-managed lifecycle is an open proposal
  (GCREX-30). It would amend this ADR and needs an ownership marker before the
  provider may stop anything.

## Alternatives rejected

- **The provider owns a per-city server and stops it on teardown.** On the
  shared default this kills the user's own work. As an opt-in, `rex server run`
  makes it possible, but only behind an ownership record. That is GCREX-30's
  proposal, and it is not the default.
- **Implement `server-stop` and trust gc to call it only when safe.** gc cannot
  tell a shared server from a city one. The provider is the only place that
  knows.
- **Remote boxes, where agents run on the Rex host.** RPP v0 has no declaration
  for where agents run. Hooks such as `pre_start` and `session_setup` assume
  gc's host.
