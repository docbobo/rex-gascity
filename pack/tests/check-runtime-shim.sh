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

# A rex CLI newer than the pack pin. The fix refuses a too-old CLI before it
# fetches, so the warm cases must not inherit the host's rex version.

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

# --- shim cases -----------------------------------------------------------

shim=$repo/pack/bin/gc-runtime-rex
[ -x "$shim" ] || fail "pack/bin/gc-runtime-rex is not executable"

cache=$work/cache
pv=$(pack_version)                 # [pack] version, via the harness helper
plat=$(host_os)-$(host_arch)
binpath=$cache/$pv/$plat/gc-runtime-rex
run_shim() {           # run_shim <extra-env...>  -- captures stdout only
  # Clear an inherited GC_RUNTIME_INSTALL_DEADLINE: an operator export must not
  # decide these cases. A case override after "$@" still wins.
  env PATH="$fake_tools:$PATH" HOME="$work/home" \
    FAKE_RELEASE_DIR="$work/release" GC_RUNTIME_CACHE_DIR="$cache" \
    GC_RUNTIME_INSTALL_DEADLINE= \
    FAKE_UNAME_S="$(host_uname_s)" FAKE_UNAME_M="$(host_uname_m)" "$@" \
    sh "$shim" protocol
}

# Cold cache: the first call installs. The executed binary answers protocol on
# stdout with EXACTLY the RPP handshake. Install chatter stays on stderr.
handshake='{"version":0,"capabilities":["report-attachment"]}'
rm -rf "$cache"; rm -f "$work/curl.log" "$work/gh.log"
stdout=$(run_shim FAKE_CURL_LOG="$work/curl.log" FAKE_GH_LOG="$work/gh.log" 2>"$work/err.log") \
  || fail "cold shim call failed: $(cat "$work/err.log")"
[ "$stdout" = "$handshake" ] || fail "cold shim stdout was not exactly the RPP handshake: [$stdout]"
[ -s "$work/curl.log" ] || fail "cold call made no download"
[ -e "$work/gh.log" ] && fail "cold call used the gh fallback after curl succeeded"
[ -x "$binpath" ] || fail "cold call did not cache $binpath"

# Warm cache: the second call makes NO network request and emits the exact handshake.
rm -f "$work/curl.log" "$work/gh.log"
stdout=$(run_shim FAKE_CURL_LOG="$work/curl.log" FAKE_GH_LOG="$work/gh.log" 2>/dev/null) \
  || fail "warm shim call failed"
[ "$stdout" = "$handshake" ] || fail "warm call did not exec the cached binary: [$stdout]"
[ -e "$work/curl.log" ] && fail "warm call used a network transport (curl)"
[ -e "$work/gh.log" ] && fail "warm call used a network transport (gh)"

# Bad checksum: use a DISPOSABLE tampered release dir; the shared $work/release
# stays intact and nothing in the working tree is touched.
rm -rf "$cache"; relbad=$work/release-bad; rm -rf "$relbad"; cp -R "$work/release" "$relbad"
printf 'tamper\n' >> "$relbad/gc-runtime-rex_${plat%-*}_${plat#*-}"   # os_arch asset
if err=$(run_shim FAKE_RELEASE_DIR="$relbad" 2>&1 >/dev/null); then fail "shim did not fail on checksum mismatch"; fi
# REQ-007: the shim's identifying prefix leads the stderr, BEFORE install.sh's
# own output (the cold-install announcement is the shim's first line).
printf '%s\n' "$err" | sed -n '1p' | grep -q '^gc-runtime-rex:' || fail "install failure did not LEAD with the shim's gc-runtime-rex: prefix"
printf '%s' "$err" | grep -q "gc-runtime-rex_${plat%-*}_${plat#*-}" || fail "checksum error did not name the full asset"
printf '%s' "$err" | grep -q "$pv" || fail "checksum error did not name the version"
[ -e "$binpath" ] && fail "cache left a binary after a failed install"
[ -z "$(ls "$cache/$pv/$plat"/.gc-runtime-rex.* 2>/dev/null)" ] || fail "a partial staging file was left behind"

