#!/bin/sh
# Guard for pack/check-release-notes.sh. Runs the gate in a throwaway git repo
# with a fake gh. Proves a good draft body passes and each bad shape fails: no
# per-PR bullet, a version that is neither the target nor a published release,
# the release.sh placeholder, and a body that does not name the target.
#
# Usage: sh pack/tests/check-release-notes.sh
# Exits non-zero with a one-line reason on the first failed assertion.
set -eu

here=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
src=$(CDPATH= cd -- "$here/../.." && pwd)
script=$src/pack/check-release-notes.sh

fail() {
  echo "check-release-notes: $*" >&2
  exit 1
}

[ -f "$script" ] || fail "pack/check-release-notes.sh does not exist"
[ -x "$script" ] || fail "pack/check-release-notes.sh is not executable"

work=$(mktemp -d "${TMPDIR:-/tmp}/check-release-notes.XXXXXX")
trap 'rm -rf "$work"' EXIT INT TERM

tag=v2026.11.06
version=${tag#v}

# --- fake gh ----------------------------------------------------------------
# FAKE_BODY_JSON answers `release view`; FAKE_LIST_JSON answers `release list`.
# Every call lands in calls.log.
mkdir -p "$work/bin"
cat > "$work/bin/gh" <<'FAKEGH'
#!/bin/sh
printf '%s\n' "$*" >> "$FAKE_GH_LOG"
case "$1 $2" in
  "release view") cat "$FAKE_BODY_JSON" ;;
  "release list") cat "$FAKE_LIST_JSON" ;;
  *) echo "fake gh: unexpected call: $*" >&2; exit 64 ;;
esac
FAKEGH
chmod 0755 "$work/bin/gh"
PATH=$work/bin:$PATH
export PATH
GH=gh
export GH
FAKE_GH_LOG=$work/calls.log
export FAKE_GH_LOG
FAKE_BODY_JSON=$work/body.json
export FAKE_BODY_JSON
FAKE_LIST_JSON=$work/list.json
export FAKE_LIST_JSON
: > "$FAKE_GH_LOG"

# Published tags pass the version check. The draft tag does not: it is not
# published yet, so a body that names it is a stale or wrong target.
cat > "$FAKE_LIST_JSON" <<'JSON'
[{"tagName":"v2026.11.05","isDraft":false},
 {"tagName":"v2026.11.04","isDraft":false},
 {"tagName":"v2026.12.01","isDraft":true}]
JSON

# --- fixture repo -----------------------------------------------------------
repo=$work/repo
mkdir -p "$repo/pack"
cp "$script" "$repo/pack/check-release-notes.sh"
git init -q -b main "$repo"
git -C "$repo" remote add origin https://github.com/example/rex-gascity.git

write_body() { # write_body <text>: body file plus the gh JSON answer
  printf '%s' "$1" > "$work/body.txt"
  python3 - "$work/body.txt" > "$FAKE_BODY_JSON" <<'PY'
import json, sys
print(json.dumps({"body": open(sys.argv[1]).read(), "isDraft": True}))
PY
}

run() { # run <args...>: runs the gate, status in $rc, output in $work/out
  set +e
  (cd "$repo" && sh pack/check-release-notes.sh "$@") > "$work/out" 2>&1
  rc=$?
  set -e
}

# --- argument errors --------------------------------------------------------
run
[ "$rc" -eq 2 ] || fail "no tag exited $rc, want 2"
run "$version"
[ "$rc" -eq 2 ] || fail "a bare version exited $rc, want 2"
run "$tag" "$work/body.txt" extra
[ "$rc" -eq 2 ] || fail "an extra argument exited $rc, want 2"

# --- a good body passes -----------------------------------------------------
# The gc and rex versions carry three parts but no four-digit year, so they are
# not version tokens. The older v2026.11.05 is a published release.
good="Rex runtime pack $tag. Import it by tag and install the provider.

Requires gc 1.5.1 and rex 0.1.0-preview.22.

## Status updates

- #121 — the Gas City UI registers the action.
- #133 — the uninstall commands use the installed version.

The previous release was v2026.11.05."

write_body "$good"
: > "$FAKE_GH_LOG"
run "$tag" "$work/body.txt"
[ "$rc" -eq 0 ] || fail "a good body file failed: $(cat "$work/out")"
grep -q '2 per-PR bullet' "$work/out" ||
  fail "the gate does not report the per-PR bullet count"
[ ! -s "$FAKE_GH_LOG" ] || ! grep -q 'release view' "$FAKE_GH_LOG" ||
  fail "a body file still read the draft: $(cat "$FAKE_GH_LOG")"

# The same body read from the draft passes and pins the origin repo.
run "$tag"
[ "$rc" -eq 0 ] || fail "a good draft body failed: $(cat "$work/out")"
grep -q 'release view' "$FAKE_GH_LOG" ||
  fail "the gate did not read the draft with gh release view"
grep -q -- '--repo example/rex-gascity' "$FAKE_GH_LOG" ||
  fail "the gate did not pin the repo from the origin remote"
grep -q 'release list' "$FAKE_GH_LOG" ||
  fail "the gate did not list published releases"

# --- bad bodies fail --------------------------------------------------------
write_body "Rex runtime pack $tag. Import it by tag.

## Fixes

Nothing worth a bullet."
run "$tag"
[ "$rc" -ne 0 ] || fail "a body with no per-PR bullet passed"
grep -q 'per-PR bullet' "$work/out" ||
  fail "the no-bullet failure does not name the bullet form"

write_body "Rex runtime pack $tag. Import it by tag.

- #140 — cut from the v2026.11.26 sources."
run "$tag"
[ "$rc" -ne 0 ] || fail "a stale target version passed"
grep -q '2026\.11\.26' "$work/out" ||
  fail "the stale-version failure does not name 2026.11.26"

write_body "Rex runtime pack $tag. Import it by tag.

- #140 — supersedes v2025.12.01."
run "$tag"
[ "$rc" -ne 0 ] || fail "an unknown version passed"
grep -q '2025\.12\.01' "$work/out" ||
  fail "the unknown-version failure does not name 2025.12.01"

write_body "Rex runtime pack $tag. Import it by tag.

- #140 — prepared on v2026.12.01."
run "$tag"
[ "$rc" -ne 0 ] || fail "a draft-only release tag passed"
grep -q '2026\.12\.01' "$work/out" ||
  fail "the draft-tag failure does not name 2026.12.01"

write_body "Rex runtime pack $tag. See docs/rex-runtime-pack.md to import and install."
run "$tag"
[ "$rc" -ne 0 ] || fail "the release.sh placeholder body passed"
grep -q 'placeholder' "$work/out" ||
  fail "the placeholder failure does not name the placeholder"

write_body "Rex runtime pack. Import it by tag.

- #140 — a body that never names the target."
run "$tag"
[ "$rc" -ne 0 ] || fail "a body without the target version passed"
grep -q 'target version' "$work/out" ||
  fail "the missing-target failure does not name the target version"

# A gh read failure is a failure, not a pass.
write_body "$good"
FAKE_BODY_JSON=$work/missing.json
export FAKE_BODY_JSON
run "$tag"
[ "$rc" -ne 0 ] || fail "a gh read failure passed"

echo "check-release-notes: ok"
