#!/bin/sh
# Install the Gas City UI extension for Rex.
#
# Usage: sh ui/install.sh [--from-source] [--uninstall] [--help]
#
# The UI is a Lua extension for the Rex server on this machine. It is not a pack
# file and not a city file: Rex reads one config file per account, so the
# install happens once per machine, outside any city.
#
# Two files are written on the host:
#
#   ~/.config/rex/gascity-ui.lua   the load stub, pointing at the installed tree
#   ~/.config/rex/init.lua         a marked block that loads the stub
#
# The extension tree itself lands in the version-keyed cache beside the provider
# binary, so installing a version never edits the config again:
#
#   ${GC_RUNTIME_CACHE_DIR:-${XDG_CACHE_HOME:-$HOME/.cache}/rex-gascity}/<version>/ui/
#
# Only init.lua is ever edited, and only by appending one marked block. The
# extension is never a dependency of it: another extension may own that file,
# and init.lua loads a dependency only when that file exists.
#
# Default (no flag): fetch gascity-ui.tar.gz for this tree's ui/version from the
# release v<version>, verify its sha256 against checksums.txt, and install it.
# A public release needs only curl; a private one needs gh with GITHUB_TOKEN or
# GH_TOKEN. --from-source installs the copy that sits beside this script, which
# is what a checkout, a branch under test, and `gc doctor --fix` use.
#
# GC_RUNTIME_INSTALL_DEADLINE bounds the whole fetch (seconds); the doctor fix
# sets it. A hand-run install is unbounded.
set -eu

repo=docbobo/rex-gascity
script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd -P)
version_file=$script_dir/version

die() {
  echo "install-gascity-ui: $*" >&2
  exit 1
}

note() {
  echo "install-gascity-ui: $*"
}

warn() {
  echo "install-gascity-ui: $*" >&2
}

usage() {
  cat <<'USAGE'
usage: sh ui/install.sh [--from-source] [--uninstall] [--help]

  --from-source   install the tree beside this script, no network
  --uninstall     remove the load stub and the init.lua block
  --help          this text
USAGE
}

from_source=
uninstall=
for arg in "$@"; do
  case "$arg" in
    --from-source) from_source=1 ;;
    --uninstall) uninstall=1 ;;
    -h|--help) usage; exit 0 ;;
    *) die "unknown argument: $arg (try --help)" ;;
  esac
done
[ -z "$uninstall" ] || [ -z "$from_source" ] || die "--uninstall and --from-source cannot be combined"

# ---------------------------------------------------------------------------
# Where things live
# ---------------------------------------------------------------------------

[ -n "${HOME:-}" ] || die "HOME is not set; cannot locate the Rex config"

# The cache root is the provider's, to the byte: source the shared helper when
# this tree ships beside the pack, and mirror it otherwise (a release tarball
# carries no pack/). check-ui-installer.sh asserts the two agree.
pack_lib=$script_dir/../pack/bin/_lib.sh
if [ -r "$pack_lib" ]; then
  . "$pack_lib"
else
  # Mirror of pack/bin/_lib.sh:gc_runtime_cache_root. Keep the two in step.
  gc_runtime_cache_root() {
    if [ -n "${GC_RUNTIME_CACHE_DIR:-}" ]; then
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
    fi
    if [ -n "${HOME:-}" ]; then
      printf '%s' "$HOME/.cache/rex-gascity"; return 0
    fi
    echo "cannot locate the rex runtime cache: set an absolute GC_RUNTIME_CACHE_DIR, XDG_CACHE_HOME, or HOME" >&2
    return 1
  }
fi

cache_root=$(gc_runtime_cache_root) || die "cannot locate the rex runtime cache"

