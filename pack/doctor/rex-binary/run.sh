#!/bin/sh
# gc doctor check: the pack's runtime provider binary and the rex CLI are
# installed, and the provider matches the version the pack declares. The pack
# declares [runtimes.rex] command = "gc-runtime-rex" and [pack] version; gc
# resolves a bare name through PATH and a path with a separator relative to
# the pack directory. A missing provider surfaces only as opaque agent state,
# so this check reports it with a remediation.
#
# Version comparison: the provider answers the `version` op with one JSON line
# (see gc-runtime-rex). The declared version comes from pack.toml, which gc
# does not otherwise expose to a check. Match = the reported version equals the
# declared version after stripping a leading `v` and any `+build-metadata`
# suffix, so a tagged `v0.1.0` and a `-ldflags`-stamped `0.1.0` both match. A
# version that cannot be determined (the provider reports none, or lacks the
# op) warns rather than errors, because the binary still handshakes; a
# determined but different version is an error. A local `go install` from an
# untagged tree reports a module pseudo-version, which will not match the
# declared release version. Build with `make build` (or `-ldflags
# "-X main.buildVersion=<version>"`) to stamp the pack's version.
#
# The rex CLI is gated on its API revision, not its marketing version: the
# installed CLI must be at least the api_revision the pack is built against
# (pack/rex-min-api-revision). The reading comes from `rex version --json`'s
# api_revision field, or the plain `rex version` line `api: revision N
# (supports ...)` when --json is unsupported. The gate is soft: a lower or
# undeterminable revision warns, it does not fail the check.
#
# Pack doctor protocol: exit 0 OK, 1 warning, 2 error; the first stdout line
# is the message and later lines are details. Read-only: it never starts a
# session or writes state.
set -eu

# Locate the pack directory from this script, not from the cwd: gc's cwd for a
# pack check is not guaranteed. The script sits at <pack>/doctor/rex-binary/.
script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd -P)
pack_dir=$(CDPATH= cd -- "$script_dir/../.." && pwd -P)
pack_toml="$pack_dir/pack.toml"

if [ -r "$pack_dir/bin/_lib.sh" ]; then . "$pack_dir/bin/_lib.sh"; fi

# normalize_version strips a leading `v` and any `+build-metadata` suffix, so
# `v0.1.0`, `0.1.0` and `0.1.0+dirty` all compare as `0.1.0`.
normalize_version() {
  v=$1
  v=${v#v}
  v=${v%%+*}
  printf '%s' "$v"
}

# The runtime command, the declared version, and the rex CLI minimum. Fall
# back to the known binary name if pack.toml cannot be read, so the check
# still reports something useful. A missing declared version skips the
# provider comparison and a missing minimum skips the CLI comparison.
command_name=
declared_version=
min_version=
if [ -r "$pack_toml" ]; then
  command_name=$(
    awk -F'"' '
      /^[[:space:]]*\[runtimes[^]]*rex[^]]*\][[:space:]]*$/ { in_section = 1; next }
      /^[[:space:]]*\[/ { in_section = 0 }
      in_section && /^[[:space:]]*command[[:space:]]*=/ { print $2; exit }
    ' "$pack_toml"
  )
  declared_version=$(
    awk -F'"' '
      /^[[:space:]]*\[pack\][[:space:]]*$/ { in_pack = 1; next }
      /^[[:space:]]*\[/ { in_pack = 0 }
      in_pack && /^[[:space:]]*version[[:space:]]*=/ { print $2; exit }
    ' "$pack_toml"
  )
fi
# The rex CLI API revision floor lives in its own file: gc decodes pack.toml
# strictly and rejects an unknown key under [runtimes.rex], so the pin cannot
# sit there. It is the api_revision of the Rex the pack is built against.
min_api_revision=
if [ -r "$pack_dir/rex-min-api-revision" ]; then
  min_api_revision=$(tr -dc '0-9' < "$pack_dir/rex-min-api-revision") || min_api_revision=
fi
if [ -z "$command_name" ]; then
  command_name=gc-runtime-rex
fi

# phys_path <path>: print the physical absolute path, or the argument unchanged
# when its directory cannot be resolved. A "./bin/..." spelling, a symlinked
# pack dir, or an alias must not hide the shipped shim's identity. Uses only
# shell builtins: the doctor PATH is minimal and may have no dirname/basename.
phys_path() {
  case $1 in
    */*) _pdir=${1%/*}; _pbase=${1##*/} ;;
    *) _pdir=.; _pbase=$1 ;;
  esac
  [ -n "$_pdir" ] || _pdir=/
  [ -n "$_pbase" ] || _pbase=$1
  _pdir=$(CDPATH= cd -- "$_pdir" 2>/dev/null && pwd -P) || { printf '%s' "$1"; return 0; }
  printf '%s/%s' "$_pdir" "$_pbase"
}

