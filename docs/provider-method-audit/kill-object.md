# Provider method audit: `kill-object`

Verdict: `not implemented`

## Deviations

| doc claim | current implementation | behavioral impact | severity |
|---|---|---|---|
| Registered in op table with `stdin=True`, 30 s default deadline, 64 KiB stdout cap, and `carrier.ops` in handshake (`docs/provider-herdr-method-io.md` §kill-object "Registration", "Conflicts") | Absent from `ops` (`cmd/gc-runtime-rex/main.go:86-103`) and `unimplemented` (`cmd/gc-runtime-rex/main.go:73-77`); `handshake` lists only `["report-attachment","report-activity"]` (`cmd/gc-runtime-rex/main.go:36`) | Calling `kill-object` exits `2` (`exitUnknown`) with `gc-runtime-rex: unknown operation "kill-object"` on stderr (`cmd/gc-runtime-rex/main.go:31,126-134`); inert for gc v1.5.1 (never calls it and treats exit `2` as no-op), breaks `carrier.ops` callers | low |
| Requires `<session-name>` arg, reads stdin JSON (`object.{id,created}`, `mode`, optional `agent_pid`), validates non-blank fields, and runs under exclusive per-name lock (`docs/provider-herdr-method-io.md` §kill-object "Inputs") | `runWithContext` returns at `cmd/gc-runtime-rex/main.go:126-134` before arg validation, stdin read, or `NamespaceForCaller` / provider construction | Missing session arg or invalid JSON is never parsed or rejected with exit `1` | low |
| Exits `0` with one-line `<outcome>\n` (`killed`, `gone`, `renamed`, `live`, `changed`) and removes record/meta/activity on `killed` (`docs/provider-herdr-method-io.md` §kill-object "Success output") | No `KillObject` method in `internal/provider/*.go`; nothing written to stdout (`cmd/gc-runtime-rex/main.go:126-134`) | Callers cannot inspect box outcomes or reap corpse/zombie panes via `kill-object` | low |
| Failure cases exit `1` with `kill-object: <message>` on stderr (`docs/provider-herdr-method-io.md` §kill-object "Failure cases") | Always exits `2` with `unknown operation "kill-object"` on stderr (`cmd/gc-runtime-rex/main.go:131-133`) | No exit `1` validation, lock-busy, or backend error reporting | low |

## Matches

- **Unknown-op exit code and empty stdout (`docs/provider-herdr-method-io.md` §Shared RPP contract "Failure envelope")**: Unregistered ops return exit `2` (`exitUnknown`) with empty stdout (`cmd/gc-runtime-rex/main.go:31,126-134`), verified by `TestUnknownAndUnimplementedOps` (`cmd/gc-runtime-rex/dispatch_test.go:100-114`).
- **Stdin untouched on unknown op (`docs/provider-herdr-method-io.md` §Shared RPP contract "Failure envelope")**: `runWithContext` exits at `cmd/gc-runtime-rex/main.go:133` before invoking any handler, leaving stdin unread.
- **Honest capability advertisement (`docs/provider-herdr-method-io.md` §kill-object "Registration")**: `handshake` (`cmd/gc-runtime-rex/main.go:36`) does not advertise `carrier.ops`, matching unimplemented status.

## Unverified

- Whether any gc release newer than v1.5.1 sends `kill-object` or expects `carrier.ops` (cannot be settled from this repo; gc v1.5.1 has no `kill-object` per `docs/provider-herdr-method-io.md` §kill-object "Registration", "Unverified").
- Whether gc ever sets `GC_EXEC_DEADLINE_MS` (`cmd/gc-runtime-rex/main.go` does not read it; `docs/provider-herdr-method-io.md` §kill-object "Unverified").
- Whether `agent_pid` in `KillObjectRequest` is used by any future gc or pack (`docs/provider-herdr-method-io.md` §kill-object "Unverified").
