#!/bin/sh
# Cut a release in the safe order: draft, CI build, verify the downloaded
# asset, attach conformance.txt, publish.
#
#   pack/release.sh v<version> [--dry-run] [--resume] [--yes]
#
# Preconditions: clean tree (tracked files), branch main, [pack].version equal
# to <version>, and no tag at another commit. A leftover draft needs --resume;
# a published release is never touched. --dry-run prints the commit and asset
# names and changes no remote state. --yes skips the interactive publish
# prompt; use it only after a human approved the release.
#
# The build workflow is dispatched with the tag. This script is the only
# publisher: CI never publishes.
set -eu

usage() {
  sed -n '2,18p' "$0" | sed 's/^# \{0,1\}//'
  exit "${1:-0}"
}

pack=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
root=$(CDPATH= cd -- "$pack/.." && pwd)
GH=${GH:-gh}
workflow=${RELEASE_WORKFLOW:-release.yml}
branch=${RELEASE_BRANCH:-main}
wait_seconds=${RELEASE_WAIT_SECONDS:-3600}
poll_seconds=${RELEASE_POLL_SECONDS:-15}

tag=
dry_run=0
resume=0
assume_yes=0
while [ $# -gt 0 ]; do
  case $1 in
    --dry-run) dry_run=1 ;;
    --resume) resume=1 ;;
    --yes) assume_yes=1 ;;
    -h|--help) usage 0 ;;
    v[0-9]*) [ -z "$tag" ] || usage 2; tag=$1 ;;
    *) echo "release: unexpected argument: $1" >&2; usage 2 ;;
  esac
  shift
done
[ -n "$tag" ] || usage 2
version=${tag#v}

die() {
  echo "release: $*" >&2
  exit 1
}

# Only tracked changes count. A stash of scratch files in the worktree is not
# a reason to stop a release.
git -C "$root" diff-index --quiet HEAD -- ||
  die "working tree is dirty; commit or stash tracked changes first"

now_branch=$(git -C "$root" rev-parse --abbrev-ref HEAD)
[ "$now_branch" = "$branch" ] ||
  die "on branch $now_branch; releases are cut from $branch"
git -C "$root" remote get-url origin >/dev/null 2>&1 ||
  die "no origin remote"
commit=$(git -C "$root" rev-parse HEAD)

declared=$(
  awk -F'"' '
    /^[[:space:]]*\[pack\][[:space:]]*$/ { in_pack = 1; next }
    /^[[:space:]]*\[/ { in_pack = 0 }
    in_pack && /^[[:space:]]*version[[:space:]]*=/ { print $2; exit }
  ' "$pack/pack.toml"
)
[ "$declared" = "$version" ] ||
  die "pack/pack.toml declares version $declared, not $version"

case $(uname -s) in
  Linux) os=linux ;;
  Darwin) os=darwin ;;
  *) die "unsupported operating system: $(uname -s)" ;;
esac
case $(uname -m) in
  x86_64|amd64) arch=amd64 ;;
  arm64|aarch64) arch=arm64 ;;
  *) die "unsupported machine: $(uname -m)" ;;
esac
asset=gc-runtime-rex_${os}_${arch}
# One release carries the provider binaries, the extension tree and one
# checksums.txt over all of them, so a wait or a verification that skipped
# gascity-ui.tar.gz would publish a tag whose UI asset is a different build.
ui_asset=gascity-ui.tar.gz
assets="gc-runtime-rex_linux_amd64 gc-runtime-rex_linux_arm64 gc-runtime-rex_darwin_amd64 gc-runtime-rex_darwin_arm64 $ui_asset checksums.txt"

json_field() {
  printf '%s' "$1" | python3 -c "import json,sys; print(json.load(sys.stdin)['$2'])"
}

# remote_tag_commit prints the commit origin's tag points at. It asks for the
# peeled ref so an annotated tag reports its commit, and touches no local ref.
# No local variables here: a sh function would clobber the caller's names.
remote_tag_commit() {
  ref_lines=$(git -C "$root" ls-remote --tags origin "refs/tags/$tag" "refs/tags/$tag^{}" 2>/dev/null) || return 0
  peeled=$(printf '%s\n' "$ref_lines" |
    awk -v t="refs/tags/$tag^{}" '$2 == t { print $1; exit }')
  [ -n "$peeled" ] || peeled=$(printf '%s\n' "$ref_lines" |
    awk -v t="refs/tags/$tag" '$2 == t { print $1; exit }')
  [ -n "$peeled" ] || return 0
  printf '%s\n' "$peeled"
}

