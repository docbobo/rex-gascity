#!/bin/sh
# Fail unless the release tag names the version pack/pack.toml declares.
#
# The release workflow runs this before it builds anything: a draft whose tag
# disagrees with the pack version would ship a provider that the pack doctor
# check then reports as a version mismatch. Release tags are v<version>, so a
# bare version fails too.
set -eu

tag=${1:-}
if [ -z "$tag" ]; then
  echo "usage: check-release-version.sh <tag>  (for example v0.1.0)" >&2
  exit 2
fi
case "$tag" in
  v*) ;;
  *)
    echo "release tag $tag must start with v (for example v0.1.0)" >&2
    exit 2
    ;;
esac

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
declared=$(
  awk -F'"' '
    /^[[:space:]]*\[pack\][[:space:]]*$/ { in_pack = 1; next }
    /^[[:space:]]*\[/ { in_pack = 0 }
    in_pack && /^[[:space:]]*version[[:space:]]*=/ { print $2; exit }
  ' "$script_dir/pack.toml"
)
if [ -z "$declared" ]; then
  echo "pack/pack.toml has no [pack] version" >&2
  exit 2
fi

if [ "${tag#v}" != "$declared" ]; then
  echo "release tag $tag does not match [pack] version $declared in pack/pack.toml" >&2
  exit 1
fi
echo "release tag $tag matches [pack] version $declared"
