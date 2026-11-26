#!/bin/sh
# pack/commands/dashboard/{run.sh,dashboard.py} against a fake `bd` and `gc`.
#
# The fake bd serves a small fixture graph and logs its argv, so each case can
# read which rig it was pointed at. The fake gc answers `gc rig list --json`,
# which drives the current-directory rig discovery.
#
# Usage: sh pack/tests/check-dashboard.sh
# Exits non-zero with a one-line reason on the first failed assertion.
set -eu

repo=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd -P)
cmd=$repo/pack/commands/dashboard/run.sh
script=$repo/pack/commands/dashboard/dashboard.py
work=$(mktemp -d "${TMPDIR:-/tmp}/check-dashboard.XXXXXX")
trap 'rm -rf "$work"' EXIT HUP INT TERM

fail() {
  echo "check-dashboard: $*" >&2
  exit 1
}

python=$(command -v python3) || fail "python3 not on PATH"
pybin=$(dirname "$python")

tools=$work/tools
mkdir -p "$tools/rig" "$tools/rig/nested" "$work/outside"
rig_real=$(CDPATH= cd -- "$tools/rig" && pwd -P)

fixture=$work/fixture.jsonl
cat > "$fixture" <<'JSONL'
{"_type":"issue","id":"rig-aaa","title":"Origin alpha","status":"open","issue_type":"task","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}
{"_type":"issue","id":"rig-bbb","title":"Build the thing","status":"in_progress","issue_type":"task","created_at":"2026-01-01T00:00:00Z","started_at":"2026-01-01T00:05:00Z","updated_at":"2026-01-01T00:05:00Z","metadata":{"gc.root_bead_id":"rig-aaa"}}
{"_type":"issue","id":"rig-ccc","title":"Finished child","status":"closed","issue_type":"task","created_at":"2026-01-01T00:00:00Z","closed_at":"2026-01-01T00:10:00Z","metadata":{"gc.root_bead_id":"rig-aaa"}}
JSONL

# Fake bd: log argv (one word per line), serve the fixture on `export`, and
# answer `context` when asked. BD_FAKE_EXIT makes it fail with that status.
cat > "$tools/bd" <<'FAKE'
#!/bin/sh
for word in "$@"; do printf '%s\n' "$word"; done >> "$BD_LOG"
exit_code=0
mode=
while [ "$#" -gt 0 ]; do
  case $1 in
    -C) shift ;;
    export) mode=export; shift ;;
    context) mode=context; shift ;;
    *) shift ;;
  esac
done
if [ -n "${BD_FAKE_EXIT:-}" ]; then
  echo boom >&2
  exit "$BD_FAKE_EXIT"
fi
case $mode in
  export) cat "$DASH_FIXTURE" ;;
  context) printf 'repo root: %s\ndatabase: %s\n' "$DASH_RIG" fake ;;
esac
FAKE

# Fake gc: `gc rig list --json` reports one rig whose path is $DASH_RIG.
# GC_FAKE_EXIT makes it fail with that status.
cat > "$tools/gc" <<'FAKE'
#!/bin/sh
exit_code=0
case "$1 $2" in
  "rig list") ;;
  *) exit 0 ;;
esac
if [ -n "${GC_FAKE_EXIT:-}" ]; then
  echo "gc rig list: nope" >&2
  exit "$GC_FAKE_EXIT"
fi
printf '{"schema_version":"1","ok":true,"rigs":[{"name":"rig","path":"%s","prefix":"rig","hq":false,"beads":"initialized"}]}\n' "$DASH_RIG"
FAKE
chmod +x "$tools/bd" "$tools/gc"

bdlog=$work/bd.argv
# run <cwd> <args...>: run dashboard.py with the fakes first on PATH. Its
# stdout lands in $work/out, stderr in $work/err, and status in $code.
run() {
  cwd=$1
  shift
  run_fx "$fixture" "$cwd" "$@"
}

# run_fx <fixture> <cwd> <args...>: like run, against another fixture file.
run_fx() {
  fx=$1
  shift
  cwd=$1
  shift
  rm -f "$bdlog"
  code=0
  (
    cd "$cwd" || exit 1
    PATH="$tools:$pybin:/usr/bin:/bin" GC_BIN= GC_FAKE_EXIT= BD_FAKE_EXIT= \
      DASH_FIXTURE="$fx" DASH_RIG="$tools/rig" BD_LOG="$bdlog" \
      "$python" "$script" "$@" > "$work/out" 2> "$work/err"
  ) || code=$?
}