# Resolve the runtime command. A separator means a path relative to the pack
# directory (or an absolute path); otherwise gc resolves it through PATH.
case "$command_name" in
  */*)
    case "$command_name" in
      /*) provider_path=$command_name ;;
      *) provider_path=$pack_dir/$command_name ;;
    esac
    ;;
  *)
    provider_path=$(command -v "$command_name" 2>/dev/null || true)
    ;;
esac

# Normalize to a physical path BEFORE comparing identities: a config of
# "./bin/gc-runtime-rex" keeps its "./" and would otherwise string-mismatch the
# shipped shim, exec it, and download on a cold cache.
pack_shim=$(phys_path "$pack_dir/bin/gc-runtime-rex")
if [ -n "$provider_path" ]; then
  provider_path=$(phys_path "$provider_path")
fi

# When the command is THIS pack's self-installing shim, probe the CACHED binary
# directly: exec'ing the shim on a cold cache would download, breaking this
# check's read-only contract. Match the shipped shim by its exact resolved path
# (not any */gc-runtime-rex), compute the same cache path the shim uses via the
# shared helper, and point the probe there. A cold cache is reported as
# remediable with no network I/O. A cache path that cannot be computed is
# reported as an error and the shim is never exec'd. Other configured
# executables keep the direct probe.
cold_cache=
shim_unresolved=
shim_unresolved_kind=
if [ "$provider_path" = "$pack_shim" ] && [ -x "$provider_path" ]; then
  if [ -z "$declared_version" ]; then
    shim_unresolved_kind=pack
    shim_unresolved="the pack declares no [pack] version, so the shim cache path cannot be computed"
  elif [ ! -r "$pack_dir/bin/_lib.sh" ]; then
    shim_unresolved_kind=pack
    shim_unresolved="the shim cache helper \`bin/_lib.sh\` is missing"
  else
    if ! gc_runtime_platform >/dev/null 2>&1; then
      shim_unresolved_kind=platform
      _plat_err=$(gc_runtime_platform 2>&1) || true
      shim_unresolved="no release asset resolves for this platform: $_plat_err"
    elif probe_bin_dir=$(gc_runtime_bin_dir "$declared_version" 2>&1); then
      provider_path=$probe_bin_dir/gc-runtime-rex
      [ -f "$provider_path" ] && [ -x "$provider_path" ] || cold_cache=1
    else
      shim_unresolved_kind=cache
      shim_unresolved="the shim cache path cannot be computed: $probe_bin_dir"
    fi
  fi
fi

