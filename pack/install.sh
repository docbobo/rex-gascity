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
# Physical identity of the shipped shim's directory: the default destination
# must never be this dir, however it is spelled (PATH entry, symlink, fallback).
pack_bin=$(CDPATH= cd -- "$pack_dir/bin" 2>/dev/null && pwd -P) || pack_bin=$pack_dir/bin

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

# One install destination. GC_RUNTIME_INSTALL_DIR forces a specific dir (the
# version-keyed cache the shim/doctor use); otherwise the dir of the provider
# on PATH, else ~/.local/bin. Never a second choice such as GOBIN. The pack's
# own shipped shim at pack_dir/bin is never chosen: a PATH that holds pack/bin
# must not overwrite the tracked file, and a standalone PATH install must not
# override the pack-relative command.
target_dir=
if [ -n "${GC_RUNTIME_INSTALL_DIR:-}" ]; then
  target_dir=$GC_RUNTIME_INSTALL_DIR
else
  if resolved=$(command -v gc-runtime-rex 2>/dev/null) && [ -n "$resolved" ]; then
    # Compare PHYSICAL directory identities (pwd -P), not logical spellings: a
    # PATH entry that is a symlink to pack/bin resolves to the shipped shim, and
    # plain pwd would keep the link's spelling and miss the identity.
    resolved_dir=$(CDPATH= cd -- "$(dirname -- "$resolved")" 2>/dev/null && pwd -P) || resolved_dir=
    if [ -n "$resolved_dir" ] && [ "$resolved_dir" != "$pack_bin" ]; then
      target_dir=$resolved_dir
    fi
  fi
  if [ -z "$target_dir" ]; then
    [ -n "${HOME:-}" ] || die "HOME is not set; cannot choose the default install directory"
    target_dir=$HOME/.local/bin
  fi
fi
target=$target_dir/gc-runtime-rex
mkdir -p "$target_dir" || die "cannot create $target_dir"

# Guard the FINAL destination for EVERY branch, including an explicit
# GC_RUNTIME_INSTALL_DIR: that dir can be pack/bin or an alias of it, and
# --from-source would pass it to go install as GOBIN and replace the tracked
# shim. Compare PHYSICAL identities (pwd -P), so a PATH entry or alias that is a
# symlink to pack/bin is caught too.
target_dir_phys=$(CDPATH= cd -- "$target_dir" 2>/dev/null && pwd -P) || die "cannot resolve $target_dir"
[ "$target_dir_phys" != "$pack_bin" ] ||
  die "refusing to install into $target_dir: it is the pack's shipped shim directory; choose another directory"

# A directory at the target path passes -x, so the shim would try to execute it,
# and mv would move the staged file INSIDE it and report success. Refuse and
# name the entry to remove.
if [ -d "$target" ]; then
  die "$target is a directory, not the provider binary; remove that directory, then retry"
fi

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
  [ -n "${GC_RUNTIME_INSTALL_DIR:-}" ] || warn_path
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

# Whole-fetch budget (seconds). A caller sets GC_RUNTIME_INSTALL_DEADLINE: the
# shim and `gc doctor --fix` do. A plain `sh pack/install.sh` run is UNBOUNDED,
# so the default PATH-install behavior does not change (REQ-008).
if [ -n "${GC_RUNTIME_INSTALL_DEADLINE:-}" ]; then
  bounded=1
  install_deadline=$GC_RUNTIME_INSTALL_DEADLINE
  case $install_deadline in ''|*[!0-9]*) install_deadline=30 ;; esac
  # Shell arithmetic reads a leading zero as an octal prefix, so an operator's
  # "0308" would abort before any fetch. Strip the zeros: the digits are
  # decimal, and the hook keeps its own (larger) value rather than the shim's
  # [1,25] clamp.
  while :; do
    case $install_deadline in
      0?*) install_deadline=${install_deadline#0} ;;
      *) break ;;
    esac
  done
  [ -n "$install_deadline" ] || install_deadline=0
else
  bounded=; install_deadline=30   # unused while unbounded
fi

# The at-most-3 fetches (checksums, asset, gh fallback) AND the at-most-2
# inventory probes share ONE budget. With a working clock each fetch gets the
# time that is left. A clock that worked at the start and LATER fails must not
# restart the budget: the remaining time is unknown, so next_fetch_budget
# returns 1 and the bounded install stops with a cause. Without a clock at all,
# each step gets an equal share of what earlier steps left, so the slices
# together never exceed the budget (a 1 s floor on every slice would let the
# steps outlast a budget smaller than their count). The count reserves every
# possible step: a reinstall runs both probes after up to three fetches, and a
# short count would let the CANDIDATE probe time out and keep a working target
# from its own replacement.
if [ -n "$bounded" ] && fetch_start=$(date +%s 2>/dev/null) && [ -n "$fetch_start" ]; then
  have_clock=1
else
  have_clock=; fetch_start=
  clockless_left=$install_deadline
  clockless_attempts=5
fi

# Detect sleep ONCE while bounded. Without it the watchdog cannot run, so the
# install degrades to unbounded instead of failing a working fetch (REQ-020).
if [ -n "$bounded" ] && command -v sleep >/dev/null 2>&1; then
  have_sleep=1
