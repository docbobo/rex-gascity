# Configuring Gas City to use Rex instead of tmux

Research notes, 2026-11-17. Scope: what Gas City is, how its session
backends work, what the existing **herdr** integration did, and the two
realistic paths to a **Rex** backend.

Repo note: this repository was empty (`git init` only, no commits, no
remote) when these notes were written, so there is no existing convention
for research notes. They live at `docs/research/`.

---

## 1. TL;DR

Gas City (`gc`) already abstracts the terminal multiplexer behind a
runtime-provider interface. tmux is the default **and** the fallback; there
are also `subprocess`, `ssh`, `k8s`, `acp`, `t3bridge`, a native **`herdr`**
backend, an `exec:<script>` escape hatch, and pack-declared runtimes.

There are two ways to run Gas City's agents under Rex:

- **Path A — exec session provider (no Go).** Write a `gc-session-rex`
  script that implements Gas City's Runtime Provider Protocol (RPP v0) by
  shelling out to the `rex` CLI, then select it with
  `GC_SESSION=exec:/path/gc-session-rex`. Fastest to prototype; every op is
  a process spawn; no native event integration.
- **Path B — native Go provider (`internal/runtime/rex`).** Mirror the
  herdr provider: a thin `rex` CLI/socket client, a `Provider`
  implementation, a `registry.Register("rex", factory)` line in `cmd/gc`,
  and the `runtimetest` conformance suite. More work, but cleaner mapping,
  native events, and it can participate in the same idle/attach machinery
  as tmux and herdr.

Recommendation: **spike Path A first** (it needs no gc source changes and
validates the op mapping), then **productise as Path B** if Rex is meant to
be a first-class backend. Follow the herdr pilot playbook: keep the
orchestrator (mayor) on tmux while validating, then flip city-wide.

---

## 2. What Gas City is

Gas City is a Go orchestrator (repo `gastownhall/gascity`, MIT) that turns a
fleet of coding agents (Claude Code, Codex, Gemini, …) into a "software
factory". You write a **formula** describing how a job gets done; the
orchestrator decomposes it into **beads**, fans ready work out to as many
agents as allowed, gates each step, retries failures, and reconciles
declared agents against live sessions.

Runtime dependencies for a local city are `gc`, `tmux`, `git`, `jq`, and a
beads provider (`bd` + `dolt`, or `GC_BEADS=file`). tmux is required even
when agents run on another backend, because tmux remains the default and
fallback.

Sources:
- <https://github.com/gastownhall/gascity>
- <https://docs.gascity.com/getting-started/how-gas-city-works>
- <https://docs.gascity.com/getting-started/quickstart>

---

## 3. The session/runtime provider model

An agent is composed along five axes: **harness**, **model**, **upstream**,
**transport**, **runtime**. The runtime is "where the session's box lives".
Backend selection is:

- **City-wide:** `[session] provider = "<name>"` in `city.toml`.
- **Process-wide (one-off):** `export GC_SESSION=<name>`.
- **Per-agent `session = ...` selects a _transport_, not a backend** — it
  accepts only `acp`, `tmux`, or omission. `session = "herdr"` (or "rex") is
  rejected/warned, and under a non-tmux city an agent's `session = "tmux"`
  pin is not honoured. So a Rex backend is a whole-city (or whole-process)
  choice, exactly like herdr.

Registration is name-based in `internal/runtime/registry`; builtins register
from `cmd/gc/runtime_registry.go` (`r.Register("herdr", ...)`,
`r.Register("tmux", ...)`, `r.RegisterPrefix("exec:", ...)`, …). Pack
runtimes declare themselves in `pack.toml`:

```toml
[runtimes.rex]
command = "scripts/gc-runtime-rex"   # pack-relative, or a PATH name
protocol = 0
```

…and are then selected the same way (`[session] provider = "rex"`).

### Core `Provider` interface

`internal/runtime/runtime.go` defines the required surface (24 methods).
Implementations must be safe for concurrent use:

