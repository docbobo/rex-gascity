#!/bin/sh
# Self-test for check-doc-links.sh: a `docs/` link inside docs/ must resolve
# relative to the containing document, not the repo root.
set -eu

script_dir=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT INT TERM

git -C "$tmp" init -q
mkdir -p "$tmp/docs/adr"
printf '(foo.md)\n(bar.md)\n' >"$tmp/docs/README.md"
printf '(0001-test.md)\n' >"$tmp/docs/adr/README.md"
: >"$tmp/docs/adr/0001-test.md"
: >"$tmp/docs/foo.md"

# Red case: from docs/bar.md, docs/foo.md resolves to docs/docs/foo.md.
printf '[x](docs/foo.md)\n' >"$tmp/docs/bar.md"
git -C "$tmp" add -A
if sh "$script_dir/check-doc-links.sh" "$tmp" >/dev/null 2>&1; then
  echo 'check-doc-links self-test: accepted docs/foo.md, which does not resolve from docs/bar.md' >&2
  exit 1
fi

# Green case: a bare-filename link resolves from the containing document.
printf '[x](foo.md)\n' >"$tmp/docs/bar.md"
git -C "$tmp" add -A
sh "$script_dir/check-doc-links.sh" "$tmp"
