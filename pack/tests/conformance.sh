#!/bin/sh
# gc's RPP conformance for the rex runtime provider pack.
#
#   pack/tests/conformance.sh [--binary <path>]
#
# Never builds. The binary comes from --binary, else gc-runtime-rex on PATH.
# It runs in a scratch city and state root, so a fetched pack/ (pack.toml, no
# go.mod, no source, no repo checkout) is enough. Python 3 reads the JSON
# report. gc supplies GC_EXEC_CITY_NAME, so GC_REX_CITY is never set here.
#
# Exit 0 means gc reported ok:true with no failed requirement. v2_ready and
# v2_failed are recorded as evidence, never gated by this script: v2_failed
# names the v2 requirements gc counted as unmet.
set -eu

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

city=$pack/../testdata/city
if [ ! -f "$city/city.toml" ]; then
  # A fetched pack has no repo testdata. A scratch city that includes the
  # pack gives gc everything it needs to resolve [runtimes.rex].
  city=$scratch/city
  mkdir -p "$city"
  printf '[workspace]\nname = "rex-conf"\nincludes = ["%s"]\n' "$pack" > "$city/city.toml"
fi

export GC_EXEC_STATE_DIR=$scratch/state
report=$scratch/conformance.json
log=$scratch/conformance.log
status=0
"$gc" runtime conformance "$binary" --city "$city" --json > "$report" 2> "$log" || status=$?
[ ! -s "$log" ] || cat "$log" >&2

version=$("$binary" version 2>/dev/null | sed -n 's/.*"version"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' || true)
gc_id=${GASCITY_PIN:-$("$gc" version 2>/dev/null || echo unknown)}
summary=$scratch/summary.md
if ! {
  echo "### rex conformance"
  echo "- gc: $gc_id"
  echo "- provider: $binary (version ${version:-unknown})"
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
print("v2_failed: %s" % json.dumps(report["v2_failed"]))
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
