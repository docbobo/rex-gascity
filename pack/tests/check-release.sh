#!/bin/sh
# Guard for pack/release.sh. Runs the release script in a throwaway git repo
# with fake gh, rex, gc and pack checks on PATH. No network, no real release.
#
# Usage: sh pack/tests/check-release.sh
# Exits non-zero with a one-line reason on the first failed assertion.
set -eu

here=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
src=$(CDPATH= cd -- "$here/../.." && pwd)
release=$src/pack/release.sh

fail() {
  echo "check-release: $*" >&2
  exit 1
}

[ -f "$release" ] || fail "pack/release.sh does not exist"
[ -x "$release" ] || fail "pack/release.sh is not executable"

work=$(mktemp -d "${TMPDIR:-/tmp}/check-release.XXXXXX")
trap 'rm -rf "$work"' EXIT INT TERM
state=$work/state

# --- fake toolchain ---------------------------------------------------------

mkdir -p "$work/bin" "$state/assets"

# One release asset, fetched once. install.sh never runs here, so any
# executable content proves the sha256 step and the conformance hand-off.
cat > "$work/asset" <<'ASSET'
#!/bin/sh
echo '{"version":"v0.2.0"}'
ASSET
chmod 0755 "$work/asset"
sum=$(shasum -a 256 "$work/asset" 2>/dev/null | awk '{print $1}')
[ -n "$sum" ] || sum=$(sha256sum "$work/asset" | awk '{print $1}')

cat > "$work/bin/gh" <<'FAKEGH'
#!/bin/sh
# Fake gh. State lives in $FAKE_GH. Records every call in calls.log.
state=$FAKE_GH
mkdir -p "$state"
printf '%s\n' "$*" >> "$state/calls.log"

assets_json() {
  python3 - "$state/assets" "$1" <<'PY'
import json, os, sys
names = sorted(os.listdir(sys.argv[1])) if os.path.isdir(sys.argv[1]) else []
print(json.dumps({
    "isDraft": sys.argv[2] == "true",
    "url": "https://example.test/releases/tag/v0.2.0",
    "assets": [{"name": n} for n in names],
}))
PY
}

case "$1 $2" in
  "release view")
    tag=$3
    if [ -f "$state/published" ]; then assets_json false
    elif [ -f "$state/draft" ]; then assets_json true
    else echo "release $tag not found" >&2; exit 1; fi
    ;;
  "release create")
    tag=$3
    touch "$state/draft"
    echo "https://example.test/releases/tag/$tag"
    ;;
  "workflow run")
    # The build produced everything; the wait loop sees it at the first poll.
    ;;
  "release download")
    tag=$3
    shift 3
    dir= pattern=
    while [ $# -gt 0 ]; do
      case $1 in
        --pattern) pattern="$pattern $2"; shift 2 ;;
        --dir) dir=$2; shift 2 ;;
        *) shift ;;
      esac
    done
    mkdir -p "$dir"
    for p in $pattern; do
      cp "$state/assets/$p" "$dir/$p" || exit 1
    done
    ;;
  "release upload")
    tag=$3
    file=$4
    cp "$file" "$state/assets/$(basename "$file")" || exit 1
    ;;
  "release edit")
    tag=$3
    case " $* " in
      *" --draft=false "*) rm -f "$state/draft"; touch "$state/published" ;;
      *) echo "fake gh: unsupported edit: $*" >&2; exit 1 ;;
    esac
    ;;
  *)
    echo "fake gh: unsupported call: $*" >&2
    exit 1
    ;;
esac
FAKEGH
chmod 0755 "$work/bin/gh"

cat > "$work/bin/rex" <<'FAKEREX'
#!/bin/sh
echo "rex 0.1.0-preview.test"
FAKEREX
chmod 0755 "$work/bin/rex"

cat > "$work/bin/gc" <<'FAKEGC'
#!/bin/sh
case "$1 $2" in
  "version --json") echo '{"version":"edge-test","commit":"testcommit"}' ;;
  "version ")      echo "edge-test" ;;
  *) echo "fake gc: unsupported call: $*" >&2; exit 1 ;;
esac
FAKEGC
chmod 0755 "$work/bin/gc"

# --- throwaway repo ---------------------------------------------------------

repo=$work/repo
git init -q "$repo"
git -C "$repo" config user.email test@example.test
git -C "$repo" config user.name "check-release"
mkdir -p "$repo/pack/tests"
cp "$release" "$repo/pack/release.sh"
chmod 0755 "$repo/pack/release.sh"
cat > "$repo/pack/pack.toml" <<'TOML'
[pack]
name = "rex"
version = "0.2.0"
schema = 2
TOML
cat > "$repo/pack/tests/verify.sh" <<'VERIFY'
#!/bin/sh
echo "fake verify"
exit "${FAKE_VERIFY_STATUS:-0}"
VERIFY
cat > "$repo/pack/tests/conformance.sh" <<'CONF'
#!/bin/sh
echo "fake conformance stdout: $*"
echo "fake conformance stderr" >&2
exit "${FAKE_CONFORMANCE_STATUS:-0}"
CONF
chmod 0755 "$repo/pack/tests/verify.sh" "$repo/pack/tests/conformance.sh"
git -C "$repo" add -A
git -C "$repo" commit -qm "test pack"
git -C "$repo" branch -M main
git init -q --bare "$work/origin.git"
git -C "$repo" remote add origin "$work/origin.git"
git -C "$repo" push -q origin main

run() {
  ( cd "$repo" && PATH="$work/bin:$PATH" FAKE_GH=$state "$@" )
}

