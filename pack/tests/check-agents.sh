#!/bin/sh
# Guard for the per-subsystem agent notes.
#
# The root AGENTS.md carries the repo-wide rules. Each subsystem keeps its local
# invariants in a nested AGENTS.md. Pi loads a nested context file only when the
# working directory is inside that subtree, so a nested file the root does not
# point at is a file no agent ever reads. This check keeps the two in step:
# every tracked nested AGENTS.md is named in the root, and every nested path the
# root names exists and is tracked.
#
# Usage: sh pack/tests/check-agents.sh
# Exits non-zero with a one-line reason on the first failed assertion.
set -eu

repo=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
root=$repo/AGENTS.md

fail() {
  echo "check-agents: $*" >&2
  exit 1
}

command -v git >/dev/null 2>&1 || fail "git is not on PATH"
[ -f "$root" ] || fail "AGENTS.md does not exist"
grep -q '^## Subsystem notes$' "$root" ||
  fail "AGENTS.md has no '## Subsystem notes' section"

# Nested files: every tracked AGENTS.md below the repo root.
nested=$(git -C "$repo" ls-files | grep '/AGENTS\.md$' || true)
[ -n "$nested" ] || fail "no nested AGENTS.md files are tracked"

# Every nested file must be named in the root, or nothing will load it.
for file in $nested; do
  grep -qF "\`$file\`" "$root" ||
    fail "AGENTS.md does not name the nested file $file"
done

# Every nested path the root names must exist and be tracked. This catches a
# typo and a reference to a file that was moved or deleted.
refs=$(grep -oE '`[A-Za-z0-9._/-]+/AGENTS\.md`' "$root" | tr -d '`' | sort -u || true)
for ref in $refs; do
  [ -f "$repo/$ref" ] || fail "AGENTS.md names a missing file: $ref"
  printf '%s\n' "$nested" | grep -qxF "$ref" ||
    fail "AGENTS.md names an untracked file: $ref"
done

echo "check-agents: ok"
