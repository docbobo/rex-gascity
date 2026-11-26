#!/bin/sh
# Guard for pack/mark-released.sh. Runs the script in a throwaway git repo
# (with a local bare origin) and a fake gh. No network, no real release.
#
# Usage: sh pack/tests/check-mark-released.sh
# Exits non-zero with a one-line reason on the first failed assertion.
set -eu

here=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
src=$(CDPATH= cd -- "$here/../.." && pwd)
script=$src/pack/mark-released.sh

fail() {
  echo "check-mark-released: $*" >&2
  exit 1
}

[ -f "$script" ] || fail "pack/mark-released.sh does not exist"
[ -x "$script" ] || fail "pack/mark-released.sh is not executable"

work=$(mktemp -d "${TMPDIR:-/tmp}/check-mark-released.XXXXXX")
trap 'rm -rf "$work"' EXIT INT TERM
state=$work/state
mkdir -p "$work/bin" "$state"

# --- fake gh ----------------------------------------------------------------
# `gh release view <tag> --json isDraft,isPrerelease` answers from a file named
# release-<tag> in $FAKE_GH. The file holds the two JSON booleans as
# "<isDraft> <isPrerelease>". No file means no such release. The latest-release
# lookup (`release view --json tagName`) answers from the file "latest".
# Every call must carry `--repo <owner/repo>`, so the check proves the script
# pins the lookup instead of trusting GH_REPO or the caller's directory. Every
# call is logged, so the check can prove the script never creates a release.
cat > "$work/bin/gh" <<'FAKEGH'
#!/bin/sh
printf '%s\n' "$*" >> "$FAKE_GH/calls.log"
[ "$1 $2" = "release view" ] || { echo "fake gh: unexpected call: $*" >&2; exit 99; }
repo=
prev=
for a in "$@"; do
  [ "$prev" = "--repo" ] && repo=$a
  prev=$a
done
[ -n "$repo" ] || { echo "fake gh: missing --repo" >&2; exit 1; }
if [ "$3" = "--json" ]; then
  [ -f "$FAKE_GH/latest" ] || { echo "release not found" >&2; exit 1; }
  printf '{"tagName": "%s"}\n' "$(cat "$FAKE_GH/latest")"
  exit 0
fi
f=$FAKE_GH/release-$3
[ -f "$f" ] || { echo "release not found" >&2; exit 1; }
read -r draft pre < "$f"
printf '{"isDraft": %s, "isPrerelease": %s}\n' "$draft" "$pre"
FAKEGH
chmod 0755 "$work/bin/gh"

# --- throwaway repo ---------------------------------------------------------
# Copy the script and write a fixed README in the release-target state
# (pack.toml ahead of the last release), which is the state the script exists
# to fix. Do not copy the repository README: a real run changes that copy, and
# the dry-run assertions would then fail. The lines below exercise every
# rewrite rule.
repo=$work/repo
origin=$work/origin.git
git init -q --bare -b main "$origin"
git init -q -b main "$repo"
git -C "$repo" config user.email check@example.test
git -C "$repo" config user.name check
git -C "$repo" config commit.gpgsign false
git -C "$repo" remote add origin "$origin"
mkdir -p "$repo/pack" "$repo/ui"
cp "$script" "$repo/pack/mark-released.sh"
cat > "$repo/README.md" <<'README'
# rex-gascity

## Status: release target, v2026.11.05

Version `2026.11.05` (tag `v2026.11.05`) is the target version. The tag is not published yet. The provider is self-installing.

CI runs `gc runtime check` and `gc runtime conformance` with no failed requirement. Import it from this repository by tag, once that tag is published.

### Import by tag

It installs the
matching release binary. The tag-based commands in this section are usable
after `v2026.11.05` is published.

```sh
gc import add https://github.com/docbobo/rex-gascity//pack --name rex --version 2026.11.05
```

`pack/pack.toml` declares `version = "2026.11.05"`, and the release tag is
`v2026.11.05`. `--version` takes the bare version, so the tag must be pushed
first.

constraint. The constraint must allow more than one tag: `--version '^2026.11.05'`.
README
printf '[pack]\nversion = "2026.11.05"\n' > "$repo/pack/pack.toml"
printf '2026.11.05\n' > "$repo/ui/version"
git -C "$repo" add -A
git -C "$repo" commit -q -m seed
git -C "$repo" push -q origin main

mark() {
  # mark <args...>: run the script inside the throwaway repo with the fake gh.
  PATH="$work/bin:$PATH" FAKE_GH=$state GH=gh sh "$repo/pack/mark-released.sh" "$@"
}

head_sha() { git -C "$repo" rev-parse HEAD; }
origin_sha() { git -C "$origin" rev-parse main; }

# A published release and a few that must be refused.
echo v2026.11.05 > "$state/latest"
echo "false false" > "$state/release-v2026.11.05"
echo "false false" > "$state/release-v2026.11.06"
echo "true false"  > "$state/release-v2026.11.07"
echo "false true"  > "$state/release-v2026.11.08"

readme_before=$(cat "$repo/README.md")
seed=$(head_sha)

# --- argument and release gates: refuse, change nothing ---------------------
if mark >/dev/null 2>&1; then fail "no tag argument was accepted"; fi
if mark 2026.11.05 >/dev/null 2>&1; then fail "a bare version without v was accepted"; fi
if mark v2026.11.05 extra >/dev/null 2>&1; then fail "a second positional argument was accepted"; fi
if mark v2026.11.99 >/dev/null 2>&1; then fail "a tag with no release was accepted"; fi
if mark v2026.11.07 >/dev/null 2>&1; then fail "a draft release was accepted"; fi
if mark v2026.11.08 >/dev/null 2>&1; then fail "a prerelease was accepted"; fi
[ "$(cat "$repo/README.md")" = "$readme_before" ] || fail "a refused run changed README.md"
[ "$(head_sha)" = "$seed" ] || fail "a refused run committed"

