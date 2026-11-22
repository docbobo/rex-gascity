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

# Containment guard. install.sh picks its default target from `command -v
# gc-runtime-rex` or `$HOME/.local/bin`. A test that inherits the ambient
# PATH therefore writes into the real home. On 2026-11-22 that replaced the
# live ~/.local/bin/gc-runtime-rex and broke the city runtime. Snapshot every
# real destination before the run and fail when the run changes one.
guard_providers=$(command -v gc-runtime-rex || true)
# Snapshot BOTH candidate directories even when they do not exist yet: a run
# that CREATES $HOME/.local/bin and installs there must be caught, not silently
# skipped. One path per line keeps a directory with spaces in its name intact
# (a word-split list would break it into two paths).
guard_dirs=
if [ -n "${HOME:-}" ]; then
  guard_dirs="$HOME/.local/bin
$HOME/bin"
fi
guard_snapshot() {
  _ifs=$IFS; IFS='
'
  for p in $guard_providers; do
    printf 'provider %s\n' "$p"
    ls -ld "$p" 2>/dev/null || echo absent
    cksum "$p" 2>/dev/null || true
  done
  for d in $guard_dirs; do
    [ -n "$d" ] || continue
    if [ -d "$d" ]; then printf 'dir %s present\n' "$d"; else printf 'dir %s absent\n' "$d"; fi
    [ -d "$d" ] || continue
    for f in "$d"/*; do
      if [ -e "$f" ] || [ -L "$f" ]; then
        ls -ld "$f" 2>/dev/null || true
        cksum "$f" 2>/dev/null || true
      fi
    done
  done
  IFS=$_ifs
}
guard_snapshot > "$work/guard-before"
trap '
  guard_snapshot > "$work/guard-after"
  if ! cmp -s "$work/guard-before" "$work/guard-after"; then
    echo "check-installer: the run changed a real provider destination (containment guard)" >&2
    diff -u "$work/guard-before" "$work/guard-after" >&2 || true
    rm -rf "$work"
    exit 1
  fi
  rm -rf "$work"
' EXIT

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
  # A DISTINCTIVE transport diagnostic the installer must PRESERVE (REQ-007):
  # the transport tests assert this exact text survives into the failure message,
  # so a generic "transport failure" that discarded curl's stderr would be caught.
  echo 'curl: (7) Failed to connect to github.com port 443: Connection refused' >&2
  exit 7
fi
if [ -n "${FAKE_CURL_SLEEP:-}" ]; then
  exec sleep "$FAKE_CURL_SLEEP"   # a hung transfer: one killable process
fi
name=${url##*/}
# Name-aware slow-but-SUCCESSFUL delays: unlike FAKE_CURL_SLEEP (a hung transfer
# that never serves), these sleep then serve, so a generous budget still
# succeeds while a tight one is reaped. They let a test drive checksums and the
# asset at different speeds to prove the fetches SHARE one whole-fetch budget.
if [ "$name" = checksums.txt ] && [ -n "${FAKE_CURL_SLEEP_CHECKSUMS:-}" ]; then
  sleep "$FAKE_CURL_SLEEP_CHECKSUMS"
elif [ "$name" != checksums.txt ] && [ -n "${FAKE_CURL_SLEEP_ASSET:-}" ]; then
  sleep "$FAKE_CURL_SLEEP_ASSET"
fi
FAKE_RELEASE_DIR=${FAKE_RELEASE_DIR:-$(dirname "$0")/../release}
code=${FAKE_CURL_HTTP:-200}
# A name-scoped failure: checksums.txt first, so a checksums-only error must be
# reported as that URL, not the binary URL curl never requested.
[ "$name" = "${FAKE_CURL_404_NAME:-}" ] && code=404
[ "$code" = 200 ] && [ ! -f "$FAKE_RELEASE_DIR/$name" ] && code=404
if [ "$code" != 200 ]; then
  # Real curl without --fail writes the error page, prints %{http_code}, exits 0.
  printf 'HTTP %s for %s\n' "$code" "$name" > "$out" 2>/dev/null || true
  printf '%s' "$code"
  exit 0
fi
cp "$FAKE_RELEASE_DIR/$name" "$out" || exit 1
[ -n "${FAKE_CURL_CHMOD:-}" ] && chmod "$FAKE_CURL_CHMOD" "$out"
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
[ -n "${FAKE_GH_SLEEP:-}" ] && exec sleep "$FAKE_GH_SLEEP"   # a stalled fallback
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
         sha256sum shasum head date sleep dash; do
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
      FAKE_CURL_SLEEP="${FAKE_CURL_SLEEP:-}" FAKE_CURL_SLEEP_ASSET="${FAKE_CURL_SLEEP_ASSET:-}" \
      FAKE_CURL_SLEEP_CHECKSUMS="${FAKE_CURL_SLEEP_CHECKSUMS:-}" \
      FAKE_CURL_404_NAME="${FAKE_CURL_404_NAME:-}" \
      FAKE_CURL_CHMOD="${FAKE_CURL_CHMOD:-}" \
      FAKE_GH_SLEEP="${FAKE_GH_SLEEP:-}" \
      GC_RUNTIME_INSTALL_DIR="${GC_RUNTIME_INSTALL_DIR:-}" \
      FAKE_GH_FAIL="${FAKE_GH_FAIL:-}" \
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

