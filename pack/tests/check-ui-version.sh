#!/bin/sh
# Guard for the UI version identity.
#
# The UI ships with the provider: one tag v<version> cuts both. ui/version must
# equal [pack] version, because the installer builds the cache path from it
# (<cache root>/<version>/ui/) and the doctor check compares the loaded copy
# against the pack's version. A mismatch would install a tree the doctor then
# reports as the wrong version, so it fails here instead.
#
# The release gate runs this beside pack/check-release-version.sh, so a tag that
# disagrees with either file never builds.
#
# Usage: sh pack/tests/check-ui-version.sh
# Exits non-zero with a one-line reason on the first failed assertion.
set -eu

repo=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
pack_toml=$repo/pack/pack.toml
ui_version_file=$repo/ui/version

fail() {
  echo "check-ui-version: $*" >&2
  exit 1
}

[ -f "$pack_toml" ] || fail "pack/pack.toml does not exist"
[ -f "$ui_version_file" ] || fail "ui/version does not exist"

declared=$(
  awk -F'"' '
    /^[[:space:]]*\[pack\][[:space:]]*$/ { in_pack = 1; next }
    /^[[:space:]]*\[/ { in_pack = 0 }
    in_pack && /^[[:space:]]*version[[:space:]]*=/ { print $2; exit }
  ' "$pack_toml"
)
[ -n "$declared" ] || fail "pack/pack.toml has no [pack] version"

ui_version=$(tr -d '[:space:]' < "$ui_version_file")
[ -n "$ui_version" ] || fail "ui/version is empty"

[ "$ui_version" = "$declared" ] ||
  fail "ui/version is $ui_version, but [pack] version is $declared in pack/pack.toml"

echo "check-ui-version: ui/version $ui_version matches [pack] version"
