#!/bin/sh
# gc doctor --fix remediation for the rex:rex-binary check: install the release
# asset that the pack version names. gc runs a fix with cwd at the pack
# directory and GC_PACK_DIR/GC_CITY_PATH/GC_PACK_STATE_DIR in the environment
# (internal/doctor/pack_checks.go, PackScriptCheck.Fix).
#
# The work is done by <pack>/install.sh: it downloads the prebuilt binary and
# verifies its checksum, so no Go toolchain and no repository checkout are
# needed. A source checkout can opt in with install.sh --from-source.
#
# Exit 0 when installed; non-zero with the reason on stderr, which gc doctor
# prints on a failed fix.
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
pack_dir=$(CDPATH= cd -- "$script_dir/../.." && pwd)

exec sh "$pack_dir/install.sh" "$@"
