#!/bin/sh
# Print per-method provider call statistics from one or more JSONL trace files
# written by GC_REX_TRACE (see docs/provider-trace.md).
#
# Usage:
#   scripts/provider-trace-stats.sh <file.jsonl> [<file.jsonl> ...]
#
# Output: one header line, then one row per method, sorted by total time:
#   method count total_us p50_us p95_us max_us
#
# Scoping to one city and one period is input selection, not a flag:
#   - The default daily path (<state dir>/trace/provider-<YYYY-MM-DD>.jsonl)
#     already scopes by city (state dir) and day (filename): pass that file.
#   - For a literal shared path, pre-filter with jq and pipe it in:
#       jq -c 'select(.city=="my-city" and .ts>="2026-11-22T00:00:00Z"
#              and .ts<"2026-11-23T00:00:00Z")' trace.jsonl |
#         scripts/provider-trace-stats.sh /dev/stdin
#
# Needs jq on PATH.
set -eu

if [ "$#" -lt 1 ]; then
  echo "usage: $0 <file.jsonl> [<file.jsonl> ...]" >&2
  exit 2
fi
command -v jq >/dev/null 2>&1 || {
  echo "provider-trace-stats: jq is not on PATH" >&2
  exit 2
}

# pctl takes the sorted durations, the count and the percentile (0-100) and
# returns the nearest-rank element.
pctl() {
  jq -r --argjson n "${2:-0}" --argjson p "${3:-50}" '
    if $n == 0 then 0
    else .[((($n * $p + 99) / 100 | floor) - 1 | if . < 0 then 0 else . end)]
    end'
}

jq -s -r '
  group_by(.method)
  | map({
      method: .[0].method,
      count: length,
      total: (map(.duration_us) | add),
      durs: (map(.duration_us) | sort)
    })
  | sort_by(-.total)
  | .[] | [.method, .count, .total, (.durs | tojson)]
  | @tsv
' "$@" |
while IFS='	' read -r method count total durs; do
  p50=$(printf '%s' "$durs" | pctl "$method" "$count" 50)
  p95=$(printf '%s' "$durs" | pctl "$method" "$count" 95)
  max=$(printf '%s' "$durs" | jq -r '.[-1]')
  echo "$method $count $total $p50 $p95 $max"
done |
{ echo "method count total_us p50_us p95_us max_us"; cat; }
