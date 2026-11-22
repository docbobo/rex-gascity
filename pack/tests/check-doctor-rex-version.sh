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

# Older than the minimum is an error that names both versions.
out=$(run_case 0.1.0-preview.21 2)
printf '%s' "$out" | grep -q '0.1.0-preview.21' ||
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
run_case 0.1.0-preview.21 2 >/dev/null

# An undeterminable version warns instead of erroring.
cat >"$tmp/rex" <<'EOF'
#!/bin/sh
exit 1
EOF
chmod +x "$tmp/rex"
run_case "" 1 >/dev/null

echo "rex doctor version check behaves correctly against minVersion $min_version"
