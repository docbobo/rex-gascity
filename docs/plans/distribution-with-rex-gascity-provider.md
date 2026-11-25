# Distributing the Gas City UI with the rex-gascity provider

**Status:** plan for review. No implementation in this change.

The UI is a Rex Lua extension today: a git checkout loaded by a hand-written
block in `~/.config/rex/user.lua`. The provider (`docbobo/rex-gascity`) is a gc
runtime pack that installs itself. The goal is one extension that installs
alone, on a bare Rex, or as part of the provider, from one source of truth.

This document records the decisions, the verified facts behind them, the
mechanics, and the open questions. It moves to `rex-gascity/docs/plans/` with
step 1 of the rollout.

## Decisions

| # | Decision |
|---|---|
| D1 | One repository, one tag, one host-side installer. The provider's install reaches the same installer, so the UI needs no second release pipeline. |
| D2 | `init.lua` holds the *reference*. The extension itself is loaded through a separate load stub, so upgrading the UI never rewrites `init.lua` again. |
| D3 | Detection comes first, and it asks `rex config check --json` — the config file as it is now. The server's action list goes stale until `rex config reload`, so it is never the probe. Nothing is written when `init.lua`, or any dependency it loads, already loads the extension. |
| D4 | `init.lua` is the **only** file ever edited. Never a dependency of it: another extension may own that file. |
| D5 | When `init.lua` cannot be written: place the stub, do not edit anything, warn, and print the exact line to add. |
| D6 | The stub is `~/.config/rex/gascity-ui.lua`. `~/.config/rex/gascity.lua` stays the extension's *settings* file and is never written. |
| D7 | A stub that needs one manual line is a **warning**, not an error. In gc doctor terms that is exit 1, and `--fix` cannot resolve it. |
| D8 | The UI moves into `rex-gascity/ui/` as a plain copy. History is not preserved; the old repository is archived. |
| D9 | The fix fetches `gascity-ui.tar.gz` from the release. `pack/` stays a runtime pack and carries no Lua. |
| D10 | macOS and Linux only, like the provider. No Windows support. |

## Verified facts

Facts are commands run on the development machine (macOS, rex
`0.1.0-preview.23`, commit `c67cc2d0c88ecc86f5d751079b809dd9e9047636`) unless a
source is named.

- **F1 — load provenance is observable.** `rex actions --json` returns actions
  from the running server, each with a `source` path. The UI's actions report
  `/Volumes/Sources/rex-gascity-ui/lua/gascity/actions.lua:296`; `init.lua`'s
  own actions report `init.lua:26`. A `required` module reports its absolute
  path, the file loaded by `dofile` reports the loaded file's spelling.
- **F2 — the same is observable without a server.** `rex config check --json`
  loads the config in the local `rex` binary and returns the same `source`
  field. `rex config check [PATH]` takes a path, and the file is `REX_CONFIG`
  when that variable is set (`rex config --help`).
- **F3 — static scanning would not be enough.** `~/.config/rex/user.lua`
  computes its `dofile` target at run time:
  `root = os.getenv("REX_GASCITY_ROOT") or "/Volumes/Sources/rex-gascity-ui"`,
  then `dofile(root .. "/init.lua")`. A scanner for literal `dofile("…")`
  calls misses this. F1/F2 see through it because they report what was
  actually registered.
- **F4 — the development machine's `init.lua` is not writable.**
  `~/.config/rex/init.lua` is a symlink to
  `/nix/store/k5kgpqxamqn8y08b1hzzlc5j5r022j7j-hm_init.lua`, a root-owned 0444
  file. The unwritable case is the normal case here, not an edge case.
- **F5 — that `init.lua` loads a dependency only when it exists.** Its last
  lines `dofile` `$XDG_CONFIG_HOME/rex/user.lua` when that file is present.
  So "already loaded" is achievable on this machine through a dependency, and
  detection must see it (D3) or the installer warns where it should not.
- **F6 — provider install mechanics** (`docs/rex-runtime-pack.md`):
  `gc import add …/rex-gascity//pack --name rex --version <bare version>` pins
  the tag `v<version>`; `gc import install` writes the pack files;
  `gc doctor --check rex-gascity-runtime:rex-binary --fix` runs
  `pack/doctor/rex-binary/fix.sh` on the host; the provider binary lands in
  `${GC_RUNTIME_CACHE_DIR:-${XDG_CACHE_HOME:-$HOME/.cache}/rex-gascity}/<version>/<os>-<arch>`;
  **gc rewrites the pack directory on install**, so a loader must not point
  into the pack directory.
