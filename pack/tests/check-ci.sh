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

# One rex pin. The conformance job reads the version from pack/rex-min-version,
# so a second hardcoded pin or a version comparison that lets the two drift is
# the bug this guards.
ci=$repo/.github/workflows/ci.yml
grep -q 'pack/rex-min-version' "$ci" ||
  { echo "check-ci: ci.yml must read the rex pin from pack/rex-min-version" >&2; exit 1; }
if grep -qE '^[[:space:]]+REX_PIN:' "$ci"; then
  echo "check-ci: ci.yml must not hardcode REX_PIN; derive it from pack/rex-min-version" >&2
  exit 1
fi
if grep -q 'version_cmp' "$ci"; then
  echo "check-ci: ci.yml must not compare the pin versions; one pin must be the whole story" >&2
  exit 1
fi

sh "$repo/scripts/ci-timings.sh" --self-test

# A failed collection must not read as "no runs" or a partial table. The fake
# gh answers; real jq runs the script's own filters. Skips when jq is absent.
if command -v jq >/dev/null 2>&1; then
  fake=$(mktemp -d)
  trap 'rm -rf "$fake"' 0
  mkdir "$fake/bin"
  cat > "$fake/bin/gh" <<'EOF'
#!/bin/sh
case $1 in
  run)
    case ${FAKE_GH_RUNS:-ok} in
      fail) echo "gh: HTTP 500" >&2; exit 1 ;;
      empty) echo '[]' ;;
      *) printf '[{"databaseId":1,"createdAt":"%s"},{"databaseId":2,"createdAt":"%s"}]\n' \
           "$(date -u +%Y-%m-%dT00:00:00Z)" "$(date -u +%Y-%m-%dT00:00:00Z)" ;;
    esac ;;
  api)
    case $2 in
      */runs/2/jobs*)
        if [ "${FAKE_GH_API:-ok}" = fail ]; then echo "gh: HTTP 502" >&2; exit 1; fi ;;
    esac
    printf 'build\t42\n' ;;
  *) exit 1 ;;
esac
EOF
  chmod +x "$fake/bin/gh"

  timings() { # timings <want> <desc> <fake run response> [fake api response]
    want=$1; desc=$2; runs=${3:-ok}; api=${4:-ok}; got=0
    PATH="$fake/bin:$PATH" GITHUB_REPOSITORY=o/r \
      FAKE_GH_RUNS="$runs" FAKE_GH_API="$api" \
      sh "$repo/scripts/ci-timings.sh" >/dev/null 2>&1 || got=$?
    [ "$got" -eq "$want" ] ||
      { echo "check-ci: ci-timings $desc exited $got, want $want" >&2; exit 1; }
  }

  timings 1 "with a failed run list" fail
  timings 1 "with a failed job lookup" ok fail
  timings 0 "with an empty run list" empty
  timings 0 "with a clean collection" ok
else
  echo "check-ci: skip ci-timings collection checks: jq is not on PATH"
fi
