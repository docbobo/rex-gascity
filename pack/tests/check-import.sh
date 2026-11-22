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
# Append each request to FAKE_CURL_LOG when set, so the import-is-enough case
# can assert "no download" and "first start downloads".
[ -n "${FAKE_CURL_LOG:-}" ] && printf '%s\n' "$url" >> "$FAKE_CURL_LOG"
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
# Link each tool into $work/tools instead of adding its directory. A
# developer's ~/.local/bin holds rex and gc-runtime-rex beside each other, and
# a directory on PATH would expose that provider to install.sh.
tools='sh awk sed grep cut mkdir mktemp mv rm cp chmod dirname cat uname head ps shasum sha256sum git date sleep tr'
link_tools() {
  for tool in $tools; do
    resolved=$(command -v "$tool" 2>/dev/null) || continue
    ln -sf "$resolved" "$work/tools/$tool"
  done
}

# Regression: the search runs with a directory that holds a provider beside
# the tools, as a developer's ~/.local/bin does. That provider must stay off
# the case PATH and must survive install.sh unchanged.
dev_tools=$work/dev-tools
mkdir -p "$dev_tools"
for tool in $tools rex; do
  resolved=$(command -v "$tool" 2>/dev/null) || continue
  ln -sf "$resolved" "$dev_tools/$tool"
done
printf '#!/bin/sh
echo developer-provider
' > "$dev_tools/gc-runtime-rex"
chmod 755 "$dev_tools/gc-runtime-rex"
( PATH=$dev_tools:$PATH; export PATH; link_tools )
leaked=$(PATH=$base_path command -v gc-runtime-rex 2>/dev/null || true)
[ -z "$leaked" ] || fail "case PATH exposes a provider: $leaked"

# The fake rex backend reads FAKEREX_STATE. A missing state dir means "no
# backend", so create it before the first gc run. The dir is shared across cases.
mkdir -p "$work/rex-state"

# The fake rex backend and its version-answering wrapper provide `rex` on the
# case PATH; a developer's real rex must not shadow it. The leak regression
# above still links the developer's rex onto $dev_tools, off the case PATH.
# Build the in-repo fake under a hidden name so the wrapper can delegate every
# non-version op to it.
go build -o "$work/tools/.fakerex" "$repo/internal/rex/fakerex/fakebinary" ||
  fail "fake rex build failed"
rexmin=$(tr -d '[:space:]' < "$repo/pack/rex-min-version")
cat > "$work/tools/rex" <<EOF
#!/bin/sh
if [ "\$1" = version ]; then
  if [ "\$2" = --json ] || [ "\$2" = -j ]; then
    "$work/tools/.fakerex" version --json | sed 's/^{/{"rex_version":"$rexmin",/'
  else
    printf 'rex %s\n' "$rexmin"
    "$work/tools/.fakerex" version
  fi
  exit 0
fi
exec "$work/tools/.fakerex" "\$@"
EOF
chmod 0755 "$work/tools/rex"

# new_city <name>: a scratch city plus its own HOME and GC_HOME. The provider
# install destination is $HOME/.local/bin, so each case gets a clean one.
new_city() {
  city=$work/city-$1
  home=$work/home-$1
  mkdir -p "$city" "$home"
  printf '[workspace]\nname = "check-import-%s"\n' "$1" > "$city/city.toml"
  case_path="$home/.local/bin:$base_path"
  case_gc_home=$work/gc-home-$1
  case_cache=
}

run_gc() {
  env -i HOME="$home" GC_HOME="$case_gc_home" PATH="$case_path" \
    FAKE_RELEASE_TAG="$version" FAKE_RELEASE_DIR="$work/release" \
    GC_RUNTIME_CACHE_DIR="${case_cache:-$work/cache-default}" \
    FAKE_CURL_LOG="${FAKE_CURL_LOG:-}" \
    FAKEREX_STATE="$work/rex-state" \
    "$gc_bin" "$@"
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

# uname_platform: map uname -s/-m the way bin/_lib.sh does, so the cache path
# this check asserts is the one the shim uses.
uname_platform() {
  case "$(uname -s)" in
    Darwin) os=darwin ;;
    Linux) os=linux ;;
    *) fail "unsupported operating system: $(uname -s)" ;;
  esac
  case "$(uname -m)" in
    x86_64|amd64) arch=amd64 ;;
    aarch64|arm64) arch=arm64 ;;
    *) fail "unsupported machine: $(uname -m)" ;;
  esac
  printf '%s-%s' "$os" "$arch"
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
grep -qx "echo developer-provider" "$dev_tools/gc-runtime-rex" ||
  fail "case A: install.sh overwrote the developer's provider"
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