- **F7 — doctor protocol** (`pack/doctor/rex-binary/run.sh`): exit 0 OK, 1
  warning, 2 error; the first stdout line is the message, later lines are
  detail; a check is read-only and never writes state.
- **F8 — doc commands are proven.** `pack/tests/check-docs.sh` extracts every
  `sh` fence from `docs/rex-runtime-pack.md` and `docs/rex-binary-install.md`
  and runs them in a scratch city with fake `curl`, fake `gh` and a stub `rex`
  that answers nothing but `exit 0`. A new fence is therefore a new test
  obligation.
- **F9 — the UI has no version today.** `git tag` in this repository is empty
  and no module carries a version. Version identity has to be introduced.
- **F10 — the local runtime** (`pack/README.md`): the pack is imported per
  city, the Rex config file is per account. The UI is global; the pack is not.
  Any install that runs from a pack must therefore be idempotent and
  last-writer-wins, and must not assume one city.

## Target layout

```text
rex-gascity/
  ui/
    init.lua              the extension entry point (today's init.lua)
    lua/gascity/*.lua     unchanged modules
    tests/                unchanged, plus installer tests
    version               equals [pack] version; enforced by a pack check
    install.sh            the single host-side installer (fetch or --from-source)
    README.md
    AGENTS.md             test command, version rule, wiring rules
  pack/
    pack.toml             [pack] version, unchanged in shape
    doctor/rex-ui/
      doctor.toml
      run.sh              read-only check
      fix.sh              calls ui/install.sh
    tests/check-ui-version.sh
    tests/check-ui-installer.sh
    tests/check-doctor-rex-ui.sh
  docs/
    plans/distribution-with-rex-gascity-provider.md   this file
    rex-gascity-ui.md     operator doc for the UI
```

The installed extension is **copied out of the checkout/release** into the
version-keyed cache beside the provider binary:

```text
${GC_RUNTIME_CACHE_DIR:-${XDG_CACHE_HOME:-$HOME/.cache}/rex-gascity}/<version>/ui/
```

Two files are written on the host:

