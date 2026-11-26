# 0005. Route provider hooks through the Runner seam

- Status: Accepted
- Date: 2026-11-26

## Context

`pre_start` and `session_setup` hooks were the same mechanism written twice. At
commit `dc97cdb`, `runPreStart` (`internal/provider/provider.go`) owned the
shell call, the combined output buffer and the process group for a hook.
`runSessionSetup` (`internal/provider/provider.go`) owned a second budget loop.
Its local `runHook` computed `min(preStartHookTimeout, remaining budget)`.

`Runner` covered only the claim probe, the drain diagnostics and the host
process table. Hooks did not use the injected seam. A test could not observe a
hook without a real process.

## Decision

We add one method `RunHook(ctx context.Context, call HookCall) error` to the
existing `Runner` interface (`internal/provider/runner.go:27-30`).
`ExecRunner.RunHook` (`internal/provider/runner.go:59`) runs `/bin/sh -c` in its
own process group and owns the combined output. One budget loop, `runHooks`
(`internal/provider/hooks.go:35`), runs each command under the smaller of the
per-hook timeout and the remaining start budget. `hookBatch`
(`internal/provider/hooks.go:63`) builds the batch. `runPreStartHooks`
(`internal/provider/hooks.go:81`) and `runSessionSetup`
(`internal/provider/provider.go:1125`) call the loop, so both hook sites share
one rule.

## Consequences

- `FakeRunner` grows `RunHook` (`internal/provider/runner.go:172`). Tests inject
  a fake hook and do not install PATH shims.
- `ExecRunner` owns the hook process group and the combined output
  (`internal/provider/runner.go:59-121`). A cancel kills the group.
- The per-hook deadline is one rule in one place. A budget or timeout change
  reaches both hook sites.
- The provider no longer imports `os/exec` or `syscall` for hooks. The hook
  call carries the command, the working directory, the full environment and the
  deadline.
- `preStartCwd` still chooses the hook directory. A `work_dir` that does not
  exist yet falls back to `/`.

## Alternatives rejected

- A hook-only interface with a second provider option (`WithHookRunner`). The
  runner seam already exists and `WithRunner` already injects it. A second
  interface adds wiring and a second fake for no gain.
- A package-level function variable for the hook exec. It is hidden global
  state, tests race on it, and `WithRunner` is already the injection point.
- A shared budget loop that keeps `runPreStart`. Two sites would still own the
  budget rule, so the copy that caused the drift stays.
