#!/bin/sh
# gc doctor check: the pack's runtime provider binary and the rex CLI are
# installed. The pack declares [runtimes.rex] command = "gc-runtime-rex"; gc
# resolves a bare name through PATH and a path with a separator relative to
# the pack directory. A missing provider surfaces only as opaque agent state,
# so this check reports it with a remediation.
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

# The runtime command the pack declares. Fall back to the known binary name if
# pack.toml cannot be read, so the check still reports something useful.
command_name=
if [ -r "$pack_toml" ]; then
  command_name=$(
    awk -F'"' '
      /^[[:space:]]*\[runtimes[^]]*rex[^]]*\][[:space:]]*$/ { in_section = 1; next }
      /^[[:space:]]*\[/ { in_section = 0 }
      in_section && /^[[:space:]]*command[[:space:]]*=/ { print $2; exit }
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

# --- provider binary ---------------------------------------------------------

provider_status=ok
provider_detail=
if [ -z "$provider_path" ]; then
  provider_status=error
  provider_detail="\`$command_name\` is not on PATH"
elif [ ! -x "$provider_path" ]; then
  provider_status=error
  provider_detail="$provider_path is not executable"
else
  # Verify the binary speaks RPP v0. The protocol op is local and cheap.
  if handshake=$("$provider_path" protocol 2>/dev/null) &&
    printf '%s' "$handshake" | grep -Eq '"version"[[:space:]]*:[[:space:]]*0'; then
    provider_detail="provider: $provider_path"
  else
    provider_status=error
    provider_detail="\`$provider_path protocol\` did not print an RPP v0 handshake"
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
  echo "the rex runtime provider binary is not installed"
  echo "$provider_detail"
  echo "Install it with \`go install github.com/docbobo/rex-gascity/cmd/gc-runtime-rex@latest\`, then put \$(go env GOBIN) on PATH. A supervisor or controller runs the city with its own PATH, so add that directory to its environment too."
  [ "$rex_status" = error ] && echo "$rex_detail"
  exit 2
fi

if [ "$rex_status" = error ]; then
  echo "the rex CLI is not installed"
  echo "$rex_detail"
  echo "Install Rex (https://rex.dev) and put \`rex\` on PATH, or set GC_REX_BIN to its path."
  echo "$provider_detail"
  exit 2
fi

echo "the rex runtime provider and rex CLI are installed"
echo "$provider_detail"
echo "$rex_detail"
