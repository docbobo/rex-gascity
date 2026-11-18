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
called by gc and exit 2 there is a *failure*, and an empty `env` value must be
*unset*, not set to empty.

## Coverage

| Op | Status | Deviations | Verdict |
|---|---|---|---|
| `protocol` | implemented | 3 | intentional |
| `start` | partially implemented | 17 | intentional (with a mistake inside) |
| `stop` | implemented | 7 | intentional (with a mistake inside) |
| `provision` | not implemented | 1 | not-implemented |
| `exec` | not implemented | 1 | not-implemented |
| `is-running` | implemented | 5 | intentional |
| `process-alive` | implemented | 7 | intentional (with a mistake inside) |
| `list-running` | implemented | 6 | intentional (with a mistake inside) |
| `is-attached` | implemented | 6 | mistake |
| `get-last-activity` | implemented | 5 | intentional (with a mistake inside) |
| `peek` | implemented | 7 | intentional (with a mistake inside) |
| `nudge` | implemented | 11 | intentional (with a mistake inside) |
| `send-keys` | implemented | 8 | intentional |
| `interrupt` | implemented | 5 | intentional |
| `clear-scrollback` | implemented | 6 | intentional |
| `watch-startup` | not implemented | 4 | not-implemented |
| `attach` | not implemented | 4 | mistake |
| `set-meta` | implemented | 5 | intentional (with a mistake inside) |
| `get-meta` | implemented | 3 | intentional |
| `remove-meta` | implemented | 5 | intentional (with a mistake inside) |
| `observe` | not implemented | 1 | not-implemented |
| `inventory` | not implemented | 2 | not-implemented |
| `kill-object` | not implemented | 1 | not-implemented |
| `server-stop` | not implemented | 1 | intentional |
| `copy-to` | not implemented | 1 | not-implemented |

## Summary of deviations

| Op | Aspect | Rex provider | herdr-core | Verdict | Rationale |
|---|---|---|---|---|---|
| `protocol` | capability set | 2 baseline tokens only | 10 tokens, 8 outside gc 1.5.1 baseline | intentional | Contract forbids non-baseline tokens; herdr targets newer gc. |
| `protocol` | locality field | absent | `"local"` | intentional | Baseline has no locality; we do not declare fs.host. |
| `protocol` | argument handling | answers before name check | via op table; name handling unverified | intentional | gc passes a name; both ignore it. |
| `start` | cancel and deadline | no SIGTERM handler, so rollback is skipped on cancel | SIGTERM cancel token, deadline-aware, `_undo` rollback | intentional (with a gap) | gc runs `stop` after a failed start, so the damage is contained. |
| `start` | config fields consumed | 6 of the 15 gc sends; rest ignored | 17 fields parsed | not-implemented | Verified: gc sends 15 fields, all omitempty; hooks and staging are a known v1 gap. |
| `start` | session_setup / session_live | silently ignored | run after launch as warnings | not-implemented | known v1 limit, no warning to gc |
| `start` | file staging | ignored | staged into work_dir before pre_start | not-implemented | known v1 limit |
| `start` | duplicate start | exit 1 "already running"; dead pane recycled | exit 1 "already exists"; 73 for fresh_only | intentional | gc matches both phrases; it never sends fresh_only. |
| `start` | dead incarnation recycle | closes dead Rex block only | kills tree, refuses while descendants survive | intentional | Rex has no process-tree API |
| `start` | identity meta seed | 8 keys incl. GC_AGENT, GC_ALIAS, GC_TEMPLATE | 4 keys incl. GT_PROCESS_NAMES | intentional | serves gc pending-create check (GCREX-13/18) |
| `start` | session naming and hierarchy | session/tab/pane from GC_RIG and name | hashed terminal name plus labels | intentional | Rex hierarchy and GCREX-11 alias contract |
| `start` | env application | `env K=V` argv prefix; empty values emitted as `env -u KEY` | spawn-file launcher; empty unsets, HOME added | intentional | Empty values now match RPP and gc's tmux adapter. |
| `start` | secret exposure | tokens visible in pane argv | 0600 spawn file unlinked before parse | intentional (limitation) | side effect of argv-prefix mechanism |
| `start` | pre_start env and limits | os.Environ plus env; no per-hook timeout | stripped env; 30 s per hook, cancel-aware | mistake | hanging hook runs until gc kills group |
| `start` | lock scope | city-wide flock across hooks and launch | per-name lock released around pre_start | mistake | slow hook stalls all city start/stop |
| `start` | partial-failure side effects | empty city session or closed dead pane may remain | pre_start precedes hc objects; `_undo` cleans | intentional | empty session reused; harmless |
| `start` | post-launch liveness check | success once Rex returns | waits for agent, fails if it exited | not-implemented | dying agent surfaces later via is-running |
| `start` | startup nudge | immediate write plus Enter; errors swallowed | readiness wait, retries, unconfirmed marker | intentional (with a mistake inside) | best effort by design; may type before ready |
| `start` | warnings channel | none | exit 0 plus warnings on stderr | not-implemented | ignored hooks invisible to gc |
| `start` | empty or malformed stdin | empty accepted as empty config | any non-JSON, incl. empty, fails | intentional | gc always sends JSON |
| `stop` | wait for exit / tree kill | Single block.close, returns on Rex ack | Kills and reaps process tree, then removes record | intentional (with a gap) | Rex owns pty teardown; descendant kill unverified |
| `stop` | survivor handling | No survivor outcome; exit 1 only on error | Exit 1, box and meta kept when descendants may survive | not-implemented | Follows from delegating teardown to Rex |
| `stop` | already-gone session | nil/exit 0 for unknown name or vanished pane | exit 0 for absent name | intentional | Same contract |
| `stop` | state cleaned | Identity meta keys and binding only | Whole meta file and activity removed | mistake | Stale user meta and activity may survive stop |
| `stop` | locking scope | One state-dir flock for all names | Per-name exclusive lock | intentional | Simpler; serializes concurrent stops |
| `stop` | cancellation | context.Background, no cancel handling | Cancel-aware pauses | gap | SIGTERM kills process anyway; unverified |
| `stop` | drained-worker diagnostic | writes `<state dir>/diagnostics/<name>.json` when the work dir has modified tracked files (before the pane closes; best effort, shared 3 s bound) | no equivalent | intentional | Leaves a trace when gc drains a worker that still held edits (GCREX-32); never fails or delays stop. |
| `provision` | op presence | Not implemented; exit 2, no proc.provision declared | Not implemented; exit 2, no proc.provision declared | not-implemented | Shared gap; gc only calls it with both capabilities declared, so exit 2 is unreachable today |
| `exec` | op presence | Not implemented; exits 2, `proc.exec` undeclared | Not implemented; exits 2, `proc.exec` never declared | not-implemented | Shared gap; gc falls back to driving ops (REX-CONTRACT §9) |
| `is-running` | server unreachable | Any client error exits 1, no stdout | `false` exit 0 if nothing of core runs | intentional | gc reads exit 1 and false alike as dead; no change needed. |
| `is-running` | liveness definition | Pane block exists and last_exit nil | Live record, pid label match, reason not broken | intentional | Backends expose different liveness data |
| `is-running` | no server start on read | Never starts Rex | Kicks core start when remnants exist | intentional | Rex is external; lifecycle not in RPP v0 |
| `is-running` | alias handling | Any alive alias-bound instance gives true | Newest record by exact name | intentional | Rex name-resolution model has aliases |
| `is-running` | side effects | Deletes stale sidecar bindings on read | Read-only apart from start kick | intentional | Sidecar self-heal; race safety unverified |
| `process-alive` | match scope | Rex foreground process plus ancestors | whole box process subtree | intentional | Rex API exposes only foreground and ancestors |
| `process-alive` | interpreter scripts | no node/bun script-argument matching | matches script arg after node/bun/npx/deno | mistake | gc rule; needs live probe of Rex argv0 |
| `process-alive` | missing pane | BlockProcess not-found returns error, exit non-zero | dead record answers false | mistake | gc reads both as dead; fix is for op correctness. |
| `process-alive` | cannot-tell answer | any Rex error exits 1; strict false on no match | empty stdout (alive) when unreadable or launching | intentional | gc reads an error as dead, so a transient Rex error can mark the agent dead. |
| `process-alive` | zombie / liveness gate | LastExit non-nil forces false | zombie never matches; intact-record gate | intentional | last_exit is Rex liveness truth (GCREX-4) |
| `process-alive` | stdin line splitting | split on newline only, trim later | splitlines then strip | intentional | same result for gc newline input |
| `process-alive` | matched fields | name, argv0, basenames of path and argv0 | basename of comm and argv0 | intentional | different process data model |
| `list-running` | aliases in output | Lists aliases only for non-empty prefix | Lists only the session label name | intentional | GCREX-18 claim identity; avoids phantom orphan beads |
| `list-running` | what counts as running | Block exists with no LastExit | Live, pid label matches, not broken | intentional | No pid-label reincarnation case in Rex |
| `list-running` | name filter | None | Drops empty, padded, control-char names | mistake | Bad name would corrupt gc's line parse |
| `list-running` | stdout cap | None | Fails over 8 MiB | intentional | Cap is only for inventory op in baseline |
| `list-running` | stopped/unreachable server | Rex error exits 1 | Parked city gives empty exit 0 | not-implemented | No park concept; Rex-down behavior unverified |
| `list-running` | scope source | Local binding store, probed per binding | City-label terminal list from core | intentional | City→session mapping; bindings record ownership |
| `is-attached` | attachment source | Rex ListClients plus SessionView active window | process snapshot plus attach-lock file | intentional | Rex has a server-side client registry |
| `is-attached` | meaning of attached | app client and rig tab active | hc attach process or lock on terminal | intentional | Rex tab tiles agents; coarser granularity |
| `is-attached` | unknown client kind | error exit 1 | n/a | mistake | gc reads exit 1 as not attached; fail-open on kill paths. |
| `is-attached` | deadline | 30 s default | probe 4 s | intentional | gc 1.5.1 baseline is 30 s |
| `is-attached` | unreachable backend | transport error exit 1 | proc/lock error exit 1 | intentional | same shape, different backend |
| `is-attached` | stop-safety use | stop does not consult it | stop skips kill if attached/unknown | intentional | gc gates attachment in the reconciler, never before stop. |
| `get-last-activity` | timestamp format | RFC3339 whole seconds UTC | RFC3339 UTC with milliseconds | intentional | Contract takes RFC3339. Precision not required. |
| `get-last-activity` | active program | Uses record UpdatedAt even if working/blocked | Returns now for working/blocked | mistake | Confirmed: gc idle-stops and escalation-kills on a stale stamp. |
| `get-last-activity` | input time | Pack input not tracked | max of status/change time and last input | mistake | Same idle and escalation paths read the stale stamp as idle. |
| `get-last-activity` | state locking | In-process mutex, last writer wins | flock with 0.5 s wait | intentional | One process per op. Race only shifts stamp slightly. |
| `get-last-activity` | reported-pane source | Queries Rex program_status each call | Reads cached state, no host query | intentional | Different carrier data sources. |
| `peek` | missing session | exit 0, empty stdout | exit 66 no screen, empty stdout | intentional | gc has no exit-66 handling; it checks is-running first. |
| `peek` | pane vanishes mid-call | Rex not-found becomes exit 1 | hc not_found becomes exit 66 no screen | mistake | missing-pane case is empty/0; ClearScrollback tolerates it already |
| `peek` | empty `lines` arg | empty string treated as 0 | empty string fails exit 1 | intentional | harmless; gc passes a number or nothing |
| `peek` | `lines`>0 row count and trailing newline | strips trailing newline before counting rows, restores it | trailing newline counts as a row; always appends newline | intentional | ours returns exactly N text rows; hc text ending unverified |
| `peek` | scrollback vs viewport | Rex format of block, bounded by scrollback | screen only, never scrolls | intentional | exact Rex format coverage unverified; needs live check |
| `peek` | output cap | pages 983040-byte replies, no total cap | exit 1 above 1 MiB | intentional | gc has no cap or truncation; the 30 s timeout is the bound. |
| `peek` | unreachable core/server | exit 1 with Rex error | exit 1, or 66 if city parked | intentional | we have no parked-city state |
| `nudge` | busy / program_status gating | none; writes over working or blocked pane | hc refuses: busy, pending_interaction | not-implemented | GCREX-16 open; program_status already read elsewhere |
| `nudge` | submit confirmation | send-key return only; no turn-start check | hc `started` flag: delivered vs delivered_unobserved | not-implemented | needs OSC 7501 idle→working; gc 1.5.1 ignores words |
| `nudge` | exit code when pane gone | exit 0 (bestEffort) | exit 80 `gone` | intentional | gc 1.5.1 treats nudge to dead agent as no-op |
| `nudge` | exit code when server down | exit 1 with Rex exec error | exit 80 `unavailable` | intentional | both fail for baseline gc; Rex exit code unverified |
| `nudge` | framing: text and submit | write then enter, two non-atomic calls | one atomic terminal.send | intentional | no atomic Rex primitive; retry may double-type |
| `nudge` | multi-line and control chars | verbatim write; no paste; behavior unverified | one JSON string to hc; unverified | not-implemented | needs live multi-line test; paste event unused |
| `nudge` | empty message | no-op exit 0 before pane lookup | sent like any text; hc reaction unverified | intentional | empty Enter would submit blank prompt |
| `nudge` | non-UTF-8 stdin | json.Marshal rewrites bytes to U+FFFD | rejected, nothing typed | mistake | silent prompt corruption |
| `nudge` | deadline and cancellation | no internal budget; kill can split write and Enter | send stops 5 s before deadline | mistake | leaves typed, unsubmitted text |
| `nudge` | retries / settle time | none | none for nudge op | intentional | behavior matches; Enter-ingest race unverified |
| `nudge` | activity bookkeeping | no input-time record | note_input stamps input time | not-implemented | low impact; screen change is seen anyway |
| `send-keys` | unknown key names | pass through; Rex errors, exit 1 | typed as literal chars, or rejected exit 80 | intentional | Rex send-key has no literal-text mode |
| `send-keys` | key vocabulary | no BTab/PPage/NPage; case-insensitive, F1-F24 | exact-case table with BTab/PPage/NPage, F1-F12 | intentional | BTab etc. likely fail in Rex |
| `send-keys` | modifier handling | any C-/M-/S- prefix, any base | prefix only on known name or single char | intentional | Rex validates final binding |
| `send-keys` | missing/dead session | exit 0 via bestEffort | gone, exit 80 | intentional | exit 80 not in gc 1.5.1 baseline |
| `send-keys` | zero keys | no-op exit 0 before lookup | looks up box first; gone if absent | intentional | fire-and-forget op |
| `send-keys` | failure handling | any Rex failure exits 1, no retry | unknown outcome exits 1, never retried | intentional | same effective result |
| `send-keys` | partial failure | one call; bad key may leave earlier keys typed | all keys validated before sending | intentional | atomicity unverified, needs Rex check |
| `send-keys` | activity side-effect | none | records input activity | intentional | Rex tracks activity natively |
| `interrupt` | mechanism | ctrl+c keystroke via Rex SendKeys | hc terminal.interrupt; signal vs key unverified | intentional | Rex has no signal API |
| `interrupt` | wait/confirm | none; returns after send | hc waits up to 3 s to settle | intentional | RPP v0 has no outcome contract |
| `interrupt` | no live program | exit 0, silent | exit 80 `gone` | intentional | gc 1.5.1 knows only exit 0/1/2 |
| `interrupt` | failure mapping | non-NotFound errors exit 1 | unsent exit 80; unknown-after-write exit 1 | intentional | Rex lacks written/unwritten distinction |
| `interrupt` | repeat/retry | single send; client retry unverified | never retried | unverified | Check Rex SendKeys retry code |
| `clear-scrollback` | what is cleared | Rex `clear` call on the pane | no-op after intactness check | intentional | Rex has clear; hc has no clearable scrollback. |
| `clear-scrollback` | missing session | exit 0 silently | exit 80 `gone` | intentional | gc 1.5.1 has no input-outcome; best-effort. |
| `clear-scrollback` | ended or dead program | NotFound ignored, exit 0 | exit 80 `gone` | intentional | Same reason; no outcome vocabulary. |
| `clear-scrollback` | unreachable / unknown outcome | any other error exits 1 | `unavailable` exit 80; lost reply exit 1 | intentional | Only exit codes 0/1/2 exist for us. |
| `clear-scrollback` | liveness pre-check | none, one Rex call | `terminal.get` before answering | intentional | One call suffices for a real clear. |
| `clear-scrollback` | peek and attached-client effect | unverified, needs Rex clear semantics | nothing cleared, none | no-impact | gc calls it only after a kill and discards the result. |
| `watch-startup` | op presence | exit 2, no output | NDJSON screen stream at 250 ms, exit 0 or 1 | not-implemented | Verified: exit 2 makes gc poll peek every 500 ms instead. |
| `watch-startup` | stdout format | none (exits 2) | one `{"content":...}` NDJSON line per screen change, 250 ms poll | not-implemented | Would need a Rex block-format poll loop; no streaming helper exists. |
| `watch-startup` | deadline and cancellation | not applicable | `start` class, ends 1 s early with exit 0, SIGTERM gives exit 1 | not-implemented | gc kills `watch-startup` outright; SIGTERM handling is optional. |
| `watch-startup` | startup/readiness detection | none; `ReadyPrompt` stored, never read | none in the op either; prompt matching stays in gc | intentional | Neither side uses the ready prompt inside `watch-startup`. |
| `attach` | op presence | exit 2, not implemented | execs `hc attach` with tty passthrough and attach flock | mistake | gc calls attach; exit 2 fails `gc session attach`. |
| `attach` | capability gating | does not declare `tty.attach` | op exists, `tty.attach` not declared either | intentional | Reserved and unread; gc calls attach without checking it. |
| `attach` | attach lock / viewer signal | `is-attached` derives viewers from `list_clients` and the active window | `attach` holds a flock that `is-attached` reads | intentional | Rex reports clients server-side; no lock needed. |
| `attach` | deadline | op never runs, so no deadline applies | `start` class: 120 s default, pre-exec only | intentional | Spec and gc agree: no deadline; the class bounds setup only. |
| `set-meta` | unknown session | Succeeds; Rex never contacted | Exit 1 `no session` | intentional | meta may precede the pane; Start reads it |
| `set-meta` | lifetime across stop | Only identity keys cleared on Stop | All meta deleted with incarnation | mistake | stale suspended or drain keys can leak into next start |
| `set-meta` | locking | In-process mutex, per-key files | Cross-process per-name flock | intentional | per-key files avoid read-modify-write |
| `set-meta` | size limits | No key or value cap | 1 MiB file read cap | intentional | unbounded values; low risk |
| `set-meta` | file hardening | 0600 temp, no symlink or owner check | 0600, O_NOFOLLOW, owner check | not-implemented | local single-user state |
| `get-meta` | empty key argument | empty value, exit 0 | exit 1, missing metadata key | intentional | gc never sends empty key |
| `get-meta` | backend failure | only corrupt local store errors | core unreachable or foreign terminal errors | intentional | Local sidecar store, no core |
| `get-meta` | metadata keying | by gc name via primary binding | by incarnation nonce of current record | intentional | Different state model; same result for unknown session |
| `remove-meta` | empty key | not rejected; deletes entry for empty key | rejects with "missing metadata key" | mistake | Cheap guard missing; gc never sends it. |
| `remove-meta` | cross-process locking | in-process mutex, per-key files, atomic unlink | exclusive per-name file lock, whole-file rewrite | intentional | Per-key files avoid read-modify-write races. |
| `remove-meta` | unknown/foreign session | always OK, no existence check | unknown OK; foreign terminal errors | intentional | Name-keyed meta independent of Rex. |
| `remove-meta` | absent key | no-op, writes nothing | rewrites meta file, may create it | intentional | Both exit 0; fewer side effects. |
| `remove-meta` | provider-owned key guard | none for GC_READY_PROMPT_PREFIX | none | intentional | Matches reference. |
| `observe` | op presence | exits 2, unknown op | JSON state answer, 4 s probe deadline | not-implemented | Not in RPP v0; gc 1.5.1 never sends it. |
| `inventory` | op presence | exit 2, unknown op | returns complete flag plus per-session records | not-implemented | Not in RPP v0; gc 1.5.1 never sends it. |
| `inventory` | bulk state read | `list-running` prints names only | one call returns state, attachment, activity and identity per session | not-implemented | gc 1.5.1 uses the per-session ops instead. |
| `kill-object` | op presence | exit 2 unknown op; no handler | stdin-driven box kill, corpse/zombie modes | not-implemented | Outside RPP v0; gc 1.5.1 never sends it. |
| `server-stop` | op presence | absent; exit 2 unknown op | implemented; stops the city core and its sessions | intentional | No wire op in gc; teardown is a tmux-only Go interface. |
| `copy-to` | op presence | No entry, exit 2 unknown op (`main.go:115-122`) | Copies src into work dir, exit 0/1 (`screen.py:47-64`) | not-implemented | No gc 1.5.1 caller; staging is `copy_files` in start. |
## Confirmed mistakes