# The config file the *server* reads. REX_CONFIG wins, exactly as `rex config
# check` documents. The stub and the reference block are computed from the
# standard config directory, so the block stays portable in a dotfiles repo.
config_home=${XDG_CONFIG_HOME:-$HOME/.config}
rex_config_dir=$config_home/rex
config_file=${REX_CONFIG:-$rex_config_dir/init.lua}
stub=${REX_GASCITY_UI:-$rex_config_dir/gascity-ui.lua}
[ -n "$stub" ] || die "REX_GASCITY_UI is empty"
case $stub in
  */*) ;;
  *) die "REX_GASCITY_UI must contain a path (got: $stub)" ;;
esac

# The stub is the installer's own file. It must never be the user's settings
# file or the config file: the loader would replace settings, and uninstall
# could delete them. Compare the paths after resolving a symlinked directory.
stub_marker='-- rex-gascity-ui loader.'
settings_file=$rex_config_dir/gascity.lua
canonical() { # canonical <path>: resolve the directory, keep the name
  _d=$(dirname -- "$1" 2>/dev/null); _b=$(basename -- "$1" 2>/dev/null)
  _d=$(CDPATH= cd -- "$_d" 2>/dev/null && pwd -P) || _d=
  if [ -n "$_d" ]; then printf '%s/%s' "$_d" "$_b"; else printf '%s' "$1"; fi
}
stub_canon=$(canonical "$stub")
if [ "$stub_canon" = "$(canonical "$config_file")" ] ||
   [ "$stub_canon" = "$(canonical "$settings_file")" ]; then
  die "REX_GASCITY_UI points at the user's settings or config file ($stub); choose another path"
fi
if [ -z "$uninstall" ] && [ -e "$stub" ] && ! grep -qF -- "$stub_marker" "$stub" 2>/dev/null; then
  die "$stub exists and is not a rex-gascity-ui stub; move it aside or set REX_GASCITY_UI to another path"
fi

marker_start='-- >>> rex-gascity-ui'
marker_end='-- <<< rex-gascity-ui'

# ---------------------------------------------------------------------------
# Version
# ---------------------------------------------------------------------------

version=
if [ -f "$version_file" ]; then
  version=$(tr -d '[:space:]' < "$version_file")
fi
[ -n "$version" ] || die "no ui/version beside this script"

# ---------------------------------------------------------------------------
# Uninstall: the stub and the block, nothing else
# ---------------------------------------------------------------------------

remove_block() { # remove_block <file>; 0 removed, 1 error, 2 no managed block
  _file=$1
  [ -f "$_file" ] || return 2
  if [ -L "$_file" ]; then
    warn "$_file is a symlink; remove the marked block where that file is generated"
    return 1
  fi
  grep -qF -- "$marker_start" "$_file" || return 2
  if ! grep -qF -- "$marker_end" "$_file"; then
    warn "$_file has the start of the rex-gascity-ui block but not its end; refusing to rewrite it"
    return 1
  fi
  _tmp=$(mktemp "$_file.gascity-ui.XXXXXX") || {
    warn "cannot create a temporary file beside $_file"
    return 1
  }
  # Seed the rewrite with the original's mode and owner. A fresh file would
  # take the umask's mode and the current user's ownership, which can make a
  # 0600 config readable by others.
  _mode=$(stat -c '%a' "$_file" 2>/dev/null || stat -f '%Lp' "$_file" 2>/dev/null || true)
  _owner=$(stat -c '%u:%g' "$_file" 2>/dev/null || stat -f '%u:%g' "$_file" 2>/dev/null || true)
  cp -p "$_file" "$_tmp" 2>/dev/null || cp "$_file" "$_tmp" 2>/dev/null || {
    rm -f "$_tmp"
    warn "cannot copy $_file"
    return 1
  }
  _awk_rc=0
  awk -v start="$marker_start" -v end="$marker_end" '
    index($0, start) == 1 {
      n = blank - 1
      while (n > 0) { print ""; n-- }
      blank = 0
      skipping = 1
      next
    }
    index($0, end) == 1 { skipping = 0; next }
    skipping { next }
    $0 == "" { blank++; next }
    { while (blank > 0) { print ""; blank-- } print }
    END {
      if (skipping) { exit 3 }
      while (blank > 0) { print ""; blank-- }
    }
  ' "$_file" > "$_tmp" || _awk_rc=$?
  if [ "$_awk_rc" -ne 0 ]; then
    rm -f "$_tmp"
    warn "cannot rewrite $_file: an unclosed rex-gascity-ui block"
    return 1
  fi
  [ -z "$_mode" ] || chmod "$_mode" "$_tmp" 2>/dev/null || true
  [ -z "$_owner" ] || chown "$_owner" "$_tmp" 2>/dev/null || true
  if command -v rex >/dev/null 2>&1; then
    if ! rex config check "$_tmp" >/dev/null 2>&1; then
      rm -f "$_tmp"
      warn "$_file still has the extension's block: the edited copy does not load cleanly, so nothing was changed"
      return 1
    fi
  fi
  if ! mv -f "$_tmp" "$_file"; then
    rm -f "$_tmp"
    warn "cannot replace $_file"
    return 1
  fi
  note "removed the rex-gascity-ui block from $_file"
}

remove_stub() { # remove_stub: delete only an installer-owned stub
  [ -e "$stub" ] || return 0
  if [ -L "$stub" ]; then
    warn "kept $stub: it is a symlink, not a rex-gascity-ui stub"
    return 1
  fi
  if ! grep -qF -- "$stub_marker" "$stub" 2>/dev/null; then
    warn "kept $stub: it is not a rex-gascity-ui stub"
    return 1
  fi
  rm -f "$stub" || { warn "cannot remove $stub"; return 1; }
  note "removed $stub"
}

if [ -n "$uninstall" ]; then
  status=0
  # The stub goes only after the config stops naming it: a symlinked or
  # otherwise unwritten config keeps its reference, so the stub must stay too.
  block_rc=0
  remove_block "$config_file" || block_rc=$?
  if [ "$block_rc" -eq 1 ]; then
    status=1
    if [ -e "$stub" ]; then
      warn "kept $stub: $config_file still loads it"
      warn "remove the rex-gascity-ui block by hand (or from the file that generates it), then delete $stub"
    fi
  elif [ "$block_rc" -eq 2 ]; then
    # No managed block. A hand-written dofile line may still load the stub.
    _stub_name=$(basename -- "$stub")
    _manual_ref=
    for _name in "$_stub_name" gascity-ui.lua; do
      if grep -qF -- "$_name" "$config_file" 2>/dev/null; then _manual_ref=1; break; fi
    done
    if [ -n "$_manual_ref" ] && [ -e "$stub" ]; then
      status=1
      warn "kept $stub: $config_file still loads it by hand"
      warn "remove the dofile line for $_stub_name from $config_file, then delete $stub"
    else
      remove_stub || status=1
    fi
  else
    remove_stub || status=1
  fi
  note "left the cache copy at $cache_root/$version/ui and the settings file gascity.lua alone"
  exit "$status"
fi

# ---------------------------------------------------------------------------
# rex is required: it is how detection sees what the config already loads
# ---------------------------------------------------------------------------

command -v rex >/dev/null 2>&1 ||
  die "the rex CLI is not on PATH; it is how this installer sees whether the extension is already loaded, so nothing was written"

installed_root=$cache_root/$version/ui

# ---------------------------------------------------------------------------
# Detection: what does the config load right now?
#
# One probe, `rex config check --json`: it loads the config file in the local
# rex binary and reports each action's `source`. The server's action list is
# deliberately not the probe — it goes stale until `rex config reload`.
# ---------------------------------------------------------------------------

probe_json=
probe_ok=
probe_err=
if [ ! -e "$config_file" ]; then
  # Nothing to probe: a config that does not exist loads nothing. The stub is
  # still installed, and the reference line is reported for the file to be
  # created from.
  probe_ok=1
  probe_json='{"actions":[]}'
elif probe_json=$(rex config check --json 2>&1); then
  probe_ok=1
else
  probe_err=$probe_json
  probe_json=
fi

# A config that does not load is not this installer's to repair: writing more
# into it would only make the complaint harder to find.
if [ -z "$probe_ok" ]; then
  [ -z "$probe_err" ] || printf '%s\n' "$probe_err" >&2
  die "the Rex config does not load, so nothing was written"
fi

live_root=
live_path=
loads=
if [ -n "$probe_ok" ]; then
  tmpdir=${TMPDIR:-/tmp}; tmpdir=${tmpdir%/}
  probe_file=$(mktemp "$tmpdir/gascity-ui-probe.XXXXXX") || die "cannot create a temporary file"
  printf '%s\n' "$probe_json" > "$probe_file"
  sources=$(
    awk '
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
    ' "$probe_file"
  )
  rm -f "$probe_file"
  first_src=$(
    printf '%s\n' "$sources" | awk -F'\t' 'NF { print $2; exit }'
  )
  if [ -n "$first_src" ]; then
    loads=1
    live_path=${first_src%:*}
    live_root=${live_path%%/lua/gascity/*}
    [ "$live_root" != "$live_path" ] || live_root=
    [ -n "$live_root" ] || die "the config loads a gascity_ action from an unexpected source: $live_path"
  fi
fi

case "$live_root" in
  "")
    : ;; # nothing loaded: install the stub, then try the reference block
  "$installed_root")
    if [ -n "$from_source" ]; then
      # --from-source refreshes the tree in place. The config already loads this
      # path, so a changed checkout must not be mistaken for a finished install.
      note "the extension is loaded from $installed_root (version $version); refreshing it from source"
    else
      note "the extension is already loaded from $installed_root (version $version)"
      note "nothing to do"
      exit 0
    fi
    ;;
  "$cache_root"/*/ui)
    note "the config loads version ${live_root#"$cache_root"/} (from $live_root); installing $version"
    ;;
  *)
    warn "the config already loads the extension from $live_root"
    warn "that is not this installer's root ($installed_root), so nothing was written and nothing was edited"
    warn "remove that load (a line in user.lua, or another init.lua block) to install this copy"
    exit 0
    ;;