# An explicit GC_RUNTIME_INSTALL_DIR must not overwrite the pack's shipped shim:
# --from-source passes that directory to go install as GOBIN, and the inventory
# guard never runs. The physical identity is compared, so an alias is caught too.
shim_before=$(cksum "$repo/pack/bin/gc-runtime-rex")
if FAKE_CURL_HTTP=200 GC_RUNTIME_INSTALL_DIR="$repo/pack/bin" \
    run_install Darwin arm64 "$work/home-shimdir" "$nogh_path"; then
  fail "install accepted the shipped shim directory as GC_RUNTIME_INSTALL_DIR"
fi
printf '%s' "$out" | grep -qi 'shipped shim' \
  || fail "shim-directory refusal did not name the shipped shim: $out"
[ "$(cksum "$repo/pack/bin/gc-runtime-rex")" = "$shim_before" ] \
  || fail "the shipped shim changed after a refused install"

# A directory at the target path passes -x (the shim would try to execute it) and
# mv would move the staged file INSIDE it and report success. Refuse by name.
dirtarget=$work/dir-target
mkdir -p "$dirtarget/gc-runtime-rex"
if FAKE_CURL_HTTP=200 GC_RUNTIME_INSTALL_DIR="$dirtarget" \
    run_install Darwin arm64 "$work/home-dirtarget" "$nogh_path"; then
  fail "install accepted a directory at the provider target path"
fi
printf '%s' "$out" | grep -q 'is a directory' \
  || fail "directory-at-target refusal did not name the directory: $out"

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

# --- guard: a download arrives without execute permission --------------------

# curl and gh write the asset mode 0644. The guard must make its temporary
# copy executable before probing it, or a working provider refuses every
# valid install and upgrade.
home=$work/home-download-mode
reset_logs
FAKE_CURL_HTTP=200 run_install Darwin arm64 "$home" "$nogh_path" ||
  fail "download-mode case: first install failed: $out"
target=$home/.local/bin/gc-runtime-rex
before=$(ls -i "$target" | awk '{print $1}')
reset_logs
FAKE_CURL_CHMOD=0644 FAKE_CURL_HTTP=200 run_install Darwin arm64 "$home" "$nogh_path" ||
  fail "download-mode case: a non-executable download blocked a valid reinstall: $out"
printf '%s' "$out" | grep -q "already installed" ||
  fail "download-mode case: reinstall did not report success: $out"
after=$(ls -i "$target" | awk '{print $1}')
[ "$before" = "$after" ] || fail "download-mode case: reinstall replaced the binary"

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

# --- install-dir mode + fallback diagnostics (before the doctor-path skips) ---

# GC_RUNTIME_INSTALL_DIR installs into the given dir and suppresses the PATH
# warning.
dest=$work/cache/2026.11.01/darwin-arm64
rm -rf "$work/cache"; reset_logs
GC_RUNTIME_INSTALL_DIR=$dest FAKE_CURL_HTTP=200 \
  run_install Darwin arm64 "$work/home-dir" "$nogh_path" ||
  fail "install with GC_RUNTIME_INSTALL_DIR failed: $out"
[ -x "$dest/gc-runtime-rex" ] || fail "install-dir mode did not write $dest/gc-runtime-rex"
printf '%s' "$out" | grep -q 'not on PATH' && fail "PATH warning not suppressed in install-dir mode"

# The installer never selects or overwrites the SHIPPED shim's dir: a DISPOSABLE
# pack copy with a dummy bin/gc-runtime-rex; with pack/bin on PATH the installer
# must neither overwrite it nor target it, but fall back to ~/.local/bin.
pkg=$work/pack; rm -rf "$pkg"; mkdir -p "$pkg/bin"
cp "$installer" "$pkg/install.sh"; cp "$pack_toml" "$pkg/pack.toml"
printf '#!/bin/sh\necho dummy-shim\n' > "$pkg/bin/gc-runtime-rex"; chmod 755 "$pkg/bin/gc-runtime-rex"
before=$("$work/tools/sha256" "$pkg/bin/gc-runtime-rex")
reset_logs
out=$(env -i HOME="$work/home-shim" PATH="$pkg/bin:$nogh_path" \
  FAKE_UNAME_S=Darwin FAKE_UNAME_M=arm64 FAKE_RELEASE_DIR="$work/release" \
  FAKE_CURL_LOG="$work/curl.log" FAKE_CURL_HTTP=200 \
  sh "$pkg/install.sh" 2>&1) || fail "install (pack/bin on PATH) failed: $out"