| Method | Purpose |
|---|---|
| `Start(ctx, name, cfg)` | create session + launch agent |
| `Stop(name)` | destroy session (idempotent if gone) |
| `Interrupt(name)` | soft interrupt (Ctrl-C / SIGINT) |
| `IsRunning(name)` | does the runtime box exist |
| `IsAttached(name)` | is a human terminal attached |
| `Attach(name)` | attach interactively (blocks) |
| `ProcessAlive(name, names)` | is the agent process in the tree |
| `Nudge(name, content)` | text + submit to wake/redirect the agent |
| `SetMeta` / `GetMeta` / `RemoveMeta` | per-session KV (drain, fingerprint) |
| `Peek(name, lines)` | capture last N lines |
| `ListRunning(prefix)` | enumerate sessions (orphan detection) |
| `GetLastActivity(name)` | last I/O time (zero = unsupported) |
| `ClearScrollback(name)` | best-effort |
| `CopyTo(name, src, relDst)` | push a file into the session fs |
| `SendKeys(name, keys...)` | raw key events, no Enter |
| `RunLive(name, cfg)` | re-apply `session_live` commands |
| `Capabilities()` | self-reported detection abilities |

### Optional capability interfaces

A provider opts into more behaviour by implementing extra interfaces:
`IdleWaitProvider` (`WaitForIdle`), `IdleSnapshotProvider` (`SnapshotIdle`),
`ExecProvider` (`Exec`), `DialogProvider` (`DismissKnownDialogs`),
`InteractionProvider`, `RelaunchProvider` (`Relaunch` in a warm box),
`ProcessTableScanner`, `ServerLifecycleProvider`
(`ConfigureServer`/`TeardownServer`), `SessionRosterProvider`,
`InventoryProvider`, `TransportCapabilityProvider`, and others. Missing
optional capabilities degrade gracefully.

### exec session provider (the no-Go escape hatch)

Set `GC_SESSION=exec:<script>` and gc delegates every provider operation to
your script: `<script> <operation> <session-name> [args...]`, invoked
directly (no shell). Operations include `start`, `stop`, `interrupt`,
`send-keys`, `clear-scrollback`, `is-running`, `attach`, `process-alive`,
`nudge`, `set-meta`/`get-meta`/`remove-meta`, `peek`, `list-running`,
`get-last-activity`, `protocol`, `is-attached`, `exec`, `provision`,
`observe`, `inventory`, `kill-object`, `server-stop`.

A `protocol` op returns a handshake:

```json
{"version": 0, "capabilities": ["report-attachment", "report-activity"], "locality": "local"}
```

Capabilities that matter for a Rex backend:
- `report-attachment` → `is-attached` is called and trusted.
- `report-activity` → `get-last-activity` results are meaningful.
- `carrier.ops` → selects the five dedicated driving ops (`nudge`, `peek`,
  `send-keys`, `interrupt`, `clear-scrollback`) with no tmux; all five are
  required, and `set-meta`/`get-meta` must round-trip the ready prompt.
- `proc.exec` → the `exec` op's exit code is the in-box command's exit code.
- `fs.host` (with `"locality": "local"`) → boxes share gc's filesystem and
  user, which is what Rex blocks on the local host would do.
- `server-lifecycle` → gc runs `server-stop` at city shutdown.

