Visualize the workstreams a Gas City rig is working on and the bead graph
under each one.

A *workstream* is the top-level "origin" bead a unit of work was launched for
(an issue build, a CodeRabbit merge, a hand-filed bug, ...). Beads created by a
formula carry their ancestry in metadata and in convoy `tracks` edges; this
command walks those links and rolls every bead up under its origin, drawing one
box per workstream (or an indented tree).

By default it shows only workstreams the rig is actually working on: a stream
is in flight when one of its beads is `in_progress`, or is `open` with a
`started_at` (a session has claimed it). It reads the graph with a single
`bd export` in the target rig.

The rig defaults to the one containing the current directory: the command asks
`gc rig list --json` for the city's rigs and picks the one whose path contains
cwd, nearest first. Run it from a rig checkout, a rig subdirectory, or the city
root.

Usage:

    gc <binding> dashboard [--rig DIR] [--scope working|attention|all]
                           [--all] [--layout box|tree] [--columns N]
                           [--watch] [--interval SECONDS] [--json]

--rig DIR         Show this rig instead of the one under the current
                  directory.
--scope MODE      working = has an in-progress bead (default); attention =
                  working plus blocked/hooked streams; all = every stream with
                  active beads.
--all             Show everything, including closed beads (implies --scope
                  all).
--layout LAYOUT   box = bordered lanes per workstream (default); tree =
                  indented tree.
--columns N       Box columns (default: auto, 2 when wide enough).
--max-rows N      Max body rows per box before folding (default: 12).
-w, --watch       Re-render on an interval; q quits, r refreshes.
--interval SEC    Seconds between refreshes in --watch (default: 60).
--frames N        With --watch, render N frames then exit.
--json            Emit the resolved model as JSON.
--no-color        Disable ANSI colors.
--width N         Output width (default: terminal).

Examples:

    gc rex dashboard
    gc rex dashboard --scope all
    gc rex dashboard --layout tree
    gc rex dashboard --watch --interval 30
    gc rex dashboard --json | jq .

Exit status: 0 on success, 1 when `bd` or the rig lookup fails, 2 for a usage
error, 127 when python3 or `bd` is missing.
