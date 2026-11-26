#!/bin/sh
# Proves the rex-binary doctor check compares the installed rex CLI's API
# revision against the pack/rex-min-api-revision pin, and treats it as a SOFT
# gate: a CLI below the pin warns (exit 1); it does not fail the check. A CLI at
# or above the pin passes. The API revision comes from `rex version --json`'s
# api_revision field, with the plain `rex version` line `api: revision N
# (supports ...)` as the fallback. The pack doctor script also checks the
# provider binary, so the test warms the version-keyed cache the shim and doctor
# share with a fake gc-runtime-rex that reports the pack's declared version.
# Only the rex CLI API revision varies between cases.
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
min_api=$(tr -dc '0-9' < "$pack_dir/rex-min-api-revision")

if [ -z "$declared" ] || [ -z "$min_api" ]; then
  echo "check-doctor-rex-api-revision: pack version or pack/rex-min-api-revision is missing" >&2
  exit 1
fi

# The below-pin case must come from the pin, not a hardcoded revision.
too_old=$((min_api - 1))

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

cat >"$tmp/gc-runtime-rex" <<EOF
#!/bin/sh
case "\${1:-}" in
  protocol) echo '{"protocol":0,"version":0}' ;;
  version) echo '{"version":"$declared"}' ;;
esac
EOF
# FAKE_REX_API_JSON sets `rex version --json`'s api_revision; FAKE_REX_API_PLAIN
# sets the plain `api: revision N` line. An unset one omits that reading, so the
# test can drive the JSON-only, plain-only and absent cases.
cat >"$tmp/rex" <<'EOF'
#!/bin/sh
case "${1:-}" in
  version)
    case "${2:-}" in
      --json)
        if [ -n "${FAKE_REX_API_JSON:-}" ]; then
          printf '{"rex_version":"%s","api_revision":%s}\n' "${FAKE_REX_VERSION:-0.1.0}" "$FAKE_REX_API_JSON"
        else
          printf '{"rex_version":"%s"}\n' "${FAKE_REX_VERSION:-0.1.0}"
        fi ;;
      *)
        printf 'rex %s\n' "${FAKE_REX_VERSION:-0.1.0}"
        if [ -n "${FAKE_REX_API_PLAIN:-}" ]; then
          printf 'api: revision %s (supports 0 to 999)\n' "$FAKE_REX_API_PLAIN"
        fi ;;
    esac ;;
esac
EOF
chmod +x "$tmp/gc-runtime-rex" "$tmp/rex"

# The doctor probes the CACHED binary (the pack shim's cache), not PATH. Place
# the fake provider at the version-keyed cache path the shim/doctor compute.
plat=$(
  . "$pack_dir/bin/_lib.sh"
  gc_runtime_platform
) || { echo "check-doctor-rex-api-revision: cannot resolve the host platform" >&2; exit 1; }
cache=$tmp/cache
bindir=$cache/$declared/$plat
mkdir -p "$bindir"
cp "$tmp/gc-runtime-rex" "$bindir/gc-runtime-rex"

fail() {
  echo "check-doctor-rex-api-revision: $*" >&2
  exit 1
}

# run_case JSON_API PLAIN_API EXPECTED_EXIT: prints the doctor output.
run_case() {
  out=$(GC_RUNTIME_CACHE_DIR="$cache" GC_REX_BIN="$tmp/rex" \
    FAKE_REX_API_JSON="$1" FAKE_REX_API_PLAIN="$2" \
    sh "$doctor" 2>&1) && status=0 || status=$?
  [ "$status" -eq "$3" ] || fail "api $1/$2: exit $status, want $3 (output: $out)"
  printf '%s' "$out"
}

# Below the pin warns, names the Rex app, and names both revisions.
out=$(run_case "$too_old" "$too_old" 1)
printf '%s\n' "$out" | grep -qi 'Rex app' ||
  fail "below-pin message does not name the Rex app: $out"
printf '%s' "$out" | grep -q "$too_old" ||
  fail "below-pin message does not name the installed revision: $out"
printf '%s' "$out" | grep -q "$min_api" ||
  fail "below-pin message does not name the required revision: $out"

# The pin itself passes, and a newer CLI passes.
run_case "$min_api" "$min_api" 0 >/dev/null
run_case "$((min_api + 10))" "$((min_api + 10))" 0 >/dev/null

# With --json unsupported, the plain `api: revision N` line is the fallback.
run_case "" "$min_api" 0 >/dev/null
run_case "" "$too_old" 1 >/dev/null

# Neither reading present warns instead of erroring.
run_case "" "" 1 >/dev/null

# A missing rex CLI is still an error, not a warning.
out=$(GC_RUNTIME_CACHE_DIR="$cache" GC_REX_BIN="$tmp/does-not-exist" \
  sh "$doctor" 2>&1) && status=0 || status=$?
[ "$status" -eq 2 ] || fail "missing rex CLI: exit $status, want 2 (output: $out)"

# --fix warms the provider cache only. It cannot update the Rex app, so a CLI
# below the pin is named but the gate is soft: the warm still runs. A fake curl
# records that the fetch was reached.
cat >"$tmp/curl" <<EOF
#!/bin/sh
: >"$tmp/curl-called"
exit 6
EOF
chmod +x "$tmp/curl"

rm -f "$tmp/curl-called"
fix_err=$(GC_REX_BIN="$tmp/rex" FAKE_REX_API_JSON="$too_old" FAKE_REX_API_PLAIN="$too_old" \
  GC_RUNTIME_CACHE_DIR="$cache" PATH="$tmp:$PATH" \
  sh "$pack_dir/doctor/rex-binary/fix.sh" 2>&1 >/dev/null) || true
[ -e "$tmp/curl-called" ] ||
  fail "fix.sh did not reach the fetch for a below-pin rex CLI (soft gate should not block)"
printf '%s' "$fix_err" | grep -qi 'Rex app' ||
  fail "fix.sh warning does not name the Rex app: $fix_err"
printf '%s' "$fix_err" | grep -q "$too_old" ||
  fail "fix.sh warning does not name the installed revision: $fix_err"
printf '%s' "$fix_err" | grep -q "$min_api" ||
  fail "fix.sh warning does not name the required revision: $fix_err"

echo "rex doctor API revision check behaves correctly against minApiRevision $min_api"
