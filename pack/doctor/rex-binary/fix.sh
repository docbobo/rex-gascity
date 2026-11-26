#!/bin/sh
# gc doctor --fix for rex-gascity-runtime:rex-binary. Warm the version-keyed
# cache the shim (pack/bin/gc-runtime-rex) execs, by installing into the same
# GC_RUNTIME_INSTALL_DIR the shim computes — not onto PATH. The cache path comes
# from the shared pack/bin/_lib.sh so it matches the shim byte-for-byte.
#
# gc runs a fix with cwd at the pack dir and GC_PACK_DIR in the environment.
# Exit 0 when warmed; non-zero with the reason on stderr, which gc doctor prints
# on a failed fix.
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
pack_dir=$(CDPATH= cd -- "$script_dir/../.." && pwd)
pack_toml=$pack_dir/pack.toml
. "$pack_dir/bin/_lib.sh"

version=$(
  awk -F'"' '
    /^[[:space:]]*\[pack\][[:space:]]*$/ { in_pack = 1; next }
    /^[[:space:]]*\[/ { in_pack = 0 }
    in_pack && /^[[:space:]]*version[[:space:]]*=/ { print $2; exit }
  ' "$pack_toml" 2>/dev/null || true
)
[ -n "$version" ] || { echo "rex-binary fix: $pack_toml declares no [pack] version" >&2; exit 1; }

# --fix warms the provider cache only. The rex CLI ships inside the Rex app, so
# a CLI below the pack's API revision pin cannot be repaired here. That gate is
# soft: name the app and the revisions on stderr, then warm anyway. An absent
# pin or a missing/unreadable rex leaves the warm untouched.
min_api_revision=
if [ -r "$pack_dir/rex-min-api-revision" ]; then
  min_api_revision=$(tr -dc '0-9' < "$pack_dir/rex-min-api-revision") || min_api_revision=
fi
if [ -n "$min_api_revision" ]; then
  rex_name=${GC_REX_BIN:-rex}
  case "$rex_name" in
    */*) rex_path=$rex_name ;;
    *) rex_path=$(command -v "$rex_name" 2>/dev/null || true) ;;
  esac
  installed_api=
  if [ -n "$rex_path" ]; then
    version_json=$("$rex_path" version --json 2>/dev/null) || version_json=
    if [ -n "$version_json" ]; then
      installed_api=$(printf '%s\n' "$version_json" | sed -n 's/.*"api_revision"[[:space:]]*:[[:space:]]*\([0-9][0-9]*\).*/\1/p' | head -n 1)
    fi
    if [ -z "$installed_api" ]; then
      installed_api=$("$rex_path" version 2>/dev/null | sed -n 's/.*api:[[:space:]]*revision[[:space:]]*\([0-9][0-9]*\).*/\1/p' | head -n 1) || installed_api=
    fi
  fi
  case ${installed_api:-} in
    ''|*[!0-9]*)
      echo "rex-binary fix: could not determine the rex CLI API revision; warming the provider cache anyway." >&2 ;;
    *)
      if [ "$installed_api" -lt "$min_api_revision" ]; then
        echo "rex-binary fix: the Rex app's rex CLI is at API revision $installed_api, below the pack's $min_api_revision. --fix warms the provider cache only and cannot update the Rex app; update the Rex app. Continuing." >&2
      fi ;;
  esac
fi

if ! bin_dir=$(gc_runtime_bin_dir "$version"); then
  echo "rex-binary fix: no release asset resolves for this platform (pack version $version)" >&2
  exit 1
fi

# --fix runs OUT OF BAND — not on gc's 30 s protocol / 120 s start deadline — so
# it must NOT inherit the shim's tight [1,25] s fetch budget. install.sh is
# unbounded when GC_RUNTIME_INSTALL_DEADLINE is unset, so set an explicit
# GENEROUS budget: a slow release the shim timed out on can finish here, and the
# fetch stays bounded rather than hanging forever. Honor an operator override so
# an even slower link can be given more.
#
# install.sh enforces that budget with a `sleep` watchdog; with no `sleep` on
# PATH it degrades to an UNBOUNDED foreground fetch. That fallback is for a
# manual run, not for --fix: a stalled fetch would hang the fix forever. Refuse
# before the fetch instead, naming the cause. --from-source installs a local
# build and never fetches, so it stays allowed.
case " $* " in
  *" --from-source "*) ;;
  *)
    command -v sleep >/dev/null 2>&1 || {
      echo "rex-binary fix: cannot bound the install: 'sleep' is not on PATH, so a stalled fetch would hang this fix past its budget. Add 'sleep' to PATH, or install a build with 'sh $pack_dir/install.sh --from-source'." >&2
      exit 1
    }
    ;;
esac

export GC_RUNTIME_INSTALL_DIR="$bin_dir"
export GC_RUNTIME_INSTALL_DEADLINE="${GC_RUNTIME_INSTALL_DEADLINE:-300}"
exec sh "$pack_dir/install.sh" "$@"
