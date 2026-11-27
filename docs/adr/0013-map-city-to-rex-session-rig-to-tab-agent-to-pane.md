# 0013. Map city to Rex session, rig to tab, agent to pane

- Status: Accepted
- Date: 2026-11-17 (backfilled from plan decision D1)

## Context

gc groups work as city, then rig, then agent. Rex offers session, then tab
(window), then pane (block). A mapping between them decides how a user sees a
city in the Rex sidebar, and how the provider finds an agent's pane again.
Rex labels are cosmetic, and a pane's block id cannot be derived from the agent
name, so a sidecar binding is the reverse map.

## Decision

One Rex session per city, labelled `gc:<city>`. One tab per rig. One pane per
agent, tiled in its rig's tab. `internal/provider/sessionname.go` owns the
`<rig>--<agent>` grammar and the labels, and the file sidecar maps the name to a
tab and a block.

## Consequences

- The Rex sidebar shows one entry per city, and rigs read as tabs.
- Agents of a rig share a tab, so they cannot be switched one by one.
- `is-attached` must resolve to the active tab and its panes, not to the
  session (`docs/rex-provider.md`, Identity Mapping).
- Starts for one rig serialize, because two agents can race to create the rig
  tab.

## Alternatives rejected

- **Session per rig, tab per agent.** Each agent gets its own switchable tab,
  but a city spreads across many sessions in the sidebar.
- **Session per agent.** This is the least code, since the session name is the
  gc name. But it loses all grouping, and a city of twenty agents is twenty
  sidebar entries.