after=$("$work/tools/sha256" "$pkg/bin/gc-runtime-rex")
[ "$before" = "$after" ] || fail "installer overwrote the shipped shim placeholder"
[ -x "$work/home-shim/.local/bin/gc-runtime-rex" ] ||
  fail "installer did not fall back to ~/.local/bin when only the shipped shim was on PATH"

# A PATH entry that is a SYMLINK to pack/bin is the SAME physical dir as the
# shipped shim, so the installer must exclude it too (REQ-008, REQ-015). A
# logical pwd keeps the link's spelling and misses the identity.
link=$work/pathlink; rm -rf "$link"; ln -s "$pkg/bin" "$link"
before=$("$work/tools/sha256" "$pkg/bin/gc-runtime-rex")
reset_logs
out=$(env -i HOME="$work/home-link" PATH="$link:$nogh_path" \
  FAKE_UNAME_S=Darwin FAKE_UNAME_M=arm64 FAKE_RELEASE_DIR="$work/release" \
  FAKE_CURL_LOG="$work/curl.log" FAKE_CURL_HTTP=200 \
  sh "$pkg/install.sh" 2>&1) || fail "install (symlink to pack/bin on PATH) failed: $out"
after=$("$work/tools/sha256" "$pkg/bin/gc-runtime-rex")
[ "$before" = "$after" ] ||
  fail "installer overwrote the shipped shim via a SYMLINK to pack/bin on PATH (logical pwd missed the identity)"
[ -x "$work/home-link/.local/bin/gc-runtime-rex" ] ||
  fail "installer did not fall back to ~/.local/bin when PATH held only a symlink to pack/bin"

# The FALLBACK itself can be a link to the shipped shim (~/.local/bin -> pack/bin)
# with no provider on PATH. The physical-identity check must catch the FALLBACK
# too, not only a PATH entry: refuse and leave the shim byte-identical.
fbhome=$work/home-fallback; rm -rf "$fbhome"; mkdir -p "$fbhome/.local"
ln -s "$pkg/bin" "$fbhome/.local/bin"
before=$("$work/tools/sha256" "$pkg/bin/gc-runtime-rex")
reset_logs
if out=$(env -i HOME="$fbhome" PATH="$nogh_path" \
    FAKE_UNAME_S=Darwin FAKE_UNAME_M=arm64 FAKE_RELEASE_DIR="$work/release" \
    FAKE_CURL_LOG="$work/curl.log" FAKE_CURL_HTTP=200 \
    sh "$pkg/install.sh" 2>&1); then
  fail "installer wrote to a fallback dir that IS the shipped shim dir"
fi
printf '%s' "$out" | grep -qi 'shipped shim' || fail "fallback-alias refusal does not name the shipped shim: [$out]"
after=$("$work/tools/sha256" "$pkg/bin/gc-runtime-rex")
[ "$before" = "$after" ] || fail "installer overwrote the shipped shim via the ~/.local/bin fallback link"

# GC_RUNTIME_INSTALL_DIR + --from-source skips the PATH warning — proven with a
# fake `go` that writes a stub and exits 0, deterministically reaching warn_path.
fakego=$work/fakego; mkdir -p "$fakego"
cat > "$fakego/go" <<'GOEOF'
#!/bin/sh
[ -n "${GOBIN:-}" ] && printf '#!/bin/sh\necho fake-built\n' > "$GOBIN/gc-runtime-rex" && chmod 755 "$GOBIN/gc-runtime-rex"
exit 0
GOEOF
chmod 755 "$fakego/go"
rm -rf "$work/cache"; reset_logs
out=$(env -i HOME="$work/home-src" PATH="$fakego:$nogh_path" \
  FAKE_UNAME_S=Darwin FAKE_UNAME_M=arm64 \
  GC_RUNTIME_INSTALL_DIR="$work/cache/src" \
  sh "$installer" --from-source 2>&1) ||
  fail "--from-source with GC_RUNTIME_INSTALL_DIR failed: $out"
[ -x "$work/cache/src/gc-runtime-rex" ] || fail "--from-source did not write the install-dir target"
printf '%s' "$out" | grep -q 'not on PATH\|add it' &&
  fail "--from-source with GC_RUNTIME_INSTALL_DIR still printed a PATH warning"

# Transport failure, no gh: reports the transport cause, not a private hint.
rm -rf "$work/cache"; reset_logs
if FAKE_CURL_MODE=transport run_install Darwin arm64 "$work/home-transport" "$nogh_path"; then
  fail "install did not fail on a curl transport failure"
fi
printf '%s' "$out" | grep -qi 'reach github' || fail "transport failure not reported as transport"
printf '%s' "$out" | grep -q 'Connection refused' || fail "transport failure dropped curl's diagnostic (REQ-007)"
printf '%s' "$out" | grep -qi 'private' && fail "a transport failure was reported as a private-release hint"

# A plain 404 with gh ALSO failing: names asset+version, not a private hint.
rm -rf "$work/cache"; reset_logs
if FAKE_CURL_HTTP=404 FAKE_GH_FAIL=1 run_install Darwin arm64 "$work/home-404" "$tools_path"; then
  fail "install did not fail on a 404 with gh failing"
