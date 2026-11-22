#!/bin/sh
# Unit + integration checks for the self-installing rex runtime shim.
# Self-contained: builds its own fake curl/gh/uname + release in a mktemp dir
# and never writes outside it. Mirrors the harness in check-installer.sh:7-120.
set -eu

repo=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
fail() { echo "check-runtime-shim: $*" >&2; exit 1; }

work=$(mktemp -d "${TMPDIR:-/tmp}/check-runtime-shim.XXXXXX") || fail "mktemp failed"
trap 'rm -rf "$work"' EXIT

fake_tools=$work/tools
mkdir -p "$fake_tools" "$work/home" "$work/release"

# --- fake toolchain ---------------------------------------------------------

cat > "$fake_tools/curl" <<'FAKE'
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
if [ -n "${FAKE_CURL_SLEEP:-}" ]; then
  exec sleep "$FAKE_CURL_SLEEP"
fi
if [ "${FAKE_CURL_MODE:-ok}" = transport ]; then
  echo 'curl: (7) Failed to connect to github.com port 443' >&2
  exit 7
fi
name=${url##*/}
if [ -n "${FAKE_CURL_SLEEP_CHECKSUMS:-}" ] && [ "$name" = checksums.txt ]; then
  sleep "$FAKE_CURL_SLEEP_CHECKSUMS"
fi
if [ -n "${FAKE_CURL_SLEEP_ASSET:-}" ] && [ "$name" != checksums.txt ]; then
  sleep "$FAKE_CURL_SLEEP_ASSET"
fi
code=${FAKE_CURL_HTTP:-200}
if [ "$code" != 200 ]; then
  printf '%s' "$code"
  exit 22
fi
FAKE_RELEASE_DIR=${FAKE_RELEASE_DIR:-$(dirname "$0")/../release}
[ -f "$FAKE_RELEASE_DIR/$name" ] || { printf '%s' 404; exit 22; }
cp "$FAKE_RELEASE_DIR/$name" "$out" || exit 1
printf '%s' 200
FAKE

cat > "$fake_tools/gh" <<'FAKE'
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

cat > "$fake_tools/uname" <<'FAKE'
#!/bin/sh
case "$1" in
  -s) printf '%s\n' "${FAKE_UNAME_S:-Darwin}" ;;
  -m) printf '%s\n' "${FAKE_UNAME_M:-arm64}" ;;
  *) printf '%s\n' "${FAKE_UNAME_S:-Darwin}" ;;
esac
FAKE

cat > "$fake_tools/sha256" <<'FAKE'
#!/bin/sh
# sha256 <file> -> hex digest, using whichever tool this host has.
if command -v sha256sum >/dev/null 2>&1; then
  sha256sum "$1" | cut -d' ' -f1
else
  shasum -a 256 "$1" | cut -d' ' -f1
fi
FAKE

chmod 755 "$fake_tools/curl" "$fake_tools/gh" "$fake_tools/uname" "$fake_tools/sha256"

# The deadline/concurrency cases need date+sleep, and the reaped-fetch logic
# sleeps; link them into the fake tool dir. dash is linked for the POSIX-sh
# cases. A host without one skips it.
for t in date sleep dash; do
  p=$(command -v "$t" 2>/dev/null) || continue
  ln -sf "$p" "$fake_tools/$t"
done

# A release asset: answers the two ops the doctor check reads.
make_asset() {
  cat > "$1" <<ASSET
#!/bin/sh
case "\$1" in
  protocol) printf '%s\n' '{"version":0,"capabilities":["report-attachment"]}' ;;
  version) printf '%s\n' '{"version":"$version"}' ;;
  *) exit 2 ;;
esac
ASSET
  chmod 755 "$1"
}

