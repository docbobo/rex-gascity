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

# A step that pipes must set `shell: bash`. The default bash -e drops a failed
# producer behind a successful consumer. The docs-only lookup then reads as a
# docs-only change, and a failed digest read as a clean report.
step_sets_shell_bash() {  # <workflow> <step name substring> <label>
  step=$(awk -v name="$2" '
    /^      - name: / {
      if (in_step) exit
      if (index($0, name)) in_step = 1
    }
    in_step
  ' "$1")
  printf '%s\n' "$step" | grep -qx '        shell: bash' ||
    { echo "check-ci: $3 step must set 'shell: bash' for pipefail" >&2; exit 1; }
}

step_sets_shell_bash "$repo/.github/workflows/ci.yml" 'Check for a docs-only change' docs-only
step_sets_shell_bash "$repo/.github/workflows/ci-timings.yml" 'Per-job p50 and p90' digest

sh "$repo/scripts/ci-timings.sh" --self-test
