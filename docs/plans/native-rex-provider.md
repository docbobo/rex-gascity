# Plan: native Rex session provider for Gas City

Status: **DONE (Phase 5 complete; `rex` promoted as the supported opt-in backend)** – D0–D6 confirmed (`docs/REX-CONTRACT.md` §11).
Phases 0–3 are implemented: the Rex contract is pinned; the `rex.Client`
interface, its CLI implementation, a fake client, and the provider core
(`SessionName`, `Binder`, `Provider` with Start/Stop/IsRunning/Peek/Nudge/
SendKeys/Interrupt) exist and have tests; and Phase 3 adds the file sidecar
(bindings, activity fingerprints, metadata), idempotent `Start`, block-only
teardown, per-agent attachment honesty, and `GetLastActivity`. Phase 4
(GCREX-5) adds the RPP v0 dispatcher (`cmd/gc-runtime-rex`), the `protocol`
handshake, the pack `[runtimes.rex]` declaration, and the user docs;
`gc runtime check rex` and `gc runtime conformance rex` pass against it.
Phase 5 (GCREX-6) is done: the pilot passed and `[session] provider = "rex"` is promoted as an opt-in backend; see `docs/rex-pilot-phase5.md`.
Pinned baseline: gc 1.5.1 and the Rex surface in `docs/REX-CONTRACT.md`; where
this plan and that document disagree, the contract wins.
Target: an opt-in Rex session backend for Gas City – a **standalone
executable** (`gc-runtime-rex`) that speaks RPP v0, shipped by a runtime pack and selectable
as the runtime name `rex` (`[session] provider = "rex"` or `GC_SESSION=rex`).

**Delivery form (D0, decided): an out-of-tree executable loaded as a pack
runtime (`[runtimes.rex]`, speaking RPP v0). It is not compiled into gc and
does not import gc's Go packages.** See §0 D0 and §3.

The `herdr` backend (`internal/runtime/herdr/`) is the closest *behavioral*
precedent and the main reference for what each op must do — but it is in-tree,
which D0 rules out for Rex, so the `cloudflare` pack runtime is the reference
for the delivery model. Read alongside `docs/research/rex-session-provider.md`
(the survey), `internal/runtime/herdr-provider-design.md` (behavior), and
`engdocs/design/runtime-provider-packs.md` (delivery).

---

## 0. Decisions (D0–D6, all confirmed)

### D0 — Delivery form: does this have to be compiled into GC? — **DECIDED: option 1**

**No.** There are three ways to ship a provider, and only one recompiles gc:

| Option | How it is selected | Compiles into gc? | Gets today |
|---|---|---|---|
| **1. Pack-declared runtime — CHOSEN** | `[runtimes.rex]` in `pack.toml` → `command = "gc-runtime-rex"`, `protocol = 0`; then `[session] provider = "rex"` | **No.** A pack can ship/install any executable that speaks Runtime Provider Protocol v0; city composition registers the name like a builtin. | The RPP v0 op set plus the optional capabilities gc 1.5.1 defines: `report-attachment`, `report-activity`, `proc.exec`, `proc.provision` (needs `proc.exec`), and the reserved `proc.stream` and `tty.attach`. See §5. `gc runtime check rex` / `gc runtime conformance rex` validate it. |
| **2. In-tree Go provider — not chosen** | builtin name `rex` via `registry.Register("rex", …)` | **Yes** — edits `cmd/gc/runtime_registry.go` and ships in the gc binary. | Direct Go `runtime.Provider` and the Go-only capability interfaces (e.g. `runtime.SessionEventProvider` for push events). |
| **3. Raw exec script** | `GC_SESSION=exec:/path/gc-session-rex` | No | Same RPP surface as (1), but the selector is `exec:<path>`, not a clean `rex` name. This is "Path A", already set aside. |

Why (1) is sufficient for a *native* provider: RPP v0 covers the whole
`Provider` contract as wire ops — `start`, `stop`, `interrupt`, `send-keys`,
`clear-scrollback`, `is-running`, `attach`, `process-alive`, `nudge`,
`set-meta`/`get-meta`/`remove-meta`, `peek`, `list-running`,
`get-last-activity`, `is-attached`, `exec`, `provision`, plus the `protocol`
handshake (the 17-row table in `docs/REX-CONTRACT.md` §9; gc also calls
`clear-scrollback`, `send-keys` and `watch-startup`).
Nothing about being "native" requires the code to live in gc's tree; it
requires speaking the protocol and being selectable by name.

What (2) uniquely buys, and why it is not worth it yet:

- **Push session events** (`runtime.SessionEventProvider`) are the only
  capability gated to a Go provider. But the exec provider's stream op
  (`proc.stream`) is still *reserved/unimplemented*, **and** — per the herdr
  provider docs — **the gc controller does not consume any provider's
  session-event stream yet**; the reconciler still discovers state on its
  periodic pass. So even the in-tree path gets no event benefit today.
- A builtin name without a pack. Option (1) already provides a clean `rex`
  selection name, so this is not a real gap.

