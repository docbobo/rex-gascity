#!/bin/sh
# Self-check for pack/tests/conformance.sh, run against a real provider.
#
#   pack/tests/check-conformance.sh [--binary <path>]
#
# Asserts the unmodified tree exits 0 with failed: 0, that the summary records
# v2_ready: true and no failed v2 requirement, and that a provider which fails
# a requirement makes conformance.sh exit non-zero.
set -eu

here=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
binary=
while [ $# -gt 0 ]; do
  case $1 in
    --binary)
      [ $# -ge 2 ] || { echo "check-conformance: --binary needs a path" >&2; exit 2; }
      binary=$2
      shift 2
      ;;
    *)
      echo "usage: $0 [--binary <path>]" >&2
      exit 2
      ;;
  esac
done
if [ -z "$binary" ]; then
  binary=$(command -v gc-runtime-rex || true)
fi
if [ -z "$binary" ]; then
  # pack/tests/verify.sh runs in a bare checkout too. This guard needs a
  # provider; the CI conformance job builds one and passes --binary.
  echo "check-conformance: skip: gc-runtime-rex is not on PATH; pass --binary"
  exit 0
fi
# An absolute path keeps working after the subshell below changes into $tmp.
binary=$(CDPATH= cd -- "$(dirname -- "$binary")" && pwd)/$(basename -- "$binary")

tmp=$(mktemp -d "${TMPDIR:-/tmp}/rexcheck.XXXXXX")
# The relative-TMPDIR case below runs a subshell from $tmp, so the path the
# trap and the redirects use must not stay relative.
tmp=$(CDPATH= cd -- "$tmp" && pwd)
trap 'rm -rf "$tmp"' EXIT INT TERM

fail() {
  echo "check-conformance: $*" >&2
  exit 1
}

summary=$tmp/summary.md
out=$tmp/out
if ! GITHUB_STEP_SUMMARY=$summary "$here/conformance.sh" --binary "$binary" > "$out" 2>&1; then
  cat "$out" >&2
  fail "conformance.sh failed on an unmodified tree"
fi

for want in 'ok: true' 'failed: 0' 'v2_ready: true' 'v2_failed: []'; do
  grep -qxF "$want" "$summary" || {
    cat "$summary" >&2
    fail "summary is missing '$want'"
  }
done

# A provider that answers nothing must be reported as a failure, never pass.
bad=$tmp/bad-provider
printf '#!/bin/sh\nexit 2\n' > "$bad"
chmod +x "$bad"
bad_summary=$tmp/bad-summary.md
if GITHUB_STEP_SUMMARY=$bad_summary "$here/conformance.sh" --binary "$bad" > "$tmp/bad-out" 2>&1; then
  fail "a failing provider exited 0"
fi
if grep -qx 'failed: 0' "$bad_summary"; then
  fail "a failing provider was reported with failed: 0"
fi
grep -q 'failed: RPP-' "$tmp/bad-out" ||
  fail "a failing provider's failed requirement was not reported"

# One transient provider failure must not fail the gate: the run is retried
# once. A broken provider still fails both attempts, so the check above holds.
flake=$tmp/flaky-provider
marker=$tmp/flake-fired
states=$tmp/flake-states
cat > "$flake" <<EOF
#!/bin/sh
case \${1:-} in
  send-keys|clear-scrollback|interrupt)
    printf '%s\n' "\${GC_EXEC_STATE_DIR:-}" >> "$states"
    if [ ! -f "$marker" ]; then
      : > "$marker"
      echo 'flaky provider: transient failure' >&2
      exit 1
    fi
    ;;
esac
exec "$binary" "\$@"
EOF
chmod +x "$flake"
GITHUB_STEP_SUMMARY=$tmp/flake-summary.md "$here/conformance.sh" --binary "$flake" \
  > "$tmp/flake-out" 2>&1 || {
  cat "$tmp/flake-out" >&2
  fail "a provider with one transient failure failed the gate"
}
grep -qx 'ok: true' "$tmp/flake-summary.md" ||
  fail "a retried run did not report ok: true"
grep -q 'retrying once' "$tmp/flake-out" ||
  fail "a transient failure was not reported as retried"
# The retry must run in its own state root, not the failed attempt's.
[ "$(grep -v '^$' "$states" | sort -u | wc -l | tr -d ' ')" -ge 2 ] ||
  fail "the retry reused the first attempt's state root"

# A relative TMPDIR must not leak into GC_EXEC_STATE_DIR.
( cd "$tmp" && TMPDIR=. GITHUB_STEP_SUMMARY=$tmp/rel-summary.md \
    "$here/conformance.sh" --binary "$binary" > "$tmp/rel-out" 2>&1 ) || {
  cat "$tmp/rel-out" >&2
  fail "conformance.sh failed with TMPDIR=."
}
grep -qx 'ok: true' "$tmp/rel-summary.md" ||
  fail "conformance.sh did not pass with TMPDIR=."

echo "check-conformance: OK"
