#!/bin/sh
# Guard for ui/install.sh.
#
# Runs the installer against a fake `rex` that emulates the one thing the
# installer reads: `rex config check --json`, which reports each action's
# `source`. The fake resolves what the config actually loads (the managed block,
# then the stub, then the root the stub names), so detection is tested for real
# without a Rex server or a Lua interpreter.
#
# No network: the fetch cases serve a local tarball through a fake curl.
#
# Usage: sh pack/tests/check-ui-installer.sh
# Exits non-zero with a one-line reason on the first failed assertion.
set -eu

repo=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
installer=$repo/ui/install.sh
[ -f "$installer" ] || { echo "check-ui-installer: $installer does not exist" >&2; exit 1; }

version=$(tr -d '[:space:]' < "$repo/ui/version")
[ -n "$version" ] || { echo "check-ui-installer: ui/version is empty" >&2; exit 1; }

fail() {
  echo "check-ui-installer: $*" >&2
  exit 1
}

tmpdir=${TMPDIR:-/tmp}; tmpdir=${tmpdir%/}
work=$(mktemp -d "$tmpdir/check-ui-installer.XXXXXX")
trap 'rm -rf "$work"' EXIT

tool_path=/usr/bin:/bin:/usr/sbin:/sbin

mkdir -p "$work/tools"

# --- fake rex ---------------------------------------------------------------

cat > "$work/tools/rex" <<'FAKE'
#!/bin/sh
# fake rex: `config check [--json] [PATH]` only, enough for the UI installer.
# It reads the config the way Rex does: the managed block loads the stub, the
# stub names the root, the root's lua/gascity/actions.lua registers the actions.
cfg=${REX_CONFIG:-$HOME/.config/rex/init.lua}
json=
sub=$1
shift
case "$sub" in
  config) ;;
  actions)
    # The installer's post-write hint. A stub server sees nothing.
    printf '%s\n' '{"actions":[{"name":"other","source":"init.lua:1"}]}'
    exit 0 ;;
  version) printf '%s\n' '{"rex_version":"0.1.0"}'; exit 0 ;;
  *) echo "fake rex: unsupported: $sub $*" >&2; exit 2 ;;
esac
[ "${1:-}" = check ] || { echo "fake rex: unsupported: $sub $*" >&2; exit 2; }
shift
for arg in "$@"; do
  case "$arg" in
    --json) json=1 ;;
    -*) ;;
    *) cfg=$arg ;;
  esac
done
[ -f "$cfg" ] || { echo "config $cfg: no such file" >&2; exit 1; }
if grep -q 'FAKE-REX-BROKEN' "$cfg"; then
  echo "$cfg:2: unexpected symbol near ')'" >&2
  exit 1
fi

root=
if grep -qF -- '-- >>> rex-gascity-ui' "$cfg"; then
  stub=${REX_GASCITY_UI:-${XDG_CONFIG_HOME:-$HOME/.config}/rex/gascity-ui.lua}
  if [ -f "$stub" ]; then
    root=$(sed -n 's/^local root = os.getenv("REX_GASCITY_ROOT") or "\(.*\)"$/\1/p' "$stub" | head -n 1)
  fi
elif [ -n "${REX_GASCITY_ROOT:-}" ]; then
  root=$REX_GASCITY_ROOT
fi

if [ -n "$json" ]; then
  if [ -n "$root" ]; then
    printf '{"path":"%s","exists":true,"bindings":1,"removals":0,"modes":0,"actions":[\n' "$cfg"
    printf '{"name":"other","kind":"lua","source":"init.lua:1"},\n'
    printf '{"name":"gascity_mail_mayor","kind":"lua","source":"%s/lua/gascity/actions.lua:296"}\n' "$root"
    printf ']}\n'
  else
    printf '{"path":"%s","exists":true,"bindings":1,"removals":0,"modes":0,"actions":[\n' "$cfg"
    printf '{"name":"other","kind":"lua","source":"init.lua:1"}\n'
    printf ']}\n'
  fi
  exit 0
fi
if [ -n "$root" ]; then
  printf '%s: the UI loads from %s\n' "$cfg" "$root"