# Unsupported platform: no asset resolves → non-zero; the shim's prefix LEADS,
# then the _lib.sh cause (the rejected arch) follows.
rm -rf "$cache"
if err=$(env PATH="$fake_tools:$PATH" HOME="$work/home" GC_RUNTIME_CACHE_DIR="$cache" \
    FAKE_UNAME_S=Linux FAKE_UNAME_M=riscv64 sh "$shim" protocol 2>&1 >/dev/null); then
  fail "shim did not fail on unsupported platform"
fi
printf '%s\n' "$err" | sed -n '1p' | grep -q '^gc-runtime-rex:' || fail "unsupported-platform failure did not LEAD with the shim's gc-runtime-rex: prefix"
printf '%s' "$err" | grep -q riscv64 || fail "unsupported-platform error did not name the rejected arch after the prefix"

# Deadline: a fetch that stalls far past the budget is REAPED BY PID at the
# budget — install.sh runs the fetch as a direct background child and kills that
# child (no job control, no tty), so the stalled curl does not outlive the
# budget. The call returns within a few seconds (near the budget), not after the
# child's full 30 s sleep, and before gc's 30 s protocol deadline, naming the
# timeout cause and the gc doctor --fix remedy. GC_RUNTIME_INSTALL_DEADLINE
# shrinks the budget; FAKE_CURL_SLEEP stalls the fake transport well past it.
rm -rf "$cache"
start=$(date +%s)
if err=$(run_shim GC_RUNTIME_INSTALL_DEADLINE=1 FAKE_CURL_SLEEP=30 2>&1 >/dev/null); then
  fail "shim did not fail on a stalled fetch"
fi
elapsed=$(( $(date +%s) - start ))
# A budget-sized bound (not merely < 30) proves the stalled fetch child was
# reaped by PID, not left running while only the install.sh shell was killed.
[ "$elapsed" -lt 10 ] || fail "stalled fetch was not reaped at the budget ($elapsed s; the child outlived the kill)"
printf '%s' "$err" | grep -qi 'tim\|deadline\|slow' || fail "timeout message does not name the cause"
printf '%s' "$err" | grep -q 'gc doctor --fix' || fail "timeout message does not give the --fix remedy"
# No orphaned child finishes the install after we returned: wait out a moment and
# prove the cache stayed empty — no binary, no partial staging file.
sleep 2
[ -e "$binpath" ] && fail "a reaped install still produced a cached binary after return"
[ -z "$(ls "$cache/$pv/$plat"/.gc-runtime-rex.* 2>/dev/null)" ] || fail "a reaped install left a partial staging file"

# Fast success is NOT held open: a cold install that finishes well under a large
# budget returns promptly (install.sh reaps its own per-fetch watchdog), not
# after the budget.
rm -rf "$cache"
start=$(date +%s)
run_shim GC_RUNTIME_INSTALL_DEADLINE=20 >/dev/null 2>&1 || fail "fast cold install failed"
elapsed=$(( $(date +%s) - start ))
[ "$elapsed" -lt 10 ] || fail "fast install was held open by the watchdog ($elapsed s ~ the 20 s budget)"

# Budget normalization: the shim must strip a leading zero from 08 (8 s, in
# range) before handing it to shell arithmetic. Passed through raw, install.sh's
# $(( )) reads it as octal and aborts before any fetch.
rm -rf "$cache"
start=$(date +%s)
if err=$(run_shim GC_RUNTIME_INSTALL_DEADLINE=08 FAKE_CURL_SLEEP=30 2>&1 >/dev/null); then
  fail "shim accepted a leading-zero budget without timing out"
fi
elapsed=$(( $(date +%s) - start ))
printf '%s' "$err" | grep -q 'value too great' && fail "leading-zero budget reached shell arithmetic as octal: $err"
printf '%s' "$err" | grep -q 'exceeded the 8s first-use budget' || fail "leading-zero budget was not normalized to 8s: $err"
[ "$elapsed" -lt 15 ] || fail "leading-zero budget did not bound the fetch ($elapsed s)"

# Restricted PATH (REQ-018): install.sh deliberately falls back to an UNBOUNDED
# foreground fetch when it has no sleep for a watchdog. That fallback is for the
# direct installer. The shim's cold-fetch policy must stay bounded, so with no
# timer it refuses BEFORE any fetch and still names the cause and the remedy.
# Build a PATH with every tool the installer needs EXCEPT sleep, so that an
# unbounded install would otherwise succeed. The fix must refuse before it runs.
rm -rf "$cache"; nosleep=$work/tools-nosleep; rm -rf "$nosleep"; mkdir -p "$nosleep"
for t in sh dirname awk mkdir mktemp cp chmod mv rm sed cut sha256sum shasum date; do
  p=$(command -v "$t" 2>/dev/null) || continue
  ln -sf "$p" "$nosleep/$t"