esac

# ---------------------------------------------------------------------------
# Install the tree into the version-keyed cache
# ---------------------------------------------------------------------------

# stage_tree <source dir>: copy <source dir> into <cache>/<version>/ui atomically.
stage_tree() {
  _src=$1
  _parent=$cache_root/$version
  _staging=$_parent/.ui.$$
  _old=$_parent/.ui.old.$$
  mkdir -p "$_parent" || die "cannot create $_parent"
  rm -rf "$_staging"
  if ! cp -R "$_src/." "$_staging"; then
    rm -rf "$_staging"
    die "cannot stage the extension in $_parent"
  fi
  [ -f "$_staging/init.lua" ] || { rm -rf "$_staging"; die "the extension tree has no init.lua"; }
  _staged_version=$(tr -d '[:space:]' < "$_staging/version" 2>/dev/null || true)
  [ "$_staged_version" = "$version" ] ||
    { rm -rf "$_staging"; die "the extension tree declares version ${_staged_version:-none}, but $version was requested"; }
  rm -rf "$_old"
  if [ -d "$_parent/ui" ]; then
    mv "$_parent/ui" "$_old" || { rm -rf "$_staging"; die "cannot move the previous tree aside"; }
  fi
  if ! mv "$_staging" "$_parent/ui"; then
    [ -d "$_old" ] && mv "$_old" "$_parent/ui" || true
    rm -rf "$_staging"
    die "cannot move the staged tree into place"
  fi
  # Keep the previous tree until the new one loads. A checkout with a Lua error
  # must not replace a tree that works: the config that already loads this path
  # is the probe, so a broken candidate fails here and the old tree comes back.
  if [ -d "$_old" ] && [ -n "$loads" ] && [ "$live_root" = "$installed_root" ] &&
     command -v rex >/dev/null 2>&1; then
    if ! rex config check "$config_file" >/dev/null 2>&1; then
      rm -rf "$_parent/ui"
      mv "$_old" "$_parent/ui" || true
      die "the new extension tree does not load; the previous tree was restored"
    fi
  fi
  rm -rf "$_old"
}

