#!/bin/sh
# Guard for the tag import flow: import the pack straight from git by tag, then
# install the provider from the matching GitHub release. No pack registry and
# no package registry is involved.
#
# The source is a local bare repository, so the check needs no network and no
# GitHub auth. A fake curl serves the release assets, which are a real build of
# cmd/gc-runtime-rex stamped with the fixture pack version. Case C covers the
# documented sha: failure: a commit whose [pack] version has no release fails
# at install and names the tag.
#
# Usage: sh pack/tests/check-import.sh
# Skips with exit 0 when gc is not on PATH. Needs git and go.
set -eu

repo=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
pack=$repo/pack

fail() {
  echo "check-import: $*" >&2
  exit 1
}

command -v gc >/dev/null 2>&1 || {
  echo "check-import: skip: gc is not on PATH"
  exit 0
}
command -v git >/dev/null 2>&1 || fail "git is not on PATH"
command -v go >/dev/null 2>&1 || fail "go is not on PATH"
gc_bin=$(command -v gc)

version=0.2.0
unreleased=0.3.0

# /tmp, not $TMPDIR: a macOS mktemp path puts a "//" into the file:// URL.
# gc reads a "//" as the tree subpath and truncates the source.
work=$(mktemp -d /tmp/check-import.XXXXXX)
trap 'rm -rf "$work"' EXIT

# --- fixture pack repository ------------------------------------------------

# The pack sits at the repository root, so the bare repository is a valid pack
# source without a tree subpath. The real pack files are copied, so install.sh
# and the doctor check are the shipped ones. Only the version differs, because
# the fixture tag must agree with [pack] version.
set_version() {
  sed "s/^version = \"[^\"]*\"/version = \"$2\"/" "$1" > "$1.tmp"
  mv "$1.tmp" "$1"
}

src=$work/src
mkdir -p "$src"
cp -R "$pack/." "$src/"
rm -rf "$src/tests"
set_version "$src/pack.toml" "$version"

git -C "$src" init -q
git -C "$src" add -A
git -C "$src" -c user.email=check@example.invalid -c user.name=check \
  commit -qm "pack $version"
git -C "$src" tag "v$version"
released=$(git -C "$src" rev-parse "v$version^{commit}")

# A second commit whose [pack] version has no release. The sha: import of this
# commit must fail at install and name the tag.
set_version "$src/pack.toml" "$unreleased"
git -C "$src" add -A
git -C "$src" -c user.email=check@example.invalid -c user.name=check \
  commit -qm "pack $unreleased, no release"
unreleased_commit=$(git -C "$src" rev-parse HEAD)

remote=$work/remote.git
git clone -q --bare "$src" "$remote"
source_url="file://$remote"

# --- release assets served by the fake curl ---------------------------------

mkdir -p "$work/release"
go build -ldflags "-X main.buildVersion=$version" \
  -o "$work/release/provider" "$repo/cmd/gc-runtime-rex" ||
  fail "go build failed"
for asset in \
  gc-runtime-rex_darwin_amd64 gc-runtime-rex_darwin_arm64 \
  gc-runtime-rex_linux_amd64 gc-runtime-rex_linux_arm64; do
  cp "$work/release/provider" "$work/release/$asset"
done
if command -v sha256sum >/dev/null 2>&1; then
  ( cd "$work/release" && sha256sum gc-runtime-rex_* > checksums.txt )
else
  ( cd "$work/release" && shasum -a 256 gc-runtime-rex_* > checksums.txt )
fi

mkdir -p "$work/tools"
cat > "$work/tools/curl" <<'FAKE'
#!/bin/sh
# fake curl: curl -sSL -o <out> -w <fmt> <url>, serving $FAKE_RELEASE_DIR.
out= url=
while [ $# -gt 0 ]; do
  case "$1" in
    -o) out=$2; shift 2 ;;
    -w|--retry) shift 2 ;;
    -*) shift ;;
    *) url=$1; shift ;;
  esac
done
name=${url##*/}
dir=${FAKE_RELEASE_DIR:?}
# Only the tag with a release serves assets. A URL for any other tag is a 404,
# as on GitHub when the release does not exist. That is what case C asserts.
case "$url" in
  */releases/download/v"${FAKE_RELEASE_TAG:?}"/*) ;;
  *) printf '%s' 404; exit 22 ;;
esac
if [ -f "$dir/$name" ]; then
  cp "$dir/$name" "$out" || exit 1
  printf '%s' 200
  exit 0
fi
printf '%s' 404
exit 22
FAKE
cat > "$work/tools/gh" <<'FAKE'
#!/bin/sh
# The private-release fallback must not reach the network here.
echo 'gh: no network in this check' >&2
exit 1
FAKE
chmod 755 "$work/tools/curl" "$work/tools/gh"

# --- isolated city environment ----------------------------------------------

# Minimal PATH: no gc-runtime-rex from the developer's machine may shadow the
# installed one, and no real gh may reach the network.
base_path="$work/tools"
for tool in sh awk sed cut mkdir mktemp mv rm cp chmod dirname cat uname \
            shasum sha256sum git; do
  resolved=$(command -v "$tool" 2>/dev/null) || continue
  dir=${resolved%/*}
  case ":$base_path:" in
    *":$dir:"*) ;;
    *) base_path="$base_path:$dir" ;;
  esac
done
# The doctor check also requires the rex CLI. Add it when present, so the
# doctor assertion runs here as well.
rex_bin=$(command -v rex 2>/dev/null || true)
[ -n "$rex_bin" ] && base_path="$base_path:${rex_bin%/*}"

# new_city <name>: a scratch city plus its own HOME and GC_HOME. The provider
# install destination is $HOME/.local/bin, so each case gets a clean one.
new_city() {
  city=$work/city-$1
  home=$work/home-$1
  mkdir -p "$city" "$home"
  printf '[workspace]\nname = "check-import-%s"\n' "$1" > "$city/city.toml"
  case_path="$home/.local/bin:$base_path"
  case_gc_home=$work/gc-home-$1
}

run_gc() {
  env -i HOME="$home" GC_HOME="$case_gc_home" PATH="$case_path" \
    FAKE_RELEASE_TAG="$version" FAKE_RELEASE_DIR="$work/release" "$gc_bin" "$@"
}

# cached_pack: the pack root gc materialized for this city. The source is the
# repository root, so the cached checkout itself is the pack.
cached_pack() {
  found=
  for candidate in "$case_gc_home"/cache/repos/*/pack.toml; do
    [ -f "$candidate" ] || continue
    found=${candidate%/pack.toml}
    break
  done
  [ -n "$found" ] || fail "no cached pack under $case_gc_home/cache/repos"
  printf '%s' "$found"
}