# --- harness helpers (shared by the Task 3-7 cases) ---
# [pack] version, read from the real pack.toml the shim/doctor read (same awk
# extractor as install.sh:37-43).
pack_version() {
  awk -F'"' '
    /^[[:space:]]*\[pack\][[:space:]]*$/ { in_pack = 1; next }
    /^[[:space:]]*\[/ { in_pack = 0 }
    in_pack && /^[[:space:]]*version[[:space:]]*=/ { print $2; exit }
  ' "$repo/pack/pack.toml"
}
# This host's real uname, fed to the fake uname so the positive cases resolve a
# SUPPORTED platform matching the mapped $plat below.
host_uname_s() { uname -s; }
host_uname_m() { uname -m; }
# Map this host's uname to the release token set the way _lib.sh/install.sh do.
host_os() {
  case "$(uname -s)" in
    Darwin) echo darwin ;; Linux) echo linux ;;
    *) echo "check-runtime-shim: unsupported host os $(uname -s)" >&2; return 1 ;;
  esac
}
host_arch() {
  case "$(uname -m)" in
    x86_64|amd64) echo amd64 ;; aarch64|arm64) echo arm64 ;;
    *) echo "check-runtime-shim: unsupported host arch $(uname -m)" >&2; return 1 ;;
  esac
}

version=$(pack_version)
[ -n "$version" ] || fail "pack.toml declares no [pack] version"

assets="gc-runtime-rex_darwin_amd64 gc-runtime-rex_darwin_arm64 gc-runtime-rex_linux_amd64 gc-runtime-rex_linux_arm64"
: > "$work/release/checksums.txt"
for asset in $assets; do
  make_asset "$work/release/$asset"
  hex=$("$fake_tools/sha256" "$work/release/$asset")
  printf '%s  %s\n' "$hex" "$asset" >> "$work/release/checksums.txt"
done

# --- _lib.sh cases ----------------------------------------------------------

lib=$repo/pack/bin/_lib.sh
[ -f "$lib" ] || fail "pack/bin/_lib.sh does not exist"

# Default cache root honors GC_RUNTIME_CACHE_DIR.
( . "$lib"
  GC_RUNTIME_CACHE_DIR=/x/cache
  got=$(gc_runtime_cache_root)
  [ "$got" = /x/cache ] || { echo "cache_root=$got" >&2; exit 1; }
) || fail "gc_runtime_cache_root did not honor GC_RUNTIME_CACHE_DIR"

# XDG fallback when GC_RUNTIME_CACHE_DIR is unset.
( . "$lib"
  unset GC_RUNTIME_CACHE_DIR
  XDG_CACHE_HOME=/x/xdg
  got=$(gc_runtime_cache_root)
  [ "$got" = /x/xdg/rex-gascity ] || { echo "cache_root=$got" >&2; exit 1; }
) || fail "gc_runtime_cache_root XDG fallback wrong"

# Platform map for darwin/arm64 via fake uname.
( . "$lib"
  PATH=$fake_tools:$PATH; export PATH
  FAKE_UNAME_S=Darwin FAKE_UNAME_M=arm64; export FAKE_UNAME_S FAKE_UNAME_M
  got=$(gc_runtime_platform) || exit 1
  [ "$got" = darwin-arm64 ] || { echo "plat=$got" >&2; exit 1; }
) || fail "gc_runtime_platform darwin-arm64 wrong"

# Unsupported arch names the rejected machine and returns non-zero.
( . "$lib"
  PATH=$fake_tools:$PATH; export PATH
  FAKE_UNAME_S=Linux FAKE_UNAME_M=riscv64; export FAKE_UNAME_S FAKE_UNAME_M
  if msg=$(gc_runtime_platform 2>&1); then
    echo "expected failure, got $msg" >&2; exit 1
  fi
  printf '%s' "$msg" | grep -q riscv64 || { echo "msg=$msg" >&2; exit 1; }
) || fail "gc_runtime_platform did not reject riscv64 naming it"

echo "check-runtime-shim: ok"   # moved to the end as later tasks append cases
