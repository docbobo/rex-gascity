#!/bin/sh
# Guard for pack/install.sh. Runs the installer against a fake release served
# by fake curl/gh/uname scripts on PATH, with a temp HOME. No network.
#
# Usage: sh pack/tests/check-installer.sh
# Exits non-zero with a one-line reason on the first failed assertion.
set -eu

repo=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
installer=$repo/pack/install.sh
pack_toml=$repo/pack/pack.toml

fail() {
  echo "check-installer: $*" >&2
  exit 1
}

[ -f "$installer" ] || fail "pack/install.sh does not exist"
[ -x "$installer" ] || fail "pack/install.sh is not executable"

work=$(mktemp -d "${TMPDIR:-/tmp}/check-installer.XXXXXX")
trap 'rm -rf "$work"' EXIT

version=$(
  awk -F'"' '
    /^[[:space:]]*\[pack\][[:space:]]*$/ { in_pack = 1; next }
    /^[[:space:]]*\[/ { in_pack = 0 }
    in_pack && /^[[:space:]]*version[[:space:]]*=/ { print $2; exit }
  ' "$pack_toml"
)
[ -n "$version" ] || fail "pack.toml declares no [pack] version"

# The release assets are not stubs. install.sh refuses to replace a working
# provider with one that cannot answer inventory, so the assets delegate every
# other op to a real provider built from source. That needs the repo's Go
# toolchain; the installers under test do not.
command -v go >/dev/null 2>&1 ||
  fail "go is required to build the provider the release assets delegate to"
real_provider=$work/gc-runtime-rex
(cd "$repo" && go build -o "$real_provider" ./cmd/gc-runtime-rex) ||
  fail "cannot build ./cmd/gc-runtime-rex"
fake_rex=$work/rex
(cd "$repo" && go build -o "$fake_rex" ./internal/rex/fakerex/fakebinary) ||
  fail "cannot build the fake rex"

# --- fake toolchain ---------------------------------------------------------

mkdir -p "$work/tools" "$work/tools-nogh" "$work/release"

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
[ -n "${FAKE_CURL_LOG:-}" ] && printf '%s\n' "$url" >> "$FAKE_CURL_LOG"
if [ "${FAKE_CURL_MODE:-ok}" = transport ]; then
  echo 'curl: (7) Failed to connect to github.com port 443' >&2
  exit 7
fi
code=${FAKE_CURL_HTTP:-200}
if [ "$code" != 200 ]; then
  printf '%s' "$code"
  exit 22
fi
name=${url##*/}
FAKE_RELEASE_DIR=${FAKE_RELEASE_DIR:-$(dirname "$0")/../release}
[ -f "$FAKE_RELEASE_DIR/$name" ] || { printf '%s' 404; exit 22; }
cp "$FAKE_RELEASE_DIR/$name" "$out" || exit 1
printf '%s' 200
FAKE

cat > "$work/tools/gh" <<'FAKE'
#!/bin/sh
# fake gh: gh release download <tag> -R <repo> -p <name>... --dir <dir> --clobber
[ -n "${FAKE_GH_LOG:-}" ] && printf '%s\n' "$*" >> "$FAKE_GH_LOG"
dir= names=
while [ $# -gt 0 ]; do
  case "$1" in
    -R) shift 2 ;;
    -p|--pattern) names="$names $2"; shift 2 ;;
    --dir) dir=$2; shift 2 ;;
    --clobber) shift ;;
    *) shift ;;
  esac
done
FAKE_RELEASE_DIR=${FAKE_RELEASE_DIR:-$(dirname "$0")/../release}
if [ -n "${FAKE_GH_FAIL:-}" ]; then
  echo 'gh: release not found' >&2
  exit 1
fi
for n in $names; do
  cp "$FAKE_RELEASE_DIR/$n" "$dir/$n" || exit 1
done
exit 0
FAKE

cat > "$work/tools/uname" <<'FAKE'
#!/bin/sh
case "$1" in
  -s) printf '%s\n' "${FAKE_UNAME_S:-Darwin}" ;;
  -m) printf '%s\n' "${FAKE_UNAME_M:-arm64}" ;;
  *) printf '%s\n' "${FAKE_UNAME_S:-Darwin}" ;;
esac
FAKE

cat > "$work/tools/sha256" <<'FAKE'
#!/bin/sh
# sha256 <file> -> hex digest, using whichever tool this host has.
if command -v sha256sum >/dev/null 2>&1; then
  sha256sum "$1" | cut -d' ' -f1