reset_state() {
  rm -rf "$state"
  mkdir -p "$state/assets"
  sha=$(shasum -a 256 "$work/asset" 2>/dev/null | awk '{print $1}')
  [ -n "$sha" ] || sha=$(sha256sum "$work/asset" | awk '{print $1}')
  for name in gc-runtime-rex_linux_amd64 gc-runtime-rex_linux_arm64 \
              gc-runtime-rex_darwin_amd64 gc-runtime-rex_darwin_arm64; do
    cp "$work/asset" "$state/assets/$name"
    printf '%s  %s\n' "$sha" "$name" >> "$state/assets/checksums.txt"
  done
}

mutations() {
  grep -E '^release (create|upload|edit|download)|^workflow run' "$state/calls.log" 2>/dev/null || true
}

# --- cases ------------------------------------------------------------------

reset_state
if run sh pack/release.sh > "$work/out" 2>&1; then
  fail "no tag argument must fail"
fi
grep -q 'usage' "$work/out" || fail "no tag argument does not print usage"

reset_state
if run sh pack/release.sh v9.9.9 --dry-run > "$work/out" 2>&1; then
  fail "a tag that disagrees with pack.toml must fail"
fi
grep -q '0.2.0' "$work/out" || fail "version mismatch does not name pack.toml's version"

reset_state
run sh pack/release.sh v0.2.0 --dry-run > "$work/out" 2>&1 || fail "dry run failed: $(cat "$work/out")"
head=$(git -C "$repo" rev-parse HEAD)
grep -q "$head" "$work/out" || fail "dry run does not print the commit"
for name in gc-runtime-rex_linux_amd64 gc-runtime-rex_linux_arm64 \
            gc-runtime-rex_darwin_amd64 gc-runtime-rex_darwin_arm64 checksums.txt; do
  grep -q "$name" "$work/out" || fail "dry run does not print asset $name"
done
grep -q 'no remote state changed' "$work/out" || fail "dry run does not say it changed nothing"
[ -z "$(mutations)" ] || fail "dry run mutated remote state: $(mutations)"

reset_state
echo "# dirty" >> "$repo/pack/pack.toml"
if run sh pack/release.sh v0.2.0 --dry-run > "$work/out" 2>&1; then
  fail "a dirty tree must fail"
fi
git -C "$repo" checkout -q -- pack/pack.toml

reset_state
git -C "$repo" checkout -q -b feature
if run sh pack/release.sh v0.2.0 --dry-run > "$work/out" 2>&1; then
  fail "a branch other than main must fail"
fi
git -C "$repo" checkout -q main

# A published release is final: neither a normal run nor --resume may touch it.
reset_state
touch "$state/published"
if run sh pack/release.sh v0.2.0 --dry-run > "$work/out" 2>&1; then
  fail "an existing published release must fail"
fi
if run sh pack/release.sh v0.2.0 --resume --dry-run > "$work/out" 2>&1; then
  fail "--resume must not accept a published release"
fi
[ -z "$(mutations)" ] || fail "a published release was mutated: $(mutations)"

# A leftover draft needs --resume.
reset_state
touch "$state/draft"
if run sh pack/release.sh v0.2.0 --dry-run > "$work/out" 2>&1; then
  fail "an existing draft without --resume must fail"
fi
grep -q -- '--resume' "$work/out" || fail "draft refusal does not name --resume"
run sh pack/release.sh v0.2.0 --resume --dry-run > "$work/out" 2>&1 ||
  fail "--resume on a draft failed: $(cat "$work/out")"
[ -z "$(mutations)" ] || fail "resume dry run mutated remote state: $(mutations)"

# Conformance failure: evidence is attached, nothing is published, cleanup prints.
reset_state
if FAKE_CONFORMANCE_STATUS=1 run sh pack/release.sh v0.2.0 --yes > "$work/out" 2>&1; then
  fail "a failing conformance must fail the release"
fi
grep -q 'result: FAIL' "$state/assets/conformance.txt" 2>/dev/null ||
  fail "failed conformance evidence was not attached"
grep -q 'release delete v0.2.0 --cleanup-tag' "$work/out" ||
  fail "failure does not print the cleanup command"
[ -f "$state/draft" ] || fail "failure removed the draft"
[ ! -f "$state/published" ] || fail "failure published the release"
grep -q 'release edit' "$state/calls.log" && fail "failure called release edit"

# A non-interactive run must not publish without --yes.
reset_state
if run sh pack/release.sh v0.2.0 < /dev/null > "$work/out" 2>&1; then
  fail "a non-interactive run without --yes must not succeed"
fi
[ ! -f "$state/published" ] || fail "run without --yes published the release"
grep -q -- '--yes' "$work/out" || fail "approval refusal does not name --yes"

# Success: attach conformance.txt, publish, print the URL and assets.
reset_state
run sh pack/release.sh v0.2.0 --yes > "$work/out" 2>&1 ||
  fail "a passing release failed: $(cat "$work/out")"
grep -q 'result: PASS' "$state/assets/conformance.txt" 2>/dev/null ||
  fail "passing conformance evidence was not attached"
grep -q 'release edit v0.2.0 --draft=false' "$state/calls.log" ||
  fail "publish did not run gh release edit --draft=false"
[ -f "$state/published" ] || fail "release is not published"
grep -q 'https://example.test/releases/tag/v0.2.0' "$work/out" ||
  fail "success does not print the release URL"
grep -q 'gc-runtime-rex_' "$work/out" || fail "success does not print the asset list"

# The evidence names the facts the bead's acceptance asks for.
for want in 'result: PASS' 'rex ver' 'gc ver' 'asset_sha256:' 'tag: v0.2.0' 'tag_commit:'; do
  grep -q "$want" "$state/assets/conformance.txt" ||
    fail "conformance.txt is missing '$want'"
done

echo "check-release: ok"
