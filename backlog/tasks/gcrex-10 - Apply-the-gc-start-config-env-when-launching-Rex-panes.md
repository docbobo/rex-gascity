---
id: GCREX-10
title: Apply the gc start-config env when launching Rex panes
status: Done
assignee:
  - '@pi'
created_date: '2026-11-17T21:23:00Z'
updated_date: '2026-11-17T21:43:00Z'
labels: []
dependencies: []
references:
  - docs/REX-CONTRACT.md
  - cmd/gc-runtime-rex/main.go
  - internal/provider/provider.go
  - internal/rex/client.go
priority: high
type: bug
ordinal: 10000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
The RPP dispatcher ignores the `env` field of the gc start config, so managed-session variables (GC_CITY, GC_SESSION_ID, GC_SESSION_NAME, GC_TEMPLATE, GC_AGENT, GC_ALIAS, GC_RIG, GC_BIN, and the rest of the Step 8 agent env) never reach a Rex pane.

Symptom: in a city with `[session] provider = "rex"`, the `my-project/core.control-dispatcher` role failed on start:
- `mkdir: cannot create directory /.gc: Read-only file system` - its start_command exports GC_WORKFLOW_TRACE from `${GC_CITY}/.gc/runtime/...`, and GC_CITY was empty so the path collapsed to `/.gc/...`.
- `gc convoy control --serve: control dispatcher follow mode requires managed session env (GC_SESSION_ID, GC_SESSION_NAME not set)` - from requireWorkflowServeFollowSessionEnv in gc.

gc builds the per-session env and ships it in the RPP `start` config (execStartConfig.Env, forwarded by internal/runtime/exec). The provider drops it: startConfig in cmd/gc-runtime-rex/main.go decodes only work_dir, command and ready_prompt_prefix; provider.Config and rex.LaunchSpec carry no Env; blockLayout emits only command/shell/cwd/login/interactive, and Rex terminal launch options take no env. So env must be applied at the command boundary (for example by wrapping the launch argv with `env K=V ...`).

This is the env portion of the open start-config hooks item already recorded under GCREX-5: `docs/REX-CONTRACT.md` - "Start-config hooks are not applied (open, GCREX-5)". The control-dispatcher is simply the first role that hard-depends on those variables, so it is not control-dispatcher-specific.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 The RPP start config env map is decoded and carried into the provider launch
- [x] #2 A launched Rex pane sees every variable from the start config env, verified by a pane that prints its environment
- [x] #3 The control-dispatcher start_command reaches `gc convoy control --serve --follow` without the missing-managed-session-env error
- [x] #4 GC_CITY is non-empty in the pane, so the control-dispatcher trace mkdir targets the city .gc/runtime path instead of /.gc
- [x] #5 Tests using the fake rex.Client assert the launched command carries the configured env; `go test ./...` and `golangci-lint run ./...` pass
- [x] #6 docs/REX-CONTRACT.md (and the provider reference) are updated to close the env portion of the GCREX-5 open item
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. Decode the RPP start config env map (json "env") in cmd/gc-runtime-rex/main.go and carry it through provider.Config.Env.
2. In provider.start, wrap the launch argv with the env applied at the command boundary (env K=V ... argv, sorted keys) when both a command and env are present; leave argv untouched when no env is supplied.
3. Add a provider test using faketest.Fake asserting the launched command carries the configured env, plus a dispatcher test that the start JSON env reaches the launched command.
4. Extend the opt-in live REX_E2E test to launch a pane that prints its environment and assert a sentinel variable from the start config is visible.
5. Update docs/REX-CONTRACT.md section 12 and docs/rex-provider.md Known v1 Limits to close the env portion of the GCREX-5 open item.
6. Run go test ./... and golangci-lint run ./... .
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Implementation (in progress):
- cmd/gc-runtime-rex/main.go: startConfig decodes env map[string]string (json "env"); opStart passes provider.Config.Env.
- internal/provider/provider.go: Config.Env added; start() builds the launch argv via launchCommand(argv, env), which prefixes 'env K=V ...' with sorted keys when both argv and env are non-empty, leaves argv unchanged otherwise, and never mutates the caller slice. Applied on both the NewWindow and NewSplit paths (shared spec).
- internal/provider/filestore.go: two pre-existing errcheck findings (unchecked os.Remove on cleanup paths) marked with '_ =' so golangci-lint run ./... is clean; no behavior change.
- Tests: provider unit tests (TestStartAppliesEnvToLaunchCommand, TestStartWithoutEnvLeavesLaunchCommand) via faketest.Fake; dispatcher test TestStartAppliesEnvFromStartConfig; opt-in live test seeds RPP_E2E_ENV and asserts it in the pane's printed environment. Live test prints env before the sentinel and peeks 200 lines so the ~40-line env does not push the sentinel out of the 50-line window.
- Docs: docs/REX-CONTRACT.md section 12 and docs/rex-provider.md Known v1 Limits record the env portion of GCREX-5 as resolved (GCREX-10); remaining hooks stay open.

