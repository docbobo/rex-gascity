#!/bin/sh
# Install the rex runtime provider from a GitHub release.
#
# Usage: sh install.sh [--from-source]
#
# The release is chosen by the [pack] version in pack.toml, so a sha: pack
# import works only when that commit's [pack] version has a published release.
# If it does not, this script fails and names the tag it tried. The doctor
# check compares versions only: it cannot detect a same-version build from a
# different commit.
#
# No build step: the release asset is a prebuilt executable named
# gc-runtime-rex_<os>_<arch>, with checksums.txt beside it. A public release
# needs only curl. A private release needs gh, with the token in GITHUB_TOKEN
# or GH_TOKEN, because plain curl cannot fetch releases/download/... with a
# token.
set -eu

repo=docbobo/rex-gascity
pack_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
pack_toml=$pack_dir/pack.toml

die() {
  echo "install-gc-runtime-rex: $*" >&2
  exit 1
}

from_source=
for arg in "$@"; do
  case "$arg" in
    --from-source) from_source=1 ;;
    -h|--help) echo "usage: sh install.sh [--from-source]"; exit 0 ;;
    *) die "unknown argument: $arg (usage: sh install.sh [--from-source])" ;;
  esac
done

version=$(
  awk -F'"' '
    /^[[:space:]]*\[pack\][[:space:]]*$/ { in_pack = 1; next }
    /^[[:space:]]*\[/ { in_pack = 0 }
    in_pack && /^[[:space:]]*version[[:space:]]*=/ { print $2; exit }
  ' "$pack_toml" 2>/dev/null || true
)
[ -n "$version" ] || die "$pack_toml declares no [pack] version"

# One install destination: the directory of the provider on PATH, else
# ~/.local/bin. Never a second choice such as GOBIN.
target_dir=
if resolved=$(command -v gc-runtime-rex 2>/dev/null) && [ -n "$resolved" ]; then
  # Resolve the directory, not the spelling: PATH may hold a relative entry.
  target_dir=$(CDPATH= cd -- "$(dirname -- "$resolved")" 2>/dev/null && pwd) || target_dir=
fi
if [ -z "$target_dir" ]; then
  [ -n "${HOME:-}" ] || die "HOME is not set; cannot choose the default install directory"
  target_dir=$HOME/.local/bin
fi
target=$target_dir/gc-runtime-rex
mkdir -p "$target_dir" || die "cannot create $target_dir"

warn_path() {
  shadow=
  target_on_path=
  old_ifs=$IFS
  IFS=:
  for dir in $PATH; do
    [ -n "$dir" ] || continue
    if [ "$dir" = "$target_dir" ]; then
      target_on_path=1
      break
    fi
    if [ -e "$dir/gc-runtime-rex" ]; then
      shadow=$dir/gc-runtime-rex
      break
    fi
  done
  IFS=$old_ifs
  if [ -n "$shadow" ]; then
    echo "warning: $shadow comes before $target_dir on PATH; gc will run that copy instead" >&2
  elif [ -z "$target_on_path" ]; then
    echo "warning: $target_dir is not on PATH; add it so gc can resolve gc-runtime-rex" >&2
  fi
}

# Explicit source-checkout fallback. Opt-in only: a source build reports the
# module pseudo-version, so the doctor check may warn that it cannot confirm
# the version. The release asset is the default path.
if [ -n "$from_source" ]; then
  module_dir=$(CDPATH= cd -- "$pack_dir/.." && pwd)
  [ -f "$module_dir/go.mod" ] ||
    die "--from-source needs a source checkout, but $module_dir/go.mod does not exist"
  command -v go >/dev/null 2>&1 ||
    die "--from-source needs a Go toolchain on PATH"
  ( cd "$module_dir" && GOBIN="$target_dir" go install ./cmd/gc-runtime-rex ) ||
    die "go install failed (see the toolchain error above)"
  echo "installed $target from source"
  echo "warning: a source build reports the module pseudo-version; gc doctor may warn" >&2
  warn_path
  exit 0
fi

case "$(uname -s)" in
  Darwin) os=darwin ;;
  Linux) os=linux ;;
  *) die "unsupported operating system: $(uname -s) (release assets exist for darwin and linux only)" ;;
