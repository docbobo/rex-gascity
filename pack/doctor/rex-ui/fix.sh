#!/bin/sh
# gc doctor fix: install the Gas City UI extension and wire the Rex config.
#
# This runs the extension's own installer, which is the only writer either way:
#   * in a checkout, ui/install.sh next to this pack (--from-source);
#   * on an installed machine with the copy already in the cache, that copy's
#     install.sh (--from-source);
#   * otherwise, gascity-ui.tar.gz fetched from this pack's release, verified
#     against the release's checksums.txt, and installed from the unpacked tree.
#
# A fetch writes only under a temporary directory, so a failed fetch leaves the
# pack directory exactly as it was.
#
# Exit status is the installer's: 0 when the UI is installed and wired, or when
# the config cannot be written and the one line to add was printed; non-zero
# when nothing could be done.
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd -P)
pack_dir=$(CDPATH= cd -- "$script_dir/../.." && pwd -P)
pack_toml=$pack_dir/pack.toml

die() {
  echo "rex-ui fix: $1" >&2
  exit 1
}

declared_version=
if [ -r "$pack_toml" ]; then
  declared_version=$(
    awk -F'"' '
      /^[[:space:]]*\[pack\][[:space:]]*$/ { in_pack = 1; next }
      /^[[:space:]]*\[/ { in_pack = 0 }
      in_pack && /^[[:space:]]*version[[:space:]]*=/ { print $2; exit }
    ' "$pack_toml"
  )
fi
[ -n "$declared_version" ] || die "pack.toml declares no [pack] version; repair the pack first"

[ -r "$pack_dir/bin/_lib.sh" ] || die "bin/_lib.sh is missing from the pack; repair the pack first"
. "$pack_dir/bin/_lib.sh"
cache_root=$(gc_runtime_cache_root) || die "the cache root cannot be computed from bin/_lib.sh"
installed_root=$cache_root/$declared_version/ui

# 1. A checkout: the extension source sits beside the pack.
checkout_installer=$pack_dir/../ui/install.sh
if [ -f "$checkout_installer" ]; then
  exec sh "$checkout_installer" --from-source "$@"
fi

# 2. The copy this pack's version already installed.
if [ -f "$installed_root/install.sh" ]; then
  exec sh "$installed_root/install.sh" --from-source "$@"
fi

# 3. Fetch the release asset. Same repository and asset name as pack/install.sh
#    (line 19 and the version-keyed tag): one release carries the provider
#    binary, this tree and one checksums.txt.
repo=docbobo/rex-gascity
base=https://github.com/$repo/releases/download/v$declared_version
asset=gascity-ui.tar.gz
deadline=${GC_RUNTIME_INSTALL_DEADLINE:-300}

tmp=$(mktemp -d "${TMPDIR:-/tmp}/rex-ui-fix.XXXXXX") || die "cannot create a temporary directory"
trap 'rm -rf "$tmp"' EXIT HUP INT TERM

# curl first (no token needed for a public release), gh second: a curl that is
# installed but cannot reach an authenticated asset must not end the fetch.
fetch() {
  if command -v curl >/dev/null 2>&1 &&
    curl -fsSL --retry 2 --max-time "$deadline" -o "$2" "$1"; then
    return 0
  fi
  if command -v gh >/dev/null 2>&1; then
    GITHUB_TOKEN=${GITHUB_TOKEN:-${GH_TOKEN:-}} gh release download "v$declared_version" \
      -R "$repo" -p "$(basename -- "$1")" --dir "$tmp" --clobber
    return
  fi
  die "cannot download $(basename -- "$1"): curl failed or is not installed, and gh is not installed; install curl, or install the extension from a checkout instead"
}

fetch "$base/checksums.txt" "$tmp/checksums.txt" || die "cannot download checksums.txt for release v$declared_version"
fetch "$base/$asset" "$tmp/$asset" || die "cannot download $asset for release v$declared_version"

sha256_of() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | cut -d' ' -f1
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | cut -d' ' -f1
  else
    die "neither sha256sum nor shasum is on PATH, so $asset cannot be verified"
  fi
}

want=$(awk -v a="$asset" '$2 == a { print $1; exit }' "$tmp/checksums.txt")
[ -n "$want" ] || die "checksums.txt for release v$declared_version has no entry for $asset"
got=$(sha256_of "$tmp/$asset")
[ "$got" = "$want" ] || die "$asset does not match checksums.txt for release v$declared_version (declared $want, downloaded $got); nothing was installed"

mkdir -p "$tmp/unpack"
tar -xzf "$tmp/$asset" -C "$tmp/unpack" || die "$asset is not a readable gzip tarball"
[ -f "$tmp/unpack/ui/install.sh" ] || die "$asset does not contain ui/install.sh"

# Run, do not exec: the trap above has to remove the temporary directory even
# after a failed install.
status=0
sh "$tmp/unpack/ui/install.sh" --from-source "$@" || status=$?
exit "$status"