# run_pack <cwd> <args...>: same, but through the run.sh wrapper.
run_pack() {
  cwd=$1
  shift
  rm -f "$bdlog"
  code=0
  (
    cd "$cwd" || exit 1
    PATH="$tools:$pybin:/usr/bin:/bin" GC_BIN= GC_FAKE_EXIT= BD_FAKE_EXIT= \
      DASH_FIXTURE="$fixture" DASH_RIG="$tools/rig" BD_LOG="$bdlog" \
      GC_PACK_DIR= sh "$cmd" "$@" > "$work/out" 2> "$work/err"
  ) || code=$?
}

has() { # has <file> <text> <what>
  grep -qF -- "$2" "$1" || fail "$3: $(basename "$1") lacks: $2"
}
code_is() { # code_is <want> <what>
  [ "$code" -eq "$1" ] || fail "$2: exit $code, want $1 ($(cat "$work/err"))"
}
bd_got() { # bd_got <word> <what>
  grep -qxF -- "$1" "$bdlog" || fail "$2: bd argv lacks: $1 ($(tr '\n' ' ' < "$bdlog"))"
}
bd_lacks() { # bd_lacks <word> <what>
  if grep -qxF -- "$1" "$bdlog"; then fail "$2: bd argv has: $1"; fi
}

# --- explicit --rig -----------------------------------------------------------

run "$work/outside" --width 80 --no-color -C "$tools/rig"
code_is 0 "explicit --rig"
has "$work/out" "Origin alpha" "explicit --rig"
has "$work/out" "Build the thing" "explicit --rig"
has "$work/out" "1 in progress" "explicit --rig"
bd_got "export" "explicit --rig"
bd_got "$tools/rig" "explicit --rig"

# --- current-directory discovery via gc rig list ------------------------------

run "$tools/rig/nested" --width 80 --no-color
code_is 0 "discovery from a rig subdirectory"
has "$work/out" "Origin alpha" "discovery"
bd_got "$rig_real" "discovery points bd at the rig"

run "$tools/rig" --width 80 --no-color
code_is 0 "discovery from the rig root"
bd_got "$rig_real" "discovery from the rig root"

# --- discovery falls back when gc is not in a city ----------------------------

code=0
rm -f "$bdlog"
(
  cd "$work/outside" || exit 1
  PATH="$tools:$pybin:/usr/bin:/bin" GC_BIN= GC_FAKE_EXIT=1 \
    DASH_FIXTURE="$fixture" DASH_RIG="$tools/rig" BD_LOG="$bdlog" \
    "$python" "$script" --width 80 --no-color > "$work/out" 2> "$work/err"
) || code=$?
code_is 0 "gc rig list failure falls back to cwd"
bd_lacks "-C" "fallback runs bd without -C"

# --- the run.sh wrapper -------------------------------------------------------

run_pack "$tools/rig" --width 80 --no-color
code_is 0 "run.sh wrapper"
has "$work/out" "Origin alpha" "run.sh wrapper"

# --- work items without a workflow ancestor, and focus beads by ancestry ------
#
# grp-wi is tracked by grp-cv but has no workflow ancestor. grp-step sits
# under grp-wf2 while a second workflow root exists. grp-loose has no
# workflow ancestor at all. Both layouts must show all three, with grp-step
# under grp-wf2 and grp-loose at the origin.

