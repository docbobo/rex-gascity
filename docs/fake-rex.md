# Fake Rex for Tests

`internal/rex/fakerex` builds a fake `rex` executable that the provider e2e
suites drive by default. The real `cli.Client` runs unmodified against this
binary, so argv, wire shapes, paging, and child processes are exercised. No
real Rex application, server, or network is involved.

```sh
make test-e2e        # the fake-backed suites; no rex on PATH needed
```

`make test` stays the fast lane (`go test ./...`) and does not need rex either.
The e2e tests select the fake when `REX_E2E` is unset or `fake`.

## Live server escape hatch

`REX_E2E=1` or `REX_E2E=live` selects the real Rex server instead. That mode
skips when no rex is on `PATH` and mutates the configured server. It is the
explicit opt-in, not the normal path. Any other `REX_E2E` value fails the
test, so `REX_E2E=1` never silently means the fake.

```sh
REX_E2E=1 go test ./internal/provider -run TestE2E -v
REX_E2E=1 go test ./cmd/gc-runtime-rex -run TestE2E -v
```

## What the fake emulates

- The CLI surface the provider uses: `version` (text and `--json`),
  `api describe`, `api call`, `send-key`, `session kill`, `block call`, and
  `block inspect`.
- The API methods those calls reach:
  - `server.version`.
  - `session.create`, `session.destroy`, `session.list`,
    `session.list_clients`, `session.view`, `session.new_window`,
    `session.new_split`, `session.move_window`, `session.focus_block`,
    `session.focus_window`, `session.set_block_label`,
    `session.set_window_label`, `session.set_label`. `session.move_window`
    bumps the revision only when the order changes, and answers
    `session_not_found` for an unknown session and `not_found` for a window
    not in the session (both exit 3), and `bad_request` (exit 1) for
    `before_window_id == window_id`.
  - `block.close`, `block.event`, and `block.method`. The supported methods
    are `paste`, `key`, `focus`, `process`, `format`, `blob_read`, `write`,
    `clear`, `program_status`, `size`, `title`, `pwd`, `input_mode`, and
    `reset`.
  - `input_mode` answers `{echo: true, canonical: false, signals: true}` by
    default. A test sets one block's mode through `fakerex.Fake.Update` and
    `fakestate.Block.InputMode`, for example
    `&fakestate.InputMode{Canonical: true}` for a secret read (#48).
  - `target.resolve` and `target.complete`.
- Real child processes for panes, started detached with stdin on a FIFO.
- Rex's format page cap. A `format` reply carries at most 983040 content
  bytes. A larger capture pages through `blob_read`, with a 49152-byte
  default page.
- Exit codes 2 (usage or validation), 3 (missing target), 4 (no server), and
  1 (server-side failure), per `docs/REX-CONTRACT.md` section 8.

## What the fake does not emulate

- **A PTY.** Panes use pipes, not terminal devices. A program sees no tty, so
  terminal-only behavior differs. The fake renders only what the program
  writes. A live Rex block also renders its prompt, which is why a
  non-empty-screen assertion uses a program that prints.
- **Rex's full key-binding table.** `ctrl+c` sends `SIGINT` to the pane's
  process group. `enter`, `return`, `tab`, `space`, `up`, `down`, `left`,
  `right`, `esc`, `escape`, and `ctrl+<letter>` write the usual bytes. Any
  other key is written literally to stdin.
- **Server persistence and the real application.** State is one JSON file in
  the test's state dir (`FAKEREX_STATE`), guarded by a flock. Nothing survives
  the test. No Rex server or network is contacted.
- **Screen rendering, scrolling, and terminal escape handling.** These belong
  to the PTY the fake does not have.

## Scripted faults

`fakerex.Fault(t, method, kind)` queues one one-shot fault for the next call to
a method. `FaultN` queues `count` of them. The kinds are:

| Kind | Effect |
|---|---|
| `FaultError` | server-side failure (exit 1) |
| `FaultUnreachable` | no server (exit 4) |
| `FaultLostReply` | the call runs, the reply is dropped |
| `FaultDelay` | the call stalls for 2 seconds |
| `FaultGarbled` | malformed reply payload |

Method keys are the API method names (`session.create`) or CLI names
(`send-key`, `block.method.<method>`).

## Conformance still runs the real rex

The fake is a test double, not a substitute for the application. `make
conformance` builds the provider and runs `pack/tests/conformance.sh`, which
drives the real rex CLI. Install the rex CLI and put it on PATH first. The
Makefile target does not install it.

The CI conformance job installs the real rex archive for the
`pack/rex-min-version` pin (`REX_URL` and `REX_SHA256` in
`.github/workflows/ci.yml`) before it runs the same script. `make test-e2e` is
the coverage a contributor gets without Rex. Conformance remains the gate on
the real CLI and server.

The fake's own contract test (`internal/rex/fakerex/contract_test.go`) reads
`docs/REX-CONTRACT.md`. It fails when the fake and the pinned contract drift.
