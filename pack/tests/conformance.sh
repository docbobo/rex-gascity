#!/bin/sh
# gc's RPP conformance for the rex runtime provider pack.
#
#   pack/tests/conformance.sh [--binary <path>]
#
# Never builds. The binary comes from --binary, else gc-runtime-rex on PATH.
# It runs in a per-attempt scratch city and state root, so a fetched pack/
# (pack.toml, no go.mod, no source, no repo checkout) is enough. Python 3 reads
# the JSON report. gc supplies GC_EXEC_CITY_NAME, so GC_REX_CITY is never set
# here.
#
# Exit 0 means gc reported ok:true with no failed requirement. v2_ready and
# v2_failed are recorded as evidence, never gated by this script: v2_failed
# names the v2 requirements gc counted as unmet.
set -eu

# A run from inside a Rex session inherits REX_SERVER, REX_SESSION, REX_BLOCK
# and GC_REX_CITY. They point the provider at the caller's live server and
# make is-attached and kill-object see foreign clients and sessions, so drop
# them: this run must test its own scratch city.
unset REX_SERVER REX_SESSION REX_BLOCK GC_REX_CITY

here=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
pack=$(CDPATH= cd -- "$here/.." && pwd)
gc=${GC_BIN:-gc}
binary=
while [ $# -gt 0 ]; do
  case $1 in
    --binary)
      [ $# -ge 2 ] || { echo "conformance: --binary needs a path" >&2; exit 2; }
      binary=$2
      shift 2
      ;;
    -h|--help)
      sed -n '2,12p' "$0"
      exit 0
      ;;
    *)
      echo "usage: $0 [--binary <path>]" >&2
      exit 2
      ;;
  esac
done

if [ -z "$binary" ]; then
  binary=$(command -v gc-runtime-rex || true)
fi
[ -n "$binary" ] || { echo "conformance: gc-runtime-rex is not on PATH; pass --binary" >&2; exit 2; }
[ -x "$binary" ] || { echo "conformance: $binary is not executable" >&2; exit 2; }
command -v "$gc" >/dev/null 2>&1 || { echo "conformance: gc is not on PATH (set GC_BIN)" >&2; exit 2; }
command -v python3 >/dev/null 2>&1 || { echo "conformance: python3 is required to read the JSON report" >&2; exit 2; }

# An absolute scratch root: with TMPDIR=. mktemp prints a relative path, and
# gc rejects a relative GC_EXEC_STATE_DIR.
scratch=$(CDPATH= cd -- "$(mktemp -d "${TMPDIR:-/tmp}/rexconf.XXXXXX")" && pwd -P)
trap 'rm -rf "$scratch"' EXIT INT TERM

report=$scratch/conformance.json
log=$scratch/conformance.log

# gc needs a city that includes the pack to resolve [runtimes.rex]. Build one
# per attempt, under the scratch root, so no attempt reuses another's city.
build_city() { # <dir>
  mkdir -p "$1"
  printf '[workspace]\nname = "rex-conf"\nincludes = ["%s"]\n' "$pack" > "$1/city.toml"
}

# report_failures names every failed requirement, by code and detail, before a
# retry overwrites the report. The JSON report is the only place gc says what
# went wrong, so a flake that the retry absorbs would otherwise leave nothing
# in the CI log to diagnose it with.
report_failures() {
  python3 - "$report" <<'PY' || true
import json, sys

try:
    with open(sys.argv[1]) as f:
        report = json.load(f)
except (OSError, ValueError):
    sys.exit(0)
for res in report.get("results", []):
    if res.get("status", "").upper() == "FAIL":
        print("conformance: failed: %s \u2014 %s" % (res.get("code"), res.get("detail", "")))
PY
}

# One retry. A driving op may already have typed when it fails, so it must not
# be retried in place; the whole run repeats instead, in its own scratch city
# and state root. A genuinely broken provider fails both attempts.
attempts=0
while :; do
  attempts=$((attempts + 1))
  status=0
  # Each attempt gets its own city and state root: a driving op may already
  # have typed when it fails, and its input or session would otherwise stay in
  # the shared state root and make the retry fail the same way.
  city=$scratch/city-$attempts
  build_city "$city"
  export GC_EXEC_STATE_DIR=$scratch/state-$attempts
  "$gc" runtime conformance "$binary" --city "$city" --json > "$report" 2> "$log" || status=$?
  [ ! -s "$log" ] || cat "$log" >&2
  if [ "$status" -eq 0 ]; then
    break
  fi
  report_failures
  if [ "$attempts" -ge 2 ]; then
    break
  fi
  echo "conformance: attempt $attempts failed (exit $status); retrying once" >&2
done

version=$("$binary" version 2>/dev/null | sed -n 's/.*"version"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' || true)
gc_id=${GASCITY_PIN:-$("$gc" version 2>/dev/null || echo unknown)}
summary=$scratch/summary.md
if ! {
  echo "### rex conformance"
  echo "- gc: $gc_id"
  echo "- provider: $binary (version ${version:-unknown})"
  [ "$attempts" -eq 1 ] || echo "- attempts: $attempts"
  python3 - "$report" <<'PY'
import json, sys

with open(sys.argv[1]) as f:
    report = json.load(f)
summary = report["summary"]
print("ok: %s" % str(report["ok"]).lower())
print("passed: %d" % summary["passed"])
print("failed: %d" % summary["failed"])
print("skipped: %d" % summary["skipped"])
print("v2_ready: %s" % str(report["v2_ready"]).lower())
print("v2_failed: %s" % json.dumps(report.get("v2_failed", [])))
PY
} > "$summary"; then
  echo "conformance: gc produced no JSON report (exit $status)" >&2
  exit 1
fi

cat "$summary"
if [ -n "${GITHUB_STEP_SUMMARY:-}" ]; then
  cat "$summary" >> "$GITHUB_STEP_SUMMARY"
fi

[ "$status" -eq 0 ] || exit "$status"
grep -qx 'ok: true' "$summary"
grep -qx 'failed: 0' "$summary"
