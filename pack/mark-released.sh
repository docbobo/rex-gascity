#!/bin/sh
# Mark a published release in README.md. This is the only thing that writes
# the README release block, and a human runs it after the release is public.
#
#   pack/mark-released.sh v<version> [--dry-run]
#
# Refuses unless `gh release view <tag>` shows a published release: not a
# draft and not a prerelease. It rewrites only the README release-state text:
# the "## Status: released, ..." heading, the "Version ... is the current
# release" paragraph, the install --version pins (bare and caret), and the
# "pack/pack.toml declares version = ..." lines. It drops the release-target
# wording ("not published yet" and "once that tag is published").
#
# --dry-run prints the diff and changes nothing. A real run commits
# "docs: mark <tag> released" and pushes the branch (main). It is idempotent:
# a README that already marks <tag> released is left alone, with no commit.
# It never tags, builds or publishes, and it does not touch pack/pack.toml or
# ui/version.
set -eu

usage() {
  sed -n '2,18p' "$0" | sed 's/^# \{0,1\}//'
  exit "${1:-0}"
}

pack=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
root=$(CDPATH= cd -- "$pack/.." && pwd)
GH=${GH:-gh}
branch=${RELEASE_BRANCH:-main}
readme=$root/README.md

tag=
dry_run=0
while [ $# -gt 0 ]; do
  case $1 in
    --dry-run) dry_run=1 ;;
    -h|--help) usage 0 ;;
    v[0-9]*) [ -z "$tag" ] || usage 2; tag=$1 ;;
    *) echo "mark-released: unexpected argument: $1" >&2; usage 2 ;;
  esac
  shift
done
[ -n "$tag" ] || usage 2
version=${tag#v}

die() {
  echo "mark-released: $*" >&2
  exit 1
}

json_field() {
  printf '%s' "$1" | python3 -c "import json,sys; print(json.load(sys.stdin)['$2'])"
}

# The release must be public: a README that advertises an install a user cannot
# complete is the defect this script exists to prevent.
release_json=$("$GH" release view "$tag" --json isDraft,isPrerelease 2>/dev/null) ||
  die "no release $tag found; mark a release only after it is published"
[ "$(json_field "$release_json" isDraft)" = "False" ] ||
  die "release $tag is a draft; publish it first"
[ "$(json_field "$release_json" isPrerelease)" = "False" ] ||
  die "release $tag is a prerelease; mark only a full release"

[ -f "$readme" ] || die "README.md not found at $readme"

tmp=$(mktemp "${TMPDIR:-/tmp}/mark-released.XXXXXX")
trap 'rm -f "$tmp"' EXIT INT TERM

# One awk pass. Every rule is anchored to a release-state line, so a version
# number in an unrelated sentence (an example, a changelog) is left alone.
awk -v new="$version" -v q="'" '
  skip_next {
    skip_next = 0
    if ($0 ~ /^after `v[0-9.]+` is published\.$/) next
  }
  /^## Status: / {
    print "## Status: released, v" new
    next
  }
  /^Version `[0-9.]+` \(tag `v[0-9.]+`\) is the / {
    sub(/^Version `[0-9.]+` \(tag `v[0-9.]+`\) is the (target version|current release)\.( The tag is not published yet\.)?/,
        "Version `" new "` (tag `v" new "`) is the current release.")
    print
    next
  }
  /^CI runs `gc runtime check`/ {
    sub(/, once that tag is published\./, ".")
    print
    next
  }
  /^matching release binary\. The tag-based commands in this section are usable$/ {
    print "matching release binary."
    skip_next = 1
    next
  }
  /^gc import add .*--version / || /^constraint\. The constraint must allow more than one tag: `--version / {
    sub("--version " q "\\^[0-9.]+" q, "--version " q "^" new q)
    sub(/--version [0-9.]+$/, "--version " new)
    print
    next
  }
  /^`pack\/pack\.toml` declares `version = "[0-9.]+"`/ {
    sub(/declares `version = "[0-9.]+"`/, "declares `version = \"" new "\"`")
    print
    next
  }
  /^`v[0-9.]+`\. `--version` takes the bare version/ {
    sub(/^`v[0-9.]+`/, "`v" new "`")
    print
    next
  }
  { print }
' "$readme" > "$tmp"

# Rewriting the heading must have matched; an empty or unrelated README would
# otherwise "succeed" by copying itself.
grep -q "^## Status: released, v$version\$" "$tmp" ||
  die "README.md has no '## Status:' heading to rewrite"

if cmp -s "$readme" "$tmp"; then
  echo "mark-released: README.md already marks $tag released; nothing to do"
  exit 0
fi

if [ "$dry_run" -eq 1 ]; then
  echo "mark-released: dry run; README.md would change as follows"
  diff -u "$readme" "$tmp" || true
  exit 0
fi

# A real run commits and pushes. Start from a clean tree on the release branch,
# level with origin, so the commit holds the README change and nothing else.
git -C "$root" diff-index --quiet HEAD -- ||
  die "working tree is dirty; commit or stash tracked changes first"
now_branch=$(git -C "$root" rev-parse --abbrev-ref HEAD)
[ "$now_branch" = "$branch" ] ||
  die "on branch $now_branch; run this on $branch"
git -C "$root" fetch -q origin "$branch" ||
  die "cannot fetch origin $branch"
# HEAD must equal origin/$branch. A local commit that is not on the remote
# would otherwise ride along with the README commit on push.
git -C "$root" merge-base --is-ancestor "origin/$branch" HEAD &&
  git -C "$root" merge-base --is-ancestor HEAD "origin/$branch" ||
  die "$branch is not level with origin/$branch; pull or push first"

cat "$tmp" > "$readme"
git -C "$root" add README.md
git -C "$root" commit -q -m "docs: mark $tag released" -- README.md
git -C "$root" push -q origin "$branch" ||
  die "push failed; the commit is local. Fix the push and run: git push origin $branch"
echo "mark-released: README.md marks $tag released; pushed to origin $branch"