sha256_of() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | cut -d' ' -f1
  else
    shasum -a 256 "$1" | cut -d' ' -f1
  fi
}

tmpdir=${TMPDIR:-/tmp}; tmpdir=${tmpdir%/}
work=$(mktemp -d "$tmpdir/gascity-ui.XXXXXX") || die "cannot create a temporary directory"
trap 'rm -rf "$work"' EXIT

tree_installed=
if [ -n "$from_source" ]; then
  stage_tree "$script_dir"
  note "installed the extension from $script_dir"
else
  [ -f "$installed_root/version" ] && [ "$(tr -d '[:space:]' < "$installed_root/version")" = "$version" ] &&
    [ -f "$installed_root/init.lua" ] && { note "the extension is already installed at $installed_root"; tree_installed=1; }

  if [ -z "$tree_installed" ]; then
    command -v tar >/dev/null 2>&1 || die "tar is not on PATH; install it, or use --from-source"

    base=https://github.com/$repo/releases/download/v$version
    asset=gascity-ui.tar.gz

    deadline=${GC_RUNTIME_INSTALL_DEADLINE:-}
    bounded=; have_sleep=
    if [ -n "$deadline" ]; then
      bounded=1
      case $deadline in ''|*[!0-9]*) deadline=120 ;; esac
      while :; do
        case $deadline in
          0?*) deadline=${deadline#0} ;;
          *) break ;;
        esac
      done
      [ -n "$deadline" ] || deadline=0
      command -v sleep >/dev/null 2>&1 && have_sleep=1
    fi
    if [ -n "$bounded" ]; then
      if fetch_start=$(date +%s 2>/dev/null) && [ -n "$fetch_start" ]; then
        :
      else
        warn "cannot read the clock; the fetch runs unbounded"
        bounded=; have_sleep=
      fi
    fi

    # run_bounded <secs> <cmd...>: reap the command after <secs>. Without a
    # watchdog it runs in the foreground. Returns 124 when reaped or when the
    # budget is already gone.
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

    remaining_budget() {
      if [ -z "$bounded" ]; then printf '%s' 0; return 0; fi
      _now=$(date +%s 2>/dev/null) || _now=
      case $_now in ''|*[!0-9]*) return 1 ;; esac
      _left=$(( fetch_start + deadline - _now ))
      [ "$_left" -gt 0 ] || return 1
      printf '%s' "$_left"
    }

    die_budget() {
      die "fetching $1 for release v$version exceeded the ${deadline}s budget (slow or stalled network); nothing was written"
    }

    # fetch_curl <file> <url>: 0 on HTTP 2xx, 1 on an HTTP error, 2 on a
    # transport failure, 124 on the watchdog.
    curl_code=000
    curl_transport=
    curl_msg=
    fetch_curl() {
      _codefile=$work/.code.$$
      _rc=0
      run_bounded "$_budget" curl -sSL -o "$1" -w '%{http_code}' "$2" 2>"$work/.curlerr" >"$_codefile" || _rc=$?
      if [ "$_rc" -eq 124 ]; then rm -f "$_codefile"; curl_code=000; return 124; fi
      _code=$(sed -n 's/^[[:space:]]*\([0-9][0-9][0-9]\)[[:space:]]*$/\1/p' "$_codefile" 2>/dev/null | sed -n '1p')
      rm -f "$_codefile"
      if [ "$_rc" -eq 0 ]; then
        curl_code=$_code
        case "$_code" in 2??) return 0 ;; esac
        return 1
      fi
      curl_transport=1
      curl_msg=$(sed -n '1p' "$work/.curlerr" 2>/dev/null)
      curl_code=000
      return 2
    }

    downloaded=
    curl_code=000
    curl_transport=
    curl_msg=
    failed_url=
    gh_failed=

    if command -v curl >/dev/null 2>&1; then
      _budget=$(remaining_budget) || die_budget checksums.txt
      _rc=0; fetch_curl "$work/checksums.txt" "$base/checksums.txt" || _rc=$?
      [ "$_rc" -eq 124 ] && die_budget checksums.txt
      if [ "$_rc" -eq 0 ]; then
        _budget=$(remaining_budget) || die_budget "$asset"
        _rc=0; fetch_curl "$work/$asset" "$base/$asset" || _rc=$?
        [ "$_rc" -eq 124 ] && die_budget "$asset"
        [ "$_rc" -eq 0 ] && downloaded=curl || failed_url=$base/$asset
      else
        failed_url=$base/checksums.txt
      fi
    fi
    if [ -z "$downloaded" ] && command -v gh >/dev/null 2>&1; then
      _budget=$(remaining_budget) || die_budget "$asset"
      _rc=0
      run_bounded "$_budget" gh release download "v$version" -R "$repo" \
        -p "$asset" -p checksums.txt --dir "$work" --clobber || _rc=$?
      [ "$_rc" -eq 124 ] && die_budget "$asset"
      if [ "$_rc" -eq 0 ]; then downloaded=gh; else gh_failed=1; fi
    fi
    [ -n "$downloaded" ] || {
      [ -z "$failed_url" ] || die "cannot fetch $failed_url for release v$version${curl_msg:+ (curl reported: $curl_msg)}${gh_failed:+ (the gh fallback also failed)}"
      die "cannot fetch $asset for release v$version: neither curl nor gh worked; install curl for a public release, or gh with GITHUB_TOKEN or GH_TOKEN for a private one"
    }

    expected=$(awk -v name="$asset" '$2 == name || $2 == "*" name { print $1; exit }' "$work/checksums.txt")
    [ -n "$expected" ] || die "checksums.txt for release v$version has no sha256 entry for $asset"
    actual=$(sha256_of "$work/$asset")
    [ "$actual" = "$expected" ] ||
      die "sha256 mismatch for $asset (release v$version): checksums.txt has $expected, the download has $actual"

    unpack=$work/unpack
    mkdir -p "$unpack" || die "cannot create $unpack"
    names=$work/names.txt
    tar -tzf "$work/$asset" > "$names" 2>/dev/null || die "$asset is not a readable tar.gz archive"
    grep -q '^ui/init\.lua$' "$names" || die "$asset does not contain ui/init.lua"
    if grep -Eq '(^|/)\.\.(/|$)|^/|^[A-Za-z]:' "$names"; then
      die "$asset contains an unsafe path; refusing to extract it"
    fi
    if grep -Ev '^ui(/|$)' "$names" | grep -q .; then
      die "$asset contains entries outside ui/; refusing to extract it"
    fi
    tar -xzf "$work/$asset" -C "$unpack" || die "cannot extract $asset"
    stage_tree "$unpack/ui"
    note "installed the extension for version $version from release v$version"
  fi
