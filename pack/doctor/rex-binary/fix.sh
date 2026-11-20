#!/bin/sh
# gc doctor --fix remediation for the rex:rex-binary check: build and install
# the provider the pack declares, version-stamped to the version pack.toml
# declares, so the check's version comparison passes.
#
# gc runs a fix with cwd at the pack directory and GC_PACK_DIR/GC_CITY_PATH/
# GC_PACK_STATE_DIR in the environment (internal/doctor/pack_checks.go
# `PackScriptCheck.Fix`, gc v1.5.1). The Go module root is the directory above
# the pack: the pack is a subdirectory of the module, whether the city composes
# it through a local include or a remote import.
#
# The binary goes to GOBIN (or GOPATH/bin), which is why it survives a pack
# cache refresh: `gc import install` / `gc pack fetch` force-checkout and
# `git clean -fd` the cache (internal/config/pack_fetch.go `updatePack`), so a
# build written inside the pack directory would be discarded. The trade-off is
# that the directory must be on PATH, which the check reports; see
# docs/rex-binary-install.md.
#
# Exit 0 when installed; non-zero with the reason on stderr, which gc doctor
# prints on a failed fix.
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
pack_dir=$(CDPATH= cd -- "$script_dir/../.." && pwd)
module_dir=$(CDPATH= cd -- "$pack_dir/.." && pwd)

fail() {
  echo "rex-binary: $*" >&2
  exit 1
}

if [ ! -f "$module_dir/go.mod" ]; then
  fail "no go.mod beside the pack ($module_dir); this pack must be composed from a source checkout (local include) or a remote import, not copied without its module"
fi

version=$(
  awk -F'"' '
    /^[[:space:]]*\[pack\][[:space:]]*$/ { in_pack = 1; next }
    /^[[:space:]]*\[/ { in_pack = 0 }
    in_pack && /^[[:space:]]*version[[:space:]]*=/ { print $2; exit }
  ' "$pack_dir/pack.toml"
)
if [ -z "$version" ]; then
  fail "$pack_dir/pack.toml declares no [pack] version to stamp"
fi

if ! command -v go >/dev/null 2>&1; then
  fail "no Go toolchain on PATH (go.mod requires Go $(awk '/^go /{print $2; exit}' "$module_dir/go.mod")); install Go, or build elsewhere with: go install github.com/docbobo/rex-gascity/cmd/gc-runtime-rex@v$version"
fi

# GOTOOLCHAIN=local makes a too-old toolchain fail here instead of trying to
# download one; GOPROXY=off holds because the module has no dependencies (no
# go.sum, no replace), so the build needs no network.
cd "$module_dir"
if ! GOTOOLCHAIN=local GOPROXY=off go install -trimpath \
  -ldflags "-X main.buildVersion=$version" ./cmd/gc-runtime-rex; then
  fail "go install failed; see the toolchain error above"
fi

gobin=$(go env GOBIN)
if [ -z "$gobin" ]; then
  gobin=$(go env GOPATH)/bin
fi
echo "installed $gobin/gc-runtime-rex (version $version)"
case ":$PATH:" in
  *":$gobin:"*) ;;
  *)
    echo "warning: $gobin is not on PATH; add it so gc can resolve gc-runtime-rex" >&2
    ;;
esac
