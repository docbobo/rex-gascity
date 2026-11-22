#!/bin/sh
# Guard for the operator docs: run every command in a sh fence of
# docs/rex-runtime-pack.md and docs/rex-binary-install.md against a scratch city.
#
# The docs are the input, not the fixture: the fences are extracted and run in
# order. A command that cannot run is a docs bug, so this fails on it.
#
# The git remote is a local repository with two tags: the base version from
# pack/pack.toml, and the next patch version. The second tag proves that an
# exact pin ignores a newer release. The fixture pack sits at the repository
# root, because gc only parses /tree/ in github.com URLs. fake curl serves the
# release assets; fake gh fails, so the curl path is the one under test. No
# network, no Go toolchain.
#
# Usage: sh pack/tests/check-docs.sh
# Exits non-zero with a one-line reason on the first failed assertion.
set -eu

repo=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
readme=$repo/docs/rex-runtime-pack.md
guide=$repo/docs/rex-binary-install.md
pack=$repo/pack
# The docs fences pin the current pack version, so the fixture takes that tag
# from pack.toml and adds a newer tag too. A version bump must not need an edit
# here, and the newer tag must not be picked for an exact pin.
base=$(awk -F'"' '/^\[pack\]/{p=1;next} /^\[/{p=0} p && /^version[[:space:]]*=/{print $2; exit}' \
  "$pack/pack.toml")
[ -n "$base" ] || fail "pack/pack.toml declares no [pack] version"
last=${base##*.}
last=${last#"${last%%[!0]*}"}
next=${base%.*}.$(( ${last:-0} + 1 ))
pack_url=https://github.com/docbobo/rex-gascity/tree/main/pack

fail() {
  echo "check-docs: $*" >&2
  exit 1
}

[ -f "$readme" ] || fail "docs/rex-runtime-pack.md does not exist"
[ -f "$guide" ] || fail "docs/rex-binary-install.md does not exist"
command -v gc >/dev/null 2>&1 || fail "gc is not on PATH"
command -v git >/dev/null 2>&1 || fail "git is not on PATH"
# Only the gc and git binaries, never their directories: gc's directory may
# hold the developer's gc-runtime-rex, which install.sh must not see.
tool_path=/usr/bin:/bin:/usr/sbin:/sbin

tmpdir=${TMPDIR:-/tmp}; tmpdir=${tmpdir%/}
work=$(mktemp -d "$tmpdir/check-docs.XXXXXX")
trap 'rm -rf "$work"' EXIT

mkdir -p "$work/tools" "$work/releases" "$work/tree"
ln -s "$(command -v gc)" "$work/tools/gc"
ln -s "$(command -v git)" "$work/tools/git"

# --- fake tools -------------------------------------------------------------

cat > "$work/tools/curl" <<'FAKE'
#!/bin/sh
# fake curl: curl -sSL -o <out> -w <fmt> <url>, serving
# $FAKE_RELEASES/<tag>/<name>.
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

cat > "$work/tools/gh" <<'FAKE'
#!/bin/sh
# fake gh: the public curl path must work, so the fallback never succeeds.
echo 'gh: HTTP 404' >&2
exit 1
FAKE

cat > "$work/tools/rex" <<'FAKE'
#!/bin/sh
# stub rex CLI: the doctor check only resolves it and tests -x.
exit 0
FAKE

chmod 755 "$work/tools/curl" "$work/tools/gh" "$work/tools/rex"

# --- fixture repository -----------------------------------------------------

provider_asset() { # provider_asset <path> <version>
  cat > "$1" <<ASSET
#!/bin/sh
case "\$1" in
  protocol) printf '%s\n' '{"version":0,"capabilities":[]}' ;;
  version) printf '%s\n' '{"version":"$2"}' ;;
  *) exit 1 ;;
esac
ASSET
  chmod 755 "$1"
}

case "$(uname -s)" in
  Darwin) os=darwin ;;
  Linux) os=linux ;;
  *) fail "unsupported host OS: $(uname -s)" ;;
esac
case "$(uname -m)" in
  x86_64|amd64) arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
  *) fail "unsupported host machine: $(uname -m)" ;;
esac
asset=gc-runtime-rex_${os}_${arch}

release() { # release <version>
  version=$1
  dir=$work/releases/v$version
  mkdir -p "$dir"
  sed "s/^version = \".*\"/version = \"$version\"/" "$pack/pack.toml" > "$work/tree/pack.toml"
  cp "$pack/install.sh" "$work/tree/install.sh"
  rm -rf "$work/tree/doctor"
  cp -R "$pack/doctor" "$work/tree/doctor"
  rm -rf "$work/tree/bin"
  cp -R "$pack/bin" "$work/tree/bin"
  (
    cd "$work/tree" || exit 1
    git add -A
    git -c user.name=check-docs -c user.email=check-docs@example.invalid \
      commit -q -m "pack $version"
    git tag "v$version"
  ) || fail "cannot commit fixture pack $version"
  provider_asset "$dir/$asset" "$version"
  (
    cd "$dir" || exit 1
    if command -v sha256sum >/dev/null 2>&1; then
      sha256sum "$asset" > checksums.txt
    else
      shasum -a 256 "$asset" > checksums.txt
    fi
  ) || fail "cannot checksum release v$version"
}