else
  printf '%s: loads 1 action\n' "$cfg"
fi
FAKE

# fake curl: curl -sSL -o <out> -w <fmt> <url>, serving $FAKE_RELEASES/<tag>/<name>.
cat > "$work/tools/curl" <<'FAKE'
#!/bin/sh
out= url=
while [ $# -gt 0 ]; do
  case "$1" in
    -o) out=$2; shift 2 ;;
    -w|--retry) shift 2 ;;
    -*) shift ;;
    *) url=$1; shift ;;
  esac
done
case "$url" in
  */releases/download/*) ;;
  *) printf '%s' 404; exit 22 ;;
esac
rest=${url#*/releases/download/}
tag=${rest%%/*}
name=${rest#*/}
dir=${FAKE_RELEASES:?}/$tag
[ -f "$dir/$name" ] || { printf '%s' 404; exit 22; }
cp "$dir/$name" "$out" || exit 1
printf '%s' 200
FAKE

chmod 755 "$work/tools/rex" "$work/tools/curl"

# --- fixture release --------------------------------------------------------

releases=$work/releases/v$version
mkdir -p "$releases"
tar -czf "$releases/gascity-ui.tar.gz" -C "$repo" ui || fail "cannot build the fixture tarball"
(
  cd "$releases" || exit 1
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum gascity-ui.tar.gz > checksums.txt
  else
    shasum -a 256 gascity-ui.tar.gz > checksums.txt
  fi
) || fail "cannot checksum the fixture tarball"

# --- harness ----------------------------------------------------------------

case_no=0
new_case() { # new_case <name>
  case_no=$((case_no + 1))
  name=$1
  home=$work/case-$case_no-home
  mkdir -p "$home/.config/rex"
  stub=$home/.config/rex/gascity-ui.lua
  cfg=$home/.config/rex/init.lua
  installed=$home/.cache/rex-gascity/$version/ui
  case_releases=
}

run_install() { # run_install [args...]
  env -i \
    HOME="$home" \
    PATH="$work/tools:$tool_path" \
    TMPDIR="$tmpdir" \
    FAKE_RELEASES="${case_releases:-$work/releases}" \
    ${REX_GASCITY_ROOT:+REX_GASCITY_ROOT="$REX_GASCITY_ROOT"} \
    sh "$installer" "$@"
}

run_install_quiet() { run_install "$@" > "$work/out.txt" 2>&1; }

expect_fail() { # expect_fail <args...>
  if run_install_quiet "$@"; then
    cat "$work/out.txt" >&2
    fail "$name: the installer succeeded where it must fail"
  fi
}

expect_ok() {
  run_install_quiet "$@" || { cat "$work/out.txt" >&2; fail "$name: the installer failed"; }
}

count_block() { # count_block <file>
  grep -cF -- '-- >>> rex-gascity-ui' "$1" 2>/dev/null || true
}

# --- case 1: fresh install writes the stub and the block ---------------------

new_case fresh
printf '%s\n' '-- existing config' 'rex.bind("x", function() end)' > "$cfg"
expect_ok
[ -f "$stub" ] || fail "$name: no stub at $stub"
[ -f "$installed/init.lua" ] || fail "$name: no tree at $installed"
[ -f "$installed/version" ] || fail "$name: no version file in the tree"
grep -qF -- '-- >>> rex-gascity-ui' "$cfg" || fail "$name: no block in init.lua"
grep -qF -- '-- <<< rex-gascity-ui' "$cfg" || fail "$name: the block is not closed"
grep -qF 'rex.bind("x", function() end)' "$cfg" || fail "$name: the existing config was not preserved"
# The stub names the installed root, and nothing else writes there.
grep -qF "$installed" "$stub" || fail "$name: the stub does not name $installed"
[ -f "$cfg.rex-gascity-ui.bak" ] || fail "$name: no backup was written"

# --- case 2: a second run changes nothing -----------------------------------