fi
printf '%s' "$out" | grep -q 'gc-runtime-rex_darwin_arm64' || fail "404 message does not name the asset"
printf '%s' "$out" | grep -q "v$version" || fail "404 message does not name the version"
printf '%s' "$out" | grep -qi 'private' && fail "a plain 404 was reported as a private-release hint"

# A checksums.txt-ONLY 404 with gh failing: name the URL curl actually requested.
# Blaming the binary URL (never requested) misdirects the operator (REQ-007).
rm -rf "$work/cache"; reset_logs
if FAKE_CURL_404_NAME=checksums.txt FAKE_GH_FAIL=1 \
    run_install Darwin arm64 "$work/home-404-cksum" "$tools_path"; then
  fail "install did not fail on a checksums-only 404 with gh failing"
fi
printf '%s' "$out" | grep -q "v$version/checksums.txt" ||
  fail "checksums 404 message does not name the checksums.txt URL: [$out]"
printf '%s' "$out" | grep -q "v$version/gc-runtime-rex_darwin_arm64" &&
  fail "checksums 404 blamed the binary URL curl never requested: [$out]"
printf '%s' "$out" | grep -q 'gc-runtime-rex_darwin_arm64' || fail "checksums 404 message does not name the asset"
printf '%s' "$out" | grep -q "v$version" || fail "checksums 404 message does not name the version"

# Authorization failure (401/403) with gh failing: IS a private-release hint.
rm -rf "$work/cache"; reset_logs
if FAKE_CURL_HTTP=403 FAKE_GH_FAIL=1 run_install Darwin arm64 "$work/home-403" "$tools_path"; then
  fail "install did not fail on a 403 with gh failing"
fi
printf '%s' "$out" | grep -qi 'private\|GITHUB_TOKEN' || fail "403 message does not give the private-release remedy"
printf '%s' "$out" | grep -q 'gc-runtime-rex_darwin_arm64' || fail "403 message does not name the asset"
printf '%s' "$out" | grep -q "v$version" || fail "403 message does not name the version"

# Checksum mismatch names the full asset + version + the cause.
rm -rf "$work/cache"; reset_logs
if FAKE_RELEASE_DIR="$work/release-bad" FAKE_CURL_HTTP=200 \
    run_install Darwin arm64 "$work/home-cksum" "$nogh_path"; then
  fail "install did not fail on a checksum mismatch"
fi
printf '%s' "$out" | grep -q 'sha256 mismatch' || fail "checksum error does not name the cause"
printf '%s' "$out" | grep -q 'gc-runtime-rex_darwin_arm64' || fail "checksum error does not name the full asset"
printf '%s' "$out" | grep -q "v$version" || fail "checksum error does not name the version"

# Offline + FAILED gh fallback (REQ-007): the transport cause must survive.
rm -rf "$work/cache"; reset_logs
if FAKE_CURL_MODE=transport FAKE_GH_FAIL=1 \
    run_install Darwin arm64 "$work/home-trans-gh" "$tools_path"; then
  fail "install did not fail on transport + failed gh fallback"
fi
[ -s "$work/gh.log" ] || fail "transport+gh case did not try gh"
printf '%s' "$out" | grep -qi 'reach github' || fail "transport cause was masked when gh also failed"
printf '%s' "$out" | grep -q 'Connection refused' || fail "curl's diagnostic was dropped when gh also failed (REQ-007)"
printf '%s' "$out" | grep -q 'gc-runtime-rex_darwin_arm64' || fail "transport+gh message does not name the asset"
printf '%s' "$out" | grep -q "v$version" || fail "transport+gh message does not name the version"
printf '%s' "$out" | grep -qi 'private' && fail "transport+gh was reported as a private-release hint"

# curl transport error + a STALLED gh fallback: the timeout (124) must keep
# curl's own diagnostic alongside the budget message (REQ-007).
rm -rf "$work/cache"; reset_logs
rc=0
out=$(env -i HOME="$work/home-trans-stall" PATH="$tools_path" \
  FAKE_UNAME_S=Darwin FAKE_UNAME_M=arm64 FAKE_RELEASE_DIR="$work/release" \
  FAKE_CURL_LOG="$work/curl.log" FAKE_GH_LOG="$work/gh.log" \
  FAKE_CURL_MODE=transport FAKE_GH_SLEEP=30 \
  GC_RUNTIME_INSTALL_DIR="$work/cache" GC_RUNTIME_INSTALL_DEADLINE=2 \
  sh "$installer" 2>&1) || rc=$?
[ "$rc" -eq 124 ] || fail "stalled gh fallback exited $rc, want 124"
printf '%s' "$out" | grep -q 'Connection refused' ||
  fail "the gh timeout dropped curl's diagnostic (REQ-007): [$out]"
printf '%s' "$out" | grep -q "v$version" || fail "the gh timeout does not name the version"