fi

# ---------------------------------------------------------------------------
# The load stub
# ---------------------------------------------------------------------------

mkdir -p "$(dirname "$stub")" || die "cannot create $(dirname "$stub")"
stub_staging=$(mktemp "$stub.XXXXXX") || die "cannot create a temporary file beside $stub"
# mktemp creates 0600. A plain redirection would have used the umask's mode,
# and the Rex server may run as another account, so restore that mode.
_umask=$(umask)
chmod "$(printf '%o' $(( 0666 & ~0$_umask )))" "$stub_staging" 2>/dev/null || true
{
  printf '%s\n' "-- rex-gascity-ui loader. Written by rex-gascity ui/install.sh."
  printf '%s\n' "-- Installed root: $installed_root. Version: $version. Delete this file to remove."
  printf '%s\n' "local root = os.getenv(\"REX_GASCITY_ROOT\") or \"$installed_root\""
  printf '%s\n' 'local entry = root .. "/init.lua"'
  printf '%s\n' 'local handle = io.open(entry, "r")'
  printf '%s\n' 'if handle == nil then'
  printf '\t%s\n' 'rex.log("warn", "gascity-ui: no extension at " .. entry)'
  printf '\t%s\n' 'return'
  printf '%s\n' 'end'
  printf '%s\n' 'handle:close()'
  printf '%s\n' 'dofile(entry)'
} > "$stub_staging" || die "cannot write $stub_staging"
if ! mv -f "$stub_staging" "$stub"; then
  rm -f "$stub_staging"
  die "cannot move the stub into place at $stub"
