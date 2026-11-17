# Plan: native Rex session provider for Gas City

Status: **DRAFT** — D0 decided; D1–D6 open. No code yet.
Target: an opt-in Rex session backend for Gas City — a **standalone Go module**
(`gc-runtime-rex`) that speaks RPP v0, shipped by a runtime pack and selectable
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

## 0. Decisions to confirm before implementation

### D0 — Delivery form: does this have to be compiled into GC? — **DECIDED: option 1**

**No.** There are three ways to ship a provider, and only one recompiles gc:

| Option | How it is selected | Compiles into gc? | Gets today |
|---|---|---|---|
| **1. Pack-declared runtime — CHOSEN** | `[runtimes.rex]` in `pack.toml` → `command = "gc-runtime-rex"`, `protocol = 0`; then `[session] provider = "rex"` | **No.** A pack can ship/install any executable that speaks Runtime Provider Protocol v0; city composition registers the name like a builtin. | The full RPP v0 op set + optional capabilities (`carrier.ops`, `observe.v1`, `report-attachment`, `report-activity`, `server-lifecycle`, `fs.host`, `proc.provision`, `input-outcome`). `gc runtime check rex` / `gc runtime conformance rex` validate it. |
| **2. In-tree Go provider — not chosen** | builtin name `rex` via `registry.Register("rex", …)` | **Yes** — edits `cmd/gc/runtime_registry.go` and ships in the gc binary. | Direct Go `runtime.Provider` and the Go-only capability interfaces (e.g. `runtime.SessionEventProvider` for push events). |
| **3. Raw exec script** | `GC_SESSION=exec:/path/gc-session-rex` | No | Same RPP surface as (1), but the selector is `exec:<path>`, not a clean `rex` name. This is "Path A", already set aside. |

Why (1) is sufficient for a *native* provider: RPP v0 covers the whole
`Provider` contract as wire ops — `start`, `stop`, `interrupt`, `send-keys`,
`clear-scrollback`, `is-running`, `attach`, `process-alive`, `nudge`,
`set-meta`/`get-meta`/`remove-meta`, `peek`, `list-running`,
`get-last-activity`, `is-attached`, `exec`, `provision`, `observe`,
`inventory`, `kill-object`, `server-stop`, plus the `protocol` handshake.
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

These are the remaining choices that shape everything else. D0 is settled;
D1–D6 assume the recommended option unless we change them.

### D1–D6

