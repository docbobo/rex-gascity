#!/bin/sh
# Proves the rex-binary doctor check fails when the installed rex CLI is older
# than the pack/rex-min-version pin, and passes when it is equal or newer. The
# pack doctor script also checks the provider binary, so the test warms the
# version-keyed cache the shim and doctor share with a fake gc-runtime-rex that
# reports the pack's declared version. Only the rex CLI version varies between
# cases.
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
pack_dir=$(CDPATH= cd -- "$script_dir/.." && pwd)
doctor=$pack_dir/doctor/rex-binary/run.sh

declared=$(
  awk -F'"' '
    /^[[:space:]]*\[pack\][[:space:]]*$/ { in_pack = 1; next }
    /^[[:space:]]*\[/ { in_pack = 0 }
    in_pack && /^[[:space:]]*version[[:space:]]*=/ { print $2; exit }
  ' "$pack_dir/pack.toml"
)
min_version=$(
  tr -d '[:space:]' < "$pack_dir/rex-min-version"
)
if [ -z "$declared" ] || [ -z "$min_version" ]; then
  echo "check-doctor-rex-version: pack version or pack/rex-min-version is missing" >&2
  exit 1
fi

# The too-old case must come from the pin, not a hardcoded preview number.
# The pin shape is X.Y.Z-preview.N; anything else fails loudly here.
case $min_version in
  *-preview.*) ;;
  *) echo "check-doctor-rex-version: pack/rex-min-version must be X.Y.Z-preview.N, got $min_version" >&2; exit 1 ;;
esac
preview_n=${min_version##*-preview.}
case $preview_n in
  ''|*[!0-9]*) echo "check-doctor-rex-version: preview number in $min_version is not numeric" >&2; exit 1 ;;
esac
too_old=${min_version%-preview.*}-preview.$(( ${min_version##*-preview.} - 1 ))

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

cat >"$tmp/gc-runtime-rex" <<EOF
#!/bin/sh
case "\${1:-}" in
  protocol) echo '{"protocol":0,"version":0}' ;;
  version) echo '{"version":"$declared"}' ;;
esac
EOF
cat >"$tmp/rex" <<'EOF'
#!/bin/sh
case "${1:-}" in
  version)
    case "${2:-}" in
      --json) printf '{"rex_version":"%s"}\n' "$FAKE_REX_VERSION" ;;
      *) printf 'rex %s\n' "$FAKE_REX_VERSION" ;;
    esac
    ;;
esac
EOF
chmod +x "$tmp/gc-runtime-rex" "$tmp/rex"

# The doctor probes the CACHED binary (the pack shim's cache), not PATH. Place
# the fake provider at the version-keyed cache path the shim/doctor compute.
plat=$(
  . "$pack_dir/bin/_lib.sh"
  gc_runtime_platform
) || { echo "check-doctor-rex-version: cannot resolve the host platform" >&2; exit 1; }
cache=$tmp/cache
bindir=$cache/$declared/$plat
mkdir -p "$bindir"
cp "$tmp/gc-runtime-rex" "$bindir/gc-runtime-rex"

fail() {
  echo "check-doctor-rex-version: $*" >&2
  exit 1
}

# run_case VERSION EXPECTED_EXIT: prints the doctor output.
run_case() {
  out=$(GC_RUNTIME_CACHE_DIR="$cache" GC_REX_BIN="$tmp/rex" FAKE_REX_VERSION="$1" \
    sh "$doctor" 2>&1) && status=0 || status=$?
  [ "$status" -eq "$2" ] || fail "rex $1: exit $status, want $2 (output: $out)"
  printf '%s' "$out"
}

# Older than the minimum is an error that names the Rex app and both versions.
out=$(run_case "$too_old" 2)
printf '%s\n' "$out" | head -n 1 | grep -q 'Rex app' ||
  fail "too-old message does not name the Rex app on its first line: $out"
printf '%s' "$out" | grep -q "$too_old" ||
  fail "too-old message does not name the installed version: $out"
printf '%s' "$out" | grep -q "$min_version" ||
  fail "too-old message does not name the required version: $out"

# Equal to the minimum passes.
run_case "$min_version" 0 >/dev/null

# Newer than the minimum passes.
run_case 0.1.0-preview.999 0 >/dev/null

# A release ranks above any preview of the same MAJOR.MINOR.PATCH.
run_case "${min_version%%-*}" 0 >/dev/null

# Version fields compare one at a time: a large patch never outranks a newer
# minor component, which fixed multipliers used to get wrong.
run_case 0.0.1000 2 >/dev/null
run_case 0.1.0-preview.1000 0 >/dev/null

# With --json unsupported, the `rex version` first line is the fallback.
cat >"$tmp/rex" <<'EOF'
#!/bin/sh
case "${1:-}" in
  version)
    case "${2:-}" in
      --json) exit 1 ;;
      *) printf 'rex %s\ncommit: deadbeef\n' "$FAKE_REX_VERSION" ;;
    esac
    ;;
esac
EOF
chmod +x "$tmp/rex"
run_case "$too_old" 2 >/dev/null

# An undeterminable version warns instead of erroring.
cat >"$tmp/rex" <<'EOF'
#!/bin/sh
exit 1
EOF
chmod +x "$tmp/rex"
run_case "" 1 >/dev/null

# --fix warms the provider cache only. It cannot update the Rex app, so a CLI
# below the minimum must stop it before any fetch.
cat >"$tmp/rex" <<'EOF'
#!/bin/sh
case "${1:-}" in
  version)
    case "${2:-}" in
      --json) printf '{"rex_version":"%s"}\n' "$FAKE_REX_VERSION" ;;
      *) printf 'rex %s\n' "$FAKE_REX_VERSION" ;;
    esac
    ;;
esac
EOF
chmod +x "$tmp/rex"
cat >"$tmp/curl" <<EOF
#!/bin/sh
: >"$tmp/curl-called"
exit 6
EOF
chmod +x "$tmp/curl"

rm -f "$tmp/curl-called"
fix_err=$(GC_REX_BIN="$tmp/rex" FAKE_REX_VERSION="$too_old" GC_RUNTIME_CACHE_DIR="$cache" \
  PATH="$tmp:$PATH" sh "$pack_dir/doctor/rex-binary/fix.sh" 2>&1 >/dev/null) && fix_status=0 || fix_status=$?
[ "$fix_status" -ne 0 ] || fail "fix.sh exits 0 for a too-old rex CLI"
[ ! -e "$tmp/curl-called" ] || fail "fix.sh fetched before it refused a too-old rex CLI"
printf '%s' "$fix_err" | grep -q 'Rex app' ||
  fail "fix.sh refusal does not name the Rex app: $fix_err"
printf '%s' "$fix_err" | grep -q "$min_version" ||
  fail "fix.sh refusal does not name the minimum version: $fix_err"

# Control: a rex CLI at the minimum must not block a cold-cache warm. The fix
# reaches the fetch, so the fake curl runs and leaves its marker.
rm -f "$tmp/curl-called"
GC_REX_BIN="$tmp/rex" FAKE_REX_VERSION="$min_version" GC_RUNTIME_CACHE_DIR="$tmp/cold-cache" \
  PATH="$tmp:$PATH" sh "$pack_dir/doctor/rex-binary/fix.sh" >/dev/null 2>&1 || true
[ -e "$tmp/curl-called" ] || fail "fix.sh did not reach the fetch for a rex CLI at the minimum"

echo "rex doctor version check behaves correctly against minVersion $min_version"
