#!/bin/sh
# Decide whether a pull request touches documentation only.
#
#   gh api --paginate repos/OWNER/REPO/pulls/N/files \
#     --jq '.[] | .filename, (.previous_filename // empty)' | scripts/docs-only.sh
#
# Exit 0 when every path is under docs/ or plans/, or is a top-level *.md file.
# Exit 1 otherwise, and on empty input: a missing file list must take the full
# CI path, never the fast one. The caller passes previous names too, so a file
# renamed out of the code counts as a code change.
set -eu

files=$(cat)
[ -n "$files" ] || exit 1
if printf '%s\n' "$files" | grep -Evq '^(docs/|plans/|[^/]+\.md$)'; then
  exit 1   # at least one path is not documentation
fi
exit 0