done
cp "$fake_tools/uname" "$nosleep/uname"; chmod 755 "$nosleep/uname"
cp "$fake_tools/curl" "$nosleep/curl"
[ -e "$nosleep/sha256sum" ] || [ -e "$nosleep/shasum" ] \
  || fail "cannot build a no-sleep PATH: no sha256 tool"
rm -f "$work/nosleep-curl.log"
if err=$(env PATH="$nosleep" HOME="$work/home" GC_RUNTIME_CACHE_DIR="$cache" \
    FAKE_RELEASE_DIR="$work/release" FAKE_UNAME_S="$(host_uname_s)" FAKE_UNAME_M="$(host_uname_m)" \
    FAKE_CURL_LOG="$work/nosleep-curl.log" GC_RUNTIME_INSTALL_DEADLINE=1 \
    sh "$shim" protocol 2>&1 >/dev/null); then
  fail "shim ran a cold install with no sleep on PATH (unbounded)"
fi
printf '%s\n' "$err" | sed -n '1p' | grep -q '^gc-runtime-rex:' || fail "no-sleep refusal did not LEAD with the shim prefix"
printf '%s' "$err" | grep -q 'sleep' || fail "no-sleep refusal did not name the missing timer"
printf '%s' "$err" | grep -q 'gc doctor --fix' || fail "no-sleep refusal did not give the --fix remedy"
[ -e "$work/nosleep-curl.log" ] && fail "shim attempted a fetch with no bounded timer"

# Both gc entry points share the install path, so BOTH 'protocol' and 'start'
# surface the DEADLINE cause and the remedy on a stalled fetch. The generic
# install-failure text also carries a --fix hint, so the deadline line is checked.
for op in protocol start; do
  rm -rf "$cache"
  if err=$(env PATH="$fake_tools:$PATH" HOME="$work/home" \
      FAKE_RELEASE_DIR="$work/release" GC_RUNTIME_CACHE_DIR="$cache" \
      FAKE_UNAME_S="$(host_uname_s)" FAKE_UNAME_M="$(host_uname_m)" \
      GC_RUNTIME_INSTALL_DEADLINE=1 FAKE_CURL_SLEEP=30 \
      sh "$shim" "$op" 2>&1 >/dev/null); then
    fail "shim ($op) did not fail on a stalled fetch"
  fi
  printf '%s' "$err" | grep -q 'exceeded the 1s first-use budget' || fail "shim ($op) did not name the deadline cause"
  printf '%s' "$err" | grep -q 'gc doctor --fix' || fail "shim ($op) timeout message lacks the --fix remedy"
done

# Concurrency (REQ-005): two OVERLAPPING cold misses. The checksum fetch is
# slowed so the second call is still cold when the first is mid-install. Both
# must therefore hit the installer. Each stages to a distinct .$$ temp and
# mv-renames. The cache ends with one complete, runnable binary and no residue.
rm -rf "$cache"; rm -f "$work/curl.log"
run_shim FAKE_CURL_LOG="$work/curl.log" FAKE_CURL_SLEEP_CHECKSUMS=2 >/dev/null 2>&1 & p1=$!
run_shim FAKE_CURL_LOG="$work/curl.log" FAKE_CURL_SLEEP_CHECKSUMS=2 >/dev/null 2>&1 & p2=$!
wait "$p1" || fail "concurrent call 1 failed"
wait "$p2" || fail "concurrent call 2 failed"
[ "$(grep -c 'checksums.txt' "$work/curl.log" 2>/dev/null || :)" -ge 2 ] \
  || fail "concurrent calls did not both take the cold path"
[ -x "$binpath" ] || fail "concurrent calls left no valid cached binary"
[ -z "$(ls "$cache/$pv/$plat"/.gc-runtime-rex.* 2>/dev/null)" ] || fail "concurrent install left a partial staging file"
[ "$("$binpath" protocol)" = "$handshake" ] \
  || fail "cached binary after concurrent install is not a complete executable"

