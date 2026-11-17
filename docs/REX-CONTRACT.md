# Rex contract (pinned)

Phase 0 artifact for `docs/plans/native-rex-provider.md`. The Rex surface the
`rex` runtime provider relies on, captured against one pinned version. **Re-run
the probes in §9 after any Rex upgrade** — the CLI/API is pre-1.0.

| Field | Value |
|---|---|
| Rex version | `0.1.0-preview.19` |
| Rex API revision | `14` |
| Server | unix socket (`REX_SERVER` / `~/.…/Application Support/rex/server.sock`) |
| gc side | RPP v0 (`internal/runtime/protocol.go`, `docs/reference/exec-session-provider.md`) |
| Captured | 2026-11-17 |

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

## 3. Topology methods

| Method | Inputs | Returns / notes |
|---|---|---|
| `session.create` | `label`, `initial_windows[]` (`layout`, `window_label`) | `initial_windows[].block_ids`, `window_id`, `revision`; atomic (all-or-nothing) |
| `session.new_window` | `session_id`, `layout` (block `{flavor, options, label}` or `split`), `window_label`, `focus` | `window_id`, `block_ids[]`, `revision` |
| `session.new_split` | `session_id`, `anchor_id` **or** `window_id`, `direction` `horizontal|vertical`, `side` `before|after`, `ratio` (0<r<1), `layout`, `focus` | creates blocks beside an existing block/split |
| `session.view` | `session_id` | `active_window_id`, `label`, `revision` (increments per change, jumps on restore), `windows[].{window_id, focused_block_id, layers[].blocks[]}`, `clients[]` |
| `session.list` | — | `sessions[] {session_id, label}` in creation order |
| `session.destroy` | `session_id` | **requires the caller to be attached to the session; otherwise fails `conflict`.** Closes every block |
| `session.focus_block` / `session.focus_window` | ids | focus a pane / tab |
| `session.set_block_label` / `session.set_window_label` / `session.set_label` | ids, label | ≤128 chars |
| `block.close` | `session_id`, `block_id` **or** `window_id`, `keep_last` | closes a pane; `keep_last` refuses the last block in a window |
| `block.event` | `session_id`, `block_id`, `event {type, payload}` | input event; `type` ∈ `key`, `paste`, `mouse`, `wheel`, `focus` |
| `block.method` / `rex block call` | `session_id`, `block_id`, method name + JSON args | call a block method (§4); `session_id` is required even though `block_id` is unique |
| `target.resolve` / `target.complete` | id/label/prefix | resolve labels, window positions, short ids |

Terminal block creator: `com.superlogical.terminal`, flavor
`com.superlogical.terminal.shell`. Launch options:
`command` (argv array), `shell` (`none` starts directly), `cwd`, `login`,
`interactive`.

## 4. Terminal block methods

| Method | Input | Output | Use |
|---|---|---|---|
| `process` | — | `{last_exit: {exit_code, error?} \| null, child, foreground, ancestors[], home}`; each process `{pid, user_id, user, name, path, invoked_path, argv0, cwd}` | `process-alive`; agent-exit detection (`last_exit`) |
| `format` | `{format: text\|html\|vt, trim, unwrap, extra_charsets, extra_cursor, max_bytes}` | `{content, id?, total?, eof?}` — `id` set means more remains; continue with `blob_read {id, offset}`. With no `max_bytes` the whole output is returned in one reply, or the call fails `too_large`; with `max_bytes` set, `total`/`eof` describe the whole result | `peek`; send output; exec output |
| `blob_read` | `{id, offset, max_bytes?}` | `{id, data, total, eof}` — all fields required; `eof` true when `data` reaches the end | `peek` paging; large events |
| `write` | `{data}` | `{bytes_written}` | type text (`nudge`) |
| `clear` | — | — | `clear-scrollback` (is a no-op while a full-screen program runs) |
| `reset` | — | — | hard reset of a terminal (fallback) |
| `program_status` | — | `{records[] {id, state: idle\|working\|done\|blocked\|error, kind, …}}` — programs report via **OSC 7501** | agent state; idle detection |
| `input_mode` | — | `{echo, canonical, signals, …}` | detect a password/raw prompt |
| `size` / `title` / `pwd` | — | terminal geometry / title+owner / cwd | diagnostics |

`program_status` is **reported by the program itself** (OSC 7501), not detected
by Rex. gc already ships a matching emitter (`internal/runtime/runtimetest/osc7501emitter`),
so an agent that emits OSC 7501 gives us native idle/busy state; otherwise fall
back to scanning `format` for the ready prompt.

