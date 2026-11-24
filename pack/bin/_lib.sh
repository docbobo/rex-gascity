#!/bin/sh
# Sourced helpers shared by pack/bin/gc-runtime-rex and
# pack/doctor/rex-binary/fix.sh so both compute the SAME cache path. Sourcing
# this file only defines functions; it has no side effects. POSIX sh.

# gc_runtime_cache_root: print the ABSOLUTE cache root. GC_RUNTIME_CACHE_DIR
# wins; else ${XDG_CACHE_HOME:-$HOME/.cache}/rex-gascity. Returns 1 with the
# cause on stderr when no root resolves, so a caller never mistakes a
# diagnostic line for a path.
gc_runtime_cache_root() {
  if [ -n "${GC_RUNTIME_CACHE_DIR:-}" ]; then
    # A relative override means a different directory for each caller: the shim
    # runs from the agent work dir, gc runs --fix from the pack dir. Reject it
    # rather than anchor it, because no single base is right for both.
    case $GC_RUNTIME_CACHE_DIR in
      /*) printf '%s' "$GC_RUNTIME_CACHE_DIR"; return 0 ;;
    esac
    echo "GC_RUNTIME_CACHE_DIR must be an absolute path (got: $GC_RUNTIME_CACHE_DIR)" >&2
    return 1
  fi
  if [ -n "${XDG_CACHE_HOME:-}" ]; then
    case $XDG_CACHE_HOME in
      /*) printf '%s/rex-gascity' "$XDG_CACHE_HOME"; return 0 ;;
    esac
    # XDG says a relative value is invalid; ignore it and use HOME instead.
  fi
  if [ -n "${HOME:-}" ]; then
    printf '%s' "$HOME/.cache/rex-gascity"; return 0
  fi
  echo "cannot locate the rex runtime cache: set an absolute GC_RUNTIME_CACHE_DIR, XDG_CACHE_HOME, or HOME" >&2
  return 1
}

# gc_runtime_platform: print "<os>-<arch>" for the release asset, or print a
# cause-naming line to stderr and return 1 for an unsupported platform. The
# os/arch sets match pack/install.sh:101-110.
gc_runtime_platform() {
  _os= _arch=
  case "$(uname -s)" in
    Darwin) _os=darwin ;;
    Linux) _os=linux ;;
    *) echo "unsupported operating system: $(uname -s) (release assets exist for darwin and linux only)" >&2; return 1 ;;
  esac
  case "$(uname -m)" in
    x86_64|amd64) _arch=amd64 ;;
    aarch64|arm64) _arch=arm64 ;;
    *) echo "unsupported machine: $(uname -m) (release assets exist for amd64 and arm64 only)" >&2; return 1 ;;
  esac
  printf '%s-%s' "$_os" "$_arch"
}

# gc_runtime_bin_dir <version>: print "<cache_root>/<version>/<os>-<arch>".
# Returns 1 (platform error already on stderr) when the platform is unsupported.
gc_runtime_bin_dir() {
  _plat=$(gc_runtime_platform) || return 1
  _root=$(gc_runtime_cache_root) || return 1
  printf '%s/%s/%s' "$_root" "$1" "$_plat"
}

# version_cmp prints -1, 0 or 1 comparing the first argument against the
# second as MAJOR.MINOR.PATCH[-preview.N]. It prints nothing when either string
# is not that scheme. Fields compare one at a time, so no field has a packing
# limit. A release ranks above any preview of the same MAJOR.MINOR.PATCH.
version_cmp() {
  awk -v a="$1" -v b="$2" '
    function parse(s, v,   rel, pre) {
      sub(/^v/, "", s)
      sub(/\+.*$/, "", s)
      if (s !~ /^[0-9]+\.[0-9]+\.[0-9]+(-preview\.[0-9]+)?$/) return 0
      split(s, rel, "-")
      split(rel[1], v, ".")
      v[4] = 999
      if (rel[2] != "") {
        split(rel[2], pre, ".")
        v[4] = pre[2]
      }
      return 1
    }
    BEGIN {
      if (!parse(a, av) || !parse(b, bv)) exit
      for (i = 1; i <= 4; i++) {
        if (av[i] + 0 < bv[i] + 0) { printf "-1"; exit }
        if (av[i] + 0 > bv[i] + 0) { printf "1"; exit }
      }
      printf "0"
    }
  '
}
