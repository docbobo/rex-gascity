#!/bin/sh
# gc <binding> dashboard: show the workstreams a rig is actively working on.
#
# The visualizer lives beside this script as dashboard.py. gc passes stdin,
# stdout, and stderr through, so --watch gets a real TTY and its q/r keys work.
# Without a --rig argument dashboard.py asks `gc rig list` for the rig that
# contains the current directory. See help.md and dashboard.py's docstring.
set -eu

here=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
dir=$here
if [ -n "${GC_PACK_DIR:-}" ] && [ -f "$GC_PACK_DIR/commands/dashboard/dashboard.py" ]; then
	dir=$GC_PACK_DIR/commands/dashboard
fi

if ! command -v python3 >/dev/null 2>&1; then
	printf 'dashboard: python3 not found on PATH\n' >&2
	exit 127
fi

exec python3 "$dir/dashboard.py" "$@"