# Bounded cold fetch under BOTH sh and non-interactive dash (REQ-018/REQ-020): a
# stalled fetch is reaped BY PID at the budget, with no staging temp and no target.
for ish in sh dash; do
  command -v "$ish" >/dev/null 2>&1 || continue
  td=$work/timeout-$ish; rm -rf "$td"; mkdir -p "$td/home"
  start=$(date +%s)
  rc=0
  out=$(env -i HOME="$td/home" PATH="$nogh_path" \
      FAKE_UNAME_S=Darwin FAKE_UNAME_M=arm64 FAKE_RELEASE_DIR="$work/release" \
      FAKE_CURL_SLEEP=30 GC_RUNTIME_INSTALL_DIR="$td/cache" \
      GC_RUNTIME_INSTALL_DEADLINE=2 "$ish" "$installer" 2>&1) || rc=$?
  # 124 is the shim's timeout signal; exit 1 would take the generic remedy.
  [ "$rc" -eq 124 ] || fail "$ish: stalled install exited $rc, want 124 (REQ-018)"
  elapsed=$(( $(date +%s) - start ))
  [ "$elapsed" -lt 10 ] || fail "$ish: stalled fetch not reaped at the budget ($elapsed s)"
  printf '%s' "$out" | grep -qi 'budget\|stalled\|exceeded\|timed out' || fail "$ish: timeout did not name the cause: [$out]"
  printf '%s' "$out" | grep -q 'checksums.txt' || fail "$ish: checksum timeout does not name checksums.txt (REQ-007)"
  printf '%s' "$out" | grep -q 'gc-runtime-rex_darwin_arm64' || fail "$ish: checksum timeout does not name the provider asset (REQ-007)"
  printf '%s' "$out" | grep -q "v$version" || fail "$ish: checksum timeout does not name the release version (REQ-007)"
  [ -z "$(ls "$td/cache"/.gc-runtime-rex.* 2>/dev/null)" ] || fail "$ish: timeout left a staging temp"
  [ ! -e "$td/cache/gc-runtime-rex" ] || fail "$ish: timeout wrote a target"
  sleep 2
  [ ! -e "$td/cache/gc-runtime-rex" ] || fail "$ish: a reaped fetch still produced a target"

  # A fast install under the same shell returns promptly. The stall mode is
  # cleared: FAKE_CURL_SLEEP=30 would otherwise hang this fetch too.
  rm -rf "$td/cache"; start=$(date +%s)
  env -i HOME="$td/home" PATH="$nogh_path" \
    FAKE_UNAME_S=Darwin FAKE_UNAME_M=arm64 FAKE_RELEASE_DIR="$work/release" \
    FAKE_CURL_HTTP=200 FAKE_CURL_SLEEP= GC_RUNTIME_INSTALL_DIR="$td/cache" \
    GC_RUNTIME_INSTALL_DEADLINE=20 "$ish" "$installer" >/dev/null 2>"$work/fast.err" ||
    { echo "DEBUG fast err:"; cat "$work/fast.err"; fail "$ish: fast install failed"; }
  elapsed=$(( $(date +%s) - start ))
  [ "$elapsed" -lt 10 ] || fail "$ish: fast install was held to the budget ($elapsed s)"
  [ -x "$td/cache/gc-runtime-rex" ] || fail "$ish: fast install did not write the target"

  # A FAST, SUCCESSFUL gh fallback must not be held to the budget by an orphaned
  # watchdog's inherited stdout.
  gd=$td/ghfast; rm -rf "$gd"; mkdir -p "$gd/home"; reset_logs
  start=$(date +%s)
  env -i HOME="$gd/home" PATH="$tools_path" \
    FAKE_UNAME_S=Darwin FAKE_UNAME_M=arm64 FAKE_RELEASE_DIR="$work/release" \
    FAKE_CURL_LOG="$work/curl.log" FAKE_GH_LOG="$work/gh.log" \
    FAKE_CURL_MODE=transport GC_RUNTIME_INSTALL_DIR="$gd/cache" \
    GC_RUNTIME_INSTALL_DEADLINE=30 "$ish" "$installer" >/dev/null 2>&1 ||
    fail "$ish: fast gh fallback failed"
  elapsed=$(( $(date +%s) - start ))
  [ "$elapsed" -lt 10 ] || fail "$ish: fast gh fallback was held to the budget ($elapsed s)"
  [ -s "$work/gh.log" ] || fail "$ish: fast gh fallback did not use gh"
  [ -x "$gd/cache/gc-runtime-rex" ] || fail "$ish: fast gh fallback did not install"
done

# REQ-008: a manual install imposes NO deadline. A clock-less PATH (no date) with
# a 10 s slow-but-successful asset must succeed when no deadline is set, and be
# reaped when one is set.
nodate=$work/sysbin-nodate; rm -rf "$nodate"; mkdir -p "$nodate"
for t in sh awk sed grep cut mkdir mktemp mv rm cp chmod dirname cat \
         sha256sum shasum head sleep; do
  p=$(command -v "$t" 2>/dev/null) || continue
  ln -sf "$p" "$nodate/$t"