**Decision (confirmed):** build the provider as **option 1** — a standalone Go
module (`gc-runtime-rex`, no gascity imports) delivered out-of-tree as a pack
runtime, installed by the pack, never linked into gc. Revisit (2) only if/when
the controller actually consumes session events and we want the builtin to
ship one.

---

These choices shape everything else. D0–D6 are all **confirmed** in
`docs/REX-CONTRACT.md` §11. The Alternative column is kept as history only.

### D1–D6

| # | Decision | Confirmed | Alternative (not chosen) |
|---|---|---|---|
| D1 | **Layout mapping** (diagrams in §4) | **Option B — CHOSEN.** Rex **session = city** (`gc:<city>`), **tab = rig**, **pane = agent** (agents tiled in the rig's tab). One Rex session per city; fewest sessions. | **Option A** — session = rig, tab = agent (each agent its own tab). **Option C** — session = agent (simplest, no grouping). |
| D2 | **Transport** | Shell out to the `rex` CLI **now**, but only through a stable `rex.Client` interface; a direct unix-socket (JSON wire) implementation can be dropped in later without touching provider logic. | Go straight to the socket — Rex's CLI is versioned and documented; the socket protocol is not. |
| D3 | **Server ownership** | The provider never stops the server and does not implement a server-teardown op. Never stop the shared Rex server — the user's own work runs on it. Teardown closes only city-owned blocks with `block.close`. `session.destroy` runs in one place: removing the city's own canonical `gc:<city>` session once its last pane is gone, behind the fail-closed empty check (GCREX-31, `docs/REX-CONTRACT.md` §12). An opt-in city-owned server is addressable with `GC_REX_SERVER` (`-S`, `gcrex-d3m0i`). The default stays the shared server. | Own a per-city server — impossible; Rex has one shared server, not one per city. |
| D4 | **Scope** | Local Rex server only (default socket, or `-S <host>` for a remote Rex that shares the filesystem). Agents run **on gc's host**, so pre_start/session_setup run locally as tmux does. There is no RPP declaration for where agents run; the handshake carries none. | Remote boxes. Out of scope for v1. |
| D5 | **Sleep tier** | Aim for `full` sleep via `report-activity` plus the block's `program_status` (OSC 7501). Fall back to `timed_only` if idle detection proves unreliable. | Ship `timed_only` first, add `full` later. |
| D6 | **v1 capability breadth** | Core ops, metadata via sidecar, activity tracker (`report-activity`), attachment (`report-attachment`), and `exec` (`proc.exec`) as the driving path gc uses. Skip a dialog matcher in v1. | Implement everything up front. |

---

## 1. Goal and non-goals

**Goal.** A first-class Rex backend so Gas City can run agent fleets under
Rex: one tab per rig, agents tiled as panes within it, live agent state and
events, and no tmux in the loop.

**Non-goals (v1).**
- Replacing tmux as the default. tmux stays default and fallback.
- Multi-host / remote boxes beyond pointing `rex` at another Rex server.
- Stopping or reconfiguring the user's shared Rex server.

---

## 2. Why Rex maps well (grounded primitives)

Confirmed against a live `rex` server (`0.1.0-preview.19`, API revision 14).
Rex terminal blocks (`com.superlogical.terminal`) expose a rich method and
event surface over the control API — richer than what herdr offered, which
removes several of herdr's bridges:

| Need | Rex primitive | vs. herdr |
|---|---|---|
| Process tree | `block call process` → `{last_exit, child, foreground, ancestors, home}`, each with `pid/user/name/path/argv0/cwd` | herdr `pane process-info` — parity |
| Read output | `block call format {"format":"text","trim":true}` → full rendered content | herdr `pane read` — parity |
| Send text | `block call write {"data": "..."}` | herdr `pane.run` — parity |
| Send keys | `block.event` (input event), `rex send-key` | herdr `pane send-keys` — parity |
| Clear scrollback | `block call clear` | herdr had **no** clear op |
| Agent state | `program_status` method + `program_status_changed` / `program_status_removed` events | herdr native `agent_status` — parity |
| Exit status | `child_exited` event (`exit_code`), `process.last_exit` | herdr via process-info — parity |
| Activity | `process_changed.generation`, `session.view.revision` (v1 does not consume `rex events`; it polls) | herdr had **no** timestamp → tracker bridge |
| Attachment | `session.list_clients` (`session.view` has no `clients` field) | herdr best-effort → now first-class |
| Create / destroy | `session.create`, `session.new_window`, `block.close`, and `session.destroy` only for the empty canonical `gc:<city>` session (GCREX-31, `docs/REX-CONTRACT.md` §12) | herdr workspace/tab/pane — parity |

Terminal methods available: `blob_read, clear, clipboard_read,
download_begin, format, input_mode, list_dir, modes, process, program_status,
pwd, reset, resize, set_theme, size, title, upload_begin, write`.
Terminal events: `bell, child_exited, clipboard_written, desktop_notification,
process_changed, program_status_changed, program_status_removed,
progress_report, pwd_changed, selection_copied, size_changed, title_changed,
url_clicked`.

The terminal flavor accepts launch options: `shell`, `command` (argv array),
`cwd`, `login`, `interactive`. That is exactly what `Start` needs.

---

## 3. Architecture and file layout

**The provider is a standalone executable, not a Go package inside gc.** The
module root is this repo (`github.com/docbobo/rex-gascity`); there is no nested
`runtime-rex/` module.
Gas City's runtime-packs direction (`engdocs/design/runtime-provider-packs.md`)
is explicit: providers must update without a new `gc` release, which rules out
in-process Go and Go plugins — the contract is the wire protocol, not Go
types. The `cloudflare` provider is the reference: a nested Go module in
`gascity-packs` that depends on **nothing from gascity**, emits
`gc-runtime-cloudflare` speaking RPP v0, is installed by the pack, and is
validated by `gc runtime check`.

So `gc-runtime-rex` is built from a **Go module with no gascity imports**. It
implements RPP v0 (`executable <op> <args…>`, JSON on stdin/stdout, exit codes
`0`/`1`/`2`, plus the `protocol` handshake). gc reaches it through its `exec:`/
proxy provider, bound to the pack-declared name `rex`.

### Layout

```
go.mod                          # github.com/docbobo/rex-gascity; requires nothing from gascity
cmd/gc-runtime-rex/             # RPP v0 op dispatcher (GCREX-5 / Phase 4)             [done]
internal/rex/client.go          # Client interface: one method per Rex op (below)          [done]
internal/rex/cli/               #   the only implementation in v1: shells out to `rex`     [done]
internal/rex/faketest/          #   fake Client for provider-core tests                    [done]
internal/provider/sessionname.go # deterministic gc-name -> rex session/tab/pane labels    [done]
internal/provider/binding.go    # Binder: session-name -> {session,window,block} binding    [done]
internal/provider/provider.go   # Start/Stop/IsRunning/Peek/Nudge/SendKeys/Interrupt        [done]
internal/provider/keymap.go     # tmux key names -> Rex key names                           [done]
internal/provider/*_test.go     # unit + e2e tests against the fake Client                  [done]
internal/provider/e2e_test.go   # live opt-in tests gated by REX_E2E (lifecycle, input path) [done]
# rex.LaunchSpec (type + CLI conversion) already lives in internal/rex/client.go.
# No separate provider-side launch-policy module is planned.
# Landed in GCREX-4: activity.go, stores.go (file sidecar), meta.go, attach.go.
# Landed in GCREX-5: cmd/gc-runtime-rex, its RPP op tests, and the live REX_E2E test.
# pack/pack.toml declares [runtimes.rex]; testdata/city/ is the verification fixture.
```

The module must not import `github.com/gastownhall/gascity/...`: Go forbids
importing another module's `internal/` tree, and the packs design forbids the
dependency by policy. We define our own config/response types from the RPP
JSON.

### Encapsulate the Rex client

The provider core must never call the `rex` CLI directly. It depends on a
`rex.Client` interface — SDK-style, **one method per Rex operation** (not a
generic `run(args…)`):

```go
type Client interface {
    // Session/window/block topology
    CreateSession(ctx context.Context, label string) (SessionID, error)
    NewWindow(ctx context.Context, sessionID SessionID, label string, spec LaunchSpec) (WindowID, BlockID, error)
    NewSplit(ctx context.Context, sessionID SessionID, anchor BlockID, direction, side string, spec LaunchSpec) (BlockID, error)
    CloseBlock(ctx context.Context, sessionID SessionID, blockID BlockID) error
    SessionView(ctx context.Context, sessionID SessionID) (SessionView, error) // windows, layouts, blocks (no clients)
    ListClients(ctx context.Context, sessionID SessionID) ([]Client, error)     // attachment source
    ListSessions(ctx context.Context) ([]Session, error)
    // Terminal ops (block methods + events). Rex requires session_id next to
    // block_id on block.method and block.event.
    BlockProcess(ctx context.Context, sessionID SessionID, blockID BlockID) (ProcessInfo, error)
    BlockFormat(ctx context.Context, sessionID SessionID, blockID BlockID) (string, error)
    BlockWrite(ctx context.Context, sessionID SessionID, blockID BlockID, data string) error
    SendKeys(ctx context.Context, sessionID SessionID, blockID BlockID, keys ...string) error
    BlockEvent(ctx context.Context, sessionID SessionID, blockID BlockID, ev Event) error
    BlockClear(ctx context.Context, sessionID SessionID, blockID BlockID) error
    // Event stream (declared but unimplemented in v1; nothing consumes it)
    Events(ctx context.Context, sessionID SessionID) (<-chan Event, error)
}
```

The interface above is a sketch; `internal/rex/client.go` is authoritative.
Two implementations are envisioned: `rex/cli` (v1, shells out, implemented) and a future
`rex/wire` (direct unix-socket JSON API). Swapping them must not touch provider
logic. The CLI is the v1 choice deliberately: it is versioned and documented,
whereas Rex's socket protocol is not.

This is the mockable boundary per the TDD mocking guidance — the Rex server is
an external system, so mock `Client`, never internal collaborators.

### How gc binds it

No gc source change. The runtime pack declares the executable:

```toml
# pack.toml
[runtimes.rex]
command = "scripts/gc-runtime-rex"   # pack-relative, or a PATH name
protocol = 0
```

City composition registers `rex`; `[session] provider = "rex"` resolves to
gc's proxy provider bound to the executable. Distribution: packs must not
carry per-platform binaries, so the pack installs the module
(`go install <module>@<version>`) or ships a script — the
`runtime-cloudflare` precedent.

The executable answers the `protocol` handshake (`docs/REX-CONTRACT.md` §9):

```json
{"version": 0, "capabilities": ["report-attachment", "report-activity"]}
```

RPP v0 has no field or capability for where agents run. Rex runs agent
processes on gc's host as gc's user (D4), so gc stages the work dir, runs
`pre_start` before start and `session_setup` after, reads transcripts, and
scans the process table – the same behavior as the tmux and herdr providers –
without declaring anything. Add `proc.exec` (and later `proc.provision`) to the
list only when the dispatcher implements `exec` with the exit-code semantics
gc expects.

---

## 4. Layout and identity

### Mapping (D1)

Rex's hierarchy is **server → session → tab (window) → pane (block)**;
gc's is **city → rig → agent**. D1 is where to put the seam between them.
(Throughout this plan, "window" means a Rex tab and "block" means a Rex pane.)

Example gc city `acme`: rigs `Indigo` and `Saffron`; agents `dog-1`, `dog-2`
under `Indigo`, `dog-3` under `Saffron`, plus the town agent `mayor`.

#### Option A — Rex session = rig, tab = agent, pane = terminal **(alternative)**

```
Rex server (shared by all gc cities)
└─ session: gc:acme:Indigo          ← one Rex session per gc rig
   ├─ tab: dog-1
   │  └─ pane: terminal (agent CLI, rig cwd)
   └─ tab: dog-2
      └─ pane: terminal (agent CLI)
└─ session: gc:acme:Saffron
   └─ tab: dog-3
      └─ pane: terminal (agent CLI)
└─ session: gc:acme:__town
   └─ tab: mayor
      └─ pane: terminal (agent CLI)
```

Mirrors herdr's final layout (workspace per rig → session; tab per agent →
tab). Each agent gets its own switchable tab.

