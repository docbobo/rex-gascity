#!/bin/sh
# Run every pack check script (pack/tests/check-*.sh) in sorted order and stop
# at the first non-zero exit. A missing check file is not an error: checks
# arrive with later beads.
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)

for check in "$script_dir"/check-*.sh; do
  [ -f "$check" ] || continue
  echo "pack check: ${check##*/}"
  if ! sh "$check"; then
    echo "pack check failed: ${check##*/}" >&2
    exit 1
  fi
done