release_json=$("$GH" release view "$tag" --json isDraft,url,assets 2>/dev/null || true)
release_exists=0
[ -n "$release_json" ] && release_exists=1

remote_tag=0
[ -n "$(git -C "$root" ls-remote --tags origin "refs/tags/$tag" 2>/dev/null)" ] && remote_tag=1

# The tag names the commit the assets were built from, so a tag anywhere else
# means the draft and this checkout disagree. Compare before downloading.
tag_commit=$commit
if [ "$remote_tag" -eq 1 ]; then
  tag_commit=$(remote_tag_commit)
  [ -n "$tag_commit" ] ||
    die "cannot read the commit of origin tag $tag"
  [ "$tag_commit" = "$commit" ] ||
    die "origin tag $tag points at $tag_commit, not at HEAD ($commit); check out that commit and retry"
fi

if [ "$release_exists" -eq 1 ]; then
  is_draft=$(json_field "$release_json" isDraft)
  release_url=$(json_field "$release_json" url)
  [ "$resume" -eq 0 ] &&
    die "release $tag already exists; pass --resume to continue an existing draft"
  [ "$is_draft" = "True" ] || die "release $tag is already published; refusing to touch it"
else
  [ "$resume" -eq 0 ] || die "no draft release $tag to resume"
fi

# A resumed draft must be tied to the tag it claims. Without an origin tag
# nothing shows the assets were built from this commit, and the report would
# name a tag_commit the assets cannot prove. Only a new release pushes the tag
# itself, so a missing tag is fatal only when a draft already exists.
[ "$release_exists" -eq 0 ] || [ "$remote_tag" -eq 1 ] ||
  die "draft release $tag exists but origin tag $tag is missing; its assets cannot be tied to $commit. Delete the draft with: $GH release delete $tag, then start a new release"

if [ "$dry_run" -eq 1 ]; then
  echo "release:    $tag"
  echo "version:    $declared (pack/pack.toml)"
  echo "commit:     $commit"
  echo "branch:     $branch"
  echo "origin:     $(git -C "$root" remote get-url origin)"
  echo "host asset: $asset"
  echo "assets:     $assets"
  echo "workflow:   $workflow"
  if [ "$release_exists" -eq 1 ]; then
    echo "state:      resuming existing draft $release_url"
  else
    echo "state:      new draft"
  fi
  echo "dry run: no remote state changed"
  exit 0
fi

if [ "$release_exists" -eq 0 ]; then
  # The workflow gate resolves refs/tags/$tag and a draft release creates no
  # tag, so the tag must exist before the dispatch. Push it at this commit.
  if [ "$remote_tag" -eq 0 ]; then
    git -C "$root" push --quiet origin "$commit:refs/tags/$tag" ||
      die "cannot push tag $tag at $commit"
  fi
  "$GH" release create "$tag" --draft --title "rex runtime pack $tag"     --notes "Rex runtime pack $tag. See docs/rex-runtime-pack.md to import and install."
  "$GH" workflow run "$workflow" -f "tag=$tag"
fi

tmp=$(mktemp -d "${TMPDIR:-/tmp}/rexrelease.XXXXXX")
trap 'rm -rf "$tmp"' EXIT INT TERM

# Bounded wait for the four binaries, the extension tree and checksums.txt. A
# arrives leaves the draft for a later --resume.
deadline=$(( $(date +%s) + wait_seconds ))
while :; do
  json=$("$GH" release view "$tag" --json isDraft,url,assets 2>/dev/null || true)
  [ -n "$json" ] || die "release $tag disappeared while waiting for the build"
  is_draft=$(json_field "$json" isDraft)
  release_url=$(json_field "$json" url)
  [ "$is_draft" = "True" ] || die "release $tag was published during the build wait"
  missing=$(printf '%s' "$json" | python3 -c '
import json, sys
have = {a["name"] for a in json.load(sys.stdin)["assets"]}
print(" ".join(n for n in sys.argv[1].split() if n not in have))
' "$assets")
  [ -z "$missing" ] && break
  [ "$(date +%s)" -lt "$deadline" ] || {
    echo "release: timed out waiting for: $missing" >&2
    echo "release: draft: $release_url" >&2
    exit 1
  }
  sleep "$poll_seconds"
done

dl=$tmp/download
"$GH" release download "$tag" --pattern "$asset" --pattern "$ui_asset" --pattern checksums.txt --dir "$dl"
[ -f "$dl/$asset" ] || die "downloaded asset $asset is missing"
[ -f "$dl/$ui_asset" ] || die "downloaded asset $ui_asset is missing"
[ -f "$dl/checksums.txt" ] || die "downloaded checksums.txt is missing"

sha256_of() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}

