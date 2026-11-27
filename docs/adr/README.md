# Architecture decision records

One file per decision a future reader would otherwise re-open: what we chose, why,
and — in `## Alternatives rejected` — what we turned down. That last section is
the point. It is what stops the next architecture review, human or agent, from
proposing the same refactor again.

Not every change needs one. Write an ADR when a decision has a rejected
alternative someone would plausibly propose again. Skip the ones that would only
restate what the code already says.

## Writing one

```sh
make adr TITLE="the pane lock is per session"
```

That writes `docs/adr/NNNN-the-pane-lock-is-per-session.md` from `template.md`
and prints its path. Fill it in, then add its row below.

- Statuses: `Proposed`, `Accepted`, `Superseded by ADR-NNNN`, `Rejected`.
- Date the decision, not the backfill. Mark `(backfilled)` when the two differ.
- Keep it to about a page. Link the longer standing documents
  (`docs/rex-provider.md`, `docs/REX-CONTRACT.md`) rather than copying them.

An ADR records a decision, not an instruction. The rules an agent must follow
live in the `AGENTS.md` files.

## Index

| # | decision | status |
| --- | --- | --- |
| [0001](0001-ui-status-row-framing.md) | The UI keeps a second row format for the status probe | Accepted |
| [0002](0002-liveness-unknown-is-not-dead.md) | A liveness read that fails answers `unknown`, never `dead` | Accepted |
| [0003](0003-retirement-policy-presets.md) | Retirement is configured by named presets, not by policy fields | Accepted |
| [0004](0004-binder-start-env.md) | `Binder` derives its labels from the projected start env | Accepted |
| [0005](0005-route-provider-hooks-through-the-runner-seam.md) | Provider hooks run through the `Runner` seam | Accepted |
| [0006](0006-the-readme-release-block-changes-only-after-a-published-release.md) | The README release block changes only after a published release | Accepted |
| [0007](0007-guard-opspec-method.md) | The `opSpec.method` and `startConfig` copies are guarded by table tests | Accepted |
| [0008](0008-dashboard-json-output-ignores-scope.md) | The dashboard `--json` model ignores `--scope` | Accepted |
| [0009](0009-keep-start-welded.md) | `start` stays welded; no `provision` or `exec` until a non-tmux launch half exists | Accepted |
| [0010](0010-provider-policy-modules-are-files-of-provider-methods-with-one-entry-point.md) | Provider policy modules are files of `Provider` methods with one entry point | Accepted |
| [0011](0011-verify-the-provider-against-gc-and-conformance-not-herdr-core-parity.md) | Verify the provider against gc and conformance, not herdr-core parity | Accepted |
| [0012](0012-ship-the-provider-as-an-out-of-tree-pack-runtime-that-drives-the-rex-cli.md) | Ship the provider as an out-of-tree pack runtime that drives the `rex` CLI | Accepted |
| [0013](0013-map-city-to-rex-session-rig-to-tab-agent-to-pane.md) | Map city to Rex session, rig to tab, agent to pane | Accepted |
| [0014](0014-the-provider-never-owns-or-stops-the-shared-rex-server.md) | The provider never owns or stops the shared Rex server | Accepted |
| [0015](0015-seed-session-identity-before-the-blocking-launch.md) | Seed session identity before the blocking launch | Accepted |
| [0016](0016-report-the-wrapped-gc-as-the-agent-process-instead-of-tracking-a-service.md) | Report the wrapped gc as the agent process instead of tracking a service | Accepted |
| [0017](0017-rex-is-the-supported-opt-in-backend-and-tmux-stays-the-default.md) | Rex is the supported opt-in backend and tmux stays the default | Accepted |
| [0018](0018-codeql-runs-advisory-for-go-and-actions.md) | CodeQL runs advisory for Go and Actions | Accepted |
| [0019](0019-the-ui-ships-from-this-repo-through-one-installer-that-edits-only-init-lua.md) | The UI ships from this repo through one installer that edits only `init.lua` | Accepted |
| [0020](0020-keep-fs-host-undeclared.md) | `fs.host` stays undeclared, `locality` stays `local` | Accepted |
| [0021](0021-recover-a-dialog-withheld-seed-inside-start.md) | Recover a dialog-withheld seed inside `start` | Accepted |
