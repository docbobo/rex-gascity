# Rex provider ↔ runtime-herdr-core parity

Method-by-method comparison of `gc-runtime-rex` (this repo) against the
`runtime-herdr-core` reference provider, for every RPP v0 operation plus
`watch-startup` and the ops herdr-core implements and we do not.

- Ours: `cmd/gc-runtime-rex/main.go` (dispatch) and `internal/provider` (behavior).
- Reference: `runtime-herdr-core` from `gastownhall/gascity-packs`, tree
  `runtime-herdr-core/assets/gc_runtime_herdr_core` (read at the commit recorded below).
- Contract: RPP v0 as pinned in [`REX-CONTRACT.md` §9](REX-CONTRACT.md) (gc 1.5.1).

herdr-core is a reference implementation, not ground truth. Three structural
differences are expected and are not defects by themselves:

1. **Hierarchy.** gc City → Rex session, Rig → tab (window), Agent → pane
   (block). herdr-core has its own box model with its own session naming.
2. **Protocol surface.** We implement RPP v0 only. herdr-core also implements
   extensions that gc 1.5.1 does not declare (`observe.v1`-style reads,
   `carrier.ops`-style object ops, `city-scoped`, `server-lifecycle`).
   Decisions for those extensions live in
   [`herdr-extensions.md`](herdr-extensions.md) (GCREX-28.1).
3. **Transport.** We drive a live Rex server over its socket (`rex` CLI/HTTP);
   herdr-core drives herdr. "Running", "attached", and "activity" therefore have
   different native meanings even when the op contract is identical.

Verdict vocabulary, per deviation:

| Verdict | Meaning |
|---|---|
| `intentional` | Deliberate consequence of 1–3 above, or of a recorded decision in `REX-CONTRACT.md`. Absent ops that we choose not to implement also carry this verdict. |
| `mistake` | Unintended divergence with an observable behavioral consequence. Fix recommended. |
| `not-implemented` | Op (or a behavior) absent from our provider. Optional ops may exit 2; `start` and `provision` may never be unknown by contract. |
| `unverified` | Difference confirmed on our side, but not confirmable without a live probe or the gc source. |

Status values in the coverage table: `implemented` (dispatch plus handler),
`partially implemented` (`start`: the op works, some start-config fields are
ignored), `not implemented` (exit 2). Sections appear in RPP v0 table order,
followed by `watch-startup`, `attach`, the meta ops, and the ops herdr-core has
and we do not.

**Reference commit.**
`gastownhall/gascity-packs` @ `0f07496` (`runtime-herdr-core`).
Our side: `d9e3785`.

