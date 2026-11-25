# runtime-herdr-core provider methods: inputs and outputs

What each provider method of the `runtime-herdr-core` reference pack consumes
and produces, in success and in failure. One section per op in the pack's op
table, plus the RPP-level handling every op shares.

This is an input/output reference, not a comparison. For how the Rex provider
differs from `runtime-herdr-core`, see
[`docs/provider-herdr-parity.md`](provider-herdr-parity.md).

## Sources and pins

Every fact below comes from one of these sources, cited inline:

- **Pack**: `gastownhall/gascity-packs`, tree
  `runtime-herdr-core/assets/gc_runtime_herdr_core/`, at commit `0f07496` –
  the same commit `docs/provider-herdr-parity.md` pins
  (`docs/provider-herdr-parity.md:40`).
  Pack citations are pack-relative (`herdrcore/ops_table.py:68`,
  `rpp/perop.py:135`, `hc/client.py:369`), so they resolve against that tree.
  `bin/…` means `assets/bin/…`.
- **gc**: `gastownhall/gascity` at tag `v1.5.1`, the contract's pinned
  baseline (`internal/runtime/exec/`, `internal/runtime/protocol.go`,
  `docs/reference/exec-session-provider.md`). gc citations are repo-relative
  (`internal/runtime/exec/exec.go:104`).
- **Parity record**: [`docs/provider-herdr-parity.md`](provider-herdr-parity.md),
  cited by section and verdict.
- **Pack tests**, for corroboration only: `runtime-herdr-core/tests/`, cited as
  `tests/unit/…` or `tests/fakehc/…`.

Facts read from a live working tree rather than a pin are labelled as such
where they appear. Two exist:

- The pack's working tree `8ed12884` is ahead of `0f07496`; `git diff 0f07496
  HEAD` touches only `README.md`, `herdrcore/config.py`, `herdrcore/core.py`
  and tests, so the `nudge` path is unchanged there.
- The live gascity tree at `/Volumes/Sources/gascity` is ahead of `v1.5.1`.
  `inventory` cites it once, labelled "live gascity tree".

"derived" means the finding is a reading of the code that no test or doc
states.

## Claim rules

Per [`AGENTS.md`](../AGENTS.md):

- Every claim cites its source: pack file and line, gc file and line, or
  parity section and verdict.
- A claim neither source settles is labelled **`unverified`**. It is not
  rounded up to fact.
- Where the pack and gc disagree, the conflict is **reported, not resolved**.
  Each op section has a "Conflicts" subsection for this; the shared contract
  section collects the ones that are not op-specific.
- A parity verdict changes only after the check is re-run against both
  sources; this document does not change verdicts.
- Where two findings disagree about the same fact, both statements are kept
  with their citations and a one-line "Sources disagree:" note.

Exit codes: the pack's `rpp/exitcodes.py:1` says its table is the contract's
"Exit Codes" section, and each code's meaning below is the meaning the pack
gives it. gc v1.5.1's "Exit Codes" section lists only 0, 1 and 2
(`docs/reference/exec-session-provider.md:71-77`); 66, 73, 75 and 80 are pack
extensions that gc reads as a plain error (conflict A in the shared contract).
Sources disagree: the pack docstring presents the whole table as gc's; gc
v1.5.1 defines only 0, 1 and 2.

## How to read a method section

Each section has the same shape:

1. **Registration** – the op-table line and the three transport facts it sets:
   whether the op reads stdin, its deadline class, and its stdout cap.
2. **Inputs** – argv, stdin body or request fields, environment, and any
   precondition that must hold before the handler does its work.
3. **Success output** – the exact stdout and exit code.
4. **Failure cases** – a table of case, exit code, stdout and source,
   including error mapping, deadline behaviour, and the stdout cap.
   Failures every op shares (deadline spent, SIGTERM, internal error, stdout
   cap, closed stdout) are in the shared contract and only referenced here.
5. **Conflicts** – pack vs gc, and the parity verdict.
6. **Unverified** – what neither source settles.

## The op table

Registration facts are from `herdrcore/ops_table.py:68-91`; the deadline
classes and their seconds from `rpp/ops.py:23-27`; the caps from
`rpp/ops.py:32-34`.

| # | Op | Registered at | Reads stdin | Deadline | Stdout cap |
|---|---|---|---|---|---|
| 1 | `protocol` | `herdrcore/ops_table.py:68` | no | default (30 s) | 64 KiB |
| 2 | `start` | `herdrcore/ops_table.py:69` | yes | start (120 s) | 64 KiB |
| 3 | `stop` | `herdrcore/ops_table.py:70` | no | default (30 s) | 64 KiB |
| 4 | `kill-object` | `herdrcore/ops_table.py:71` | yes | default (30 s) | 64 KiB |
| 5 | `observe` | `herdrcore/ops_table.py:72` | yes | probe (4 s) | 1 MiB |
| 6 | `inventory` | `herdrcore/ops_table.py:73` | no | default (30 s) | 8 MiB |
| 7 | `is-running` | `herdrcore/ops_table.py:74` | no | default (30 s) | 64 KiB |
| 8 | `process-alive` | `herdrcore/ops_table.py:75` | yes | default (30 s) | 64 KiB |
| 9 | `list-running` | `herdrcore/ops_table.py:76` | no | default (30 s) | 8 MiB |
| 10 | `is-attached` | `herdrcore/ops_table.py:77` | no | probe (4 s) | 64 KiB |
| 11 | `get-last-activity` | `herdrcore/ops_table.py:78` | no | default (30 s) | 64 KiB |
| 12 | `peek` | `herdrcore/ops_table.py:79` | no | default (30 s) | 1 MiB |
| 13 | `get-meta` | `herdrcore/ops_table.py:80` | no | default (30 s) | 64 KiB |
| 14 | `set-meta` | `herdrcore/ops_table.py:81` | yes | default (30 s) | 64 KiB |
| 15 | `remove-meta` | `herdrcore/ops_table.py:82` | no | default (30 s) | 64 KiB |
| 16 | `nudge` | `herdrcore/ops_table.py:83` | yes | default (30 s) | 64 KiB |
| 17 | `send-keys` | `herdrcore/ops_table.py:84` | no | default (30 s) | 64 KiB |
| 18 | `interrupt` | `herdrcore/ops_table.py:85` | no | default (30 s) | 64 KiB |
| 19 | `clear-scrollback` | `herdrcore/ops_table.py:86` | no | default (30 s) | 64 KiB |
| 20 | `watch-startup` | `herdrcore/ops_table.py:87` | no | start (120 s) | 64 KiB |
| 21 | `attach` | `herdrcore/ops_table.py:88` | no | start (120 s) | 64 KiB |
| 22 | `copy-to` | `herdrcore/ops_table.py:89` | no | default (30 s) | 64 KiB |
| 23 | `server-stop` | `herdrcore/ops_table.py:90` | no | default (30 s) | 64 KiB |
| 24 | `x-kick-core` | `herdrcore/ops_table.py:91` | no | default (30 s) | 64 KiB |

`x-kick-core` is registered with `housekeeping=False`, so it skips the
per-op spawn-file sweep every other op runs first
(`herdrcore/ops_table.py:45-46,91`). `protocol` is also served by a fast path
that never loads the op table (`main.py:30-36`); see its section.

## Shared RPP contract

The pack's `rpp/` package (ops, perop, cancel, exitcodes, outcome, redact,
wire and others) is declared "provider-neutral, names no backend"
(`rpp/__init__.py:1-8`). `hc/*` and `herdrcore/*` are backend-specific, and
`handshake.py` declares this pack's capabilities (`hc/client.py:1-34`,
`herdrcore/context.py:78-89,122-134`, `handshake.py:33-47`). Every op section
below refers to this section for the envelope, exit codes, deadlines,
cancellation and caps instead of repeating them.

Most of what the kit does beyond exit 0/1/2, the 30 s and 120 s timeouts and
SIGTERM then SIGKILL is not in gc v1.5.1. `observe.v1`, `input-outcome`,
exit 66/73/75/80, `GC_EXEC_DEADLINE_MS`, stdout caps, `fs.host`, `carrier.ops`,
`conformance-faults`, `server-lifecycle`, `GC_EXEC_CITY_PATH` and `GC_RPP_FAULT`
all return no hits under `internal/`, `docs/reference/` or `cmd/` of v1.5.1
(`rg`, non-test, non-svg). See "Inherited from gc" and "Conflicts" below.

### Dispatch (one process per op)

1. gc execs the script directly, no shell: `<script> <operation>
   <session-name> [args...]` (`docs/reference/exec-session-provider.md:61-68`;
   `internal/runtime/exec/exec.go:83`). The pack entry is
   `bin/gc-runtime-herdr-core`: `#!/usr/bin/env -S python3 -I -S` (`bin/gc-runtime-herdr-core:1`),
   which records `_STARTED = time.monotonic()` after its initial standard-library
   imports and before importing the pack (`bin/gc-runtime-herdr-core:8-15`),
   then `sys.exit(main(sys.argv,
   started=_STARTED))` (`bin/gc-runtime-herdr-core:17`).
2. `main.main` (`main.py:51`). Three pack-only prefixes are intercepted before
   RPP: `x-version` (`main.py:53-54`), `x-doctor` (`main.py:55-58`) and
   `x-command` (`main.py:59-62`). Everything else goes to `run_per_op`
   (`main.py:63-72`) with `os.environ`, `sys.stdin.buffer`,
   `sys.stdout.buffer`, `sys.stderr`, and `debug = GC_HERDR_CORE_DEBUG=="1"`
   (`main.py:27,71`). `main.py` is the only module that reads the process
   environment (`main.py:8-9`).
3. Op lookup: `ops_for(argv[1] or "")` (`main.py:66`). `protocol` gets a static
   one-entry table and no backend code is loaded (`main.py:30,35-36`). Any
   other string, **including unknown ones**, builds the full table via
   `build_ops(live_host(sys.executable))` (`main.py:37-40`; table at
   `herdrcore/ops_table.py:61-93`). Then `ops.get(op)` (`rpp/perop.py:116-117`).
4. `OpRequest` is built inside the `try` (`rpp/perop.py:131-141`):
   `op=argv[1]`, `args=tuple(argv[2:])` (so `args[0]` is the session name,
   `rpp/perop.py:134`), `stdin` = `_read_stdin(...)` **only if
   `spec.reads_stdin`, else `b""`** (`rpp/perop.py:135`; field doc
   `rpp/ops.py:99-101`), `env=dict(environ)` (`rpp/perop.py:109`),
   `cancel=CancelToken()` (`rpp/perop.py:126`), `deadline`
   (`rpp/perop.py:129`; see "Deadlines and cancellation"), `redactor` seeded
   from the environment (`rpp/perop.py:110-111`), and `emit` = write plus flush
   to stdout (`rpp/perop.py:122-124`). Dataclass: `rpp/ops.py:44-57`. The
   default `emit` raises `OpError("this transport does not stream op output")`
   (`rpp/ops.py:40-41,55`).
5. Pre-handler gates (`rpp/perop.py:142-145`): `cancel.check()`, then
   `deadline.expired()` raises `OpError("no time left before the op's
   deadline")`. The SIGTERM handler is installed at `rpp/perop.py:127-128`
   (`rpp/perop.py:50-57`) – note this is *after* the unknown-op return, so
   unknown ops install no handler.
6. Handler: `spec.handler(request)` (`rpp/perop.py:146`). In the herdr table
   each handler is wrapped by `_wrap` (`herdrcore/ops_table.py:41-53`): it
   builds `OpContext` (`herdrcore/context.py:78-89`), runs the
   `sweep_spawn_files` housekeeping (except for `x-kick-core`), and maps
   `StateDirError`, `ConfigError`, `MetaError` and `LockFileError` to
   `OpError(str(err))` and `LockTimeout` to `OpError("busy: …")`
   (`herdrcore/ops_table.py:48-51`). Handlers read only the request, never
   `os.environ` (`rpp/ops.py:6-8`; `herdrcore/context.py:84` uses
   `request.env`).
7. `OpResult`, then the cap check, then write stdout, then write stderr, then
   `return int(result.code)` (`rpp/perop.py:158-170`); `sys.exit` of that value
   in `bin/gc-runtime-herdr-core:17`.
8. Request fields `OpContext` derives: the city key from `GC_EXEC_CITY_PATH`
   and the state root from `GC_EXEC_STATE_DIR` (`rpp/statedir.py:38-39,55-63,179-192`,
   `hc/paths.py:118-120`, `herdrcore/context.py:86`), and the fault from `GC_RPP_FAULT`
   (`herdrcore/context.py:34-44,88`). gc documents only `GC_EXEC_STATE_DIR`
   (`docs/reference/exec-session-provider.md:402-404`); `GC_EXEC_CITY_PATH` has
   no hit in v1.5.1.

### Success envelope

- `OpResult(code, stdout: bytes, message)` (`rpp/ops.py:60-66`).
  `OpResult.ok(stdout=b"")` is exit 0 plus raw bytes (`rpp/ops.py:68-70`).
  `OpResult.answer(line)` is exit 0 plus `line + "\n"` in UTF-8
  `surrogateescape` (`rpp/ops.py:72-75`). `answer` is used for one-line scalars
  and JSON; `ok` for empty or multi-line bodies (`peek`) or typed bytes (the
  outcome line, `rpp/outcome.py:113`).
- The exit code on success is always 0 (`ExitCode.OK`, `rpp/exitcodes.py:11`).
  "No such session" is also exit 0 only "where the op allows it"
  (`rpp/exitcodes.py:12`).
- The runner writes nonempty buffered stdout if it fits the cap, regardless of
  exit code (`rpp/perop.py:158-165`). A success may carry a stderr `message`
  (`rpp/perop.py:166-167`), for example `start` warnings
  (`docs/provider-herdr-parity.md` §start).
- JSON is compact, ASCII-only, no NaN (`rpp/jsonval.py:29-35`). Handshake:
  `{"version":0,"capabilities":[…],"locality":"local"}\n`; test
  `tests/unit/test_rpp_perop.py:61-67`.
- **Streaming (`watch-startup`).** The handler calls `ctx.request.emit(bytes)`
  per change (`herdrcore/screen.py:41`; the line is `{"content":…}` plus `\n`,
  `rpp/wire.py:307-314`). `emit` writes and flushes immediately
  (`rpp/perop.py:122-124`) and bypasses the stdout cap (the cap only tests
  `result.stdout`, `rpp/perop.py:158`); the handler returns `OpResult.ok()`
  (`herdrcore/screen.py:44`). The loop runs while `deadline.budget(0.25) > 0`
  (`herdrcore/screen.py:25,35`). Leaving by that budget check normally gives
  exit 0 around the 1 s margin; a read near the margin can fail instead
  (`herdrcore/screen.py:35-44`, `hc/client.py:364-371`).
- A `BrokenPipeError` raised inside `emit` reaches the generic handler exception
  path (exit 1, `internal error: BrokenPipeError…`, `rpp/perop.py:122-124,153-156`);
  only the final buffered write has the separate `OSError` guard
  (`rpp/perop.py:161-165`). No dedicated test was found for this streaming case.

### Failure envelope

- **Ordinary buffered failure:** `OpResult.fail` gives empty stdout and, for a
  nonempty message, one redacted, prefixed stderr message write (`rpp/ops.py:77-79`,
  `rpp/perop.py:147-167`). This is not a general failure invariant: typed exit 80
  has buffered stdout; already emitted streaming bytes remain after failure;
  debug can add a traceback, a message can span physical lines, missing-op
  usage is unprefixed and unredacted, and a buffered stdout write error can
  return before stderr is attempted (see below). Other caught `Exception`s
  become **exit 1 `<op>: internal error: <ExcType>: <msg>`**
  (`rpp/perop.py:147-154`; test `tests/unit/test_rpp_perop.py:215`).
- `OpError(message, code=ExitCode.FAIL)` (`rpp/ops.py:82-88`) becomes
  `OpResult.fail(message, code)` (`rpp/ops.py:77-79`; `rpp/perop.py:147-148`).
  Handlers may also *return* `OpResult.fail(…, code)` (for example
  `herdrcore/reads.py:366`, `herdrcore/lifecycle.py:756`).
- Mapping (`rpp/perop.py:147-165`):
  - `OpError` gives its code (default 1). `Cancelled` gives exit 1, message
    `canceled (<reason>)` (`rpp/cancel.py:34-36`; `rpp/perop.py:149-150`).
    `KeyboardInterrupt` gives exit 1 `canceled (interrupted)`
    (`rpp/perop.py:151-152`). Any other `Exception` gives exit 1 internal
    error (`rpp/perop.py:153-154`); with `GC_HERDR_CORE_DEBUG=1` a redacted
    traceback is a second stderr write (`rpp/perop.py:155-156,168-169`).
  - `BaseException`s that are not `Exception` or `KeyboardInterrupt` (for
    example `SystemExit` from a handler) are not caught (derived from the
    `except` list at `rpp/perop.py:147-154`; no `SystemExit` handling in
    `rpp/` or `main.py`, `rg`).
  - Over-cap answer: exit 1, `answer of N bytes is over the C-byte cap`,
    stdout dropped (`rpp/perop.py:158-159`; test
    `tests/unit/test_rpp_perop.py:233-241`).
  - stdout write failure (`OSError`, gc stopped reading): return exit 1
    immediately, no stderr (`rpp/perop.py:161-165`; test
    `tests/unit/test_rpp_perop.py:281`).
- **stderr:** `"<op>: <message>"` (`rpp/perop.py:4,167`), newline appended if
  absent; embedded newlines are preserved, the write is flushed, and `OSError`
  is swallowed (`rpp/perop.py:83-88`). Redaction
  (`rpp/redact.py`): sensitive-key environment values of at least 4 bytes
  (`rpp/redact.py:26,81-85`); session environment values of at least 8 bytes,
  or `GC_INSTANCE_TOKEN` of any length (`rpp/redact.py:30,87-96`; handlers
  register through `request.redactor.add_start_env`, test
  `tests/unit/test_rpp_perop.py:205`); and `KEY=value` / `--flag value` secret
  patterns (`rpp/redact.py:51-56,102`). The reason is that gc embeds stderr
  verbatim (`rpp/redact.py:3-4`; `rpp/exitcodes.py:15`). Success **stdout is
  not redacted** by the runner (no redact call at `rpp/perop.py:158-165`;
  derived).
- **Unknown op: exit 2, stdin untouched.** stderr is `<op>: unknown operation`
  (redacted) (`rpp/perop.py:117-120`); the runner returns before `OpRequest`
  and `_read_stdin` (`rpp/perop.py:135` is after). Test
  `tests/unit/test_rpp_perop.py:70-75` (`stdin_pos == 0`; covers `exec`,
  `provision`, `check-image`, `frobnicate`, `start`).
  `herdrcore/ops_table.py:9-10` notes that `exec`, `provision` and
  `check-image` are absent. No or empty op gives `usage: <program> <op>
  [args...]`, exit 2, unredacted (`rpp/perop.py:112-115`; test
  `tests/unit/test_rpp_perop.py:78`).
- **Input outcome: exit 80** (only `nudge`, `send-keys`, `interrupt` and
  `clear-scrollback`; `rpp/outcome.py:31,60-62`; `rpp/exitcodes.py:30-33`).
  stdout is one JSON line with required `outcome` and optional `detail`, omitted
  if the cleaned detail is empty (`rpp/outcome.py:96-100,109`); stderr is
  `<op>: <word>[: detail]` (`rpp/outcome.py:114-115`). `delivered` is exit 0
  with empty stdout; `delivered_unobserved` is exit 0 plus one outcome line,
  `nudge` only (`rpp/outcome.py:51-54,110-113`). The driving handlers' separate
  `inputs.Unknown` result is an ordinary exit 1 (`herdrcore/inputs.py:181-185`).
  `outcome_result` rejects nudge-only words for the other ops with `ValueError`,
  which becomes an internal error; this is not runtime validation of arbitrary
  words against the declared vocabulary (`rpp/outcome.py:47-62,103-115`).

### Exit-code table

Source: `rpp/exitcodes.py:8-33`, locked by
`tests/unit/test_rpp_exitcodes.py:8-17`.

| Code | Name | Trigger in the pack | Cite |
|---|---|---|---|
| 0 | OK | an answer, including an allowed "no such session"; `delivered`; `delivered_unobserved` | `rpp/exitcodes.py:11`; `rpp/outcome.py:110-113` |
| 1 | FAIL | `OpError` default; `Cancelled`; interrupt; internal error; over-cap; deadline and stdin failures; stdout write failure; driving handler's `inputs.Unknown` | `rpp/exitcodes.py:14`; `rpp/ops.py:78,85`; `rpp/perop.py:75,145,149-165`; `herdrcore/inputs.py:181-185` |
| 2 | UNKNOWN_OP | op not in the table; missing or empty op (usage) | `rpp/exitcodes.py:17`; `rpp/perop.py:115,120` |
| 66 | NO_SCREEN | `peek` only: no record, or hc `not_found` | `rpp/exitcodes.py:21-22`; `herdrcore/reads.py:364-371` |
| 73 | FRESH_COLLISION | `start` with `fresh_only` when a box holds the name | `rpp/exitcodes.py:24-25`; `herdrcore/lifecycle.py:756` |
| 75 | TEMPFAIL | defined ("start only: backend at capacity; gc retries"); **no emitter found** (`rg TEMPFAIL assets` hits only the definition) | `rpp/exitcodes.py:27-28` |
| 80 | INPUT_OUTCOME | a non-success outcome word from the four driving ops | `rpp/exitcodes.py:30-33`; `rpp/outcome.py:115` |

The module docstring claims the table is gc's "Exit Codes" section
(`rpp/exitcodes.py:1`); gc's section lists only 0, 1 and 2
(`docs/reference/exec-session-provider.md:71-77`). See conflict A.

### Deadlines and cancellation

- `DeadlineClass` is `"default" | "start" | "probe"` (`rpp/ops.py:21`); the
  seconds are `default` 30.0, `start` 120.0 and `probe` 4.0
  (`rpp/ops.py:23-27`).
- Assignment (`herdrcore/ops_table.py:64-91`): `start` class – `start`,
  `watch-startup`, `attach`; `probe` class – `observe`, `is-attached`;
  `default` class – everything else (including `protocol`, `rpp/ops.py:116`,
  `stop`, `kill-object`, `inventory`, `peek`, `nudge`, `x-kick-core`).
- `GC_EXEC_DEADLINE_MS` (`rpp/perop.py:31`) is accepted only if it fully
  matches `[0-9]{1,12}` (`rpp/perop.py:35,45-47`); it then replaces the class
  default with `ms/1000` seconds (`rpp/ops.py:127`). An absent, malformed,
  negative, padded or decimal value reads as absent (`rpp/perop.py:42-44`;
  tests `tests/unit/test_rpp_perop.py:255-274`). `"0"` parses to 0 and the op
  fails with "no time left" (test `tests/unit/test_rpp_perop.py:158-168`, which
  uses `GC_EXEC_DEADLINE_MS=0`).
- The clock origin is the entry script's `time.monotonic()` sample after its
  initial `os`, `sys` and `time` imports and before importing the pack
  (`bin/gc-runtime-herdr-core:8-17`). Interpreter startup before the sample
  does not count; pack imports after the sample do (`rpp/ops.py:128`,
  `rpp/cancel.py:84-88`).
- `Deadline.budget(cap, margin=1.0)` is `min(cap, remaining - 1.0)`; a value of
  0 or less means "must not start" (`rpp/cancel.py:27-28,98-103`). Waits that
  use this budget use the margin; it is not a universal bound on inner waits
  (`rpp/cancel.py:12-14,66-74`).
- Expiry checks in the runner: (a) each fd-backed stdin poll step
  (`rpp/perop.py:72-75`), (b) once before the handler (`rpp/perop.py:143-145`).
  **No runner timer or alarm enforces the deadline during the handler.** A
  pack-side deadline needs an explicit budget/remaining check or a budgeted
  client call (`herdrcore/screen.py:35`, `hc/client.py:364-371`);
  `cancel.pause()` checks cancellation, not the op budget (`rpp/cancel.py:66-74`).
- **Stdin open at the deadline margin.** For fd-backed process stdin,
  `_read_stdin` polls in `POLL_INTERVAL_S=0.1` steps (`rpp/cancel.py:24`,
  `rpp/perop.py:60-80`). Once `budget(0.1) <= 0`, about 1 s before the
  deadline, it raises `OpError("stdin was still open at the op's deadline")`:
  exit 1, that message on stderr, handler never runs (`rpp/perop.py:73-75`;
  test `tests/unit/test_rpp_perop.py:135-155`). With less than 1 s left the
  fd-backed path fails even if data is ready, because the check precedes
  `poll` (`rpp/perop.py:73-76`); the in-memory path reads immediately
  (`rpp/perop.py:64-67`).
- **Cancellation.** gc sends SIGTERM to the process group and SIGKILL 2 s later
  (`docs/reference/exec-session-provider.md:87-97`;
  `internal/execgrace/execgrace.go:133-141`, `internal/execgrace/signal_unix.go:30,46,51`;
  `internal/runtime/exec/exec.go:92`). In the pack the handler `on_sigterm`
  only sets the flag (`rpp/perop.py:50-57`; `rpp/cancel.py:6-9`). The flag is
  polled in the stdin read (`rpp/perop.py:72`), before the handler
  (`rpp/perop.py:143`), in `CancelToken.pause` (`rpp/cancel.py:66-74`) and in
  client calls (`herdrcore/context.py:131`, `hc/client.py:364-371,396-403`).
  Direct `Cancelled` from SIGTERM (including pre-handler checks) gives exit 1
  `<op>: canceled (terminated)` when it reaches the runner (`rpp/perop.py:149-150`;
  tests `tests/unit/test_rpp_perop.py:313,371,400`). `HcCancelled` is also an
  `HcError` (`hc/errors.py:107-112`); handlers may map it first: `observe`
  answers exit 0 unknown, `inventory` exit 0 incomplete on listing, `stop`
  reports `cannot stop …`, reads can report `cannot read …`, and input ops
  use stage- and writtenness-dependent results (`herdrcore/reads.py:143-147,201-205,274-278`,
  `herdrcore/lifecycle.py:517-521`, `herdrcore/inputs.py:218-224,260-264,277-281`).
  Separately, gc reports a call it canceled as canceled whatever the exit code
  (`docs/reference/exec-session-provider.md:98-99`;
  `internal/runtime/exec/exec.go:114-115`). Wait granularity is 0.1 s
  (`rpp/cancel.py:24,74`); the SIGKILL after gc's 2 s cannot be handled.
- gc kills `exec` and `watch-startup` outright with no SIGTERM
  (`docs/reference/exec-session-provider.md:119-121`), so the pack's SIGTERM
  path for `watch-startup` is unused by v1.5.1 (parity §watch-startup agrees).

### Stdout caps and the outcome detail cap

- `SMALL_STDOUT_CAP` = 64 KiB, `OBSERVE_STDOUT_CAP` = 1 MiB,
  `INVENTORY_STDOUT_CAP` = 8 MiB (`rpp/ops.py:32-34`). `OpSpec.stdout_cap` is
  mandatory (`rpp/ops.py:103`); the default in `spec()` is SMALL
  (`herdrcore/ops_table.py:64`).
- Assignment (`herdrcore/ops_table.py:72-79`): **OBSERVE** is `observe` and
  `peek`; **INVENTORY** is `inventory` and `list-running`; **SMALL** is every
  other op.
- Over the cap: the whole answer is discarded, exit 1, `<op>: answer of N
  bytes is over the C-byte cap`, empty stdout (`rpp/perop.py:158-159`; test
  `tests/unit/test_rpp_perop.py:233-241`). The cap is checked after the handler
  returns, so the answer is fully buffered first (derived). `emit` output is
  uncapped (see "Success envelope").
- Outcome `detail`: `DETAIL_CAP = 1 << 10` = 1024 UTF-8 bytes
  (`rpp/outcome.py:56`). `clean_detail` redacts, maps control characters to a
  space, drops lone surrogates, cuts on a character boundary and strips
  (`rpp/outcome.py:77-93`); test `tests/unit/test_rpp_outcome.py:123-132`. The
  outcome line is ASCII-encoded via `dumps` (`rpp/outcome.py:100`).

### Inherited from gc v1.5.1 vs pack choices

**Inherited (gc source cited):**

- One process per op, no shell, `<op> <name> [args]`
  (`docs/reference/exec-session-provider.md:61-68`).
- In the buffered `runError` path, exit 0 is success, 1 is failure with stderr,
  and 2 counts as unknown-op success except for `start` and `provision`
  (`docs/reference/exec-session-provider.md:71-85`,
  `internal/runtime/exec/exec.go:159-161,174-181`). `attach`, `exec` and
  `watch-startup` have separate paths (see their op sections and `exec`'s
  separate handling in `internal/runtime/exec/exec.go:894-904`). Pack side:
  `rpp/exitcodes.py:11-17`, `rpp/perop.py:120`.
- stderr is embedded verbatim in gc errors: `errMsg := TrimSpace(stderr)`
  (`internal/runtime/exec/exec.go:162-165,169`), so the pack redacts
  (`rpp/redact.py:3-4`).
- Default 30 s and start 120 s (`internal/runtime/exec/exec.go:56-57`), mirrored
  at `rpp/ops.py:23-27` (`rpp/ops.py:28-30` says so).
- Cancel is SIGTERM to the group, SIGKILL after 2 s
  (`docs/reference/exec-session-provider.md:87-97`;
  `internal/execgrace/execgrace.go:133-141`), mirrored at `rpp/perop.py:50-57`.
- stdin payloads: `nudge`, `process-alive` and `start` take stdin
  (`internal/runtime/exec/exec.go:99,617,708`), mirrored by `reads_stdin=True`
  for these (`herdrcore/ops_table.py:69,75,83`). gc strips the trailing `\n`
  from stdout (`internal/runtime/exec/exec.go:104`), so the pack's single `\n`
  is harmless.
- Handshake `{"version","capabilities"}`; unknown capability strings are
  ignored (`internal/runtime/protocol.go:60-70`;
  `internal/runtime/exec/handshake.go:24-41`).
- gc runs with its own environment
  (`docs/reference/exec-session-provider.md:406-407`), mirrored by the
  environment snapshot (`rpp/perop.py:109`).
- The `report-attachment` and `report-activity` tokens
  (`internal/runtime/protocol.go:21,25`).

**Pack-specific (not in gc v1.5.1):** exit 66, 73, 75 and 80 and the outcome
vocabulary; `GC_EXEC_DEADLINE_MS` and the 1 s `DEADLINE_MARGIN_S`; the 4 s
`probe` class (`rpp/ops.py:26,30` cites gc `exec/observe.go`; gc v1.5.1 has no
`internal/runtime/exec/observe.go`); the stdout caps; the stdin-open-at-deadline
failure; `x-version`, `x-doctor`, `x-command` and `x-kick-core`;
`GC_HERDR_CORE_DEBUG`; `GC_RPP_FAULT`; `emit` streaming; 8 of 10 handshake
tokens (parity §protocol deviation 1); and redaction rules beyond gc's
(`rpp/redact.py:7-16`; `rpp/redact.py:35-56` uses the same sensitive-key
marker list and assignment-pattern text as `internal/execenv/execenv.go:24,59-73`,
while replacement markers differ – `[REDACTED]` vs `[redacted]` – and
`add_start_env` adds pack-specific session-env rules).

### Conflicts, pack vs gc v1.5.1 (reported, not resolved)

- **A. Exit-code source.** `rpp/exitcodes.py:1` says the table is gc's
  `docs/reference/exec-session-provider.md` "Exit Codes" section. gc v1.5.1 lists 0, 1 and 2
  only (`docs/reference/exec-session-provider.md:71-77`) and treats any other
  non-zero exit as a plain error (`internal/runtime/exec/exec.go:162-169`;
  parity §protocol verdict). 66, 73, 75 and 80 are pack extensions.
- **B. Exit 2 scope.** `rpp/exitcodes.py:18-19`: exit 2 is a failure "for start
  and ops the handshake declares". gc v1.5.1: a failure only for `start` and
  `provision` (`internal/runtime/exec/exec.go:174-181`;
  `docs/reference/exec-session-provider.md:79-85`). Exit 2 from an op tied to a
  declared capability (for example `is-attached` with `report-attachment`, or
  `nudge` with `input-outcome`) is still treated as success in v1.5.1
  (`internal/runtime/exec/exec.go:159-161`).
- **C. Doc sections and files that do not exist in v1.5.1.** `rpp/ops.py:35-37`
  cites "exec-session-provider.md 'observe.v1'", `rpp/outcome.py:1-2` cites
  "Typed input outcomes", and `rpp/ops.py:30` cites `exec/observe.go`, which is
  absent (`ls internal/runtime/exec/` at the `v1.5.1` tree has no
  `internal/runtime/exec/observe.go` and the v1.5.1 doc has no `observe.v1` or
  input-outcome sections). This is confirmed version skew: the pack targets a
  newer gc (also `handshake.py:20-22`, gastownhall/gascity#9129). Separately,
  `rpp/perop.py:31-33` states "No gc release sends it yet" of
  `GC_EXEC_DEADLINE_MS`; `GC_EXEC_DEADLINE_MS` has no hit in the pinned v1.5.1
  tree, which is consistent with that statement rather than conflicting with it
  (later gc releases are outside the pin).
- **D. Stdout cap.** The pack fails an answer over 1 MiB, 8 MiB or 64 KiB. gc
  v1.5.1 buffers stdout whole with no cap
  (`internal/runtime/exec/exec.go:94-95,104`: in-memory buffers and `TrimRight`;
  parity §peek deviation 6 agrees). Ordinary buffered ops have a 30 s timeout;
  `start` gets 120 s, `attach` has no gc timeout, and `watch-startup` has a
  separate lifetime (`internal/runtime/exec/exec.go:53-64,196-202,223,383-393`).
- **E. `attach` deadline.** Pack: 120 s `start` class
  (`herdrcore/ops_table.py:88`). gc: none, it holds the terminal
  (`docs/reference/exec-session-provider.md:116-117`;
  `internal/runtime/exec/exec.go:196-201`). Parity §attach agrees on the
  mismatch.
- **F. Exit 80 under gc v1.5.1.** gc maps it to a generic error carrying
  stderr (`internal/runtime/exec/exec.go:162-169`); `nudge` and the other
  driving ops fail rather than reporting an outcome (parity rows `nudge`,
  `send-keys`, `interrupt`, `clear-scrollback`).

### Unverified

- Handler deadline/cancel polling by inspection: `hc` client calls use
  `Deadline.budget` and `CancelToken` (`hc/client.py:364-371`); `watch-startup`
  and `start` readiness/agent-wait loops check `deadline.budget` and
  `cancel.pause` (`herdrcore/screen.py:35`, `herdrcore/lifecycle.py:1004-1111`);
  reap pauses use `cancel.pause` only (`herdrcore/lifecycle.py:348`,
  `rpp/cancel.py:66-74`); filesystem staging and `copy-to` perform no
  cancellation polling (`herdrcore/staging.py:216-235`); and `attach`'s
  blocking `flock` has no deadline or cancel poll (`herdrcore/attach.py:126`).
  Exact worst-case elapsed time across operations remains `unverified`.
- Whether gc ever sends `GC_EXEC_DEADLINE_MS`: none in v1.5.1; later gc is
  unread (the live tree is ahead of the pin and is not used).
- Whether `TEMPFAIL` (75) is used by a newer pack or only reserved.
- Whether non-redaction behaviour or session-env rules beyond the shared
  marker list and assignment pattern match other gc paths; `rpp/redact.py:35-56`
  and `internal/execenv/execenv.go:24,59-73` share the same markers and pattern
  text with differing replacement markers (`[REDACTED]` vs `[redacted]`).
- gc stdin for ops with `stdinData == nil`: Go `os/exec` gives `/dev/null` when
  `cmd.Stdin` is nil (`internal/runtime/exec/exec.go:98-100` only sets it when
  non-nil); this is Go stdlib behaviour, not in either source – `unverified`
  as a statement about gc.
- Closed-pipe `emit` behaviour: `OSError` / `BrokenPipeError` in `emit` reaches
  the generic handler exception path (exit 1 `internal error`,
  `rpp/perop.py:122-124,153-156`) as derived above; no dedicated test was found,
  and partial physical-write behaviour remains `unverified`.
- `x-*` ops: no hit for `x-version|x-doctor|x-command|x-kick` in gc v1.5.1
  `internal/`, `cmd/` or `docs/reference/` (`rg`), so gc does not send them;
  they are operator and `gc` pack commands (`main.py:3-6`).

## protocol

### Registration

- Table entry: `"protocol": handshake_op(HANDSHAKE)` at
  `herdrcore/ops_table.py:68` (line 67 is `table: Dict[...] = {`). Parity
  §protocol cites `herdrcore/ops_table.py:68`, which matches the pin.
- `handshake_op` builds `OpSpec(handler=protocol, reads_stdin=False,
  deadline="default", stdout_cap=SMALL_STDOUT_CAP)` (`rpp/ops.py:109-116`).
  `SMALL_STDOUT_CAP = 64 << 10` = 65536 bytes (`rpp/ops.py:32`). The deadline
  class `default` is 30 s (`rpp/ops.py:23-27`); `GC_EXEC_DEADLINE_MS`
  overrides it (`rpp/perop.py:31,129`).
- The encoded line is built once, when the spec is built (`rpp/ops.py:111`),
  not per call.
- **The `herdrcore/ops_table.py` entry is not the live path for `protocol`.**
  `main.py:30` builds `PROTOCOL_ONLY = {"protocol": handshake_op(HANDSHAKE)}`
  and `ops_for` returns it for `op == "protocol"` (`main.py:33-36`), so
  `herdrcore.ops_table`, `build_ops` and `live_host` are never imported
  (`main.py:37-40`). Test: `tests/unit/test_herdr_core_entrypoint.py:201-208`
  (`test_protocol_loads_no_backend_code`). The `herdrcore/ops_table.py` entry is a
  duplicate for table completeness. The parity doc does not mention the
  `main.py:30-36` path.

### Inputs

- argv: `gc-runtime-herdr-core protocol` (`main.py:3`, `main.py:63-66`). gc runs
  exactly `script protocol` (`internal/runtime/exec/handshake.go:24`). Extra
  args would be passed in `OpRequest.args` (`rpp/perop.py:134`) and ignored by
  the handler (`rpp/ops.py:113-114`). A missing op name gives usage, exit 2
  (`rpp/perop.py:112-115`).
- stdin: not read (`reads_stdin=False`; `rpp/perop.py:135` yields `b""`). gc
  passes `nil` stdin data (`internal/runtime/exec/handshake.go:24`;
  `internal/runtime/exec/exec.go:98-99` sets `cmd.Stdin` only when non-nil, so
  the child gets the null device – Go `os/exec` behaviour, `unverified` in
  this tree).
- Environment: the handshake handler reads none. The runner snapshots the
  environment for redaction and reads `GC_EXEC_DEADLINE_MS`
  (`rpp/perop.py:109-111`, `rpp/perop.py:129`). `main.py` reads `GC_HERDR_CORE_DEBUG`
  (`main.py:27`, `main.py:71`). Test
  `test_protocol_reads_no_state_and_ignores_the_callers_python_environment`
  (`tests/unit/test_herdr_core_entrypoint.py:121-132`) proves the op ignores
  `PYTHONPATH`, `GC_EXEC_STATE_DIR` and `HOME`, and writes no files.
- Preconditions: none on the pack side. On gc's side the handshake is lazy,
  not "first": `Provider.Protocol()` runs it once per provider via `sync.Once`
  and caches value and error (`internal/runtime/exec/handshake.go:16-21`;
  fields at `internal/runtime/exec/exec.go:33-39`). It fires on the first
  capability probe: `IsAttached` (`internal/runtime/exec/exec.go:685`),
  `Capabilities()` (`internal/runtime/exec/exec.go:823-826`), and the `exec`
  and provision gates (`internal/runtime/exec/exec.go:278-279`, `internal/runtime/exec/exec.go:901`).
  `NewProvider` itself runs nothing (`internal/runtime/exec/exec.go:53-59`).
  `gc doctor` pack-runtimes calls it directly
  (`cmd/gc/doctor_pack_runtimes.go:60`). The 30 s `timeout` applies
  (`internal/runtime/exec/exec.go:53-59`, `internal/runtime/exec/exec.go:62-64`).

### Success output

- Exit 0, stdout one line plus `\n`, stderr empty (`rpp/ops.py:73-75`; test
  `tests/unit/test_herdr_core_entrypoint.py:85-90`).
- Shape built by `Handshake.encode` (`rpp/wire.py:61-65`): keys in order
  `version` (always `0`), `capabilities` (array, pack order), and `locality`
  (only when set; herdr sets `"local"`). Compact JSON, ASCII
  (`rpp/jsonval.py:29-35`). Rendered from the pinned tree (210 bytes plus
  `\n`):

  ```json
  {"version":0,"capabilities":["observe.v1","report-attachment","report-activity","city-scoped","conformance-faults","server-lifecycle","carrier.ops","idle-boundary","input-outcome","fs.host"],"locality":"local"}
  ```

- Source of the values: `handshake.py:33-47`. Pinned by the test `HANDSHAKE`
  dict (`tests/unit/test_herdr_core_entrypoint.py:67-82`) and, for the encoding
  form, by `tests/unit/test_rpp_perop.py:61-66`.
- Not declared, and tested as such: `proc.exec`, `proc.provision`,
  `input-fence`, `tty.attach`, `proc.stream` (`handshake.py:13-15`;
  `tests/unit/test_herdr_core_entrypoint.py:103-108`).
- gc decoder: `json.Unmarshal` into `ProtocolInfo{Version int "version";
  Capabilities []string "capabilities,omitempty"}`
  (`internal/runtime/protocol.go:61-64`;
  `internal/runtime/exec/handshake.go:31-34`), then `Validate`, which rejects a
  negative version (`internal/runtime/protocol.go:77-81`;
  `internal/runtime/exec/handshake.go:35-37`). `locality` has no field, so Go
  drops it. gc stdout is `TrimRight "\n"`
  (`internal/runtime/exec/exec.go:104`).
- Keys gc v1.5.1 acts on: `version` (only the `<0` check; it is never compared
  to 0) and `capabilities`. The capability strings it consumes are six:
  `report-attachment`, `report-activity`, `proc.exec`, `proc.stream`,
  `tty.attach`, `proc.provision` (`internal/runtime/protocol.go:21-55`). From
  the herdr line gc reads two: `report-attachment` (gates `is-attached`,
  `internal/runtime/exec/exec.go:685`; `docs/reference/exec-session-provider.md` "Capabilities"
  row 1) and `report-activity` (sets `CanReportActivity`,
  `internal/runtime/exec/exec.go:824`). Unknown strings are ignored by design
  (`internal/runtime/protocol.go:15-16`; `docs/reference/exec-session-provider.md` "Protocol
  Handshake"; test `internal/runtime/exec/handshake_test.go:91-102`).

### Failure cases

Generic envelope failures are in the shared contract. The rows below are the
ones that matter for `protocol`.

| Case | Exit | stdout | Source |
|---|---|---|---|
| Normal answer | 0 | handshake line | `rpp/perop.py:160-163`, `rpp/perop.py:170` |
| stdout over 64 KiB | 1 | nothing (stderr `protocol: answer of N bytes is over the 65536-byte cap`) | `rpp/perop.py:158-159`. Unreachable: the line is 211 bytes. |
| Pipe to gc closed on write | 1 | nothing | `rpp/perop.py:164-165`; test `tests/unit/test_rpp_perop.py:277-291` |
| SIGTERM or deadline before the handler runs | 1 | nothing | `rpp/perop.py:143-145`, `rpp/perop.py:149-150`. The handler is a constant return, so mid-handler expiry is moot, but entry deadline failure is straightforward via `GC_EXEC_DEADLINE_MS=0` (`rpp/perop.py:39-47,144-145`). |
| Unexpected exception in the handler | 1 | nothing, stderr `protocol: internal error: ...` | `rpp/perop.py:153-154` |
| Unknown op (for example `exec`, `provision`, `frobnicate`) | 2 | empty, stderr `<op>: unknown operation`, stdin untouched | `rpp/perop.py:117-120`; tests `tests/unit/test_rpp_perop.py:69-76` and `tests/unit/test_herdr_core_entrypoint.py:58-59`. Any non-`protocol` op loads the full table via `build_ops` (`main.py:37-40`); a name absent there gets this exit 2. |
| gc: exit 2 from `protocol` | gc treats it as success; empty output means version 0 and no capabilities | – | `internal/runtime/exec/exec.go:153-160`, `internal/runtime/exec/exec.go:174-181` (only `start` and `provision` fail on 2); `internal/runtime/exec/handshake.go:28-30`. Not a herdr-core outcome (it always answers). |
| gc: exit 0 with empty stdout | version 0, no capabilities | – | `internal/runtime/exec/handshake.go:28-30` |
| gc: non-JSON or wrong-typed JSON | `Protocol()` returns error `exec provider <script> protocol: invalid handshake JSON: ...`, cached | – | `internal/runtime/exec/handshake.go:31-34`. Probes degrade to no capabilities (`internal/runtime/exec/handshake.go:45-51`); doctor pack-runtimes reports failure (`cmd/gc/doctor_pack_runtimes.go:60-62`); test `internal/runtime/exec/handshake_test.go:104-116`. |
| gc: `version < 0` | error, cached | – | `internal/runtime/protocol.go:77-81`; `internal/runtime/exec/handshake.go:35-37`; test `internal/runtime/exec/handshake_test.go:118-124` |
| gc: other non-zero exit (1, 75 ...) | error wrapping stderr, cached | – | `internal/runtime/exec/exec.go:153-171`; test `internal/runtime/exec/handshake_test.go:126-142` |
| gc: 30 s timeout or cancel | error, cached | – | `internal/runtime/exec/exec.go:53-59`, `internal/runtime/exec/exec.go:79-92`, `internal/runtime/exec/exec.go:123-135` |
| gc: stdout cap | none; output is buffered in a `bytes.Buffer` | – | `internal/runtime/exec/exec.go:94-95` |

The handshake error is never retried within one provider instance
(`internal/runtime/exec/handshake.go:17-20`; test
`internal/runtime/exec/handshake_test.go:144`, named
`HandshakeRunsOncePerProvider`).

### Conflicts

Pack vs gc v1.5.1; not resolved.

1. Capabilities the pack declares that gc v1.5.1 does not know:
   `observe.v1`, `city-scoped`, `conformance-faults`, `server-lifecycle`,
   `carrier.ops`, `idle-boundary`, `input-outcome`, `fs.host` (8 of 10). A
   search of the v1.5.1 Go code for these strings finds no consumer. Effect:
   ignored (`internal/runtime/protocol.go:15-16`). Matches parity §protocol
   row 1 (verdict "intentional").
2. `locality` is not a field of gc v1.5.1's `ProtocolInfo`
   (`internal/runtime/protocol.go:61-64`) and is not in the doc's handshake
   example (`docs/reference/exec-session-provider.md` "Protocol Handshake"). Go drops it
   silently. Parity row 2 agrees. `fs.host`'s dependency on `locality`
   therefore does nothing on v1.5.1; the pack says it needs a newer gc
   (`handshake.py:20-22`).
3. The pack docstring says "gc's process-table cleanup ... `proc.exec` without
   `carrier.ops`" (`handshake.py:16-20`). gc v1.5.1 has no `carrier.ops` token,
   so under v1.5.1 the pack's `carrier.ops` gives no fencing. `unverified`
   beyond the symbol search.
4. The pack's `handshake.py:6-9` describes `carrier.ops` as "gc drives the box
   only through the pack's own `nudge`...". Under v1.5.1 gc drives those ops
   regardless of the token, because the absence of `proc.exec` is what selects
   the dedicated ops (`docs/reference/exec-session-provider.md` capability row `proc.exec`,
   "without it, gc uses the dedicated driving ops"). Consistent, but the token
   adds nothing on v1.5.1.
5. Registration line: parity cites `herdrcore/ops_table.py:68`, and the pin
   puts the entry there. The live path is `main.py:30-36`, which the parity
   doc does not mention.
6. Parity row 3 says it is unverified whether herdr requires a session name.
   Source answer: no – the handler ignores args and a bare `protocol` works
   (`rpp/perop.py:112-120`, `rpp/ops.py:113-114`; test
   `tests/unit/test_herdr_core_entrypoint.py:85-86`). This closes row 3's
   unverified clause for the pack; the Rex side is separate.

### Unverified

- That Go's `os/exec` gives the child the null device when `cmd.Stdin` is nil
  (standard library behaviour, not in the gc tree).
- Exit and output if the Python interpreter cannot start or
  `gc_runtime_herdr_core` fails to import (outside the cited files; the entry
  script is `ENTRY` in the tests and was not read).
- Whether any gc newer than v1.5.1 reads `locality` or the eight unknown tokens
  (not in the pinned tree; the pack docstrings at `handshake.py:20-22` say so,
  which is a pack claim, not a gc fact).

Note (confirmed static construction): `Handshake.encode` hardcodes version `0`
and `HANDSHAKE` hardcodes the capability tuple (`rpp/wire.py:62`,
`handshake.py:33-47`), so negative-version or mistyped-capabilities output cannot
occur from this pack.

## start

### Registration

- `"start": spec(lifecycle.op_start, stdin=True, deadline="start")` –
  `herdrcore/ops_table.py:69`. `spec()` defaults: `cap=SMALL_STDOUT_CAP`,
  `housekeeping=True` (`herdrcore/ops_table.py:64`).
- `reads_stdin=True` – the transport reads all of stdin before calling the
  handler (`rpp/perop.py:135`), polling so SIGTERM and the deadline can
  interrupt (`rpp/perop.py:60-80`; tests `tests/unit/test_rpp_perop.py:135,371`).
- Deadline class `start` = 120 s (`rpp/ops.py:23-27`), counted from the entry
  script's pre-pack timestamp `_STARTED` (`bin/gc-runtime-herdr-core:12`,
  `rpp/ops.py:119-128`); `GC_EXEC_DEADLINE_MS` overrides it
  (`rpp/perop.py:31,129`), but gc 1.5.1 never sets it (no `cmd.Env` in
  `internal/runtime/exec/exec.go`; parity §start row 1). gc side:
  `startTimeout: 120 * time.Second` (`internal/runtime/exec/exec.go:57`), used by
  `Start` (`internal/runtime/exec/exec.go:223`).
- Stdout cap `SMALL_STDOUT_CAP` = 64 KiB (`rpp/ops.py:32`); enforced at
  `rpp/perop.py:158-159`. Not reachable: `start` never prints to stdout (see
  "Success output").
- Housekeeping: `sweep_spawn_files` runs first
  (`herdrcore/ops_table.py:45-46`; errors swallowed,
  `herdrcore/lifecycle.py:741-746`).
- `_wrap` maps `StateDirError`, `ConfigError`, `MetaError` and `LockFileError`
  to `OpError` (exit 1) and `LockTimeout` to `busy: ...`
  (`herdrcore/ops_table.py:48-51`).
- Parity §start verdict: **intentional (with a mistake inside)**; status
  partially implemented.

### Inputs

**argv:** `<script> start <name>` (`internal/runtime/exec/exec.go:223`:
`"start", name`; `docs/reference/exec-session-provider.md:127`). The pack
reads only `args[0]`; empty or missing gives `OpError("missing session name")`
(`herdrcore/reads.py:58-59`, called first at `herdrcore/lifecycle.py:764`).
Extra args are ignored.

**stdin:** one JSON object. gc v1.5.1 `startConfig` sends 15 fields, all
`omitempty` (`internal/runtime/exec/json.go:26-42`); the pack parser
`StartConfig` reads 17 (`rpp/wire.py:80-99`, `parse_start_config`
`rpp/wire.py:102-129`).

| field | gc 1.5.1 sends | pack reads | pack uses it |
|---|---|---|---|
| `work_dir` | `internal/runtime/exec/json.go:27` | `rpp/wire.py:107` | staging root (`herdrcore/staging.py:242-257`), existence check (`herdrcore/lifecycle.py:839-840`), spawn file and cwd (`herdrcore/lifecycle.py:853,860`), meta (`herdrcore/lifecycle.py:843`), setup cwd (`herdrcore/lifecycle.py:612-613`) |
| `command` | `internal/runtime/exec/json.go:28` | `rpp/wire.py:108` | spawn file `command`; launched as `/bin/sh -c`, or the login shell when empty (`herdrcore/envplan.py:118-123`, `herdrcore/lifecycle.py:852`) |
| `lifecycle` | `internal/runtime/exec/json.go:29` (only `"one_shot"`) | `rpp/wire.py:109` | `== one_shot` skips the "agent exited" failures (`herdrcore/lifecycle.py:161,1030-1031,1123-1124`) |
| `env` | `internal/runtime/exec/json.go:30` | `rpp/wire.py:110` (`str_map`) | see the env map below |
| `process_names` | `internal/runtime/exec/json.go:31` | `rpp/wire.py:111` | `_await_agent` waits up to 30 s (`herdrcore/lifecycle.py:1010-1011,1004-1024`) |
| `nudge` | `internal/runtime/exec/json.go:32` | `rpp/wire.py:112` | first turn after readiness (`herdrcore/lifecycle.py:819-822,1113-1133`) |
| `ready_prompt_prefix` | `internal/runtime/exec/json.go:33` | `rpp/wire.py:113` | readiness and dialog screen checks (`herdrcore/lifecycle.py:1078,1106-1107`) |
| `ready_delay_ms` | `internal/runtime/exec/json.go:34` | `rpp/wire.py:114` | readiness delay, capped at 60 s (`herdrcore/lifecycle.py:1064`) |
| `pre_start` | `internal/runtime/exec/json.go:35` | `rpp/wire.py:115` | run before the box exists, **fatal** (`herdrcore/lifecycle.py:793-796`) |
| `session_setup` | `internal/runtime/exec/json.go:36` | `rpp/wire.py:116` | run after launch, warnings only (`herdrcore/lifecycle.py:811-814`) |
| `session_setup_script` | `internal/runtime/exec/json.go:37` | `rpp/wire.py:117` | same (`herdrcore/lifecycle.py:815-818`) |
| `session_live` | `internal/runtime/exec/json.go:38` | `rpp/wire.py:118` | never run; one warning `SESSION_LIVE_SKIPPED` (`herdrcore/lifecycle.py:162-164,823-824`) |
| `pack_overlay_dirs`, `overlay_dir`, `copy_files[{src,rel_dst}]` | `internal/runtime/exec/json.go:39-41` | `rpp/wire.py:119-124` | staged into `work_dir` before `pre_start` (`herdrcore/lifecycle.py:789`, `herdrcore/staging.py:238-258`) |
| `fresh_only` | **no** | `rpp/wire.py:126` | `_check_name` gives exit 73 (`herdrcore/lifecycle.py:755-756,784,835`) |
| `work_dir_exclude` | **no** | `rpp/wire.py:125` | read, never used (only `rpp/wire.py:98,125` mention it; `rg` over the pack) |

Unknown fields are ignored (`tests/unit/test_rpp_wire.py:98`); wrong types are
refused naming the field, never the value (`tests/unit/test_rpp_wire.py:116-122`;
`rpp/jsonval.py:80-83`). Everything is optional except `copy_files[].src`
(`rpp/wire.py:122`; `tests/unit/test_rpp_wire.py:89-95,115`).

**`env` map semantics.** gc: "An empty value means the variable must be unset
... the script itself runs with gc's environment, so a script that starts the
agent as its own child must apply those empty values too"
(`docs/reference/exec-session-provider.md:269-273`); `omitempty` drops only an
empty *map* (`internal/runtime/exec/json.go:30`), so empty-string values reach
the script. Pack, agent env: a value sets, `""` pops (`herdrcore/envplan.py:67-71`;
spawn file `herdrcore/launch.py:68`; test
`tests/unit/test_herdrcore_launch.py:201-210`). Pack, setup hooks: the same
rule on gc's environment minus `HC_SCOPE`, `HC_SOCKET` and `HC_SESSION`
(`herdrcore/lifecycle.py:599-609`, `herdrcore/envplan.py:41`). gc's `HOME` (the
op environment) is added when `env` has no `HOME` key (`herdrcore/envplan.py:51-58`,
applied `herdrcore/lifecycle.py:851`; test
`tests/unit/test_herdrcore_lifecycle.py:75`). The identity seed keeps only
non-empty `GC_SESSION_ID`, `GC_RUNTIME_EPOCH`, `GC_INSTANCE_TOKEN` and
`GT_PROCESS_NAMES`, plus `GC_BIN` (`rpp/wire.py:43`,
`herdrcore/metastore.py:88-91`). Values are registered for stderr redaction
(`herdrcore/lifecycle.py:769`, `rpp/redact.py:87-96`). Parity row 9 records the
same "empty = unset" rule for the Rex provider.

**Preconditions.** A name. No existing session: a live intact incarnation of
the name is a failure (see the table); finished or broken records are recycled
first, tree-killed and removed (`herdrcore/lifecycle.py:757-760,787,488-505`).
A non-empty configured `work_dir` need not exist before staging and `pre_start`,
but must be a directory when the box is created (`herdrcore/lifecycle.py:839-840`;
`tests/unit/test_herdrcore_staging.py:322`); an empty `work_dir` is accepted and
run cwd falls back to home (`herdrcore/lifecycle.py:860`).

**Environment variables read by the op** (the op environment is gc's own; gc
sets none for the child): `GC_EXEC_CITY_PATH` (city key;
`rpp/statedir.py:39,55-58`; gc 1.5.1 sets it nowhere findable, see
Unverified), `GC_EXEC_DEADLINE_MS` (`rpp/perop.py:31`), `HOME`
(`herdrcore/envplan.py:48-58`), `GC_AGENT_SLICE` (`herdrcore/lifecycle.py:630`),
`XDG_RUNTIME_DIR` (`herdrcore/lifecycle.py:689`, Linux only), and
`GC_RPP_FAULT` (`herdrcore/context.py:34,88`; `unavailable` fails `start`,
`herdrcore/core.py:361-362`). Test-only under `GC_HC_TEST=1`:
`GC_HC_TEST_SPAWN_ROOT` (`herdrcore/lifecycle.py:737`), `GC_HC_TEST_USERSCOPE`
and `GC_HC_TEST_SYSTEMD_RUN` (`herdrcore/lifecycle.py:198-200`). From the
*start config* env: `GC_PROVIDER` (staging, `herdrcore/lifecycle.py:789`),
`GC_BIN` (`herdrcore/lifecycle.py:1010`) and `GC_AGENT_SLICE`
(`herdrcore/lifecycle.py:630`).

### Success output

- Exit **0**, stdout **empty** (`OpResult(ExitCode.OK, b"", ...)`,
  `herdrcore/lifecycle.py:825`). gc's op table shows a dash for the stdout of `start`
  (`docs/reference/exec-session-provider.md:127`) and discards it
  (`internal/runtime/exec/exec.go:223`, `_, err =`).
- Warnings (ignored hooks, unconfirmed first turn, staging skips,
  `session_live`, scope fallback, survivor-abandon) go to stderr as
  `start: <w1>; <w2>` with exit 0 (`herdrcore/lifecycle.py:825`,
  `rpp/perop.py:166-167`). gc drops stderr on success
  (`internal/runtime/exec/exec.go:102-104` returns stdout only). Parity row 16
  records the Rex side as "none".
- After exit 0 gc itself dismisses startup dialogs when `process_names` is set
  or the provider declares it (`internal/runtime/exec/exec.go:227`, `internal/runtime/exec/exec.go:343-382`;
  `internal/runtime/startup_hints.go:20-25`). The pack withholds the first turn
  while a dialog shows (`herdrcore/lifecycle.py:1079-1080`).
- Success order (`herdrcore/lifecycle.py:778-825`): `ensure_core`; name lock,
  list, `_check_name`, recycle; unlock; stage; `pre_start`; lock, recheck,
  work_dir check, write seed meta, shims, spawn file (0600,
  `herdrcore/spawnfile.py:148`), `terminal.run` launcher, label pid; wait for
  the agent (5 s, or 30 s with `process_names`); `_fail_if_ended`;
  `session_setup*`; first turn; `session_live` warning.

### Failure cases

All failures: stdout empty, stderr `start: <redacted message>`
(`rpp/perop.py:147-167`), exit 1 unless noted. Redaction: `rpp/perop.py:167`.

| Case | Exit | stdout | Source |
|---|---|---|---|
| name missing or empty | 1 `missing session name` | empty | `herdrcore/reads.py:58-59` |
| stdin still open at the deadline | 1 `stdin was still open at the op's deadline` | empty | `rpp/perop.py:75` |
| empty, non-JSON, non-object, trailing data, mistyped field, `copy_files[i]` without `src` | 1 `invalid start config: <field path>: ...` | empty | `rpp/wire.py:103,129`; `rpp/jsonval.py:25,65,82-83`; `herdrcore/lifecycle.py:767-768`; `tests/unit/test_rpp_wire.py:116-125` |
| `GC_RPP_FAULT=unavailable`, hc binary not resolvable, foreign or unreadable core, core won't start or answer, no time to start the core | 1 | empty | `herdrcore/core.py:355,361-362,281,292,300,339-344` |
| other hc error (timeout, connection lost, rpc, protocol, no time left) | 1 `cannot start 'N': <err>` | empty | `herdrcore/lifecycle.py:772-775`; `hc/client.py:368-370` |
| live intact box of that name | 1 `session "N" already exists` (gc collision phrase) | empty | `herdrcore/lifecycle.py:759`; hc `name_taken` gives the same text (`herdrcore/lifecycle.py:871-872`); `tests/unit/test_herdrcore_lifecycle.py:110-113` |
| `fresh_only` and any record exists | **73** `... already exists (fresh_only)` | empty | `herdrcore/lifecycle.py:755-756`; `rpp/exitcodes.py:24`; gc never sends `fresh_only` |
| earlier incarnation's processes may survive (up to 15 min) | 1 `cannot start 'N' while an earlier incarnation's processes may survive ...` | empty | `herdrcore/lifecycle.py:498-505,187` |
| staging error (relative or empty `work_dir` with staging fields, symlink in destination, OSError) | 1 | empty | `herdrcore/staging.py:243,257`; `herdrcore/lifecycle.py:790-791` |
| `pre_start[i]` exits non-zero, times out (30 s each, cut to deadline - 5 s) or has no time | 1 `pre_start[i] exited N: <last line>` | empty | `herdrcore/lifecycle.py:168,616-624,793-796`; `tests/unit/test_herdrcore_staging.py:199-201` |
| `work_dir` not a directory after staging and `pre_start` | 1 | empty | `herdrcore/lifecycle.py:839-840`; `tests/unit/test_herdrcore_lifecycle.py:504-507` |
| name lock busy (waits min(30 s, deadline budget)) | 1 `busy: ...` | empty | `herdrcore/context.py:48,152-159`; `herdrcore/ops_table.py:50-51` |
| state-dir, config, meta, lock-file or spawn-dir error | 1 (`StateDirError` etc.); another `OSError` from the spawn file gives `internal error: ...` | empty | `herdrcore/ops_table.py:48-49`; `herdrcore/spawnfile.py:125,148`; `rpp/perop.py:153-154`; created box rolled back `herdrcore/lifecycle.py:887-891` |
| `terminal.run` fails, or the reply is lost with nothing found | 1 `cannot start 'N': ...`; spawn file, terminal and meta rolled back (`_undo`) | empty | `herdrcore/lifecycle.py:868-891,919-970`; `tests/unit/test_herdrcore_lifecycle.py:410` |
| launcher cannot read the spawn file or exec the agent (exit 127, message on the terminal screen), so the box is dead | 1 `the agent exited during startup (exit code 127)`; the dead box stays for gc's `stop` | empty | `herdrcore/launch.py:24,66-67,83-84`; `herdrcore/lifecycle.py:798-800,1027-1036` |
| agent exits during startup (not one-shot) | 1 `the agent exited during startup (exit code N / signal S)`; with `lifecycle=one_shot`, exit 0 | empty (exit 1 case) | `herdrcore/lifecycle.py:1030-1036`; `tests/unit/test_herdrcore_lifecycle.py:578-582` |
| agent ends before its first turn (not one-shot) | 1 `the agent exited during startup, before its first turn` | empty | `herdrcore/lifecycle.py:1122-1125` |
| SIGTERM or cancel (gc timeout or abandon) | 1 `canceled (terminated)` (`op_start` and `_record_refusal` re-raise `HcCancelled` as `Cancelled`, `herdrcore/lifecycle.py:472,772-774`); before launch the spawn file, terminal and meta are removed; after launch the box stays | empty | `rpp/perop.py:50-57,149-150`; `herdrcore/lifecycle.py:772-774,798-800`; `tests/unit/test_herdrcore_lifecycle.py:475-480`, `tests/unit/test_herdrcore_launch.py:304` |
| deadline reached | no pack-side hard abort: inner waits are cut with `deadline.budget` (`herdrcore/lifecycle.py:617,1012,1060`; `rpp/cancel.py:98-103`, 1 s margin; the first turn reserves 5 s); `no time left before the op's deadline` at entry (`rpp/perop.py:144-145`); gc cancels at 120 s, which is the row above. gc reports cancellation whatever the exit code and skips the collision check (`internal/runtime/exec/exec.go:114-115`; `docs/reference/exec-session-provider.md:98-99,303-304`) | empty | see Unverified 3 |
| stdout over cap | n/a, stdout is always empty | – | `rpp/perop.py:158-159` |
| unexpected exception | 1 `internal error: <Type>: <msg>` | empty | `rpp/perop.py:153-154` |
| exit 2 | never produced by `start`; gc would read it as a failure for `start` | – | `internal/runtime/exec/exec.go:153-180`; `docs/reference/exec-session-provider.md:77,82-85` |

Non-fatal (exit 0 plus a stderr warning): `session_setup[i]` or
`session_setup_script` failure, an undelivered or withheld first turn (which
also writes `GC_STARTUP_DELIVERY_UNCONFIRMED` into meta), `session_live`,
scope fallback, and abandoned survivors
(`herdrcore/lifecycle.py:811-824,1136-1146,694-698,495-497`).

On any failed start gc runs `stop <name>` unless `is-running` said `true`
beforehand or stderr says `already exists` or `already running`
(`internal/runtime/exec/exec.go:262-270,144`;
`docs/reference/exec-session-provider.md:298-307`); the pack's post-launch
failures rely on that (`herdrcore/lifecycle.py:798-800`).

### Conflicts

Pack vs gc v1.5.1; not resolved.

1. **`pre_start` is fatal in the pack, non-fatal in gc's spec** (pack vs gc
   v1.5.1 conflict). Doc: "Non-fatal: warn on stderr if a command fails, but
   don't abort start" (`docs/reference/exec-session-provider.md:328-333`). Pack:
   `raise OpError("pre_start[i] ...")` (`herdrcore/lifecycle.py:793-796`, test
   `tests/unit/test_herdrcore_staging.py:199-201`). The Rex provider follows
   gc's contract: a failing or timed-out hook warns and the start continues;
   only whole-start cancellation aborts (GCREX-54; parity §start row 11). The
   pack still follows its fatal contract.
2. **Extra inputs gc 1.5.1 never sends** (version skew): `fresh_only` (gives
   exit 73) and `work_dir_exclude` (unused). Parity rows 2 and 5 agree on
   `fresh_only`; `work_dir_exclude` is not in the parity doc (it counts 17 fields
   at `rpp/wire.py:80-99`, correct only as a count of parsed fields). Exit 73 has
   no meaning to gc 1.5.1; any non-0/1/2 code is a plain error and the "already
   exists" text still matches the collision detector
   (`internal/runtime/exec/exec.go:144,185-193`).
3. **`fs.host` assumption** (version skew / compatibility). The pack docstrings
   say gc "leaves those fields out" and the pack does nothing for them
   (`herdrcore/lifecycle.py:19-26`, `herdrcore/staging.py:1-8`). gc 1.5.1 parses
   six tokens and ignores `fs.host` (`internal/runtime/protocol.go:18-58`,
   parity §protocol), and `marshalStartConfig` always sends the fields
   (`internal/runtime/exec/json.go:50-66`). So at v1.5.1 the pack does the
   staging, `pre_start` and `session_setup` itself (the "older gc" path,
   `herdrcore/staging.py:1-8`). Consistent, not a bug, but the pack's default
   narrative does not match this gc.
4. **Readiness and startup dialogs** (pack-vs-gc compatibility / Rex comparisons):
   - **The pack types the nudge itself and waits up to 60 s for readiness.** gc:
     `nudge` is typed by the script once the agent is ready
     (`docs/reference/exec-session-provider.md:321-326`);
     `ready_prompt_prefix` and `ready_delay_ms` are sent but gc does not wait on
     them (`docs/reference/exec-session-provider.md:354-357`). The pack waits
     (`herdrcore/lifecycle.py:171,1051-1111`). Matches the gc doc; parity row 15
     notes Rex waits for `program_status` idle and ignores `ready_prompt_prefix`
     (`intentional`).
   - **Startup dialogs.** The pack withholds the first turn while a dialog shows
     and records `GC_STARTUP_DELIVERY_UNCONFIRMED`
     (`herdrcore/lifecycle.py:1079-1080,1126-1127,1136-1146`); gc dismisses
     dialogs only after `start` returns (`internal/runtime/exec/exec.go:227`), so
     a withheld turn is never typed later (the pack states it,
     `herdrcore/lifecycle.py:61-64`). Parity row 15 notes the Rex provider does
     not withhold.
   - **Warnings are invisible to gc** (stderr on exit 0 is dropped,
     `internal/runtime/exec/exec.go:102-104`; parity row 16).
5. **Rex-vs-pack comparisons and parity-row confirmations** (not gc conflicts):
   - **Agent liveness check after launch.** The pack fails `start` if the agent
     already exited (`herdrcore/lifecycle.py:1027-1036`); Rex does not (parity
     row 14, `not-implemented`).
   - **Tree-kill recycle and survivor refusal** (`herdrcore/lifecycle.py:488-505`)
     have no Rex equivalent (parity row 6, `intentional`).
   - **Lock scope.** The per-name lock is released across staging and
     `pre_start`, retaken and rechecked (`herdrcore/lifecycle.py:782-797,833-838`);
     Rex holds one city-wide flock (parity row 12, Rex `mistake`).
   - **Env hand-off.** The spawn file is 0600 and unlinked before parse
     (`herdrcore/launch.py:30-48`) vs an `env K=V` argv prefix in Rex (parity
     row 10, `intentional (limitation)`).
   - **Parity row 5 confirmation:** the pack prints `session "<n>" already exists`
     (`herdrcore/lifecycle.py:759,872`).
   - **Parity row 17 confirmation:** any non-JSON, including empty, fails
     (`tests/unit/test_rpp_wire.py:125`).

### Unverified

1. `GC_EXEC_CITY_PATH` (`rpp/statedir.py:39`): `rg` for `EXEC_CITY` in gc
   v1.5.1 found no setter. Whether gc's supervisor or a wrapper exports it
   (otherwise the city key is `UNSCOPED_CITY_KEY`, `rpp/statedir.py:55-58`) is
   not settled by either source.
2. Whether any real `hc` core reports instant agent death as a failed
   `terminal.run` rather than a dead record: only the fake core
   (`tests/fakehc`) was inspected.
3. Behaviour exactly at the 120 s boundary: the pack has no hard stop;
   `ctx.request.deadline` only shortens waits. Whether a slow `hc` call (up to
   `run` 15 s, `hc/client.py:131`) can push a start past gc's kill was not
   tested; gc's SIGTERM then triggers the rollback path (`rpp/perop.py:50-57`)
   with `ROLLBACK_S` 1.5 s (`herdrcore/lifecycle.py:181`) against gc's 2 s grace
   (`internal/runtime/exec/exec.go:92`).
4. Whether gc `fs.host` support exists in the working tree (the pack cites
   `gastownhall/gascity#9023`, `herdrcore/staging.py:5`); v1.5.1 has none (`rg
   fs.host` in the v1.5.1 tree: no Go matches).
5. Process cleanup of `pre_start` hook grandchildren on cancel: `run_bounded`
   starts a new session (`start_new_session=True`), kills the process group with
   `SIGKILL` in `finally`, and waits (`herdrcore/command.py:45-52,69-76`).
   Same-group cleanup is confirmed; detached or escaped grandchildren and real
   platform execution remain `unverified`. Parity row 11 flags the Rex side only.
6. The Linux-only scope path (`herdrcore/lifecycle.py:684-730`) was read, not
   run; `systemd-run` behaviour is unverified.
7. Town tab order (#47): the Rex provider moves a newly created town tab first
   in its canonical session, once, best-effort (parity row `start` / town tab
   order). It is Rex-only. Whether RPP v0 `start` carries any tab-order
   requirement is **unverified**: neither gc `v1.5.1` nor herdr-core `0f07496`
   mentions window or tab order (`git grep`/`grep` for
   `move_window|tab order|window order|reorder`, no match).

## stop

### Registration

- `"stop": spec(lifecycle.op_stop)` is at `herdrcore/ops_table.py:70`.
- `reads_stdin=False` (default `stdin=False`, `herdrcore/ops_table.py:64`;
  `OpSpec(... reads_stdin=stdin ...)` at `herdrcore/ops_table.py:65`). The
  runner reads stdin only when `spec.reads_stdin` (`rpp/perop.py:135`), so
  stdin is never touched.
- Deadline class `default` = 30 s (`herdrcore/ops_table.py:64`,
  `rpp/ops.py:23-24`), counted from `_STARTED` (`bin/gc-runtime-herdr-core:12`,
  `rpp/ops.py:119-128`).
  `GC_EXEC_DEADLINE_MS` overrides it (`rpp/perop.py:31`); gc 1.5.1 never sends
  it per the parity doc §watch-startup. gc's own timeout for `stop` is
  `p.timeout` = 30 s (`internal/runtime/exec/exec.go:56`; `Stop` uses `p.run`,
  `internal/runtime/exec/exec.go:581-583`).
- Stdout cap `SMALL_STDOUT_CAP` = 64 KiB (`herdrcore/ops_table.py:64`,
  `rpp/ops.py:32`). Housekeeping is on (`sweep_spawn_files` runs first,
  `herdrcore/ops_table.py:45-46`; its `OSError` and `StateDirError` are
  swallowed, `herdrcore/lifecycle.py:741-746`).
- Parity §stop verdict: **intentional**, status implemented; it covers the
  herdr-core path, kill, locks, cancellation, absent name and core-down.

### Inputs

- argv: `<script> stop <name>`. gc sends exactly `stop <name>`, no flags, no
  stdin (`internal/runtime/exec/exec.go:581-583`;
  `docs/reference/exec-session-provider.md:128`). No `--all` form exists in
  either source.
- The pack takes only `args[0]`; extra args are ignored
  (`herdrcore/reads.py:57-60`). A missing or empty name gives
  `OpError("missing session name")` (`herdrcore/reads.py:58-59`).
- Preconditions: none. No record is required. `list_city(client, ctx, name)`
  filters on city plus `gc.session` labels, including finished records
  (`herdrcore/reads.py:68-73`; `herdrcore/lifecycle.py:516`). Zero records make
  `_recycle` return `[]` (`herdrcore/lifecycle.py:426-427`), which is a success.
- gc contract: "`stop` must succeed (exit 0) even if the session doesn't exist"
  (`docs/reference/exec-session-provider.md:368-369`); `runtime.Provider.Stop`
  "Returns nil if the session does not exist"
  (`internal/runtime/runtime.go:162-165`).

### Success output

- stdout empty; exit 0 (`herdrcore/lifecycle.py:525` `OpResult.ok()` gives
  `b""` and `ExitCode.OK`, `rpp/ops.py:68-70`). stderr empty. gc discards stdout
  anyway (`internal/runtime/exec/exec.go:582` `_, err :=`).
- Absent name: exit 0, empty stdout
  (`tests/unit/test_herdrcore_lifecycle.py:118-125`; parity §stop row 3,
  `intentional`). A second stop likewise.
- Core unreachable and nothing of it runs: it sweeps by stored pid and token,
  starts no core, and exits 0 if nothing survives
  (`herdrcore/lifecycle.py:519-522,528-536`;
  `tests/unit/test_herdrcore_lifecycle.py:127-139`; parity §stop herdr-core
  paragraph).
- Success path (`herdrcore/lifecycle.py:511-525`): exclusive per-name lock,
  list records, `_recycle` (`herdrcore/lifecycle.py:420-446`), then per record
  `kill_incarnation` (`herdrcore/lifecycle.py:327-368`: snapshot; capture by
  subtree plus instance token; hc `kill`; reap TERM then KILL after 2 s,
  `herdrcore/procs.py:639-643`; rescan up to 3x, `herdrcore/lifecycle.py:183,318`;
  `_remove_record` waits up to 3 s for a "live" removal,
  `herdrcore/lifecycle.py:182,214-225`). Then `forget` drops meta and activity
  (`herdrcore/lifecycle.py:389-397,445`).
  `tests/unit/test_herdrcore_carrier.py:45-52` shows the meta key is gone after
  `stop`.

### Failure cases

All failures: stdout empty, stderr `stop: <message>` (redacted), via
`rpp/perop.py:147-166`. Exit codes other than 0 and 1 are never produced by
`stop`. Generic runner failures (deadline spent, SIGTERM, internal error) are in
the shared contract.

| Case | Exit | stdout | Source |
|---|---|---|---|
| empty or missing name | 1 | empty | `herdrcore/reads.py:58-59`; OpError to `rpp/perop.py:147-148` |
| hc unreachable AND `nothing_runs` false (holder or core process may run, process table unreadable, listener elsewhere, backend unavailable) | 1 | empty | `herdrcore/lifecycle.py:518-521`; `herdrcore/corescan.py:123-138`; also kicks a core start via `unreachable` (`herdrcore/reads.py:91-95`); `tests/unit/test_herdrcore_server.py:360-368` |
| hc unreachable AND nothing runs, sweep leaves survivors | 1 | empty | `herdrcore/lifecycle.py:522,523-524`; `herdrcore/server.py:329` |
| other hc error (timeout, connection lost, protocol, gate, RPC error from list, kill or remove, `HcCancelled`) | 1, message `cannot stop 'N': <err>` | empty | `herdrcore/lifecycle.py:517-521`; `hc/errors.py:56-107`. `kill` errors `not_found` and `finished` are ignored (`herdrcore/lifecycle.py:249-250`); another kill error is raised after the tree is reaped and the record stays (`herdrcore/lifecycle.py:362-363`). `remove` `not_found` is ignored, `live` is retried for 3 s then raised (`herdrcore/lifecycle.py:221-224`) |
| descendants may survive (unreadable process table, opaque subtree, still running after KILL, respawning after 3 rescans, fenced infrastructure) | 1, `stopped 'N', but descendants may survive (the box stays for a retry): …` | empty | `herdrcore/lifecycle.py:523-524`, `herdrcore/lifecycle.py:295,298,320`; test `tests/unit/test_herdrcore_treekill.py:576-596`. The record and meta are kept for retry |
| name lock held for the whole wait (exclusive, `min(30, max(0.1, budget(30)))` s) | 1, `busy: exclusive lock <path> still held after Xs` | empty | `herdrcore/context.py:152-159,48`; `rpp/flock.py:96-98`; `herdrcore/ops_table.py:50-51` |
| state dir, config, metastore or lock-file error | 1 | empty | `herdrcore/ops_table.py:48-49` (`StateDirError, ConfigError, MetaError, LockFileError`). `forget` swallows `MetaError` (`herdrcore/lifecycle.py:393-396`) |
| gc cancel (SIGTERM, then SIGKILL after 2 s) | 1, `canceled (terminated)` on direct `Cancelled` (e.g. reap pauses or pre-handler), or `cannot stop 'N': <err>` when `HcCancelled` is caught by `except HcError` (`herdrcore/lifecycle.py:517-521`, `hc/errors.py:107-112`) | empty | `rpp/perop.py:46-57,149-150`; `rpp/cancel.py:66-74` (pauses wake within 0.1 s); gc `internal/runtime/exec/exec.go:92` (`execgrace.Apply(cmd, 2*time.Second)`). A cancel mid-kill leaves a partial stop; the record stays unless already removed (parity §stop row 6 `gap`) |
| no time left before the deadline at handler entry | 1, `no time left before the op's deadline` | empty | `rpp/perop.py:143-145` |
| deadline reached mid-op | no pack-side stop: reap and pause waits check only the cancel token, not the deadline (`herdrcore/lifecycle.py:348`, `rpp/cancel.py:66-74`), and no post-handler timer is installed (`rpp/perop.py:142-159`); budgeted hc calls can still fail before/after reap, and gc cancels at 30 s (`internal/runtime/exec/exec.go:56,82-83`) | empty | external gc termination timing `unverified` in live run |
| unexpected exception | 1, `internal error: <Type>: <msg>` | empty | `rpp/perop.py:153-155` |
| stdout over cap | n/a: stdout is always empty | – | `rpp/perop.py:158-159` |
| unknown-op exit 2 | not reachable: `stop` is in the table | – | `herdrcore/ops_table.py:70` |

gc side: any non-zero exit other than 2 becomes `exec provider <script> stop
<name>: <stderr>` (`internal/runtime/exec/exec.go:162-169`). Exit 2 would be
read as success for `stop` (`internal/runtime/exec/exec.go:159-161,174-180`).
That does not apply here, but would also swallow a script's accidental exit 2.

### Conflicts

Pack vs gc v1.5.1; not resolved.

1. **Survivor failure vs idempotent-stop wording.** gc doc: `stop` "must
   succeed (exit 0) even if the session doesn't exist"
   (`docs/reference/exec-session-provider.md:368-369`);
   `internal/runtime/runtime.go:162-165` says "destroys the named session and
   cleans up its resources" (this does not prohibit operational errors for an
   existing session, nor promise universal success). The pack exits 1 with the
   record kept when descendants may survive (`herdrcore/lifecycle.py:523-524`),
   a pack failure-semantics choice; gc's caller treats exit 1 as an error
   (`internal/runtime/exec/exec.go:162-169`). Parity §stop overall Verdict is
   `intentional` (row 1 is `intentional` with a gap; row 2's verdict is
   `not-implemented` for the Rex survivor-outcome gap while its herdr column
   describes the pack's exit-1 failure).
2. **Core down with something running gives exit 1**
   (`herdrcore/lifecycle.py:519-521`;
   `tests/unit/test_herdrcore_server.py:360-368`): when the core is unreadable,
   the pack cannot tell whether a known-absent name is absent and fails closed.
3. **Core-start side effect on a failed stop.** `stop` kicks a core start on
   `HcUnreachable` (`herdrcore/reads.py:91-95`, via
   `herdrcore/lifecycle.py:520`). gc's docs say nothing about `stop` starting
   infrastructure (`unverified` as a conflict – no gc text forbids it).

Parity doc vs sources: no disagreement found on the claims re-checked (herdr
`op_stop` at `herdrcore/lifecycle.py:511`, the exit 1 text at
`herdrcore/lifecycle.py:523-524`, the per-name lock at
`herdrcore/lifecycle.py:515`).

### Unverified

1. Whether gc ever sends `stop` for a name the pack has no record for under a
   different city key (`city-scoped`); the label filter makes it a no-op
   success (`herdrcore/reads.py:68-73`), but no test names this.
2. Exact live elapsed timing when the 30 s deadline passes mid-reap: by code
   inspection (`herdrcore/lifecycle.py:348`, `rpp/cancel.py:66-74`,
   `rpp/perop.py:142-159`), reap pauses check only `cancel` (not `Deadline`), no
   post-handler timer interrupts reap solely for elapsed deadline, and only
   budgeted hc calls or gc's external 30 s SIGTERM end it (not run live).
3. Whether a custom/real `ProcSource.kill` can raise `ProcError` out of `reap`
   and reach the `internal error` row (`kill_captured` catches snapshot
   `ProcError`, `herdrcore/procs.py:660-663`, while `kill` calls at
   `herdrcore/procs.py:673-678` are not wrapped).
4. hc `kill` and `remove` error kinds beyond `not_found`, `finished` and `live`
   (`herdrcore/lifecycle.py:221-224,249-250`) – the hc source was not read.
5. Parity §stop item "block.close kills detached descendants" concerns the Rex
   provider, not herdr-core; the herdr-core test shows hc's kill ends process
   groups only, not detached children
   (`tests/unit/test_herdrcore_treekill.py:592`).

## kill-object

### Registration

- `"kill-object": spec(lifecycle.op_kill_object, stdin=True)` at
  `herdrcore/ops_table.py:71`. Handler `op_kill_object` at
  `herdrcore/lifecycle.py:542`; worker `_kill_object` at
  `herdrcore/lifecycle.py:560-593`.
- `reads_stdin=True` (`herdrcore/ops_table.py:71`, `OpSpec` at
  `rpp/ops.py:94-103`).
- Deadline class `"default"` (spec default, `herdrcore/ops_table.py:64`), which
  is 30 s when gc sends no `GC_EXEC_DEADLINE_MS` (`rpp/ops.py:23-27`).
  `rpp/perop.py:31` defines `DEADLINE_ENV`, and `rpp/perop.py:129` reads
  `env.get(DEADLINE_ENV)` to construct `deadline`. gc v1.5.1 has no hit for it
  (whole-tree grep).
- Stdout cap `SMALL_STDOUT_CAP` = 64 KiB (`herdrcore/ops_table.py:64`,
  `rpp/ops.py:32`).
- Housekeeping is on, so `lifecycle.sweep_spawn_files(ctx)` runs before the
  handler (`herdrcore/ops_table.py:44-46`).
- Parity §kill-object verdict: `not-implemented` (§kill-object "Verdict", table
  row at `docs/provider-herdr-parity.md:79`, `docs/provider-herdr-parity.md:207`).

**Can gc v1.5.1 call it? No.**

- gc's op set has no `kill-object`. The ops gc's exec provider runs are attach,
  check-image, clear-scrollback, copy-to, get-last-activity, get-meta,
  interrupt, is-attached, is-running, list-running, nudge, peek, process-alive,
  protocol, provision, remove-meta, send-keys, set-meta, start and stop
  (`internal/runtime/exec/*.go`, non-test run-call grep). A case-insensitive
  search for `kill.?object` over the whole v1.5.1 tree returns zero hits.
- gc does not know the token `carrier.ops` either.
  `internal/runtime/protocol.go:15-56` lists only report-attachment,
  report-activity, proc.exec, proc.stream, tty.attach and proc.provision.
  Unknown tokens are ignored (`internal/runtime/protocol.go:14`;
  `docs/reference/exec-session-provider.md:183`).
- Parity §kill-object "Verdict" agrees: "no kill-object op, method or capability
  appears anywhere in the tree".
- So `kill-object` is a pack-defined extension. Nothing in gc v1.5.1 sends it.
  It is exercised only by pack tests or a future gc.

### Inputs

- argv: `<script> kill-object <session-name>`. `args[0]` is required. Missing or
  empty gives OpError "missing session name"
  (`herdrcore/reads.py:57-60`, called at `herdrcore/lifecycle.py:543`).
- stdin (read in full by the transport before the handler,
  `rpp/perop.py:135`) is one JSON object, parsed by
  `parse_kill_object_request` (`rpp/wire.py:276-287`):

  ```json
  {"object": {"id": "<string>", "created": "<string>"}, "mode": "corpse"|"zombie", "agent_pid": "<string, optional>"}
  ```

  - `object.id` and `object.created` are required strings. Blank or
    whitespace-only values are refused (`rpp/wire.py:142-150`,
    `BoxObject.__post_init__`).
  - `mode` is required and must be exactly `corpse` or `zombie`
    (`rpp/wire.py:268-273`).
  - `agent_pid` is optional and defaults to `""` (`rpp/wire.py:284`). It is
    parsed but never read by `_kill_object` (`herdrcore/lifecycle.py:550-551`
    passes only id, created and mode).
  - `object.id` is the hc terminal address and `object.created` is its created
    stamp. Both come from the `object` field of an `observe` answer (test
    `tests/unit/test_herdrcore_lifecycle.py:231-232`, `observe(...)["object"]`).
- Preconditions (checked inside the handler, not as errors; see "Success
  output"):
  - the box at `object.id` exists with the same `created`
    (`herdrcore/lifecycle.py:561-568`);
  - the box's `gc.session` label equals the argv name
    (`herdrcore/lifecycle.py:569-570`);
  - corpse mode needs the box to be not intact and not attached
    (`herdrcore/lifecycle.py:578-580`);
  - zombie mode needs an intact box, not attached, a readable proc snapshot,
    metadata present, a pid, and agent state exactly `dead`
    (`herdrcore/lifecycle.py:581-588`).
- Runs under an exclusive per-name lock (`herdrcore/lifecycle.py:550`;
  `herdrcore/context.py:152-159`, wait at most 30 s, capped to the op's
  remaining budget).

### Success output

- Exit 0. stdout is one line, `<outcome>\n` (`OpResult.answer`,
  `rpp/ops.py:72-75`), with outcome in `KillOutcome =
  killed|gone|renamed|live|changed` (`rpp/wire.py:138`). Returned at
  `herdrcore/lifecycle.py:557`.
- Outcome meanings (all five also exit 0):

| outcome | when | source |
|---|---|---|
| `gone` | `terminal.get` answers `not_found`, or `created` differs | `herdrcore/lifecycle.py:561-568` |
| `renamed` | the box's `gc.session` label is not the argv name | `herdrcore/lifecycle.py:569-570` |
| `live` | corpse mode and the box is intact or attached | `herdrcore/lifecycle.py:578-580` |
| `changed` | zombie mode and not intact, attached, no snapshot, meta or pid, or agent state not `dead` | `herdrcore/lifecycle.py:581-588` |
| `killed` | `kill_incarnation` returned no problems, then `forget` dropped meta and activity | `herdrcore/lifecycle.py:589,592-593` |

- If the process table cannot be read, the box counts as attached
  (`herdrcore/lifecycle.py:572-577`). So corpse gives `live` and zombie gives
  `changed`.
- `killed` removes the record (`herdrcore/lifecycle.py:370-372`
  `_remove_record`). Tests assert that `observe` then reads `absent`
  (`tests/unit/test_herdrcore_lifecycle.py:245,255`).
- Tests for each outcome: `tests/unit/test_herdrcore_lifecycle.py:231-246`
  (`live`, `changed`, `gone` twice, `killed`); `tests/unit/test_herdrcore_lifecycle.py:249-255` (corpse `killed`);
  `tests/unit/test_herdrcore_lifecycle.py:529-533` (zombie `changed` when the agent is unreadable).

### Failure cases

All failures print nothing to stdout and one `kill-object: <message>` line to
stderr, redacted (`rpp/perop.py:160-169`). Generic runner failures are in the
shared contract.

| Case | Exit | stdout | Source |
|---|---|---|---|
| no session arg | 1 | empty | `herdrcore/reads.py:57-60`; `rpp/perop.py:147-148` |
| stdin not JSON, not an object, missing `object`, `id`, `created` or `mode`, blank id or created, bad `mode` | 1 | empty | `herdrcore/lifecycle.py:545-548`; `rpp/wire.py:279-287`; tests `tests/unit/test_rpp_wire.py:143-153` |
| unknown object or wrong `created` | 0 | `gone` | `herdrcore/lifecycle.py:561-568` (not a failure) |
| hc `terminal.get` fails other than `not_found` (unreachable, timeout, RPC error) | 1, `cannot kill the object: <err>` | empty | `herdrcore/lifecycle.py:551-554,562-566`. An unreachable core also kicks a start (`herdrcore/reads.py:91-95`). |
| hc kill fails (kind other than `not_found` or `finished`) | 1 | empty | `herdrcore/lifecycle.py:243-253,359-362`. The tree is still reaped; the record stays; the same message path applies (`herdrcore/lifecycle.py:552-554`). |
| hc `remove` fails (not `not_found`; `live` retried until `RM_WAIT_S`) | 1 | empty | `herdrcore/lifecycle.py:214-225,552-554` |
| process kill leaves survivors or an unreadable process | 1, `killed, but descendants may survive (the box stays for a retry): …` | empty | `herdrcore/lifecycle.py:589-591`; test `tests/unit/test_herdrcore_treekill.py:765-770` |
| meta file, state-dir, config or lock-file error | 1 | empty | `herdrcore/ops_table.py:48-49` |
| name lock not acquired in time | 1, `busy: …` | empty | `herdrcore/ops_table.py:50-51` |
| stdin still open at the deadline | 1 | empty | `rpp/perop.py:73-75` |
| deadline passed before the handler starts | 1, "no time left before the op's deadline" | empty | `rpp/perop.py:143-145` |
| SIGTERM or cancel mid-op | 1 | empty | direct `Cancelled` (stdin, pre-handler, or reap pauses) gives `canceled (terminated)` (`rpp/perop.py:149-150`); `HcCancelled` in `terminal.get`/`list_city`/`kill`/`remove` is caught by `except HcError` at `herdrcore/lifecycle.py:552-554` (`cannot kill the object: <err>`, `hc/errors.py:107-112`) |
| unexpected exception | 1, `internal error: …` | empty | `rpp/perop.py:153-154` |
| answer over the cap (64 KiB) | 1 | empty | `rpp/perop.py:158-159`. Unreachable in practice: answers are at most 8 bytes. |
| stdout write hits BrokenPipe | 1 | n/a | `rpp/perop.py:162-165` |
| deadline expires inside `kill_incarnation` | no pack-side interrupt solely from elapsed deadline (`cancel.pause` at `herdrcore/lifecycle.py:348` checks cancellation only; budgeted hc calls can fail before/after reap); external gc SIGTERM/SIGKILL ends it | empty (on gc cancel or hc error) | `herdrcore/lifecycle.py:348`, `rpp/cancel.py:66-74`, `rpp/perop.py:142-159` (live timing `unverified`) |

### Conflicts

- Parity verdict: `not-implemented` (§kill-object "Verdict", rows at
  `docs/provider-herdr-parity.md:79` and `docs/provider-herdr-parity.md:207`).
  Our provider exits 2 for unknown ops, which RPP v0 treats as a no-op success
  for non-start ops (`internal/runtime/protocol.go:70`, `internal/runtime/exec/exec.go:159-160`;
  `internal/runtime/exec/exec.go:172-181`, where `exitTwoIsFailure` is true only
  for `start` and `provision`). Parity says "no contract breach" and "No
  action". This matches the gc source.
- Pack vs gc v1.5.1: no contradiction, because gc never calls the op. Three
  points to record:
  - `kill-object` and its handshake token `carrier.ops` (`handshake.py:41`) are
    absent from gc v1.5.1. The pack is ahead of gc's pin. The parity doc cites
    the pack by an absolute, non-portable `/tmp/gascity-packs/...` path
    (`docs/provider-herdr-parity.md:1019`) with no embedded revision metadata,
    while its header pins `0f07496` (`docs/provider-herdr-parity.md:40`).
  - Exit 2 for an unknown op: gc v1.5.1 would treat our exit 2 as success. The
    pack's own op table also treats unknown ops as exit 2
    (`rpp/perop.py:117-120`). No difference.
  - The parity line cites are slightly off against the pin: it says
    `herdrcore/lifecycle.py:542-557` and `herdrcore/lifecycle.py:560-592`. At `0f07496`
    `_kill_object` ends at 593 (`herdrcore/lifecycle.py:593`). It also cites the
    module doc at `herdrcore/lifecycle.py:67-80`; at the pin the "stop and
    kill-object" paragraph starts at `herdrcore/lifecycle.py:67`. The substance
    is unchanged.
- Parity says `_kill_object` "re-checks that the box still exists with the same
  `created` and session label". Confirmed
  (`herdrcore/lifecycle.py:561-570`). Parity's statement that zombie also
  refuses on "unreadable" is confirmed: `snapshot is None or meta is None or
  record.pid is None` gives `changed` (`herdrcore/lifecycle.py:582-583`).

### Unverified

- Exact elapsed timing and external gc cancellation when the op deadline
  expires mid-reap (the pack installs no post-handler timer and reap pauses check
  `cancel.pause` only, `herdrcore/lifecycle.py:348`, `rpp/cancel.py:66-74`;
  budgeted hc calls fail before or after reap).
- Whether any gc version after v1.5.1 sends `kill-object` or declares
  `carrier.ops`. The pin is v1.5.1 only; later gc trees were not read.
- Whether `GC_EXEC_DEADLINE_MS` is ever sent by gc. The pack says "No gc release
  sends it yet" (`rpp/perop.py:32-33`); gc v1.5.1 has no hit.
- Exact hc error kinds besides `not_found`, `finished` and `live` that the real
  hc core can return for `terminal.get`, `terminal.kill` and `terminal.remove`.
  Only the pack's client side was read.
- `agent_pid` in `KillObjectRequest` is confirmed unused for `kill-object` in
  this pin (whole-tree search finds `agent_pid` only in `ObserveAnswer` and
  `KillObjectRequest` wire definitions, `rpp/wire.py:168,265,284`, and `reads.py:155`,
  and `op_kill_object` passes only `id`, `created` and `mode` at
  `herdrcore/lifecycle.py:550-551`). Whether a future gc or pack uses it remains
  `unverified`.

## observe

### Registration

- `herdrcore/ops_table.py:72`: `"observe": spec(reads.op_observe, stdin=True,
  deadline="probe", cap=OBSERVE_STDOUT_CAP)`.
- `reads_stdin=True`: stdin is read in full before the handler
  (`rpp/perop.py:135`). `spec` builds `OpSpec(...)` at
  `herdrcore/ops_table.py:64-65`.
- `deadline="probe"` = 4.0 s (`rpp/ops.py:23-27`), unless gc sets
  `GC_EXEC_DEADLINE_MS` (`rpp/perop.py:31,129,39-47`; digits only, 1–12, else
  ignored).
- Cap `OBSERVE_STDOUT_CAP = 1 << 20` (`rpp/ops.py:33`).
- The handler is wrapped (`herdrcore/ops_table.py:41-53`): it runs
  `lifecycle.sweep_spawn_files` first (`herdrcore/ops_table.py:46`, which
  swallows `OSError` and `StateDirError`, `herdrcore/lifecycle.py:741-746`).
  `StateDirError`, `ConfigError`, `MetaError` and `LockFileError` escaping the
  handler become exit 1 (`herdrcore/ops_table.py:48-49`); `LockTimeout` becomes
  `busy: …` exit 1 (`herdrcore/ops_table.py:50-51`).
- The handshake declares `observe.v1` (`handshake.py:35`, `rpp/wire.py:25`).
- Parity §observe (`docs/provider-herdr-parity.md:960`) verdict:
  **not-implemented**.

### Inputs

- argv: `observe <name>` (`rpp/perop.py:116,134`: `op=argv[1]`,
  `args=argv[2:]`). Only `args[0]` is used; extra args are ignored
  (`herdrcore/reads.py:57-60`).
- stdin: **not JSON**. It is a list of agent process names, one per line.
  Decoded as UTF-8 with `surrogateescape`, split by lines, stripped, blanks
  dropped (`herdrcore/reads.py:63-65`). Empty stdin means no names.
- There are no "attachment" or "activity" request fields. Attachment and
  activity are not read or returned by `observe` (`herdrcore/reads.py:131-156`);
  they belong to `is-attached` and `get-last-activity` and `inventory`.
- Preconditions: no running core is required. A down core answers `unknown` and
  kicks a start (`herdrcore/reads.py:143-147`, `herdrcore/reads.py:91-95`). Reads run under a
  shared per-name lock with a 1 s wait (`herdrcore/context.py:46`, `herdrcore/context.py:152-159`).
- `GC_RPP_FAULT=unavailable` acts as an unreachable core
  (`hc/client.py:311-312`; test `tests/unit/test_herdrcore_lifecycle.py:276-282`).

### Success output

Exit 0, one compact ASCII JSON line plus `\n`. Compact JSON, ASCII only
(`rpp/jsonval.py:29-34`); `OpResult.answer` appends `\n` (`rpp/ops.py:73-75`).
Encoder: `rpp/wire.py:182-191`.

| state | when | JSON | source |
|---|---|---|---|
| `absent` | the core is reachable and holds no record for the name; or the core is down because `server-stop` parked it and nothing runs | `{"state":"absent"}` | `herdrcore/reads.py:138-139`, `herdrcore/reads.py:144-145`; test `tests/unit/test_herdrcore_core.py:108` |
| `unknown` | lock timeout, lock-file error, or any `HcError` (unreachable, timeout, rpc, protocol) | `{"state":"unknown"}` | `herdrcore/reads.py:141-147`; test `tests/unit/test_herdrcore_lifecycle.py:282` |
| `dead` | the newest record is not intact (`is_intact` false) | `{"state":"dead","object":{"id":…,"created":…},"identity":…}` (no `agent` or `agent_pid`) | `herdrcore/reads.py:149-150`; `herdrcore/records.py:53-62` |
| `running` | the record is intact | `{"state":"running","object":{…},"agent":"alive"\|"dead"?,"agent_pid":"<pid>"?,"identity":{…}\|null}` | `herdrcore/reads.py:151-156` |

Field rules:

- `object` = `{"id": <terminal address>, "created": <RFC 3339 UTC, ms>}`
  (`herdrcore/records.py:70-71`, `rpp/wire.py:152-153`, `rpp/wire.py:298-304`).
- `identity`: `null` when the meta file cannot be read
  (`herdrcore/reads.py:148`, `rpp/wire.py:156-158`); `{}` when there is no
  meta; else the subset of `GC_SESSION_ID`, `GC_RUNTIME_EPOCH`,
  `GC_INSTANCE_TOKEN` and `GT_PROCESS_NAMES` that is present
  (`herdrcore/metastore.py:56-58`, `rpp/wire.py:43`; test
  `tests/unit/test_rpp_wire.py:171-176`). A record without a `gc.obj` label also
  gives `{}` (`herdrcore/reads.py:82-84`).
- `agent` is `alive` or `dead`, omitted when it cannot be told: no names, no
  snapshot, no pid, launcher not yet at the agent, or an unreadable process
  under the box (`herdrcore/reads.py:104-118`). The wire type also allows
  `"unknown"` (`rpp/wire.py:136`), but this handler never emits it.
- `agent_pid` is a string. With `alive` it is the found agent's pid
  (`herdrcore/reads.py:115`). With no names it is the box program's pid,
  `str(record.pid)` (`herdrcore/reads.py:152-153`; test
  `tests/unit/test_herdrcore_lifecycle.py:62-72`). With `dead` or undetermined
  it is omitted.
- The newest record wins when a name has several (`herdrcore/records.py:38-50`).
- Encoder invariants raise `WireError` (which becomes exit 1 via the op
  boundary, `rpp/perop.py:153-154`): a present state needs `object`; absent and
  unknown carry only `state`; dead with `agent:"alive"` is refused
  (`rpp/wire.py:171-180`; test `tests/unit/test_rpp_wire.py:183-191`).
- A `ProcError` while snapshotting gives `agent` omitted, not an error
  (`herdrcore/reads.py:121-125`).

### Failure cases

| Case | Exit | stdout | Source |
|---|---|---|---|
| invalid request: missing or empty `<name>` | 1 | empty; stderr `observe: missing session name` | `herdrcore/reads.py:58-59`; `rpp/perop.py:147-148,166-167`; `rpp/ops.py:82-88` |
| stdin is not JSON | n/a: not parsed. Bad bytes never fail (surrogateescape) | – | `herdrcore/reads.py:64` |
| no record for the name | 0 | `{"state":"absent"}` (not a failure) | `herdrcore/reads.py:138-139` |
| state lock busy for more than 1 s, or lock-file error | 0 | `{"state":"unknown"}` | `herdrcore/reads.py:141-142`; `herdrcore/context.py:46` |
| hc unreachable, timeout (query cap 1.5 s, hello 1 s), rpc or protocol error | 0 | `{"state":"unknown"}`; unreachable also kicks a start | `herdrcore/reads.py:143-147`, `herdrcore/reads.py:91-95`; `hc/client.py:140-148`; `hc/errors.py:56-92` |
| core parked by `server-stop`, nothing running | 0 | `{"state":"absent"}` | `herdrcore/reads.py:98-101,144-145` |
| `GC_RPP_FAULT=unavailable` | 0 | `{"state":"unknown"}` | `hc/client.py:311-312`; test `tests/unit/test_herdrcore_lifecycle.py:282` |
| probe deadline: stdin still open at the 1 s margin (about 3 s from `_STARTED` under the 4 s probe class) | 1 | empty; `stdin was still open at the op's deadline` | `rpp/perop.py:71-75`; `rpp/cancel.py:98-103` |
| probe deadline already spent before the handler | 1 | empty; `no time left before the op's deadline` | `rpp/perop.py:144-145` |
| hc call with no time left | 0 | `{"state":"unknown"}` (`HcTimeout` is an `HcError`) | `hc/client.py:368-370`; `herdrcore/reads.py:143` |
| SIGTERM or cancel while reading stdin or before the handler (`Cancelled`) | 1 | empty; `canceled…` | `rpp/perop.py:50-57,143,149-150` (note: `HcCancelled` inside `list_city` is caught as `HcError` at `herdrcore/reads.py:143-147` and returns exit 0 `{"state":"unknown"}`, `hc/errors.py:107-112`) |
| state dir, config or meta error outside the handler's try | 1 | empty; message on stderr | `herdrcore/ops_table.py:48-49` |
| answer over 1 MiB | 1 | empty (nothing written); `answer of N bytes is over the 1048576-byte cap` | `rpp/perop.py:158-159`; test `tests/unit/test_rpp_perop.py:229-240` (cap=100 stand-in) |
| any other exception | 1 | empty; `internal error: …` | `rpp/perop.py:153-154` |
| gc closed stdout | 1 | – | `rpp/perop.py:164-165` |

Notes: reachability of the 1 MiB cap is `unverified` beyond the generic cap test
(counting four identity fields does not bound string lengths; metadata's 1 MiB
read limit and JSON encoding overhead also bear on total size). The pack has no
hard timer that kills the op at 4 s. Past 4 s, gc is expected to kill it
(SIGTERM, then SIGKILL after 2 s, `rpp/perop.py:51`); that is gc behaviour this
pin cannot show (`unverified`).

### Conflicts

Parity §observe verdict: **not-implemented** (`docs/provider-herdr-parity.md:960-982`).
Our Rex provider exits 2 with no stdin read; herdr-core answers JSON. The doc
says `observe.v1` is outside RPP v0 and that gc 1.5.1 never sends it.

Pack vs gc v1.5.1:

1. **`observe` does not exist in gc v1.5.1.** gc v1.5.1 has no
   `internal/runtime/exec/observe.go` (`v1.5.1` tree listing), which the pack
   cites (`rpp/ops.py:28-30`; the test docstring at
   `tests/unit/test_rpp_wire.py:1-3` also names `exec/observe.go` and
   `exec/inventory.go`, neither of which exists at the pin). The tag has
   `internal/runtime/exec/exec.go`, `internal/runtime/exec/seams.go`, `internal/runtime/exec/handshake.go`, `internal/runtime/exec/json.go` and `internal/runtime/exec/cutover.go` only. No
   `observe` op is sent anywhere in `internal/runtime/exec/exec.go`. Conflict
   with the pack's comment (the pack targets a newer gc). Confirms the parity
   doc (`docs/provider-herdr-parity.md:966`).
2. **`observe.v1` is not in gc's capability table.** The table
   (`docs/reference/exec-session-provider.md:191-198`) lists
   `report-attachment`, `report-activity`, `proc.exec`, `proc.provision`,
   `proc.stream` and `tty.attach`. No `observe.v1`. The pack cites
   `exec-session-provider.md "observe.v1"` (`rpp/ops.py:35-37`); that section is
   absent in v1.5.1. The pack's "4 s probe / 1 MiB cap" are therefore not
   checkable against gc 1.5.1.
3. **gc 1.5.1 timeouts:** 30 s default and 120 s start
   (`internal/runtime/exec/exec.go:56-57`); there is no 4 s probe class. The
   pack's 4 s is a newer-gc value (matches parity §is-attached row 4,
   `docs/provider-herdr-parity.md:583`).
4. **gc's `Observe` is a Go-side fold** of `get-last-activity`, `process-alive`
   and `is-attached` (`internal/runtime/exec/seams.go:226-236`); it sends no
   `observe` op. For `process-alive` gc sends names joined with `\n`, no
   trailing newline (`internal/runtime/exec/exec.go:707`), and an empty name
   list skips the op (`internal/runtime/exec/exec.go:704-706`). The pack's
   `stdin_names` parses this form fine.
5. Exit 2 from an unknown op is success in gc
   (`internal/runtime/exec/exec.go:159-161`,
   `docs/reference/exec-session-provider.md:71-81`). Our provider's exit 2 for
   `observe` is therefore the gc-compatible choice. In gc v1.5.1, `exitTwoIsFailure`
   tests only the op names `start` and `provision` (`internal/runtime/exec/exec.go:174-181`);
   handshake declarations do not make exit 2 a failure for other ops (the broader
   "ops the handshake declares" statement is a pack docstring claim at `rpp/exitcodes.py:18-19`).
6. Citation drift in the parity doc, not a behaviour conflict: it cites the
   1 MiB cap at `rpp/ops.py:30`; at this pin the constant is at
   `rpp/ops.py:33` (`rpp/ops.py:28-30` is the deadline docstring).

### Unverified

- gc's behaviour for `observe` itself (op name, request and response, 4 s,
  1 MiB, rules 4–7 and 11): no v1.5.1 source or doc defines it. The pack's
  description of the `observe.v1` contract is the only source.
- Whether a newer gc sends names as one per line with or without a trailing
  newline for `observe`; the pack tolerates both.
- What gc does on non-zero exit, empty stdout or malformed JSON from `observe`:
  not in v1.5.1.
- That the 1 MiB path is reachable.
- Whether the pack's `GC_EXEC_DEADLINE_MS` is sent by any gc (the pack says none
  yet, `rpp/perop.py:32-33`); no hit in gc v1.5.1 Go sources via `rg`.
- Behaviour of gc after the 4 s probe timeout (kill timing) against this pack.

## inventory

### Registration

- `"inventory": spec(reads.op_inventory, cap=INVENTORY_STDOUT_CAP)` is at
  `herdrcore/ops_table.py:73`. The parity doc also cites `herdrcore/ops_table.py:73`
  (`docs/provider-herdr-parity.md:991`).
- `reads_stdin=False`: `stdin` defaults to False in `spec`
  (`herdrcore/ops_table.py:64`), and `rpp/perop.py:135` sets `stdin=b""` for
  such ops. stdin is never read.
- Deadline class `"default"` (spec default, `herdrcore/ops_table.py:64`) = 30 s
  (`rpp/ops.py:23-27`). `GC_EXEC_DEADLINE_MS` overrides it (`rpp/perop.py:129`,
  `rpp/perop.py:39-47`). gc v1.5.1 never sends that variable (no match in the v1.5.1 tree).
- Cap `INVENTORY_STDOUT_CAP = 8 << 20` = 8 MiB (`rpp/ops.py:34`). The handler
  wrapper runs `lifecycle.sweep_spawn_files` first
  (`herdrcore/ops_table.py:45-46`), which swallows `OSError` and
  `StateDirError` (`herdrcore/lifecycle.py:741-746`).
- Parity §inventory (`docs/provider-herdr-parity.md:985-1010`, matrix rows
  `docs/provider-herdr-parity.md:205-206`) verdict: `not-implemented`.

### Inputs

- argv: `<script> inventory`. Extra args are ignored: `op_inventory` never reads
  `ctx.request.args` (`herdrcore/reads.py:197-224`). No flags.
- Environment: `GC_EXEC_CITY_PATH` (city key; unset or empty gives an
  "unscoped" key, `rpp/statedir.py:55-58`), `GC_EXEC_STATE_DIR` (must be
  absolute, `rpp/statedir.py:188-190`), `GC_RPP_FAULT=unavailable`
  (`herdrcore/context.py:88,92-94`) and `GC_EXEC_DEADLINE_MS`.
- Preconditions: `OpContext.__init__` resolves paths without creating
  directories (`herdrcore/context.py:86`, `hc/paths.py:100-135`), requiring a
  resolvable machine id, password-database home and absolute `GC_EXEC_STATE_DIR`
  (when set). Private state-directory hardening (`ensure_private_dir`,
  `rpp/statedir.py:205-223`) is lazy when `state_dir()` is called
  (`herdrcore/context.py:98-107`): `sweep_spawn_files` swallows `StateDirError`
  (`herdrcore/lifecycle.py:741-746`), and `read_meta`, `activity` and
  `_collect_garbage` catch `StateDirError` (`herdrcore/reads.py:86-87,222-223`,
  `herdrcore/activity.py:60-61,84-85`). No name lock is taken (compare
  `observe`, `herdrcore/reads.py:135`). A dead core is not a precondition: it
  yields an answer (see "Failure cases").

### Success output

- One line, compact ASCII JSON (`rpp/jsonval.py:29-35`), newline-terminated
  (`rpp/ops.py:73-75`), exit 0. It is an object, not an array and not NDJSON.
- Top level, in order: `complete` (bool), `sessions` (array)
  (`rpp/wire.py:255-256`).
- `sessions`: one entry per `gc.session` name among the city's records, live and
  finished, sorted by name (`herdrcore/reads.py:200,211,215`). The record used
  is the newest by `created_ms`, then address (`herdrcore/records.py:34-40,79-86`).
  Names are unique by construction; a duplicate would raise `WireError`
  (`rpp/wire.py:250-253`).
- Entry keys, in order (`rpp/wire.py:228-240`):
  - `name`;
  - `state` ("running" if `is_intact`, else "dead", `herdrcore/reads.py:237,250`;
    `herdrcore/records.py:53-62`);
  - `object` = `{"id": <hc address>, "created": <RFC 3339 ms UTC>}`
    (`herdrcore/records.py:70-71`; `rpp/wire.py:152-153`);
  - `agent` (omitted when None; "alive" or "dead" only for a running box with
    readable meta, `herdrcore/reads.py:238-240,104-118`);
  - `attached` (bool or null; null when the process table or lock is unreadable,
    `herdrcore/reads.py:241-246`);
  - `last_activity` (an RFC 3339 ms string for running; null for dead,
    `herdrcore/reads.py:247`; `herdrcore/activity.py:126-137`);
  - `restart_requested` (bool, null if meta is unreadable,
    `herdrcore/reads.py:255`);
  - `identity` (object, `{}` when there is no meta, null if unreadable,
    `herdrcore/reads.py:256`).
- The JSON key is `object`, not `box` (`rpp/wire.py:233`; test
  `tests/unit/test_rpp_wire.py:201-242`). `box` is only the Python field name.
  Parity says "box" (`docs/provider-herdr-parity.md:991`): imprecise wording,
  not a behaviour conflict.
- A name that fails `readable_name` (empty, untrimmed, control character;
  `rpp/wire.py:194-196`) is omitted and sets `complete=false`
  (`herdrcore/reads.py:216-219`). The `{"name":...}`-only entry shape
  (`rpp/wire.py:230-231`) is not emitted by `op_inventory`.
- Side effects: it refreshes activity.json with one screen read per intact box
  that never reported OSC 7501, while more than 2 s of deadline remain
  (`herdrcore/reads.py:171-194`), and removes meta files older than 600 s that no
  record carries (`herdrcore/reads.py:52-54,162-168,220-223`).
- gc v1.5.1 reads: nothing (see "Conflicts").
- Tests: `tests/unit/test_herdrcore_lifecycle.py:189-205` (shape, newest record,
  dead entry identity, restart flag); `tests/unit/test_herdrcore_lifecycle.py:346-357` (activity refresh); `tests/unit/test_herdrcore_lifecycle.py:360` and
  following (an unread screen reads as active now);
  `tests/unit/test_herdrcore_procs.py:362-363` (`attached`).

### Failure cases

| Case | Exit | stdout | Source |
|---|---|---|---|
| core up, listing works | 0 | answer line | `herdrcore/reads.py:197-224` |
| core unreachable, city not parked | 0 | `{"complete":false,"sessions":[]}`; also kicks a start | `herdrcore/reads.py:201-205,91-95`; tests `tests/unit/test_herdrcore_lifecycle.py:283-284,322-336`, `tests/unit/test_herdrcore_server.py:509-510,519-520` |
| core down, city parked (after `server-stop`) and nothing of it runs | 0 | `{"complete":true,"sessions":[]}`; no kick | `herdrcore/reads.py:98-101,202-203`; `herdrcore/core.py:307-314`; test `tests/unit/test_herdrcore_server.py:436-442` |
| any other `HcError` on the listing (RPC error, timeout, lost connection, protocol, cancel mid-call) | 0 | `{"complete":false,"sessions":[]}` | `herdrcore/reads.py:201-205`; `hc/errors.py:56,73-92,107` (all subclass `HcError`; code mapping confirmed, no dedicated test for non-unreachable kinds) |
| unreadable session name | 0 | the answer without that name, `complete:false` | `herdrcore/reads.py:216-219` |
| meta or state dir unreadable for a record | 0 | entry with `restart_requested` and `identity` null | `herdrcore/reads.py:80-88,255-256` |
| activity.json unreadable or unwritable; GC fails | 0 | normal answer | `herdrcore/activity.py:60-61,84-85`; `herdrcore/reads.py:222-223` |
| process table unreadable | 0 | entries with `attached` null and `agent` omitted | `herdrcore/reads.py:206-210,241-246,110`; `rpp/wire.py:227-228` (`agent=None` omits `"agent"`) |
| screen read fails or time is short | 0 | `last_activity` = now | `herdrcore/reads.py:187-192`; `herdrcore/activity.py:12-17,135-136` |
| state-dir or path error building the context (relative `GC_EXEC_STATE_DIR`, no passwd home) | 1 | empty; stderr `inventory: <msg>` | `herdrcore/ops_table.py:44,48-49`; `rpp/statedir.py:155,157,190`; `rpp/perop.py:147-148,166-167` |
| symlink, wrong-owner or uncreatable state dir on lazy `state_dir()` paths that do not catch `StateDirError` (context construction resolves paths only) | 1 | empty | `herdrcore/context.py:86,98-107`; `rpp/statedir.py:205-223` (`sweep_spawn_files`, `read_meta`, `activity` and `_collect_garbage` catch `StateDirError`) |
| machine id file unreadable (`MachineIdError`, not caught by `_wrap`) | 1 | empty; stderr `inventory: internal error: MachineIdError: ...` | `rpp/statedir.py:90,135`; `rpp/perop.py:153-154` (code mapping confirmed; no dedicated test) |
| deadline already expired before the handler | 1 | empty | `rpp/perop.py:144-145` |
| SIGTERM or cancel before the handler | 1 | empty | `rpp/perop.py:143,149-150,50-57` (note: `HcCancelled` on `list_city` is caught as `HcError` at `herdrcore/reads.py:201-205` and returns exit 0 `{"complete":false,"sessions":[]}`, `hc/errors.py:107-112`) |
| deadline passes mid-op | no code of its own | n/a | The handler has no end-of-op check. It bounds only screen reads (`herdrcore/reads.py:187`) and hc calls (`herdrcore/context.py:128-133`). gc kills the script at its 30 s timeout (`internal/runtime/exec/exec.go:56,79-80`). The pack's behaviour between those is unverified |
| answer over 8 MiB | 1 | empty (nothing printed) | `rpp/perop.py:158-159`; generic test `tests/unit/test_rpp_perop.py:233-240` (uses `observe`, cap 100) |
| stdout closed by the reader | 1 | partial or none | `rpp/perop.py:161-165` |
| unexpected exception | 1 | empty; stderr `internal error: <Type>: <msg>` | `rpp/perop.py:153-154` |

### Conflicts

gc v1.5.1 vs pack; parity verdict.

- **gc v1.5.1 never sends `inventory`.** The only `run(...)` ops in
  `internal/runtime/exec/exec.go` are start and provision, stop, nudge, peek,
  send-keys, interrupt, clear-scrollback, is-running, is-attached,
  process-alive, set, get and remove-meta, list-running (`internal/runtime/exec/exec.go:764`), check-image,
  copy-to and get-last-activity (`internal/runtime/exec/exec.go:176,582-839`).
  `rg` finds no `"inventory"`, `"observe"`, `"kill-object"` or `observe.v1` in
  non-test Go or docs under v1.5.1. `docs/reference/exec-session-provider.md`
  at v1.5.1 has no `inventory` section.
- v1.5.1 capability tokens: report-attachment, report-activity, proc.exec,
  proc.stream, tty.attach, proc.provision
  (`internal/runtime/protocol.go:21-55`). `Has` is an exact match and
  `Validate` checks only the version (`internal/runtime/protocol.go:67-80`). The
  pack's `observe.v1` (`handshake.py:35`) is therefore ignored by v1.5.1.
- gc `stop` in v1.5.1 uses `ListRunning("")` (`cmd/gc/cmd_stop.go:636-637`),
  which is `list-running` (`internal/runtime/exec/exec.go:763-764`); the pack's
  `list-running` shares the 8 MiB cap (`herdrcore/ops_table.py:76`). If gc did
  send `inventory`, exit 2 would be read as success: `inventory` is not in
  `exitTwoIsFailure` (`internal/runtime/exec/exec.go:153-181`).
- The pack's spec text for `inventory` (shape, `complete`, null attributes,
  8 MiB) matches the live gascity tree's doc
  `/Volumes/Sources/gascity/docs/reference/exec-session-provider.md:1199-1232`
  (`object` key, `complete`, name rules). **That is a live working tree, ahead
  of the v1.5.1 pin**, so this is context, not a v1.5.1 claim.
- **Conflict (pack vs live doc, outside the pin):** the live doc says "Any
  failure is a non-zero exit"
  (`/Volumes/Sources/gascity/docs/reference/exec-session-provider.md:1214-1215`,
  live tree). The pack answers exit 0 with `complete:false` for a core it could
  not read (`herdrcore/reads.py:204-205`), and the live doc's `complete:false`
  paragraph allows that. No real conflict was read. `unverified` against gc's
  decoder, which is not in the v1.5.1 tree.
- **Parity record:** verdict `not-implemented`
  (`docs/provider-herdr-parity.md:996-1000`; matrix
  `docs/provider-herdr-parity.md:205-206`). Confirmed against both sources: the
  pack registers and implements it (`herdrcore/ops_table.py:73`,
  `herdrcore/reads.py:197-224`); gc v1.5.1 never calls it. The Rex provider
  exits 2 as an unknown op (`docs/provider-herdr-parity.md:989`). Two wording
  nits for the parity doc, with no verdict change: "box" should be "object"
  (`docs/provider-herdr-parity.md:991` vs `rpp/wire.py:233`), and "unreachable
  core returns incomplete and empty" omits that every `HcError` does the same.

### Unverified

- Exact pack behaviour when the 30 s deadline lapses mid-scan (no deadline check
  outside screen reads).
- Real-hc error occurrence and missing dedicated test coverage for non-unreachable
  `HcError` kinds (`herdrcore/reads.py:201-205` explicitly maps all `HcError`
  subclasses to `complete:false`, exit 0) and `MachineIdError` (`rpp/statedir.py:90`
  escapes `_wrap` to generic exit 1 `internal error` at `rpp/perop.py:153-154`).
- `lock_held` error mapping (`herdrcore/attach.py:42-54`): `open` `OSError` is
  mapped to `LockFileError` (`attached = None` at `herdrcore/reads.py:245`),
  while an unexpected `flock` `OSError` other than `BlockingIOError` can escape
  to generic exit 1 `internal error`; real platform occurrence is `unverified`.
- How a gc newer than v1.5.1 decodes `complete:false` with empty `sessions`: its
  decoder is not in the pinned tree.
- Whether Python's name sort (`herdrcore/reads.py:215`, code-point order)
  matters to any consumer: no gc v1.5.1 consumer exists.

## is-running

### Registration

- `"is-running": spec(reads.op_is_running)` at `herdrcore/ops_table.py:74`; the
  pin has it on 74, matching the parity doc.
- `spec` defaults (`herdrcore/ops_table.py:64`): `stdin=False`
  (`reads_stdin=False`), `deadline="default"`, `cap=SMALL_STDOUT_CAP`,
  `housekeeping=True`.
- Deadline class `default` = 30 s (`rpp/ops.py:23-27`) unless
  `GC_EXEC_DEADLINE_MS` is set (`rpp/perop.py:31,129` via `deadline_for`,
  `rpp/ops.py:127`). gc 1.5.1 has a 30 s op timeout
  (`internal/runtime/exec/exec.go:56`); the parity doc says gc never sends the
  env var (§cancel and deadline row 1).
- Stdout cap `SMALL_STDOUT_CAP` = 64 KiB (`rpp/ops.py:32`). An over-cap output
  becomes exit 1 with empty stdout (`rpp/perop.py:158-159`). Not reachable here:
  the answer is 5–6 bytes.
- `_wrap` (`herdrcore/ops_table.py:41-53`) runs `lifecycle.sweep_spawn_files(ctx)`
  first (`herdrcore/ops_table.py:46`; it swallows `OSError` and `StateDirError`,
  `herdrcore/lifecycle.py:741-746`; tested by
  `tests/unit/test_herdrcore_launch.py:174-180`), then the handler.
- Parity §is-running (`docs/provider-herdr-parity.md:465`) verdict:
  `intentional (with a mistake inside)`.

### Inputs

- argv: `gc-runtime-herdr-core is-running <name>` (`rpp/perop.py:134`: the
  runner takes `argv[2:]` as args). gc calls `script is-running <name>`
  (`docs/reference/exec-session-provider.md:130`;
  `internal/runtime/exec/exec.go:666`).
- Only `args[0]` is used (`herdrcore/reads.py:57-60`, `herdrcore/reads.py:288`).
  Extra args are ignored by the handler (no code reads `args[1:]`; whether any
  caller passes extras is `unverified`). A missing or empty name gives
  `OpError("missing session name")` and exit 1 (`herdrcore/reads.py:58-59`).
- stdin is not read (`reads_stdin=False`; `rpp/perop.py:135`).
- Environment: `GC_EXEC_CITY_PATH` picks the city (`rpp/statedir.py:39,55-58`;
  unset or empty gives the "unscoped" key, not an error). `GC_EXEC_STATE_DIR`
  must be absolute or `StateDirError` (`rpp/statedir.py:187-190`).
  `GC_RPP_FAULT=unavailable` makes every hc call raise `HcUnreachable`
  (`hc/client.py:311-312`).
- Precondition: none. It does not need the core up and does not start one on
  the "nothing runs" path (`herdrcore/reads.py:263-278`).

### Success output

`op_is_running` (`herdrcore/reads.py:287-289`): the newest record for the name
via `current_record(..., liveness=True)`, then
`encode_bool(record is not None and is_intact(record))`.

| Answer | stdout | exit | Source |
|---|---|---|---|
| running | `true\n` | 0 | `rpp/wire.py:293-295`; `rpp/ops.py:73-75` (`answer` appends `\n`) |
| not running | `false\n` | 0 | same |

- Both are exit 0 with a word, never JSON, never empty. Unit test:
  `tests/unit/test_rpp_perop.py:180-182` (`b"true\n"`);
  `tests/unit/test_herdrcore_server.py:437-449` (`false`), `tests/unit/test_herdrcore_server.py:527-536`
  (`(0, "false")`), `tests/unit/test_herdrcore_server.py:424-433` (`code == 0 and stdout.strip() == b"false"`).
- `false` covers: no record for the name (never started, or removed), a finished
  record, or a live record that is not intact (`is_intact`,
  `herdrcore/records.py:53-62`: live, pid set, reason not in `BROKEN_REASONS`
  (`herdrcore/records.py:26`), and the pid label equals the pid or, with no
  label, the terminal is not `keep`). Tests:
  `tests/unit/test_herdrcore_lifecycle.py:140-146` (exited program), `tests/unit/test_herdrcore_lifecycle.py:152-163`
  (kept terminal after holder loss).
- gc's reader (`internal/runtime/exec/exec.go:650-680`): `run()` right-trims `\n`
  (`internal/runtime/exec/exec.go:104`); `boxOccupancy` does `TrimSpace` and
  accepts only the literals `true` (occupied, definite) and `false` (vacant,
  definite). Anything else, empty output, or any error is "cannot tell", and
  `IsRunning` reads it as false (`internal/runtime/exec/exec.go:652-654`). Doc:
  `docs/reference/exec-session-provider.md:130,365`.
- Only definite **true** protects the pre-existing box: `Start` computes
  `foreignBox := occupancyKnown && occupiedBefore`
  (`internal/runtime/exec/exec.go:216-217`), and `cleanupAfterStartFailure`
  skips teardown only when `foreignBox` is true or `startErr` is
  `runtime.ErrSessionExists` (`internal/runtime/exec/exec.go:262-270`). Both
  definite `false` (`(false, true)`) and indeterminate (`(false, false)`) set
  `foreignBox` to false and leave post-failure cleanup enabled unless `startErr`
  is `ErrSessionExists`. The comment at `internal/runtime/exec/exec.go:255-259`
  explains that a pack without `is-running` keeps prior behaviour (withholding
  teardown only when `foreignBox` is true), whereas `boxOccupancy`'s own
  docstring (`internal/runtime/exec/exec.go:657-660`) overstates that an
  indeterminate answer must withhold teardown. Exit 0 `false` and exit 1 give
  the same `IsRunning() == false` and the same `Start` cleanup behaviour,
  differing only in `boxOccupancy`'s `definite` boolean. Exit 2 gives a `nil`
  error plus empty output (`internal/runtime/exec/exec.go:159`,
  `internal/runtime/exec/exec.go:174-181`; `is-running` is not in
  `exitTwoIsFailure`), so it also yields `(false, false)`.

### Failure cases

Stdout is empty on every non-zero exit (`OpResult.fail` has `b""`;
`rpp/perop.py` writes stdout only from the result). That makes 8 distinct
failure rows (exit 1) plus the two exit-0 `false` cases. Generic runner failures
are in the shared contract.

| Case | Exit | stdout | Source |
|---|---|---|---|
| unknown name (no record) | 0 | `false\n` | `herdrcore/reads.py:273` (`newest` gives None, `herdrcore/records.py:48-50`) |
| core unreachable, nothing of it runs (no socket holder, no core process, no listener elsewhere), parked or not | 0 | `false\n` | no start kicked: `herdrcore/reads.py:275-276`, `herdrcore/corescan.py:123-140`; `tests/unit/test_herdrcore_server.py:527-536` |
| core unreachable, but a holder or core process still runs | 1 | empty | stderr `is-running: cannot read the city's core: <hc msg>` (`herdrcore/reads.py:277-278`; the runner emits `"<op>: <msg>"`); a start is kicked (`herdrcore/reads.py:91-95`, which calls `herdrcore/core.py:407-429`). Tests `tests/unit/test_herdrcore_server.py:360-368`, `tests/unit/test_herdrcore_server.py:387-414` |
| core unreachable and the process table unreadable (`ProcError`) | 1 | empty | `nothing_runs` returns False (`herdrcore/corescan.py:136-139`); test `tests/unit/test_herdrcore_server.py:378-384` |
| `GC_RPP_FAULT=unavailable` | 1 | empty | `nothing_runs` is False (`herdrcore/corescan.py:129-130`); `unverified` by a named test for `is-running` |
| hc error other than `HcUnreachable` (RPC, timeout, protocol, lost connection) | 1 | empty | `herdrcore/reads.py:277-278`: not unreachable, so no `nothing_runs` shortcut; no kick (`herdrcore/reads.py:91-95`) |
| deadline hit mid-call | 1 | empty | `HcTimeout` "no time left before the op's deadline" (`hc/client.py:368-370`) goes to `herdrcore/reads.py:277-278`; or "no time left before the op's deadline" before the handler (`rpp/perop.py:144-145`) |
| SIGTERM or cancel | 1 | empty | `Cancelled` becomes `OpResult.fail` `canceled (terminated)` (`rpp/perop.py:149-150`); `HcCancelled` (`hc/errors.py:107-112`, `hc/client.py:367`) is caught by `except HcError` in `current_record` (`herdrcore/reads.py:274-278`) and becomes `cannot read the city's core: ...` |
| state-dir errors (`GC_EXEC_STATE_DIR` relative, unsafe dir) | 1 | empty | `StateDirError` from `OpContext.__init__`'s `resolve_paths` (`herdrcore/context.py:78`, `rpp/statedir.py:187-190`) is mapped by `_wrap` (`herdrcore/ops_table.py:48-49`). Note `parked()` swallows `StateDirError` as "not parked" (`herdrcore/core.py:307-314`) |
| lock timeout (kick lock) | n/a | n/a | `kick` returns False on `LockTimeout`, `OSError` or `ConfigError` (`herdrcore/core.py:413-417`); `_wrap` maps `LockTimeout` to exit 1 `busy:` (`herdrcore/ops_table.py:50-51`) only if one escapes |
| any other exception | 1 | empty | `internal error: <Type>: <msg>` (`rpp/perop.py:153-154`) |
| missing or empty name | 1 | empty | `is-running: missing session name` (`herdrcore/reads.py:58-59`) |

### Conflicts

- Parity doc verdict: `intentional (with a mistake inside)`. The "mistake" is
  operational, not a code bug: a Rex outage reads as every session dead (parity
  doc §Operational risks, "Rex outage reads as 'everything is dead'"). For
  herdr-core the pack answers `false` (exit 0) when nothing of the core runs and
  exit 1 when something might; to gc both are `IsRunning() == false`
  (`internal/runtime/exec/exec.go:652-654`).
- No pack-vs-gc v1.5.1 conflict on the op's contract: the argv, the
  `true`/`false` words, and exit 0/1 all match
  `docs/reference/exec-session-provider.md:130,365` and
  `internal/runtime/exec/exec.go:665-680`.
- Pack vs gc note: exit-0 `false` gives `boxOccupancy` `(false, true)` while exit
  1 gives `(false, false)`, and both yield `IsRunning() == false`
  (`internal/runtime/exec/exec.go:652-654`). In gc v1.5.1's `Start`, both
  `(false, true)` and `(false, false)` also leave post-failure cleanup enabled
  (`foreignBox` is true only for `(true, true)`;
  `internal/runtime/exec/exec.go:216-217,262-270`), despite `boxOccupancy`'s
  docstring (`internal/runtime/exec/exec.go:657-660`). Parity §is-running row 1's
  "Exit 1 and `false` are the same answer to gc" therefore holds for both
  `IsRunning` and v1.5.1's `Start` cleanup.
- Line-number drift in the parity doc: it cites `rpp/wire.py:294` (actual
  `rpp/wire.py:293-295`) and `herdrcore/ops_table.py:74` (correct). Its
  `internal/runtime/exec/exec.go:665-680` is correct.
- The pack docstring cites gc `exec/observe.go` and declares `observe.v1`; gc
  v1.5.1 has no `internal/runtime/exec/observe.go` (`internal/runtime/exec/`
  lists none) and does not parse that token. `is-running` is unaffected (gc
  1.5.1 still calls it, `internal/runtime/exec/seams.go:59,131`;
  `internal/runtime/exec/exec.go:216,653`). Version skew only.

### Unverified

- Whether any caller passes extra argv beyond `<name>` (the pack handler ignores
  them by construction, `herdrcore/reads.py:57-60`).
- The exact stderr text of hc errors (`<method>: ...`); the method name for the
  list call was not read.
- A named test for `GC_RPP_FAULT=unavailable` on `is-running` (the tests found
  cover `server-stop` with the fault, not `is-running`).
- Behaviour of `current_record` when `list_terminals` returns a malformed record
  (no test read).
- Whether gc's seam layer (the production path, `internal/runtime/seam_adapter.go`)
  adds retries around `is-running`; the parity doc says up to two probes
  (§process-alive, §exit codes verdict), not re-read here.

## process-alive

### Registration

- `herdrcore/ops_table.py:75`: `"process-alive": spec(reads.op_process_alive,
  stdin=True)`.
- `reads_stdin=True` (`herdrcore/ops_table.py:65` maps `stdin=` to
  `OpSpec.reads_stdin`; `rpp/ops.py:95`).
- Deadline class `default` (spec default, `herdrcore/ops_table.py:64`): 30 s
  (`rpp/ops.py:24`), replaced by `GC_EXEC_DEADLINE_MS` when set and numeric
  (`rpp/perop.py:31,39`, `rpp/ops.py:127`). gc's own timeout is also 30 s
  (`internal/runtime/exec/exec.go:56`).
- Stdout cap `SMALL_STDOUT_CAP` = 64 KiB (`rpp/ops.py:32`;
  `herdrcore/ops_table.py:64`).
- Housekeeping stays on: `_wrap` runs `sweep_spawn_files` before the handler
  (`herdrcore/ops_table.py:38-45`; it swallows `OSError` and `StateDirError`,
  `herdrcore/lifecycle.py:741-746`).
- Parity §process-alive (`docs/provider-herdr-parity.md:499`) verdict:
  "intentional (with a mistake inside)".

### Inputs

- argv: `process-alive <name>`. gc sends exactly that:
  `p.run(stdin, "process-alive", name)`
  (`internal/runtime/exec/exec.go:708`; table
  `docs/reference/exec-session-provider.md:132`). The name is
  `request.args[0]`; missing or empty gives OpError `missing session name`
  (`herdrcore/reads.py:57-60`).
- stdin = process names, one per line, not a pid or process table. gc sends
  `strings.Join(processNames, "\n")`: no trailing newline
  (`internal/runtime/exec/exec.go:707`). Doc: "process names (1/line)"
  (`docs/reference/exec-session-provider.md:132`). The rppcheck probe sends
  `<cmd-first-field>\n` (`internal/runtime/rppcheck/rppcheck.go:452`).
- gc never calls the op with an empty name list: it returns `true` locally
  (`internal/runtime/exec/exec.go:704-706`).
- Pack parse: `stdin_names` decodes UTF-8 with surrogateescape, `splitlines()`,
  strip, drop blanks (`herdrcore/reads.py:63-65`). stdin is read fully, polled,
  before the handler (`rpp/perop.py:60-77,135`).
- Preconditions: none on stdin. State: the name's newest record must be intact
  for any non-trivial answer (`herdrcore/reads.py:295-296`; `is_intact`
  `herdrcore/records.py:53-62`).

### Success output

Exit 0 in every case below. Handler `herdrcore/reads.py:292-303`; the line is
written by `OpResult.answer` = text + `\n` (`rpp/ops.py:73-76`), `encode_bool`
gives `true`/`false` (`rpp/wire.py:293-295`).

| Case | stdout | Source |
|---|---|---|
| no record, or the record is not intact (including city stopped, or core down with nothing running) | `false\n` | `herdrcore/reads.py:293-296`, `current_record(liveness=True)` `herdrcore/reads.py:263-276`; tests `tests/unit/test_herdrcore_server.py:192,536` |
| intact record, no names after parsing (empty or all-blank stdin) | `true\n` | `herdrcore/reads.py:298-299`; test `tests/unit/test_herdrcore_lifecycle.py:274` |
| intact, a process in the box subtree matches a name | `true\n` | `herdrcore/reads.py:300-304`, `agent_state` `herdrcore/reads.py:104-118`; test `tests/unit/test_herdrcore_lifecycle.py:275` |
| intact, no match and no unreadable process | `false\n` | `herdrcore/reads.py:301-304`; tests `tests/unit/test_herdrcore_lifecycle.py:276,554` |
| cannot tell (process table unreadable, launcher not at the agent yet, record has no pid, or no match with an opaque process under the box) | empty stdout, exit 0 | `herdrcore/reads.py:301-304`, `herdrcore/reads.py:104-117`; tests `tests/unit/test_herdrcore_lifecycle.py:526-527,574-576` |

gc's reader (`internal/runtime/exec/exec.go:703-718`): an error gives `false`;
else `TrimSpace(out)` (after `run` already trimmed the trailing `\n`,
`internal/runtime/exec/exec.go:104`); `""` or `"true"` means alive, anything
else means dead. So empty stdout reads as alive; only `false` (or any other
string, or an error) reads as dead. The comment at
`internal/runtime/exec/exec.go:712-717` confirms the intent. rppcheck is
stricter: stdout must be exactly `true` or `false` or the check FAILs
(`internal/runtime/rppcheck/rppcheck.go:459-461`); empty stdout would fail that
check, but the gc runtime does not use it.

Name matching (for the "match" row): `expand_names`
(`herdrcore/procs.py:550-561`; alias `kimi` to `kimi-code`
`herdrcore/procs.py:65`; `gc` to the basename of the stored `gc_bin`),
`matches` (`herdrcore/procs.py:564-583`: a zombie never matches; basename of
comm or argv[0]; node, bun, npx or deno script arg, `herdrcore/procs.py:63-64`),
`find_agent` (`herdrcore/procs.py:608-613`). Meta unreadable gives `meta=None`
and no `gc_bin` expansion (`herdrcore/reads.py:80-88`, `herdrcore/reads.py:298` path).

### Failure cases

All failures: stdout empty, stderr `process-alive: <message>` (redacted), per
`rpp/perop.py:147-170`. Exit 1 = `ExitCode.FAIL`
(`rpp/exitcodes.py:14`). gc turns any non-0, non-2 exit into an error, which
`ProcessAlive` reads as `false` (`internal/runtime/exec/exec.go:709-711`,
`runError` `internal/runtime/exec/exec.go:153-170`).

| Case | Exit | stdout | Source |
|---|---|---|---|
| missing or empty `<name>` | 1 | empty | `herdrcore/reads.py:57-60`; `rpp/perop.py:147` |
| empty stdin | 0 | `true\n` if the record is intact, else `false\n` (not a failure) | `herdrcore/reads.py:63-65,295-299`; test `tests/unit/test_herdrcore_lifecycle.py:274` |
| malformed stdin (invalid UTF-8, junk, `\r` or U+2028 separators) | 0 | no decode error: surrogateescape; junk is just a non-matching name, giving `false` (or empty if it cannot tell) | `herdrcore/reads.py:63-65` |
| core unreachable (not the liveness-gated case: something of the core still runs) | 1 | empty | `herdrcore/reads.py:274-278`, `herdrcore/reads.py:295` (`cannot read the city's core: ...`); kicks a start if `HcUnreachable` (`herdrcore/reads.py:91-95`) |
| core unreachable, nothing of the core runs | 0 | `false\n` | `herdrcore/reads.py:274-276`, `herdrcore/reads.py:295-297`; tests `tests/unit/test_herdrcore_server.py:192,536` |
| other hc error (`HcRpcError`, timeouts) on list | 1 | empty | `herdrcore/reads.py:274-278` |
| process table unreadable (`ProcError`) | 0 | empty (cannot tell) | `herdrcore/reads.py:121-125`, `herdrcore/reads.py:301-303`; test `tests/unit/test_herdrcore_lifecycle.py:565-576` |
| meta unreadable (`MetaError`, `StateDirError`, `OSError`) | 0 | continues with `meta=None` | `herdrcore/reads.py:80-88` |
| `StateDirError`, `ConfigError`, `MetaError` or `LockFileError` escaping | 1 | empty | `herdrcore/ops_table.py:48-49` |
| `LockTimeout` | 1 | empty, message `busy: ...` | `herdrcore/ops_table.py:50-51` (`unverified` that this op takes any lock; no lock call in `op_process_alive`) |
| stdin still open at the deadline | 1 | empty | `rpp/perop.py:73-75`; test `tests/unit/test_rpp_perop.py:135,155` (`nudge`) |
| deadline expired before the handler | 1 | empty | `rpp/perop.py:144-145`; test `tests/unit/test_rpp_perop.py:168` (`stop`) |
| SIGTERM or cancel | 1 | empty | `Cancelled` gives `canceled (terminated)` (`rpp/perop.py:149-150`; `tests/unit/test_rpp_perop.py:371,400`); `HcCancelled` on `list_city` is caught as `HcError` (`herdrcore/reads.py:274-278`) and becomes `cannot read the city's core: ...` (`hc/errors.py:107-112`) |
| deadline hit inside an hc call | 1 | empty | `HcTimeout` ("no time left before the op's deadline" or `"timed out after ...s"`, `hc/client.py:364-371,395-403`) caught by `current_record` (`herdrcore/reads.py:274-278`) -> `cannot read the city's core: ...` |
| unexpected exception | 1 | empty | `rpp/perop.py:153-156` |
| answer over the cap | 1 | empty | `rpp/perop.py:158-159`; unreachable here: answers are at most 6 bytes |
| exit 2 | never produced (the op is in the table) | – | `rpp/perop.py:117-120` applies only to unregistered ops; gc would read 2 as alive, `internal/runtime/exec/exec.go:159`, `internal/runtime/exec/exec.go:712-717` |

### Conflicts

- No pack-vs-gc conflict on stdin shape, argv, or the true/false/empty
  vocabulary. The pack's empty-stdout "cannot tell" is designed for gc's
  `s == ""` meaning alive (`herdrcore/reads.py:301`;
  `internal/runtime/exec/exec.go:718`).
- The pack answers `true` for empty names; gc never sends empty names
  (`internal/runtime/exec/exec.go:704-706`), so that branch is only reachable by
  a direct caller (or all-blank lines).
- Pack separators: `splitlines()` splits more than `\n`
  (`herdrcore/reads.py:64`); gc sends `\n` only. Same for gc input (parity
  deviation 6, "intentional").
- The pack doc comment `rpp/ops.py:25-27` says the 4 s probe deadline is from gc
  `exec/observe.go`; gc v1.5.1 has no `internal/runtime/exec/observe.go` (parity
  `docs/provider-herdr-parity.md:966`). Not relevant to `process-alive` (class
  `default`).
- Parity record check: every pack line cite in the parity doc at line 505
  matches the pin: `herdrcore/ops_table.py:75`, `herdrcore/reads.py:292-303`,
  `herdrcore/reads.py:63-65`, `herdrcore/procs.py:550-561`, `herdrcore/procs.py:564-583`,
  `herdrcore/procs.py:608-613`. Parity says `agent_state` at `herdrcore/reads.py:104-118`
  (matches) and `herdrcore/procs.py:504-522` for `subtree` (confirmed: `subtree`
  explicitly walks descendants at `herdrcore/procs.py:504-522`).
  The parity verdict stands: intentional, with the interpreter-script gap
  (deviation 2, "mistake") a Rex-side matter, not a pack-vs-gc conflict.
- Parity deviation 4 (Rex answers `false` on a transient error, and gc then
  reads dead) is consistent with `internal/runtime/exec/exec.go:709-711`.

### Unverified

- Real process visibility (`Snapshot` / `Proc.opaque` at runtime) and exact
  elapsed timing (`hc/client.py:364-371,395-403` settles the `HcTimeout`/cancel
  error class and message format; `herdrcore/procs.py:504-522` settles `subtree`
  descendant walking; and `internal/runtime/seam_adapter.go:88-99,152-159`,
  `internal/runtime/exec/seams.go:58-62,159-167,229-236` and
  `internal/runtime/exec/cutover.go:33-37` establish that `seamProvider` runs
  the two `is-running` liveness probes in `attach` before `Observe` folds
  `GetLastActivity`, `ProcessAlive` and `IsAttached`).
- Behaviour with a record whose `pid` is None but `is_intact` is true
  (`agent_state` returns None and the stdout is empty; `is_intact` itself returns
  False for `pid is None`, `herdrcore/records.py:55`, so this is unreachable at
  the handler).

## list-running

### Registration

- `"list-running": spec(reads.op_list_running, cap=INVENTORY_STDOUT_CAP)` is at
  `herdrcore/ops_table.py:76`; line 75 is `process-alive`. The parity doc also
  says `herdrcore/ops_table.py:76`.
- `spec` defaults: `stdin=False`, `deadline="default"`, `housekeeping=True`
  (`herdrcore/ops_table.py:64`). So `reads_stdin=False`, deadline class
  `default`, and the spawn-file sweep runs before the handler
  (`herdrcore/ops_table.py:45-46`, `herdrcore/lifecycle.py:741-746`; sweep errors
  are swallowed).
- Deadline: class `default` = 30.0 s (`rpp/ops.py:24-28`).
  `GC_EXEC_DEADLINE_MS` overrides it if set (`rpp/perop.py:31,39-47`,
  `rpp/ops.py:119-128`). gc v1.5.1 never sends it: the pack says "No gc release
  sends it yet" (`rpp/perop.py:32-33`), and `rg GC_EXEC_DEADLINE_MS` over gc
  v1.5.1 finds nothing outside tests. gc's own timeout is 30 s
  (`internal/runtime/exec/exec.go:56,62-63`).
- Cap: `INVENTORY_STDOUT_CAP = 8 << 20` (`rpp/ops.py:34`).
- stdin is never read: `_read_stdin` runs only when `spec.reads_stdin`
  (`rpp/perop.py:135`).
- Parity §list-running (`docs/provider-herdr-parity.md:534`) verdict:
  "intentional (with a mistake inside)"; status implemented.

### Inputs

- gc argv: `script list-running <prefix>`
  (`internal/runtime/exec/exec.go:764`;
  `docs/reference/exec-session-provider.md:138`). gc always passes one arg,
  possibly `""`. Callers use `""` (`cmd/gc/cmd_stop.go:637`,
  `cmd/gc/controller.go:1156`, `cmd/gc/cmd_start.go:1726`) or a session name
  (`cmd/gc/session_async_start_drift.go:122,156`).
- Pack: `prefix = args[0] if args else ""` (`herdrcore/reads.py:308`). Extra args
  are ignored by the handler (nothing reads `args[1:]`; whether any caller
  sends extras is `unverified`). No flags exist.
- The match is `name.startswith(prefix)` – case-sensitive, no glob or regex
  (`herdrcore/reads.py:319`).
- Preconditions: none. The op needs no existing session. It reads the core with
  `ctx.client(read=True)` (`herdrcore/reads.py:310`). If the core is down, the op
  may kick a start of it (`herdrcore/reads.py:91-95,314`).

### Success output

- stdout: one gc session name per line, each `name + "\n"`, UTF-8 with
  `surrogateescape` (`herdrcore/reads.py:321`). No JSON, no quoting, no escaping.
- Ordering: `sorted(...)` on Python `str` (code-point order)
  (`herdrcore/reads.py:316`). The pack writes the sorted list as-is. gc does not
  sort here (`internal/runtime/exec/exec.go:771`).
- A name is listed only if all hold (`herdrcore/reads.py:319`):
  - it starts with the prefix;
  - its newest record `found[0]` (`herdrcore/records.py:79-86`, newest by
    `created_ms`, `herdrcore/records.py:34-40`) is `is_intact`: live, pid set,
    reason not in `BROKEN_REASONS`, and the pid label equals the pid, or there is
    no label and the terminal is not `keep` (`herdrcore/records.py:53-62`);
  - `readable_name`: non-empty, equal to its `.strip()`, no Unicode `Cc`
    character (`rpp/wire.py:194-196`).
- Source set: city-labelled, non-finished terminals
  (`herdrcore/reads.py:68-73,310`). Records without a session label are dropped
  (`herdrcore/records.py:83-85`).
- Empty cases, all exit 0 with empty stdout:
  - no matching names;
  - the city is stopped: core unreachable, parked, and nothing runs
    (`herdrcore/reads.py:312-313`, `herdrcore/core.py:307-314`,
    `herdrcore/corescan.py:123-135`). Test:
    `tests/unit/test_herdrcore_server.py:445-446` (code 0, text "").
- Exit code on success: 0 (`OpResult.ok`, `rpp/ops.py:69-70`).
- gc parser (`internal/runtime/exec/exec.go:763-772`): `p.run` returns
  `strings.TrimRight(stdout, "\n")` (`internal/runtime/exec/exec.go:104`). Empty
  gives `[]string{}`. Otherwise `strings.Split(out, "\n")`. No per-line trim, no
  `\r` handling, no blank-line skip. So gc treats a name with `\n` as two names,
  and padding is kept verbatim. This is why `readable_name` exists.
- Doc: "one name per line"
  (`docs/reference/exec-session-provider.md:138,366-367`).
- Tests: `tests/unit/test_herdrcore_lifecycle.py:258-263` (prefix `mc-` gives
  `mc-a`, `mc-b`; after `stop mc-a`, `""` gives `mc-b`, `other`).

### Failure cases

| Case | Exit | stdout | Source |
|---|---|---|---|
| core unreachable, not a stopped city | 1 | empty | `herdrcore/reads.py:311-315` (`OpError`); `rpp/perop.py:147-148`. Kicks a start if `HcUnreachable` (`herdrcore/reads.py:91-95`) |
| `GC_RPP_FAULT=unavailable` (test hook) | 1 | empty | `hc/client.py:311-312`. Test `tests/unit/test_herdrcore_lifecycle.py:266-268` (`code == 1 and stdout == b""`) |
| core call times out, connection lost, or bad reply (`HcTimeout`, `HcConnectionLost`, `HcProtocolError`) | 1 | empty | `hc/errors.py:56-82`. All `HcError`s hit `herdrcore/reads.py:311,315`. `city_stopped` is true only for `HcUnreachable` (`herdrcore/reads.py:101`). |
| state-dir, config, meta, or lock-file error during op setup or handler | 1 | empty | `herdrcore/ops_table.py:48-49` (`StateDirError`, `ConfigError`, `MetaError`, `LockFileError` become `OpError`). Which of these `list-running` can raise: unverified (the handler reads no meta and takes no lock). |
| lock wait timeout | 1 | empty | `herdrcore/ops_table.py:50-51`. Not reachable from this handler (no lock): unverified. |
| deadline passed before the handler | 1 | empty | `rpp/perop.py:144-145`. A deadline hit during the core call is an `HcTimeout`, see above. |
| SIGTERM or cancel | 1 | empty | `rpp/perop.py:143,149-150` (`HcCancelled` on `list_city` is caught by `except HcError` at `herdrcore/reads.py:311-315` and maps to `cannot read the city's core: ...`, `hc/errors.py:107-112`) |
| any other exception | 1 | empty | `rpp/perop.py:153-154` ("internal error: ...") |
| answer over 8 MiB (8,388,608 bytes) | 1 | empty | `rpp/perop.py:158-159`. gc sees a normal failed op. |
| unencodable name | 1 (lone surrogates outside U+DC80–U+DCFF) | empty | Byte-escape surrogates `U+DC80–U+DCFF` round-trip as raw bytes via `surrogateescape` (`herdrcore/reads.py:321`). `readable_name` excludes `Cc`, not `Cs` (`rpp/wire.py:194`), and `jsonval.loads` accepts JSON-escaped surrogates (`rpp/jsonval.py:21-25`), so an arbitrary lone surrogate (e.g. `\ud800`) passes `readable_name` and raises `UnicodeEncodeError` at `herdrcore/reads.py:321`, becoming exit 1 `internal error` (`rpp/perop.py:153-154`). Empty, padded, or `Cc`-containing names are omitted (`rpp/wire.py:194-196`). |
| stdout pipe closed by gc | 1 | partial | `rpp/perop.py:160-166` |
| unknown or extra args | 0 | normal | Extra args are ignored (`herdrcore/reads.py:308` reads only `args[0]`; whether callers send extras is unverified). |

gc side on failure: a non-zero exit (not 2) goes through `runError` and becomes
an error with stderr (`internal/runtime/exec/exec.go:153-169`). `ListRunning`
returns `nil, err` (`internal/runtime/exec/exec.go:765-767`). `gc stop` marks
the inventory incomplete and exits 1 (`cmd/gc/cmd_stop.go:637-650,861-864`).
Exit 2 would be read as success with empty output
(`internal/runtime/exec/exec.go:159-161`). The pack never returns 2 from this op,
because `OpError` defaults to code 1 (`rpp/ops.py:78`): unverified for all
`OpError` raisers, true for the paths above. gc time-out: 30 s
(`internal/runtime/exec/exec.go:56,80`).

### Conflicts

- **The 8 MiB cap is not in gc v1.5.1.** The pack applies
  `INVENTORY_STDOUT_CAP` to `list-running`. gc v1.5.1 has no cap on any op
  (`internal/runtime/exec/exec.go:94-95` uses an unbounded `bytes.Buffer`). The
  doc at `docs/reference/exec-session-provider.md` has no `inventory`,
  `observe`, or cap text at v1.5.1 (`rg` finds none), though the pack cites it
  (`rpp/ops.py:35-37`). No gc v1.5.1 behaviour conflicts, because gc accepts any
  size. The pack just fails earlier. Parity row 4 agrees ("`list-running` is not
  capped in the RPP v0 baseline"), verdict intentional. The pack's reason for
  capping `list-running` is **unverified**.
- **Trailing-newline form.** The pack emits `name\n` per line. gc trims trailing
  `\n`s (`internal/runtime/exec/exec.go:104`), so the form is compatible.
- **Name filter.** The pack's `readable_name` omits names gc could mis-split. gc
  has no filter and no spec rule (`internal/runtime/exec/exec.go:771`).
  Compatible. Parity row 3 flags the *Rex provider's* missing guard as
  "mistake". It is not a pack-vs-gc conflict.
- **Prefix semantics.** gc defines only "sessions whose names have the prefix"
  (`internal/runtime/runtime.go:215-217`), and the pack's `startswith` matches.
  Matches the parity "Verdict" line.
- **Line-number note.** The source and the parity doc both say
  `herdrcore/ops_table.py:76`.
- **Parity-doc claims re-checked against sources.** It cites
  `herdrcore/reads.py:307-308,310-313,314-321`, `herdrcore/records.py:53-62`,
  `rpp/wire.py:194-196`, `rpp/perop.py:158-159`, `rpp/ops.py:34`,
  `cmd/gc/cmd_stop.go:861-864`. All hold. One imprecision: the doc says
  `herdrcore/reads.py:68-73,310` for city scoping, which is right. The doc says
  `rpp/ops.py:35` for the cap statement, but the `INVENTORY_STDOUT_CAP` constant
  is at `rpp/ops.py:34` and the docstring at `rpp/ops.py:35-37`. This is
  trivial.
- Parity verdict: **intentional (with a mistake inside)** (the mistake is on the
  Rex side, row 3). Status: implemented. No verdict changed.

### Unverified

- Whether the handler can raise `StateDirError`, `ConfigError`, `MetaError`,
  `LockFileError`, or `LockTimeout` (the cases in the table come from the
  `_wrap` catch list, not from a traced path).
- Whether `OpContext(request, host)` (`herdrcore/context.py`) fails on a missing
  or unwritable state dir for this op.
- Whether any caller sends extra args beyond `args[0]` (the handler ignores
  them by construction, `herdrcore/reads.py:308`).
- Why the pack caps `list-running` at 8 MiB (no gc v1.5.1 source says so).
- Whether `hc` can return duplicate session labels across live records that
  survive `group_by_session`. Only the newest record's `is_intact` is tested
  (`herdrcore/reads.py:319`), so a stale older live record is ignored.
- The sort order for non-BMP or `surrogateescape` names vs gc consumers: gc does
  not rely on order, but not every caller was traced.
- The exact upper-bound runtime of `hc` `list_terminals` under the 30 s deadline
  (`CallTimeouts.for_op(read=True)` not read).

## is-attached

### Registration

- `"is-attached": spec(reads.op_is_attached, deadline="probe")` –
  `herdrcore/ops_table.py:77`. The parity doc cites `herdrcore/ops_table.py:77` for this op; its
  `list-running` section cites `herdrcore/ops_table.py:76`.
- `spec()` defaults: `stdin=False`, `cap=SMALL_STDOUT_CAP`
  (`herdrcore/ops_table.py:64`). So `reads_stdin=False` and the stdout cap is
  64 KiB (`rpp/ops.py:32`: `64 << 10`). stdin is never read
  (`rpp/perop.py:135`: only when `spec.reads_stdin`).
- `deadline="probe"` = 4.0 s (`rpp/ops.py:26-30`, `DEFAULT_DEADLINES_S`), used
  only when gc sends no `GC_EXEC_DEADLINE_MS` (`rpp/perop.py:31,129`;
  `deadline_for`, `rpp/ops.py:119-128`). The deadline counts from `_STARTED`
  (`bin/gc-runtime-herdr-core:12`, `rpp/ops.py:128`). Rationale (docstring `rpp/ops.py:28-30`): "4 s for
  observe and a v2-ready is-attached (gc `exec/observe.go` probeOpTimeout)".
  Test: `tests/unit/test_rpp_cancel.py:95-101`.
- Capability: the handshake declares `report-attachment` (`handshake.py:36`;
  `herdrcore/ops_table.py:4`; `rpp/wire.py:26` as an allowed value).
- `housekeeping=True` default: `sweep_spawn_files` runs before the handler
  (`herdrcore/ops_table.py:41-47`).
- Parity §is-attached (`docs/provider-herdr-parity.md:568`) verdict:
  **intentional**.

### Inputs

- argv: `<entry> is-attached <name>`. The handler reads `args[0]` as the exact gc
  session name via `session_arg` (`herdrcore/reads.py:57-60`). A missing or empty
  arg gives `OpError("missing session name")`, exit 1. No prefix form. Extra args
  are ignored (nothing reads `args[1:]`; `rpp/perop.py:133`).
- Preconditions: a readable city core (`current_record`,
  `herdrcore/reads.py:263-278`); the name is the gc session label
  (`LABEL_SESSION`, `herdrcore/reads.py:68-73`). No stdin, no environment other
  than the city paths and `GC_EXEC_DEADLINE_MS`. Needs a readable process table
  for the viewer scan (`herdrcore/attach.py:68-83`).
- `is-attached` uses `current_record(ctx, name)` without `liveness=True`
  (`herdrcore/reads.py:326`), unlike `is-running` and `process-alive`
  (`herdrcore/reads.py:288,295`). A down core therefore kicks a start and fails
  (see "Failure cases") instead of answering `false`, unless the city is parked
  and nothing of it runs.

### Success output

- Handler (`herdrcore/reads.py:324-334`): no record gives
  `OpResult.answer("false")`; otherwise `encode_bool(attached(...))`.
  `OpResult.answer` = line + `\n`, exit 0 (`rpp/ops.py:72-75`); `encode_bool`
  gives `true`/`false` (`rpp/wire.py:293-295`).
- Attached: stdout `true\n`, exit 0. Not attached (or no record): stdout
  `false\n`, exit 0.
- "Attached" = `lock_held(attach lock of the record's object)` OR the record's
  address or name is in the `hc attach` / `herdr-core attach` viewer set
  (`herdrcore/attach.py:86-90`, `lock_held` `herdrcore/attach.py:42-54`, `_attach_target` `herdrcore/attach.py:57-63`,
  `viewer_targets` `herdrcore/attach.py:68-83`). Viewer processes must be the same uid and their
  environ's `HC_SOCKET` or `HC_SESSION` must match this city
  (`herdrcore/attach.py:77-81`). It never queries the server for clients.
- The record may be finished: `current_record` returns the newest record "live or
  finished" (`herdrcore/reads.py:263`) and `attached()` does not call
  `is_intact` (`herdrcore/attach.py:86-90`). A finished record with a held lock or
  a viewer reads `true` (code behaviour confirmed; no dedicated unit test
  found).
- gc reader: `Provider.IsAttached` (`internal/runtime/exec/exec.go:684-692`):
  returns false if `report-attachment` is not declared
  (`internal/runtime/exec/exec.go:685-686`, via `handshakeCapability`,
  `internal/runtime/exec/handshake.go:45-51`); runs `p.run(nil, "is-attached",
  name)` (`internal/runtime/exec/exec.go:688`); any error gives false
  (`internal/runtime/exec/exec.go:689-691`); otherwise `out == "true"`
  (`internal/runtime/exec/exec.go:692`). `run` trims trailing `\n` only
  (`internal/runtime/exec/exec.go:104`), so `true\n` matches. Anything else,
  including empty stdout, is false.
- Does gc v1.5.1 call it? Yes, only when the handshake declares
  `report-attachment`: `docs/reference/exec-session-provider.md:193`;
  `internal/runtime/exec/exec.go:685`. Callers: `execAttachment.Observe`
  (`internal/runtime/exec/seams.go:233`),
  `internal/worker/runtime_handle.go:380`, `internal/session/manager.go:1975`.
  Capability exposure: `internal/runtime/exec/exec.go:823`,
  `internal/runtime/protocol.go:17-20`. Doc op row:
  `docs/reference/exec-session-provider.md:141` ("`true` or `false`").
- Exit 2: gc maps exit 2 to a nil error
  (`internal/runtime/exec/exec.go:155-158`;
  `docs/reference/exec-session-provider.md:77`); `is-attached` is not in
  `exitTwoIsFailure` (`start` and `provision` only,
  `internal/runtime/exec/exec.go:170-175`). Result: empty `out`, so `IsAttached`
  = false. The pack never returns 2 for this op (it is in the table); exit 2
  appears only for unknown ops (`rpp/perop.py:119-120`). Parity §is-attached does
  not discuss exit 2 for this op explicitly; its Verdict text states "the op's
  output must be exactly `true`, and any error becomes `false`" (consistent with
  `internal/runtime/exec/exec.go:688-692`).

### Failure cases

All failures: stdout empty, message on stderr as `is-attached: <msg>`
(`rpp/perop.py:166-167`). gc reads every one as `false`
(`internal/runtime/exec/exec.go:689-691`). Generic runner failures are in the
shared contract.

| Case | Exit | stdout | Source |
|---|---|---|---|
| unknown name (no record) | 0 | `false\n` | `herdrcore/reads.py:326-328`; test `tests/unit/test_herdrcore_procs.py:352` (`nobody`) |
| missing or empty name arg | 1 | empty | `herdrcore/reads.py:57-60` |
| core unreachable, city not parked or something still runs | 1, `cannot read the city's core: …`; also kicks a core start | empty | `herdrcore/reads.py:272-278`, `unreachable` `herdrcore/reads.py:91-95`; test `tests/unit/test_herdrcore_lifecycle.py:322-336` (`is-attached` gives code 1) |
| core unreachable, parked AND `nothing_runs` | 0 | `false\n` | `herdrcore/reads.py:274-276` (`parked(ctx) and nothing_runs(ctx)` AND logic confirmed at `:275`, `herdrcore/core.py:307-315`; no dedicated test for this op) |
| other hc error (non-unreachable) | 1 `cannot read the city's core: …` | empty | `herdrcore/reads.py:272-278` |
| process table unreadable or viewer environ unreadable (`ProcError`) | 1 `cannot tell whether 'name' is attached: …` | empty | `herdrcore/reads.py:333-334`; tests `tests/unit/test_herdrcore_procs.py:373-381`, `tests/unit/test_herdrcore_procs.py:384` and following (viewer environ) |
| attach lock file unopenable (`LockFileError`, not ENOENT) | 1, same message | empty | `herdrcore/reads.py:333-334`, `herdrcore/attach.py:42-54` |
| state dir, config, meta or lock-timeout errors | 1 | empty | `herdrcore/ops_table.py:41-52` (`_wrap`) |
| fault `GC_RPP_FAULT=unavailable` | 1 | empty | `hc/client.py:312`; test `tests/unit/test_herdrcore_lifecycle.py:285` |
| probe deadline (4 s, or gc's `GC_EXEC_DEADLINE_MS`) already spent before the handler | 1 `no time left before the op's deadline` | empty | `rpp/perop.py:144-145` |
| deadline hits during an hc call | 1 `cannot read the city's core: ...` | empty | `HcTimeout` subclasses `HcError` (not `HcUnreachable`, `hc/errors.py:56-72`), so `hc/client.py:364-371` raises `HcTimeout` and `current_record` maps it via `except HcError` at `herdrcore/reads.py:274-278` (no dedicated test) |
| SIGTERM or cancel | 1 `canceled (terminated)` (direct `Cancelled`), or `cannot read the city's core: ...` (`HcCancelled` caught by `except HcError`, `herdrcore/reads.py:274-278`, `hc/errors.py:107-112`) | empty | `rpp/perop.py:149-152` |
| unexpected exception | 1 `internal error: …` | empty | `rpp/perop.py:153-156` |
| stdout over cap | 1 `answer of N bytes is over the 65536-byte cap` | empty | `rpp/perop.py:158-159`; not reachable for a 6-byte answer |
| process-table read raced by an exiting viewer | the viewer is skipped, not an error | – | `herdrcore/attach.py:79-80` |

gc-side deadline: gc v1.5.1 applies the 30 s default to `is-attached` (it uses
`p.run`, `internal/runtime/exec/exec.go:62-64`, timeout set at
`internal/runtime/exec/exec.go:56`; doc
`docs/reference/exec-session-provider.md:116-119`). On timeout gc cancels, and
the error gives false (`internal/runtime/exec/exec.go:129,689-691`).

### Conflicts

1. **Probe deadline source – conflict (pack vs gc v1.5.1).** The pack's
   rationale cites gc `exec/observe.go` `probeOpTimeout` (`rpp/ops.py:30`; test
   comment `tests/unit/test_rpp_cancel.py:99`). In tag v1.5.1 there is no
   `internal/runtime/exec/observe.go` (the directory holds `internal/runtime/exec/exec.go`,
   `internal/runtime/exec/handshake.go`, `internal/runtime/exec/json.go`, `internal/runtime/exec/seams.go`, `internal/runtime/exec/cutover.go`), no `probeOpTimeout`, no
   `observe.v1`, and no `GC_EXEC_DEADLINE_MS` anywhere in the tree (grep over the
   v1.5.1 tree: no hits). v1.5.1 gives `is-attached` the 30 s default
   (`internal/runtime/exec/exec.go:56,62-64`;
   `docs/reference/exec-session-provider.md:116-119`). The 4 s class is a
   pack-side design aimed at a later gc. Matches parity row 4 ("gc 1.5.1 baseline
   is 30 s … 4 s is a v2 observe-probe value"). Practical effect with v1.5.1: the
   pack still enforces 4 s internally, tighter than gc's 30 s; harmless.
2. **Capability gating.** gc v1.5.1 honours `report-attachment`
   (`internal/runtime/exec/exec.go:685`); the pack declares it
   (`handshake.py:36`). No conflict.
3. **Semantics.** The pack answers via the process table plus a lock, not a
   server client list (`herdrcore/attach.py:86-90`); gc only needs `true`/`false`
   (`internal/runtime/exec/exec.go:692`). No conflict. Parity rows 1 and 2 agree
   (intentional).
4. **Error handling.** The pack exits 1 on "cannot tell"
   (`herdrcore/reads.py:333-334`); gc reads that as `false`
   (`internal/runtime/exec/exec.go:689-691`), i.e. fail-open for gc's attachment
   gates. The parity doc names this (row 5 and Verdict). Parity §is-attached
   row 6 further attributes a stop-side "cannot tell as attached" gate to
   `herdrcore/lifecycle.py:574-582`. In the pinned pack, `herdrcore/lifecycle.py:574-577`
   is inside `_kill_object`, not `op_stop` (`herdrcore/lifecycle.py:511-525`),
   `_recycle` (`herdrcore/lifecycle.py:420-446`), or `kill_incarnation`
   (`herdrcore/lifecycle.py:327-372`), none of which check attachment. That
   parity row-6 attribution is an error in `docs/provider-herdr-parity.md`;
   `kill-object` carries the conservative attachment guard, not `stop`. Per
   parity row 6, gc v1.5.1 never calls `is-attached` before the `stop` op –
   not re-checked (`unverified` here).
5. **Parity verdict: intentional**, no change. The parity claims "`herdrcore/ops_table.py:77`" and "`herdrcore/reads.py:324-334`, `herdrcore/attach.py:68-83,86-90,42-54`" are
   correct against the pin. The parity text "probe deadline … `rpp/ops.py:26-30`"
   is correct. The parity "Follow-up: confirm no per-op deadline override in
   main.go" is about the Rex provider, not this pack.

### Unverified

- Exact stderr text and dedicated test coverage when the 4 s budget runs out
  inside `list_terminals` (`HcTimeout` is confirmed to subclass `HcError` rather
  than `HcUnreachable`, `hc/errors.py:56-72`, so `current_record` maps it to
  exit 1 `cannot read the city's core: ...` at `herdrcore/reads.py:274-278`).
- Dedicated test coverage for a finished record with a held lock or viewer
  (`attached()` at `herdrcore/attach.py:86-90` does not filter out finished
  records) and for the `parked(ctx) and nothing_runs(ctx)` branch on this op
  (`herdrcore/reads.py:275` explicit AND logic confirmed).
- That gc v1.5.1 never invokes `is-attached` in the `stop` path (parity row 6
  claim; not re-checked).
- Whether any gc version after v1.5.1 sends `GC_EXEC_DEADLINE_MS` or defines
  `probeOpTimeout` (not in the pinned tree; outside both sources).
- Corroborating tests read: `tests/unit/test_herdrcore_procs.py:346-382` (lock
  gives `true`, none gives `false`, `nobody` gives `false`, broken `/proc` gives
  exit 1 "cannot tell"), `tests/unit/test_herdrcore_core.py:249-258` (a real
  `attach` viewer reads `true`), `tests/unit/test_herdrcore_lifecycle.py:285,322-336,555`,
  `tests/unit/test_herdrcore_server.py:447` (`false` after server stop).

## get-last-activity

### Registration

- `herdrcore/ops_table.py:78`: `"get-last-activity":
  spec(reads.op_get_last_activity)`.
- Defaults of `spec` (`herdrcore/ops_table.py:64`): `stdin=False`
  (`reads_stdin=False`); `deadline="default"`; `cap=SMALL_STDOUT_CAP`;
  `housekeeping=True` (runs `lifecycle.sweep_spawn_files` before the handler,
  `herdrcore/ops_table.py:41-47`).
- Deadline class `default` = 30 s unless gc sends `GC_EXEC_DEADLINE_MS`
  (`rpp/ops.py:23-27`, `rpp/perop.py:31,129`). gc's own timeout is 30 s
  (`internal/runtime/exec/exec.go:56`).
- Stdout cap `SMALL_STDOUT_CAP` = 64 KiB (`rpp/ops.py:32`); over-cap gives exit 1,
  no stdout (`rpp/perop.py:158-159`). Unreachable for a one-line answer.
- The capability `report-activity` is declared (`rpp/wire.py:27`;
  `tests/unit/test_herdr_core_entrypoint.py:72`).
- Parity §get-last-activity (`docs/provider-herdr-parity.md:599-617`; summary
  rows `docs/provider-herdr-parity.md:66,141-145`) verdict **intentional**.

### Inputs

- argv: `get-last-activity <name>`; stdin not read (`reads_stdin=False`,
  `rpp/perop.py:135`). gc sends exactly `p.run(nil, "get-last-activity", name)`
  with nil stdin (`internal/runtime/exec/exec.go:839`;
  `docs/reference/exec-session-provider.md:139`).
- `session_arg` (`herdrcore/reads.py:57-60`) takes `args[0]`; missing or empty
  gives `OpError("missing session name")`. Extra args are ignored by the
  handler (nothing reads `args[1:]`; whether callers send extras is
  `unverified`).
- Preconditions: none beyond a reachable hc core. `current_record`
  (`herdrcore/reads.py:263-279`) lists the city's records for the name and takes
  the newest (`herdrcore/records.py:48`). The handler needs the newest record to
  be intact (`herdrcore/records.py:53-61`: live, has a pid, `reason` not in
  `BROKEN_REASONS` (`herdrcore/records.py:26`), pid label matches).

### Success output

- One line, **bare** (not JSON): RFC 3339 UTC with milliseconds and a literal
  `Z`, then `\n`. Example `2026-11-09T10:05:00.123Z`. Built by
  `rfc3339_ms(epoch_ms)` (`rpp/wire.py:298-304`: `%Y-%m-%dT%H:%M:%S` + `.mmm` +
  `Z`) and `OpResult.answer` (`rpp/ops.py:72-75`). Exit 0 (`ExitCode.OK`,
  `rpp/exitcodes.py:11`). Timezone UTC; the input is epoch **milliseconds**
  (`host.now_ms()`).
- Value (`herdrcore/activity.py:126-137`):
  1. status `working` or `blocked` gives now (`herdrcore/activity.py:29,130-131`);
  2. a program that sent OSC 7501 (`record.reported`) gives `max(status_ms or
     created_ms, input_ms)` (`herdrcore/activity.py:133-134`), from the cached
     activity.json entry (`herdrcore/reads.py:343-350`, specifically `:349-350`),
     with no screen read;
  3. never reported: the screen is read via `terminal.read`
     (`herdrcore/reads.py:342-346`, `hc/client.py:250-254`), sha256-hashed, and
     the value is `max(changed_ms, input_ms)`
     (`herdrcore/activity.py:94-104,135-137`). First sight of a screen = now;
  4. no `seen` entry or `changed_ms == 0` (no `gc.obj` label, or the lock wait
     fell back) gives now (`herdrcore/activity.py:135-136`).
- "Never active" case: **the pack never prints empty for a box that exists and is
  intact**; the unknown-activity fallback is "now"
  (`herdrcore/activity.py:135-136`, docstring `herdrcore/activity.py:19-22`), a
  deliberate choice so a busy box is not read as idle. Empty stdout plus exit 0
  happens only when there is no record or the box is not intact (see "Failure
  cases").
- gc parser: `GetLastActivity` (`internal/runtime/exec/exec.go:838-852`): `run`
  returns stdout with the trailing `\n` trimmed
  (`internal/runtime/exec/exec.go:104`); `""` gives a zero time and nil error
  (`internal/runtime/exec/exec.go:843-845`,
  `docs/reference/exec-session-provider.md:371-372`, test
  `internal/runtime/exec/exec_test.go:1395-1412`); `time.Parse(time.RFC3339,
  out)`; a parse failure gives a zero time and nil error
  (`internal/runtime/exec/exec.go:846-851`, test
  `internal/runtime/exec/exec_test.go:1414-1430`). Go's RFC 3339 parse accepts
  fractional seconds: checked by running `time.Parse(time.RFC3339,
  "2026-11-09T10:05:00.123Z")`, which parsed to `10:05:00.123 +0000 UTC` (Go
  stdlib behaviour, not in gc source). The gc test uses whole seconds only
  (`internal/runtime/exec/exec_test.go:54,1389`), so ms input has no gc test.
- Seam: production gc reaches it via `Observe`, which discards the error
  (`internal/runtime/exec/seams.go:229-231`); parity line 306
  (`docs/provider-herdr-parity.md:306`) notes gc calls it regardless of
  `report-activity`.

### Failure cases

Generic runner failures are in the shared contract.

| Case | Exit | stdout | Source |
|---|---|---|---|
| no record for the name | 0 | empty | `herdrcore/reads.py:338-340` |
| record exists but is not live, is finished, has no pid, has a broken reason, or has a pid-label mismatch (dead or replaced box) | 0 | empty | `herdrcore/reads.py:338-340`; `herdrcore/records.py:53-61` |
| city parked (`server-stop`) or nothing of the core runs, and core unreachable | 0 | empty | `herdrcore/reads.py:263-276` (`parked` / `nothing_runs` give `None`). Note `liveness=False` here, so it needs `parked(ctx)`: an unreachable core in a non-parked city is the next row |
| core unreachable, city not parked | 1 | empty; stderr `get-last-activity: cannot read the city's core: …`; also kicks a start (`herdrcore/reads.py:91-95`) | `herdrcore/reads.py:277-279` |
| hc error listing records (not unreachable) | 1 | empty | `herdrcore/reads.py:277-279` |
| hc error reading the screen (unreported pane), including `not_found` if the terminal vanishes after the listing | 1 | empty; stderr `cannot read the screen: …` | `herdrcore/reads.py:343-346` |
| missing or empty `<name>` | 1 | empty; `missing session name` | `herdrcore/reads.py:57-60`; `rpp/perop.py:147-148` |
| deadline already expired before the handler | 1 | empty; `no time left before the op's deadline` | `rpp/perop.py:144-145` |
| deadline or cancel (SIGTERM) during an hc call | 1 (`HcTimeout` / `HcCancelled` are `HcError`, caught at `herdrcore/reads.py:274-278` or `:346-347`; direct `Cancelled` goes to `rpp/perop.py:149-150`) | empty | `hc/client.py:364-371,395-403`; `hc/errors.py:67,107-112`; `herdrcore/reads.py:274-278,346-347`; `rpp/perop.py:149-150` |
| state dir, config, metadata or lock error escaping the wrapper | 1 | empty | `herdrcore/ops_table.py:46-49` |
| activity.json unreadable, lock wait of 0.5 s exceeded, or write fails | 0 (not an error) | a timestamp from an unpersisted entry (so "now" for unreported screens) | `herdrcore/activity.py:36-37,74-85` (`_update` falls back to `change(Seen())`) |
| answer over the 64 KiB cap | 1 | none | `rpp/perop.py:158-159` (cannot occur for a one-line answer) |
| unexpected exception (for example an `rfc3339_ms` `ValueError` on negative ms, `rpp/wire.py:300-301`) | 1 | empty; `internal error: …` | `rpp/perop.py:153-155` |

- No exit code other than 0 or 1 is produced. Exit 2 would read as success in gc
  (`internal/runtime/exec/exec.go:159-161`, `exitTwoIsFailure`
  `internal/runtime/exec/exec.go:174-181`), but the pack never emits it for a
  known op (`rpp/exitcodes.py:17-19`).
- gc side: a non-zero exit makes `GetLastActivity` return `(zero, err)`
  (`internal/runtime/exec/exec.go:840-842`); the seam ignores the error
  (`internal/runtime/exec/seams.go:230`).

### Conflicts

- Pack vs gc v1.5.1: **no conflict**. The output is RFC 3339
  (`docs/reference/exec-session-provider.md:139`), empty means
  "unsupported/unknown" and is accepted
  (`docs/reference/exec-session-provider.md:371-372`), and the ms fraction parses
  (see "Success output"). Only the doc's wording "RFC3339 or empty" is silent on
  fractional seconds.
- Parity doc: verdict **intentional** (`docs/provider-herdr-parity.md:617`);
  matches the pack on the ms format (parity line 141, `rpp/wire.py:298-304`),
  working/blocked giving now (line 142, `herdrcore/activity.py:129-131`), the
  input floor (line 143, `herdrcore/activity.py:132-137`), the flock of 0.5 s
  (line 144, `herdrcore/activity.py:74-85`), and cached state for reported panes
  (line 145, `herdrcore/reads.py:349-350`; parity cites `reads.py:348-349`).
  Citations in the doc's herdr-core paragraph (`herdrcore/reads.py:336-351`,
  `herdrcore/activity.py:94-104`, `rpp/wire.py:298-304`, `handshake.py:37`) all
  hold at this pin (`handshake.py:37` declares `report-activity`, and
  `rpp/wire.py:27` lists it as an allowed `Literal` value).
- Parity gap not captured in the doc: parity row 5 and the paragraph say an
  unreported box whose screen was not read returns "now" only implicitly; this
  section states it explicitly (`herdrcore/activity.py:135-136`).
- Gap in the doc: the "no record or dead box gives empty, exit 0" rows are
  described only as `OpResult.ok()` for "missing or not intact"; the hc-failure
  exit-1 paths (`herdrcore/reads.py:277-279`, `herdrcore/reads.py:343-346`) are not in the pack's
  own deviation table (row 5 covers only the Rex side).

### Unverified

- No pack test exercises the `get-last-activity` op end to end. Only
  `inventory`'s `last_activity` is tested
  (`tests/unit/test_herdrcore_lifecycle.py:200,346-373`, the same
  `herdrcore/activity.py` code); `tests/unit/test_rpp_wire.py:211-237` tests the
  inventory field format only.
- No gc test feeds a millisecond RFC 3339 value; acceptance rests on the Go
  stdlib run noted above.
- `hc` behaviour for a terminal that finished between listing and `read_screen`
  (whether real `hc` answers `not_found`, mapped to exit 1 `OpError` at
  `herdrcore/reads.py:346-347`; `CallTimeouts.for_op` at `hc/client.py:140-148`
  gives 1 s hello / 3 s query budgets, and `_timeout`/`_arm` at
  `hc/client.py:364-371,395-403` raise `HcTimeout`/`HcCancelled`, both caught as
  `HcError`).
- Whether any caller sends extra argv beyond `<name>` (the handler ignores
  them by construction, `herdrcore/reads.py:57-60`, and gc sends only `<name>`,
  `internal/runtime/exec/exec.go:839`).
- Concurrency: whether gc calls the op concurrently for one name (parity row 4
  also marks this unverified).

## peek

### Registration

- `"peek": spec(reads.op_peek, cap=OBSERVE_STDOUT_CAP)` at
  `herdrcore/ops_table.py:79` (the parity doc also says `herdrcore/ops_table.py:79`).
- `reads_stdin=False`: `spec` defaults `stdin=False`
  (`herdrcore/ops_table.py:64`). The runner only reads stdin when
  `spec.reads_stdin` (`rpp/perop.py:135`), so stdin is not read.
- Deadline class `"default"` (`herdrcore/ops_table.py:64`) = 30.0 s
  (`rpp/ops.py:23-25`). `GC_EXEC_DEADLINE_MS` overrides it when well-formed
  (`rpp/perop.py:39-47`, `rpp/ops.py:118-128`).
- Cap `OBSERVE_STDOUT_CAP = 1 << 20` = 1 MiB (`rpp/ops.py:33`).
- Every op is wrapped by `_wrap` (`herdrcore/ops_table.py:41-53`), which runs
  `lifecycle.sweep_spawn_files` first (housekeeping default True).
  `StateDirError`, `ConfigError`, `MetaError` and `LockFileError` become exit 1,
  and `LockTimeout` becomes exit 1 `busy: …`.
- Parity §peek (`docs/provider-herdr-parity.md:632-652`) verdict:
  **intentional** (§peek Verdict, line 652; summary table line 67
  `implemented | 7 | intentional`).

### Inputs

- argv: `peek <name> [lines]`. gc always sends both:
  `p.run(nil, "peek", name, strconv.Itoa(lines))`
  (`internal/runtime/exec/exec.go:621-623`); docs `script peek <name> <lines>`,
  stdin none, "captured text"
  (`docs/reference/exec-session-provider.md:137`).
- `name`: `session_arg` raises exit 1 `missing session name` when args are empty
  or `args[0]==""` (`herdrcore/reads.py:57-60`).
- `lines`: `args[1]`, default `"0"` when absent (`herdrcore/reads.py:359`). Parsed
  with `int()` (`herdrcore/reads.py:360-363`). Extra args are ignored by the
  handler (the code only reads `args[0]` and `args[1]`; whether callers send
  extras is `unverified`). Python `int()` also accepts forms such as `" 5"` and
  `"+5"`; gc never sends them (`internal/runtime/exec/exec.go:622`).
- `lines <= 0` (0 or negative) means the whole screen
  (`herdrcore/reads.py:376`). `lines > 0` means the last N rows
  (`herdrcore/reads.py:376-377`).
- Preconditions: a reachable hc core, or a city whose core is unreachable
  while both parked and nothing of the core runs (which yields exit 66;
  `herdrcore/reads.py:272-278`, `liveness=False`). The op does not check
  `is_intact`: a finished record is still readable. Test: a dead one-shot's
  screen still peeks, exit 0 (`tests/unit/test_herdrcore_lifecycle.py:140-145`).

### Success output

- Flow (`herdrcore/reads.py:357-378`): `current_record` finds the name's newest
  record, live or finished (`herdrcore/reads.py:262-279`,
  `herdrcore/records.py:48-50`). Then `read_screen(record.address).text` runs
  `terminal.read` with `{"terminal": …, "lines": 0}`. That is the screen only,
  never a scroll (`hc/client.py:250-254`). Then `text.split("\n")`, take
  `rows[-lines:]` when `lines > 0`, and `"\n".join(rows) + "\n"` encoded
  UTF-8/`surrogateescape` (`herdrcore/reads.py:375-378`).
- Exit 0. stdout = the screen text, **always ending in exactly one added `\n`**.
  If the hc text already ends in `\n`, the split yields a trailing empty row,
  which is kept and counted toward N. Examples (derived from the code, not run):
  text `"a\nb"` with lines 0 gives `"a\nb\n"`. Text `"a\nb\n"` with lines 0
  gives `"a\nb\n\n"`. Text `"a\nb\n"` with lines 1 gives `"\n"`. Whether real hc
  text ends in `\n` is unverified (parity row 4, "hc text ending unverified").
- Empty screen (`text == ""`): rows `[""]`, stdout `"\n"`, exit 0 (parity row 4,
  `herdrcore/reads.py:375-378`).
- Vanished pane: hc `HcRpcError` with `kind == "not_found"` gives exit 66, stdout
  empty, stderr `peek: no screen for '<name>'` (`herdrcore/reads.py:369-371`;
  the stderr prefix from `rpp/perop.py:166-167`, `f"{op}: {result.message}"`).
  `OpResult.fail` always has stdout `b""` (`rpp/ops.py:78-79`). Same answer as
  "no record". Parity deviation row 2: intentional.
- Line count honoured: yes, as above (counts rows of the raw split; see the
  trailing-newline caveat). Test: `peek mc-a 5` shows `bye-now`
  (`tests/unit/test_herdrcore_lifecycle.py:143-144`); `peek <name> 120` shows
  started output (`tests/unit/test_herdrcore_carrier.py:15-34`).
- gc's reader: `Provider.Peek` tries the exec carrier first, then falls back to
  `peekOp` on `ErrExecUnsupported` (`internal/runtime/exec/exec.go:750-758`; the
  carrier fallback fires on exit 2 with `proc.exec` undeclared,
  `internal/runtime/exec/exec.go:894-904`). `peekOp` goes through `run`, which
  returns `strings.TrimRight(stdout, "\n")` on exit 0
  (`internal/runtime/exec/exec.go:102-105`). So gc strips every trailing `\n`:
  the added newline, an empty screen's `"\n"`, and a final blank row are all
  invisible to gc.

### Failure cases

Generic runner failures are in the shared contract.

| Case | Exit | stdout | Source |
|---|---|---|---|
| missing or empty `<name>` | 1 | empty | `herdrcore/reads.py:57-60`; `OpError` to `rpp/perop.py:166-167` |
| `lines` not an int (including `""`, `"abc"`, `"1.5"`) | 1 | empty; stderr `peek: peek: lines must be an integer, got '…'` (the message carries its own `peek:` plus the op prefix; the double prefix is inferred from `herdrcore/reads.py:363` and `rpp/perop.py:167`, unverified by test) | `herdrcore/reads.py:360-363` |
| no record for the name | 66 | empty; `no screen for '<name>'` | `herdrcore/reads.py:364-366` (`ExitCode.NO_SCREEN`, `rpp/exitcodes.py:21-22`); tests `tests/unit/test_herdrcore_lifecycle.py:148-149`, `tests/unit/test_herdrcore_server.py:448` (stopped city); `tests/unit/test_rpp_perop.py:185-192` (66 mapping) |
| core unreachable, city parked AND nothing of the core runs | 66 | empty | `herdrcore/reads.py:262-279` (`current_record` with `liveness=False` requires `parked(ctx) and nothing_runs(ctx)` to return None) |
| core unreachable otherwise (unparked empty/down core, or parked core with core remnants) | 1 | empty; `cannot read the city's core: …`; also kicks a start (`herdrcore/reads.py:91-95`, `herdrcore/reads.py:277-279`) | `herdrcore/reads.py:272-279` |
| hc `not_found` on read | 66 | empty | `herdrcore/reads.py:369-371` |
| other `HcRpcError` / `HcError` (including `HcTimeout`, connection lost, protocol) | 1 | empty; `cannot read the screen: …` | `herdrcore/reads.py:372-374`; `hc/errors.py:56-98` |
| deadline already spent before the handler | 1 | empty; `no time left before the op's deadline` | `rpp/perop.py:144-145` |
| deadline hit during an hc call | 1 via `HcTimeout` (query budget 3 s, or 1.5 s for probe ops; `peek` is not a probe op, `hc/client.py:128-148`) | empty | `herdrcore/reads.py:372-374` |
| SIGTERM or cancel | 1 `canceled` | empty | `rpp/perop.py:50-58,149-150` |
| answer over 1 MiB | 1 | **empty** (replaced, nothing printed); `answer of N bytes is over the 1048576-byte cap` | `rpp/perop.py:158-159`; test `tests/unit/test_rpp_perop.py:233-240` (generic cap test using `observe`; not peek-specific) |
| hc reply over 16 MiB | 1 (`HcProtocolError`, via `cannot read the screen`) | empty | `hc/client.py:105,435-441` (the newline check at `:435-438` returns before the `> MAX_REPLY_BYTES` check at `:440-441`, so a final chunk containing `\n` slightly past 16 MiB can pass `hc` and still hit the runner's 1 MiB cap) |
| stdout write fails (gc stopped reading) | 1 | partial or none | `rpp/perop.py:161-165` |

Unexpected exceptions become exit 1 `internal error: …`
(`rpp/perop.py:153-154`).

### Conflicts

- **Exit 66 vs gc v1.5.1.** gc has no exit-66 handling: `runError` maps only exit
  2 specially, and any other non-zero exit becomes an error wrapping stderr
  (`internal/runtime/exec/exec.go:153-170`).
  `docs/reference/exec-session-provider.md:71-77` lists only 0, 1 and 2. The
  pack's own docstring calls 66 "peek, from a v2-ready script"
  (`rpp/exitcodes.py:21-22`). In gc v1.5.1, a missing-session peek is therefore an
  ordinary `exec provider … peek …: no screen for '<name>'` error, not an empty
  answer. Parity row 1: intentional. Parity row 1 says gc checks `is-running`
  before peeking (`internal/runtime/seam_adapter.go:189-194`; `internal/worker/runtime_handle.go:186-190`); those
  two paths were not re-verified. In v1.5.1 `seamProvider.Peek` returns `""` when
  there is no attachment (`internal/runtime/seam_adapter.go:189-195`, seen but
  not traced further). Treat the claim as unverified.
- **Trailing-newline and row counting.** The pack counts a trailing empty row and
  always appends `\n`. gc trims trailing `\n` (`internal/runtime/exec/exec.go:104`),
  so gc cannot see this. Parity row 4: intentional.
- **Screen vs scrollback.** The pack reads the screen only (`hc/client.py:250-254`).
  gc's tmux doc says "all scrollback when lines <= 0"
  (`internal/runtime/exec/exec.go:750-752`, the carrier comment). Parity row 5:
  intentional. Whether the hc screen holds scrollback is unverified.
- **Output cap.** The pack fails above 1 MiB. gc has no cap, buffers whole, and
  trims only trailing newlines (`internal/runtime/exec/exec.go:94-104`). Parity
  row 6: intentional.
- **Empty `lines`.** The pack fails with exit 1. gc never sends an empty value
  (`internal/runtime/exec/exec.go:622`). Parity row 3: intentional.
- Parity verdict for the op: **intentional** (§peek Verdict, line 652). No
  pack-vs-parity-doc conflict found beyond what is noted here. The parity doc
  cites `herdrcore/reads.py:364-371`, `herdrcore/reads.py:375-378`, and `rpp/perop.py:158-159` –
  all match the pinned tree.

### Unverified

- Whether hc `terminal.read` text ends in `\n` (this decides the extra blank row
  at lines=N). No test pins it.
- Whether the hc screen includes scrollback or just the viewport (the `how`,
  `complete` and `why` fields of `ReadResult` are parsed, `hc/parse.py:154-161`,
  but `peek` ignores them: incomplete reads still exit 0).
- The exact stderr text for a bad `lines` value (double `peek:` prefix) – no
  test.
- Whether any caller sends extra argv beyond `lines` (ignored by
  `herdrcore/reads.py:57-60,359`; no dedicated test).
- No peek-specific test for the over-1 MiB path or for the hc `not_found`-after-
  record-found path (only the no-record path is tested).
- gc call sites that gate `peek` on `is-running` (`internal/runtime/seam_adapter.go`,
  `internal/worker/runtime_handle.go`) – not re-traced.

## get-meta

### Registration

- `"get-meta": spec(reads.op_get_meta)` – `herdrcore/ops_table.py:80`. Handler
  `op_get_meta` – `herdrcore/reads.py:404-409`.
- `spec()` defaults: `stdin=False` (`reads_stdin=False`), `deadline="default"`,
  `cap=SMALL_STDOUT_CAP`, `housekeeping=True` – `herdrcore/ops_table.py:64`. So
  stdin is never read (`rpp/perop.py:135`: `_read_stdin` only `if
  spec.reads_stdin`, else `b""`).
- Deadline class `default` = 30 s unless gc sends `GC_EXEC_DEADLINE_MS` –
  `rpp/ops.py:23-26`, `rpp/ops.py:127`, `rpp/perop.py:31`. gc side: 30 s provider
  timeout – `internal/runtime/exec/exec.go:56`, used by `run` at
  `internal/runtime/exec/exec.go:62-64`.
- Stdout cap `SMALL_STDOUT_CAP` = 64 KiB – `rpp/ops.py:32`. Over cap gives exit 1,
  empty stdout, stderr message – `rpp/perop.py:158-159`.
- Housekeeping: `lifecycle.sweep_spawn_files(ctx)` runs before the handler –
  `herdrcore/ops_table.py:41-50`.
- The op is in gc's core op table
  (`docs/reference/exec-session-provider.md:135`), not an extension. gc interface
  `GetMeta(name, key string) (string, error)` – `internal/runtime/runtime.go:206`;
  exec impl `internal/runtime/exec/exec.go:738-742`.
- Parity §get-meta (`docs/provider-herdr-parity.md:902`, verdict **intentional**,
  `docs/provider-herdr-parity.md:918`).

### Inputs

- argv: `<exe> get-meta <name> <key>`
  (`docs/reference/exec-session-provider.md:135`;
  `internal/runtime/exec/exec.go:741`). The pack reads `args[0]`=name,
  `args[1]`=key; extra args are ignored (`herdrcore/reads.py:57-61,384-388`).
- Preconditions: the name is non-empty (`herdrcore/reads.py:57-60`, "missing
  session name"); the key is present and non-empty (`herdrcore/reads.py:386-387`,
  "missing metadata key"). No other validation of name or key (no charset check;
  the key is a plain dict lookup, `herdrcore/reads.py:409`).
- The session need not exist (see "Success output"). No lock is taken (the read
  path has no `ctx.name_lock`; writers use temp + rename under a lock –
  `herdrcore/metastore.py:16-17` docstring, `herdrcore/reads.py:413`).
- Lookup path: `current_record` (newest record, live or finished;
  `herdrcore/reads.py:264-278`), then `object_of` label `gc.obj`
  (`herdrcore/records.py:74-76`), then `MetaStore.read(obj)`
  (`herdrcore/metastore.py:105-125`) from `<state>/meta/<obj>.json`
  (`herdrcore/reads.py:76-77`, `herdrcore/metastore.py:100-103`).

### Success output

- stdout = the raw value, UTF-8 (surrogateescape), **no trailing newline added,
  no JSON, no framing**; one key only, so ordering is moot –
  `herdrcore/reads.py:409`.
- A missing key gives empty stdout, exit 0 – `herdrcore/reads.py:409`
  (`values.get(key, "")`).
- Empty meta, no meta file, unknown session or no record gives empty stdout,
  exit 0 – `herdrcore/reads.py:405-408`; `herdrcore/metastore.py:105-111` (None
  when there is no file). Test: `tests/unit/test_herdrcore_lifecycle.py:209-210`
  (unknown session, exit 0, `b""`), `tests/unit/test_herdrcore_lifecycle.py:218` (set to empty gives `b""`); after
  `stop` the value is gone: `tests/unit/test_herdrcore_carrier.py:50-52`.
- Value round trip: `tests/unit/test_herdrcore_carrier.py:37-42`
  (byte-for-byte including trailing spaces, e.g. `"λ>  "`).
- Exit code 0 – `OpResult.ok` (`rpp/ops.py:69-70`), returned via
  `rpp/perop.py:170`.
- gc parser: `internal/runtime/exec/exec.go:102-105` – on exit 0 returns
  `strings.TrimRight(stdout, "\n")` (all trailing `\n`, not spaces). Empty gives
  `("", nil)` (`internal/runtime/exec/exec.go:738-740` comment). Docs: "value
  (empty = not set)" `docs/reference/exec-session-provider.md:135`, `docs/reference/exec-session-provider.md:366`.
- Consequence: a stored value ending in `\n` loses those newlines at gc; the pack
  emits them. Spaces survive.
- Exit 2 from the op is read by gc as success with `""` –
  `internal/runtime/exec/exec.go:159-160`, `internal/runtime/exec/exec.go:174-181` (only `start` and
  `provision` are exempt). The pack never returns 2 from this op (exit 2 is only
  for an unknown op, `rpp/perop.py:113-120`).

### Failure cases

All failures: stdout empty; stderr `get-meta: <message>`
(`rpp/perop.py:147-148,167-168`). Generic runner failures are in the shared
contract.

| Case | Exit | stdout | Source |
|---|---|---|---|
| missing or empty name | 1 | empty | `herdrcore/reads.py:57-60` |
| missing or empty key | 1 | empty | `herdrcore/reads.py:384-387` |
| core unreadable (unreachable; kicks a start unless parked AND nothing runs) | 1 | empty | `herdrcore/reads.py:264-278,91-95` (`get-meta` passes `liveness=False`, so only `parked(ctx) and nothing_runs(ctx)` returns None and exits 0 with empty stdout, `herdrcore/reads.py:272-276`; an unparked empty/down core or a parked core with remnants exits 1 `cannot read the city's core`) |
| record not started by the pack (no `gc.obj` label) | 1 | empty | `herdrcore/reads.py:395-397` |
| `gc.obj` not a nonce | 1 | empty | `herdrcore/metastore.py:100-102` via `herdrcore/reads.py:398-401` |
| meta file unreadable (symlink, wrong owner, I/O) or corrupt JSON | 1 | empty | `herdrcore/metastore.py:107-110,123-125`; `herdrcore/config.py:197-212`; `herdrcore/reads.py:398-401`; `_wrap` maps `StateDirError` and `MetaError` to `OpError`, `herdrcore/ops_table.py:46-51` |
| state dir or config error | 1 | empty | `herdrcore/ops_table.py:46-49` |
| deadline already expired before the handler | 1 | empty | `rpp/perop.py:144-145` |
| deadline or cancel during the core read | 1 (`HcTimeout` and `HcCancelled` are `HcError`, so `herdrcore/reads.py:277-278`; a plain `Cancelled` goes to `rpp/perop.py:149-150`) | empty | `hc/errors.py:56,73,107`; `herdrcore/reads.py:277-278` |
| SIGTERM or interrupt | 1 | empty | `rpp/perop.py:149-152` |
| unexpected exception | 1 | empty | `rpp/perop.py:153-154` |
| value over 64 KiB | 1 | empty | `rpp/perop.py:158-159` |
| stdout closed early | 1 | partial | `rpp/perop.py:162-165` |
| lock timeout | n/a – `get-meta` takes no lock | – | – |

gc maps any non-zero exit other than 2 to `exec provider ... get-meta ...:
<stderr>` as an error (`internal/runtime/exec/exec.go:153-169`). On gc's own
timeout it kills the process; the error comes from
`internal/runtime/exec/exec.go:102-117`.

There are 12 distinct failure rows (11 reachable for `get-meta`; lock timeout is
n/a).

### Conflicts

- Parity doc verdict: **intentional** (§get-meta,
  `docs/provider-herdr-parity.md:918`). Its cited lines match the pin:
  `herdrcore/ops_table.py:80`,
  `herdrcore/reads.py:277-278,384-388,396-397,400-401,404-409`, test
  `tests/unit/test_herdrcore_lifecycle.py:208-209` (actual lines 209-210, off by
  one – cosmetic).
- The parity doc says "Both emit the raw value with no trailing newline …
  matching the contract". The pack side is confirmed (`herdrcore/reads.py:409`).
  Not stated in the parity doc: gc strips trailing `\n` from the result
  (`internal/runtime/exec/exec.go:104`), so values ending in a newline are lossy
  end to end. Not a pack-vs-gc conflict (gc's doc does not promise newline
  preservation), but worth a note.
- Op class: `get-meta` is a core RPP v0 op
  (`docs/reference/exec-session-provider.md:135`), not an extension; there is no
  capability gate in `internal/runtime/protocol.go` for it.
- No pack-vs-gc conflict found. "Empty = not set" agrees
  (`docs/reference/exec-session-provider.md:135,366`;
  `herdrcore/reads.py:409`).
- Pack-specific behaviour beyond gc: metadata dies with `stop` (per-incarnation
  file; `herdrcore/metastore.py:1-19`, test
  `tests/unit/test_herdrcore_carrier.py:44-52`); it may be seeded by `start` with
  identity keys (`herdrcore/metastore.py:84-89`; test
  `tests/unit/test_herdrcore_lifecycle.py:214`).

### Unverified

- Whether gc ever calls `GetMeta` with an empty key (the parity doc says "gc never
  sends one"; gc call sites were not audited) – unverified.
- Exact wall-clock behaviour when the deadline expires inside the core client
  (which Hc error class fires, and whether the 1 s margin, `rpp/cancel.py:27,98`,
  yields exit 1 or a late success) – partly read, not exercised.
- Whether `list_terminals` could return a record whose `gc.obj` is not a nonce in
  practice – unverified (only the guard `herdrcore/metastore.py:101-102` is
  verified).
- Non-UTF-8 stored values: by source inspection, `set-meta` decodes raw stdin
  with `utf-8`/`surrogateescape` without trimming (`herdrcore/reads.py:427`),
  `MetaStore` stores those byte-escape surrogates (`U+DC80–U+DCFF`) as ASCII
  JSON `\udcXX` escapes (`herdrcore/metastore.py:75-85`, `rpp/jsonval.py:29-35`),
  and `get-meta` re-encodes them with `surrogateescape`
  (`herdrcore/reads.py:409`). Thus invalid UTF-8 bytes round-trip for readable
  sidecars on direct pack calls (subject to gc stripping trailing `\n`,
  `internal/runtime/exec/exec.go:104`, and the 1 MiB sidecar read truncation,
  `herdrcore/config.py:197`); no dedicated unit test was found.

## set-meta

### Registration

- `"set-meta": spec(reads.op_set_meta, stdin=True)` –
  `herdrcore/ops_table.py:81`.
- `reads_stdin=True` (`herdrcore/ops_table.py:65`: `OpSpec` built from `stdin`).
  Deadline class `default` (spec default `herdrcore/ops_table.py:64`) = 30 s when
  gc sends no `GC_EXEC_DEADLINE_MS` (`rpp/ops.py:23-27`, `rpp/perop.py:129`).
  Stdout cap `SMALL_STDOUT_CAP` = 64 KiB (`herdrcore/ops_table.py:64`,
  `rpp/ops.py:32`); the op writes no stdout, so the cap cannot trip.
- Housekeeping stays on (`herdrcore/ops_table.py:64`, `herdrcore/ops_table.py:44-46`):
  `sweep_spawn_files` runs before the handler.
- gc side: `Provider.SetMeta` runs `script set-meta <name> <key>` with the value
  as stdin (`internal/runtime/exec/exec.go:731-736`); default timeout 30 s
  (`internal/runtime/exec/exec.go:56`). Doc table:
  `docs/reference/exec-session-provider.md:134`.
- Parity §set-meta (`docs/provider-herdr-parity.md:870-890`): status
  implemented; verdict **intentional**.

### Inputs

- argv: `set-meta <name> <key>` (`internal/runtime/exec/exec.go:734`;
  `docs/reference/exec-session-provider.md:134`). Pack: `_meta_args` reads
  `args[0]` as the name (`herdrcore/reads.py:57-60`, `herdrcore/reads.py:384-388`) and `args[1]` as
  the key. Extra args are ignored (only indices 0 and 1 are read; no length check
  beyond `< 2`).
- stdin = the raw value bytes. Not JSON, not one value per key: exactly one key
  per call. `op_set_meta` decodes with `utf-8`/`surrogateescape`, no trim, no
  newline strip (`herdrcore/reads.py:425-427`). gc sends `[]byte(value)`
  (`internal/runtime/exec/exec.go:734`; a non-nil empty slice is piped as empty
  stdin, `internal/runtime/exec/exec.go:98-100`). stdin is read to EOF before the
  handler (`rpp/perop.py:60-80,135`).
- Empty value: stored as `""`, overwriting any prior value (not a delete). Pack
  test `tests/unit/test_herdrcore_lifecycle.py:217-218` (`set-meta K stdin=b""`
  gives exit 0, `get-meta` gives empty). gc requires this:
  `internal/runtime/runtimetest/conformance.go:498-516`
  (`SetMeta_EmptyValueOverwrites`). Parity §set-meta does not list empty values;
  verified here from source and tests.
- Preconditions: the name must have a record in the city's core
  (`herdrcore/reads.py:263-273`, `herdrcore/reads.py:391-394`, `herdrcore/reads.py:415-418`). The record must carry a
  `gc.obj` label (`herdrcore/reads.py:395-397`, `herdrcore/records.py:74-76`).
  The key must be non-empty (`herdrcore/reads.py:386`). Identity keys
  (`GC_INSTANCE_TOKEN` etc.) are plain keys, writable and seen by `observe` (test
  `tests/unit/test_herdrcore_lifecycle.py:215-216`; `herdrcore/metastore.py:6-8`).
- A missing meta file is not an error: a fresh `Meta(session=name)` is created
  (`herdrcore/reads.py:420`).
- Effect: under the exclusive per-name lock (`herdrcore/reads.py:413`;
  `herdrcore/context.py:152-159`, wait up to 30 s, cut to the deadline), read the
  file, merge the key (`herdrcore/metastore.py:64-70`), rewrite the whole file as
  `<state>/meta/<obj>.json` via a 0600 temp file plus rename, no fsync
  (`herdrcore/metastore.py:127-128`, `herdrcore/config.py:174-194`). The value is
  stored JSON-escaped, ASCII (`herdrcore/metastore.py:75-85`;
  `rpp/jsonval.py:29-35`). Sidecar content is not on the wire.

### Success output

- Exit 0, stdout empty, stderr empty: `_mutate_meta` returns `OpResult.ok()`
  (`herdrcore/reads.py:422`; `rpp/ops.py:68-70`). No echo. Pack tests assert only
  `code == 0` (`tests/unit/test_herdrcore_lifecycle.py:203,215,217`;
  `tests/unit/test_herdrcore_carrier.py:39-41`).
- Round trip: bytes in = bytes out via `get-meta` (`herdrcore/reads.py:409`),
  including trailing spaces: test `tests/unit/test_herdrcore_carrier.py:36-43`
  (`"› "`, `"λ>  "`). Caveat: gc TrimRights `\n` from `get-meta` stdout
  (`internal/runtime/exec/exec.go:104`), so a value ending in `\n` does not round
  trip through gc (not a `set-meta` defect; it is `get-meta`-side).
- gc v1.5.1 does call the op: `SetMeta` (`internal/runtime/runtime.go:200-202`,
  `internal/runtime/exec/exec.go:731-736`); callers include
  `cmd/gc/session_wake.go:317-328` (`GC_DRAIN_ACK` etc.),
  `cmd/gc/cmd_runtime_drain.go:64,122-123`, `internal/session/lifecycle.go:122`.
  gc ignores stdout on success (`internal/runtime/exec/exec.go:731-736`: `_, err
  :=`).

### Failure cases

All failures: stdout empty; stderr = `set-meta: <message>`
(`rpp/perop.py:166-167`). Generic runner failures are in the shared contract.

| Case | Exit | stdout | Source |
|---|---|---|---|
| missing or empty session name | 1 `missing session name` | empty | `herdrcore/reads.py:57-60` |
| missing or empty key | 1 `missing metadata key` | empty | `herdrcore/reads.py:386-387` |
| unknown session (no record) | 1 `no session '<name>'` | empty | `herdrcore/reads.py:415-418` (`remove-meta` succeeds here, `herdrcore/reads.py:416-417`); test `tests/unit/test_herdrcore_lifecycle.py:212` |
| core unreachable, city parked AND nothing runs (or reachable with no matching record) | 1 `no session '<name>'` (record `None`) | empty | `herdrcore/reads.py:272-276`, `herdrcore/reads.py:415-418` (`set-meta` uses `liveness=False`; an unparked unreachable core or parked core with remnants fails at `current_record` below) |
| core unreadable otherwise | 1 `cannot read the city's core: ...` | empty | `herdrcore/reads.py:277-279` |
| record not started by the pack (no `gc.obj`) | 1 `'<name>' runs in a terminal the pack did not start` | empty | `herdrcore/reads.py:395-397` |
| meta file corrupt, symlink, wrong owner or bad nonce | 1 (message of `MetaError`) | empty | `herdrcore/metastore.py:101-125`, `herdrcore/config.py:197-211`, `herdrcore/reads.py:398-401`, `herdrcore/ops_table.py:48-49` |
| state-dir, config or lock-file error | 1 | empty | `herdrcore/ops_table.py:48-49` |
| name lock wait expires | 1 `busy: ...` | empty | `herdrcore/ops_table.py:50-51`, `herdrcore/context.py:157` |
| write fails (OSError from temp or rename) | 1 `internal error: OSError: ...` | empty | `herdrcore/config.py:174-194` (not in the `herdrcore/ops_table.py:48` tuple), then `rpp/perop.py:153-154` |
| stdin still open at the deadline | 1 `stdin was still open at the op's deadline` | empty | `rpp/perop.py:73-75` |
| deadline already spent before the handler | 1 `no time left before the op's deadline` | empty | `rpp/perop.py:144-145` |
| SIGTERM or cancel | 1 | empty | `rpp/perop.py:72,143,149-152` (direct `Cancelled` gives `canceled (terminated)`; `HcCancelled` in `current_record` is caught by `except HcError` at `herdrcore/reads.py:274-278` and becomes `cannot read the city's core: ...`); gc cancel docs `docs/reference/exec-session-provider.md` "Cancellation" |
| stdout over cap | n/a (stdout is always empty) | – | `rpp/perop.py:158-159` |
| malformed JSON, unknown key or too-large value | n/a: stdin is not parsed, there is no key allowlist, no set-side size cap | – | `herdrcore/reads.py:425-427`; parity deviation 5 |
| invalid UTF-8 value | accepted; raw bytes kept as `U+DC80–U+DCFF` surrogate escapes (ASCII `\udcXX` in JSON) | – | `herdrcore/reads.py:427`, `rpp/jsonval.py:29-35` (code path round-trips via `get-meta` at `herdrcore/reads.py:409` for readable sidecars; no dedicated test) |

- Exit 2 never occurs (`rpp/perop.py:117-120` only for unregistered ops). gc
  would treat exit 2 as success for `set-meta`
  (`internal/runtime/exec/exec.go:159,174-180`); any other non-zero is an error
  with stderr embedded (`internal/runtime/exec/exec.go:162-169`).

### Conflicts

- Contract match: stdin value, empty stdout, 0/1 exits
  (`docs/reference/exec-session-provider.md:134,363-364`;
  `internal/runtime/exec/exec.go:731-736`). No conflict found.
- Behaviour gc leaves unspecified, pack-specific: an unknown session gives exit 1
  (parity deviation 1, intentional; gc docs do not state the case, and the
  `internal/runtime/runtimetest/conformance.go` meta tests use a started session,
  `internal/runtime/runtimetest/conformance.go:457-540`). Meta lives only for the
  incarnation (`herdrcore/lifecycle.py:389-396` `forget`; parity deviation 2; test
  `tests/unit/test_herdrcore_carrier.py:46-52`). A per-name exclusive flock, 30 s
  (parity deviation 3). A 1 MiB read cap on the sidecar file
  (`herdrcore/config.py:197`; parity deviation 5, not-implemented).
- Parity §set-meta: status implemented; verdict **intentional**. It describes the
  Rex provider vs herdr-core; the herdr-core claims were rechecked and match the
  source. Small line drift: the parity doc cites `herdrcore/reads.py:416-420` for
  the unknown-session check, the pinned source is `herdrcore/reads.py:415-418`.
- Parity says `set-meta` "runs under default 30 s deadline": matches
  `herdrcore/ops_table.py:64`, `rpp/ops.py:23-27`.

### Unverified

- Source-derived invalid-UTF-8 round-trip has no dedicated unit test: raw
  bytes decoded via `surrogateescape` at `herdrcore/reads.py:427` serialize as
  ASCII JSON `\udcXX` escapes (`herdrcore/metastore.py:75-85`,
  `rpp/jsonval.py:29-35`) and re-encode via `surrogateescape` in `get-meta`
  (`herdrcore/reads.py:409`) for readable sidecars on direct pack calls (note gc
  strips trailing `\n` at `internal/runtime/exec/exec.go:104`, and arbitrary
  lone surrogates outside `U+DC80–U+DCFF` are a separate case).
- Behaviour of a set that pushes the sidecar over 1 MiB (writable, and
  `read_private_file` at `herdrcore/config.py:197,212` truncates at 1 MiB via
  `handle.read(limit)` rather than refusing; if the truncated prefix is invalid
  JSON, `MetaStore.read` raises `MetaError` at `herdrcore/metastore.py:112-125`,
  so `get-meta`/`set-meta`/`remove-meta` fail exit 1 via `_meta_target` at
  `herdrcore/reads.py:398-401`, whereas `observe` and `inventory` use
  `read_meta` at `herdrcore/reads.py:80-88,148-156`, which catches `MetaError`
  and emits `identity: null` with exit 0). Whether an arbitrary >1 MiB payload
  truncates to invalid vs valid JSON is unverified; parity also marks this
  unverified.
- Whether a set racing `stop`'s `forget` can leave a file (parity deviation 3 says
  yes; not re-derived).
- gc's concurrency expectations for same-key `set-meta` (not stated in either
  source).

## remove-meta

### Registration

- `"remove-meta": spec(reads.op_remove_meta)` – `herdrcore/ops_table.py:82`.
- `spec()` defaults: `stdin=False`, `deadline="default"`, `cap=SMALL_STDOUT_CAP`,
  `housekeeping=True` – `herdrcore/ops_table.py:64`. So `reads_stdin=False`,
  deadline class `default`, stdout cap 64 KiB (`rpp/ops.py:32`). stdin is never
  read: `rpp/perop.py:135` reads it only when `spec.reads_stdin`.
- The `default` deadline is 30 s unless `GC_EXEC_DEADLINE_MS` is set
  (`rpp/ops.py:23-25`, `rpp/perop.py:31,129`). gc's own timeout is 30 s
  (`internal/runtime/exec/exec.go:53-58`). gc v1.5.1 never sets
  `GC_EXEC_DEADLINE_MS` (no match in a grep of `*.go` and `*.md`), so the 30 s
  default applies (inference from the grep).
- Handler: `op_remove_meta` – `herdrcore/reads.py:430-432`. Wrapped by `_wrap`
  (`herdrcore/ops_table.py:41-53`), which first runs `sweep_spawn_files`
  (`herdrcore/ops_table.py:45-46`; errors swallowed,
  `herdrcore/lifecycle.py:741-746`).
- The 64 KiB cap is unreachable: the handler only returns `OpResult.ok()` with
  empty stdout (`herdrcore/reads.py:417,422`).
- Parity §remove-meta (`docs/provider-herdr-parity.md:932-956`; table row 76;
  rows `docs/provider-herdr-parity.md:199-203`) verdict: **intentional**; summary
  row 76 `implemented / 5 deviations / intentional`.

### Inputs

- argv: `remove-meta <name> <key>`. gc: `p.run(nil, "remove-meta", name, key)` –
  `internal/runtime/exec/exec.go:744-747`; doc table
  `docs/reference/exec-session-provider.md:136`. No stdin (gc passes nil,
  `internal/runtime/exec/exec.go:98`).
- `name`: `args[0]`, must be non-empty, else `missing session name` –
  `herdrcore/reads.py:57-60`. `key`: `args[1]`, must exist and be non-empty, else
  `missing metadata key` – `herdrcore/reads.py:386-387`. Extra args are ignored
  (`herdrcore/reads.py:384-388` reads only [0] and [1]).
- No other validation of name or key (no charset or length check) –
  `herdrcore/reads.py:384-388`. Any non-empty key is accepted; no key is reserved
  (`herdrcore/reads.py:430-432`).
- Preconditions: none on session state. The op resolves the name's newest record,
  live or finished (`herdrcore/reads.py:263-278`, `list_city(... finished=True)`
  at `herdrcore/reads.py:73`). The core need not answer if nothing runs (see
  "Failure cases").

### Success output

- Exit 0, empty stdout, empty stderr: `OpResult.ok()`
  (`herdrcore/reads.py:417,422`; `rpp/ops.py:68-70`; `rpp/perop.py:160-169` writes
  stdout only if non-empty and a message only if set).
- Cases that return 0:
  - Key present on a pack-started session: the key is popped and the whole meta
    file rewritten (`herdrcore/reads.py:420-421`,
    `herdrcore/metastore.py:64-70,127-128`; temp file + rename, mode 0600,
    `herdrcore/config.py:174-194`).
  - Key absent (idempotent): `Meta.with_value(key, None)` does
    `values.pop(key, None)` (`herdrcore/metastore.py:64-70`) and the file is still
    rewritten (`herdrcore/reads.py:421`). Exit 0, empty stdout. If the incarnation
    has no meta file, `Meta(session=name)` is written (`herdrcore/reads.py:420`),
    so `remove-meta` can create a file. Parity row 4 says the same (verified).
  - Unknown session (no record): returns OK without writing –
    `herdrcore/reads.py:415-417`. Test:
    `tests/unit/test_herdrcore_lifecycle.py:211` (`remove-meta nobody K` gives
    code 0).
  - Core unreachable, city parked AND nothing of the core runs: `current_record`
    (`liveness=False`) returns None, then the same OK (`herdrcore/reads.py:275-276`;
    `herdrcore/core.py:307-314`; `herdrcore/corescan.py:123-140`). The check
    applies to the **whole core**, not just this session name, and requires both
    `parked(ctx)` and `nothing_runs(ctx)`: parking alone or only this name not
    running is insufficient, and an unparked down core or parked core with
    remnants fails exit 1 at `current_record`.
  - Present key on a started session: test `test_metadata_ops` removes
    `GC_INSTANCE_TOKEN` (code 0) and `observe` identity no longer has it –
    `tests/unit/test_herdrcore_lifecycle.py:219-220`.
- Does gc call it? Yes. `Provider.RemoveMeta`
  (`internal/runtime/exec/exec.go:744-747`) is reached through
  `execRuntime.RemoveMeta` (`internal/runtime/exec/seams.go:83-93`). Callers:
  `cmd/gc/cmd_runtime_drain.go:69-77,120-121,201,221` (drain-ack, `GC_DRAIN`,
  `GC_RESTART_REQUESTED`, `GC_DRIFT_RESTART`) and
  `cmd/gc/session_wake.go:340-350` (drain-ack keys). gc ignores stdout: `run`
  returns trimmed stdout, `RemoveMeta` drops it
  (`internal/runtime/exec/exec.go:104,745-746`). Conformance: after `RemoveMeta`,
  `GetMeta` must be empty
  (`internal/runtime/runtimetest/conformance.go:479-494`). gc does not itself
  reject an empty key at the exec layer
  (`internal/runtime/exec/exec.go:744-747`); callers pass literals or constants
  (inferred from the call sites above).
- gc sees exit 0 as success (`internal/runtime/exec/exec.go:103-105`;
  `docs/reference/exec-session-provider.md:71-76`).

### Failure cases

All exit 1, stdout empty, stderr `remove-meta: <message>` after redaction –
`rpp/perop.py:147-148,166-167`. Generic runner failures are in the shared
contract.

| Case | Exit | stdout | Source |
|---|---|---|---|
| empty or missing `<name>` | 1 | empty | `herdrcore/reads.py:58-59` ("missing session name") |
| empty or missing `<key>` | 1 | empty | `herdrcore/reads.py:386-387` ("missing metadata key"); parity row 1 (intentional) |
| newest record has no `gc.obj` label (terminal the pack did not start) | 1 | empty | `herdrcore/reads.py:395-397` |
| meta file unreadable, corrupt, symlink or foreign owner, or bad nonce | 1 | empty | `MetaError` `herdrcore/reads.py:398-401`, `herdrcore/metastore.py:100-103,105-125`; `StateDirError`, `MetaError`, `ConfigError` and `LockFileError` re-raised as `OpError` `herdrcore/ops_table.py:48-49` |
| core cannot be read (unreachable or failing, not the "nothing runs" case) | 1 | empty | `herdrcore/reads.py:277-278` ("cannot read the city's core: ..."); unreachable also kicks a start (`herdrcore/reads.py:91-95`) |
| name lock not obtained | 1 | empty | `LockTimeout` becomes "busy: ..." `herdrcore/ops_table.py:50-51`; wait = min(30 s, max(0.1 s, remaining deadline)) `herdrcore/context.py:152-159`, `MUTATION_LOCK_WAIT_S=30` `herdrcore/context.py:48` |
| deadline already expired before the handler, or SIGTERM or cancel | 1 | empty | `rpp/perop.py:143-145` ("no time left before the op's deadline"); direct `Cancelled` becomes `canceled (terminated)` (`rpp/perop.py:149-150`); `HcCancelled` in `current_record` becomes `cannot read the city's core: ...` (`herdrcore/reads.py:274-278`) |
| deadline passes mid-op | 1 or killed | empty | gc kills the op group after 30 s (`internal/runtime/exec/exec.go:79-92`); pack-side enforcement beyond lock wait and cancel: unverified |
| write failure (ENOSPC, EACCES on rename or temp) | 1 | empty | not caught by `_wrap`; falls to `rpp/perop.py:153-156` ("internal error: <Type>: <msg>") |
| stdout over cap | – | – | cannot happen; stdout is always empty (`herdrcore/reads.py:417,422`), cap check `rpp/perop.py:158-159` |

- Exit 2 from this op would be read by gc as success
  (`internal/runtime/exec/exec.go:159-161,172-181`;
  `docs/reference/exec-session-provider.md:71-76`), but the pack never returns 2
  for a known op (`rpp/perop.py:117-121` only for unknown ops).
- gc turns exit 1 into `exec provider <script> remove-meta <name> <key>: <stderr>`
  (`internal/runtime/exec/exec.go:162-169`). Some gc callers ignore the error
  (for example the herdr and tmux providers `_ =`); the drain and wake callers log
  or join it (`cmd/gc/session_wake.go:350-353`).

### Conflicts

- Pack vs gc v1.5.1: no contract conflict. gc requires only "exit 0, no stdout,
  key then reads as empty"
  (`docs/reference/exec-session-provider.md:136`,
  `internal/runtime/runtimetest/conformance.go:479-494`); the pack satisfies it,
  including the idempotent absent key and the unknown session (test
  `tests/unit/test_herdrcore_lifecycle.py:211`).
- Parity doc §remove-meta verdict: **intentional**; summary row 76 `implemented /
  5 deviations / intentional`. Re-checked against the pinned pack, the herdr-core
  claims agree: `herdrcore/reads.py:430-432`, `herdrcore/reads.py:384-388`, `herdrcore/reads.py:413`, `herdrcore/reads.py:415-417`,
  `herdrcore/reads.py:396-397`, `herdrcore/reads.py:419-421` all match. Two citation nits, no verdict change: (a) the
  parity doc cites `herdrcore/metastore.py:16,64-70` for temp + rename; line 16
  is the module docstring, and the real writer is `herdrcore/metastore.py:127-128`
  plus `herdrcore/config.py:174-194`; (b) its text "(a file is created if none
  existed)" is correct (`herdrcore/reads.py:420`).
- Not covered by the parity doc (could add rows): the non-`gc.obj` terminal error
  appears (row 3), but core-unreachable exit 1, lock-timeout "busy", and
  corrupt-meta exit 1 are not listed. Also, `remove-meta` on a finished but not
  stopped incarnation still rewrites its file (the record lookup includes
  finished, `herdrcore/reads.py:73`), and after `stop` deletes the file
  (`herdrcore/metastore.py:17-18`) a leftover record would get a fresh file – the
  second point is inference, not tested.
- The Rex provider side (`main.go`, `stores.go`) was not checked; those claims in
  the parity doc are out of scope here.

### Unverified

- Pack-side deadline enforcement during the handler; only gc's kill is verified.
- Whether a record left after `stop` can exist with the meta file deleted, and so
  whether `remove-meta` can then recreate a file.
- No pack test asserts the empty-key, foreign-terminal or corrupt-meta failures
  for `remove-meta` (`rg` of the tests found only
  `tests/unit/test_herdrcore_lifecycle.py:211,219`).
- Real exit and stderr text under a real `gc` run (all conclusions are from
  source reading, no execution).

## nudge

### Registration

- `nudge` is `spec(inputs.op_nudge, stdin=True)` in the op table:
  `herdrcore/ops_table.py:83`. `spec()` defaults pass `deadline="default"` and
  `cap=SMALL_STDOUT_CAP` (`herdrcore/ops_table.py:64-65`).
- So the op reads stdin, has the default deadline class, and a 64 KiB stdout cap
  (`rpp/ops.py:32`: `SMALL_STDOUT_CAP = 64 << 10`).
- The `nudge` path is unchanged at the pack's working tree `8ed12884` (see
  "Sources and pins").
- Parity §nudge (`docs/provider-herdr-parity.md`); row 3 classifies the exit-80
  `gone` answer as `intentional`.

### Inputs

- **argv.** `<script> nudge <name>`. The per-op runner passes `argv[2:]` as
  `args` (`rpp/perop.py:132-135`). The handler takes the session name from
  `args[0]`, the empty string if absent (`herdrcore/inputs.py:338-339`, `_name`).
  gc invokes `script nudge <name>` and nothing else
  (`docs/reference/exec-session-provider.md:133`); it passes no extra argv
  (`internal/runtime/exec/exec.go:617`, `nudgeOp`).
- **stdin.** The full stdin stream, read by the runner because
  `reads_stdin=True` (`rpp/perop.py:135`). The handler decodes it as UTF-8
  (`herdrcore/inputs.py:344`). gc sends the flattened message text on stdin
  (`internal/runtime/exec/exec.go:613,617`; contract table
  `docs/reference/exec-session-provider.md:133` and "stdin for values"
  `docs/reference/exec-session-provider.md:363`). gc returns before calling the
  op when the message is empty (`internal/runtime/exec/exec.go:614-616`); any
  other caller with empty stdin reaches `nudge()` as an empty text.
- **env.** Optional `GC_EXEC_DEADLINE_MS` (integer milliseconds) sets the op
  deadline; a missing or malformed value falls back to the class default
  (`rpp/perop.py:31-47,129`; `rpp/ops.py:119-128`). The pack notes no released gc
  sends it (`rpp/perop.py:32-33`).
- **Preconditions.** `name` must be non-empty, else `gone` ("no session name")
  before any hc call (`herdrcore/inputs.py:213-214`). A non-empty `name` must
  match a city terminal whose newest record is intact; otherwise `gone`
  (`herdrcore/inputs.py:215-229`, `reads.list_city` at `herdrcore/reads.py:68`,
  `records.newest` at `herdrcore/records.py:48`, `records.is_intact` at
  `herdrcore/records.py:53`). The text may be any bytes decodable as UTF-8 (see
  the `rejected` failure case).

### Success output

Both success forms exit 0 and print nothing on stderr
(`rpp/outcome.py:103-113`):

- **`delivered`.** Exit 0, empty stdout (`rpp/outcome.py:110-111`; produced when
  `SendResult.started is not False`, `herdrcore/inputs.py:238-241,265-266`). A
  program that never reports status is plain `delivered` (`herdrcore/inputs.py`
  docstring `herdrcore/inputs.py:27-34`).
- **`delivered_unobserved`.** Exit 0, one stdout line
  `{"outcome":"delivered_unobserved","detail":"..."}` (the shared encoder at
  `rpp/outcome.py:96-100` omits `"detail"` if empty after cleaning, though
  `nudge` always supplies non-empty detail here, `rpp/outcome.py:112-113`).
  Produced when hc reports `started is False` (`herdrcore/inputs.py:239-241`)
  or when the write reached hc whole but its reply or answer did not arrive
  (`herdrcore/inputs.py:260-264`). gc reads only
  the exit code and discards `nudge` stdout
  (`internal/runtime/exec/exec.go:617`), so the line is informational for other
  callers.

Corroborating pack tests: `tests/unit/test_herdrcore_inputs.py:102-104`
(delivered), `tests/unit/test_herdrcore_inputs.py:124-128` (delivered_unobserved), `tests/unit/test_herdrcore_inputs.py:170-175` (lost reply after
write).

### Failure cases

Exit code meanings: see the shared contract (`rpp/exitcodes.py:11,14,17,30-33`).
Non-success words print one `{"outcome":"<word>","detail":"..."}` stdout line
(the shared encoder omits `"detail"` when empty after cleaning,
`rpp/outcome.py:96-100`, though `nudge`'s failure paths provide detail) and
`<word>[: detail]` on stderr (`rpp/outcome.py:114-115`).

| Case | Exit | stdout | Source |
|---|---|---|---|
| stdin is not valid UTF-8, `rejected`, nothing typed | 80 | one `rejected` line, detail "the nudge text is not UTF-8; nothing was typed" | `herdrcore/inputs.py:342-348` |
| empty name, `gone` ("no session name") | 80 | one `gone` line | `herdrcore/inputs.py:213-214,269-274` |
| no terminal record for the name, `gone` ("no box holds the name") | 80 | one `gone` line | `herdrcore/inputs.py:225-226` |
| newest record not intact, `gone` ("the box's program has ended") | 80 | one `gone` line | `herdrcore/inputs.py:227-228` |
| hc `not_found` or `finished` refusal, `gone` | 80 | one `gone` line | `herdrcore/inputs.py:61-67,197-206` |
| hc `blocked` refusal, `pending_interaction` | 80 | one `pending_interaction` line | `herdrcore/inputs.py:64-66,197-206` |
| hc `status_mismatch` refusal, `busy` | 80 | one `busy` line | `herdrcore/inputs.py:66,197-206` |
| hc `unavailable` refusal, `not_sent` | 80 | one `not_sent` line | `herdrcore/inputs.py:65,197-206` |
| core unreachable during list, `unavailable` | 80 | one `unavailable` line | `herdrcore/inputs.py:220-224,188-194` |
| send request never reached hc, or was cut short (`written` false or `partial`), `not_sent` | 80 | one `not_sent` line | `herdrcore/inputs.py:260-262,188-194` |
| gate refuses the core version (`HcGateError`), untyped `hc version unsupported` | 1 | none | `herdrcore/inputs.py:190-191,201-203`; `hc/client.py:356` |
| listing the city raises `HcRpcError`, untyped "cannot list the city's terminals" | 1 | none | `herdrcore/inputs.py:218-219` |
| hc refusal kind not in `REFUSALS` and not a version kind, untyped "the core refused the input" | 1 | none | `herdrcore/inputs.py:199-203` |

Corroborating pack tests: `tests/unit/test_herdrcore_inputs.py:109-115`
(non-UTF-8 rejected), `tests/unit/test_herdrcore_inputs.py:132-144` (gone for unknown or dead box), `tests/unit/test_herdrcore_inputs.py:147-151`
(pending_interaction), `tests/unit/test_herdrcore_inputs.py:178-188` (cut-short send is `not_sent`), `tests/unit/test_herdrcore_inputs.py:191-198`
(gate refusal exit 1), `tests/unit/test_herdrcore_inputs.py:201-209` (refusal mapping).

**Deadline and timeout.** The op deadline is the "default" class: 30 s, or the
`GC_EXEC_DEADLINE_MS` value when gc sends one (`rpp/ops.py:23-27`,
`rpp/perop.py:129`). gc's own per-op timeout is 30 s
(`internal/runtime/exec/exec.go:56`) and it cancels with SIGTERM to the process
group, then SIGKILL after 2 s
(`docs/reference/exec-session-provider.md:88-98`; `rpp/perop.py:50-57`). The
`nudge` send stops waiting 5 s before the op deadline (`NUDGE_SEND_RESERVE_S`,
`herdrcore/inputs.py:162-165`, passed at `herdrcore/inputs.py:270`; override
applied in `herdrcore/context.py:122-127`; applied at
`hc/client.py:256-262,364-371`). A send hc still holds when the pack stops
waiting is `delivered_unobserved`, not an error, so gc does not re-send
(`herdrcore/inputs.py:27-37,260-264`). A call with no time left before the
deadline is not sent: `HcTimeout(..., written=False)` gives `not_sent`
(`hc/client.py:369-371`; `herdrcore/inputs.py:188-194`). If stdin is still open
at the deadline the op fails before the handler runs (`rpp/perop.py:60-80`).
Similarly, direct `Cancelled` from SIGTERM (e.g. stdin read or pre-handler)
gives exit 1 `canceled (terminated)` (`rpp/perop.py:149-150`), whereas
`HcCancelled` maps to `unavailable` (exit 80, list), `not_sent` (exit 80, send
unwritten/partial), or `delivered_unobserved` (exit 0, send written whole;
`herdrcore/inputs.py:220-224,260-264`).

**stdout cap.** `SMALL_STDOUT_CAP` = 64 KiB for this op (`rpp/ops.py:32`,
`herdrcore/ops_table.py:64-65`). An answer over the cap is replaced by exit 1
with a stderr message (`rpp/perop.py:158-159`). The only non-empty success line
is `delivered_unobserved`; its detail is capped separately at 1024 UTF-8 bytes
and redacted (`rpp/outcome.py:56-57,77-93`). ASCII JSON escaping
(`rpp/jsonval.py:29-35`) can expand those 1024 UTF-8 bytes (e.g. `\uXXXX`
escapes), though the 64 KiB cap still never binds for this encoder.

### Conflicts

Reported, not resolved.

- gc's contract says **"Best-effort interrupt/nudge: Return 0 even if the session
  doesn't exist"** (`docs/reference/exec-session-provider.md:370`), but the pack
  answers `gone` with exit 80 for a missing or dead box
  (`herdrcore/inputs.py:213-214,225-228`). Under gc `v1.5.1` this matters: gc
  declares no `input-outcome` capability
  (`internal/runtime/protocol.go:15-64`) and treats any non-zero, non-2 exit as a
  plain failure (`internal/runtime/exec/exec.go:153-170`), so a gone pane
  surfaces as a `nudge` error. The pack emits exit 80 unconditionally for
  non-success typed outcomes – `op_nudge`, `answer_result` and `outcome_result`
  perform no caller opt-in or capability check (`herdrcore/inputs.py:181-185,342-348`,
  `rpp/outcome.py:103-115`), and `handshake.py:43` declares `input-outcome`
  statically, so both direct callers and gc v1.5.1 receive exit 80; parity
  §nudge row 3 classifies the exit-80 `gone` answer as `intentional`.
- Internal pack inconsistency: `hc/client.py:14-17` says `send` "gets the
  deadline less two seconds" and `CallTimeouts.send_reserve` defaults to `2.0`
  (`hc/client.py:136`), but the `nudge` path overrides it to 5 s
  (`herdrcore/inputs.py:162-165,270`; `herdrcore/context.py:126-127`). The code
  value (5 s) is the operative one.

### Unverified

- hc's behaviour on an **empty** `nudge` text (no pack test or live probe; the
  parity record §nudge row 7 labels it unverified).
- hc `terminal.send` typing and paste semantics beyond the JSON contract, for
  example control bytes (the parity record §nudge rows 5–6 label the hc side
  unverified).
- Whether any consumer reads the `delivered_unobserved` stdout line at baseline:
  gc `v1.5.1` discards `nudge` stdout (`internal/runtime/exec/exec.go:617`); no
  released gc sends `GC_EXEC_DEADLINE_MS` (`rpp/perop.py:32-33`).

## send-keys

### Registration

- `"send-keys": spec(inputs.op_send_keys)` at `herdrcore/ops_table.py:84`.
- `spec()` defaults (`herdrcore/ops_table.py:64`): `stdin=False`
  (`reads_stdin=False`), `deadline="default"`, `cap=SMALL_STDOUT_CAP`.
- Default deadline class = 30 s (`rpp/ops.py:23-27`); `SMALL_STDOUT_CAP` = 64 KiB
  (`rpp/ops.py:32`). gc's own per-op timeout is 30 s
  (`internal/runtime/exec/exec.go:56`, `NewProvider`; `run` uses it,
  `internal/runtime/exec/exec.go:61-63`).
- Handler: `op_send_keys` calls `answer_result(ctx, "send-keys", send_keys(ctx,
  _name(ctx), ctx.request.args[1:]))` (`herdrcore/inputs.py:351-352`). stdin is
  never read (`rpp/perop.py:135`, only when `reads_stdin`).
- Parity §send-keys (`docs/provider-herdr-parity.md:709`) overall verdict:
  **intentional (with a mistake inside)**; status `implemented`, summary row at
  `docs/provider-herdr-parity.md:69` (`intentional`).

### Inputs

- **argv.** `<script> send-keys <name> [<key>...]`. The runner passes `argv[2:]`
  as `args` (`rpp/perop.py:134`). `name` = `args[0]`, `""` if absent
  (`herdrcore/inputs.py:338-339`); keys = `args[1:]`
  (`herdrcore/inputs.py:352`).
- **gc side.** gc passes each key as its own argv word:
  `append([]string{"send-keys", name}, keys...)`
  (`internal/runtime/exec/exec.go:625-627`). No stdin (`p.run(nil, …)`). gc
  reaches this op only as the fallback after `exec` answers exit 2 with
  `proc.exec` undeclared (`internal/runtime/exec/exec.go:804-808`,
  `internal/runtime/exec/exec.go:894-904`). The pack has no `exec` row
  (`herdrcore/ops_table.py:67-92`), so every gc `SendKeys` takes this path. The
  exec doc lists `send-keys` among the dedicated driving ops
  (`docs/reference/exec-session-provider.md:163`), but its op table
  (`docs/reference/exec-session-provider.md:125-140`) has no `send-keys` row, so
  no argv grammar is documented there.
- **Keys gc sends** (all are in the pack's table): `Enter`, `Up`, `Down`
  (`internal/runtime/dialog.go:421,488,492,565,569`), `Escape`
  (`internal/session/submit.go:266`), `C-u` (`internal/session/submit.go:485`).
  Parity §send-keys deviation 2 gives the same list.
- **Key grammar** (`translate_key`, `herdrcore/inputs.py:126-148`),
  case-sensitive:
  1. Exact match in `TMUX_KEYS` (`herdrcore/inputs.py:82-107`) gives one hc name:
     Enter, Escape/Esc, Tab, BTab→`shift+tab`, BSpace, Space,
     Up/Down/Left/Right, Home, End, PageUp/PgUp/PPage, PageDown/PgDn/NPage,
     IC/Insert, DC/Delete, F1–F12.
  2. Modifier prefix: while `len(rest) > 2` and `rest[:2]` is `C-`, `M-` or `S-`,
     strip it to `ctrl+`, `alt+` or `shift+` in typed order
     (`herdrcore/inputs.py:113,130-134`). The base is a `TMUX_KEYS` name or a
     single character lowercased; a single-character base must be in
     `HC_CHARACTERS` (`herdrcore/inputs.py:110,136-139`). Anything else gives
     None. Tests: `C-c`, `M-x`, `C-M-a`, `C-Up` map; `C-!` is rejected
     (`tests/unit/test_herdrcore_inputs.py:26-55,58-60`).
  3. Otherwise **literal text**, char by char (`herdrcore/inputs.py:140-148`):
     `[a-z0-9]` and ``` `\[],=-.';/ ``` as themselves, space gives `space`, `A-Z`
     gives `shift+<lower>` (`herdrcore/inputs.py:110,116-123`). Any other char
     (`!`, `é`, `"`, newline) or the empty string gives None. Test: `"ok go"`
     gives `o k space g o` (`tests/unit/test_herdrcore_inputs.py:48`).
  - Derived from the code, no test: lower-case or mis-cased names are literal
    text (`enter` types `e n t e r`; `Return` types `shift+r e t u r n`). `C-` alone
    (length 2) is literal (`shift+c`, `-`); `C- ` is rejected (space is not in
    `HC_CHARACTERS`; use `C-Space`).
- **`translate_keys`** (`herdrcore/inputs.py:151-159`) translates all keys first;
  the first untranslatable key aborts the whole call before any hc call.
- **Preconditions.** Translation runs before the lookup
  (`herdrcore/inputs.py:285-288`), so a bad key is `rejected` even for a name no
  box holds (`tests/unit/test_herdrcore_inputs.py:233-243`). Then `name` must be
  non-empty and match a city terminal whose newest record is intact, else `gone`
  (`_target`, `herdrcore/inputs.py:209-229`). Zero keys with a live box gives
  `delivered` with no hc call (`herdrcore/inputs.py:292-293`); zero keys with no
  box gives `gone`.

### Success output

- Exit 0, **empty stdout**, empty stderr: `InputOutcome("delivered")`
  (`herdrcore/inputs.py:301`), then `OpResult.ok()` (`rpp/outcome.py:110-111`).
  `send_keys` never answers `delivered_unobserved`; that word is `nudge`-only
  (`rpp/outcome.py:51-54,60-62`).
- It first calls hc `terminal.keys` with the translated list
  (`herdrcore/inputs.py:295`; `hc/client.py:264-266`, budget 10 s,
  `hc/client.py:132`), then stamps the input floor for `get-last-activity`
  (`_noted`, `herdrcore/inputs.py:232-235,300`; `activity.note_input`
  `herdrcore/activity.py:88-91`).
- gc's reader: `sendKeysOp` discards stdout (`_, err := p.run(…)`,
  `internal/runtime/exec/exec.go:626`); `runWithContext` returns the trimmed
  stdout and a nil error on exit 0 (`internal/runtime/exec/exec.go:102-104`).
  Test: `test_send_keys_translates_and_delivers`
  (`tests/unit/test_herdrcore_inputs.py:228-231`): keys `["down","enter"]`,
  `(0, None)`.

### Failure cases

Exit codes: see the shared contract (`rpp/exitcodes.py:11,14,30`). Exit 80
prints one `{"outcome":"<word>","detail":"…"}` line on stdout (`detail` is
optional in `rpp/outcome.py:96-100` and omitted if empty after cleaning, though
`send-keys` supplies non-empty detail) and `<word>[: detail]` on stderr
(`rpp/outcome.py:114-115`). Exit 1 prints nothing on stdout
(`OpResult.fail`, `rpp/ops.py:78-79`). `send-keys` may only answer words outside
`NUDGE_ONLY_WORDS` (`rpp/outcome.py:51-62`): `not_sent`, `rejected`,
`unavailable`, `gone`.

| Case | Exit | stdout | Source |
|---|---|---|---|
| a key has no hc spelling (unknown or odd char, bad modifier base, empty arg) | 80 | `rejected`; the detail names the key, "nothing was typed" | `herdrcore/inputs.py:285-287`; `tests/unit/test_herdrcore_inputs.py:233-243,58-60` |
| empty or absent name | 80 | `gone` ("no session name") | `herdrcore/inputs.py:213-214` |
| no record carries the name | 80 | `gone` ("no box holds the name") | `herdrcore/inputs.py:225-226`; `tests/unit/test_herdrcore_inputs.py:131-134` (`test_a_name_no_box_holds_is_gone`) |
| newest record not intact (finished or broken) = dead box | 80 | `gone` ("the box's program has ended") | `herdrcore/inputs.py:227-228`; `tests/unit/test_herdrcore_inputs.py:138-145` |
| hc `not_found` or `finished` refusal | 80 | `gone` | `herdrcore/inputs.py:61-63,199,206`; `REFUSAL_DETAILS` `herdrcore/inputs.py:72-73` |
| hc `unavailable` refusal (terminal starting or waiting for a shell) | 80 | `not_sent` | `herdrcore/inputs.py:65,199,206`; `tests/unit/test_herdrcore_inputs.py:260-263` |
| hc `blocked` refusal | 80 | `not_sent` (narrowed from `pending_interaction`) | `herdrcore/inputs.py:64,204-205`. hc does not refuse keys while blocked: `tests/unit/test_herdrcore_inputs.py:310` |
| hc `status_mismatch` refusal | 80 | `not_sent` (narrowed from `busy`) | `herdrcore/inputs.py:66,204-205` |
| core down (connect fails) during list | 80 | `unavailable` | `herdrcore/inputs.py:220-224`; `tests/unit/test_herdrcore_inputs.py:221-225` (fault `unavailable`) |
| request not written, or only partly written (`not err.written or err.partial`) | 80 | `not_sent` | `_written_unknown` then `_unsent`, `herdrcore/inputs.py:277-281,188-194` |
| request written, reply lost, hc timeout after write, or op deadline hit mid-call | **1** | none; stderr "outcome unknown: the keys may have reached the program (…)". Never retried | `herdrcore/inputs.py:279-281,298-299`; `hc/client.py:19-22` (keys not retried); `tests/unit/test_herdrcore_inputs.py:244-257`, `tests/unit/test_herdrcore_inputs.py:313-320` (`lose_reply`) |
| no time left before the deadline for the call | 80 | `not_sent` (`HcTimeout`, `written=False`) | `hc/client.py:369-371`; `herdrcore/inputs.py:188-194` |
| hc refusal with another kind | 1 | none; stderr "the core refused the input (<kind>)" | `herdrcore/inputs.py:203` |
| version-gate refusal (`HcGateError`) or version kind (`needs_update`, …) | 1 | none; "hc version unsupported: run gc doctor" | `herdrcore/inputs.py:80,167,190-191,201-202,222-223`; `hc/client.py:356` |
| listing the city raises an hc RPC error | 1 | none | `herdrcore/inputs.py:218-219` |
| stdout over cap | 1 | none | `rpp/perop.py:158-159`; unreachable in practice (detail is capped at 1024 UTF-8 bytes before ASCII JSON escaping, `rpp/outcome.py:56`, `rpp/jsonval.py:29-35`, so the 64 KiB cap does not bind) |
| no time left when the handler would start, or direct `Cancelled` (SIGTERM before handler) | 1 | none (`no time left before the op's deadline` or `canceled (terminated)`) | `rpp/perop.py:144-145,149-150` (`HcCancelled` in `_target` gives exit 80 `unavailable`, `herdrcore/inputs.py:220-224`; in `terminal.keys` gives exit 80 `not_sent` if unwritten/partial or exit 1 `outcome unknown` if written, `herdrcore/inputs.py:277-281`) |
| gc kills the op at its 30 s timeout | gc reports an error | n/a | `internal/runtime/exec/exec.go:79-86,100-120`; the pack deadline is the same 30 s (`rpp/ops.py:23-27`) |

gc side: any exit other than 0 and 2 is an error wrapping stderr
(`internal/runtime/exec/exec.go:153-170`), so exits 1 and 80 both surface as an
error from `SendKeys`; callers wrap it (`internal/session/submit.go:266-267`,
`internal/session/submit.go:485-486`). gc v1.5.1 has no `input-outcome` capability: no mention in
`internal/runtime/protocol.go:15-64` or the exec doc, so it does not read the
word on stdout (`rg` over both found no match).

### Conflicts

- **Missing or dead session: pack vs gc.** Pack: `gone`, exit 80
  (`herdrcore/inputs.py:213-228`). gc exec doc: "Best-effort interrupt/nudge:
  Return 0 even if the session doesn't exist"
  (`docs/reference/exec-session-provider.md:370`). That line names `interrupt` and
  `nudge`, **not `send-keys`**. A stronger gc source is the runtime conformance
  suite: `SendKeys_MissingSession` expects no error for a nonexistent session
  (`internal/runtime/runtimetest/conformance.go:599-603`; section header
  "Signaling (best-effort)" `internal/runtime/runtimetest/conformance.go:591`).
  The pack disagrees: it exits 80, which gc reads as an error
  (`internal/runtime/exec/exec.go:153-170`). Parity §send-keys deviation 4 records
  this as `matches` after GCREX-70 declared `input-outcome`; gc 1.5.1 reads the
  same exit 80 as a plain error. The parity doc does not cite
  `internal/runtime/runtimetest/conformance.go`. In gc v1.5.1, neither CLI suite
  runs `SendKeys_MissingSession`: `cmd/gc/cmd_runtime_check.go:60-64` invokes
  `rppcheck` and `cmd/gc/cmd_runtime_conformance.go:53` invokes
  `runtimecontract` (`internal/runtime/runtimecontract/catalog.go:85-95,113-125`
  explicitly excludes the six legacy driving ops), so pack exit 80 does not fail
  those CLI suites.
- **Written-but-unanswered keys.** The pack exits 1 and never retries
  (`herdrcore/inputs.py:279-281`); gc treats exit 1 as a plain error. Parity
  deviation 6: `intentional`, "same effective outcome". No conflict with the gc
  source.
- **Output shape.** The pack's empty stdout on success matches gc, which discards
  stdout (`internal/runtime/exec/exec.go:626`).
- **Unknown key / literal text.** Parity deviation 1 (`intentional`) is about the
  Rex provider; the herdr-core pack types unknown multi-char strings as literal
  text rather than passing them through. gc never sends literal text via
  `send-keys` (keys listed under "Inputs"), so no gc path reaches it. Parity row 5
  (zero keys looks up first) and row 7 (all keys validated first) match the code
  (`herdrcore/inputs.py:285-293`).
- **Parity-doc line refs are stale vs the pinned pack.** The parity doc cites
  `herdrcore/inputs.py:70-97`, `herdrcore/inputs.py:123-148`, `herdrcore/inputs.py:151-160`, `herdrcore/inputs.py:274-281`, `herdrcore/inputs.py:285-287`; at
  `0f07496` the same code is at `herdrcore/inputs.py:82-107`, `herdrcore/inputs.py:126-148`,
  `herdrcore/inputs.py:151-159`, `herdrcore/inputs.py:277-281`, `herdrcore/inputs.py:284-301`. The behaviour described matches. The parity
  doc's herdr-core paragraph says `_noted` at `herdrcore/inputs.py:299`; it is at
  `herdrcore/inputs.py:300`.
- Parity overall verdict for `send-keys`: **intentional (with a mistake
  inside)**; status `implemented`, summary row at
  `docs/provider-herdr-parity.md:69` (`intentional`).

### Unverified

- Whether hc `terminal.keys` is atomic on a partly invalid or mid-list refused key
  list; the pack pre-validates, so it never sends one (parity row 7 and follow-ups
  leave Rex-side atomicity unverified; the hc side was not checked).
- Whether any non-gc caller triggers `SendKeys_MissingSession` in Go test runs;
  neither `gc runtime check` (`cmd/gc/cmd_runtime_check.go:60-64`) nor `gc
  runtime conformance` (`cmd/gc/cmd_runtime_conformance.go:53`,
  `internal/runtime/runtimecontract/catalog.go:85-95,113-125`) invokes it against
  an exec pack.
- How hc handles `shift+<letter>` and punctuation names beyond the fake: the unit
  test only checks that `fakehc.terminal.key_bytes` accepts them
  (`tests/unit/test_herdrcore_inputs.py:52-55`), not real hc.
- Which hc refusal kinds `terminal.keys` can actually return (the pack maps the
  five in `REFUSALS`; the fault tests show `blocked` does not refuse keys,
  `tests/unit/test_herdrcore_inputs.py:310`).
- Whether any non-gc caller sends keys outside the table (for example `C-c`,
  `BTab`): the pack handles them (`tests/unit/test_herdrcore_inputs.py:26-48`) but
  gc 1.5.1 sends none.

## interrupt

### Registration

- `"interrupt": spec(inputs.op_interrupt)` – `herdrcore/ops_table.py:85`.
- `spec()` defaults: `stdin=False`, `deadline="default"`, `cap=SMALL_STDOUT_CAP`
  (`herdrcore/ops_table.py:64`). So `reads_stdin=False` (stdin never read;
  `rpp/perop.py:135` reads it only when `spec.reads_stdin`), deadline class
  `default` = 30 s unless gc sends `GC_EXEC_DEADLINE_MS` (`rpp/ops.py:23-27`,
  `rpp/perop.py:31,129`), stdout cap 64 KiB (`rpp/ops.py:32`). An over-cap stdout
  becomes exit 1 (`rpp/perop.py:158-159`); never reachable here (at most one short
  line).
- Housekeeping on (`housekeeping=True`): `lifecycle.sweep_spawn_files(ctx)` runs
  before the handler (`herdrcore/ops_table.py:45-46`).
- gc side: `script interrupt <name>`, no stdin, no stdout
  (`docs/reference/exec-session-provider.md:129`); gc timeout 30 s
  (`internal/runtime/exec/exec.go:53-57`).
- The handshake declares `carrier.ops` and `input-outcome` (`handshake.py:41,43`);
  `proc.exec` is not declarable (`rpp/wire.py:20-37`).
- Parity §interrupt (`docs/provider-herdr-parity.md:743-771`) verdict:
  **intentional**.

### Inputs

- argv: `interrupt <name>`; the name is `args[0]`, else `""` (`_name`
  `herdrcore/inputs.py:338-339`, `op_interrupt` at `herdrcore/inputs.py:355-356`).
  Extra args are ignored by the handler (whether callers send extras is
  `unverified`).
- Preconditions: none enforced beyond the name resolving to an intact box (below).
  An empty name gives `gone` (`herdrcore/inputs.py:213-214`; test
  `tests/unit/test_herdrcore_inputs.py:279-280`).
- Flow (`herdrcore/inputs.py:304-317`): `_target` lists city terminals filtered by
  session label (`herdrcore/reads.py:68-73`), takes `newest`
  (`herdrcore/records.py:48-50`), and requires `is_intact`
  (`herdrcore/records.py:53-62`: live, has pid, reason not broken, pid label
  matches). Then `client.interrupt(record.address)` = hc `terminal.interrupt
  {terminal, timeout: 3.0}` (`hc/client.py:268-270`, `INTERRUPT_WAIT_S`
  `hc/client.py:102`). hc's reply (`outcome`, `key`, `hc/parse.py:149-151`) is
  discarded: "Whatever hc saw (stopped, exited, still working): the stop key was
  pressed" (`herdrcore/inputs.py:310`).
- Call timeout: min(5 s, remaining − 1 s margin) (`hc/client.py:133`,
  `rpp/cancel.py:27,98-103`). Never retried: only hello, get, list and read are
  idempotent (`hc/client.py:87,300-305`; comment `herdrcore/inputs.py:314`).

### Success output

- Exit 0, stdout empty. `InputOutcome("delivered")` gives `OpResult.ok()` with no
  line (`herdrcore/inputs.py:317`; `rpp/outcome.py:110-111`). stderr empty. It does
  not touch `activity.note_input` (unlike `nudge` and `send-keys`) –
  `herdrcore/inputs.py:304-317` has no `_noted` call.
- hc's `outcome` (stopped, exited, still working) does not change this:
  `test_interrupt_is_delivered_whatever_hc_observed` gives `(0, None)`
  (`tests/unit/test_herdrcore_inputs.py:274-276`).
- **Nonexistent session: NOT exit 0.** Exit 80, one stdout line
  `{"outcome":"gone","detail":"no box holds the name"}\n` (`detail` is optional
  in the shared encoder `rpp/outcome.py:96-100` when empty, though non-empty
  here; `rpp/outcome.py:114-115`, `herdrcore/inputs.py:225-226`), stderr
  `interrupt: gone: no box holds the name`. Test: `test_a_name_no_box_holds_is_gone`
  (`tests/unit/test_herdrcore_inputs.py:132-135`).
- gc reader: `exec.Provider.Interrupt` tries the carrier, and on
  `ErrExecUnsupported` calls `interruptOp` = `p.run(nil,"interrupt",name)`
  (`internal/runtime/exec/exec.go:630-648`); `exec` exit 2 with no `proc.exec`
  yields `ErrExecUnsupported` (`internal/runtime/exec/exec.go:894-904`). `run`
  returns stdout only on exit 0 and discards it (`_, err :=`,
  `internal/runtime/exec/exec.go:631`); any exit other than 0 and 2 is an error
  embedding stderr (`internal/runtime/exec/exec.go:153-181`, `runError`).
- Contract line: "**Best-effort interrupt/nudge**: Return 0 even if the session
  doesn't exist." (`docs/reference/exec-session-provider.md:370`). The exit table
  lists only 0, 1 and 2 (`docs/reference/exec-session-provider.md:71-77`).

### Failure cases

Stdout line shape for exit 80: `{"outcome":"<word>","detail":"..."}\n` (`detail`
is optional in `rpp/outcome.py:96-100` and omitted if empty after cleaning,
though `interrupt` paths supply detail); stderr `interrupt: <word>[: detail]`
(`rpp/outcome.py:114-115`). Generic runner failures are in the shared contract.

| Case | Exit | stdout | Source |
|---|---|---|---|
| no name, or no box holds it | 80 | `gone` | `herdrcore/inputs.py:213-214,225-226` |
| dead, finished or broken record (newest not intact) | 80 | `gone` ("the box's program has ended") | `herdrcore/inputs.py:227-228`; test `tests/unit/test_herdrcore_inputs.py:138-144`; fault `dead` `tests/unit/test_herdrcore_inputs.py:330-335` |
| core unreachable (socket down) | 80 | `unavailable` | `herdrcore/inputs.py:220-224`, `_unsent` `herdrcore/inputs.py:188-194`; test `tests/unit/test_herdrcore_inputs.py:220-224`; also kicks a start `herdrcore/reads.py:91-95` |
| list call fails with another `HcError` (timeout, protocol, no time left) | 80 | `unavailable` | `herdrcore/inputs.py:220-224` (any non-Rpc, non-gate `HcError`); no dedicated test (unverified) |
| list call refused by hc (`HcRpcError`) | 1 | – | `herdrcore/inputs.py:218-219` ("cannot list the city's terminals") |
| hc refuses `interrupt`: `not_found` or `finished` | 80 | `gone` | `herdrcore/inputs.py:61-67,197-206` |
| hc refuses: `unavailable` | 80 | `not_sent` | `herdrcore/inputs.py:61-67` |
| hc refuses: `blocked`, `status_mismatch` | 80 | `not_sent` (nudge-only words remapped) | `herdrcore/inputs.py:204-205`; `rpp/outcome.py:51-56` |
| hc refuses: version kinds (`needs_update`, `unsupported_generation`, `incompatible`) | 1 | – ("hc version unsupported: run gc doctor") | `herdrcore/inputs.py:80,167,201-202` |
| hc refuses: any other kind | 1 | – (names the kind) | `herdrcore/inputs.py:203`; nudge-based test `tests/unit/test_herdrcore_inputs.py:211-216` |
| version gate refuses the core | 1 | – | `herdrcore/inputs.py:190-191,222-223`; test `tests/unit/test_herdrcore_inputs.py:191-197` (parametrized over `interrupt`) |
| request not written, or cut short (for example deadline before send, cancel before write) | 80 | `not_sent` | `herdrcore/inputs.py:277-281` then `_unsent`; `hc/client.py:364-371` (`written=False`) |
| request written, reply lost, or deadline during the 3 s wait | 1 | – ("outcome unknown: the interrupt may have reached the program") | `herdrcore/inputs.py:277-281`; tests `tests/unit/test_herdrcore_inputs.py:266-271` (call count 1), `tests/unit/test_herdrcore_inputs.py:317-320` (`lose_reply`) |
| deadline already expired before the handler | 1 | – ("no time left before the op's deadline") | `rpp/perop.py:144-145` |
| cancel (SIGTERM) | 1 (direct `Cancelled` or written `HcCancelled`), or 80 (unwritten/partial `HcCancelled`) | empty (exit 1) or `unavailable`/`not_sent` line (exit 80) | direct `Cancelled` before handler gives `canceled (terminated)` (`rpp/perop.py:143,149-150`); `HcCancelled` in `_target` gives exit 80 `unavailable` (`herdrcore/inputs.py:220-224`); in `client.interrupt` gives exit 80 `not_sent` if unwritten/partial or exit 1 `outcome unknown` if written (`herdrcore/inputs.py:277-281`) |
| state-dir, config or lock errors in the wrapper | 1 | – | `herdrcore/ops_table.py:48-51` |
| stdout over cap | 1 | – | `rpp/perop.py:158-159` (unreachable in practice) |
| core down via `GC_RPP_FAULT=unavailable` | 80 | `unavailable` | `hc/client.py:311-312` (inferred from `_gated_call`); test `tests/unit/test_herdrcore_inputs.py:220-224` |

Deadline wording: with gc's default 30 s the `interrupt` call gets at most 5 s,
so deadline expiry only matters when gc sends a short `GC_EXEC_DEADLINE_MS` (the
test uses 3000 ms).

### Conflicts

1. **Nonexistent or dead session** – the pack exits 80 plus a `gone` line; the gc
   v1.5.1 contract says return 0. The Rex provider declares `input-outcome`
   with `carrier.ops` (GCREX-70), so `runWithContext` answers a missing or dead
   pane with exit 80 `gone` (`cmd/gc-runtime-rex/main.go:278-281`); a gc that
   reads the capability projects it to `ErrSessionNotFound`, and gc 1.5.1 reads
   the same exit 80 as a plain error, regressing the contract line
   `docs/reference/exec-session-provider.md:370`. gc 1.5.1 has no
   `input-outcome` (no hits for `input-outcome` or `carrier.ops` in
   `internal/runtime/` or the docs; `idle-boundary` appears only in comments at
   `internal/runtime/hybrid/hybrid.go:113`, `internal/runtime/auto/auto.go:280`).
   Its exit table is 0/1/2 (`docs/reference/exec-session-provider.md:71-77`), so
   exit 80 is an ordinary failure: `exec provider <script> interrupt <name>:
   interrupt: gone: …` (`internal/runtime/exec/exec.go:153-181`). Parity row 3
   records `matches` against herdr-core (`docs/provider-herdr-parity.md`
   §interrupt). The conflict that remains is with gc v1.5.1's best-effort
   contract line above: that baseline has no synthetic input-outcome type to
   project exit 80 into, so the deviation is gc-version dependent rather than
   hidden.
2. Unknown capability strings are ignored by gc
   (`internal/runtime/protocol.go:16`;
   `docs/reference/exec-session-provider.md:183`), so declaring `input-outcome`
   does not break v1.5.1.
3. Pack vs parity doc agree on: mechanism via hc `terminal.interrupt`, the 3 s
   settle wait, no retry, and unknown-after-write giving exit 1 (parity
   `docs/provider-herdr-parity.md:747`, `docs/provider-herdr-parity.md:750-753`, rows 1, 2, 4, 5). Parity's
   line cites checked: `herdrcore/ops_table.py:85`, `herdrcore/inputs.py:304-319`,
   `herdrcore/inputs.py:277-281`, `herdrcore/inputs.py:314`, `hc/client.py:268-270` match; `herdrcore/inputs.py:209-229`
   is `_target`; `hc/client.py:102` is `INTERRUPT_WAIT_S`.
4. Parity verdict for `interrupt`: **intentional**; the open point row 5 (retry)
   is Rex-side (`docs/provider-herdr-parity.md:756-757`). Not changed by this
   check.
5. gc's own sequencing: gc never retries `interrupt` itself (parity row 5; the
   `internal/session/submit.go` cites are parity-only, not re-read here).

### Unverified

- What hc `terminal.interrupt` actually sends (SIGINT vs `\x03`); the test fake
  uses SIGINT (`tests/fakehc/terminal.py:363`) and
  `tests/unit/test_hc_client.py:83` accepts `esc` or `ctrl+c`. The real hc source
  was not read.
- Whether any caller sends extra argv after `<name>` (the handler ignores them
  by construction, `herdrcore/inputs.py:338-339,355-356`).
- Exit and stdout for a list-call timeout (`HcTimeout` gives `unavailable`) –
  derived from the code, no test.
- Which gc release first reads exit 80 / `input-outcome` as typed.
- The `internal/session/submit.go:273-305` claims in parity (idle-wait,
  Escape-then-C-c fallback) – not re-verified here.
- No unit test drives `interrupt` through the hc refusals `not_found`,
  `finished` or `unavailable` (the refusal test uses `terminal.send`,
  `tests/unit/test_herdrcore_inputs.py:200-208`); the interrupt mapping is shared
  code (`_refused`).

## clear-scrollback

### Registration

- `"clear-scrollback": spec(inputs.op_clear_scrollback)` at
  `herdrcore/ops_table.py:86`. Parity cites `herdrcore/ops_table.py:86` – matches.
- `spec()` defaults (`herdrcore/ops_table.py:64-65`) give `reads_stdin=False`
  (stdin is `b""`, `rpp/perop.py:135`), deadline class `default` = 30 s
  (`rpp/ops.py:23-27`, `deadline_for` `rpp/ops.py:119-128`), and stdout cap
  `SMALL_STDOUT_CAP` = 64 KiB (`rpp/ops.py:32`; check at `rpp/perop.py:158-159`).
- `GC_EXEC_DEADLINE_MS` overrides the class default (`rpp/perop.py:129`). gc
  v1.5.1 never sets it: no hit in the tree (`grep` over `*.go`, `*.md`), so the
  30 s default applies. gc's own timeout is also 30 s
  (`internal/runtime/exec/exec.go:60-64`, `p.run` uses `p.timeout`).
- Handler `op_clear_scrollback` (`herdrcore/inputs.py:359-360`), then
  `clear_scrollback` (`herdrcore/inputs.py:320-335`), then `answer_result`
  (`herdrcore/inputs.py:181-185`), then `outcome_result`
  (`rpp/outcome.py:103-115`).
- Parity §clear-scrollback (`docs/provider-herdr-parity.md:773`) verdict
  **intentional**.

### Inputs

- argv: `gc-runtime-herdr-core clear-scrollback <name>`. gc sends exactly
  `clear-scrollback <name>` (`internal/runtime/exec/exec.go:634-637`,
  `p.run(nil, "clear-scrollback", name)`). stdin nil. On the raw `exec.Provider`
  fallback path, gc reaches this op only after the carrier's `exec` probe returns
  exit 2 with `proc.exec` undeclared (`internal/runtime/exec/exec.go:777-781,894-904`;
  the pack answers `exec` with exit 2, parity §exec), costing two process
  invocations once the handshake is cached (or an extra lazy `protocol`
  invocation on the first exit-2 probe, `internal/runtime/exec/exec.go:901`,
  `internal/runtime/exec/handshake.go:16-24`). On the production `seamProvider`
  path (`internal/runtime/seam_adapter.go:88-99`,
  `internal/runtime/exec/seams.go:58-62,159-167`), gc first runs `is-running`
  twice (`rt.Open` and `tp.Open`) before those driving calls, so a successful
  seam-backed clear costs four invocations (plus `protocol` if uncached); a
  missing or dead session can short-circuit earlier on `is-running`.
- Name: `_name` takes `args[0]`, else `""` (`herdrcore/inputs.py:338-339`). An
  empty name answers `gone` ("no session name") (`herdrcore/inputs.py:213-214`).
  Extra args are ignored (only `args[0]` is read).
- Preconditions: the core must be reachable, and a live record carrying the gc
  name label must exist whose program is intact (`herdrcore/inputs.py:215-228`,
  `herdrcore/records.py:53-62`). Name to record: the newest record for the name
  (`herdrcore/records.py:43-50`, via `list_city`, `herdrcore/inputs.py:217`).

### Success output

- Exit 0, stdout empty, stderr empty. `word == "delivered"` gives `OpResult.ok()`
  (`rpp/outcome.py:110-111`). `delivered_unobserved` is nudge-only
  (`rpp/outcome.py:51-53`, `rpp/outcome.py:60-62`) and never returned here.
- What "success" means: `herdrcore/inputs.py:320-335`. It looks up the box, issues
  one `terminal.get` (`herdrcore/inputs.py:328`) and, if the program is still
  intact (`herdrcore/inputs.py:333`), answers `delivered`. **Nothing is cleared or
  sent**; docstring `herdrcore/inputs.py:321-322`: "hc keeps no scrollback a
  client can clear: delivered (a no-op)". Test
  `test_clear_scrollback_is_a_delivered_no_op` asserts exit 0, no word, and empty
  `input_log` and `key_log` (`tests/unit/test_herdrcore_inputs.py:283-286`).
  Parity §clear-scrollback "herdr-core" agrees.
- gc reader: yes, gc v1.5.1 calls it – `ClearScrollback`
  (`internal/runtime/exec/exec.go:777-782`) calls `clearScrollbackOp`
  (`internal/runtime/exec/exec.go:634-637`), and the reconciler calls
  `sp.ClearScrollback(name)` after a successful kill, discarding the result:
  `cmd/gc/session_reconciler.go:3739` (max-age) and `cmd/gc/session_reconciler.go:3894` (idle). gc reads only
  the exit code: 0 gives nil (`internal/runtime/exec/exec.go:104-105`); stdout is
  trimmed and dropped (the `p.run` result is ignored,
  `internal/runtime/exec/exec.go:635`).
- Missing session on gc's side: gc has no special case. A "missing session" is the
  pack's `gone`, exit 80, then a gc error (`internal/runtime/exec/exec.go:159`,
  `internal/runtime/exec/exec.go:165-171`). gc does not treat it as success. The only exit gc maps to success
  is 2 (`internal/runtime/exec/exec.go:159`, `internal/runtime/exec/exec.go:174-181`), which the pack does not
  return for this op. The caller discards the error either way
  (`cmd/gc/session_reconciler.go:3739`, `cmd/gc/session_reconciler.go:3894`).
- gc v1.5.1 has no `input-outcome` reader: no hit for `input-outcome`,
  `InputOutcome` or `exit 80` in v1.5.1 Go or docs (grep). Exit 80 is therefore a
  generic non-zero failure for it (`internal/runtime/exec/exec.go:165-171`).

### Failure cases

All exit 80 cases print one line `{"outcome":"<word>","detail":"..."}` plus `\n`
on stdout (`detail` is optional in `rpp/outcome.py:96-100` and omitted if empty
after cleaning, though these paths provide detail), and `<word>[: detail]`
prefixed with the op name on stderr (`rpp/outcome.py:96-100,114-115`;
`rpp/perop.py:166-167`). Exit 1 prints nothing to stdout, only stderr
(`rpp/ops.py:78-79`). Generic runner failures are in the shared contract.

| Case | Exit | stdout | Source |
|---|---|---|---|
| no name (`clear-scrollback` with no or empty arg) | 80 | `{"outcome":"gone","detail":"no session name"}` | `herdrcore/inputs.py:213-214` (same path as the `interrupt` test `test_interrupt_without_a_name_is_gone`, `tests/unit/test_herdrcore_inputs.py:279`; no clear-scrollback-specific test: unverified) |
| no box holds the name | 80 | `{"outcome":"gone","detail":"no box holds the name"}` | `herdrcore/inputs.py:225-226`; test `test_a_name_no_box_holds_is_gone` (`tests/unit/test_herdrcore_inputs.py:131-135`, parametrized over `DRIVING` including clear-scrollback, `tests/unit/test_herdrcore_inputs.py:97`) |
| dead box (record finished, pid mismatch, broken reason) | 80 | `{"outcome":"gone","detail":"the box's program has ended"}` | `herdrcore/inputs.py:227-228`, `herdrcore/records.py:53-62`; test `test_a_dead_box_is_gone_and_gets_nothing` (`tests/unit/test_herdrcore_inputs.py:137-145`); fault `dead` `test_fault_dead_reads_dead_and_every_driving_op_is_gone` (`tests/unit/test_herdrcore_inputs.py:330`), `hc/client.py:282-287` |
| program ended between the list and the `get` | 80 | `gone`, detail "the box's program has ended" | `herdrcore/inputs.py:333-334` (no dedicated test: unverified) |
| core unreachable at list time | 80 | `{"outcome":"unavailable","detail":"cannot reach the city's core; nothing was sent: ..."}` | `herdrcore/inputs.py:220-224`; test `test_driving_ops_with_the_core_down_are_unavailable` (`tests/unit/test_herdrcore_inputs.py:221-224`). Also kicks a core start via `unreachable` (`herdrcore/inputs.py:221`, `herdrcore/reads.py:91-95`) |
| hc version or generation gate at list time | 1 | empty; stderr `hc version unsupported: run gc doctor (...)` | `herdrcore/inputs.py:222-223`, `herdrcore/inputs.py:167`, `answer_result` `herdrcore/inputs.py:183-184` |
| hc RPC error while listing | 1 | empty; stderr `cannot list the city's terminals: ...` | `herdrcore/inputs.py:218-219` |
| hc refuses `terminal.get`: `not_found` or `finished` | 80 | `gone`; detail "the box's terminal is gone" or "the box's program has ended" | `herdrcore/inputs.py:329-330`, `herdrcore/inputs.py:197-206`, `herdrcore/inputs.py:61-63`, `herdrcore/inputs.py:72-78` |
| hc refuses `terminal.get`: `unavailable` | 80 | `not_sent`; detail "the terminal is not ready for input; nothing was typed" | `herdrcore/inputs.py:64-65`, `herdrcore/inputs.py:75-76` |
| hc refuses `terminal.get`: `blocked` or `status_mismatch` | 80 | `not_sent` (nudge-only words `pending_interaction` and `busy` remapped) with the kind's detail | `herdrcore/inputs.py:204-206`, `herdrcore/inputs.py:64-66`; `rpp/outcome.py:51-53`. Not reachable in practice (`get` is a read); no test: unverified |
| hc refuses with a version kind (`needs_update`, `unsupported_generation`, `incompatible`) | 1 | empty; stderr `hc version unsupported...` | `herdrcore/inputs.py:201-202`, `herdrcore/inputs.py:80` |
| hc refuses with any other kind | 1 | empty; stderr `the core refused the input (<kind>): ...` | `herdrcore/inputs.py:203` |
| `terminal.get` fails, request not written or partial (for example the core went away) | 80 | `unavailable` (`HcUnreachable`) or `not_sent` (other); detail "nothing reached the core" | `herdrcore/inputs.py:331-332`, `herdrcore/inputs.py:277-280`, `herdrcore/inputs.py:188-194` |
| `terminal.get` fails after the request was written (reply lost, timeout) | 1 | empty; stderr `clear-scrollback: outcome unknown: clearing the scrollback may have reached the program (...)` | `herdrcore/inputs.py:279-281`; test `test_clear_scrollback_reply_lost_after_write_exits_1` (`tests/unit/test_herdrcore_inputs.py:289-295`), fault `lose_reply` (`tests/unit/test_herdrcore_inputs.py:313-320`). `get` is retried once on connection loss (`HcConnectionLost`, including hello-stage drops; `hc/client.py:87,98,296-305`; test `tests/unit/test_herdrcore_inputs.py:291-292`), not on `HcTimeout` or `HcProtocolError` |
| deadline expires before the handler starts | 1 | empty; stderr `clear-scrollback: no time left before the op's deadline` | `rpp/perop.py:143-145` |
| deadline or SIGTERM during the call | 1 (direct `Cancelled` or written+unanswered `HcError`/`HcCancelled`), or 80 (`unavailable` / `not_sent` when unwritten or partial) | empty (exit 1), or outcome line (exit 80) | `rpp/perop.py:143-150` does not override returned handler results: `HcTimeout`/`HcCancelled` during `_target` gives exit 80 `unavailable` (`herdrcore/inputs.py:220-224`); during `terminal.get` when unwritten/partial gives exit 80 `not_sent` (`unavailable` for `HcUnreachable`, `herdrcore/inputs.py:188-194,277-281,331-332`); when fully written and unanswered gives exit 1 `outcome unknown` (`herdrcore/inputs.py:277-281`); direct `Cancelled` gives exit 1 `canceled (terminated)` (`rpp/perop.py:149-150`). Query budget 3.0 s (`hc/client.py:128-148`) |
| any unexpected exception | 1 | empty; stderr `internal error: ...` | `rpp/perop.py:153-154` |
| stdout over 64 KiB | 1 | empty | `rpp/perop.py:158-159`; unreachable (detail is capped at 1024 UTF-8 bytes before ASCII JSON escaping, `rpp/outcome.py:56`, `rpp/jsonval.py:29-35`, so the 64 KiB cap does not bind) |
| gc closed the pipe | 1 | – | `rpp/perop.py:164-165` |

### Conflicts

- The pack answers with `input-outcome` exit 80; gc v1.5.1 does not know the token
  (`handshake.py:43`, parity §protocol row 1) and treats any non-0, non-2 exit as
  an error (`internal/runtime/exec/exec.go:159-171`). Result:
  `gone`/`unavailable`/`not_sent` surface to gc as an error, where before
  GCREX-70 a missing or dead session was exit 0. gc discards the error at both
  call sites (`cmd/gc/session_reconciler.go:3739`, `cmd/gc/session_reconciler.go:3894`), so there is no
  behaviour change. Matches parity rows 2–4 (intentional).
- No-op: the pack clears nothing; gc's own carrier form is `tmux clear-history`
  (`internal/runtime/carrier.go:118-121`). Consistent with parity row 1
  (intentional), and with gc ignoring the effect (parity row 6, no-impact).
- Parity wording "Reads `terminal.get` before answering" (row 5,
  `herdrcore/inputs.py:324`): the `get` is at `herdrcore/inputs.py:328`; `herdrcore/inputs.py:324` is
  the `_target` call. The list-then-get sequence is two core reads
  (`herdrcore/inputs.py:217`, `herdrcore/inputs.py:328`), not one – parity row 5 describes only the
  `get`. Minor citation drift: parity cites `herdrcore/inputs.py:320-336`,
  `herdrcore/inputs.py:327-328`, `herdrcore/inputs.py:332-333`, `herdrcore/inputs.py:213-229`; the pinned file has `herdrcore/inputs.py:320-335`, `herdrcore/inputs.py:331-332`
  (unknown after write), `herdrcore/inputs.py:333-334` (ended), `herdrcore/inputs.py:213-228`. Parity's "lost reply exits
  1 (`herdrcore/inputs.py:327-328`)" is actually `herdrcore/inputs.py:331-332` plus
  `herdrcore/inputs.py:277-281` at this pin.
- Parity: "a missing name … answers `gone`, unreachable core answers `unavailable`"
  – confirmed (`herdrcore/inputs.py:213-214`, `herdrcore/inputs.py:225-228`, `herdrcore/inputs.py:220-224`). Parity omits
  the version-gate and unknown-refusal exit-1 cases and the `not_sent` refusal
  mapping (rows above); that is a coverage gap, not a conflict.
- Parity verdict: intentional. No source found that disagrees. No change
  recommended.

### Unverified

- Whether any test covers an empty name, a mid-op program end
  (`herdrcore/inputs.py:333-334`), or hc `blocked` / `status_mismatch` refusals
  for `clear-scrollback` (only `interrupt` has the empty-name test; the
  `terminal.send` and `keys` refusal tests at
  `tests/unit/test_herdrcore_inputs.py:203` are for other ops).
- Real hc behaviour of `terminal.get` refusal kinds (the hc source is not in
  either tree).
- Whether Rex's `clear` drops the visible screen or affects `peek` and attached
  clients (parity row 6, unverified by parity; irrelevant to gc).

## watch-startup

### Registration

- `"watch-startup": spec(screen.op_watch_startup, deadline="start")` –
  `herdrcore/ops_table.py:87`.
- `reads_stdin=False`: `spec()` defaults `stdin=False`
  (`herdrcore/ops_table.py:64`, `OpSpec(... reads_stdin=stdin ...)`). The per-op
  runner passes `stdin=b""` when not read (`rpp/perop.py:135`). stdin is never
  touched.
- Deadline class `start` = 120 s (`rpp/ops.py:23-27`), or `GC_EXEC_DEADLINE_MS` if
  set (`rpp/ops.py:127`, `rpp/perop.py:31,129`); counted from `_STARTED`
  (`bin/gc-runtime-herdr-core:12`, `rpp/ops.py:128`).
- Cap: default `SMALL_STDOUT_CAP` = 64 KiB (`rpp/ops.py:32`; `spec()` default
  `herdrcore/ops_table.py:64`). The cap is checked only on `result.stdout`
  (`rpp/perop.py:158-159`); `emit` writes straight to stdout
  (`rpp/perop.py:122-124`) and `op_watch_startup` returns an empty-stdout
  `OpResult.ok()` (`herdrcore/screen.py:44`). **The cap does not apply to streamed
  lines.**
- Streaming hook: `OpRequest.emit` (`rpp/ops.py:55-57`); the default `_no_stream`
  raises `OpError` (`rpp/ops.py:40-41`); the per-op `emit` = `stdout.write(data);
  stdout.flush()` (`rpp/perop.py:122-124`).
- gc side: the Operations table in `docs/reference/exec-session-provider.md` has
  no `watch-startup` row; it is described only in prose
  (`docs/reference/exec-session-provider.md:116-121`, `docs/reference/exec-session-provider.md:169-170`, `docs/reference/exec-session-provider.md:294-296`).
  Parity: "outside the 17-row table".
- Parity §watch-startup (`docs/provider-herdr-parity.md:805`) verdict:
  `not-implemented`.

### Inputs

- argv: `<script> watch-startup <name>`; gc builds it as
  `exec.CommandContext(watchCtx, p.script, "watch-startup", name)`
  (`internal/runtime/exec/exec.go:390`). **No timeout argument**: the pack reads
  only `args[0]` (`herdrcore/reads.py:57-60`, `herdrcore/screen.py:29`). Any extra
  args are ignored (verified: `session_arg` reads only `args[0]`).
- A missing or empty name gives `OpError("missing session name")`
  (`herdrcore/reads.py:58-59`), exit 1.
- Preconditions: a record for the name must exist, live **or finished**
  (`current_record` docstring and `newest()`, `herdrcore/reads.py:263-278`,
  `herdrcore/records.py:48-50`); none gives `no session 'X'`
  (`herdrcore/screen.py:30-32`). It does not use `live_record`, so a dead box is
  not rejected up front (contrast `copy-to`, `herdrcore/screen.py:53`).
- gc only runs the watcher after a successful `start`, when
  `ShouldAcceptStartupDialogs(cfg)` (`internal/runtime/exec/exec.go:343-346`;
  `internal/runtime/startup_hints.go:20-25`: explicit setting, else
  `process_names` or `EmitsPermissionWarning`). Doc:
  `docs/reference/exec-session-provider.md:288-296`.
- Wrapper: `_wrap` runs the `sweep_spawn_files` housekeeping first and maps
  `StateDirError`, `ConfigError`, `MetaError` and `LockFileError` to exit 1, and
  `LockTimeout` to exit 1 `busy: …` (`herdrcore/ops_table.py:41-58`).

### Success output

Streaming protocol. Pack:

1. Loop `while deadline.budget(0.25) > 0` (`herdrcore/screen.py:35`;
   `WATCH_POLL_S`=0.25 `herdrcore/screen.py:25`; `budget` = min(cap, remaining −
   1 s margin) `rpp/cancel.py:27,98-103`).
2. Each iteration reads the screen only, `lines:0` (`herdrcore/screen.py:37`,
   `hc/client.py:250-254`), never history.
3. If the text differs from the last emitted text (`last=None` initially, so the
   first read always emits, even `""`), it writes one line
   `{"content":"<screen>"}\n` (`herdrcore/screen.py:40-42`). The line is
   `dumps({"content": text})` (`rpp/wire.py:307-314`): compact, ASCII-only JSON
   (`rpp/jsonval.py:29-35`), then `\n`, `.encode("ascii")`. Newlines in the screen
   are `\n` escapes, so one event = one physical line.
4. `cancel.pause(0.25)` (`herdrcore/screen.py:43`), then repeat. Unchanged screens
   emit nothing.
5. Loop exit (when `deadline.budget(0.25) <= 0`, around 1 s before the deadline,
   so roughly 119 s by default; each iteration takes read duration plus the
   250 ms pause, `herdrcore/screen.py:35-44`) gives `OpResult.ok()` = **exit 0,
   no further stdout** (`herdrcore/screen.py:44`), though a read near the
   margin can fail with `cannot read the screen` instead of reaching the normal
   exit-0 loop end (`herdrcore/screen.py:36-39`, `hc/client.py:364-371`). There
   is no "ready" or "done" sentinel line; a normal end is "time ran out", not
   "startup finished". Test: `tests/unit/test_herdrcore_staging.py:205-231`
   (stream with a 3.5 s deadline; the last line contains "two"; no repeated
   lines). Wire shape test: `tests/unit/test_rpp_wire.py:273`.

gc reader and parser (`internal/runtime/exec/exec.go`):

- Reader: `bufio.NewScanner(stdout)`, one `json.Unmarshal` per line into
  `startupWatchEvent{Content string "json:content"}`
  (`internal/runtime/exec/exec.go:42-44,421-425`). The default Scanner token limit
  is 64 KiB (Go stdlib; no `scanner.Buffer` call in `internal/runtime/exec/exec.go`) – an over-long line
  gives `scanner.Err()` (`internal/runtime/exec/exec.go:451,459-461,474-476`).
  Unverified whether a real screen's escaped JSON line can reach that size.
- First event: signals `first` (`internal/runtime/exec/exec.go:435-438`). If none
  arrives within `startupWatchFirstEventTimeout` =
  `runtime.StartupDialogTimeout()` = 8 s (`internal/runtime/exec/exec.go:46`,
  `internal/runtime/dialog.go:15,29-31`), gc cancels and falls back to `peek`
  (`internal/runtime/exec/exec.go:494-497`). The pack emits the first line on its
  first read (`last=None`), so it is normally well inside 8 s.
- Later events go to the `events` channel, then to
  `AcceptStartupDialogsFromStreamWithStatus`
  (`internal/runtime/exec/exec.go:443-448,357-362`;
  `internal/runtime/dialog.go:182`). The reader has no end-of-stream marker; gc
  decides readiness itself and then calls `closeWatch`, which calls `cancel()`
  (`internal/runtime/exec/exec.go:514-517`).
- Cancel: `exec.CommandContext` default kill = SIGKILL; `WaitDelay` 250 ms
  (`internal/runtime/exec/exec.go:389-393`). Doc: "killed outright, with SIGKILL
  … no SIGTERM first" (`docs/reference/exec-session-provider.md:119-121`).
  Killed, 143 and 137 are treated as a normal close
  (`internal/runtime/exec/exec.go:534-543,522-532`). So the pack's SIGTERM handler
  (`rpp/perop.py:50-57`) is a courtesy only.
- If streaming is inconclusive or `closeErr != nil`, gc falls back to
  `AcceptStartupDialogs` with `peek` and `send-keys`
  (`internal/runtime/exec/exec.go:363-374`).

### Failure cases

Generic runner failures are in the shared contract.

| Case | Exit | stdout | Source |
|---|---|---|---|
| missing name | 1 | empty; stderr `watch-startup: missing session name` | `herdrcore/reads.py:58-59`, `rpp/perop.py:147-148,166-167` |
| no record for the name | 1 | empty; stderr `no session 'X'` | `herdrcore/screen.py:30-32` |
| core unreachable on the first record read | 1 (a start is kicked) | empty; stderr `cannot read the city's core: …` | `herdrcore/reads.py:263-278` (`unreachable` kicks, `herdrcore/reads.py:91-95`) |
| parked or stopped city, nothing runs | 1 (`no session`) | empty; stderr `no session …` | `herdrcore/reads.py:274-276` returns None |
| hc screen read fails mid-stream (`HcError`: unreachable, rpc error, timeout, lost) | 1 | lines emitted so far stay; no final line; stderr `cannot read the screen: …` | `herdrcore/screen.py:36-39` |
| dead or finished box (record exists; hc read fails or returns the last screen) | 1 if the read errors; else keeps streaming and ends 0 | as above | `herdrcore/reads.py:263`; hc behaviour for finished terminals is **unverified** (the hc source is not in the clone; the fake hc `tests/fakehc/server.py:729-737` just returns screen text) |
| deadline reached (normal end) | 0 | emitted lines only; stderr none | `herdrcore/screen.py:35,44`, `rpp/cancel.py:98-103` |
| deadline gone before the handler (for example a tiny `GC_EXEC_DEADLINE_MS`) | 1, no lines | empty; stderr `no time left before the op's deadline` | `rpp/perop.py:144-145` |
| hc call has no time left | 1 | lines so far; stderr `cannot read the screen: … no time left` | `hc/client.py:368-370` |
| SIGTERM or cancel | 1 | lines so far; stderr `canceled (terminated)` (direct `Cancelled` in `cancel.pause(0.25)` or pre-handler, `rpp/perop.py:149-150`, `rpp/cancel.py:66-74`), or `cannot read the screen: ...` (`HcCancelled` caught by `except HcError` at `herdrcore/screen.py:38-39`, `hc/errors.py:107-112`) | `rpp/perop.py:50-57,149-150`, `rpp/cancel.py:66-74` (wakes within 100 ms), `herdrcore/screen.py:38-39` |
| SIGKILL (what gc actually sends) | signal-killed (Go `ExitError`; conventionally 137 in shell-derived status, not a provider wire exit) | lines so far | `internal/runtime/exec/exec.go:389-393`; doc `docs/reference/exec-session-provider.md:119-121`; gc treats signaled close, 137 and 143 as a normal close `internal/runtime/exec/exec.go:534-543` |
| stdout closed (gc stopped reading) | `emit`'s `write`/`flush` raises `BrokenPipeError`; not an `OpError`, so caught as a generic `Exception`: exit 1 | stderr `internal error: …` | `rpp/perop.py:153-155` (the `OSError` guard at `rpp/perop.py:162-165` covers only the final write) |
| cap interaction | n/a | n/a | the cap only tests `result.stdout` (`rpp/perop.py:158-159`); streamed bytes are uncapped, and the final result stdout is empty |
| unknown-op exit 2 | not used by the pack | n/a | the op is registered (`herdrcore/ops_table.py:87`) |

gc's reaction to exits: exit 2 with no event means "unsupported", and gc falls
back to `peek` (`internal/runtime/exec/exec.go:453-456`); exit 0 with no event is
also unsupported (`internal/runtime/exec/exec.go:470`); another non-zero exit
before any event makes the start fail
(`internal/runtime/exec/exec.go:464-469,508-511`); non-zero after events gives
`closeErr`, then a `peek` fallback (`internal/runtime/exec/exec.go:363-374`)
unless it reads as canceled (`internal/runtime/exec/exec.go:534-543`). A bad JSON
line gives a decode error, which fails the start if it is the first
(`internal/runtime/exec/exec.go:425-433,508-511`).

### Conflicts

- **Parity verdict: `not-implemented`** (our provider exits 2 for
  `watch-startup`, no output; parity §watch-startup "Verdict", Deviations rows
  1–3). Row 4 (readiness detection) `intentional`. Our exit 2 matches gc's
  fallback (`internal/runtime/exec/exec.go:453-456`) – the parity doc's claim is
  **confirmed** by gc v1.5.1.
- Pack vs gc – no wire conflict: the line format `{"content": …}` matches gc's
  `startupWatchEvent` (`internal/runtime/exec/exec.go:42-44` vs
  `rpp/wire.py:313-314`); the exit 0/1 mapping is consistent with gc's reading
  above.
- Deadline mismatch (minor, not a break): the pack assigns the 120 s `start`
  class, but gc's doc gives `watch-startup` no fixed deadline ("runs only while gc
  clears startup dialogs", `docs/reference/exec-session-provider.md:116-118`) and
  gc's watcher has no timeout other than ctx and close
  (`internal/runtime/exec/exec.go:389`). The `DEFAULT_DEADLINES_S` comment lists
  only 30, 120 and 4 s (`rpp/ops.py:28-30`) and does not name `watch-startup`. So
  120 s is the pack's choice (parity row 3 calls it "start class"). gc never
  relies on it, because it SIGKILLs the watcher once dialogs are handled.
- Parity wording to adjust if reused: it says herdr "ends 1 s early with exit 0" –
  true (`rpp/cancel.py:27`), and "`GC_EXEC_DEADLINE_MS` which gc 1.5.1 never
  sends" – confirmed across the pinned gc v1.5.1 tree (no occurrence in the
  pinned tree, and `exec.go:390` sets no `Env` override).
- Parity gap that this check closes: "read gc 1.5.1's caller" – done above; the
  fallback is `peek` polling plus `send-keys`
  (`internal/runtime/exec/exec.go:369-381`), only when
  `ShouldAcceptStartupDialogs` (`internal/runtime/exec/exec.go:343-346`). The
  parity doc's cited line numbers (`internal/runtime/exec/exec.go:453-456`,
  `internal/runtime/exec/exec.go:376-380`, `internal/runtime/exec/exec.go:464-469`) match v1.5.1.

### Unverified

- hc `terminal.read` behaviour on a finished or dead terminal (no hc source in the
  pin).
- Whether any real screen's JSON line exceeds gc's 64 KiB default scanner token
  (Go stdlib limit, no override in `internal/runtime/exec/exec.go`).
- Whether any inherited external wrapper sets `GC_EXEC_DEADLINE_MS` or whether
  a later gc release sends it (the pinned gc v1.5.1 tree has no occurrence and
  `internal/runtime/exec/exec.go:390` sets no `Env` override).
- Closed-pipe `emit` runtime behaviour: code inspection confirms `OSError` /
  `BrokenPipeError` in `emit` reaches the generic exit-1 `internal error` path
  (`rpp/perop.py:122-124,153-155`); no dedicated test was found (the staging test
  uses `io.BytesIO`), and partial physical-write behaviour remains `unverified`.
- Real-time end of stream (roughly 119 s by default, depending on read
  duration + 250 ms pause, or `cannot read the screen` if a read crosses the
  margin) – derived from `budget` and `pause` arithmetic, not tested beyond the
  3.5 s case.

## attach

### Registration

- `"attach": spec(attach.op_attach, deadline="start")` –
  `herdrcore/ops_table.py:88`.
- `reads_stdin=False`: `spec()` defaults `stdin=False`
  (`herdrcore/ops_table.py:64`); `attach` passes no `stdin=`. `run_per_op` reads
  stdin only when `spec.reads_stdin` (`rpp/perop.py:135`). gc's tty passthrough
  stdin is therefore inherited by the exec'd `hc`, not consumed by the pack.
- Deadline class `start` = 120 s (`rpp/ops.py:23-27`); `GC_EXEC_DEADLINE_MS`
  overrides it (`rpp/perop.py:129`, `rpp/ops.py:127`). gc v1.5.1 never sends that
  variable (`rg GC_EXEC_DEADLINE_MS` over `.go` and `.md` in the tag: no hits), so
  120 s applies. It serves as the runner/client setup budget (`hello()` is
  budgeted at `herdrcore/attach.py:119`), not a hard bound on every pre-exec
  operation (the shared attach `flock` at `herdrcore/attach.py:126` is blocking
  with no deadline or cancel poll; see "Success output").
- Stdout cap: `SMALL_STDOUT_CAP` = 64 KiB, the `spec()` default
  (`herdrcore/ops_table.py:64`, `rpp/ops.py:32`). Never reached: the success path
  returns nothing (it execs), and failures carry empty stdout.
- `housekeeping=True` default, so `lifecycle.sweep_spawn_files(ctx)` runs first
  (`herdrcore/ops_table.py:45-46`).
- Capability: `tty.attach` is not declared and is not declarable
  (`rpp/wire.py:24-39`; `handshake.py` lists no such token). gc 1.5.1 documents it
  as "Reserved" (`docs/reference/exec-session-provider.md:198`); `CanAttachTTY` is
  set from it (`internal/runtime/exec/exec.go:826`) but only combinators read it
  (`internal/runtime/auto/auto.go:431`, `internal/runtime/hybrid/hybrid.go:264`).
  gc's `Attach` never checks it (`internal/runtime/exec/exec.go:696-697`).
- Parity §attach (`docs/provider-herdr-parity.md:839`): status implemented.

### Inputs

- argv: `gc-runtime-herdr-core attach <name>`; `args[0]` = gc session name
  (`herdrcore/reads.py:57-60`, `herdrcore/attach.py:105`). gc calls `script attach
  <name>` (`internal/runtime/exec/exec.go:696-697`; doc
  `docs/reference/exec-session-provider.md:131`).
- stdin: not read by the pack; gc wires `os.Stdin/Stdout/Stderr` straight to the
  script (`internal/runtime/exec/exec.go:196-202`, `runWithTTY`).
- Environment (read from `ctx.env`): `TERM`, `COLORTERM`, `TERM_PROGRAM` and
  `TERM_PROGRAM_VERSION` are copied into the new env when non-empty
  (`herdrcore/attach.py:38`, `herdrcore/attach.py:123`). Everything else is dropped: the exec env is
  `core_env()` – `HOME`, `USER`, `LOGNAME`, `PATH` (hc dir + base), locale,
  `ctx.paths.discovery_env()` (`HC_SESSION`, `HC_SOCKET`, `XDG_STATE_HOME`,
  `XDG_CONFIG_HOME`, `hc/paths.py:82-89`), `XDG_RUNTIME_DIR` and
  `DBUS_SESSION_BUS_ADDRESS` (`herdrcore/core.py:119-149`). No `GC_*` variables
  are passed by this builder; `OpContext` uses `GC_EXEC_STATE_DIR` and
  `GC_EXEC_CITY_PATH` only to derive paths, without propagating them into `hc`'s
  environment.
- Preconditions: a live, intact record for the name (`herdrcore/reads.py:281-284`,
  `herdrcore/records.py:53-62`); an executable absolute-path `hc`
  (`herdrcore/config.py:133-156`); the city's core answers `hello()`
  (`herdrcore/attach.py:119`). A tty is not checked (`rg isatty|tty` over the
  pack: only `rpp/wire.py` comments).

### Success output

- Success never returns to Python: `os.execve(hc_bin, [hc_bin, "attach",
  record.address], env)` (`herdrcore/attach.py:128`) replaces the process. It
  blocks as the viewer for as long as `hc attach` runs, so the op does not "spawn
  a viewer and return".
- Before exec: a shared `flock` on the incarnation's attach lock
  (`herdrcore/attach.py:124-126`, path `herdrcore/context.py:147-163`), made
  inheritable (`herdrcore/attach.py:127`) so the kernel holds it until `hc` exits.
  That lock is what `is-attached` tests (`herdrcore/attach.py:42-56`, `herdrcore/attach.py:88`).
  Skipped when the record has no `gc.obj` label (`herdrcore/attach.py:109`,
  `herdrcore/attach.py:124`); then `is-attached` falls back to the process-snapshot match on `hc
  attach <address>` (`herdrcore/attach.py:59-90`).
- stdout, stderr and exit: the pack writes nothing. Bytes and exit code are
  whatever `hc attach` produces; the pack never maps them. Exit 0 = `hc attach`
  exited 0 (detach). gc turns any non-zero exit into `*exec.ExitError` from
  `cmd.Run()` (`internal/runtime/exec/exec.go:201`), surfaced as `gc session
  attach: exit status N` / `gc session new: attaching: ...`
  (`cmd/gc/cmd_session.go:398-400`, `cmd/gc/cmd_session.go:507`, `cmd/gc/cmd_session.go:1553`). Exit 2 is not given gc's
  "unknown op = success" mapping on this path (`runWithTTY` bypasses `run`; parity
  `docs/provider-herdr-parity.md:856`).
- No-TTY answer: the pack has none. It does not test for a tty and does not print
  "not supported". What `hc attach` does with stdin or stdout that is not a tty is
  `unverified` (the `hc` source is not in either tree). The fake `hc` ignores tty
  entirely: it checks `terminal.get`, then blocks until SIGTERM and exits 0
  (`tests/fakehc/cli.py:257-267`).
- Deadline after exec: nothing enforces it. The 120 s class is checked only before
  the handler (`rpp/perop.py:144`) and inside hc calls made pre-exec
  (`herdrcore/attach.py:119`, `herdrcore/context.py:122-134`). The only timers in
  the pack are in `herdrcore/doctor.py:526` (not on this path). gc applies none
  either (`internal/runtime/exec/exec.go:196-202` has no context; doc
  `docs/reference/exec-session-provider.md:116-117`: `attach` "none").
- Signals: pre-exec, SIGTERM sets the cancel token (`rpp/perop.py:50-57`); after
  exec the handler is gone and SIGTERM takes `hc`'s own disposition (standard exec
  semantics; `unverified` by test – `tests/unit/test_herdrcore_core.py:264-265`
  terminates the viewer but asserts no code).

### Failure cases

All stdout empty; stderr = `attach: <message>` via `rpp/perop.py:166-167`,
redacted. Generic runner failures are in the shared contract.

| Case | Exit | stdout | stderr / message | Source |
|---|---|---|---|---|
| no session arg | 1 | empty | `missing session name` | `herdrcore/reads.py:58-59`, `rpp/perop.py:147-148` |
| unknown, dead or not-intact name | 1 | empty | `no running session 'x'` | `herdrcore/attach.py:106-108`, `herdrcore/records.py:53-62`; test `tests/unit/test_herdrcore_core.py:261` (`attach nobody` gives 1) |
| core down at the first read | 1 | empty | `cannot read the city's core: ...`; also kicks a pack start (`herdrcore/reads.py:91-95`, `herdrcore/reads.py:277-278`). If parked with nothing running it reads as no record, so the row above | `herdrcore/reads.py:263-278` |
| core stops answering between the read and `hello()` | 1 | empty | `the city's core is down (...); gc restarts it` | `herdrcore/attach.py:118-121` |
| core down, test | 1 | empty | stderr contains `core` | `tests/unit/test_herdrcore_core.py:377-388` |
| `hc` missing, unpinned or relative path | 1 | empty | `hc not installed: ...` / `hc_bin ... names an unpinned hc ...` | `herdrcore/attach.py:110-113`, `herdrcore/config.py:143-155` |
| state dir, config, meta or lock-file error | 1 | empty | the error text | `herdrcore/ops_table.py:48-49` |
| lock file open fails (`OSError`) | 1 | empty | `internal error: <Type>: ...` (not caught in `op_attach`) | `herdrcore/attach.py:125`, `rpp/perop.py:153-154` |
| `execve` fails (`OSError`) | 1 | empty | `internal error: ...`; the guard `exec of hc attach returned` (`herdrcore/attach.py:129`) is unreachable in practice | `herdrcore/attach.py:128-129`, `rpp/perop.py:153-154` |
| deadline already spent at entry | 1 | empty | `no time left before the op's deadline` | `rpp/perop.py:144-145` |
| `hello()` times out or is cancelled pre-exec | 1 | empty | `the city's core is down (...)` (including `HcTimeout` and `HcCancelled`, caught by `except HcError` at `herdrcore/attach.py:119-121`) / `canceled (terminated)` (direct `Cancelled` pre-handler, `rpp/perop.py:149-150`) | `herdrcore/attach.py:119-121`, `rpp/perop.py:149-150`, `hc/errors.py:107-112` |
| no TTY | not checked by the pack | whatever `hc attach` does; `unverified` | whatever `hc attach` does; `unverified` | see "Success output" |
| `hc attach` fails after exec (box exits, server error) | hc's own code | hc's own output | hc's own output; gc reports `exit status N` | `herdrcore/attach.py:128`, `internal/runtime/exec/exec.go:201` |
| stdout cap | n/a | no stdout is ever returned | – | `rpp/perop.py:158-159` |
| op absent / exit 2 | n/a: the op is registered, so the pack never answers 2 itself (`rpp/perop.py:117-120` applies only to missing ops) | – | – | `herdrcore/ops_table.py:88` |

### Conflicts

Pack vs gc v1.5.1; parity verdict.

- Parity §attach (`docs/provider-herdr-parity.md:839-856`): status implemented;
  herdr-core deviation rows 1–4 (`docs/provider-herdr-parity.md:851-854`) all
  `intentional`; follow-up `GCREX-19.17`
  (`docs/provider-herdr-parity.md:454`, `docs/provider-herdr-parity.md:311`). Its herdr-core paragraph
  (`docs/provider-herdr-parity.md:845`) matches the source read; each cite above
  was re-checked and no disagreement found. The "exit-2 failure" verdict
  (`docs/provider-herdr-parity.md:856`) is about gc's reading of an *absent* op:
  `runWithTTY` never maps exit 2, so an unregistered `attach` shows as `exit
  status 2` (`internal/runtime/exec/exec.go:196-202`; `cmd/gc/cmd_session.go:399,508,1554`). herdr-core is not affected because it registers the op; the
  Rex hint (exit 0) is the sibling fix (GCREX-20).
- **Pack-vs-gc deadline difference** (declared intentional): the gc doc says
  `attach` has no deadline (`docs/reference/exec-session-provider.md:116-117`);
  the pack registers `start` = 120 s (`herdrcore/ops_table.py:88`). The pack's
  budget covers only pre-exec runner/client setup (`hello()` at
  `herdrcore/attach.py:119`), not a hard bound on the blocking `flock`
  (`herdrcore/attach.py:126`), and gc sends no deadline var, so there is no
  live clash. Parity row 4 (`docs/provider-herdr-parity.md:854`) agrees.
- **Capability context** (not a pack-vs-gc conflict): the pack never declares
  `tty.attach` although it implements a tty-passthrough `attach`; gc 1.5.1
  treats the token as reserved and does not require it to call `Attach`
  (`internal/runtime/protocol.go:44-47`, `internal/runtime/exec/exec.go:696-697`).
  Parity row 2 (`docs/provider-herdr-parity.md:852`).
- Parity row 1 contrast: Rex = hint + exit 0, herdr-core = real passthrough. gc's
  contract only requires "tty passthrough"
  (`docs/reference/exec-session-provider.md:131`), which herdr-core meets via exec.
- Exec / `seamProvider` path: gc runs `is-running` (`place.IsRunning`) before
  `attach` (`internal/runtime/exec/seams.go:160-167`,
  `internal/runtime/seam_adapter.go:139-149`); a dead box gives
  `ErrSessionNotFound` in gc before the script runs, so the pack's `no running
  session` is a second guard.

### Unverified

- `hc attach` behaviour: the no-TTY answer, raw mode, SIGWINCH, the detach key,
  and exit codes (no hc source in either tree; parity
  `docs/provider-herdr-parity.md:845` also marks it unverified).
- Whether SIGTERM after exec kills `hc` cleanly (no test asserts it).
- Which of the two "core down" messages appears in practice when the core is down
  at op start (the code order says the first: `herdrcore/reads.py:277-278`).
- The `GC_EXEC_DEADLINE_MS` claim is a negative grep over `.go` and `.md` in the
  tag; other file types were not searched.

## copy-to

### Registration

- `"copy-to": spec(screen.op_copy_to)` – `herdrcore/ops_table.py:89`.
- `spec()` defaults: `stdin=False`, `deadline="default"`, `cap=SMALL_STDOUT_CAP`,
  `housekeeping=True` – `herdrcore/ops_table.py:64-65`. So `reads_stdin=False`,
  deadline class `default`, cap 64 KiB (`rpp/ops.py:32`), and the spawn-file sweep
  runs first (`herdrcore/ops_table.py:45-46`).
- The default deadline is 30 s (`rpp/ops.py:23-27`) unless `GC_EXEC_DEADLINE_MS` is
  set (`rpp/perop.py:31`, `rpp/perop.py:129`). gc v1.5.1 never sets it: `rg GC_EXEC_DEADLINE`
  over the v1.5.1 tree has no match. gc's own timeout for a non-start op is 30 s
  (`internal/runtime/exec/exec.go:56`, `internal/runtime/exec/exec.go:63`).
- stdin is not read (`rpp/perop.py:135`, spec `reads_stdin=False`).
- The handshake does not declare `copy-to`; it appears nowhere in `handshake.py`
  or `rpp/wire.py` (`rg copy` hits only `copy_files`). Parity §copy-to agrees:
  "not a declared capability" (`docs/provider-herdr-parity.md:1083`).
- Parity §copy-to verdict: `not-implemented` (for our Rex provider;
  `docs/provider-herdr-parity.md:1079`).

### Inputs

- argv: `copy-to <name> <src> [rel-dst]` – `herdrcore/screen.py:48-52`. Fewer than
  2 args gives `OpError("usage: copy-to <name> <src> [rel-dst]")`
  (`herdrcore/screen.py:49-50`). `rel-dst` defaults to `""`
  (`herdrcore/screen.py:52`). Extra args are ignored (unverified by test).
- gc always sends 3 args: `p.run(nil, "copy-to", name, src, relDst)`
  (`internal/runtime/exec/exec.go:795-797`). `relDst` may be `""`.
- Preconditions:
  - A live, intact record for `name`: `live_record`
    (`herdrcore/reads.py:281-284`) and `is_intact`
    (`herdrcore/records.py:53-62`).
  - The work dir is `meta.work_dir`, else `record.cwd`, and must be absolute
    (`herdrcore/screen.py:56-59`).
  - `src` is a file or directory. A missing `src` is a silent no-op
    (`herdrcore/staging.py:221-222`).
- Behaviour (`stage_path`, `herdrcore/staging.py:216-235`):
  - `rel` is resolved below the work dir, and an escape is refused (`_parts`,
    `herdrcore/staging.py:75-81`; `herdrcore/staging.py:223-225`).
  - A directory `src` merges into the destination (`herdrcore/staging.py:232-233`).
  - A file `src` lands at `rel`, or inside `rel` if that is a directory, or at the
    work dir root plus the basename if `rel` is empty
    (`herdrcore/staging.py:226-228`).
  - A `src` that is already the destination is skipped
    (`herdrcore/staging.py:230-231`).
  - Writes never go through a symlink (`_open_dir`,
    `herdrcore/staging.py:84-105`; `_copy_file`, `herdrcore/staging.py:108-135`).
    Missing parent dirs are created at mode 0755 (`herdrcore/staging.py:91`).
  - A non-regular file `src` fails (`herdrcore/staging.py:113-114`). A non-regular
    existing destination fails (`herdrcore/staging.py:121-122`).

### Success output

- stdout: empty. Exit code 0. `return OpResult.ok()` (`herdrcore/screen.py:64`;
  `rpp/ops.py:69-70`). Nothing is written to stderr.
- A missing `src` also gives exit 0 with empty stdout
  (`herdrcore/staging.py:221-222`).
- Can gc v1.5.1 call it? It is an extension, not RPP v0.
  - The exec provider has `CopyTo` (`internal/runtime/exec/exec.go:793-797`) and
    `execPlace.Stage` loops over it (`internal/runtime/exec/seams.go:122-127`).
    `seamProvider.CopyTo` is the only route to `Stage`
    (`internal/runtime/seam_adapter.go:219-225`).
  - `docs/reference/exec-session-provider.md` has no `copy-to` (`rg copy-to docs`
    has no hits).
  - Non-test callers of `runtime.Provider.CopyTo` in v1.5.1 are only pass-through
    wrappers (`internal/runtime/hybrid/hybrid.go:240`,
    `internal/runtime/auto/auto.go:407`, `cmd/gc/status_provider.go:216`) and the
    tmux provider's own use (`internal/runtime/tmux/adapter.go:593`). No controller
    call that reaches exec was found (`rg '\.CopyTo\('` excluding tests and
    `internal/runtime/exec/seams.go`); this does not rule out an indirect caller (unverified).
  - Parity: "no gc 1.5.1 caller; staging is `copy_files` in start" (parity doc table
    row `copy-to` at `docs/provider-herdr-parity.md:209`, Verdict at
    `docs/provider-herdr-parity.md:1079`). Staging goes in the start JSON
    `copy_files` (`internal/runtime/exec/json.go:41`).

### Failure cases

`rpp/perop.py:167` prefixes the op name to stderr: `copy-to: <message>`. For
staging errors this gives a doubled prefix, `copy-to: copy-to: <reason>`, from
`herdrcore/screen.py:63` plus `rpp/perop.py:167`. The pack test only checks
`"copy-to:" in stderr` (`tests/unit/test_herdrcore_staging.py:275`). stdout is
empty on every failure (`OpResult.fail`, `rpp/ops.py:77-79`). Generic runner
failures are in the shared contract.

| Case | Exit | stdout | Source |
|---|---|---|---|
| fewer than 2 args | 1 | empty; stderr `copy-to: usage: copy-to <name> <src> [rel-dst]` | `herdrcore/screen.py:49-50`, `rpp/perop.py:147-148` |
| no record, dead or broken box, or not intact | 1 | empty; stderr `copy-to: no running session '<name>'` | `herdrcore/screen.py:53-55`, `herdrcore/reads.py:281-284`, `herdrcore/records.py:53-62` |
| destination leaves the work dir (`../x`) | 1 | empty; stderr `copy-to: copy-to: the destination leaves the work directory` | `herdrcore/staging.py:223-225`, `herdrcore/screen.py:62-63`; test `tests/unit/test_herdrcore_staging.py:242-243` |
| write through a symlink, non-regular src or dst, OS error | 1 | empty; stderr `copy-to: copy-to: <reason>` | `herdrcore/staging.py:97-105`, `herdrcore/staging.py:113-122`, `herdrcore/screen.py:62-63`; test `tests/unit/test_herdrcore_staging.py:274-275` |
| work dir not absolute | 1 | empty; stderr `copy-to: '<name>' has no absolute work directory` | `herdrcore/screen.py:58-59` |
| hc core unreadable or down (`HcError` in `current_record`) | 1 | empty; stderr `copy-to: cannot read the city's core: <err>`. A kick is also started if the core is unreachable. | `herdrcore/reads.py:272-278` |
| metadata unreadable | not a failure: falls back to `record.cwd` | – | `herdrcore/reads.py:80-88`, `herdrcore/screen.py:56-57` |
| state dir, config, meta or lock-file error | 1 | empty; stderr `copy-to: <err>`; a lock timeout is `busy: <err>` | `herdrcore/ops_table.py:48-51` |
| deadline already expired before the handler | 1 | empty; stderr `copy-to: no time left before the op's deadline` | `rpp/perop.py:144-145` |
| deadline expires or SIGTERM during the copy | not settled: `Cancelled` gives exit 1. `stage_path` never checks the token, so the copy may run on. | – | `rpp/perop.py:149-150`; unverified |
| unexpected exception | 1 | empty; stderr `copy-to: internal error: <Type>: <msg>` | `rpp/perop.py:153-154` |
| stdout over cap (64 KiB) | cannot happen: stdout is always empty | – | `rpp/perop.py:158-159` |
| unknown op (a build without `copy-to`) | 2 | empty; stderr `copy-to: unknown operation` | `rpp/perop.py:117-120` |

gc side of failures:

- Exit 2 is success for every op except `start` and `provision`
  (`internal/runtime/exec/exec.go:159-161`, `internal/runtime/exec/exec.go:174-181`; doc
  `docs/reference/exec-session-provider.md:77`, `docs/reference/exec-session-provider.md:79-85`).
- Any other non-zero exit returns `exec provider <script> copy-to <args>:
  <stderr>` (`internal/runtime/exec/exec.go:162-169`). `execPlace.Stage` returns
  that error and aborts the batch (`internal/runtime/exec/seams.go:122-127`).
- The doc comments (`internal/runtime/exec/exec.go:794`,
  `internal/runtime/runtime.go:230`) say "best-effort: returns nil", but the code
  returns the error. `seamProvider.CopyTo` swallows only an unknown session
  (`internal/runtime/seam_adapter.go:221-224`).

### Conflicts

- Parity verdict: `not-implemented` for our Rex provider
  (`docs/provider-herdr-parity.md:1079`). `copy-to` is outside the 17 RPP v0 rows,
  and the pack side is correct as the parity doc describes it
  (`herdrcore/screen.py:47-64`, cap and deadline default). The parity doc's claims
  were re-checked against the pinned sources and no disagreement was found on the
  facts it states.
- Our provider returns exit 2, which gc treats as a successful no-op
  (`internal/runtime/exec/exec.go:159-161`). That is safe only because gc 1.5.1 has
  no caller in the exec path (see "Success output").
- Doc-comment vs behaviour discrepancy in gc: "Best-effort: returns nil on error"
  (`internal/runtime/exec/exec.go:794`, `internal/runtime/runtime.go:230`) vs
  `exec.Provider.CopyTo` at `internal/runtime/exec/exec.go:795-797`, which
  returns the error from `p.run` unless the exit is 2
  (`internal/runtime/exec/exec.go:159-169`). The pack itself exits 1 on real
  failures.
- Minor pack quirk: the duplicated `copy-to: copy-to:` prefix on staging errors
  (`herdrcore/screen.py:63` plus `rpp/perop.py:167`). No effect on gc, which embeds
  stderr verbatim.
- Missing-source compatibility and dead-session difference:
  `internal/runtime/runtimetest/conformance.go:586` expects a missing `src` to be
  a no-op on a running session, and the pack agrees
  (`herdrcore/staging.py:221-222`). Separately, on a dead session (which that
  conformance test does not settle), a direct script call exits 1 for a dead
  box (`herdrcore/screen.py:53-55`), while gc's `seamProvider.CopyTo` returns
  nil when `attach` finds no live session
  (`internal/runtime/seam_adapter.go:221-224`).

### Unverified

- Whether any gc v1.5.1 controller path calls `Provider.CopyTo` on an exec
  provider. Only wrapper and test callers were found.
- Cancellation behaviour mid-copy: `stage_path` has no cancel or deadline checks
  (`herdrcore/staging.py:216-235`), but the cancel token was not traced across
  threads.
- Whether any caller sends extra argv beyond 3 (`herdrcore/screen.py:48-52`
  ignores extras by construction; no test).
- Behaviour with a `src` symlink: `os.path.exists` and `os.stat` follow it, so a
  symlink to a file is copied as a regular file. Inferred from
  `herdrcore/staging.py:113`, no test.
- Not run: no tests were executed. Corroboration is by reading
  `tests/unit/test_herdrcore_staging.py:234-277`.

## server-stop

### Registration

- `"server-stop": spec(server.op_server_stop)` at `herdrcore/ops_table.py:90`;
  handler `op_server_stop` at `herdrcore/server.py:88`.
- `reads_stdin=False`: `spec()` defaults `stdin=False`
  (`herdrcore/ops_table.py:64`), and the runner reads stdin only when
  `spec.reads_stdin` (`rpp/perop.py:135`). stdin is never read.
- Deadline class `default` = 30 s (`herdrcore/ops_table.py:64`,
  `rpp/ops.py:23-27`), counted from `_STARTED` (`bin/gc-runtime-herdr-core:12`,
  `rpp/ops.py:128`);
  `GC_EXEC_DEADLINE_MS` overrides (`rpp/perop.py:31,129`). Inner waits are cut to
  end 1.0 s before the deadline (`rpp/cancel.py:27,98-103`).
- Stdout cap `SMALL_STDOUT_CAP` = 64 KiB (`herdrcore/ops_table.py:64`,
  `rpp/ops.py:32`); moot, stdout is always empty.
- Capability: `server-lifecycle`, declared in the handshake (`handshake.py:40`;
  `herdrcore/ops_table.py:5-6`; test
  `tests/unit/test_herdr_core_entrypoint.py:75`). Housekeeping on
  (`sweep_spawn_files` first, `herdrcore/ops_table.py:45-46`).
- Parity §server-stop (`docs/provider-herdr-parity.md:1038`): verdict
  **intentional**, status `not implemented` (Rex side).

**gc v1.5.1 has no `server-stop` wire op.**

- gc's exec provider never sends it: `Stop` sends only `stop <name>`
  (`internal/runtime/exec/exec.go:581-583`); the only handshake capabilities gc
  knows are `report-attachment`, `report-activity`, `proc.exec`, `proc.stream`,
  `tty.attach`, `proc.provision` (`internal/runtime/protocol.go:21-55`).
  `server-lifecycle` and `server-stop` appear nowhere in gc's exec code or
  `docs/reference/exec-session-provider.md` (`rg` over the tree; capability table
  at `docs/reference/exec-session-provider.md:189-196`).
- gc has server teardown only as the optional Go interface
  `runtime.ServerLifecycleProvider` (`internal/runtime/runtime.go:533-547`),
  implemented by tmux and the native herdr provider
  (`internal/runtime/tmux/adapter.go:47`,
  `internal/runtime/herdr/provider.go:49,86`), not by the exec provider. `gc stop`
  calls it by type assertion and skips providers without it
  (`cmd/gc/cmd_stop.go:464-473`).
- So the pack's `server-lifecycle` token is an unknown capability, which gc ignores
  (`internal/runtime/protocol.go:15-16`;
  `docs/reference/exec-session-provider.md:183`;
  `internal/runtime/REQUIREMENTS.md:131` RUNTIME-RPP-008). The parity §server-stop
  verdict agrees: "there is no wire op at all", `server-lifecycle` not in the
  baseline.

### Inputs

- argv: `<script> server-stop`. No flags, no positional args: `op_server_stop`
  never reads `ctx.request.args` (`herdrcore/server.py:88-109`). Extra args are
  ignored (`rpp/perop.py:134` only stores them). gc would send none (it never
  sends the op, see above).
- Scope comes from the environment, not argv: city = `GC_EXEC_CITY_PATH`
  (`rpp/statedir.py:39`), hashed with machine identity into `city_id`
  (`hc/paths.py:118-119`; `rpp/statedir.py:55-63`). Unset or empty = bucket
  `unscoped` (`rpp/statedir.py:42,55-58`), so there is no "unknown scope" error.
- Other env: `GC_EXEC_STATE_DIR` (state root, `rpp/statedir.py:38`,
  `hc/paths.py:120`); `GC_RPP_FAULT` (`herdrcore/context.py:25,39-42`):
  `unavailable` = backend down, others inject hc call faults;
  `GC_EXEC_DEADLINE_MS` (`rpp/perop.py:31`).
- Preconditions: none. A core need not run, no record need exist (tests
  `tests/unit/test_herdrcore_server.py:171-185`).
- Locks: an exclusive `kick` lock for the whole op, waiting `max(0, budget(16 s))`
  (`herdrcore/server.py:82,96`; `herdrcore/core.py:67,74`). A per-name exclusive
  lock per session (`herdrcore/server.py:207`; `herdrcore/context.py:152-159`).

### Success output

- Exit 0, stdout empty (`OpResult(ExitCode.OK, b"", ...)`,
  `herdrcore/server.py:109`). stderr empty unless a warning: `server-stop:
  <warning>` when the "stopped" mark cannot be written
  (`herdrcore/server.py:179-183`; `rpp/perop.py:166-167`).
- Core answering, city sessions ended, no foreign terminals: sessions ended per
  name (`herdrcore/server.py:123-124,204-223`), city parked, `server.stop`, wait
  for the core to be gone, sweep (`herdrcore/server.py:133-138`). Test
  `tests/unit/test_herdrcore_server.py:150-168` (code 0, stderr "", one
  `server.stop`, socket gone, meta dir empty).
- Nothing running: core down and no pack metadata gives park + sweep, exit 0, no
  core started (`herdrcore/server.py:165-166`; tests
  `tests/unit/test_herdrcore_server.py:171-185`). A second call also exits 0
  (`tests/unit/test_herdrcore_server.py:171-178`).
- Foreign terminal live: city sessions ended, core left running, not parked,
  exit 0 (`herdrcore/server.py:131-132`; test
  `tests/unit/test_herdrcore_server.py:204-220`, `tests/unit/test_herdrcore_server.py:541`). Parity §server-stop
  agrees.
- Core down but the pack keeps metadata: starts the core to remove hc's kept
  records, then stops it, exit 0 (`herdrcore/server.py:156-159`; test
  `tests/unit/test_herdrcore_server.py:453`).

### Failure cases

All failures: stdout empty, stderr `server-stop: <message>` (redacted,
`rpp/perop.py:166-167`); the exit is always 1 (`OpResult.fail` default,
`rpp/ops.py:78-79`). The op never returns 2, 66, 73, 75 or 80. Generic runner
failures are in the shared contract.

| Case | Exit | stdout | Source |
|---|---|---|---|
| `GC_RPP_FAULT=unavailable` | 1 `the backend is unavailable (...): the city's sessions may survive` | empty | `herdrcore/server.py:89-90`; test `tests/unit/test_herdrcore_server.py:417-421` |
| kick lock held past the wait (a core start in flight) | 1 `a start of the city's core is in flight: ...` | empty | `herdrcore/server.py:104-106`; test `tests/unit/test_herdrcore_server.py:966-977` |
| lock-file error | 1 same text plus `(err)` | empty | `herdrcore/server.py:104-106` |
| session still alive after kill (descendant survives, unreadable metadata, `still listed`) | 1 `the city's sessions may survive: 'name': ...`; the core is **not** stopped, the record is kept for retry | empty | `herdrcore/server.py:107-108,125-126,130,204-223`; tests `tests/unit/test_herdrcore_server.py:258-279`, `tests/unit/test_herdrcore_server.py:630` and following |
| core `server.stop` done but the core still answers or runs after 10 s | 1 `the city's core was stopped but still answers/still runs: pids` | empty | `herdrcore/server.py:79,226-248` |
| core down, listener on another socket (`core.lock`) | 1, and unparked, the sweep still runs | empty | `herdrcore/server.py:152-155`; `herdrcore/core.py:264-272`; test `tests/unit/test_herdrcore_server.py:688` |
| core down, cannot start it to remove kept records | 1 `hc may keep the city's records, and its core cannot be started ...`, unparked | empty | `herdrcore/server.py:160-164`; test `tests/unit/test_herdrcore_server.py:481` |
| core process runs but does not answer | 1 `the city's core runs but does not answer: pids` | empty | `herdrcore/server.py:420-422`; test `tests/unit/test_herdrcore_server.py:401-414` |
| process table unreadable | 1 `cannot read the process table (...)` | empty | `herdrcore/server.py:352-354,393-395`; test `tests/unit/test_herdrcore_server.py:378-384` |
| program outlives kill, fenced infrastructure program, or holder with an unreadable or blind program | 1 | empty | `herdrcore/server.py:371-378,408-419`; tests `tests/unit/test_herdrcore_server.py:387`, `tests/unit/test_herdrcore_server.py:666`, `tests/unit/test_herdrcore_server.py:678`, `tests/unit/test_herdrcore_server.py:944` |
| other hc error (not unreachable) | 1 `the city's core does not answer as it should (err)` | empty | `herdrcore/server.py:101-102` |
| state dir, config or metastore error | 1 | empty | `herdrcore/ops_table.py:48-49` |
| core stays up for anything still running at the end | adds `unpark` problems (exit 1 only if clearing the mark fails) | empty | `herdrcore/server.py:103,186-201` |
| gc cancel (SIGTERM) | 1 `canceled (terminated)` on direct `Cancelled` (pre-handler, `cancel.pause` in `_await_core_gone`, or re-raised from `_stop_core_down` at `herdrcore/server.py:160-162`), or `the city's sessions may survive: the city's core does not answer as it should (...)` when `HcCancelled` from `_stop_with_core` is caught by `except HcError` (`herdrcore/server.py:101-102`, `hc/errors.py:107-112`); inside `_await_core_gone` (`herdrcore/server.py:235-236`), `except HcError` also treats `HcCancelled` on `probe.hello()` as `answering = True` until `cancel.pause` checks cancellation | empty | `rpp/perop.py:50-57,149-150`; `herdrcore/server.py:101-102,160-162,235-236`; a partial stop is possible |
| no time left at entry | 1 `no time left before the op's deadline` | empty | `rpp/perop.py:143-145` |
| deadline reached mid-op | no hard stop; waits are budgeted to the deadline minus 1 s (`herdrcore/server.py:96,228`; `herdrcore/core.py:279,293`); gc kills at 30 s; later reads can answer unknown and kick infrastructure despite the mark when a holder or core still runs (`herdrcore/core.py:33-40`, `herdrcore/reads.py:91-95`), without clearing the mark (explicit removal is `unpark` at `herdrcore/core.py:323-329`, called by `ensure_core` and `server-stop` cleanup) | empty | `unverified` as to the exact end state |
| unexpected exception | 1 `internal error: <Type>: <msg>` | empty | `rpp/perop.py:153-155` |
| unknown/unset scope | not an error: `GC_EXEC_CITY_PATH` unset or empty selects the shared `unscoped` bucket (`rpp/statedir.py:55-58`) and runs normal `server-stop` logic against it (including teardown, foreign-terminal checks, or failures); only an empty successful teardown gives exit 0 (`herdrcore/server.py:165-166`) | empty | `rpp/statedir.py:55-58`; `herdrcore/server.py:165-166` |

### Conflicts

1. **gc never sends the op.** The pack docstring says "gc runs it after its own
   stops, as a backstop" (`herdrcore/server.py:4`); a test docstring says "`gc
   stop` runs server-stop" (`tests/unit/test_herdrcore_doctor.py:280`). In gc
   v1.5.1 the exec provider has no such call and no `ServerLifecycleProvider` (see
   "Registration"). Reported as a conflict: under the pin, this op fires only by
   hand or by tests. A later gc may send it: `unverified`.
2. **`server-lifecycle` is not a known gc capability at v1.5.1**; it is ignored,
   not rejected (`internal/runtime/protocol.go:15-16`). The parity doc says the
   same.
3. The same applies to the other tokens in the pack handshake (`observe.v1`,
   `city-scoped`, `conformance-faults`, `carrier.ops`, `idle-boundary`,
   `input-outcome`, `fs.host`, `handshake.py:35-44`); they are outside this op.
4. **Parity verdict: `intentional`** (the Rex provider omits the op; herdr-core's
   core is per-city so its stop is scoped, Rex's server is shared). Re-checked
   against both sources: the parity line refs `herdrcore/server.py:88-109`,
   `herdrcore/server.py:116-138`, `herdrcore/server.py:204-223`, `herdrcore/server.py:226-248` and `herdrcore/ops_table.py:90` match the
   pin; one parity-record discrepancy is that `docs/provider-herdr-parity.md`
   §server-stop Verdict mentions only tmux as implementing
   `ServerLifecycleProvider`, whereas gc v1.5.1 also implements it on the
   native herdr provider (`internal/runtime/herdr/provider.go:49,86`). One
   parity wording to note: "refuses when the core still answers after stop"
   corresponds to `_await_core_gone` (`herdrcore/server.py:226-248`).
5. If a gc exit 2 or another non-zero came back: moot for this op, since gc does
   not call it. For reference, exit 2 is success for non-start ops
   (`internal/runtime/exec/exec.go:159-161,174-180`).

### Unverified

1. Whether any gc release after v1.5.1 sends `server-stop` or knows
   `server-lifecycle` (live trees not read, per the pins).
2. The end state when the 30 s deadline expires mid-op without a cancel; the code
   budgets waits but it was not run (`herdrcore/server.py:96,228`).
3. `hc` `server.stop` semantic details (timeout 10 s, `hc/client.py:135,206-208`)
   are the pack's own; the hc source was not read.
4. Whether gc's `exec` provider is wrapped by something that exposes
   `ServerLifecycleProvider` for exec packs at v1.5.1: `rg` found only the tmux and
   herdr providers.

## x-kick-core

### Registration

- Registered as `KICK_OP: spec(_kick, housekeeping=False)`
  (`herdrcore/ops_table.py:91`). `KICK_OP = "x-kick-core"`
  (`herdrcore/core.py:73`). An internal pack op, not gc-callable (see "Conflicts").
- `spec()` defaults apply (`herdrcore/ops_table.py:64`): `reads_stdin=False`,
  `deadline="default"`, `stdout_cap=SMALL_STDOUT_CAP`. The default class = 30 s
  (`rpp/ops.py:23-27`). The cap = 64 KiB (`rpp/ops.py:32`). stdout is always
  empty, so the cap cannot trigger.
- Handler: `_kick` calls `run_kick(ctx)` then returns `OpResult.ok()`
  (`herdrcore/ops_table.py:56-58`).
- `housekeeping=False` makes `_wrap` skip `lifecycle.sweep_spawn_files(ctx)`
  (`herdrcore/ops_table.py:45-46`). Reason: the source states no reason for this
  op. Inference (unverified): the kick is a background start and should not do the
  per-op sweep.
- `_wrap` still builds `OpContext` and maps `StateDirError`, `ConfigError`,
  `MetaError` and `LockFileError` to `OpError` (exit 1), and `LockTimeout` to
  `OpError("busy: ...")` (`herdrcore/ops_table.py:44,48-51`).

### Inputs

- Argv: `gc-runtime-herdr-core x-kick-core`, no further args. `main` sends any
  other first word to `run_per_op` (`main.py:63-72`). The op name is `argv[1]`
  (`rpp/perop.py:116`).
- stdin is not read (`reads_stdin=False`; `rpp/perop.py:135` gives `b""`).
- The sole spawn site: `kick()` (`herdrcore/core.py:407-429`):
  `Popen([ctx.host.python, "-I", "-S", entry, KICK_OP], env=env, cwd="/",
  stdin/stdout/stderr=DEVNULL, start_new_session=True)`
  (`herdrcore/core.py:420-428`). `entry` = `<pack_dir>/assets/bin/gc-runtime-herdr-core`
  (`herdrcore/core.py:418`). `env` = the read op's env minus
  `GC_EXEC_DEADLINE_MS` (`herdrcore/core.py:419`), so the kicked op gets the 30 s
  default deadline, not gc's remaining time (`rpp/perop.py:129`).
- `kick()` runs from `reads.unreachable` (`herdrcore/reads.py:91-95`) on
  `HcUnreachable`. Callers of `unreachable`: `herdrcore/reads.py:146,204,277,314`,
  `herdrcore/lifecycle.py:520,553`, `herdrcore/inputs.py:221`.
- Preconditions checked by `kick()` before the spawn (else no process): not
  `backend_unavailable` (`GC_RPP_FAULT=unavailable`), `kick_due`, `hc_binary()`
  resolves, and the kick lock is free (`herdrcore/core.py:410-417`). `run_kick`
  re-checks under the lock.

### Success output

- stdout empty, exit 0, stderr empty (`_kick` gives `OpResult.ok()`: code OK,
  stdout `b""`, message `""`; `rpp/ops.py:68-70`, `rpp/perop.py:160-170`).
- Exit 0 covers four paths:
  - core started: writes kick.json `{ok:true, detail:"started"}`
    (`herdrcore/core.py:451`).
  - core already answers: `{ok:true, detail:"already running"}`
    (`herdrcore/core.py:441-444`).
  - kick not due: returns, writes nothing (`herdrcore/core.py:436-437`).
  - another kick holds the lock: returns, writes nothing
    (`herdrcore/core.py:452-453`).
- Nothing waits for the exit code: `kick()` discards the `Popen` and does not wait
  (`herdrcore/core.py:420-429`). The only durable result is kick.json
  (`herdrcore/core.py:389-391`, in `ctx.pack_state`).

### Failure cases

All stdout empty. stderr from a child spawned by `kick()` goes to DEVNULL
(`herdrcore/core.py:426`), so the message is lost unless the op is run by hand.
Unhandled exceptions become exit 1 `internal error: <Type>: <msg>`
(`rpp/perop.py:153-154`). The column "kick.json written" is folded into the
stdout column below. Generic runner failures are in the shared contract.

| Case | Exit | stdout | Source |
|---|---|---|---|
| city parked and `nothing_runs` | 0 | empty; kick.json not written | `kick_due` false (`herdrcore/core.py:401-402`), then `run_kick` returns (`herdrcore/core.py:436-437`) |
| city parked but a holder or core still runs | proceeds as normal | empty; kick.json per outcome | `herdrcore/core.py:394-402` (the docstring explains why) |
| last kick failed less than 5 min ago (`KICK_BACKOFF_MS`) | 0 | empty; kick.json not written | `herdrcore/core.py:72,403-404,436-437` |
| kick lock held by another kick or `server-stop` | 0 | empty; kick.json not written | `ctx.lock(..., wait_s=0.0)` raises `LockTimeout`, caught (`herdrcore/core.py:435,452-453`) |
| core already answers hello | 0 | empty; kick.json written, ok, "already running" | `herdrcore/core.py:441-444` |
| core unreachable, `hc server start` ok, hello answers | 0 | empty; kick.json written, ok, "started" | `herdrcore/core.py:447,451`; `start_core` `herdrcore/core.py:275-301` |
| `hc_binary()` fails (`ConfigError`) | 1 | empty; kick.json written, `ok:false`, redacted text | caught `herdrcore/core.py:448-450`, re-raised; `_wrap` gives `OpError` (`herdrcore/ops_table.py:48-49`) |
| core listens on another socket (`check_core_lock`) | 1 | empty; kick.json written, `ok:false` | `OpError` from `check_core_lock` (`herdrcore/core.py:264-272`), recorded `herdrcore/core.py:448-450` |
| `hc server start` non-zero or timed out | 1 | empty; kick.json written, `ok:false` | `OpError "cannot start the city's core: hc server start ..."` (`herdrcore/core.py:288-292`) |
| core never answers hello within budget | 1 | empty; kick.json written, `ok:false` | `OpError "the city's core did not answer ..."` (`herdrcore/core.py:293-300`, raise at `herdrcore/core.py:300`) |
| `hello` raises another `HcError`, or `OSError` | 1 (`internal error: ...`) | empty; kick.json written, `ok:false` | caught and re-raised (`herdrcore/core.py:448-450`); neither is an `OpError`, so `perop` maps it (`rpp/perop.py:153-154`) |
| `backend_unavailable` (`GC_RPP_FAULT=unavailable`) | 1 (when `kick_due` and lock acquired), or 0 (backoff or lock contention) | empty; kick.json `ok:false` on exit 1 path (not written on exit 0 skip or `StateDirError`/`LockFileError`) | `herdrcore/core.py:432-453`, `hc/client.py:311-312` (`run_kick` has no `backend_unavailable` check: when due and unlocked, `hello()` raises `HcUnreachable`, `start_core` runs `hc server start` if binary/helper resolves, and follow-up `hello()` at `:293-300` remains unavailable -> exit 1 + kick.json `ok:false`; `kick()` itself never spawns, `:410-411`, `tests/unit/test_herdrcore_core.py:236-239`) |
| deadline already expired at start | 1, `no time left before the op's deadline` | empty; kick.json not written | `rpp/perop.py:144-145` |
| start budget of 0 or less | 1, `no time left to start the city's core` | empty; kick.json written (inside the `OpError` catch) | `herdrcore/core.py:280-281`; `herdrcore/core.py:448-450` |
| SIGTERM or cancel during start | 1 | empty; command-token `Cancelled` (`herdrcore/command.py:63` or `cancel.pause` at `herdrcore/core.py:301`) bypasses the caught tuple so kick.json is not written; `HcCancelled` during `hello()` (`herdrcore/core.py:293-300,441`, `hc/errors.py:107-112`) is an `HcError` caught at `herdrcore/core.py:448-450`, writes kick.json `ok:false`, and re-raises to `rpp/perop.py:149-150` | `rpp/perop.py:149-150`; `herdrcore/command.py:63` `cancel.check()`; `herdrcore/core.py:448-450`; `hc/errors.py:107-112` |
| lock file unusable (`LockFileError`) | 1 | empty; kick.json not written | `_wrap` gives `OpError` (`herdrcore/ops_table.py:48-49`); `rpp/flock.py:31` |
| stdout cap | cannot occur | n/a | stdout is always `b""` (`rpp/ops.py:32`, `rpp/perop.py:158`) |

Deadline detail: class "default" = 30 s (`rpp/ops.py:23-27`). `start_core` cuts
`CORE_START_WAIT_S` (15 s, `herdrcore/core.py:67`) with `deadline.budget`, which
subtracts a 1 s margin (`rpp/cancel.py:27,98-103`). The hello wait runs a second
budget after the start command (`herdrcore/core.py:293`), so the total is bounded
by the 30 s deadline, not by 15 s. The time spent by `hc server start` is included
in `deadline.remaining()`.

### Conflicts

**Can gc v1.5.1 call it? No evidence.**

- Searches in the v1.5.1 tree: `rg -n 'x-kick|kick-core' .` gave no matches. `rg -n
  'x-kick|kick' internal/runtime docs/reference/exec-session-provider.md` gave only
  unrelated hits (`internal/runtime/tmux/tmux.go:2710,2786`,
  `internal/runtime/t3bridge/provider.go:2628`,
  `internal/runtime/herdr-provider-design.md:99`). `rg 'x-'` in
  `internal/runtime/exec`, `internal/runtime/protocol.go` and the reference doc
  finds no `x-` op, only words like "tmux-in-box". gc's op set is fixed in
  `internal/runtime/exec/exec.go` and `docs/reference/exec-session-provider.md`.
- The op is not in the RPP v0 op table and is not declared by the handshake; the
  pack launches it itself (`herdrcore/core.py:420`).
- The pack's exit-2 rule would apply if gc sent an unknown op
  (`rpp/exitcodes.py:17-19`), but `x-kick-core` is in the table, so a stray gc call
  would run it (unverified that gc cannot be made to; no gc code path found).
- Conflicts pack vs gc v1.5.1: none, since gc does not mention the op. Related
  note: the parity doc already records that the kick op is in the table
  (`docs/provider-herdr-parity.md:441`) and that reads kick a core start
  (`docs/provider-herdr-parity.md:471`, `docs/provider-herdr-parity.md:479`).
- Parity doc: `docs/provider-herdr-parity.md` has per-method sections for protocol,
  start, stop, provision, exec, is-running, process-alive, list-running,
  is-attached, get-last-activity, peek, nudge, send-keys, interrupt,
  clear-scrollback, watch-startup, attach, set-meta, get-meta, remove-meta,
  observe, inventory, kill-object, server-stop and copy-to
  (`docs/provider-herdr-parity.md:290-1065`). There is no `x-kick-core` section.
  This is a gap, and there is no verdict to report. Verdict: n/a (internal, not
  gc-callable).

### Unverified

- Why `housekeeping=False` was chosen (no source comment).
- Whether any external conditions (e.g. a broken `hc` binary or helper failure,
  backoff, or lock contention) alter a hand-run `GC_RPP_FAULT=unavailable`
  invocation (`herdrcore/core.py:432-453`, `hc/client.py:311-312` settle the
  code paths above).
- That nothing reaps the `Popen` child or reads its exit code (nothing in `kick()`
  does; the Python-side reaping is not checked).
- Which exceptions `resolve_paths` and `OpContext.__init__` can raise beyond those
  `_wrap` catches.
- No test in `tests/` invokes `x-kick-core` directly; coverage is indirect:
  `tests/unit/test_herdrcore_lifecycle.py:322-343` (a read with the core down
  starts it and writes kick.json ok), `tests/unit/test_herdrcore_core.py:224-233`
  (`kick()` backoff, spawn), `tests/unit/test_herdrcore_server.py:887-901,966-991`
  (parked, kick lock). `tests/unit/conftest.py:145,150` waits for `x-kick-core`
  processes to exit.