else
  shasum -a 256 "$1" | cut -d' ' -f1
fi
FAKE

chmod 755 "$work/tools/curl" "$work/tools/gh" "$work/tools/uname" "$work/tools/sha256"
# tools-nogh: the public path must work with curl alone.
cp "$work/tools/curl" "$work/tools/uname" "$work/tools/sha256" "$work/tools-nogh/"

# A release asset: serves the two ops the doctor check reads itself and
# delegates the rest to the real provider, so install.sh's inventory guard
# accepts it.
make_asset() {
  cat > "$1" <<ASSET
#!/bin/sh
case "\$1" in
  protocol) printf '%s\n' '{"version":0,"capabilities":["report-attachment"]}' ;;
  version) printf '%s\n' '{"version":"$version"}' ;;
  *) exec "$real_provider" "\$@" ;;
esac
ASSET
  chmod 755 "$1"
}

# A stub asset: the pre-fix asset shape. It answers protocol and version and
# exits 2 for everything else, which fails the installer's inventory guard.
make_stub() {
  cat > "$1" <<STUB
#!/bin/sh
case "\$1" in
  protocol) printf '%s\n' '{"version":0,"capabilities":["report-attachment"]}' ;;
  version) printf '%s\n' '{"version":"$version"}' ;;
  *) exit 2 ;;
esac
STUB
  chmod 755 "$1"
}

assets="gc-runtime-rex_darwin_amd64 gc-runtime-rex_darwin_arm64 gc-runtime-rex_linux_amd64 gc-runtime-rex_linux_arm64"
: > "$work/release/checksums.txt"
for asset in $assets; do
  make_asset "$work/release/$asset"
  hex=$("$work/tools/sha256" "$work/release/$asset")
  printf '%s  %s\n' "$hex" "$asset" >> "$work/release/checksums.txt"
done

# release-bad: valid assets, a checksums.txt whose arm64 entry is wrong.
cp -R "$work/release/." "$work/release-bad/"
awk -v name=gc-runtime-rex_darwin_arm64 '
  $2 == name { $1 = "0000000000000000000000000000000000000000000000000000000000000000" }
  { print }
' "$work/release/checksums.txt" > "$work/release-bad/checksums.txt"

# sysbin holds the shell utilities the installer runs, linked into a directory
# of its own. A real /usr/bin may carry gc-runtime-rex, and then the installer
# would pick that directory as the install target instead of ~/.local/bin.
sysbin=$work/sysbin
mkdir -p "$sysbin"
for t in sh awk sed grep cut mkdir mktemp mv rm cp chmod dirname cat \
         sha256sum shasum; do
  p=$(command -v "$t" 2>/dev/null) || continue
  ln -sf "$p" "$sysbin/$t"
done
base_path=$sysbin
tools_path="$work/tools:$base_path"
nogh_path="$work/tools-nogh:$base_path"

# run_install <uname-s> <uname-m> <home> <path> [args...]
# Sets $out to the combined output and returns the installer's exit code.
run_install() {
  us=$1; um=$2; home=$3; path=$4; shift 4
  mkdir -p "$home"
  if out=$(
    env -i HOME="$home" PATH="$path" \
      FAKE_UNAME_S="$us" FAKE_UNAME_M="$um" \
      FAKE_RELEASE_DIR="${FAKE_RELEASE_DIR:-$work/release}" \
      FAKE_CURL_LOG="$work/curl.log" FAKE_GH_LOG="$work/gh.log" \
      FAKE_CURL_HTTP="${FAKE_CURL_HTTP:-200}" FAKE_CURL_MODE="${FAKE_CURL_MODE:-ok}" \
      sh "$installer" "$@" 2>&1
  ); then
    return 0
  fi
  return 1
}

reset_logs() {
  : > "$work/curl.log"
  : > "$work/gh.log"
}