install_provider() {
  pack_root=$(cached_pack)
  env -i HOME="$home" GC_HOME="$case_gc_home" PATH="$case_path" \
    FAKE_RELEASE_TAG="$version" FAKE_RELEASE_DIR="$work/release" \
    sh "$pack_root/install.sh" >"$home/install.out" 2>&1
}

assert_provider_version() {
  target=$home/.local/bin/gc-runtime-rex
  [ -x "$target" ] || fail "$1: install.sh did not write $target"
  "$target" version | grep -q "\"version\":\"$version\"" ||
    fail "$1: installed provider does not report version $version: $("$target" version)"
}

lock_has() {
  grep -q "$1" "$work/city-$2/packs.lock" ||
    fail "$2: packs.lock does not contain '$1': $(cat "$work/city-$2/packs.lock")"
}

# --- case A: import by tag ---------------------------------------------------

new_city tag
run_gc import add "$source_url" --name rex --version "$version" \
  --city "$work/city-tag" >/dev/null ||
  fail "case A: gc import add by tag failed"
grep -q "source = \"$source_url\"" "$work/city-tag/pack.toml" ||
  fail "case A: city pack.toml does not record $source_url"
grep -q "version = \"$version\"" "$work/city-tag/pack.toml" ||
  fail "case A: city pack.toml does not record version $version"
lock_has "commit = \"$released\"" tag
lock_has "version = \"$version\"" tag

run_gc import install --city "$work/city-tag" >/dev/null ||
  fail "case A: gc import install failed"
install_provider || fail "case A: install.sh failed: $(cat "$home/install.out")"
assert_provider_version "case A"
run_gc import check --city "$work/city-tag" >/dev/null ||
  fail "case A: gc import check failed"

# --- case B: sha: import of the released commit ------------------------------

new_city sha-released
run_gc import add "$source_url" --name rex --version "sha:$released" \
  --city "$work/city-sha-released" >/dev/null ||
  fail "case B: gc import add with sha: failed"
lock_has "commit = \"$released\"" sha-released
lock_has "version = \"sha:$released\"" sha-released

run_gc import install --city "$work/city-sha-released" >/dev/null ||
  fail "case B: gc import install failed"
install_provider || fail "case B: install.sh failed: $(cat "$home/install.out")"
assert_provider_version "case B"

# --- case C: sha: import of an unreleased commit -----------------------------

new_city sha-unreleased
run_gc import add "$source_url" --name rex --version "sha:$unreleased_commit" \
  --city "$work/city-sha-unreleased" >/dev/null ||
  fail "case C: gc import add with an unreleased sha: failed"
lock_has "commit = \"$unreleased_commit\"" sha-unreleased

run_gc import install --city "$work/city-sha-unreleased" >/dev/null ||
  fail "case C: gc import install failed"
if install_provider; then
  fail "case C: install.sh succeeded for a commit with no release"
fi
grep -q "v$unreleased" "$home/install.out" ||
  fail "case C: install failure does not name release v$unreleased: $(cat "$home/install.out")"
[ ! -e "$home/.local/bin/gc-runtime-rex" ] ||
  fail "case C: install.sh wrote a provider for an unreleased commit"

# --- doctor, when the rex CLI is available -----------------------------------

if [ -z "$rex_bin" ]; then
  echo "check-import: skip doctor assertion: rex CLI is not on PATH"
  echo "check-import: ok"
  exit 0
fi

new_city doctor
run_gc import add "$source_url" --name rex --version "$version" \
  --city "$work/city-doctor" >/dev/null ||
  fail "doctor: gc import add failed"
run_gc import install --city "$work/city-doctor" >/dev/null ||
  fail "doctor: gc import install failed"
run_gc doctor --check rex:rex-binary --city "$work/city-doctor" --fix >/dev/null 2>&1 ||
  fail "doctor: gc doctor --fix failed"
assert_provider_version "doctor"
run_gc doctor --check rex:rex-binary --city "$work/city-doctor" >/dev/null ||
  fail "doctor: rex:rex-binary is not green after --fix"

echo "check-import: ok"
