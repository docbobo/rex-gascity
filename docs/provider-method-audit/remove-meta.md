# Method Audit: `remove-meta`

**Verdict:** `deviations`

## Deviations

| Doc claim | Current implementation | Behavioral impact | Severity |
|---|---|---|---|
| `## Shared RPP contract` §Dispatch #8: `GC_EXEC_CITY_PATH` selects city; `GC_EXEC_STATE_DIR` sets state root. | `cmd/gc-runtime-rex/main.go:153` calls `provider.NamespaceForCaller()` (`internal/provider/namespace.go:45-53`), which reads `GC_REX_CITY` (`internal/provider/statedir.go:79-84`) and `GC_REX_STATE_DIR` / `XDG_STATE_HOME` (`internal/provider/statedir.go:13-28`). | `GC_EXEC_CITY_PATH` and `GC_EXEC_STATE_DIR` are ignored; callers setting only `GC_EXEC_STATE_DIR` fall back to `$XDG_STATE_HOME/rex-gascity`. | medium |
| `## remove-meta` §Failure cases: exits 1 if newest record lacks `gc.obj` (foreign terminal) or if core is unreachable (unless parked and nothing runs). | `Provider.RemoveMeta` (`internal/provider/meta.go:37-39`) never contacts Rex; `metaName` (`internal/provider/meta.go:9-15`) checks only `Binder.Cached` (`internal/provider/binding.go:268-274`). | Foreign panes and an unreachable Rex server both return exit 0 instead of exit 1. | low |
| `## remove-meta` §Failure cases: exits 1 on unreadable, corrupt, symlink, foreign-owner, or bad-nonce meta file. | `fileMetaStore.Delete` (`internal/provider/stores.go:245-247`) calls `os.Remove` (`internal/provider/filestore.go:108-111,135-140`) without reading, parsing, or checking symlinks/owners/nonces. | Corrupt or `000`-mode key files and symlinks are unlinked with exit 0; only parent-dir errors (e.g. `EACCES`, `EROFS`) exit 1. | low |
| `## remove-meta` §Success output: idempotent absent-key case rewrites the session meta file (creating one if none existed). | `fileMetaStore` (`internal/provider/stores.go:203-216`) keeps one file per key; `removeIfPresent` (`internal/provider/filestore.go:135-140`) ignores `fs.ErrNotExist` and writes nothing. | No file is created on absent key or unknown session; `GetMeta` still reads empty. | low |
| `## remove-meta` §Failure cases & `## Shared RPP contract` §Dispatch #6: acquires per-name lock; timeout exits 1 with `busy: ...`. | `fileTable.remove` (`internal/provider/filestore.go:108-111`) takes only an in-process `sync.Mutex`; no cross-process lock. | Concurrent invocations never fail with lock timeout `"busy: ..."`. | low |
| `## remove-meta` §Registration, §Failure cases & `## Shared RPP contract` §Deadlines: 30 s default or `GC_EXEC_DEADLINE_MS`, pre-handler deadline/cancel check, 64 KiB cap, `sweep_spawn_files`. | `runWithContext` (`cmd/gc-runtime-rex/main.go:46,121-166`) uses `signal.NotifyContext` only; `RemoveMeta` ignores `ctx` (`internal/provider/meta.go:37`); no `GC_EXEC_DEADLINE_MS`, 30 s timer, cap check, or spawn sweep. | Relies on gc's 30 s kill; a pre-cancelled/SIGTERM'd call still exits 0 if `os.Remove` finishes. 64 KiB cap is unreachable (empty stdout). | low |
| `## remove-meta` §Failure cases & `## Shared RPP contract` §Failure envelope: stderr is redacted `remove-meta: <message>`. | `cmd/gc-runtime-rex/main.go:145,149,155,160` writes unredacted `gc-runtime-rex: remove-meta: <msg>` to stderr. | Minor stderr format difference; `remove-meta` takes no secret values in args/stdin. | low |

## Matches

- **Registration & stdin (`## remove-meta` §Registration, §Inputs):** Registered at `cmd/gc-runtime-rex/main.go:108` (`required: 2`); `opRemoveMeta` (`cmd/gc-runtime-rex/main.go:336-338`) ignores `stdin`.
- **Argument validation (`## remove-meta` §Inputs, §Failure cases):** Missing/empty `<name>` exits 1 (`cmd/gc-runtime-rex/main.go:144-147`); missing/empty `<key>` exits 1 via `keyedOps` (`cmd/gc-runtime-rex/main.go:87-91,148-151`); extra args (`args[2:]`) are ignored (`cmd/gc-runtime-rex/main.go:336-338`); no reserved keys.
- **Success envelope (`## remove-meta` §Success output):** Writes nothing to stdout or stderr and exits 0 (`cmd/gc-runtime-rex/main.go:159-166,336-338`).
- **Delete & idempotency (`## remove-meta` §Success output):** Deletes only the named key across canonical and legacy namespace stores (`internal/provider/composite.go:44-50`, `internal/provider/stores.go:245-247`), resolving aliases via `metaName` (`internal/provider/meta.go:9-15`); unknown sessions and absent keys exit 0 (`internal/provider/filestore.go:135-140`). Verified by `TestMetaRoundTripAcrossCalls` (`cmd/gc-runtime-rex/dispatch_test.go:244-258`) and `TestRemoveMetaRemovesOnlyNamedKey` (`internal/provider/meta_test.go:33-46`).
- **Write/unlink & state-dir failures (`## remove-meta` §Failure cases):** State-dir resolution errors (`cmd/gc-runtime-rex/main.go:153-157`) and OS unlink errors (`internal/provider/filestore.go:135-140`) exit 1 with empty stdout (`cmd/gc-runtime-rex/main.go:159-165`).

## Unverified

- Runtime behavior (exit code and stderr) when `gc` invokes `gc-runtime-rex remove-meta` end-to-end (conclusions are from code inspection and unit tests in `dispatch_test.go` and `meta_test.go`).
