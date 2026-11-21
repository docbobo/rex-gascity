#!/bin/sh
# Cut a release in the safe order: draft, CI build, verify the downloaded
# asset, attach conformance.txt, publish.
#
#   pack/release.sh v<version> [--dry-run] [--resume] [--yes]
#
# Preconditions: clean tree (tracked files), branch main, [pack].version equal
# to <version>, and no release or tag for it. A leftover draft needs --resume;
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
assets="gc-runtime-rex_linux_amd64 gc-runtime-rex_linux_arm64 gc-runtime-rex_darwin_amd64 gc-runtime-rex_darwin_arm64 checksums.txt"

json_field() {
  printf '%s' "$1" | python3 -c "import json,sys; print(json.load(sys.stdin)['$2'])"
}

release_json=$("$GH" release view "$tag" --json isDraft,url,assets 2>/dev/null || true)
release_exists=0
[ -n "$release_json" ] && release_exists=1

remote_tag=0
[ -n "$(git -C "$root" ls-remote --tags origin "refs/tags/$tag" 2>/dev/null)" ] && remote_tag=1

if [ "$release_exists" -eq 1 ]; then
  is_draft=$(json_field "$release_json" isDraft)
  release_url=$(json_field "$release_json" url)
  [ "$resume" -eq 0 ] &&
    die "release $tag already exists; pass --resume to continue an existing draft"
  [ "$is_draft" = "True" ] || die "release $tag is already published; refusing to touch it"
else
  [ "$remote_tag" -eq 0 ] || die "tag $tag already exists without a release"
  [ "$resume" -eq 0 ] || die "no draft release $tag to resume"
fi

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
  "$GH" release create "$tag" --draft --target "$commit"     --title "rex runtime pack $tag"     --notes "Rex runtime pack $tag. See pack/README.md to import and install."
  "$GH" workflow run "$workflow" -f "tag=$tag"
fi

tmp=$(mktemp -d "${TMPDIR:-/tmp}/rexrelease.XXXXXX")
trap 'rm -rf "$tmp"' EXIT INT TERM

# Bounded wait for the four binaries and checksums.txt. A build that never
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
"$GH" release download "$tag" --pattern "$asset" --pattern checksums.txt --dir "$dl"
[ -f "$dl/$asset" ] || die "downloaded asset $asset is missing"
[ -f "$dl/checksums.txt" ] || die "downloaded checksums.txt is missing"

expected=$(awk -v a="$asset" '$2 == a || $2 == "*" a {print $1; exit}' "$dl/checksums.txt")
[ -n "$expected" ] || die "checksums.txt has no entry for $asset"
if command -v sha256sum >/dev/null 2>&1; then
  actual=$(sha256sum "$dl/$asset" | awk '{print $1}')
else
  actual=$(shasum -a 256 "$dl/$asset" | awk '{print $1}')
fi
[ "$actual" = "$expected" ] ||
  die "sha256 mismatch for $asset: expected $expected, got $actual"

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

tag_commit=$commit
if [ "$release_exists" -eq 1 ]; then
  git -C "$root" fetch --quiet origin "refs/tags/$tag:refs/tags/$tag" 2>/dev/null || true
  tag_commit=$(git -C "$root" rev-parse "$tag^{commit}" 2>/dev/null || echo unknown)
fi

if [ "$conf_status" -eq 0 ]; then result=PASS; else result=FAIL; fi
{
  echo "result: $result"
  echo "exit_code: $conf_status"
  echo "rex version: $rex_version"
  echo "gc version: $gc_version"
  echo "gc commit: $gc_commit"
  echo "asset: $asset"
  echo "asset_sha256: $actual"
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
