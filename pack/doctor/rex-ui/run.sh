#!/bin/sh
# gc doctor check: the Gas City UI extension is installed and the Rex config
# loads it from the installed copy at the pack's version.
#
# The UI is optional — Rex runs without it — so an install that is absent, or
# present but not wired, is a warning (exit 1). Two things are errors (exit 2),
# because they cannot be repaired by writing: a config file that does not load,
# and no rex CLI on PATH (detection has no substitute).
#
# Detection is `rex config check --json`: it loads the config in the local rex
# binary and reports each action's `source`. The server's action list is not
# consulted — it reports what was loaded at server start, so it goes stale
# until `rex config reload`. This check never starts a server, never edits a
# file, and never writes state.
#
# Pack doctor protocol: exit 0 OK, 1 warning, 2 error; the first stdout line is
# the message and later lines are details.
set -eu

# Locate the pack directory from this script, not from the cwd: gc's cwd for a
# pack check is not guaranteed. The script sits at <pack>/doctor/rex-ui/.
script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd -P)
pack_dir=$(CDPATH= cd -- "$script_dir/../.." && pwd -P)
pack_toml=$pack_dir/pack.toml

# The declared version and the cache helpers come from the pack. A missing
# helper is pack repair, not a host problem, so it is reported as an error.
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

cache_root=
if [ -r "$pack_dir/bin/_lib.sh" ]; then
  . "$pack_dir/bin/_lib.sh"
  cache_root=$(gc_runtime_cache_root 2>/dev/null) || cache_root=
fi

error_pack() {
  echo "the Gas City UI check cannot locate the pack's install path"
  echo "$1"
  echo "Restore the pack files (for example \`gc import install\`, or \`git checkout -- pack\`): this check needs \`bin/_lib.sh\` and a \`[pack] version\` to know which copy of the UI belongs to this pack."
  exit 2
}

[ -n "$declared_version" ] || error_pack "pack.toml declares no [pack] version"
[ -n "$cache_root" ] || error_pack "the cache root cannot be computed from \`bin/_lib.sh\`"

installed_root=$cache_root/$declared_version/ui

# The config file the *server* reads. REX_CONFIG wins, exactly as `rex config
# check` documents. The stub sits beside it, and REX_GASCITY_UI overrides the
# stub path, matching ui/install.sh.
config_file=${REX_CONFIG:-${XDG_CONFIG_HOME:-$HOME/.config}/rex/init.lua}
stub=${REX_GASCITY_UI:-${XDG_CONFIG_HOME:-$HOME/.config}/rex/gascity-ui.lua}

# The rex CLI is how this check sees what the config loads. Without it there is
# no answer at all, which is an error rather than a silent pass.
rex_name=${GC_REX_BIN:-rex}
case "$rex_name" in
  */*) rex_path=$rex_name ;;
  *) rex_path=$(command -v "$rex_name" 2>/dev/null || true) ;;
esac
if [ -z "$rex_path" ]; then
  echo "the rex CLI is not on PATH, so the Rex config cannot be checked"
  echo "\`$rex_name\` is not on PATH"
  echo "Install Rex (https://rex.dev) and put \`rex\` on PATH, or set GC_REX_BIN to its path, then run this check again."
  exit 2
fi

# One probe, read-only: the config as it is on disk right now. A missing file is
# not a failure to load — it is a machine where nothing is installed yet — so
# the probe is skipped instead of being reported as an error.
probe_err=
if [ ! -e "$config_file" ]; then
  probe_json='{"actions":[]}'
elif ! probe_json=$("$rex_path" config check --json 2>&1); then
  probe_err=$probe_json
  echo "$config_file does not load, so the Gas City UI cannot be checked"
  [ -z "$probe_err" ] || printf '%s\n' "$probe_err"
  echo "Fix that complaint first. Neither this check nor \`gc doctor --check rex-gascity-runtime:rex-ui --fix\` will write into a config that does not load."
  exit 2
fi

# Collect the `source` of every action a gascity_* name registered, and keep
# the first. The path before ":<line>" is the file that registered it; the
# directory above lua/gascity/ is the copy that is loaded.
sources=$(
  printf '%s\n' "$probe_json" | awk '
    /"name"[[:space:]]*:/ {
      n = $0
      sub(/.*"name"[[:space:]]*:[[:space:]]*"/, "", n)
      sub(/".*/, "", n)
      cur = n
    }
    /"source"[[:space:]]*:/ {
      s = $0
      sub(/.*"source"[[:space:]]*:[[:space:]]*"/, "", s)
      sub(/".*/, "", s)
      if (cur ~ /^gascity_/) printf "%s\t%s\n", cur, s
    }
  '
)
live_path=$(printf '%s\n' "$sources" | awk -F'\t' 'NF { print $2; exit }')
live_root=
if [ -n "$live_path" ]; then
  live_root=${live_path%%/lua/gascity/*}
  [ "$live_root" != "$live_path" ] || live_root=
fi

if [ -z "$live_root" ]; then
  if [ -e "$stub" ]; then
    echo "the Gas City UI is installed but $config_file does not load it"
    echo "stub: $stub"
    if [ -L "$config_file" ] || [ ! -w "$config_file" ]; then
      echo "$config_file cannot be written by this account, so the reference line has to be added where that file is generated:"
      echo "  dofile((os.getenv(\"REX_GASCITY_UI\") or (os.getenv(\"XDG_CONFIG_HOME\") or ((os.getenv(\"HOME\") or \"\") .. \"/.config\")) .. \"/rex/gascity-ui.lua\"))"
    else
      echo "Run \`gc doctor --check rex-gascity-runtime:rex-ui --fix\` to append the reference block to $config_file."
    fi
    exit 1
  fi
  echo "the Gas City UI is not installed"
  echo "nothing in $config_file loads the extension, and there is no stub at $stub"
  echo "Run \`gc doctor --check rex-gascity-runtime:rex-ui --fix\` to install v$declared_version and wire the config. Rex runs without the UI, so leaving it out is fine."
  exit 1
fi

if [ "$live_root" = "$installed_root" ]; then
  echo "the Gas City UI is installed and loaded from the installed copy"
  echo "extension: $installed_root (version $declared_version)"
  echo "config: $config_file"
  exit 0
fi

# A different version of the same pack. `case` rather than `[ ... = ... ]`:
# a pattern that matches more than one version directory still matches here.
case "$live_root" in
"$cache_root"/*/ui)
  loaded_version=${live_root#"$cache_root"/}
  loaded_version=${loaded_version%/ui}
  echo "the config loads Gas City UI version $loaded_version, but this pack ships $declared_version"
  echo "loaded: $live_path"
  echo "installed: $installed_root"
  echo "Run \`gc doctor --check rex-gascity-runtime:rex-ui --fix\` to install v$declared_version and point the stub at it."
  exit 1
  ;;
esac

echo "the config loads the Gas City UI from another copy"
echo "loaded: $live_path"
echo "installed: $installed_root"
echo "That copy is not this pack's install path, so nothing was changed. Remove that load (a line in \`user.lua\`, or another \`init.lua\` block) and run \`gc doctor --check rex-gascity-runtime:rex-ui --fix\`."
exit 1
