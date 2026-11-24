# A city-owned Rex server: `rex server run` for start/stop and attach

**Status:** investigation and recommendation. No implementation in this change.
Closes GCREX-30 (bead `gcrex-xo0s`).

D3 (`docs/plans/native-rex-provider.md:83`, `docs/REX-CONTRACT.md:596`) says the
provider never owns the Rex server. One shared server also serves the user's
other work. The alternative column says a per-city server is "impossible".
`rex server run` makes it possible. This note records what was measured. It
also recommends a staged revision of D3.

## Summary of the recommendation

1. **D3 stays the default.** With no configuration, the provider still uses the
   user's server. It never stops that server. `server-stop` stays unimplemented.
2. **Add an opt-in city-owned server.** City supervision starts it, not the
   provider op path. `server-stop` becomes implementable behind an ownership
   record. The provider stops only a server whose state-dir marker it wrote.
3. **Use `--restore=false` for that server.** Restore brings dead panes back as
   empty shells with their old block ids. That contradicts gc's dead-pane model.
4. **`--exit-empty` is not the lifecycle.** With zero sessions the server exits
   about 10 s after start. Also, GCREX-65 keeps the canonical `gc:<city>`
   session. So the flag never fires in a healthy city. The lifecycle is
   supervision, plus SIGTERM (clean session save) or `rex -S <addr> server
   stop`.

## Method

Rex `0.1.0-preview.21`, API revision 18, macOS. Every experiment ran against a
throwaway state dir under `/tmp`. The user's server (pid 39842,
`unix:///Users/boris/Library/Application%20Support/rex/server.sock`) ran the
whole time. It was never addressed, except for read-only comparisons. Nothing
in this note touches a real city.

## Experiments

### 1. Foreground start with its own state dir and socket

```sh
rex server run --state-dir /tmp/gcrex30/state \
  --listen unix:///tmp/gcrex30/s.sock --log-format text
```

Observed: `Rex server ready ... exit_empty=false`. The socket appeared as
`srw------- boris wheel`. `rex -S unix:///tmp/gcrex30/s.sock server status`
reported `listener: unix (unix, peer_credentials, primary)` and
`you: uid 501 over unix`. State lives under
`<state-dir>/servers/<hash>/snapshot/{snapshot.db,blocks/...}`. It is owner-only
(`drwx------`), with `meta.clean_shutdown` and a per-block `state_gen`.

### 2. SIGTERM

`kill -TERM` on the server pid. The log showed these lines in order:

```
DEBUG closing sessions ... count=1
INFO  Rex server stopped
```

The process exits 0. The snapshot is written. The socket file is removed.
SIGINT behaves the same way. The CLI advertises that Ctrl+C, SIGTERM and
`rex server stop` save sessions before they stop. `rex -S <addr> server stop`
does the same and prints `Rex server stopped.` It exits 0 and removes the
socket.

### 3. SIGHUP

```
WARN ignoring SIGHUP ... reason="this server has no terminal to hang up and no
log file to reopen" hint="send SIGTERM to stop it"
```

The server keeps running. SIGHUP is not a stop signal for a supervised child.

### 4. Crash: `kill -9`

Observed after the kill:

- The socket file **stays** on disk (`srw-------`). The next `rex -S` gets a
  connection error, not "no server".
- The snapshot keeps `meta.clean_shutdown = 0`. `run_count` increments on the
  next start.
- Blocks with no written state generation are **lost**. A block created with
  `rex run` just before the kill was absent after restart. The error was
  `block target ... not found`. Blocks with a snapshot come back.
- The last second of terminal state is lost. `state_gen` and `state_written_at`
  do not advance for a write made after the last flush.

So a crash is recoverable but not transparent. It leaves a stale socket. The
provider must treat that socket as "dead", not "in use".

### 5. Restore semantics

Restart with restore (the default):

```
INFO restored saved sessions ... sessions=1 skipped_sessions=0 ... clean_shutdown=false
INFO starting restored blocks ... blocks=2
INFO started restored blocks ... duration=13.7ms
INFO terminal: not replaying creation options on restore dropped=[command]
```

- Session id and label come back identical (`session:...`, `gc-probe`).
- Block ids come back identical. Each block is a **new shell**. The `command`
  creation option is dropped. The previous process is not resumed. The
  captured scrollback is empty.
