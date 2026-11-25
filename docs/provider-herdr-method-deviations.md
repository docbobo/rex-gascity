# Provider method deviations

Compiled audit of every `runtime-herdr-core` provider op against the current
`gc-runtime-rex` implementation. The contract under audit is
[`docs/provider-herdr-method-io.md`](provider-herdr-method-io.md); the per-op
work is kept in [`docs/provider-method-audit/`](provider-method-audit/), one
report per op. Read this document for the summary and the deviations; read the
per-op report for the full citation trail and the matches.

Method: one read-only subagent per op (24 ops), each re-reading the method's
section, the shared RPP contract, and the implementing Go: the op table and
handlers in `cmd/gc-runtime-rex/main.go`, the provider methods in
`internal/provider/*.go`, and the Rex calls in `internal/rex/**`. Every
implementation claim cites a file and line; the parent then re-checked every
citation mechanically against the repo, the pinned pack
(`/tmp/gascity-packs-0f07496`) and gc v1.5.1 (`/tmp/gascity-v1.5.1`): 371
resolvable citations, 0 missing files, 0 out-of-range lines. "gc cares" means
gc v1.5.1 actually exercises the difference; many deviations are inherited
choices that gc v1.5.1 never triggers.

Verdict meanings: `match` (no deviation found), `deviations` (implemented, with
differences), `not implemented` (op absent from both the `ops` table and the
`unimplemented` set, so it exits 2). A deliberate choice to follow gc v1.5.1
instead of the pack is recorded in the op's report, not silently dropped.


## Verdicts at a glance

| op | verdict | deviations | medium | low | info |
|---|---|---|---|---|---|
| `protocol` | [deviations](provider-method-audit/protocol.md) | 4 | 0 | 4 | 0 |
| `start` | [deviations](provider-method-audit/start.md) | 10 | 4 | 6 | 0 |
| `stop` | [deviations](provider-method-audit/stop.md) | 6 | 0 | 6 | 0 |
| `kill-object` | [not implemented](provider-method-audit/kill-object.md) | 4 | 0 | 4 | 0 |
| `observe` | [not implemented](provider-method-audit/observe.md) | 5 | 0 | 3 | 0 |
| `inventory` | [not implemented](provider-method-audit/inventory.md) | 4 | 0 | 4 | 0 |
| `is-running` | [deviations](provider-method-audit/is-running.md) | 5 | 1 | 4 | 0 |
| `process-alive` | [deviations](provider-method-audit/process-alive.md) | 5 | 1 | 3 | 0 |
| `list-running` | [deviations](provider-method-audit/list-running.md) | 7 | 1 | 4 | 2 |
| `is-attached` | [deviations](provider-method-audit/is-attached.md) | 4 | 0 | 4 | 0 |
| `get-last-activity` | [deviations](provider-method-audit/get-last-activity.md) | 5 | 0 | 5 | 0 |
| `peek` | [deviations](provider-method-audit/peek.md) | 6 | 1 | 5 | 0 |
| `get-meta` | [deviations](provider-method-audit/get-meta.md) | 5 | 0 | 5 | 0 |
| `set-meta` | [deviations](provider-method-audit/set-meta.md) | 5 | 1 | 4 | 0 |
| `remove-meta` | [deviations](provider-method-audit/remove-meta.md) | 7 | 1 | 6 | 0 |
| `nudge` | [deviations](provider-method-audit/nudge.md) | 7 | 1 | 5 | 1 |
| `send-keys` | [deviations](provider-method-audit/send-keys.md) | 8 | 2 | 6 | 0 |
| `interrupt` | [deviations](provider-method-audit/interrupt.md) | 7 | 2 | 5 | 0 |
| `clear-scrollback` | [deviations](provider-method-audit/clear-scrollback.md) | 8 | 1 | 6 | 1 |
| `watch-startup` | [not implemented](provider-method-audit/watch-startup.md) | 5 | 0 | 3 | 0 |
| `attach` | [deviations](provider-method-audit/attach.md) | 5 | 0 | 2 | 3 |
| `copy-to` | [not implemented](provider-method-audit/copy-to.md) | 3 | 0 | 1 | 0 |
| `server-stop` | [not implemented](provider-method-audit/server-stop.md) | 5 | 0 | 5 | 0 |
| `x-kick-core` | [not implemented](provider-method-audit/x-kick-core.md) | 5 | 0 | 5 | 0 |
| **total** | | **136** | **17** | **105** | **7** |

Seven ops are not implemented at all: `kill-object`, `observe`, `inventory`,
`watch-startup`, `copy-to`, `server-stop` and `x-kick-core`. Four of
those (`kill-object`, `observe`, `inventory`, `x-kick-core`) are absent from the
`unimplemented` set too, so they exit 2 with `unknown operation`; the other three
(`watch-startup`, `copy-to`, `server-stop`) are listed in `unimplemented` and
exit 2 with `operation ... not implemented`. gc v1.5.1 never calls `observe`,
`inventory`, or `copy-to`; it does call `watch-startup` (falls back to `peek`
polling), `kill-object`, `x-kick-core`, and `server-stop`, which is why those
exit-2 gaps carry more weight than the rest.

No deviation was rated `high`. Nothing here has been changed: this audit is
read-only, and no parity verdict was altered.

## Per-op deviations

Each table is verbatim from the op's report; see that report for matches, full prose and unverified items.

### `protocol` — deviations

| Doc claim | Current implementation | Behavioral impact | Severity |
|---|---|---|---|
| `## protocol` § Success output: declares 10 capabilities (`observe.v1`, `report-attachment`, `report-activity`, `city-scoped`, `conformance-faults`, `server-lifecycle`, `carrier.ops`, `idle-boundary`, `input-outcome`, `fs.host`). | `cmd/gc-runtime-rex/main.go:58` hardcodes 7 capabilities: `["report-attachment","report-activity","idle-boundary","carrier.ops","input-outcome","observe.v1","city-scoped"]`. | gc 1.5.1 ignores the non-baseline tokens; the pinned edge build consumes `idle-boundary`, `carrier.ops`, `input-outcome`, `observe.v1` and `city-scoped`. The remaining 3 tokens stay undeclared because their behavior is not implemented. | low |
| `## protocol` § Success output: includes `"locality":"local"` in handshake JSON. | `cmd/gc-runtime-rex/main.go:43` omits `locality` (pinned by `cmd/gc-runtime-rex/dispatch_test.go:93-95`). | None under gc v1.5.1 (`ProtocolInfo` drops unknown fields per `## protocol` § Conflicts 2). A newer gc reading `locality` (e.g. for `fs.host`) will not see `"local"`. | low |
| `## protocol` § Failure cases & `## Shared RPP contract` § Deadlines and cancellation: `GC_EXEC_DEADLINE_MS` overrides deadline (`=0` fails before handler) and pre-handler `SIGTERM` fails with exit 1. | `cmd/gc-runtime-rex/main.go:138-144` does not read `GC_EXEC_DEADLINE_MS` and does not check `ctx.Err()` before writing `handshake`, so it still writes stdout and exits `0`. | `GC_EXEC_DEADLINE_MS=0` or a `SIGTERM` delivered before dispatch does not fail `protocol`. gc v1.5.1 does not send `GC_EXEC_DEADLINE_MS` and treats canceled calls as canceled regardless of exit code. | low |
| `## Shared RPP contract` § Failure envelope: stdout write failure returns exit 1 immediately with no stderr. | `cmd/gc-runtime-rex/main.go:139-142` writes `gc-runtime-rex: protocol: write output: <err>` to `stderr` before returning `exitFail` (`1`) (tested in `cmd/gc-runtime-rex/dispatch_test.go:420-426`). | Extra diagnostic line on `stderr` when `stdout` write fails; exit code (`1`) matches. | low |

Full report: [provider-method-audit/protocol.md](provider-method-audit/protocol.md)

### `start` — deviations