done
nd_path="$work/tools-nogh:$nodate"
# The <=3 fetches SHARE one whole-fetch budget. With a real clock a slow-but-
# SUCCESSFUL checksums fetch (5 s) consumes most of a 6 s budget, so the stalled
# ASSET fetch is reaped at only the ~1 s remainder and the whole run ends near 6 s.
# A fresh per-fetch budget would run 5+6 = 11 s. The timeout message names the
# asset, which proves the asset request actually ran (FAKE_CURL_SLEEP never logs).
rm -rf "$work/cache"; reset_logs
start=$(date +%s)
rc=0
out=$(env -i HOME="$work/home-budget" PATH="$nogh_path" \
    FAKE_UNAME_S=Darwin FAKE_UNAME_M=arm64 FAKE_RELEASE_DIR="$work/release" \
    FAKE_CURL_SLEEP_CHECKSUMS=5 FAKE_CURL_SLEEP_ASSET=30 \
    GC_RUNTIME_INSTALL_DIR="$work/cache" GC_RUNTIME_INSTALL_DEADLINE=6 \
    sh "$installer" 2>&1) || rc=$?
elapsed=$(( $(date +%s) - start ))
[ "$rc" -eq 124 ] || fail "shared-budget case: exited $rc, want 124: [$out]"
[ "$elapsed" -lt 9 ] || fail "shared-budget case: the fetches did not share one budget ($elapsed s; a per-fetch budget needs 5+6)"
[ "$elapsed" -ge 5 ] || fail "shared-budget case: the slow checksums did not delay the run ($elapsed s)"
# die_budget names the asset even when the CHECKSUMS request timed out, so match
# the "fetching <asset> for release" phrase: only the asset timeout prints it.
printf '%s' "$out" | grep -q 'fetching gc-runtime-rex_darwin_arm64 for release' ||
  fail "shared-budget case: the ASSET request never ran (a checksums timeout also names the asset): [$out]"

# PR39 review: a clock that answered at the start and LATER fails must STOP the
# bounded install. Restarting the budget would let the fetches outlive gc's 30 s
# handshake. The installer exits with the cause and never asks for a fetch; it
# must not fall back to a full or sliced budget.
flick=$work/tools-flickdate; rm -rf "$flick"; mkdir -p "$flick"
real_date=$(command -v date)
cat > "$flick/date" <<FLICKEOF
#!/bin/sh
n=0
[ -f "\$FAKE_DATE_COUNTER" ] && n=\$(cat "\$FAKE_DATE_COUNTER")
n=\$(( n + 1 ))
printf '%s\\n' "\$n" > "\$FAKE_DATE_COUNTER"
[ "\$n" -le 1 ] || exit 1
exec "$real_date" "\$@"
FLICKEOF
chmod 755 "$flick/date"
rm -f "$work/date-counter"
rm -rf "$work/cache"; reset_logs
start=$(date +%s)
rc=0
out=$(env -i HOME="$work/home-flick" PATH="$flick:$nogh_path" \
    FAKE_DATE_COUNTER="$work/date-counter" \
    FAKE_UNAME_S=Darwin FAKE_UNAME_M=arm64 FAKE_RELEASE_DIR="$work/release" \
    FAKE_CURL_LOG="$work/curl.log" \
    FAKE_CURL_SLEEP_ASSET=30 GC_RUNTIME_INSTALL_DIR="$work/cache" \
    GC_RUNTIME_INSTALL_DEADLINE=4 sh "$installer" 2>&1) || rc=$?
elapsed=$(( $(date +%s) - start ))
[ "$rc" -ne 0 ] || fail "flickering-clock case: the lost clock did not stop the install: [$out]"
printf '%s' "$out" | grep -qi 'clock' ||
  fail "flickering-clock case: the cause did not name the clock: [$out]"
printf '%s' "$out" | grep -q 'unbound variable\|parameter not set' &&
  fail "flickering clock left a budget variable unset: [$out]"
[ ! -s "$work/curl.log" ] ||
  fail "flickering-clock case: a fetch ran after the clock was lost: [$(cat "$work/curl.log")]"
[ "$elapsed" -lt 5 ] || fail "flickering-clock case: the install was not stopped promptly ($elapsed s)"

rm -rf "$work/cache-manual"; reset_logs
if ! out=$(env -i HOME="$work/home-manual" PATH="$nd_path" \
    FAKE_UNAME_S=Darwin FAKE_UNAME_M=arm64 FAKE_RELEASE_DIR="$work/release" \
    FAKE_CURL_LOG="$work/curl.log" FAKE_CURL_HTTP=200 FAKE_CURL_SLEEP_ASSET=10 \
    GC_RUNTIME_INSTALL_DIR="$work/cache-manual" \
    sh "$installer" 2>&1); then
  fail "manual install (no deadline) was wrongly reaped (REQ-008): $out"