# --- case D: import is enough ------------------------------------------------

new_city coldcache
case_cache=$work/cache-coldcache
coldcache=$case_cache
run_gc import add "$source_url" --name rex --version "$version" \
  --city "$work/city-coldcache" >/dev/null || fail "coldcache: import add failed"
run_gc import install --city "$work/city-coldcache" >/dev/null ||
  fail "coldcache: import install failed"

# Cold cache + no provider on PATH: the doctor reports missing with NO download
# and NO cache write (read-only cold check). --verbose prints the details that
# carry the remediation.
rm -rf "$coldcache"
: > "$work/curl.log"
if FAKE_CURL_LOG=$work/curl.log \
  run_gc doctor --check rex-gascity-runtime:rex-binary --verbose \
  --city "$work/city-coldcache" >"$work/cold-doctor.out" 2>&1; then
  fail "coldcache: cold doctor passed before any install"
fi
grep -q "the rex runtime provider is not yet installed" "$work/cold-doctor.out" ||
  fail "coldcache: cold doctor does not report the not-yet-installed state: $(cat "$work/cold-doctor.out")"
grep -q "gc doctor --fix" "$work/cold-doctor.out" ||
  fail "coldcache: cold doctor remediation does not name gc doctor --fix: $(cat "$work/cold-doctor.out")"
grep -q "start a session" "$work/cold-doctor.out" ||
  fail "coldcache: cold doctor remediation does not name starting a session: $(cat "$work/cold-doctor.out")"
[ -s "$work/curl.log" ] && fail "coldcache: cold doctor downloaded"
[ ! -e "$coldcache" ] || fail "coldcache: cold doctor wrote to the cache"

# A real gc-selected start installs through the pack-relative command AND runs
# the conformance lifecycle against the fake rex backend (start/is-running/stop).
: > "$work/curl.log"
FAKE_CURL_LOG=$work/curl.log \
  run_gc runtime check rex --city "$work/city-coldcache" >/dev/null 2>&1 ||
  fail "coldcache: gc runtime check rex (real start, fake rex backend) failed"
[ -s "$work/curl.log" ] || fail "coldcache: first real start made no download"
[ -x "$coldcache/$version/$(uname_platform)/gc-runtime-rex" ] ||
  fail "coldcache: start did not warm the cache"

# Warm: the rex-binary doctor check is GREEN with no --fix and no new fetch. The
# green verdict depends on the `rex` wrapper answering the doctor's rex_version
# probe at >= the pin; the bare fake would come back "version could not be
# determined". A warning also exits 0 (only blocking failures change the exit
# status), so assert the OK line, not the exit status alone.
: > "$work/curl.log"
if ! FAKE_CURL_LOG=$work/curl.log \
  run_gc doctor --check rex-gascity-runtime:rex-binary --verbose \
  --city "$work/city-coldcache" >"$work/warm-doctor.out" 2>&1; then
  fail "coldcache: warm rex-binary doctor check failed after a real start: $(cat "$work/warm-doctor.out")"
fi
grep -q "the rex runtime provider and rex CLI are installed" "$work/warm-doctor.out" ||
  fail "coldcache: warm rex-binary doctor check did not report OK: $(cat "$work/warm-doctor.out")"
[ -s "$work/curl.log" ] && fail "coldcache: doctor fetched again after a warm start"

# Separately, --fix warms the SAME cache (not $HOME/.local/bin).
rm -rf "$coldcache"
run_gc doctor --check rex-gascity-runtime:rex-binary --city "$work/city-coldcache" --fix >/dev/null 2>&1 ||
  fail "coldcache: --fix failed to warm the cache"
[ -x "$coldcache/$version/$(uname_platform)/gc-runtime-rex" ] ||
  fail "coldcache: --fix did not warm the cache the shim uses"
[ ! -e "$home/.local/bin/gc-runtime-rex" ] ||
  fail "coldcache: --fix wrote to \$HOME/.local/bin instead of the cache"

echo "check-import: ok"
