# Rex Provider Pilot and Promotion Record (GCREX-6, Phase 5)

Date: 2026-11-17. Provider: `gc-runtime-rex` (RPP v0). gc 1.5.1.

## Outcome

The pilot ran two scratch cities under the machine-wide supervisor: a worker city on the `rex` provider and a mayor city on tmux. Three kill/restart cycles showed no wedged session. Teardown left no orphan panes. The pilot found two provider defects (unbounded rig-tab splitting and an error on a vanished pane) and fixed both. **Decision: promote `[session] provider = "rex"` as the supported opt-in backend for a city.** tmux stays the default and the fallback. Open items are listed at the end.

## Run configuration

| Item | Value |
|---|---|
| Worker city | `/tmp/gcrex-pilot`, minimal template, `[session] provider = "rex"` |
| Worker agent | `worker`, `start_command = sh -c 'while :; do echo worker-tick $(date +%s); sleep 5; done'`, `prompt_mode = "none"`; mayor suspended |
| Pack | repo pack imported as `[imports.rex]` (`file:///Volumes/Sources/rex-gas-city//pack`, locked to commit `51ed0fa`) |
| Mayor city | `/tmp/gcrex-pilot-mayor`, default (tmux) provider, `mayor` agent with a shell keepalive `start_command` |
| Mayor on tmux | own tmux server (socket `gcrex-pilot-mayor`, sessions `mayor` and `bd__dog-gpm-wisp-ic0`); never appeared in `rex session ls` |
| Supervisor | launchd service; both cities ran under it |
| Dry run | `gc start --dry-run`: `Dry-run: 1 agent(s) would start in city "gcrex-pilot"`; 1 agent suspended |
| Selector check | `/tmp/gcrex-envcheck` has no `[session]` block; a foreground `GC_SESSION=rex GC_REX_CITY=gcrex-envcheck gc supervisor run` started the worker in rex session `gc:gcrex-envcheck` |

The mayor city stands for "mayor pinned to tmux": the controller-side agent runs on tmux, independent of the rex provider.

The `GC_SESSION=rex` selector applies only to a foreground controller, so the launchd-supervised worker city used the durable `[session] provider = "rex"` form instead; the `gcrex-envcheck` run confirms the env selector picks the same backend for a city with no `[session]` config.

## Version skew

| Component | Version | API revision |
|---|---|---|
| rex CLI | `0.1.0-preview.20`, commit `e33d2481b81b202b2600a0f40bd440a5282653a9`, built 2026-11-16 | 16 (supports 0 to 16) |
| Running server | `0.1.0-preview.19`, pid 52060, started 2026-11-14T16:00:12Z | 14 |

The server revision (14) is inside the CLI's supported range (0..16), so the pilot does **not** require a restarted server. No restart was performed (D3: the server is shared and the user's own work runs on it). Evidence: `REX_E2E=1 go test ./cmd/gc-runtime-rex -run TestE2EVersionSkew -v` passed and logged `rex CLI: rex 0.1.0-preview.20; api revision 16, supports 0 to 16; server api_revision 14`.

## Work cycles, liveness, teardown

Three `gc session kill worker` cycles. Each time:

- within ~3s the session went `active` -> `asleep` with reason `runtime-missing`;
- within ~15s it was `active` again (`session,config,attached`);
- no session was wedged or stuck.

While the worker was down, its rig tab was absent (the worker pane closed and the last block closing removed the tab). The city session remained for the up phase. Final `gc status`: `worker running`, `1 running, 3 unknown of 4 agents`.

Teardown: both cities were `gc stop`-ped and unregistered, the supervisor was stopped and its launchd service uninstalled, `~/.gc/cities.toml` is back to `cities = []`, the tmux city server is gone, and `rex session ls` shows only the pre-existing sessions (Nix Config, nixpkgs, rex-gas-city). No `gc:`-labelled sessions and no orphan panes remained.

## Errors observed and dispositions

