---
schema: gc.build.implementation-summary.v1
workflow: {id: gcrex-7ufw1, formula: do-work}
methodology: {pack: gascity, name: build-basic}
producer: {formula: do-work, stage: implement, attempt: 3}
status: blocked
trace:
  upstream:
    - path: beads/gcrex-gk1k
      hash: bead:gcrex-gk1k
      ids: [AC-1, AC-2, AC-3, AC-4, DOD-1, DOD-2, DOD-3]
    - path: AGENTS.md
      hash: git:777e4d9
  coverage:
    - {id: AC-1, status: covered}
    - {id: AC-2, status: blocked, rationale: "The named regression test must exercise gc core (CachingStore close announcement / runproj fold); no such code or dependency exists in rex-gas-city."}
    - {id: AC-3, status: covered}
    - {id: AC-4, status: covered}
    - {id: DOD-1, status: covered}
    - {id: DOD-2, status: covered}
    - {id: DOD-3, status: blocked, rationale: "No behavior change is possible in this repo; the fix belongs to gc upstream."}
---

## Summary

GCREX-36 is a gc-core defect, not a rex-gas-city defect. A graph.v2 run root
closed in the store with no `bead.closed` event keeps reading `active` in
`GET /runs` and `/runs/census`, because the run projection folds the event log
and `CanonicalRunStatusForLane` is terminal only when the folded root bead is
closed. This repo (module `github.com/docbobo/rex-gascity`) is the Rex runtime
provider; it contains no run projection, event bus, or CachingStore and does not
depend on gc core. The defect is therefore unreproducible as a rex-gas-city code
change and unfixable here. This step (attempt 3) re-verified the live gap, the
absence of a seam, and the gc source chain, produced no source change, and is
blocked pending an upstream gastownhall/gascity fix.

Sources for that chain, per AGENTS.md (both checked):

- gc pinned baseline tag `v1.5.1`: `internal/runproj/canonical_status.go:52-75`
  (`CanonicalRunStatusForLane` returns a terminal status only when the folded
  root bead's `Status == "closed"`, else `active` once a member started);
  `internal/api/huma_handlers_runs.go:140-159` (`runFold` ->
  `BuildRunSummaryWithAllLanes` -> `CountCanonicalRunStatuses` for the census);
  `internal/runproj/enrich.go:25,36` (`staleLatchAfterMs` 24h,
  `staleRunSilenceMs` 72h self-heal).
- gc working tree HEAD `890a58a3e` (post-tag):
  `internal/beads/caching_store_close_announce.go:22-33`
  (`trackCloseTransitionLocked`; a closed row installed with no cached row, not
  announced, is only marked in `silentCloses` until a refresh with close
  evidence drains it) and `internal/beads/caching_store_read_announce.go:10-24`
  which names this exact hole: "every consumer that folds bead.* events - the
  run projection behind GET /runs ... keeps the stale row until some later
  change", and records that the close queue was meant to close it (#6860).
  Both announce files exist only at HEAD, not at tag `v1.5.1`; the installed
  build (`1.5.1+7497`) is ahead of the tag, so the missed-drain branch stays
  **unverified** against the pinned baseline. The installed build is a
  Nix-wrapped, stripped binary (183 dynamic symbols; `.gc-wrapped` carries no
  Go build info), so which of those sources the running gc contains cannot be
  determined from the binary either; the branch stays unverified for it too.

`docs/provider-herdr-parity.md` was checked first: it records provider ops and
parity only, and covers neither the run projection nor `/runs/census`, so it
does not adjudicate this claim.

## Intended Behavior

A run root that is closed in the bead store must read terminal (`completed`,
`failed`, or `canceled`) in `GET /v0/city/<city>/runs` and `/runs/census` even
when the folded event log never recorded a `bead.closed` for that root, rather
than staying `active` on its last mid-flight phase for hours.

## Changed Files

No source files changed. Added evidence artifact:
`plans/GCREX-36/implementation-summary-gcrex-4zefi.md`.

## Verification

First verification command (seam check, run in worktree at `777e4d9`; re-run by
attempt 2 at `6c12959`):

```sh
git grep -lE 'runproj|runs/census|bead\.closed|CanonicalRunStatus|BuildRunSummary|CachingStore' -- '*.go'
```

Result (attempt 1 and attempt 2 agree): no matches (exit 1). The module has no
seam for the defect.

Final proof command (live probe of my-city; attempt 3 re-ran it 2026-11-20
16:09 CET on the installed gc build):

```sh
curl -s http://127.0.0.1:8372/v0/city/my-city/runs/census
curl -s http://127.0.0.1:8372/v0/city/my-city/runs
gc bd show mc-4km --json
gc bd show mc-4n6 --json
grep -cE '"type":"bead.closed".*"subject":"mc-(4km|4n6)"' /Users/boris/my-city/.gc/events.jsonl
```

Result: gap still reproduces, unchanged from attempts 1 and 2. Census reports
`active:4` (`pending:0`); `/runs` lists `mc-4km` and `mc-4n6` as `active` (both
`rex-backlog-work`); both store rows are `closed` with `gc.outcome=pass`
(`mc-4km` closed_at `2026-11-18T16:36:43Z`, `mc-4n6` `2026-11-18T15:17:58Z`);
`bead.closed` count for both ids across `/Users/boris/my-city/.gc/events.jsonl`
is `0`. Last root lines remain `bead.updated status=open` at seq 19005
(`mc-4km`, ts `2026-11-18T17:36:04.883341+01:00`) and seq 17357 (`mc-4n6`, ts
`2026-11-18T16:16:07.813198+01:00`). No gc commit touching `internal/beads` or
`internal/runproj` has landed at HEAD `890a58a3e` since 2026-11-18, so the
upstream fix does not exist yet.

`make test`: pass (`go test ./...`; cmd/gc-runtime-rex, internal/launch,
internal/provider, internal/rex/cli ok; no code change since attempt 1).
`golangci-lint run ./...`: pass (0 issues).

## Remaining Risks

- The fix (close-announcement/reconcile so an out-of-band close reaches the bus)
  belongs to gastownhall/gascity; until it lands and my-city is upgraded,
  `/runs` and `/runs/census` keep over-reporting active runs. The projection's
  own self-heal (`internal/runproj/enrich.go:25,36`, 24h/72h) had not cleared
  these two roots: both still read `active` on 2026-11-20 16:09 CET, two days
  after their last event, past both self-heal windows.
- AC-2 and DOD-3 cannot be satisfied in this repo: there is no code path to
  regress, so no named test can fail without a change here.
- GCREX-36 stays open for a human to move upstream; re-running this workflow here
  will produce the same blocked result.

## Coverage

| ID | Status |
| --- | --- |
| AC-1 | covered |
| AC-2 | blocked |
| AC-3 | covered |
| AC-4 | covered |
| DOD-1 | covered |
| DOD-2 | covered |
| DOD-3 | blocked |
