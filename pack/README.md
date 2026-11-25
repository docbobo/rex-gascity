# The rex runtime pack

The operator quickstart is in
[`docs/rex-runtime-pack.md`](../docs/rex-runtime-pack.md). It covers import,
install, auth, upgrades, the source checkout, the release flow, and
troubleshooting.

This directory holds the pack payload and its tooling: `pack.toml`,
`install.sh`, `release.sh`, `commands/`, `doctor/`, and `tests/`.
`commands/status-update/` is `gc <binding> status-update`, which shows the
mayor's status update in a floating Rex pane.
