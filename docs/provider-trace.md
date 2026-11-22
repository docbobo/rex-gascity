# Provider method trace

`gc-runtime-rex` can append one JSONL line per provider op so you can measure
which provider methods are hot before you optimize anything. The trace is off
by default and costs nothing when off. The only cost is one `os.Getenv` at
process start.

## Turn it on

Set `GC_REX_TRACE` in the environment gc passes to the provider. gc forwards
the caller's environment unchanged. So export it in the controller or session
environment that launches `gc-runtime-rex`, not in a Rex or `city.toml`
config.

| Value | Destination |
|---|---|
| unset or empty | off |
| `1` or `on` | `<state dir>/trace/provider-<YYYY-MM-DD>.jsonl` (UTC day) |
| any other value | that literal path, used unchanged |

The state dir is the resolved city's dir (`provider.StateDir`, keyed on the
resolved city). Under gc `v1.5.1` gc projects no city pin into the op process.
So the resolved city is `GC_REX_CITY` from gc's environment, else `default`.
Set `GC_REX_CITY` in the same environment to keep the default path under your
city's state dir.

The default path is named by day but does not rotate or delete old files.
Remove yesterday's files yourself.

## The line

One line per eligible op, appended as JSON:

```json
{"ts":"2026-11-22T10:00:00.123456789Z","op":"start","method":"StartNew","duration_us":1200,"outcome":"ok","exit_code":0,"city":"my-city","session":"web","agent":"web","v":1}
```

| Field | Meaning |
|---|---|
| `ts` | RFC3339Nano UTC start of the provider call |
| `op` | the RPP op name |
| `method` | the provider method the op handler invokes (`start`→`StartNew`, `peek`→`PeekLines`). It is the op name itself when the handler calls no provider method. |
| `duration_us` | integer microseconds. It may be `0` for a sub-microsecond call. |
| `outcome` | `ok` or `error` |
| `exit_code` | the process exit code |
| `city` | the resolved city (`CityFromEnv`), always present |
| `session` | the op's name argument. It is omitted when the op takes no name. |
| `agent` | the agent part of the session name. It is omitted with `session`. |
| `v` | schema version, `1` |

Only these fields are written. No environment map is serialized. No
start-config value is emitted except the resolved `city`, which derives from
the environment. No rig id and no parent-op id are recorded.

The pre-dispatch replies `protocol` and `version` write no line. Any unknown
or unimplemented op writes no line. A failed op still writes one line with
`outcome=error` and its real exit code. Tracing never changes an op's own
outcome or exit code.

## Size cap

The cap is a best-effort 128 MiB per file. Before appending, the writer checks
that the current size plus the full next line would not exceed 128 MiB. If it
would, the writer skips the line and prints one stderr warning. It never
truncates, rotates mid-op, or waits on another writer.

Each op is its own process and the check is not locked. So two writers near
the cap can both append and push the file past 128 MiB by at most one line
each. A destination that is not a regular file (a pipe, a directory) is
rejected up front with one warning. The op then runs untraced.

The unbounded overshoot is accepted on purpose, not tested. A strict test
would interleave two writers between the size check and the append. The two
calls are synchronous here, so a Go test cannot observe that window without a
dependency-injected stat hook. The requirements do not ask for that hook, so
the overshoot stays a design property. The tested behavior is narrower: a line
that fits under the cap is appended (`TestTraceNearCapFitsStillAppends`), and
a line that does not fit is skipped (`TestTraceCapSkipsLine`).

## Read the stats

```sh
scripts/provider-trace-stats.sh <file.jsonl> [<file.jsonl> ...]
```

The script needs `jq`. It prints one row per method, sorted by total time:

```
method count total_us p50_us p95_us max_us
StartNew 1 1200 1200 1200 1200
PeekLines 5 1100 200 500 500
```

Percentiles use nearest rank over the method's sorted durations.

To read one city over one period, select the input. There is no reader flag.
The default daily path already scopes by city (state dir) and day (filename).
Pass that file. For a literal shared path, pre-filter with `jq`:

```sh
jq -c 'select(.city=="my-city" and .ts>="2026-11-22T00:00:00Z"
       and .ts<"2026-11-23T00:00:00Z")' trace.jsonl |
  scripts/provider-trace-stats.sh /dev/stdin
```

## Deferred

In-call phase timing (lock wait, store read, rex call, shell out) is not in v1.
The schema leaves room for optional `phase`/`spans` fields. Add them later
behind the same switch without a schema break.
