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
  python3 - "$state" "$1" <<'PY'
import json, os, sys
state, draft = sys.argv[1], sys.argv[2]
assets = os.path.join(state, "assets")
names = sorted(os.listdir(assets)) if os.path.isdir(assets) else []
body = ""
p = os.path.join(state, "body")
if os.path.isfile(p):
    body = open(p).read()
print(json.dumps({
    "isDraft": draft == "true",
    "url": "https://example.test/releases/tag/v0.2.0",
    "assets": [{"name": n} for n in names],
    "body": body,
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
  "release list")
    echo '[{"tagName":"v0.1.0","isDraft":false}]'
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
# Signing key for the release tag. release.sh pushes a signed annotated tag;
# ssh signing keeps the test hermetic (no gpg keyring, no passphrase).
ssh-keygen -t ed25519 -N '' -f "$work/tagkey" -q
git -C "$repo" config gpg.format ssh
git -C "$repo" config user.signingkey "$work/tagkey.pub"
mkdir -p "$repo/pack/tests"
cp "$release" "$repo/pack/release.sh"
chmod 0755 "$repo/pack/release.sh"
cp "$src/pack/check-release-notes.sh" "$repo/pack/check-release-notes.sh"
chmod 0755 "$repo/pack/check-release-notes.sh"
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
# The real conformance.sh exits 2 on a --binary it cannot execute, and that is
# what a freshly downloaded 0644 asset is until release.sh chmods it.
[ -x "$2" ] || { echo "fake conformance: not executable: $2" >&2; exit 2; }
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
  ( cd "$repo" && PATH="$work/bin:$PATH" FAKE_GH=$state GH_REPO=example/rex-gascity "$@" )
}

reset_state() {
  rm -rf "$state"
  mkdir -p "$state/assets"
  # A draft whose notes a human already wrote. A new release and a resumed
  # draft both start from this body unless a case overrides it.
  printf '%s\n' "Rex runtime pack v0.2.0. Import it by tag." "" "- #121 — a tested bullet." > "$state/body"
  sha=$(shasum -a 256 "$work/asset" 2>/dev/null | awk '{print $1}')
  [ -n "$sha" ] || sha=$(sha256sum "$work/asset" | awk '{print $1}')
  for name in gc-runtime-rex_linux_amd64 gc-runtime-rex_linux_arm64 \
              gc-runtime-rex_darwin_amd64 gc-runtime-rex_darwin_arm64; do
    cp "$work/asset" "$state/assets/$name"
    printf '%s  %s\n' "$sha" "$name" >> "$state/assets/checksums.txt"
  done
  # The release carries the extension tree as well, and release.sh waits for it.
  # Built from the checkout so it is a real tarball with a real checksum line.
  tar -czf "$state/assets/gascity-ui.tar.gz" -C "$src" ui
  ui_sha=$(shasum -a 256 "$state/assets/gascity-ui.tar.gz" 2>/dev/null | awk '{print $1}')
  [ -n "$ui_sha" ] || ui_sha=$(sha256sum "$state/assets/gascity-ui.tar.gz" | awk '{print $1}')
  printf '%s  %s\n' "$ui_sha" gascity-ui.tar.gz >> "$state/assets/checksums.txt"
  # gh release download writes 0644; the release script must chmod the asset.
  chmod 0644 "$state"/assets/*
  git -C "$work/origin.git" tag -d v0.2.0 >/dev/null 2>&1 || true
  git -C "$repo" tag -d v0.2.0 >/dev/null 2>&1 || true
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
git -C "$repo" tag v0.2.0
git -C "$repo" push -q origin refs/tags/v0.2.0
run sh pack/release.sh v0.2.0 --resume --dry-run > "$work/out" 2>&1 ||
  fail "--resume on a draft failed: $(cat "$work/out")"
[ -z "$(mutations)" ] || fail "resume dry run mutated remote state: $(mutations)"

# A tag at another commit is not this release: refuse before any download.
reset_state
git -C "$repo" commit -q --allow-empty -m "a later commit"
git -C "$repo" tag v0.2.0 HEAD~1
git -C "$repo" push -q origin refs/tags/v0.2.0
if run sh pack/release.sh v0.2.0 --yes > "$work/out" 2>&1; then
  fail "a tag at another commit must fail without a release"
fi
grep -q "$(git -C "$repo" rev-parse HEAD~1)" "$work/out" ||
  fail "refusal does not name the tag's commit"
[ ! -f "$state/draft" ] || fail "release created a draft under a tag at another commit"
[ -z "$(grep -E '^release download' "$state/calls.log" 2>/dev/null)" ] ||
  fail "release downloaded assets before comparing the tag commit"

# The same holds for --resume: a draft and a tag that disagree cannot be
# resumed, because the assets were built from the tag's commit.
reset_state
touch "$state/draft"
git -C "$repo" tag v0.2.0 HEAD~1
git -C "$repo" push -q origin refs/tags/v0.2.0
if run sh pack/release.sh v0.2.0 --resume --yes > "$work/out" 2>&1; then
  fail "--resume must refuse a tag at another commit"
fi
grep -q "$(git -C "$repo" rev-parse HEAD~1)" "$work/out" ||
  fail "resume refusal does not name the tag's commit"
[ -z "$(grep -E '^release download' "$state/calls.log" 2>/dev/null)" ] ||
  fail "resume downloaded assets before comparing the tag commit"
[ -z "$(mutations | grep -E '^release (upload|edit)')" ] ||
  fail "resume mutated the release under a tag at another commit"
git -C "$repo" push -q origin :refs/tags/v0.2.0
git -C "$repo" tag -d v0.2.0 >/dev/null

# A draft whose origin tag is gone cannot be resumed: nothing shows the assets
# were built from this commit, so a resume would publish unverifiable bytes.
reset_state
touch "$state/draft"
if run sh pack/release.sh v0.2.0 --resume --yes > "$work/out" 2>&1; then
  fail "--resume must refuse a draft whose origin tag is missing"
fi
grep -q 'Delete the draft' "$work/out" ||
  fail "missing-tag refusal does not tell the operator to delete the draft"
[ -z "$(grep -E '^release download' "$state/calls.log" 2>/dev/null)" ] ||
  fail "resume downloaded assets without an origin tag"
[ -z "$(mutations | grep -E '^release (upload|edit)')" ] ||
  fail "resume published a draft without an origin tag"

# A body that is still release.sh's placeholder never publishes, even with
# --yes and --resume. The gate fails after the conformance run, so the draft
# and its assets stay in place for a later resume.
reset_state
touch "$state/draft"
printf '%s\n' 'Rex runtime pack v0.2.0. See docs/rex-runtime-pack.md to import and install.' > "$state/body"
git -C "$repo" tag v0.2.0
git -C "$repo" push -q origin refs/tags/v0.2.0
if run sh pack/release.sh v0.2.0 --resume --yes > "$work/out" 2>&1; then
  fail "a placeholder release body was published"
fi
grep -q 'release notes' "$work/out" ||
  fail "the placeholder body failure does not name the release notes gate"
[ -f "$state/draft" ] || fail "a placeholder body removed the draft"
[ ! -f "$state/published" ] || fail "a placeholder body published the release"
[ -z "$(mutations | grep -E '^release (upload|edit)')" ] ||
  fail "a placeholder body mutated the release"
git -C "$repo" push -q origin :refs/tags/v0.2.0
git -C "$repo" tag -d v0.2.0 >/dev/null

# git ls-remote reports an annotated tag object next to its peeled commit. Read
# the peeled ref, or an annotated tag at another commit would pass the check.
reset_state
git -C "$repo" commit -q --allow-empty -m "a later commit"
git -C "$repo" tag -a v0.2.0 -m "annotated release tag" HEAD~1
git -C "$repo" push -q origin refs/tags/v0.2.0
if run sh pack/release.sh v0.2.0 --yes > "$work/out" 2>&1; then
  fail "an annotated tag at another commit must fail"
fi
grep -q "$(git -C "$repo" rev-parse HEAD~1^{commit})" "$work/out" ||
  fail "the refusal does not name the annotated tag's commit"
[ -z "$(grep -E '^release download' "$state/calls.log" 2>/dev/null)" ] ||
  fail "release downloaded assets under an annotated tag at another commit"

touch "$state/draft"
if run sh pack/release.sh v0.2.0 --resume --yes > "$work/out" 2>&1; then
  fail "--resume must refuse an annotated tag at another commit"
fi
[ -z "$(mutations | grep -E '^release (upload|edit)')" ] ||
  fail "resume published under an annotated tag at another commit"

# An annotated tag at HEAD peels to this commit and is resumable.
git -C "$repo" push -q origin :refs/tags/v0.2.0
git -C "$repo" tag -d v0.2.0 >/dev/null
git -C "$repo" tag -a v0.2.0 -m "annotated release tag" HEAD
git -C "$repo" push -q origin refs/tags/v0.2.0
run sh pack/release.sh v0.2.0 --resume --yes > "$work/out" 2>&1 ||
  fail "--resume under an annotated tag at HEAD failed: $(cat "$work/out")"
[ -f "$state/published" ] || fail "an annotated tag at HEAD did not publish"
grep -q "tag_commit: $(git -C "$repo" rev-parse HEAD)" "$state/assets/conformance.txt" ||
  fail "an annotated tag at HEAD recorded the wrong tag_commit"

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
[ "$(git -C "$work/origin.git" rev-parse "refs/tags/v0.2.0^{commit}")" = "$(git -C "$repo" rev-parse HEAD)" ] ||
  fail "release did not push the tag at HEAD"
[ "$(git -C "$work/origin.git" cat-file -t refs/tags/v0.2.0)" = tag ] ||
  fail "release pushed a lightweight tag"
git -C "$work/origin.git" cat-file tag refs/tags/v0.2.0 | grep -q 'BEGIN SSH SIGNATURE' ||
  fail "release tag is not signed"
grep -q 'https://example.test/releases/tag/v0.2.0' "$work/out" ||
  fail "success does not print the release URL"
grep -q 'gc-runtime-rex_' "$work/out" || fail "success does not print the asset list"

# The evidence names the facts the bead's acceptance asks for.
for want in 'result: PASS' 'rex ver' 'gc ver' 'asset_sha256:' 'tag: v0.2.0' 'tag_commit:'; do
  grep -q "$want" "$state/assets/conformance.txt" ||
    fail "conformance.txt is missing '$want'"
done
grep -q 'ui_asset:' "$state/assets/conformance.txt" ||
  fail "conformance.txt does not name the UI asset"

# A UI tarball that does not match its checksums.txt line is refused, and the
# draft is left unpublished: the extension and the provider binary have to come
# from one build.
reset_state
line=$(grep -n 'gascity-ui.tar.gz' "$state/assets/checksums.txt" | head -n 1 | cut -d: -f1)
[ -n "$line" ] || fail "the fixture release has no gascity-ui.tar.gz line"
{
  head -n "$((line - 1))" "$state/assets/checksums.txt"
  printf '%s  %s\n' 0000000000000000000000000000000000000000000000000000000000000000 gascity-ui.tar.gz
  tail -n "+$((line + 1))" "$state/assets/checksums.txt"
} > "$work/checksums.tmp"
mv "$work/checksums.tmp" "$state/assets/checksums.txt"
if run sh pack/release.sh v0.2.0 --yes > "$work/out" 2>&1; then
  fail "a release with a mismatched gascity-ui.tar.gz must fail"
fi
grep -q 'sha256 mismatch for gascity-ui.tar.gz' "$work/out" ||
  fail "the mismatch refusal does not name gascity-ui.tar.gz"
[ ! -f "$state/published" ] || fail "a mismatched UI asset published the release"
[ -f "$state/draft" ] || fail "a refused UI asset removed the draft"

# --- the UI asset in the release workflow ------------------------------------
# check-release runs release.sh against a fake gh, so it never executes the
# workflow. The workflow's own steps are the only place gascity-ui.tar.gz and
# its checksum line are made. Run those two steps' scripts here, on a copy of
# the real ui/ tree, and assert what they produce.
wf=$src/.github/workflows/release.yml
[ -f "$wf" ] || fail ".github/workflows/release.yml does not exist"

step_script() { # step_script <step name>: print the step's run block
  awk -v name="      - name: $1" '
    $0 == name { in_step = 1; next }
    in_step && /^      - / { exit }
    in_step && /^        run: \|$/ { in_run = 1; next }
    in_step && in_run && /^          / { sub(/^          /, ""); print; next }
    in_step && in_run { exit }
  ' "$wf"
}

pkg=$(step_script "Package the UI")
sums=$(step_script "Write checksums.txt")
[ -n "$pkg" ] || fail "release.yml has no 'Package the UI' step with a run block"
[ -n "$sums" ] || fail "release.yml has no 'Write checksums.txt' run block"

# The gate runs both version guards, so a tag that disagrees with ui/version
# stops the release before build and upload.
gate=$(awk '/^  gate:/ { in_gate = 1 } in_gate && /^  [a-z]+:/ && !/^  gate:/ { exit } in_gate' "$wf")
printf '%s\n' "$gate" | grep -q 'sh pack/tests/check-ui-version.sh' ||
  fail "release.yml's gate does not run pack/tests/check-ui-version.sh"

tar=tar
if ! tar --version 2>/dev/null | grep -q 'GNU tar'; then
  command -v gtar >/dev/null 2>&1 && tar=gtar
fi
if ! "$tar" --version 2>/dev/null | grep -q 'GNU tar'; then
  # The workflow runs on ubuntu-latest, where tar is GNU tar. A developer
  # machine without it (macOS bsdtar) skips, and says so; CI must not skip,
  # because a silent skip there would leave the UI asset unchecked.
  if [ -n "${CI:-}" ]; then
    fail "GNU tar is required in CI: install it, or the 'Package the UI' step cannot be checked"
  fi
  echo "check-release: skip UI asset checks: GNU tar is not on PATH (install gtar)"
  echo "check-release: ok"
  exit 0
fi

[ -f "$src/ui/init.lua" ] || fail "ui/init.lua does not exist"
[ -f "$src/ui/install.sh" ] || fail "ui/install.sh does not exist"

sha_of() {
  if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | awk '{print $1}'
  else shasum -a 256 "$1" | awk '{print $1}'; fi
}

# Build in a scratch tree: the repo's ui/ plus the four provider assets the
# build jobs would have left in dist/. tar here is the GNU tar found above.
mkdir -p "$work/bin-gnu" "$work/uiwf/dist"
ln -s "$(command -v "$tar")" "$work/bin-gnu/tar"
# The workflow's checksum step runs on ubuntu-latest, so it calls sha256sum.
# Stand that in on a machine that only has shasum.
if ! command -v sha256sum >/dev/null 2>&1; then
  printf '%s\n' '#!/bin/sh' 'exec shasum -a 256 "$@"' > "$work/bin-gnu/sha256sum"
  chmod +x "$work/bin-gnu/sha256sum"
fi
cp -R "$src/ui" "$work/uiwf/ui"
for name in gc-runtime-rex_linux_amd64 gc-runtime-rex_linux_arm64 \
            gc-runtime-rex_darwin_amd64 gc-runtime-rex_darwin_arm64; do
  cp "$work/asset" "$work/uiwf/dist/$name"
done

# The same shell the runner uses; $pkg runs from the repo root, $sums in dist.
# The scripts go to files, not `bash -c`, so nothing in the YAML is re-parsed.
printf '%s\n' "$pkg" > "$work/pkg.sh"
printf '%s\n' "$sums" > "$work/sums.sh"
( cd "$work/uiwf" && PATH="$work/bin-gnu:$PATH" bash -eo pipefail "$work/pkg.sh" ) ||
  fail "the 'Package the UI' step failed"
( cd "$work/uiwf/dist" && PATH="$work/bin-gnu:$PATH" bash -eo pipefail "$work/sums.sh" ) >/dev/null ||
  fail "the 'Write checksums.txt' step failed"

ui_asset=$work/uiwf/dist/gascity-ui.tar.gz
[ -f "$ui_asset" ] || fail "the workflow did not produce gascity-ui.tar.gz"
gzip -t "$ui_asset" 2>/dev/null || fail "gascity-ui.tar.gz is not a gzip file"
listing=$(tar -tzf "$ui_asset" 2>/dev/null) || fail "gascity-ui.tar.gz is not a readable tarball"
for want in ui/init.lua ui/install.sh; do
  printf '%s\n' "$listing" | grep -qx "$want" ||
    fail "gascity-ui.tar.gz does not contain $want"
done
printf '%s\n' "$listing" | grep -qvE '^ui(/|$)' &&
  fail "gascity-ui.tar.gz has an entry outside ui/"

ui_sha=$(sha_of "$ui_asset")
grep -qx "$ui_sha  gascity-ui.tar.gz" "$work/uiwf/dist/checksums.txt" ||
  fail "checksums.txt has no sha256 line for gascity-ui.tar.gz"
for name in gc-runtime-rex_linux_amd64 gc-runtime-rex_linux_arm64 \
            gc-runtime-rex_darwin_amd64 gc-runtime-rex_darwin_arm64; do
  grep -q "  $name\$" "$work/uiwf/dist/checksums.txt" ||
    fail "checksums.txt lost the line for $name"
done

# Same tree, same bytes.
( cd "$work/uiwf" && rm dist/gascity-ui.tar.gz &&
  PATH="$work/bin-gnu:$PATH" bash -eo pipefail "$work/pkg.sh" )
[ "$(sha_of "$ui_asset")" = "$ui_sha" ] || fail "gascity-ui.tar.gz is not deterministic"

# A mismatch must be caught: a changed archive no longer matches its line.
printf 'x' >> "$ui_asset"
[ "$(sha_of "$ui_asset")" != "$ui_sha" ] || fail "sha256 did not change with the archive"
if grep -qx "$(sha_of "$ui_asset")  gascity-ui.tar.gz" "$work/uiwf/dist/checksums.txt"; then
  fail "a tampered gascity-ui.tar.gz still matches checksums.txt"
fi

echo "check-release: ok"
