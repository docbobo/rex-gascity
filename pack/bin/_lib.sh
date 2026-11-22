#!/bin/sh
# Sourced helpers shared by pack/bin/gc-runtime-rex and
# pack/doctor/rex-binary/fix.sh so both compute the SAME cache path. Sourcing
# this file only defines functions; it has no side effects. POSIX sh.

# gc_runtime_cache_root: print the cache root. GC_RUNTIME_CACHE_DIR wins; else
# ${XDG_CACHE_HOME:-$HOME/.cache}/rex-gascity.
gc_runtime_cache_root() {
  if [ -n "${GC_RUNTIME_CACHE_DIR:-}" ]; then
    printf '%s' "$GC_RUNTIME_CACHE_DIR"
  else
    printf '%s' "${XDG_CACHE_HOME:-$HOME/.cache}/rex-gascity"
  fi
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
  printf '%s/%s/%s' "$(gc_runtime_cache_root)" "$1" "$_plat"
}
