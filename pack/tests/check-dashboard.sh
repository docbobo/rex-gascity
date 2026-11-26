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

# Python before 3.12 rejects a backslash inside an f-string replacement field.
# That is a SyntaxError at load, before argument parsing, so --help and --json
# would fail on a 3.9 runtime. Keep such escapes out of f-strings.
if ! "$python" - "$script" <<'PY'
import re, sys

src = open(sys.argv[1]).read()
bad = [
    (n, line)
    for n, line in enumerate(src.splitlines(), 1)
    if re.search(r"f['\"]", line) and re.search(r"\{[^}]*\\", line)
]
for n, line in bad:
    print(f"{sys.argv[1]}:{n}: {line.strip()}", file=sys.stderr)
sys.exit(1 if bad else 0)
PY
then
  fail "f-string replacement field contains a backslash (needs Python 3.12+)"
fi

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

# --- deep convoy chains do not overflow the recursion limit ------------------

deep=$work/fixture-deep.jsonl
"$python" - "$deep" <<'PY'
import json
import sys

rows = []
n = 1200


def add(**kw):
    kw.setdefault("_type", "issue")
    kw.setdefault("created_at", "2026-01-01T00:00:00Z")
    kw.setdefault("updated_at", "2026-01-01T00:00:00Z")
    rows.append(kw)


add(
    id="deep-shared",
    title="Shared ordinary bead",
    status="in_progress",
    issue_type="task",
    started_at="2026-01-01T00:05:00Z",
)
for i in range(n):
    deps = [{"type": "tracks", "depends_on_id": "deep-shared"}]
    if i + 1 < n:
        deps.append({"type": "tracks", "depends_on_id": f"deep-cv{i + 1}"})
    add(
        id=f"deep-cv{i}",
        title=f"Convoy {i}",
        status="open",
        issue_type="convoy",
        dependencies=deps,
    )
add(
    id="deep-leaf",
    title="Leaf under the chain",
    status="in_progress",
    issue_type="task",
    started_at="2026-01-01T00:05:00Z",
    metadata={"gc.root_bead_id": "deep-cv0"},
)
with open(sys.argv[1], "w") as fh:
    for row in rows:
        fh.write(json.dumps(row) + "\n")
PY

run_fx "$deep" "$work/outside" --width 80 --no-color -C "$tools/rig"
code_is 0 "deep convoy chain"
has "$work/out" "deep-leaf" "deep convoy chain shows the leaf"

# --- convoy targets resolve independently of traversal order -----------------