fi
[ -x "$work/cache-manual/gc-runtime-rex" ] || fail "manual install did not write the target"
rm -rf "$work/cache-manual"
if env -i HOME="$work/home-manual" PATH="$nd_path" \
    FAKE_UNAME_S=Darwin FAKE_UNAME_M=arm64 FAKE_RELEASE_DIR="$work/release" \
    FAKE_CURL_HTTP=200 FAKE_CURL_SLEEP_ASSET=10 \
    GC_RUNTIME_INSTALL_DIR="$work/cache-manual" \
    GC_RUNTIME_INSTALL_DEADLINE=9 sh "$installer" >/dev/null 2>&1; then
  fail "a bounded install did not reap a slow fetch"
fi
[ ! -e "$work/cache-manual/gc-runtime-rex" ] || fail "a reaped install wrote the target"

# The existing-provider 'inventory' probe shares the fetch budget. A provider
# left blocking by a concurrent cold install must not hang this installer past
# gc's deadline, and a timed-out probe is NOT treated as a stub.
blockdir=$work/block-bin; rm -rf "$blockdir"; mkdir -p "$blockdir"
printf '#!/bin/sh\nsleep 30\n' > "$blockdir/gc-runtime-rex"
chmod 755 "$blockdir/gc-runtime-rex"
start=$(date +%s)
if out=$(env -i HOME="$work/home-block" PATH="$nogh_path" \
    FAKE_UNAME_S=Darwin FAKE_UNAME_M=arm64 FAKE_RELEASE_DIR="$work/release" \
    FAKE_CURL_LOG="$work/curl.log" FAKE_CURL_HTTP=200 \
    GC_RUNTIME_INSTALL_DIR="$blockdir" GC_RUNTIME_INSTALL_DEADLINE=4 \
    sh "$installer" 2>&1); then
  fail "install did not stop on a blocking inventory probe"
fi
elapsed=$(( $(date +%s) - start ))
printf '%s' "$out" | grep -q 'inventory' ||
  fail "blocking-probe timeout did not name the probe: $out"
printf '%s' "$out" | grep -q 'refusing to replace a working gc-runtime-rex' &&
  fail "a PROBE TIMEOUT was reported as a stub, so nothing would install: $out"
[ "$elapsed" -lt 12 ] || fail "the inventory probe was not bounded ($elapsed s)"

# PR39 review: without a clock the fetch slices share what earlier fetches left.
# The old 1 s floor gave every attempt its own second, so 3 attempts could outlast
# a budget under 3 s. With deadline=1 the fast checksums fetch spends the budget,
# so the asset request must not launch at all.
rm -rf "$work/cache-noclk"; reset_logs
rc=0
out=$(env -i HOME="$work/home-noclk" PATH="$nd_path" \
    FAKE_UNAME_S=Darwin FAKE_UNAME_M=arm64 FAKE_RELEASE_DIR="$work/release" \
    FAKE_CURL_LOG="$work/curl.log" FAKE_CURL_HTTP=200 \
    GC_RUNTIME_INSTALL_DIR="$work/cache-noclk" \
    GC_RUNTIME_INSTALL_DEADLINE=1 sh "$installer" 2>&1) || rc=$?
[ "$rc" -eq 124 ] || fail "clock-less slice case: exited $rc, want 124: [$out]"
printf '%s' "$out" | grep -q 'exceeded the 1s budget' ||
  fail "clock-less slice case: the cause did not name the spent budget: [$out]"
grep -q 'gc-runtime-rex_darwin_arm64' "$work/curl.log" 2>/dev/null &&
  fail "clock-less slice case: the asset launched after the budget was spent"
[ ! -e "$work/cache-noclk/gc-runtime-rex" ] || fail "clock-less slice case: wrote the target"

# PR39 review: a bounded, CLOCK-LESS reinstall must reserve slices for BOTH
# inventory probes as well as the fetches. With three slices the existing
# provider's probe spent the last one, so the CANDIDATE probe timed out and a
# working target blocked its own replacement. A working target must reinstall
# (here: already current) and must stay intact.
reinstall=$work/cache-noclk-reinstall; rm -rf "$reinstall"; mkdir -p "$reinstall"
cp "$work/release/gc-runtime-rex_darwin_arm64" "$reinstall/gc-runtime-rex"
chmod 755 "$reinstall/gc-runtime-rex"
before=$("$work/tools/sha256" "$reinstall/gc-runtime-rex")
reset_logs
rc=0
out=$(env -i HOME="$work/home-noclk-reinstall" PATH="$nd_path" \
    FAKE_UNAME_S=Darwin FAKE_UNAME_M=arm64 FAKE_RELEASE_DIR="$work/release" \
    FAKE_CURL_LOG="$work/curl.log" FAKE_CURL_HTTP=200 \
    GC_RUNTIME_INSTALL_DIR="$reinstall" \
    GC_RUNTIME_INSTALL_DEADLINE=20 sh "$installer" 2>&1) || rc=$?