| # | Observation | Disposition |
|---|---|---|
| 1 | Worker session was `gc:default`, not `gc:gcrex-pilot`. The launchd supervisor does not carry `GC_REX_CITY`, and RPP carries no city. | **Open.** Confirms the GCREX-5 "no city" limit in a real supervised city. Follow-up: pin `GC_REX_CITY` at the supervisor level or add a supervisor-level env mechanism. |
| 2 | `gc stop` on the worker city removed its Rex session (`gc:default`) entirely. The provider never calls `session.destroy`; a separate probe showed a session whose only block was closed stays listed for at least 30s. The gc-side path that removes the session is not visible in the supervisor log. | **Accepted.** gc owns the lifecycle of its own sessions. Provider teardown stays block-only; `TestE2EIdentityActivityAttachment` asserts the session survives a provider `Stop`. |
| 3 | Server restart recovery not exercised live. | **Accepted with reason.** D3 forbids restarting the shared server. The stale-binding path (stored block id re-validated against `session.view`, re-resolved by label) is covered live by `TestE2EIdentityActivityAttachment`. |
| 4 | Rig-tab splitting always halved the first pane (see below). | **Fixed.** |
| 5 | A launched command that exits closes its Rex block (the provider sets no keep_open); closing the last block removes the tab, and `block.method process` then exits 3 (`rex.ErrNotFound`). | **Fixed.** `IsRunning` treats a vanished pane as `(false, nil)`. Covered by `TestE2EAgentExitLiveness`. |

Liveness detail: while a command runs, `process.last_exit` is null and `foreground.name` is `bash` with `argv0` `/bin/sh`, so process-name heuristics must not be used. `last_exit` stays the liveness truth. `ProcessAlive(names)` stays the strict foreground/ancestor check for the RPP op.

## Rig-tab split policy (decision)

- **Old:** always split `w.Blocks[0]` horizontally. A live probe of 14 splits gave widths 1/2, 1/4, ... 1/16384 – unusable.
- **New (shipped):** split the largest-area pane along its longer axis (horizontal when width >= height, else vertical), side `after`, ratio 50%. Panes stay balanced. If the layout has no usable geometry, fall back to the first block, horizontally.
- **No per-tab pane cap.** A live probe reached 45 panes with minimum area 1/64 and no layout limit. A balanced probe produced 17 panes: 16 at 0.25x0.25 and two at 0.125x0.25.
- **Geometry source:** `session.view` `layers[].layout` (a rect-carrying split tree), parsed into `rex.Block.Rect`.
- **Evidence:** live `TestE2ERigSplitGrid` (8 agents in one rig -> one tab, 8 blocks, minimum area >= 0.5/8) passes; unit tests `TestNextSplit` cover the selection.

## Test evidence (2026-11-17)

| Command | Result |
|---|---|
| `REX_E2E=1 go test ./internal/provider -run TestE2E -v -timeout 20m` | all PASS, `ok ... 6.955s` |
| `REX_E2E=1 go test ./cmd/gc-runtime-rex -run TestE2E -v` | `TestE2EDispatcherLifecycle`, `TestE2EVersionSkew` PASS, `ok ... 0.580s` |
| `make check` | 14 checks: 14 passed, 0 failed, 0 skipped (resolved runtime "rex" from pack "rex") |
| `make conformance` | 8 requirements: 6 passed, 0 failed, 2 skipped (exec/provision optional) |
| `go test ./...`, `go vet ./...`, `gofmt` | clean |
| `golangci-lint run ./...` | 2 pre-existing errcheck findings in `internal/provider/filestore.go`, untouched by this work |

Provider live tests: `TestE2ERigSplitGrid` 1.16s, `TestE2EAgentLifecycle` 2.17s, `TestE2EInputPath` 0.58s, `TestE2EIdentityActivityAttachment` 0.66s (`activity source=format_poll advanced=true`, `IsAttached=true appClient=true`), `TestE2EAgentExitLiveness` 1.44s, `TestE2EParallelStarts` 0.76s.

## Promotion decision

Promote `[session] provider = "rex"` as the supported opt-in backend. Basis: three clean work cycles with no wedge, no orphans after teardown, version skew shown to be within the supported range, the two provider defects fixed and covered by live tests, and the check and conformance suites green. tmux remains the default and the fallback (replacing it stays a non-goal).

## Residual open items

- **Namespace under a supervisor** (`gc:default`): pin `GC_REX_CITY` at the supervisor level or add a supervisor-level env mechanism.
- **Start-config hooks** (`env`, `nudge`, `pre_start`, `session_setup*`, `session_live`, file staging) are still not applied; see `docs/rex-provider.md` Known v1 Limits.
- **Server restart** is not exercised live (D3).
- **Session removal on `gc stop`**: the gc-side path is not identified.
- The 2 pre-existing lint findings in `internal/provider/filestore.go`.
