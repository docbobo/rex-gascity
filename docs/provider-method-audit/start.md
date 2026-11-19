Verdict: `deviations`

## Deviations

| Doc claim | Current implementation | Behavioral impact | Severity |
|---|---|---|---|
| `startConfig` parses 17 fields including `lifecycle`, `process_names`, `ready_prompt_prefix`, `ready_delay_ms`, `session_setup`, `session_setup_script`, `session_live`, `overlay_dir`, `copy_files`, `fresh_only`, `work_dir_exclude` (`## start` > Inputs) | `cmd/gc-runtime-rex/main.go:71-79` parses `work_dir`, `command`, `env`, `pre_start`, `nudge`, `lifecycle`, `pack_overlay_dirs`; `lifecycle` only distinguishes `one_shot` (the liveness failure gate), all other fields are ignored | `session_setup*`, `overlay_dir`, `copy_files` never run/stage; `session_live` emits no warning; `fresh_only` never exits 73; `ready_prompt_prefix`/`ready_delay_ms` ignored | medium |
| Empty/missing `work_dir` with staging fields fails (`## start` > Failure cases) | `internal/provider/overlay.go:18-20` returns `nil` when `workDir == ""` or `GC_PROVIDER == ""` | Staging is silently skipped instead of failing | low |
| Staging runs before `pre_start` (`## start` > Inputs, Success order) | `internal/provider/provider.go:334-365` runs `pre_start`, checks `work_dir`, then calls `stagePackOverlays` | `pre_start` sees un-staged `work_dir`; pack overrides win over `pre_start` | low |
| `pre_start` is fatal (pack) vs non-fatal (gc v1.5.1) (`## start` > Conflicts #1) | `internal/provider/provider.go:349-351` aborts `start` on `pre_start` failure; hook timeout is `min(30s, remaining)` of 120 s budget with no 5 s reserve (`provider.go:336-347`) | Matches herdr-core (conflicts with gc v1.5.1); slightly looser timeout near 120 s | low |
| `GC_EXEC_DEADLINE_MS`, 120 s op deadline, and stdin-open deadline poll (`## Shared RPP contract` > Deadlines) | `cmd/gc-runtime-rex/main.go:45,196` uses `context.Background()` + `io.ReadAll(stdin)`; `provider.go:177-181` notes `GC_EXEC_DEADLINE_MS` is unread | No provider-side 120 s cap on Rex calls or stdin read; relies on gc's SIGTERM | low |
| `GC_EXEC_CITY_PATH` and `GC_RPP_FAULT` read from env (`## start` > Inputs) | `internal/provider/namespace.go:42-50` reads `GC_REX_CITY`/`GC_CITY_PATH`/`GC_CITY`; `GC_RPP_FAULT` is unread | Fault-injection env var has no effect; city key uses `CityFromEnv` | low |
| Redacted stderr; exit-0 warnings on stderr (`## Shared RPP contract` > Failure envelope, `## start` > Success output) | `cmd/gc-runtime-rex/main.go:164-166` prints raw `err` to stderr without redaction; `opStart` (`main.go:206-220`) writes no exit-0 warnings | Secret values in errors are not redacted; undelivered nudge writes only meta (`GC_STARTUP_DELIVERY_UNCONFIRMED`), no stderr warning | low |

## Matches

- **Registration & args**: `start` registered with 1 required non-empty `<name>` arg (`cmd/gc-runtime-rex/main.go:96,147-150`); extra args ignored (`main.go:195`).
- **Success envelope**: Exits 0 with empty stdout (`cmd/gc-runtime-rex/main.go:194,209-221`, `## start` > Success output).
- **Empty stdin**: Empty or whitespace-only stdin fails with `parse start config: empty stdin`, exit 1, empty stdout and no pane launched (`cmd/gc-runtime-rex/main.go:230-243`, `TestStartEmptyStdinFails`; `## start` > Failure cases).
- **Collision & recycle**: `StartNew` (`internal/provider/provider.go:267-281`) serializes under `withNameLock`, checks `IsRunning`, returns `ErrAlreadyRunning` -> `"session %q already running"` (`main.go:216-219`, matches gc's collision detector), and recycles dead panes (`provider.go:303-312`).
- **Agent `env` unset semantics**: `launch.Command` (`internal/launch/launch.go:115-128`) emits `env -u K` for `""` values when `command` is non-empty.
- **`pre_start` env unset semantics**: an empty `env` value removes the key from the hook environment instead of setting `K=` (`launch.mergeEnv`, `internal/launch/launch.go:233-252`, via `Plan.MergedEnv` at `internal/provider/provider.go:409`; `TestMergeEnvEmptyValueUnsets`, `TestPreStartEnvEmptyValueIsUnset`), and a non-empty value is passed through unchanged (`TestMergeEnvNonEmptyValuePassedThrough`, `TestPreStartEnv`). Matches gc's "empty = unset" env contract (`docs/reference/exec-session-provider.md:269` at v1.5.1) and herdr-core's setup-hook env (`herdrcore/envplan.py:67-71`, `herdrcore/lifecycle.py:599-609`). A bare shell still starts without `env` (an intentional limitation; parity row 9).
- **Directory & overlay safety**: Checks `work_dir` is an existing directory (`internal/provider/provider.go:356-358`) and refuses symlinks/non-regular files during `pack_overlay_dirs` staging (`internal/provider/overlay.go:53-58`).
- **Seeded identity & rollback**: Seeds `startIdentityMetaKeys` and provisional binding before blocking launch (`provider.go:369-378`) and clears them on error/SIGTERM rollback (`provider.go:289-297`, `main.go:45`).
- **First-turn nudge**: `deliverStartupNudge` (`internal/provider/nudge_delivery.go:94-173`) waits for `program_status` idle, retries with `ctrl+u` clear, and marks `GC_STARTUP_DELIVERY_UNCONFIRMED` in meta when unconfirmed.
- **Post-launch liveness**: `Start`/`StartNew` call `failIfAgentEnded` (`internal/provider/provider.go:302-318`) after the launch, and `waitForStartupReady` (`nudge_delivery.go:286-336`) stops a readiness wait when the pane's program ends; the first case fails `"<name>: the agent exited during startup"`, the second `"<name>: the agent exited during startup, before its first turn"` (exit 1, `TestStartInstantExitFails`, `TestStartMissedFirstTurnFails`). `lifecycle=one_shot` skips both (`TestStartOneShotInstantExitSucceeds`, `TestStartOneShotMissedFirstTurnSucceeds`), matching herdr-core's `_fail_if_ended`/`first_turn` (`herdrcore/lifecycle.py:1027-1036,1113-1125`). The dead pane is left in place for gc's post-failure stop.

## Unverified

- Whether gc or a wrapper ever sets `GC_EXEC_DEADLINE_MS`, `GC_EXEC_CITY_PATH`, or `fresh_only` against this provider in production (absent in pinned gc v1.5.1).
- Real `rex` CLI behavior on `NewWindow`/`NewSplit` cancellation near gc's 2 s SIGTERM-to-SIGKILL grace window (`cmd/gc-runtime-rex/main.go:41-46`).
- Whether a real instant Rex exit is visible to the single immediate post-launch liveness probe (`failIfAgentEnded`), rather than only to a wait (the pack's 5 s/30 s `_await_agent`); the dispatcher tests prove the failure against the fake's `ProcessFunc`, not against a real Rex process read.
- Cleanup of detached grandchild processes spawned by `pre_start` hooks on cancellation (`internal/provider/provider.go:614-644`).