## 5. Terminal block events

`bell`, `child_exited` (`{exit_code, error?}`), `process_changed`
(`{generation, foreground}`), `program_status_changed`,
`program_status_removed`, `title_changed`, `pwd_changed`, `size_changed`,
`clipboard_written`, `selection_copied`, `desktop_notification`,
`progress_report`, `url_clicked`.

`rex events --json` streams these plus session create/remove/state changes.
Treat events as **level-triggered hints to re-poll**, never authoritative
transitions (Rex replays/omits under load).

## 6. Input/key names

`rex send-key` / `block.event {type:"key"}` use Rex key bindings: modifiers
joined with `+` (`ctrl+c`, `shift+left`); named keys (`enter`, `esc`); a
bracketed name (`[Slash]`) selects a physical key. Translate gc/tmux names:
`C-c` → `ctrl+c`, `Enter` → `enter`, `Escape` → `esc`.

`write` types literal text (escape sequences allowed, e.g. `\u001b[A` for Up);
`block.event {type:"paste", payload:{text}}` pastes. `nudge` = `write` text +
submit (Enter).

## 7. Transport abstraction (must stay swappable)

The provider core depends on a `rex.Client` interface, SDK-style, one method per
operation. v1 implementation (`rex/cli`) shells out; a future `rex/wire`
implementation talks the unix socket directly. Provider logic must not know
which is in use. See plan §3 for the interface sketch.

The socket protocol itself is **not documented/stable**; v1 uses the CLI
deliberately.

## 8. RPP v0 (gc → provider)

Invocation: `executable <op> <name> [args…]`, JSON on stdin where the op takes
structured input, JSON/text on stdout. No shell. One process per op.

Exit codes: `0` success · `1` failure (stderr message) · `2` unknown op
(forward-compatible; from a *declared* op it is a failure) · `66` `peek` no
screen (v2-ready only) · `75` `start` temporary/capacity (retryable) · `80`
input not delivered (`input-outcome` only).

Ops: `start`, `stop`, `interrupt`, `send-keys`, `clear-scrollback`,
`is-running`, `attach`, `process-alive`, `nudge`, `set-meta`, `get-meta`,
`remove-meta`, `peek`, `list-running`, `get-last-activity`, `protocol`,
`is-attached`, `exec`, `provision`, `observe`, `inventory`, `kill-object`,
`server-stop`.

Handshake (`protocol` op):

```json
{"version": 0, "capabilities": ["fs.host", "report-attachment", "report-activity", "carrier.ops", "idle-boundary", "observe.v1"], "locality": "local"}
```

Capabilities we intend to declare: `fs.host` (boxes are gc's host/user),
`report-attachment`, `report-activity`, `carrier.ops` (dedicated driving ops,
no tmux), `idle-boundary` (needs `carrier.ops` + `set-meta`/`get-meta`
round-tripping `GC_READY_PROMPT_PREFIX`), `observe.v1` (reconciler-v2 reads).
**Do not declare `server-lifecycle`** — teardown would stop the shared Rex
server (D3).

## 9. Re-verification probes

Run after any Rex upgrade; each must match the tables above.

```bash
rex --version; rex whoami --json | jq .api_revision
rex api describe session.create
rex api describe session.new_window
rex api describe session.new_split
rex api describe session.view
rex api describe session.destroy
rex api describe block.close
rex api describe block.event
rex block inspect "$REX_BLOCK" --json | jq '.api.methods | keys'
rex block call -b "$REX_BLOCK" process
printf '{"format":"text","trim":true}' | rex block call -b "$REX_BLOCK" format -
rex block call -b "$REX_BLOCK" program_status
rex events --help
```

## 10. Open items to resolve in Phase 1

- **`format` paging:** resolved in Phase 1 (GCREX-2). `peek` requests
  `format {max_bytes: 983040}` and follows `id` with `blob_read` until `eof`.
  Verified against the live server with a ~4 MiB capture.
- **`session.destroy` needs attachment:** teardown may have to `session.attach`
  (or use `block.close` only and leave empty sessions).
- **Pane process-identity stability:** `block_id` is persisted in the sidecar;
  confirm ids survive a server restart (vs. address-by-label).
- **Rig-tab growth:** repeated `session.new_split` into one tab has layout
  limits; decide the split policy when a rig has many agents.
