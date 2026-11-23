#!/bin/sh
# Report p50 and p90 wall time per CI job over the last 30 days.
#
#   scripts/ci-timings.sh                 # ask GitHub about this repository
#   scripts/ci-timings.sh --days 7        # shorter window
#   scripts/ci-timings.sh --self-test     # check the percentile math, no network
#   ... | scripts/ci-timings.sh -         # summarize saved "job<TAB>seconds"
#
# Live mode needs gh and jq. The duration is the job's own window from the
# Actions API, so it includes runner setup, which the in-job timer does not.
#
# The weekly digest workflow runs this on a schedule and writes the table to
# the run summary. The job layout and budgets are documented in docs/ci.md.
set -eu

days=30
limit=100

usage() {
  echo "usage: $0 [--days N] [--limit N] [--self-test] [-]" >&2
  exit 2
}

# summary reads "job<TAB>seconds" records and prints one row per job, with
# nearest-rank percentiles so a p90 of n=10 runs is the 9th duration.
summary() {
  sort -k1,1 -k2,2n | awk -F'\t' '
    $1 != job { flush() }
    { job = $1; n++; a[n] = $2 + 0 }
    function rank(p,   i) {
      i = int((n * p + 99) / 100)
      return i < 1 ? 1 : (i > n ? n : i)
    }
    function flush() {
      if (n == 0) return
      printf "%-24s %4d %6d %6d %6d\n", job, n, a[rank(50)], a[rank(90)], a[n]
      n = 0
    }
    END { flush() }
  ' | { printf '%-24s %4s %6s %6s %6s\n' job runs p50 p90 max; cat; }
}

self_test() {
  # build: p50 = 3rd of 5, p90 = 5th of 5. test: p50 = 2nd of 3, p90 = 3rd.
  out=$(printf 'build\t10\nbuild\t20\nbuild\t30\nbuild\t40\nbuild\t50\ntest\t5\ntest\t5\ntest\t121\n' | summary)
  row() { printf '%s\n' "$out" | awk -v j="$1" '$1 == j { print $3, $4, $5 }'; }
  got=$(row build)
  [ "$got" = "30 50 50" ] || { echo "ci-timings: build row is '$got', want '30 50 50'" >&2; exit 1; }
  got=$(row test)
  [ "$got" = "5 121 121" ] || { echo "ci-timings: test row is '$got', want '5 121 121'" >&2; exit 1; }
  [ "$(printf '%s\n' "$out" | wc -l | tr -d ' ')" -eq 3 ] ||
    { echo "ci-timings: expected one header and two rows" >&2; exit 1; }
  echo "ci-timings: self-test ok"
}

stdin_mode=
while [ $# -gt 0 ]; do
  case $1 in
    --days) [ $# -ge 2 ] || usage; days=$2; shift 2 ;;
    --limit) [ $# -ge 2 ] || usage; limit=$2; shift 2 ;;
    --self-test) self_test; exit 0 ;;
    -) stdin_mode=1; shift ;;
    *) usage ;;
  esac
done

[ -n "$stdin_mode" ] && { summary; exit 0; }

command -v gh >/dev/null 2>&1 || { echo "ci-timings: gh is not on PATH" >&2; exit 2; }
command -v jq >/dev/null 2>&1 || { echo "ci-timings: jq is not on PATH" >&2; exit 2; }

repo=${GITHUB_REPOSITORY:-$(gh repo view --json nameWithOwner -q .nameWithOwner)}
cutoff=$(($(date +%s) - days * 86400))

runs=$(gh run list --repo "$repo" --workflow ci.yml --limit "$limit" \
  --json databaseId,createdAt |
  jq -r --argjson cutoff "$cutoff" \
    '.[] | select((.createdAt | fromdate) >= $cutoff) | .databaseId')

[ -n "$runs" ] || { echo "ci-timings: no ci.yml run in the last $days days" >&2; exit 0; }

# Skipped and cancelled jobs are excluded: a cancelled job's window is
# truncated, so it would report a false speed-up.
for id in $runs; do
  gh api "repos/$repo/actions/runs/$id/jobs?per_page=100" --jq \
    '.jobs[] | select(.started_at != null and .completed_at != null
       and .conclusion != "skipped" and .conclusion != "cancelled")
     | [.name, ((.completed_at | fromdate) - (.started_at | fromdate))] | @tsv'
done | summary
