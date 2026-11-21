# Remote rigs and remote Rex sessions

Question raised 2026-11-21: if every rig maps to one Rex session, can a rig
start on a remote machine?

Verdict: no rig is remote today. Rex can carry a remote session, but the
provider and gc both assume the session host is the local host.

This note records the check, with sources. Claims are labelled
`unverified` when neither source settles it.

## What Rex already provides

- `rex -S SERVER` selects a server: a host label or an address
  (`rex --help`, global options). Every subcommand takes it.
- `rex server tailscale on` serves a server over the tailnet
  (`rex server tailscale --help`).
- `rex forward` forwards TCP ports, for reaching services on the other side.
- A session belongs to the server, so a block created with `-S box` runs its
  command on `box`.

Consequence: one session per rig is a natural mapping. Session = rig, blocks =
its agents.

## What the provider would need

- `internal/rex/cli/cli.go:21-31`: `Client` drives the server through the `rex`
  CLI, one short-lived process per operation, and `Bin` defaults to `"rex"`.
  A `Server` field prepended as `-S <server>` to `argv` in `run` would target a
  remote server. Contained change.
- Zero-code variant: a PATH wrapper named `gc-runtime-rex` that adds
  `-S "$REX_SERVER"`. Agent config accepts `env` key/value pairs
  (gc `internal/config/config.go:720`). `unverified`: that gc passes that env
  to the runtime provider process, not only to the agent command.

## What breaks today

1. `internal/provider/reap.go:100` runs local
   `ps -axo pid=,ppid=,stat=,comm=,lstart=`. Pids and `ps` are per host. For a
   remote session the local table is the wrong table. Survivor identity and
   descendant expansion in stop and kill-object depend on that table.
2. The rig directory. It must exist at the same path on the remote host, and
   the git worktrees are local clones.
3. The claim probe and the drain diagnostic run through the local `Runner`
   (`internal/provider/runner.go`). They would read the wrong filesystem.
4. gc has no host dimension on a rig. `gc rig add` takes a path only
   (`cmd/gc/cmd_rig.go:68-75`). Multi-host in gc is a remote city
   (`gc context add`, `--city-url`) or a remote session runtime.

## How gc does multi-host today

gc remote-ises the session runtime and the city, never the rig.

- Session providers: `internal/runtime/exec/`, `internal/runtime/tmux/`,
  `internal/runtime/ssh/` (tmux over ssh), `internal/runtime/k8s/` (pods), and
  pack runtimes such as this one.
- k8s: `GC_SESSION=k8s` selects `internal/runtime/k8s/exec.go`, which creates
  pods through client-go and runs sessions inside them with `remotecommand`.
  The controller runs in a pod:
  `contrib/session-scripts/gc-controller-k8s deploy <city-path>` runs
  `gc start --foreground` in one fixed-name pod. It deletes the old pod first,
  because `RollingUpdate` would run two controllers against one city.
- The agent image is built separately (`internal/buildimage`,
  `contrib/k8s/Dockerfile.controller`).
- Remote clients use the HTTP+SSE control plane: `gc context add`,
  `rig add --git-url` (the server clones), then `sling`. Writes use signed
  grants (`gc-write-mint`). The read plane is unauthenticated, so a TLS or
  private-network front is a must.

The rig directory lives on the controller's volume. So "a rig on another
machine" means the whole city is on that machine.

## The pattern a remote Rex provider must copy

`internal/runtime/ssh/provider.go:690-710`: `processAlive` runs `ps` **inside
the target box** through `p.conn.Exec` (`processAliveScript`), and degrades to
tmux pane liveness when the snapshot fails.

A remote Rex provider needs the same split. Ask the Rex host for its process
table. Do not call local `ps` from `internal/provider/reap.go`.

## Shapes

- **A. City on the box (supported).** k8s pod, ssh host, or a remote city.
  Rigs are local there, Rex is local there. If the goal is "my rigs run
  elsewhere", this works now.
- **B. Per-rig remote Rex session (not supported).** Needs three things.
  Server targeting in the provider. Host-side process inspection. A
  rig-to-host/path mapping in gc.

## Cheapest experiment

One agent on `-S box` with read-only operations (`start`, `observe`, `peek`).
Leave `stop` out until process inspection is host-side.

## Open question

Should gc grow a host dimension on rigs? The alternative is simpler. Each
remote box runs its own city, and clients stay thin over the control plane.