git init -q "$work/tree" || fail "cannot init the fixture repository"
for version in "$base" "$next"; do
  release "$version"
done
remote=file://$work/tree

# --- fence extraction -------------------------------------------------------

extract() { # extract <doc> <name>
  awk -v out="$work/$2.list" '
    /^```/ {
      if (on) { print "```" >> out; on = 0; next }
      if ($0 ~ /^```sh[ \t]*$/) { on = 1 }
      next
    }
    on { print >> out }
  ' "$1"
}

split_fences() { # split_fences <name> -> prints the number of fences
  rm -rf "$work/$1"; mkdir -p "$work/$1"
  awk -v dir="$work/$1" '
    BEGIN { n = 0 }
    /^```$/ { n++; next }
    { print >> (dir "/" n ".sh") }
  ' "$work/$1.list"
  n=$(ls "$work/$1" | wc -l | tr -d ' ')
  printf '%s' "$n"
}

run_doc() { # run_doc <doc> <name> <minimum fences> [expect a hand-run install]
  doc=$1 name=$2 minimum=$3 manual=${4:-}
  : > "$work/$name.list"
  extract "$doc" "$name"
  grep -q '```' "$work/$name.list" || fail "$name has no sh fence"
  count=$(split_fences "$name")
  [ "$count" -ge "$minimum" ] ||
    fail "$name has $count sh fences, expected at least $minimum"

  city=$work/city-$name
  mkdir -p "$city/pack" "$city/home/.gc-city"
  printf '[city]
name = "%s"
' "$name" > "$city/city.toml"
  cp -R "$work/tree/doctor" "$work/tree/bin" "$work/tree/install.sh" \
    "$work/tree/pack.toml" "$city/pack/"

  i=0
  while [ "$i" -lt "$count" ]; do
    script=$work/$name/$i.sh
    if grep -q "$pack_url" "$script"; then
      sed "s|$pack_url|$remote|g" "$script" > "$script.run"
    else
      cp "$script" "$script.run"
    fi
    if ! ( cd "$city" && env -i \
      HOME="$city/home" \
      GC_HOME="$city/home/.gc-city" \
      PATH="$work/tools:$city/home/.local/bin:$tool_path" \
      FAKE_RELEASES="$work/releases" \
      TMPDIR="$tmpdir" \
      sh -eu "$script.run" ) > "$script.log" 2>&1; then
      echo "check-docs: $name fence $i failed:" >&2
      cat "$script.log" >&2
      exit 1
    fi
    i=$((i + 1))
  done

  # The flow's `gc doctor --fix` fence warms the version-keyed cache. gc runs
  # the provider through the pack's shipped bin/gc-runtime-rex shim, not PATH.
  # The fence env above leaves GC_RUNTIME_CACHE_DIR and XDG_CACHE_HOME unset, so
  # the root is $HOME/.cache. The last import fence pins $base exactly, so the
  # newer $next tag must be ignored.
  cached=$city/home/.cache/rex-gascity/$base/$os-$arch/gc-runtime-rex
  [ -x "$cached" ] || fail "$name left no provider in the version-keyed cache"
  "$cached" version | grep -q "\"version\":\"$base\"" ||
    fail "$name left a provider that is not $base"

  # A hand-run `sh pack/install.sh` fence installs onto PATH, not into the
  # cache: assert its target directly, or the fence could pass and leave no
  # binary where the doc says it goes.
  if [ -n "$manual" ]; then
    found=$(grep -c '^sh pack/install\.sh$' "$work/$name.list" || true)
    [ "$found" -eq 1 ] ||
      fail "$name has $found hand-run install fences, expected 1"
    want=$(awk -F'"' '/^\[pack\]/{p=1;next} /^\[/{p=0} p && /^version[[:space:]]*=/{print $2; exit}' \
      "$city/pack/pack.toml")
    installed=$city/home/.local/bin/gc-runtime-rex
    [ -x "$installed" ] || fail "$name manual install left no provider on PATH"
    "$installed" version | grep -q "\"version\":\"$want\"" ||
      fail "$name manual install left a provider that is not $want"
  fi
}

run_doc "$readme" readme 4
run_doc "$guide" guide 3 manual

echo "check-docs: ok"