new_case idempotent
printf '%s\n' '-- existing config' > "$cfg"
expect_ok
before_cfg=$(cat "$cfg")
before_stub=$(cat "$stub")
expect_ok
[ "$(cat "$cfg")" = "$before_cfg" ] || fail "$name: the config changed on the second run"
[ "$(cat "$stub")" = "$before_stub" ] || fail "$name: the stub changed on the second run"
[ "$(count_block "$cfg")" = 1 ] || fail "$name: the block appears $(count_block "$cfg") times"

# --- case 3: a block that is already present is left alone -------------------

new_case preexisting-block
printf '%s\n' '-- existing config' '-- >>> rex-gascity-ui (managed; do not edit this block)' '-- <<< rex-gascity-ui' > "$cfg"
expect_ok
[ "$(count_block "$cfg")" = 1 ] || fail "$name: the block was duplicated"

# --- case 4: a read-only init.lua places the stub and reports the line -------

new_case read-only
printf '%s\n' '-- existing config' > "$cfg"
chmod 0444 "$cfg"
expect_ok
chmod 0644 "$cfg"
[ -f "$stub" ] || fail "$name: the stub was not written"
[ "$(count_block "$cfg")" = 0 ] || fail "$name: the read-only config was written"
grep -q 'add one line where that file is generated' "$work/out.txt" ||
  fail "$name: the manual line was not reported"
grep -q 'no other file was touched' "$work/out.txt" || fail "$name: the warning is incomplete"

# --- case 5: a symlinked init.lua is never replaced -------------------------

new_case symlink
real=$home/real-init.lua
printf '%s\n' '-- generated elsewhere' > "$real"
ln -s "$real" "$cfg"
expect_ok
[ -L "$cfg" ] || fail "$name: the symlink was replaced"
if grep -qF -- '-- >>> rex-gascity-ui' "$real"; then
  fail "$name: the symlink target was written"
fi
[ -f "$stub" ] || fail "$name: the stub was not written"
grep -q 'symlink' "$work/out.txt" || fail "$name: the symlink was not named"

# --- case 6: a config from another root is left completely alone ------------

new_case other-root
printf '%s\n' '-- existing config' > "$cfg"
other=$work/case-6-other-copy
mkdir -p "$other/lua/gascity"
REX_GASCITY_ROOT=$other expect_ok
unset REX_GASCITY_ROOT
[ -e "$stub" ] && fail "$name: the stub was written for a foreign load"
[ "$(count_block "$cfg")" = 0 ] || fail "$name: the foreign config was written"
grep -qF "$other" "$work/out.txt" || fail "$name: the live root was not reported"

# --- case 7: a config that does not load is left untouched ------------------

new_case broken-config
printf '%s\n' '-- FAKE-REX-BROKEN' > "$cfg"
expect_fail
[ "$(count_block "$cfg")" = 0 ] || fail "$name: the broken config was written"
[ -e "$stub" ] && fail "$name: the stub was written into a broken config"
grep -q 'does not load' "$work/out.txt" || fail "$name: the complaint was not reported"

# --- case 8: no init.lua at all places the stub and reports the line --------

new_case no-config
expect_ok
[ -f "$stub" ] || fail "$name: the stub was not written"
grep -q 'does not exist' "$work/out.txt" || fail "$name: the missing config was not named"

# --- case 9: uninstall removes the block and the stub, nothing else ---------

new_case uninstall
printf '%s\n' 'rex.bind("keep", function() end)' > "$cfg"
expect_ok
printf '%s\n' '-- gascity settings' > "$home/.config/rex/gascity.lua"
expect_ok --uninstall
[ "$(count_block "$cfg")" = 0 ] || fail "$name: the block survived"
grep -qF 'rex.bind("keep", function() end)' "$cfg" || fail "$name: the rest of the config was lost"
[ -e "$stub" ] && fail "$name: the stub survived"
[ -f "$installed/init.lua" ] || fail "$name: the cache copy was removed"
[ -f "$home/.config/rex/gascity.lua" ] || fail "$name: the settings file was removed"

# --- case 10: fetch install, with checksums --------------------------------