#### Option B — Rex session = city, tab = rig, pane = agent **(chosen)**

```
Rex server (shared by all gc cities)
└─ session: gc:acme                 ← one Rex session per gc city
   ├─ tab: Indigo                   ← one Rex tab per gc rig
   │  ├─ pane: dog-1   (tiled side by side in the tab)
   │  └─ pane: dog-2
   ├─ tab: Saffron
   │  └─ pane: dog-3
   └─ tab: __town
      └─ pane: mayor
```

This is herdr's earlier default (agents tiled as panes in a shared tab).
Fewest Rex sessions, but agents are not each their own tab.

#### Option C — Rex session = agent, tab = terminal

```
Rex server (shared by all gc cities)
└─ session: gc:acme:Indigo--dog-1   ← one Rex session per gc agent
   └─ tab: terminal
      └─ pane: terminal (agent CLI)
└─ session: gc:acme:Indigo--dog-2
   └─ tab: terminal
      └─ pane: terminal (agent CLI)
└─ session: gc:acme:__town--mayor
   └─ tab: terminal
      └─ pane: terminal (agent CLI)
```

Simplest 1:1 mapping (Rex session name = gc session name), no rig grouping.
The Rex sidebar fills up with one session per agent.

#### Comparison

| | Rex session | Rex tab | Rex pane | Rig grouping | Switchable per agent | Name→id resolution |
|---|---|---|---|---|---|---|
| A | rig (`gc:city:rig`) | agent | terminal | yes (session) | yes (own tab) | sidecar: name → session + tab + block |
| **B (chosen)** | city (`gc:city`) | rig | agent | yes (tab) | no (tiled in a rig tab) | sidecar: name → tab + block |
| C | agent | terminal | terminal | no | yes (own session) | trivial: session name = gc name |

