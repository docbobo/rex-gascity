#!/bin/sh
# Golden-file guard for scripts/provider-trace-stats.sh: the reader over the
# fixed sample must print the golden table exactly. The golden asserts the
# per-method total duration_us, not only the percentiles (REQ-008, AC-3).
#
# Usage: sh pack/tests/check-trace-stats.sh
# Skips with exit 0 when jq is not on PATH.
set -eu

repo=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)

command -v jq >/dev/null 2>&1 || {
  echo "check-trace-stats: skip: jq is not on PATH"
  exit 0
}

fail() {
  echo "check-trace-stats: $*" >&2
  exit 1
}

sample=$repo/testdata/provider-trace/sample.jsonl
golden=$repo/testdata/provider-trace/sample.golden
[ -f "$sample" ] || fail "missing $sample"
[ -f "$golden" ] || fail "missing $golden"

actual=$(sh "$repo/scripts/provider-trace-stats.sh" "$sample")
expected=$(cat "$golden")
if [ "$actual" != "$expected" ]; then
  printf '%s\n' "check-trace-stats: reader output differs from the golden file" >&2
  printf '%s\n' "--- want" >&2
  printf '%s\n' "$expected" >&2
  printf '%s\n' "--- got" >&2
  printf '%s\n' "$actual" >&2
  exit 1
fi