Without `carrier.ops`, gc tries tmux over `exec` first; a Rex script that
does not run tmux in its box should declare `carrier.ops` (or accept that
the default carrier's tmux probes will fail).

Sources:
- <https://docs.gascity.com/guides/configuring-an-agent>
- <https://docs.gascity.com/reference/exec-session-provider>
- `internal/runtime/runtime.go`, `cmd/gc/runtime_registry.go`

---

## 4. How the herdr integration did it (the reference)

herdr (<https://herdr.dev>) is a terminal workspace manager for AI coding
agents. Gas City ships a native, opt-in `herdr` backend — the closest
analogue to what a Rex backend would be.

- **Layout:** one shared herdr session-server per city, one workspace per
  rig (plus one for the town), one tab per agent. Agent names map 1:1 to gc
  session names, with a deterministic name mapping (`herdrAgentName`: lower-
  case, charmap to `-`, 24-char head + fnv32 hash beyond 32 chars).
- **Pane binding is the stable handle:** Start persists pane/tab/workspace
  IDs, launch mode, exact session name, and a timestamp in a **sidecar file**
  (`GC_HERDR_*`). Name→pane resolution goes registry-name first, then the
  sidecar binding verified by a live probe; confirmed-gone panes clear the
  binding (ids recycle).
- **Registration:** `r.Register("herdr", factory)` in `cmd/gc`; the factory
  builds `sessionherdr.New(cityName, providerStateDir("herdr", cityPath), …)`.
  It passes `hostFilesystemAndPreStart` (boxes run on the host, pre_start
  runs there).
- **Validation:** passes the `runtimetest` Provider conformance suite; live
  tests are opt-in (`make test-herdr-live`). The default suite runs against a
  fake backend.
- **Event stream:** the provider implements `runtime.SessionEventProvider`
  and consumes herdr's push events, but the **controller does not yet consume
  it** — the reconciler still discovers state on its periodic pass; events
  only accelerate activity reporting. (Latency, not correctness.)
- **What tmux provides that herdr had to bridge:**
  - no formal signal API → soft `Interrupt` = `send-keys ctrl+c`; hard kill
    via the PID from `pane process-info`;
  - no per-session KV → sidecar file;
  - no `ClearScrollback` → best-effort no-op;
  - `IsAttached` and `GetLastActivity` not directly exposed → best-effort /
    a polling activity tracker.

### Hard-won lessons (transferable to Rex)

1. **Pin the backend's version.** herdr 0.7.4/0.7.5 changed the CLI surface
   and caused a pane/shell spawn storm (496 stray shells). Drive a real
   binary in the live tests and re-run them after every upgrade.
2. **Model liveness explicitly.** herdr clears agent names on occupant change
   (shell→TUI boot handoff), so name-keyed lookups go dark on a live agent;
   the adapter had to key on **pane ID + sidecar** and reap exited panes to
   stop leaks.
3. **Start ordering matters.** Seed the binding before the (seconds-long)
   launch so reconcile ticks that fire mid-boot do not roll the fresh runtime
   back.
4. **Pilot behind the selector.** Flip to the new backend but pin the mayor
   (orchestrator) to tmux first — a wedged orchestrator cannot self-recover.
   Promote only when boring.
5. **Conformance suite is the safety net.** `gc runtime check <name>` is the
   smoke test; `gc runtime conformance <name>` is the requirement-coded
   suite (`RPP-*`).

Sources:
- <https://docs.gascity.com/reference/herdr-provider>
- <https://deepwiki.com/gastownhall/gascity/5.4-herdr-and-t3-bridge-providers>
- `internal/runtime/herdr-provider-design.md`, `internal/runtime/herdr/`

---

## 5. Rex capability inventory (what we drive it with)

Rex's model: **session → window (tab) → block (pane)**. The CLI is
`rex`, version here `0.1.0-preview.19`, API revision 14, server on a unix
socket (`~/.…/Application Support/rex/server.sock`). Relevant verbs:

| Need | Rex command |
|---|---|
| Create a session (with initial command) | `rex new NAME -c CWD -- CMD…` → `--json` gives `session_id`, `initial_windows[0].block_ids` |
| Create a tab/window | `rex window new -s SESSION LABEL -- CMD…` (`--json`, `-c`, `--focus=false`) |
| Run a command in a new block | `rex run -s SESSION -w WINDOW --label TEXT -c CWD -- CMD…` (`--json`) |
| Split beside a block | `rex split -s SESSION -b BLOCK --split=right -- CMD…` |
| Send text (no Enter) | `rex send -b BLOCK -- 'text'` (or `rex block send`) |
| Send keys | `rex send-key -b BLOCK ctrl+c`, `enter` (Rex key-binding syntax) |
| Read screen + scrollback | `rex capture -b BLOCK --trim` (`--format=text|html|vt`) |
| List sessions | `rex ls --json` |
| List blocks in a session | `rex block ls -s SESSION --json` (id, label, window_id, …) |
| Inspect session/block | `rex session inspect --json` / `rex block inspect BLOCK --json` |
| Stream events | `rex events -s SESSION --json` (or `--all`) |
| Wait for a block/window/session | `rex wait TARGET [--json] [--for DURATION]` |
| Close a block | `rex block close BLOCK` |
| Destroy a session | `rex kill SESSION` |
| Clients / attachment | `rex client ls`, `rex session inspect` (clients list) |
| Low-level control API | `rex api list` / `describe` / `call` (e.g. `session.view`, `session.focus_block`, `block.close`) |
| Actions | `rex actions --json`; `rex -C CLIENT do session.select session_id=…` |

Observations from a live server (read-only):
- `rex block ls --json` reports `block_id`, `label`, `creator_name`,
  `flavor`, `host_id`, `placement`, `window_id`, `owner` — but **no**
  attached flag or activity timestamp.
- `rex session inspect --json` reports windows, layouts, blocks, and the
  attached `clients` list → usable for `IsAttached`.
- `rex api list` exposes `session.list`, `session.view`,
  `session.describe_block`, `client.list`, `block.close`, `block.event`,
  `block.method`, `server.stop`, etc.
- `rex events` streams session creation/removal/state changes → usable for
  `ListRunning` deltas and an activity tracker.

---

## 6. RPP / Provider → Rex mapping

Proposed mapping (Path A script, or the internals of a Path B provider):

| gc operation | Rex | Notes |
|---|---|---|
| `Start(name, cfg)` | `rex new <city-session>` (once) + `rex window new … -- CMD` or `rex run … -- CMD` | Bake `cfg.WorkDir` into `-c`; env via wrapper. Persist the returned `window_id`/`block_id` in a sidecar. |
| `Stop(name)` | `rex block close <block>` (idempotent) or `rex kill <session>` | Close the agent's window/block; keep the shared city session. |
| `Interrupt(name)` | `rex send-key -b <block> ctrl+c` | Keystroke, not a real signal (same gap as herdr). |
| `IsRunning(name)` | `rex block ls --json` presence, or `rex session inspect` | Prefer a sidecar binding verified by a live `rex block inspect`. |
| `IsAttached(name)` | `rex session inspect` clients list | Map attachment to the session/window holding the agent. |
| `Attach(name)` | `rex -C CLIENT do session.select session_id=…` / `session.focus_block` | Or focus the block in the user's Rex app. |
| `ProcessAlive(name, names)` | `rex block inspect` / terminal child status | Find the agent process in the block's process tree if Rex exposes it; otherwise best-effort. |
| `Nudge(name, content)` | `rex send -b <block> -- '<text>'` then `rex send-key -b <block> enter` | Two steps: text then submit. Rex `send` does not append Enter. |
| `SendKeys(name, keys…)` | `rex send-key -b <block> KEY…` | Translate tmux key names (`C-c`, `Enter`) to Rex syntax (`ctrl+c`, `enter`). |
| `Peek(name, lines)` | `rex capture -b <block> --trim` then keep last `lines` | Rex capture returns screen + scrollback. |
| `ListRunning(prefix)` | `rex ls --json` and/or `rex block ls --json` with the gc label prefix | Depends on the layout choice below. |
| `GetLastActivity(name)` | `rex events -s … --json` / poll `rex capture` | No timestamp in the API → provider-maintained tracker (same bridge herdr needed). |
| `SetMeta/GetMeta/RemoveMeta` | — | No general KV → sidecar file (as herdr). |
| `ClearScrollback(name)` | — | Best-effort no-op, or reconfigure the block. |
| `CopyTo(name, src, relDst)` | host filesystem | With `fs.host`/local boxes, a plain host copy. |
| `RunLive(name, cfg)` | re-issue `rex run …` in the block | Or `rex send` the `session_live` commands. |
| `Exec(name, argv)` | `rex run --wait --json` + capture | `--wait` gives the child exit status; output must be captured separately. |
| `ServerLifecycle` | `rex server stop` is **too broad** | The Rex server is shared by the user's other work; prefer city-scoped session teardown (close the city session / `GC_EXEC_CITY_PATH`), or no `server-stop`. |

### Layout options

Two viable hierarchies:

1. **One Rex session per gc session (agent = session).** Simplest for the
   driving ops: `rex new <name>`, `rex kill <name>`, `rex ls` for
   `ListRunning`. Cost: many sessions in the Rex UI, no rig grouping.
2. **One Rex session per city, window per rig, block per agent** (mirrors
   herdr). Nicer UI and rig grouping, but `ListRunning` must enumerate
   windows/blocks by label and everything keys off a sidecar binding, like
   herdr's `panebinding.go`.

For a first spike, option 1 is materially less code. For production parity
with herdr, option 2 is the right shape.

---

## 7. Known gaps and bridges

| Gap | Bridge |
|---|---|
| No per-session KV in Rex | Sidecar JSON keyed by session name (herdr does exactly this). Required for drain signaling + config fingerprint. |
| No `GetLastActivity` timestamp | Provider-side activity tracker driven by `rex events --json`, plus `rex capture` change detection. |
| `IsAttached` not a first-class field | Derive from `rex session inspect`'s clients list / `rex client ls`. |
| `ProcessAlive` needs the agent process identity | Inspect the block's process tree if exposed; otherwise report "no check possible" (the interface returns true for an empty process-name list). |
| No hard signal API | Soft interrupt = `send-key ctrl+c`; hard kill = close the block / `rex kill`. |
| `server-stop` would stop a **shared** Rex server | Do not declare `server-lifecycle`, or scope teardown strictly to the city's own session and leave the server running while anything else uses it. |
| Rex is a moving target (`0.1.0-preview.N`) | Pin the version, keep live tests opt-in but run them after every bump — the herdr 0.7.4/0.7.5 storm is the cautionary tale. |

---

## 8. Concrete configuration

### A. Local trial with an exec script (no gc changes)

```bash
# 1. Write the adapter (implements the RPP v0 ops over the rex CLI):
#    e.g. /path/to/gc-session-rex
chmod +x /path/to/gc-session-rex

# 2. Point one gc process at it (leaves you on tmux otherwise):
export GC_SESSION=exec:/path/to/gc-session-rex
gc start <scratch-city>
```

Prefer an absolute path: a PATH lookup resolves per gc process, so the
supervisor, `gc start`, and cron could otherwise pick different files.

Verify:

```bash
gc config show              # effective [session] provider
gc runtime check exec:/path/to/gc-session-rex
gc runtime conformance exec:/path/to/gc-session-rex
```

### B. Native provider selected city-wide

Once `internal/runtime/rex` exists and is registered as `"rex"`:

```toml
# city.toml
[session]
provider = "rex"
```

Reload with `gc reload` (or restart the city). Running sessions keep their
current backend until they next restart.

### C. Pack-declared runtime

```toml
# pack.toml
[runtimes.rex]
command = "scripts/gc-runtime-rex"
protocol = 0
```

```toml
# city.toml
[session]
provider = "rex"
```

Prerequisites for Rex: the `rex` binary on PATH and a reachable Rex server
(auto-started locally, or a named `-S` host). Note the local Nix role at
`~/.config/nixpkgs/roles/gascity/default.nix` currently pulls `gascity`,
`beads`, `dolt`, and `beads-rust`; it would need `rex` added if the backend
is adopted.

---

## 9. Risks / things to validate before trusting it

- **Idle detection.** gc waits for a ready prompt (`ready_prompt_prefix`,
  default `❯ `) and no busy indicator. Rex capture must be reliable enough
  for the two-capture idle check within the 5 s budget; otherwise declare
  `timed_only` sleep (interactive sessions never sleep) rather than lie.
- **`ListRunning` completeness.** Orphan detection closes sessions it
  cannot see; an incomplete list risks reaping live agents. Mirror herdr's
  sidecar binding.
- **Attachment semantics.** Rex attachment is per-client/session, not
  per-block; a client viewing the session may false-positive as "attached",
  which holds sessions awake. Test honestly.
- **Startup dialogs.** tmux/herdr have matchers for trust/permission
  dialogs. Confirm how Rex surfaces them (the block shows the TUI; same
  capture approach should work).
- **Shared server teardown.** Never let gc stop the user's Rex server.
- **Preview churn.** Rex's CLI/socket API is pre-1.0; pin and test.

---

## 10. Suggested next steps

1. Decide the layout (agent-per-session vs city/rig/agent hierarchy).
2. Build the Path A `gc-session-rex` spike: `protocol` handshake
   (`carrier.ops`, `report-attachment`, `report-activity`), then `start`,
   `is-running`, `peek`, `nudge`, `send-keys`, `interrupt`, `stop`,
   `list-running`. Validate with `gc runtime check`.
3. If it holds: add `internal/runtime/rex/` (client + provider +
   capabilities + sidecar), register `"rex"` in `cmd/gc/runtime_registry.go`,
   and pass `gc runtime conformance`.
4. Pilot on a scratch city with the mayor pinned to tmux, watch
   liveness/wedge detection, then promote city-wide.

---

## 11. Sources

Gas City (primary):
- Repo: <https://github.com/gastownhall/gascity>
- Docs index: <https://docs.gascity.com/llms.txt>
- herdr provider: <https://docs.gascity.com/reference/herdr-provider>
- Exec provider: <https://docs.gascity.com/reference/exec-session-provider>
- Configuring an agent: <https://docs.gascity.com/guides/configuring-an-agent>
- How Gas City works: <https://docs.gascity.com/getting-started/how-gas-city-works>
- Source read at `main`: `internal/runtime/runtime.go` (Provider + capability
  interfaces), `internal/runtime/herdr-provider-design.md`,
  `internal/runtime/herdr/provider.go`, `cmd/gc/runtime_registry.go`.
- DeepWiki herdr/T3: <https://deepwiki.com/gastownhall/gascity/5.4-herdr-and-t3-bridge-providers>

Rex (primary):
- Local `rex` CLI `0.1.0-preview.19`, API revision 14: `rex --help`,
  per-command `--help`, `rex api list`, `rex session inspect --json`,
  `rex block ls --json`, `rex client ls`.
- Rex skill guide: `rex skill show`.
- Local Nix role: `~/.config/nixpkgs/roles/gascity/default.nix`.