# Command wiring (REQ-002/REQ-015): the pack must point gc at the committed
# shim by a pack-relative path. A separator is what makes gc resolve it against
# the pack dir instead of PATH, so assert the exact value and the separator.
cmd=$(awk -F'"' '
  /^[[:space:]]*\[runtimes[^]]*rex[^]]*\][[:space:]]*$/ { s=1; next }
  /^[[:space:]]*\[/ { s=0 }
  s && /^[[:space:]]*command[[:space:]]*=/ { print $2; exit }
' "$repo/pack/pack.toml")
[ "$cmd" = "bin/gc-runtime-rex" ] || fail "pack.toml command is '$cmd', expected bin/gc-runtime-rex"
case "$cmd" in */*) : ;; *) fail "command must contain a separator for pack-relative resolution" ;; esac

# Doctor fix (REQ-009): `gc doctor --fix` must warm the SAME version-keyed
# cache the shim execs, through the shared _lib.sh, not install onto PATH.
rm -rf "$cache"
env PATH="$fake_tools:$PATH" HOME="$work/home" \
  FAKE_RELEASE_DIR="$work/release" GC_RUNTIME_CACHE_DIR="$cache" \
  GC_REX_BIN="$fake_tools/rex-ok" GC_RUNTIME_INSTALL_DEADLINE= \
  FAKE_UNAME_S="$(host_uname_s)" FAKE_UNAME_M="$(host_uname_m)" \
  sh "$repo/pack/doctor/rex-binary/fix.sh" >/dev/null 2>&1 \
  || fail "fix.sh failed"
[ -x "$binpath" ] || fail "fix.sh did not warm the cache the shim uses"

# Recovery path (REQ-018): a shim call under the TIGHT first-use budget times
# out on a slow release; fix.sh runs out of band with a generous budget and
# warms the SAME cache; the next shim call then fetches nothing.
rm -rf "$cache"; : > "$work/curl.log"
if run_shim GC_RUNTIME_INSTALL_DEADLINE=1 FAKE_CURL_SLEEP_ASSET=3 \
    FAKE_CURL_LOG="$work/curl.log" >/dev/null 2>&1; then
  fail "slow release did not time out under the shim's tight 1 s budget"
fi
[ -x "$binpath" ] && fail "a timed-out shim call still cached a binary"
env PATH="$fake_tools:$PATH" HOME="$work/home" \
  FAKE_RELEASE_DIR="$work/release" GC_RUNTIME_CACHE_DIR="$cache" \
  GC_REX_BIN="$fake_tools/rex-ok" \
  FAKE_UNAME_S="$(host_uname_s)" FAKE_UNAME_M="$(host_uname_m)" \
  GC_RUNTIME_INSTALL_DEADLINE= FAKE_CURL_SLEEP_ASSET=3 \
  sh "$repo/pack/doctor/rex-binary/fix.sh" >/dev/null 2>&1 \
  || fail "fix.sh did not recover a slow release the shim could not fetch in time"
[ -x "$binpath" ] || fail "fix.sh did not warm the cache after the shim timed out"
: > "$work/curl.log"
run_shim FAKE_CURL_LOG="$work/curl.log" >/dev/null 2>&1 || fail "warm shim call after fix failed"
[ -s "$work/curl.log" ] && fail "shim fetched again after fix.sh warmed the cache"

# Doctor check (REQ-010, Task 6): the check probes the CACHED binary directly
# instead of exec'ing the self-installing shim. A cold cache is read-only:
# no download and no write; the remediation names --fix / the entry point and
# gives no PATH advice. A warm cache passes with no new fetch. A cache path the
# check cannot compute is reported as an error and the shim is never exec'd.
#
# The doctor runs against a COPY of the pack whose shipped shim records each
# execution. A regression that execs the shim leaves the marker set, so the
# warm case fails instead of silently passing.
shim_pack=$work/shim-pack
rm -rf "$shim_pack"
mkdir -p "$shim_pack/bin" "$shim_pack/doctor/rex-binary"
cp "$repo/pack/pack.toml" "$shim_pack/pack.toml"
cp "$repo/pack/rex-min-version" "$shim_pack/rex-min-version" 2>/dev/null || true
cp "$repo/pack/bin/_lib.sh" "$shim_pack/bin/_lib.sh"
cp "$repo/pack/bin/gc-runtime-rex" "$shim_pack/bin/gc-runtime-rex.real"
cat > "$shim_pack/bin/gc-runtime-rex" <<'WRAP'
#!/bin/sh
[ -n "${SHIM_LOG:-}" ] && printf '%s\n' "shim ran" >> "$SHIM_LOG"
exec "$(dirname -- "$0")/gc-runtime-rex.real" "$@"
WRAP
chmod 755 "$shim_pack/bin/gc-runtime-rex" "$shim_pack/bin/gc-runtime-rex.real"
cp "$repo/pack/doctor/rex-binary/run.sh" "$shim_pack/doctor/rex-binary/run.sh"
chmod 755 "$shim_pack/doctor/rex-binary/run.sh"

doctor=$shim_pack/doctor/rex-binary/run.sh
[ -x "$doctor" ] || fail "pack/doctor/rex-binary/run.sh is not executable"
printf '#!/bin/sh\ncase "${1:-}" in\n  version) case "${2:-}" in\n    --json) printf '\''{"rex_version":"999.0.0"}\\n'\'' ;;\n    *) printf '\''rex 999.0.0\\n'\'' ;;\n  esac ;;\nesac\n' > "$fake_tools/rex-ok"
chmod 755 "$fake_tools/rex-ok"
run_check() {          # run_check <extra-env...>  -- captures combined output
  env PATH="$fake_tools:$PATH" HOME="$work/home" \
    GC_RUNTIME_CACHE_DIR="$cache" GC_REX_BIN="$fake_tools/rex-ok" \
    SHIM_LOG="$work/shim.log" GC_RUNTIME_INSTALL_DEADLINE= \
    FAKE_UNAME_S="$(host_uname_s)" FAKE_UNAME_M="$(host_uname_m)" \
    FAKE_CURL_LOG="$work/curl.log" "$@" \
    sh "$doctor"
}

rm -rf "$cache"; : > "$work/curl.log"; : > "$work/shim.log"
if out=$(run_check 2>&1); then fail "cold-cache check unexpectedly passed"; fi
[ -s "$work/curl.log" ] && fail "cold-cache check performed a download (not read-only)"
[ -e "$cache" ] && fail "cold-cache check wrote the cache (not read-only)"
[ -s "$work/shim.log" ] && fail "cold-cache check exec'd the shipped shim"
printf '%s' "$out" | grep -qi 'not yet installed' \
  || fail "cold-cache message did not say the provider is not yet installed: $out"
printf '%s' "$out" | grep -q -- '--fix' \
  || fail "cold-cache remedy did not name --fix: $out"
printf '%s' "$out" | grep -qi 'start a session' \
  || fail "cold-cache remedy did not name session start: $out"
printf '%s' "$out" | grep -q 'bin/gc-runtime-rex' \
  || fail "cold-cache remedy did not name the entry point: $out"
printf '%s' "$out" | grep -qi 'add .*to.*PATH\|on PATH so gc' \
  && fail "cold-cache remediation still gives PATH advice: $out"

run_shim >/dev/null 2>&1 || fail "warm start failed"
: > "$work/curl.log"; : > "$work/shim.log"
run_check >/dev/null 2>&1 || fail "warm-cache check did not pass"
[ -s "$work/curl.log" ] && fail "warm-cache check fetched again"
[ -s "$work/shim.log" ] && fail "warm-cache check exec'd the shipped shim instead of the cached binary"

# Unsupported platform: the cache path cannot be computed. The check must report
# that and must not exec the shim (which would reject the platform).
rm -rf "$cache"; : > "$work/curl.log"; : > "$work/shim.log"
if out=$(run_check FAKE_UNAME_S=Linux FAKE_UNAME_M=riscv64 2>&1); then
  fail "unsupported-platform check unexpectedly passed"
fi
[ -s "$work/shim.log" ] && fail "unsupported-platform check exec'd the shipped shim"
[ -s "$work/curl.log" ] && fail "unsupported-platform check performed a download"
printf '%s' "$out" | grep -qi 'cache.*could not be located' \
  || fail "unsupported-platform check did not report the cache lookup failure: $out"
printf '%s' "$out" | grep -q 'riscv64' \
  || fail "unsupported-platform check did not name the rejected machine: $out"

# Missing cache helper: the cache path cannot be computed. The check must report
# that and must not exec the shim.
mv "$shim_pack/bin/_lib.sh" "$shim_pack/bin/_lib.sh.off"
rm -rf "$cache"; : > "$work/shim.log"
if out=$(run_check 2>&1); then fail "missing-helper check unexpectedly passed"; fi
[ -s "$work/shim.log" ] && fail "missing-helper check exec'd the shipped shim"
printf '%s' "$out" | grep -qi 'cache.*could not be located' \
  || fail "missing-helper check did not report the cache lookup failure: $out"
printf '%s' "$out" | grep -q '_lib.sh' \
  || fail "missing-helper check did not name the missing helper: $out"
mv "$shim_pack/bin/_lib.sh.off" "$shim_pack/bin/_lib.sh"

# Non-executable shipped shim: a pack-file problem, not a cache problem. The
# remedy must name the entry point and distinguish pack repair from warming.
chmod 644 "$shim_pack/bin/gc-runtime-rex"
rm -rf "$cache"; : > "$work/shim.log"
if out=$(run_check 2>&1); then fail "non-executable-shim check unexpectedly passed"; fi
[ -s "$work/shim.log" ] && fail "non-executable-shim check exec'd the shim"
printf '%s' "$out" | grep -qi 'not executable' \
  || fail "non-executable-shim check did not say the shim is not executable: $out"
printf '%s' "$out" | grep -q 'bin/gc-runtime-rex' \
  || fail "non-executable-shim remedy did not name the entry point: $out"
printf '%s' "$out" | grep -q 'gc import install' \
  || fail "non-executable-shim remedy did not name pack repair: $out"
printf '%s' "$out" | grep -q -- '--fix' \
  || fail "non-executable-shim remedy did not name --fix: $out"
printf '%s' "$out" | grep -qi 'warm.*cache\|cache.*warm' \
  || fail "non-executable-shim remedy did not distinguish cache warming: $out"
chmod 755 "$shim_pack/bin/gc-runtime-rex"

# A "./bin/..." spelling must resolve to the SAME shipped shim. Before the fix
# the "./" survived, the identity check missed, and the check EXEC'd the shim,
# which downloaded and installed during a read-only cold-cache check.
sed 's|^command = .*|command = "./bin/gc-runtime-rex"|' \
  "$repo/pack/pack.toml" > "$shim_pack/pack.toml"
grep -q '^command = "./bin/gc-runtime-rex"' "$shim_pack/pack.toml" \
  || fail "test harness bug: the ./bin command was not written"
rm -rf "$cache"; : > "$work/curl.log"; : > "$work/shim.log"
if out=$(run_check 2>&1); then fail "dot-slash cold-cache check unexpectedly passed"; fi
[ -s "$work/shim.log" ] && fail "dot-slash cold-cache check exec'd the shipped shim"
[ -s "$work/curl.log" ] && fail "dot-slash cold-cache check performed a download"
[ -e "$cache" ] && fail "dot-slash cold-cache check wrote the cache (not read-only)"
printf '%s' "$out" | grep -qi 'not yet installed' \
  || fail "dot-slash cold-cache message did not say the provider is not yet installed: $out"
cp "$repo/pack/pack.toml" "$shim_pack/pack.toml"

# gc doctor --fix must not hang on a stalled fetch with no sleep on PATH.
# fix.sh refuses BEFORE the fetch; the unbounded installer fallback stays
# reserved for a direct, manual install.
rm -rf "$cache"; rm -f "$work/nosleep-curl.log"
if out=$(env PATH="$nosleep" HOME="$work/home" GC_RUNTIME_CACHE_DIR="$cache" \
    FAKE_RELEASE_DIR="$work/release" GC_RUNTIME_INSTALL_DEADLINE= \
    FAKE_UNAME_S="$(host_uname_s)" FAKE_UNAME_M="$(host_uname_m)" \
    FAKE_CURL_LOG="$work/nosleep-curl.log" \
    sh "$repo/pack/doctor/rex-binary/fix.sh" 2>&1); then
  fail "fix.sh ran with no sleep on PATH (unbounded fetch)"
fi
printf '%s' "$out" | grep -q 'sleep' \
  || fail "no-sleep fix refusal did not name the missing timer: $out"
[ -e "$work/nosleep-curl.log" ] && fail "no-sleep fix attempted a fetch"
[ -e "$cache" ] && fail "no-sleep fix warmed a cache"

echo "check-runtime-shim: ok"   # moved to the end as later tasks append cases