- So a pane that gc recorded as dead comes back as a live, empty shell. It
  keeps the same block id. A reconciler that lists panes sees a stranger it
  did not start.

Restart with `--restore=false`:

```
{"sessions": []}
```

The snapshot files do not change. Only the `terminal/.locks` entries differ. So
the data is retained for a later restore, but nothing is resurrected. This is
the setting that matches gc's model.

### 6. `--exit-empty`

Start with no sessions:

```
20:08:28 INFO Rex server ready ... exit_empty=true
20:08:38 INFO Rex server stopped
```

The server exits about 10 s after start with zero sessions. It does not idle.
With one session, killing the session also stops the server after the same
grace, with exit 0. Closing the last block of a session does **not** stop it.
The session `empty` was still listed after 14 s and the server was alive.

### 7. Two servers, and three at once

Two city servers plus the user's server ran at the same time:

```
city A sessions:      ["gc-probe"]
city B sessions:      ["city-b"]
default server:       ["rex-gas-city","gc:my-city"]
```

Sockets are independent. State dirs are independent. Session labels do not
collide. A second server on an **in-use socket** fails cleanly:

```
Error: server endpoint is already in use at unix:///tmp/gcrex30v/s.sock
```

### 8. Addressing: `-S`, `--autostart`, `REX_SERVER`

- `-S/--server` is a global flag. It accepts a socket address or a host label.
- `--autostart` defaults to true. It starts only the **personal** server. With
  a city socket that is absent, the CLI says so and starts nothing:
  `Rex starts a server on its own only for your personal socket, below
  /Users/boris/Library/Application%20Support/rex`.
- Panes inherit `REX_SERVER` from the server process environment. They then
  carry the city address. But it is one process-wide value. A single process
  that touches two servers cannot express both. The provider must pass `-S` on
  every invocation.

### 9. Human attach and the client registry

- The CLI can register a city socket as a named host:
  `rex hosts add my-city unix:///tmp/.../s.sock`. Then `rex hosts check` reports
  `status: reachable`. Then `rex -S my-city attach` works, as does any other
  command. `rex hosts remove my-city` cleans up. The host entry is per-user and
  syncs with the user's home.
- Clients are registered **per server**. `rex -S <city> client ls` lists only
  clients connected to that server. The idle list contains one short-lived
  `cli` client per invocation. A long-running viewer (the app or `rex attach`)
  stays connected. So it stays listed.

## How the provider addresses a city-owned server

One seam, one field. Today the CLI client runs with no server argument. It
follows the ambient default. `internal/rex/cli/cli.go:39` runs
`exec.CommandContext(ctx, c.bin(), argv...)`. `Client` is just
`{Bin string}` at `internal/rex/cli/cli.go:22-27`. There is no `-S` handling
and no `REX_SERVER` handling in `internal/rex` or `internal/provider`.

Minimal change, if adopted:

1. Add `Server string` to `internal/rex/cli.Client`. Prefix `-S <server>` to
   `argv` in `run` (`internal/rex/cli/cli.go:39`). Every operation goes through
   `run`, so nothing else changes.
2. Resolve the value once per process from configuration (city config or
   `GC_REX_SERVER`). Construct the client in `cmd/gc-runtime-rex`'s `deps`
   instead of calling `cli.New()` bare. `provider.New(city, client)` already
   takes the client, so provider logic is untouched.
3. Panes inherit the server environment. `internal/provider/provider.go:599`
   merges the plan env. So `REX_SERVER` reaches agent processes from the
   server. The provider must still pass `-S`. It cannot rely on panes'
   environment for its own calls. Also, one process may serve more than one
   city.
4. `is-attached` needs no new argument. It is derived from `session.view` plus
   `session.list_clients` (`internal/provider/attach.go:23-90`). Whichever
   server the client addresses answers both. A viewer on the city server is
   visible. A viewer on the personal server is not, and must not be counted.

## Attach

What a human runs today, with a city-owned server:

```sh
rex hosts add my-city unix:///run/gc/my-city/rex.sock   # once per user
rex -S my-city attach
```