[ "$rc" -eq 0 ] ||
  fail "clock-less reinstall case: exited $rc, want 0: [$out]"
printf '%s' "$out" | grep -q 'cannot prove it is not a stub' &&
  fail "clock-less reinstall case: the candidate probe never ran: [$out]"
after=$("$work/tools/sha256" "$reinstall/gc-runtime-rex")
[ "$before" = "$after" ] || fail "clock-less reinstall case: the working target changed"
assert_provider "$reinstall/gc-runtime-rex" noclk-reinstall

# REQ-020: a bounded install without sleep degrades to unbounded, not fail-open.
nosleep=$work/sysbin-nosleep; rm -rf "$nosleep"; mkdir -p "$nosleep"
for t in sh awk sed grep cut mkdir mktemp mv rm cp chmod dirname cat \
         sha256sum shasum head date; do
  p=$(command -v "$t" 2>/dev/null) || continue
  ln -sf "$p" "$nosleep/$t"
done
ns_path="$work/tools-nogh:$nosleep"
[ ! -e "$nosleep/sleep" ] || fail "test harness bug: sleepless PATH carries sleep"
out=$(env -i HOME="$work/home-nosleep" PATH="$ns_path" \
  FAKE_UNAME_S=Darwin FAKE_UNAME_M=arm64 FAKE_RELEASE_DIR="$work/release" \
  FAKE_CURL_LOG="$work/curl.log" FAKE_CURL_HTTP=200 \
  GC_RUNTIME_INSTALL_DIR="$work/cache-nosleep" GC_RUNTIME_INSTALL_DEADLINE=9 \
  sh "$installer" 2>&1) ||
  fail "bounded install without sleep did not degrade to unbounded (REQ-020): $out"
[ -x "$work/cache-nosleep/gc-runtime-rex" ] ||
  fail "bounded install without sleep did not install a good fetch (REQ-020)"

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

# No provider yet and no cache: the check probes the CACHED binary and reports
# a remediable cold cache. It names the --fix / entry-point remedy and does NOT
# advise editing PATH. The probe is read-only: it must not create the cache.
home=$work/home-doctor
mkdir -p "$home"
doctor_path="$rexdir:$nogh_path"
reset_logs
if out=$(env -i HOME="$home" PATH="$doctor_path" \
    FAKE_UNAME_S=Darwin FAKE_UNAME_M=arm64 FAKE_RELEASE_DIR="$work/release" \
    FAKE_CURL_LOG="$work/curl.log" FAKE_GH_LOG="$work/gh.log" \
    "$gc_bin" doctor --check rex-gascity-runtime:rex-binary --city "$city" --verbose 2>&1); then
  fail "gc doctor passed with no provider installed: $out"
fi
printf '%s' "$out" | grep -qi -- '--fix\|bin/gc-runtime-rex\|start a session' \
  || fail "gc doctor cold remediation does not name --fix / the entry point: $out"
printf '%s' "$out" | grep -qi 'add .*to.*PATH\|on PATH so gc' \
  && fail "gc doctor cold remediation still gives PATH advice: $out"
[ -e "$home/.cache/rex-gascity" ] &&
  fail "gc doctor check wrote the cache on a cold read (not read-only)"
if printf '%s' "$out" | grep -q "go install"; then
  fail "gc doctor still offers a Go build: $out"
fi

# --fix warms the version-keyed cache the shim reads, under HOME (no
# GC_RUNTIME_CACHE_DIR), not $HOME/.local/bin. The cached provider then makes
# gc doctor green without any install on the check path.
reset_logs
env -i HOME="$home" PATH="$doctor_path" \
  FAKE_UNAME_S=Darwin FAKE_UNAME_M=arm64 FAKE_RELEASE_DIR="$work/release" \
  FAKE_CURL_LOG="$work/curl.log" FAKE_GH_LOG="$work/gh.log" \
  "$gc_bin" doctor --check rex-gascity-runtime:rex-binary --city "$city" --fix --verbose > "$work/fix.out" 2>&1 ||
  { cat "$work/fix.out" >&2; fail "gc doctor --fix failed"; }
cached=$home/.cache/rex-gascity/$version/darwin-arm64/gc-runtime-rex
[ -x "$cached" ] || fail "gc doctor --fix did not warm the cache: $cached"
[ ! -e "$home/.local/bin/gc-runtime-rex" ] ||
  fail "gc doctor --fix wrote to ~/.local/bin, not the cache"
"$cached" version | grep -q "$version" || fail "cached provider does not report $version"
assert_provider "$cached" doctor
out=$(env -i HOME="$home" PATH="$doctor_path" "$gc_bin" doctor --check rex-gascity-runtime:rex-binary --city "$city" 2>&1) ||
  fail "gc doctor is not green after --fix: $out"
printf '%s' "$out" | grep -q "rex-gascity-runtime:rex-binary" || fail "gc doctor did not report the rex-gascity-runtime:rex-binary check"

echo "check-installer: ok"