# Both published assets are verified against the one checksums.txt: a UI
# tarball built from another commit is refused rather than published. No `local`
# here, so the names are prefixed to leave the caller's alone.
verify_asset() { # verify_asset <name> <sha256>
  _expected=$(awk -v a="$1" '$2 == a || $2 == "*" a {print $1; exit}' "$dl/checksums.txt")
  [ -n "$_expected" ] || die "checksums.txt has no entry for $1"
  [ "$2" = "$_expected" ] || die "sha256 mismatch for $1: expected $_expected, got $2"
}

actual=$(sha256_of "$dl/$asset")
verify_asset "$asset" "$actual"
ui_actual=$(sha256_of "$dl/$ui_asset")
verify_asset "$ui_asset" "$ui_actual"

# gh release download writes 0644. The checksum covers the content, not the
# mode, and conformance.sh refuses a --binary it cannot execute.
chmod 0755 "$dl/$asset"

sh "$pack/tests/verify.sh" ||
  die "pack checks failed; release $tag is still a draft"

set +e
sh "$pack/tests/conformance.sh" --binary "$dl/$asset"   > "$tmp/conformance.out" 2> "$tmp/conformance.err"
conf_status=$?
set -e

rex_version=$("${REX_BIN:-rex}" version 2>/dev/null | head -1 || true)
[ -n "$rex_version" ] || rex_version=unknown
gc_bin=${GC_BIN:-gc}
gc_version=$("$gc_bin" version 2>/dev/null | head -1 || true)
[ -n "$gc_version" ] || gc_version=unknown
gc_commit=$("$gc_bin" version --json 2>/dev/null |
  python3 -c 'import json,sys; print(json.load(sys.stdin)["commit"])' 2>/dev/null || true)
[ -n "$gc_commit" ] || gc_commit=unknown

if [ "$conf_status" -eq 0 ]; then result=PASS; else result=FAIL; fi
{
  echo "result: $result"
  echo "exit_code: $conf_status"
  echo "rex version: $rex_version"
  echo "gc version: $gc_version"
  echo "gc commit: $gc_commit"
  echo "asset: $asset"
  echo "asset_sha256: $actual"
  echo "ui_asset: $ui_asset"
  echo "ui_asset_sha256: $ui_actual"
  echo "host_os: $os"
  echo "host_arch: $arch"
  echo "timestamp: $(date -u +%Y-%m-%dT%H:%M:%SZ)"
  echo "tag: $tag"
  echo "tag_commit: $tag_commit"
  echo "stdout:"
  sed 's/^/  /' "$tmp/conformance.out"
  echo "stderr:"
  sed 's/^/  /' "$tmp/conformance.err"
} > "$tmp/conformance.txt"

cat "$tmp/conformance.out"

if [ "$conf_status" -ne 0 ]; then
  cat "$tmp/conformance.err" >&2
  # Evidence for whoever inspects the draft. --clobber is safe here: this file
  # is generated, unlike a binary asset.
  "$GH" release upload "$tag" "$tmp/conformance.txt" --clobber
  echo "release: conformance FAILED (exit $conf_status)" >&2
  echo "release: draft left in place: $release_url" >&2
  echo "release: cleanup: $GH release delete $tag --cleanup-tag" >&2
  exit 1
fi

# The draft body is written by hand, so it can be empty, stale or truncated.
# Check it here: the draft must exist before gh can read it, and every publish
# path, including --yes, must pass through. --dry-run exits above instead.
sh "$pack/check-release-notes.sh" "$tag" ||
  die "release notes for $tag failed the gate; the draft is unchanged; resume with: $0 $tag --resume"

if [ "$assume_yes" -ne 1 ]; then
  if [ -t 0 ]; then
    printf 'Publish %s? [y/N] ' "$tag"
    read -r answer || answer=
    case $answer in
      y|Y|yes|YES) ;;
      *)
        echo "release: not published; $tag is still a draft"
        echo "release: resume with: $0 $tag --resume"
        exit 1
        ;;
    esac
  else
    echo "release: refusing to publish without approval; pass --yes after a human approved" >&2
    echo "release: $tag is still a draft; resume with: $0 $tag --resume" >&2
    exit 1
  fi
fi

"$GH" release upload "$tag" "$tmp/conformance.txt" --clobber
"$GH" release edit "$tag" --draft=false

published_json=$("$GH" release view "$tag" --json url,assets)
echo "release: published $(json_field "$published_json" url)"
printf '%s' "$published_json" | python3 -c '
import json, sys
for a in json.load(sys.stdin)["assets"]:
    print("release: asset", a["name"])
'