# assert_provider <path> <label>: the installed provider answers the ops the
# supervisor calls every cycle, not only the two the doctor check reads.
# inventory and observe are read-only and need only the fake rex on PATH.
assert_provider() {
  bindir=$work/assert-bin
  mkdir -p "$bindir"
  ln -sf "$fake_rex" "$bindir/rex"
  ah=$work/assert-home-$2
  mkdir -p "$ah"
  inv=$(env -i HOME="$ah" PATH="$bindir" FAKEREX_STATE="$ah/state" "$1" inventory 2>&1) ||
    fail "$2: installed provider does not answer inventory: $inv"
  printf '%s' "$inv" | grep -q '"complete":true' ||
    fail "$2: inventory is not a complete payload: $inv"
  obs=$(env -i HOME="$ah" PATH="$bindir" FAKEREX_STATE="$ah/state" "$1" observe probe-session 2>&1) ||
    fail "$2: installed provider does not answer observe: $obs"
  printf '%s' "$obs" | grep -q '"state":' ||
    fail "$2: observe is not a session state: $obs"
}

# --- arch and os mapping ----------------------------------------------------

for pair in "Darwin arm64 darwin arm64" "Darwin x86_64 darwin amd64" \
            "Linux x86_64 linux amd64" "Linux aarch64 linux arm64"; do
  set -- $pair
  us=$1; um=$2; os=$3; arch=$4
  home=$work/home-$os-$arch
  reset_logs
  FAKE_CURL_HTTP=200 run_install "$us" "$um" "$home" "$nogh_path" ||
    fail "install failed for $us/$um: $out"
  target=$home/.local/bin/gc-runtime-rex
  [ -x "$target" ] || fail "install for $us/$um did not write $target"
  "$target" version | grep -q "$version" || fail "installed $us/$um binary does not report $version"
  assert_provider "$target" "$us-$um"
  grep -q "/v$version/gc-runtime-rex_${os}_${arch}$" "$work/curl.log" ||
    fail "installer for $us/$um did not request the ${os}_${arch} asset: $(cat "$work/curl.log")"
  [ ! -s "$work/gh.log" ] || fail "public install for $us/$um used gh instead of curl"
  printf '%s' "$out" | grep -q "$target" || fail "install output does not name the install target"
done

# --- unsupported platforms --------------------------------------------------

reset_logs
if FAKE_CURL_HTTP=200 run_install Darwin armv7l "$work/home-armv7" "$nogh_path"; then
  fail "install succeeded on unsupported machine armv7l"
fi
printf '%s' "$out" | grep -q "armv7l" || fail "unsupported-machine error does not name armv7l"
reset_logs
if FAKE_CURL_HTTP=200 run_install MINGW64_NT-10.0 x86_64 "$work/home-win" "$nogh_path"; then
  fail "install succeeded on Windows"
fi
printf '%s' "$out" | grep -qi "MINGW" || fail "unsupported-OS error does not name the OS"

# --- install directory rule -------------------------------------------------

home=$work/home-reuse
reuse=$work/reuse-bin
mkdir -p "$reuse"
printf '#!/bin/sh\nexit 1\n' > "$reuse/gc-runtime-rex"   # a stale provider on PATH
chmod 755 "$reuse/gc-runtime-rex"
reset_logs
FAKE_CURL_HTTP=200 run_install Darwin arm64 "$home" "$reuse:$nogh_path" ||
  fail "install failed when reusing an existing install directory: $out"
"$reuse/gc-runtime-rex" version | grep -q "$version" ||
  fail "install did not replace the provider in the directory command -v reports"
assert_provider "$reuse/gc-runtime-rex" reuse
[ ! -e "$home/.local/bin/gc-runtime-rex" ] || fail "install wrote to ~/.local/bin although a provider was on PATH"

# --- relative PATH entry ----------------------------------------------------

# command -v reports the relative spelling, relbin/gc-runtime-rex. The install
# target must still be that directory, not ~/.local/bin.
home=$work/home-relative
relbin=$work/relbin
mkdir -p "$home" "$relbin"
printf '#!/bin/sh\nexit 1\n' > "$relbin/gc-runtime-rex"   # a stale provider
chmod 755 "$relbin/gc-runtime-rex"
reset_logs
if ! out=$(
  cd "$work" && env -i HOME="$home" PATH="relbin:$nogh_path" \
    FAKE_UNAME_S=Darwin FAKE_UNAME_M=arm64 \
    FAKE_RELEASE_DIR="$work/release" \
    FAKE_CURL_LOG="$work/curl.log" FAKE_GH_LOG="$work/gh.log" \
    FAKE_CURL_HTTP=200 FAKE_CURL_MODE=ok \
    sh "$installer" 2>&1
); then
  fail "install failed with a relative PATH entry: $out"
fi
[ ! -e "$home/.local/bin/gc-runtime-rex" ] ||
  fail "a relative PATH entry was not resolved; install went to ~/.local/bin"
