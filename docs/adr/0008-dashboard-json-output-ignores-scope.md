# 0008. dashboard JSON output ignores --scope

- Status: Accepted
- Date: 2026-11-26

## Context

`gc rex dashboard` has two outputs. The terminal view renders workstreams and
applies `--scope`. The JSON path serializes every active stream into an
origin-to-bead-ids map (`pack/commands/dashboard/dashboard.py:1311-1324`).

`--scope` selects terminal streams in `Renderer.in_scope`
(`pack/commands/dashboard/dashboard.py:505-511`). It never reaches the JSON
path. `active_statuses_for(..., respect_scope=False)` keeps the JSON status set
the same for every scope (`pack/commands/dashboard/dashboard.py:1087-1108`).

The asymmetry is deliberate. JSON is a data model for scripts
(`pack/commands/dashboard/help.md:42-43`). A filtered payload would make
`--json --scope working` silently hide active workstreams from a consumer. The
help text did not state the rule, so a reader could take `--scope` as
universal. CodeRabbit raised that ambiguity on docbobo/rex-gascity#174
(comment `cr-comment:v1:786d5a6a9940ce6a47c3bdcc`).

## Decision

We keep `--json` unfiltered. `--scope` affects the terminal view only.

- The `--scope` help says "`--json ignores --scope`"
  (`pack/commands/dashboard/dashboard.py:1241-1250`).
- The `--json` help says "`--scope affects the terminal view only`"
  (`pack/commands/dashboard/dashboard.py:1292-1298`).
- `help.md` repeats both statements (`pack/commands/dashboard/help.md:28-32`,
  `pack/commands/dashboard/help.md:42-43`).
- `pack/tests/check-dashboard.sh:654-663` compares the JSON payload for
  `--scope working`, `--scope attention`, and `--scope all`, and needs
  identical bytes.

`--all` is a separate flag. It widens the status set to include closed beads,
including for `--json` (`pack/commands/dashboard/dashboard.py:1088-1098`).
That is not a scope effect, and the help says so.

## Consequences

- A caller can pass one argument list to the terminal and JSON outputs and get
  the same JSON contract.
- `--json --scope working` returns every active workstream, not only working
  ones. A consumer that wants a subset filters the payload itself.
- A change that applies `in_scope` to the JSON path fails the comparison test.

## Alternatives rejected

- Filter the JSON payload with `Renderer.in_scope`. The payload would then
  depend on a presentation flag. `--json --scope working` would drop
  `attention` streams from the data model, so scripts would need `--scope all`
  to see the graph. The same query would return different JSON for different
  viewers.
- Reject `--json` with a non-default `--scope` as a usage error. It breaks
  callers that pass one shared argument list to both outputs. It adds
  validation for a flag that is already harmless.
- Document nothing and let the code speak. A reviewer already proposed the
  ambiguity once. A stated contract plus a comparison test stops the next
  review from raising it again.