| # | Decision | Recommended | Alternative |
|---|---|---|---|
| D1 | **Layout mapping** (diagrams in §4) | **Option B — CHOSEN.** Rex **session = city** (`gc:<city>`), **tab = rig**, **pane = agent** (agents tiled in the rig's tab). One Rex session per city; fewest sessions. | **Option A** — session = rig, tab = agent (each agent its own tab). **Option C** — session = agent (simplest, no grouping). |
| D2 | **Transport** | Shell out to the `rex` CLI **now**, but only through a stable `rex.Client` interface; a direct unix-socket (JSON wire) implementation can be dropped in later without touching provider logic. | Go straight to the socket — Rex's CLI is versioned and documented; the socket protocol is not. |
| D3 | **Server ownership** | Provider does **not** implement `ServerLifecycleProvider`. Never stop the shared Rex server — the user's own work runs on it. Scope teardown to the city's own windows/sessions by label prefix. | Own a per-city server — impossible; Rex has one shared server, not one per city. |
| D4 | **Scope** | Local Rex server only (default socket, or `-S <host>` for a remote Rex that shares the filesystem). Agents run **on the host** (`fs.host` semantics), so pre_start/session_setup run locally as tmux does. | Remote boxes. Out of scope for v1. |
| D5 | **Sleep tier** | Aim for `full` sleep via `IdleSnapshotProvider` + `report-activity`-equivalent (Rex has real `program_status`). Fall back to `timed_only` if idle detection proves unreliable. | Ship `timed_only` first, add `full` later. |
| D6 | **v1 capability breadth** | Core `Provider` + `IdleWaitProvider`, `IdleSnapshotProvider`, `RelaunchProvider`, metadata via sidecar, activity tracker, attachment. Skip `ExecProvider` (optional) and `DialogProvider` in v1. | Implement everything up front. |

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
| Activity | `process_changed.generation`, `session.view.revision`, `rex events --json` | herdr had **no** timestamp → tracker bridge |
| Attachment | `session.view` clients list, `client.list` | herdr best-effort → now first-class |
| Create / destroy | `session.create`, `session.new_window`, `block.close`, `session.destroy` | herdr workspace/tab/pane — parity |

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

**The provider is a standalone executable, not a Go package inside gc.**
Gas City's runtime-packs direction (`engdocs/design/runtime-provider-packs.md`)
is explicit: providers must update without a new `gc` release, which rules out
in-process Go and Go plugins — the contract is the wire protocol, not Go
types. The `cloudflare` provider is the reference: a nested Go module in
`gascity-packs` that depends on **nothing from gascity**, emits
`gc-runtime-cloudflare` speaking RPP v0, is installed by the pack, and is
validated by `gc runtime check`.

So `gc-runtime-rex` is a **standalone Go module with no gascity imports**. It
implements RPP v0 (`executable <op> <args…>`, JSON on stdin/stdout, exit codes
`0`/`1`/`2`, plus the `protocol` handshake). gc reaches it through its `exec:`/
proxy provider, bound to the pack-declared name `rex`.

### Layout

```
runtime-rex/                    # nested Go module; go.mod requires nothing from gascity
  cmd/gc-runtime-rex/           # main: RPP v0 op dispatcher (stdin/stdout/exit codes)
  internal/rex/                 # Client interface: SDK-style, one method per Rex op (below)
  internal/rex/cli/             #   the only implementation in v1: shells out to `rex`
  internal/rex/faketest/        #   fake Client for provider-core tests
  internal/provider/            # core logic behind our own interface (not gc's runtime.Provider)
  internal/provider/binding.go     # session-name -> {session,window,block} sidecar binding + probe
  internal/provider/sessionname.go # deterministic gc-name -> rex session/tab/pane labels
  internal/provider/launchspec.go  # start config -> terminal block options (command, cwd, shell, login)
  internal/provider/activity.go    # event-driven + polled activity tracker
  internal/provider/meta.go        # sidecar KV (set-meta/get-meta/remove-meta)
  internal/rex/cli/fake_test.go    # fake `rex` binary on PATH, drives the CLI adapter tests
  internal/provider/live_test.go   # opt-in live test (env gate)
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
    CloseBlock(ctx context.Context, blockID BlockID) error
    SessionView(ctx context.Context, sessionID SessionID) (SessionView, error) // windows, layouts, blocks, clients
    ListSessions(ctx context.Context) ([]Session, error)
    // Terminal ops (block methods + events)
    BlockProcess(ctx context.Context, blockID BlockID) (ProcessInfo, error)
    BlockFormat(ctx context.Context, blockID BlockID) (string, error)
    BlockWrite(ctx context.Context, blockID BlockID, data string) error
    BlockEvent(ctx context.Context, blockID BlockID, ev Event) error
    BlockClear(ctx context.Context, blockID BlockID) error
    // Event stream
    Events(ctx context.Context, sessionID SessionID) (<-chan Event, error)
}
```

Two implementations are envisioned: `rex/cli` (v1, shells out) and a future
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

The executable claims host semantics in its `protocol` handshake:

```json
{"version": 0, "capabilities": ["fs.host", "report-attachment", "report-activity", "carrier.ops"], "locality": "local"}
```

`fs.host` + `"locality": "local"` is the right claim: Rex runs agent processes
on the host filesystem as gc's user, so gc stages the work dir, runs
`pre_start` before start and `session_setup` after, reads transcripts, and
scans the process table — the same behavior as the tmux and herdr providers.

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
all rigs), so `IsAttached` must resolve to the active tab/pane (§5). C is the
least code but loses grouping.