**Chosen: B.** One Rex session per city is the tidiest in the Rex sidebar and
matches the way gc already groups work by rig — rigs become tabs, agents become
the panes inside them. Trade-off: agents share a tab (tiled) instead of each
having its own switchable tab (A), and attachment is coarser (one session holds
all rigs), so `IsAttached` must resolve to the active tab and its panes (§5). C is the
least code but loses grouping.

`sessionname.go` holds `type SessionName string`, which owns the
`<rig>--<agent>` grammar and the `__town` fallback and exposes `Rig()`,
`Agent()`, `IsTown()`, `TabLabel()` (the rig tab label) and `PaneLabel()` (the
agent pane label). A package-level `rexSessionLabel(city)` returns the city
session label `gc:<city>`.
Unlike herdr, Rex session/tab labels are cosmetic, so we do not need herdr's
aggressive name-mangling for validity — but the **pane/block id** is not
derivable from the name, so the persisted binding (the file sidecar, GCREX-4) is the reverse map.

### Binding (`binding.go`)

Rex ids are not guaranteed stable across server restarts (Rex persists
sessions and "jumps ahead" the view revision on restore), and labels are not
unique across windows. So key on ids. The sidecar stores them (GCREX-4): one
JSON file per gc session name under the per-city state directory —
`$GC_REX_STATE_DIR/<city>`, else `$XDG_STATE_HOME/rex-gascity/<city>`, else
`~/.local/state/rex-gascity/<city>` (the city component is percent-encoded by `safeName`) — with atomic (temp + rename) writes:

