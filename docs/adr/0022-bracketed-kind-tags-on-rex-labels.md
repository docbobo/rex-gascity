# 0022. Bracketed kind tags on Rex labels

- Status: Accepted
- Date: 2026-11-28

## Context

A Rex label is the only thing the operator sees in the Rex sidebar, and the only
thing the provider can read back from a window with no sidecar record. Before
this change the three levels used three unrelated shapes:

- session `gc:<city>` (`internal/provider/sessionname.go`, `rexSessionLabel`);
- tab `Rig: <rig>` (`internal/provider/launch_identity.go`, the pre-tag tab
  label);
- pane `<agent> (<suffix>)`, where the suffix was either the whole session name
  or a `#<bead id>` that discovery had to verify by reconstructing the pool
  name (`internal/provider/label.go`, `newPaneParts` / `paneLabelFor`).

A reader cannot tell `Indigo` from a city pin, `gc:<city>` from a session name,
or `#mc-wisp-kvqib` from a session the provider must reconstruct. Discovery has
to special-case each level: the pane is the only level that carries an identity
inside parentheses, and the `#` suffix is the only level whose meaning depends
on a name-shape check (`identityMatch`).

## Decision

We write a bracketed kind tag on every generated label:

- session `[city] <city>`, tab `[rig] <rig>` / `[city] <pin>`,
  pane `[<agent>] (<identity>)`, with the pool form
  `[<agent>] (<agent>-<bead id>) #<bead id>`.

The tag names the kind; the identity after it is compared, never re-derived.
One parenthesis rule serves all three levels: the contents of `(...)` are the
identity. The `#` tail is display-only and is dropped first when a label must be
clamped. A pane with no agent identity keeps today's bare label (`mayor`).
The pre-tag labels (`gc:<city>`, `Rig: <rig>`, the bare pin, the untagged pane
form) stay accepted on read, so a session or window created by an older build
is found rather than duplicated.

## Consequences

- Discovery becomes one rule per level with no name-shape reconstruction for
  the pool form: a tagged parenthesis names the session directly, and an
  untagged suffix names nobody.
- An upgrade joins the existing tab (`Rig: <rig>`, bare pin) and the existing
  session label instead of creating a second one; a split into a legacy tab
  records the legacy label it actually carries.
- Clamping is deterministic: the display-only tail goes first, then the agent
  (marked `…`), and only then the whole suffix. A clamped label still names its
  session because the parenthesis is never cut.
- Every label is longer by the tag (`[rig] `, `[agent] `), which costs about
  8 runes of the 128-rune budget; the `#` tail keeps labels distinct only up to
  the limit.
- A pane label is still display-only: nothing resolves by the agent text.

## Alternatives rejected

- Keep the shapes and add a per-level prefix on read only: a reader still
  cannot tell a pinned city tab from a rig tab, and the `#` verification stays.
- Put the tag after the identity (`<city> [city]`): the name is what an
  operator scans, and Rex sorts and truncates from the left.
- Keep `#<bead id>` load-bearing as the pool identity: it is display-only data
  derived from the parenthesis, and comparing it would re-introduce the
  name-shape check this change removes.
- Drop the legacy rungs and relabel every live window: the provider does not
  own those windows, a relabel would be an unrelated action against a running
  session, and an upgrade would start a duplicate tab.
