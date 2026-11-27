# 0011. Verify the provider against gc and conformance, not herdr-core parity

- Status: Accepted
- Date: 2026-11-27

## Context

The provider began as a port measured against `runtime-herdr-core`, the reference
exec provider in `gastownhall/gascity-packs`. Four documents carried that
measurement:

- `docs/provider-herdr-parity.md`: per-op verdicts.
- `docs/provider-herdr-method-io.md`: herdr-core's inputs and outputs, per op.
- `docs/provider-herdr-method-deviations.md`: the compiled audit.
- `docs/provider-method-audit/`: 25 per-op reports.

Supporting notes were `docs/herdr-extensions.md` and
`docs/research/rex-provider-extensions.md`. Together they ran to about 7,500
lines. `internal/provider/AGENTS.md` required all three top-level documents to
change with every provider method, and `provider-herdr-parity.md` alone took 392
commits.

That measurement has done its job. The "Confirmed mistakes" table in the parity
doc was empty: GCREX-20 through GCREX-108 fixed every entry. The provider now has
Rex-only ops (`idle`, `program-status`) and capabilities that herdr-core does not
have, so a herdr-core verdict no longer says whether the provider is right. Two
checks do say it:

- the pinned gc source (`v1.5.1`), which defines what gc sends and how it reads
  the answer;
- `gc runtime conformance`, which runs in CI (`.github/workflows/ci.yml`, job
  `conformance`, via `pack/tests/conformance.sh`), together with the provider's
  unit and fake-backed e2e tests.

## Decision

We verify provider behavior against the pinned gc source and the executable
checks: `make conformance` and `make test`. We no longer keep a herdr-core
comparison. The six documents above are deleted.

Their content that still applies moved as follows:

- The deliberate gaps and operational risks went to `docs/rex-provider.md`.
- The Rex-only ops are already in `docs/REX-CONTRACT.md` §9.
- The `send-keys` key grammar is stated in `internal/provider/keymap.go` and
  pinned by `keymap_test.go`.

Earlier ADRs that cite the parity doc now link to it at commit `09f11b8`, the
last commit that held it.

## Consequences

- Changing a provider method means updating `docs/rex-provider.md`, plus
  `docs/REX-CONTRACT.md` §9 when the op table or a token changes. The test that
  pins the change goes with it.
- herdr-core remains a source of ideas. Behavior ported from it (the key grammar,
  the startup dialog list) cites the herdr-core file and line in code, not a
  comparison document.
- A claim that neither gc source nor a check settles is labelled `unverified`,
  as the root `AGENTS.md` requires.
- Historical verdicts can still be read at `09f11b8`.

## Alternatives rejected

- **Keep the parity docs and update them each round.** Every round paid for
  three documents that move together, and the four layers drifted against each
  other. They measured the provider against a peer rather than the consumer. gc's
  source and conformance answer the question that matters: does gc get the right
  answer?
- **Merge them into one parity doc.** This is smaller, but it keeps the same
  question and the same per-change tax, and it still needs re-running against a
  moving herdr-core.
- **Keep only the per-op audit reports.** They had not changed since 2026-11-19
  for most ops, so they were snapshots. Snapshots belong in git history, which
  already keeps them.