else
  have_sleep=
fi

# next_fetch_budget: set budget_for_next to the seconds for the next fetch, or
# return 1 when a working clock became unreadable. budget_for_next=0 means the
# budget is exhausted: run_bounded reaps without launching the request.
budget_for_next=0
next_fetch_budget() {
  if [ -n "$have_clock" ]; then
    now=$(date +%s 2>/dev/null) || now=
    case $now in ''|*[!0-9]*) return 1 ;; esac
    budget_for_next=$(( fetch_start + install_deadline - now ))
  else
    if [ "$clockless_attempts" -le 0 ]; then budget_for_next=0; return 0; fi
    budget_for_next=$(( (clockless_left + clockless_attempts - 1) / clockless_attempts ))
    clockless_left=$(( clockless_left - budget_for_next ))
    clockless_attempts=$(( clockless_attempts - 1 ))
  fi
  [ "$budget_for_next" -gt 0 ] || budget_for_next=0
  return 0
}

# A lost clock stops the bounded install with the cause instead of giving the
# next fetch a fresh budget that could outlive gc's handshake deadline.
next_fetch_budget_or_die() {
  next_fetch_budget ||
    die "cannot read the clock (date) partway through a bounded install; the remaining fetch budget is unknown, so the install stops instead of restarting the ${install_deadline}s budget (unset GC_RUNTIME_INSTALL_DEADLINE for an unbounded manual install)"
}

# run_bounded <secs> <cmd...>: run a fetch with a watchdog. Unbounded, or with
# no sleep, runs the command in the foreground. Otherwise the fetch runs as a
# direct background child so its PID is killable without job control. The fetch
# keeps the caller's stdout/stderr redirections; the watchdog and its sleep run
# under /dev/null so an orphaned sleep never holds the caller's pipe open.
# Returns 124 when the watchdog reaped the fetch, and without launching at all
# when the shared budget is exhausted.
run_bounded() {
  _secs=$1; shift
  if [ -z "$bounded" ] || [ -z "$have_sleep" ]; then "$@"; return $?; fi
  case $_secs in ''|*[!0-9]*) _secs=0 ;; esac
  [ "$_secs" -gt 0 ] || return 124
  "$@" </dev/null &
  _child=$!
  ( sleep "$_secs"; kill -TERM "$_child" 2>/dev/null
    sleep 1; kill -KILL "$_child" 2>/dev/null ) </dev/null >/dev/null 2>&1 &
  _wd=$!
  _rc=0; wait "$_child" 2>/dev/null || _rc=$?
  kill -TERM "$_wd" 2>/dev/null || true
  wait "$_wd" 2>/dev/null || true
  case $_rc in 143|137) return 124 ;; esac
  return "$_rc"
}

# fetch_curl <file> <url>: 0 on HTTP 2xx. Sets curl_code, or curl_transport
# on a transport failure, and always records failed_url: a failure message must
# name the URL curl actually requested (checksums.txt first, then the asset).
# A 404 leaves an error page behind, so the status is checked before the file
# is trusted. Returns 124 when the budget reaps it.
fetch_curl() {
  failed_url=$2
  _codef=$tmp/.code.$$
  next_fetch_budget_or_die
  _rc=0
  run_bounded "$budget_for_next" curl -sSL -o "$1" -w '%{http_code}' "$2" 2>"$tmp/.curlerr" >"$_codef" || _rc=$?
  if [ "$_rc" -eq 124 ]; then
    rm -f "$_codef"
    curl_code=000; return 124
  fi
  code=$(sed -n 's/^[[:space:]]*\([0-9][0-9][0-9]\)[[:space:]]*$/\1/p' "$_codef" 2>/dev/null | sed -n '1p')
  rm -f "$_codef"
  if [ "$_rc" -eq 0 ]; then
    curl_code=$code
    case "$code" in
      2??) return 0 ;;
    esac
    return 1
  fi
  curl_transport=1
  curl_transport_msg=$(sed -n '1p' "$tmp/.curlerr" 2>/dev/null)
  curl_code=000
  return 2
}

downloaded=
curl_code=000
curl_transport=
curl_transport_msg=
failed_url=
gh_failed=

# A reaped fetch exits 124, which the shim maps to its first-use-budget remedy.
# Name the URL's role, the asset, and the release so every timeout message is
# self-contained; keep curl's own diagnostic when one preceded the gh timeout.
die_budget() {
  _msg="fetching $1 for release v$version (provider asset $asset) exceeded the ${install_deadline}s budget (slow or stalled network)"
  [ -z "${curl_transport_msg:-}" ] || _msg="$_msg; curl reported: $curl_transport_msg"
  [ "${curl_code:-000}" = 000 ] || _msg="$_msg; $failed_url returned HTTP $curl_code"
  echo "install-gc-runtime-rex: $_msg" >&2
  exit 124
}

