# Rex contract (pinned)

Phase 0 artifact for `docs/plans/native-rex-provider.md`. The Rex surface the
`rex` runtime provider relies on, captured against pinned versions. **Re-run the
probes in §10 after any Rex upgrade** — the CLI/API is pre-1.0.

## 0. Pinned versions

Two Rex versions matter, and they are not the same number. The provider drives
the **CLI** (§7), but the **running server** answers. The server's version is
fixed when that process starts and only moves when someone restarts it — a
restart the provider must never perform (D3).

| Side | Version | Commit | Built | API revision | Probe |
|---|---|---|---|---|---|
| `rex` CLI (pinned surface) | `0.1.0-preview.20` | `e33d2481b81b202b2600a0f40bd440a5282653a9` | 2026-11-16 | 16 (supports 0–16) | `rex version --json` |
| Rex server (running process) | `0.1.0-preview.19` | `ad5a836cb95bf1685a2b2976762949c44f1d0828` | 2026-11-13 | 14 | `rex api call server.version` |

The **effective surface is `min(16, 14) = 14`**: the CLI advertises 16 but
negotiates down to the server's 14. Every table below was verified at revision
14. A server restart on the current build moves it to 16; re-run §10 then.

| Field | Value |
|---|---|
| Server | unix socket (`REX_SERVER` / `~/Library/Application Support/rex/server.sock`) |
| gc side | RPP v0, `docs/reference/exec-session-provider.md` in the gc repo (§9) |
| gc version | `1.5.1` (pinned by `~/.config/nixpkgs/roles/gascity/default.nix` via numtide/llm-agents.nix) |
| RPP verifier | `gc runtime check <executable>` / `gc runtime conformance <executable>` (the executable lands in Phase 4) |
| Captured | 2026-11-17 (server `.19` / rev 14) |
| Re-verified | 2026-11-17 against CLI `.20` / rev 16, negotiated to rev 14, and against the gc 1.5.1 source |

---

## 1. Hierarchy and vocabulary

```
Rex server  →  session  →  tab (window)  →  pane (block)
gc:  city          rig        agent
```

One shared Rex server per machine. `window` = tab, `block` = pane. D1 maps
gc city → Rex session, gc rig → Rex tab, gc agent → Rex pane (Option B).

## 2. Connection

- The `rex` CLI connects automatically and auto-starts a local server.
- `-S <host>` targets another Rex server; `-C <client>` targets a client.
- Every op is a short-lived CLI invocation in v1 (behind `rex.Client`; §7).
- **Version skew is normal.** A CLI can be newer than the server it reaches
  (§0). Pin both; the negotiated API revision is `min(cli, server)`.
