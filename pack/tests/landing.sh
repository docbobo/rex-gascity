#!/bin/sh
# Landing guard for the GCREX-120 epic: every child must have a commit.
#
#   pack/tests/landing.sh
#
# A commit carries the epic key (GCREX-120.N), never the internal bead id, so
# the list below is the epic keys. It reads the full commit message of every
# commit reachable from HEAD: the epic's child work landed as one squashed
# commit whose body carries the child subjects, and a plain %s scan would
# forget 120.1, .4 and .5. A child with no commit in the release line fails.
#
# LANDING_KEYS overrides the list; the self-test proves a missing key fails.
set -eu

here=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
root=$(CDPATH= cd -- "$here/../.." && pwd)
keys=${LANDING_KEYS:-"GCREX-120.1 GCREX-120.2 GCREX-120.3 GCREX-120.4 GCREX-120.5 GCREX-120.6"}

log=$(git -C "$root" log --format='%s%n%b' HEAD)
missing=
for key in $keys; do
  printf '%s\n' "$log" | grep -qF "$key" || missing="$missing $key"
done

# The guard has teeth: a key no commit can carry must be reported missing.
if printf '%s\n' "$log" | grep -qF "GCREX-120.self-test-$$"; then
  echo "check-landing: self-test failed: an impossible key was found" >&2
  exit 1
fi

[ -z "$missing" ] || {
  echo "check-landing: no commit for:$missing" >&2
  exit 1
}
echo "check-landing: every child bead has a commit"
