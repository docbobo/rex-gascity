# Method Audit: `copy-to`

**Verdict:** `not implemented`

## Deviations

| Doc claim | Current implementation | Behavioral impact | Severity |
|---|---|---|---|
| `copy-to` is registered in the op table (`## The op table` #22, `## copy-to > Registration`) with `reads_stdin=False`, `default` 30 s deadline, 64 KiB stdout cap, and `housekeeping=True`. | Absent from `ops` (`cmd/gc-runtime-rex/main.go:97-113`) and `unimplemented` (`cmd/gc-runtime-rex/main.go:82-86`); no `CopyTo` method or `copy-to` handler exists in `internal/provider/*.go` (only `stagePackOverlays`/`copyTree` in `internal/provider/overlay.go:16-66` for `start` pack overlays). | `gc-runtime-rex copy-to <name> <src> [rel-dst]` exits `2` (`exitUnknown`) with `gc-runtime-rex: unknown operation "copy-to"` on stderr and empty stdout (`cmd/gc-runtime-rex/main.go:139-145`). Under gc v1.5.1, exit `2` is treated as a successful no-op for non-`start`/`provision` ops (`## copy-to > Failure cases`, `## Shared RPP contract > Inherited from gc v1.5.1 vs pack choices`), and gc v1.5.1 has no known controller caller in the exec path (`## copy-to > Success output`). | low (gc v1.5.1) / medium (if a caller relies on `copy-to` staging) |
| Accepts `copy-to <name> <src> [rel-dst]`, validates >= 2 args, requires a live intact record and absolute `work_dir`/`cwd`, and stages `src` into the work dir via `stage_path` (missing `src` is a no-op; refuses `../` escapes, symlinks, and non-regular files) with exit `0` / `1` (`## copy-to > Inputs`, `## copy-to > Success output`, `## copy-to > Failure cases`). | Returns `exitUnknown` (`2`) before arg-count validation (`cmd/gc-runtime-rex/main.go:147-150`), state-dir resolution (`cmd/gc-runtime-rex/main.go:157`), or any filesystem work. | No files are copied; even `gc-runtime-rex copy-to` (fewer than 2 args) or `copy-to` on a missing session or invalid destination exits `2` instead of `1`. | low (gc v1.5.1) / medium (if a caller relies on `copy-to` staging) |
| Enforces 30 s `default` deadline, `GC_EXEC_DEADLINE_MS` override, and 64 KiB `SMALL_STDOUT_CAP` (`## copy-to > Registration`, `## Shared RPP contract > Deadlines and cancellation`, `## Shared RPP contract > Stdout caps`). | `cmd/gc-runtime-rex/main.go:46,163-179` wires `SIGTERM`/`SIGINT` via `signal.NotifyContext`, but has no `GC_EXEC_DEADLINE_MS` parsing, no 30 s deadline timer, and no 64 KiB stdout cap check. | None for `copy-to` (exits `2` immediately); broader dispatcher relies on gc's 30 s timeout and `SIGTERM`/`SIGKILL` rather than a self-enforced deadline. | low |

## Matches

- **Unknown-op exit `2` without reading stdin (`## Shared RPP contract > Failure envelope > Unknown op`, `## copy-to > Failure cases`):** `cmd/gc-runtime-rex/main.go:139-145` returns `exitUnknown` (`2`) with empty stdout and `gc-runtime-rex: unknown operation "copy-to"` on stderr before reading `stdin` or calling `provider.NamespaceForCaller()` (`cmd/gc-runtime-rex/main.go:157`). `TestUnknownAndUnimplementedOps` (`cmd/gc-runtime-rex/dispatch_test.go:108-120`) verifies this unknown-op path for other unrecognized ops.
- **Handshake capability omission (`## copy-to > Registration`, `## copy-to > Conflicts`):** `handshake` (`cmd/gc-runtime-rex/main.go:36`) declares only `["report-attachment","report-activity"]`, matching both the pack spec (`copy-to` is not declared in `handshake.py`) and gc v1.5.1's capability set. Covered by `TestProtocolHandshake` (`cmd/gc-runtime-rex/dispatch_test.go:84-106`).
- **gc v1.5.1 exit-2 compatibility (`## copy-to > Conflicts`, `## Shared RPP contract > Inherited from gc v1.5.1 vs pack choices`):** Returning exit `2` matches gc v1.5.1's exec-provider rule where exit `2` is a no-op success for every op other than `start` and `provision`.

## Unverified

- Whether any indirect gc v1.5.1 controller path invokes `runtime.Provider.CopyTo` on an exec provider (`## copy-to > Unverified`).
- Mid-copy cancellation behavior (`stage_path` does not poll the cancel token) and `src` symlink handling in the reference pack (`## copy-to > Unverified`).
- Behavior when extra arguments beyond `[rel-dst]` are passed to `copy-to` in the reference pack (`## copy-to > Unverified`).
