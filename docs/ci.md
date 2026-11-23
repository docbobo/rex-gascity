# CI

`.github/workflows/ci.yml` runs on every pull request and on `main`. The
critical path is the slower of two jobs, so a change only pays off if it cuts
the slower one.

| Job | Runs | Budget |
|---|---|---|
| `pack-verify` | pack checks, then the fake-backed e2e suites | 180 s |
| `Conformance` | vet, test, lint, build, pinned rex, conformance | 240 s |

A docs-only pull request skips the Conformance steps and finishes in under a
minute there, so its verdict time is `pack-verify`. The target is 300 s at p90.

## Measured cost

Run 45255663571 (2026-11-23) is representative before the cache landed:

| Step | pack-verify | Conformance |
|---|---|---|
| checkout and runner setup | 5 s | 17 s |
| `go install` the pinned gc | 308 s | 175 s |
| `go vet`, `go test`, lint, build | — | 31 s |
| pack checks | 68 s | — |
| conformance harness and run | — | 58 s |
| fake-backed e2e | 16 s | — |

The pinned `gc` build was the bottleneck in both jobs. Over the 30 days
before this workflow changed, `scripts/ci-timings.sh` reported p50/p90 of
371/527 s for `pack-verify` and 352/559 s for `Conformance`.

After the change, measured on pull request #55:

| Job | code pull request, warm Go cache | docs-only pull request, cold cache |
|---|---|---|
| `pack-verify` | 116 s | 358 s |
| `Conformance` | 145 s | 5 s |

"Cold" means no Go build cache for that branch. A pull request run restores
caches from its base branch, while a cache a pull request saves stays scoped
to that PR's merge ref (GitHub dependency-caching reference). `main` runs CI on
every push, so a docs-only pull request into `main` normally restores that
cache and `pack-verify` lands near 105 s.

## Caches

`actions/setup-go` runs with `cache: true` and
`cache-dependency-path: go.mod`. This repository has no Go dependencies, so
`go.sum` does not exist and the default cache key would be skipped. The Go
build cache then makes `go install gc@pin` cheap once it is warm: the first
cold build in the 45286698759 run took 301-338 s, and later runs link from
cached objects.

The built `~/go/bin/gc` binary is deliberately not cached. A `go install`
binary links against the runner image, and `ubuntu-latest` is not one image:
the same key restored a binary built on another image and `gc` died with
`libicui18n.so.78: cannot open shared object file`. The Go build cache gives
the speed without that coupling.

## Docs-only fast path

The Conformance job reads the pull request's changed file list and treats the
change as docs-only when every file is under `docs/`, under `plans/`, or a
top-level `*.md`. It then skips Go setup, the Go steps, and conformance.
`scripts/docs-only.sh` reads the list and returns the decision. The list
includes previous names, so a file renamed out of the code is a code change.

The job is not path-filtered. A branch-protection check that never runs stays
pending and blocks the merge, so the job reports success instead.

`pack-verify` still runs `pack/tests/verify.sh`, and that includes
`check-docs.sh`, which executes the `sh` fences of the operator docs. A docs
change is still tested.

Push runs, `workflow_dispatch` runs, and a failed file-list lookup all take the
full path.

## Measuring

Every job prints its own wall time in the log and the run summary. The timer
starts at the first step, so it leaves out runner setup. Use the API window for
budgets.

```sh
scripts/ci-timings.sh           # p50 and p90 per job, last 30 days
scripts/ci-timings.sh --days 7  # shorter window
make timings                    # the same script
```

`scripts/docs-only.sh` holds the docs-only path rules. `pack/tests/check-ci.sh`
tests both helpers in `pack/tests/verify.sh`, so neither the digest nor the
fast-path decision can silently go wrong.

`.github/workflows/ci-timings.yml` runs the same script every Monday and writes
the table to that run's summary.

## Known limits

- The binary cache is keyed on the pin alone. A different gc source tree with
  the same pin is not a supported state.
- `pack-verify` has no docs-only path. Its pack checks already cost under two
  minutes with a warm cache, and splitting the job would add setup time to the
  critical path. The docs-only verdict time is `pack-verify`, so the 300 s p90
  target holds only while the base branch keeps a Go build cache. On a cold
  cache the `gc` install costs about 300 s once.
- `Conformance` keeps `timeout-minutes: 20` as the backstop for a hang, not as
  a budget.