"$relbin/gc-runtime-rex" version | grep -q "$version" ||
  fail "install did not replace the provider under the relative PATH entry"

# --- PATH shadow warning ----------------------------------------------------

home=$work/home-shadow
shadow=$work/shadow-bin
installdir=$work/install-bin
mkdir -p "$shadow" "$installdir"
printf '#!/bin/sh\nexit 1\n' > "$shadow/gc-runtime-rex"   # earlier, not executable
chmod 644 "$shadow/gc-runtime-rex"
make_asset "$installdir/gc-runtime-rex"
reset_logs
FAKE_CURL_HTTP=200 run_install Darwin arm64 "$home" "$shadow:$installdir:$nogh_path" ||
  fail "install failed with a shadowing PATH entry: $out"
printf '%s' "$out" | grep -q "warning" || fail "no warning for a shadowing PATH entry"
printf '%s' "$out" | grep -q "$shadow/gc-runtime-rex" ||
  fail "shadow warning does not name $shadow/gc-runtime-rex"

# --- checksum policy --------------------------------------------------------

home=$work/home-bad
reset_logs
if FAKE_RELEASE_DIR=$work/release-bad FAKE_CURL_HTTP=200 \
    run_install Darwin arm64 "$home" "$nogh_path"; then
  fail "install succeeded with a wrong checksums.txt entry"
fi
printf '%s' "$out" | grep -q "sha256" || fail "checksum failure does not mention sha256"
[ ! -e "$home/.local/bin/gc-runtime-rex" ] || fail "install wrote a binary despite a checksum failure"

# --- private release: gh fallback -------------------------------------------

home=$work/home-private
reset_logs
FAKE_CURL_HTTP=404 run_install Darwin arm64 "$home" "$tools_path" ||
  fail "install failed on a private release with gh available: $out"
grep -q "release download v$version" "$work/gh.log" || fail "installer did not fall back to gh release download"
[ -x "$home/.local/bin/gc-runtime-rex" ] || fail "gh fallback did not install the provider"

# No curl and no gh: the error names both. A PATH with the shell utilities
# only, because /usr/bin carries curl on most hosts.
none=$work/tools-none
mkdir -p "$none"
for t in sh awk sed cut mkdir mktemp mv rm cp chmod dirname cat; do
  p=$(command -v "$t" 2>/dev/null) || continue
  ln -sf "$p" "$none/$t"
done
for t in sha256sum shasum; do
  p=$(command -v "$t" 2>/dev/null) || continue
  ln -sf "$p" "$none/$t"
done
cp "$work/tools/uname" "$none/uname"
home=$work/home-none
reset_logs
if FAKE_CURL_HTTP=200 run_install Darwin arm64 "$home" "$none"; then
  fail "install succeeded without curl or gh"
fi
printf '%s' "$out" | grep -q "curl" || fail "no-tools error does not name curl"

# --- repeat install ---------------------------------------------------------

home=$work/home-repeat
reset_logs
FAKE_CURL_HTTP=200 run_install Darwin arm64 "$home" "$nogh_path" || fail "first install failed: $out"
target=$home/.local/bin/gc-runtime-rex
before=$(ls -i "$target" | awk '{print $1}')
reset_logs
FAKE_CURL_HTTP=200 run_install Darwin arm64 "$home" "$nogh_path" || fail "repeat install failed: $out"
after=$(ls -i "$target" | awk '{print $1}')
[ "$before" = "$after" ] || fail "repeat install replaced the binary (inode $before -> $after)"
printf '%s' "$out" | grep -q "already installed" || fail "repeat install does not report success"

# --- replace guard: a working provider survives a stub release --------------

# A stub asset answers protocol and version but not inventory. Replacing a
# working provider with it turns every supervisor cycle into a failure, so
# the installer reports the conflict and leaves the provider in place.
home=$work/home-guard
reset_logs
FAKE_CURL_HTTP=200 run_install Darwin arm64 "$home" "$nogh_path" ||
  fail "guard case: first install failed: $out"
target=$home/.local/bin/gc-runtime-rex
cp "$target" "$work/guard-good"
stubdir=$work/release-stub
cp -R "$work/release/." "$stubdir/"
make_stub "$stubdir/gc-runtime-rex_darwin_arm64"
hex=$("$work/tools/sha256" "$stubdir/gc-runtime-rex_darwin_arm64")
awk -v name=gc-runtime-rex_darwin_arm64 -v hex="$hex" '
  $2 == name { $1 = hex }
  { print }