- **Block ops are session-scoped.** `rex block call`, `rex send-key`, and
  `rex block inspect` resolve a block inside the caller's session unless `-s
  <session>` (or `-w <window>`) narrows it. The provider always passes the
  session explicitly. `rex api call` takes `session_id` in the JSON body.
- **`--timeout` is a connect/request timeout, not a call budget.** It does not
  bound `rex events` streaming (§5).

## 3. Topology methods

| Method | Inputs | Returns / notes |
|---|---|---|
| `session.create` | `label`, `initial_windows[]` (`layout`, `window_label`) | `session_id`, `initial_windows[] {window_id, block_ids[], revision}`; atomic (all-or-nothing) |
| `session.new_window` | `session_id`, `layout` (block `{flavor, options, label}` or `split`), `window_label`, `focus` | `window_id`, `block_ids[]`, `revision` |
| `session.new_split` | `session_id`, `anchor_id` **or** `window_id`, `direction` `horizontal|vertical`, `side` `before|after`, `ratio` (0<r<1), `layout`, `focus` | creates blocks beside an existing block/split |
| `session.view` | `session_id` | `session_id`, `active_window_id`, `label`, `revision` (increments per change, jumps on restore), `windows[].{window_id, label, focused_block_id, layers[]}`; each `layers[].blocks[]` entry is `{block_id, flavor, label, owner, creator_name, host_id}`. **The view has no `clients` field** |
| `session.list` | — | `sessions[] {session_id, label}` in creation order |
| `session.list_clients` | `session_id` | `clients[] {client_id, principal, info, connected_at, connection_state}` — the attached-client source (§12), not `session.view` |
| `session.destroy` | `session_id` (required) | closes every window and block in the session, hanging up the programs in them. Schema requires an **attached** connection (`session.attach` / `session.bootstrap` / `session.attach_all`) or the call fails `conflict`; an unknown id fails `session_not_found`. **Destructive — never use it in a probe or in teardown**; the provider tears down with `block.close` and leaves the session (§12) |
| `session.focus_block` / `session.focus_window` | ids | focus a pane / tab |
| `session.set_block_label` / `session.set_window_label` / `session.set_label` | ids, label | ≤128 chars |
| `block.close` | `session_id`, `block_id` **or** `window_id`, `keep_last` | returns `{block_id, revision}`; `keep_last` refuses the last block in a window (`bad_request`, exit 1) |
| `block.event` | `session_id`, `block_id`, `event {type, payload}` | input event; `type` ∈ `key`, `paste`, `mouse`, `wheel`, `focus`. A `paste` payload is `{"text": "..."}`; a `focus` payload is `{"action": "in"\|"out"}` |
| `block.method` / `rex block call` | `session_id`, `block_id`, method name + JSON args | call a block method (§4); `session_id` is required even though `block_id` is unique |
| `target.resolve` / `target.complete` | `kinds[]` (`session\|window\|block\|client`), `queries[]`, optional `context` | resolve labels, window positions, short ids; a target request needs at least one kind (`bad_request` otherwise) |

Terminal block creator: `com.superlogical.terminal`, flavor
`com.superlogical.terminal.shell`. Launch options:
`command` (argv array), `shell` (`none` starts directly), `cwd`, `login`,
`interactive`.

## 4. Terminal block methods

Recurring shape: a method with no input is called with the arg omitted; a
method with input takes one JSON object, and `rex block inspect <block> --json`
carries every method's full input and output schema.

| Method | Input | Output | Use |
|---|---|---|---|
| `process` | — | `{last_exit: {exit_code, error?} \| null, child, foreground, ancestors[], home}`; each process `{pid, user_id, user, name, path, invoked_path, argv0, script?, cwd}` (`script` is set for interpreters such as node) | `process-alive`; agent-exit detection (`last_exit`) |
| `format` | `{format: text\|html\|vt, trim, unwrap, extra_charsets, extra_cursor, extra_hyperlink, max_bytes}` | `content` always. With `max_bytes` set, `total` is always present and **either** `id` (more remains; `eof` is absent) **or** `eof: true` (complete). With no `max_bytes`, the whole capture comes in one reply or the call fails `too_large` | `peek`; send output; exec output |
| `blob_read` | `{id?, offset?, max_bytes?}` | `{id, data, total, eof}` — all four always present. `id` 0/omitted reads the newest result, `offset` defaults to 0, `max_bytes` defaults to 49152 and is clamped to 983040; results are kept ~5 minutes, then `not_found` | `peek` paging; large events |
| `write` | `{data}` | `{bytes_written}` | type text (`nudge`) |
| `clear` | — | — | `clear-scrollback` (no-op while a full-screen program runs) |
| `reset` | — | — | hard reset of a terminal (fallback) |
| `program_status` | — | `{records[] {id, state: idle\|working\|done\|blocked\|error, kind, …}}` — programs report via **OSC 7501** | agent state; idle detection |
| `input_mode` | — | `{echo, canonical, signals, …}` | detect a password/raw prompt |
| `size` / `title` / `pwd` | — | terminal geometry + owning client / title + owner / cwd + owner | diagnostics |

**Reply cap.** A control response is capped at **1 048 576 bytes**; `format`
with no `max_bytes` on a larger capture fails `too_large` (413, exit 1). The
provider therefore asks for `max_bytes: 983040` and follows `id` with
`blob_read` until `eof`. 983040 is also `blob_read`'s clamp: our request size,
not the cap.

**Paging rule.** More output remains when the reply carries `id`; read it with
`blob_read {id, offset}` until the `eof` field is true. A `blob_read` with no
`id` is legal (newest result); it fails `not_found` (exit 3) only when nothing
is stored.

`program_status` is **reported by the program itself** (OSC 7501), not detected
by Rex: a program that emits OSC 7501 gives us native idle/busy state. gc 1.5.1
ships no emitter, and the provider cannot add one — writing the sequence to the
pane reaches the program's stdin, not Rex's parser. The emitter is the program
in the pane: **Pi implements the Program Status Protocol** (interactive mode
reports `working`/`blocked`/`done`/`error`/`idle`, gated on terminal capability
detection or `PI_PROGRAM_STATUS=1`), and gc-managed Pi panes report under Rex
(live-verified `{"state":"idle","app":"pi"}`), so `program_status` is populated
for our sessions. When it is empty the provider falls back to scanning `format`
for the ready prompt.

## 5. Terminal block events

`bell`, `child_exited` (`{exit_code, error?}`), `process_changed`
(`{generation, foreground, ancestors}`), `program_status_changed`,
`program_status_removed`, `title_changed`, `pwd_changed`, `size_changed`,
`clipboard_written`, `selection_copied`, `desktop_notification`,
`progress_report`, `url_clicked`.

`rex events --json` streams these plus session create/remove/state changes as
newline-delimited JSON. It runs until interrupted; `--timeout` only covers
connecting and starting delivery (§2). Treat events as **level-triggered hints
to re-poll**, never authoritative transitions (Rex replays/omits under load).

## 6. Input/key names

`rex send-key` / `block.event {type:"key"}` use Rex key bindings: modifiers
joined with `+` (`ctrl+c`, `shift+left`); named keys (`enter`, `esc`); a
bracketed name (`[Slash]`) selects a physical key. `--action` sends one action
instead of press+release. Translate gc/tmux names: `C-c` → `ctrl+c`,
`Enter` → `enter`, `Escape` → `esc`.

`write` types literal text (escape sequences allowed, e.g. `\u001b[A` for Up);
`block.event {type:"paste", payload:{text}}` pastes. `nudge` = `write` text +
submit (Enter).

Live multi-line observation (GCREX-21, 2026-11-18, Rex revision 14, Pi 1.5.2):
a nudge of `line one\nline two` into an idle interactive Pi pane arrived as
**one submitted turn**. Both lines appeared in the same composer/message and Pi
answered them together, so `write` does not turn an embedded newline into an
Enter that submits the first line; the separate Enter submits the whole text.
This is consistent with Rex paste-like insertion, but the wire mechanism
(bracketed paste vs literal newline insertion) was not inspected. A single
observation; control bytes other than newline were not re-tested.

The RPP `nudge` op rejects non-UTF-8 stdin with exit 1 before any Rex call, and
bounds the `write` + Enter pair with one submit deadline (10 s, or the op
context's earlier deadline). Once the write succeeds, Enter is attempted even if
the parent context is cancelled, so a cancel cannot leave text typed and
unsubmitted; a failed Enter is reported as "text was typed but submit key
failed" and recorded under the `GC_NUDGE_SUBMIT_FAILED` meta key.

## 7. Transport abstraction (must stay swappable)

The provider core depends on a `rex.Client` interface, SDK-style, one method per
operation. v1 implementation (`rex/cli`) shells out; a future `rex/wire`
implementation talks the unix socket directly. Provider logic must not know
which is in use. See plan §3 for the interface sketch.

The socket protocol itself is **not documented/stable**; v1 uses the CLI
deliberately. The CLI is the pinned surface (§0): it reports its own version and
API-revision range, and negotiates with whatever server it reaches.

## 8. Rex CLI exit and error codes

Observed at revision 14. Failures print `Error: <message>` on stderr. A
server-side failure is wrapped as
`Error: stream response "<conn>" failed: <server message> (status=<http> code=<code>)`
— the HTTP status and API error code live in the message, not the exit code.
This is the **CLI's** code space; RPP exit codes are separate and defined in §9.

| Exit | Meaning | Evidence |
|---|---|---|
| `0` | success | any op |
| `1` | server reported a failure, or the request/reply failed to decode | `format` over the cap → `control response exceeds 1048576 bytes (status=413 code=too_large)`; `block.close` + `keep_last` → `cannot close the last block in window <id> (status=400 code=bad_request)`; `block.close` with `window_id:"nonsense"` → `netapi: decoding args ... (status=400 code=bad_request)` |
| `2` | client-side usage or validation error | unknown flag (`rex --version`), unknown command, invalid JSON args, a JSON argument that fails validation (`rex api call session.view '{"session_id":"deadbeef"}'`) |
| `3` | target not found: client-side positional resolution or a server `404` | `rex api describe no.such.method`; `rex block call -b block:deadbeef …`; `rex block inspect block:deadbeef`; `rex window list -s session:deadbeef`; `blob_read` for an expired result → `not found: no result is stored (status=404 code=not_found)` |
| `4` | the client could not reach a server: no server on the socket, or the connection failed | `REX_SERVER=unix:///tmp/nonexistent-rex.sock rex api call session.view '{"session_id":"…"}'` → `no Rex server is running at /tmp/nonexistent-rex.sock` (GCREX-41) |

A malformed id lands on different codes by path, all observed: a bad **JSON
argument** is `2`, a bad **positional target** is `3`, and a bad field that
reaches the **server decoder** is `1`. Exit `4` is a later addition to the
observed set: the rex CLI uses it for a server it cannot reach, which the
provider maps to `rex.ErrUnavailable` so `list-running` can answer the
parked-city empty exit `0` instead of failing the inventory.

## 9. RPP v0 (gc → provider)

Pinned baseline: gc **`1.5.1`** (verify with `gc version`). Source of truth:
`docs/reference/exec-session-provider.md` (the protocol contract) and
`internal/runtime/protocol.go` (the capability constants) in the gc repository
`gastownhall/gascity` at that version; `internal/runtime/REQUIREMENTS.md` is the
behaviour ledger (`RUNTIME-RPP` rows). Those paths are in the gc repo, **not in
this repo**. The mechanical verifier is `gc runtime check <executable>` (or
`gc runtime conformance`), which runs the handshake and exercises every declared
capability; it is implemented in Phase 4.

Invocation: `<script> <operation> <session-name> [args…]` — the script is exec'd
directly, no shell, one process per op. Each call has a deadline: `start` and
`provision` 120 s, `attach` none, everything else 30 s. Each invocation runs in
its own process group; a cancelled call gets `SIGTERM` to the group and then
`SIGKILL` after a 2 s grace (`exec` and `watch-startup` are killed outright).
Trap `TERM`, not just `EXIT`.

Exit codes — the spec has exactly three:

| Code | Meaning |
|---|---|
| `0` | success |
| `1` | failure; stderr carries the message |
| `2` | unknown operation, treated as **success** (forward compatibility) by gc's `run` helper. Not universal: `start` and `provision` treat it as a failure (those two may never be unknown), `attach` runs through `runWithTTY` and so surfaces exit 2 as a failure, and `exec` reads it as "unsupported" before falling back to the driving ops. A provider must therefore never answer `attach` with exit 2. |

Beyond the three spec codes there is herdr-core's **extended input-outcome code
`80`**: the op prints one `{"outcome":"<word>","detail":"…"}` line on stdout
and `<word>[: detail]` on stderr, with `<word>` in `not_sent`, `rejected`,
`unavailable`, `gone`, `busy` or `pending_interaction`. Our provider answers
`send-keys` whose key has no Rex spelling with exit `80` `rejected` (GCREX-43);
`interrupt` with `80` `gone` for a missing or dead pane, `unavailable` when the
Rex server is unreachable, and `not_sent` for another pre-send failure
(GCREX-44); and `nudge` with `80` `rejected` for non-UTF-8 stdin,
`pending_interaction` for a blocked pane, `busy` for a working pane,
`unavailable` when the server is unreachable, and `not_sent` for a write or
submit that did not land (GCREX-45). A delivered interrupt stays exit `0`; a
nudge that reached the pane but whose turn start was not observed exits `0`
with one `delivered_unobserved` line; and an interrupt send failure that may
have written the request stays exit `1` (outcome unknown). A nudge to a missing
or dead pane still exits `0` (best effort), and `nudge`'s live-claim guard stays
a plain exit `1`. The key grammar and the exit-80 words follow
`docs/provider-herdr-method-io.md` §send-keys, §interrupt and §nudge. gc 1.5.1
does not
declare `input-outcome` and reads exit `80` as a plain error, so the typed word
has no meaning to it. That is not fully inert for `interrupt`: the contract asks
for a best-effort exit `0` on a gone session (gc
`docs/reference/exec-session-provider.md:370`), but our gone answer is now exit
`80`, which gc reads as a plain error. gc's `RuntimeHandle.Interrupt`
propagates it unguarded (gc `internal/worker/runtime_handle.go:243-248`); the
managed `SessionHandle` path pre-checks `IsRunning` and never reaches that case
(gc `internal/worker/handle_lifecycle.go:322-330`,
gc `internal/session/submit.go:261-275`). `docs/provider-herdr-method-io.md`
§interrupt Conflicts #1 records the deviation. For `nudge` the exit-80 words
stay inert at the pinned baseline: gc already read the previous `busy`,
`blocked` and send failures as exit `1` and the unconfirmed delivery as exit
`0`, so the typed word and the added `delivered_unobserved` line change nothing
gc acts on; a nudge to a missing or dead pane is still a best-effort exit `0`
(`gc docs/reference/exec-session-provider.md:370`).

Operations — the spec's 17 rows:

| Operation | Invocation | Stdin | Stdout |
|---|---|---|---|
| `start` | `script start <name>` | JSON config | — |
| `stop` | `script stop <name>` | — | — |
| `interrupt` | `script interrupt <name>` | — | — |
| `is-running` | `script is-running <name>` | — | `true` or `false` |
| `attach` | `script attach <name>` | tty passthrough | tty passthrough |
| `process-alive` | `script process-alive <name>` | process names (1/line) | `true`, `false`, or empty (cannot tell; gc reads empty as alive) |
| `nudge` | `script nudge <name>` | message text | — |
| `set-meta` | `script set-meta <name> <key>` | value on stdin | — |
| `get-meta` | `script get-meta <name> <key>` | — | value (empty = not set) |
| `remove-meta` | `script remove-meta <name> <key>` | — | — |
| `peek` | `script peek <name> <lines>` | — | captured text |
| `list-running` | `script list-running <prefix>` | — | one name per line |
| `get-last-activity` | `script get-last-activity <name>` | — | RFC3339 or empty |
| `protocol` | `script protocol` | — | handshake JSON (below) |
| `is-attached` | `script is-attached <name>` | — | `true` or `false` |
| `exec` | `script exec <name>` | command | combined output (op exit == command exit) |
| `provision` | `script provision <name>` | JSON config | — |

Beyond that table, gc also calls `clear-scrollback`, `send-keys`, and
`watch-startup`. The dedicated **driving ops** (`interrupt`, `nudge`, `peek`,
`clear-scrollback`, `send-keys`) are deliberately **not** conformance
requirements: gc drives input and reads output over `exec` (`RPP-CONN-001`) when
a runtime implements it, and falls back to the driving ops when `exec` is
unsupported (exit 2). `watch-startup` stays dedicated because the
request/response `exec` connection cannot carry a stream.

Handshake (`protocol` op):

```json
{"version": 0, "capabilities": ["report-attachment", "report-activity"]}
```

There is **no `locality` field**. A script that does not implement `protocol`
(exit 2) is version 0 with no optional capabilities. Unknown capability strings
are ignored (forward compatibility); malformed handshake JSON is an error.

The pinned baseline has exactly **six** capability strings
(`internal/runtime/protocol.go`):

| Capability | Meaning |
|---|---|
| `report-attachment` | `is-attached <name>` is called and trusted |
| `report-activity` | `get-last-activity <name>` is meaningful for idle/health decisions |
| `proc.exec` | `exec`'s process exit carries the in-box command's exit, disambiguating the overloaded exit 2 |
| `proc.provision` | implements `provision` (box without agent); requires `proc.exec` |
| `proc.stream` | reserved: the persistent bidirectional `stream` op |
| `tty.attach` | reserved: the interactive PTY `attach` connection op |

**Not in the pinned baseline — do not declare.** The plan
(`docs/plans/native-rex-provider.md`) and the research notes use capability names
that do not exist in gc 1.5.1, so `gc runtime check` would ignore them:
`fs.host`, `carrier.ops`, `observe.v1`, `input-outcome` appear nowhere in the
source. `idle-boundary` is design-doc language for sleep tiers
(`engdocs/design/idle-session-sleep.md`), not a token. `server-lifecycle`
appears only in a release-gate evidence file for a provider interface, not in
`protocol.go`. Treat all of them as design shorthand. Concretely, `fs.host` is
not a declaration — agents simply run on gc's host (D4) — and idle state is
reported through `report-activity` plus the block's `program_status`, not
through a handshake token. `GC_READY_PROMPT_PREFIX` is real in gc but is a
tmux-provider environment variable, not part of RPP; round-tripping it through
`set-meta`/`get-meta` is our design.

## 10. Re-verification probes

Run after any Rex upgrade. Every line must match the tables above. Uses
`REX_BLOCK` / `REX_SESSION` from the calling pane. It creates exactly one
scratch session, kills only the id it just created, and never calls
`session.destroy` — that op is destructive and the provider does not use it.
Set `GC_SRC` to a gc checkout to re-check the §9 capability names.

```bash
set -uo pipefail
say() { printf '\n== %s ==\n' "$1"; }

say "CLI (pinned surface) — §0"
rex version --json | jq '{rex_version, commit, built_at, api_revision}'

say "Server (running process) — §0"
rex api call server.version | jq '{rex_version, commit, built_at, api_revision}'

say "Exit codes — §8 (want 3, 2, 2)"
rex api describe no.such.method >/dev/null 2>&1; echo "unknown method -> $?"
rex --version >/dev/null 2>&1;                echo "unknown flag   -> $?"
rex block call -b "$REX_BLOCK" format 'nope' >/dev/null 2>&1; echo "bad JSON args  -> $?"

say "Topology methods — §3"
for m in session.create session.new_window session.new_split session.view \
         session.list session.list_clients session.destroy session.focus_block \
         session.focus_window session.set_block_label session.set_window_label \
         session.set_label block.close block.event block.method target.resolve; do
  rex api describe "$m" >/dev/null 2>&1 || echo "MISSING $m"
done
echo "checked 16 methods (no MISSING lines = all present)"

say "Terminal block methods — §4 (names the provider relies on)"
rex block inspect "$REX_BLOCK" --json \
  | jq -r '.api.methods | keys[]' \
  | grep -Ex 'process|format|blob_read|write|clear|reset|program_status|input_mode|size|title|pwd' \
  | sort

say "Terminal block events — §5"
rex block inspect "$REX_BLOCK" --json | jq -r '.api.events | keys[]' | sort

say "Scratch session round-trip — §3, §4, §6"
# Safety: kill only a session:* id this block just created. An empty or
# unexpected value aborts instead of being passed to a teardown command.
SID=$(rex session new gc-probe-scratch --json | jq -r '.session_id // empty')
case "$SID" in
  session:*) ;;
  *) echo "no scratch session id (got '$SID'); aborting"; exit 1 ;;
esac
trap 'rex session kill "$SID" >/dev/null 2>&1' EXIT
view() { rex api call session.view "$(jq -nc --arg s "$SID" '{session_id:$s}')"; }
WID=$(view | jq -r '.windows[0].window_id')
BID=$(view | jq -r '.windows[0].layers[0].blocks[0].block_id')
echo "session=$SID window=$WID block=$BID"

rex api call session.new_split "$(jq -nc --arg s "$SID" --arg a "$BID" \
  '{session_id:$s, anchor_id:$a, direction:"horizontal", side:"after", ratio:0.5,
    layout:{block:{flavor:"com.superlogical.terminal.shell", label:"probe"}}}')" \
  | jq -c '{block_ids}'

rex block call -s "$SID" -b "$BID" write '{"data":"echo probe-ok; seq 1 40000\n"}'
sleep 1
fmt=$(rex block call -s "$SID" -b "$BID" format '{"format":"text","trim":true,"max_bytes":5000}')
echo "$fmt" | jq -c '{saw_probe:(.content|contains("probe-ok")), has_id:(.id!=null), has_eof:has("eof"), total}'
BRID=$(echo "$fmt" | jq -r '.id // empty')
if [ -n "$BRID" ]; then
  rex block call -s "$SID" -b "$BID" blob_read "$(jq -nc --argjson i "$BRID" '{id:$i, offset:5000}')" \
    | jq -c '{datalen:(.data|length), total, eof}'
else
  echo "ERROR: format returned no id; paging path not exercised"
fi
rex block call -s "$SID" -b "$BID" process | jq -c '{last_exit, foreground:.foreground.name}'
rex block call -s "$SID" -b "$BID" input_mode | jq -c .
rex block call -s "$SID" -b "$BID" size | jq -c '{columns, rows}'
rex block call -s "$SID" -b "$BID" pwd | jq -c '{pwd}'
rex block call -s "$SID" -b "$BID" program_status | jq -c '.records|length'

# Attached clients come from session.list_clients, not session.view.
rex api call session.list_clients "$(jq -nc --arg s "$SID" '{session_id:$s}')" \
  | jq -c '{client_count:(.clients|length)}'

# Reply cap: >1 MiB with no max_bytes must fail too_large (exit 1).
rex block call -s "$SID" -b "$BID" write '{"data":"head -c 1200000 /dev/zero | tr \"\\0\" x; echo\n"}'
sleep 3
rex block call -s "$SID" -b "$BID" format '{"format":"text"}' >/dev/null 2>&1
echo "format over cap -> $? (want 1: too_large)"

rex send-key -s "$SID" -b "$BID" ctrl+c && echo "ctrl+c ok"
rex send-key -s "$SID" -b "$BID" '[Slash]' && echo '[Slash] ok'

rex api call block.event "$(jq -nc --arg s "$SID" --arg b "$BID" \
  '{session_id:$s, block_id:$b, event:{type:"paste", payload:{text:"x"}}}')" >/dev/null \
  && echo "block.event paste ok"
rex api call block.close "$(jq -nc --arg s "$SID" --arg b "$BID" '{session_id:$s, block_id:$b}')" \
  | jq -c '{block_id, revision}'

# session.destroy is checked from its schema, never executed (destructive).
rex api describe session.destroy | grep -qE 'must be attached' \
  && echo "session.destroy: attachment requirement present" \
  || echo "MISSING session.destroy attachment requirement"
rex api describe session.destroy | grep -qE 'session_not_found' \
  && echo "session.destroy: session_not_found present" \
  || echo "MISSING session_not_found"

say "Events stream — §5 (exit 124 is the expected kill)"
timeout 3 rex events --json >/dev/null 2>&1; echo "rex events --json -> $? (want 124)"

say "gc side — §9"
gc version
gc runtime check --help | sed -n 's/^The protocol contract is /  protocol contract: /p'
if [ -n "${GC_SRC:-}" ]; then
  spec="$GC_SRC/docs/reference/exec-session-provider.md"
  echo "  capabilities in $GC_SRC/internal/runtime/protocol.go:"
  grep -oE 'ProtocolCapability[A-Za-z]+ *= *"[^"]+"' "$GC_SRC/internal/runtime/protocol.go" \
    | sed 's/.*"\(.*\)"/    \1/'
  echo "  exit-code rows in the spec :"
  sed -n '/^## Exit Codes/,/^## Cancellation/p' "$spec" | grep -E '^\| [0-2] \|' | sed 's/^/    /'
  echo "  op rows in the spec (want 17): $(sed -n '/^## Operations/,/^### Protocol Handshake/p' "$spec" | grep -cE '^\| `[a-z-]+` \|')"
else
  echo "  GC_SRC unset; set it to the gc checkout to re-check the capabilities, exit codes and op table"
fi
```

## 11. Decisions confirmed (plan D0–D6)

Phase 0 also confirms the plan's decisions against the live surface:

| Decision | Confirmation |
|---|---|
| D0 delivery form | Out-of-tree RPP v0 executable via pack `[runtimes.rex]`. §9 records the op set and handshake; `gc runtime check` is the verifier. |
| D1 mapping | session = city, tab = rig, pane = agent. §1; supported by `session.create` + `session.new_window` + `session.new_split`. |
| D2 transport | Shell out to the CLI behind `rex.Client`; the socket protocol is undocumented and revision-numbered (§7). |
| D3 server ownership | Never stop the shared server. `session.destroy` closes only that session's blocks; `server.stop` exists and must not be called. The version skew in §0 is another reason: the server only moves on restart. |
| D4 scope | Local server via the default socket or `-S <host>`; agents run on gc's host. RPP has no declaration for this. |
| D5 sleep tier | Aim for `full` via `report-activity` plus `program_status` (OSC 7501 — emitted by the pane program; Pi does, and gc-managed Pi panes report under Rex). Fall back to `timed_only` if classification proves unreliable. |
| D6 v1 breadth | Core `Provider` + idle/wait + relaunch + metadata sidecar + activity + attachment; skip `ExecProvider` and `DialogProvider`. |

## 12. Open items

- **Resolved since capture.** `format` paging (Phase 1, GCREX-2): the hard
  control-response cap is 1 048 576 bytes; `peek` requests `max_bytes: 983040`
  and follows `id` with `blob_read` until `eof`.
- **`session.destroy` is destructive and unused.** The schema requires an
  attached connection (or `conflict`) and rejects an unknown id with
  `session_not_found`; `session_id` is mandatory. A fresh CLI call on a scratch
  session returned exit 0 (observed once; the recurring probe only reads the
  schema, never destroys), so the CLI satisfies the attachment itself — the
  requirement is real, only the source of attachment differs. The provider
  never calls it: teardown closes city-owned blocks with `block.close` (D3), and
  §10 asserts its contract from the schema instead of executing it.
- **Attachment signals.** `session.view` has no `clients` field; attached
  clients come from `session.list_clients`. That is where the D1 risk lands: any
  app client attached to the city session makes every agent in it read as
  attached, so resolve attachment to the active tab / `focused_block_id` and
  report honestly when it cannot be refined. **Resolved (GCREX-4):** the
  provider counts only `info.kind == "app"` clients (its own CLI connection is
  listed too), and reports attached only when the agent's rig tab is the
  session's active window and still contains the agent's pane — Option B tiles
  a rig's agents in one tab, so a human watching that tab sees all of them,
  while an inactive tab's remembered focused block is not visibility. An
  unclassifiable client kind returns an error rather than a false yes.
  **Live-verified:** a fresh scratch session already lists the server-wide Rex
  app client(s), and its first window becomes active even with `focus:false`,
  so a single-agent city still reads attached; `Start` does send `focus:false`
  so a background rig does not steal the active window. Attachment remains
  session-coarse: the surface cannot say which tab a given client is viewing.
- **Version skew policy.** The CLI is `.20`/rev 16 while the running server is
  `.19`/rev 14. **Resolved (GCREX-6):** server rev 14 is inside the CLI's
  supported range 0..16, so the pilot and promotion need no restarted server;
  none was restarted (D3). See `docs/rex-pilot-phase5.md`.
- **RPP capability naming.** The plan's `carrier.ops`/`idle-boundary`/`observe.v1`
  vocabulary has no basis in gc 1.5.1 (§9). Rewrite the plan's capability
  language, or confirm a newer gc adds those tokens, before Phase 4.
  **Resolved (GCREX-8):** the plan now uses only the six gc 1.5.1 strings; the
  earlier vocabulary is stated as not in the pinned baseline.
- **Pane process-identity stability:** `block_id` is persisted in the sidecar.
  **Resolved (GCREX-4):** ids are never trusted blindly — a binding is
  re-validated against `session.view`, and a stale or post-restart id is cleared
  and re-resolved by label, so a restart is absorbed.
- **OSC 7501 policy (resolved, GCREX-4; emitter clarified).** The provider does
  not emit OSC 7501 — it cannot, because the pane program owns that stream. Pi
  implements the protocol and gc-managed Pi panes report under Rex
  (live-verified `{"state":"idle","app":"pi"}`), so `program_status` is normally
  populated. `GetLastActivity` prefers `program_status` when Rex reports records
  and otherwise polls `block format`, fingerprinting the output in the activity
  sidecar; it returns which source is in force. The format-poll fallback is the
  documented v1 path. Consuming `program_status` for nudge busy-gating and
  submit confirmation is a separate, still-open item.
- **State directory (resolved, GCREX-4).** Bindings, activity fingerprints and
  metadata live under `$GC_REX_STATE_DIR/<city>`, else
  `$XDG_STATE_HOME/rex-gascity/<city>`, else
  `~/.local/state/rex-gascity/<city>`, one JSON file per `sha256(name)`.
- **Empty-tab teardown (resolved, GCREX-4).** A live probe closed a scratch
  session's only block and `session.view` then reported zero windows: Rex
  removes a tab when its last block closes. Teardown is block-only
  (`block.close`); the provider never calls `session.close_window`.
- **Delivery and handshake (resolved, GCREX-5).** `gc-runtime-rex`
  (`cmd/gc-runtime-rex`) is the out-of-tree RPP v0 dispatcher; `pack/pack.toml`
  declares `[runtimes.rex]` with `command = "gc-runtime-rex"` and `protocol = 0`,
  and both `gc runtime check rex` and `gc runtime conformance rex` pass against
  `testdata/city` (14/14 checks; 6/8 requirements with `exec`/`provision` as
  optional skips). The handshake declares version 0, `report-attachment` and
  `report-activity`, and no `locality` field; `proc.exec`/`proc.provision` are
  not declared because `exec`/`provision` stay unimplemented (exit 2).
- **No city in an RPP invocation (resolved, GCREX-17; pilot GCREX-6).** RPP
  carries only the op and session name, and the exec runtime provider passes the
  caller's environment through unchanged, so `GC_REX_CITY`, `GC_CITY_PATH` and
  `GC_CITY` differ between the controller, session processes and nudge pollers.
  The canonical namespace is now caller-independent: `GC_REX_CITY` when set,
  else `default`. `basename(GC_CITY_PATH)` and `GC_CITY` are additional
  read-only namespaces, searched after the canonical one, so a binding written
  by an older build still resolves; deletes cover every namespace. Writes and
  the state lock stay in the canonical directory. Setting `GC_REX_CITY` pins
  the read set to exactly that namespace, so two pinned cities sharing a
  supervisor never cross-resolve. Cities that share a supervisor without
  pinning still share `gc:default`; pinning remains the way to isolate them.
  **Extended (GCREX-35):** an unpinned caller also discovers the sibling city
  state directories under its state base (decoded back to city names and
  trusted only when they re-encode to the same directory), because gc's CLI
  fallback provider is invoked with no `GC_REX_CITY`/`GC_CITY_PATH`/`GC_CITY`
  at all. Reads widen, writes and the lock stay canonical, and revalidation
  against the live `gc:<city>` session keeps a dead city's record from
  answering as running. With no pin an identical name in two live cities
  resolves in read-set order; a pinned caller never reads siblings.
- **Remaining start-config hooks (resolved for the hook/staging fields, GCREX-5/GCREX-56).** The
  dispatcher once ignored the start config's `session_setup`,
  `session_setup_script`, `session_live`, `copy_files` and `overlay_dir`.
  Rex launch options carry no `env` option of their own.
  **Resolved (GCREX-10):** the `env` map is applied. The dispatcher decodes
  `env` from the start config and wraps the launch argv with `env K=V ...`
  (sorted keys) at the command boundary, so a launched pane's process sees
  every managed-session variable. A bare shell (no command) cannot be seeded,
  since Rex has no per-pane env option.
  **Resolved (GCREX-14):** the `nudge` field is decoded and delivered.
  After a successful pane launch, the startup nudge text is written to the
  pane and submitted with Enter, so an agent receives its initial prompt
  immediately. Delivery uses the just-recorded binding's SessionID/BlockID
  directly — no `livePane` re-resolve — so there is no readiness race. The
  nudge is not delivered on a failed launch, an already-running session
  (Start's idempotent path or StartNew's ErrAlreadyRunning), or when the
  nudge text is empty. Delivery is best effort: a transport failure is not
  reported as a start failure, so gc never tears down a healthy pane over it.
  **Resolved (GCREX-15):** the `pre_start` hook runs before the pane launches.
  Each entry is executed in order via `/bin/sh -c`, with cwd set to the start
  config's `work_dir` when it already exists and `/` otherwise (mirroring
  runtime-herdr-core's `_setup_cwd`, so a hook may create the work dir) and the
  process environment merged with the start-config `env` map. A non-zero exit
  aborts the launch and leaves no seeded identity or binding behind; after the
  hooks a configured `work_dir` must exist or the start fails. An empty or
  absent `pre_start` is a no-op.
  **Resolved (GCREX-33):** after `pre_start` and the `work_dir` check, the
  provider stages each `pack_overlay_dirs` entry's `per-provider/<GC_PROVIDER>/`
  subtree into the work dir, later entries winning. This is what lets a pack's
  per-provider Pi hook override replace a managed hook the controller's
  preserve predicate would keep.
  **Resolved (GCREX-56):** `copy_files` and `overlay_dir` are staged before
  `pre_start` (a `copy_files` destination that leaves the work dir is refused
  with a warning); `session_setup`/`session_setup_script` run after the launch
  as warnings only; `session_live` warns `SESSION_LIVE_SKIPPED`; `fresh_only`
  refuses a held name with exit 73; and `ready_prompt_prefix`/`ready_delay_ms`
  gate the startup-readiness wait. `process_names` remains unimplemented
  (no Rex process-appeared signal).
- **Rig-tab growth (resolved, GCREX-6).** Always splitting the first pane
  halved it each time (14 splits: 1/2 ... 1/16384). The provider now splits
  the largest-area pane along its longer axis, side `after`, 50%, reading
  geometry from `session.view` `layers[].layout`; with no geometry it falls
  back to the first block, horizontally. A live probe reached 45 panes with no
  layout limit, so no cap was added.

---

## 13. RPP start-config environment and session-name forms (gc 1.5.1)

Added by GCREX-11. Evidence: a captured RPP argv/stdin trace from a live city
(`~/my-city`, rig `my-project`, gc 1.5.1) cross-checked against the gc v1.5.1
source (`internal/session/lifecycle.go`, `internal/agent/session_name.go`,
`cmd/gc/pool_session_name.go`, `cmd/gc/build_desired_state.go`,
`internal/runtime/envsecret.go`, `internal/runtime/fingerprint.go`,
`docs/reference/exec-beads-provider.md`). Every `GC_*` below appeared in the
`start` config `env` map unless the row says otherwise.

### 13.1 Identity encoding (source of truth)

`internal/agent/session_name.go`:

- `SanitizeQualifiedNameForSession`: `/` → `--`, `.` → `__`.
- `UnsanitizeQualifiedNameFromSession` (best effort, not lossless):
  `--` → `/`, `__` → `.`.
- `SessionNameFor(city, agentName, "")` ==
  `SanitizeQualifiedNameForSession(agentName)` when no session template is set.

`cmd/gc/pool_session_name.go`:

- `PoolSessionName(template, beadID) = Sanitize(path.Base(template)) + "-" + beadID`.

### 13.2 Session-name forms

| Form | Observed example | Name rule |
|---|---|---|
| Named agent | `mayor` | `Sanitize(qualified name)` |
| Pool instance (wisp) | `pi-mc-wisp-ybq1` | `PoolSessionName(GC_TEMPLATE, GC_SESSION_ID)` |
| Wisp / session-bead session | `core__control-dispatcher-mc-wisp-6znw` | `PoolSessionName(GC_TEMPLATE, GC_SESSION_ID)` |
| Rig-qualified identity (probe form) | `my-project--core__control-dispatcher`, `my-project--pi` | `Sanitize(rig + "/" + templateBase)` |

A pool instance starts under the runtime name (`pi-mc-wisp-ybq1`,
`core__control-dispatcher-mc-wisp-6znw`); it carries no `--`, so the rig is
present only in `GC_RIG`. gc probes the pool with the rig-qualified identity
(`my-project--core__control-dispatcher`, `my-project--pi`), which is also what
`Unsanitize` derives from a live instance name.

Which op uses which form (observed names):

| Op | Form | Observed argv |
|---|---|---|
| `start` | runtime instance name | `start mayor`, `start pi-mc-wisp-ybq1`, `start core__control-dispatcher-mc-wisp-6znw` |
| `is-running` | runtime instance name and rig-qualified identity | `is-running mayor`, `is-running pi-mc-wisp-ybq1`, `is-running my-project--core__control-dispatcher` |
| `list-running` | rig-qualified sanitized prefix; also short runtime prefixes and empty | `list-running my-project--pi-`, `list-running my-project--core__control-dispatcher-`, `list-running my-project--gc__task-decomposer-`, `list-running pi-`, `list-running` |
| `get-meta` | runtime instance name, and the canonical rig-qualified identity | `get-meta mayor GC_PROVIDER`, `get-meta pi-mc-wisp-ybq1 GC_DRAIN_ACK`, `get-meta core__control-dispatcher-mc-wisp-6znw GC_SESSION_ID`, `get-meta my-project--core__control-dispatcher suspended` |
| `stop`, `nudge`, `peek`, … | runtime instance name | `stop mayor`, `stop pi-mc-wisp-ybq1` |

The provider must therefore resolve a rig-qualified `is-running`/`list-running`
(and a rig-qualified `get-meta`) against a live instance whose primary binding
is the short runtime name (see the alias contract in `docs/rex-provider.md`).
It must not return aliases from `list-running` with an empty prefix: gc's
adoption barrier and dead-session reconciler treat every listed name as a real
session and would mint a phantom orphan bead for an alias.

### 13.3 `GC_*` inventory

The **durable provider contract is exactly `GC_RIG`, `GC_TEMPLATE`, `GC_ALIAS`,
`GC_AGENT`** (GCREX-11). The provider reads these four from the start config to
derive the Rex tab and to record rig-qualified aliases. Every other `GC_*` is
incidental to the provider: present in the start config, but not part of the
provider contract.

| Variable | Meaning | When set | Role |
|---|---|---|---|
| `GC_RIG` | rig name (`my-project`); empty for town-level sessions | rig-scoped sessions (empty otherwise) | **Durable**: tab label, alias |
| `GC_TEMPLATE` | qualified template (`my-project/pi`, `my-project/core.control-dispatcher`, `mayor`) | when `Info.Template` is non-empty | **Durable**: alias `Sanitize(GC_TEMPLATE)` |
| `GC_ALIAS` | public routing/display alias (`mayor`, `my-project/core.control-dispatcher`; empty for an unaliased pool instance) | always (explicit empty clears a stale default) | **Durable**: alias `Sanitize(GC_ALIAS)` |
| `GC_AGENT` | durable ownership identity (`mayor`, `mc-wisp-ybq1`, `my-project/core.control-dispatcher`) | when the assignee identity is non-empty | **Durable**: alias `Sanitize(GC_AGENT)` when qualified |
| `GC_SESSION_ID` | session/incarnation id (`mc-wisp-ybq1`) | always | incidental |
| `GC_SESSION_NAME` | runtime session name (`pi-mc-wisp-ybq1`) | always | incidental (same value as the `start` argv name) |
| `GC_SESSION_ORIGIN` | `named`, `ephemeral`, … | when `Info.SessionOrigin` is non-empty | incidental |
| `GC_INSTANCE_TOKEN` | fence token against stale incarnations | always; secret, may be empty in the env | incidental |
| `GC_CONTINUATION_EPOCH` | conversation identity epoch | always | incidental |
| `GC_RUNTIME_EPOCH` | runtime incarnation counter (generation) | always | incidental |
| `GC_PROVIDER` | gc provider family (`pi`) | agent sessions the manager starts; absent from the control-dispatcher start in the capture | incidental |
| `GC_CITY`, `GC_CITY_PATH` | city root | always (city runtime env) | incidental |
| `GC_CITY_RUNTIME_DIR` | canonical city runtime dir | always | incidental |
| `GC_CONTROL_DISPATCHER_TRACE_DEFAULT` | default control-dispatcher trace path | always | incidental |
| `GC_RIG_ROOT` | rig root path | rig-scoped sessions (empty otherwise) | incidental |
| `GC_DIR` | agent work dir | always (`SyncWorkDirEnv`) | incidental |
| `GC_HOME` | Gas City home (`~/.gc`) | always | incidental |
| `GC_BIN` | gc binary wrapper path (`.gc-wrapped`) | always | incidental |
| `GC_BEADS` | beads provider selector (`bd`) | always | incidental |
| `GC_BEADS_SCOPE_ROOT` | bead-store scope root | always for managed sessions | incidental |
| `GC_DOLT_HOST`, `GC_DOLT_PORT` | Dolt coordinates | always (may be empty) | incidental |
| `GC_DISABLE_USAGE_METRICS` | usage-metrics opt-out (`1`) | when the parent sets it | incidental |
| `GC_SUPERVISOR_PRESERVE_SESSIONS_ON_SIGNAL` | supervisor signal behavior (`1`) | managed-city sessions | incidental |
| `GC_STARTUP_PROMPT_DELIVERED` | launch-only marker that the startup prompt was delivered | some launches (named/restart); absent from the control-dispatcher start | incidental |
| `GC_CONTROLLER_TOKEN` | controller-only token | always; scrubbed to empty at the agent boundary | incidental |
| `GC_SPAWN_ORIGIN`, `GC_TRIGGER_BEAD_ID`, `GC_TRIGGER_BEAD_STORE_REF`, `GC_TRIGGER_WORK_BEAD_ID`, `GC_TRIGGER_WORK_STORE_REF` | demand-spawn markers (`GC_SPAWN_ORIGIN="demand"`) | demand spawns only | incidental |
| `GC_PACKER_PACK` | pack that requested the spawn | pack spawns only | incidental |

`GC_WORKFLOW_TRACE` appears only inside the control-dispatcher launch command
(a `sh -c` prefix), not in the start-config `env` map.

### 13.4 Provider meta keys (get-meta, not start-config env)

gc reads these from the provider over the `get-meta` op, not from the start
config: `GC_PROVIDER`, `GC_SESSION_ID`, `GC_DRAIN_ACK`, `GC_ALIAS`, `GC_AGENT`,
and `suspended`. The capture also shows `get-meta … GC_INSTANCE_TOKEN`, and the
source reads `GC_CONTINUATION_EPOCH` and `GC_RUNTIME_EPOCH` the same way. The
provider serves these from its metadata sidecar (`set-meta`/`get-meta`/
`remove-meta`), which is separate from the launch environment. The meta ops
resolve a rig-qualified alias to the pane's primary name, so a key written
under the runtime instance name is readable under the canonical identity.
