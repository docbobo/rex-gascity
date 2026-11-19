# Method audit: `set-meta`

Verdict: `deviations`

## Deviations

| Doc claim | Current implementation | Behavioral impact | Severity |
|---|---|---|---|
| `## set-meta` § Inputs, § Failure cases: takes an exclusive per-name cross-process `flock` (30 s wait, cut to deadline) and read-modify-writes `<state>/meta/<obj>.json` with `O_EXCL\|O_NOFOLLOW`, owner/nonce checks, and `MetaError` on corrupt files. | `internal/provider/stores.go:205-233`, `internal/provider/filestore.go:27-50,74-96`: uses one JSON file per `(name, key)` with only an in-process `sync.Mutex`, no `flock` (`lock.go` unused), no owner/symlink/nonce checks, and overwrites without reading existing state. | Concurrent `set-meta` on the same key is last-rename-wins across processes, and a `set-meta` racing `Stop`'s `clearIncarnations` (`provider.go:559,582`) can leave a key behind; corrupt existing files are overwritten rather than rejected. | low |
| `## set-meta` § Registration, § Failure cases & `## Shared RPP contract` § Deadlines and cancellation: `default` 30 s deadline (overridden by `GC_EXEC_DEADLINE_MS`), pre-handler deadline/cancel gates, and stdin-open-at-deadline timeout (`exit 1`). | `cmd/gc-runtime-rex/main.go:47,169-174,350-356`, `internal/provider/meta.go:56`: `GC_EXEC_DEADLINE_MS` is not read, no 30 s deadline is attached to `ctx`, `io.ReadAll(stdin)` blocks until EOF, and `SetMeta` ignores `ctx`. | `set-meta` does not time out after 30 s or on `GC_EXEC_DEADLINE_MS=0`, and an open `stdin` pipe is only interrupted by an external `SIGTERM`/`SIGINT` (`main.go:47`). | low |
| `## set-meta` § Inputs: stores UTF-8 `surrogateescape` JSON-escaped strings in `<state>/meta/<obj>.json`. | `internal/provider/stores.go:195-202,231-233`: stores `value_b64` (standard base64 of raw bytes) in `<state>/meta/<sha256(name)>/<sha256(name\0key)>.json`. | On-disk format differs; round-trip behavior through `get-meta` is byte-identical. | low |
| `## set-meta` § Registration & `## Shared RPP contract` § Dispatch: runs `sweep_spawn_files` housekeeping, uses `GC_EXEC_STATE_DIR`/`GC_EXEC_CITY_PATH`, and honors `GC_RPP_FAULT`. | `cmd/gc-runtime-rex/main.go:163-168`, `internal/provider/namespace.go:47-55`, `internal/provider/statedir.go:13-30`: no spawn-file sweep, no `GC_RPP_FAULT`, and state dir is resolved from `GC_REX_STATE_DIR` / `XDG_STATE_HOME` / `GC_REX_CITY`. | Fault injection and `GC_EXEC_STATE_DIR`/`GC_EXEC_CITY_PATH` overrides have no effect; state dir layout is provider-specific. | low |

## Matches

- **Precondition (`## set-meta` § Inputs, § Failure cases; GCREX-49):** `Provider.SetMeta` (`internal/provider/meta.go:56-62`) resolves the name through the cached binder (`meta.go:14-21`, `binding.go:250-266`) and requires a stored binding, so an unknown, stopped or unstarted session returns `no session` and exit 1 and stores nothing; a name with a record stores. Decided against gc v1.5.1 call sites, which all act on sessions the runtime lists and therefore keep a binding: `cmd/gc/cmd_runtime_drain.go:64,122-124,186,209` and `cmd/gc/session_wake.go:317-328` (reconciler drain), and `internal/session/lifecycle.go` `SyncRuntimeAlias`, gated on `IsRunning` (`internal/session/manager.go:1705`, `cmd/gc/session_beads.go:2176`). `remove-meta` still succeeds for an unknown session, like the pack (`reads.py:416-417`).
- **Registration & CLI validation (`## set-meta` § Registration, § Inputs):** Registered at `cmd/gc-runtime-rex/main.go:115` as `set-meta <name> <key>` (`required: 2`) and in `keyedOps` (`main.go:96`). Missing/empty `<name>` or `<key>` fails before dispatch with exit `1`, empty `stdout`, and a `stderr` diagnostic (`main.go:154-161`, `cmd/gc-runtime-rex/dispatch_test.go:129-152`). Extra positional arguments are ignored (`main.go:355`).
- **Stdin & byte preservation (`## set-meta` § Inputs):** `opSetMeta` (`main.go:350-356`) reads all of `stdin` to EOF with no JSON parsing, trimming, or newline stripping. Empty `stdin` (`""`) is stored as `""` and overwrites any prior value (`stores.go:231-233`); arbitrary bytes including spaces, newlines, and invalid UTF-8 are preserved (`meta_test.go:25-36`, `dispatch_test.go:245-256`).
- **Success envelope (`## set-meta` § Success output):** Exits `0` with empty `stdout` and empty `stderr` (`main.go:169-182,350-356`).
- **Write-failure handling (`## set-meta` § Failure cases):** Errors from `MkdirAll`, `CreateTemp`, `Write`, `Close`, or `Rename` (`filestore.go:27-50`) propagate out of `SetMeta` (`meta.go:56-63`, `meta_test.go:125-133`) and exit `1` with empty `stdout` and `stderr` output (`main.go:171-174`).
- **Incarnation lifetime (`## set-meta` § Conflicts):** `Stop` calls `clearIncarnations` -> `p.meta.Clear(n)` (`provider.go:559,565,582`, `stores.go:240-242`), removing all metadata for the session so keys do not leak across restarts.

## Unverified

- Whether any gc release newer than `v1.5.1` sets `GC_EXEC_DEADLINE_MS` or relies on `set-meta` failing for an unknown session.
- gc's concurrency expectations for same-key `set-meta` calls or `set-meta` racing `stop` (noted as unverified in `## set-meta` § Unverified).
