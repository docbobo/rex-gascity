#!/bin/sh
# gc doctor --fix for rex-gascity-runtime:rex-binary. Warm the version-keyed
# cache the shim (pack/bin/gc-runtime-rex) execs, by installing into the same
# GC_RUNTIME_INSTALL_DIR the shim computes — not onto PATH. The cache path comes
# from the shared pack/bin/_lib.sh so it matches the shim byte-for-byte.
#
# gc runs a fix with cwd at the pack dir and GC_PACK_DIR in the environment.
# Exit 0 when warmed; non-zero with the reason on stderr, which gc doctor prints
# on a failed fix.
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
pack_dir=$(CDPATH= cd -- "$script_dir/../.." && pwd)
pack_toml=$pack_dir/pack.toml
. "$pack_dir/bin/_lib.sh"

version=$(
  awk -F'"' '
    /^[[:space:]]*\[pack\][[:space:]]*$/ { in_pack = 1; next }
    /^[[:space:]]*\[/ { in_pack = 0 }
    in_pack && /^[[:space:]]*version[[:space:]]*=/ { print $2; exit }
  ' "$pack_toml" 2>/dev/null || true
)
[ -n "$version" ] || { echo "rex-binary fix: $pack_toml declares no [pack] version" >&2; exit 1; }

if ! bin_dir=$(gc_runtime_bin_dir "$version"); then
  echo "rex-binary fix: no release asset resolves for this platform (pack version $version)" >&2
  exit 1
fi

# --fix runs OUT OF BAND — not on gc's 30 s protocol / 120 s start deadline — so
# it must NOT inherit the shim's tight [1,25] s fetch budget. install.sh is
# unbounded when GC_RUNTIME_INSTALL_DEADLINE is unset, so set an explicit
# GENEROUS budget: a slow release the shim timed out on can finish here, and the
# fetch stays bounded rather than hanging forever. Honor an operator override so
# an even slower link can be given more.
export GC_RUNTIME_INSTALL_DIR="$bin_dir"
export GC_RUNTIME_INSTALL_DEADLINE="${GC_RUNTIME_INSTALL_DEADLINE:-300}"
exec sh "$pack_dir/install.sh" "$@"