# --- dry run: prints the diff, changes nothing ------------------------------
out=$(mark v2026.11.05 --dry-run 2>&1) || fail "--dry-run failed: $out"
printf '%s\n' "$out" | grep -q '^-## Status: release target, v2026.11.05$' ||
  fail "--dry-run did not print the heading diff"
printf '%s\n' "$out" | grep -q '^+## Status: released, v2026.11.05$' ||
  fail "--dry-run did not print the new heading"
[ "$(cat "$repo/README.md")" = "$readme_before" ] || fail "--dry-run changed README.md"
[ "$(head_sha)" = "$seed" ] || fail "--dry-run committed"
[ "$(origin_sha)" = "$seed" ] || fail "--dry-run pushed"

# --- the lookup is pinned to the repo, not GH_REPO or the caller's directory --
slug=$(basename "$work")/origin
out=$(GH_REPO=other/repo mark v2026.11.05 --dry-run 2>&1) ||
  fail "a GH_REPO from another repository broke the pinned lookup: $out"
grep -q -- "--repo $slug" "$state/calls.log" || fail "gh was not pinned with --repo $slug"
grep -q 'other/repo' "$state/calls.log" && fail "the lookup used GH_REPO"

# --- a local commit that is not on origin is refused -----------------------
echo local > "$repo/local-only.txt"
git -C "$repo" add local-only.txt
git -C "$repo" commit -q -m "local only"
if mark v2026.11.05 >/dev/null 2>&1; then
  fail "a local commit that is not on origin was accepted"
fi
git -C "$repo" reset -q --hard origin/main
[ "$(head_sha)" = "$seed" ] || fail "the local-only reset did not return to the seed"
[ "$(cat "$repo/README.md")" = "$readme_before" ] || fail "the local-only refusal changed README.md"

# --- real run: marks v2026.11.05 released -----------------------------------
mark v2026.11.05 >/dev/null 2>&1 || fail "real run for a published release failed"

readme=$repo/README.md
grep -q '^## Status: released, v2026.11.05$' "$readme" || fail "heading not rewritten"
grep -q 'Version `2026.11.05` (tag `v2026.11.05`) is the current release\.' "$readme" ||
  fail "current-release paragraph not rewritten"
grep -q 'target version\|not published yet\|once that tag is published\|is published\.$' "$readme" &&
  fail "release-target wording survived"
grep -q -- "--version 2026.11.05\$" "$readme" || fail "bare --version pin not rewritten"
grep -q -- "--version '\^2026.11.05'" "$readme" || fail "caret constraint not rewritten"
grep -q 'declares `version = "2026.11.05"`, and the release tag is$' "$readme" ||
  fail "pack.toml declaration line not rewritten"
grep -q '^`v2026.11.05`\. `--version` takes the bare version' "$readme" ||
  fail "release tag line not rewritten"

# Only README.md changed, with one commit that carries the documented subject.
changed=$(git -C "$repo" diff --name-only "$seed" HEAD)
[ "$changed" = "README.md" ] || fail "the commit touched more than README.md: $changed"
[ "$(git -C "$repo" rev-list --count "$seed..HEAD")" = 1 ] || fail "expected exactly one commit"
[ "$(git -C "$repo" log -1 --format=%s)" = "docs: mark v2026.11.05 released" ] ||
  fail "commit subject is not 'docs: mark v2026.11.05 released'"
[ "$(origin_sha)" = "$(head_sha)" ] || fail "the commit was not pushed to origin main"
[ "$(cat "$repo/pack/pack.toml")" = "$(printf '[pack]\nversion = "2026.11.05"')" ] ||
  fail "pack/pack.toml changed"
[ "$(cat "$repo/ui/version")" = "2026.11.05" ] || fail "ui/version changed"

# --- idempotent: a second run is a no-op ------------------------------------
after=$(head_sha)
mark v2026.11.05 >/dev/null 2>&1 || fail "a repeated run failed"
[ "$(head_sha)" = "$after" ] || fail "a repeated run committed again"
[ "$(origin_sha)" = "$after" ] || fail "a repeated run pushed"

# --- an older published tag cannot move the pins backward -------------------
echo v2026.11.06 > "$state/latest"
if mark v2026.11.05 >/dev/null 2>&1; then fail "an older tag moved the README pins backward"; fi
[ "$(head_sha)" = "$after" ] || fail "the older-tag refusal committed"
grep -q '^## Status: released, v2026.11.05$' "$readme" || fail "the older-tag refusal changed README.md"

# --- released -> released: the next release moves every pin -----------------
mark v2026.11.06 >/dev/null 2>&1 || fail "marking the next release failed"
grep -q '^## Status: released, v2026.11.06$' "$readme" || fail "heading did not move to the next release"
grep -q '2026\.11\.05' "$readme" && fail "an old release pin survived the move"
grep -q -- '--version 2026\.11\.06$' "$readme" || fail "the bare --version pin did not move"
grep -q -- "--version '\^2026.11.06'" "$readme" || fail "caret constraint did not move"

# --- the script never tags, builds or publishes -----------------------------
if grep -q -E '^release (create|edit|upload|delete)' "$state/calls.log"; then
  fail "the script modified a release"
fi
if [ -n "$(git -C "$repo" tag)" ]; then fail "the script created a tag"; fi

echo "check-mark-released: ok"
