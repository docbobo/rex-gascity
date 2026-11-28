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
  `[<agent>] (#<bead id>) #<session id>`.

The tag names the kind. An unmarked parenthesis is the complete session name
and is compared by equality. A parenthesis starting with `#` is the pool
identity: it is verified as `Sanitize(agent)+"-"+bead id`, the one name-shape
check the pane keeps. The marker is required: without it `[worker] (seat)` is
byte-for-byte both the town form for session `seat` and the pool form for
`worker-seat`, so one label would name two sessions. The trailing `#` tail is
display-only and is dropped first when a label must be clamped. A pane with no
agent identity keeps today's bare label (`mayor`).
The pre-tag labels (`gc:<city>`, `Rig: <rig>`, the bare pin, the untagged pane
form) stay accepted on read, so a session or window created by an older build
is found rather than duplicated.

## Consequences

- Discovery has one rule per level: an unmarked parenthesis names the session
  whose complete name it equals, and a marked parenthesis names the pool seat
  its identity verifies. An unmarked suffix is never re-derived as a pool
  name, so one label cannot name two sessions. A complete name that itself
  starts with `#` is written with one escape, `(\#seat)`, and read back by
  stripping one; an escaped suffix is never a pool identity.
- An upgrade joins the existing tab (`Rig: <rig>`, bare pin) and the existing
  session label instead of creating a second one; a split into a legacy tab
  records the legacy label it actually carries.
- Clamping is deterministic per form. The pool form drops the display-only
  tail and never cuts the agent; if the marked identity cannot fit, the long
  form takes over. The long and town forms keep the identity and cut the agent
  first with the tail and then without it (marked `…`), and only then fall back
  to the bare session name. A clamped label still names its session because the
  parenthesis is never cut.
- Every label is longer by the tag (`[rig] `, `[agent] `), which costs about
  8 runes of the 128-rune budget; the `#` tail keeps labels distinct only up to
  the limit.
- A pane label is still display-only: nothing resolves by the agent text.

## Alternatives rejected

- Keep the shapes and add a per-level prefix on read only: a reader still
  cannot tell a pinned city tab from a rig tab, and the `#` verification stays.
- Keep the unmarked pool identity `[<agent>] (<agent>-<bead id>)`: it is
  byte-identical to the town form `[<agent>] (<name>)` for a session literally
  named the bead id, so one label names both that session and the pool seat.
  Marking the pool identity is what keeps one label to one session.
- Put the tag after the identity (`<city> [city]`): the name is what an
  operator scans, and Rex sorts and truncates from the left.
- Keep `#<bead id>` load-bearing as the pool identity: it is display-only data
  derived from the parenthesis, and comparing it would re-introduce the
  name-shape check this change removes.
- Drop the legacy rungs and relabel every live window: the provider does not
  own those windows, a relabel would be an unrelated action against a running
  session, and an upgrade would start a duplicate tab.