Final verification evidence:
- Unit: go build ./... , go test ./..., golangci-lint run ./... all clean. TestStartAppliesEnvToLaunchCommand locks the env-wrapped argv (["env","A=1","B=x y=z'q","GC_CITY=/tmp/c","sleep","300"]) and asserts no caller mutation; TestStartWithoutEnvLeavesLaunchCommand; dispatcher TestStartAppliesEnvFromStartConfig (start JSON env -> launched command).
- Live REX_E2E=1 go test ./cmd/gc-runtime-rex -run TestE2EDispatcherLifecycle: PASS; the pane printed the seeded RPP_E2E_ENV value (AC2). Switched the probe to printenv so a failure log no longer dumps the whole environment.
- Live scratch probe with the core pack control-dispatcher start_command and env {GC_CITY,GC_SESSION_ID,GC_SESSION_NAME}: output was the missing-builtin-pack warning and 'agent "my-project/core.control-dispatcher" not found in config' -- no missing-managed-session-env error, so the serve-follow env gate passed (AC3). The trace dir was created at $GC_CITY/.gc/runtime and /.gc was not created (AC4). The earlier mechanism probe also printed CITY=[<tmp>], MANAGED_ENV_OK, MKDIR_OK.
- Independent review (gcrex10-env-review) verdict APPROVE WITH NITS, 0 blockers/major; its docs/test nits were applied. DISCLOSURE: both subagents ran on deepseek/deepseek-v4.1-flash despite a requested model, so the review was context-isolated but same-family, not cross-family independent.
- Known limits kept honest in docs: a bare shell launch (empty command) still cannot be seeded because Rex has no per-pane env option; the wrapper relies on 'env' resolving via PATH.
- Incidental: internal/provider/filestore.go errcheck findings (pre-existing at HEAD) marked '_ =' so the mandated lint check passes; no behavior change.
<!-- SECTION:NOTES:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
Apply the gc start-config env when launching Rex panes.

What changed
- cmd/gc-runtime-rex/main.go: the RPP start config now decodes env (map[string]string, json "env"); opStart carries it in provider.Config.Env.
- internal/provider/provider.go: Config.Env added; start() builds the launch argv via launchCommand(argv, env), which prefixes 'env K=V ...' with sorted keys when both a command and env are supplied, leaves argv untouched otherwise, and never mutates the caller slice. The same spec serves the new-window and new-split paths.
- internal/provider/filestore.go: two pre-existing errcheck findings on cleanup-path os.Remove calls marked '_ =' so golangci-lint run ./... is clean; no behavior change.
- Tests: provider unit tests via the fake rex.Client (env-wrapped argv incl. a value with spaces/=/quote, and the no-env case), a dispatcher test that the start JSON env reaches the launched command, and the opt-in live test seeds RPP_E2E_ENV and reads it back with printenv.
- Docs: docs/REX-CONTRACT.md section 12 and docs/rex-provider.md Known v1 Limits record the env portion of the GCREX-5 open item as resolved (GCREX-10); the remaining start-config hooks stay open.

Why
gc ships managed-session variables (GC_CITY, GC_SESSION_ID, GC_SESSION_NAME, GC_TEMPLATE, ...) in the RPP start config, and the provider dropped them, so the control-dispatcher ran with an empty GC_CITY and failed at 'mkdir: /.gc: Read-only file system' and the serve-follow managed-session-env gate. Rex launch options carry no env, so the variables are applied at the command boundary.

Verification
- go build ./... , go test ./..., golangci-lint run ./...: clean.
- Live REX_E2E=1 go test ./cmd/gc-runtime-rex -run TestE2EDispatcherLifecycle: PASS; the pane's environment contained the seeded RPP_E2E_ENV value (AC2).
- Live scratch probe of the core pack control-dispatcher start_command with GC_CITY/GC_SESSION_ID/GC_SESSION_NAME: it reached 'gc convoy control --serve --follow' with no missing-managed-session-env error (AC3) and created its trace dir under $GC_CITY/.gc/runtime, not /.gc (AC4).
- Independent review: APPROVE WITH NITS, 0 blockers/major; nits applied. Disclosure: the review ran on the same model family (deepseek) because the requested cross-family model was not honored, so it is context-isolated, not cross-family independent.

Risks / follow-ups
- A bare-shell launch (empty command) cannot be seeded; the provider has no way to inject env into a Rex default shell. Documented.
- The wrapper relies on 'env' resolving via the pane PATH.
- The literal control-dispatcher run used a scratch city (its my-project rig was absent), so it verified the env gate, not a full production dispatch.
<!-- SECTION:FINAL_SUMMARY:END -->