`sessionname.go` holds `rexSessionFor(city)` (the city session `gc:<city>`),
`tabLabelFor(rig)` (the rig tab), and `paneLabelFor(gcName)` (the agent pane).
Unlike herdr, Rex session/tab labels are cosmetic, so we do not need herdr's
aggressive name-mangling for validity — but the **pane/block id** is not
derivable from the name, so the sidecar is the reverse map.

### Binding (`binding.go`)

Rex ids are stable is not guaranteed across server restarts (Rex persists
sessions and "jumps ahead" the view revision on restore), and labels are not
unique across windows. So key on ids, stored in a sidecar:

```
<stateDir>/bindings/<sha256(gcName)>.json
  { gc_name, rex_session_id, rex_session_label,
    window_id, block_id, launch_mode, created_at, ready_prompt }
```

With Option B, `rex_session_id` is the shared city session and `window_id` is
the rig tab — both the same for every agent in a city/rig; `block_id` is the
agent's pane and is the field that actually identifies the agent.

Resolution order (as herdr's `resolveBinding`):

1. Read the sidecar.
2. Live-probe with `block inspect` / `session.view`.
3. If confirmed gone, clear the binding (ids recycle) and maybe re-create.
4. Transport/observation error clears nothing — report `ErrRuntimeUnavailable`.

---

## 5. RPP op mapping

Each row is an **RPP v0 op** (the wire name gc uses) and the Rex calls it
makes. The op names match gc's provider semantics.

| RPP op | Rex call(s) | Notes |
|---|---|---|
| `Start` | ensure the city session (`session.create {label: gc:city}`, tolerate "exists") → find-or-create the rig tab (`session.new_window` if this is the rig's first agent, else `session.new_split` beside an existing pane in that tab) with `{flavor: com.superlogical.terminal.shell, options: launchspec, label: agent}`, `focus=false` → record `window_id` (tab) + `block_ids[0]` | `launchspec` = `{command: cfg.Argv, cwd: cfg.WorkDir, login/interactive}`. `pre_start` on host before, `session_setup` after (`fs.host`). Serialize same-rig Starts with a mutex (two agents can race to create the rig tab). |
| `Stop` | `block.close {block_id}` (idempotent); if it was the rig tab's last pane, close the tab; leave the city session and other rigs alone | Never stop the server, and never destroy the shared city session out from under other rigs. `StopForCleanup` classifies "gone". |
| `Interrupt` | `block.event` ctrl+c (or `rex send-key ctrl+c`) | Keystroke, not a signal — same as herdr. Hard kill = `block.close`. |
| `IsRunning` | sidecar + `block inspect` live probe; use `process` `last_exit` + foreground to detect agent exit | A terminal may outlive its program; running means agent process present, not merely block present. |
| `IsAttached` | `session.view`: the city session lists clients; refine to the agent via the active tab / `focused_block_id` | Coarse by default: with B every agent shares the city session, so session-level attachment over-reports. Prefer the focused tab/pane; when it cannot be refined, report honestly (non-zero) rather than always-true. |
| `Attach` | `session.focus_block` + `session.focus_window` (the rig tab), via `rex api call` / client action | Focus in the user's Rex app rather than stealing a tty. |
| `ProcessAlive` | `block call process`; match `foreground`/`ancestors` names/`argv0`/`path` against `processNames` | Empty `processNames` ⇒ true. |
| `Nudge` | `block call write {"data": "<text>"}` then submit (Enter via `block.event` / `send-key enter`); confirm via `program_status_changed` | Two steps. Handle busy: type nothing if a turn is running unless `ImmediateNudge`; confirm the submit. |
| `SendKeys` | `block.event` with key presses (translate tmux `C-c`/`Enter` → Rex `ctrl+c`/`enter`) | |
| `Peek` | `block call format {"format":"text","trim":true}` → take last `lines` | Full content available; `lines<=0` ⇒ all. |
| `ListRunning` | `session.list` for `gc:` city sessions, then `session.view` per session to enumerate rig tabs and their agent panes; reconstruct gc names via the sidecar | Miss = orphan reaping risk; the sidecar is authoritative for our own sessions. |
| `GetLastActivity` | activity tracker (below) | |
| `ClearScrollback` | `block call clear` | Real support (herdr lacked it). |
| `CopyTo` | host filesystem copy (fs.host) | Provider-agnostic. |
| `RunLive` | re-issue `session_live` commands into the block (`write`) | Best-effort. |
| `SetMeta`/`GetMeta`/`RemoveMeta` | sidecar `meta` store | Rex has no PKV. |

### Optional capabilities

Expressed as Go interfaces in-tree, or as RPP `protocol` capabilities
out-of-tree. Behaviour is the same; only the surface differs. **D0 is
out-of-tree, so the RPP column is what we implement**; the in-tree column is
kept for reference.

| Behaviour | In-tree interface (D0 option 2) | Out-of-tree RPP capability (D0 option 1) |
|---|---|---|
| Wait for a safe idle boundary | `IdleWaitProvider.WaitForIdle` | `idle-boundary` + `carrier.ops`, or `proc.exec` |
| One-shot idle probe | `IdleSnapshotProvider.SnapshotIdle` | `carrier.ops` `peek` |
| Relaunch in a warm box | `RelaunchProvider.Relaunch` | `proc.provision` + `proc.exec` |
| Report attachment honestly | `AttachmentObserverWithError` | `report-attachment` |
| Report activity | `GetLastActivity` | `report-activity` |
| Reconciler-v2 reads | `InventoryProvider` | `observe.v1` |
| Server teardown | `ServerLifecycleProvider` | `server-lifecycle` — **not declared** (D3) |

Details:

- **Idle.** Poll `block call format` (or `program_status`) for the ready prompt
  (`ready_prompt_prefix`, default `❯ `) with no busy indicator, two
  consecutive reads 200 ms apart. Prefer `program_status` when it classifies
  the agent. An indeterminate reading must report *indeterminate*, never a
  false busy/idle.
- **Relaunch.** Close the block, create a new block in the same rig tab with
  the new launch spec (the warm box is the city session + rig tab, which
  persist).
- **`ExecProvider`.** Optional; skip in v1 (herdr did). `proc.exec` in RPP is
  about the `exec` op's exit-code semantics, not a requirement to implement it.
- **Server lifecycle.** Not implemented/declared (D3).

---

## 6. Activity and events

- `activity.go`: a tracker seeded from `process_changed` generations and
  `program_status_changed`, falling back to polling `format` /
  `session.view.revision`. Declared to gc via the RPP `report-activity`
  handshake capability (the wire equivalent of `CanReportActivity = true`);
  the claim backstop stays active.
- `rex events --json` is consumed **inside the provider** by the activity
  tracker, as **level-triggered hints to re-poll**, never authoritative
  transitions (Rex replays/omits events under load).
- **No event push to gc in v1.** An out-of-tree provider cannot implement
  `runtime.SessionEventProvider`: RPP's `proc.stream` is reserved/
  unimplemented, and gc's controller does not consume any provider's event
  stream today. Events stay internal.
- **Startup dialogs:** v1 relies on reading the launch's block like tmux/herdr's
  matcher; a dedicated dialog capability is a follow-up.

---

## 7. Metadata, attachment, liveness details

- **Meta:** sidecar KV under the city runtime dir, keyed by gc session name,
  exposed over the RPP `set-meta`/`get-meta`/`remove-meta` ops. Must
  store/return `GC_READY_PROMPT_PREFIX` byte-for-byte if we declare
  `idle-boundary` (gc requires this for a `carrier.ops` carrier).
- **Attachment:** `session.view` lists clients per session. With B every agent
  shares the city session, so refine to the agent's rig tab /
  `focused_block_id`; a client attached to that tab counts as attached to the
  agent. Be honest: exit non-zero when attachment cannot be determined (this
  governs whether a session is held awake).
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
| S3 | **RPP v0 op contract** — run `gc-runtime-rex <op> <name>`; assert stdout/exit codes plus the `protocol` handshake | wire encoding, exit codes (0/1/2/66/75/80), capability declaration, stdin config | real Rex |
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

**Phase 0 — Pin the Rex contract.** Capture the exact `rex` CLI/API surface
against a pinned Rex version (session.create/new_window layout schema, block
method I/O, event schemas, key syntax), and the RPP v0 op/handshake shape for
the chosen out-of-tree form. Write it to `REX-CONTRACT.md`. Confirm D1–D6.
*Gate: contract doc reviewed.*

**Phase 1 — Client + layout + Start/Stop/IsRunning/Peek.** First usable
provider: create the city session and rig tab, start an agent pane, read
output, stop, detect liveness, and wire the RPP op dispatcher so `gc runtime
check` can drive the executable. Keep it out of any live city until the check
passes. *Gate: manual smoke test in a scratch city on a pinned Rex.*

**Phase 2 — Input path.** Nudge/SendKeys/Interrupt with submit confirmation,
busy handling, and the interrupt keystroke. *Gate: drive a real agent through
a turn and an interrupt.*

**Phase 3 — Identity, meta, activity, attachment.** Sidecar KV, activity
tracker, attachment honesty (no gc event push — see §6). *Gate: idle
detection and orphan sweep behave on a scratch city.*

**Phase 4 — Delivery + config.** Out-of-tree: the pack `[runtimes.rex]`
declaration, the `gc-runtime-rex` build, and the `protocol` handshake. In-tree:
`registry.Register("rex", …)`. Either way: docs, `gc config show`,
`gc runtime check rex` / `gc runtime conformance rex`, Nix/pack packaging.
*Gate: `gc runtime conformance rex` green; docs reviewed.*

**Phase 5 — Pilot.** `GC_SESSION=rex` on a scratch city with the mayor
pinned to tmux; watch liveness/wedge detection over several work cycles;
then promote to `[session] provider = "rex"`. *Gate: errors reviewed and
acceptable.*

---

## 11. Risks and open questions

- **Preview churn.** Rex is `0.1.0-preview.N`; the CLI/API surface can move.
  Pin a version, keep live tests opt-in, re-run them on every bump (the
  herdr 0.7.4/0.7.5 storm is the cautionary tale).
- **Idle detection accuracy.** The `full` sleep tier hinges on a reliable
  ready-prompt read. Validate against real agent TUIs; default to
  `timed_only` if flaky.
- **`ListRunning` completeness.** Under-reporting risks reaping live agents.
  The sidecar must be authoritative for our own sessions.
- **Attachment semantics.** With Option B every agent shares one city session,
  so a client viewing *any* rig reads every agent as attached. Resolve to the
  active tab / `focused_block_id`, and when it cannot be refined, report
  honestly (non-zero) rather than always-true.
- **Program status portability.** `program_status` classification may vary
  by agent CLI; verify it classifies the harnesses we care about, and fall
  back to a capture-scan when it reports `unknown`.
- **Shared server.** Hard rule: never `server.stop`. Confirm teardown only
  closes city-owned windows/sessions.

---

## 12. Out of scope (v1)

- Remote/multi-host boxes beyond `-S <host>` to a Rex server.
- A `DialogProvider` matcher for startup dialogs.
- `ExecProvider` one-shot command execution.
- Replacing tmux as the default backend.
- A direct unix-socket transport — deferred, and enabled by the
  `rex.Client` interface (§3).