| Doc claim | Current implementation | Behavioral impact | Severity |
|---|---|---|---|
| Empty stdin fails exit 1 (`## start` > Failure cases) | `cmd/gc-runtime-rex/main.go:200-205` only unmarshals when `len(bytes.TrimSpace(raw)) > 0`, treating empty stdin as `{}` | Empty stdin starts a default shell instead of failing | medium |
| `startConfig` parses 17 fields including `lifecycle`, `process_names`, `ready_prompt_prefix`, `ready_delay_ms`, `session_setup`, `session_setup_script`, `session_live`, `overlay_dir`, `copy_files`, `fresh_only`, `work_dir_exclude` (`## start` > Inputs) | `cmd/gc-runtime-rex/main.go:63-70` only parses `work_dir`, `command`, `env`, `pre_start`, `nudge`, `pack_overlay_dirs`; all other fields are ignored | `session_setup*`, `overlay_dir`, `copy_files` never run/stage; `session_live` emits no warning; `fresh_only` never exits 73; `ready_prompt_prefix`/`ready_delay_ms` ignored | medium |
| Empty/missing `work_dir` with staging fields fails (`## start` > Failure cases) | `internal/provider/overlay.go:18-20` returns `nil` when `workDir == ""` or `GC_PROVIDER == ""` | Staging is silently skipped instead of failing | low |
| Staging runs before `pre_start` (`## start` > Inputs, Success order) | `internal/provider/provider.go:334-365` runs `pre_start`, checks `work_dir`, then calls `stagePackOverlays` | `pre_start` sees un-staged `work_dir`; pack overrides win over `pre_start` | low |
| `pre_start` env treats `""` as unset (`## start` > Inputs) | `internal/launch/launch.go:171-188` (`MergeEnv` via `provider.go:335`) sets `K=` instead of unsetting `K`; `launch.Command` (`launch.go:103-105`) also skips `env` when `command` is empty | `pre_start` hooks see empty env vars rather than unset; bare shell starts without `env` | medium |
| `pre_start` is fatal (pack) vs non-fatal (gc v1.5.1) (`## start` > Conflicts #1) | `internal/provider/provider.go:349-351` aborts `start` on `pre_start` failure; hook timeout is `min(30s, remaining)` of 120 s budget with no 5 s reserve (`provider.go:336-347`) | Matches herdr-core (conflicts with gc v1.5.1); slightly looser timeout near 120 s | low |
| `GC_EXEC_DEADLINE_MS`, 120 s op deadline, and stdin-open deadline poll (`## Shared RPP contract` > Deadlines) | `cmd/gc-runtime-rex/main.go:45,196` uses `context.Background()` + `io.ReadAll(stdin)`; `provider.go:177-181` notes `GC_EXEC_DEADLINE_MS` is unread | No provider-side 120 s cap on Rex calls or stdin read; relies on gc's SIGTERM | low |
| `GC_EXEC_CITY_PATH` and `GC_RPP_FAULT` read from env (`## start` > Inputs) | `internal/provider/namespace.go:42-50` reads `GC_REX_CITY`/`GC_CITY_PATH`/`GC_CITY`; `GC_RPP_FAULT` is unread | Fault-injection env var has no effect; city key uses `CityFromEnv` | low |
| Redacted stderr; exit-0 warnings on stderr (`## Shared RPP contract` > Failure envelope, `## start` > Success output) | `cmd/gc-runtime-rex/main.go:164-166` prints raw `err` to stderr without redaction; `opStart` (`main.go:206-220`) writes no exit-0 warnings | Secret values in errors are not redacted; undelivered nudge writes only meta (`GC_STARTUP_DELIVERY_UNCONFIRMED`), no stderr warning | low |

Town tab order (#47) is a Rex-only, best-effort `start` behavior, not a
deviation from a documented claim: see the parity row `start` / town tab
order in [provider-herdr-parity.md](provider-herdr-parity.md). The gc
comparison is **unverified**.

Full report: [provider-method-audit/start.md](provider-method-audit/start.md)

### `stop` — deviations

| Doc claim (`## stop` / `## Shared RPP contract`) | Current implementation | Behavioral impact | Severity |
|---|---|---|---|
| **Deadline & `GC_EXEC_DEADLINE_MS`** (`## stop` §Registration; `## Shared RPP contract` §Deadlines): `default` 30 s deadline from start, overridable by `GC_EXEC_DEADLINE_MS`; pre-handler gate fails with exit 1 `no time left before the op's deadline`. | `cmd/gc-runtime-rex/main.go:48-49,128-174,218-220` passes `signal.NotifyContext(context.Background(), ...)` with no timeout and never reads `GC_EXEC_DEADLINE_MS` (`internal/provider/provider.go:176-177`). A live-pane stop makes two `BlockProcess` round-trips on the critical path (pre-close snapshot at `provider.go:777`, post-close re-read at `:791`). | Standalone runs have no 30 s op deadline and ignore `GC_EXEC_DEADLINE_MS`. Under `gc` v1.5.1, `gc` still enforces its 30 s timeout externally via SIGTERM. | low |
| **Per-name lock wait bound & error** (`## stop` §Failure cases): waits `min(30, max(0.1, budget(30)))` s and fails with exit 1 `busy: exclusive lock <path> still held after Xs`. | `internal/provider/lock.go:13,62-97` uses `lockTimeout = 60 * time.Second` for both `withNameLock` and `withStateLock` (`provider.go:532-534`), returning `provider: timed out waiting for lock <path>`. | Contended locks wait up to 60 s instead of <=30 s (so `gc`'s 30 s SIGTERM preempts it), and the timeout message differs. | low |
| **Descendant reaping & survivor check** (`## stop` §Success output, §Failure cases, §Conflicts #1): snapshots subtree + instance token, reaps TERM then KILL (2 s), rescans up to 3x; exits 1 (`stopped 'N', but descendants may survive...`) and keeps record/meta on survivors. | `Provider.Stop` snapshots the pane subtree before `block.close`, unions in what the pane still reports from a second `BlockProcess` after the close (`ErrNotFound` and a read error are tolerated), reaps it TERM -> 2 s -> KILL -> 2 s -> rescan (`internal/provider/reap.go:287`, `unionProcs` at `:244`, `provider.go:777,791`) and fails with exit 1 rendering each survivor as pid plus command name, appending the first signal error when one could not be delivered. | Attribution is the pane's reported processes, not a token sweep, so a process reparented away before the snapshot and never reported again is missed (GCREX-50). | low |
| **Core/server unreachable** (`## stop` §Success output, §Failure cases, §Conflicts #2–3): when core is down, sweeps by stored PID/token without starting core and exits 0 if nothing runs; otherwise exits 1 and kicks core start. | `Provider.stop` asks `Binder.lookup` (local stores only) before `binder.Resolve`; an unreachable Rex with no stored binding clears the name and exits 0, a stored binding returns exit 1 and keeps the record (`internal/provider/provider.go`, `binding.go:197`). | Exit 1 only while a binding for the name is known; a lost binding record with a still-live pane reports success because the label scan needs Rex (GCREX-51). | low |
| **Stderr redaction** (`## stop` §Failure cases; `## Shared RPP contract` §Failure envelope): redacts sensitive env values and `KEY=value` / `--flag value` patterns on stderr. | `cmd/gc-runtime-rex/main.go:165` writes `gc-runtime-rex: %s: %v\n` verbatim with no redaction. | Any secret text appearing in an underlying error would be emitted unredacted to `gc`. | low |
| **Stderr prefix & SIGTERM message** (`## stop` §Failure cases; `## Shared RPP contract` §Failure envelope): errors use `<op>: <message>`; SIGTERM `Cancelled` yields `stop: canceled (terminated)`. | `cmd/gc-runtime-rex/main.go:48,165` prefixes errors with `gc-runtime-rex: stop: ...`; SIGTERM cancels `ctx` via `signal.NotifyContext`, surfacing `context canceled`. | Minor stderr text difference; exit code 1 and `gc`'s own cancel reporting are unaffected. | low |
| **Housekeeping sweep** (`## stop` §Registration; `## Shared RPP contract` §Dispatch #6): runs `sweep_spawn_files` before handler. | `cmd/gc-runtime-rex/main.go:128-174` and `internal/provider/provider.go:531-566` perform no spawn-file sweep. | None – Rex provider does not use spawn files. | low |

Full report: [provider-method-audit/stop.md](provider-method-audit/stop.md)

### `kill-object` — not implemented

| doc claim | current implementation | behavioral impact | severity |
|---|---|---|---|
| Registered in op table with `stdin=True`, 30 s default deadline, 64 KiB stdout cap, and `carrier.ops` in handshake (`docs/provider-herdr-method-io.md` §kill-object "Registration", "Conflicts") | Absent from `ops` (`cmd/gc-runtime-rex/main.go:86-103`) and `unimplemented` (`cmd/gc-runtime-rex/main.go:73-77`); `handshake` lists only `["report-attachment","report-activity"]` (`cmd/gc-runtime-rex/main.go:43`) | Calling `kill-object` exits `2` (`exitUnknown`) with `gc-runtime-rex: unknown operation "kill-object"` on stderr (`cmd/gc-runtime-rex/main.go:31,126-134`); inert for gc v1.5.1 (never calls it and treats exit `2` as no-op), breaks `carrier.ops` callers | low |
| Requires `<session-name>` arg, reads stdin JSON (`object.{id,created}`, `mode`, optional `agent_pid`), validates non-blank fields, and runs under exclusive per-name lock (`docs/provider-herdr-method-io.md` §kill-object "Inputs") | `runWithContext` returns at `cmd/gc-runtime-rex/main.go:126-134` before arg validation, stdin read, or `NamespaceForCaller` / provider construction | Missing session arg or invalid JSON is never parsed or rejected with exit `1` | low |
| Exits `0` with one-line `<outcome>\n` (`killed`, `gone`, `renamed`, `live`, `changed`) and removes record/meta/activity on `killed` (`docs/provider-herdr-method-io.md` §kill-object "Success output") | No `KillObject` method in `internal/provider/*.go`; nothing written to stdout (`cmd/gc-runtime-rex/main.go:126-134`) | Callers cannot inspect box outcomes or reap corpse/zombie panes via `kill-object` | low |
| Failure cases exit `1` with `kill-object: <message>` on stderr (`docs/provider-herdr-method-io.md` §kill-object "Failure cases") | Always exits `2` with `unknown operation "kill-object"` on stderr (`cmd/gc-runtime-rex/main.go:131-133`) | No exit `1` validation, lock-busy, or backend error reporting | low |

Full report: [provider-method-audit/kill-object.md](provider-method-audit/kill-object.md)

### `observe` — not implemented

| Doc claim | Current implementation | Behavioral impact | Severity |
|---|---|---|---|
| `observe` registered (`## observe > Registration`, `## The op table` #5) and `observe.v1` declared in handshake (`## observe > Registration`) | Absent from `ops` (`cmd/gc-runtime-rex/main.go:97-113`) and `unimplemented` (`cmd/gc-runtime-rex/main.go:82-86`); `handshake` declares only `report-attachment` and `report-activity` (`cmd/gc-runtime-rex/main.go:43`); no `Observe` method in `internal/provider/*.go` | `gc-runtime-rex observe <name>` exits 2 (`exitUnknown`) with `gc-runtime-rex: unknown operation "observe"` on stderr (`cmd/gc-runtime-rex/main.go:139-145`) | low (gc v1.5.1 has no `observe`) / medium (for newer gc expecting `observe.v1`) |
| Accepts `observe <name>`, reads stdin as newline-separated process names, runs under per-name lock (`## observe > Inputs`) | Returns `exitUnknown` (2) before arg validation (`cmd/gc-runtime-rex/main.go:147-150`), state dir lookup (`cmd/gc-runtime-rex/main.go:157`), or any stdin read | `stdin` is untouched; even `gc-runtime-rex observe` (missing `<name>`) exits 2 instead of 1 | low |
| Exits 0 with one compact ASCII JSON line (`absent`, `unknown`, `dead`, or `running` with `object`, `identity`, `agent`, `agent_pid`) (`## observe > Success output`) | No handler exists; stdout is always empty on exit 2 (`cmd/gc-runtime-rex/main.go:139-145,163-166`) | No JSON observation emitted; gc v1.5.1 instead folds `get-last-activity`, `process-alive`, and `is-attached` (`cmd/gc-runtime-rex/main.go:105,108,109`) | low (gc v1.5.1) / medium (newer gc) |
| Enforces 4 s `probe` deadline, `GC_EXEC_DEADLINE_MS`, 1 s stdin-open margin, and 1 MiB `OBSERVE_STDOUT_CAP` (`## observe > Registration`, `## Shared RPP contract > Deadlines and cancellation`, `## Shared RPP contract > Stdout caps`) | Not implemented in `cmd/gc-runtime-rex/main.go` (uses `signal.NotifyContext` for SIGTERM/SIGINT at `cmd/gc-runtime-rex/main.go:46`; no `GC_EXEC_DEADLINE_MS` or stdout cap) | None for `observe` (exits 2 immediately); broader dispatcher relies on gc SIGTERM/SIGKILL | low |
| `GC_RPP_FAULT=unavailable` / down core returns exit 0 `{"state":"unknown"}` and kicks core; parked core returns `{"state":"absent"}` (`## observe > Inputs`, `## observe > Failure cases`) | No `GC_RPP_FAULT` fault injection or daemon-kick logic in `cmd/gc-runtime-rex/main.go` or `internal/provider/*.go` | None for `observe` (always exits 2) | low |

Full report: [provider-method-audit/observe.md](provider-method-audit/observe.md)

### `inventory` — not implemented

| Doc claim | Current implementation | Behavioral impact | Severity |
|---|---|---|---|
| Registered in ops table with `reads_stdin=False`, `default` 30 s deadline, 8 MiB `INVENTORY_STDOUT_CAP` (`## inventory` § Registration) | Absent from `ops` (`cmd/gc-runtime-rex/main.go:97-114`) and `unimplemented` (`cmd/gc-runtime-rex/main.go:83-87`); no `Inventory` method in `internal/provider/*.go`. `runWithContext` returns exit 2 (`cmd/gc-runtime-rex/main.go:30,136-144`) with `gc-runtime-rex: unknown operation "inventory"`. | Calling `inventory` fails with exit 2 instead of returning an inventory object. No impact on gc v1.5.1, which never calls `inventory`. | low |
| Exit 0 single-line ASCII JSON `{"complete":bool,"sessions":[...]}` with sorted entries (`name`, `state`, `object`, `agent`, `attached`, `last_activity`, `restart_requested`, `identity`), plus screen-read activity refresh and >600 s meta GC (`## inventory` § Success output) | Not implemented; `inventory` exits 2 with empty stdout (`cmd/gc-runtime-rex/main.go:136-144`). | Callers cannot fetch a city-wide session inventory or trigger its side effects in one op; gc v1.5.1 uses `list-running` instead. | low |
| Graceful exit 0 `{"complete":false,"sessions":[]}` on `HcError` (kicking unparked core) or `{"complete":true,"sessions":[]}` when parked; exit 1 on state-dir/path/cap/signal errors (`## inventory` § Failure cases) | Exits 2 before `NamespaceForCaller()` (`cmd/gc-runtime-rex/main.go:154`) or any provider call. | State-dir, unreachable-core, and parked-city conditions are never evaluated for `inventory`. | low |
| `GC_EXEC_DEADLINE_MS` / 30 s deadline enforcement and 8 MiB stdout cap (`## inventory` § Registration; `## Shared RPP contract` § Deadlines and cancellation, § Stdout caps) | `cmd/gc-runtime-rex/main.go` neither reads `GC_EXEC_DEADLINE_MS` nor caps `out.Bytes()` (`cmd/gc-runtime-rex/main.go:165`). | Inert while `inventory` exits 2; would matter if `inventory` is implemented later. | low |

Full report: [provider-method-audit/inventory.md](provider-method-audit/inventory.md)

### `is-running` — deviations

| Doc claim | Current implementation | Behavioral impact | Severity |
|---|---|---|---|
| `## is-running` §Inputs: `GC_EXEC_CITY_PATH` selects city; `GC_EXEC_STATE_DIR` (must be absolute) sets state root. | `cmd/gc-runtime-rex/main.go:149` calls `provider.NamespaceForCaller()` (`internal/provider/namespace.go:45-53`), which reads `GC_REX_CITY` (`internal/provider/statedir.go:79-84`) and `GC_REX_STATE_DIR` / `XDG_STATE_HOME` (`internal/provider/statedir.go:11-26`); no absolute-path check. | `GC_EXEC_CITY_PATH` and `GC_EXEC_STATE_DIR` are ignored; callers using only gc's `GC_EXEC_STATE_DIR` fall back to `$XDG_STATE_HOME/rex-gascity`. | medium |
| `## is-running` §Inputs & §Failure cases: does not need core up; when core is unreachable and nothing of it runs, returns exit 0 `false\n` (otherwise exit 1 and kicks core start). | `IsRunning` (`internal/provider/provider.go:595-598`) calls `Binder.Live` (`internal/provider/binding.go:282-313`), which always queries Rex (`Resolve`/`scan` or `revalidate`/`liveOnly`), never starts Rex, and returns exit 1 on any Rex error (`cmd/gc-runtime-rex/main.go:154-157`). | None for gc v1.5.1: `IsRunning` and `Start` cleanup treat exit 1 and exit 0 `false` identically (`boxOccupancy` returns `(false, false)` vs `(false, true)`, both `IsRunning() == false`). | low |
| `## is-running` §Registration & `## Shared RPP contract` §Deadlines: 30 s default deadline or `GC_EXEC_DEADLINE_MS`; 64 KiB stdout cap; `sweep_spawn_files` housekeeping. | `runWithContext` (`cmd/gc-runtime-rex/main.go:42-48,140-159`) uses `context.Background()` with signal cancellation only; no `GC_EXEC_DEADLINE_MS` or 30 s timer, no stdout cap, no spawn-file sweep. | Relies on gc's 30 s SIGTERM/SIGKILL. 64 KiB cap is unreachable for 5–6 byte `true\n`/`false\n` output. | low |
| `## is-running` §Inputs: `GC_RPP_FAULT=unavailable` forces `HcUnreachable`. | Not implemented in `cmd/gc-runtime-rex/main.go` or `internal/provider/`. | Fault-injection env var is ignored. | low |
| `## Shared RPP contract` §Failure envelope: stderr errors are redacted (`rpp/redact.py`) and follow `"<op>: <message>"` (e.g. `canceled (terminated)`). | `cmd/gc-runtime-rex/main.go:155` writes `gc-runtime-rex: is-running: <err>` to stderr without redaction; SIGTERM yields Go's `context canceled`. | Error text differs slightly; `is-running` takes no secrets in args/stdin, though `rex` CLI stderr is passed through unredacted. | low |

Full report: [provider-method-audit/is-running.md](provider-method-audit/is-running.md)

### `process-alive` — deviations

| Doc claim | Current implementation | Behavioral impact | Severity |
|---|---|---|---|
| `default` 30 s deadline and `GC_EXEC_DEADLINE_MS` override polled on stdin and checked pre-handler (`## Shared RPP contract` §Deadlines and cancellation; `## process-alive` §Registration) | `main` uses `signal.NotifyContext(context.Background(), ...)` with no timeout and ignores `GC_EXEC_DEADLINE_MS` (`cmd/gc-runtime-rex/main.go:120`). `opProcessAlive` reads stdin through `readNameList` (`cmd/gc-runtime-rex/main.go:585`), which stops after the first 64 KiB and drops bytes past it. There is still no deadline polling on stdin | The 64 KiB byte cap bounds how much stdin is read, and bytes past it are dropped. A stdin that stays open but sends nothing, and a hung Rex call, still rely on gc's 30 s process timeout instead of self-bounding. `GC_EXEC_DEADLINE_MS` has no effect (unused in gc v1.5.1) | Low |
| `find_agent` walks the full box `subtree`, `matches` checks `node`/`bun`/`npx`/`deno` script args, and zombies never match (`## process-alive` §Success output, §Conflicts) | `processAlive` checks only `info.Foreground` and `info.Ancestors` (`internal/provider/liveness.go:60-68`); `processMatches` checks `Name`, `Argv0`, and basenames of `Path`/`Argv0` (`internal/provider/liveness.go:72-90`); `rex.Process` has no `argv` args or zombie state (`internal/rex/client.go:103-111`) | Interpreter-launched agents (`node /path/to/claude` with `Name="node"`, `Argv0="node"`) and non-foreground/non-ancestor child processes report `false` (dead) | High |
| Indeterminate state (unreadable process table, opaque child, no match with opaque process) returns exit 0 with empty stdout, which gc reads as alive (`## process-alive` §Success output, §Failure cases) | `processAlive` is strict: any non-matching or empty `Foreground`/`Ancestors` returns `false\n` (`internal/provider/liveness.go:52-69`), and non-`ErrNotFound` `BlockProcess` errors return exit 1 (`internal/provider/liveness.go:125-127`, `cmd/gc-runtime-rex/main.go:175-179`) | Indeterminate or transient `BlockProcess` errors report dead (`false` / exit 1) to gc instead of alive (passes `rppcheck`, which rejects empty stdout) | Medium |
| Core unreachable with nothing running returns exit 0 `false\n` (`## process-alive` §Success output, §Failure cases) | `p.binder.Resolve` propagates Rex connection errors (`internal/provider/binding.go:349-360,440-448`) and `opProcessAlive` exits 1 with empty stdout (`internal/provider/liveness.go:116-119`, `cmd/gc-runtime-rex/main.go:175-179`) | Rex down produces exit 1 + stderr instead of exit 0 `false\n`; gc still treats exit 1 as `false` (dead) | Low |
| Stdin decoded with UTF-8 `surrogateescape` and split with `splitlines()` (`## process-alive` §Inputs, §Failure cases) | `opProcessAlive` and `observe` share `readNameList` (`cmd/gc-runtime-rex/main.go:585`), which splits on `"\n"` only, trims each line and drops blank lines (`processNames`, `cmd/gc-runtime-rex/main.go:567`). `expandProcessNames` trims again (`internal/provider/liveness.go:42`) | `\r\n` works, but bare `\r` or Unicode line separators (e.g. U+2028) are not split; only affects direct callers since gc sends `\n` | Low |

Full report: [provider-method-audit/process-alive.md](provider-method-audit/process-alive.md)

### `list-running` — deviations

| Doc claim | Current implementation | Behavioral impact | Severity |
|---|---|---|---|
| `## list-running` § Success output: `readable_name` drops empty, whitespace-padded, or Unicode `Cc` control-character names (so `\n` cannot split into two lines). | `internal/provider/provider.go:654-671` and `cmd/gc-runtime-rex/main.go:324-326` perform no `readable_name` / control-char / whitespace-padding filter and print every matching name with `fmt.Fprintln`. | A binding name or alias containing `\n`, leading/trailing whitespace, or `Cc` chars would be printed verbatim and mis-parsed by gc's `strings.Split(out, "\n")` (`## list-running` § Success output; `docs/provider-herdr-parity.md:564` row 3). Low real-world risk because gc supplies session names. | low |
| `## list-running` § Success output: one gc session label per record, no aliases (`herdrcore/reads.py:319`). | `internal/provider/provider.go:658-671` appends matching `b.Aliases` when `prefix != ""`, while omitting aliases when `prefix == ""` (`provider.go:660-665`, `provider_alias_test.go:121-161`). | Deliberate alias-claim behavior for targeted prefix probes (`GCREX-18`); empty-prefix `ListRunning("")` matches gc's adoption/reconciler expectation (`provider.go:660-663`). | info |
| `## list-running` § Success output: stopped city (core unreachable, parked, nothing running) returns empty `stdout` and exit `0`. | `internal/provider/provider.go:646-652` returns any non-`ErrNotFound` `BlockProcess` error, so `cmd/gc-runtime-rex/main.go:171-174` exits `1` when Rex is unreachable during a live-candidate probe (`internal/rex/cli/cli.go:40-58`). (If no matching binding exists in the sidecar store, it skips probing and exits `0` empty.) | Unreachable Rex fails with exit `1` whenever a matching binding exists instead of returning empty exit `0`. Matches `docs/provider-herdr-parity.md:566` row 5 (`not-implemented`); gc treats exit `1` as an incomplete inventory (`## list-running` § Failure cases). | medium |
| `## list-running` § Success output: source set is core's city-labelled, non-finished terminals (server truth). | `internal/provider/provider.go:635-657` iterates local sidecar binding stores (`compositeBindingStore.List()` across `NamespaceForCaller()` dirs, `main.go:161-167`, `composite.go:63-79`, `namespace.go:54-62`) and probes `BlockProcess` per prefix-matching binding (`liveness.go:108-119`), ignoring `LastExit != nil` and `rex.ErrNotFound`. | Sessions not bound by this provider are never listed; conversely, an active pane whose `LastExit == nil` is listed without pid-label / `BROKEN_REASONS` checks (`docs/provider-herdr-parity.md:563,567` rows 2 & 6). Deliberate City→Rex mapping. | info |
| `## list-running` § Registration, § Failure cases; `## Shared RPP contract` § Stdout caps: discards output and exits `1` above 8 MiB (`INVENTORY_STDOUT_CAP`). | `cmd/gc-runtime-rex/main.go:175` writes full `out.Bytes()` with no 8 MiB cap check. | Outputs over 8 MiB succeed instead of failing; matches gc v1.5.1's uncapped buffering (`## list-running` § Conflicts; `docs/provider-herdr-parity.md:565` row 4). | low |
| `## list-running` § Registration, § Failure cases; `## Shared RPP contract` § Deadlines and cancellation: 30 s `default` deadline and `GC_EXEC_DEADLINE_MS` override (pre-handler `=0` -> exit `1`). | `cmd/gc-runtime-rex/main.go:46,169-179` wires `SIGTERM`/`SIGINT` to `ctx` but has no `GC_EXEC_DEADLINE_MS` parsing and no 30 s deadline timer. | `GC_EXEC_DEADLINE_MS` is ignored; long runs rely on gc's own 30 s timeout (`## list-running` § Registration). | low |
| `## Shared RPP contract` § Failure envelope: `stdout` write failure returns exit `1` with no `stderr`. | `cmd/gc-runtime-rex/main.go:175-178` writes `gc-runtime-rex: list-running: write output: <err>` to `stderr` before returning `1`. | Extra `stderr` line on `stdout` write failure; exit code (`1`) matches. | low |

Full report: [provider-method-audit/list-running.md](provider-method-audit/list-running.md)

### `is-attached` — deviations

| Doc claim | Current implementation | Behavioral impact | Severity |
|---|---|---|---|
| Attachment = local attach lockfile OR `/proc` viewer process (`hc attach` / `herdr-core attach`), never querying the server; finished records with a held lock/viewer return `true` (`## is-attached` § Success output) | Queries Rex server: `p.binder.Resolve` (`internal/provider/attach.go:19`) returns `false, nil` if missing (`:23-25`); `p.client.ListClients` (`:26`) checks client `Kind` (`"app"` sets `hasApp`, `"cli"`/`"peer"` ignored, unknown `Kind` returns `true` at `:36-42`); `p.client.SessionView` (`:48`) checks `ActiveWindowID == WindowID` and block presence (`:60-68`). | Server-backed check instead of local lock/`/proc`; gone/finished panes return `false` rather than `true`; unclassifiable client kinds fail-closed (`true`). Intentional for Rex; gc only reads `true`/`false`. | low |
| `deadline="probe"` (4.0 s) or `GC_EXEC_DEADLINE_MS` enforced from process start; expired budget exits 1 (`## is-attached` § Registration, § Failure cases; `## Shared RPP contract` § Deadlines and cancellation) | `main` wires `signal.NotifyContext(context.Background(), ...)` (`cmd/gc-runtime-rex/main.go:45`) with no timeout; neither `main.go` nor `internal/provider` reads `GC_EXEC_DEADLINE_MS` or sets a 4.0 s deadline. | Under gc v1.5.1 (30 s default, no `GC_EXEC_DEADLINE_MS`), relies on gc's 30 s timeout and Rex CLI timeouts rather than a 4 s self-deadline. | low |
| Unreachable core returns exit 0 `false\n` when `parked(ctx) and nothing_runs(ctx)`, otherwise exits 1 and kicks a core start (`## is-attached` § Inputs, § Failure cases) | No parked-city check or core auto-start. Transport errors in `Resolve`, `ListClients`, or `SessionView` (`internal/provider/attach.go:19-52`) propagate to `runWithContext` (`cmd/gc-runtime-rex/main.go:161-163`) and exit 1. | Unreachable Rex server always exits 1 (which gc v1.5.1 maps to `false`), even when parked, and never starts Rex. | low |
| Supports `GC_RPP_FAULT=unavailable`, `GC_EXEC_CITY_PATH`, `GC_HERDR_CORE_DEBUG=1`, stderr redaction, and 64 KiB stdout cap (`## is-attached` § Registration, § Failure cases; `## Shared RPP contract`) | None of these env vars, redaction, or stdout cap checks exist (`cmd/gc-runtime-rex/main.go:154-169`, `internal/provider/statedir.go:72-86`). | No impact on gc v1.5.1 (`true\n`/`false\n` is 5–6 bytes; gc v1.5.1 does not set these env vars). Fault-injection harnesses using `GC_RPP_FAULT` will not trigger. | low |

Full report: [provider-method-audit/is-attached.md](provider-method-audit/is-attached.md)

### `get-last-activity` — deviations

| Doc claim | Current implementation | Behavioral impact | Severity |
|---|---|---|---|
| `## get-last-activity` § Success output: outputs bare RFC 3339 UTC with milliseconds (`%Y-%m-%dT%H:%M:%S.mmmZ`). | `cmd/gc-runtime-rex/main.go:329` formats with `time.RFC3339` (whole seconds, e.g. `2026-11-17T18:00:00Z\n`; pinned in `cmd/gc-runtime-rex/dispatch_test.go:288`). | Sub-second precision is truncated. gc v1.5.1 parses `time.RFC3339`, which accepts both whole-second and fractional formats. | low |
| `## get-last-activity` § Inputs, § Failure cases: requires an intact/live record; a finished or dead box exits `0` with empty stdout. | `internal/provider/activity.go:54` uses `p.binder.Resolve` (`internal/provider/binding.go:330-357`), which checks `SessionView` membership (`stillBound`) without checking `BlockProcess` / `LastExit != nil` (unlike `Binder.Live` at `binding.go:258-296`). | If a pane's process exited (`LastExit != nil`) while its Rex block remains in `SessionView`, `GetLastActivity` still queries status/screen and returns a timestamp instead of empty stdout. | low |
| `## get-last-activity` § Success output: reported non-`working`/`blocked` pane uses `max(status_ms or created_ms, input_ms)` with no screen read. | `internal/provider/activity.go:71-73` requires `!latest.IsZero()`; a `program_status` record with zero `UpdatedAt` falls through to `BlockFormat` screen polling (`internal/provider/activity_test.go:132-146`) rather than using `binding.CreatedAt`. | Extra `BlockFormat` call and `"now"`/`format_poll` stamp instead of `CreatedAt` when `UpdatedAt` is zero. | low |
| `## get-last-activity` § Failure cases: unreachable core in a parked city (`server-stop`) exits `0` empty; in a non-parked city exits `1` and kicks a start. | `cmd/gc-runtime-rex/main.go:102-119` has no `server-stop`; `internal/provider/activity.go:54-57` returns any `Resolve` error and `cmd/gc-runtime-rex/main.go:171-174` exits `1` without kicking a start. | Unreachable Rex always exits `1` with empty stdout. Under gc v1.5.1, `Observe` discards `GetLastActivity` errors. | low |
| `## get-last-activity` § Registration & `## Shared RPP contract`: enforces `GC_EXEC_DEADLINE_MS` / 30 s deadline, 64 KiB stdout cap, stderr redaction, and silent stderr on stdout write failure. | `cmd/gc-runtime-rex/main.go:47-48,162-179` does not read `GC_EXEC_DEADLINE_MS`, set a 30 s `ctx` deadline, check a 64 KiB cap, or redact stderr; stdout write failure writes to stderr before exiting `1` (`main.go:175-178`). | `GC_EXEC_DEADLINE_MS=0` is ignored (gc v1.5.1 does not send it and enforces its own 30 s timeout); 64 KiB cap is unreachable for a one-line timestamp. | low |

Full report: [provider-method-audit/get-last-activity.md](provider-method-audit/get-last-activity.md)

### `peek` — deviations

| Doc claim | Current implementation | Behavioral impact | Severity |
|---|---|---|---|
| `## peek` § Success output, § Failure cases: missing record, hc `not_found`, or unreachable core with parked empty city exits `66` (`no screen for '<name>'`), empty `stdout`. | `internal/provider/provider.go:609-619` returns `"", nil` when `Resolve` finds no binding or `BlockFormat` returns `rex.ErrNotFound`, so `cmd/gc-runtime-rex/main.go:307-311` exits `0` with empty `stdout` and `stderr` (`dispatch_test.go:224-226,533-535`). Unreachable Rex always exits `1` (`main.go:171-174`). | Missing/vanished pane succeeds silently instead of failing with `66`; under gc v1.5.1, exit `0` empty avoids an error (`## peek` § Conflicts). | medium |
| `## peek` § Failure cases: `lines` not an int (including `""`, `"abc"`, `"1.5"`) exits `1`, empty `stdout`. | `cmd/gc-runtime-rex/main.go:300-306` checks `if len(args) > 1 && args[1] != ""`, so `""` is treated as `lines = 0` (success) instead of failing. (`strconv.Atoi` also rejects `" 5"`, which Python `int()` accepts.) | `peek <name> ""` returns the full output instead of exit `1`. gc v1.5.1 always sends `strconv.Itoa(lines)` (`## peek` § Conflicts). | low |
| `## peek` § Success output: splits raw text on `\n`, keeps any trailing empty row in `[-lines:]`, and always appends `\n` (empty screen -> `"\n"`). | `internal/provider/provider.go:612-638` returns `BlockFormat` output verbatim when `lines <= 0` (empty screen -> `""`, no `\n` added); `lastLines` strips one trailing `\n` before splitting and preserves trailing `\n` only if present. | Empty screen emits `""` not `"\n"`; `"a\nb\n"` with `lines=1` emits `"b\n"` not `"\n"`. Invisible to gc v1.5.1, which trims trailing `\n` (`## peek` § Conflicts). | low |
| `## peek` § Registration, § Failure cases; `## Shared RPP contract` § Stdout caps: discards output and exits `1` above 1 MiB (`OBSERVE_STDOUT_CAP`). | `cmd/gc-runtime-rex/main.go:175` writes full `out.Bytes()` with no 1 MiB cap check (`BlockFormat` pages until EOF at `internal/rex/cli/cli.go:327-350`). | Outputs over 1 MiB succeed instead of failing; matches gc v1.5.1's uncapped buffering (`## peek` § Conflicts). | low |
| `## peek` § Registration, § Failure cases; `## Shared RPP contract` § Deadlines and cancellation: 30 s `default` deadline and `GC_EXEC_DEADLINE_MS` override (pre-handler `=0` -> exit `1`). | `cmd/gc-runtime-rex/main.go:46,163-179` wires `SIGTERM`/`SIGINT` to `ctx` but has no `GC_EXEC_DEADLINE_MS` parsing and no 30 s deadline timer. | `GC_EXEC_DEADLINE_MS` is ignored; long calls rely on gc's own timeout. | low |
| `## Shared RPP contract` § Failure envelope: `stdout` write failure returns exit `1` with no `stderr`. | `cmd/gc-runtime-rex/main.go:175-178` writes `gc-runtime-rex: peek: write output: <err>` to `stderr` before returning `1`. | Extra `stderr` line on `stdout` write failure; exit code (`1`) matches. | low |

Full report: [provider-method-audit/peek.md](provider-method-audit/peek.md)

### `get-meta` — deviations

| Doc claim | Current implementation | Behavioral impact | Severity |
|---|---|---|---|
| `## get-meta` § Inputs, § Failure cases: looks up `current_record` in the city's core, requires a `gc.obj` label (foreign record exits `1`), and exits `1` when an unparked core is down or unreadable. | `internal/provider/meta.go:10-20,55-64`: `GetMeta` never contacts Rex (`ctx` is unused) and never checks session existence or `gc.obj`; `metaName` only resolves a binding alias if present and otherwise keys the store by `name` directly. | `get-meta` exits `0` with empty `stdout` when Rex is down or the session is unknown/foreign instead of failing with exit `1`. Matches gc v1.5.1 (`docs/provider-herdr-method-io.md:2606-2607` notes gc treats empty + exit `0` as unset). | low |
| `## get-meta` § Inputs, § Failure cases, § Unverified: reads `<state>/meta/<obj>.json` with `gc.obj` nonce validation, symlink (`O_NOFOLLOW`), owner, and 1 MiB read-cap checks. | `internal/provider/stores.go:275-303`, `internal/provider/filestore.go:55-67`: `fileMetaStore` reads `<state>/meta/<sha256(name)>/<sha256(name\x00key)>.json` via `os.ReadFile` (`value_b64` standard base64) with no nonce, symlink, owner, or 1 MiB truncation checks. | On-disk layout and file-security/size guards differ from the pack; round-trip values (including invalid UTF-8) are byte-identical and corrupt JSON/base64 still exits `1`. gc v1.5.1 does not inspect the sidecar format. | low |
| `## get-meta` § Registration, § Failure cases & `## Shared RPP contract` § Deadlines and cancellation: `default` 30 s deadline (overridden by `GC_EXEC_DEADLINE_MS`) and pre-handler deadline/cancel gates (`exit 1`). | `cmd/gc-runtime-rex/main.go:47,169-174,358-365`, `internal/provider/meta.go:56`, `internal/provider/provider.go:153-155`: `GC_EXEC_DEADLINE_MS` is not read, no 30 s deadline is attached to `ctx`, and `GetMeta` ignores `ctx`. | `GC_EXEC_DEADLINE_MS=0` does not fail pre-handler and `get-meta` has no Go-side 30 s timeout (`SIGTERM`/`SIGINT` still cancels `ctx` at `main.go:47`, though local reads are synchronous). Deliberately follows gc v1.5.1 (`provider.go:153-155`), which enforces its own 30 s timeout and does not set `GC_EXEC_DEADLINE_MS`. | low |
| `## get-meta` § Registration, § Failure cases & `## Shared RPP contract` § Stdout caps: `SMALL_STDOUT_CAP` = 64 KiB; values over 64 KiB are discarded and exit `1`. | `cmd/gc-runtime-rex/main.go:170-179,358-365`: `opGetMeta` writes the full value to `out` and `runWithContext` writes `out.Bytes()` to `stdout` without a size check. | Values larger than 64 KiB succeed with exit `0` instead of failing with exit `1`. gc v1.5.1 does not specify a 64 KiB stdout cap (`docs/provider-herdr-method-io.md:129-135`). | low |
| `## get-meta` § Registration & `## Shared RPP contract` § Dispatch: runs `sweep_spawn_files` housekeeping, uses `GC_EXEC_STATE_DIR`/`GC_EXEC_CITY_PATH`, and honors `GC_RPP_FAULT`. | `cmd/gc-runtime-rex/main.go:163-168`, `internal/provider/namespace.go:47-55`, `internal/provider/statedir.go:13-30`: no spawn-file sweep, no `GC_RPP_FAULT`, and state dir is resolved from `GC_REX_STATE_DIR` / `XDG_STATE_HOME` / `GC_REX_CITY`. | `GC_EXEC_STATE_DIR`, `GC_EXEC_CITY_PATH`, and `GC_RPP_FAULT` have no effect; state dir layout is provider-specific. | low |

Full report: [provider-method-audit/get-meta.md](provider-method-audit/get-meta.md)

### `set-meta` — deviations

| Doc claim | Current implementation | Behavioral impact | Severity |
|---|---|---|---|
| `## set-meta` § Inputs, § Failure cases: requires a session record in the city's core with `gc.obj`; unknown session, foreign terminal, or unreachable core exits `1`. | `internal/provider/meta.go:10-23`: `SetMeta` never contacts Rex or checks session existence; `metaName` only resolves a cached alias if present and otherwise keys the store by `name` directly. | `set-meta` succeeds (exit `0`) for unknown, stopped, or unstarted sessions instead of failing with exit `1`. | medium |
| `## set-meta` § Inputs, § Failure cases: takes an exclusive per-name cross-process `flock` (30 s wait, cut to deadline) and read-modify-writes `<state>/meta/<obj>.json` with `O_EXCL\|O_NOFOLLOW`, owner/nonce checks, and `MetaError` on corrupt files. | `internal/provider/stores.go:205-233`, `internal/provider/filestore.go:27-50,74-96`: uses one JSON file per `(name, key)` with only an in-process `sync.Mutex`, no `flock` (`lock.go` unused), no owner/symlink/nonce checks, and overwrites without reading existing state. | Concurrent `set-meta` on the same key is last-rename-wins across processes, and a `set-meta` racing `Stop`'s `clearIncarnations` (`provider.go:559,582`) can leave a key behind; corrupt existing files are overwritten rather than rejected. | low |
| `## set-meta` § Registration, § Failure cases & `## Shared RPP contract` § Deadlines and cancellation: `default` 30 s deadline (overridden by `GC_EXEC_DEADLINE_MS`), pre-handler deadline/cancel gates, and stdin-open-at-deadline timeout (`exit 1`). | `cmd/gc-runtime-rex/main.go:47,169-174,350-356`, `internal/provider/meta.go:21`: `GC_EXEC_DEADLINE_MS` is not read, no 30 s deadline is attached to `ctx`, `io.ReadAll(stdin)` blocks until EOF, and `SetMeta` ignores `ctx`. | `set-meta` does not time out after 30 s or on `GC_EXEC_DEADLINE_MS=0`, and an open `stdin` pipe is only interrupted by an external `SIGTERM`/`SIGINT` (`main.go:47`). | low |
| `## set-meta` § Inputs: stores UTF-8 `surrogateescape` JSON-escaped strings in `<state>/meta/<obj>.json`. | `internal/provider/stores.go:195-202,231-233`: stores `value_b64` (standard base64 of raw bytes) in `<state>/meta/<sha256(name)>/<sha256(name\0key)>.json`. | On-disk format differs; round-trip behavior through `get-meta` is byte-identical. | low |
| `## set-meta` § Registration & `## Shared RPP contract` § Dispatch: runs `sweep_spawn_files` housekeeping, uses `GC_EXEC_STATE_DIR`/`GC_EXEC_CITY_PATH`, and honors `GC_RPP_FAULT`. | `cmd/gc-runtime-rex/main.go:163-168`, `internal/provider/namespace.go:47-55`, `internal/provider/statedir.go:13-30`: no spawn-file sweep, no `GC_RPP_FAULT`, and state dir is resolved from `GC_REX_STATE_DIR` / `XDG_STATE_HOME` / `GC_REX_CITY`. | Fault injection and `GC_EXEC_STATE_DIR`/`GC_EXEC_CITY_PATH` overrides have no effect; state dir layout is provider-specific. | low |

Full report: [provider-method-audit/set-meta.md](provider-method-audit/set-meta.md)

### `remove-meta` — deviations

| Doc claim | Current implementation | Behavioral impact | Severity |
|---|---|---|---|
| `## Shared RPP contract` §Dispatch #8: `GC_EXEC_CITY_PATH` selects city; `GC_EXEC_STATE_DIR` sets state root. | `cmd/gc-runtime-rex/main.go:153` calls `provider.NamespaceForCaller()` (`internal/provider/namespace.go:45-53`), which reads `GC_REX_CITY` (`internal/provider/statedir.go:79-84`) and `GC_REX_STATE_DIR` / `XDG_STATE_HOME` (`internal/provider/statedir.go:13-28`). | `GC_EXEC_CITY_PATH` and `GC_EXEC_STATE_DIR` are ignored; callers setting only `GC_EXEC_STATE_DIR` fall back to `$XDG_STATE_HOME/rex-gascity`. | medium |
| `## remove-meta` §Failure cases: exits 1 if newest record lacks `gc.obj` (foreign terminal) or if core is unreachable (unless parked and nothing runs). | `Provider.RemoveMeta` (`internal/provider/meta.go:37-39`) never contacts Rex; `metaName` (`internal/provider/meta.go:9-15`) checks only `Binder.Cached` (`internal/provider/binding.go:268-274`). | Foreign panes and an unreachable Rex server both return exit 0 instead of exit 1. | low |
| `## remove-meta` §Failure cases: exits 1 on unreadable, corrupt, symlink, foreign-owner, or bad-nonce meta file. | `fileMetaStore.Delete` (`internal/provider/stores.go:245-247`) calls `os.Remove` (`internal/provider/filestore.go:108-111,135-140`) without reading, parsing, or checking symlinks/owners/nonces. | Corrupt or `000`-mode key files and symlinks are unlinked with exit 0; only parent-dir errors (e.g. `EACCES`, `EROFS`) exit 1. | low |
| `## remove-meta` §Success output: idempotent absent-key case rewrites the session meta file (creating one if none existed). | `fileMetaStore` (`internal/provider/stores.go:203-216`) keeps one file per key; `removeIfPresent` (`internal/provider/filestore.go:135-140`) ignores `fs.ErrNotExist` and writes nothing. | No file is created on absent key or unknown session; `GetMeta` still reads empty. | low |
| `## remove-meta` §Failure cases & `## Shared RPP contract` §Dispatch #6: acquires per-name lock; timeout exits 1 with `busy: ...`. | `fileTable.remove` (`internal/provider/filestore.go:108-111`) takes only an in-process `sync.Mutex`; no cross-process lock. | Concurrent invocations never fail with lock timeout `"busy: ..."`. | low |
| `## remove-meta` §Registration, §Failure cases & `## Shared RPP contract` §Deadlines: 30 s default or `GC_EXEC_DEADLINE_MS`, pre-handler deadline/cancel check, 64 KiB cap, `sweep_spawn_files`. | `runWithContext` (`cmd/gc-runtime-rex/main.go:46,121-166`) uses `signal.NotifyContext` only; `RemoveMeta` ignores `ctx` (`internal/provider/meta.go:37`); no `GC_EXEC_DEADLINE_MS`, 30 s timer, cap check, or spawn sweep. | Relies on gc's 30 s kill; a pre-cancelled/SIGTERM'd call still exits 0 if `os.Remove` finishes. 64 KiB cap is unreachable (empty stdout). | low |
| `## remove-meta` §Failure cases & `## Shared RPP contract` §Failure envelope: stderr is redacted `remove-meta: <message>`. | `cmd/gc-runtime-rex/main.go:145,149,155,160` writes unredacted `gc-runtime-rex: remove-meta: <msg>` to stderr. | Minor stderr format difference; `remove-meta` takes no secret values in args/stdin. | low |

Full report: [provider-method-audit/remove-meta.md](provider-method-audit/remove-meta.md)

### `nudge` — deviations

| Doc claim | Current implementation | Behavioral impact | Severity |
|---|---|---|---|
| Missing or dead box exits 80 with `{"outcome":"gone",...}` (`## nudge` §Inputs, §Failure cases, §Conflicts). | `livePane` (`internal/provider/provider.go:926-976`) returns `ErrSessionNotFound`; `runWithContext` types it as exit 80 `gone`, and the handshake declares `input-outcome` with `carrier.ops` (GCREX-70). | Matches the pack; gc projects `gone` to `runtime.ErrSessionNotFound`. | low |
| Empty `<name>` exits 80 `gone` (`## nudge` §Inputs, §Failure cases). | `runWithContext` (`cmd/gc-runtime-rex/main.go:107,147-150`) rejects missing or empty `<name>` with exit 1 and empty stdout. | Callers get exit 1 usage error instead of exit 80 `gone`. | low |
| Invalid UTF-8, `blocked`, `busy`, `unavailable`, and unwritten/partial send exit 80 with `{"outcome":...}` on stdout (`## nudge` §Failure cases, `## Shared RPP contract` §Input outcome). | The handshake declares `input-outcome` (GCREX-70). Invalid UTF-8, `ErrNudgeBusy`/`ErrNudgeBlocked` (`internal/provider/nudge.go:317-324`), and transport/submit errors (`internal/provider/nudge.go:343-348,367-371`) exit 1 with empty stdout and stderr text (`cmd/gc-runtime-rex/main.go`), not typed exit-80 JSON. | Non-gc callers expecting typed exit-80 JSON outcomes for those four words still receive exit 1; gc reads the declared `gone` path but not those. | medium |
| Unconfirmed delivery or lost reply after write exits 0 with `{"outcome":"delivered_unobserved",...}` on stdout (`## nudge` §Success output). | `opNudge` discards stdout (`cmd/gc-runtime-rex/main.go:256`). `nudgeReportBinding` (`internal/provider/nudge.go:395-404,457-474`) writes `<stateDir>/diagnostics/<name>-nudge-unconfirmed.json` and returns `nil` (exit 0, empty stdout). | Callers cannot read `delivered_unobserved` from stdout; gc v1.5.1 discards `nudge` stdout anyway. | low |
| `GC_EXEC_DEADLINE_MS` sets op deadline; send reserves 5 s before deadline (`## nudge` §Inputs, §Failure cases). | `GC_EXEC_DEADLINE_MS` is not read (`cmd/gc-runtime-rex/main.go:118-174`). `nudgeSubmitContext` (`internal/provider/nudge.go:409-420`) caps write+submit to `defaultNudgeSubmitBudget` (10 s) or `ctx.Deadline()`; confirmation uses `defaultNudgeConfirmWindow` (3 s, `internal/provider/nudge.go:92,374`). | Custom deadline env var is ignored; write+submit and confirmation use fixed budgets. | low |
| Empty stdin reaches handler as empty text and still checks target box (`## nudge` §Inputs). | `NudgeReport` (`internal/provider/nudge.go:191-193`) returns `nil` immediately when `text == ""` before calling `livePane`. | Empty stdin exits 0 without checking whether the pane exists (already exit 0 via `bestEffort`). | low |
| No pool-claim precondition in doc (`## nudge` §Inputs). | `nudgeReportBinding` (`internal/provider/nudge.go:354-360`) refuses `defaultPoolClaimNudge` / `poolClaimNudgeSignature` with `ErrNudgeClaimHeld` (exit 80 `busy`, GCREX-69) when `claimProbe` reports a live claim. | Prevents re-prompting a busy pool seat with `gc hook --claim`; the `busy` word makes gc's stalled-claim backstop refund the reservation instead of re-dispatching the held claim. Intentional Rex extension (GCREX-33). | info |

Full report: [provider-method-audit/nudge.md](provider-method-audit/nudge.md)

### `send-keys` — deviations

| Doc claim (`## send-keys` / `## Shared RPP contract`) | Current implementation | Behavioral impact | Severity |
|---|---|---|---|
| **Missing or dead session** (`## send-keys` §Failure cases, §Conflicts #1): exits 80 `gone` (`{"outcome":"gone",...}`) when no box holds `<name>` or the program has ended. | `runWithContext` types `ErrSessionNotFound` from `internal/provider/provider.go:722-725,755-769` as exit 80 `gone`, and the handshake declares `input-outcome` with `carrier.ops` (GCREX-70; `dispatch_test.go` `TestSendKeysGoneExits80`). | Matches herdr-core and the pack; gc projects `gone` to `runtime.ErrSessionNotFound`. | low |
| **Absent or empty `<name>`** (`## send-keys` §Failure cases): exits 80 `gone` ("no session name"). | `cmd/gc-runtime-rex/main.go:149-152` rejects missing or empty `args[0]` with exit 1 (`exitFail`), empty stdout, and usage on stderr (`dispatch_test.go:127`). | Fails with exit 1 instead of exit 80 `gone`. | low |
| **Zero keys on missing/dead session** (`## send-keys` §Inputs Preconditions): zero keys still looks up `<name>`, returning exit 0 on a live box and exit 80 `gone` on a missing/dead box. | `cmd/gc-runtime-rex/main.go` and `internal/provider/provider.go:719-721` return `nil` (exit 0) immediately when `len(keys) == 0`, before `livePane` or any Rex call (`input_test.go:206-215`). | Zero keys succeeds even if the Rex server is unreachable, and on missing/dead sessions (zero keys is no input). | low |
| **Key grammar & pre-validation** (`## send-keys` §Inputs Key grammar, §Failure cases): pre-validates all keys before lookup; case-sensitive `TMUX_KEYS` (`BTab`, `PPage`, `NPage`, `F1`–`F12`); `C-`/`M-`/`S-` require `len > 2` and keep typed order; splits literal text char-by-char; rejects unknown keys/chars with exit 80 `rejected`. | `internal/provider/provider.go:722-732` calls `livePane` first, then `rexKey` (`internal/provider/keymap.go:9-87`): case-insensitive named keys (`Return`, `Backspace`, `F1`–`F24`, but no `BTab`/`PPage`/`NPage`); `splitMods` normalizes order to `shift,ctrl,alt` and accepts `len == 2`; no literal text splitting; unknown/invalid keys pass through verbatim to `rex send-key` (which errors at runtime -> exit 1). | `BTab`/`PPage`/`NPage`, multi-char literal text, and invalid keys behave differently; invalid keys fail with exit 1 after `livePane` instead of pre-validated exit 80 `rejected`. (`gc` v1.5.1 only sends `Enter`, `Up`, `Down`, `Escape`, `C-u`, all supported.) | medium |
| **Exit 80 `input-outcome` envelope** (`## send-keys` §Failure cases; `## Shared RPP contract` §Failure envelope): returns exit 80 with `{"outcome":...,"detail":...}` on stdout for `rejected`, `gone`, `not_sent`, `unavailable`. | `rejected` is exit 80 with JSON (GCREX-43); `gone`/`unavailable`/`not_sent` are typed in the provider (GCREX-42) and `runWithContext` answers exit 80 for them. The handshake declares `input-outcome` and `carrier.ops` (GCREX-70). | Matches the pack; gc 1.5.1 reads exit 80 as a plain error, the pinned edge build reads the typed word. | low |
| **Deadline & `GC_EXEC_DEADLINE_MS`** (`## send-keys` §Registration; `## Shared RPP contract` §Deadlines): `default` 30 s deadline from start, overridable by `GC_EXEC_DEADLINE_MS`; pre-handler gate fails with exit 1. | `cmd/gc-runtime-rex/main.go:48-49,128-174` uses `signal.NotifyContext(context.Background(), ...)` with no timeout and ignores `GC_EXEC_DEADLINE_MS`. | Standalone runs have no 30 s deadline; under `gc` v1.5.1, `gc` enforces its 30 s timeout externally via SIGTERM. | low |
| **Stderr prefix, redaction, SIGTERM message** (`## Shared RPP contract` §Failure envelope): stderr is `<op>: <message>` with secret redaction; SIGTERM yields `send-keys: canceled (terminated)`. | `cmd/gc-runtime-rex/main.go:48,165` prefixes with `gc-runtime-rex: send-keys: ...` without redaction; SIGTERM cancels `ctx`, surfacing `context canceled`. | Minor stderr text difference; any secret in an error is unredacted. | low |
| **Housekeeping sweep** (`## Shared RPP contract` §Dispatch #6): runs `sweep_spawn_files` before handler. | `cmd/gc-runtime-rex/main.go:128-174` and `internal/provider/provider.go:718-738` perform no spawn-file sweep. | None – Rex provider does not use spawn files. | low |

Full report: [provider-method-audit/send-keys.md](provider-method-audit/send-keys.md)

### `interrupt` — deviations

| Doc claim | Current implementation | Behavioral impact | Severity |
|---|---|---|---|
| Nonexistent or dead session exits 80 with `{"outcome":"gone","detail":...}` on stdout (`§interrupt` Success output, Failure cases; note `§interrupt` Conflicts #1 says gc v1.5.1 expects exit 0) | `opInterrupt` returns `p.Interrupt` directly; `runWithContext` types `ErrSessionNotFound` from `livePane` as exit 80 `gone`. The handshake declares `input-outcome` with `carrier.ops` (GCREX-70), and gc projects `gone` to `runtime.ErrSessionNotFound` (gc `internal/runtime/exec/outcome.go:193`). | Matches the pack and herdr-core. | low |
| Empty `<name>` (`""`) yields exit 80 `gone` (`§interrupt` Inputs, Failure cases) | `runWithContext` rejects `rest[0] == ""` with exit 1 (`exitFail`) and usage on stderr (`cmd/gc-runtime-rex/main.go:165-167`) | Empty session name fails with exit 1 instead of exit 80 `gone` | low |
| Handshake declares `carrier.ops` and `input-outcome` (`§interrupt` Registration) | `handshake` declares `["report-attachment","report-activity","idle-boundary","carrier.ops","input-outcome","observe.v1","city-scoped"]` (`cmd/gc-runtime-rex/main.go:58`) | gc selects the driving ops and reads their typed exit 80 | low |
| Unreachable server, non-RPC errors, unwritten cancel, and hc `not_found`/`finished`/`unavailable` refusals exit 80 (`unavailable`, `not_sent`, or `gone`) (`§interrupt` Failure cases) | `p.Interrupt` returns wrapped client/resolve errors; `runWithContext` maps every non-`ErrSessionNotFound` error to exit 1 (`exitFail`) with empty stdout (`internal/provider/provider.go:925-941`; `cmd/gc-runtime-rex/main.go:197-198`) | Server-down and pre-send transport failures surface as hard exit-1 errors instead of typed exit-80 outcomes | medium |
| `default` 30 s deadline, `GC_EXEC_DEADLINE_MS` override, 1 s margin, and `min(5 s, remaining - 1 s)` call timeout (`§interrupt` Registration, Inputs; `§Shared RPP contract` Deadlines) | `main` uses `signal.NotifyContext(context.Background(), ...)` with no timeout; `GC_EXEC_DEADLINE_MS` is not read; `p.Interrupt` sets no 5 s call timeout (`cmd/gc-runtime-rex/main.go:52`; `internal/provider/provider.go:238-241,922-941`) | Hung `rex` calls rely on gc's external 30 s process timeout rather than self-enforcing a 30 s / 5 s budget | low |
| Calls `hc` `terminal.interrupt` with a 3 s settle wait (`§interrupt` Inputs) | Sends `ctrl+c` keystroke via `rex send-key` with no settle wait (`internal/provider/provider.go:67-72,930`; `internal/rex/cli/cli.go:370-378`) | Relies on terminal line discipline / app handling `ctrl+c` rather than a backend interrupt call and 3 s wait | low |
| Redacts secrets/env values from stderr (`§Shared RPP contract` Failure envelope) | `runWithContext` writes `err` to stderr verbatim without redaction (`cmd/gc-runtime-rex/main.go:197`) | Sensitive tokens present in an underlying error message would leak to stderr | low |

Full report: [provider-method-audit/interrupt.md](provider-method-audit/interrupt.md)

### `clear-scrollback` — deviations

| Doc claim | Current implementation | Behavioral impact | Severity |
|---|---|---|---|
| **What is cleared (`## clear-scrollback` § Success output, § Conflicts):** sends nothing and clears nothing (`terminal.get` intactness check only, then answers `delivered`). | `internal/provider/provider.go:621-630` resolves the binding via `ResolveFresh` and calls `p.client.BlockClear(ctx, binding.SessionID, binding.BlockID)`, which invokes `rex api call block.method {"block_id":...,"method":"clear","session_id":...}` (`internal/rex/cli/cli.go:379-382,421-432`). | Performs a real server-side scrollback clear on Rex rather than a no-op. `gc` v1.5.1 expects a real clear (`## clear-scrollback` § Conflicts). | info |
| **Missing or dead session (`## clear-scrollback` § Failure cases, § Conflicts):** no box or dead/ended program exits `80` with `{"outcome":"gone","detail":...}` on stdout and `clear-scrollback: gone: ...` on stderr. | `ClearScrollback` (`internal/provider/provider.go:856-872`) resolves with `livePane` and returns `paneLookupOutcome`'s typed `gone` for `ErrSessionNotFound`; a mid-call `rex.ErrNotFound` from `BlockClear` is also `gone`, `rex.ErrUnavailable` is `unavailable`, and any other error stays exit 1. `opClearScrollback` (`cmd/gc-runtime-rex/main.go:467-469`) returns the provider error directly (GCREX-57; GCREX-70 types the `ErrSessionNotFound` path). | Matches the pack (`80` `gone`). `gc` v1.5.1 reads it as a plain error but discards `ClearScrollback` errors at its reconciler call sites, so gc behavior is unchanged. | low |
| **Liveness pre-check (`## clear-scrollback` § Inputs, § Failure cases):** checks that the record's program is intact and issues `terminal.get` before answering; dead program exits `80` `gone`. | `internal/provider/provider.go:622` uses `ResolveFresh` (which validates session/tab/block presence in `SessionView`, `internal/provider/binding.go:355-393`, without calling `BlockProcess` to check `LastExit`), then calls `BlockClear` directly. | A bound pane whose program exited (`LastExit != nil`) while the block still exists receives `block.method clear` (succeeding with exit `0` or ignoring `rex.ErrNotFound`) instead of returning `gone`. | low |
| **Absent or empty `<name>` (`## clear-scrollback` § Inputs, § Failure cases):** missing or empty `args[0]` exits `80` with `{"outcome":"gone","detail":"no session name"}`. | `cmd/gc-runtime-rex/main.go:109,154-157` requires 1 non-empty positional arg and rejects missing/empty `<name>` with exit `1` (`exitFail`), empty stdout, and usage on stderr (`cmd/gc-runtime-rex/dispatch_test.go:127`). | Fails with exit `1` usage error instead of exit `80` `gone`. | low |
| **Unreachable core / refusal / `input-outcome` exit `80` (`## clear-scrollback` § Failure cases, `## Shared RPP contract` § Failure envelope):** unreachable core or unwritten/partial failure exits `80` (`unavailable` / `not_sent`) with outcome JSON on stdout; `handshake` declares `input-outcome`. | The handshake declares `input-outcome` (GCREX-70). Any non-`ErrNotFound` error from `Resolve` or `BlockClear` exits `1` with empty stdout and `gc-runtime-rex: clear-scrollback: <err>` on stderr. | `gc` 1.5.1 treats exit `1` as an error (and discards the result at both reconciler call sites); non-`gc` callers do not receive structured `input-outcome` JSON for clear-scrollback. | low |
| **Deadline & `GC_EXEC_DEADLINE_MS` (`## clear-scrollback` § Registration, `## Shared RPP contract` § Deadlines and cancellation):** 30 s `default` deadline overridable by `GC_EXEC_DEADLINE_MS`, with pre-handler expiry check and `canceled (terminated)` on SIGTERM. | `cmd/gc-runtime-rex/main.go:48-49,131-181` uses `signal.NotifyContext` for `SIGTERM`/`SIGINT` with no 30 s timer and does not read `GC_EXEC_DEADLINE_MS`; cancellation surfaces `context canceled` (exit `1`). | Standalone runs have no 30 s deadline and ignore `GC_EXEC_DEADLINE_MS`; under `gc` v1.5.1, `gc` enforces its 30 s timeout externally via SIGTERM. | low |
| **Stderr prefix, redaction, and stdout write error (`## Shared RPP contract` § Failure envelope):** stderr uses `<op>: <message>` with secret redaction; stdout write failure returns exit `1` with no stderr. | `cmd/gc-runtime-rex/main.go:173,177` prefixes stderr with `gc-runtime-rex: clear-scrollback: ...` without redaction, and writes `write output` to stderr if `stdout.Write` fails. | Minor stderr formatting difference; any secret in an error string would not be redacted. | low |
| **Housekeeping sweep (`## Shared RPP contract` § Dispatch #6):** runs `sweep_spawn_files` before handler. | `cmd/gc-runtime-rex/main.go:131-181` and `internal/provider/provider.go:621-630` perform no spawn-file sweep. | None – Rex provider does not use spawn files. | low |

Full report: [provider-method-audit/clear-scrollback.md](provider-method-audit/clear-scrollback.md)

### `watch-startup` — not implemented

| Doc claim | Current implementation | Behavioral impact | Severity |
|---|---|---|---|
| `watch-startup` registered (`## watch-startup > Registration`, `## The op table` #20) | Listed in `unimplemented` (`cmd/gc-runtime-rex/main.go:87-91`, line 90) and absent from `ops` (`cmd/gc-runtime-rex/main.go:99-116`); no `WatchStartup` method in `internal/provider/*.go` | `gc-runtime-rex watch-startup <name>` exits `2` (`exitUnknown`) with `gc-runtime-rex: operation "watch-startup" not implemented` on stderr and empty stdout (`cmd/gc-runtime-rex/main.go:141-149`). gc v1.5.1 treats exit `2` with no event as unsupported and falls back to `peek` polling + `send-keys` (`## watch-startup > Conflicts`). | low (gc v1.5.1 falls back to `peek`) |
| Accepts `watch-startup <name>`, validates non-empty `<name>` (missing/empty exits `1` `watch-startup: missing session name`), requires existing record (`no session 'X'` exits `1`), ignores extra args, runs `sweep_spawn_files` housekeeping (`## watch-startup > Inputs`, `## watch-startup > Failure cases`) | Returns `exitUnknown` (`2`) before arg validation (`cmd/gc-runtime-rex/main.go:165-167`), `NamespaceForCaller` state-dir lookup (`cmd/gc-runtime-rex/main.go:160`), or any Rex call | `gc-runtime-rex watch-startup` (missing `<name>`) and `gc-runtime-rex watch-startup ""` also exit `2` instead of `1`; no state dir or Rex server is touched. | low |
| Streams one compact ASCII NDJSON line `{"content":"<screen>"}\n` via `emit` on first read and on each screen-only (`lines:0`) change every 250 ms (`WATCH_POLL_S`), ending ~1 s before deadline with exit `0` (`## watch-startup > Success output`, `## Shared RPP contract > Success envelope`) | No streaming hook in `runWithContext` (handlers write to a buffered `bytes.Buffer` at `cmd/gc-runtime-rex/main.go:169-174`); no handler emits `{"content":...}` lines | No NDJSON screen stream is emitted; gc v1.5.1 loses the streaming optimization and polls `peek` when `ShouldAcceptStartupDialogs` is true (`## watch-startup > Conflicts`). | low (gc v1.5.1) |
| Enforces 120 s `start` deadline (or `GC_EXEC_DEADLINE_MS`), 1 s `budget` margin, `SIGTERM` cancellation (`exit 1` `canceled (terminated)`), and `BrokenPipeError` exit `1` on closed stdout (`## watch-startup > Registration`, `## watch-startup > Failure cases`, `## Shared RPP contract > Deadlines and cancellation`) | `cmd/gc-runtime-rex/main.go:46` wires `SIGTERM`/`SIGINT` via `signal.NotifyContext`, but has no `GC_EXEC_DEADLINE_MS` parsing, no 120 s deadline timer, and no streaming `emit` path | None for `watch-startup` (exits `2` immediately); gc v1.5.1 SIGKILLs `watch-startup` outright without `SIGTERM` and sets no `GC_EXEC_DEADLINE_MS` (`## watch-startup > Conflicts`). | low |
| Core unreachable on record read exits `1` and kicks core; mid-stream `HcError` exits `1` with `cannot read the screen: …` keeping prior emitted lines (`## watch-startup > Failure cases`) | No core-kick logic or `watch-startup` screen-read loop exists in `cmd/gc-runtime-rex/main.go` or `internal/provider/*.go` | None for `watch-startup` (always exits `2`). | low |

Full report: [provider-method-audit/watch-startup.md](provider-method-audit/watch-startup.md)

### `attach` — deviations

| Doc claim | Current implementation | Behavioral impact | Severity |
|---|---|---|---|
| `## attach` § Success output: `os.execve(hc_bin, [hc_bin, "attach", record.address], env)` replaces the process and blocks as an interactive TTY viewer (`hc attach`); writes nothing itself and exits with `hc`'s exit code. | `cmd/gc-runtime-rex/main.go:233-241` (`opAttach`, `attachHint`) writes a one-line hint (`Rex has no in-terminal attach; open the Rex app and select the session "<name>" to attach\n`) to `stdout` and returns `nil` (exit `0` at `main.go:180`). | Deliberate divergence from the herdr-core pack: Rex has no in-terminal attach. Matches gc v1.5.1's `Attach` contract ("tty passthrough", `docs/provider-herdr-parity.md:851,856`) better than exiting `2`, because gc v1.5.1 runs `attach` via `runWithTTY` without mapping exit `2` to success (`gc session attach` would fail with `exit status 2`). | info |
| `## attach` § Inputs & § Failure cases: requires a live, intact record for `<name>`, an executable pinned `hc`, and a responding core (`hello()`), exiting `1` on unknown/dead session or core down. | `cmd/gc-runtime-rex/main.go:233-236` never calls `Provider` or Rex (`p` is `_`); `TestAttachSucceedsWithHint` (`cmd/gc-runtime-rex/dispatch_test.go:328-353`) pins exit `0` with the hint for a live pane, an unknown session, and an unreachable Rex client (`downRex`). | An unknown session or unreachable Rex server exits `0` with the hint instead of failing `gc session attach`. Note `runWithContext` still calls `provider.NamespaceForCaller()` (`main.go:164-168`) before `opAttach`, so an invalid state dir / missing city path exits `1` (`state dir: ...`) even though `p` is unused. | low |
| `## attach` § Success output: holds a shared `flock` on the incarnation's attach lock across `execve` so `is-attached` can detect attached viewers. | `cmd/gc-runtime-rex/main.go:233-236` takes no lock. `internal/provider/attach.go:18-73` (`IsAttached`, `watchesAgent`) probes viewer attachment server-side via `ListClients` and `SessionView` instead of a lock file. | None: intentional per `docs/provider-herdr-parity.md:853` because Rex tracks connected app clients server-side. | info |
| `## attach` § Registration & `## Shared RPP contract` § Deadlines and cancellation: `start` deadline (120 s) overridden by `GC_EXEC_DEADLINE_MS` (fails pre-handler when expired), `housekeeping=True` spawn-file sweep, and `SMALL_STDOUT_CAP` (64 KiB) check. | `cmd/gc-runtime-rex/main.go:131-180` (`runWithContext`) does not read `GC_EXEC_DEADLINE_MS`, applies no 120 s timer, runs no spawn-file sweep, and enforces no 64 KiB stdout cap (signal cancellation only via `signal.NotifyContext` at `main.go:46`). | None under gc v1.5.1: gc never sets `GC_EXEC_DEADLINE_MS`, `runWithTTY` applies no deadline to `attach` (`docs/provider-herdr-parity.md:854`), and `attachHint` is ~90–120 bytes (well under 64 KiB). | info |
| `## Shared RPP contract` § Failure envelope: stdout write failure returns exit `1` immediately with no stderr. | `cmd/gc-runtime-rex/main.go:176-179` writes `gc-runtime-rex: attach: write output: <err>` to `stderr` before returning `exitFail` (`1`). | Extra diagnostic line on `stderr` when `stdout` write fails; exit code (`1`) matches. | low |

Full report: [provider-method-audit/attach.md](provider-method-audit/attach.md)

### `copy-to` — not implemented

| Doc claim | Current implementation | Behavioral impact | Severity |
|---|---|---|---|
| `copy-to` is registered in the op table (`## The op table` #22, `## copy-to > Registration`) with `reads_stdin=False`, `default` 30 s deadline, 64 KiB stdout cap, and `housekeeping=True`. | Absent from `ops` (`cmd/gc-runtime-rex/main.go:97-113`) and `unimplemented` (`cmd/gc-runtime-rex/main.go:82-86`); no `CopyTo` method or `copy-to` handler exists in `internal/provider/*.go` (only `stagePackOverlays`/`copyTree` in `internal/provider/overlay.go:16-66` for `start` pack overlays). | `gc-runtime-rex copy-to <name> <src> [rel-dst]` exits `2` (`exitUnknown`) with `gc-runtime-rex: unknown operation "copy-to"` on stderr and empty stdout (`cmd/gc-runtime-rex/main.go:139-145`). Under gc v1.5.1, exit `2` is treated as a successful no-op for non-`start`/`provision` ops (`## copy-to > Failure cases`, `## Shared RPP contract > Inherited from gc v1.5.1 vs pack choices`), and gc v1.5.1 has no known controller caller in the exec path (`## copy-to > Success output`). | low (gc v1.5.1) / medium (if a caller relies on `copy-to` staging) |
| Accepts `copy-to <name> <src> [rel-dst]`, validates >= 2 args, requires a live intact record and absolute `work_dir`/`cwd`, and stages `src` into the work dir via `stage_path` (missing `src` is a no-op; refuses `../` escapes, symlinks, and non-regular files) with exit `0` / `1` (`## copy-to > Inputs`, `## copy-to > Success output`, `## copy-to > Failure cases`). | Returns `exitUnknown` (`2`) before arg-count validation (`cmd/gc-runtime-rex/main.go:147-150`), state-dir resolution (`cmd/gc-runtime-rex/main.go:157`), or any filesystem work. | No files are copied; even `gc-runtime-rex copy-to` (fewer than 2 args) or `copy-to` on a missing session or invalid destination exits `2` instead of `1`. | low (gc v1.5.1) / medium (if a caller relies on `copy-to` staging) |
| Enforces 30 s `default` deadline, `GC_EXEC_DEADLINE_MS` override, and 64 KiB `SMALL_STDOUT_CAP` (`## copy-to > Registration`, `## Shared RPP contract > Deadlines and cancellation`, `## Shared RPP contract > Stdout caps`). | `cmd/gc-runtime-rex/main.go:46,163-179` wires `SIGTERM`/`SIGINT` via `signal.NotifyContext`, but has no `GC_EXEC_DEADLINE_MS` parsing, no 30 s deadline timer, and no 64 KiB stdout cap check. | None for `copy-to` (exits `2` immediately); broader dispatcher relies on gc's 30 s timeout and `SIGTERM`/`SIGKILL` rather than a self-enforced deadline. | low |

Full report: [provider-method-audit/copy-to.md](provider-method-audit/copy-to.md)

### `server-stop` — not implemented

| Doc claim | Current implementation | Behavioral impact | Severity |
|---|---|---|---|
| `server-stop` registered in the op table with `reads_stdin=False`, `default` 30 s deadline, 64 KiB stdout cap, and `housekeeping=True`, and `server-lifecycle` declared in the handshake (`## server-stop` §Registration, `## The op table` #23) | Absent from `ops` (`cmd/gc-runtime-rex/main.go:94-111`) and `unimplemented` (`cmd/gc-runtime-rex/main.go:80-84`); `handshake` declares only `["report-attachment","report-activity"]` (`cmd/gc-runtime-rex/main.go:43`); no `server-stop`, `server-lifecycle`, or parked-city lifecycle code exists in `internal/provider/*.go`. `runWithContext` exits `2` (`exitUnknown`) with `gc-runtime-rex: unknown operation "server-stop"` on stderr (`cmd/gc-runtime-rex/main.go:32,135-143`). | Calling `server-stop` exits `2` with empty stdout instead of stopping a server. Inert for gc v1.5.1, which has no `server-stop` wire op and ignores unknown handshake capabilities (`## server-stop` §Registration, §Conflicts); `gc stop` skips `ServerLifecycleProvider` on exec providers (`docs/provider-herdr-parity.md` §server-stop). Deliberate per shared-Rex-server design (D3). | low |
| Accepts `<script> server-stop` with no required args, ignores extra args, derives scope from `GC_EXEC_CITY_PATH` (defaulting to `unscoped`), reads `GC_EXEC_STATE_DIR` and `GC_RPP_FAULT`, and runs under an exclusive `kick` lock (`## server-stop` §Inputs) | `runWithContext` rejects `server-stop` at `cmd/gc-runtime-rex/main.go:135-143` before arg count checks (`cmd/gc-runtime-rex/main.go:144-147`) or `provider.NamespaceForCaller()` (`cmd/gc-runtime-rex/main.go:154`); `GC_RPP_FAULT` and `GC_EXEC_CITY_PATH` are not read anywhere in `cmd/gc-runtime-rex/main.go` or `internal/provider/*.go`. | `stdin` is untouched and no state dir or lock is opened; even extra args exit `2` instead of `0` or `1`. Inert for gc v1.5.1. | low |
| Exits `0` with empty stdout (and optional stderr warning if the parked mark cannot be written) after ending city sessions and stopping the core (unless foreign terminals keep it running), or `0` as a no-op when nothing is running (`## server-stop` §Success output) | Returns exit `2` with empty stdout and `gc-runtime-rex: unknown operation "server-stop"` on stderr (`cmd/gc-runtime-rex/main.go:139-143`); never stops a Rex server or sweeps/parks city state. | No server teardown or city parking occurs; gc v1.5.1 treats exit `2` on non-start ops as success and never sends `server-stop` (`## server-stop` §Conflicts). | low |
| All failures exit `1` with empty stdout and redacted `server-stop: <message>` on stderr (`unavailable`, kick lock timeout, surviving sessions, core still answering/running, unreadable process table, `ConfigError`/`MetaError`/`LockFileError`, SIGTERM cancel, deadline expired at entry) (`## server-stop` §Failure cases) | Always exits `2` at `cmd/gc-runtime-rex/main.go:135-143` before any handler, lock, or `NamespaceForCaller()` error path. | None of the exit `1` failure paths exist for `server-stop`. Inert for gc v1.5.1. | low |
| Enforces 30 s `default` deadline / `GC_EXEC_DEADLINE_MS` with 1 s `deadline.budget` margin and 64 KiB `SMALL_STDOUT_CAP` (`## server-stop` §Registration; `## Shared RPP contract` §Deadlines and cancellation, §Stdout caps) | `cmd/gc-runtime-rex/main.go` does not read `GC_EXEC_DEADLINE_MS` (`internal/provider/provider.go:151-152` notes this explicitly), enforces no 30 s runner timer, and imposes no 64 KiB stdout cap. | Inert for `server-stop` because it exits `2` immediately before any I/O or wait. | low |

Full report: [provider-method-audit/server-stop.md](provider-method-audit/server-stop.md)

### `x-kick-core` — not implemented

| doc claim | current implementation | behavioral impact | severity |
|---|---|---|---|
| Registered in op table with `reads_stdin=False`, `housekeeping=False`, `default` 30 s deadline, 64 KiB stdout cap (`## x-kick-core` §Registration) | Absent from `ops` (`cmd/gc-runtime-rex/main.go:119-136`) and `unimplemented` (`cmd/gc-runtime-rex/main.go:105-109`); no kick handler or `kick.json` logic in `internal/provider/*.go`. `runWithContext` exits `2` (`exitUnknown`) with `gc-runtime-rex: unknown operation "x-kick-core"` on stderr (`cmd/gc-runtime-rex/main.go:31-39,157-163`). | Calling `x-kick-core` exits `2` instead of running a core kick. Inert for gc v1.5.1 (never calls `x-kick-core`; `## x-kick-core` §Conflicts) and Rex has no internal `x-kick-core` spawner. | low |
| Invoked as `<bin> x-kick-core` with no further args, spawned by `kick()` on `HcUnreachable` with `GC_EXEC_DEADLINE_MS` stripped (`## x-kick-core` §Inputs) | `runWithContext` rejects `x-kick-core` at `cmd/gc-runtime-rex/main.go:132-140` before arg checks or `NamespaceForCaller`; no `kick()` or `x-kick-core` spawn site exists in `internal/provider/*.go`. | Read ops never spawn a background `x-kick-core` process when Rex is unreachable; Rex CLI calls fail directly in the op. | low |
| Exit `0` with empty stdout/stderr across 4 paths (`started`, `already running`, kick not due, lock held), writing `kick.json` in `pack_state` on `started`/`already running` (`## x-kick-core` §Success output) | Returns exit `2` with empty stdout and `unknown operation` on stderr (`cmd/gc-runtime-rex/main.go:157-163`); no `kick.json` is ever written. | No durable `kick.json` status file or background core start occurs. | low |
| Failure cases exit `1` (`ConfigError`, `check_core_lock`, `hc server start` failure, hello timeout, `HcError`/`OSError`, `LockFileError`, expired deadline, start budget <= 0, `Cancelled`) with `kick.json` `ok:false` on post-lock failures (`## x-kick-core` §Failure cases) | Always exits `2` before `NamespaceForCaller()` (`cmd/gc-runtime-rex/main.go:175`) or any lock/backend call. | No exit `1` kick failure paths or `kick.json` failure records exist. | low |
| `GC_EXEC_DEADLINE_MS` / 30 s default deadline with 1 s `deadline.budget` margin (`## x-kick-core` §Registration, §Failure cases; `## Shared RPP contract` §Deadlines and cancellation) | `cmd/gc-runtime-rex/main.go` does not read `GC_EXEC_DEADLINE_MS` (`internal/provider/provider.go:238-241` notes this explicitly) and enforces no 30 s runner timer. | Inert while `x-kick-core` exits `2`; would matter only if implemented. | low |

Full report: [provider-method-audit/x-kick-core.md](provider-method-audit/x-kick-core.md)

## Tracked fix work

The 17 `medium` deviations are tracked as Backlog tasks, each linked to the op's
audit report. Every task is labelled `for-gascity` and `tdd`, and its notes
state the TDD rule: write the failing test that reproduces the deviation first,
watch it go red, then make the smallest change that turns it green (AGENTS.md;
the `tdd` skill).

| Task | Deviations covered |
|---|---|
| GCREX-40 | GC_EXEC_CITY_PATH / GC_EXEC_STATE_DIR ignored (`is-running`, `remove-meta`) |
| GCREX-41 | `list-running`: unreachable Rex fails instead of empty exit 0 |
| GCREX-42 | `send-keys`: missing or dead session exits 0 (exit 80 `gone` expected) — closed by declaring `input-outcome`/`carrier.ops` in GCREX-70 |
| GCREX-43 | `send-keys`: key grammar not pre-validated, no exit 80 `rejected` |
| GCREX-44 | `interrupt`: exit 0 / exit 1 instead of exit 80 outcomes — exit 80 `gone` closed by GCREX-70; unreachable-server `unavailable`/`not_sent` remain |
| GCREX-45 | `nudge`: no exit-80 outcome JSON, unconfirmed send not on stdout |
| GCREX-46 | `process-alive`: indeterminate state reported dead |
| GCREX-47 | `process-alive`: interpreter-launched agents do not match (audited `high`) |
| GCREX-48 | `peek`: missing screen exits 0 instead of 66 |
| GCREX-49 | `set-meta`: no session-record precondition |
| GCREX-52 | `start`: empty stdin starts a default shell |
| GCREX-53 | `start`: no post-launch exit / first-turn check |
| GCREX-54 | `start`: failing `pre_start` is fatal where gc tolerates it |
| GCREX-55 | `start`: empty `pre_start` env value passed as `K=` not unset |
| GCREX-56 | `start`: 8 documented `startConfig` fields silently dropped |
| GCREX-57 | `clear-scrollback`: missing or dead session exits 0 (exit 80 `gone` expected) |

GCREX-47 is the one `high`-severity deviation; it is tracked here rather than
left out, because it shares a file and a decision with GCREX-46.

`stop` now carries no `medium` deviation: the descendant reap and survivor
check (GCREX-50) and the unreachable-Rex behaviour (GCREX-51) are implemented
and pinned by the tests cited in
[provider-method-audit/stop.md](provider-method-audit/stop.md).

Fix work is expected to change behaviour only where the task says so; several of
these deviations are deliberate gc v1.5.1 compatibility choices, and GCREX-42,
GCREX-48, GCREX-50, GCREX-51, GCREX-54 and GCREX-56 make that trade-off
explicitly before changing code.
