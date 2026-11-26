# 0004. Binder start env

- Status: Accepted
- Date: 2026-11-26

## Context

`ResolveRequest` carried `Hint`, `LegacyTabs` and `LegacyPanes`
(`internal/provider/binding.go`). `provider.start` built those labels from the
projected start env (`internal/provider/provider.go`). `Binder` derived the
same labels again for its fallback scan. Two derivations had to agree. A
divergence launches a duplicate pane, or hides a live one.

The env holds every input. `legacyPaneLabelFor`
(`internal/provider/sessionname.go`) needs `GC_SESSION_NAME`, `GC_ALIAS`,
`GC_AGENT`, `BEADS_ACTOR` and `GC_RIG`. `legacyTabLabelFor` needs `GC_RIG`.

This ADR records a decision from issue #93. It also records a conflict. The
requirements' Out Of Scope forbids `docs/` changes beyond the parity and method
documents. The root `AGENTS.md` "Recording decisions" rule needs this ADR.
Under the root authority order, the root limit wins.

## Decision

We pass the projected start env as `ResolveRequest.StartEnv`. `Binder` derives
every label from that env and the name, in one place (`resolveLabels`).

## Consequences

- One label derivation exists. A caller passes facts, not precomputed labels.
- `ResolveRead` and the record rescan still use the stored labels. A read op
  has no start env.
- Rediscovery of an env-derived pane survives a lost sidecar record (BR-11,
  AC-9).
- `provider.start` still calls `tabLabel` for the pane it launches.

## Alternatives rejected

- Keep a narrow `RecordHint` field. It still puts label derivation in the
  caller, so the precedence rules stay split.
- Put the start env on `Binder` at construction. The provider builds one
  `Binder` per process. The env differs per start call.
- Accept the loss of env-derived rediscovery. That violates BR-11 and AC-9.
- Leave the three label fields in place. They duplicate the derivation that
  `Binder` already does.