- the stub `~/.config/rex/gascity-ui.lua` (or `$REX_CONFIG`'s directory),
- the marked reference block in `init.lua`.

## The load stub

```lua
-- rex-gascity-ui loader. Written by rex-gascity ui/install.sh.
-- Installed root: <abs root>. Version: <version>. Delete this file to remove.
local root = os.getenv("REX_GASCITY_ROOT") or "<abs root>"
local entry = root .. "/init.lua"
local handle = io.open(entry, "r")
if handle == nil then
	rex.log("warn", "gascity-ui: no extension at " .. entry)
	return
end
handle:close()
dofile(entry)
```

`REX_GASCITY_ROOT` is kept because `ui/README.md` documents it for trying a
branch. The stub is rewritten on every install and on upgrade; it is never
appended to.

## The reference block in `init.lua`

Appended once, in a marked block:

```lua
-- >>> rex-gascity-ui (managed; do not edit this block)
local gascity_ui = os.getenv("REX_GASCITY_UI")
	or (os.getenv("HOME") or "") .. "/.config/rex/gascity-ui.lua"
local gascity_ui_file = io.open(gascity_ui, "r")
if gascity_ui_file == nil then
	rex.log("warn", "gascity-ui: no loader at " .. gascity_ui)
else
	gascity_ui_file:close()
	dofile(gascity_ui)
end
-- <<< rex-gascity-ui
```

Computed through `HOME` rather than an absolute path so a dotfiles repository
stays portable; `REX_GASCITY_UI` is an explicit override. The existence guard
keeps a missing stub a log line, not a config error.

## Detection

Run before anything is written. One probe decides: `rex config check --json`,
which loads the config file in the local `rex` binary and reports the same
`source` field (F2).

The server's action list (`rex actions --json`) is deliberately **not** the
probe. It reports what was loaded when the server started, so a config edited
since the last `rex config reload` makes it stale, and it makes every answer
depend on whether a server happens to be running. It is read only after a
write, to say whether a reload is needed (step 8), and never to decide.

Caveat: `rex config check` loads the file locally, so it follows the config's
own `SHARED_CONFIG` policy, which can differ from how a running server was
started. The check still decides; the post-write server query shows the
difference when there is one.

Collect every action whose name starts with `gascity_`, and take its `source`
path. Then:

| Probe result | Action |
|---|---|
| no `gascity_*` action | install the stub, then try the reference block |
| `source` under the installed root, version matches | print OK; touch nothing |
| `source` under the installed root, version differs | rewrite stub and cache copy; the reference already exists |
| `source` somewhere else (checkout, another copy) | write nothing, edit nothing; report the live path and that config is already wired (drift note) |
| config fails to load | error, change nothing; print the complaint |

The fourth row is the development machine's state today: `user.lua` loads the
checkout, so the installer reports "loaded from `/Volumes/Sources/rex-gascity-ui`"
and makes no edit. That is the intended behaviour, not a warning.

`rex` missing from `PATH` is an error, not a degraded path: the installer stops
before the first write and says so, and the doctor check exits 2. Detection has
no substitute, and a half-wired install is worse than none.

## Installer algorithm

`ui/install.sh`, POSIX sh, `set -eu`, honouring the conventions the provider's
`pack/install.sh` already sets (bounded fetch, stage-then-rename, sha256 from
`checksums.txt`, `--from-source`, one-line diagnostics prefixed
`install-gascity-ui:`).

1. Resolve the config file: `REX_CONFIG` if set, else
   `${XDG_CONFIG_HOME:-$HOME/.config}/rex/init.lua`. Report the resolved path
   and `uname -n`; the file belongs to the machine the *server* runs on.
2. Require `rex` on `PATH`. Without it there is no detection, so the installer
   stops here, before the first write, with an error.
3. Resolve the version: `ui/version` in a checkout, or the release tag
   argument. Refuse to continue when the installed copy and the requested
   version disagree in a way that is not an upgrade.
4. Fetch or copy the extension into `<cache root>/<version>/ui/` — stage in a
   `.$$` sibling, verify sha256 when fetched, `mv` into place atomically.
5. Write the stub `~/.config/rex/gascity-ui.lua` (marker + absolute root).
6. Detect (previous section). Stop here when a load already exists.
7. Try the reference block:
   a. read `init.lua`, resolve the physical path;
   b. if it already carries the marker, done (idempotent);
   c. if it is not a regular file, or a symlink, or the append fails, go to
      the unwritable case;
   d. back up once to `<file>.rex-gascity-ui.bak`; refuse to continue when a
      different backup already exists;
   e. re-stat: size or mtime changed since the read means someone else is
      editing, so stop;
   f. append the block at end of file, after a newline; never reorder, never
      rewrite existing text;
   g. run `rex config check`; on any complaint restore the backup and report
      that nothing changed.
8. Report what the probe now sees. When a server answers, `rex actions --json`
   says whether that server needs `rex config reload` to catch up.

Never write through a symlink and never rename over one: `init.lua` in a Nix
system is a symlink into a read-only store, and replacing it would break the
generated file rather than add to it.

## Unwritable `init.lua`

The stub is in place, nothing else is written, and the message names the
problem and the fix. Exit 1 in doctor terms (D7), exit 0 for a hand-run
install that only needs one manual step.

```text
rex-gascity-ui: extension installed at <root> (<version>), but the load
reference could not be added.
rex-gascity-ui: /Users/boris/.config/rex/init.lua is not writable
(symlink to /nix/store/k5kgpqxamqn8y08b1hzzlc5j5r022j7j-hm_init.lua, mode 0444).
rex-gascity-ui: add one line where that file is generated:
  dofile((os.getenv("XDG_CONFIG_HOME") or (os.getenv("HOME") or "") .. "/.config") .. "/rex/gascity-ui.lua")
rex-gascity-ui: the stub is ready at /Users/boris/.config/rex/gascity-ui.lua.
rex-gascity-ui: no other file was touched.
```

The installer does not offer to edit `user.lua`, or any other dependency
(D4): another extension may own it, and `init.lua` loads it only conditionally
(F5).

## Doctor check

`pack/doctor/rex-ui/doctor.toml` — description only, matching the sibling
check's shape. `run.sh`:

- read-only, never writes state (F7);
- resolves the pack directory from its own path, as the sibling does;
- detection via `rex config check --json` (a check may not start a server);
- exit 0 when the extension is loaded from the installed root at the pack's
  version; exit 1 when the stub exists and the reference is missing, or when
  the loaded copy is not the installed one (name both paths); exit 2 when the
  config fails to load or `rex` is missing;
- `--fix` runs `ui/install.sh`, which is why the fix can only resolve the
  first two rows.

## Versioning

`ui/version` must equal `[pack] version` in `pack/pack.toml`. One tag
`v<version>` cuts both, exactly as the provider binary is stamped today. A new
pack check `pack/tests/check-ui-version.sh` enforces the equality, in the
shape of `pack/tests/check-release-version.sh`. The installed root carries the
version in its path, so version identity costs no Lua change.

Release assets for tag `v<version>`:

| Asset | Purpose |
|---|---|
| `gc-runtime-rex_<os>_<arch>` | unchanged |
| `gascity-ui.tar.gz` | the `ui/` tree, for a host with no checkout |
| `checksums.txt` | sha256 of both |

## Tests

TDD order, following `AGENTS.md`: each behaviour lands with the test that
fails without it.

- `pack/tests/check-ui-version.sh` — version equality, and that a mismatch
  fails the release gate.
- `pack/tests/check-ui-installer.sh` — the installer against a fake `rex`
  (a JSON answer for the check probe), a fake `curl`/`gh` from
  `check-docs.sh`, and a scratch `HOME`. Cases: fresh install writes stub and
  block; second run changes nothing; block already present; read-only
  `init.lua` places the stub and reports the line; loaded from another root
  writes nothing; config that fails to load is left untouched, backup
  restored; symlinked `init.lua` is never replaced.
- `pack/tests/check-doctor-rex-ui.sh` — exit 0/1/2 for the rows above, and
  that `--fix` cannot turn a missing reference into a pass.
- `ui/tests/run.lua` — unchanged; the installer is POSIX sh and its tests are
  shell tests.
- `pack/tests/check-docs.sh` — the new UI doc's fences run in that harness.
  Its stub `rex` answers nothing but `exit 0` today, so a fence that calls
  `rex config check --json` needs the stub to learn that one probe. Decide per
  doc: the UI operator doc gets the install commands; `docs/rex-runtime-pack.md`
  gets a pointer to it.

## Docs

- `ui/README.md` — install alone, install with the provider, upgrade,
  uninstall, `REX_GASCITY_ROOT`, the wiring rules (which file is written, and
  when it is not).
- `ui/AGENTS.md` — for agents working on the extension: how to run
  `ui/tests/run.lua`, that `ui/version` must equal `[pack] version`, and that
  the extension never edits `init.lua` itself (D4).
- `docs/rex-gascity-ui.md` — operator doc: the two install paths, what the
  doctor check reports, and the unwritable-`init.lua` remedy with the exact
  line.
- `docs/rex-runtime-pack.md` — a short section naming the UI check and
  pointing at the operator doc.
- Root `README.md` — the pack now ships an operator-facing companion; the
  install section gains the UI check.

## Rollout

1. Layout and identity: move the tree into `rex-gascity/ui/`, add
   `ui/version`, `install.sh` writing only the stub, and
   `check-ui-version.sh`. No `init.lua` editing yet.
2. Reference block and full detection, with the installer and doctor tests.
3. `pack/doctor/rex-ui/` — check, then fix.
4. Release plumbing: `gascity-ui.tar.gz` in `.github/workflows/release.yml`,
   checksummed with the provider assets.
5. Docs.
6. Migrate the development machine: install from the checkout
   (`--from-source`), then delete the hand-written block from `user.lua`, and
   confirm the detection row "loaded from the installed root".

Uninstall is part of step 1: `ui/install.sh --uninstall` removes the marker
block, deletes the stub, and leaves the cache copy and `gascity.lua` alone.

## Open questions

1. **`REX_GASCITY_UI` and `REX_GASCITY_ROOT`.** Two environment overrides is
   one too many. **Recommendation:** keep `REX_GASCITY_ROOT` (documented
   today) for the extension root, use `REX_GASCITY_UI` only for the stub path,
   and document both in one table.
2. **To verify before step 3: may a pack `--fix` fetch a release asset?** The
   fix fetches `gascity-ui.tar.gz` from the same release as the provider binary
   (D9). `pack/doctor/rex-binary/fix.sh` already reaches the network, so the
   mechanism is there; confirm that a doctor fix is not restricted to local
   files, and that a failed fetch leaves the pack directory clean.

## Non-goals

- No Rex extension directory. Rex reads one config file per account; this plan
  works with that, and does not ask Rex to change.
- No single-file bundle of the Lua modules. The version-keyed directory is
  enough, and a bundle would need a build step for no user-visible gain.
- No change to the provider protocol, `pack.toml`'s `[runtimes.rex]` binding,
  or the provider binary.
- No Windows support, matching the provider (D10).