# Resolve the rex CLI. The provider drives it for every session operation, so
# it is a prerequisite too. GC_REX_BIN overrides the PATH lookup.
rex_name=${GC_REX_BIN:-rex}
case "$rex_name" in
  */*)
    rex_path=$rex_name
    ;;
  *)
    rex_path=$(command -v "$rex_name" 2>/dev/null || true)
    ;;
esac

# --- provider binary and version --------------------------------------------

provider_status=ok
provider_message="the rex runtime provider and rex CLI are installed"
provider_detail=
remediation=

if [ -n "$shim_unresolved" ]; then
  provider_status=error
  provider_message="the rex runtime provider cache could not be located"
  provider_detail="$shim_unresolved"
  case "$shim_unresolved_kind" in
    platform)
      remediation="No rex runtime release asset exists for this platform. Use a supported platform (darwin or linux, amd64 or arm64), or install a build with \`sh $pack_dir/install.sh --from-source\`. Warming the cache cannot help." ;;
    pack)
      remediation="This is pack repair, not a cache warm: restore the pack files (for example \`gc import install\`, or \`git checkout -- pack\`), because \`gc doctor --fix\` and a session start both need \`bin/_lib.sh\` to find the cache." ;;
    *)
      remediation="Fix the cache location, then run \`gc doctor --fix\`: set an absolute \`GC_RUNTIME_CACHE_DIR\`, or set \`XDG_CACHE_HOME\` or \`HOME\`." ;;
  esac
elif [ -n "$cold_cache" ]; then
  provider_status=error
  provider_message="the rex runtime provider is not yet installed"
  provider_detail="no cached binary at $provider_path"
  remediation="Run \`gc doctor --fix\` to warm the cache, or start a session — the \`bin/gc-runtime-rex\` entry point installs the v$declared_version release into the cache on first use. A failed first run is a download error (offline, private repo, or unknown platform)."
elif [ -z "$provider_path" ]; then
  provider_status=error
  provider_message="the rex runtime provider binary is not installed"
  provider_detail="\`$command_name\` is not on PATH"
  remediation="This command is resolved through PATH, so warming the cache cannot help: install \`gc-runtime-rex\` somewhere on PATH (for example \`sh $pack_dir/install.sh\`, which writes to the provider on PATH or \`~/.local/bin\`), or point the pack's runtime command at the shipped shim \`bin/gc-runtime-rex\`, which installs into its own cache on first use."
elif [ ! -x "$provider_path" ]; then
  provider_status=error
  provider_message="the rex runtime provider binary is not executable"
  provider_detail="$provider_path is not executable"
  if [ "$provider_path" = "$pack_shim" ]; then
    remediation="Restore the shipped \`bin/gc-runtime-rex\` entry point (for example \`gc import install\` or \`git checkout -- pack/bin/gc-runtime-rex\`). That is pack repair; \`gc doctor --fix\` and session start warm an empty cache and do not repair the pack file."
  else
    remediation="Reinstall it with \`gc doctor --fix\` or \`sh $pack_dir/install.sh\`."
  fi
else
  handshake=$("$provider_path" protocol 2>/dev/null) || handshake=""
  if printf '%s' "$handshake" | grep -Eq '"version"[[:space:]]*:[[:space:]]*0'; then
    provider_detail="provider: $provider_path"
    if [ -n "$declared_version" ]; then
      reported_version=
      version_json=
      if version_json=$("$provider_path" version 2>/dev/null); then
        reported_version=$(printf '%s' "$version_json" | sed -n 's/.*"version"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p')
      fi
      normalized_reported=$(normalize_version "$reported_version")
      normalized_declared=$(normalize_version "$declared_version")
      if [ -z "$normalized_reported" ]; then
        provider_status=warn
        provider_message="the rex runtime provider version could not be determined"
        provider_detail="provider: $provider_path (version could not be determined)"
        remediation="The provider reported no version (a source build reports the module pseudo-version or \"(devel)\", and an older binary has no \`version\` op). Install the release asset with \`gc doctor --fix\` or \`sh $pack_dir/install.sh\`."
      elif [ "$normalized_reported" = "$normalized_declared" ]; then
        provider_detail="provider: $provider_path (version $reported_version)"
        if printf '%s' "$version_json" | grep -Eq '"modified"[[:space:]]*:[[:space:]]*true'; then
          remediation="The provider was built from a modified working tree; its version still matches pack version $declared_version."
        fi
      else
        provider_status=error
        provider_message="the rex runtime provider version does not match the pack"
        provider_detail="provider: $provider_path reports version $reported_version, but the pack declares $declared_version"
        remediation="Install the provider that matches pack version $declared_version with \`gc doctor --fix\` or \`sh $pack_dir/install.sh\`. The check compares versions only, so it cannot detect a same-version build from a different commit."
      fi
    fi
  else
    provider_status=error
    provider_message="the rex runtime provider did not answer the RPP handshake"
    provider_detail="\`$provider_path protocol\` did not print an RPP v0 handshake"
    remediation="Reinstall it with \`gc doctor --fix\` or \`sh $pack_dir/install.sh\`; a released build answers \`protocol\` with an RPP v0 handshake."
  fi
fi

# --- rex CLI -----------------------------------------------------------------

rex_status=ok
rex_message=
rex_detail=
rex_remediation=
if [ -z "$rex_path" ]; then
  rex_status=error
  rex_message="the rex CLI is not installed"
  rex_detail="\`$rex_name\` is not on PATH"
  rex_remediation="Install Rex (https://rex.dev) and put \`rex\` on PATH, or set GC_REX_BIN to its path."
elif [ ! -x "$rex_path" ]; then
  rex_status=error
  rex_message="the rex CLI is not executable"
  rex_detail="$rex_path is not executable"
  rex_remediation="Reinstall Rex (https://rex.dev) or set GC_REX_BIN to an executable."
else
  rex_detail="rex: $rex_path"
  if [ -n "$min_api_revision" ]; then
    installed_api=
    version_json=$("$rex_path" version --json 2>/dev/null) || version_json=
    if [ -n "$version_json" ]; then
      installed_api=$(printf '%s\n' "$version_json" | sed -n 's/.*"api_revision"[[:space:]]*:[[:space:]]*\([0-9][0-9]*\).*/\1/p' | head -n 1)
    fi
    if [ -z "$installed_api" ]; then
      installed_api=$("$rex_path" version 2>/dev/null | sed -n 's/.*api:[[:space:]]*revision[[:space:]]*\([0-9][0-9]*\).*/\1/p' | head -n 1) || installed_api=
    fi
    case ${installed_api:-} in
      ''|*[!0-9]*)
        rex_status=warn
        rex_message="the rex CLI API revision could not be determined"
        rex_detail="rex: $rex_path (API revision could not be determined)"
        rex_remediation="The CLI reported no api_revision. Update Rex so \`rex version --json\` prints an api_revision field." ;;
      *)
        if [ "$installed_api" -lt "$min_api_revision" ]; then
          rex_status=warn
          rex_message="the Rex app's rex CLI is older than the pack's API revision"
          rex_detail="rex: $rex_path reports API revision $installed_api, but the pack was built against $min_api_revision"
          rex_remediation="Update the Rex app to a build at API revision $min_api_revision or newer. The rex CLI is bundled with the app, so a CLI-only update cannot fix this. This is a warning, not a failure."
        else
          rex_detail="rex: $rex_path (API revision $installed_api)"
        fi ;;
    esac
  fi
