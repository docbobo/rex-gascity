# 0018. CodeQL runs advisory for Go and Actions

- Status: Accepted
- Date: 2026-11-24 (backfilled from the CodeQL evaluation, bead `gcrex-6pk2r`)

## Context

The gates we run do not cover dataflow or workflow security:

- `go vet` and `golangci-lint` (errcheck, govet, ineffassign, staticcheck,
  unused);
- the tests, `pack/tests/verify.sh` and conformance.

The repository is public, so CodeQL minutes are free. CodeQL supports Go and
GitHub Actions. It does not support Lua, so the `ui/` extension is outside its
reach. Go has no `none` build mode, so `autobuild` is the only non-manual
choice.

Ruleset `27750059` requires only `Conformance` and `pack-verify`.

## Decision

`.github/workflows/codeql.yml` analyzes `go` (autobuild) and `actions` (no build)
with the default suite. It runs on pull requests, on pushes to `main`, weekly,
and on dispatch. It is advisory: no ruleset requires it.

After four weeks of alerts, about 2026-12-22, we read the alert count. Then we
decide whether to require `Analyze (go)` or add a `code_scanning` rule. Only a
`code_scanning` rule makes findings block.

## Consequences

- Findings show in code scanning but never block a merge until the review above
  changes that.
- Lua in `ui/` has no static security analysis.
- `govulncheck` is not covered and remains a separate follow-up.

## Alternatives rejected

- **Gate on CodeQL now.** There is no alert history yet to judge noise against.
- **Add `javascript-typescript` or `python`.** No such source exists here.
- **The `security-extended` suite.** Switch only if the first month shows
  findings worth the extra runtime.
- **Analyze the Lua UI with CodeQL.** CodeQL has no Lua extractor (CodeQL
  supported-languages page, read 2026-11-27).
