---
schema: gc.build.implementation-summary.v1
workflow: {id: gcrex-x8gvv, formula: do-work}
methodology: {pack: gascity, name: build-basic}
producer: {formula: do-work, stage: implement, attempt: 1}
status: approved
trace:
  upstream:
    - path: beads/gcrex-youi
      hash: bead:gcrex-youi
      ids: [AC-1, AC-2, AC-3]
    - path: internal/provider/provider.go
      hash: sha256:4790a21cfb3b0fa5a805ebdea12309afcee5d8347313f674f5d677c3252085bc
    - path: internal/provider/focus_test.go
      hash: sha256:57b6204cefec67ed754cf295a5d30ee19cf0548eb8a1362dfcfc090718bd30e7
  coverage:
    - {id: AC-1, status: covered}
    - {id: AC-2, status: covered}
    - {id: AC-3, status: covered}
---

## Summary

A provider start now launches a brand-new session's tab with
`session.new_window {focus: true}`, so the operator lands in the new tab
instead of staying wherever they were. The transport already passed
`LaunchSpec.Focus` through to `session.new_window`/`session.new_split`; the
provider simply never set it. The launch in `internal/provider/provider.go`
(inside the tab-locked launch closure) now sets `windowSpec.Focus = !found`,
where `found` is the answer from the pre-launch `Binder.Resolve`. Nothing else
about the launch path changed.

Decision and rationale: gc's start contract carries no explicit
create-vs-reattach signal, so the rule keys off the binding lookup. A name with
no binding (and no surviving record that answers to it) is a new session and
gets focus. An already-live pane returns before any launch, so a re-attach
cannot move focus. A dead pane's relaunch is an existing session and keeps
`focus:false`. A `session.new_split` into an existing rig tab also keeps
`focus:false`: the tab already exists, and moving the active tab there is the
focus-stealing behaviour the original `focus:false` decision was made to avoid.
Focus is requested through the launch spec, never derived from the tab label.

Commit: `6630e54` on worktree `/Volumes/Sources/rex-gas-city/worktrees/gcrex-youi`.

## Intended Behavior

- Starting a session that has no binding yet creates its tab with
  `focus: true`, making that tab the session's active window so the operator
  sees the new session.
- Re-attaching an existing live session (idempotent start) launches nothing and
  leaves the active window alone.
- Relaunching a session whose pane died keeps `focus: false`; it is an existing
  session.
- Starting a second agent in a rig that already has a tab splits beside an
  existing pane with `focus: false`, so the active tab does not move.
- Attachment (`is-attached`) now reads a newly created session as attached once
  an app client exists, because its tab is genuinely active.

## Changed Files

- `internal/provider/provider.go` — the window branch of the launch closure
  copies the plan's spec and sets `Focus = !found` before `NewWindow`; the split
  branch is unchanged.
- `internal/provider/focus_test.go` (new) — `TestStartFocusesOnlyOnCreate`
  covers create (focus true), live re-attach (no launch), and dead-pane
  relaunch (focus false).
- `internal/rex/client.go` — `LaunchSpec.Focus` doc now states the
  create-only rule instead of "provider launches leave it false".
- `internal/provider/attach.go` — `watchesAgent` doc now matches the new rule.
- `docs/REX-CONTRACT.md` — the attachment bullet records that
  `session.new_window` sends `focus:true` only for a new session.

## Verification

First verification command, run in the worktree before the fix (test written
first, red as expected):

```sh
go test ./internal/provider/ -run TestStartFocusesOnlyOnCreate -v
```

Result: FAIL — `focus_test.go:29: new session launched with focus=false, want true`.
After the fix: PASS.

Final proof command, run in the worktree at commit `6630e54`:

```sh
make test && go vet ./...
```

Result: PASS — `go test ./...` ok for `cmd/gc-runtime-rex`,
`internal/launch`, `internal/provider`, `internal/rex/cli`; `go vet ./...`
clean. The pre-commit hook ran `golangci-lint` on the staged Go files:
`0 issues`.

Existing split expectations still hold:
`TestStartSplitsTheLargestPaneAlongItsLongerAxis` passes unchanged (split
keeps `focus:false`).

| ID | Status |
| --- | --- |
| AC-1 | covered |
| AC-2 | covered |
| AC-3 | covered |

## Remaining Risks

- Live Rex behaviour of `session.new_window {focus:true}` is not exercised by a
  live probe in this step; the flag is documented in
  `docs/REX-CONTRACT.md` §`session.new_window` and the CLI already serialized
  it before this change. If focusing a window does not activate its session in
  the app when another session is in front, the operator can still stay on the
  other session; the bead asked for the tab/window focus, which this change
  does send.
- Automatic agent spawns (beads, pools) that land in a rig with no existing tab
  will now bring that tab forward. That is the requested behaviour ("new
  session becomes focused"), but it means a burst of first-in-rig starts can
  move the operator's view more than before. Splitting into existing tabs never
  moves it.
