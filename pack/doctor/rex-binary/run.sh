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
# declared release version; build with `make build` (or `-ldflags
# "-X main.buildVersion=<version>"`) to stamp the pack's version.
#
# Pack doctor protocol: exit 0 OK, 1 warning, 2 error; the first stdout line
# is the message and later lines are details. Read-only: it never starts a
# session or writes state.
set -eu

# Locate the pack directory from this script, not from the cwd: gc's cwd for a
# pack check is not guaranteed. The script sits at <pack>/doctor/rex-binary/.
script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
pack_dir=$(CDPATH= cd -- "$script_dir/../.." && pwd)
pack_toml="$pack_dir/pack.toml"

# normalize_version strips a leading `v` and any `+build-metadata` suffix, so
# `v0.1.0`, `0.1.0` and `0.1.0+dirty` all compare as `0.1.0`.
normalize_version() {
  v=$1
  v=${v#v}
  v=${v%%+*}
  printf '%s' "$v"
}

# The runtime command and the declared version. Fall back to the known binary
# name if pack.toml cannot be read, so the check still reports something
# useful; a missing declared version skips the comparison.
command_name=
declared_version=
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
if [ -z "$command_name" ]; then
  command_name=gc-runtime-rex
fi

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

if [ -z "$provider_path" ]; then
  provider_status=error
  provider_message="the rex runtime provider binary is not installed"
  provider_detail="\`$command_name\` is not on PATH"
  remediation="Install it with \`gc doctor --fix\` or \`sh $pack_dir/install.sh\` (downloads the v$declared_version release asset for this platform and verifies its checksum), then put its directory on PATH. A supervisor or controller runs the city with its own PATH, so add that directory to its environment too. A source checkout can use \`sh $pack_dir/install.sh --from-source\` instead."
elif [ ! -x "$provider_path" ]; then
  provider_status=error
  provider_message="the rex runtime provider binary is not executable"
  provider_detail="$provider_path is not executable"
  remediation="Reinstall it with \`gc doctor --fix\` or \`sh $pack_dir/install.sh\`."
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
rex_detail=
if [ -z "$rex_path" ]; then
  rex_status=error
  rex_detail="\`$rex_name\` is not on PATH"
elif [ ! -x "$rex_path" ]; then
  rex_status=error
  rex_detail="$rex_path is not executable"
else
  rex_detail="rex: $rex_path"
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
  echo "the rex CLI is not installed"
  echo "$rex_detail"
  echo "Install Rex (https://rex.dev) and put \`rex\` on PATH, or set GC_REX_BIN to its path."
  echo "$provider_detail"
  exit 2
fi

if [ "$provider_status" = warn ]; then
  echo "$provider_message"
  echo "$provider_detail"
  if [ -n "$remediation" ]; then echo "$remediation"; fi
  echo "$rex_detail"
  exit 1
fi

echo "the rex runtime provider and rex CLI are installed"
echo "$provider_detail"
if [ -n "$remediation" ]; then echo "$remediation"; fi
echo "$rex_detail"
exit 0