esac
case "$(uname -m)" in
  x86_64|amd64) arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
  *) die "unsupported machine: $(uname -m) (release assets exist for amd64 and arm64 only)" ;;
esac
asset=gc-runtime-rex_${os}_${arch}
base=https://github.com/$repo/releases/download/v$version

tmp=$(mktemp -d "${TMPDIR:-/tmp}/gc-runtime-rex.XXXXXX") || die "cannot create a temporary directory"
trap 'rm -rf "$tmp"' EXIT

sha256_of() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | cut -d' ' -f1
  else
    shasum -a 256 "$1" | cut -d' ' -f1
  fi
}

# fetch_curl <file> <url>: 0 on HTTP 2xx. Sets curl_code, or curl_transport
# on a transport failure. A 404 leaves an error page behind, so the status is
# checked before the file is trusted.
fetch_curl() {
  if code=$(curl -sSL -o "$1" -w '%{http_code}' "$2"); then
    curl_code=$code
    case "$code" in
      2??) return 0 ;;
    esac
    return 1
  fi
  curl_transport=1
  curl_code=000
  return 2
}

downloaded=
curl_code=000
curl_transport=
gh_failed=
if command -v curl >/dev/null 2>&1; then
  if fetch_curl "$tmp/checksums.txt" "$base/checksums.txt" &&
    fetch_curl "$tmp/$asset" "$base/$asset"; then
    downloaded=curl
  fi
fi
if [ -z "$downloaded" ] && command -v gh >/dev/null 2>&1; then
  if gh release download "v$version" -R "$repo" -p "$asset" -p checksums.txt \
    --dir "$tmp" --clobber; then
    downloaded=gh
  else
    gh_failed=1
  fi
fi
if [ -z "$downloaded" ]; then
  if ! command -v curl >/dev/null 2>&1 && ! command -v gh >/dev/null 2>&1; then
    die "neither curl nor gh is installed; install curl for a public release, or gh with GITHUB_TOKEN or GH_TOKEN for a private release"
  fi
  if [ -n "$gh_failed" ]; then
    die "gh release download v$version failed; a private release needs gh with GITHUB_TOKEN or GH_TOKEN that can read $repo"
  fi
  if [ -n "$curl_transport" ]; then
    die "cannot reach github.com: curl failed (network problem); check the network, or use gh with GITHUB_TOKEN or GH_TOKEN"
  fi
  case "$curl_code" in
    401|403|404)
      die "release v$version has no $asset, or $repo is private; a private release needs gh with GITHUB_TOKEN or GH_TOKEN, because curl cannot fetch releases/download/... with a token" ;;
    *)
      die "release v$version: HTTP $curl_code fetching $asset" ;;
  esac
fi

expected=$(awk -v name="$asset" '$2 == name || $2 == "*" name { print $1; exit }' "$tmp/checksums.txt")
[ -n "$expected" ] || die "checksums.txt for release v$version has no sha256 entry for $asset"
actual=$(sha256_of "$tmp/$asset")
[ "$actual" = "$expected" ] ||
  die "sha256 mismatch for $asset: release checksums.txt has $expected, download has $actual"

if [ -f "$target" ] && [ -x "$target" ] && [ "$(sha256_of "$target")" = "$expected" ]; then
  echo "gc-runtime-rex $version is already installed at $target"
else
  staging=$target_dir/.gc-runtime-rex.$$
  if ! cp "$tmp/$asset" "$staging" || ! chmod 0755 "$staging"; then
    rm -f "$staging"
    die "cannot write $staging"
  fi
  if ! mv -f "$staging" "$target"; then
    rm -f "$staging"
    die "cannot move $staging to $target"
  fi
  echo "installed $target (version $version)"
fi

warn_path

# Private repository rule: releases/download/... is not readable with a bearer
# token over curl, so a private release needs gh (GITHUB_TOKEN or GH_TOKEN)
# and this script falls back to gh automatically. The public URL is tried
# first, so the same script serves a public repository with curl alone.
#
# sha: policy: the release is v[pack].version, not the imported commit. A
# sha: import therefore works only when that commit's [pack] version has a
# published release; otherwise this script fails and names the tag.
