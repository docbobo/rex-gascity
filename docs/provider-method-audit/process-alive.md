Verdict: deviations

## Deviations

| Doc claim | Current implementation | Behavioral impact | Severity |
|---|---|---|---|
| `default` 30 s deadline and `GC_EXEC_DEADLINE_MS` override polled on stdin and checked pre-handler (`## Shared RPP contract` §Deadlines and cancellation; `## process-alive` §Registration) | `main` uses `signal.NotifyContext(context.Background(), ...)` with no timeout and ignores `GC_EXEC_DEADLINE_MS` (`cmd/gc-runtime-rex/main.go:50`); `opProcessAlive` calls `io.ReadAll(stdin)` without deadline polling (`cmd/gc-runtime-rex/main.go:282`) | Hung stdin or Rex calls rely on gc's 30 s process timeout instead of self-bounding; `GC_EXEC_DEADLINE_MS` has no effect (unused in gc v1.5.1) | Low |
| `find_agent` walks the full box `subtree` (`## process-alive` §Success output, §Conflicts) | `processAlive` checks only `info.Foreground` and `info.Ancestors` (`internal/provider/liveness.go:81-109`); `rex.Process` has no zombie state (`internal/rex/client.go:126-134`) | A non-foreground/non-ancestor child process is not seen and reports `false` (dead); the interpreter-script part is resolved by the `Script` match (GCREX-47) | Low |
| Core unreachable with nothing running returns exit 0 `false\n` (`## process-alive` §Success output, §Failure cases) | `p.binder.Resolve` propagates Rex connection errors (`internal/provider/binding.go:349-360,440-448`) and `opProcessAlive` exits 1 with empty stdout (`internal/provider/liveness.go:155-159`, `cmd/gc-runtime-rex/main.go:175-179`) | Rex down produces exit 1 + stderr instead of exit 0 `false\n`; gc still treats exit 1 as `false` (dead) | Low |
| Stdin decoded with UTF-8 `surrogateescape` and split with `splitlines()` (`## process-alive` §Inputs, §Failure cases) | `opProcessAlive` splits on `"\n"` only (`cmd/gc-runtime-rex/main.go:351`) and `expandProcessNames` trims whitespace (`internal/provider/liveness.go:39`) | `\r\n` works, but bare `\r` or Unicode line separators (e.g. U+2028) are not split; only affects direct callers since gc sends `\n` | Low |

## Matches

- **Registration & args**: Registered in `ops` as `process-alive <name>` requiring 1 arg and reading stdin (`cmd/gc-runtime-rex/main.go:114,346-368`), matching `## process-alive` §Registration & §Inputs.
- **Missing or empty `<name>`**: Rejected before provider setup with exit 1, empty stdout, and prefixed stderr (`cmd/gc-runtime-rex/main.go:157-160`), matching `## process-alive` §Inputs & §Failure cases.
- **Empty or all-blank stdin**: `strings.Split` + trailing-empty trim (`cmd/gc-runtime-rex/main.go:351-354`) and `expandProcessNames` trim/skip (`internal/provider/liveness.go:39-44`) yield empty `names`, returning `true\n` when `LastExit == nil` (`internal/provider/liveness.go:82-85`), matching `## process-alive` §Success output.
- **Cannot-tell answer**: an unreadable process call (any non-`ErrNotFound` `BlockProcess` error, `internal/provider/liveness.go:172-176`), an empty process table, or an opaque process (no `name`/`path`/`argv0`/`script`) with no match returns exit 0 with empty stdout, which gc reads as alive (`internal/provider/liveness.go:81-109,165-188`; `cmd/gc-runtime-rex/main.go:355-368`), matching `## process-alive` §Success output and §Failure cases. GCREX-46.
- **Interpreter scripts**: Rex reports the interpreter's `script` path; `processMatches` matches its basename with and without the extension (`internal/provider/liveness.go:120-147`; `internal/rex/client.go:126-134`), so `node /path/to/claude` and `node agent.js` match `claude` and `agent`, matching `## process-alive` §Success output's node/bun/npx/deno rule. GCREX-47.
- **Name aliases & `gc_bin` expansion**: `expandProcessNames` expands `kimi` to `kimi-code` and `gc` to `filepath.Base(GCBin)` with `GC_BIN` env fallback (`internal/provider/liveness.go:14,32-59,181`), matching `## process-alive` §Success output.
- **Missing or vanished pane**: Unresolved session or `rex.ErrNotFound` from `BlockProcess` returns `false, nil` -> exit 0 `false\n` (`internal/provider/liveness.go:161-164`, `cmd/gc-runtime-rex/main.go:362-363`), matching `## process-alive` §Success output.
- **Cancellation**: `main` catches `SIGTERM`/`SIGINT` via `signal.NotifyContext` (`cmd/gc-runtime-rex/main.go:50`), aborting in-flight calls and exiting 1 with empty stdout (`cmd/gc-runtime-rex/main.go:175-179`), matching `## Shared RPP contract` §Deadlines and cancellation.

## Unverified

- Runtime behavior of `rex block.method process` (`internal/rex/cli/cli.go:291-306`), specifically whether the live Rex daemon ever omits processes from `Foreground`/`Ancestors` or reports empty process metadata.
- Whether any gc release newer than pinned `v1.5.1` sends `GC_EXEC_DEADLINE_MS` (absent in `v1.5.1` per `## Shared RPP contract` §Unverified).
