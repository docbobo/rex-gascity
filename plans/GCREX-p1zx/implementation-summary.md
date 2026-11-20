---
schema: gc.build.implementation-summary.v1
workflow: {id: gcrex-d990, formula: do-work}
methodology: {pack: gascity, name: build-basic}
producer: {formula: do-work, stage: implement, attempt: 1}
status: approved
trace:
  upstream:
    - path: beads/gcrex-p1zx
      hash: bead:gcrex-p1zx
      ids: [REQ-001, REQ-002, REQ-003, REQ-004, REQ-005]
    - path: internal/provider/binding.go
      hash: sha256:f398e22f9bea03c60783985ba994aca2899d25e9507673d41e75794e307aa088
    - path: internal/provider/pane_label_test.go
      hash: sha256:9f9777ee8fb5afb682d38e3c57ab61308d6ff00574a78af8e3252d55d987e6d8
  coverage:
    - {id: REQ-001, status: covered}
    - {id: REQ-002, status: covered}
    - {id: REQ-003, status: covered}
    - {id: REQ-004, status: covered}
    - {id: REQ-005, status: covered}
---

# Implementation Summary: GCREX-p1zx — Same-tab bare-label adoption

## Summary

`Binder.scan` (`internal/provider/binding.go`) tab-scoped path accepted a bare
label equal to the session name through `identityMatch`, so a recordless
resolve for `rig--worker` scanned tab `Rig: rig`, adopted the legacy pane of
`rig--rig--worker` (labelled `rig--worker`), and `Stop("rig--worker")` closed
that other session's pane. The scan now accepts a bare label only when it equals
the expected pane label (`pane`, from the hint or stored record) or when it
carries the ` (<session>)` suffix; the implicit `label == string(name)` match is
gone. The global fallback remains suffix-only.

## Intended Behavior

- Tab-scoped scan matches a pane label when it equals the expected pane label
  (`hint.PaneLabel`/stored `PaneLabel`, or the name-derived label) or ends with
  ` (<GC_SESSION_NAME>)`.
- A bare label equal to the session name is no longer accepted implicitly in a
  candidate tab; a recordless resolve does not adopt another session's pane.
- Global fallback scan is unchanged: suffix-only.
- Hinted/recorded bare-label panes for 125-128 rune town-level session names
  still resolve and are not relaunched.
- Out of scope: legacy-label adoption; recordless discovery of bare-label
  125-128 rune panes (accepted trade-off).

Coverage:

| ID | Status |
| --- | --- |
| REQ-001 | covered |
| REQ-002 | covered |
| REQ-003 | covered |
| REQ-004 | covered |
| REQ-005 | covered |

## Changed Files

- `internal/provider/binding.go` — drop `identityMatch`; `paneMatches` now
  accepts only `label == pane || strings.HasSuffix(label, " (<session>)")`;
  comment records why a bare label is ambiguous in these tabs.
- `internal/provider/pane_label_test.go` — `TestTabScopedBareLabelNotAdopted`
  (recordless `IsRunning` false and `Stop` closes nothing when `Rig: rig` holds
  a `rig--worker`-labelled pane of `rig--rig--worker`) and
  `TestBareLabelPaneFoundWithAttribution` (hinted/recorded bare-label pane still
  found for 125-128 rune names).
- `plans/GCREX-p1zx/implementation-summary.md` — this artifact.

## Verification

First verification command — the red proof, run against the pre-fix
`binding.go`:

```sh
go test ./internal/provider/ -run 'TestTabScopedBareLabelNotAdopted'
```

Observed: FAIL — `IsRunning = true; tab-scoped bare label "rig--worker" of
another session must not be adopted`. With the fix restored the same test
passes.

Final proof command — acceptance suite:

```sh
go test ./... && go vet ./... && golangci-lint run ./...
```

Observed: PASS — `go test ./...` all packages ok; `go vet ./...` exit 0;
`golangci-lint run ./...` reports `0 issues.`

## Remaining Risks

- Recordless discovery of a bare-label pane for a 125-128 rune town-level
  session name is not supported: without a hint or stored record the scan
  cannot tell it from another session's pane. A record lost on such a session
  may relaunch a pane. Accepted trade-off in the bead.
- Legacy panes labelled with the pre-PR #5 bare label are not adopted
  recordlessly; they still resolve when a hint or stored record carries the
  label.