The Rex app is the open question. The app connects to the personal server. The
Rex documentation says not to set `REX_SERVER` for the app. So the app cannot be
pointed at a city server as far as these experiments go. This claim is
**unverified**. No app-side test was possible here. So the `attach` hint
(`cmd/gc-runtime-rex/main.go:482-490`, "open the Rex app and select the session
...") is **wrong** for a city-owned server. The app will not show that session.
The hint must name the address or the registered host label instead.

`attach` itself stays a hint. Rex still has no in-terminal attach. What changes
is the text. The text is now potentially correct for a terminal client.
`is-attached` is unaffected in code. Its meaning changes. A viewer is attached
only if the viewer is on the same server.

## Failure modes

| Failure | Observed behaviour | Consequence for gc |
|---|---|---|
| Server crash (`kill -9`) with live panes | Socket file stays. Snapshot keeps `clean_shutdown=0`. Blocks without a written generation are lost. The last state flush is lost. | Treat a connect error as dead, not as "in use". Remove the stale socket before restart. Expect lost panes. |
| City restart with `--restore=true` | Same session id and block ids return as fresh empty shells. `command` is dropped. | The reconciler sees live panes it did not start. Stale panes reappear. Do not use restore for a city server. |
| City restart with `--restore=false` | Nothing is returned. The snapshot does not change. | Matches gc. `Start` starts every pane again. |
| Two cities on one host | Independent state dirs, sockets and sessions. The same socket fails with `server endpoint is already in use`. | Fine, if the socket path is per city and the stale-socket case is distinct. |
| Socket permissions | `srw-------` (0600), `peer_credentials`, principal `uid 501`. | Only the owning user can connect. Another user's city must run its own server under its own uid. |
| Remote or tailnet | `--listen http://...` is refused without `--allow-anonymous`. That flag admits every caller with full access. `--extra-listen tailnet://:PORT` and `--tailscale` are the identified options. | v1 is local only, as D4 says. Do not expose TCP. |
| Provider restart while the city server runs | The client reconnects. Sessions and panes survive. | `Start` must find the existing session and pane, not create a second. That is already true today. |
| `--exit-empty` with the canonical session | It never fires while `gc:<city>` exists. | Not a lifecycle. Do not use it as one. |

## Recommendation on D3

**Revise D3, in stages. Keep the shared server as the default.**

- Stage 1 (this note only): record that a city-owned server is *possible*. The
  earlier "impossible" claim was about the old single-server Rex.
- Stage 2 (implemented, `gcrex-d3m0i`): an opt-in city server. City
  supervision starts it with
  `rex server run --state-dir <city> --listen unix://<city socket> --restore=false`.
  The provider addresses it with `-S`. No op-path change is needed to start it.
  The opt-in is `GC_REX_SERVER=<addr>`. It works as a socket path or a
  `rex hosts` label. `cli.Client` sends it as `-S` on every call. Unset keeps
  the user's shared server.
- Stage 3: `server-stop` behind an ownership record (state dir, instance id,
  pid). The provider stops only what it owns. With no record it keeps today's
  exit 2.

Affected tasks:

| Task | Effect |
|---|---|
| server-stop (GCREX-1, parity `### server-stop`) | Filed as `gcrex-ijj2z`. It becomes implementable only with an ownership record. Keep the shared-server refusal. |
| watch-startup (GCREX-5) | A supervised city server adds start latency and a readiness wait. The op must tolerate both. The server bring-up itself is `gcrex-d3m0i`. |
| GCREX-20 attach | Filed as `gcrex-brazb`. The hint must name `-S <addr>` or a host label, not the app. |
| GCREX-30 probes (this task) | Records the measured behaviour above. No code. |
| herdr-extension design (`docs/plans/native-rex-provider.md`) | herdr's per-city core is now achievable in Rex. The parity rationale in `docs/provider-herdr-parity.md` `### server-stop` ("Rex's is not scoped") is no longer the whole story. |
| GCREX-65 session lifecycle | `--exit-empty` and session destroy must be decided together. Do not use exit-empty while the canonical session is permanent. |

## Unverified

- Whether the Rex app can be pointed at a non-default server. The CLI path
  (`rex hosts` plus `-S`) works. The app path was not testable here.
- Behaviour on a socket whose owning process died on a different boot. This
  covers stale-socket handling across reboots.
- Whether restore can skip blocks whose snapshot generation is stale. No such
  flag was found in `rex server run --help`.