fi
note "wrote the load stub at $stub"

# ---------------------------------------------------------------------------
# The reference block in init.lua
# ---------------------------------------------------------------------------

report_manual_line() {
  warn "$config_file is not writable (${1:-cannot append to it})."
  warn "add one line where that file is generated:"
  warn "  dofile((os.getenv(\"REX_GASCITY_UI\") or (os.getenv(\"XDG_CONFIG_HOME\") or ((os.getenv(\"HOME\") or \"\") .. \"/.config\")) .. \"/rex/gascity-ui.lua\"))"
  warn "the stub is ready at $stub."
  warn "no other file was touched."
}

if [ -n "$loads" ] && [ "$live_root" = "$installed_root" ]; then
  note "the reference already loads the installed copy"
  exit 0
fi

if [ ! -e "$config_file" ]; then
  report_manual_line "it does not exist"
  exit 0
fi

if [ -L "$config_file" ]; then
  report_manual_line "it is a symlink to $(readlink "$config_file" 2>/dev/null || printf '?')"
  exit 0
fi

if [ ! -f "$config_file" ] || [ ! -w "$config_file" ]; then
  report_manual_line "it is not a writable regular file"
  exit 0
fi

if grep -qF -- "$marker_start" "$config_file"; then
  if grep -qF -- "$marker_end" "$config_file"; then
    note "the init.lua block is already present"
    exit 0
  fi
  warn "$config_file has the start of the rex-gascity-ui block but not its end"
  warn "repair the block by hand, or remove its lines and run this installer again"
  exit 0
