#!/bin/sh
# Refuse a release whose notes are not ready to publish.
#
#   pack/check-release-notes.sh <tag> [body-file]
#
# With no body-file, the body comes from the draft release named by <tag>; the
# repo is pinned from the origin remote, so a fork's gh default cannot answer.
# The body must name the target version ("pack v<version>"), carry at least one
# "- #N — " bullet, name no version other than the target or a published
# release, and not still be the placeholder that pack/release.sh creates.
set -eu

usage() {
  sed -n '2,9p' "$0" | sed 's/^# \{0,1\}//'
  exit "${1:-0}"
}

pack=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
root=$(CDPATH= cd -- "$pack/.." && pwd)
GH=${GH:-gh}

tag=
body_file=
while [ $# -gt 0 ]; do
  case $1 in
    -h|--help) usage 0 ;;
    v[0-9]*) [ -z "$tag" ] || usage 2; tag=$1 ;;
    *) [ -z "$body_file" ] || usage 2; body_file=$1 ;;
  esac
  shift
done
[ -n "$tag" ] || usage 2
version=${tag#v}

die() {
  echo "check-release-notes: $*" >&2
  exit 1
}

origin=$(git -C "$root" remote get-url origin 2>/dev/null) || origin=
case $origin in
  git@github.com:*) slug=${origin#git@github.com:} ;;
  https://github.com/*) slug=${origin#https://github.com/} ;;
  ssh://git@github.com/*) slug=${origin#ssh://git@github.com/} ;;
  *)
    [ -n "${GH_REPO:-}" ] ||
      die "origin is not a GitHub remote: ${origin:-missing} (set GH_REPO to pin the repo)"
    slug=$GH_REPO
    ;;
esac
slug=${slug%.git}

if [ -n "$body_file" ]; then
  [ -f "$body_file" ] || die "body file $body_file does not exist"
  body=$(cat "$body_file")
else
  json=$("$GH" release view "$tag" --repo "$slug" --json body,isDraft 2>/dev/null) ||
    die "cannot read release $tag from $slug with gh"
  body=$(printf '%s' "$json" |
    python3 -c 'import json,sys; print(json.load(sys.stdin)["body"], end="")') ||
    die "cannot read the body of release $tag"
fi

first=$(printf '%s\n' "$body" | sed -n '1p' | sed 's/^[[:space:]]*//')
case $first in
  "Rex runtime pack $tag. See docs/"*)
    die "body of $tag is still the pack/release.sh placeholder; write the release notes"
    ;;
esac

# The target token must not run into another digit: "pack v2026.11.060" names
# a different version than "pack v2026.11.06". A period after the target (a
# sentence continues) still matches.
printf '%s\n' "$body" |
  grep -Eq "pack $(printf '%s' "$tag" | sed 's/\./\\./g')([^0-9]|$)" ||
  die "body of $tag does not name the target version as \"pack $tag\""

bullets=$(printf '%s\n' "$body" | grep -cE '^- #[0-9]+ — ' || true)
[ "$bullets" -ge 1 ] ||
  die "body of $tag has no per-PR bullet of the form \"- #N — \"; found 0"
echo "release notes: $bullets per-PR bullet(s)"

# A published tag may be named (for example "the previous release was
# v2026.11.05"). A draft tag is not published, so it may not.
published=$(
  "$GH" release list --repo "$slug" --limit 1000 --json tagName,isDraft 2>/dev/null |
    python3 -c 'import json,sys
for r in json.load(sys.stdin):
    if not r["isDraft"]:
        t = r["tagName"]
        print(t[1:] if t.startswith("v") else t)'
) || die "cannot list releases from $slug with gh"

bad=
for token in $(printf '%s\n' "$body" |
    grep -oE '[0-9]{4}\.[0-9]{1,2}\.[0-9]+' | sort -u); do
  [ "$token" = "$version" ] && continue
  if printf '%s\n' "$published" | grep -Fxq "$token"; then
    continue
  fi
  bad="$bad $token"
done
[ -z "$bad" ] ||
  die "body of $tag names version(s) that are neither $version nor a published release:$bad"

echo "release notes for $tag pass the gate"
