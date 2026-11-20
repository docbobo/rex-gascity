---
schema: gc.build.implementation-summary.v1
workflow: {id: gcrex-zcey, formula: do-work}
methodology: {pack: gascity, name: build-basic}
producer: {formula: do-work, stage: implement, attempt: 1}
status: approved
trace:
  upstream:
    - path: beads/gcrex-30ib
      hash: bead:gcrex-30ib
      ids: [CR-4, CR-5]
    - path: internal/provider/binding.go
      hash: git:069f63e
    - path: internal/provider/sessionname.go
      hash: git:069f63e
  coverage:
    - {id: CR-4, status: covered}
    - {id: CR-5, status: covered}
---

# Implementation Summary: GCREX-5qel round 2 — recordless cross-tab discovery + bare-session fallback

## Summary

Both new CodeRabbit threads on PR #5 are fixed on `gcrex-5qel-pane-label` in
commit `7cd889d`, on top of `069f63e`, and pushed to the PR branch.

1. **Recordless cross-tab discovery** (`internal/provider/binding.go`,
   thread `PRRT_kwDOVyObGc6572Ox`). `Binder.scan` uses the hint tab label as
   the tab to search. A recordless resolve (`IsRunning`/`Stop` pass no hint)
   therefore used only the name-derived tab plus the town and legacy town tabs,
   so a no-`--` session such as `pi-mc-wisp-ybq1` started with
   `GC_RIG=my-project` (tab `Rig: my-project`) was reported not running and
   left alive by `Stop`. When `hint.TabLabel` is empty the scan now falls back
   to matching the session identity in every window of the city's Rex session;
   the identity is unique per city, so a cross-tab match is safe. The
   stored-record/hint path is unchanged and stays tab-scoped.
2. **Bare-session fallback label** (`internal/provider/binding.go`, thread
   `PRRT_kwDOVyObGc657p2T`). `paneLabelFor` falls back to the bare (clamped)
   session name when `<agent> (<GC_SESSION_NAME>)` cannot fit in 128 runes;
   `scan` only accepted the name-derived agent or the ` (<session>)` suffix, so
   such a pane was undiscoverable after record loss. `paneMatches` now also
   accepts `label == string(name)`. Legacy panes are not adopted: old labels
   were the alias, not the session name.

Out of scope, per gcrex-5qel: the legacy pane-label thread
`PRRT_kwDOVyObGc657Hhl` stays declined.

Coverage:

| ID | Status |
| --- | --- |
| CR-4 | covered |
| CR-5 | covered |

## Intended Behavior

- A recordless `IsRunning`/`Stop` for a no-`--` session started with `GC_RIG`
  finds its pane in the `Rig: <rig>` tab, without deriving a rig from the name.
- A recordless resolve still matches `<agent> (<GC_SESSION_NAME>)` labels, and
  now also a bare session-name label, which is what `paneLabelFor` emits for
  session names of 125-128 runes where the parenthesised suffix no longer fits.
- A resolve with a stored record or a start-env hint keeps its existing
  tab-scoped search; no cross-tab scan happens on that path.
- Session names longer than 128 runes stay undiscoverable by label, as
  documented: identity past 128 runes cannot be represented in Rex.

## Changed Files

- `internal/provider/binding.go` — `scan` extracts the binding construction
  into `accept`; `identityMatch` accepts `label == string(name)` or the
  ` (<session>)` suffix; `paneMatches` delegates to it. After the tab-scoped
  loop, an empty-hint scan searches every window by identity.
- `internal/provider/pane_label_test.go` — `TestRecordlessCrossTabDiscovery`
  (recordless `IsRunning` and `Stop` for `pi-mc-wisp-ybq1` in a `Rig:
  my-project` tab) and `TestRecordlessDiscoveryBareSessionFallback` (recordless
  `IsRunning` and `Stop` for bare-session labels at 125, 126, 127 and 128
  runes).
- `plans/GCREX-5qel/implementation-summary-gcrex-wgml.md` — this artifact.

## Verification

First (red, before the fix) — reproduces both threads through the provider:

```sh
go test ./internal/provider/ -run 'TestRecordlessCrossTabDiscovery|TestRecordlessDiscoveryBareSessionFallback'
```

Observed: FAIL — `IsRunning(pi-mc-wisp-ybq1) = false, <nil>; want true, nil`,
`Stop did not close block:pi-agent (pi-mc-wisp-ybq1); closed []`, and
`IsRunning = false, <nil>; want true, nil` for each of len=125, 126, 127, 128.

Final proof (after the fix) — full suite, vet and lint:

```sh
go test ./... && go vet ./... && golangci-lint run ./...
```

Observed: PASS — every package `ok` (`internal/provider` included), vet clean,
`0 issues.` from golangci-lint.

Pushed to PR #5: commit `7cd889d` on `gcrex-5qel-pane-label`
(https://github.com/docbobo/rex-gascity/pull/5), open and ready; the repo has no
CI checks configured on the branch (`no checks reported`).

## Remaining Risks

- The cross-tab fallback trusts the session name as a unique identity key. That
  holds for gc/city-generated names; if two city sessions could share a name in
  one Rex session, the first identity match wins.
- A bare-name pane label (session name of 125-128 runes) is only distinguishable
  by the exact session name, so two different cities' sessions with the same
  name are still disambiguated by the city's Rex session, not the label alone.
- The local `gcrex-kz9p` worktree still holds the `gcrex-5qel-pane-label` branch
  at `069f63e`; it was not touched. The push updated origin, so that worktree
  needs a fetch/checkout before its next use.
- The running `gc-runtime-rex` binary must be rebuilt (`make install`) for the
  operator to observe the fix; the same caveat as the GCREX-kz9p summary.