**gc baseline.** Findings marked *verified* were checked against gc tag
`v1.5.1` (the contract's pinned baseline) in its source tree: the exec provider
client `internal/runtime/exec/`, the capability constants in
`internal/runtime/protocol.go`, and the contract document
`docs/reference/exec-session-provider.md`. Where earlier drafts of this
document said a gc behavior was unverified, the verified text now cites
`exec/exec.go` line numbers. Corrections that changed a verdict: `attach` is
called by gc and exit 2 there is a *failure* (resolved in GCREX-20 by answering
exit 0 with a hint), and an empty `env` value must be *unset*, not set to empty.

## Coverage

| Op | Status | Deviations | Verdict |
|---|---|---|---|
| `protocol` | implemented | 3 | intentional |
| `start` | partially implemented | 19 | intentional |
| `stop` | implemented | 8 | intentional |
| `provision` | not implemented | 1 | not-implemented |
| `exec` | not implemented | 1 | not-implemented |
| `is-running` | implemented | 5 | intentional |
| `process-alive` | implemented | 6 | intentional |
| `list-running` | implemented | 6 | intentional |
| `is-attached` | implemented | 6 | intentional |
| `get-last-activity` | implemented | 5 | intentional |
| `peek` | implemented | 5 | intentional |
| `nudge` | implemented | 11 | intentional |
| `send-keys` | implemented | 8 | intentional |
| `interrupt` | implemented | 4 | intentional |
| `clear-scrollback` | implemented | 6 | intentional |
| `watch-startup` | not implemented | 4 | not-implemented |
| `attach` | implemented | 4 | intentional |
| `set-meta` | implemented | 5 | intentional |
| `get-meta` | implemented | 3 | intentional |
| `remove-meta` | implemented | 5 | intentional |
| `observe` | implemented (declared unconditionally since GCREX-111) | 1 | partial |
| `inventory` | implemented (declared unconditionally since GCREX-111) | 2 | partial |
| `kill-object` | implemented (declared unconditionally since GCREX-111) | 1 | partial |
| `server-stop` | not implemented | 1 | intentional |
| `copy-to` | not implemented | 1 | not-implemented |

## Summary of deviations

| Op | Aspect | Rex provider | herdr-core | Verdict | Rationale |
|---|---|---|---|---|---|
| `protocol` | capability set | 7 tokens: `report-attachment` and `report-activity` (baseline) plus `idle-boundary`, `carrier.ops`, `input-outcome`, `observe.v1` and `city-scoped`, each declared alongside the behavior it gates | 10 tokens, 3 outside ours | intentional | Contract permits a non-baseline token only in the change that implements the behavior it gates; herdr's other extras target newer gc. |
| `protocol` | locality field | `"local"` | `"local"` | matches | Both state that the boxes run on the controller host. A baseline gc drops the field, and the edge build reads it for the storage-split check. We do not declare `fs.host`, which adds work-dir staging rules. |
| `protocol` | argument handling | answers before name check | via op table; name handling unverified | intentional | gc passes a name; both ignore it. |
| `start` | cancel and deadline | SIGTERM cancels the op context; the rollback defer runs before exit | SIGTERM cancel token, deadline-aware, `_undo` rollback | intentional | GCREX-25.2 installed the handler; gc's follow-up `stop` is now a backstop, not the only cleanup. |
| `start` | config fields consumed | 15 of the 15 gc v1.5.1 sends plus `fresh_only`; the eight GCREX-56 fields run/stage/warn, and `process_names` seeds `GT_PROCESS_NAMES` | 17 parsed (pack `0f07496`), all optional | partial | Verified against gc v1.5.1 (`internal/runtime/exec/json.go:26-42`): gc sends 15 fields, all omitempty. `process_names` has no Rex process-appeared signal, so it seeds observe.v1's liveness metadata instead of driving gc's 30 s agent wait (GCREX-107); `fresh_only`/`work_dir_exclude` are never sent by gc v1.5.1. GCREX-56 closed the hooks, staging, readiness and fresh_only gaps. |
| `start` | session_setup / session_live | `session_setup`/`session_setup_script` run after launch as warnings; `session_live` emits `SESSION_LIVE_SKIPPED`; both are stderr warnings gc drops on success | run after launch as warnings | intentional | GCREX-56; matches herdr-core (`herdrcore/lifecycle.py:811-824`). gc 1.5.1 discards a successful start's stderr (`exec/exec.go:102-104`), so the warning is an operator diagnostic only. |
| `start` | file staging | `overlay_dir` (universal tree minus per-provider/.gc, plus per-provider subtree) and `copy_files` staged before `pre_start`; `pack_overlay_dirs` per-provider subtree stays after `pre_start` | staged into work_dir before pre_start | partial | GCREX-56; `pack_overlay_dirs` deliberately runs after `pre_start` so its override beats a managed hook (row ``pack_overlay_dirs``). gc JSON-merges managed hook/settings files; this provider overwrites them (`herdrcore/staging.py:225-258`). |
| `start` | duplicate start / fresh_only | exit 1 "already running"; dead pane recycled; `fresh_only` refuses a held name with exit 73 `session "N" already exists (fresh_only)` | exit 1 "already exists"; 73 for fresh_only | intentional | GCREX-56 added the fresh_only refusal (`herdrcore/lifecycle.py:755-756`); gc v1.5.1 never sends `fresh_only` and reads any non-0/1/2 code as a plain error. |
| `start` | dead incarnation recycle | closes dead Rex block only | kills tree, refuses while descendants survive | intentional | Rex has no process-tree API |
| `start` | identity meta seed | 8 keys incl. GC_AGENT, GC_ALIAS, GC_TEMPLATE, plus GT_PROCESS_NAMES | 4 keys incl. GT_PROCESS_NAMES | intentional | serves gc pending-create check (GCREX-13/18); GCREX-107 adds GT_PROCESS_NAMES from `process_names` or env |
| `start` | session naming and hierarchy | session/tab/pane from GC_RIG and name | hashed terminal name plus labels | intentional | Rex hierarchy and GCREX-11 alias contract |
| `start` | env application | `env K=V` argv prefix; empty values emitted as `env -u KEY` | spawn-file launcher; empty unsets, HOME added | intentional | Empty values now match RPP and gc's tmux adapter. |
| `start` | pool work-query routing | for an unaliased pool/demand seat, sets `BEADS_ACTOR` to `GC_TEMPLATE`; `GC_ALIAS` stays empty | passes gc's identity through | intentional deviation | gc 1.5.1's hook picks its work-query agent from `firstNonEmpty(GC_ALIAS, BEADS_ACTOR, GC_AGENT, GC_SESSION_NAME)` (`hookSessionAgentForQuery`, `cmd/gc/cmd_hook.go:818`) and `hookWorkQueryStores` (`cmd/gc/hook_cross_store.go:107`) resolves the rig from it. gc's own blank `GC_ALIAS` plus bead-id `BEADS_ACTOR` resolves to no rig, so the query never reads the store holding the routed step (#5716); routing the actor to the template fixes discovery while `GC_ALIAS` stays empty, so the claim is recorded per-seat under `GC_SESSION_ID` and seats single-flight (GCREX-32). |
| `start` | secret exposure | tokens visible in pane argv | 0600 spawn file unlinked before parse | intentional (limitation) | side effect of argv-prefix mechanism |
| `start` | pre_start env and limits | os.Environ plus env (`""` unsets); per-hook 30 s, capped by the start budget | stripped env; 30 s per hook, cancel-aware | intentional | GCREX-25.2 closed the mistake: a hook cannot outlive the derived budget. GCREX-55 closed the empty-value mistake: an empty `env` value unsets the key. |
| `start` | lock scope | per-name and per-rig (tab) locks; the city-wide flock covers only store/session mutations | per-name lock released around pre_start | intentional | GCREX-25.2 closed the mistake: a slow hook or launch no longer stalls the rest of the city. |
| `start` | partial-failure side effects | empty city session or closed dead pane may remain | pre_start precedes hc objects; `_undo` cleans | intentional | empty session reused; harmless |
| `start` | post-launch liveness check | a single post-launch `failIfAgentEnded` pane probe plus the nudge readiness wait fail `the agent exited during startup[ before its first turn]`; `lifecycle=one_shot` skips both | waits for agent processes (5 s, 30 s with `process_names`), fails if it exited | intentional (with a gap) | GCREX-53 closed the missing failure. Rex exposes no distinct process-appeared signal, so the `_await_agent` wait is not replicated; whether a real instant exit is visible to the single immediate probe is unverified (row 14). |
| `start` | startup nudge | readiness wait, a screen read that withholds on a startup dialog, write plus Enter, submit confirmation, bounded resend with a clear+retype and a busy check, unconfirmed marker, per-stage timing diagnostic; errors swallowed | readiness wait, retries, unconfirmed marker | intentional | best effort by design; a lost Enter is resent down a bounded ladder, and a startup dialog withholds the nudge (GCREX-59); `<state dir>/diagnostics/<name>-startup-timing.json` records the stages (GCREX-58) |
| `start` | readiness signal | `ready_prompt_prefix` read like gc's `paneShowsReadyPrompt` (bottom-up, NBSP normalised, one border skipped, numbered-menu cursor row excluded); `ready_delay_ms` applies only when no prefix is set; bounded by the window (outcome `prompt-bound`), settle, or program_status idle; a startup dialog ends the wait as `dialog` and withholds the first turn | prompt prefix first, then the delay, then reported status, then settled; withholds on a dialog | intentional | GCREX-56; the wait is not tied to program_status when a prefix is set (gc v1.5.1 `internal/runtime/tmux/tmux.go:4199-4231`; herdrcore/readiness.py:78-86). GCREX-59 added the startup-dialog withhold (`withholdsFirstTurn`, herdrcore/readiness.py:36-93). |
| `start` | warnings channel | exit 0 plus `start: <warning>` lines on stderr for ignored/failed `session_setup*`, `session_live`, and a refused `copy_files` destination; gc drops a successful start's stderr | exit 0 plus warnings on stderr | intentional | GCREX-56; parity, not a fix: gc 1.5.1 returns only stdout on success (`exec/exec.go:102-104`), so the warnings are for an operator reading stderr. |
| `start` | empty or malformed stdin | empty accepted as empty config | any non-JSON, incl. empty, fails | intentional | gc always sends JSON |
| `start` | town tab order | when `start` creates the town tab in this city's canonical session, it re-reads `session.view` and moves the tab before `windows[0]` with `session.move_window`, unless it is already first (`placeTownTabFirst`, `internal/provider/launch_pane.go`, #47). Once per creation: the check runs on every town-tab creation, including a relaunch that recreates the tab. A relaunch moves the recreated tab first again. A split, a rig tab, a legacy session, or a start that creates no town tab does not reassert the order. Best-effort: a failed read or move is a `start:` warning, never a failure | no tab or window reorder found (`grep -rniE 'move_window\|tab order\|window order\|reorder'` over `runtime-herdr-core/assets/gc_runtime_herdr_core` at `0f07496`: no match). **unverified** | intentional | Rex-only presentation of the Rex hierarchy (structural difference 1). gc claim **unverified**: `git grep` of gc `v1.5.1` (`internal/runtime`, `docs/reference/exec-session-provider.md`, non-test) for `window order\|tab order\|move_window\|reorder` finds nothing, and this doc had no tab-order row, so neither source affirms that RPP v0 `start` has no tab-order requirement (requirements OQ-1). |
| `stop` | wait for exit / tree kill | Subtree snapshot before `block.close`, a second read of the pane's still-reported processes after the close unioned into it, then TERM, 2 s, KILL, 2 s, rescan (`reap.go`, `provider.go:777-793`) | Kills and reaps process tree, then removes record | intentional (with a gap) | Rex owns pty teardown; the pack reaps what the close leaves behind. The tree is the pane's foreground process and every host process below it at snapshot time, plus whatever the pane still reports after the close (a process started between snapshot and close), not an instance token, so attribution is a snapshot rather than a token sweep (GCREX-50). Neither upstream provider re-reads after close: herdr's `Stop` is close plus `clearMeta`, tmux snapshots once before kill. |
| `stop` | survivor handling | Exit 1 with `stopped '<name>', but descendants may survive (the pane stays for a retry): <pids>`; binding, meta and activity kept | Exit 1, box and meta kept when descendants may survive | implemented | GCREX-50; tests `TestStopFailsWhileDescendantsSurvive`, `TestStopSucceedsWhenTheSubtreeDies`. An unreadable process table fails the same way, so success means every process the pre-close snapshot captured or the post-close re-read still reported was observed gone; a process the host table never showed (or a sibling outside the foreground's subtree) is outside that guarantee. A failed post-close re-read does not fail the stop: the pre-close snapshot stays authoritative for what it captured (`TestStopReapsAProcessThePaneStartedAfterTheSnapshot`). A signal that fails for one process does not stop the pass, and a foreground that is a leaf narrows the snapshot to that process (`TestPaneSubtreePinsAForegroundLeaf`, `TestStopKeepsSignallingAfterASignalFailure`). |
| `stop` | already-gone session | nil/exit 0 for unknown name or vanished pane; no process-table read | exit 0 for absent name | intentional | Same contract; `TestStopAlreadyDeadSessionScansNoProcessTable`. |
| `stop` | state cleaned | Whole meta sidecar, activity entry and binding removed (GCREX-26 resolved the mistake) | Whole meta file and activity removed | intentional | Stop and its no-binding branch clear every meta key for the name and the activity entry (`clearIncarnations`, `provider.go:778`; `stores.go:238`, `:125`), so nothing leaks into the next start. |
| `stop` | locking scope | Per-name flock plus the city-wide state flock for the store mutations | Per-name exclusive lock | intentional | The per-name lock serializes a stop with a concurrent start/stop of the same name; the state flock still serializes concurrent stops. Accepted: the whole reap (up to 2 s + 2 s plus the `ps` calls) runs inside both locks, so a slow reap holds the city-wide lock until the op's external bound — under gc v1.5.1 that is SIGTERM at 30 s (`internal/runtime/exec/exec.go`) — and `reap` honours `ctx` only in `waitGone`, not in the signal calls or the `ps` forks. The reap touches no store, so the upgrade path is to snapshot under the locks and release the state lock before reaping. |
| `stop` | cancellation | context.Background, no cancel handling | Cancel-aware pauses | gap | SIGTERM kills process anyway; unverified |
| `stop` | drained-worker diagnostic | writes `<state dir>/diagnostics/<name>.json` when the work dir has modified tracked files (before the pane closes; best effort, shared 3 s bound) | no equivalent | intentional | Leaves a trace when gc drains a worker that still held edits (GCREX-32); never fails or delays stop. |
| `stop` | core/server unreachable | exit 0 with no binding for the name, exit 1 with a binding present (GCREX-51); no pid sweep | sweeps by pid/token, exit 0 when nothing runs, else exit 1 | intentional (with a gap) | A lost binding with a live pane reports success; the label scan needs Rex. |
| `provision` | op presence | Not implemented; exit 2, no proc.provision declared | Not implemented; exit 2, no proc.provision declared | not-implemented | Shared gap; gc only calls it with both capabilities declared, so exit 2 is unreachable today |
| `exec` | op presence | Not implemented; exits 2, `proc.exec` undeclared | Not implemented; exits 2, `proc.exec` never declared | not-implemented | Shared gap; gc falls back to driving ops (REX-CONTRACT §9) |
| `is-running` | server unreachable | Any client error exits 1, no stdout | `false` exit 0 if nothing of core runs | intentional | gc reads exit 1 and false alike as dead; no change needed. |
| `is-running` | liveness definition | Pane block exists and last_exit nil | Live record, pid label match, reason not broken | intentional | Backends expose different liveness data |
| `is-running` | no server start on read | Never starts Rex | Kicks core start when remnants exist | intentional | Rex is external; lifecycle not in RPP v0 |
| `is-running` | alias handling | Any alive alias-bound instance gives true | Newest record by exact name | intentional | Rex name-resolution model has aliases |
| `is-running` | side effects | Deletes stale sidecar bindings on read | Read-only apart from start kick | intentional | Sidecar self-heal; race safety unverified |
| `process-alive` | match scope | Rex foreground process plus ancestors | whole box process subtree | intentional | Rex API exposes only foreground and ancestors |
| `process-alive` | interpreter scripts | matches the interpreter's reported `script` basename, with and without its extension | matches script arg after node/bun/npx/deno | intentional | Rex reports the script path for interpreters (`docs/REX-CONTRACT.md:89`), so no argv array is needed (GCREX-47). Confirmed live (GCREX-31): a pane started as `node /tmp/gcrex31-agent.js` reports `name` and `argv0` `node`, with the path only in `script`. |
| `process-alive` | missing pane | `ErrNotFound` from `BlockProcess` returns `false`, nil (GCREX-23) | dead record answers false | intentional | Same answer; a pane that vanished mid-call no longer exits 1. |
| `process-alive` | zombie / liveness gate | LastExit non-nil forces false | zombie never matches; intact-record gate | intentional | last_exit is Rex liveness truth (GCREX-4) |
| `process-alive` | stdin line splitting | split on newline, trim each line and drop blank lines, in one parser shared with `observe` (`readNameList`, `cmd/gc-runtime-rex/main.go:585`). It reads only the first 64 KiB of stdin | splitlines then strip | intentional | same result for complete gc newline input within the 64 KiB read budget (gc `v1.5.1:internal/runtime/exec/exec.go:707` joins the whole list with `\n` and no cap) |
| `process-alive` | matched fields | name, argv0, basenames of path and argv0 | basename of comm and argv0 | intentional | different process data model |
| `list-running` | aliases in output | Lists aliases only for non-empty prefix | Lists only the session label name | intentional | GCREX-18 claim identity; avoids phantom orphan beads |
| `list-running` | what counts as running | Block exists with no LastExit | Live, pid label matches, not broken | intentional | No pid-label reincarnation case in Rex |
| `list-running` | name filter | Drops empty, padded, control-char names | Drops empty, padded, control-char names | intentional | GCREX-27 closed the mistake: a newline would split one session across two output lines and padding would leave a name gc cannot match, so both sides drop it before printing. |
| `list-running` | stdout cap | None | Fails over 8 MiB | intentional | Cap is only for inventory op in baseline |
| `list-running` | stopped/unreachable server | Rex transport error (`rex.ErrUnavailable`, CLI exit 4) returns empty, exit 0 | Parked city gives empty exit 0 | intentional | GCREX-41: no park concept, but an unreachable server answers the parked-city inventory |
| `list-running` | scope source | Local binding store, probed per binding | City-label terminal list from core | intentional | City→session mapping; bindings record ownership |
| `is-attached` | attachment source | Rex view block owner plus ListClients kind plus SessionView active window | process snapshot plus attach-lock file | intentional | Rex has a server-side client registry and per-block ownership |
| `is-attached` | meaning of attached | an app-kind client owns the block and the rig tab is active | hc attach process or lock on terminal | intentional | GCREX-108: the session-level app client no longer counts; a box the app never displayed has no owner. Rex tab tiles agents, so granularity is a tab, not a pane. |
| `is-attached` | stale ownership | owner still erring toward true (exit 0) | no equivalent | intentional | GCREX-108: ownership is a last-requested claim, not a heartbeat. An owner no longer in the client list may still be watching, so answer true rather than close the pane. |
| `is-attached` | unknown client kind | owner's kind unclassifiable answers true, exit 0 | n/a | intentional | GCREX-22: an unclassifiable kind means we cannot tell, and gc reads any is-attached error as not attached (fail-open), so the provider answers true. |
| `is-attached` | deadline | 30 s default | probe 4 s | intentional | gc 1.5.1 baseline is 30 s |
| `is-attached` | unreachable backend | transport error exit 1 | proc/lock error exit 1 | intentional | same shape, different backend |
| `is-attached` | stop-safety use | stop does not consult it | stop skips kill if attached/unknown | intentional | gc gates attachment in the reconciler, never before stop. |
| `get-last-activity` | timestamp format | RFC3339 whole seconds UTC | RFC3339 UTC with milliseconds | intentional | Contract takes RFC3339. Precision not required. |
| `get-last-activity` | active program | Returns now for `working`/`blocked` (`activity.go`) | Returns now for `working`/`blocked` | intentional | GCREX-24 closed the mistake: a stale working/blocked stamp made gc idle-stop and escalation-kill. |
| `get-last-activity` | input time | max of status/change time and last input (`activity.go`, `nudge.go`, `provider.go`) | max of status/change time and last input | intentional | GCREX-24 closed the mistake: nudge and send-keys record a floor so typed input is visible before its output renders. |
| `get-last-activity` | state locking | In-process mutex, last writer wins | flock with 0.5 s wait | intentional | One process per op. Race only shifts stamp slightly. |
| `get-last-activity` | reported-pane source | Queries Rex program_status each call | Reads cached state, no host query | intentional | Different carrier data sources. |
| `peek` | empty `lines` arg | empty string treated as 0 | empty string fails exit 1 | intentional | harmless; gc passes a number or nothing |
| `peek` | `lines`>0 row count and trailing newline | strips trailing newline before counting rows, restores it | trailing newline counts as a row; always appends newline | intentional | ours returns exactly N text rows; hc text ending unverified |
| `peek` | scrollback vs viewport | `format` returns scrollback plus the viewport | screen only, never scrolls | intentional | Confirmed live (GCREX-31): a 24-row pane with 200 numbered lines returned 206 rows, including `SCROLL-1`. |
| `peek` | output cap | pages 983040-byte replies, no total cap | exit 1 above 1 MiB | intentional | gc has no cap or truncation, so the 30 s timeout is the bound. Confirmed live (GCREX-31): a 1 217 315-byte capture answered `too_large` without `max_bytes` and paged to `eof` with it. |
| `peek` | unreachable core/server | exit 66 no screen for `ErrUnavailable`, else exit 1 with Rex error | exit 1, or 66 if city parked | intentional | decision-2: every no-screen cause is 66; no parked-city state. gc 1.5.1 reads 66 as an error. |
| `nudge` | busy / program_status gating | reads `program_status` first; refuses `working` with `ErrNudgeBusy` and `blocked` (kind named) with `ErrNudgeBlocked`, typing nothing, and answers exit 80 `busy`/`pending_interaction`; `NudgeModeImmediate` overrides | hc refuses: busy, pending_interaction | intentional | GCREX-16 implemented; GCREX-45 added the typed words. gc 1.5.1 reads only the exit code, so the gate must be provider-side. |
| `nudge` | submit confirmation | polls `program_status` for idle→working or a newer `updated_at` within a bounded window; format-fingerprint fallback; unconfirmed exits 0 with a `delivered_unobserved` line | hc `started` flag: delivered vs delivered_unobserved | intentional | GCREX-16 implemented; GCREX-45 emits herdr's success line. gc 1.5.1 ignores the report/words, so retyping stays avoided by not erroring. |
| `nudge` | exit code when pane gone | exit 80 `gone` | exit 80 `gone` | matches | declared input-outcome (GCREX-70) makes gc read the typed word |
| `nudge` | exit code when server down | exit 80 `unavailable`, nothing typed on a pre-send lookup failure, text already typed when the write or submit fails | exit 80 `unavailable` | intentional | GCREX-45, both fail for baseline gc. Confirmed with the CLI (GCREX-31): a down server exits 4, not 3, so it reads as `unavailable`. |
| `nudge` | framing: text and submit | write then enter, two non-atomic calls under one shared submit deadline; a post-write Enter failure says the text was typed and records `GC_NUDGE_SUBMIT_FAILED` | one atomic terminal.send | intentional | no atomic Rex primitive; GCREX-21 bounds the split and makes it visible. |
| `nudge` | multi-line and control chars | verified live: a two-line nudge reached Pi as one submitted turn; `write` inserts the newline without submitting; `block.event paste` still unused | one JSON string to hc; unverified | intentional | GCREX-21 live probe into Pi 1.5.2; observed once on Rex revision 14. |
| `nudge` | empty message | no-op exit 0 before pane lookup | sent like any text; hc reaction unverified | intentional | empty Enter would submit blank prompt |
| `nudge` | non-UTF-8 stdin | the provider rejects invalid UTF-8 (`checkNudgeText`, `internal/provider/nudge.go:233`; `ErrNudgeNotUTF8`, `input.go:14`) with exit 80 `rejected` before any Rex call, for the dispatcher and in-process callers alike | rejected, nothing typed | intentional | GCREX-21 rejected before typing; GCREX-45 changed the exit to herdr's 80. gc 1.5.1 reads either non-zero as failure. |
| `nudge` | deadline and cancellation | one submit deadline (10 s or the op context's earlier deadline) covers write and Enter; Enter still runs after a successful write when the parent is cancelled; an already-cancelled context writes nothing | send stops 5 s before deadline | intentional | GCREX-21 bounds the split; the remaining window is the transport's own call latency. |
| `nudge` | retries / settle time | none | none for nudge op | intentional | behavior matches; multi-line ingest is now verified, other Enter-ingest races unverified |
| `nudge` | activity bookkeeping | records input time after a successful write (`nudge.go`) | note_input stamps input time | intentional | GCREX-24 closed the gap: the input floor keeps a typed-but-unrendered nudge visible to idle detection. |
| `nudge` | secret-read gating | reads `input_mode` in both modes, after the Wait-mode busy/blocked gate and before the turn baseline. `canonical && !echo` withholds with exit 80 `pending_interaction` (`ErrSecretRead`) and types nothing. Startup records `withheld: the pane is reading a secret` and skips the `ctrl+u` clear when the pre-clear probe sees a secret read. The probe is a snapshot, so a read that starts after it can still receive the clear key. A failed or timed-out read (2 s, capped at half of a context deadline) fails open (`inputmode.go`). | no equivalent | intentional | #48, Rex-only. The predicate is **unverified**: neither this doc nor gc v1.5.1 covers `input_mode`. Its basis is `REX-CONTRACT.md` §4 and the #48 live probe (rex 0.1.0-preview.21, api 18). Raw-mode secret prompts read `canonical:false` and are not seen. gc 1.5.1 reads only the exit code. |
| `send-keys` | unknown key names | typed as literal chars, or rejected exit 80 (`keymap.go`) | typed as literal chars, or rejected exit 80 | intentional | GCREX-43 implemented the herdr-core grammar: literal text splits per character and an untypable key rejects before any lookup. |
| `send-keys` | key vocabulary | exact-case table with BTab/PPage/NPage, F1-F12 (`keymap.go`) | exact-case table with BTab/PPage/NPage, F1-F12 | intentional | GCREX-43 aligned the table to herdr-core's TMUX_KEYS. |
| `send-keys` | modifier handling | prefix only on a table name or a single literal char, in typed order (`keymap.go`) | prefix only on known name or single char | intentional | GCREX-43 aligned to herdr-core; Rex still validates the final binding. |
| `send-keys` | missing/dead session | typed `gone`, exit 80 | gone, exit 80 | matches | GCREX-42 classifies it; GCREX-70 declares input-outcome so gc reads the word |
| `send-keys` | zero keys | no-op exit 0 before lookup | looks up box first; gone if absent | intentional | fire-and-forget op |
| `send-keys` | failure handling | pre-send `unavailable`/`not_sent` exit 80; unknown-after-write exit 1, no retry | unknown outcome exits 1, never retried | intentional | GCREX-42; same effective result at baseline |
| `send-keys` | partial failure | all keys validated before sending (`provider.go`) | all keys validated before sending | intentional | GCREX-43 translates every key before the pane lookup, so a bad key never reaches Rex. Confirmed live (GCREX-31): `enter ctrl+` exited 2 and sent nothing. |
| `send-keys` | activity side-effect | records input time after a successful send (`provider.go`) | records input activity | intentional | GCREX-24 closed the gap: both stamp the input floor that `get-last-activity` reads. |
| `send-keys` | secret-read gating | a list of only ctrl+c (`C-c`) is sent with no read. Any other list reads `input_mode` and on `canonical && !echo` sends nothing, exit 80 `not_sent` (`ErrSecretRead`). A mixed list is withheld whole. A failed read fails open (`provider.go`, `inputmode.go`). | no equivalent | intentional | #48, Rex-only. The predicate is **unverified** (see the `nudge` secret-read row). gc may not answer `busy`/`pending_interaction` for send-keys (`890a58a3e:internal/runtime/exec/outcome.go:54-68`), so the word is `not_sent`. |
| `interrupt` | mechanism | ctrl+c keystroke via Rex SendKeys | hc terminal.interrupt; signal vs key unverified | intentional | Rex has no signal API |
| `interrupt` | wait/confirm | none; returns after send | hc waits up to 3 s to settle | intentional | RPP v0 has no outcome contract |
| `interrupt` | no live program | exit 80 `gone` | exit 80 `gone` | matches | GCREX-44 classifies it; GCREX-70 declared input-outcome (with carrier.ops) and dropped the bestEffort wrapper, so gc reads `gone` as ErrSessionNotFound |
| `interrupt` | failure mapping | pre-send exit 80 `unavailable`/`not_sent`; unknown-after-write exit 1 | unsent exit 80; unknown-after-write exit 1 | intentional | GCREX-44; Rex lacks a written/unwritten distinction |
| `interrupt` | repeat/retry | single send; client retry unverified | never retried | unverified | Check Rex SendKeys retry code |
| `clear-scrollback` | what is cleared | Rex `clear` call on the pane | no-op after intactness check | intentional | Rex has clear; hc has no clearable scrollback. |
| `clear-scrollback` | missing session | exit 80 `gone`, one stdout line, no clear | exit 80 `gone` | intentional | GCREX-57; gc 1.5.1 discards the error, so the typed outcome costs gc nothing. |
| `clear-scrollback` | ended or dead program | exit 80 `gone`, no clear | exit 80 `gone` | intentional | GCREX-57; `livePane` checks the program before clearing. |
| `clear-scrollback` | unreachable / unknown outcome | pre-send `unavailable`/`not_sent` exit 80; lost reply exit 1 | `unavailable` exit 80; lost reply exit 1 | intentional | GCREX-57; Rex cannot distinguish a dropped reply from an unsent request. |
| `clear-scrollback` | liveness pre-check | `livePane`/`paneAlive` before the clear | `terminal.get` before answering | intentional | GCREX-57 closed the gap: a dead program is `gone` before any clear. |
| `clear-scrollback` | peek and attached-client effect | verified live (GCREX-31): `clear` drops the scrollback and the visible screen, so a later peek shows blank rows, and the attached app client stays | nothing cleared, none | no-impact | gc calls it only after a kill and never re-peeks. |
| `watch-startup` | op presence | exit 2, no output | NDJSON screen stream at 250 ms, exit 0 or 1 | not-implemented | Verified: exit 2 makes gc poll peek every 500 ms instead. |
| `watch-startup` | stdout format | none (exits 2) | one `{"content":...}` NDJSON line per screen change, 250 ms poll | not-implemented | Would need a Rex block-format poll loop; no streaming helper exists. |
| `watch-startup` | deadline and cancellation | not applicable | `start` class, ends 1 s early with exit 0, SIGTERM gives exit 1 | not-implemented | gc kills `watch-startup` outright; SIGTERM handling is optional. |
| `watch-startup` | startup/readiness detection | none; the dead `Binding.ReadyPrompt` was removed (GCREX-26) | none in the op either; prompt matching stays in gc | intentional | Neither side uses the ready prompt inside `watch-startup`; the write-only field is gone. |
| `attach` | op presence | exit 0 with a one-line hint to open the Rex app; never contacts Rex | execs `hc attach` with tty passthrough and attach flock | intentional | gc calls attach; a hint satisfies the call without a pty passthrough, and exit 0 keeps `gc session attach` from failing. |
| `attach` | capability gating | does not declare `tty.attach` | op exists, `tty.attach` not declared either | intentional | Reserved and unread; gc calls attach without checking it. |
| `attach` | attach lock / viewer signal | `is-attached` derives viewers from `list_clients` and the active window | `attach` holds a flock that `is-attached` reads | intentional | Rex reports clients server-side; no lock needed. |
| `attach` | deadline | hint handler retires immediately; no deadline applies (spec: `attach` has none) | `start` class: 120 s default, pre-exec only | intentional | Spec and gc agree: no deadline; the class bounds setup only. |
| `set-meta` | unknown session | Exit 1 `no session`; requires a stored session record, Rex never contacted (GCREX-49) | Exit 1 `no session` | intentional | now matches herdr-core; the precondition is the cached binding, not a live probe |
| `set-meta` | lifetime across stop | Every key cleared on Stop (GCREX-26 resolved the mistake) | All meta deleted with incarnation | intentional | `clearIncarnations` clears the whole sidecar, so stale `suspended`/`GC_DRAIN_ACK` cannot leak into the next start. |
| `set-meta` | locking | In-process mutex, per-key files | Cross-process per-name flock | intentional | per-key files avoid read-modify-write |
| `set-meta` | size limits | No key or value cap | 1 MiB file read cap | intentional | unbounded values; low risk |
| `set-meta` | file hardening | 0600 temp, no symlink or owner check | 0600, O_NOFOLLOW, owner check | not-implemented | local single-user state |
| `get-meta` | empty key argument | exit 1, metadata key must not be empty (GCREX-26 resolved the gap) | exit 1, missing metadata key | intentional | Empty key is rejected at dispatch (`main.go:136`); gc never sends one, so this is hygiene only. |
| `get-meta` | backend failure | only corrupt local store errors | core unreachable or foreign terminal errors | intentional | Local sidecar store, no core |
| `get-meta` | metadata keying | by gc name via primary binding | by incarnation nonce of current record | intentional | Different state model; same result for unknown session |
| `remove-meta` | empty key | exit 1, metadata key must not be empty (GCREX-26 resolved the mistake) | rejects with "missing metadata key" | intentional | Empty key is rejected at dispatch (`main.go:136`); gc never sends it. |
| `remove-meta` | cross-process locking | in-process mutex, per-key files, atomic unlink | exclusive per-name file lock, whole-file rewrite | intentional | Per-key files avoid read-modify-write races. |
| `remove-meta` | unknown/foreign session | always OK, no existence check | unknown OK; foreign terminal errors | intentional | Name-keyed meta independent of Rex. |
| `remove-meta` | absent key | no-op, writes nothing | rewrites meta file, may create it | intentional | Both exit 0; fewer side effects. |
| `remove-meta` | provider-owned key guard | none for GC_READY_PROMPT_PREFIX | none | intentional | Matches reference. |
| `observe` | op presence | implemented (GCREX-101, declared unconditionally since GCREX-111): JSON state answer, 4 s probe budget of its own, or the op deadline when gc set one | JSON state answer, 4 s probe deadline | partial | Not in RPP v0 and gc 1.5.1 never sends it; the op and its `observe.v1` token are unconditional since GCREX-111. A recorded block Rex no longer knows is not proof the name is free: the op rescans the record's labels and answers for a pane that took them, absent only when the scan finds nothing (observe.v1 rule 7; `TestObserveAnswersForPaneRestoredUnderName`). Agent liveness (RPP-OBSERVE-006) reads one gc-shaped process snapshot per observe (`internal/provider/agentsnapshot.go`) and omits `agent` rather than guess it dead. `last_activity` (RPP-OBSERVE-015) is an `inventory` field, not an `observe` one. A slow backend answers `unknown` inside rule 11's bound (`TestObserveAnswersUnknownWithinBound`). |
| `inventory` | op presence | implemented (GCREX-101, completed by GCREX-105, declared unconditionally since GCREX-111): `{complete, sessions[]}` | returns complete flag plus per-session records | partial | Entries carry state, identity and `last_activity`, so RPP-OBSERVE-007 and -015 pass; attachment and restart flags are absent. A binding whose block Rex no longer knows, or a record written before its pane exists, is omitted while the listing stays complete, per rule 7 (`TestInventorySkipsVanishedBinding`, `TestInventoryOmitsAProvisionalRecord`). |
| `inventory` | bulk state read | gated: one call returns state, identity and last activity per session, from one Rex liveness probe per session and one city snapshot | one call returns state, attachment, activity and identity per session | partial | gc 1.5.1 uses the per-session ops instead. Completeness is claimed only when the binding store is listable and no city pane names an unlisted session. |
| `kill-object` | op presence | implemented (GCREX-101, declared unconditionally since GCREX-111): stdin-driven, `corpse` kills a dead, unattached box and answers `live` on a running or attached one, `gone` when the recorded block is already gone or another pane took the name; `zombie` answers `changed` | stdin-driven box kill, corpse/zombie modes | partial | Outside RPP v0; gc 1.5.1 never sends it. Zombie mode kills only when the agent check (RPP-OBSERVE-006) proves the agent dead. An exact-object kill of a pane that took a vanished record's name answers `gone` because the read ops never adopt it, so the re-check knows only the recorded box — the safe direction, pinned by `TestKillObjectGoneForRestoredPane`. |
| `server-stop` | op presence | absent; exit 2 unknown op | implemented; stops the city core and its sessions | intentional | No wire op in gc; teardown is a tmux-only Go interface. |
| `copy-to` | op presence | No entry, exit 2 unknown op (`main.go:115-122`) | Copies src into work dir, exit 0/1 (`screen.py:47-64`) | not-implemented | No gc 1.5.1 caller; staging is `copy_files` in start. |
## Confirmed mistakes

The deviations marked `mistake` across all sections, deduplicated and ranked by
what they can do to a running city: user-visible failures first, then wrong
answers about liveness, then lost or corrupted input, then state that leaks
across incarnations, then hygiene. Every entry has been checked against gc
1.5.1 source. Two verdicts flipped on that check: `is-running` (from mistake to
intentional, because gc reads a failure and `false` alike as dead) and `attach`
(from conformant omission to mistake). GCREX-20 has since resolved `attach` by
answering exit 0 with a hint, so it is no longer listed here. GCREX-22 has
since resolved `is-attached` by answering true for an unclassifiable client
kind, and GCREX-108 has since resolved its session-level mistake: a block the
app never displayed answers `false` because attachment now follows the pane's
own owner (RPP-OBSERVE-017). GCREX-23 has since resolved the missing-pane handling in `process-alive`
and `peek`, so those items are no longer listed; GCREX-46 has since resolved the
`process-alive` cannot-tell answer (a transient error, empty table or opaque
process now answers empty stdout, which gc reads as alive); GCREX-47 has since
resolved the interpreter-script gap by matching Rex's reported `script`
basename, with and without its extension. GCREX-24 has since resolved `get-last-activity` (return
now for `working`/`blocked`, plus the pack-input floor from nudge/send-keys), so
it is no longer listed either. GCREX-25.1 has since resolved the empty-env
mistake in `start` (values are emitted as `env -u KEY`), so it is no longer
listed here either. GCREX-25.2 has since resolved the `start` robustness
mistakes (a per-hook timeout bounded by the start budget, and pre_start plus the
blocking launch moved off the city-wide state lock) and added the SIGTERM
handler, so they are no longer listed. GCREX-26 has since resolved the
incarnation-hygiene mistake
in `stop`/`set-meta` (every meta key and the activity entry are cleared with the
incarnation) and the empty-key mistake in the meta ops, so those items are no
longer listed either. GCREX-27 has since resolved the `list-running`
readable-name mistake (a name that is empty, padded, or holds a control
character is dropped before printing), so the table below is now empty.

| # | Where | Mistake | Consequence in gc 1.5.1 | Sections |
|---|---|---|---|---|

Two earlier candidates were withdrawn after the gc check: the missing SIGTERM
handler in `start` (gc runs `stop <name>` after every failed or cancelled start,
so the damage was contained; GCREX-25.2 has since added the handler), and the
missing
`BTab`/`PPage`/`NPage` keys in `send-keys` (gc's entire vocabulary is
`Enter`/`Up`/`Down`/`Escape`/`C-u`, all mapped).

## Gaps that are deliberate, not mistakes

Recorded so they are not re-litigated. Verified against gc 1.5.1, `observe`,
`inventory`, `kill-object`, `server-stop` and `copy-to` are never sent, so those
omissions are inert; `exec` and `provision` are never attempted without the
matching capability declared, so their absence is inert too; `watch-startup`
exiting 2 is handled by gc falling back to `peek` polling every 500 ms. `attach`
was the one user-visible exit-2 omission; GCREX-20 now answers it with exit 0
and a hint, so no omission is user-visible.
`nudge` busy-gating and submit confirmation landed under GCREX-16, and input
validation plus the bounded submit under GCREX-21. The start-config gaps
(hooks, file staging, warning channel) are recorded
in `docs/REX-CONTRACT.md` §12 and `docs/rex-provider.md`.

## Operational risks that are not parity bugs

- **Rex outage reads as "everything is dead".** gc maps any `is-running`
  failure to `false` (`exec/exec.go:665-680`), so while the Rex server is
  unreachable every session looks dead and the reconciler starts replacements.
  There is no "unknown" channel for an exec provider in 1.5.1.
- **`rex` exit code 3 is broad, not overloaded.** `internal/rex/cli/cli.go`
  maps *any* `rex` exit 3 to `ErrSessionNotFound`, and the driving ops treat
  that as success. Confirmed against the CLI (GCREX-31, `REX-CONTRACT.md`
  §10.1): exit 3 is a 404 `not_found` answer and never a transport failure. A
  down or unreachable server exits 4, which our client reads as unavailable, so
  a nudge, clear or interrupt against a down server reports the right word. The
  one other exit-3 cause is an expired `blob_read` result, and its five-minute
  hold outlives the 30 s op deadline.
- **Probe fan-out.** gc's seam layer makes one `IsAttached`/`GetLastActivity`/
  `ProcessAlive` call cost up to four provider invocations (two `is-running`
  probes first). Each extra Rex round trip is paid on every observation.

## Proposed follow-up work

Not implemented in this task. Each item closes a set of the sections above.

1. **Rex-side probes.** Done in GCREX-31 against rex `0.1.0-preview.21`: exit
   codes, transport failure, `clear` then `format`, `format` coverage and size,
   `send-key` atomicity, and `process` on a node script. Results are in
   `REX-CONTRACT.md` §10.1. Exit 3 is not a transport failure, so no follow-up
   task was needed.

### protocol

**Status** — implemented

**Our implementation.** `runWithContext` handles `protocol` before the `ops` table lookup (`cmd/gc-runtime-rex/main.go:213`), so it needs no session-name argument. It prints the `handshake` constant plus a newline (`main.go:50`, `main.go:214`): `{"version":0,"locality":"local","capabilities":["report-attachment","report-activity","idle-boundary","carrier.ops","input-outcome","observe.v1","city-scoped"]}`. It reads no stdin and touches no state. `locality` states the deployment fact that Rex boxes run on the controller host under the same user. The edge build's `ProtocolInfo.DeclaresLocal` reads it (`internal/runtime/protocol.go:213-233`), and `sessionRuntimeIsRemote` uses it to allow a storage split (`cmd/gc/storage_verb_exclusion.go:314`). A baseline gc drops it. `fs.host` is not declared because it adds work-dir staging rules. It exits 0, or 1 if the stdout write fails (`main.go:215-218`). `report-attachment` and `report-activity` are in the six-token gc 1.5.1 baseline (`docs/REX-CONTRACT.md:302-312`); `idle-boundary` is not in the baseline and is declared only because the same change adds the `idle` op it gates (`REX-CONTRACT.md:314-321`). `carrier.ops` and `input-outcome` are declared by GCREX-70: gc selects the five session ops instead of exec and reads their typed exit 80 only with both tokens (gc `internal/runtime/exec/outcome.go:193`). Since GCREX-111 the handshake also declares `observe.v1` and `city-scoped`. `observe.v1` is the reconciler-v2 profile token (gc edge `internal/runtime/protocol.go:89`). It obliges `observe`, `inventory`, `kill-object`, `fresh_only` on `start` and exit 2 read as a failure. `city-scoped` marks GCREX-105's real per-city state isolation (RPP-OBSERVE-014). The `main.go:42-49` comment says `proc.exec`, `proc.provision`, `proc.stream` and `tty.attach` are not declared. The default 30 s deadline applies (set by gc).

**herdr-core.** `ops_table.py:68` maps `protocol` to `handshake_op(HANDSHAKE)` (`rpp/ops.py:109-116`). It encodes once, reads no stdin (`reads_stdin=False`), uses the default deadline and a small stdout cap. `HANDSHAKE` (`handshake.py:33-47`) declares ten tokens: `observe.v1`, `report-attachment`, `report-activity`, `city-scoped`, `conformance-faults`, `server-lifecycle`, `carrier.ops`, `idle-boundary`, `input-outcome`, `fs.host`, plus `locality:"local"`. `Handshake.encode` emits `locality` only when it is set (`rpp/wire.py:61-65`). It deliberately omits `proc.exec`, `proc.provision` and `input-fence` (`handshake.py:13-15`). The `handshake.py` docstring says its `fs.host` needs a newer gc (gastownhall/gascity#9032/#9129) (`handshake.py:20-22`).

**Deviations.**

| # | Aspect | Rex provider | herdr-core | Verdict | Rationale |
|---|---|---|---|---|---|
| 1 | capability set | 7 tokens: `report-attachment` and `report-activity` (baseline) plus `idle-boundary`, `carrier.ops`, `input-outcome`, `observe.v1` and `city-scoped`, each gating an implemented op | 10 tokens; 3 are outside ours (`conformance-faults`, `server-lifecycle`, `fs.host`) | intentional | Both declare the baseline pair and `idle-boundary`/`carrier.ops`/`input-outcome`; `observe.v1` and `city-scoped` carry the observe profile and city isolation; ours adds no feature gap. `REX-CONTRACT.md:302-312` lists the baseline and `:314-321` permits a non-baseline token only in the change that implements the behavior it gates. gc 1.5.1 ignores unknown tokens. herdr's other extras belong to features we do not implement and a newer gc. |
| 2 | `locality` field | `"local"` | `"local"` (needed by `fs.host`) | matches | Both state the boxes run on the controller host. A baseline gc drops it, and the edge build reads it for the storage-split check. We do not declare `fs.host`. |
| 3 | argument handling | `protocol` answers with no session name (`main.go:206`) | goes through the op table; whether it requires a name is unverified (need `rpp/perop.py`/`main.py`) | intentional | gc passes a name anyway. Both ignore it as far as the code shows. |

**Verdict.** intentional. Exit-code exceptions in gc 1.5.1: exit 2 is success only for ops it runs through its `run` helper (everything except `start` and `provision`); `attach` returns an error on any non-zero exit (`exec/exec.go:196-201`), `exec` reads exit 2 as "unsupported" unless `proc.exec` is declared and any other non-zero exit as the command's result (`:894-908`), and `watch-startup` reads exit 2 before its first line as "unsupported" (`:454-457`). Exit 66 has no meaning to gc: any code other than 0, 1 or 2 is a plain error. Also note the seam layer: in production gc wraps the exec provider in `seamProvider`, so one `IsAttached`, `GetLastActivity` or `ProcessAlive` call fans out to up to four ops (two `is-running` probes first, then `get-last-activity`, `is-attached`, `process-alive`), and when `is-running` fails the others are never called (`runtime/seam_adapter.go:88-159`, `exec/seams.go:229-236`). Our handshake declares the baseline tokens we implement plus `idle-boundary`, `carrier.ops`, `input-outcome`, `observe.v1` and `city-scoped`, each non-baseline but declared because the same change implements the behavior it gates (`idle`; the carrier ops; typed driving-op outcomes; the observe profile and per-city isolation in GCREX-111). herdr's other extras belong to its own provider features and a newer gc. Verified in gc 1.5.1: it parses exactly six tokens (`report-attachment`, `report-activity`, `proc.exec`, `proc.stream`, `tty.attach`, `proc.provision`; `internal/runtime/protocol.go:18-58`), ignores unknown strings by design (`protocol.go:15-16`, pinned by `exec/handshake_test.go:91-102`), and degrades to no capabilities on a malformed handshake (`exec/handshake.go:45-51`). The edge build adds `observe.v1` and `city-scoped` to that set (`internal/runtime/protocol.go:89`, `runtimecontract/catalog.go:162`). Only three gates consume them: `is-attached` requires `report-attachment`, `provision` requires `proc.provision` plus `proc.exec`, and `proc.exec` decides what exit 2 from `exec` means. `get-last-activity` is called regardless of `report-activity`.

**Cross-references.**
GCREX-19.1 — this task
GCREX-19.4 — `provision` and `proc.provision` gating
GCREX-19.17 — `attach` and `tty.attach` gating
GCREX-19.16 — `watch-startup`
`docs/REX-CONTRACT.md` §9 — baseline capability table and the "declare only alongside the behavior it gates" list

**Follow-ups.**
- Re-check the `report-attachment` and `report-activity` tokens against `gc runtime check` when `is-attached` or `get-last-activity` change (gap).
- Add `proc.exec`, `proc.provision` and `tty.attach` only together with their ops (gap).


### start

**Status** — partially implemented

**Rex-only extensions.** Layout operations beyond create are recorded in
[`research/rex-provider-extensions.md`](research/rex-provider-extensions.md) §3.

**Our implementation.** `start` is dispatched at `cmd/gc-runtime-rex/main.go:107` to `opStart` (`main.go:262`). It reads all of stdin; empty or whitespace-only stdin fails with `parse start config: empty stdin` (exit 1, nothing launched), otherwise JSON is decoded into every gc v1.5.1 field, plus `fresh_only` (`main.go:74-91`, `main.go:272-294`); `lifecycle` only distinguishes `one_shot` for the post-launch liveness gate. `process_names` fills `GT_PROCESS_NAMES` when env leaves it unset (trimmed, comma-joined; GCREX-107); `work_dir_exclude` is ignored. `Provider.Start`/`StartNew` (`internal/provider/provider.go:364`, `:386`) take a per-name flock (`lock.go:38`, 60 s wait) for the whole op and refuse a live pane with `session "<name>" already running` (exit 1, `main.go:295-298`), or, with `fresh_only`, any held name (live or dead) with exit 73 `session "<name>" already exists (fresh_only)` (`provider.go:346-358`, `main.go:292-294`), then `start` (`provider.go:465`) does: resolve existing binding and close a dead pane (`:548-571`), find or create the `gc:<city>` Rex session under the short city-wide state flock (`launch_pane.go:28`, `lock.go:26`), run `pre_start` via `/bin/sh -c` with `os.Environ()` plus `env` (`hooks.go:81-102`, `runner.go:59-121`), each hook under a per-hook timeout capped by the start budget (`hooks.go:35-59`), require `work_dir` to exist (`provider.go:600-608`), stage `overlay_dir` and `copy_files` before `pre_start` and each `pack_overlay_dirs` entry's `per-provider/<GC_PROVIDER>/` subtree after it (`overlay.go:59-159`, called at `provider.go:491` and `:500`, with `:577-582` and `:615-617`), seed identity meta keys (`GC_SESSION_ID`, `GC_INSTANCE_TOKEN`, `GC_RUNTIME_EPOCH`, `GC_CONTINUATION_EPOCH`, `GC_AGENT`, `GC_TEMPLATE`, `GC_ALIAS`, `GC_PROVIDER`; `identity.go:65-105`) and a provisional binding (`:447-:458`), then read the session view and launch under the per-rig (tab) flock (`:469`, `lock.go:52`) with `NewSplit`/`NewWindow` (tab = `GC_RIG` else name prefix before `--`, `binding.go:85`; pane = agent part, `sessionname.go:11-30`) and record the final binding (`:486`, `:494`). The env is applied by prefixing argv with `env K=V …`, only when a command exists; empty values are emitted as `env -u KEY` (`internal/launch/launch.go:154-175`, applied at `:462`). After the launch, `Start`/`StartNew` run `session_setup`/`session_setup_script` as warnings-only hooks and warn `SESSION_LIVE_SKIPPED` for a non-empty `session_live` (`provider.go:1125-1161`), then probe the pane once and fail `the agent exited during startup` when the program has already ended (`provider.go:435-447`). The startup nudge first waits for the pane program to report `program_status` `idle`, up to 15 s, or for the shorter 3 s settle delay when the program reports no status at all; with `ready_delay_ms` it also floors the wait, and with `ready_prompt_prefix` it waits for the pane's prompt instead of `program_status` (`nudge_delivery.go:294-392`); it then reads the pane's screen and withholds the nudge when it shows a startup dialog gc knows (`readiness.go:78-228`, `nudge_delivery.go:374-407,470-472`), otherwise writes plus Enter, best effort, errors dropped, and resends a delivered-but-unconfirmed submit down a bounded backoff ladder (clearing the drafted line with `ctrl+u` and re-typing it, unless the pane has since gone busy; a failed clear stops the ladder) (`nudge_delivery.go:110-200`, `nudge.go:275-340`). A nudge readiness wait that sees the pane die fails `the agent exited during startup, before its first turn` (`nudge_delivery.go:286-336`); `lifecycle=one_shot` skips both. On error after seeding, a deferred rollback clears the identity keys and forgets the binding (`launch_pane.go:80-97`). The handler runs under a `signal.NotifyContext` for SIGTERM/SIGINT (`main.go:52`, `runWithContext` at `main.go:136`), which cancels the op context; a cancel aborts the in-flight pre_start or Rex call and lets the rollback defer run before gc's 2 s forced kill. gc 1.5.1 sends no deadline env var, so the start budget is the provider's own 120 s default (`defaultStartBudget`, `provider.go:268`).

**herdr-core.** `start` is `deadline="start"`, `stdin=True` (`herdrcore/ops_table.py:69`) and `op_start` (`herdrcore/lifecycle.py:763`) parses 17 start-config fields strictly; empty or non-JSON stdin fails (`rpp/wire.py:102-129`, `rpp/jsonval.py:20-26`). The transport installs a SIGTERM handler that sets a cancel token, polls stdin against the deadline, honours `GC_EXEC_DEADLINE_MS` and defaults to 120 s (`rpp/perop.py:50-57`, `:60-80`, `:129`, `:143-145`; `rpp/ops.py:23-27`). `_start` (`lifecycle.py:778-825`) takes a per-name lock, rejects a live box (`session "<n>" already exists`, or exit 73 when `fresh_only`, `:754-760`), recycles dead incarnations (killing the process tree and refusing while old descendants survive, `:488-506`), stages files (`herdrcore/staging.py:238`), runs `pre_start` with a 30 s per-hook limit (`:793-796`, `:616-624`, `:168`) outside the lock, then `_create` re-locks, rechecks the name, writes seed metadata (`GC_SESSION_ID`, `GC_RUNTIME_EPOCH`, `GC_INSTANCE_TOKEN`, `GT_PROCESS_NAMES`, `gc_bin`, `work_dir`; `herdrcore/metastore.py:88-91`, `rpp/wire.py:43`) and launches via a 0600 spawn file that the launcher unlinks before reading (`herdrcore/launch.py:30-48`). The agent env is the launcher env minus `HC_*`, with session values applied (`""` unsets) and gc's `HOME` added (`herdrcore/envplan.py:51-76`). Any failure or cancel in `_create` runs `_undo` (kill tree and token holders, remove spawn file, metadata; `lifecycle.py:887-891`, `:919-970`). After launch it waits for the agent (5 s, or 30 s with `process_names`, `:1004-1024`), fails with `the agent exited during startup` unless `lifecycle=one_shot` (`:1027-1036`), runs `session_setup*` as warnings (`:810-818`), then types the nudge after a readiness wait of up to 60 s (`:1051-1133`), recording `GC_STARTUP_DELIVERY_UNCONFIRMED` when not confirmed (`:1136-1146`). Warnings go to stderr with exit 0 (`:825`).

**Deviations.**

| # | Aspect | Rex provider | herdr-core | Verdict | Rationale |
|---|---|---|---|---|---|
| 1 | cancel and deadline | `signal.NotifyContext` cancels the op context on SIGTERM/SIGINT (`main.go:41`); a cancel aborts the pre_start/Rex call and the rollback defer (`provider.go:301-:311`) runs before exit | SIGTERM sets a cancel token, ops poll it, `_undo` rolls back within 1.5 s (`perop.py:50-57`, `lifecycle.py:181`, `:887-891`) | intentional | GCREX-25.2 added the handler. gc cancels with SIGTERM to the adapter's process group and force-kills after 2 s (`exec/exec.go:92`, `execgrace/execgrace.go:133-165`), then runs `stop <name>` itself unless `is-running` was literally `true` beforehand (`exec.go:262-270`); the rollback now runs first and gc's `stop` is a backstop. Residual risk: an orphan pane if the process dies in the narrow window between the launch call and the binding record. |
| 2 | config fields consumed | 14 of gc v1.5.1's 15 fields (`process_names` ignored; `lifecycle` only as the `one_shot` liveness gate), plus `fresh_only` (`main.go:74-91`) | 17 fields (`wire.py:80-99`) | partial | Verified against gc v1.5.1: it sends 15 fields, all `omitempty` (`exec/json.go:26-42`): `work_dir`, `command`, `lifecycle` (only `"one_shot"`), `env`, `process_names`, `nudge`, `ready_prompt_prefix`, `ready_delay_ms`, `pre_start`, `session_setup`, `session_setup_script`, `session_live`, `pack_overlay_dirs`, `overlay_dir`, `copy_files`. GCREX-56 closed the hooks, staging, readiness and `fresh_only` gaps; `process_names` has no Rex process-appeared signal. gc never sends `fresh_only`; `work_dir_exclude` is pack-only. |
| 3 | `session_setup`, `session_setup_script`, `session_live` | `session_setup`/`session_setup_script` run as warnings-only hooks after launch; `session_live` warns `SESSION_LIVE_SKIPPED` (`provider.go:1125-1161`) | run after launch as non-fatal warnings; `session_live` reported skipped (`lifecycle.py:810-824`) | intentional | GCREX-56; warnings go to stderr, which gc 1.5.1 drops on a successful start (`exec/exec.go:102-104`), so they are an operator diagnostic only. Pinned by `TestStartSessionSetupRunsAsWarningsOnly`, `TestStartWarnsSessionLiveSkipped`. |
| 4 | file staging (`copy_files`, `overlay_dir`, `pack_overlay_dirs`) | `overlay_dir` (universal subtree minus `per-provider`/`.gc`, plus `per-provider/<GC_PROVIDER>`) and `copy_files` staged before `pre_start`; `pack_overlay_dirs` per-provider subtree staged after `pre_start` (`overlay.go:59-159`, `provider.go:491-502`, `:615-617`) | staged into `work_dir` before `pre_start` (`lifecycle.py:789`, `staging.py:238-258`) | partial | GCREX-56 staged `copy_files`/`overlay_dir`; `pack_overlay_dirs` deliberately runs after `pre_start` so its override beats a managed hook. gc JSON-merges managed hook/settings files, this provider overwrites them. Pinned by `TestStartStagesOverlayDirAndCopyFiles`, `TestStartCopyFilesRefusesEscapingDestination`. |
| 5 | duplicate start / `fresh_only` | live pane: exit 1 `session "<n>" already running` (`main.go:295-298`); dead pane closed and relaunched (`provider.go:468-478`); `fresh_only`: exit 73 `session "<n>" already exists (fresh_only)` (`provider.go:346-358`); gc never sends `fresh_only`, so exit 73 is unreachable from gc. | live box: exit 1 `session "<n>" already exists`; exit 73 for `fresh_only` (`lifecycle.py:754-760`) | intentional | Both fail and recycle. Verified: gc lower-cases the start stderr and matches the substrings `already exists` or `already running` (`exec/exec.go:144`, `:185-193`), with no exit-code check, so both phrasings match and the difference is cosmetic. A collision makes gc skip its post-failure `stop` and re-observe the session via `get-meta`, so `GC_SESSION_ID`/`GC_INSTANCE_TOKEN` must match the pending create. Pinned by `TestStartNewFreshOnlyRefusesAHeldName`, `TestStartFreshOnlyExits73`. |
| 6 | recycle of a dead incarnation | closes the dead Rex block only (`provider.go:186`); no process-tree reap, no refusal | kills tree and token holders, refuses start while descendants may survive (`lifecycle.py:488-506`) | intentional | Rex has no process-tree or token API. Descendants left by a dead pane are not handled: unverified. |
| 7 | identity meta seed | 8 keys incl. `GC_AGENT`, `GC_TEMPLATE`, `GC_ALIAS`, `GC_PROVIDER`, `GC_CONTINUATION_EPOCH`; no `GT_PROCESS_NAMES`, no `gc_bin` file (`identity.go:65-74`) | 4 keys incl. `GT_PROCESS_NAMES`; `GC_ALIAS`/`GC_AGENT` not seeded (`wire.py:43`) | intentional | Ours serves `get-meta` for gc's pending-create check (GCREX-13, GCREX-18); `GC_BIN` basename goes on the binding (`provider.go:373`). `process-alive` takes names on stdin, so no stored `process_names`. |
| 8 | session naming and hierarchy | `gc:<city>` session, tab from `GC_RIG` or `<rig>--` prefix, pane from agent part; aliases from `GC_TEMPLATE`/`GC_ALIAS`/`GC_AGENT`/`GC_RIG` (`binding.go:77-120`) | opaque name hashed to an hc terminal name plus `gc.*` labels (`naming.py:159-177`) | intentional | Reflects Rex hierarchy and GCREX-11 alias contract; herdr has no rig/tab concept. |
| 9 | env application | `env K=V …` argv prefix, only with a command; a bare shell gets no env; empty values are emitted as `env -u KEY` (`provider.go:596-628`, applied at `:418`) | spawn-file launcher; `""` unsets, gc `HOME` added when absent (`envplan.py:14-16`, `:51-76`) | intentional | Rex launch options carry no env (`REX-CONTRACT.md:483-484`), so an argv prefix is the available mechanism. Empty values now match the protocol's "empty = unset" rule (`docs/reference/exec-session-provider.md` §Session environment) and gc's tmux adapter (`tmux/tmux.go:614-627`); GCREX-25.1 re-verified this against the current source and `TestLaunchCommandEmptyEnvIsUnset`. For an unaliased pool seat the provider sets `BEADS_ACTOR` to `GC_TEMPLATE` and leaves `GC_ALIAS` empty: gc's hook resolves its work-query agent from `firstNonEmpty(GC_ALIAS, BEADS_ACTOR, GC_AGENT, GC_SESSION_NAME)` (`cmd/gc/cmd_hook.go:818`) and `hookWorkQueryStores` (`cmd/gc/hook_cross_store.go:107`) needs that identity to resolve the rig, so gc's own blank `GC_ALIAS` plus bead-id `BEADS_ACTOR` never reaches the routed step (#5716). Keeping `GC_ALIAS` empty is what makes the claim per-seat: gc records it under alias first (`hookClaimAssigneeIdentity`, `cmd/gc/cmd_hook.go:866`), and the GCREX-18 deviation that set `GC_ALIAS` to the template made every slot claim under one string no session bead answers to, so a second seat adopted the first's `in_progress` bead and `releaseOrphanedPoolAssignments` (`cmd/gc/pool_session_name.go:211`) reopened the work. |
| 10 | secret exposure | env values incl. `GC_INSTANCE_TOKEN` appear in the pane's argv (`provider.go:301-306`) | secrets pass via a 0600 file unlinked before parse (`launch.py:30-48`) | intentional (limitation) | Consequence of the argv-prefix mechanism; visibility in `ps` and Rex launch records: unverified. gc's own providers route non-allow-listed env through a private file, not argv (`internal/runtime/envsecret.go`). |
| 11 | `pre_start` env and limits | `os.Environ()` overlaid with `env`, where an empty value unsets the key (`mergeEnv` removes it) instead of setting `K=`; each hook under a per-hook timeout (30 s default, `WithPreStartHookTimeout`) capped by the remaining start budget (the op context deadline, else 120 s, `WithStartBudget`); a hook failure or timeout warns and the start continues; only whole-start cancellation aborts; error carries full combined output (`launch.go:235-260`, `hooks.go:35-102`, `provider.go:591-593`) | gc env minus `HC_*`, `""` unsets; 30 s per hook bounded by op deadline and cancel; a failing hook warns and does not abort (`docs/reference/exec-session-provider.md:328-333` at v1.5.1); last output line only (`lifecycle.py:599-624`) | intentional | GCREX-25.2 closed the mistake: a hanging hook is bounded by the derived budget instead of running until gc kills the group. GCREX-55 closed the empty-value mistake: `mergeEnv` now removes an empty overlay key, so a hook sees it unset rather than `K=`, matching gc's env contract (`docs/reference/exec-session-provider.md:269` at v1.5.1) and herdr-core (`envplan.py:67-71`, `lifecycle.py:599-609`); pinned by `TestMergeEnvEmptyValueUnsets`, `TestPreStartEnvEmptyValueIsUnset`, `TestMergeEnvNonEmptyValuePassedThrough`. GCREX-54 chose gc's non-fatal contract over the pack's fatal one; pinned by `TestPreStartFailureWarnsAndContinues`, `TestPreStartSecondCommandWarnsAndLaterHooksRun`, `TestPreStartHookTimeoutWarnsAndContinues`, `TestPreStartHookCappedByStartBudget` and `TestPreStartHookCappedByContextDeadline`. |
| 12 | lock scope | per-name flock across the whole start (`lock.go:38`) plus a per-rig (tab) flock across the view read and the blocking launch (`lock.go:52`); the city-wide state flock covers only the find-or-create-session and identity/binding writes (`lock.go:26`), never pre_start or the launch | per-name lock released before `pre_start`, re-taken and rechecked in `_create` (`lifecycle.py:782-787`, `:833-838`) | intentional | GCREX-25.2 closed the mistake: a slow hook or launch no longer stalls every other start/stop of the city. The per-name lock serializes same-name starts, the tab lock keeps the create-tab/join-tab decision atomic per rig, and `TestSlowPreStartDoesNotHoldStateLock`/`TestSlowLaunchDoesNotHoldStateLock` pin that neither holds the state lock. |
| 13 | partial-failure side effects | on a work_dir failure or whole-start cancellation the Rex session `gc:<city>` may already be created and a dead pane already closed (`provider.go:186`, `:197`, `:210-226`); a failing `pre_start` hook no longer fails the start | `pre_start` runs before any hc object exists; `_undo` removes the created box (`lifecycle.py:793-797`, `:919-970`) | intentional | Empty city session is harmless and reused (`FindSession`, `provider.go:192-201`); the pilot accepted gc owning session lifetime (`rex-pilot-phase5.md:52`). |
| 14 | post-launch liveness check | `Start`/`StartNew` probe the launched pane once after the launch (`failIfAgentEnded`, `provider.go:302-318`) and fail `the agent exited during startup` when it has already ended; a startup-nudge readiness wait that sees the pane die fails `the agent exited during startup, before its first turn` (`nudge_delivery.go:286-336`); `lifecycle=one_shot` skips both (`TestStartInstantExitFails`, `TestStartMissedFirstTurnFails`, `TestStartOneShotInstantExitSucceeds`, `TestStartOneShotMissedFirstTurnSucceeds`) | waits for agent processes (5 s, 30 s with `process_names`) and fails with `the agent exited during startup` unless `lifecycle=one_shot` (`lifecycle.py:1004-1036`), and fails `...before its first turn` when `_ready` sees the agent end (`:1113-1125`) | intentional (with a gap) | GCREX-53 closed the missing failure. Rex exposes no distinct "agent process appeared" signal, so the 5 s/30 s `_await_agent` wait is not replicated: an exit visible to the post-launch probe is caught there, and, for a nudge-bearing start, the readiness wait also fails when the pane's program ends during the wait. Whether a real instant Rex exit is visible to the single immediate probe, rather than needing a wait like the pack's `_await_agent`, is unverified. `process_names` (which would select the 30 s wait) is still ignored. |
| 15 | startup nudge | waits for `program_status` `idle` (15 s window) or a 3 s settle delay when no status is reported; with `ready_prompt_prefix` it waits for the pane's prompt instead, and `ready_delay_ms` floors the wait (capped 60 s) (`nudge_delivery.go:294-392`), then reads the pane's screen and, when it shows a startup dialog gc knows, withholds the nudge (`readiness_outcome` `dialog`, `GC_STARTUP_DELIVERY_UNCONFIRMED` `withheld: …`); otherwise it writes plus Enter and observes the bounded confirmation window; a delivered but unconfirmed submit is resent down a 2/4/6/8 s ladder after clearing the drafted line with `ctrl+u` (skipped if the pane has gone busy; a failed clear stops the ladder), and a confirmed delivery clears `GC_STARTUP_DELIVERY_UNCONFIRMED`, and a readiness wait that sees the pane's program end fails the start unless `lifecycle=one_shot`; delivery failures are otherwise swallowed (`nudge_delivery.go:110-200`, `nudge.go:214-340`) | waits up to 60 s for prompt, delay or settled screen, withholds on a startup dialog, retries the submit with backoff, records `GC_STARTUP_DELIVERY_UNCONFIRMED` and warns (`lifecycle.py:1051-1146`) | intentional (with a gap) | Best effort is by design (GCREX-14, GCREX-34; gc queued-nudge backstop). Re-verified against gc 1.5.1 `sendStartupNudgeWithRetry` (`internal/runtime/tmux/adapter.go:1712`, called at `:1593`) and `defaultReadyProbeTimeout` (`:944`): gc waits for a prompt and retries with a busy check; this provider waits for the program's own `idle` record, confirms the submit and resends a lost Enter with the same busy-check shape, and withholds on a startup dialog (GCREX-59). Each stage is timed into `<state dir>/diagnostics/<name>-startup-timing.json`: `launch_ms`, `readiness_ms` with `readiness_outcome` (`idle`/`window-bound`/`settle-bound`/`prompt`/`prompt-bound`/`dialog`/`ended`/`canceled`/`disabled`), `write_ms`, `confirm_ms`, `confirmed`, per-resend `attempts[]` and `total_ms`, so a slow start is attributable to a stage and the bound is known to be (or not be) the cost (GCREX-58). GCREX-56 honoured `ready_prompt_prefix` and `ready_delay_ms`; the write-only `Binding.ReadyPrompt` field was removed (GCREX-26). Pinned by `TestStartReadinessUsesReadyDelayMs`, `TestStartReadinessUsesReadyPromptPrefix`, `TestShowsDialogMatchesHerdrCoreReadiness`, `TestWithholdsFirstTurnMatchesHerdrCoreReadiness`, `TestStartupNudgeWithheldOnStartupDialog`, `TestStartupNudgeDeliveredWhenDialogGone`. |
| 16 | warnings channel | exit 0 plus `start: <warning>` on stderr for failed `session_setup*`, `session_live` and a refused `copy_files` destination (`provider.go:46-51`, `main.go:185`); gc drops a successful start's stderr | exit 0 plus `start: <warnings>` on stderr (`lifecycle.py:825`, `perop.py:166-167`) | intentional | GCREX-56 emits the warnings; gc 1.5.1 returns only stdout on success (`exec/exec.go:102-104`), so the channel is for an operator. |
| 17 | empty or malformed stdin | empty or whitespace-only stdin fails `parse start config: empty stdin` (exit 1, nothing launched); malformed JSON fails (`main.go:230-243`; `TestStartEmptyStdinFails`) | any non-JSON, including empty, fails (`wire.py:102-129`) | intentional | Now matches herdr-core; gc always sends a marshalled config object, so real calls are unaffected. |

**Verdict.** intentional. The Rex mapping, identity seeding and idempotent recycle are deliberate and tested. GCREX-25.2 resolved the two operational mistakes: `pre_start` is now bounded by a per-hook timeout capped by the start budget (row 11), and `pre_start` plus the blocking launch run outside the city-wide state lock under per-name and per-rig locks (row 12). GCREX-25.2 also added the SIGTERM handler (row 1), so a cancel runs the rollback before gc's forced kill. GCREX-54 chose gc's non-fatal `pre_start` contract: a failing hook warns and the start continues (row 11). The empty-env case (row 9) is intentional: GCREX-25.1 emits `env -u KEY`. GCREX-56 closed the hooks, staging, readiness and `fresh_only` gaps (rows 2-5, 15, 16). GCREX-59 closed the startup-dialog withhold. GCREX-107 consumes `process_names` as observe.v1 identity metadata; the remaining gap is gc's process-appeared wait, which Rex has no signal for.

**Cross-references.**
GCREX-2 — original start/stop/is-running/peek core
GCREX-10 — env map applied by argv prefix
GCREX-11 — `GC_*` inventory, tab/alias mapping (`REX-CONTRACT.md` §13)
GCREX-13 — identity seeded before launch, rollback on failure
GCREX-14 — startup nudge delivery
GCREX-15 — `pre_start` hook, `work_dir` check
GCREX-16 — nudge busy-gating (related to row 15)
GCREX-18 — `GC_ALIAS`/`GC_AGENT` claim identity
GCREX-5 — RPP v0 delivery, remaining start-config hooks (`REX-CONTRACT.md:479-509`)
GCREX-25.2 — pre_start timeout, per-name/per-rig locking, SIGTERM rollback

**Follow-ups.**
- Wait for `process_names` when the config names the agent's processes (gap; no Rex process-appeared signal).
- The startup nudge confirms the submit and resends a delivered-but-unconfirmed submit down a bounded ladder (clearing and re-typing), recording `GC_STARTUP_DELIVERY_UNCONFIRMED` only when the ladder is exhausted; GCREX-59 added the startup-dialog withhold. GCREX-56 honoured `ready_prompt_prefix`/`ready_delay_ms`.
- Check that the agent is alive after launch and fail the start if it exited (gap).
- Keep `already running` in the live-pane error; gc matches it (`exec/exec.go:144`) (no action).
- Consider a launcher or file-based env hand-off so tokens stay out of pane argv (gap).


### stop

**Status** — implemented

**Our implementation.** Dispatch is `cmd/gc-runtime-rex/main.go:90` (one required arg) to `opStop` (`main.go:206`), which calls `Provider.Stop` (`internal/provider/provider.go:531`). `Stop` takes a per-name flock and the exclusive state-dir flock (`internal/provider/lock.go:38`, `:26`), resolves the binding, and issues a single Rex `block.close` for the agent's pane (`provider.go:739-793`, `internal/rex/cli/cli.go:181`). It is idempotent: no binding returns nil after clearing the whole incarnation (`provider.go:759`), and a `rex.ErrNotFound` from the close is swallowed (`provider.go:763`; tests `provider_test.go:144`, `teardown_test.go:113,138`). Any other close error is returned and the binding is kept (`teardown_test.go:125`). Cleanup clears every meta key and the activity entry through `clearIncarnations` (`provider.go:778-794`; `stores.go:238` file meta, `:125` file activity) as well as the binding (`binding.go:435`), so no `set-meta` key or activity stamp outlives the incarnation (GCREX-26). The op reads no stdin, writes nothing to stdout, exits 0 on success and 1 on error (`main.go:114-152`). It runs with `context.Background()` (`main.go:147`). Before the close it snapshots the pane's subtree — the foreground process Rex reports and every host process below it (`ps -axo pid=,ppid=,stat=,comm=`; each captured process is named by its `ps` `comm` in that same snapshot, falling back to Rex's `Name` only for a reported pid the table does not show — Darwin's `comm` is argv0, so Rex's executable name (`node`) would not match a live row (`pi`); a survivor is identified by pid plus the snapshot's `ps` `lstart`, so a renamed descendant still counts and a reused pid does not; the reported ancestors supply, with no foreground, the walk's root, but are not themselves reaped); the snapshot must precede the close because a detached descendant is reparented to init and can no longer be attributed to the pane. A `BlockProcess` failure fails the stop before the close, leaving the pane open for the retry. After the close it reaps what the pane left: rescan, TERM, 2 s wait, KILL, 2 s wait, final rescan (`internal/provider/reap.go`, `provider.go:777,788`; GCREX-50). A survivor, or an unreadable process table, fails the stop with `stopped '<name>', but descendants may survive (the pane stays for a retry): <pid> (<name>), ...` — `: <signal error>` appended when a signal could not be delivered — and keeps the binding, every meta key and the activity entry; a retried stop starts over from a fresh snapshot.

**herdr-core.** `op_stop` (`herdrcore/lifecycle.py:511`) takes the exclusive per-name lock, lists the city's records for that name, and calls `_recycle` (`lifecycle.py:420`). `_recycle` calls `kill_incarnation` per record (`lifecycle.py:327-368`): it snapshots the process table, captures the tree by instance token, asks hc to kill, reaps and rescans the token, and only then removes the record. Then `forget` drops the record's meta file and activity (`lifecycle.py:389-397`). If descendants may survive, the box and its meta stay and the op exits 1 with "descendants may survive" (`lifecycle.py:523-524`; `test_herdrcore_treekill.py:576`). A second stop on an absent name exits 0 (`test_herdrcore_lifecycle.py:118`). With the core unreachable and nothing running, it sweeps by stored pid/token and exits 0 without starting a core (`lifecycle.py:518-522,528`; `test_herdrcore_lifecycle.py:127`). An unreachable core with something still running is exit 1.

**Deviations.**

| # | Aspect | Rex provider | herdr-core | Verdict | Rationale |
|---|---|---|---|---|---|
| 1 | wait for exit / tree kill | Single `block.close`; returns on Rex ack; no process-table check (`provider.go:433`) | Kills and reaps the process tree by token, rescans, and removes the record only when clean (`lifecycle.py:348-367`) | intentional (with a gap) | Rex owns the pane's pty and teardown, so pack-side reaping is not in the contract. Whether `block.close` kills detached descendants is unverified; it needs a Rex-side check. |
| 2 | survivor handling / exit code | No "may survive" outcome; exit 1 only on close/transport error (`teardown_test.go:125`) | Exit 1 and the box and meta stay for a retry when descendants may survive (`lifecycle.py:523`) | not-implemented | Follows from row 1. gc retry semantics rely on this only in herdr. |
| 3 | already-gone session | nil/exit 0 for unknown name or vanished pane (`provider.go:426-429`, `:433`) | exit 0 for absent name (`test_herdrcore_lifecycle.py:118`) | intentional | Same contract. We also clear seeded identity when no pane exists (`provider.go:429`). |
| 4 | state cleaned | Whole meta sidecar and activity entry removed plus the binding (`provider.go:778-794`, `:769`; `stores.go:238`, `:125`) | Whole meta file and activity entry removed (`lifecycle.py:389-397`; `test_herdrcore_lifecycle.py:124`) | intentional | GCREX-26 closed the mistake: stop and the no-binding branch call `clearIncarnations`, which clears every key for the name (identity seed and user/provider keys alike) and the activity record, so a restart reads nothing stale. `seedStartIdentity` still clears the identity keys on a rolled-back start (`identity.go:46`). |
| 5 | locking scope | A per-name flock across the whole stop (`lock.go:38`) plus the state-dir-wide flock for the store mutations (`lock.go:26`), including the reap | Per-name exclusive lock (`lifecycle.py:515`) | intentional | The per-name lock serializes start/stop of one name; the state flock still serializes the store mutations of concurrent stops, with a 60 s wait that can reach the 30 s op deadline. Accepted: the reap's 2 s + 2 s waits plus the `ps` calls run under the city-wide lock, so a stop with stubborn processes holds it until the external bound — gc v1.5.1 SIGTERMs the op at 30 s (`exec/exec.go`) — and `reap` honours `ctx` only in `waitGone`, not in the signal calls or the `ps` forks; a SIGTERM during the reap can therefore arrive after `block.close` and before the store mutations, leaving binding/meta/activity for the retry. The reap touches no store; the upgrade path is to take the snapshot under the locks and release the state lock before reaping. Rex is the only provider that re-checks after the close: herdr's `Stop` is close plus `clearMeta` (`internal/runtime/herdr/provider_stop_other.go:24-38`), tmux snapshots once before kill and does not fail on survivors past SIGKILL (`internal/runtime/tmux/tmux.go:1524-1552`). |
| 6 | cancellation | `signal.NotifyContext` cancels the op context on SIGTERM/SIGINT (`main.go:41`); the blocking close and the lock waits honour it | Cancel-aware pauses (`ctx.request.cancel.pause`, `lifecycle.py:348`) | intentional | GCREX-25.2 added the handler. gc sends SIGTERM to the process group and force-kills after 2 s (`exec/exec.go:92`), so partial effects between `block.close` and `Forget` are possible but bounded. |
| 7 | drained-worker diagnostic | Before `block.close` (or from the stored binding when the pane is already gone), if the binding's `WorkDir` is a git work tree with modified tracked files, writes `<state dir>/diagnostics/<name>.json` with worker, aliases, `GC_SESSION_ID`, changed files and the newest-mtime file (`internal/provider/drain_diag.go`, called at `provider.go:425` and `:432`); untracked files ignored, git runs with `GIT_OPTIONAL_LOCKS=0`; best effort, one shared 3 s bound | none | intentional | Added for GCREX-32 so a drained worker's lost edits are traceable. Skipped silently for a clean tree, non-git dir, empty work dir or empty state dir. |
| 8 | core/server unreachable | `rex.ErrUnavailable` (CLI exit 4) from `Resolve`: with no stored binding answering to the name (exact or alias, `Binder.lookup`, `binding.go:197`) the name's meta and activity are cleared and stop exits 0; with a binding present the error is returned, exit 1, and the binding, meta and activity stay (`Provider.stop`, `provider.go:701-711` as of slice 1, commit `b0d44f4`; GCREX-51). No pid sweep, no server start kick. | Core down and `nothing_runs`: `_stop_without_core` sweeps by stored pid/token, exit 0 without starting a core; otherwise exit 1 and a start kick (`lifecycle.py:518-522`, `:528`) | intentional (with a gap) | gc calls stop as a backstop after a failed or cancelled start and joins a stop error into the start error (`internal/runtime/exec/exec.go:262-270`, call at `:266`; `Stop` returns the op error, `:581-583`; gc `v1.5.1`), so exit 1 on an absent name would add a misleading cleanup error alongside the preserved start error (`errors.Join`). Exit 1 is kept while a binding is known because the pane may still be live and stop cannot close it while Rex is down. The gap: a lost binding record with a live pane reports success, because the label scan that could find it needs Rex, and there is no pid sweep to fall back on. Pinned by `TestStopUnreachableWithoutBindingSucceeds`, `TestStopUnreachableWithoutBindingClearsMeta`, `TestStopUnreachableRexUnknownNameExitsZero`, `TestStopUnreachableWithBindingKeepsError`, `TestStopUnreachableAliasBindingKeepsError`. |

**Verdict.** intentional. Fire-and-forget pane close is the right fit for the Rex mapping, and idempotence and exit codes match. Stop now clears every meta key and the activity entry with the incarnation (GCREX-26), matching herdr's `forget`.

**Cross-references.**
GCREX-2 — start/stop/is-running/peek baseline
GCREX-12 — retired session whose Rex pane stays live
GCREX-13 — identity seeding that stop clears
GCREX-17 — name resolution (alias) used by stop
GCREX-19.24 — server-stop comparison
GCREX-51 — unreachable server with no binding exits 0

**Follow-ups.**
- Confirm that `block.close` kills detached descendants, or document the limit (gap).


### provision

**Status** — not implemented

**Our implementation.** `provision` is listed in the `unimplemented` map (`cmd/gc-runtime-rex/main.go:161-165`) and has no `ops` entry. Dispatch hits the `!known` branch, prints `operation "provision" not implemented` to stderr and returns `exitUnknown` = 2 (`main.go:228-235`). stdin is not read and stdout stays empty. The handshake does not declare `proc.provision` or `proc.exec` (`main.go:42-46`; `docs/REX-CONTRACT.md:468-471`). Provision's 120 s deadline (`docs/REX-CONTRACT.md:188-189`) never comes into play.

**herdr-core.** `provision` is absent from the op table (`herdrcore/ops_table.py:61-91`). The module docstring says `exec`, `provision` and `check-image` "exit 2 without reading stdin" (`herdrcore/ops_table.py:9-10`). `HANDSHAKE` lists ten capabilities and none is `proc.provision` or `proc.exec` (`handshake.py:34-45`); the docstring says it "never declares" them (`ops_table.py:6-9`). Unit tests assert both facts (`tests/unit/test_rpp_perop.py:69`, `tests/unit/test_herdr_core_entrypoint.py:103-107,147`). This is a shared gap.

**Deviations.** None observed.

Contract consequence. `docs/REX-CONTRACT.md:200` says exit 2 means "unknown operation, treated as success", but "from `start` and `provision` it is a failure – those two may never be unknown". So for `provision` (unlike every other op) exit 2 is read as a failure, not as a forward-compat no-op. `provision` is one of the 17 RPP v0 rows (`REX-CONTRACT.md:222`). `proc.provision` means "implements `provision` (box without agent); requires `proc.exec`" (`REX-CONTRACT.md:250`), and `proc.exec` is also undeclared. We stay safe only because gc calls `provision` only when `proc.provision` is declared, and we do not declare it. Confirmed in gc 1.5.1: `provisionBox` runs `provision` only when the handshake declares both `proc.provision` and `proc.exec` (`exec/exec.go:277-296`), and `provision` is its only caller; otherwise gc falls back to `start`. A failed `provision` is not followed by `stop` (`docs/reference/exec-session-provider.md` §provision), so a future Rex `provision` would have to roll back after itself. What breaks: provisioning a box without an agent (create the box, no agent process) is unavailable. Declaring `proc.provision` without implementing the op would make every such call fail with exit 2. Conformance reports `exec`/`provision` as optional SKIP (`REX-CONTRACT.md:467-468`).

**Verdict.** not-implemented. Both providers omit the op and the capability, so the gap is shared and consistent with the handshake. It is a deliberate non-declaration, not a protocol violation. The never-unknown rule would only bite if we declared the capability.

**Cross-references.**
GCREX-5 — recorded `exec`/`provision` as unimplemented v1 limit and conformance skip
GCREX-8 — capability vocabulary alignment
GCREX-19.4 — this comparison
`docs/REX-CONTRACT.md` §9 (lines 188-200, 222, 250, 467-471)

**Follow-ups.**
- Keep `proc.provision` undeclared until `exec` and `provision` both exist (gap).
- Do not add `provision` to `ops` without declaring `proc.exec` first, since `proc.provision` requires it (gap).
- If box-without-agent is wanted, design `exec` (Rex pane command run with in-box exit code) first, then `provision` (gap).


### exec

**Status** — not implemented

**Our implementation.** `exec` is listed in `unimplemented` (`cmd/gc-runtime-rex/main.go:161-165`) and is absent from the `ops` table (`main.go:175-193`). `run` looks the op up in `ops`, finds nothing, writes `operation "exec" not implemented` to stderr and returns `exitUnknown` = 2 (`main.go:228-235`). That path runs before the arg check, state-dir resolution and the handler, so stdin is never read, nothing is written to stdout and no Rex state is touched; the 30 s deadline never applies. The handshake declares `report-attachment`, `report-activity`, `idle-boundary`, `carrier.ops`, `input-outcome`, `observe.v1` and `city-scoped` and withholds `proc.exec`, `proc.provision`, `proc.stream` and `tty.attach` (`main.go:42-46`; `docs/rex-provider.md:212-213`; `docs/REX-CONTRACT.md:468-471`). `cmd/gc-runtime-rex/dispatch_test.go:107-120` pins exit 2 with empty stdout for `exec`.

**herdr-core.** Same result. `build_ops` has no `exec` row (`herdrcore/ops_table.py:61-91`; the table holds protocol, start, stop, kill-object, observe, inventory, is-running, process-alive, list-running, is-attached, get-last-activity, peek, the meta ops, nudge, send-keys, interrupt, clear-scrollback, watch-startup, attach, copy-to, server-stop, and the kick op). `perop.py:115-120` returns `ExitCode.UNKNOWN_OP` (2) with `exec: unknown operation` on stderr, before any stdin read. `handshake.py:10-20` says `proc.exec`, `proc.provision` and `input-fence` "never are" declared. The reason it gives: under `proc.exec` gc would drive the box by piping tmux commands to `exec`. `rpp/wire.py:36-39` leaves `proc.exec`, `proc.provision`, `tty.attach` and `input-fence` out of the declarable token set. Tests assert the handshake has none of `proc.exec`, `proc.provision`, `input-fence`, `tty.attach`, `proc.stream` (`tests/unit/test_herdr_core_entrypoint.py:103-107`). They also assert `exec` exits 2 with empty stdout and an unread stdin payload (`test_herdr_core_entrypoint.py:111-118`, `:147-`; `tests/unit/test_rpp_perop.py:69-75`).

**Deviations.** None observed. Both providers leave `exec` out of the op table, exit 2 without reading stdin and withhold `proc.exec`. This is a shared gap, not a deviation. The only differences are the reason (herdr-core: a security stance, so gc cannot pipe tmux text to `exec` (`handshake.py:10-20`); ours: work not done yet, `docs/plans/native-rex-provider.md:412`, `:436`) and `proc.stream`/`tty.attach`, which both withhold.

Contract note (gc 1.5.1): `exec` is one of the 17 RPP v0 rows (`docs/REX-CONTRACT.md:221`), so exit 2 is the forward-compatible "not supported" answer: exit 2 is "unknown operation, treated as success" for every op except `start`/`provision` (`REX-CONTRACT.md:196-201`). `REX-CONTRACT.md:225-230` (§9) states the effect. gc drives input and reads output over `exec` (`RPP-CONN-001`) when a runtime implements it, and falls back to the driving ops when `exec` is unsupported. Verified in gc 1.5.1 (`internal/runtime/exec/exec.go`): the fallback fires only on exit 2 with `proc.exec` undeclared (`exec.go:894-904`); exit 0 or any other code is read as the in-box command's result, and each of the five driving methods calls its dedicated op on `ErrExecUnsupported` (`exec.go:643-646`, `:724-729`, `:753-758`, `:777-782`, `:804-809`). The carrier path discards the in-box exit code (`carrier.go:70-73`), so a runtime that answered `exec` with 0 or 1 would make every driving op a silent no-op, and with `proc.exec` undeclared every driving call costs two processes (the `exec` probe, then the dedicated op). Those dedicated driving ops are `interrupt`, `nudge`, `peek`, `clear-scrollback` and `send-keys`. We implement all five (`main.go:85-96`), so conformance holds. `watch-startup` stays dedicated either way, because the request/response `exec` connection cannot carry a stream (`REX-CONTRACT.md:229-230`). Overloaded exit-2 hazard: exit 2 from `exec` could mean "op unknown" or "the in-box command exited 2". `proc.exec` is the capability that disambiguates it ("`exec`'s process exit carries the in-box command's exit", `REX-CONTRACT.md:249`). Without `proc.exec`, gc reads exit 2 as unsupported and falls back. If `exec` is ever implemented, `proc.exec` must be declared with it, and `exec` must return the command's exit code (`docs/plans/native-rex-provider.md:243-246`, `:436-437`). `proc.stream` is reserved for the persistent bidirectional `stream` op, so there is no `exec`-style op to implement (`REX-CONTRACT.md:251`), and `proc.provision` requires `proc.exec` (`:250`). Neither provider declares either capability. gc 1.5.1 always attempts the `exec` op (it is not capability-gated) and only the exit-2 branch consults `proc.exec` (`exec/exec.go:894-904`).

**Verdict.** not-implemented. Both providers omit `exec` and fall back to dedicated driving ops. Exit 2 is the contract-valid answer and the fallback needs no action. Implementing `exec` is optional and would only add a second driving path.

**Cross-references.**
GCREX-5 — delivery and handshake; `exec`/`provision` stay unimplemented and `proc.exec` is undeclared (`REX-CONTRACT.md:464-471`)
GCREX-3 — the driving ops (`nudge`, `send-keys`, `interrupt`) that serve as the fallback
GCREX-19.4 — `provision`, the other exit-2 op, which requires `proc.exec`
GCREX-19.16 — `watch-startup`, which stays dedicated
GCREX-19.17 — `attach`, which maps to the reserved `tty.attach`
`docs/REX-CONTRACT.md` §9 — op table, fallback rule, capability table
`docs/rex-provider.md:268` — lists unimplemented ops that exit 2
`docs/plans/native-rex-provider.md:412`, `:436` — the "declare `proc.exec` once `exec` carries the exit" plan

**Follow-ups.**
- Keep `exec` unimplemented and `proc.exec` undeclared while the driving ops serve as the fallback (gap, no change needed).
- If `exec` is ever added, return the in-box command's exit code, declare `proc.exec` in the same change, and extend the `dispatch_test.go:107-120` and handshake tests (gap).
- Keep `docs/rex-provider.md:268` and `REX-CONTRACT.md:468-471` aligned with any change, and keep `proc.stream`/`tty.attach` undeclared until implemented (gap).


### is-running

**Status** — implemented

**Our implementation.** Dispatch `cmd/gc-runtime-rex/main.go:84` (1 arg) → `opIsRunning` (`main.go:193`) → `Provider.IsRunning` (`internal/provider/provider.go:408`). No stdin. Stdout is `true`/`false` plus newline (`printBool`, `main.go:158`), buffered and flushed only on success (`main.go:100`, `main.go:129-142`). Running means: a binding for the name (or an alias) revalidates against `SessionView` (`binding.go:243`, rescan by labels if stale, `provider.go:429-432`) and `BlockProcess` for that block succeeds with `LastExit == nil` (`liveness.go:105-115`). A missing block or `ErrNotFound` is `false` (`liveness.go:107-110`). A pane with `LastExit != nil` is `false`. `program_status` and process names are not used, because Rex runs commands under a shell (`liveness.go:100-104`). No pane or session means `false`, exit 0 (`binding.go:194-204`, `provider.go:408-415`). Any other client error, including an unreachable Rex server (`ListSessions`/`SessionView`/`BlockProcess`), is returned and the process exits 1 with stderr `gc-runtime-rex: is-running: …` (`main.go:123-127`). No cross-process cache: the sidecar binding store is only a lookup hint and is revalidated on every call. An unpinned caller (no `GC_REX_CITY`/`GC_CITY_PATH`/`GC_CITY`) discovers the sibling city state directories under its base and reads their bindings too (GCREX-35), so a live pane a pinned controller started still answers `true`; the `gc:<city>` session label still has to revalidate. Stale bindings are deleted as a side effect (`provider.go:434-437`, `binding.go:300-302`). Any alias match that is alive makes the answer `true` (`provider.go:405-407`).

**herdr-core.** `ops_table.py:74` → `reads.op_is_running` (`reads.py:287`): `current_record(..., liveness=True)` then `encode_bool(record is not None and is_intact(record))` (`wire.py:294`, `true`/`false`). Intact means the record is live, has a pid, its reason is not in `BROKEN_REASONS`, and the pid label matches (`records.py:53-62`, `records.py:26`). A finished or broken record is `false`. Each call asks the core afresh (`reads.py:7-8`). If the core is unreachable and `nothing_runs` (no socket holder or core process), the answer is `false` with no start kicked (`reads.py:275-276`, docstring `reads.py:10-12`). If the core is unreachable but something of it still runs, or the error is not `HcUnreachable`, it kicks a start and exits 1 (`reads.py:277-278`, `reads.py:91-95`).

**Deviations.**

| # | Aspect | Rex provider | herdr-core | Verdict | Rationale |
|---|---|---|---|---|---|
| 1 | server unreachable | Any client error → exit 1, no stdout (`main.go:123-127`) | `false`, exit 0 when nothing of the core runs; else exit 1 (`reads.py:275-278`) | intentional | Verified: gc reads exit 1, exit 2, empty and unparseable output all as `false` ("dead"); the exec provider has no unknown channel (`exec/exec.go:652-654`, `:665-680`; `internal/runtime/liveness.go:31-43`). Exit 1 and `false` are the same answer to gc, so no change is needed. Operational risk, not a parity bug: during a Rex outage every session reads dead and the reconciler starts replacements. |
| 2 | liveness definition | Pane exists and `LastExit == nil` (`liveness.go:105-115`) | Record live, pid label matches, reason not broken (`records.py:53-62`) | intentional | Different backends. Rex exposes `last_exit` and block existence only. |
| 3 | no core start on read | Never starts Rex | Kicks a start on unreachable-with-remnants (`reads.py:91-95`) | intentional | Rex is an external app we do not own. Server lifecycle is not in RPP v0. |
| 4 | alias handling | Any alive alias-bound instance gives `true` (`provider.go:405-407`) | Single newest record by name (`reads.py:273`, `newest`) | intentional | GCREX-17/18 name-resolution model. herdr has no alias set. |
| 5 | side effects | Deletes stale sidecar bindings during the read (`provider.go:434-437`) | Read-only, apart from the start kick | intentional | Sidecar self-heal. Concurrent ops may race on the store (lock behavior unverified). |

**Verdict.** `intentional (with a mistake inside)`. The semantics (pane block exists and `last_exit` is nil) fit Rex, and the output vocabulary matches. The difference with herdr-core (exit 1 vs `false` for a down core) is invisible to gc: both mean dead. The real consequence is operational: while Rex is unreachable the reconciler sees every session as dead and tries to start replacements, so a brief Rex outage can churn sessions.

**Cross-references.**
GCREX-2 — original Start/Stop/IsRunning/Peek
GCREX-12 — retired session whose pane stays live (liveness semantics)
GCREX-17 — name resolution across the namespace candidate set
GCREX-35 — unpinned callers discover the pinned sibling namespaces
GCREX-11 — env-derived tab/pane labels used in rescans
`docs/REX-CONTRACT.md` §9 — RPP v0 exit codes

**Follow-ups.**
- No code change is needed for gc: exit 1 and `false` are equal. Decide outage behavior separately, e.g. retry the Rex connection briefly inside the 30 s op (gap).
- Add a test for is-running with a dead Rex socket (gap).
- Check that stale-binding deletion on a read is safe under concurrent ops (gap).


### process-alive

**Status** — implemented

**Our implementation.** Dispatch is `cmd/gc-runtime-rex/main.go:114` (`{1, "process-alive <name>", opProcessAlive}`). `opProcessAlive` (`main.go:346-368`) reads all of stdin, splits on `\n`, drops one trailing empty element, calls `Provider.ProcessAlive`, and prints `true`/`false` (`printBool`, `main.go:226-228`) for a definite answer; `LivenessUnknown` prints nothing and exits 0, the pack's cannot-tell answer that gc reads as alive (`herdrcore/reads.py:301-302`; gc `exec/exec.go:709-718`). `ProcessAlive` (`internal/provider/liveness.go:139-151`, reading through `readPaneLife(...).match`, `internal/provider/panelive.go:47,125`) resolves the binding (no binding returns dead, nil), calls Rex `process` once for the pane (`BlockProcess`): `ErrNotFound` is dead (GCREX-23), any other error is unknown (GCREX-46), so a transient or unreadable process call never reads as dead. It then expands names with `expandProcessNames` (`liveness.go:32-59`: TrimSpace each line, drop blanks, add the `kimi`→`kimi-code` alias, add the basename of `GC_BIN` for the `gc` hint, falling back to the provider's own `$GC_BIN` for legacy bindings). `processAlive` (`liveness.go:90-117`) is dead if `LastExit != nil`; alive if the expanded list is empty; alive iff the pane's `Foreground` process or any of its `Ancestors` matches; unknown if the process list is empty or holds an opaque process (no `name`, `path`, `argv0` or `script`) after no match; dead otherwise. `prochost.Match` (`internal/prochost/prochost.go:87`, called from `liveness.go:107`) is exact equality of a name against Rex `name`, `argv0`, `basename(path)`, `basename(argv0)`, or the basename of the interpreter's reported `script` with and without its extension (GCREX-47); no substring matching. Multiple names mean any-of. Touches no state files; deadline is the default 30 s. gc skips the op unless `process_names` is non-empty (`exec/exec.go:703-706`), and in production only after two successful `is-running` probes.

**herdr-core.** Dispatch is `herdrcore/ops_table.py:75` (`stdin=True`). `op_process_alive` (`herdrcore/reads.py:292-303`) parses stdin with `stdin_names` (`reads.py:63-65`: `splitlines()`, strip, drop blanks), then returns `false` if the name has no intact record (`reads.py:295-296`), `true` if no names (`reads.py:297-298`), else runs `agent_state` (`reads.py:104-118`) and prints `true`/`false` via `encode_bool` (`rpp/wire.py:293-295`). `agent_state` expands names with `expand_names` (`herdrcore/procs.py:550-561`: strip, drop blanks, `kimi-code` alias, basename of the stored `gc_bin` for `gc`) and `find_agent` (`procs.py:608-613`) searches the box's whole host-process subtree (`subtree`, `procs.py:504-522`) from the box pid. `matches` (`procs.py:564-583`) is exact basename of `comm` or `argv[0]`; if `argv[0]` is one of `node/bun/npx/deno` it also tests the first non-flag, non-`run/exec/x` argument, with and without extension; zombies never match. It answers an empty stdout (gc reads that as alive) when it cannot tell: process table unreadable, launcher not yet at the agent, or no match with an unreadable process in the subtree (`reads.py:110-117,301-302`). Any-of semantics over names.

**Deviations.**

| # | Aspect | Rex provider | herdr-core | Verdict | Rationale |
|---|---|---|---|---|---|
| 1 | match scope | Rex-reported foreground process plus its ancestors (`liveness.go:97-103`) | every process in the box's subtree (`procs.py:608-613`) | intentional | Rex exposes only foreground/ancestors via `process` (`docs/REX-CONTRACT.md:89`); no host process table walk. A background child of the agent is not seen. |
| 2 | interpreter scripts | matches Rex's reported interpreter `script` basename, with and without its extension (`prochost.Candidates` at `internal/prochost/prochost.go:67`, called from `liveness.go:107`, GCREX-47) | matches script arg of node/bun/npx/deno (`procs.py:63,576-582`) | intentional | Rex reports the script path for interpreters (`docs/REX-CONTRACT.md:89`). An agent run as `node agent.js` with hint `agent` now reads alive, matching the pack's with-and-without-extension rule. Confirmed live (GCREX-31): `name` and `argv0` are both `node`, and the script path is only in `script`. |
| 3 | missing pane | `BlockProcess` `ErrNotFound` maps to `false`, nil (`panelive.go:47-61`, GCREX-23) | not applicable; a dead record answers `false` (`reads.py:295-296`) | intentional | Matches `paneAlive` (`liveness.go:130-132`, both through `readPaneLife`, `panelive.go:47`); a pane that vanished mid-call now answers dead instead of exiting 1. |
| 4 | zombie / liveness gate | `LastExit != nil` forces `false` (`liveness.go:91-93`) | zombie never matches (`procs.py:567`); intact record gate (`reads.py:295`) | intentional | Same intent, different primitive: Rex `last_exit` is its liveness truth (GCREX-4). |
| 5 | stdin line splitting | split on `\n` only (`main.go:351`); whitespace trimmed later in `expandProcessNames` | `splitlines()` (also `\r`, `\v`, `\f`, U+2028 etc.) then strip (`reads.py:64-65`) | intentional | gc sends `\n`-separated names; result is identical for that input. All-blank stdin gives alive in both. |
| 6 | matched fields | Rex `name`, `argv0`, `basename(path)`, `basename(argv0)`, and `basename(script)` with/without extension (`prochost.Candidates` at `internal/prochost/prochost.go:67`, called from `liveness.go:107`) | basename of `comm`, basename of `argv[0]`, script arg (`procs.py:569-573`) | intentional | Different process data model. Ours also matches a full-path name equal to `argv0` and the Rex `name` (Rex runs commands under the default shell with `argv0` set; see `docs/plans/native-rex-provider.md:643`). |

**Verdict.** intentional. Output vocabulary, stdin shape, empty-input and any-of semantics, aliases, the `gc`→`GC_BIN` basename rule, and interpreter-script matching now match herdr-core. GCREX-46 closed the cannot-tell deviation: an unreadable process call, an empty process table, or an opaque process with no match now answers empty stdout (exit 0) like herdr-core, so a transient Rex error never reads as dead. GCREX-47 closed the interpreter-script gap by matching Rex's reported `script` basename with and without extension. The foreground/ancestor scope is forced by the Rex `process` API; a process outside that tree is not seen.

**Cross-references.**
GCREX-4 — made `last_exit` the liveness truth and `ProcessAlive` strict with empty ⇒ alive
GCREX-2 — `IsRunning` semantics that `process-alive` sits beside
GCREX-18 — `GC_BIN`/claim identity seeding that the `gc` hint depends on
`docs/REX-RETIRE.md` — the `.gc-wrapped` false-zombie that added the `GC_BIN` basename rule
`docs/REX-CONTRACT.md` §9 (line 211) and `process` method (line 89)
`docs/plans/native-rex-provider.md:643` — why name heuristics are not used for `is-running`

**Follow-ups.**
- Add a test for all-blank stdin and CRLF input in `opProcessAlive`. (gap)


### list-running

**Status** — implemented

**Rex-only extensions.** Rename, move, and alias-set returns are recorded in
[`research/rex-provider-extensions.md`](research/rex-provider-extensions.md) §2.

**Our implementation.** Dispatch is `ops["list-running"]` with 0 required args (`cmd/gc-runtime-rex/main.go:91`); `opListRunning` takes `args[0]` as the prefix, default `""`, and prints one name per line with `Fprintln` (`main.go:260-273`). `Provider.ListRunning` walks the provider's sidecar binding stores (`internal/provider/provider.go:919-...`), which are the canonical per-city state dir plus, for an unpinned caller, the discovered sibling namespaces (GCREX-35), not every Rex session on the server. A binding is skipped unless its name or an alias has the string prefix (`provider.go:506-509,551-562`). It is kept if `BlockProcess` finds the block with no `LastExit`; a missing block is dead and skipped, an unreachable server (`rex.ErrUnavailable`) returns the parked-city empty listing and exit 0, and any other error fails the whole op (`provider.go:652-665`, `liveness.go:105-115`). Names are matched with case-sensitive `strings.HasPrefix`. Aliases are listed only for a non-empty prefix (`provider.go:520-536`). The result is sorted and compacted (`provider.go:538`). Stdin is not read, the exit code is 0 or 1, and empty output is exit 0. There is no stdout cap; names that are empty, padded, or hold a control character are dropped before printing (`readableName`, `provider.go`; GCREX-27), matching herdr-core's `readable_name`. The 30 s deadline comes from gc.

**herdr-core.** Dispatch is `spec(reads.op_list_running, cap=INVENTORY_STDOUT_CAP)` (`herdrcore/ops_table.py:76`). The handler reads `args[0]` as the prefix, default `""` (`herdrcore/reads.py:307-308`). It lists the city's non-finished terminals filtered by the city label (`reads.py:68-73,310`), so it is city-scoped. A name is kept if it starts with the prefix (case-sensitive `str.startswith`), its newest record `is_intact` (live, has a pid, not a broken reason, pid label matches; `records.py:53-62`), and `readable_name` holds (non-empty, untrimmed, no control character; `rpp/wire.py:194-196`). Names are grouped by session label, sorted, and written as `name\n` lines (`reads.py:314-321`). A stopped city (parked, nothing running) gives empty exit 0. Other core read failures exit 1 (`reads.py:311-313`). Output over 8 MiB (`rpp/ops.py:34`) is turned into a failure (`rpp/perop.py:158-159`).

**Deviations.**

| # | Aspect | Rex provider | herdr-core | Verdict | Rationale |
|---|---|---|---|---|---|
| 1 | aliases in output | Non-empty prefix also lists matching aliases; empty prefix never does (`provider.go:520-536`) | Only the one gc session label per record; no aliases | intentional | GCREX-18 alias claim identity; empty-prefix exclusion avoids phantom orphan beads (comment `provider.go:520-523`). |
| 2 | what counts as running | Block exists and `LastExit == nil` (`liveness.go:114`) | `is_intact`: live, pid label matches recorded pid, not broken (`records.py:53-62`) | intentional | Rex has no pid-label reincarnation case; a closed block is dead. Replaced-shell detection is unverified for Rex. |
| 3 | name filter | `readableName` drops empty, padded, or control-char names (`provider.go`; GCREX-27) | `readable_name` drops empty, padded, or control-char names (`wire.py:194`) | intentional | A newline would split one session across two lines and padding would leave a name gc cannot match; both sides now drop it before printing. |
| 4 | stdout cap | None | Fails over 8 MiB (`perop.py:158`) | intentional | gc's `inventory` cap only applies to the `inventory` op (`ops.py:35`); `list-running` is not capped in the RPP v0 baseline. Not needed. Note that gc treats a `list-running` failure as an incomplete inventory, which blocks "stopped" confirmation in `gc stop` (`cmd/gc/cmd_stop.go:861-864`). |
| 5 | stopped/unreachable server | A transport error (`rex.ErrUnavailable`, rex CLI exit 4) returns empty, exit 0; a missing target still skips (`provider.go:653-659`, `internal/rex/cli/cli.go:52`) | Parked city gives empty exit 0 (`reads.py:311-312`) | intentional | GCREX-41: Rex has no park concept, but its "no Rex server is running" exit 4 is the unreachable signal, so `list-running` answers the parked-city inventory and gc still enumerates the fleet instead of aborting on an incomplete inventory. |
| 6 | scope source | Local binding store in city state dir, probed per binding; sessions not bound by this provider are invisible | Core's terminal list filtered by city label, server truth | intentional | Mapping is City→Rex session; bindings are the ownership record. Stale or unrecorded panes are not listed. |

**Verdict.** intentional. Prefix semantics, case sensitivity, sorting, name filtering, and one-name-per-line output match. The differences are alias listing and binding-store scope, which are deliberate. GCREX-27 closed the readable-name guard (row 3), so no gap remains in the printed output.

**Cross-references.**
GCREX-18 — alias/claim identity that drives alias listing and the empty-prefix exception
GCREX-35 — the binding store set an unpinned caller reads
GCREX-2 — start/stop/is-running; shared `paneAlive` liveness
GCREX-4 — identity/metadata; binding store
GCREX-41 — return the parked-city empty listing when Rex is unreachable
GCREX-27 — drop names that are empty, padded, or hold a control character before printing
`docs/REX-CONTRACT.md` §9 — list-running row

**Follow-ups.**
- Document that unbound Rex panes are never listed (gap).


### is-attached

**Status** — implemented

**Rex-only extensions.** The client registry and per-client identity are
recorded in [`research/rex-provider-extensions.md`](research/rex-provider-extensions.md) §1.

**Our implementation.** `is-attached` is in the op table with arity 1 (`cmd/gc-runtime-rex/main.go:189`), handled by `opIsAttached` (`main.go:577`), which prints a bool and exits 0, or returns the error (exit 1). `Provider.IsAttached` (`internal/provider/attach.go:23-60`) resolves the binding via `Binder.Resolve` (revalidates the cached sidecar binding against Rex, `binding.go:270-295`). It then reads the block's `owner` from its own `session.view` reply: each `layers[].blocks[]` entry carries `owner`, the client whose size the terminal uses, empty for a box no client has displayed (`docs/REX-CONTRACT.md` §3; parsed into `rex.Block.Owner` in `internal/rex/cli/cli.go`). An empty owner answers `false` without consulting the client list, so an app client attached to the session no longer counts for a box it never showed (GCREX-108, fixes RPP-OBSERVE-017). A present owner is looked up in `ListClients`: `app` continues, `cli`/`peer` answer `false`, any other kind answers true because an unclassifiable owner may be a viewer and gc reads any is-attached error as not attached (`attach.go:50-58`). An owner no longer in the client list answers true: ownership is a last-requested claim, not a heartbeat, so a stale claim errs toward true rather than closing a pane someone may still be watching. For an app owner it answers true only when the rig tab is the server-side active window and still holds the agent's block (`attach.go:59`, `watchesAgent` `attach.go:80-90`). So "attached" means "an app-kind client owns this pane and its tab is active", not a session-level app presence. The handshake declares `report-attachment`, `report-activity`, `idle-boundary`, `carrier.ops`, `input-outcome`, `observe.v1` and `city-scoped` (`main.go:46`). A missing binding gives `false`; a Rex transport error on `SessionView` or `ListClients` propagates as exit 1; there is no answer cache beyond the binding store. Deadline is the 30 s default; no per-op override found in `main.go` (unverified beyond a grep for Timeout/Deadline).

**herdr-core.** `op_is_attached` (`herdrcore/reads.py:324-334`) returns `false` with no record, otherwise snapshots local processes (`viewer_targets`, `attach.py:68-83`: same-uid `hc attach` argv whose environ matches this city's socket/session) and ORs that with a held `flock` on a per-object attach lock file (`attach.py:86-90`, `lock_held` `attach.py:42-54`). It never asks the server for a client list; evidence is process table plus lock file. A `ProcError`/`LockFileError` becomes `OpError` (exit 1) (`reads.py:333-334`). Op is registered with the `probe` deadline, 4 s (`ops_table.py:77`, `rpp/ops.py:26-30`), a gc v2 observe-time probe timeout, and not the 30 s baseline. The same `attached()` is reused by stop to avoid killing watched sessions, treating "cannot tell" as attached (`lifecycle.py:574-582`).

**Deviations.**

| # | Aspect | Rex provider | herdr-core | Verdict | Rationale |
|---|---|---|---|---|---|
| 1 | attachment source | Rex view block ownership plus client list (`ListClients`) plus `SessionView` | local process snapshot plus attach-lock file | intentional | Rex has a server-side client registry and per-block ownership; herdr has neither. |
| 2 | meaning of "attached" | app-kind client owns the block and the rig tab is the active window | an `hc attach` process or lock targets this terminal | intentional | GCREX-108 makes the pane's own ownership the session half; a session-level app client is not enough. Rex tab tiles several agents, so the tab remains the finest visibility Rex reports. |
| 3 | stale ownership | owner no longer listed answers true, exit 0 | no equivalent | intentional | GCREX-108: ownership is a last-requested claim, not a heartbeat. A claim whose client vanished may still have a human watching, and errs toward true for the same fail-open reason as row 4. |
| 4 | unknown client kind | owner's kind unclassifiable answers true, exit 0 (`attach.go:50-58`) | n/a (no client kinds) | intentional | GCREX-22 closed this mistake. gc reads any is-attached error as `false`, i.e. not attached (`exec/exec.go:690-692`), which is fail-open: config-drift restart, progress-stall recycle and the drain-ack escalation kill would then go ahead (`session_reconciler.go:2844`, `:2974`, `:3375`, `:6122`; `drain_ack_escalation.go:296`). Answering true when the kind is unclassifiable keeps those paths closed. |
| 5 | deadline | 30 s default | `probe` 4 s | intentional | gc 1.5.1 baseline is 30 s for is-attached; 4 s is a v2 observe-probe value. |
| 6 | unreachable backend | Rex transport error returned, exit 1 | proc/lock read error to exit 1; no server involved | intentional | Matches; ours depends on the Rex server, herdr's on `/proc`/ps. Note the same fail-open reading: any error is "not attached" to gc. |
| 7 | stop-safety use | `IsAttached` not consulted by stop (`provider.go:380-405`) | `stop` skips kill when attached or cannot tell (`lifecycle.py:574-582`) | intentional | Verified: gc never calls `is-attached` before the `stop` op (`exec/exec.go:581-584`; `city_runtime.go:4199-4235`). Its attachment gates sit in the reconciler and escalation paths, before gc decides to stop (`session_reconciler.go:6122`, `:6304`; `drain_ack_escalation.go:296`). herdr's stop-side protection is an extra safeguard, not a gc requirement. |

**Verdict.** intentional. The attachment source differs because Rex has a real client registry and per-block ownership, and `report-attachment` is declared with a meaningful answer. GCREX-108 corrected the session half: the pane's own `owner` (an app client) decides, so a running box the app never displayed answers `false` instead of the old session-level `true`; the tab half still requires the rig tab to be active. An unclassifiable owner kind answers true (GCREX-22), and a stale owner whose client vanished also answers true because ownership is a last-requested claim, not a heartbeat; both keep gc's fail-open reading from closing a watched pane. The Rex-unreachable case still exits 1 and gc still reads that as not attached, which is the only honest answer when the server is down. Verified in gc 1.5.1: `is-attached` is only invoked when `report-attachment` is declared (`exec/exec.go:684-691`), the op's output must be exactly `true`, and any error becomes `false`. Only the unverified stop-safety use (row 7) is a possible gap.

**Cross-references.**
GCREX-4 — identity/metadata/activity/attachment phase; defines `IsAttached` and `report-attachment`
GCREX-5 — RPP v0 delivery, handshake declaration
docs/REX-CONTRACT.md §9 — `is-attached` row, `report-attachment` capability

**Follow-ups.**
- Optional hardening: have `Stop` warn or refuse when `IsAttached` is true; a plain `gc stop` or city shutdown is not gated by gc and would kill an attached Rex pane (gap).
- Confirm no per-op deadline override exists in `main.go` and that 30 s is enforced (gap).


### get-last-activity

**Status** — implemented

**Our implementation.** Dispatched at `cmd/gc-runtime-rex/main.go:517` (1 arg, no stdin) to `opGetLastActivity`, which calls `Provider.GetLastActivity` (`internal/provider/activity.go:53-95`). It resolves the binding; no binding returns zero time with source `none`. Otherwise it reads the block's `program_status` records and takes the newest (`newestProgramStatus`, `activity.go:99-109`); when that newest state is `working` or `blocked` it reports `time.Now().UTC()` (`activity.go:68-70`). A reportable record otherwise uses its `UpdatedAt`. With no records it hashes `block format` output (sha256) and compares it with the stored fingerprint, which is keyed by block id (`activity.go:75-94`). If the fingerprint is unchanged it returns the stored time. If it changed, or is unseen, it stamps `time.Now().UTC()` and saves it (`stores.go:18-27, 95-109`, one JSON file per name). Every returned time is raised to the recorded pack-input floor `ActivityRecord.InputAt` (`activity.go:114-120`) when the record's identity is the resolved block; `noteInput` sets that floor after a successful nudge write (`nudge.go:153`) and a successful send-keys (`provider.go:728`). The handler prints `At.UTC().Format(time.RFC3339)` plus newline (whole seconds, `Z`) only when `At` is non-zero. Unknown or missing session prints nothing and exits 0 (`dispatch_test.go:257-286`); `activity_test.go` covers the working/blocked rule, the input floor and the unknown case. Rex errors exit 1. Capabilities `report-activity` and `idle-boundary` are declared (`main.go:46`). Verified in gc 1.5.1: `get-last-activity` is called regardless of `report-activity` (`exec/exec.go:838-852`); the capability only sets `CanReportActivity`, which feeds sleep/idle and nudge decisions.

**herdr-core.** `op_get_last_activity` (`herdrcore/reads.py:336-351`) returns `OpResult.ok()` (no stdout) when the record is missing or not intact. For a pane that never reported OSC 7501 it reads the screen (a read failure raises `OpError`, exit 1) and calls `activity.note_screen`. `note_screen` (`activity.py:94-104`) hashes the screen with sha256 and keeps hash and `changed_ms` per incarnation in `activity.json`, under an exclusive flock with a 0.5 s wait (`activity.py:74-85`). `last_activity_ms` (`activity.py:126-137`) returns now when the status is `working` or `blocked`. A reported pane gets `max(status_ms or created_ms, input_ms)`. An unreported pane gets `max(changed_ms, input_ms)`, or now if its screen was not read. `input_ms` is set by `note_input` whenever the pack types into the box (`activity.py:88-91`). Output goes through `rfc3339_ms` (`rpp/wire.py:298-304`): UTC with milliseconds, `.mmmZ`. `report-activity` is declared (`handshake.py:37`).

**Deviations.**

| # | Aspect | Rex provider | herdr-core | Verdict | Rationale |
|---|---|---|---|---|---|
| 1 | timestamp format | `RFC3339`, whole seconds (`main.go:280`) | RFC3339 UTC with ms (`wire.py:298-304`) | intentional | The gc RPP v0 contract takes RFC3339. Both are UTC `Z`. Millisecond precision is not required. |
| 2 | active program (`working`/`blocked`) | Returns now when the newest state is `working`/`blocked` (`activity.go:68-70`); GCREX-24 | Returns now for `working`/`blocked` (`activity.py:129-131`) | intentional | GCREX-24 closed the mistake. Confirmed in gc 1.5.1: it idle-stops or sleeps on `now - last_activity > timeout` (`idle_tracker.go:159-163`), and the same value gates drain-escalation kills (`drain_ack_escalation.go:303-306`), progress-stall recycle (`session_reconciler.go:2967-2972`) and the claim backstop (`execution_backstop.go:279-282`); before the fix a stale value from a silent but busy Rex agent was read as idle. herdr-core made this call explicitly (`activity.py:12-17`). |
| 3 | input time | Records `InputAt` on a successful nudge write or send-keys and returns `max(base, InputAt)` (`activity.go:114-133`, `nudge.go:153`, `provider.go:728`); GCREX-24 | Takes `max(…, input_ms)` from `note_input` (`activity.py:88-91, 132-137`) | intentional | GCREX-24 closed the mistake. Before the fix, after gc typed into a pane that showed no output change yet the time stayed old and the idle/escalation paths read it as idle. The floor is ignored when the record's identity is not the resolved block. |
| 4 | state locking | `sync.Mutex` in-process only. Last-writer-wins file per name (`stores.go:98-102`). Write errors are ignored | flock on `activity` with a 0.5 s wait, falls back to a non-persisted entry on timeout (`activity.py:74-85`) | intentional | One process per op. Concurrent calls for the same name can race, but the race only moves the stamp by the interval between the calls. Low impact. Unverified whether `GetLastActivity` is ever called concurrently. |
| 5 | error on Rex failure for a reported pane | Fails with exit 1 if `program_status` errors (`activity.go:49-52`) | Reads cached `activity.json` and never queries the host for reported panes (`reads.py:348-349`) | intentional | The source of "reported" differs. herdr-core gets status from the carrier record. We query Rex per call. gc parses the answer with `time.RFC3339` (`exec/exec.go:846`), discards a read error (`exec/seams.go:230`) and treats zero as unknown, never idle, so failing is the safe direction. |

**Verdict.** intentional. The sources differ (Rex `program_status` and `block format` against hc records and screen reads) and the fallback logic is the same (hash the screen, keep when it changed). GCREX-24 added herdr-core's "working/blocked is active now" rule and the pack-input floor, so a silent but busy agent no longer looks idle.

**Cross-references.**
GCREX-4 — implemented `GetLastActivity`, the sidecar, and the OSC 7501 policy (`docs/REX-CONTRACT.md:448-456`)
GCREX-16 — consuming `program_status` for nudge gating, which shares the record source
GCREX-5 — handshake declares `report-activity`
GCREX-6 — pilot uses idle detection
GCREX-17 — session resolution that feeds `binder.Resolve`
`docs/plans/native-rex-provider.md` §6 (activity) and the capability table (lines 397-428)

**Follow-ups.**
- Decide whether millisecond RFC3339 is worth emitting (gap)
- Re-verify the working/blocked rule and the input floor against gc's idle tracker if the provider stops querying Rex per call (gap)


### peek

**Status** — implemented

**Rex-only extensions.** Paged capture and the `format`/`blob_read` split are
recorded in [`research/rex-provider-extensions.md`](research/rex-provider-extensions.md) §4.

**Our implementation.** Dispatched as `peek <name> [lines]` with one required arg (`cmd/gc-runtime-rex/main.go:185`); `run` buffers stdout and flushes it only on success (`main.go:255-275`). `opPeek` (`main.go:578-596`) reads no stdin, treats an absent or empty `lines` as 0, and fails exit 1 on a non-integer (`strconv.Atoi`, `main.go:580-586`). `Provider.PeekLines` (`internal/provider/provider.go:892-926`) resolves the binding (for an unpinned caller through the discovered sibling namespaces, GCREX-35), then calls `BlockFormat`; `lines <= 0` returns the whole capture, otherwise `lastLines` (`provider.go:928-939`) keeps the last N newline-split rows and preserves whether the input ended in `\n`. `BlockFormat` (`internal/rex/cli/cli.go:327-357`) calls Rex `format` with `{format: text, trim: true, max_bytes: 983040}` and follows `id` through `blob_read` until `eof`, so there is no output cap beyond what Rex holds (`cli.go:310-313`, `docs/REX-CONTRACT.md:90-91,408`). A session with no screen to read answers exit 66 with `no screen for '<name>'` on stderr and empty stdout: `PeekLines` returns `provider.ErrNoScreen` for a name with no binding (`Resolve` returns `ok=false`), for `rex.ErrNotFound` from `Resolve` or `BlockFormat` (session or pane vanished mid-call, GCREX-23), and for `rex.ErrUnavailable` from `Resolve` or `BlockFormat`; `opPeek` maps it to `exitError{code: exitNoScreen}` (`main.go:296`, `:508-510`), and `run` writes the message to stderr and no stdout (`main.go:265-268`). `PeekLines` (`provider.go:892-926`) also reads `BlockProcess` once, through `readPaneLife` and `peekErr` (`panelive.go:47,138`), after `Resolve` to learn whether the box is dead (a non-nil `LastExit`): `ErrNotFound`/`ErrUnavailable` there is exit 66 like the `format` call, and any other error stays exit 1. A dead box's capture that wrote nothing is exit 1 `peek <name>: the dead box's capture wrote nothing` (`provider.go:917-919`, re-read by `exitedAfterEmptyCapture`, `panelive.go:156`), never a blank screen: RPP-OBSERVE-016 reads exit 0 with no output as nothing to classify why the box died. A running box that has not rendered yet answers exit 0 with an empty screen, because its blank screen is a legitimate capture. Pinned by `TestPeekDeadBoxWithScreen`, `TestPeekDeadBoxEmptyCaptureIsAFailure` and `TestPeekRunningBlankScreenSucceeds` (`provider_test.go:270-341`), and at op level by `TestObserveV1PeekReadsADeadBox`, `TestObserveV1PeekDeadBoxEmptyCaptureIsAFailure`, `TestObserveV1PeekRunningBlankScreenSucceeds` and `TestObserveV1PeekNoScreenAfterStop` (`dispatch_test.go:354-416`). The older no-screen cases are pinned by `TestPeekNoScreenWhenNoRecord`, `TestPeekNoScreenWhenPaneVanished` and `TestPeekNoScreenWhenRexUnavailable` (`dispatch_test.go:236-262`); the exit-66 choice is recorded in `backlog/decisions/decision-2`. All other errors (a `SessionView` or `BlockFormat` failure that is not unavailable/not-found, a non-integer `lines`) stay exit 1 with the message on stderr (`main.go:269-270`). No deadline is applied in-process (`signal.NotifyContext` over `context.Background()`, `main.go:108`).

**herdr-core.** `peek` is registered with `cap=OBSERVE_STDOUT_CAP` (1 MiB) and no stdin (`herdrcore/ops_table.py:79`, `rpp/ops.py:33`). `op_peek` (`herdrcore/reads.py:357-378`) defaults `lines` to `"0"`, fails exit 1 on `int()` ValueError (an empty string also fails), and returns exit 66 (`NO_SCREEN`) with `no screen for 'name'` when no record exists or hc answers `not_found` (`reads.py:364-371`; `rpp/exitcodes.py:21-22`; tests `test_herdrcore_lifecycle.py:148-149`). It reads the screen only (`terminal.read` with `lines: 0`, `hc/client.py:250-254`), splits on `\n`, keeps `rows[-lines:]` when `lines > 0`, and always joins with `\n` plus one trailing `\n`, encoded with `surrogateescape` (`reads.py:375-378`). An answer over the cap is replaced by exit 1 with an "over the cap" message and no stdout (`rpp/perop.py:158-159`). A core that cannot be reached is exit 1 `cannot read the city's core` (`reads.py:272-278`), except for a parked city or one with nothing running, where `current_record` returns `None` and the result is exit 66 (`reads.py:275-276`, via `parked`/`nothing_runs`). Other hc failures are exit 1 `cannot read the screen` (`reads.py:372-374`).

**Deviations.**

| # | Aspect | Rex provider | herdr-core | Verdict | Rationale |
|---|---|---|---|---|---|
| 1 | `lines` empty-string arg | `""` is treated as 0, so the whole capture (`main.go:245`) | `""` fails exit 1 (`reads.py:359-363`) | intentional | gc always sends a decimal integer (`exec/exec.go:622`), so an empty arg is unreachable from gc; harmless if it happens. The dispatcher already rejects an empty name (`main.go:124`). |
| 2 | `lines` > 0 row counting and trailing newline | Strips one trailing `\n` before counting rows, then restores it only if the capture had one (`provider.go:471-479`); empty capture returns `""` | Splits the raw text, so a trailing `\n` adds an empty last row that counts toward N; always appends `\n`, so an empty screen returns `\n` (`reads.py:375-378`) | intentional | Ours returns exactly N text rows. gc strips every trailing `\n` from the result (`exec/exec.go:104`), so trailing-newline differences are invisible to it. |
| 3 | scrollback vs viewport | Rex `format` of the block, described as "bounded by terminal scrollback" (`backlog/tasks/gcrex-2…Peek.md` final notes, line 73), not a viewport-only read | Screen only, and scrolling is avoided because it sends input to full-screen programs (`hc/client.py:250-252`) | intentional | The Rex `format` method is non-destructive. GCREX-31 confirmed live that it returns scrollback plus the viewport: a 24-row pane with 200 numbered lines returned 206 rows, including the first line. |
| 4 | output cap | Pages to completion at 983040 bytes per reply, with no total cap (`cli.go:310-357`) | Fails exit 1 with no stdout above 1 MiB (`perop.py:158-159`, `ops.py:33`) | intentional | The cap is herdr's `observe.v1` contract. Verified: gc 1.5.1 has no cap and no truncation, it buffers stdout whole and strips only trailing `\n` (`exec/exec.go:95`, `:104`). The 30 s op timeout is the only bound. A large scrollback is multi-MiB for us (`e2e_test.go` reads ~4 MiB), which is fine. GCREX-31 confirmed that a 1 217 315-byte capture paged to `eof` at 983040 bytes per reply, and answered `too_large` without `max_bytes`. |
| 5 | unreachable core / server | exit 66 `no screen for …` for `rex.ErrNotFound` or `rex.ErrUnavailable` from `Resolve` or `BlockFormat`; exit 1 with the Rex error text for any other failure (`provider.go:775-784`, `main.go:508-510`) | exit 1 `cannot read the city's core`, or exit 66 when the city is parked or nothing runs (`reads.py:272-278`) | intentional | Decision-2: every "no screen to read" cause answers 66, including an unreachable Rex, which herdr-core answers as exit 1 except for a parked city. Our provider has no parked-city state. gc v1.5.1 reads exit 66 as an ordinary error (`exec/exec.go:153-170`), so a peek that races an unreachable Rex fails instead of returning empty; the vanished-session case is mostly guarded because `RuntimeHandle.Peek` checks `IsRunning` first (`runtime_handle.go:187-190`). Revisit when gc handles exit 66 (upstream, no overlay). Pinned by `TestPeekNoScreenWhenRexUnavailable`. |
| 6 | dead box with a blank capture | exit 1 for a dead box whose `format` wrote nothing (`provider.go:1042-1047`); a running box's blank screen is exit 0 with empty stdout | exit 66 only when no record exists; a core that cannot answer is exit 1 (`reads.py:364-374`) | intentional | Rule 13 of RPP-OBSERVE-016: a dead box is peeked to classify why it died, so an empty capture of a dead box is unreadable, not a blank screen. A dead box that did render answers exit 0 with its screen. The cost is one extra `BlockProcess` read per peek. |

**Verdict.** intentional. The `peek` contract is the same on the happy path: whole output, or the last N rows, to stdout on exit 0. A missing session and a pane that vanishes mid-call now answer exit 66 like herdr-core (GCREX-48), so they are no longer deviations. gc v1.5.1 has no exit-66 handling and reads it as an ordinary error (`exec/exec.go:153-170`. The `peek` error in `runtime/dialog.go:558-560` is returned at once), so on a race that `runtime_handle.go:187-190` does not guard, a peek fails where it used to return empty (`backlog/decisions/decision-2`). The remaining deviation is our lack of a parked-city state. GCREX-31 closed rows 3 and 4: `format` covers scrollback plus the viewport, and the paged read reaches `eof` on a multi-MiB capture.

**Cross-references.**
GCREX-2 — built Peek and paged `format` following; notes that Peek is bounded by terminal scrollback.
GCREX-5 — RPP v0 delivery; peek is a driving op, not a conformance requirement when `exec` exists (`docs/REX-CONTRACT.md:225-229`).
GCREX-8 — aligned the plan with the pinned Rex contract (`format` / `blob_read`, §9).
GCREX-17 — name resolution across the candidate set that `Resolve` uses.
GCREX-35 — an unpinned caller discovers the pinned sibling namespaces to resolve the binding.
`docs/REX-CONTRACT.md` §9 (`format`/`blob_read` rows, line 90-91; RPP table, line 216) and §12 (line 408).

**Follow-ups.**
- Add a dispatch test for an empty `lines` arg (gap).
- Recorded in GCREX-31: `rex api call block.method format` on a pane with history returned 206 rows, including `SCROLL-1`, so it returns scrollback plus the viewport (`docs/REX-CONTRACT.md` §10.1).
- Keep answering exit 2 to `exec`: gc reads exit 1 from `exec` as the command's result with no fallback (`exec/exec.go:906-908`), which would break the driving-op fallback (gap).


### nudge

**Status** — implemented

**Our implementation.** Dispatch is `ops["nudge"]` with 1 required arg (`cmd/gc-runtime-rex/main.go:86`). `opNudge` reads stdin and calls `Provider.NudgeReport`. The provider rejects invalid UTF-8 (`checkNudgeText`, `nudge.go:233`) and classifies a missing or dead pane as `gone` (`resolveInputPane` → `classifyInput`, `input.go:134`). `handlerExitCode` only renders the `*InputOutcomeError` (`main.go:318`). A delivered-but-unconfirmed report exits 0 with a `delivered_unobserved` line (`main.go:432-434`) instead of only writing the diagnostic. `Nudge` returns nil for an empty message before touching Rex. Otherwise `livePane` resolves the binding and probes `BlockProcess`; no binding, a missing block, or `LastExit != nil` gives `ErrSessionNotFound` (`provider.go:614-634`, `liveness.go:105-115`). `NudgeReport` first refuses a fallback pool-claim prompt while the session holds a live claim (`ErrNudgeClaimHeld`, `nudge.go:354-360`; GCREX-69), typing nothing and answering exit 80 `busy`, then reads `program_status` and refuses a `working` pane with `ErrNudgeBusy` or a `blocked` pane with `ErrNudgeBlocked` (kind named), typing nothing, and answers exit 80 `busy`/`pending_interaction` (`internal/provider/nudge.go:356-372`; GCREX-45); `NudgeModeImmediate` bypasses that gate (`nudge.go:357`). It captures a format fingerprint when no status record exists, then writes the raw text (`block.method write {data}`) and sends `rex send-key … -- enter` (`nudge.go:379-413`, `internal/rex/cli/cli.go:356-370`). A successful write also records the input floor (`noteInput`, `nudge.go:402`), so the delivery is visible to `get-last-activity` before the program's output renders (GCREX-24). One submit deadline covers both calls: the op context's earlier deadline, else 10 s (`nudge.go:474-485`). After a successful write the Enter is attempted on a context detached from parent cancellation but still bounded by that deadline (`nudge.go:406-412`), so a cancel cannot leave text typed and unsubmitted. A failed write or Enter answers exit 80 `unavailable` when the server is unreachable and `not_sent` otherwise (`nudge.go:394-413`, `nudge.go:322-341`; GCREX-45); a failed Enter still stores `GC_NUDGE_SUBMIT_FAILED` in the session meta, and the next successful submit clears it (`nudge.go:410-414`). After the submit it confirms a turn start (program_status idle→working or newer `updated_at`, else a format-fingerprint change) within the bounded confirmation window; a delivered-but-unconfirmed nudge is a report, not an error, so gc does not retype it, and the dispatcher emits one `delivered_unobserved` line on exit 0 (`nudge.go:418-467`, `main.go:322-333`). An untyped failure (a Rex exec error before the send) still exits 1 with stderr text (`main.go:185`). The process uses `context.Background()`, so gc's 30 s kill is the outer bound; the provider has its own shorter submit budget. The only stdout a nudge writes is the `delivered_unobserved` line on a success, or one exit-80 `{"outcome":…}` line on a rejected, busy, pending-interaction, unavailable or not-sent failure (`main.go:176-183`).

**herdr-core.** Dispatch is `spec(inputs.op_nudge, stdin=True)` (`herdrcore/ops_table.py:83`). `op_nudge` UTF-8-decodes stdin and answers `rejected` (exit 80) on bad bytes without typing (`herdrcore/inputs.py:342-348`). `nudge` lists the city's terminals and takes the newest record; a missing or non-intact record is `gone` and a core that cannot be reached is `unavailable` (`inputs.py:209-229,269-274`). It then calls `hc terminal.send` once, which types and submits in one request (`hc/client.py:256-262`). The send gets all the op's remaining time minus a 5 s reserve (`inputs.py:162-165`, `hc/client.py:259-262`). Busy and blocked gating happens in the hc core, and the pack maps its refusals: `status_mismatch` → `busy`, `blocked` → `pending_interaction`, `unavailable` → `not_sent`, `not_found`/`finished` → `gone` (`inputs.py:61-66,197-206`). A send hc took whole is `delivered`, or `delivered_unobserved` when the program reports status but was not seen starting the turn (`inputs.py:238-241`). A send that hc may have typed but whose reply was lost is also `delivered_unobserved` (`inputs.py:260-264`). A non-delivery is exit 80 with one `{"outcome":…}` line (`rpp/outcome.py:103-115`); only unknown outcomes and unsupported-core refusals exit 1. The op notes the input time for activity (`inputs.py:232-235`, `activity.py:88-91`). It does not retry `unavailable`; only the startup nudge does (`retry_unavailable=True`, `herdrcore/lifecycle.py:1129`, `inputs.py:256-258`).

**Deviations.**

| # | Aspect | Rex provider | herdr-core | Verdict | Rationale |
|---|---|---|---|---|---|
| 1 | busy / program_status gating | Refuses a fallback pool-claim prompt while a claim is live (`ErrNudgeClaimHeld`, `nudge.go:354-360`); otherwise reads `program_status` first; refuses `working` with `ErrNudgeBusy` and `blocked` with `ErrNudgeBlocked` (kind named), writing nothing, and answers exit 80 `busy`/`pending_interaction` (`nudge.go:362-388`); `NudgeModeImmediate` overrides (`nudge.go:209-220`) | hc refuses on status: `busy`, `pending_interaction` (`inputs.py:61-66`) | intentional | GCREX-16 implemented the gate; GCREX-45 added the typed words; GCREX-69 answered the pool-claim refusal as `busy` so gc's backstop refunds it. gc 1.5.1 reads only the exit code; a gc declaring `input-outcome` reads the word. |
| 2 | submit confirmation | Polls `program_status` for idle→working or a newer `updated_at` within a bounded window, with a format-fingerprint fallback; an unconfirmed delivery is a report, not an error, and exits 0 with a `delivered_unobserved` line (`nudge.go:418-467`, `main.go:322-333`) | `started` flag from hc gives `delivered` vs `delivered_unobserved` (`inputs.py:238-241`) | intentional | GCREX-16 implemented; GCREX-45 emits herdr's success line. Verified in gc 1.5.1: no outcome vocabulary exists and stdout is discarded (`exec.go:613-638`); only the exit code is read, so not erroring is what prevents a retype. |
| 3 | exit code when the pane or session is gone | Exit 80 `gone` (`main.go:278-281`) | Exit 80 `gone` (`inputs.py:213-228`, `outcome.py:114-115`) | matches | GCREX-70 declared `input-outcome` with `carrier.ops`, so the installed edge build projects `gone` to `ErrSessionNotFound`; gc 1.5.1 reads the same exit 80 as a plain error, where the old best-effort wrapper exited 0. |
| 4 | exit code when the server is down | Exit 80 `unavailable` (`nudge.go:322-341`, GCREX-45). Nothing is typed on a pre-send lookup failure, but a write or submit failure after the binding resolved has already typed the text. A non-3 `rex` exit is not `ErrNotFound` (`cli.go:41-62`). GCREX-31 confirmed that a down server exits 4 | Exit 80 `unavailable`, nothing typed (`inputs.py:192-194,224`) | intentional | GCREX-45 aligned the word. For gc 1.5.1 both are a plain failure. GCREX-31 confirmed that `internal/rex/cli/cli.go` maps only exit 3 to `ErrNotFound`, and a transport failure exits 4, so a transport failure never reads as `gone`. The lost-reply case stays exit 1. |
| 5 | framing: text and submit | Two non-atomic calls under one shared submit deadline: raw `write`, then `enter`. A post-write Enter failure exits 80 `not_sent` (or `unavailable` on an unreachable server) with "text was typed but submit key failed" and stores `GC_NUDGE_SUBMIT_FAILED` (`nudge.go:409-415`, `input_test.go`; GCREX-45). | One atomic `terminal.send` type+submit (`hc/client.py:256-262`) | intentional | Rex has no atomic type-and-submit primitive (`REX-CONTRACT.md:143` defines nudge as write+Enter). gc retries a failed nudge from the queue (15 s delay, 5 attempts) or the idle backstop (3 attempts), re-typing the whole text each time (`cmd/gc/cmd_nudge.go:44-45`, `:1917-1952`); GCREX-21 bounds the split and makes the typed-but-unsubmitted case distinguishable in stderr and meta. |
| 6 | multi-line and control characters | Verified live (GCREX-21): a nudge of `line one\nline two` into an idle Pi 1.5.2 pane arrived as one submitted turn; both lines were in the same message. `write` inserts the newline without submitting; the separate Enter submits the whole text. `block.event paste` remains unused. | Passed as one JSON string to hc `terminal.send` (`inputs.py:254`); hc's paste/newline handling is unverified | intentional | Live probe on Rex revision 14, 2026-11-18 (recorded in `REX-CONTRACT.md` §6). One observation; control bytes other than newline were not re-tested. |
| 7 | empty message | Returns nil before resolving the pane, so exit 0 even for a dead pane (`provider.go:564-566`) | Empty text goes through `_target` and `client.send` like any text (`inputs.py:269-274`); hc's reaction is unverified | intentional | Typing nothing and pressing Enter would submit an empty prompt. No-op is safer. Only differs for a dead pane (exit 0 vs 80). |
| 8 | non-UTF-8 stdin | `checkNudgeText` (`nudge.go:233`) validates the text before any Rex call, in `NudgeReport` and again in `nudgeReportBinding`, so nothing is typed and `json.Marshal` cannot rewrite the bytes | Rejected, nothing typed (`inputs.py:344-348`) | intentional | GCREX-21 rejected before typing; GCREX-45 changed the exit to herdr's 80. gc 1.5.1 reads either non-zero as failure. |
| 9 | deadline and cancellation | One submit deadline covers write and Enter: the op context's earlier deadline, else 10 s (`nudge.go:218-231`). After a successful write the Enter runs on a context detached from parent cancellation but bounded by the same deadline; an already-cancelled context writes nothing (`nudge.go:103-108,146-163`) | Send waits until deadline minus a 5 s reserve so the answer always reaches gc (`inputs.py:162-165`) | intentional | GCREX-21 closed the mistake: a cancel or unbounded stall can no longer split the two steps, and an entered submit still reports the typed-but-unsubmitted case. |
| 10 | retries / settle time | None; no pause between write and Enter | None for the nudge op (only the startup nudge retries `unavailable`, `lifecycle.py:1129`) | intentional | Behavior matches at the provider level: gc never retries the call itself, and it retries from the queue only on error. The multi-line Pi probe now shows a complete write is ingested before the separate Enter; other TUI ingest races remain unverified. |
| 11 | activity bookkeeping | Records input time after a successful write (`noteInput`, `nudge.go:153`) | `note_input` stamps input time (`inputs.py:232-235,263-265`) | intentional | GCREX-24 closed the gap: the input floor keeps a typed-but-unrendered nudge visible to `get-last-activity` before the program's output renders. |
| 12 | pool-claim refusal | A fallback pool-claim prompt sent while the seat holds a live claim is refused without typing and answered exit 80 `busy` (`ErrNudgeClaimHeld`, `nudge.go:354-360`; GCREX-69) | no such precondition; the doc has none | intentional | Rex extension (GCREX-33). At gc `890a58a3e` / `544beb8d6` (#9137 only, not in `v1.5.1`; inert at the pin) the `busy` word makes the stalled-claim backstop refund the reservation instead of re-dispatching the seat's claim; a failed-delivery word would let it abandon in-flight work. At `v1.5.1` exit 80 is a plain error and the attempt is spent, like the old exit 1 — no regression. |

**Verdict.** intentional. The fire-and-forget model matches herdr-core, and a gone pane answers the same typed exit 80 `gone` (row 3, `matches`) since GCREX-70 declared `input-outcome` with `carrier.ops`; gc 1.5.1 reads it as a plain error instead of the old exit 0. GCREX-16 closed the `program_status` gating and confirmation gaps; GCREX-21 closed non-UTF-8 input, bounded the write→Enter window, made a post-write Enter failure distinguishable, and verified multi-line delivery into Pi; GCREX-24 closed the activity-bookkeeping gap (row 11); GCREX-45 replaced the exit 1 the failures shared with herdr's typed exit-80 words and emits the `delivered_unobserved` success line (rows 1, 2, 4, 5, 8); GCREX-69 answers the live-claim refusal as exit 80 `busy`, so a consuming gc reads the word to refund a busy nudge instead of spending a delivery attempt (row 12).

**Cross-references.**
GCREX-3 — original input path: write text then Enter, `livePane` → `ErrSessionNotFound`
GCREX-16 — `program_status` busy gating and submit confirmation; closes rows 1, 2
GCREX-21 — non-UTF-8 rejection, bounded write→Enter submit, distinguishable post-write Enter failure; closes rows 5, 6, 8, 9
GCREX-45 — typed exit-80 nudge outcomes (`busy`, `pending_interaction`, `unavailable`, `not_sent`, `rejected`) and the `delivered_unobserved` success line; aligns rows 1, 2, 4, 5, 8
GCREX-69 — pool-claim refusal answered as `busy` and the handshake declares `input-outcome`; closes rows 1, 12
GCREX-24 — records the pack-input floor after a successful write; closes row 11 and feeds `get-last-activity`
GCREX-14 — startup nudge: write+Enter, best effort; GCREX-34 adds the readiness wait, submit confirmation and a bounded resend (`provider.go:455-572`)
GCREX-4 — deferred nudge gating; `activity.go` already consumes `program_status`
`docs/REX-CONTRACT.md` §6 (`write`, nudge = write + Enter), §9 (nudge row, driving ops not conformance)
`docs/rex-provider.md` — Known v1 Limits, "No busy-gated nudge"

**Follow-ups.**
- Probe control characters other than newline (ESC, Tab) through `write` into Pi; the multi-line path is verified, the control-byte path is not (gap).
- Check what `rex` exits with when the server is down; make sure it is not exit 3, so a transport failure is not misread as `gone` or a plain exit 1 (gap).


### send-keys

**Status** — implemented

**Our implementation.** Dispatched at `cmd/gc-runtime-rex/main.go:299` (min 1 arg) to `opSendKeys` (`main.go:299-305`). It ignores stdin. Zero keys returns nil before any lookup (`main.go:300-303`, `internal/provider/provider.go:711-713`). Otherwise `Provider.SendKeys` runs the key grammar before the pane lookup (`provider.go:710-717`), then resolves the live pane (`provider.go:718-721`) and calls `rex send-key -s … -b … -- <keys…>` (`provider.go:722-725`, `internal/rex/cli/cli.go:363-370`). `translateKeys` (`internal/provider/keymap.go:64-78`) translates every key before any Rex call via `translateKey` (`keymap.go:80-100`), mirroring herdr-core's grammar (GCREX-43): an exact-case table (`tmuxKeys`, `keymap.go:30-54`) including BTab→`shift+tab`, PPage/NPage and F1–F12; `C-`/`M-`/`S-` prefixes while more than one character remains, on a table name or a single literal character, kept in typed order (`peelMods`, `keymap.go:136-156`); and literal text typed character by character (lowercase, digits, space→`space`, uppercase→`shift+<lower>`, `literalKeys` `keymap.go:177-193`). A key with no Rex spelling rejects the whole call with a `KeyRejectedError` (`keymap.go:11-27`). `SendKeys` turns it into a `rejected` `*InputOutcomeError` (`classifyInput` at `stageValidate`, `input.go:84`) before the lookup. A missing binding or dead pane is classified by `classifyInput` at the lookup stage (`input.go:84`) as a typed `gone` outcome. An unreachable server is `unavailable`, and another pre-send failure is `not_sent`. A send failure after a possible write stays exit 1 (outcome unknown) (GCREX-42). `deliverInput` (`input.go:154`) resolves through `resolveInputPane`, runs the #48 guard and the send, and stamps the floor per `inputPolicies[send-keys]` (`input.go:67`). The dispatcher only renders the outcome. GCREX-42 typed the word and GCREX-70 dropped the baseline `bestEffort` wrapper when the handshake declared `input-outcome`, which the installed edge build reads as `ErrSessionNotFound` and gc 1.5.1 reads as a plain error. A successful send records the pack-input floor (`noteInput`, `provider.go:727`) for `get-last-activity` (GCREX-24). Success leaves stdout empty and exits 0; a rejected key exits 80; any other Rex error exits 1. The deadline is the default 30 s.

**herdr-core.** `op_send_keys` (`herdrcore/inputs.py:351-352`, table at `ops_table.py:84`) passes `args[1:]` to `send_keys` (`inputs.py:284-301`) and ignores stdin. `translate_keys` (`inputs.py:151-160`) runs before any lookup. `translate_key` (`inputs.py:123-148`) uses an exact-case table `TMUX_KEYS` (`inputs.py:70-97`: Enter, Escape/Esc, Tab, BTab, BSpace, Space, arrows, Home/End, PageUp/PgUp/PPage, PageDown/PgDn/NPage, IC/Insert, DC/Delete, F1–F12). It also handles `C-`/`M-`/`S-` prefixes on a table name or a single character, and treats any other string as literal text typed char by char (lowercase, digits, a few punctuation chars; uppercase becomes `shift+x`; space becomes `space`) (`inputs.py:99-148`). A key with no hc spelling (empty string, unsupported char, bad modifier base) rejects the whole call as `rejected` before anything is sent (`inputs.py:285-287`). The outcome is exit 0 when delivered, otherwise exit 80 plus one outcome line, or exit 1 when the result is unknown (`inputs.py:181-185`, `rpp/outcome.py:103-115`). A gone box (no record, or not intact) answers `gone` (exit 80), not success (`inputs.py:209-229`). Zero keys with a live box is `delivered` (`inputs.py:293-294`). A written-but-unanswered request is never retried and exits 1 (`inputs.py:274-281`).

**Deviations.**

| # | Aspect | Rex provider | herdr-core | Verdict | Rationale |
|---|---|---|---|---|---|
| 1 | unknown key names | translated char by char as literal text, or `rejected` exit 80 if untypable (`keymap.go`, `main.go`) | translated char by char as literal text, or `rejected` exit 80 if untypable (`inputs.py:140-148`, `285-287`) | intentional | GCREX-43 implemented the grammar. Verified in gc 1.5.1: `send-keys` carries only tmux key names, never literal text (`exec.go:626`, `dialog.go`, `submit.go`), so gc never depends on the literal path. |
| 2 | key vocabulary / case | exact-case table incl. BTab, PPage, NPage, F1–F12 (`keymap.go`) | exact-case table incl. BTab, PPage, NPage, F1–F12 (`inputs.py:70-97`) | intentional | GCREX-43 aligned the table to herdr-core. gc 1.5.1 emits only `Enter`, `Up`, `Down`, `Escape` and `C-u` through `send-keys`, all covered; `C-c` goes through `interrupt`. |
| 3 | modifier handling | prefix only on a table name or a single literal char, in typed order (`keymap.go`) | prefix only on table name or single char; else reject (`inputs.py:132-139`) | intentional | GCREX-43 aligned to herdr-core. Rex still validates the final binding, so an unspellable key is now rejected earlier with exit 80. |
| 4 | missing/dead session | typed `gone` via `classifyInput` at the lookup stage (`input.go:84`; GCREX-42; GCREX-70) | `gone`, exit 80 (`inputs.py:209-229`, `rpp/outcome.py:114-115`) | matches | Both answer the same typed word; the installed edge build reads it as `ErrSessionNotFound`, gc 1.5.1 as a plain error instead of the old exit 0. |
| 5 | zero keys | no-op exit 0 before lookup, even for an unknown name (`main.go:216-218`) | looks up target first (after key translation), so a gone box is `gone` (`inputs.py:289-294`) | intentional | Matches the spec of a fire-and-forget op. |
| 6 | ambiguous failure / retry | a pre-send `unavailable`/`not_sent` failure is typed exit 80; any other Rex CLI failure exits 1 with a wrapped error (`provider.go:901-920`); no retry logic | unknown outcome exits 1 and is never retried (`inputs.py:274-281`) | intentional | GCREX-42 typed the pre-send words; the ambiguous post-write case keeps exit 1. Same effective outcome at baseline; we have no retries either. |
| 7 | partial failure | all keys validated before sending, so a bad key never reaches Rex (`provider.go:710-717`) | all keys validated before any is sent (`inputs.py:285-287`) | intentional | GCREX-43 closed the mistake: `translateKeys` rejects the whole call before the lookup. Not reachable from gc: it sends dialog navigation as repeated `Down`/`Up` then `Enter` in separate calls (`dialog.go:696,712`), and every name it emits is mapped. |
| 8 | activity side-effect | records input time after a successful send (`noteInput`, `provider.go:728`) | records input via `_noted` (`inputs.py:299`) | intentional | GCREX-24 closed the gap: both stamp the input floor that `get-last-activity` reads. |

**Verdict.** intentional. The grammar now matches herdr-core: exact-case table, modifier rules, literal-text splitting, and exit-80 `rejected` for an unspellable key (GCREX-43). GCREX-42 added the typed `gone`/`unavailable`/`not_sent` classification, and GCREX-70 made a missing session answer exit 80 `gone` (row 4, `matches`). Verified against gc 1.5.1: its whole `send-keys` vocabulary is `Enter`/`Up`/`Down`/`Escape`/`C-u`, and all are mapped (row 2).

**Cross-references.**
GCREX-3 — nudge/send-keys/interrupt behavior in our provider
GCREX-42 — typed `gone`/`unavailable`/`not_sent` for `send-keys`; GCREX-70 drops the baseline wrapper when the handshake declares `input-outcome`
GCREX-5 — Phase 4 RPP v0 delivery
GCREX-1 — pinned Rex key syntax (`docs/REX-CONTRACT.md:136`) and driving-op status (`REX-CONTRACT.md:224-226`)

**Follow-ups.**
- GCREX-43 added BTab→`shift+tab`, PPage→`page_up`, NPage→`page_down` and the full literal/modifier grammar for non-gc callers.
- GCREX-43 pre-validates every key before sending, closing the atomicity question.
- No action needed: gc 1.5.1 never sends literal text through `send-keys` (verified)


### interrupt

**Status** — implemented

**Our implementation.** Dispatched at `cmd/gc-runtime-rex/main.go:124` (arity 1, no stdin) to `opInterrupt` (`main.go:410-412`), which returns `Provider.Interrupt` directly. `Interrupt` (`internal/provider/provider.go:1231`) calls `deliverInput` (`input.go:154`). That call classifies a missing or dead pane as `gone` and applies no guard and no input floor (`inputPolicies[interrupt]`, `input.go:75`). Its send callback calls `client.SendKeys(..., "ctrl+c")` (`interruptKey`, `provider.go:71`). That is a keystroke through Rex's key API, not a signal; the doc comment says Rex has no signal API (`provider.go:922-924`). It sends once, does not wait or confirm anything, and does not retry. A missing binding or an exited pane program gives `ErrSessionNotFound`, which `classifyInput` (`input.go:84`) turns into a typed `gone` outcome. An unreachable server (live lookup or send) gives exit 80 `unavailable`; any other pre-send failure gives exit 80 `not_sent`. A send failure that may have written the request stays exit 1 as `interrupt <name>: …` (outcome unknown). A delivered interrupt is exit 0 with no output. Deadline is the default 30 s. No state is touched. Note for a Pi-first city: gc 1.5.1 does not interrupt Pi sessions at all; it stops and restarts them (`internal/session/submit.go:520-525`).

**herdr-core.** `ops_table.py:85` maps `interrupt` to `inputs.op_interrupt` (`herdrcore/inputs.py:355-356`), which runs `interrupt()` (`inputs.py:304-319`). It lists the city, takes the newest box and requires it to be intact (`inputs.py:209-229`). Then it calls hc `terminal.interrupt` with `timeout: 3.0` (`hc/client.py:102,268-270`), so hc waits up to 3 s for the program to settle. The client budget is 5 s (`hc/client.py:133`). The signal or key hc uses is not in this clone. The reply carries `outcome` and `key` (`hc/parse.py:149-151`). The test fake does `killpg(pid, SIGINT)` (`tests/fakehc/terminal.py:363-364`), but the real hc is unverified; I would need the hc source. A missing or finished box answers `gone`, an unreachable core `unavailable`. Both exit 80 with one stdout line (`rpp/outcome.py:103-115`). Success is exit 0 with empty stdout. The call is never retried, because a second Ctrl-C can end the program (`inputs.py:314-316`, `hc/client.py:21-24`). If the request may have been written but no reply came, it exits 1 as unknown (`inputs.py:277-281`).

**Deviations.**

| # | Aspect | Rex provider | herdr-core | Verdict | Rationale |
|---|---|---|---|---|---|
| 1 | mechanism | `ctrl+c` keystroke via Rex SendKeys (`provider.go:901`; `interruptKey`, `provider.go:71`) | hc `terminal.interrupt` (`hc/client.py:268`); real signal/key unverified | intentional | Matches gc, which models an interrupt as `C-c` (`carrier.go:113-116`); Rex has no signal API (`provider.go:922-924`). Whether hc signals the group needs hc source. |
| 2 | wait/confirm | none; returns once keys are sent | hc waits up to 3 s for the program to settle (`hc/client.py:102`) | intentional | RPP v0 has no outcome contract, and gc does not wait after `interrupt` on exec providers: its idle wait needs `IdleWaitProvider`, which the exec carrier does not implement (`submit.go:273-282`). |
| 3 | no live program | exit 80 `gone` (`main.go:278-281`) | exit 80 `gone: …` (`outcome.py:114`) | matches | GCREX-70 declared `input-outcome` with `carrier.ops`, so the installed edge build reads `gone` as `ErrSessionNotFound`; gc 1.5.1 reads the same exit 80 as a plain error, where the old wrapper exited 0. |
| 4 | failure mapping | pre-send errors exit 80 `unavailable`/`not_sent`; post-send errors exit 1 (`provider.go:925-955`; GCREX-44) | unsent maps to `not_sent`/`unavailable` (exit 80). Unknown after write exits 1 (`inputs.py:277-281`) | intentional | Rex cannot tell written from unwritten, so every ambiguous send failure stays exit 1 rather than claiming `not_sent`. |
| 5 | repeat/retry | one SendKeys, no retry; client retry policy unverified | never retried (`inputs.py:314`) | unverified | gc never retries `interrupt` itself, but codex/kimi/gemini may see an `Escape` first and `C-c` as fallback (`submit.go:290-305`). Rex client retry on a dropped connection remains to check; a second Ctrl-C can end the program. |

**Verdict.** intentional. Row 3 now matches herdr-core: a missing session answers exit 80 `gone`, which the installed edge build reads as `ErrSessionNotFound` after GCREX-70 declared `input-outcome` with `carrier.ops`; gc 1.5.1 reads the same exit 80 as a plain error instead of the old best-effort exit 0. Row 4 keeps the typed `unavailable`/`not_sent` pre-send words, while the ambiguous post-write case keeps exit 1. The behaviour fits RPP v0 and Rex's key-only API. Row 5 is the only open point, and it needs a read of the Rex client's retry code.

**Cross-references.**
GCREX-3 — agent input path (Nudge/SendKeys/Interrupt)
GCREX-5 — RPP v0 delivery, exit-code contract
`docs/REX-CONTRACT.md` §9 — interrupt row (line 208), driving ops not required

**Follow-ups.**
- Check whether `internal/rex` SendKeys retries after a dropped connection; if so, disable it for `ctrl+c` (gap).
- Read the hc source to confirm whether `terminal.interrupt` is SIGINT to the foreground group or `\x03` (gap).


### clear-scrollback

**Status** — implemented

**Our implementation.** Dispatched at `cmd/gc-runtime-rex/main.go:114` to `opClearScrollback` (`main.go:348-350`), which reads no stdin and returns `Provider.ClearScrollback` directly (GCREX-57 removed the `bestEffort` wrapper). `ClearScrollback` (`internal/provider/provider.go:1100`) calls `deliverInput` (`input.go:154`). No binding or an exited program is `ErrSessionNotFound`, which `classifyInput` (`input.go:84`) answers as exit 80 `gone` with one stdout line and no Rex mutation. `inputPolicies[clear-scrollback].sendNotFound` answers `gone` for a mid-call `rex.ErrNotFound` (`input.go:77`). Otherwise it calls `client.BlockClear` on the bound session/block (the Rex `clear` block method, `docs/REX-CONTRACT.md:93`). `rex.ErrUnavailable` is `unavailable`, and any other mid-call error stays exit 1 (outcome unknown). This is a server-side Rex call on the pane's terminal, not an injected tty escape. The contract notes it is a no-op while a full-screen program runs (`REX-CONTRACT.md:93`). GCREX-31 confirmed live that `clear` drops the visible screen as well as the scrollback, so a later `peek` via `format` shows blank rows. An attached app client stays connected (`docs/REX-CONTRACT.md` §10.1). A live clear is exit 0 with empty stdout and stderr (`TestClearScrollbackDeliveredExitsZero`). Deadline is the 30 s default. The call is idempotent: a second clear on an already-empty buffer is not an error. Verified in gc 1.5.1: gc calls it only from the session reconciler, right after a successful kill for max-age or idle timeout, and discards the result (`cmd/gc/session_reconciler.go:3739`, `:3894`). It never re-peeks. The tmux carrier form is `tmux clear-history`, history only (`carrier.go:118-121`).

**herdr-core.** `op_clear_scrollback` (`herdrcore/inputs.py:359`, table entry `ops_table.py:86`) calls `clear_scrollback` (`inputs.py:320-336`). That function sends nothing: its docstring says hc keeps no scrollback a client can clear, so it only does a `terminal.get` to check the box is still intact and then answers `delivered` (exit 0, empty stdout). The test asserts no input or keys are written (`tests/unit/test_herdrcore_inputs.py:283-286`). A missing name, no box, or ended program answers `gone`, and an unreachable core answers `unavailable`. Each of these exits 80 with one outcome line (`rpp/outcome.py:103-115`, `rpp/exitcodes.py`), because herdr declares `input-outcome` (`handshake.py:43`). A lost reply after the request was written exits 1 (`inputs.py:327-328`; test `test_herdrcore_inputs.py:289-295`).

**Deviations.**

| # | Aspect | Rex provider | herdr-core | Verdict | Rationale |
|---|---|---|---|---|---|
| 1 | what is cleared | Calls Rex `clear` on the pane (`provider.go:489`) | Nothing; a no-op after an intactness check (`inputs.py:320-336`) | intentional | Rex has a clear method; hc has no clearable scrollback. |
| 2 | missing session | Exit 80 `gone`, one stdout line, no clear (`provider.go:688-703`; GCREX-57) | Exit 80 `gone` (`inputs.py:213-229`) | intentional | Now matches herdr-core's word. gc 1.5.1 discards the error at both reconciler call sites, so the typed outcome does not change gc behavior. |
| 3 | ended or dead program | `livePane` sees the exited program; exit 80 `gone`, no clear (`provider.go:688-703`, `:834-850`; GCREX-57) | Exit 80 `gone` (`inputs.py:332-333`) | intentional | Same as row 2; the liveness pre-check now matches herdr-core's `terminal.get`. |
| 4 | unreachable server / unknown outcome | Pre-send `unavailable`/`not_sent` exit 80; lost reply after write exits 1 (`provider.go:688-703`, `:823-832`; GCREX-57) | `unavailable` exits 80; lost reply after write exits 1 (`inputs.py:327-328`) | intentional | Rex cannot distinguish a written request with a lost reply from an unsent one, so the ambiguous case stays exit 1. |
| 5 | verifying liveness first | `livePane` calls `BlockProcess` (`paneAlive`) before the clear (`provider.go:688-692`, `:834-850`; GCREX-57) | Reads `terminal.get` before answering (`inputs.py:324`) | intentional | Now matches: a dead program is `gone` before any clear is attempted. |
| 6 | effect on peek / attached clients | Verified live (GCREX-31): `clear` truncates the server scrollback and the visible screen, so a later `peek` shows blank rows, and the attached app client stays | None: nothing is cleared, so peek and clients are unaffected | no-impact | gc calls `clear-scrollback` only from the session reconciler after a successful kill and discards the result (`session_reconciler.go:3739`, `:3894`), so neither effect matters to gc. |

**Verdict.** intentional. GCREX-57 closed rows 2-5: a missing or dead pane answers exit 80 `gone` with no clear, and pre-send failures use the typed `unavailable`/`not_sent` words. Only the ambiguous lost-reply case stays exit 1. Row 1 remains an intentional deviation: we do a real Rex clear while herdr-core has nothing to clear. gc 1.5.1 discards `ClearScrollback` errors at both reconciler call sites, so the typed failures do not change gc behavior. GCREX-31 closed row 6: `clear` drops the scrollback and the visible screen, and an attached client stays. gc still calls the op only after a kill and discards the result.

**Cross-references.**
GCREX-5 — Phase 4 RPP v0 delivery; added the driving ops including clear-scrollback.
GCREX-1 — pinned Rex contract; `clear` method row (`REX-CONTRACT.md:93`).
GCREX-3 — the driving ops; nudge/send-keys/clear-scrollback share the
`ErrSessionNotFound` → exit 80 `gone` classification (GCREX-70).
GCREX-44 — interrupt's typed `gone`/`unavailable`/`not_sent` outcomes.
GCREX-57 — clear-scrollback's typed `gone` outcome for a missing or dead pane, with no mutation.
`docs/REX-CONTRACT.md` §9 (line 224-226) — driving ops not conformance requirements.

**Follow-ups.**
- Probed in GCREX-31: after `clear`, `peek` returned blank rows and the attached app client stayed (`docs/REX-CONTRACT.md` §10.1).
- Recorded in GCREX-31 at `docs/REX-CONTRACT.md:93`, which now states the scrollback, viewport and client effect.


### watch-startup

**Status** — not implemented

**Our implementation.** `watch-startup` is listed in the `unimplemented` map (`cmd/gc-runtime-rex/main.go:74-79`); `run` prints `operation "watch-startup" not implemented` to stderr and returns `exitUnknown` (2) before any arg check, state-dir lookup, or Rex call (`cmd/gc-runtime-rex/main.go:123-131`). It reads no stdin, writes nothing to stdout, and touches no state; `cmd/gc-runtime-rex/dispatch_test.go:108-118` pins exit 2 with empty stdout. The handshake does not declare any capability that implies a stream (`proc.stream` is reserved in gc 1.5.1, `docs/REX-CONTRACT.md:251`). `GC_READY_PROMPT_PREFIX` is a plain meta key the provider never acts on; the write-only `Binding.ReadyPrompt` field it used to feed was removed in GCREX-26, so no startup detection runs in the provider or dispatcher. Startup-dialog dismissal happens inside Rex during the launch (`docs/REX-IDENTITY.md:8-18`), and `docs/plans/native-rex-provider.md:464-465` defers a dialog capability.

**herdr-core.** `watch-startup` is a table entry with deadline class `start` (`herdrcore/ops_table.py:87`), so 120 s by default (`rpp/ops.py:23-27`); herdr also honours `GC_EXEC_DEADLINE_MS` (`rpp/perop.py:129`), which gc 1.5.1 never sends (its deadlines are fixed at 30 s, 120 s for `start`/`provision`, and none for `attach`). `op_watch_startup` (`herdrcore/screen.py:28-44`) requires an existing record (`no session 'X'` → exit 1, `screen.py:30-32`), then loops while `deadline.budget(0.25) > 0`: it reads the screen only, never history (`screen.py:5-7,37`), and emits one NDJSON line `{"content": "<screen text>"}` on stdout whenever the text differs from the last emitted text, flushed per line (`screen.py:40-42`, `rpp/wire.py:307-314`, `rpp/perop.py:122-124`). It polls every 250 ms (`screen.py:25,43`). The only failure is a failed screen read, exit 1 with `cannot read the screen: …` (`screen.py:38-39`); there is no startup-failure or ready detection in the op, and the readiness decision stays with gc. The loop ends 1 s before the deadline (`rpp/cancel.py:27,98-103`) and returns exit 0 (`screen.py:44`), so a normal end is "time ran out", not "startup finished". SIGTERM sets the cancel token (`rpp/perop.py:50-57`); the next `pause` raises `Cancelled` within 100 ms (`rpp/cancel.py:66-74`), giving exit 1 `canceled (terminated)` (`rpp/perop.py:149-150`). Per the contract gc kills `watch-startup` outright (`docs/REX-CONTRACT.md:191`), so the SIGTERM path is a courtesy. The only test is `tests/unit/test_herdrcore_staging.py:205-231` (one line per change, no repeats, deadline 3.5 s). Contract side: `docs/REX-CONTRACT.md:224-230` lists `watch-startup` among the calls gc makes beyond the 17-row table and says it stays dedicated because the request/response `exec` connection cannot carry a stream. §9 gives exit 2 as "unknown operation, treated as success" for everything except `start`/`provision` (`docs/REX-CONTRACT.md:200`), so exit 2 is success here. §9 names no explicit fallback for `watch-startup`; what gc does after a silent success is `unverified` (needs gc 1.5.1 source: the caller of `watch-startup` in `internal/runtime/exec`). The herdr-core docstring says the stream "saves gc one `peek` process per poll" (`screen.py:3-5`), which implies the fallback is gc polling `peek`; we implement `peek` (`cmd/gc-runtime-rex/main.go:98`). The consumer is gc's startup-dialog clearing (`screen.py:3-4`), not readiness proper. `GC_READY_PROMPT_PREFIX` is a tmux-provider variable, not an RPP rule (`docs/REX-CONTRACT.md:264-266`); herdr-core's `watch-startup` does not read it (no reference in `screen.py`).

**Deviations.**

| # | Aspect | Rex provider | herdr-core | Verdict | Rationale |
|---|---|---|---|---|---|
| 1 | op presence / exit code | exits 2, no output (`main.go:74-79,123-131`) | streams NDJSON screen events, exits 0 or 1 (`screen.py:28-44`) | not-implemented | Verified in gc 1.5.1: exit 2 with empty stdout marks the watcher unsupported, then gc polls `peek` every 500 ms (120 lines) and answers dialogs through `send-keys` (`exec.go:453-456`, `:376-380`) |
| 2 | stdout format | none | one `{"content": ...}` line per screen change, 250 ms poll (`screen.py:25,40-42`) | not-implemented | Would need a `block.read`/format poll loop over Rex; no existing helper streams |
| 3 | deadline and cancellation | not applicable (returns instantly); no 120 s `start` class | `start` deadline, ends 1 s early with exit 0, SIGTERM → exit 1 (`ops_table.py:87`, `cancel.py:98-103`, `perop.py:149-150`) | not-implemented | gc SIGKILLs `watch-startup` outright, so SIGTERM handling is optional |
| 4 | startup/readiness detection | no detection; the write-only `ReadyPrompt` field was removed (`binding.go`, GCREX-26) | none in the op either; prompt matching stays in gc | intentional | Neither side uses the ready prompt inside `watch-startup`; Rex dismisses dialogs in-launch (`REX-IDENTITY.md:8-18`) |

**Verdict.** `not-implemented`. Exit 2 is a legal, documented RPP v0 answer for `watch-startup` and is treated as success. The loss is gc's streaming optimisation: gc falls back to `peek` polling every 500 ms and `send-keys` dialog handling (`exec.go:376-380`, `:453-456`). That fallback runs only when `ShouldAcceptStartupDialogs` is true (`startup_hints.go:20-25`), and exit 1 with no output would fail `start` (`exec.go:464-469`), so exit 2 is the safe answer. `GC_READY_PROMPT_PREFIX` and the nudge path do not interact with it: `nudge` runs from `start`'s config after launch (`docs/rex-provider.md`, GCREX-14 note), not from a watch.

**Cross-references.**
GCREX-1 — pinned §9: `watch-startup` is outside the 17-row table, killed outright, exit 2 = success.
GCREX-5 — dispatcher: `watch-startup` kept as exit 2 (recorded v1 limit; probed).
GCREX-14 — startup nudge is delivered by `start`, independent of any startup watcher.
GCREX-16 — `program_status` / OSC 7501 is the preferred readiness signal if a watcher is ever built.
GCREX-3 — `peek` is the likely fallback path.
`docs/REX-CONTRACT.md` §9 — op table, exit codes, `watch-startup` rationale.
`docs/rex-provider.md` — "Unimplemented operations exit 2".

**Follow-ups.**
- Read gc 1.5.1's caller of `watch-startup` and record what exit 2 / empty stdout does to startup-dialog clearing (gap).
- If the fallback is peek polling and Rex dismisses dialogs itself, document `watch-startup` as intentionally exit 2 and close the gap (gap).
- Only build a streaming `watch-startup` (poll block format/read every 250 ms, emit `{"content":...}` on change, end 1 s before `GC_EXEC_DEADLINE_MS`, exit 0) if the pilot shows slow or stuck startup dialogs (gap).


### attach

**Status** — implemented

**Rex-only extensions.** Multi-client attach to one pane is recorded in
[`research/rex-provider-extensions.md`](research/rex-provider-extensions.md) §5.

**Our implementation.** `attach` is registered in the `ops` table (`cmd/gc-runtime-rex/main.go:83`) with arity 1 and handled by `opAttach` (`main.go:379`), which writes `attachHint(name)` to stdout and exits 0. The hint is one line: `Rex has no in-terminal attach; open the Rex app and select the session "<name>" to attach`. The handler takes no provider action and makes no Rex call, so an unknown session or an unreachable server cannot fail it; the only failure is a stdout write error (exit 1). The handshake declares `report-attachment`, `report-activity`, `idle-boundary`, `carrier.ops`, `input-outcome`, `observe.v1` and `city-scoped`, but not `tty.attach` (`main.go:42-46`; `docs/rex-provider.md:212-213`). `internal/provider/attach.go` continues to hold only `IsAttached`, `blockOwner` and `watchesAgent` (`attach.go:23-90`), the read-only viewer probe behind `is-attached` (`main.go:577`); the hint lives in the dispatcher because there is no Rex primitive it needs. `cmd/gc-runtime-rex/dispatch_test.go:302-326` covers a live pane, an unknown session and an unreachable Rex client, each exit 0 with the hint on stdout and empty stderr. The plan sketched a future `Attach` as `session.focus_block` + `session.focus_window` (`docs/plans/native-rex-provider.md:391`); the hint is the contract-permitted alternative and adds no client method.

**herdr-core.** `ops_table.py:88` registers `attach` with `deadline="start"`. In herdr's table that class is 120 s (`rpp/ops.py:23-27`), or `GC_EXEC_DEADLINE_MS` if gc sends it (`rpp/ops.py:127`), not "none". The deadline only bounds the pre-exec work, because `op_attach` ends in `os.execve` (`attach.py:128`), which replaces the Python process. The handler resolves the live record (`attach.py:105-108`) and the `hc` binary (`:110-113`). It checks that the city's core answers `hello()` so `hc` will not start a core with the wrong environment (`:118-121`). It builds the core's allowlist env plus `TERM`, `COLORTERM`, `TERM_PROGRAM`, `TERM_PROGRAM_VERSION` (`:122-123`, `:38`). It takes a shared `flock` on the incarnation's attach lock, inheritable across the exec (`:124-127`), then execs `hc attach <terminal-address>` (`:128`). Stdin, stdout and the tty therefore pass straight to `hc`. Raw mode, SIGWINCH resize, the detach key, exit codes on detach vs error, and client cleanup are not in this pack: they belong to `hc attach`, which is outside the clone (unverified; would need the `hc` source). Pre-exec failures are `OpError` and become exit 1 (`rpp/perop.py:147-148`). Cleanup is the kernel closing the lock fd when `hc` exits, which releases the lock that `is-attached` tests (`attach.py:42-56`, `:88`). The pack does not declare `tty.attach` either (`rpp/wire.py:36-39`).

**Deviations.**

| # | Aspect | Rex provider | herdr-core | Verdict | Rationale |
|---|---|---|---|---|---|
| 1 | op presence | exit 0, one-line hint to open the Rex app; no Rex call | execs `hc attach` with tty passthrough | intentional | gc 1.5.1 calls `attach` and treats exit 2 as failure; the hint removes the exit-2 failure without a pty passthrough, and `tty.attach` stays undeclared |
| 2 | capability gating | does not declare `tty.attach` | op exists but does not declare `tty.attach` either | intentional | the capability is reserved in gc 1.5.1 and has no consumer; gc calls `attach` without checking it, so both packs omit it harmlessly |
| 3 | attach lock / viewer signal | `is-attached` derives viewers from `session.view` block owner + `session.list_clients` + active window (`attach.go:23-60`) | `attach` holds a flock that `is-attached` reads (`attach.py:88,124-127`) | intentional | Rex reports clients server-side; no lock needed |
| 4 | deadline | hint handler retires immediately; no deadline applies (spec: `attach` has none) | `start` class: 120 s default, pre-exec only | intentional | confirmed: the spec says no deadline for `attach`, and gc's `runWithTTY` uses no context (`exec/exec.go:195-202`); herdr's class only bounds setup before `execve` |

**Verdict.** `intentional`. `attach` was the one op where "absent = exit 2" was not conformant in practice: gc 1.5.1 calls `attach` from `gc session attach` and `gc session new` after an `is-running` check and a possible `start` (`runtime/seam_adapter.go:139-149`, `exec/seams.go:170-172`), and it runs it through `runWithTTY`, which never goes through the exit-2 mapping, so the raw `*exec.ExitError` became `gc session attach: exit status 2` with exit 1 (`exec/exec.go:195-202`, `cmd/gc/cmd_session.go:399`, `:508`, `:1554`). GCREX-20 answers `attach` with exit 0 and a hint instead, which removes the user-visible failure. There is no deadline and no `tty.attach` gate, and the handler neither opens a pty nor focuses a pane; herdr's real passthrough is the richer behavior, but the hint keeps `gc session attach` working without declaring `tty.attach`. `docs/REX-CONTRACT.md` §9 now records `attach` (and `exec`) as the exceptions to "exit 2 is success".

**Cross-references.**
GCREX-4 — identity/metadata/activity/attachment; `attach.go` lives here as `IsAttached`
GCREX-19.9 — `is-attached` comparison; covers the attachment probe, not the tty op
GCREX-5 — RPP v0 delivery; handshake and unimplemented-op list
`docs/REX-CONTRACT.md` §9 — op table, deadlines, `tty.attach` reserved
`docs/plans/native-rex-provider.md:391,415` — future `Attach` via focus; do not declare `tty.attach`

**Follow-ups.**
`docs/plans/city-owned-rex-server.md` — GCREX-30 experiments. A city-owned server is reachable with `rex hosts add` and `-S <label>`. The Rex app cannot be pointed at it (unverified). So the hint text is wrong for that mode and must name the address instead.

- If a real attach is wanted, spike `rex attach`/pty passthrough, then declare `tty.attach` only after `gc runtime conformance` accepts it (gap).
- Keep `docs/REX-CONTRACT.md` §9 and `docs/rex-provider.md` aligned with the hint behavior if the op changes (gap).


### set-meta

**Status** — implemented

**Our implementation.** Dispatched at `cmd/gc-runtime-rex/main.go:102` (2 required args; an empty name is rejected at `main.go:133` and an empty key at `main.go:136`). `opSetMeta` (`main.go:333-339`) reads all of stdin, converts to string with no trim, and calls `Provider.SetMeta` (`internal/provider/meta.go:56-62`). That requires a stored session record and maps the name to the primary binding name (`meta.go:14-21`), then calls `fileMetaStore.Set` (`internal/provider/stores.go:222`). The store writes one file per (name,key) at `<state>/meta/<sha256(name)>/<sha256(name\0key)>.json` (`stores.go:161`), the value base64-encoded so any bytes round-trip (`stores.go:141-148`). The write is temp file plus rename in the same dir (`filestore.go:27-51`) with no fsync and no chmod (CreateTemp gives 0600; dirs 0755). Locking is an in-process mutex only (`stores.go:~153`); `withStateLock` (`lock.go:209`) is not used. Concurrent processes on different keys cannot clobber each other, and on the same key last rename wins. Set never contacts Rex, but it requires a session record: the cached binding that answers to the name. An unknown, stopped or unstarted session fails with `no session` and exit 1 and stores nothing, matching herdr-core (GCREX-49); a name with a record stores. Stdout is empty, exit 0, and any write error gives exit 1. The op runs under the default 30 s deadline, and ctx is unused. Key validation is the dispatcher's non-empty check (`main.go:136`); there is no key or value size cap and no in-op reserved key. The provider seeds or clears the identity keys `GC_SESSION_ID`, `GC_INSTANCE_TOKEN`, `GC_RUNTIME_EPOCH`, `GC_CONTINUATION_EPOCH`, `GC_AGENT`, `GC_TEMPLATE`, `GC_ALIAS` and `GC_PROVIDER` (`identity.go:20-56`). Stop (`provider.go:741-771`) clears every key for the name through `clearIncarnations` (`provider.go:778-794`), so no key (`suspended`, `GC_DRAIN_ACK`, `GC_READY_PROMPT_PREFIX`) survives stop and restart.

**herdr-core.** `op_set_meta` (`herdrcore/reads.py:425-427`) decodes stdin as UTF-8 with surrogateescape (bytes preserved, no trim) and calls `_mutate_meta` (`reads.py:412-422`). That takes the exclusive per-name lock (`context.py:152-159`, wait up to `MUTATION_LOCK_WAIT_S` = 30 s, cancel-aware), loads the current record's object and its single `meta/<nonce>.json` file, and rewrites the whole file with the value merged in. Write is a 0600 `O_EXCL|O_NOFOLLOW` temp file, then rename, with no fsync (`config.py:174-194`). A missing key arg gives an error (`reads.py:384-388`). An unknown session gives `no session <name>` (error exit 1) for set, and a silent success for remove (`reads.py:416-420`). A session in a terminal the pack did not start is an error (`reads.py:395-397`). The metadata file is deleted when the incarnation ends (`lifecycle.py:389-396`, `forget`), so all meta, including user keys, dies with the incarnation. Metadata is seeded at start with the identity keys (`metastore.py:88-91`). Reads of the file are capped at 1 MiB (`config.py:197`, `read_private_file` limit). That makes a set that grows the file past 1 MiB writable but not readable back (unverified: needs a run to confirm the error path). No other value or key limit appears in `metastore.py`.

**Deviations.**

| # | Aspect | Rex provider | herdr-core | Verdict | Rationale |
|---|---|---|---|---|---|
| 1 | unknown session | Exit 1 `no session` when no stored binding answers to name; writes nothing (`meta.go:56-62`) | Exit 1 `no session` (`reads.py:416-420`) | intentional | GCREX-49: the precondition is the cached session record (the pack-started pane), so set matches herdr-core. `remove-meta` still succeeds for an unknown session, like the pack. |
| 2 | lifetime across stop | Every key cleared on Stop (`provider.go:778-794`) | Whole meta file removed when the incarnation ends (`lifecycle.py:389-396`) | intentional | GCREX-26 closed the mistake: `clearIncarnations` clears the whole sidecar, so no stale `suspended`, `GC_DRAIN_ACK` or `GC_READY_PROMPT_PREFIX` leaks into the next start. |
| 3 | locking | In-process mutex only (`stores.go`); one file per key, last writer wins per key | Cross-process exclusive per-name flock, 30 s wait (`context.py:152-159`) | intentional | Per-key files make a read-modify-write lock unnecessary. Set vs Stop's identity clear is not serialised, so a set racing a stop can survive. |
| 4 | file permissions | CreateTemp 0600 file, dir 0755 (`filestore.go:29,36`); no symlink or owner check on read | 0600, O_NOFOLLOW, owner check on read (`config.py:178,197-204`) | intentional | Local single-user state; hardening is not needed for RPP v0 conformance. |
| 5 | size limits | None on key or value; filename is a hash so any key length works | 1 MiB read cap on the file (`config.py:197`); no explicit set-side cap | not-implemented | Large values are unbounded. Low risk, but there is no stated bound; gc values are small. |

**Verdict.** intentional. Behavior matches RPP v0 (stdin to store, empty stdout, 0/1 exit). Stop now clears the whole sidecar with the incarnation (GCREX-26), matching herdr-core.

**Cross-references.**
GCREX-4 — identity/metadata/activity design and the sidecar store
GCREX-5 — Phase 4 RPP v0 delivery, set-meta/get-meta wiring
GCREX-17 — alias resolution used by `metaName`
GCREX-18 — `GC_ALIAS`/`GC_AGENT` seeding into meta
docs/REX-CONTRACT.md §9 (op table row `set-meta`, line 213), §13.4 (provider meta keys)

**Follow-ups.**
- Optionally serialise set against Stop with `withStateLock` (gap).
- Document that there is no value size cap, or add one (gap).


### get-meta

**Status** — implemented

**Our implementation.** Dispatch entry `get-meta <name> <key>` (2 args) at `cmd/gc-runtime-rex/main.go:103`; an empty key is rejected with exit 1 at `main.go:136` before the handler runs (`main.go:341-347`) and calls `Provider.GetMeta` and writes the value with `io.WriteString` – no trailing newline added, stdin unread. `Provider.GetMeta` (`internal/provider/meta.go:31-40`) resolves the name to the primary binding via `metaName` (`meta.go:10-19`, rig-qualified aliases), then reads `fileMetaStore.Get` (`internal/provider/stores.go:204-220`): one JSON file per (name,key), value base64 so bytes round-trip. A missing file, an old entry without `value_b64`, an unknown key and an unknown session all give `"", nil`: empty stdout, exit 0, with no session-existence check and no Rex call (`meta.go:26-27`). Only an unreadable/corrupt file or bad base64 is an error (`stores.go:167-168`, `readJSON` `filestore.go:55-67`), exit 1. Default 30 s deadline; no state is written.

**herdr-core.** `op_get_meta` (`herdrcore/reads.py:404-409`, table entry `ops_table.py:80`) requires a non-empty key (`reads.py:384-388`, else `OpError` → exit 1), looks up the session's newest record (`current_record`, `reads.py:263-278`) and its per-incarnation meta file (`metastore.py:105-125`), and emits `values.get(key, "")` as raw bytes, no newline (`reads.py:409`). Unknown session, session with no meta file, and unset key all return empty stdout, exit 0 (`reads.py:407-408`; test `tests/unit/test_herdrcore_lifecycle.py:208-209`). It exits 1 when the core cannot be read (`reads.py:277-278`), when the terminal was not started by the pack (`reads.py:396-397`), or when the meta file is corrupt (`reads.py:400-401`). Metadata is tied to the incarnation (`<state>/meta/<obj>.json`, `metastore.py:1-19`), so it dies with `stop`; it may also be seeded by `start`.

**Deviations.**

| # | Aspect | Rex provider | herdr-core | Verdict | Rationale |
|---|---|---|---|---|---|
| 1 | empty key argument | `get-meta n ""` exits 1, metadata key must not be empty (`main.go:136`, GCREX-26) | empty key is `OpError`, exit 1 (`reads.py:386-387`) | intentional | GCREX-26 closed the gap: the empty key is rejected at dispatch and pinned by `TestMetaOpsRejectEmptyKey`. gc never sends one, so this is hygiene only. |
| 2 | backend failure | never contacts Rex; only a corrupt store file gives exit 1 | exit 1 when the herdr core is unreachable or the terminal is foreign (`reads.py:277-278,396-397`) | intentional | Our store is local sidecar files (`docs/plans/native-rex-provider.md:401`); there is no core to fail. |
| 3 | metadata lifetime / keying | keyed by gc name (resolved to primary binding), independent of a live pane | keyed by incarnation nonce of the current record; unknown session is empty | intentional | Different state model; observable result for unknown name is the same (empty, exit 0). |

**Verdict.** intentional. Both emit the raw value with no trailing newline, and both answer exit 0 with empty stdout for an unset key and for an unknown session (best-effort, not an error), matching the contract's "empty = not set". Differences are only in how missing-backend errors surface.

**Cross-references.**
GCREX-4 — phase 3 identity/metadata/activity/attachment, introduced the meta store
GCREX-13 — seeds identity keys so `get-meta GC_SESSION_ID` works before launch
GCREX-17 — alias/candidate-set resolution used by `metaName`
GCREX-11 — start-config env inventory feeding get-meta keys
`docs/REX-CONTRACT.md` §9 (op table, line 214), §13.4 (provider meta keys)
`docs/rex-provider.md` (alias resolution for meta ops, ~line 225)

**Follow-ups.**
- Add a unit test pinning `get-meta` on an unknown session and unknown key to exit 0 with empty stdout, and no trailing newline (gap).


### remove-meta

**Status** — implemented

**Our implementation.** Dispatch entry `remove-meta` requires 2 args (`cmd/gc-runtime-rex/main.go:104`); the arity check rejects an empty session name (`main.go:133`) and the empty-key guard rejects an empty key (`main.go:136`). `opRemoveMeta` calls `p.RemoveMeta(ctx, name, key)` (`main.go:350`), which resolves the name through `metaName` (alias to primary binding, `internal/provider/meta.go:9-18`) and calls `meta.Delete` (`meta.go:44-46`). The file store keeps one JSON file per (name,key) and `Delete` does `os.Remove`, treating `ErrNotExist` as success (`internal/provider/stores.go:151-159,194-200`). It reads no stdin, writes no stdout, exits 0 on success. It never contacts Rex and does not check that the session exists. Locking is an in-process mutex only (`stores.go:194-196`); the unlink is atomic and per-key files mean concurrent processes touching other keys cannot clobber each other (`stores.go:151`). No key is reserved, including `GC_READY_PROMPT_PREFIX`; it is deleted like any other and no longer feeds a Start fallback (the write-only ready-prompt field was removed in GCREX-26).

**herdr-core.** `op_remove_meta` calls `_mutate_meta(ctx, name, key, None)` (`herdrcore/reads.py:430-432`). A missing key argument raises `OpError("missing metadata key")` (`reads.py:384-388`). It takes the per-name exclusive lock (`reads.py:413`); an unknown session returns OK for remove (`reads.py:415-417`), while set raises. A session in a terminal the pack did not start is an error (`reads.py:396-397`). Otherwise it pops the key from the in-memory dict and rewrites the whole meta file (temp + rename under the lock, `metastore.py:16,64-70`, `reads.py:419-421`), even if the key was absent (a file is created if none existed). No key is protected. Exit 0, no stdout.

**Deviations.**

| # | Aspect | Rex provider | herdr-core | Verdict | Rationale |
|---|---|---|---|---|---|
| 1 | empty key | rejected at dispatch with exit 1 (`main.go:136`, GCREX-26) | `OpError` "missing metadata key" (`reads.py:386`) | intentional | GCREX-26 closed the mistake: the guard is pinned by `TestMetaOpsRejectEmptyKey`. gc never sends an empty key. |
| 2 | cross-process locking | in-process mutex; per-key files, atomic unlink (`stores.go:194`) | exclusive per-name file lock, whole-file rewrite (`reads.py:413`) | intentional | Per-key files avoid read-modify-write races, so no lock is needed. |
| 3 | unknown/foreign session | always OK; meta keyed by name, no existence check (`meta.go:44`) | unknown session OK; foreign terminal is an error (`reads.py:396,415`) | intentional | Our meta is name-keyed and independent of Rex; no foreign-terminal concept. |
| 4 | absent key | no-op, no file written (`stores.go:196-198`) | rewrites meta file, may create it (`reads.py:420`) | intentional | Both exit 0; ours has fewer side effects. |
| 5 | provider-owned key guard | none for `GC_READY_PROMPT_PREFIX` | none | intentional | Matches reference; `GC_READY_PROMPT_PREFIX` feeds no Start fallback since GCREX-26 removed the write-only ready-prompt. |

**Verdict.** intentional. Behavior is idempotent for absent keys and missing sessions, as RPP expects; the empty-key guard (row 1) now matches herdr-core (GCREX-26).

**Cross-references.**
GCREX-4 — metadata ops (set/get/remove) design
GCREX-18 — claim identity keys (`GC_ALIAS`/`GC_AGENT`) stored via meta
docs/REX-CONTRACT.md §9 — remove-meta op row




### observe

**Status** — implemented (GCREX-101, declared unconditionally since GCREX-111)

**Our implementation.** `observe` is in the `ops` map as `opObserve` (`cmd/gc-runtime-rex/main.go`), and the handshake declares `observe.v1` unconditionally since GCREX-111 (`TestHandshakeDeclaresObserveV1`). gc 1.5.1 ignores the token, so the op stays inert there. `opObserve` drains a bounded stdin of process names and calls `Provider.Observe` (`internal/provider/observe.go`): no stored binding answers `absent`; a stored binding whose liveness Rex reports as unavailable (`rex.ErrUnavailable`) answers `unknown`, not `absent` (`TestObserveAbsentForUnknownName`); a block Rex no longer knows (`rex.ErrNotFound`) is not proof the name is free: the op rescans the record's labels without saving, answers for a pane that took them (`TestObserveAnswersForPaneRestoredUnderName`), and answers `absent` only when the scan finds nothing, leaving the stale binding for Start or Stop (`TestObserveAbsentWhenPaneVanished`); a dead pane answers `dead`; a live one answers `running` with the stored object and the start-config identity (`TestObserveRunningCarriesObjectAndIdentity`). A pane that took the name is answered with `"identity":{}` — gc's "a box with no gc identity (one an operator made)" — never an absent or null identity, which gc reads as "identity not yet readable" (rule 5) and would keep pending forever (`TestObserveAnswersForPaneRestoredUnderName`); null is kept for a started box whose sidecar is not yet readable. gc's reading is verified on the edge tree (`internal/runtime/exec/observe.go`: `parseObserve` leaves a null identity nil, `ObserveIdentity` turns that into `ErrRuntimeUnavailable: identity not yet readable`, and returns an empty map for `{}`); the pinned v1.5.1 baseline has no observe.v1 parser at all, since it never sends these ops. The object is derived from the binding alone, so `observe` takes no name lock and reads fresh.

**Object.** `object.id` pairs the Rex block id with the incarnation's creation time (`block:dog-1/2026-11-21T13:06:26.645083Z`), because Rex may reuse a block id for a relaunched pane under the same labels (the fake client models exactly that: `CloseBlock`, `internal/rex/faketest/fake.go`). Both parts come from the binding, so two observes of one box are byte-identical and a new start under the name differs — the check gc's `probeObserveObjectStable` makes (`internal/runtime/runtimecontract/observe.go:275`, edge tree) and `TestObserveObjectStableAndNewPerBox` pins. A discovered pane has no record and no creation time, so its block id stands alone. `object.created` is `Binding.CreatedAt` (`internal/provider/binding.go:36`) in RFC3339; a zero record time reports the zero instant, so a present box never carries an empty object.

**Probe bound.** `observe` budgets its Rex reads (`observeBudget`, `internal/provider/observe.go`): the 4 s probe bound less a 250 ms margin, or `GC_EXEC_DEADLINE_MS` less that margin when gc set it (gc sets that variable on every op under a deadline, `docs/reference/exec-session-provider.md:1346`). A backend that outlives the budget answers `unknown`, not an error, so the op never overruns rule 11's 4 s bound (`TestObserveAnswersUnknownWithinBound`). The conformance runner sets no deadline, so the default bound is what makes RPP-OBSERVE-013 pass.

**Agent liveness** is implemented (RPP-OBSERVE-006). One gc-shaped snapshot of the host process table serves the whole answer. `agentState` (`internal/provider/agentsnapshot.go:94`) calls `prochost.PSHost.Table(ctx, true)` (`internal/prochost/ps.go:37`). That read combines the identity table (`ps -axo pid=,ppid=,stat=,lstart=,comm=`) and the wide table (`ps -ww -axo pid=,ppid=,args=`). `mergeRows` joins them (`internal/prochost/ps.go:66`). `-ww` is load-bearing on the wide read. Without it, procps cuts the command line at `COLUMNS`, and a cut line basenames to nothing. That would read a live agent dead. `comm` is the final identity column. BSD ps cuts a non-final `comm` to 15 bytes, and a cut name would become an exact match candidate. `snapshotAgent` walks the box's foreground pid and every descendant. A Rex pane's foreground process can be the agent itself. It matches each row the way gc's table does. It reuses `prochost.Match` and `prochost.RowCandidates` (`internal/prochost/prochost.go:87,107`) and `expandProcessNames` (`internal/provider/liveness.go:42`): the command name, the `argv[0]` basename, and, for an interpreter (`node`, `bun`, `npx`, `deno`), the first non-flag argument that is not a runner subcommand (`run`, `exec`, `x`) and is not the value of a value-taking option such as `--require` (an option whose value is missing makes the row opaque, so the answer is unknown, never dead), matched by basename with and without its extension. This is gc's rule as stated in `docs/reference/exec-session-provider.md`, "Agent liveness" (edge tree), and as implemented in `internal/runtime/proctable/names.go` (`ProcessMatchesNameSet`): a command name alone is not enough because the kernel truncates `comm` and Node 25 names its main thread. The answer is `alive` with the matching pid, `dead` only for a readable subtree that holds no match, and otherwise omitted: no names on stdin, an unreadable or empty snapshot, a box root missing from the snapshot (its subtree is not observable), or an opaque descendant that could be the agent. gc reads an absent `agent` as alive, so the uncertain answers stay on the safe side. Pinned by `TestSnapshotAgent` (interpreter form, command-name truncation, wrapper shell, alias, deep descendant, unreadable and root-absent snapshots are unknown) and `TestObserveAgentFromProcessSnapshot` (alive with `agent_pid`, dead for a name not present, omitted for an unreadable snapshot). `last_activity` (RPP-OBSERVE-015) is still absent. A `dead` answer that races a `Stop` can carry no identity, because `Stop` clears the start identity (`clearStartIdentity`, `internal/provider/identity.go`) and gc reads an absent identity as "not yet readable" (rule 5), so the answer is safe and short-lived (F3).

**herdr-core.** `ops_table.py:72` registers `observe` with `stdin=True`, `deadline="probe"` and `cap=OBSERVE_STDOUT_CAP`. The probe deadline is 4 s (`rpp/ops.py:23-27`) and the stdout cap is 1 MiB (`rpp/ops.py:30`). stdin is a list of agent process names, one per line (`reads.py:62-64`, `op_observe` at `reads.py:131`). It takes a shared name lock and lists the city's records for that session (`reads.py:134-137`). It prints one JSON `ObserveAnswer` (`rpp/wire.py:162-182`): `absent` (no record, or city stopped), `unknown` (lock timeout or core unreachable), `dead` (box not intact), or `running` with box object, agent state, agent pid and identity (`reads.py:138-158`). It always exits 0 with an answer. It belongs to the `observe.v1` capability, which the handshake declares (`handshake.py:3`). The comment at `rpp/ops.py:28-30` cites a gc `exec/observe.go` probe timeout; no such file exists in gc 1.5.1, so the 4 s is herdr-local. Verified in gc 1.5.1: `observe` is never sent, and its own `Observe` is a Go-side fold of `get-last-activity`, `process-alive` and `is-attached` (`exec/seams.go:229-236`).

**Deviations.**

| # | Aspect | Rex provider | herdr-core | Verdict | Rationale |
|---|---|---|---|---|---|
| 1 | op presence | `observe` returns a JSON state answer and the handshake declares `observe.v1` | `observe` returns a JSON state answer | partial | `observe.v1` is outside the RPP v0 baseline. gc 1.5.1 ignores the token and never sends the op. |
| 2 | probe bound | answers within 4 s: a bounded budget over its Rex reads (`observeBudget`) turns an overrun into `unknown` | 4 s `deadline="probe"`, killed and read as unknown by gc | match | Rule 11 allows either: this provider ends its own call before gc's kill, so the answer is `unknown` rather than a kill (RPP-OBSERVE-013, `TestObserveAnswersUnknownWithinBound`). |

**Verdict.** partial. `observe` is not one of the 17 RPP v0 rows, and `docs/REX-CONTRACT.md:255-258` states `observe.v1` does not appear in gc 1.5.1 source, so gc 1.5.1 never sends the op. The op answers the edge source (`docs/reference/exec-session-provider.md`, observe.v1 rules 1-13) for `absent`/`unknown`/`dead`/`running` and the identity; agent liveness uses the process snapshot, and `last_activity` belongs to `inventory` (RPP-OBSERVE-015, passing).

**Cross-references.**
GCREX-8 — resolved the capability-naming issue; `observe.v1` is not in the baseline (`docs/REX-CONTRACT.md:438-442`)
GCREX-19.9 — is-attached comparison, the other op using the probe deadline
GCREX-19.10 — get-last-activity comparison

**Follow-ups.**
- Revisit only if gc changes the observe.v1 rules; the token is declared since GCREX-111 (gap).


### inventory

**Status** — implemented (GCREX-101, declared unconditionally since GCREX-111)

**Our implementation.** `inventory` is in the `ops` map (GCREX-101) and needs no token of its own: it is part of the `observe.v1` profile. `Provider.Inventory` (`internal/provider/observe.go`) lists the binding store and returns `{complete, sessions[]}`. Each entry carries the object, the state (`running`/`dead` from one Rex liveness probe per session) and, for a box the provider started, the start-config identity read from one sidecar snapshot per session; a pane that took the name without Start is answered `running` like `observe`, with no inherited created time and `"identity":{}` (`TestInventoryRestoredPaneLeavesBindingAlone`), and the binding store is left untouched. gc's `internal/runtime/exec/inventory.go` `inventoryEntry` reads a null identity as unknown (`IdentityKnown` false) and `{}` as a known empty identity, and drops an entry whose `state` is missing or unreadable, so an entry this provider lists without a state is invisible to gc rather than read as unknown. `dead` entries report `agent:"dead"`, a known fact rather than the running agent state the profile wants. Attachment and restart flags are absent. Each running entry carries `last_activity` (RPP-OBSERVE-015) from the provider's activity store, floored by the binding's start time so a box gc has not polled yet still answers one (`TestInventoryLastActivityFromStore`); a record written before its pane exists (a start in flight) is omitted, because Rex rejects an empty block id and the name is not running yet (`TestInventoryOmitsAProvisionalRecord`).

`complete` is `true` only when the binding store is a `Lister` and no city pane names a session no binding lists; a pane whose liveness cannot be read (or a pane listing that fails) makes it `false`, and a non-listable store answers `false` with an empty list. That is rule 7: `complete:true` needs proof nothing else holds a name, and the pane label is the only evidence Rex gives. A pool seat's short pane label names its session as `identityMatch` reads it, so a listed pool seat keeps the listing complete (`TestInventoryCompleteForPoolShortFormPane`).

The closest RPP v0 op stays `list-running [prefix]` (`cmd/gc-runtime-rex/main.go`, `opListRunning`), which prints one session name per line and nothing else.

**herdr-core.** `inventory` is registered with `INVENTORY_STDOUT_CAP` (`ops_table.py:73`), which is 8 MiB (`rpp/ops.py:34`). `op_inventory` (`herdrcore/reads.py:197-224`) lists the city's records and returns `{"complete": bool, "sessions": [...]}` (`rpp/wire.py:247-256`). Each entry has `name`, `state` (running or dead), `box`, `agent`, `attached`, `last_activity`, `restart_requested` and `identity` (`reads.py:227-258`, `rpp/wire.py:200-210`). It reads screens for agents that never reported OSC 7501 (`reads.py:159-195`), and it garbage-collects stale metadata (`reads.py:162-168, 220-223`). A stopped city returns complete and empty. An unreachable core returns incomplete and empty. Unreadable names set `complete=false` (`reads.py:203-218`). `list-running` in herdr uses the same cap and returns only names of intact sessions (`ops_table.py:76`, `reads.py:307-319`).

**Deviations.**

| # | Aspect | Rex provider | herdr-core | Verdict | Rationale |
|---|---|---|---|---|---|
| 1 | op presence | returns `{complete, sessions[]}` and the handshake declares `observe.v1` | Returns `{complete, sessions[]}` records (`reads.py:197`) | partial | Not one of the RPP v0 17 rows. gc 1.5.1 ignores the token and never sends the op. |
| 2 | bulk state read | one call returns state, identity and last activity per session; no attachment or restart flag | One call returns state, attachment, activity and identity per session | partial | gc 1.5.1 uses per-session `is-attached`, `get-last-activity`, `process-alive` and `get-meta` instead (`exec/seams.go:229-236`). Attachment remains the gap; activity (RPP-OBSERVE-015) is answered from the store. |

**Verdict.** partial. `inventory` is an optional extension in the `exec:` provider operation list (`docs/research/rex-session-provider.md:129-133`) and is not in RPP v0 or the 1.5.1 baseline (`docs/REX-CONTRACT.md` §9). herdr-core's handshake advertises `observe.v1` and `carrier.ops` (`handshake.py:34-41`), and `inventory` belongs to that set. Verified in gc 1.5.1: `inventory` is never sent, and there is no capability token for it in that baseline. gc's "runtime inventory" in `gc stop` is `list-running` with completeness inferred from the error (`cmd/gc/cmd_stop.go:627-646`), and per-session state comes from `is-attached`, `get-last-activity`, `process-alive` and `get-meta` (`exec/seams.go`). The op and `observe.v1` are declared unconditionally since GCREX-111, and gc 1.5.1 ignores both.

**Cross-references.**

GCREX-4 — covers per-session `is-attached`, `get-last-activity` and metadata, which `inventory` would batch
GCREX-19.1 — handshake; without `observe.v1` gc should not expect `inventory`

**Follow-ups.**

- Default path: no action: gc 1.5.1 never sends `inventory` (verified).
- GCREX-101 added the op, GCREX-105 completed it, GCREX-111 made the declaration unconditional. RPP-OBSERVE-007 (completeness) and -015 (`last_activity`) pass; the handshake also declares `city-scoped`, which RPP-OBSERVE-014 verifies.

**Cross-references (implementation).**
GCREX-101 — the tracer: handshake token, `observe`, `inventory`, `kill-object`
GCREX-105 — completes it: `last_activity` from the activity store, provisional records skipped, `city-scoped` in the handshake


### kill-object

**Status** — implemented (GCREX-101, declared unconditionally since GCREX-111)

**Our implementation.** `kill-object` is in the `ops` map (GCREX-101) and needs no token of its own: it is part of the `observe.v1` profile. `opKillObject` decodes `{object:{id,created}, mode}` and calls `Provider.KillObject` (`internal/provider/observe.go`). It rejects a missing object id or created, and any `mode` other than `corpse` or `zombie`. The whole check-then-kill runs under the name lock. It never kills by name: no binding, or an id/created mismatch, answers `gone`. A pane that took a vanished record's name therefore answers `gone` to an exact-object kill for that pane, because `observe`/`inventory` do not adopt it and the re-check compares against the recorded box only (`TestKillObjectGoneForRestoredPane`); accepting the rescan's pane would make the kill follow the name, which is what the op exists to prevent. `corpse` mode kills only a box nothing runs in and no client is attached to, and answers `live` otherwise, so gc's believe-dead-then-recheck kill cannot destroy a box that came back or close the pane an operator has open (`TestKillObjectCorpseLiveWhenAttached`); an unreadable attachment is an error, not a kill. `zombie` mode takes the same re-check and kills only a running box whose agent is definitely dead: it reads the box's foreground pid and `GT_PROCESS_NAMES`, expands the names, snapshots the process table once (`snapshotAgent`, RPP-OBSERVE-006), and refuses to kill when the request's `agent_pid` is still a live process outside the box's subtree. `agent_pid` is the matched agent `Observe` reported, which may be a descendant of the foreground root, and a reported pid that has since exited does not block the kill (`TestKillObjectZombieKillsWhenAgentPIDIsADescendant`, `TestKillObjectZombieChangedWhenAgentPIDIsAnotherBox`). A live or unreadable agent, a missing `GT_PROCESS_NAMES`, or a pane whose program already exited answers `changed` (`TestKillObjectZombieLeavesLiveAgentBox`, `TestKillObjectZombieChangedWhenSnapshotUnreadable`, `TestKillObjectZombieChangedWithoutProcessNames`, `TestKillObjectZombieChangedWhenAgentPIDDiffers`); a dead agent's box is killed (`TestKillObjectZombieKillsDeadAgentBox`); an attached one answers `changed` (`TestKillObjectZombieChangedWhenAttached`). The snapshot can only prove death, never revive. A killed box is removed by closing exactly the binding that was re-checked (`stopBinding`, the same close `stop` performs, under the state lock), never by a fresh resolve of the name, so a pane that took the name's labels after the recorded one vanished is left alone (`TestKillObjectDoesNotCloseRecreatedPane`); the binding, meta and identity are cleared and the record does not linger for gc's recycle loop.

Rex has no process-tree kill, so the kill is the pane close `stop` performs (the `stop` row's survivor handling applies).

**herdr-core.** `op_kill_object` (`/tmp/gascity-packs/runtime-herdr-core/assets/gc_runtime_herdr_core/herdrcore/lifecycle.py:542-557`) is registered in `ops_table.py:71` with `stdin=True`. It parses stdin via `parse_kill_object_request` (`rpp/wire.py:276-287`) into `{"object": {"id", "created"}, "mode": "corpse"|"zombie", "agent_pid"?}`; invalid input is an op error. Under `ctx.name_lock(name, shared=False)` it calls `_kill_object` (`lifecycle.py:560-592`), which re-checks that the box still exists with the same `created` and session label, then: in `corpse` mode it refuses if the box is intact or attached (`"live"`); in `zombie` mode it kills only on a definite `dead` state and refuses if intact, attached or unreadable (`"changed"`). Kill is `kill_incarnation` (TERM then KILL over the whole process tree, `lifecycle.py:67-80` module doc), then `forget`. Answers are `KillOutcome` scalars `killed|gone|renamed|live|changed` (`rpp/wire.py:138`). Documented in `README.md:188` (`kill-object corpse` removes a dead box) and `README.md:356`. Tests: `tests/unit/test_herdrcore_lifecycle.py`, `test_herdrcore_treekill.py`.

**Deviations.**

| # | Aspect | Rex provider | herdr-core | Verdict | Rationale |
|---|---|---|---|---|---|
| 1 | op presence | implemented and stdin-driven, declared through `observe.v1` | implemented, stdin-driven (`ops_table.py:71`) | partial | Not one of the 17 RPP v0 rows; extra. |

**Verdict.** partial. `kill-object` is outside the RPP v0 table, and gc 1.5.1 has no `kill-object` op, method or capability anywhere in the tree (whole-tree search, zero hits), so the gap is inert. Rex has no process-tree kill, so the kill is the pane close `stop` performs. The only mention in our repo is the escape-hatch list in `docs/research/rex-session-provider.md:129-133`.

**Cross-references.**
GCREX-2 — stop semantics, the closest offered behavior (`stop`).
`docs/REX-CONTRACT.md` §9 — RPP v0 op table and exit codes; `kill-object` not listed.
`docs/research/rex-session-provider.md:133` — lists `kill-object` among exec operations.

**Follow-ups.**
- Default path: no action: gc 1.5.1 never sends `kill-object` (verified).
- GCREX-101 added the op, GCREX-111 made the declaration unconditional. RPP-OBSERVE-010 passes in `make conformance`.
- `zombie` is a one-shot liveness read: a process that dies between the snapshot and the close is killed anyway, because nothing revives inside a closing box.


### server-stop

**Status** — not implemented

**Our implementation.** `server-stop` is absent from the `ops` map (`cmd/gc-runtime-rex/main.go:175-193`) and from `unimplemented` (`main.go:161-165`). A call falls through to the unknown-op branch (`main.go:228-235`): it writes `unknown operation "server-stop"` to stderr and exits 2, which gc treats as success for non-start ops. Stdin is not read. The handshake declares `report-attachment`, `report-activity`, `idle-boundary`, `carrier.ops`, `input-outcome`, `observe.v1` and `city-scoped` (`main.go:42-46`), so `server-lifecycle` is not declared. The code never calls `server.stop`; the decision is recorded in `docs/plans/native-rex-provider.md:83` (D3, "does not implement a server-teardown op") and `:438` ("Server lifecycle. Not implemented (D3)"), and in `docs/REX-CONTRACT.md:400` (§11 D3, "must not be called").

**herdr-core.** `op_server_stop` (`herdrcore/server.py:88-109`) stops the city's core (`server.stop`, `herdrcore/server.py:134`, `herdrcore/commands.py:209`) and ends every session the pack started for the city, under each name's lock (`server.py:116-138`, `_end_session` at `server.py:204-223`). It only touches terminals labelled with its city id (`server.py:112-113`, `:121`). Foreign terminals (not started by the city) keep the core running, and the op returns 0 (`server.py:127-132`). It refuses with exit 1 when a record's metadata cannot be read, when the core still answers after stop, or when the core or its processes survive (`server.py:107-108`, `:226-248`). Lock timeout on the kick lock gives exit 1 (`server.py:104-106`). It takes no stdin and writes empty stdout; its message string is the warning or failure text (`server.py:109`, `rpp/ops.py:69-79`). Exit codes are 0 or 1 only. The pack declares `server-lifecycle` for this op (`herdrcore/ops_table.py:5-6`, `:90`), which is not in the gc 1.5.1 baseline.

**Deviations.**

| # | Aspect | Rex provider | herdr-core | Verdict | Rationale |
|---|---|---|---|---|---|
| 1 | op presence | absent; exit 2 unknown op (`main.go:228-235`) | implemented; stops core and city sessions (`server.py:88`) | intentional | D3: shared Rex server must never be stopped (`native-rex-provider.md:83`, `REX-CONTRACT.md:400`) |

**Verdict.** `intentional`. Verified in gc 1.5.1: there is no wire op at all. Server teardown is an optional Go interface, `runtime.ServerLifecycleProvider` (`internal/runtime/runtime.go:533-547`), implemented only by the tmux provider, and `gc stop` does nothing for a provider without it (`cmd/gc/cmd_stop.go:464-473`). Exit-2 handling is therefore moot. The op is not in the RPP v0 17-row table and `server-lifecycle` is not in the pinned baseline (`docs/REX-CONTRACT.md` §9, "Not in the pinned baseline — declare only alongside the behavior it gates"). Rex has one shared server used by the user's other work, so stopping it would be destructive. herdr-core's core is per-city, so its stop is scoped; Rex's is not. Exit 2 is the correct RPP response to an op we do not implement.

**Cross-references.**
GCREX-1 — §9 pins the op table and records `server-stop` and `server-lifecycle` as outside the baseline.
GCREX-4, GCREX-65, GCREX-31 — teardown closes blocks (`block.close`); `session.destroy` runs only for this city's empty canonical `gc:<city>` session, behind the fail-closed empty check, never for the shared server or another session; never `server.stop`.
GCREX-8 — removed `server-lifecycle` from the plan's capability list.
`docs/REX-CONTRACT.md` §9 (not-in-baseline list, ~line 255-262) and §11 D3 (line 400).
`docs/plans/native-rex-provider.md` §0 D3 (line 83), and Known gaps "Server lifecycle" (line 438), "Shared server" (line 644).
`docs/research/rex-session-provider.md:150, 286, 314` — earlier research that proposed `server-lifecycle`; rejected.

`docs/plans/city-owned-rex-server.md` — GCREX-30 experiments on `rex server run`. A city-owned server makes this op implementable behind an ownership record (state dir, instance id, pid). The note recommends a staged revision of D3. The default stays the shared server and this op stays absent.

**Follow-ups.** None. Do not declare `server-lifecycle` or add a `server-stop` handler. City teardown stays on `stop` (`block.close`, then the guarded removal of an empty canonical session).


### copy-to

**Status** — not implemented

**Our implementation.** `copy-to` is absent from the `ops` map (`cmd/gc-runtime-rex/main.go:81-97`) and from the `unimplemented` set (`cmd/gc-runtime-rex/main.go:74-80`). A call falls through to the unknown-op branch (`cmd/gc-runtime-rex/main.go:115-122`): stderr `unknown operation "copy-to"`, exit 2, nothing on stdout. No file is copied and no state is touched. The closest op in our table is `peek` (`cmd/gc-runtime-rex/main.go:90`, `opPeek` at `cmd/gc-runtime-rex/main.go:243-258`, `PeekLines` at `internal/provider/provider.go:457-467`). Peek reads the agent's rendered pane text. It does not touch files, so it is a name match, not a functional equivalent.

**herdr-core.** Registered as `"copy-to": spec(screen.op_copy_to)` with the default deadline (`herdrcore/ops_table.py:89`). `op_copy_to` (`herdrcore/screen.py:47-64`) takes `<name> <src> [rel-dst]` from argv, with no stdin. It needs a live record (`herdrcore/reads.py:281-284`). The work directory is the stored `work_dir`, or the record's `cwd` if none (`screen.py:56-59`). It copies the file or tree `src` into that directory at `rel-dst` via `stage_path` (`herdrcore/staging.py:216-235`). It never writes through a symlink (`staging.py:108-135`, tested at `tests/unit/test_herdrcore_staging.py:260-277`). A missing `src` is a silent no-op with exit 0 (`staging.py:221-222`). A source that is already the destination is skipped (`staging.py:230-231`). Success is exit 0 with empty stdout (`screen.py:64`). Any `OpError` or staging/OS error exits 1 with `copy-to: <reason>` on stderr (`screen.py:50-63`; tests `test_herdrcore_staging.py:240-243`, `274-275`). The op never reads or copies the screen or scrollback. It is a file-staging op, not a read op.

**Deviations.**

| # | Aspect | Rex provider | herdr-core | Verdict | Rationale |
|---|---|---|---|---|---|
| 1 | op presence | No `copy-to` entry; exits 2 as unknown op (`main.go:115-122`) | Implemented: copies `src` into work dir, exit 0/1 (`screen.py:47-64`) | not-implemented | RPP v0 does not list `copy-to`. Whether gc 1.5.1 calls it is unverified (see Verdict). |

**Verdict.** `not-implemented`. `copy-to` is not one of the 17 RPP v0 rows (`docs/REX-CONTRACT.md` §9), so it is outside the baseline and treated as optional. Verified in gc 1.5.1: the op exists (`exec/exec.go:795`) but has no production caller; file staging for exec packs goes in the `start`/`provision` JSON as `copy_files` (`exec/json.go:41`). The exit-2 silent-success path only matters to a caller that does not exist. Not a gap for 1.5.1; the real staging gap is `copy_files` in `### start`. `peek` does not cover this.

**Cross-references.**
GCREX-19 — parent; requires each absent op to be marked not-implemented with its optional status.
GCREX-19.1 — protocol handshake; `copy-to` is not a declared capability in either implementation.
docs/REX-CONTRACT.md §9 — RPP v0 op table; `copy-to` is not among the 17 rows, and the driving-op and `exec` text (`REX-CONTRACT.md:224-228`) does not mention it.

**Follow-ups.**
- Implement `copy_files` staging in `start` (see `### start` row 4); `copy-to` itself has no gc 1.5.1 caller (no action).

## Rex-specific extension: the `idle` snapshot (GCREX-63)

This section is not a herdr-core comparison: herdr-core has no idle-snapshot
op. It records how a Rex city can answer gc's point-in-time pane-idle snapshot,
the extension the herdr-only decision table calls `idle-boundary`
(`docs/herdr-extensions.md`).

**The gap.** gc's pool execution backstop and idle nudge have two guards against
draining or nudging a live seat: the coarse activity clock and, when its activity
is already stale, a point-in-time snapshot that vetoes the clock. The snapshot is
only available when the routed backend implements `runtime.IdleSnapshotProvider`
(`internal/runtime/runtime.go:757` in the working tree, ahead of the `v1.5.1`
pin). tmux implements it (`internal/runtime/tmux/adapter.go:760`, tri-state:
`(false, nil)` is the busy reading and a pane showing neither a busy indicator
nor the ready prompt is `runtime.ErrIdleIndeterminate`). gc's exec backend does
not: `seamBackedProvider` lists `IdleWaitProvider` and `IdleProbeBudgetProvider`
and not `IdleSnapshotProvider` (`internal/runtime/exec/cutover.go:46-48`), so for
a Rex city `snapshotHoldFor` returns `""`
(`cmd/gc/execution_backstop.go:493`) and the activity clock decides alone. That
is the failure that drained a working worker on 2026-11-19
(`execution-claim-nudge: … unexecuted after 0/3 nudge attempts`).

**The seam: an op, gated by a token.** We hold both halves of the answer already:
`peek` returns pane content, and the provider reads the pane program's
`program_status` records for `get-last-activity` (`internal/provider/activity.go`)
and nudge busy-gating (`internal/provider/nudge.go`). GCREX-63 exposes them as a
new RPP op, `idle <name>`, answering one word — `idle`, `busy` or
`indeterminate` — with exit 0 for every state and exit 1 when the read could not
be made (a failed read is never `idle`). The op is gated by the `idle-boundary`
capability token (`internal/runtime/protocol.go:60` in the working tree), the
same token gc's exec backend already uses for `WaitForIdle`
(`internal/runtime/exec/idle.go:36`). The reading is the point-in-time sibling
of that wait; the token is the idle-observation gate and the op is the reading.

**Baseline behaviour is unchanged.** gc 1.5.1 has no `idle-boundary` token
(`internal/runtime/protocol.go` at the pin declares only the six baseline
strings) and ignores unknown capability strings, so the added token changes
nothing for a `v1.5.1` city. It is also inert on gc's working tree until a
carrier op is declared: `idleBoundaryDeclared` additionally needs `carrier.ops`
or `proc.exec` (`internal/runtime/exec/idle.go:36`), neither of which this
change declares. GCREX-28.2 declares the carrier; a gc that adds a `SnapshotIdle`
on the exec backend then reads the op where the tmux leaf is read
(`cmd/gc/execution_backstop.go:493`, the snapshot read at `:498`,
`internal/runtime/program_status_shadow_fresh.go:101`,
`cmd/gc/idle_nudge.go:673`).

**Cross-references.**
GCREX-63 — this extension.
docs/herdr-extensions.md — the `idle` design section and the `idle-boundary`
verdict, moved from *defer* to *implement*.
docs/REX-CONTRACT.md §9 — RPP v0 op table; `idle` is beyond the 17 rows, like
`clear-scrollback`, `send-keys` and `watch-startup`.

## Rex-specific extension: the judged program-status reading (GCREX-64)

This section is not a herdr-core comparison: herdr-core has no program-status
op. It records how a Rex city answers gc's optional `runtime.ProgramStatusProvider`,
the judged agent reading gc reads beside its own screen scrape.

**The gap.** gc reads a session's judged agent state through
`runtime.ProgramStatusProvider` (`internal/runtime/program_status.go:184` in the
working tree, ahead of the `v1.5.1` pin). The interface is a working-tree
feature: `internal/runtime/program_status.go` does not exist at `v1.5.1`, and
`git grep ProgramStatusProvider v1.5.1 -- '*.go'` finds nothing, so a pinned
city has no such reading at all. On the working tree only tmux implements it
(`internal/runtime/tmux/program_status_read.go:207`); the exec backend's
`seamBackedProvider` lists `IdleWaitProvider` and `IdleProbeBudgetProvider` and
no `ProgramStatusProvider` (`internal/runtime/exec/cutover.go:42-60`), and the
exec package names `ProgramStatus` nowhere. Consumers that lose the reading
today:

- the doctor capture check (`cmd/gc/doctor_program_capture.go:65`, `:69`);
- the capture heal lane (`cmd/gc/program_capture_heal.go:127`, fed by the
  leaf's judged inventory reading, `internal/runtime/tmux/program_status_read.go:264`);
- the starved-delivery hold in the conversation lane
  (`cmd/gc/busy_delivery_starvation.go:103`, through
  `runtime.ProgramRecordTypingHold`, `internal/runtime/program_status_hold.go:80`);
- the worker phase (`internal/worker/program_status.go:12`, `ProgramStatusOf`).

**The record is already read.** The provider reads the pane program's OSC 7501
`program_status` records for `get-last-activity` (`internal/provider/activity.go`)
and for nudge busy-gating (`internal/provider/nudge.go`, which already
distinguishes working from blocked). GCREX-64 exposes them as a new RPP op,
`program-status <name>`, one JSON line carrying the judged reading.

**The reading's shape.** `ProgramStatusReading`
(`internal/provider/programstatus.go`) carries `validity` and, only when valid,
`state` (`idle`/`working`/`done`/`blocked`/`error`) and `kind` (`blocked` with
`permission`/`question`/`auth`), plus `since`/`updated` (the newest record's
`UpdatedAt`) and `reported`. It is a reduced `runtime.ProgramStatus`: because Rex
captures OSC 7501 itself, the provider has the program's record but not gc's tap
nonce, pipe liveness or capture arm, so it cannot judge gc's `silent`, `cleared`
or `reader_lost` states and never claims them. The validities it does produce are
`valid`, `not_armed` and `exited`; `unread` is gc's switch-off zero value and the
provider is not consulted then.

**not_armed and a missing session.** A live pane whose newest record is absent,
empty or not a known root word reads `validity: not_armed` (there is no root
record to judge). A session with no pane is `ErrSessionNotFound` (exit 1); a Rex
transport failure is returned as-is (exit 1), which a consuming exec adapter maps
to `ErrRuntimeUnavailable`; a pane whose program has exited reads
`validity: exited` and keeps no state. A failed read is never a reading. The
provider resolves the binding fresh and holds no reading across calls, so a
stale reading cannot be served; a consumer bounds a live record's age with gc's
`ProgramStatusHoldLimit` (`internal/runtime/program_status.go:123, 10 min`) as it
does for tmux.

**The capture arm is out of scope.** gc resolves `ProgramStatusArm` from config
(`internal/config/program_status.go:90, 108`) and carries it on the launch
config (`internal/runtime/runtime.go:1621`), and only tmux consumes it
(`internal/runtime/tmux/adapter.go:1574`, `:1721`). The exec start config does
not carry it — `internal/runtime/exec/json.go:27-51` has no `program_status`
field — and the exec backend implements no `HealProgramCapture`
(`ProgramCaptureHealerFor`, `internal/runtime/program_status.go:260`, resolves
the tmux leaf only). So none of the arm's flags can reach a Rex pane, and each is
out of scope for a reason:

- **Capture / App / Relay.** Rex captures OSC 7501 itself and stores the records;
the provider arms no pane tap and wraps no relay, so there is nothing to gate and
nothing for these flags to select. The reading is available without them.
- **HoldMidTurn.** gc's flag arms the tmux pane and the conversation lane reads a
fresh record to veto a mid-turn typing. Rex's nudge already refuses a working or
blocked pane unconditionally (`internal/provider/nudge.go`), which is the
behaviour the flag drives, so the flag itself is out of scope as an arm.
- **DecideStartup.** gc's flag is the tmux startup ladder. The provider's own
startup readiness already waits on the program's `idle` record
(`internal/provider/nudge_delivery.go`), so the flag is out of scope as an arm.
- **DecideSettle.** The provider implements no interrupt idle-wait to decide, so
the flag is out of scope.

**No capability token.** gc's working tree defines no program-status or capture
token (`internal/runtime/protocol.go:18-118` lists none; the pin has none
either), so the op is exposed but undeclared and the handshake is unchanged
(`cmd/gc-runtime-rex/main.go:46`). A gc that adds an exec-side
`ProgramStatusProvider` and a token that gates it declares the token in that
change, per the gated-declaration rule in `docs/REX-CONTRACT.md` §9; until then
the op is inert on both the working tree and the pin.

**Cross-references.**
GCREX-64 — this extension.
docs/herdr-extensions.md — the program-status note; the extension carries no
herdr-only token decision.
docs/REX-CONTRACT.md §9 — RPP v0 op table; `program-status` is beyond the 17 rows,
like `clear-scrollback`, `send-keys` and `watch-startup`.