order=$work/fixture-order.jsonl
cat > "$order" <<'JSONL'
{"_type":"issue","id":"ord-leaf","title":"Leaf under C","status":"in_progress","issue_type":"task","created_at":"2026-01-01T00:00:00Z","started_at":"2026-01-01T00:05:00Z","updated_at":"2026-01-01T00:05:00Z","metadata":{"gc.root_bead_id":"ord-c"}}
{"_type":"issue","id":"ord-c","title":"Convoy C","status":"open","issue_type":"convoy","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z","dependencies":[{"type":"tracks","depends_on_id":"ord-a"},{"type":"tracks","depends_on_id":"ord-b"}]}
{"_type":"issue","id":"ord-b","title":"Convoy B","status":"open","issue_type":"convoy","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z","dependencies":[{"type":"tracks","depends_on_id":"ord-a"},{"type":"tracks","depends_on_id":"ord-s"}]}
{"_type":"issue","id":"ord-a","title":"Target A","status":"open","issue_type":"task","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z","metadata":{"gc.root_bead_id":"ord-root"}}
{"_type":"issue","id":"ord-s","title":"Target S","status":"open","issue_type":"task","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z","metadata":{"gc.root_bead_id":"ord-root"}}
{"_type":"issue","id":"ord-root","title":"Shared root","status":"open","issue_type":"task","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}
JSONL

run_fx "$order" "$work/outside" --json -C "$tools/rig"
code_is 0 "convoy targets resolve independently"
cat > "$work/want-order.json" <<'JSON'
{
  "ord-root": [
    "ord-a",
    "ord-b",
    "ord-c",
    "ord-leaf",
    "ord-root",
    "ord-s"
  ]
}
JSON
if ! cmp -s "$work/out" "$work/want-order.json"; then
  fail "convoy order changed the origin ($(cat "$work/out"))"
fi

# --- the span covers the stream's whole lifetime -----------------------------

span=$work/fixture-span.jsonl
"$python" - "$span" <<'PY'
import json
import sys
from datetime import datetime, timedelta, timezone

now = datetime.now(timezone.utc)


def iso(dt):
    return dt.strftime("%Y-%m-%dT%H:%M:%SZ")


rows = [
    {
        "id": "span-org",
        "title": "Span origin",
        "status": "open",
        "issue_type": "task",
        "created_at": iso(now - timedelta(days=1)),
        "updated_at": iso(now - timedelta(days=1)),
    },
    {
        "id": "span-old",
        "title": "Old closed bead",
        "status": "closed",
        "issue_type": "task",
        "created_at": iso(now - timedelta(days=10)),
        "started_at": iso(now - timedelta(days=10)),
        "closed_at": iso(now - timedelta(days=9)),
        "updated_at": iso(now - timedelta(days=9)),
        "metadata": {"gc.root_bead_id": "span-org"},
    },
    {
        "id": "span-new",
        "title": "Active sibling",
        "status": "in_progress",
        "issue_type": "task",
        "created_at": iso(now - timedelta(days=1)),
        "started_at": iso(now - timedelta(days=1)),
        "updated_at": iso(now - timedelta(days=1)),
        "metadata": {"gc.root_bead_id": "span-org"},
    },
]
with open(sys.argv[1], "w") as fh:
    for row in rows:
        fh.write(json.dumps({"_type": "issue", **row}) + "\n")
PY

run_fx "$span" "$work/outside" --width 120 --no-color -C "$tools/rig"
code_is 0 "span from lifetime members"
once "$work/out" "span 10d 0h" "span starts at the closed bead"

# --- the column count is capped to boxes that fit ----------------------------

cols=$work/fixture-columns.jsonl
"$python" - "$cols" <<'PY'
import json
import sys

rows = [
    {
        "id": f"col-{i}",
        "title": f"Stream {i}",
        "status": "in_progress",
        "issue_type": "task",
        "created_at": "2026-01-01T00:00:00Z",
        "started_at": "2026-01-01T00:05:00Z",
        "updated_at": "2026-01-01T00:05:00Z",
    }
    for i in range(4)
]
with open(sys.argv[1], "w") as fh:
    for row in rows:
        fh.write(json.dumps({"_type": "issue", **row}) + "\n")
PY

run_fx "$cols" "$work/outside" --width 80 --columns 4 --no-color -C "$tools/rig"
code_is 0 "column count fits the width"
"$python" - "$script" "$work/out" <<'PY'
import importlib.util
import sys

spec = importlib.util.spec_from_file_location("dash", sys.argv[1])
dash = importlib.util.module_from_spec(spec)
spec.loader.exec_module(dash)
wide = [line for line in open(sys.argv[2]).read().splitlines() if dash.dwidth(line) > 80]
assert not wide, "lines wider than 80 columns: %r" % (wide,)
PY

# --- long bead prefixes and long origin IDs stay inside their boxes ---------

# At --width 60 with two columns the box interior is 25 columns. A nested bead
# prefix (indent, icon, a 20-column ID, a duration) leaves no room for a title,
# and a 24-column origin ID overruns the header decorations. Both must be
# trimmed instead of pushing the row past the terminal width.
prefix=$work/fixture-prefix.jsonl
cat > "$prefix" <<'JSONL'
{"_type":"issue","id":"prf-0123456789abcdefghij","title":"Header stream","status":"open","issue_type":"task","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}
{"_type":"issue","id":"prf-wf","title":"Workflow","status":"open","issue_type":"task","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:01:00Z","metadata":{"gc.kind":"workflow","gc.root_bead_id":"prf-0123456789abcdefghij","gc.formula_name":"build-basic"}}
{"_type":"issue","id":"prf-cv","title":"Convoy","status":"open","issue_type":"convoy","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:02:00Z","dependencies":[{"type":"tracks","depends_on_id":"prf-wi-0123456789"}],"metadata":{"gc.root_bead_id":"prf-wf"}}
{"_type":"issue","id":"prf-wi-0123456789","title":"wix","status":"in_progress","issue_type":"task","created_at":"2026-01-01T00:00:00Z","started_at":"2026-01-01T00:02:00Z","updated_at":"2026-01-01T00:02:00Z","metadata":{"gc.root_bead_id":"prf-wf"}}
{"_type":"issue","id":"prf-leaf-0123456789","title":"leafx","status":"in_progress","issue_type":"task","created_at":"2026-01-01T00:00:00Z","started_at":"2026-01-01T00:02:00Z","updated_at":"2026-01-01T00:02:00Z","metadata":{"gc.root_bead_id":"prf-wi-0123456789"}}
{"_type":"issue","id":"prf-two-0123456789ab","title":"Second stream","status":"in_progress","issue_type":"task","created_at":"2026-01-01T00:00:00Z","started_at":"2026-01-01T00:03:00Z","updated_at":"2026-01-01T00:03:00Z"}
JSONL

run_fx "$prefix" "$work/outside" --width 60 --columns 2 --no-color -C "$tools/rig"
code_is 0 "long prefixes stay inside the boxes"
has "$work/out" "leafx" "long prefix keeps the nested title"
"$python" - "$script" "$work/out" <<'PY'
import importlib.util
import sys

spec = importlib.util.spec_from_file_location("dash", sys.argv[1])
dash = importlib.util.module_from_spec(spec)
spec.loader.exec_module(dash)
wide = [line for line in open(sys.argv[2]).read().splitlines() if dash.dwidth(line) > 60]
assert not wide, "lines wider than 60 columns: %r" % (wide,)

PY

run_fx "$prefix" "$work/outside" --width 60 --layout tree --no-color -C "$tools/rig"
code_is 0 "long prefixes stay inside the tree rows"
"$python" - "$script" "$work/out" <<'PY'
import importlib.util
import sys

spec = importlib.util.spec_from_file_location("dash", sys.argv[1])
dash = importlib.util.module_from_spec(spec)
spec.loader.exec_module(dash)
wide = [line for line in open(sys.argv[2]).read().splitlines() if dash.dwidth(line) > 60]
assert not wide, "tree lines wider than 60 columns: %r" % (wide,)
PY

# --- an exhausted title budget drops the title, not the width bound --------

# A 40-column ID and an 80-column title at --width 60 leave no room for the
# title. truncate(0) means "no limit", so the row must omit the title.
huge=$work/fixture-huge.jsonl
"$python" - "$huge" <<'PY'
import json
import sys

row = {
    "id": "huge-" + "x" * 35,
    "title": "w " * 40,
    "status": "open",
    "issue_type": "task",
    "created_at": "2026-01-01T00:00:00Z",
    "updated_at": "2026-01-01T00:00:00Z",
}
with open(sys.argv[1], "w") as fh:
    fh.write(json.dumps({"_type": "issue", **row}) + "\n")
PY

run_fx "$huge" "$work/outside" --width 60 --scope all --layout tree --no-color -C "$tools/rig"
code_is 0 "huge ID and title at width 60"
"$python" - "$script" "$work/out" <<'PY'
import importlib.util
import sys

spec = importlib.util.spec_from_file_location("dash", sys.argv[1])
dash = importlib.util.module_from_spec(spec)
spec.loader.exec_module(dash)
wide = [line for line in open(sys.argv[2]).read().splitlines() if dash.dwidth(line) > 60]
assert not wide, "huge row wider than 60 columns: %r" % (wide,)
PY

# --- a drained title budget omits the title, never disables truncation ------

# A long origin ID and its meta tag can consume the whole row. The title must
# be dropped instead of passed to truncate() as width 0, which that helper
# reads as "unlimited" and renders in full.
budget=$work/fixture-title-budget.jsonl
"$python" - "$budget" <<'PY'
import json
import sys

origin = "bud-" + "a" * 36
rows = [
    {
        "id": origin,
        "title": "Title " + "b" * 74,
        "status": "open",
        "issue_type": "task",
        "created_at": "2026-01-01T00:00:00Z",
        "started_at": "2026-01-01T00:00:00Z",
        "updated_at": "2026-01-01T00:05:00Z",
    },
    {
        "id": "bud-done",
        "title": "Finished sibling",
        "status": "closed",
        "issue_type": "task",
        "created_at": "2026-01-01T00:00:00Z",
        "closed_at": "2026-01-01T00:04:00Z",
        "updated_at": "2026-01-01T00:04:00Z",
        "metadata": {"gc.root_bead_id": origin},
    },
]
with open(sys.argv[1], "w") as fh:
    for row in rows:
        fh.write(json.dumps({"_type": "issue", **row}) + "\n")
PY

run_fx "$budget" "$work/outside" --width 60 --layout tree --scope all --no-color -C "$tools/rig"
code_is 0 "drained title budget"
"$python" - "$script" "$work/out" <<'PY'
import importlib.util
import sys

spec = importlib.util.spec_from_file_location("dash", sys.argv[1])
dash = importlib.util.module_from_spec(spec)
spec.loader.exec_module(dash)
wide = [line for line in open(sys.argv[2]).read().splitlines() if dash.dwidth(line) > 60]
assert not wide, "drained-budget lines wider than 60 columns: %r" % (wide,)
PY

# --- a pinned compact row counts the icon's two columns --------------------

# The pinned icon (📌) is two columns under dwidth, but the compact-row budget
# reserved one. A pinned origin with a long ID therefore rendered one column
# wider than --width. Reserve the icon's measured width instead.
pinned=$work/fixture-pinned.jsonl
"$python" - "$pinned" <<'PY'
import json
import sys

row = {
    "id": "pin-" + "c" * 33,
    "title": "Pinned origin with a long title that needs the whole row",
    "status": "pinned",
    "issue_type": "task",
    "created_at": "2026-01-01T00:00:00Z",
    "updated_at": "2026-01-01T00:00:00Z",
}
with open(sys.argv[1], "w", encoding="utf-8") as fh:
    fh.write(json.dumps({"_type": "issue", **row}, ensure_ascii=False) + "\n")
PY

run_fx "$pinned" "$work/outside" --width 60 --status pinned --scope all \
  --layout tree --no-color -C "$tools/rig"
code_is 0 "pinned compact row at width 60"
"$python" - "$script" "$work/out" <<'PY'
import importlib.util
import sys

spec = importlib.util.spec_from_file_location("dash", sys.argv[1])
dash = importlib.util.module_from_spec(spec)
spec.loader.exec_module(dash)
wide = [
    line
    for line in open(sys.argv[2], encoding="utf-8").read().splitlines()
    if dash.dwidth(line) > 60
]
assert not wide, "pinned row wider than 60 columns: %r" % (wide,)
PY

# --- bd export records split on the JSON Lines separator only ----------------

# The Go encoder writes U+0085 literally, and str.splitlines() would split
# the record there, leaving two fragments that json.loads cannot parse.
next=$work/fixture-nextline.jsonl
"$python" - "$next" <<'PY'
import json
import sys

row = {
    "id": "nel-org",
    "title": "Next\u0085line origin",
    "status": "in_progress",
    "issue_type": "task",
    "created_at": "2026-01-01T00:00:00Z",
    "started_at": "2026-01-01T00:05:00Z",
    "updated_at": "2026-01-01T00:05:00Z",
}
with open(sys.argv[1], "w", encoding="utf-8") as fh:
    fh.write(json.dumps({"_type": "issue", **row}, ensure_ascii=False) + "\n")
PY

run_fx "$next" "$work/outside" --width 100 --no-color -C "$tools/rig"
code_is 0 "U+0085 in a title"
has "$work/out" "line origin" "U+0085 record still renders"

# --- titles cannot inject terminal control sequences ------------------------

ctl=$work/fixture-control.jsonl
"$python" - "$ctl" <<'PY'
import json
import sys

row = {
    "id": "ctl-org",
    "title": "Control \x1b[31mred\x1b[0m origin",
    "status": "in_progress",
    "issue_type": "task",
    "created_at": "2026-01-01T00:00:00Z",
    "started_at": "2026-01-01T00:05:00Z",
    "updated_at": "2026-01-01T00:05:00Z",
}
with open(sys.argv[1], "w") as fh:
    fh.write(json.dumps({"_type": "issue", **row}) + "\n")
PY

run_fx "$ctl" "$work/outside" --width 100 --no-color -C "$tools/rig"
code_is 0 "control characters in a title"
has "$work/out" "Control" "control title text is kept"
"$python" - "$work/out" <<'PY'
import sys

text = open(sys.argv[1], encoding="utf-8").read()
bad = sorted({ch for ch in text if ord(ch) < 32 and ch != "\n"})
assert not bad, "control characters reached the terminal: %r" % (bad,)
PY

# --- attention scope includes hooked work ------------------------------------

hook=$work/fixture-hooked.jsonl
cat > "$hook" <<'JSONL'
{"_type":"issue","id":"hook-org","title":"Hooked origin","status":"open","issue_type":"task","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}
{"_type":"issue","id":"hook-bead","title":"Hooked bead","status":"hooked","issue_type":"task","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:01:00Z","metadata":{"gc.root_bead_id":"hook-org"}}
JSONL

run_fx "$hook" "$work/outside" --width 100 --no-color --scope attention -C "$tools/rig"
code_is 0 "attention scope with a hooked bead"
has "$work/out" "Hooked origin" "attention scope shows the hooked stream"
has "$work/out" "Hooked bead" "attention scope shows the hooked bead"

run_fx "$hook" "$work/outside" --width 100 --no-color --layout tree --scope attention -C "$tools/rig"
code_is 0 "attention tree with a hooked bead"
has "$work/out" "Hooked bead" "attention tree shows the hooked bead"

run_fx "$hook" "$work/outside" --width 100 --no-color -C "$tools/rig"
code_is 0 "working scope with a hooked bead"
if grep -qF "Hooked origin" "$work/out"; then
  fail "working scope shows a hooked-only stream"
fi

run_fx "$hook" "$work/outside" --width 100 --no-color --scope attention --status open -C "$tools/rig"
code_is 0 "attention scope with an explicit --status"
if grep -qF "Hooked origin" "$work/out"; then
  fail "explicit --status was widened with hooked"
fi

run_fx "$hook" "$work/outside" --json --scope working -C "$tools/rig"
code_is 0 "--json with --scope"
has "$work/out" '"hook-org"' "--json is the unfiltered model"

cp "$work/out" "$work/json-scope-working.json"
run_fx "$hook" "$work/outside" --json --scope attention -C "$tools/rig"
code_is 0 "--json with --scope attention"
if ! cmp -s "$work/out" "$work/json-scope-working.json"; then
  fail "--scope attention changed JSON membership"
fi
run_fx "$hook" "$work/outside" --json --scope all -C "$tools/rig"
code_is 0 "--json with --scope all"
if ! cmp -s "$work/out" "$work/json-scope-working.json"; then
  fail "--scope all changed JSON membership"
fi
run_fx "$hook" "$work/outside" --json --status hooked -C "$tools/rig"
code_is 0 "--json with --status hooked"
has "$work/out" '"hook-bead"' "--status hooked is honored by --json"
run_fx "$hook" "$work/outside" --json --all -C "$tools/rig"
code_is 0 "--json with --all"
has "$work/out" '"hook-bead"' "--all includes hooked beads in --json"

# --- watch interval is validated ---------------------------------------------

# CodeRabbit on PR #174: with --frames 2 and a NaN interval the run printed a
# frame and then failed in time.sleep(). The guard must reject the value before
# watch mode renders anything, so assert empty stdout as well as the exit code.
run "$work/outside" --watch --frames 2 --interval nan -C "$tools/rig"
code_is 2 "nan interval is rejected"
has "$work/err" "--interval" "nan interval is a usage error"
if [ -s "$work/out" ]; then
  fail "nan interval rendered a frame before the usage error"
fi

run "$work/outside" --watch --frames 1 --interval 0 -C "$tools/rig"
code_is 2 "zero interval is rejected"
if [ -s "$work/out" ]; then
  fail "zero interval rendered a frame before the usage error"
fi

run "$work/outside" --watch --frames 1 --interval inf -C "$tools/rig"
code_is 2 "infinite interval is rejected"

# --- terminal setup errors still restore the terminal ------------------------

"$python" - "$script" <<'PY'
import importlib.util
import io
import sys
import types

spec = importlib.util.spec_from_file_location("dash", sys.argv[1])
dash = importlib.util.module_from_spec(spec)
spec.loader.exec_module(dash)


class Out(io.StringIO):
    def __init__(self, boom_at=None):
        super().__init__()
        self.boom_at = boom_at
        self.written = []

    def isatty(self):
        return True

    def write(self, text):
        if self.boom_at is not None and self.boom_at in text:
            raise KeyboardInterrupt
        self.written.append(text)
        return len(text)

    def flush(self):
        pass


class In(io.StringIO):
    def isatty(self):
        return True

    def fileno(self):
        return 0


calls = []


def install(setcbreak):
    dash._termios = types.SimpleNamespace(
        tcgetattr=lambda fd: ["saved"],
        tcsetattr=lambda fd, how, attrs: calls.append(attrs),
        TCSADRAIN=1,
    )
    dash._tty = types.SimpleNamespace(setcbreak=setcbreak)
    dash.render_frame = lambda *a, **k: "frame"
    dash.signal.signal = lambda *a, **k: None
    dash.sys.stdin = In()


def args():
    return types.SimpleNamespace(frames=0, interval=0.01)


# Ctrl-C while entering cbreak mode must not escape watch().
calls.clear()


def raise_interrupt(fd):
    raise KeyboardInterrupt


install(raise_interrupt)
dash.sys.stdout = Out()
assert dash.watch(args(), dash.Paint(False), "label") == 0
assert calls == [["saved"]], calls

# Ctrl-C while entering the alternate screen must still clear both.
calls.clear()


def ok_cbreak(fd):
    return None


install(ok_cbreak)
out = Out(boom_at="\x1b[?1049h")
dash.sys.stdout = out
assert dash.watch(args(), dash.Paint(False), "label") == 0
assert calls == [["saved"]], calls
assert "\x1b[?25h\x1b[?1049l" in "".join(out.written), out.written

# Ctrl-C during SIGWINCH handler install must still restore the handler.
# The real signal.signal installs the handler before it returns the old one,
# so an interrupt on that line must not lose the old handler.
calls.clear()
install(ok_cbreak)
sig = {"current": "previous", "installs": []}


def get_signal(signum):
    return sig["current"]


def set_signal(signum, handler):
    old = sig["current"]
    sig["current"] = handler
    sig["installs"].append(handler)
    if len(sig["installs"]) == 1:
        raise KeyboardInterrupt  # Ctrl-C after the install, before the return
    return old


dash.signal.getsignal = get_signal
dash.signal.signal = set_signal
dash.sys.stdout = Out()
assert dash.watch(args(), dash.Paint(False), "label") == 0
assert sig["current"] == "previous", sig["installs"]
PY


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