new_case fetch
printf '%s\n' '-- existing config' > "$cfg"
expect_ok
[ -f "$installed/init.lua" ] || fail "$name: the fetched tree is missing"
grep -qF -- '-- >>> rex-gascity-ui' "$cfg" || fail "$name: no block after a fetch"
[ -f "$work/tools/curl" ] || fail "the fixture lost its fake curl"
grep -q 'from release v' "$work/out.txt" || fail "$name: the fetch was not reported"

# --- case 11: a sha256 mismatch is refused ---------------------------------

new_case bad-checksum
printf '%s\n' '-- existing config' > "$cfg"
printf '%s\n' '0000000000000000000000000000000000000000000000000000000000000000  gascity-ui.tar.gz' > "$releases/checksums.txt"
expect_fail
printf '%s\n' "$(cat "$releases/checksums.txt")" >/dev/null # keep sh quiet about the next rewrite
(
  cd "$releases" || exit 1
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum gascity-ui.tar.gz > checksums.txt
  else
    shasum -a 256 gascity-ui.tar.gz > checksums.txt
  fi
)
[ -e "$installed" ] && fail "$name: a mismatched download was installed"
[ -e "$stub" ] && fail "$name: a mismatched download wrote the stub"
grep -q 'sha256 mismatch' "$work/out.txt" || fail "$name: the mismatch was not named"

# --- case 12: a tarball copy uses the same cache root as the pack helper -----

new_case mirror
mkdir -p "$work/case-12-unpack"
tar -xzf "$releases/gascity-ui.tar.gz" -C "$work/case-12-unpack" || fail "$name: cannot extract the fixture"
[ -f "$work/case-12-unpack/ui/install.sh" ] || fail "$name: the tarball has no installer"
mkdir -p "$home/.config/rex"
printf '%s\n' '-- existing config' > "$cfg"
env -i HOME="$home" PATH="$work/tools:$tool_path" TMPDIR="$tmpdir" \
  sh "$work/case-12-unpack/ui/install.sh" --from-source > "$work/out.txt" 2>&1 ||
  { cat "$work/out.txt" >&2; fail "$name: the tarball installer failed"; }
[ -f "$home/.cache/rex-gascity/$version/ui/init.lua" ] ||
  fail "$name: the tarball copy did not use the mirrored cache root"
for override in GC_RUNTIME_CACHE_DIR=$work/custom-cache XDG_CACHE_HOME=$work/xdg-cache; do
  key=${override%%=*}; val=${override#*=}
  with_lib=$(env -i HOME="$home" "$key=$val" sh -c ". '$repo/pack/bin/_lib.sh'; gc_runtime_cache_root") ||
    fail "$name: the pack helper failed for $key"
  mirrored=$(env -i HOME="$home" "$key=$val" sh -c "$(sed -n '/^[[:space:]]*gc_runtime_cache_root() {/,/^[[:space:]]*}/p' "$work/case-12-unpack/ui/install.sh"); gc_runtime_cache_root") ||
    fail "$name: the mirrored helper failed for $key"
  [ "$with_lib" = "$mirrored" ] ||
    fail "$name: the mirrored cache root is $mirrored, but the pack helper says $with_lib for $key"
done

# --- case 13: a release whose tree declares another version is refused ------

new_case tarball-version-mismatch
printf '%s\n' '-- existing config' > "$cfg"
alt=$work/releases-alt/v$version
mkdir -p "$alt" "$work/alt-src/ui"
cp -R "$repo/ui/." "$work/alt-src/ui/"
printf '%s\n' '0.0.0' > "$work/alt-src/ui/version"
tar -czf "$alt/gascity-ui.tar.gz" -C "$work/alt-src" ui || fail "$name: cannot build the alternate tarball"
(
  cd "$alt" || exit 1
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum gascity-ui.tar.gz > checksums.txt
  else
    shasum -a 256 gascity-ui.tar.gz > checksums.txt
  fi
) || fail "$name: cannot checksum the alternate tarball"
case_releases=$work/releases-alt
expect_fail
case_releases=
[ -e "$stub" ] && fail "$name: a mismatched release wrote the stub"
[ -e "$installed" ] && fail "$name: a mismatched release was installed"
grep -q 'declares version' "$work/out.txt" || fail "$name: the mismatch was not named"

echo "check-ui-installer: ok ($case_no cases)"