fi

backup=$config_file.rex-gascity-ui.bak
if [ -e "$backup" ] && ! cmp -s "$backup" "$config_file"; then
  die "$backup already exists and differs from $config_file; move it aside, then run this installer again (nothing was changed)"
fi

before_size=$(wc -c < "$config_file" | tr -d ' ')
before_mtime=$(ls -ln "$config_file" 2>/dev/null | awk '{ print $6, $7, $8 }')
if [ ! -e "$backup" ]; then
  cp "$config_file" "$backup" || die "cannot write the backup $backup"
fi

after_size=$(wc -c < "$config_file" | tr -d ' ')
after_mtime=$(ls -ln "$config_file" 2>/dev/null | awk '{ print $6, $7, $8 }')
if [ "$before_size" != "$after_size" ] || [ "$before_mtime" != "$after_mtime" ]; then
  die "$config_file changed while this installer was reading it; nothing was appended"
fi

block=$work/block.lua
{
  printf '%s\n' "$marker_start (managed; do not edit this block)"
  printf '%s\n' 'local gascity_ui = os.getenv("REX_GASCITY_UI")'
  printf '\t%s\n' 'or (os.getenv("XDG_CONFIG_HOME") or ((os.getenv("HOME") or "") .. "/.config")) .. "/rex/gascity-ui.lua"'
  printf '%s\n' 'local gascity_ui_file = io.open(gascity_ui, "r")'
  printf '%s\n' 'if gascity_ui_file == nil then'
  printf '\t%s\n' 'rex.log("warn", "gascity-ui: no loader at " .. gascity_ui)'
  printf '%s\n' 'else'
  printf '\t%s\n' 'gascity_ui_file:close()'
  printf '\t%s\n' 'dofile(gascity_ui)'
  printf '%s\n' 'end'
  printf '%s\n' "$marker_end"
} > "$block" || die "cannot write the block"

# restore_backup <what happened>: the verified backup is the config's earlier
# bytes, whether this run made it or an earlier run did.
restore_backup() { # restore_backup <what happened>
  if cp "$backup" "$config_file" 2>/dev/null; then
    warn "$config_file $1; the backup was restored"
  else
    warn "$config_file $1, and the backup could not be restored"
    warn "restore it by hand from $backup"
  fi
}

if ! printf '\n' >> "$config_file" || ! cat "$block" >> "$config_file"; then
  restore_backup "could not take the block"
  die "cannot append the block to $config_file"
fi

# The appended block must load. `rex config check` reads the file without
# touching the running server, so a bad edit fails here instead of at the next
# restart.
if ! check_out=$(rex config check "$config_file" 2>&1); then
  restore_backup "does not load after the append"
  [ -z "$check_out" ] || warn "$check_out"
  die "the reference block was not added"
fi

note "added the rex-gascity-ui block to $config_file (backup: $backup)"
if rex actions --json >/dev/null 2>&1; then
  if rex actions --json 2>/dev/null | grep -q '"gascity_'; then
    note "a running server already sees the extension"
  else
    note "run 'rex config reload' so a running server picks the extension up"
  fi
fi
note "done: version $version is installed at $installed_root"