if command -v curl >/dev/null 2>&1; then
  _rc=0; fetch_curl "$tmp/checksums.txt" "$base/checksums.txt" || _rc=$?
  [ "$_rc" -eq 124 ] && die_budget checksums.txt
  if [ "$_rc" -eq 0 ]; then
    _rc=0; fetch_curl "$tmp/$asset" "$base/$asset" || _rc=$?
    [ "$_rc" -eq 124 ] && die_budget "$asset"
    [ "$_rc" -eq 0 ] && downloaded=curl
  fi
fi
if [ -z "$downloaded" ] && command -v gh >/dev/null 2>&1; then
  _rc=0
  next_fetch_budget_or_die
  run_bounded "$budget_for_next" gh release download "v$version" -R "$repo" \
    -p "$asset" -p checksums.txt --dir "$tmp" --clobber || _rc=$?
  [ "$_rc" -eq 124 ] && die_budget "$asset"
  if [ "$_rc" -eq 0 ]; then
    downloaded=gh
  else
    gh_failed=1
  fi
fi
if [ -z "$downloaded" ]; then
  if ! command -v curl >/dev/null 2>&1 && ! command -v gh >/dev/null 2>&1; then
    die "neither curl nor gh is installed; install curl for a public release, or gh with GITHUB_TOKEN or GH_TOKEN for a private release ($asset, v$version)"
  fi
  # Name the URL that actually failed: checksums.txt is tried first, so a
  # checksums-only failure must not be blamed on the binary URL. The asset and
  # the release stay in every message (REQ-007).
  _from=${failed_url:+ from $failed_url}
  _gh=${gh_failed:+ (the gh fallback also failed)}
  if [ -n "$curl_transport" ]; then
    die "cannot reach github.com: ${curl_transport_msg:-curl failed (network problem)}; failed to fetch $asset$_from for release v$version; check the network, or use gh with GITHUB_TOKEN or GH_TOKEN$_gh"
  fi
  case "$curl_code" in
    404)
      die "release v$version: HTTP 404$_from for provider asset $asset$_gh" ;;
    401|403)
      die "release v$version: HTTP $curl_code$_from for provider asset $asset; $repo may be private, so a private release needs gh with GITHUB_TOKEN or GH_TOKEN$_gh" ;;
    *)
      die "release v$version: HTTP $curl_code$_from for provider asset $asset$_gh" ;;
  esac
fi

expected=$(awk -v name="$asset" '$2 == name || $2 == "*" name { print $1; exit }' "$tmp/checksums.txt")
[ -n "$expected" ] || die "checksums.txt for release v$version has no sha256 entry for $asset"
actual=$(sha256_of "$tmp/$asset")
[ "$actual" = "$expected" ] ||
  die "sha256 mismatch for $asset (release v$version): checksums.txt has $expected, download has $actual"

# curl and gh write the asset without execute permission. The guard probes
# the temporary copy, so make it executable first.
chmod 0755 "$tmp/$asset" || die "cannot make $tmp/$asset executable"

# Never replace a working provider with an asset that cannot answer
# inventory. A stub (a test fixture or a broken release) answers only
# protocol and version; installing it turns every supervisor cycle into an
# inventory/observe failure. Report the conflict instead.
#
# Both probes run under the same budget as the fetches: a provider published by
# a concurrent cold install could otherwise block this installer past gc's
# deadline. Returns 0 when the binary answers, 1 when it does not, and 2 when the
# probe hit the budget — a timeout does NOT show that the provider is a stub.
answers_inventory() {
  _bin=$1
  [ -f "$_bin" ] && [ -x "$_bin" ] || return 1
  next_fetch_budget_or_die
  _rc=0; run_bounded "$budget_for_next" "$_bin" inventory >/dev/null 2>&1 || _rc=$?
  case $_rc in
    0) return 0 ;;
    124) return 2 ;;
    *) return 1 ;;
  esac
}
_rc=0; answers_inventory "$target" || _rc=$?
[ "$_rc" -ne 2 ] ||
  die "$target did not answer 'inventory' inside the ${install_deadline}s budget; an unresponsive provider is not a stub, so nothing is replaced (raise GC_RUNTIME_INSTALL_DEADLINE to probe it)"
if [ "$_rc" -eq 0 ]; then
  _rc=0; answers_inventory "$tmp/$asset" || _rc=$?
  [ "$_rc" -ne 1 ] ||
    die "$target answers inventory, but $asset does not; refusing to replace a working gc-runtime-rex (remove $target first to force the install)"
  [ "$_rc" -ne 2 ] ||
    die "$asset did not answer 'inventory' inside the ${install_deadline}s budget; cannot prove it is not a stub, so the working $target is kept (raise GC_RUNTIME_INSTALL_DEADLINE to probe it)"
fi

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

# A cache install is not on PATH, so skip the PATH warning.
[ -n "${GC_RUNTIME_INSTALL_DIR:-}" ] || warn_path

# Private repository rule: releases/download/... is not readable with a bearer
# token over curl, so a private release needs gh (GITHUB_TOKEN or GH_TOKEN)
# and this script falls back to gh automatically. The public URL is tried
# first, so the same script serves a public repository with curl alone.
#
# sha: policy: the release is v[pack].version, not the imported commit. A
# sha: import therefore works only when that commit's [pack] version has a
# published release; otherwise this script fails and names the tag.