' "$stubdir/checksums.txt" > "$stubdir/checksums.txt.new"
mv "$stubdir/checksums.txt.new" "$stubdir/checksums.txt"
reset_logs
if FAKE_RELEASE_DIR=$stubdir FAKE_CURL_HTTP=200 run_install Darwin arm64 "$home" "$nogh_path"; then
  fail "guard case: install replaced a working provider with a stub asset: $out"
fi
printf '%s' "$out" | grep -q "refusing to replace" ||
  fail "guard case: error does not report the conflict: $out"
cmp -s "$target" "$work/guard-good" || fail "guard case: the working provider was replaced"
assert_provider "$target" guard

# --- repair: a matching but non-executable binary ---------------------------

home=$work/home-repair
reset_logs
FAKE_CURL_HTTP=200 run_install Darwin arm64 "$home" "$nogh_path" ||
  fail "repair case: first install failed: $out"
target=$home/.local/bin/gc-runtime-rex
chmod 644 "$target"
reset_logs
FAKE_CURL_HTTP=200 run_install Darwin arm64 "$home" "$nogh_path" ||
  fail "repair case: install failed on a matching 0644 provider: $out"
[ -x "$target" ] ||
  fail "repair case: install reported success but left $target non-executable"
"$target" version | grep -q "$version" ||
  fail "repair case: repaired provider does not run and report $version"

# --- doctor path ------------------------------------------------------------

if ! command -v gc >/dev/null 2>&1; then
  echo "check-installer: skip doctor path: gc is not on PATH"
  exit 0
fi
# The fake rex stands in for the CLI. It needs no install and answers the
# session state observe reads, so this path runs wherever gc is present.

city=$work/city
mkdir -p "$city"
printf '[workspace]\nname = "installer-check"\n\n[imports.rex]\nsource = "%s"\n' "$repo/pack" > "$city/city.toml"

gc_bin=$(command -v gc)
rexdir=$work/rex-bin
mkdir -p "$rexdir"
ln -sf "$fake_rex" "$rexdir/rex"

# No provider on PATH: the check names the installer.
home=$work/home-doctor
mkdir -p "$home"
# After the fix, ~/.local/bin is on PATH, as an operator's environment has it.
doctor_path="$home/.local/bin:$rexdir:$nogh_path"
reset_logs
if out=$(env -i HOME="$home" PATH="$rexdir:$base_path" \
    FAKE_UNAME_S=Darwin FAKE_UNAME_M=arm64 FAKE_RELEASE_DIR="$work/release" \
    FAKE_CURL_LOG="$work/curl.log" FAKE_GH_LOG="$work/gh.log" \
    "$gc_bin" doctor --check rex-gascity-runtime:rex-binary --city "$city" --verbose 2>&1); then
  fail "gc doctor passed with no provider installed: $out"
fi
printf '%s' "$out" | grep -q "install.sh" || fail "gc doctor does not name pack/install.sh: $out"
if printf '%s' "$out" | grep -q "go install"; then
  fail "gc doctor still offers a Go build: $out"
fi

# --fix installs a binary whose version matches pack.toml, then doctor is green.
reset_logs
env -i HOME="$home" PATH="$doctor_path" \
  FAKE_UNAME_S=Darwin FAKE_UNAME_M=arm64 FAKE_RELEASE_DIR="$work/release" \
  FAKE_CURL_LOG="$work/curl.log" FAKE_GH_LOG="$work/gh.log" \
  "$gc_bin" doctor --check rex-gascity-runtime:rex-binary --city "$city" --fix --verbose > "$work/fix.out" 2>&1 ||
  { cat "$work/fix.out" >&2; fail "gc doctor --fix failed"; }
target=$home/.local/bin/gc-runtime-rex
[ -x "$target" ] || fail "gc doctor --fix did not install the provider"
"$target" version | grep -q "$version" || fail "gc doctor --fix installed a binary that does not match pack.toml"
assert_provider "$target" doctor
out=$(env -i HOME="$home" PATH="$doctor_path" "$gc_bin" doctor --check rex-gascity-runtime:rex-binary --city "$city" 2>&1) ||
  fail "gc doctor is not green after --fix: $out"
printf '%s' "$out" | grep -q "rex-gascity-runtime:rex-binary" || fail "gc doctor did not report the rex-gascity-runtime:rex-binary check"

echo "check-installer: ok"
