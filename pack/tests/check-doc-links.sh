#!/bin/sh
# Guard for docs/: every docs path a tracked file names must exist, and every
# doc must be indexed.
#
# 1. A `docs/<path>.md` reference in any tracked file, or a relative `.md`
#    link inside docs/, must name a file that exists. URLs are skipped. So are
#    the ADRs (they are records and may name a deleted doc), `docs/reference/`
#    (that path is gc's own tree, cited from the pinned gc checkout) and the
#    self-test, which writes deliberate fixture paths.
# 2. Each docs/*.md must be listed in docs/README.md, and each docs/adr/NNNN-*.md
#    in docs/adr/README.md.
# 3. docs/ has no subdirectory but adr/. Plans, specs and research notes do
#    not live in docs/ (root AGENTS.md, "Recording decisions").
#
# Usage: sh pack/tests/check-doc-links.sh [repo-root]
set -eu

repo=${1:-$(CDPATH='' cd -- "$(dirname -- "$0")/../.." && pwd)}
cd "$repo"

errors=0
fail() {
  echo "check-doc-links: $*" >&2
  errors=$((errors + 1))
}

# 1a. docs/<path>.md references anywhere, minus URLs, ADRs and gc's tree.
refs=$(git ls-files -z -- . ':!docs/adr/' ':!pack/tests/check-doc-links-selftest.sh' |
  xargs -0 grep -I -o -E '(https?://[^ )>"]*)?docs/[A-Za-z0-9_./-]+\.md' 2>/dev/null |
  grep -v -E ':https?://' | grep -v ':docs/reference/' | sort -u) || true
for line in $refs; do
  path=${line#*:}
  [ -e "$path" ] || fail "${line%%:*}: names $path, which does not exist"
done

# 1b. Relative .md links inside docs/ (outside adr/).
for doc in $(git ls-files -- 'docs/*.md' ':!docs/adr/'); do
  dir=$(dirname -- "$doc")
  links=$(grep -o -E '\]\([^)#: ]+\.md' "$doc" | sed 's/^](//') || true
  for link in $links; do
    [ -e "$dir/$link" ] || fail "$doc: links $link, which does not exist"
  done
done

# 2. Indexes.
if [ -f docs/README.md ]; then
  for doc in docs/*.md; do
    name=$(basename -- "$doc")
    [ "$name" = README.md ] && continue
    grep -q -F "($name)" docs/README.md || fail "docs/README.md does not list $name"
  done
else
  fail "docs/README.md does not exist"
fi
for adr in docs/adr/[0-9][0-9][0-9][0-9]-*.md; do
  name=$(basename -- "$adr")
  grep -q -F "($name)" docs/adr/README.md || fail "docs/adr/README.md does not list $name"
done

# 3. Only adr/ below docs/.
for d in docs/*/; do
  [ "$d" = docs/adr/ ] || fail "${d%/} is not allowed: docs/ holds reference docs and adr/ only"
done

[ "$errors" -eq 0 ] || exit 1
echo "check-doc-links: ok"