fi

# --- report, worst finding first --------------------------------------------

if [ "$provider_status" = error ]; then
  echo "$provider_message"
  echo "$provider_detail"
  if [ -n "$remediation" ]; then echo "$remediation"; fi
  if [ "$rex_status" = error ]; then echo "$rex_detail"; fi
  exit 2
fi

if [ "$rex_status" = error ]; then
  echo "$rex_message"
  echo "$rex_detail"
  if [ -n "$rex_remediation" ]; then echo "$rex_remediation"; fi
  echo "$provider_detail"
  exit 2
fi

if [ "$provider_status" = warn ]; then
  echo "$provider_message"
  echo "$provider_detail"
  if [ -n "$remediation" ]; then echo "$remediation"; fi
  if [ "$rex_status" = warn ]; then
    echo "$rex_message"
    echo "$rex_detail"
    if [ -n "$rex_remediation" ]; then echo "$rex_remediation"; fi
  else
    echo "$rex_detail"
  fi
  exit 1
fi

if [ "$rex_status" = warn ]; then
  echo "$rex_message"
  echo "$rex_detail"
  if [ -n "$rex_remediation" ]; then echo "$rex_remediation"; fi
  echo "$provider_detail"
  exit 1
fi

echo "the rex runtime provider and rex CLI are installed"
echo "$provider_detail"
if [ -n "$remediation" ]; then echo "$remediation"; fi
echo "$rex_detail"
exit 0
