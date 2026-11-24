## Scope

`internal/rex/` is the client for the `rex` CLI: argv construction, the wire
protocol, paging, and fault handling. `internal/rex/fakerex/` is the fake the
provider e2e suites drive; `internal/rex/faketest/` is the shared test harness.
The repo-wide rules in the root `AGENTS.md` apply here; this file adds only
local invariants.

## The pinned surface

- `docs/REX-CONTRACT.md` is the pinned Rex CLI and API surface and the exit
  codes. Cite it for any argv, wire, or fault change.
- A change here that alters the wire must update `internal/rex/fakerex/` in the
  same change, and `docs/fake-rex.md` must still describe what the fake covers.
  A fake that has drifted from the contract makes the e2e suites lie.

## Tests

- Unit tests do not shell out to a real `rex`. The fake is the default.
- The live server is only for the explicit escape hatch (`REX_E2E=1` or
  `REX_E2E=live`).
- A behavior change lands with the named test that fails without it. See the
  root "Test-driven development" rule.
