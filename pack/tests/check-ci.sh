#!/bin/sh
# Guards for the CI helpers: the docs-only decision and the timing percentiles.
# Neither needs the network, so this runs in pack/tests/verify.sh.
#
# A docs-only misjudgment would skip conformance on a code change, so the
# path rules are checked both ways here, including the empty list.
set -eu

repo=$(dirname -- "$0")/../..
docs_only=$repo/scripts/docs-only.sh

expect() { # expect <exit status> <description>; paths arrive on stdin
  want=$1
  desc=$2
  got=0
  sh "$docs_only" >/dev/null 2>&1 || got=$?
  [ "$got" -eq "$want" ] ||
    { echo "check-ci: $desc exited $got, want $want" >&2; exit 1; }
}

printf 'README.md\ndocs/ci.md\nplans/next.md\nAGENTS.md\n' | expect 0 "documentation paths"
printf 'docs/ci.md\ninternal/provider/session.go\n' | expect 1 "docs plus Go"
printf 'pack/pack.toml\n' | expect 1 "pack file"
printf '.github/workflows/ci.yml\n' | expect 1 "workflow file"
printf 'internal/provider/session.go\npack/pack.toml\n' | expect 1 "renamed-out code file"
: | expect 1 "empty file list"

# The docs-only step pipes `gh api` into docs-only.sh. The default bash -e drops
# a failed lookup behind a successful script, so the job skips conformance on an
# unreadable file list. An explicit `shell: bash` turns on pipefail.
docs_step=$(awk '
  /^      - name: / {
    if (in_step) exit
    if ($0 ~ /Check for a docs-only change/) in_step = 1
  }
  in_step
' "$repo/.github/workflows/ci.yml")
printf '%s\n' "$docs_step" | grep -qx '        shell: bash' ||
  { echo "check-ci: docs-only step must set 'shell: bash' for pipefail" >&2; exit 1; }

sh "$repo/scripts/ci-timings.sh" --self-test
