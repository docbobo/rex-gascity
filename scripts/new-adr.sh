#!/bin/sh
# Create the next docs/adr/NNNN-<slug>.md from docs/adr/template.md.
#
#   make adr TITLE="the pane lock is per session"
#
# The number is one past the highest existing NNNN-, so the file name and the
# index row agree.
set -eu

title=${1:-}
if [ -z "$title" ]; then
  echo 'usage: make adr TITLE="the decision in a few words"' >&2
  exit 2
fi

dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)/docs/adr
template=$dir/template.md
if [ ! -f "$template" ]; then
  echo "new-adr: no template at $template" >&2
  exit 1
fi

# A file name is letters, digits and single hyphens.
slug=$(printf '%s' "$title" \
  | tr '[:upper:]' '[:lower:]' \
  | sed -e 's/[^a-z0-9][^a-z0-9]*/-/g' -e 's/^-//' -e 's/-$//')
if [ -z "$slug" ]; then
  echo 'new-adr: the title has no letters or digits' >&2
  exit 2
fi

# The highest number in use, with its leading zeros dropped so the arithmetic
# below is decimal, not octal.
last=0
for f in "$dir"/[0-9][0-9][0-9][0-9]-*.md; do
  [ -f "$f" ] || continue
  n=$(basename "$f" | sed -e 's/-.*//' -e 's/^0*//')
  [ -n "$n" ] || n=0
  if [ "$n" -gt "$last" ]; then
    last=$n
  fi
done

number=$(printf '%04d' "$((last + 1))")
file=$dir/$number-$slug.md
if [ -e "$file" ]; then
  echo "new-adr: $file exists" >&2
  exit 1
fi

sed -e "s/{{NUMBER}}/$number/" -e "s/{{TITLE}}/$title/" -e "s/{{DATE}}/$(date -u +%Y-%m-%d)/" \
  "$template" >"$file"
echo "$file"
