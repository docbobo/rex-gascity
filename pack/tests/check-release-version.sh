#!/bin/sh
# Proves the release gate rejects a tag that does not name the version in
# pack/pack.toml, and accepts the matching `v<version>` tag. The workflow's
# first job runs pack/check-release-version.sh; without that script this check
# fails, so the gate cannot be dropped silently.
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
pack_dir=$(CDPATH= cd -- "$script_dir/.." && pwd)
gate=$pack_dir/check-release-version.sh

declared=$(
  awk -F'"' '
    /^[[:space:]]*\[pack\][[:space:]]*$/ { in_pack = 1; next }
    /^[[:space:]]*\[/ { in_pack = 0 }
    in_pack && /^[[:space:]]*version[[:space:]]*=/ { print $2; exit }
  ' "$pack_dir/pack.toml"
)
if [ -z "$declared" ]; then
  echo "check-release-version: [pack] version is missing from pack.toml" >&2
  exit 1
fi

fail() {
  echo "check-release-version: $*" >&2
  exit 1
}

# A tag that names the declared version passes.
if ! "$gate" "v$declared" >/dev/null; then
  fail "the gate rejected the matching tag v$declared"
fi

# A tag that does not name the declared version fails.
if "$gate" "v$declared.1" >/dev/null 2>&1; then
  fail "the gate accepted the mismatched tag v$declared.1"
fi

# A bare version without the leading v also fails: pack/release.sh and the
# install path use v<version> tags.
if "$gate" "$declared" >/dev/null 2>&1; then
  fail "the gate accepted the untagged version $declared"
fi

# A missing tag is a usage error, not a pass.
if "$gate" >/dev/null 2>&1; then
  fail "the gate passed with no tag"
fi

echo "release gate behaves correctly for declared version $declared"