grouping=$work/fixture-group.jsonl
cat > "$grouping" <<'JSONL'
{"_type":"issue","id":"grp-org","title":"Origin stream","status":"open","issue_type":"task","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}
{"_type":"issue","id":"grp-wf","title":"Workflow one","status":"open","issue_type":"task","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:01:00Z","metadata":{"gc.kind":"workflow","gc.root_bead_id":"grp-org","gc.formula_name":"build-basic"}}
{"_type":"issue","id":"grp-wf2","title":"Workflow two","status":"open","issue_type":"task","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:01:00Z","metadata":{"gc.kind":"workflow","gc.root_bead_id":"grp-org","gc.formula_name":"review"}}
{"_type":"issue","id":"grp-cv","title":"Convoy for the work item","status":"open","issue_type":"convoy","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:02:00Z","dependencies":[{"type":"tracks","depends_on_id":"grp-wi"}],"metadata":{"gc.root_bead_id":"grp-org"}}
{"_type":"issue","id":"grp-wi","title":"Tracked work item","status":"in_progress","issue_type":"task","created_at":"2026-01-01T00:00:00Z","started_at":"2026-01-01T00:02:00Z","updated_at":"2026-01-01T00:02:00Z","metadata":{"gc.root_bead_id":"grp-org"}}
{"_type":"issue","id":"grp-focus","title":"Focus under work item","status":"in_progress","issue_type":"task","created_at":"2026-01-01T00:00:00Z","started_at":"2026-01-01T00:02:00Z","updated_at":"2026-01-01T00:02:00Z","metadata":{"gc.root_bead_id":"grp-wi"}}
{"_type":"issue","id":"grp-step","title":"Step under workflow two","status":"in_progress","issue_type":"task","created_at":"2026-01-01T00:00:00Z","started_at":"2026-01-01T00:02:00Z","updated_at":"2026-01-01T00:02:00Z","metadata":{"gc.root_bead_id":"grp-wf2"}}
{"_type":"issue","id":"grp-loose","title":"Step at the origin","status":"in_progress","issue_type":"task","created_at":"2026-01-01T00:00:00Z","started_at":"2026-01-01T00:02:00Z","updated_at":"2026-01-01T00:02:00Z","metadata":{"gc.root_bead_id":"grp-org"}}
JSONL

once() { # once <file> <text> <what>
  n=$(grep -cF -- "$2" "$1" || true)
  [ "$n" -eq 1 ] || fail "$3: $2 appears $n times, want 1"
}
before() { # before <file> <first> <second> <what>
  a=$(grep -nF -- "$2" "$1" | head -1 | cut -d: -f1)
  b=$(grep -nF -- "$3" "$1" | head -1 | cut -d: -f1)
  [ -n "$a" ] && [ -n "$b" ] && [ "$a" -lt "$b" ] || fail "$4: want $2 before $3"
}
indent_is() { # indent_is <file> <text> <spaces> <what>
  n=$(awk -v t="$2" 'index($0, t) { match($0, /^ */); print RLENGTH; exit }' "$1")
  [ "$n" = "$3" ] || fail "$4: $2 indent is $n, want $3"
}

run_fx "$grouping" "$work/outside" --width 120 --no-color --layout tree -C "$tools/rig"
code_is 0 "grouping tree layout"
once "$work/out" grp-wi "grouping tree shows the orphan work item"
once "$work/out" grp-focus "grouping tree shows its focus bead"
once "$work/out" grp-step "grouping tree shows the step under workflow two"
once "$work/out" grp-loose "grouping tree shows the origin step"
before "$work/out" grp-wf2 grp-step "grouping tree step follows workflow two"
before "$work/out" grp-step grp-wi "grouping tree origin bucket follows the workflows"
indent_is "$work/out" grp-step 6 "grouping tree nests the step under the workflow"
indent_is "$work/out" grp-wi 3 "grouping tree puts the orphan work item at the origin"
indent_is "$work/out" grp-loose 3 "grouping tree puts the origin step at the origin"

run_fx "$grouping" "$work/outside" --width 120 --no-color --layout box -C "$tools/rig"
code_is 0 "grouping box layout"
once "$work/out" grp-wi "grouping box shows the orphan work item"
once "$work/out" grp-focus "grouping box shows its focus bead"
once "$work/out" grp-step "grouping box shows the step under workflow two"
once "$work/out" grp-loose "grouping box shows the origin step"
before "$work/out" grp-wf2 grp-step "grouping box step follows workflow two"
before "$work/out" grp-step grp-wi "grouping box origin bucket follows the workflows"

# --- --json -------------------------------------------------------------------

run "$work/outside" --json -C "$tools/rig"
code_is 0 "--json"
cat > "$work/want.json" <<'JSON'
{
  "rig-aaa": [
    "rig-aaa",
    "rig-bbb"
  ]
}
JSON
if ! cmp -s "$work/out" "$work/want.json"; then
  fail "--json payload is not the expected model ($(cat "$work/out"))"
fi

# --- errors -------------------------------------------------------------------

run "$work/outside" --width 80 --no-color -C "$tools/rig" --nope
code_is 2 "unknown flag is a usage error"

code=0
(
  cd "$work/outside" || exit 1
  PATH="$tools:$pybin:/usr/bin:/bin" GC_BIN= GC_FAKE_EXIT=1 BD_FAKE_EXIT=3 \
    DASH_FIXTURE="$fixture" DASH_RIG="$tools/rig" BD_LOG="$bdlog" \
    "$python" "$script" --width 80 --no-color > "$work/out" 2> "$work/err"
) || code=$?
[ "$code" -eq 1 ] || fail "bd failure: exit $code, want 1"
has "$work/err" "failed" "bd failure message"

echo "check-dashboard: ok"
