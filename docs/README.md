# Documentation

`docs/` holds reference docs and decision records only. Plans, specs and
research notes live in issues and beads, and their decisions become ADRs.
`pack/tests/check-doc-links.sh` fails when a doc is missing from this index.

## For users

People who run a city on the `rex` provider.

- [rex-runtime-pack.md](rex-runtime-pack.md): quickstart. Import the pack, install, authenticate, upgrade, and the release flow.
- [rex-binary-install.md](rex-binary-install.md): how the installer and the `rex` shim work, and the version-keyed cache.
- [rex-gascity-ui.md](rex-gascity-ui.md): the Gas City UI Rex extension. Install it, use the `rex-ui` doctor check, and configure it.
- [rex-provider.md](rex-provider.md): provider reference. Configuration, identity mapping, state, environment, known limits and operational risks.
- [provider-trace.md](provider-trace.md): the op trace schema and the stats reader, for debugging a city.

## For us

Maintainers and agents working on this repository.

- [REX-CONTRACT.md](REX-CONTRACT.md): the pinned Rex CLI and API surface the provider uses, and its exit codes. The fake's contract test parses it.
- [fake-rex.md](fake-rex.md): what the in-repo fake `rex` covers, its limits, and the live-server escape hatch (`REX_E2E`).
- [ci.md](ci.md): the CI job map, the measured cost and the time budgets.
- [adr/](adr/README.md): architecture decision records. Each one holds one decision and the alternatives it rejected.