```
<stateDir>/bindings/<sha256(gcName)>.json
  { gc_name, rex_session_id, rex_session_label,
    window_id, block_id, launch_mode, created_at, ready_prompt }
```

The default `New` keeps everything in memory so
tests stay hermetic; `WithStateDir(dir)` opts into the file sidecar, which
GCREX-5's dispatcher must pass on every op — one process per op means in-memory
state would not survive.

With Option B, `rex_session_id` is the shared city session and `window_id` is
the rig tab — both the same for every agent in a city/rig; `block_id` is the
agent's pane and is the field that actually identifies the agent.

The store is a cache; `session.view` plus the label scan stays the source of
truth. Resolution order (as herdr's `resolveBinding`):

1. Read the stored binding.
2. Live-probe with `session.view`.
3. If confirmed gone, clear the binding and rescan by label. `Start` also
   closes a stale pane before relaunching, so re-Start cannot leak a duplicate.
4. Transport/observation error clears nothing — it is returned as is.

---

## 5. RPP op mapping

Each row is a provider behavior (the contract method name, e.g. `Start`,
`GetLastActivity`, `CopyTo`, `RunLive`) and the Rex calls it makes. These are
not all wire ops: the dispatcher maps them onto the pinned wire operations in
docs/REX-CONTRACT.md §9, which has no `CopyTo` or `RunLive` rows and adds
`protocol`, `exec`, `provision` and `watch-startup`.

| Provider behavior | Rex call(s) | Notes |
|---|---|---|
| `Start` | ensure the city session (`session.create {label: gc:city}`, tolerate "exists") → find-or-create the rig tab (`session.new_window` if this is the rig's first agent, else `session.new_split` beside an existing pane in that tab) with `{flavor: com.superlogical.terminal.shell, options: launchspec, label: agent}`, `focus=false` → record `window_id` (tab) + `block_ids[0]` | `launchspec` = `{command: cfg.Argv, cwd: cfg.WorkDir, login/interactive}`. `pre_start` on host before, `session_setup` after. Serialize same-rig Starts with a mutex (two agents can race to create the rig tab). |
| `Stop` | `block.close {block_id}` (idempotent; a pane already gone is `rex.ErrNotFound` and counts as success); leave the city session and other rigs alone | Never stop the server, and never destroy the shared city session out from under other rigs. Rex removes a tab when its last block closes (live-verified), so teardown is block-only and needs no window call. |
| `Interrupt` | `block.event` ctrl+c (or `rex send-key ctrl+c`) | Keystroke, not a signal — same as herdr. Hard kill = `block.close`. |
| `IsRunning` | sidecar binding + `block call process`; `last_exit` only | Running means the pane exists and `last_exit` is unset. `IsRunning` and the input ops do not inspect foreground/process names; only `ProcessAlive(names)` does. |
| `IsAttached` | `session.list_clients` on the city session (an app client must exist), then `session.view` | Attached only when the agent's rig tab is the session's active window and still contains the agent's pane. Tiled panes in the active tab are visible. An app client is listed on every session, so app presence alone is not attachment. `Start` launches with `focus=false` so a background rig does not steal focus. Unknown client kinds answer true (GCREX-22). |
| `Attach` | `session.focus_block` + `session.focus_window` (the rig tab), via `rex api call` / client action | Focus in the user's Rex app rather than stealing a tty. |
| `ProcessAlive` | `block call process`; strict tree matching of `foreground`/`ancestors` names/`argv0`/`path` against `processNames` | Empty `processNames` ⇒ true. Only this op inspects process names. |
| `Nudge` | read `program_status` → refuse `working`/`blocked` (typed errors, nothing typed) → `block call write {"data": "<text>"}` then submit (Enter via `block.event` / `send-key enter`) → poll for an idle→working transition or a newer `updated_at`, else a format-fingerprint change | Busy gating and submit confirmation were deferred from GCREX-4 and are implemented in GCREX-16 (`NudgeModeImmediate` / `GC_REX_NUDGE_IMMEDIATE` overrides the gate). An unconfirmed delivery is a report, not an error, so gc does not retype it. |
| `SendKeys` | `block.event` with key presses (translate tmux `C-c`/`Enter` → Rex `ctrl+c`/`enter`) | |
| `Peek` | `block call format {"format":"text","trim":true,"max_bytes":983040}`, follow `id` with `blob_read` until `eof` → take last `lines` | Full content; `lines<=0` ⇒ all. |
| `ListRunning` | `session.list` for `gc:` city sessions, then `session.view` per session to enumerate rig tabs and their agent panes; reconstruct gc names via the sidecar | Miss = orphan reaping risk; the sidecar is authoritative for our own sessions. |
| `GetLastActivity` | `program_status` records, else `block format` poll fingerprint (§6) | The stored fingerprint is tied to the pane's block id, so a replaced pane starts a fresh baseline. |
| `ClearScrollback` | `block call clear` | Real support (herdr lacked it). |
| `CopyTo` | host filesystem copy | Provider-agnostic; gc does it on its own host. |
| `RunLive` | re-issue `session_live` commands into the block (`write`) | Best-effort. |
| `SetMeta`/`GetMeta`/`RemoveMeta` | sidecar `meta` store | Rex has no PKV. |

### Optional capabilities

gc 1.5.1 defines exactly six capability strings; unknown strings are ignored
(`docs/REX-CONTRACT.md` §9).

| Capability | Status for this provider |
|---|---|
| `report-attachment` | Declared by the gc-runtime-rex handshake (GCREX-5). Backs `is-attached` (via `session.list_clients`). |
| `report-activity` | Declared by the gc-runtime-rex handshake (GCREX-5). Backs `get-last-activity` (§6). |
| `proc.exec` | Not declared in v1: the disabled `exec` op is left unimplemented (exit 2), so gc drives through the dedicated ops. Declare it once `exec` propagates the in-box command's exit code. |
| `proc.provision` | Not planned for v1. Requires `proc.exec`. |
| `proc.stream` | Reserved in gc; do not declare. |
| `tty.attach` | Reserved in gc; do not declare. |

The dedicated driving ops (`interrupt`, `nudge`, `peek`, `clear-scrollback`,
`send-keys`) are not conformance requirements; they are the fallback when
`exec` exits 2. `watch-startup` stays a dedicated op.

> **Only the six strings above exist in gc 1.5.1.** Capability names from
> earlier drafts of this plan are not in the pinned baseline (gc ignores them)
> and must not be declared; see `docs/REX-CONTRACT.md` §9 for the list. Idle
> state is reported through `report-activity` plus `program_status`.

Details:

- **Idle.** `GetLastActivity` prefers `program_status`; otherwise it
  fingerprints `block format` and reports when the fingerprint last changed.
  It does not parse a ready prompt; a ready-prompt scan is a future option.
  An indeterminate reading must report *indeterminate*, never a false
  busy/idle.
- **Relaunch.** Close the block, create a new block in the same rig tab with
  the new launch spec (the warm box is the city session + rig tab, which
  persist).
- **`exec`.** gc prefers `exec` for driving; implement it with the dispatcher
  and declare `proc.exec` once its exit code carries the command's exit.
- **Server lifecycle.** Not implemented (D3).

---

## 6. Activity and events

- `activity.go` (GCREX-4): `GetLastActivity` reads the pane's `program_status`
  records (OSC 7501) and returns the latest `updated_at`. The emitter is the
  pane program, not the provider: Pi implements OSC 7501 and gc-managed Pi
  panes report under Rex, so records are normally present. When Rex reports
  none the provider falls back to polling `block format` and fingerprints the
  output in the activity sidecar, which keeps an idle pane's timestamp stable
  across the one-process-per-op dispatcher. The call returns the source in
  force (`program_status` | `format_poll` | `none`); the provider does **not**
  emit OSC 7501 in v1, and the format-poll fallback is the documented v1 path
  (`provider.ActivityPolicy`). Declared to gc via the RPP `report-activity`
  handshake capability (GCREX-5).
- v1 does **not** consume `rex events`. `GetLastActivity` polls
  `program_status` and, when there are no records, `block format`.
  `rex.Client.Events` is unimplemented. A later version may use events as
  level-triggered hints to re-poll, never as authoritative transitions (Rex
  replays/omits events under load).
- **No event push to gc in v1.** An out-of-tree provider cannot implement
  `runtime.SessionEventProvider`: RPP's `proc.stream` is reserved/
  unimplemented, and gc's controller does not consume any provider's event
  stream today. The provider consumes no events either.
- **Startup dialogs:** v1 relies on reading the launch's block like tmux/herdr's
  matcher; a dedicated dialog capability is a follow-up.

---

## 7. Metadata, attachment, liveness details

- **Meta:** sidecar KV under the city runtime dir, keyed by gc session name,
  exposed over the RPP `set-meta`/`get-meta`/`remove-meta` ops. Must
  store/return `GC_READY_PROMPT_PREFIX` byte-for-byte. This is our design, not an RPP rule:
  `GC_READY_PROMPT_PREFIX` is a tmux-provider variable.
- **Attachment:** `session.list_clients` lists clients per session (`session.view` does not). With B every agent
  shares the city session, and an app client is listed on every session, so
  app presence alone is not attachment. The agent counts as attached only
  when its rig tab is the session's active window and still contains the
  agent's pane (tiled panes in the active tab are visible). `Start` launches
  with `focus=false`, so a background rig does not steal focus. Answer true
  when the client kind cannot be classified, since gc reads any is-attached
  error as not attached (GCREX-22; this governs whether a session is held
  awake).
- **Liveness honesty:** when the Rex server is unreachable, report
  `ErrRuntimeUnavailable` — never "no sessions exist". Destructive reconciler
  paths defer on it.

---

## 8. Configuration and prerequisites

Prerequisites: the `rex` binary on PATH and a reachable Rex server.

```bash
# one-off trial
export GC_SESSION=rex
gc start <scratch-city>
```

```toml
# city.toml — city-wide
[session]
provider = "rex"
```

```toml
# pack.toml — the chosen out-of-tree delivery (D0 option 1)
[runtimes.rex]
command = "scripts/gc-runtime-rex"   # pack-relative, or a PATH name
protocol = 0
```

The pack **must** declare `[runtimes.rex]`; the executable speaks RPP v0 on
stdin/stdout and is installed by the pack (a `go install` step, since packs
must not carry platform binaries). The local Nix role at
`~/.config/nixpkgs/roles/gascity/default.nix` would need `rex` added if this
ships.

`gc config show` shows the effective provider. `gc reload` picks up a
selector change; running sessions keep their backend until they restart.

---

## 9. Seams and testing (TDD)

The provider is an executable, so the **primary seam is the RPP v0 op
contract**, not a Go interface. gc's own docs point `gc runtime check` /
`gc runtime conformance` at an arbitrary executable; that is the acceptance
seam. Proposed seams (confirm before writing any test):

| # | Seam | Catches | Misses |
|---|---|---|---|
| S1 | **Provider core vs a fake `rex.Client`** — transport-agnostic behavior | op semantics, liveness classification, idempotency, sidecar binding, error mapping | real Rex, CLI argv/JSON, RPP encoding |
| S2 | **CLI adapter (`rex/cli`) vs a fake `rex` binary on PATH** | argv construction, JSON parsing, exit-code handling, key-name translation | provider core, real Rex |
| S3 | **RPP v0 op contract** — run `gc-runtime-rex <op> <name>`; assert stdout/exit codes plus the `protocol` handshake | wire encoding, exit codes (0/1/2), capability declaration, stdin config | real Rex |
| S4 | **`gc runtime check rex` / `gc runtime conformance rex`** (acceptance) | contract completeness the provider cannot self-certify | anything the suite omits |
| S5 | **Live Rex journey** (opt-in, not CI) | real `format`/`process`/`program_status`, persist/restore races, version drift | determinism |

Recommended day-to-day seam: **S1** (provider core, with a fake `rex.Client`),
then **S2** for the CLI adapter; **S3–S5** are gates. Work in vertical slices
(start → peek → process-alive → not-running → stop → nudge → interrupt), one
test per cycle, and no test at an unconfirmed seam.

Also:
- **Fakes.** A fake `rex.Client` for core tests; a fake `rex` binary on PATH
  for adapter tests (herdr's `fakeherdr_test.go`). The default suite must not
  need a real Rex.
- **Crash/restart.** Restart the Rex server mid-run; bindings re-resolve and
  no orphan blocks leak (herdr's storm lesson).

---

## 10. Phased delivery (review gates)

Each phase ends at a reviewable checkpoint.

**Phase 0 — Pin the Rex contract. (Done.)** Delivered: `docs/REX-CONTRACT.md`
(Rex CLI/API surface, RPP v0 baseline for gc 1.5.1), its §10 re-verification
probe recipe, and the D0–D6 confirmations in §11.
*Gate: contract doc and probe reviewed.*

**Phase 1 — Client + layout + Start/Stop/IsRunning/Peek. (Implemented.)**
`rex.Client`, the CLI implementation, the fake, and the provider core create
the city session and rig tab, start an agent pane, read output, stop, and
detect liveness. The RPP dispatcher is deferred to Phase 4 (GCREX-5).
*Gate: manual smoke test in a scratch city on a pinned Rex.*

**Phase 2 — Input path. (Implemented; gating and confirmation added by GCREX-16.)**
Nudge now reads `program_status` before typing (refusing a working or blocked
pane without typing), writes the text, sends the submit keystroke, and confirms
a turn start within a bounded window (program_status, else a format fingerprint).
`NudgeModeImmediate` (RPP: `GC_REX_NUDGE_IMMEDIATE=1`) bypasses the gate.
SendKeys delivers arbitrary translated keys; Interrupt sends ctrl+c and
reports `ErrSessionNotFound` for a dead pane. *Gate: drive a real agent through
a turn and an interrupt.*

**Phase 3 — Identity, meta, activity, attachment. (Implemented.)** The file
sidecar (bindings, activity fingerprints, metadata) under the per-city state
dir, `WithStateDir` wiring, idempotent `Start`, block-only teardown, attachment
honesty (`session.list_clients` filtered to app clients plus the active
rig tab containing the agent's pane), a strict `ProcessAlive(names)` op, and `GetLastActivity`
(program_status preferred, `block format` fingerprint fallback; see §6). No gc event push.
*Gate: `TestE2EIdentityActivityAttachment` (live) verifies that identity
persists across provider instances, a stale id re-resolves by label, activity
is reported with a source that advances on output, attachment is per active
rig tab, and teardown leaves the shared session.*

**Phase 4 — Delivery + config (GCREX-5). (Implemented.)** The
`cmd/gc-runtime-rex` RPP v0 dispatcher, the `protocol` handshake
(`report-attachment`, `report-activity`), the pack `[runtimes.rex]` declaration
(`pack/pack.toml`), the Makefile/install path, and `docs/rex-provider.md`.
Both `gc runtime check rex` and `gc runtime conformance rex` pass against the
pack (§9 S3/S4). `exec`, `provision`, `attach` and `watch-startup` stay
unimplemented (exit 2), so `proc.exec`/`proc.provision` are not declared.
Nix/pack packaging beyond `go install` is still open.
*Gate: `gc runtime check rex` and `gc runtime conformance rex` green; docs reviewed.*

**Phase 5 — Pilot.** `GC_SESSION=rex` on a scratch city with the mayor
pinned to tmux; watch liveness/wedge detection over several work cycles;
then promote to `[session] provider = "rex"`. *Gate: errors reviewed and
acceptable.*

**Done (GCREX-6).** Pilot ran on scratch cities (worker on rex, mayor on tmux)
with three kill/restart cycles and no wedge. Errors reviewed and acceptable:
the rig-split and vanished-pane defects were fixed; the `gc:default` namespace
under a supervisor stays open; server-restart recovery was not exercised live
(D3). Promoted as an opt-in backend; tmux stays the default. Record:
`docs/rex-pilot-phase5.md`.

---

## 11. Risks and open questions

- **RPP capability naming (resolved).** Earlier vocabulary is replaced by the
  six gc 1.5.1 strings (§5).
- **Preview churn.** Rex is `0.1.0-preview.N`; the CLI/API surface can move.
  Pin a version, keep live tests opt-in, re-run them on every bump (the
  herdr 0.7.4/0.7.5 storm is the cautionary tale).
- **Rig-tab growth (resolved, GCREX-6).** Each new pane splits the
  largest-area pane along its longer axis, so panes stay balanced; a live
  probe reached 45 panes with no layout limit, so no cap was added. See
  `docs/rex-pilot-phase5.md`.
- **Idle detection accuracy** (status: v1 uses `program_status` or the format
  fingerprint; not changed by the pilot). The `full` sleep tier hinges on a reliable
  reliable activity signal; today it is `program_status` or a format
  fingerprint, and a ready-prompt scan is a future option. Validate against real agent TUIs; default to
  `timed_only` if flaky.
- **`ListRunning` completeness.** Under-reporting risks reaping live agents.
  The file sidecar has landed (GCREX-4, `WithStateDir`); it must
  be authoritative for our own sessions.
- **Attachment semantics.** With Option B every agent shares one city session,
  so a client viewing *any* rig reads every agent as attached. Resolve to
  the active tab containing the agent's pane, and when it cannot be refined, report
  honestly (non-zero) rather than always-true.
- **Program status portability.** `program_status` classification may vary
  by agent CLI. Verify it classifies the harnesses we care about, and treat an
  empty or missing status (including a report that classifies nothing) as
  indeterminate. REX-CONTRACT lists `idle|working|done|blocked|error`. The
  emitter is the pane program, not the provider: Pi implements OSC 7501 and
  gc-managed Pi panes report under Rex, so `program_status` is normally present.
  **Decided (GCREX-4): v1 does not emit OSC 7501.** `GetLastActivity` prefers
  `program_status` when it is present and otherwise polls `format`, returning
  the source in force; an empty or missing status is never reported as a false
  idle/busy.
- **Liveness is `last_exit`, deliberately (GCREX-4).** A live probe showed Rex runs a launched command as the default shell with `argv0` set to the command (`/bin/sh` arrives as `name: bash`, `path: /bin/bash`, `argv0: /bin/sh`), so argv0/process-name heuristics misclassify live panes and made a live input test fail as "agent exited". v1 therefore treats `last_exit` as the liveness truth for `IsRunning` and the input ops, and exposes `ProcessAlive(names)` (strict foreground/ancestor matching, empty => alive) for the RPP `process-alive` op where gc supplies authoritative names.
- **Shared server.** Hard rule: never `server.stop`. Provider teardown closes
  city-owned blocks (`block.close`) and leaves the city session even when empty
  (GCREX-65, `docs/REX-CONTRACT.md` §12). This is the provider-level guarantee
  only: in the pilot, `gc stop` removed the city's Rex session through a path we
  did not identify, so treat `gc stop`'s session behaviour as unresolved until
  that path is found and tested.

---

## 12. Out of scope (v1)

- Remote/multi-host boxes beyond `-S <host>` to a Rex server.
- A `DialogProvider` matcher for startup dialogs.
- `ExecProvider` one-shot command execution.
- Replacing tmux as the default backend.
- A direct unix-socket transport — deferred, and enabled by the
  `rex.Client` interface (§3).