The deviations marked `mistake` across all sections, deduplicated and ranked by
what they can do to a running city: user-visible failures first, then wrong
answers about liveness, then lost or corrupted input, then state that leaks
across incarnations, then hygiene. Every entry has been checked against gc
1.5.1 source; the two verdicts that flipped on that check are `attach` (from
conformant omission to mistake) and `is-running` (from mistake to intentional,
because gc reads a failure and `false` alike as dead).

| # | Where | Mistake | Consequence in gc 1.5.1 | Sections |
|---|---|---|---|---|
| 1 | `attach` | The op is not implemented and exits 2 | gc calls `attach` from `gc session attach` and `gc session new` and does not run it through the exit-2 mapping: the user sees `gc session attach: exit status 2` and exit 1 instead of a working attach (`exec/exec.go:196-201`, `cmd/gc/cmd_session.go:399`, `:508`, `:1554`) | §attach |
| 2 | `nudge` | Text and Enter are two calls with no internal deadline, and non-UTF-8 stdin is silently rewritten to U+FFFD by `json.Marshal` | gc retries a failed nudge from the queue (15 s, 5 attempts) or the idle backstop (3), re-typing the whole text, so an exit 1 after the write duplicates the prompt; bad bytes corrupt it silently | §nudge rows 5, 8, 9; §start row 15 |
| 3 | `get-last-activity` | No "working or blocked means now" rule, and no pack-input floor | gc idle-stops or sleeps a session whose stamp is older than the timeout, and the same value gates drain-escalation kills, progress-stall recycle and the claim backstop, so a silent but busy agent is treated as idle | §get-last-activity rows 2, 3 |
| 4 | `is-attached` | An unclassifiable client kind becomes `ErrAttachmentUnknown`/exit 1 | gc reads any error as "not attached", which is fail-open: config-drift restart, progress-stall recycle and the drain-ack escalation kill proceed against a watched pane | §is-attached row 3 |
| 5 | `process-alive` | `ErrNotFound` from `BlockProcess` is returned as an error instead of `false`; no interpreter-script matching, so `node agent.js` does not match `agent` | gc reads the error as dead, which is the intended answer, so this is an op-correctness fix rather than a gc-behavior fix; the interpreter gap can still make a scripted agent read dead | §process-alive rows 2, 3 |
| 6 | `peek` | A pane that vanishes between resolve and `BlockFormat` fails with exit 1, while an already-missing pane returns empty with exit 0 | gc sees a failed peek and skips its zombie forensics or surfaces an error; no state changes, so severity is low | §peek row 2 |
| 7 | `start` | `K=""` in the launch env sets an empty value where the protocol says "unset" (gc's tmux adapter uses `env -u`) | Benign for `GC_ALIAS` (gc reads it with `os.Getenv`), a contract deviation for `GC_CONTROLLER_TOKEN`, `BEADS_*`, `CLAUDECODE`, `CODEX_*` | §start row 9 |
| 8 | `start` | `pre_start` has no timeout, and one city-wide lock is held across hooks and launch | A hanging hook runs until gc's cancel and stalls every other start/stop of the city | §start rows 11, 12 |
| 9 | `stop`, `set-meta` | Stop clears only the identity keys; user and provider keys (`suspended`, `GC_DRAIN_ACK`, `GC_READY_PROMPT_PREFIX`) and last-activity survive the incarnation | A restarted agent of the same name can inherit stale flags and a stale activity stamp | §stop row 4; §set-meta row 2 |
| 10 | `list-running` | No readable-name guard when printing | A padded or control-character name would corrupt gc's line parse; low risk, names come from gc | §list-running row 3 |
| 11 | meta ops | `set-meta`/`get-meta`/`remove-meta` accept an empty key where herdr-core rejects it | gc never sends an empty key, so this is hygiene only | §remove-meta row 1; §get-meta row 1 |

Two earlier candidates were withdrawn after the gc check: the missing SIGTERM
handler in `start` (gc runs `stop <name>` after every failed or cancelled start,
so the damage is contained; it stays as a gap), and the missing
`BTab`/`PPage`/`NPage` keys in `send-keys` (gc's entire vocabulary is
`Enter`/`Up`/`Down`/`Escape`/`C-u`, all mapped).

## Gaps that are deliberate, not mistakes

Recorded so they are not re-litigated. Verified against gc 1.5.1, `observe`,
`inventory`, `kill-object`, `server-stop` and `copy-to` are never sent, so those
omissions are inert; `exec` and `provision` are never attempted without the
matching capability declared, so their absence is inert too; `watch-startup`
exiting 2 is handled by gc falling back to `peek` polling every 500 ms, and
`attach` is the one exit-2 omission that is user-visible (mistake 1).
`nudge` busy-gating and submit confirmation are open work under GCREX-16, and
the start-config gaps (hooks, file staging, post-launch liveness check, warning
channel) are recorded in `docs/REX-CONTRACT.md` §12 and `docs/rex-provider.md`.

## Operational risks that are not parity bugs

- **Rex outage reads as "everything is dead".** gc maps any `is-running`
  failure to `false` (`exec/exec.go:665-680`), so while the Rex server is
  unreachable every session looks dead and the reconciler starts replacements.
  There is no "unknown" channel for an exec provider in 1.5.1.
- **`rex` exit code 3 is overloaded in our client.** `internal/rex/cli/cli.go`
  maps *any* `rex` exit 3 to `ErrSessionNotFound`, and the driving ops treat
  that as success. If Rex ever uses exit 3 for a transport failure, a nudge,
  interrupt or clear against a down server would report success. Worth
  confirming with the Rex CLI.
- **Probe fan-out.** gc's seam layer makes one `IsAttached`/`GetLastActivity`/
  `ProcessAlive` call cost up to four provider invocations (two `is-running`
  probes first). Each extra Rex round trip is paid on every observation.

## Proposed follow-up work

Not implemented in this task. Each item closes a set of the sections above; the
numbering of the mistake table is what they refer to.

1. **Make `attach` succeed.** Answer `attach` with exit 0 and a hint ("open Rex
   and select the rig tab") or focus the Rex pane, then fix
   `docs/REX-CONTRACT.md` §9, which wrongly generalises "exit 2 is success" to
   `attach`. Closes 1.
2. **Input path hardening.** Bound the write→Enter window with a context
   deadline shorter than gc's 30 s; reject non-UTF-8 stdin before any write;
   keep `BTab`/`PPage`/`NPage` as optional hardening. Closes 2.
3. **Liveness answers.** Catch `ErrNotFound` in `PeekLines`; map `ErrNotFound`
   from `BlockProcess` to `false` in `ProcessAlive`; answer `true` from
   `is-attached` when a client kind cannot be classified; decide on
   interpreter-script matching after a live probe. Closes 4, 5, 6.
4. **Start robustness.** Emit `env -u KEY` for empty values; add a per-hook
   timeout; move `pre_start` out of the city-wide lock; add a SIGTERM handler
   so rollback runs before gc's forced kill. Closes 7, 8.
5. **Incarnation hygiene.** Clear all meta for the name (and the activity
   entry) on stop and on the no-binding path; reject empty meta keys; decide
   whether `Binding.ReadyPrompt` stays, since nothing reads it. Closes 9, 11.
6. **Activity truthfulness.** Return now for `working`/`blocked`; record pack
   input time in `nudge`/`send-keys` and use it as a floor. Closes 3.
7. **List output guard.** Skip names that are empty, padded or contain control
   characters. Closes 10.
8. **GCREX-16** (existing): `program_status` busy gating and submit
   confirmation for `nudge`.
9. **Rex-side probes and one CLI check.** Confirm that `rex` never exits 3 for a
   transport failure (see "Operational risks"); probe `block.method clear` and
   `format` coverage; probe a `node agent.js` pane's `argv0` for the
   interpreter-script decision.

### protocol

**Status** — implemented

**Our implementation.** `run()` handles `protocol` before the `ops` table lookup (`cmd/gc-runtime-rex/main.go:108`), so it needs no session-name argument. It prints the `handshake` constant plus a newline (`main.go:35`, `main.go:109`): `{"version":0,"capabilities":["report-attachment","report-activity"]}`. It reads no stdin, touches no state and has no `locality` field. It exits 0, or 1 if the stdout write fails (`main.go:110-112`). Both tokens are in the six-token gc 1.5.1 baseline (`docs/REX-CONTRACT.md:242-251`) and match the §9 example (`REX-CONTRACT.md:235`). The `main.go:33-34` comment says `proc.exec`, `proc.provision`, `proc.stream` and `tty.attach` are not declared. The default 30 s deadline applies (set by gc).

**herdr-core.** `ops_table.py:68` maps `protocol` to `handshake_op(HANDSHAKE)` (`rpp/ops.py:109-116`). It encodes once, reads no stdin (`reads_stdin=False`), uses the default deadline and a small stdout cap. `HANDSHAKE` (`handshake.py:33-47`) declares ten tokens: `observe.v1`, `report-attachment`, `report-activity`, `city-scoped`, `conformance-faults`, `server-lifecycle`, `carrier.ops`, `idle-boundary`, `input-outcome`, `fs.host`, plus `locality:"local"`. `Handshake.encode` emits `locality` only when it is set (`rpp/wire.py:61-65`). It deliberately omits `proc.exec`, `proc.provision` and `input-fence` (`handshake.py:13-15`). The `handshake.py` docstring says its `fs.host` needs a newer gc (gastownhall/gascity#9032/#9129) (`handshake.py:20-22`).

**Deviations.**

| # | Aspect | Rex provider | herdr-core | Verdict | Rationale |
|---|---|---|---|---|---|
| 1 | capability set | 2 tokens, both in the gc 1.5.1 baseline | 10 tokens; 8 are outside the baseline (`observe.v1`, `city-scoped`, `conformance-faults`, `server-lifecycle`, `carrier.ops`, `idle-boundary`, `input-outcome`, `fs.host`) | intentional | `REX-CONTRACT.md:253-262` says not to declare non-baseline tokens. gc 1.5.1 ignores unknown tokens. herdr targets a newer gc. We implement none of the matching features. |
| 2 | `locality` field | absent | `"local"` (needed by `fs.host`) | intentional | Baseline has no `locality` (`REX-CONTRACT.md:237`). We do not declare `fs.host`. |
| 3 | argument handling | `protocol` answers with no session name (`main.go:108`) | goes through the op table; whether it requires a name is unverified (need `rpp/perop.py`/`main.py`) | intentional | gc passes a name anyway. Both ignore it as far as the code shows. |

**Verdict.** intentional. Exit-code exceptions in gc 1.5.1: exit 2 is success only for ops it runs through its `run` helper (everything except `start` and `provision`); `attach` returns an error on any non-zero exit (`exec/exec.go:196-201`), `exec` reads exit 2 as "unsupported" unless `proc.exec` is declared and any other non-zero exit as the command's result (`:894-908`), and `watch-startup` reads exit 2 before its first line as "unsupported" (`:454-457`). Exit 66 has no meaning to gc: any code other than 0, 1 or 2 is a plain error. Also note the seam layer: in production gc wraps the exec provider in `seamProvider`, so one `IsAttached`, `GetLastActivity` or `ProcessAlive` call fans out to up to four ops (two `is-running` probes first, then `get-last-activity`, `is-attached`, `process-alive`), and when `is-running` fails the others are never called (`runtime/seam_adapter.go:88-159`, `exec/seams.go:229-236`). Our handshake declares only implemented baseline tokens. herdr's extras belong to its own provider features and a newer gc. Verified in gc 1.5.1: it parses exactly six tokens (`report-attachment`, `report-activity`, `proc.exec`, `proc.stream`, `tty.attach`, `proc.provision`; `internal/runtime/protocol.go:18-58`), ignores unknown strings by design (`protocol.go:15-16`, pinned by `exec/handshake_test.go:91-102`), and degrades to no capabilities on a malformed handshake (`exec/handshake.go:45-51`). Only three gates consume them: `is-attached` requires `report-attachment`, `provision` requires `proc.provision` plus `proc.exec`, and `proc.exec` decides what exit 2 from `exec` means. `get-last-activity` is called regardless of `report-activity`.

**Cross-references.**
GCREX-19.1 — this task
GCREX-19.4 — `provision` and `proc.provision` gating
GCREX-19.17 — `attach` and `tty.attach` gating
GCREX-19.16 — `watch-startup`
`docs/REX-CONTRACT.md` §9 — baseline capability table and "do not declare" list

**Follow-ups.**
- Re-check the `report-attachment` and `report-activity` tokens against `gc runtime check` when `is-attached` or `get-last-activity` change (gap).
- Add `proc.exec`, `proc.provision` and `tty.attach` only together with their ops (gap).


### start

**Status** — partially implemented

**Our implementation.** `start` is dispatched at `cmd/gc-runtime-rex/main.go:82` to `opStart` (`main.go:162`). It reads all of stdin; empty stdin gives a zero config, otherwise JSON is decoded into six fields: `work_dir`, `command`, `ready_prompt_prefix`, `env`, `pre_start`, `nudge` (`main.go:54-60`, `main.go:164-173`); the other nine fields gc sends (`lifecycle`, `process_names`, `ready_delay_ms`, `session_setup`, `session_setup_script`, `session_live`, `copy_files`, `overlay_dir`, `pack_overlay_dirs`) are ignored. `Provider.StartNew` (`internal/provider/provider.go:139-150`) takes the city-wide state flock (`lock.go:21`, 60 s wait, `lock.go:15`), refuses a live pane with `session "<name>" already running` (exit 1, `main.go:182-185`), then `start` (`provider.go:152-284`) does: resolve existing binding and close a dead pane (`:174-190`), find or create the `gc:<city>` Rex session (`:192-201`), run `pre_start` via `/bin/sh -c` with `os.Environ()` plus `env` (`:210-217`, `:660`), require `work_dir` to exist (`:222-226`), seed identity meta keys (`GC_SESSION_ID`, `GC_INSTANCE_TOKEN`, `GC_RUNTIME_EPOCH`, `GC_CONTINUATION_EPOCH`, `GC_AGENT`, `GC_TEMPLATE`, `GC_ALIAS`, `GC_PROVIDER`; `identity.go:65-105`) and a provisional binding (`:232-236`), launch with `NewSplit`/`NewWindow` (tab = `GC_RIG` else name prefix before `--`, `binding.go:77-84`; pane = agent part, `sessionname.go:11-30`) and record the final binding (`:256-283`). The env is applied by prefixing argv with `env K=V …`, only when a command exists (`provider.go:292-307`). The nudge is written plus Enter right after the launch returns, best effort, errors dropped (`:243-254`). On error after seeding, a deferred rollback clears the identity keys and forgets the binding (`:155-164`). The handler uses `context.Background()` (`main.go:138`) and installs no signal handler (no `signal` use in `main.go`); the 120 s deadline is enforced only by gc killing the process group.

**herdr-core.** `start` is `deadline="start"`, `stdin=True` (`herdrcore/ops_table.py:69`) and `op_start` (`herdrcore/lifecycle.py:763`) parses 17 start-config fields strictly; empty or non-JSON stdin fails (`rpp/wire.py:102-129`, `rpp/jsonval.py:20-26`). The transport installs a SIGTERM handler that sets a cancel token, polls stdin against the deadline, honours `GC_EXEC_DEADLINE_MS` and defaults to 120 s (`rpp/perop.py:50-57`, `:60-80`, `:129`, `:143-145`; `rpp/ops.py:23-27`). `_start` (`lifecycle.py:778-825`) takes a per-name lock, rejects a live box (`session "<n>" already exists`, or exit 73 when `fresh_only`, `:754-760`), recycles dead incarnations (killing the process tree and refusing while old descendants survive, `:488-506`), stages files (`herdrcore/staging.py:238`), runs `pre_start` with a 30 s per-hook limit (`:793-796`, `:616-624`, `:168`) outside the lock, then `_create` re-locks, rechecks the name, writes seed metadata (`GC_SESSION_ID`, `GC_RUNTIME_EPOCH`, `GC_INSTANCE_TOKEN`, `GT_PROCESS_NAMES`, `gc_bin`, `work_dir`; `herdrcore/metastore.py:88-91`, `rpp/wire.py:43`) and launches via a 0600 spawn file that the launcher unlinks before reading (`herdrcore/launch.py:30-48`). The agent env is the launcher env minus `HC_*`, with session values applied (`""` unsets) and gc's `HOME` added (`herdrcore/envplan.py:51-76`). Any failure or cancel in `_create` runs `_undo` (kill tree and token holders, remove spawn file, metadata; `lifecycle.py:887-891`, `:919-970`). After launch it waits for the agent (5 s, or 30 s with `process_names`, `:1004-1024`), fails with `the agent exited during startup` unless `lifecycle=one_shot` (`:1027-1036`), runs `session_setup*` as warnings (`:810-818`), then types the nudge after a readiness wait of up to 60 s (`:1051-1133`), recording `GC_STARTUP_DELIVERY_UNCONFIRMED` when not confirmed (`:1136-1146`). Warnings go to stderr with exit 0 (`:825`).

**Deviations.**

| # | Aspect | Rex provider | herdr-core | Verdict | Rationale |
|---|---|---|---|---|---|
| 1 | cancel and deadline | `context.Background()`, no SIGTERM handler (`main.go:138`; gc never sends a deadline env var); rollback defer (`provider.go:155-164`) does not run when gc kills the group | SIGTERM sets a cancel token, ops poll it, `_undo` rolls back within 1.5 s (`perop.py:50-57`, `lifecycle.py:181`, `:887-891`) | intentional (with a gap) | gc cancels with SIGTERM to the adapter's process group and force-kills after 2 s (`exec/exec.go:92`, `execgrace/execgrace.go:133-165`), then runs `stop <name>` itself unless `is-running` was literally `true` beforehand (`exec.go:262-270`). Our idempotent `Stop` clears the seed and closes the pane, so the net effect for gc is benign. Residual risk: an orphan pane if the process dies between the launch call and the binding record. |
| 2 | config fields consumed | 6 of the 15 gc sends; `lifecycle`, `process_names`, `ready_delay_ms`, `session_*`, staging fields ignored (`main.go:54-60`) | 17 fields (`wire.py:80-99`) | not-implemented | Verified: gc 1.5.1 sends 15 fields, all `omitempty` (`exec/json.go:23-39`): `work_dir`, `command`, `lifecycle` (only `"one_shot"`), `env`, `process_names`, `nudge`, `ready_prompt_prefix`, `ready_delay_ms`, `pre_start`, `session_setup`, `session_setup_script`, `session_live`, `pack_overlay_dirs`, `overlay_dir`, `copy_files`. gc never sends `fresh_only`. Hook and staging gap documented (`REX-CONTRACT.md:482-484`, `:508-509`). |
| 3 | `session_setup`, `session_setup_script`, `session_live` | silently ignored | run after launch as non-fatal warnings; `session_live` reported skipped (`lifecycle.py:810-824`) | not-implemented | Known v1 limit; no warning surfaces to gc. |
| 4 | file staging (`copy_files`, `overlay_dir`, `pack_overlay_dirs`) | ignored; nothing copied | staged into `work_dir` before `pre_start` (`lifecycle.py:789`, `staging.py:238-258`) | not-implemented | Known v1 limit (`rex-provider.md:265`). |
| 5 | duplicate start | live pane: exit 1 `session "<n>" already running` (`main.go:184`); dead pane closed and relaunched (`provider.go:179-190`) | live box: exit 1 `session "<n>" already exists`; exit 73 for `fresh_only` (`lifecycle.py:754-760`) | intentional | Both fail and recycle. Verified: gc lower-cases the start stderr and matches the substrings `already exists` or `already running` (`exec/exec.go:144`, `:185-193`), with no exit-code check, so both phrasings match and the difference is cosmetic. A collision makes gc skip its post-failure `stop` and re-observe the session via `get-meta`, so `GC_SESSION_ID`/`GC_INSTANCE_TOKEN` must match the pending create. gc never sends `fresh_only`, so exit 73 is unreachable from gc. |
| 6 | recycle of a dead incarnation | closes the dead Rex block only (`provider.go:186`); no process-tree reap, no refusal | kills tree and token holders, refuses start while descendants may survive (`lifecycle.py:488-506`) | intentional | Rex has no process-tree or token API. Descendants left by a dead pane are not handled: unverified. |
| 7 | identity meta seed | 8 keys incl. `GC_AGENT`, `GC_TEMPLATE`, `GC_ALIAS`, `GC_PROVIDER`, `GC_CONTINUATION_EPOCH`; no `GT_PROCESS_NAMES`, no `gc_bin` file (`identity.go:65-74`) | 4 keys incl. `GT_PROCESS_NAMES`; `GC_ALIAS`/`GC_AGENT` not seeded (`wire.py:43`) | intentional | Ours serves `get-meta` for gc's pending-create check (GCREX-13, GCREX-18); `GC_BIN` basename goes on the binding (`provider.go:373`). `process-alive` takes names on stdin, so no stored `process_names`. |
| 8 | session naming and hierarchy | `gc:<city>` session, tab from `GC_RIG` or `<rig>--` prefix, pane from agent part; aliases from `GC_TEMPLATE`/`GC_ALIAS`/`GC_AGENT`/`GC_RIG` (`binding.go:77-120`) | opaque name hashed to an hc terminal name plus `gc.*` labels (`naming.py:159-177`) | intentional | Reflects Rex hierarchy and GCREX-11 alias contract; herdr has no rig/tab concept. |
| 9 | env application | `env K=V …` argv prefix, only with a command; a bare shell gets no env (`provider.go:292-307`); empty values are emitted as `env -u KEY` | spawn-file launcher; `""` unsets, gc `HOME` added when absent (`envplan.py:14-16`, `:51-76`) | intentional | Rex launch options carry no env (`REX-CONTRACT.md:483-484`), so an argv prefix is the available mechanism. Empty values now match the protocol's "empty = unset" rule (`docs/reference/exec-session-provider.md` §Session environment) and gc's tmux adapter (`tmux/tmux.go:614-627`). The provider passes gc's identity through: for an unaliased pool seat `GC_ALIAS` stays empty and `GC_AGENT`/`BEADS_ACTOR` keep gc's session bead id. It does **not** synthesize a `GC_ALIAS` from `GC_TEMPLATE`: that GCREX-18 deviation was removed in GCREX-32 because it failed two ways. First, the work query runs its assigned and in-progress tiers over `GC_SESSION_ID`, `GC_SESSION_NAME` and `GC_ALIAS` (`internal/config/workquery.go:459`), so every seat probed the same identity and a second seat adopted the first seat's `in_progress` bead at once (`hookClaimExistingAssignment` / `certifyHookAdoption`, `cmd/gc/cmd_hook_claim.go:472-477`). Second, gc records the claim under alias first (`cmd/gc/cmd_hook.go:866`, `hookClaimAssigneeIdentity`), so claims landed under one string no open session bead answers to and `releaseOrphanedPoolAssignments` (`cmd/gc/pool_session_name.go:211`) reopened the work. |
| 10 | secret exposure | env values incl. `GC_INSTANCE_TOKEN` appear in the pane's argv (`provider.go:301-306`) | secrets pass via a 0600 file unlinked before parse (`launch.py:30-48`) | intentional (limitation) | Consequence of the argv-prefix mechanism; visibility in `ps` and Rex launch records: unverified. gc's own providers route non-allow-listed env through a private file, not argv (`internal/runtime/envsecret.go`). |
| 11 | `pre_start` env and limits | `os.Environ()` overlaid with `env`; no per-hook timeout; error carries full combined output (`provider.go:211-213`, `:638-684`) | gc env minus `HC_*`, `""` unsets; 30 s per hook bounded by op deadline and cancel; last output line only (`lifecycle.py:599-624`) | mistake | A hanging hook runs until gc cancels the call; the group gets SIGTERM and only the leader is force-killed after 2 s (`exec/exec.go:92`), so a hook's shell child can outlive the grace. |
| 12 | lock scope | one city-wide flock held across the whole start, including `pre_start` and the blocking launch (`provider.go:140-149`, `lock.go:21`) | per-name lock released before `pre_start`, re-taken and rechecked in `_create` (`lifecycle.py:782-787`, `:833-838`) | mistake | A slow hook or launch stalls every other `start`/`stop` of the city (60 s lock wait, `lock.go:15`). |
| 13 | partial-failure side effects | on `pre_start` or work_dir failure the Rex session `gc:<city>` may already be created and a dead pane already closed (`provider.go:186`, `:197`, `:210-226`) | `pre_start` runs before any hc object exists; `_undo` removes the created box (`lifecycle.py:793-797`, `:919-970`) | intentional | Empty city session is harmless and reused (`FindSession`, `provider.go:192-201`); the pilot accepted gc owning session lifetime (`rex-pilot-phase5.md:52`). |
| 14 | post-launch liveness check | success once Rex returns; no wait for the agent, no exited-during-startup failure | waits for agent processes and fails the start if it already exited (`lifecycle.py:1004-1036`) | not-implemented | An immediately dying agent surfaces later via `is-running`, not as a start error. Whether Rex's launch call errors on instant exit: unverified. |
| 15 | startup nudge | written plus Enter immediately after launch, no readiness wait, failures swallowed with no marker (`provider.go:243-254`) | waits up to 60 s for prompt, delay or settled screen, withholds on a startup dialog, retries, records `GC_STARTUP_DELIVERY_UNCONFIRMED` and warns (`lifecycle.py:1051-1146`) | intentional (with a mistake inside) | Best effort is by design (GCREX-14; gc queued-nudge backstop); typing before the TUI is ready can lose the prompt. `ready_prompt_prefix` is only stored on the binding (`provider.go:354-372`), not used to gate the nudge. |
| 16 | warnings channel | none; start prints nothing | exit 0 plus `start: <warnings>` on stderr (`lifecycle.py:825`, `perop.py:166-167`) | not-implemented | Ignored hooks, unconfirmed nudge and skipped staging are invisible to gc. |
| 17 | empty or malformed stdin | empty stdin accepted as an empty config, which starts a default shell (`main.go:169-173`); malformed JSON fails | any non-JSON, including empty, fails (`wire.py:102-129`) | intentional | gc always sends JSON; ours is more lenient, low risk. |

**Verdict.** intentional (with a mistake inside). The Rex mapping, identity seeding and idempotent recycle are deliberate and tested. The mistakes are operational: `pre_start` without a timeout (row 11) and a city-wide lock held across hooks and launch (row 12); the empty-env case (row 9) is a contract deviation. Missing signal handling (row 1) is a gap rather than a bug for gc, which runs `stop` after a failed start. The rest are known gaps (hooks, staging, liveness check, nudge readiness).

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

**Follow-ups.**
- Handle SIGTERM in `main.go` and run the rollback before exit, closing the window before gc's forced kill (gap; gc runs `stop` afterwards, so the damage is contained).
- Add a per-hook timeout bounded by the op deadline to `runPreStart` (mistake).
- Run `pre_start` outside the state lock, or use a per-name lock, and recheck liveness before launch (mistake).
- Emit `env -u KEY` for empty env values instead of `env KEY=`, as gc's tmux adapter does (`tmux/tmux.go:614-627`) (mistake).
- Return stderr warnings with exit 0 for ignored `session_setup*`/`session_live`, staging fields and an unconfirmed nudge (gap).
- Run `session_setup`, `session_setup_script`, and stage `copy_files`/`overlay_dir`/`pack_overlay_dirs` (gap).
- Gate the startup nudge on `ready_prompt_prefix` or a settled screen; record an undelivered nudge in meta (gap).
- Check that the agent is alive after launch and fail the start if it exited (gap).
- Keep `already running` in the live-pane error; gc matches it (`exec/exec.go:144`) (no action).
- Consider a launcher or file-based env hand-off so tokens stay out of pane argv (gap).


### stop

**Status** — implemented

**Our implementation.** Dispatch is `cmd/gc-runtime-rex/main.go:83` (one required arg) to `opStop` (`main.go:189`), which calls `Provider.Stop` (`internal/provider/provider.go:408`). `Stop` takes the exclusive state-dir flock (`internal/provider/lock.go:21`, 60 s timeout at `lock.go:15`), resolves the binding, and issues a single Rex `block.close` for the agent's pane (`provider.go:412-441`, `internal/rex/cli/cli.go:181`). It is idempotent: no binding returns nil after clearing seeded identity (`provider.go:426-429`), and a `rex.ErrNotFound` from the close is swallowed (`provider.go:433`; tests `provider_test.go:144`, `teardown_test.go:113,138`). Any other close error is returned and the binding is kept (`teardown_test.go:125`). Cleanup is limited to the identity keys in `startIdentityMetaKeys` (`identity.go:18-32`) and the binding (`binding.go:396`); other `set-meta` keys and the activity store are not touched. The op reads no stdin, writes nothing to stdout, exits 0 on success and 1 on error (`main.go:99-146`). It runs with `context.Background()` (`main.go:138`) and does not wait for the process to exit: it returns when Rex acknowledges `block.close`.

**herdr-core.** `op_stop` (`herdrcore/lifecycle.py:511`) takes the exclusive per-name lock, lists the city's records for that name, and calls `_recycle` (`lifecycle.py:420`). `_recycle` calls `kill_incarnation` per record (`lifecycle.py:327-368`): it snapshots the process table, captures the tree by instance token, asks hc to kill, reaps and rescans the token, and only then removes the record. Then `forget` drops the record's meta file and activity (`lifecycle.py:389-397`). If descendants may survive, the box and its meta stay and the op exits 1 with "descendants may survive" (`lifecycle.py:523-524`; `test_herdrcore_treekill.py:576`). A second stop on an absent name exits 0 (`test_herdrcore_lifecycle.py:118`). With the core unreachable and nothing running, it sweeps by stored pid/token and exits 0 without starting a core (`lifecycle.py:518-522,528`; `test_herdrcore_lifecycle.py:127`). An unreachable core with something still running is exit 1.

**Deviations.**

| # | Aspect | Rex provider | herdr-core | Verdict | Rationale |
|---|---|---|---|---|---|
| 1 | wait for exit / tree kill | Single `block.close`; returns on Rex ack; no process-table check (`provider.go:433`) | Kills and reaps the process tree by token, rescans, and removes the record only when clean (`lifecycle.py:348-367`) | intentional (with a gap) | Rex owns the pane's pty and teardown, so pack-side reaping is not in the contract. Whether `block.close` kills detached descendants is unverified; it needs a Rex-side check. |
| 2 | survivor handling / exit code | No "may survive" outcome; exit 1 only on close/transport error (`teardown_test.go:125`) | Exit 1 and the box and meta stay for a retry when descendants may survive (`lifecycle.py:523`) | not-implemented | Follows from row 1. gc retry semantics rely on this only in herdr. |
| 3 | already-gone session | nil/exit 0 for unknown name or vanished pane (`provider.go:426-429`, `:433`) | exit 0 for absent name (`test_herdrcore_lifecycle.py:118`) | intentional | Same contract. We also clear seeded identity when no pane exists (`provider.go:429`). |
| 4 | state cleaned | Only `startIdentityMetaKeys` plus binding; user meta keys and activity remain (`identity.go:18`, `provider.go:436-439`) | Whole meta file and activity entry removed (`lifecycle.py:389-397`; `test_herdrcore_lifecycle.py:124`) | mistake | A restarted session can read stale `set-meta` values and stale last-activity: `seedStartIdentity` clears only the identity keys (`identity.go:46`), so nothing on our side resets them, while herdr deletes the whole meta file with the incarnation. |
| 5 | locking scope | One state-dir-wide flock for all names (`lock.go:28`) | Per-name exclusive lock (`lifecycle.py:515`) | intentional | Simpler. Stops of different agents serialize, with a 60 s wait that can reach the 30 s op deadline. |
| 6 | cancellation | `context.Background()`, no signal handling in `run` (`main.go:138`) | Cancel-aware pauses (`ctx.request.cancel.pause`, `lifecycle.py:348`) | gap | gc sends SIGTERM to the process group and force-kills after 2 s (`exec/exec.go:92`), so partial effects between `block.close` and `Forget` are possible but bounded. |
| 7 | drained-worker diagnostic | Before `block.close` (or from the stored binding when the pane is already gone), if the binding's `WorkDir` is a git work tree with modified tracked files, writes `<state dir>/diagnostics/<name>.json` with worker, aliases, `GC_SESSION_ID`, changed files and the newest-mtime file (`internal/provider/drain_diag.go`, called at `provider.go:425` and `:432`); untracked files ignored, git runs with `GIT_OPTIONAL_LOCKS=0`; best effort, one shared 3 s bound | none | intentional | Added for GCREX-32 so a drained worker's lost edits are traceable. Skipped silently for a clean tree, non-git dir, empty work dir or empty state dir. |

**Verdict.** intentional (with a mistake inside). Fire-and-forget pane close is the right fit for the Rex mapping, and idempotence and exit codes match. Stop leaves user meta and activity behind, which herdr removes.

**Cross-references.**
GCREX-2 — start/stop/is-running/peek baseline
GCREX-12 — retired session whose Rex pane stays live
GCREX-13 — identity seeding that stop clears
GCREX-17 — name resolution (alias) used by stop
GCREX-19.24 — server-stop comparison

**Follow-ups.**
- Verify and, if needed, clear all meta keys and the activity entry on stop (mistake).
- Confirm that `block.close` kills detached descendants, or document the limit (gap).
- Consider per-name locking so one stuck stop does not block others (gap).


### provision

**Status** — not implemented

**Our implementation.** `provision` is listed in the `unimplemented` map (`cmd/gc-runtime-rex/main.go:74-79`) and has no `ops` entry. Dispatch hits the `!known` branch, prints `operation "provision" not implemented` to stderr and returns `exitUnknown` = 2 (`main.go:116-123`). stdin is not read and stdout stays empty. The handshake does not declare `proc.provision` or `proc.exec` (`main.go:32-34`; `docs/REX-CONTRACT.md:468-471`). Provision's 120 s deadline (`docs/REX-CONTRACT.md:188-189`) never comes into play.

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

**Our implementation.** `exec` is listed in `unimplemented` (`cmd/gc-runtime-rex/main.go:74-79`) and is absent from the `ops` table (`main.go:81-97`). `run` looks the op up in `ops`, finds nothing, writes `operation "exec" not implemented` to stderr and returns `exitUnknown` = 2 (`main.go:115-123`). That path runs before the arg check, state-dir resolution and the handler, so stdin is never read, nothing is written to stdout and no Rex state is touched; the 30 s deadline never applies. The handshake declares only `report-attachment` and `report-activity` and withholds `proc.exec`, `proc.provision`, `proc.stream` and `tty.attach` (`main.go:33-35`; `docs/rex-provider.md:119-123`; `docs/REX-CONTRACT.md:468-471`). `cmd/gc-runtime-rex/dispatch_test.go:107-120` pins exit 2 with empty stdout for `exec`.

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

**Our implementation.** Dispatch `cmd/gc-runtime-rex/main.go:84` (1 arg) → `opIsRunning` (`main.go:193`) → `Provider.IsRunning` (`internal/provider/provider.go:408`). No stdin. Stdout is `true`/`false` plus newline (`printBool`, `main.go:158`), buffered and flushed only on success (`main.go:100`, `main.go:129-142`). Running means: a binding for the name (or an alias) revalidates against `SessionView` (`binding.go:243`, rescan by labels if stale, `provider.go:429-432`) and `BlockProcess` for that block succeeds with `LastExit == nil` (`liveness.go:105-115`). A missing block or `ErrNotFound` is `false` (`liveness.go:107-110`). A pane with `LastExit != nil` is `false`. `program_status` and process names are not used, because Rex runs commands under a shell (`liveness.go:100-104`). No pane or session means `false`, exit 0 (`binding.go:194-204`, `provider.go:408-415`). Any other client error, including an unreachable Rex server (`ListSessions`/`SessionView`/`BlockProcess`), is returned and the process exits 1 with stderr `gc-runtime-rex: is-running: …` (`main.go:123-127`). No cross-process cache: the sidecar binding store is only a lookup hint and is revalidated on every call. Stale bindings are deleted as a side effect (`provider.go:434-437`, `binding.go:300-302`). Any alias match that is alive makes the answer `true` (`provider.go:405-407`).

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
GCREX-11 — env-derived tab/pane labels used in rescans
`docs/REX-CONTRACT.md` §9 — RPP v0 exit codes

**Follow-ups.**
- No code change is needed for gc: exit 1 and `false` are equal. Decide outage behavior separately, e.g. retry the Rex connection briefly inside the 30 s op (gap).
- Add a test for is-running with a dead Rex socket (gap).
- Check that stale-binding deletion on a read is safe under concurrent ops (gap).


### process-alive

**Status** — implemented

**Our implementation.** Dispatch is `cmd/gc-runtime-rex/main.go:89` (`{1, "process-alive <name>", opProcessAlive}`). `opProcessAlive` (`main.go:226-240`) reads all of stdin, splits on `\n`, drops one trailing empty element, calls `Provider.ProcessAlive`, and prints `true`/`false` (`printBool`, `main.go:158-160`); exit 0 on an answer, an error returns non-zero. `ProcessAlive` (`internal/provider/liveness.go:120-137`) resolves the binding (no binding returns `false`, nil), calls Rex `process` once for the pane (`BlockProcess`), then `expandProcessNames` (`liveness.go:32-53`: TrimSpace each line, drop blanks, add the `kimi`→`kimi-code` alias, add the basename of `GC_BIN` for the `gc` hint, falling back to the provider's own `$GC_BIN` for legacy bindings). `processAlive` (`liveness.go:59-75`) is false if `LastExit != nil`; true if the expanded list is empty; otherwise true iff the pane's `Foreground` process or any of its `Ancestors` matches. `processMatches` (`liveness.go:79-98`) is exact equality of a name against Rex `name`, `argv0`, `basename(path)` or `basename(argv0)`; no substring and no interpreter-script logic. Multiple names mean any-of. Touches no state files; deadline is the default 30 s. gc skips the op unless `process_names` is non-empty (`exec/exec.go:703-706`), and in production only after two successful `is-running` probes.

**herdr-core.** Dispatch is `herdrcore/ops_table.py:75` (`stdin=True`). `op_process_alive` (`herdrcore/reads.py:292-303`) parses stdin with `stdin_names` (`reads.py:63-65`: `splitlines()`, strip, drop blanks), then returns `false` if the name has no intact record (`reads.py:295-296`), `true` if no names (`reads.py:297-298`), else runs `agent_state` (`reads.py:104-118`) and prints `true`/`false` via `encode_bool` (`rpp/wire.py:293-295`). `agent_state` expands names with `expand_names` (`herdrcore/procs.py:550-561`: strip, drop blanks, `kimi-code` alias, basename of the stored `gc_bin` for `gc`) and `find_agent` (`procs.py:608-613`) searches the box's whole host-process subtree (`subtree`, `procs.py:504-522`) from the box pid. `matches` (`procs.py:564-583`) is exact basename of `comm` or `argv[0]`; if `argv[0]` is one of `node/bun/npx/deno` it also tests the first non-flag, non-`run/exec/x` argument, with and without extension; zombies never match. It answers an empty stdout (gc reads that as alive) when it cannot tell: process table unreadable, launcher not yet at the agent, or no match with an unreadable process in the subtree (`reads.py:110-117,301-302`). Any-of semantics over names.

**Deviations.**

| # | Aspect | Rex provider | herdr-core | Verdict | Rationale |
|---|---|---|---|---|---|
| 1 | match scope | Rex-reported foreground process plus its ancestors (`liveness.go:66-72`) | every process in the box's subtree (`procs.py:608-613`) | intentional | Rex exposes only foreground/ancestors via `process` (`docs/REX-CONTRACT.md:89`); no host process table walk. A background child of the agent is not seen. |
| 2 | interpreter scripts | none: `node foo.js` matches `node`, not `foo` (`liveness.go:79-98`) | matches script arg of node/bun/npx/deno (`procs.py:63,576-582`) | mistake | gc's process-table rule (cited in `procs.py:21-26`) has it; an agent run as `node agent.js` with hint `agent` reads dead. Unverified whether Rex `name`/`argv0` ever carry the script; needs a live probe. |
| 3 | missing pane | `BlockProcess` `ErrNotFound` is returned as an error (`liveness.go:125-128`), so exit non-zero | not applicable; a dead record answers `false` (`reads.py:295-296`) | mistake | `paneAlive` maps `ErrNotFound` to dead (`liveness.go:107-110`) but `ProcessAlive` does not. Verified: gc reads exit 1 as not alive, the same as `false` (`exec/exec.go:709-711`), and in production a missing pane already reads dead earlier because `is-running` returns false (`seam_adapter.go:88-99`). The fix is for the op's own correctness, not for gc. |
| 4 | cannot-tell answer | any Rex error is an exit-1 error; never an empty answer | empty stdout (reads as alive) when unreadable or launching (`reads.py:301-302`) | intentional | Rex gives a definite process record or an error; no opaque-process case. Strict `false` on no match is a deliberate choice (`liveness.go:55-58`). Verified: gc reads empty stdout and exit 2 as alive and only `false` or an error as dead (`exec/exec.go:712-718`), so on a transient Rex error answering `false` (as we do) can put the agent on gc's zombie path. |
| 5 | zombie / liveness gate | `LastExit != nil` forces `false` (`liveness.go:60-62`) | zombie never matches (`procs.py:567`); intact record gate (`reads.py:295`) | intentional | Same intent, different primitive: Rex `last_exit` is its liveness truth (GCREX-4). |
| 6 | stdin line splitting | split on `\n` only (`main.go:231`); whitespace trimmed later in `expandProcessNames` | `splitlines()` (also `\r`, `\v`, `\f`, U+2028 etc.) then strip (`reads.py:64-65`) | intentional | gc sends `\n`-separated names; result is identical for that input. All-blank stdin gives alive in both. |
| 7 | matched fields | Rex `name`, `argv0`, `basename(path)`, `basename(argv0)` (`liveness.go:80-86`) | basename of `comm`, basename of `argv[0]` (`procs.py:569-573`) | intentional | Different process data model. Ours also matches a full-path name equal to `argv0` and the Rex `name` (Rex runs commands under the default shell with `argv0` set; see `docs/plans/native-rex-provider.md:643`). |

**Verdict.** intentional (with a mistake inside). Output vocabulary, stdin shape, empty-input and any-of semantics, aliases and the `gc`→`GC_BIN` basename rule match herdr-core. The foreground/ancestor scope is forced by the Rex `process` API. Rows 2 and 3 are the gaps: no interpreter-script matching and an unhandled missing pane.

**Cross-references.**
GCREX-4 — made `last_exit` the liveness truth and `ProcessAlive` strict with empty ⇒ alive
GCREX-2 — `IsRunning` semantics that `process-alive` sits beside
GCREX-18 — `GC_BIN`/claim identity seeding that the `gc` hint depends on
`docs/REX-RETIRE.md` — the `.gc-wrapped` false-zombie that added the `GC_BIN` basename rule
`docs/REX-CONTRACT.md` §9 (line 211) and `process` method (line 89)
`docs/plans/native-rex-provider.md:643` — why name heuristics are not used for `is-running`

**Follow-ups.**
- Map `rex.ErrNotFound` from `BlockProcess` in `ProcessAlive` to `false`, nil, as `paneAlive` does. (mistake)
- Probe a live pane running `node script.js` and decide whether to add interpreter-script matching to `processMatches`. (gap)
- Add a test for all-blank stdin and CRLF input in `opProcessAlive`. (gap)


### list-running

**Status** — implemented

**Our implementation.** Dispatch is `ops["list-running"]` with 0 required args (`cmd/gc-runtime-rex/main.go:91`); `opListRunning` takes `args[0]` as the prefix, default `""`, and prints one name per line with `Fprintln` (`main.go:260-273`). `Provider.ListRunning` walks the provider's own binding store (`internal/provider/provider.go:498-505`), which is the per-city state dir (`main.go:130-135`), not every Rex session on the server. A binding is skipped unless its name or an alias has the string prefix (`provider.go:506-509,551-562`). It is kept if `BlockProcess` finds the block with no `LastExit`; a missing block is dead and skipped, any other error fails the whole op (`provider.go:510-519`, `liveness.go:105-115`). Names are matched with case-sensitive `strings.HasPrefix`. Aliases are listed only for a non-empty prefix (`provider.go:520-536`). The result is sorted and compacted (`provider.go:538`). Stdin is not read, the exit code is 0 or 1, and empty output is exit 0. There is no stdout cap and no name filtering. The 30 s deadline comes from gc.

**herdr-core.** Dispatch is `spec(reads.op_list_running, cap=INVENTORY_STDOUT_CAP)` (`herdrcore/ops_table.py:76`). The handler reads `args[0]` as the prefix, default `""` (`herdrcore/reads.py:307-308`). It lists the city's non-finished terminals filtered by the city label (`reads.py:68-73,310`), so it is city-scoped. A name is kept if it starts with the prefix (case-sensitive `str.startswith`), its newest record `is_intact` (live, has a pid, not a broken reason, pid label matches; `records.py:53-62`), and `readable_name` holds (non-empty, untrimmed, no control character; `rpp/wire.py:194-196`). Names are grouped by session label, sorted, and written as `name\n` lines (`reads.py:314-321`). A stopped city (parked, nothing running) gives empty exit 0. Other core read failures exit 1 (`reads.py:311-313`). Output over 8 MiB (`rpp/ops.py:34`) is turned into a failure (`rpp/perop.py:158-159`).

**Deviations.**

| # | Aspect | Rex provider | herdr-core | Verdict | Rationale |
|---|---|---|---|---|---|
| 1 | aliases in output | Non-empty prefix also lists matching aliases; empty prefix never does (`provider.go:520-536`) | Only the one gc session label per record; no aliases | intentional | GCREX-18 alias claim identity; empty-prefix exclusion avoids phantom orphan beads (comment `provider.go:520-523`). |
| 2 | what counts as running | Block exists and `LastExit == nil` (`liveness.go:114`) | `is_intact`: live, pid label matches recorded pid, not broken (`records.py:53-62`) | intentional | Rex has no pid-label reincarnation case; a closed block is dead. Replaced-shell detection is unverified for Rex. |
| 3 | name filter | None; any binding name is printed | `readable_name` drops empty, padded, or control-char names (`wire.py:194`) | mistake | A name with a newline or padding would corrupt gc's line parse. Names come from gc, so the risk is low, but the guard is missing. |
| 4 | stdout cap | None | Fails over 8 MiB (`perop.py:158`) | intentional | gc's `inventory` cap only applies to the `inventory` op (`ops.py:35`); `list-running` is not capped in the RPP v0 baseline. Not needed. Note that gc treats a `list-running` failure as an incomplete inventory, which blocks "stopped" confirmation in `gc stop` (`cmd/gc/cmd_stop.go:861-864`). |
| 5 | stopped/unreachable server | Rex client error on any live-candidate probe returns exit 1 (`provider.go:514-516`) | Parked city gives empty exit 0 (`reads.py:311-312`) | not-implemented | Rex has no park/stop-city concept in v0. If Rex is down, our ops fail rather than report an empty list. Behavior on Rex-down is unverified; I would need the client's error mapping. |
| 6 | scope source | Local binding store in city state dir, probed per binding; sessions not bound by this provider are invisible | Core's terminal list filtered by city label, server truth | intentional | Mapping is City→Rex session; bindings are the ownership record. Stale or unrecorded panes are not listed. |

**Verdict.** intentional (with a mistake inside). Prefix semantics, case sensitivity, sorting, and one-name-per-line output match. The differences are alias listing and binding-store scope, which are deliberate. The missing readable-name guard (row 3) is the only gap.

**Cross-references.**
GCREX-18 — alias/claim identity that drives alias listing and the empty-prefix exception
GCREX-2 — start/stop/is-running; shared `paneAlive` liveness
GCREX-4 — identity/metadata; binding store
`docs/REX-CONTRACT.md` §9 — list-running row

**Follow-ups.**
- Skip (or reject) names that are empty, padded, or contain control characters before printing (mistake).
- Add a test for Rex-unreachable behavior in `ListRunning` and decide whether it should be exit 1 or empty (gap).
- Document that unbound Rex panes are never listed (gap).


### is-attached

**Status** — implemented

**Our implementation.** `is-attached` is in the op table with arity 1 (`cmd/gc-runtime-rex/main.go:93`), handled by `opIsAttached` (`main.go:286-293`), which prints a bool and exits 0, or returns the error (exit 1). `Provider.IsAttached` (`internal/provider/attach.go:22-52`) resolves the binding via `Binder.Resolve` (revalidates the cached sidecar binding against Rex, `binding.go:270-295`). It then calls `ListClients` on the session. Kinds `cli` and `peer` are ignored, `app` counts, and any other kind returns `ErrAttachmentUnknown` (exit 1) (`attach.go:34-43`). If an app client exists, it calls `SessionView` and answers true only when the rig tab is the server-side active window and still holds the agent's block (`attach.go:63-73`). So "attached" means "a human app client exists and is looking at this tab", not a per-block viewer. The handshake declares `report-attachment` (and `report-activity`) (`main.go:35`). A missing binding gives `false`; a Rex transport error propagates as exit 1; there is no answer cache beyond the binding store. Deadline is the 30 s default; no per-op override found in `main.go` (unverified beyond a grep for Timeout/Deadline).

**herdr-core.** `op_is_attached` (`herdrcore/reads.py:324-334`) returns `false` with no record, otherwise snapshots local processes (`viewer_targets`, `attach.py:68-83`: same-uid `hc attach` argv whose environ matches this city's socket/session) and ORs that with a held `flock` on a per-object attach lock file (`attach.py:86-90`, `lock_held` `attach.py:42-54`). It never asks the server for a client list; evidence is process table plus lock file. A `ProcError`/`LockFileError` becomes `OpError` (exit 1) (`reads.py:333-334`). Op is registered with the `probe` deadline, 4 s (`ops_table.py:77`, `rpp/ops.py:26-30`), a gc v2 observe-time probe timeout, and not the 30 s baseline. The same `attached()` is reused by stop to avoid killing watched sessions, treating "cannot tell" as attached (`lifecycle.py:574-582`).

**Deviations.**

| # | Aspect | Rex provider | herdr-core | Verdict | Rationale |
|---|---|---|---|---|---|
| 1 | attachment source | Rex server client list (`ListClients`) plus `SessionView` | local process snapshot plus attach-lock file | intentional | Rex has a server-side client registry; herdr has none. |
| 2 | meaning of "attached" | app client present and rig tab is the active window | an `hc attach` process or lock targets this terminal | intentional | Rex tab tiles several agents; no per-pane viewer exists. Granularity is coarser. |
| 3 | unknown client kind | error `ErrAttachmentUnknown`, exit 1 | n/a (no client kinds) | mistake | gc reads exit 1 as `false`, i.e. not attached (`exec/exec.go:690-692`). That is fail-open: config-drift restart, progress-stall recycle and the drain-ack escalation kill then go ahead (`session_reconciler.go:2844`, `:2974`, `:3375`, `:6122`; `drain_ack_escalation.go:296`). When the client kind cannot be classified, answer `true`. |
| 4 | deadline | 30 s default | `probe` 4 s | intentional | gc 1.5.1 baseline is 30 s for is-attached; 4 s is a v2 observe-probe value. |
| 5 | unreachable backend | Rex transport error returned, exit 1 | proc/lock read error to exit 1; no server involved | intentional | Matches; ours depends on the Rex server, herdr's on `/proc`/ps. Note the same fail-open reading: any error is "not attached" to gc. |
| 6 | stop-safety use | `IsAttached` not consulted by stop (`provider.go:380-405`) | `stop` skips kill when attached or cannot tell (`lifecycle.py:574-582`) | intentional | Verified: gc never calls `is-attached` before the `stop` op (`exec/exec.go:581-584`; `city_runtime.go:4199-4235`). Its attachment gates sit in the reconciler and escalation paths, before gc decides to stop (`session_reconciler.go:6122`, `:6304`; `drain_ack_escalation.go:296`). herdr's stop-side protection is an extra safeguard, not a gc requirement. |

**Verdict.** intentional. The attachment source differs because Rex has a real client registry, and `report-attachment` is declared with a meaningful answer. Verified in gc 1.5.1: `is-attached` is only invoked when `report-attachment` is declared (`exec/exec.go:684-691`), the op's output must be exactly `true`, and any error becomes `false`. Only the unverified stop-safety use (row 6) is a possible gap.

**Cross-references.**
GCREX-4 — identity/metadata/activity/attachment phase; defines `IsAttached` and `report-attachment`
GCREX-5 — RPP v0 delivery, handshake declaration
docs/REX-CONTRACT.md §9 — `is-attached` row, `report-attachment` capability

**Follow-ups.**
- Optional hardening: have `Stop` warn or refuse when `IsAttached` is true; a plain `gc stop` or city shutdown is not gated by gc and would kill an attached Rex pane (gap).
- Answer `true` instead of `ErrAttachmentUnknown`/exit 1 when the client kind is unknown, so gc's destructive paths stay closed (mistake).
- Confirm no per-op deadline override exists in `main.go` and that 30 s is enforced (gap).


### get-last-activity

**Status** — implemented

**Our implementation.** Dispatched at `cmd/gc-runtime-rex/main.go:92` (1 arg, no stdin) to `opGetLastActivity` (`main.go:275-284`), which calls `Provider.GetLastActivity` (`internal/provider/activity.go:40-75`). It resolves the binding; no binding returns zero time with source `none`. Otherwise it takes the newest `UpdatedAt` across the block's `program_status` records (`activity.go:49-61`). With no records it hashes `block format` output (sha256) and compares it with the stored fingerprint, which is keyed by block id (`activity.go:63-74`). If the fingerprint is unchanged it returns the stored time. If it changed, or is unseen, it stamps `time.Now().UTC()` and saves it (`stores.go:18-24, 98-102`, one JSON file per name). The handler prints `At.UTC().Format(time.RFC3339)` plus newline (whole seconds, `Z`) only when `At` is non-zero. Unknown or missing session prints nothing and exits 0 (`dispatch_test.go:258-274`). Rex errors exit 1. Capability `report-activity` is declared (`main.go:35`). Verified in gc 1.5.1: `get-last-activity` is called regardless of `report-activity` (`exec/exec.go:838-852`); the capability only sets `CanReportActivity`, which feeds sleep/idle and nudge decisions.

**herdr-core.** `op_get_last_activity` (`herdrcore/reads.py:336-351`) returns `OpResult.ok()` (no stdout) when the record is missing or not intact. For a pane that never reported OSC 7501 it reads the screen (a read failure raises `OpError`, exit 1) and calls `activity.note_screen`. `note_screen` (`activity.py:94-104`) hashes the screen with sha256 and keeps hash and `changed_ms` per incarnation in `activity.json`, under an exclusive flock with a 0.5 s wait (`activity.py:74-85`). `last_activity_ms` (`activity.py:126-137`) returns now when the status is `working` or `blocked`. A reported pane gets `max(status_ms or created_ms, input_ms)`. An unreported pane gets `max(changed_ms, input_ms)`, or now if its screen was not read. `input_ms` is set by `note_input` whenever the pack types into the box (`activity.py:88-91`). Output goes through `rfc3339_ms` (`rpp/wire.py:298-304`): UTC with milliseconds, `.mmmZ`. `report-activity` is declared (`handshake.py:37`).

**Deviations.**

| # | Aspect | Rex provider | herdr-core | Verdict | Rationale |
|---|---|---|---|---|---|
| 1 | timestamp format | `RFC3339`, whole seconds (`main.go:280`) | RFC3339 UTC with ms (`wire.py:298-304`) | intentional | The gc RPP v0 contract takes RFC3339. Both are UTC `Z`. Millisecond precision is not required. |
| 2 | active program (`working`/`blocked`) | Uses the record `UpdatedAt`, which can be stale while the program works (`activity.go:53-61`) | Returns now for `working`/`blocked` (`activity.py:129-131`) | mistake | Confirmed in gc 1.5.1: it idle-stops or sleeps on `now - last_activity > timeout` (`idle_tracker.go:159-163`), and the same value gates drain-escalation kills (`drain_ack_escalation.go:303-306`), progress-stall recycle (`session_reconciler.go:2967-2972`) and the claim backstop (`execution_backstop.go:279-282`). A stale value from a silent but busy Rex agent is read as idle. herdr-core made this call explicitly (`activity.py:12-17`). |
| 3 | input time | Not tracked. Nudge and send-keys do not touch activity (`activity.go`) | Takes `max(…, input_ms)` from `note_input` (`activity.py:88-91, 132-137`) | mistake | After gc types into a pane that shows no output change yet, the time stays old, and the same idle/escalation paths above read it as idle. |
| 4 | state locking | `sync.Mutex` in-process only. Last-writer-wins file per name (`stores.go:98-102`). Write errors are ignored | flock on `activity` with a 0.5 s wait, falls back to a non-persisted entry on timeout (`activity.py:74-85`) | intentional | One process per op. Concurrent calls for the same name can race, but the race only moves the stamp by the interval between the calls. Low impact. Unverified whether `GetLastActivity` is ever called concurrently. |
| 5 | error on Rex failure for a reported pane | Fails with exit 1 if `program_status` errors (`activity.go:49-52`) | Reads cached `activity.json` and never queries the host for reported panes (`reads.py:348-349`) | intentional | The source of "reported" differs. herdr-core gets status from the carrier record. We query Rex per call. gc parses the answer with `time.RFC3339` (`exec/exec.go:846`), discards a read error (`exec/seams.go:230`) and treats zero as unknown, never idle, so failing is the safe direction. |

**Verdict.** intentional (with a mistake inside). The sources differ (Rex `program_status` and `block format` against hc records and screen reads) and the fallback logic is the same (hash the screen, keep when it changed). We omit the "working/blocked is active now" rule and the pack-input floor, so a silent but busy agent can look idle.

**Cross-references.**
GCREX-4 — implemented `GetLastActivity`, the sidecar, and the OSC 7501 policy (`docs/REX-CONTRACT.md:448-456`)
GCREX-16 — consuming `program_status` for nudge gating, which shares the record source
GCREX-5 — handshake declares `report-activity`
GCREX-6 — pilot uses idle detection
GCREX-17 — session resolution that feeds `binder.Resolve`
`docs/plans/native-rex-provider.md` §6 (activity) and the capability table (lines 397-428)

**Follow-ups.**
- Return now when the newest `program_status` state is `working` or `blocked` (mistake)
- Record a pack-input time in `nudge`/`send-keys` and use it as a floor for the result (gap)
- Add a test where an old `working` record yields a fresh time (gap)
- Decide whether millisecond RFC3339 is worth emitting (gap)


### peek

**Status** — implemented

**Our implementation.** Dispatched as `peek <name> [lines]` with one required arg (`cmd/gc-runtime-rex/main.go:90`); `run` buffers stdout and flushes it only on success (`main.go:136-145`). `opPeek` (`main.go:243-259`) reads no stdin, treats an absent or empty `lines` as 0, and fails exit 1 on a non-integer (`strconv.Atoi`, `main.go:245-250`). `Provider.PeekLines` (`internal/provider/provider.go:457-467`) resolves the binding, then calls `BlockFormat`; `lines <= 0` returns the whole capture, otherwise `lastLines` (`provider.go:470-481`) keeps the last N newline-split rows and preserves whether the input ended in `\n`. `BlockFormat` (`internal/rex/cli/cli.go:327-357`) calls Rex `format` with `{format: text, trim: true, max_bytes: 983040}` and follows `id` through `blob_read` until `eof`, so there is no output cap beyond what Rex holds (`cli.go:310-313`, `docs/REX-CONTRACT.md:90-91,408`). A name with no pane (`Resolve` returns `ok=false`) gives empty stdout and exit 0 (`provider.go:458-461`; `TestPeekEmptyWhenNoPane`, `provider_test.go:246`). All other errors (Rex unreachable, `SessionView` failure) become exit 1 with the message on stderr (`main.go:138-141`). No deadline is applied in-process (`context.Background()`, `main.go:137`).

**herdr-core.** `peek` is registered with `cap=OBSERVE_STDOUT_CAP` (1 MiB) and no stdin (`herdrcore/ops_table.py:79`, `rpp/ops.py:33`). `op_peek` (`herdrcore/reads.py:357-378`) defaults `lines` to `"0"`, fails exit 1 on `int()` ValueError (an empty string also fails), and returns exit 66 (`NO_SCREEN`) with `no screen for 'name'` when no record exists or hc answers `not_found` (`reads.py:364-371`; `rpp/exitcodes.py:21-22`; tests `test_herdrcore_lifecycle.py:148-149`). It reads the screen only (`terminal.read` with `lines: 0`, `hc/client.py:250-254`), splits on `\n`, keeps `rows[-lines:]` when `lines > 0`, and always joins with `\n` plus one trailing `\n`, encoded with `surrogateescape` (`reads.py:375-378`). An answer over the cap is replaced by exit 1 with an "over the cap" message and no stdout (`rpp/perop.py:158-159`). A core that cannot be reached is exit 1 `cannot read the city's core` (`reads.py:272-278`), except for a parked city or one with nothing running, where `current_record` returns `None` and the result is exit 66 (`reads.py:275-276`, via `parked`/`nothing_runs`). Other hc failures are exit 1 `cannot read the screen` (`reads.py:372-374`).

**Deviations.**

| # | Aspect | Rex provider | herdr-core | Verdict | Rationale |
|---|---|---|---|---|---|
| 1 | missing session | exit 0, empty stdout (`provider.go:458-461`) | exit 66 `no screen for …`, empty stdout (`reads.py:364-371`) | intentional | 66 is v2-ready only (`exitcodes.py:22`); RPP v0 has exit codes 0/1/2, and gc has no exit-66 handling, so 66 would be an ordinary error (`exec/exec.go:153-170`). gc checks `is-running` before it peeks (`seam_adapter.go:189-194`; `runtime_handle.go:186-190`), so this case rarely reaches the op. |
| 2 | pane vanishes between resolve and format | Rex exit 3 becomes `ErrNotFound`, which `PeekLines` does not catch, so exit 1 (`cli.go:46-54`, `provider.go:462-464`) | hc `not_found` is the same "no screen" answer as a missing record (`reads.py:369-371`) | mistake | The missing-session case returns empty and exit 0, but this race fails. `ClearScrollback` already tolerates `ErrNotFound` (`provider.go:490`). gc reads exit 1 as a failed peek and skips its forensics or shows an error; no state changes, so severity is low. |
| 3 | `lines` empty-string arg | `""` is treated as 0, so the whole capture (`main.go:245`) | `""` fails exit 1 (`reads.py:359-363`) | intentional | gc always sends a decimal integer (`exec/exec.go:622`), so an empty arg is unreachable from gc; harmless if it happens. The dispatcher already rejects an empty name (`main.go:124`). |
| 4 | `lines` > 0 row counting and trailing newline | Strips one trailing `\n` before counting rows, then restores it only if the capture had one (`provider.go:471-479`); empty capture returns `""` | Splits the raw text, so a trailing `\n` adds an empty last row that counts toward N; always appends `\n`, so an empty screen returns `\n` (`reads.py:375-378`) | intentional | Ours returns exactly N text rows. gc strips every trailing `\n` from the result (`exec/exec.go:104`), so trailing-newline differences are invisible to it. |
| 5 | scrollback vs viewport | Rex `format` of the block, described as "bounded by terminal scrollback" (`backlog/tasks/gcrex-2…Peek.md` final notes, line 73); not a viewport-only read | Screen only; scrolling is avoided because it sends input to full-screen programs (`hc/client.py:250-252`) | intentional | The Rex `format` method is non-destructive. Exactly what it covers (scrollback plus viewport, or viewport) is unverified; would need a live `format` on a pane with history. |
| 6 | output cap | Pages to completion at 983040 bytes per reply; no total cap (`cli.go:310-357`) | Fails exit 1 with no stdout above 1 MiB (`perop.py:158-159`, `ops.py:33`) | intentional | The cap is herdr's `observe.v1` contract. Verified: gc 1.5.1 has no cap and no truncation, it buffers stdout whole and strips only trailing `\n` (`exec/exec.go:95`, `:104`); the 30 s op timeout is the only bound. A large scrollback is multi-MiB for us (`e2e_test.go` reads ~4 MiB), which is fine. |
| 7 | unreachable core / server | exit 1 with the Rex error text (`main.go:138-141`) | exit 1, or exit 66 when the city is parked or nothing runs (`reads.py:272-278`) | intentional | Our provider has no "parked city" state. A down Rex server is a plain failure for us. |

**Verdict.** intentional (with a mistake inside). The `peek` contract is the same on the happy path: whole output, or the last N rows, to stdout on exit 0. The deviations stem from RPP v0 (no exit 66) and Rex's paged `format`; row 2 is the one real bug; rows 5 and 6 need live checks.

**Cross-references.**
GCREX-2 — built Peek and paged `format` following; notes that Peek is bounded by terminal scrollback.
GCREX-5 — RPP v0 delivery; peek is a driving op, not a conformance requirement when `exec` exists (`docs/REX-CONTRACT.md:225-229`).
GCREX-8 — aligned the plan with the pinned Rex contract (`format` / `blob_read`, §9).
GCREX-17 — name resolution across the candidate set that `Resolve` uses.
`docs/REX-CONTRACT.md` §9 (`format`/`blob_read` rows, line 90-91; RPP table, line 216) and §12 (line 408).

**Follow-ups.**
- Catch `rex.ErrNotFound` from `BlockFormat` in `PeekLines` and return `""`, nil, as `ClearScrollback` does (mistake).
- Add a dispatch test for the vanished-pane case and for an empty `lines` arg (gap).
- Run a live `rex api call block.method format` on a pane with history and record whether it returns scrollback or only the viewport in `docs/REX-CONTRACT.md` (gap).
- Keep answering exit 2 to `exec`: gc reads exit 1 from `exec` as the command's result with no fallback (`exec/exec.go:906-908`), which would break the driving-op fallback (gap).


### nudge

**Status** — implemented

**Our implementation.** Dispatch is `ops["nudge"]` with 1 required arg (`cmd/gc-runtime-rex/main.go:86`). `opNudge` reads all of stdin, then calls `Provider.Nudge` and passes the result through `bestEffort`, which turns `ErrSessionNotFound` into exit 0 (`main.go:206-211,151-156`). `Nudge` returns nil for an empty message before touching Rex (`internal/provider/provider.go:564-566`). Otherwise `livePane` resolves the binding and probes `BlockProcess`; no binding, a missing block, or `LastExit != nil` gives `ErrSessionNotFound` (`provider.go:614-634`, `liveness.go:105-115`). It then does two separate calls: `BlockWrite` of the raw text (`block.method write {data}`), then `rex send-key … -- enter` (`provider.go:570-577`, `internal/rex/cli/cli.go:356-370`). No trailing newline is added, there is no bracketed paste (the `block.event paste` path is unused), no `program_status` read, no busy gate, no retry or settle delay, and no confirmation beyond `send-key` returning (comment `provider.go:557-562`; deferred per `docs/rex-provider.md:269`, GCREX-16). Any other error, such as a Rex exec failure or a failed write or submit, exits 1 with stderr text (`main.go:138-141`). The process uses `context.Background()`, so the only deadline is gc's 30 s kill (`main.go:138`). Nothing is written to stdout and no state is touched.

**herdr-core.** Dispatch is `spec(inputs.op_nudge, stdin=True)` (`herdrcore/ops_table.py:83`). `op_nudge` UTF-8-decodes stdin and answers `rejected` (exit 80) on bad bytes without typing (`herdrcore/inputs.py:342-348`). `nudge` lists the city's terminals and takes the newest record; a missing or non-intact record is `gone` and a core that cannot be reached is `unavailable` (`inputs.py:209-229,269-274`). It then calls `hc terminal.send` once, which types and submits in one request (`hc/client.py:256-262`). The send gets all the op's remaining time minus a 5 s reserve (`inputs.py:162-165`, `hc/client.py:259-262`). Busy and blocked gating happens in the hc core, and the pack maps its refusals: `status_mismatch` → `busy`, `blocked` → `pending_interaction`, `unavailable` → `not_sent`, `not_found`/`finished` → `gone` (`inputs.py:61-66,197-206`). A send hc took whole is `delivered`, or `delivered_unobserved` when the program reports status but was not seen starting the turn (`inputs.py:238-241`). A send that hc may have typed but whose reply was lost is also `delivered_unobserved` (`inputs.py:260-264`). A non-delivery is exit 80 with one `{"outcome":…}` line (`rpp/outcome.py:103-115`); only unknown outcomes and unsupported-core refusals exit 1. The op notes the input time for activity (`inputs.py:232-235`, `activity.py:88-91`). It does not retry `unavailable`; only the startup nudge does (`retry_unavailable=True`, `herdrcore/lifecycle.py:1129`, `inputs.py:256-258`).

**Deviations.**

| # | Aspect | Rex provider | herdr-core | Verdict | Rationale |
|---|---|---|---|---|---|
| 1 | busy / program_status gating | None. Writes whatever the pane is doing, including over a `working` turn or a `blocked` dialog (`provider.go:563-578`) | hc refuses on status: `busy`, `pending_interaction` (`inputs.py:61-66`) | not-implemented | GCREX-16 is open. `activity.go` already reads `program_status`; `Nudge` does not. Gap is documented in `docs/rex-provider.md:269`. gc reads only the exit code (verified), so the gating must happen inside the provider. |
| 2 | submit confirmation | `send-key` returning means Enter reached the pane; nothing observes a turn start (`provider.go:557-562`) | `started` flag from hc gives `delivered` vs `delivered_unobserved` (`inputs.py:238-241`) | not-implemented | Verified in gc 1.5.1: no outcome vocabulary exists and stdout is discarded (`exec.go:613-638`); only the exit code is read. Same GCREX-16 work, needs OSC 7501 idle→working. |
| 3 | exit code when the pane or session is gone | Exit 0 (`bestEffort`, `main.go:151-156`) | Exit 80 `gone` (`inputs.py:213-228`, `outcome.py:114-115`) | intentional | gc 1.5.1 treats nudge to a dead agent as a no-op, like tmux `send-keys`. Exit 80 is a plain error to gc (`exec.go:153-171`), so herdr's outcome code is not usable at baseline. |
| 4 | exit code when the server is down | Exit 1 with the Rex exec error. A non-3 `rex` exit is not `ErrNotFound` (`cli.go:41-62`, `provider.go:621-623`). Exit 3 on a down server is unverified | Exit 80 `unavailable`, nothing typed (`inputs.py:192-194,224`) | intentional | For a baseline gc both are failure. Risk to confirm with the Rex CLI: `internal/rex/cli/cli.go` maps any `rex` exit 3 to `ErrNotFound`, which `bestEffort` turns into exit 0, so if Rex exits 3 for a transport failure a nudge to a down server would report success. |
| 5 | framing: text and submit | Two non-atomic calls: raw `write`, then `enter`. A submit failure exits 1 after the text is already typed (`provider.go:570-577`, `input_test.go:50-70`) | One atomic `terminal.send` type+submit (`hc/client.py:256-262`) | intentional | Rex has no atomic type-and-submit primitive (`REX-CONTRACT.md:143` defines nudge as write+Enter). gc retries a failed nudge from the queue (15 s delay, 5 attempts) or the idle backstop (3 attempts), re-typing the whole text each time (`cmd/gc/cmd_nudge.go:44-45`, `:1917-1952`), so an exit 1 after the write but before Enter will duplicate the text. |
| 6 | multi-line and control characters | Text goes verbatim to `write`; the contract says `write` interprets escape sequences (`REX-CONTRACT.md:141-143`), so embedded newlines or ESC bytes act as keystrokes. Whether Rex wraps in bracketed paste is unverified | Passed as one JSON string to hc `terminal.send` (`inputs.py:254`); hc's paste/newline handling is unverified | not-implemented | Verified for gc: it sends raw `\n`-joined text on stdin and uses no bracketed paste (`runtime.go:131-146`, `exec.go:613-619`), so the provider decides. `block.event paste` exists (`REX-CONTRACT.md:143`) and is not used. Rex-side behavior still needs a live multi-line test into Pi. |
| 7 | empty message | Returns nil before resolving the pane, so exit 0 even for a dead pane (`provider.go:564-566`) | Empty text goes through `_target` and `client.send` like any text (`inputs.py:269-274`); hc's reaction is unverified | intentional | Typing nothing and pressing Enter would submit an empty prompt. No-op is safer. Only differs for a dead pane (exit 0 vs 80). |
| 8 | non-UTF-8 stdin | `string(text)` goes into `json.Marshal`, which replaces invalid bytes with U+FFFD, so mangled text is typed and submitted (`main.go:211`, `cli.go:68`) | Rejected, nothing typed (`inputs.py:344-348`) | mistake | Silent corruption of a prompt. Low likelihood; gc's text is normally UTF-8. |
| 9 | deadline and cancellation | `context.Background()`; no internal budget or reserve; a gc SIGTERM at 30 s can land between write and Enter (`main.go:138`, `provider.go:570-577`) | Send waits until deadline minus a 5 s reserve so the answer always reaches gc (`inputs.py:162-165`) | mistake | A kill between the two steps leaves typed, unsubmitted text. Each `rex` call has no per-call timeout either. Small window, but cheap to bound. |
| 10 | retries / settle time | None; no pause between write and Enter | None for the nudge op (only the startup nudge retries `unavailable`, `lifecycle.py:1129`) | intentional | Behavior matches at the provider level: gc never retries the call itself, and it retries from the queue only on error. With `write`+`enter` an agent TUI slow to ingest the write could miss the Enter. Unverified. |
| 11 | activity bookkeeping | Does not record that input was typed (no hit in `activity.go`) | `note_input` stamps input time (`inputs.py:232-235,263-265`) | not-implemented | Ours reads `program_status` or a format fingerprint (`activity.go`), so a nudge changes the screen anyway. Low impact. |

**Verdict.** intentional (with a mistake inside). The fire-and-forget model, exit 0 for a gone pane, and exit 1 for transport failure fit gc 1.5.1's tmux-parity baseline, where herdr's typed exit-80 outcomes are not readable. The real gaps are the open `program_status` gating and confirmation work (GCREX-16), non-UTF-8 handling, and an unbounded write→Enter window. Multi-line behavior is unverified.

**Cross-references.**
GCREX-3 — original input path: write text then Enter, `livePane` → `ErrSessionNotFound`
GCREX-16 — `program_status` busy gating and submit confirmation; closes rows 1, 2
GCREX-14 — startup nudge: same write+Enter, best effort, no retry (`provider.go:236-255`)
GCREX-4 — deferred nudge gating; `activity.go` already consumes `program_status`
`docs/REX-CONTRACT.md` §6 (`write`, nudge = write + Enter), §9 (nudge row, driving ops not conformance)
`docs/rex-provider.md` — Known v1 Limits, "No busy-gated nudge"

**Follow-ups.**
- Gate `Nudge` on `program_status`: refuse or queue on `working`, surface `blocked`, keep the format-poll fallback (gap; GCREX-16).
- Confirm submit by polling for an idle→working transition after Enter (gap; GCREX-16).
- Reject non-UTF-8 stdin with exit 1 before any write instead of letting `json.Marshal` rewrite it (mistake).
- Bound the write→Enter window: derive a ctx deadline shorter than gc's 30 s and skip or fail cleanly (mistake).
- Live-test a multi-line nudge and one with control characters into a Pi pane; decide between `write` and `block.event paste` (gap).
- Check what `rex` exits with when the server is down; make sure it is not exit 3, so `bestEffort` does not turn it into success (gap).
- Record input time for activity on a successful nudge, if idle detection needs it (gap).


### send-keys

**Status** — implemented

**Our implementation.** Dispatched at `cmd/gc-runtime-rex/main.go:87` (min 1 arg) to `opSendKeys` (`main.go:214`). It ignores stdin. Zero keys returns nil before any lookup (`main.go:216-218`, `internal/provider/provider.go:583-586`). Otherwise `Provider.SendKeys` resolves the live pane (`provider.go:618-633`), translates each arg with `rexKey` and calls `rex send-key -s … -b … -- <keys…>` (`provider.go:591-595`, `internal/rex/cli/cli.go:363-370`). `rexKey` (`internal/provider/keymap.go:9-19`) peels any number of `C-`/`M-`/`S-` prefixes into `shift+ctrl+alt+` order (`keymap.go:24-49`). It maps tmux names case-insensitively: Enter/Return, Escape/Esc, BSpace, DC, IC, PageUp/PgUp, PageDown/PgDn, Tab, Space, Home, End, arrows, F1–F24 (`keymap.go:54-96`). Anything else passes through unchanged, so Rex rejects unknown names (`keymap.go:5-8`), and that error exits 1. There is no `-l` literal mode, and a multi-character arg such as `hello` is not typed as text. A missing binding or dead pane (`ErrSessionNotFound`) is mapped to exit 0 by `bestEffort` (`main.go:151-156`). Stdout is empty, exit codes are 0/1, and the deadline is the default 30 s.

**herdr-core.** `op_send_keys` (`herdrcore/inputs.py:351-352`, table at `ops_table.py:84`) passes `args[1:]` to `send_keys` (`inputs.py:284-301`) and ignores stdin. `translate_keys` (`inputs.py:151-160`) runs before any lookup. `translate_key` (`inputs.py:123-148`) uses an exact-case table `TMUX_KEYS` (`inputs.py:70-97`: Enter, Escape/Esc, Tab, BTab, BSpace, Space, arrows, Home/End, PageUp/PgUp/PPage, PageDown/PgDn/NPage, IC/Insert, DC/Delete, F1–F12). It also handles `C-`/`M-`/`S-` prefixes on a table name or a single character, and treats any other string as literal text typed char by char (lowercase, digits, a few punctuation chars; uppercase becomes `shift+x`; space becomes `space`) (`inputs.py:99-148`). A key with no hc spelling (empty string, unsupported char, bad modifier base) rejects the whole call as `rejected` before anything is sent (`inputs.py:285-287`). The outcome is exit 0 when delivered, otherwise exit 80 plus one outcome line, or exit 1 when the result is unknown (`inputs.py:181-185`, `rpp/outcome.py:103-115`). A gone box (no record, or not intact) answers `gone` (exit 80), not success (`inputs.py:209-229`). Zero keys with a live box is `delivered` (`inputs.py:293-294`). A written-but-unanswered request is never retried and exits 1 (`inputs.py:274-281`).

**Deviations.**

| # | Aspect | Rex provider | herdr-core | Verdict | Rationale |
|---|---|---|---|---|---|
| 1 | unknown key names | pass through unchanged; Rex errors, exit 1 (`keymap.go:80`) | translated char by char as literal text, or `rejected` exit 80 if untypable (`inputs.py:140-148`, `285-287`) | intentional | Verified in gc 1.5.1: `send-keys` carries only tmux key names, never literal text (`exec.go:626`, `dialog.go`, `submit.go`). Rex has no literal-text mode in `send-key`, so a multi-char arg such as `hello` reaches Rex as one unknown key name, which gc never sends. |
| 2 | key vocabulary / case | case-insensitive names, F1–F24, `Return`; no BTab/PPage/NPage (`keymap.go:54-96`) | exact-case table incl. BTab, PPage, NPage, F1–F12 (`inputs.py:70-97`) | intentional | gc 1.5.1 emits only `Enter`, `Up`, `Down`, `Escape` and `C-u` through `send-keys`, all mapped here; `C-c` goes through `interrupt`. It never emits `BTab`, `PPage` or `NPage`, so no gc path hits them. Adding them is optional hardening, not a defect. |
| 3 | modifier handling | any number of `C-/M-/S-` prefixes, any base incl. multi-char unknown (`keymap.go:24-49`) | prefix only on table name or single char; else reject (`inputs.py:132-139`) | intentional | Rex validates the final binding, so we need not pre-reject. |
| 4 | missing/dead session | exit 0 via `bestEffort` (`main.go:151-156`) | `gone`, exit 80 (`inputs.py:209-229`, `rpp/outcome.py:114-115`) | intentional | Exit 80 is `input-outcome`, not in gc 1.5.1 (CONTEXT baseline); tmux `send-keys` to a missing session is treated as best-effort. |
| 5 | zero keys | no-op exit 0 before lookup, even for an unknown name (`main.go:216-218`) | looks up target first (after key translation), so a gone box is `gone` (`inputs.py:289-294`) | intentional | Matches the spec of a fire-and-forget op. |
| 6 | ambiguous failure / retry | any Rex CLI failure exits 1 with wrapped error (`provider.go:595-597`); no retry logic | unknown outcome exits 1 and is never retried (`inputs.py:274-281`) | intentional | Same effective outcome; we have no retries either. |
| 7 | partial failure | all keys go in one `rex send-key` call; one bad name fails the call (`cli.go:367-369`); partial delivery unverified (needs Rex behavior) | all keys validated before any is sent (`inputs.py:285-287`) | intentional | Not reachable from gc: it sends dialog navigation as repeated `Down`/`Up` then `Enter` in separate calls (`dialog.go:696,712`), and every name it emits is mapped. A bad key from another caller would still fail the whole call. |
| 8 | activity side-effect | none (`provider.go:583-599`) | records input via `_noted` (`inputs.py:299`) | intentional | Rex has native activity; `get-last-activity` is separate (GCREX-4). |

**Verdict.** intentional (with a mistake inside). The translation is a deliberate tmux→Rex mapping and best-effort exit 0 matches the v0 baseline. Verified against gc 1.5.1: its whole `send-keys` vocabulary is `Enter`/`Up`/`Down`/`Escape`/`C-u`, and all are mapped.

**Cross-references.**
GCREX-3 — nudge/send-keys/interrupt behavior in our provider
GCREX-5 — Phase 4 RPP v0 delivery
GCREX-1 — pinned Rex key syntax (`docs/REX-CONTRACT.md:136`) and driving-op status (`REX-CONTRACT.md:224-226`)

**Follow-ups.**
- Optional: add BTab→`shift+tab`, PPage→`page_up`, NPage→`page_down` to `rexBaseKey` for non-gc callers (gap, gc never emits them)
- Verify whether Rex `send-key` is atomic on a bad key in a multi-key list; pre-validate if not (mistake)
- No action needed: gc 1.5.1 never sends literal text through `send-keys` (verified)


### interrupt

**Status** — implemented

**Our implementation.** Dispatched at `cmd/gc-runtime-rex/main.go:85` (arity 1, no stdin) to `opInterrupt` (`main.go:202-203`), which wraps `Provider.Interrupt` in `bestEffort`. `bestEffort` turns `ErrSessionNotFound` into exit 0 (`main.go:151-156`). `Interrupt` (`internal/provider/provider.go:604-613`) resolves the pane with `livePane` (`provider.go:618-634`), then calls `client.SendKeys(..., "ctrl+c")` (`interruptKey`, `provider.go:36`). That is a keystroke through Rex's key API, not a signal; the doc comment says Rex has no signal API (`provider.go:601-603`). It sends once, does not wait or confirm anything, and does not retry. A missing binding or an exited pane program gives `ErrSessionNotFound`, so exit 0 with no output. Any other error (transport, SendKeys) exits 1 as `interrupt <name>: …`. Deadline is the default 30 s. No state is touched. Note for a Pi-first city: gc 1.5.1 does not interrupt Pi sessions at all; it stops and restarts them (`internal/session/submit.go:520-525`).

**herdr-core.** `ops_table.py:85` maps `interrupt` to `inputs.op_interrupt` (`herdrcore/inputs.py:355-356`), which runs `interrupt()` (`inputs.py:304-319`). It lists the city, takes the newest box and requires it to be intact (`inputs.py:209-229`). Then it calls hc `terminal.interrupt` with `timeout: 3.0` (`hc/client.py:102,268-270`), so hc waits up to 3 s for the program to settle. The client budget is 5 s (`hc/client.py:133`). The signal or key hc uses is not in this clone. The reply carries `outcome` and `key` (`hc/parse.py:149-151`). The test fake does `killpg(pid, SIGINT)` (`tests/fakehc/terminal.py:363-364`), but the real hc is unverified; I would need the hc source. A missing or finished box answers `gone`, an unreachable core `unavailable`. Both exit 80 with one stdout line (`rpp/outcome.py:103-115`). Success is exit 0 with empty stdout. The call is never retried, because a second Ctrl-C can end the program (`inputs.py:314-316`, `hc/client.py:21-24`). If the request may have been written but no reply came, it exits 1 as unknown (`inputs.py:277-281`).

**Deviations.**

| # | Aspect | Rex provider | herdr-core | Verdict | Rationale |
|---|---|---|---|---|---|
| 1 | mechanism | `ctrl+c` keystroke via Rex SendKeys (`provider.go:609`) | hc `terminal.interrupt` (`hc/client.py:268`); real signal/key unverified | intentional | Matches gc, which models an interrupt as `C-c` (`carrier.go:113-116`); Rex has no signal API (`provider.go:601`). Whether hc signals the group needs hc source. |
| 2 | wait/confirm | none; returns once keys are sent | hc waits up to 3 s for the program to settle (`hc/client.py:102`) | intentional | RPP v0 has no outcome contract, and gc does not wait after `interrupt` on exec providers: its idle wait needs `IdleWaitProvider`, which the exec carrier does not implement (`submit.go:273-282`). |
| 3 | no live program | exit 0, silent (`main.go:152`) | exit 80 `gone: …` (`outcome.py:114`) | intentional | gc 1.5.1 knows only exit 0/1/2; 80 is an extension. tmux-style best effort. |
| 4 | failure mapping | all non-NotFound errors exit 1 | unsent maps to `not_sent`/`unavailable` (exit 80). Unknown after write exits 1 (`inputs.py:277-281`) | intentional | Rex has no written/unwritten distinction. 80 is outside RPP v0. |
| 5 | repeat/retry | one SendKeys, no retry; client retry policy unverified | never retried (`inputs.py:314`) | unverified | gc never retries `interrupt` itself, but codex/kimi/gemini may see an `Escape` first and `C-c` as fallback (`submit.go:290-305`). Rex client retry on a dropped connection remains to check; a second Ctrl-C can end the program. |

**Verdict.** intentional. The behaviour fits RPP v0 and Rex's key-only API. Row 5 is the only open point, and it needs a read of the Rex client's retry code.

**Cross-references.**
GCREX-3 — agent input path (Nudge/SendKeys/Interrupt)
GCREX-5 — RPP v0 delivery, exit-code contract
`docs/REX-CONTRACT.md` §9 — interrupt row (line 208), driving ops not required

**Follow-ups.**
- Check whether `internal/rex` SendKeys retries after a dropped connection; if so, disable it for `ctrl+c` (gap).
- Read the hc source to confirm whether `terminal.interrupt` is SIGINT to the foreground group or `\x03` (gap).


### clear-scrollback

**Status** — implemented

**Our implementation.** Dispatched at `cmd/gc-runtime-rex/main.go:88` to `opClearScrollback` (`main.go:222`), which reads no stdin, writes no stdout and wraps `Provider.ClearScrollback` in `bestEffort` (`main.go:149-155`), so `ErrSessionNotFound` becomes exit 0. `ClearScrollback` (`internal/provider/provider.go:485-492`) resolves the name to a stored binding; with no binding it returns nil. Otherwise it calls `client.BlockClear` on the bound session/block (the Rex `clear` block method, `docs/REX-CONTRACT.md:93`) and ignores `rex.ErrNotFound`. This is a server-side Rex call on the pane's terminal, not an injected tty escape. The contract notes it is a no-op while a full-screen program runs (`REX-CONTRACT.md:93`). Whether `clear` also drops the visible screen, and what a later `peek` (which uses `format`) returns, is unverified: it needs Rex server source or a live probe. Other errors exit 1. Deadline is the 30 s default. The call is idempotent: a second clear on an already-empty buffer is not an error (test at `internal/provider/phase4_test.go:56-72`). Verified in gc 1.5.1: gc calls it only from the session reconciler, right after a successful kill for max-age or idle timeout, and discards the result (`cmd/gc/session_reconciler.go:3739`, `:3894`); it never re-peeks. The tmux carrier form is `tmux clear-history`, history only (`carrier.go:118-121`).

**herdr-core.** `op_clear_scrollback` (`herdrcore/inputs.py:359`, table entry `ops_table.py:86`) calls `clear_scrollback` (`inputs.py:320-336`). That function sends nothing: its docstring says hc keeps no scrollback a client can clear, so it only does a `terminal.get` to check the box is still intact and then answers `delivered` (exit 0, empty stdout). The test asserts no input or keys are written (`tests/unit/test_herdrcore_inputs.py:283-286`). A missing name, no box, or ended program answers `gone`, and an unreachable core answers `unavailable`. Each of these exits 80 with one outcome line (`rpp/outcome.py:103-115`, `rpp/exitcodes.py`), because herdr declares `input-outcome` (`handshake.py:43`). A lost reply after the request was written exits 1 (`inputs.py:327-328`; test `test_herdrcore_inputs.py:289-295`).

**Deviations.**

| # | Aspect | Rex provider | herdr-core | Verdict | Rationale |
|---|---|---|---|---|---|
| 1 | what is cleared | Calls Rex `clear` on the pane (`provider.go:489`) | Nothing; a no-op after an intactness check (`inputs.py:320-336`) | intentional | Rex has a clear method; hc has no clearable scrollback. |
| 2 | missing session | Exit 0, silent (`main.go:151-155`, `provider.go:486-488`) | Exit 80 `gone` (`inputs.py:213-229`) | intentional | gc 1.5.1 has no `input-outcome` and treats exit 80 as failure; best-effort fits the baseline. |
| 3 | ended or dead program | Rex `clear` on a dead pane: `ErrNotFound` ignored, `ok`; other errors exit 1 (`provider.go:489-491`) | Exit 80 `gone` (`inputs.py:332-333`) | intentional | Same reason as row 2; no outcome vocabulary. |
| 4 | unreachable server / unknown outcome | Any non-NotFound error exits 1 (`provider.go:489-491`) | `unavailable` exits 80; lost reply after write exits 1 (`inputs.py:327-328`) | intentional | Our exit-code set is only 0/1/2. |
| 5 | verifying liveness first | No pre-check; relies on binding plus `clear` result | Reads `terminal.get` before answering (`inputs.py:324`) | intentional | One call instead of two; no confirmation needed for a real clear. |
| 6 | effect on peek / attached clients | Unverified: needs Rex `clear` semantics (does it truncate server scrollback, redraw attached clients?) | None: nothing is cleared, so peek and clients are unaffected | no-impact | gc calls `clear-scrollback` only from the session reconciler after a successful kill and discards the result (`session_reconciler.go:3739`, `:3894`), so neither effect matters to gc. |

**Verdict.** intentional. We do a real Rex clear and map every miss to success. herdr-core cannot clear, so it only reports a delivery outcome that gc 1.5.1 does not read. Rex's `clear` semantics on `peek` output and attached clients remain unverified (row 6), but gc calls the op only after a kill and discards the result, so it does not matter to gc.

**Cross-references.**
GCREX-5 — Phase 4 RPP v0 delivery; added the driving ops including clear-scrollback.
GCREX-1 — pinned Rex contract; `clear` method row (`REX-CONTRACT.md:93`).
GCREX-3 — nudge/send-keys/interrupt share `bestEffort` missing-session handling.
`docs/REX-CONTRACT.md` §9 (line 224-226) — driving ops not conformance requirements.

**Follow-ups.**
- Probe Rex `clear` live: check that `peek` after it returns no old scrollback, and what attached clients see (gap).
- Document the result in `docs/REX-CONTRACT.md:93`, which only states the full-screen no-op (gap).


### watch-startup

**Status** — not implemented

**Our implementation.** `watch-startup` is listed in the `unimplemented` map (`cmd/gc-runtime-rex/main.go:74-79`); `run` prints `operation "watch-startup" not implemented` to stderr and returns `exitUnknown` (2) before any arg check, state-dir lookup, or Rex call (`cmd/gc-runtime-rex/main.go:116-122`). It reads no stdin, writes nothing to stdout, and touches no state; `cmd/gc-runtime-rex/dispatch_test.go:108-118` pins exit 2 with empty stdout. The handshake does not declare any capability that implies a stream (`proc.stream` is reserved in gc 1.5.1, `docs/REX-CONTRACT.md:251`). `GC_READY_PROMPT_PREFIX` is a plain meta key that `Start` copies into `Binding.ReadyPrompt` (`internal/provider/provider.go:32,356-372`); nothing reads `ReadyPrompt` afterwards, so no startup detection runs in the provider or dispatcher (only `filestore.go` and `binding.go` store it). Startup-dialog dismissal happens inside Rex during the launch (`docs/REX-IDENTITY.md:8-18`), and `docs/plans/native-rex-provider.md:464-465` defers a dialog capability.

**herdr-core.** `watch-startup` is a table entry with deadline class `start` (`herdrcore/ops_table.py:87`), so 120 s by default (`rpp/ops.py:23-27`); herdr also honours `GC_EXEC_DEADLINE_MS` (`rpp/perop.py:129`), which gc 1.5.1 never sends (its deadlines are fixed at 30 s, 120 s for `start`/`provision`, and none for `attach`). `op_watch_startup` (`herdrcore/screen.py:28-44`) requires an existing record (`no session 'X'` → exit 1, `screen.py:30-32`), then loops while `deadline.budget(0.25) > 0`: it reads the screen only, never history (`screen.py:5-7,37`), and emits one NDJSON line `{"content": "<screen text>"}` on stdout whenever the text differs from the last emitted text, flushed per line (`screen.py:40-42`, `rpp/wire.py:307-314`, `rpp/perop.py:122-124`). It polls every 250 ms (`screen.py:25,43`). The only failure is a failed screen read, exit 1 with `cannot read the screen: …` (`screen.py:38-39`); there is no startup-failure or ready detection in the op, and the readiness decision stays with gc. The loop ends 1 s before the deadline (`rpp/cancel.py:27,98-103`) and returns exit 0 (`screen.py:44`), so a normal end is "time ran out", not "startup finished". SIGTERM sets the cancel token (`rpp/perop.py:50-57`); the next `pause` raises `Cancelled` within 100 ms (`rpp/cancel.py:66-74`), giving exit 1 `canceled (terminated)` (`rpp/perop.py:149-150`). Per the contract gc kills `watch-startup` outright (`docs/REX-CONTRACT.md:191`), so the SIGTERM path is a courtesy. The only test is `tests/unit/test_herdrcore_staging.py:205-231` (one line per change, no repeats, deadline 3.5 s). Contract side: `docs/REX-CONTRACT.md:224-230` lists `watch-startup` among the calls gc makes beyond the 17-row table and says it stays dedicated because the request/response `exec` connection cannot carry a stream. §9 gives exit 2 as "unknown operation, treated as success" for everything except `start`/`provision` (`docs/REX-CONTRACT.md:200`), so exit 2 is success here. §9 names no explicit fallback for `watch-startup`; what gc does after a silent success is `unverified` (needs gc 1.5.1 source: the caller of `watch-startup` in `internal/runtime/exec`). The herdr-core docstring says the stream "saves gc one `peek` process per poll" (`screen.py:3-5`), which implies the fallback is gc polling `peek`; we implement `peek` (`cmd/gc-runtime-rex/main.go:90`). The consumer is gc's startup-dialog clearing (`screen.py:3-4`), not readiness proper. `GC_READY_PROMPT_PREFIX` is a tmux-provider variable, not an RPP rule (`docs/REX-CONTRACT.md:264-266`); herdr-core's `watch-startup` does not read it (no reference in `screen.py`).

**Deviations.**

| # | Aspect | Rex provider | herdr-core | Verdict | Rationale |
|---|---|---|---|---|---|
| 1 | op presence / exit code | exits 2, no output (`main.go:74-79,117-123`) | streams NDJSON screen events, exits 0 or 1 (`screen.py:28-44`) | not-implemented | Verified in gc 1.5.1: exit 2 with empty stdout marks the watcher unsupported, then gc polls `peek` every 500 ms (120 lines) and answers dialogs through `send-keys` (`exec.go:453-456`, `:376-380`) |
| 2 | stdout format | none | one `{"content": ...}` line per screen change, 250 ms poll (`screen.py:25,40-42`) | not-implemented | Would need a `block.read`/format poll loop over Rex; no existing helper streams |
| 3 | deadline and cancellation | not applicable (returns instantly); no 120 s `start` class | `start` deadline, ends 1 s early with exit 0, SIGTERM → exit 1 (`ops_table.py:87`, `cancel.py:98-103`, `perop.py:149-150`) | not-implemented | gc SIGKILLs `watch-startup` outright, so SIGTERM handling is optional |
| 4 | startup/readiness detection | no detection; `ReadyPrompt` stored, never read (`provider.go:356-372`) | none in the op either; prompt matching stays in gc | intentional | Neither side uses the ready prompt inside `watch-startup`; Rex dismisses dialogs in-launch (`REX-IDENTITY.md:8-18`) |

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
- Decide whether `Binding.ReadyPrompt` stays: it is written and never read (gap).


### attach

**Status** — not implemented

**Our implementation.** `attach` is in the `unimplemented` map (`cmd/gc-runtime-rex/main.go:74-79`); `run` prints `operation "attach" not implemented` to stderr and returns `exitUnknown` (2) before any argument check or Rex call (`main.go:115-122`). The handshake declares only `report-attachment` and `report-activity`, not `tty.attach` (`main.go:35`; `docs/rex-provider.md:123`, `:268`). `internal/provider/attach.go` is misnamed for this purpose: it holds only `IsAttached` and `watchesAgent` (`attach.go:22-73`), the read-only viewer probe behind the `is-attached` op (`main.go:93`, `:287`), and `attach_internal_test.go` only tests `watchesAgent` (`attach_internal_test.go:9`). There is no attach path in the provider that is not an op: nothing opens a pty, execs a Rex client or focuses a pane. The plan only sketches a future `Attach` as `session.focus_block` + `session.focus_window` in the user's Rex app, "rather than stealing a tty" (`docs/plans/native-rex-provider.md:391`).

**herdr-core.** `ops_table.py:88` registers `attach` with `deadline="start"`. In herdr's table that class is 120 s (`rpp/ops.py:23-27`), or `GC_EXEC_DEADLINE_MS` if gc sends it (`rpp/ops.py:127`), not "none". The deadline only bounds the pre-exec work, because `op_attach` ends in `os.execve` (`attach.py:128`), which replaces the Python process. The handler resolves the live record (`attach.py:105-108`) and the `hc` binary (`:110-113`). It checks that the city's core answers `hello()` so `hc` will not start a core with the wrong environment (`:118-121`). It builds the core's allowlist env plus `TERM`, `COLORTERM`, `TERM_PROGRAM`, `TERM_PROGRAM_VERSION` (`:122-123`, `:38`). It takes a shared `flock` on the incarnation's attach lock, inheritable across the exec (`:124-127`), then execs `hc attach <terminal-address>` (`:128`). Stdin, stdout and the tty therefore pass straight to `hc`. Raw mode, SIGWINCH resize, the detach key, exit codes on detach vs error, and client cleanup are not in this pack: they belong to `hc attach`, which is outside the clone (unverified; would need the `hc` source). Pre-exec failures are `OpError` and become exit 1 (`rpp/perop.py:147-148`). Cleanup is the kernel closing the lock fd when `hc` exits, which releases the lock that `is-attached` tests (`attach.py:42-56`, `:88`). The pack does not declare `tty.attach` either (`rpp/wire.py:36-39`).

**Deviations.**

| # | Aspect | Rex provider | herdr-core | Verdict | Rationale |
|---|---|---|---|---|---|
| 1 | op presence | exit 2 "not implemented" | execs `hc attach` with tty passthrough | mistake | gc 1.5.1 calls `attach` and treats exit 2 as failure: `gc session attach`/`new` print `exit status 2` and exit 1, so a Rex-backed session cannot be attached |
| 2 | capability gating | does not declare `tty.attach` | op exists but does not declare `tty.attach` either | intentional | the capability is reserved in gc 1.5.1 and has no consumer; gc calls `attach` without checking it, so both packs omit it harmlessly |
| 3 | attach lock / viewer signal | `is-attached` derives viewers from `session.list_clients` + active window (`attach.go:22-52`) | `attach` holds a flock that `is-attached` reads (`attach.py:88,124-127`) | intentional | Rex reports clients server-side; no lock needed |
| 4 | deadline | op never runs, so no deadline matters | `start` class: 120 s default, pre-exec only | intentional | confirmed: the spec says no deadline for `attach`, and gc's `runWithTTY` uses no context (`exec/exec.go:195-202`); herdr's class only bounds setup before `execve` |

**Verdict.** `mistake`. `attach` is the one op in this document where "absent = exit 2" is not conformant in practice. gc 1.5.1 calls `attach` from `gc session attach` and `gc session new` after an `is-running` check and a possible `start` (`runtime/seam_adapter.go:139-149`, `exec/seams.go:170-172`), and it runs it through `runWithTTY`, which never goes through the exit-2 mapping: the raw `*exec.ExitError` becomes `gc session attach: exit status 2` with exit 1 (`exec/exec.go:195-202`, `cmd/gc/cmd_session.go:399`, `:508`, `:1554`). There is no deadline and no `tty.attach` gate. A user-visible command therefore fails instead of degrading. `docs/REX-CONTRACT.md` §9 should record `attach` (and `exec`) as the exceptions to "exit 2 is success".

**Cross-references.**
GCREX-4 — identity/metadata/activity/attachment; `attach.go` lives here as `IsAttached`
GCREX-19.9 — `is-attached` comparison; covers the attachment probe, not the tty op
GCREX-5 — RPP v0 delivery; handshake and unimplemented-op list
`docs/REX-CONTRACT.md` §9 — op table, deadlines, `tty.attach` reserved
`docs/plans/native-rex-provider.md:391,415` — future `Attach` via focus; do not declare `tty.attach`

**Follow-ups.**
- Exit 0 from `attach` with a hint ("open Rex and select the rig tab") or focus the Rex pane, so `gc session attach` succeeds instead of printing `exit status 2` (mistake).
- If a real attach is wanted, spike `rex attach`/pty passthrough, then declare `tty.attach` only after `gc runtime conformance` accepts it (gap).
- Correct `docs/REX-CONTRACT.md` §9: `attach` and `exec` do not follow the exit-2-is-success rule (gap).


### set-meta

**Status** — implemented

**Our implementation.** Dispatched at `cmd/gc-runtime-rex/main.go:94` (2 required args, empty name/key rejected with exit 1 at `main.go:124`). `opSetMeta` (`main.go:295-301`) reads all of stdin, converts to string with no trim, and calls `Provider.SetMeta` (`internal/provider/meta.go:24-26`). That maps the name to the primary binding name (`meta.go:9-18`) and calls `fileMetaStore.Set` (`internal/provider/stores.go:187`). The store writes one file per (name,key) at `<state>/meta/<sha256(name)>/<sha256(name\0key)>.json` (`stores.go:161`), the value base64-encoded so any bytes round-trip (`stores.go:141-148`). The write is temp file plus rename in the same dir (`filestore.go:27-51`) with no fsync and no chmod (CreateTemp gives 0600; dirs 0755). Locking is an in-process mutex only (`stores.go:~153`); `withStateLock` (`lock.go:209`) is not used. Concurrent processes on different keys cannot clobber each other, and on the same key last rename wins. Set never contacts Rex, so it succeeds for a session that does not exist or is not running. Stdout is empty, exit 0, and any write error gives exit 1. The op runs under the default 30 s deadline, and ctx is unused. There is no key validation beyond non-empty, no key or value size cap, and no in-op reserved key. The provider reads `GC_READY_PROMPT_PREFIX` in Start (`provider.go:32,359`) and seeds or clears the identity keys `GC_SESSION_ID`, `GC_INSTANCE_TOKEN`, `GC_RUNTIME_EPOCH`, `GC_CONTINUATION_EPOCH`, `GC_AGENT`, `GC_TEMPLATE`, `GC_ALIAS` and `GC_PROVIDER` (`identity.go:20-56`). Stop (`provider.go:385-400`) clears only those identity keys, so other keys (`suspended`, `GC_DRAIN_ACK`, `GC_READY_PROMPT_PREFIX`) survive stop and restart, and GetMeta still returns them.

**herdr-core.** `op_set_meta` (`herdrcore/reads.py:425-427`) decodes stdin as UTF-8 with surrogateescape (bytes preserved, no trim) and calls `_mutate_meta` (`reads.py:412-422`). That takes the exclusive per-name lock (`context.py:152-159`, wait up to `MUTATION_LOCK_WAIT_S` = 30 s, cancel-aware), loads the current record's object and its single `meta/<nonce>.json` file, and rewrites the whole file with the value merged in. Write is a 0600 `O_EXCL|O_NOFOLLOW` temp file, then rename, with no fsync (`config.py:174-194`). A missing key arg gives an error (`reads.py:384-388`). An unknown session gives `no session <name>` (error exit 1) for set, and a silent success for remove (`reads.py:416-420`). A session in a terminal the pack did not start is an error (`reads.py:395-397`). The metadata file is deleted when the incarnation ends (`lifecycle.py:389-396`, `forget`), so all meta, including user keys, dies with the incarnation. Metadata is seeded at start with the identity keys (`metastore.py:88-91`). Reads of the file are capped at 1 MiB (`config.py:197`, `read_private_file` limit). That makes a set that grows the file past 1 MiB writable but not readable back (unverified: needs a run to confirm the error path). No other value or key limit appears in `metastore.py`.

**Deviations.**

| # | Aspect | Rex provider | herdr-core | Verdict | Rationale |
|---|---|---|---|---|---|
| 1 | unknown session | Succeeds; store is keyed by name, Rex never contacted (`meta.go:20-26`) | Exit 1 `no session` (`reads.py:416-420`) | intentional | gc may set meta before or after a pane exists; Start itself reads meta (`provider.go:359`), and seeding precedes pane creation. |
| 2 | lifetime across stop | Only identity keys cleared on Stop; other keys persist (`provider.go:393-400`, `identity.go:20-28`) | Whole meta file removed when the incarnation ends (`lifecycle.py:389-396`) | mistake | Stale `suspended`, `GC_DRAIN_ACK` or `GC_READY_PROMPT_PREFIX` from a prior life can leak into the next start of the same name. Needs a gc check on whether it expects persistence (unverified). |
| 3 | locking | In-process mutex only (`stores.go`); one file per key, last writer wins per key | Cross-process exclusive per-name flock, 30 s wait (`context.py:152-159`) | intentional | Per-key files make a read-modify-write lock unnecessary. Set vs Stop's identity clear is not serialised, so a set racing a stop can survive. |
| 4 | file permissions | CreateTemp 0600 file, dir 0755 (`filestore.go:29,36`); no symlink or owner check on read | 0600, O_NOFOLLOW, owner check on read (`config.py:178,197-204`) | intentional | Local single-user state; hardening is not needed for RPP v0 conformance. |
| 5 | size limits | None on key or value; filename is a hash so any key length works | 1 MiB read cap on the file (`config.py:197`); no explicit set-side cap | not-implemented | Large values are unbounded. Low risk, but there is no stated bound; gc values are small. |

**Verdict.** intentional (with a mistake inside). Behavior matches RPP v0 (stdin to store, empty stdout, 0/1 exit). The one real gap is row 2: non-identity keys outlive Stop where herdr-core deletes everything with the incarnation.

**Cross-references.**
GCREX-4 — identity/metadata/activity design and the sidecar store
GCREX-5 — Phase 4 RPP v0 delivery, set-meta/get-meta wiring
GCREX-17 — alias resolution used by `metaName`
GCREX-18 — `GC_ALIAS`/`GC_AGENT` seeding into meta
docs/REX-CONTRACT.md §9 (op table row `set-meta`, line 213), §13.4 (provider meta keys)

**Follow-ups.**
- Decide whether Stop should remove all keys for the name, not only the identity set; clear them in `Provider.Stop` and the no-binding branch (mistake).
- Add a test that `suspended` and `GC_DRAIN_ACK` set before a stop are absent after a fresh start (mistake).
- Optionally serialise set against Stop with `withStateLock` (gap).
- Document that there is no value size cap, or add one (gap).


### get-meta

**Status** — implemented

**Our implementation.** Dispatch entry `get-meta <name> <key>` (2 args) at `cmd/gc-runtime-rex/main.go:95`; handler `opGetMeta` at `main.go:303-309` calls `Provider.GetMeta` and writes the value with `io.WriteString` – no trailing newline added, stdin unread. `Provider.GetMeta` (`internal/provider/meta.go:28-37`) resolves the name to the primary binding via `metaName` (`meta.go:10-19`, rig-qualified aliases), then reads `fileMetaStore.Get` (`internal/provider/stores.go:169-186`): one JSON file per (name,key), value base64 so bytes round-trip. A missing file, an old entry without `value_b64`, an unknown key and an unknown session all give `"", nil`: empty stdout, exit 0, with no session-existence check and no Rex call (`meta.go:26-27`). Only an unreadable/corrupt file or bad base64 is an error (`stores.go:167-168`, `readJSON` `filestore.go:55-67`), exit 1. Default 30 s deadline; no state is written.

**herdr-core.** `op_get_meta` (`herdrcore/reads.py:404-409`, table entry `ops_table.py:80`) requires a non-empty key (`reads.py:384-388`, else `OpError` → exit 1), looks up the session's newest record (`current_record`, `reads.py:263-278`) and its per-incarnation meta file (`metastore.py:105-125`), and emits `values.get(key, "")` as raw bytes, no newline (`reads.py:409`). Unknown session, session with no meta file, and unset key all return empty stdout, exit 0 (`reads.py:407-408`; test `tests/unit/test_herdrcore_lifecycle.py:208-209`). It exits 1 when the core cannot be read (`reads.py:277-278`), when the terminal was not started by the pack (`reads.py:396-397`), or when the meta file is corrupt (`reads.py:400-401`). Metadata is tied to the incarnation (`<state>/meta/<obj>.json`, `metastore.py:1-19`), so it dies with `stop`; it may also be seeded by `start`.

**Deviations.**

| # | Aspect | Rex provider | herdr-core | Verdict | Rationale |
|---|---|---|---|---|---|
| 1 | empty key argument | `get-meta n ""` reads a hashed file, returns empty, exit 0 (`main.go:95`, `stores.go:162-165`; no key check – unverified by test) | empty key is `OpError`, exit 1 (`reads.py:386-387`) | intentional | Verified: gc never sends an empty key; every in-tree caller passes a constant (`session_wake.go:301-310`), and the contract only says empty value = not set (`docs/REX-CONTRACT.md:214`). |
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
- Decide whether `get-meta n ""` should exit 1 like herdr-core; low value (gap).


### remove-meta

**Status** — implemented

**Our implementation.** Dispatch entry `remove-meta` requires 2 args (`cmd/gc-runtime-rex/main.go:96`); the arity check also rejects an empty session name but not an empty key (`main.go:124`). `opRemoveMeta` calls `p.RemoveMeta(ctx, name, key)` (`main.go:312`), which resolves the name through `metaName` (alias to primary binding, `internal/provider/meta.go:9-18`) and calls `meta.Delete` (`meta.go:44-46`). The file store keeps one JSON file per (name,key) and `Delete` does `os.Remove`, treating `ErrNotExist` as success (`internal/provider/stores.go:151-159,194-200`). It reads no stdin, writes no stdout, exits 0 on success. It never contacts Rex and does not check that the session exists. Locking is an in-process mutex only (`stores.go:194-196`); the unlink is atomic and per-key files mean concurrent processes touching other keys cannot clobber each other (`stores.go:151`). No key is guarded, including `GC_READY_PROMPT_PREFIX` (`internal/provider/provider.go:32`, read as a fallback in Start at `provider.go:44`); it is deleted like any other.

**herdr-core.** `op_remove_meta` calls `_mutate_meta(ctx, name, key, None)` (`herdrcore/reads.py:430-432`). A missing key argument raises `OpError("missing metadata key")` (`reads.py:384-388`). It takes the per-name exclusive lock (`reads.py:413`); an unknown session returns OK for remove (`reads.py:415-417`), while set raises. A session in a terminal the pack did not start is an error (`reads.py:396-397`). Otherwise it pops the key from the in-memory dict and rewrites the whole meta file (temp + rename under the lock, `metastore.py:16,64-70`, `reads.py:419-421`), even if the key was absent (a file is created if none existed). No key is protected. Exit 0, no stdout.

**Deviations.**

| # | Aspect | Rex provider | herdr-core | Verdict | Rationale |
|---|---|---|---|---|---|
| 1 | empty key | not rejected at dispatch; deletes the file for key "" (`main.go:124`) | `OpError` "missing metadata key" (`reads.py:386`) | mistake | Minor; gc never sends an empty key, but the guard is cheap. |
| 2 | cross-process locking | in-process mutex; per-key files, atomic unlink (`stores.go:194`) | exclusive per-name file lock, whole-file rewrite (`reads.py:413`) | intentional | Per-key files avoid read-modify-write races, so no lock is needed. |
| 3 | unknown/foreign session | always OK; meta keyed by name, no existence check (`meta.go:44`) | unknown session OK; foreign terminal is an error (`reads.py:396,415`) | intentional | Our meta is name-keyed and independent of Rex; no foreign-terminal concept. |
| 4 | absent key | no-op, no file written (`stores.go:196-198`) | rewrites meta file, may create it (`reads.py:420`) | intentional | Both exit 0; ours has fewer side effects. |
| 5 | provider-owned key guard | none for `GC_READY_PROMPT_PREFIX` (`provider.go:32`) | none | intentional | Matches reference. Effect: Start's fallback ready-prompt is lost after removal; gc sets it only through set-meta. |

**Verdict.** intentional (with a mistake inside). Behavior is idempotent for absent keys and missing sessions, as RPP expects. Only the empty-key check (row 1) is a minor gap.

**Cross-references.**
GCREX-4 — metadata ops (set/get/remove) design
GCREX-18 — claim identity keys (`GC_ALIAS`/`GC_AGENT`) stored via meta
docs/REX-CONTRACT.md §9 — remove-meta op row

**Follow-ups.**
- Reject empty key in `opRemoveMeta` (and set/get-meta) with exit 1 (mistake).
- Add a test that remove-meta of `GC_READY_PROMPT_PREFIX` is allowed and Start then has no fallback (gap).


### observe

**Status** — not implemented

**Our implementation.** `observe` is not in the `ops` map (`cmd/gc-runtime-rex/main.go:81-97`) and not in the `unimplemented` set (`main.go:74-79`, which lists only exec, provision, attach, watch-startup). `run` hits the `!known` branch and prints `unknown operation "observe"` to stderr, returning `exitUnknown` = 2 (`main.go:116-123`). No stdin is read and no state is touched. There is no partial substitute: `is-running`, `process-alive`, `list-running`, `get-last-activity` and `is-attached` are separate ops (`main.go:83-95`), and the handshake declares no `observe.v1` (`docs/REX-CONTRACT.md:255-258`).

**herdr-core.** `ops_table.py:72` registers `observe` with `stdin=True`, `deadline="probe"` and `cap=OBSERVE_STDOUT_CAP`. The probe deadline is 4 s (`rpp/ops.py:23-27`) and the stdout cap is 1 MiB (`rpp/ops.py:30`). stdin is a list of agent process names, one per line (`reads.py:62-64`, `op_observe` at `reads.py:131`). It takes a shared name lock and lists the city's records for that session (`reads.py:134-137`). It prints one JSON `ObserveAnswer` (`rpp/wire.py:162-182`): `absent` (no record, or city stopped), `unknown` (lock timeout or core unreachable), `dead` (box not intact), or `running` with box object, agent state, agent pid and identity (`reads.py:138-158`). It always exits 0 with an answer. It belongs to the `observe.v1` capability, which the handshake declares (`handshake.py:3`). The comment at `rpp/ops.py:28-30` cites a gc `exec/observe.go` probe timeout; no such file exists in gc 1.5.1, so the 4 s is herdr-local. Verified in gc 1.5.1: `observe` is never sent, and its own `Observe` is a Go-side fold of `get-last-activity`, `process-alive` and `is-attached` (`exec/seams.go:229-236`).

**Deviations.**

| # | Aspect | Rex provider | herdr-core | Verdict | Rationale |
|---|---|---|---|---|---|
| 1 | op presence | `observe` exits 2 (unknown op) | `observe` returns a JSON state answer | not-implemented | `observe.v1` is outside the RPP v0 baseline; we declare no such capability. |

**Verdict.** not-implemented. `observe` is not one of the 17 RPP v0 rows, so exit 2 (treated as success, forward compatible) is correct and the op is optional. `docs/REX-CONTRACT.md:255-258` states `observe.v1` does not appear in gc 1.5.1 source, so gc 1.5.1 never sends it; exit 2 is never reached in practice. A newer gc that adds an `observe.v1` capability would be the trigger to revisit.

**Cross-references.**
GCREX-8 — resolved the capability-naming issue; `observe.v1` is not in the baseline (`docs/REX-CONTRACT.md:438-442`)
GCREX-19.9 — is-attached comparison, the other op using the probe deadline
GCREX-19.10 — get-last-activity comparison

**Follow-ups.**
- Revisit only if gc adds `observe.v1` to its capability set; then design a Rex-backed state answer (gap).


### inventory

**Status** — not implemented

**Our implementation.** `inventory` is not in the `ops` map (`cmd/gc-runtime-rex/main.go:78-94`) or the `unimplemented` list (`main.go:74-79`). It falls to the unknown-op branch, which prints `unknown operation "inventory"` to stderr and returns `exitUnknown` (2) (`main.go:114-122`). The closest op is `list-running [prefix]` (`main.go:91`, `opListRunning` at `main.go:260-273`), which prints one session name per line and nothing else. It gives no state, agent, attachment, activity, restart flag or identity. It also has no completeness signal.

**herdr-core.** `inventory` is registered with `INVENTORY_STDOUT_CAP` (`ops_table.py:73`), which is 8 MiB (`rpp/ops.py:34`). `op_inventory` (`herdrcore/reads.py:197-224`) lists the city's records and returns `{"complete": bool, "sessions": [...]}` (`rpp/wire.py:247-256`). Each entry has `name`, `state` (running or dead), `box`, `agent`, `attached`, `last_activity`, `restart_requested` and `identity` (`reads.py:227-258`, `rpp/wire.py:200-210`). It reads screens for agents that never reported OSC 7501 (`reads.py:159-195`), and it garbage-collects stale metadata (`reads.py:162-168, 220-223`). A stopped city returns complete and empty. An unreachable core returns incomplete and empty. Unreadable names set `complete=false` (`reads.py:203-218`). `list-running` in herdr uses the same cap and returns only names of intact sessions (`ops_table.py:76`, `reads.py:307-319`).

**Deviations.**

| # | Aspect | Rex provider | herdr-core | Verdict | Rationale |
|---|---|---|---|---|---|
| 1 | op presence | `inventory` exits 2 (`main.go:114-122`) | Returns `{complete, sessions[]}` records (`reads.py:197`) | not-implemented | Not one of the RPP v0 17 rows. Exit 2 is the correct answer for an unknown op. |
| 2 | bulk state read | `list-running` prints names only (`main.go:260`) | One call returns state, attachment, activity and identity per session | not-implemented | gc 1.5.1 uses per-session `is-attached`, `get-last-activity`, `process-alive` and `get-meta` instead (`exec/seams.go:229-236`). |

**Verdict.** `not-implemented`. `inventory` is an optional extension in the `exec:` provider operation list (`docs/research/rex-session-provider.md:129-133`) and is not in RPP v0 or the 1.5.1 baseline (`docs/REX-CONTRACT.md` §9). herdr-core's handshake advertises `observe.v1` and `carrier.ops` (`handshake.py:34-41`), and `inventory` belongs to that set. Verified in gc 1.5.1: `inventory` is never sent, and there is no capability token for it. gc's "runtime inventory" in `gc stop` is `list-running` with completeness inferred from the error (`cmd/gc/cmd_stop.go:627-646`), and per-session state comes from `is-attached`, `get-last-activity`, `process-alive` and `get-meta` (`exec/seams.go`). Exit 2 is moot.

**Cross-references.**

GCREX-4 — covers per-session `is-attached`, `get-last-activity` and metadata, which `inventory` would batch
GCREX-19.1 — handshake; without `observe.v1` gc should not expect `inventory`

**Follow-ups.**

- No action: gc 1.5.1 never sends `inventory` (verified).
- Keep exit 2. Revisit only if a future gc version needs it and the contract adds a capability for it. (gap)


### kill-object

**Status** — not implemented

**Our implementation.** No `kill-object` entry in the `ops` map (`cmd/gc-runtime-rex/main.go:81-97`) and no entry in `unimplemented` (`cmd/gc-runtime-rex/main.go:74-79`). Any unrecognized op exits 2 (`main.go:72-73`, `main.go:120`), and per RPP v0 exit 2 means unknown operation, treated as success by gc for non-start ops. Nothing is read from stdin and nothing is killed. Closest offering is `stop` (`internal/provider/provider.go:380-405`, `stop` at `:387-405`), which closes the agent's pane via `p.client.CloseBlock` and clears start identity. It does not kill the process tree and does not take a stdin object.

**herdr-core.** `op_kill_object` (`/tmp/gascity-packs/runtime-herdr-core/assets/gc_runtime_herdr_core/herdrcore/lifecycle.py:542-557`) is registered in `ops_table.py:71` with `stdin=True`. It parses stdin via `parse_kill_object_request` (`rpp/wire.py:276-287`) into `{"object": {"id", "created"}, "mode": "corpse"|"zombie", "agent_pid"?}`; invalid input is an op error. Under `ctx.name_lock(name, shared=False)` it calls `_kill_object` (`lifecycle.py:560-592`), which re-checks that the box still exists with the same `created` and session label, then: in `corpse` mode it refuses if the box is intact or attached (`"live"`); in `zombie` mode it kills only on a definite `dead` state and refuses if intact, attached or unreadable (`"changed"`). Kill is `kill_incarnation` (TERM then KILL over the whole process tree, `lifecycle.py:67-80` module doc), then `forget`. Answers are `KillOutcome` scalars `killed|gone|renamed|live|changed` (`rpp/wire.py:138`). Documented in `README.md:188` (`kill-object corpse` removes a dead box) and `README.md:356`. Tests: `tests/unit/test_herdrcore_lifecycle.py`, `test_herdrcore_treekill.py`.

**Deviations.**

| # | Aspect | Rex provider | herdr-core | Verdict | Rationale |
|---|---|---|---|---|---|
| 1 | op presence | exit 2, unknown op (`main.go:120`) | implemented, stdin-driven (`ops_table.py:71`) | not-implemented | Not one of the 17 RPP v0 rows; herdr-core extra. |

**Verdict.** `not-implemented`. Exit 2 is the correct RPP v0 response for an op outside the baseline table, so no contract breach. Verified in gc 1.5.1: no `kill-object` op, method or capability appears anywhere in the tree (whole-tree search, zero hits), so the gap is inert. The only mention in our repo is the escape-hatch list in `docs/research/rex-session-provider.md:129-133`.

**Cross-references.**
GCREX-2 — stop semantics, the closest offered behavior (`stop`).
`docs/REX-CONTRACT.md` §9 — RPP v0 op table and exit codes; `kill-object` not listed.
`docs/research/rex-session-provider.md:133` — lists `kill-object` among exec operations.

**Follow-ups.**
- No action: gc 1.5.1 never sends `kill-object` (verified). Revisit only if a future gc adds it.


### server-stop

**Status** — not implemented

**Our implementation.** `server-stop` is absent from the `ops` map (`cmd/gc-runtime-rex/main.go:81-97`) and from `unimplemented` (`main.go:74-79`). A call falls through to the unknown-op branch (`main.go:112-122`): it writes `unknown operation "server-stop"` to stderr and exits 2, which gc treats as success for non-start ops. Stdin is not read. The handshake declares only `report-attachment` and `report-activity` (`main.go:35`), so `server-lifecycle` is not declared. The code never calls `server.stop`; the decision is recorded in `docs/plans/native-rex-provider.md:83` (D3, "does not implement a server-teardown op") and `:438` ("Server lifecycle. Not implemented (D3)"), and in `docs/REX-CONTRACT.md:400` (§11 D3, "must not be called").

**herdr-core.** `op_server_stop` (`herdrcore/server.py:88-109`) stops the city's core (`server.stop`, `herdrcore/server.py:134`, `herdrcore/commands.py:209`) and ends every session the pack started for the city, under each name's lock (`server.py:116-138`, `_end_session` at `server.py:204-223`). It only touches terminals labelled with its city id (`server.py:112-113`, `:121`). Foreign terminals (not started by the city) keep the core running, and the op returns 0 (`server.py:127-132`). It refuses with exit 1 when a record's metadata cannot be read, when the core still answers after stop, or when the core or its processes survive (`server.py:107-108`, `:226-248`). Lock timeout on the kick lock gives exit 1 (`server.py:104-106`). It takes no stdin and writes empty stdout; its message string is the warning or failure text (`server.py:109`, `rpp/ops.py:69-79`). Exit codes are 0 or 1 only. The pack declares `server-lifecycle` for this op (`herdrcore/ops_table.py:5-6`, `:90`), which is not in the gc 1.5.1 baseline.

**Deviations.**

| # | Aspect | Rex provider | herdr-core | Verdict | Rationale |
|---|---|---|---|---|---|
| 1 | op presence | absent; exit 2 unknown op (`main.go:112-122`) | implemented; stops core and city sessions (`server.py:88`) | intentional | D3: shared Rex server must never be stopped (`native-rex-provider.md:83`, `REX-CONTRACT.md:400`) |

**Verdict.** `intentional`. Verified in gc 1.5.1: there is no wire op at all. Server teardown is an optional Go interface, `runtime.ServerLifecycleProvider` (`internal/runtime/runtime.go:533-547`), implemented only by the tmux provider, and `gc stop` does nothing for a provider without it (`cmd/gc/cmd_stop.go:464-473`). Exit-2 handling is therefore moot. The op is not in the RPP v0 17-row table and `server-lifecycle` is not in the pinned baseline (`docs/REX-CONTRACT.md` §9, "Not in the pinned baseline — do not declare"). Rex has one shared server used by the user's other work, so stopping it would be destructive. herdr-core's core is per-city, so its stop is scoped; Rex's is not. Exit 2 is the correct RPP response to an op we do not implement.

**Cross-references.**
GCREX-1 — §9 pins the op table and records `server-stop` and `server-lifecycle` as outside the baseline.
GCREX-4 — teardown is block-only (`block.close`); never `session.destroy` or `server.stop`.
GCREX-8 — removed `server-lifecycle` from the plan's capability list.
`docs/REX-CONTRACT.md` §9 (not-in-baseline list, ~line 255-262) and §11 D3 (line 400).
`docs/plans/native-rex-provider.md` §0 D3 (line 83), and Known gaps "Server lifecycle" (line 438), "Shared server" (line 644).
`docs/research/rex-session-provider.md:150, 286, 314` — earlier research that proposed `server-lifecycle`; rejected.

**Follow-ups.** None. Do not declare `server-lifecycle` or add a `server-stop` handler. City teardown stays on `stop` (`block.close`).


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


