# 0019. The UI ships from this repo through one installer that edits only init.lua

- Status: Accepted
- Date: 2026-11-25 (backfilled from the distribution plan, D1–D10)

## Context

The Gas City UI is a Rex Lua extension. It was a separate git checkout, loaded
by a hand-written block in `~/.config/rex/user.lua`. The provider is a gc pack
that installs itself. The extension must install alone on a bare Rex, or as part
of the provider, from one source of truth.

Rex reads one config file per account, and other extensions may own the files
that config loads.

## Decision

- The UI lives in `ui/`. It is a plain copy, and the old repository is archived.
  It shares this repo's tag and release. `ui/install.sh` is the one host-side
  installer, and the provider's doctor fix (`pack/doctor/rex-ui/fix.sh`) reaches
  the same installer, using the `gascity-ui.tar.gz` asset from the same release.
  `pack/` carries no Lua.
- `init.lua` holds only a marker block that loads the stub
  `~/.config/rex/gascity-ui.lua` (or `$REX_GASCITY_UI`), and the stub loads the
  versioned extension. An upgrade never rewrites `init.lua` again.
- `init.lua` is the only file the installer ever edits. Detection asks
  `rex config check --json` first. When anything already loads the extension,
  the installer writes nothing.
- When `init.lua` cannot be written, the installer places the stub, warns
  (doctor exit 1, which `--fix` cannot clear), and prints the line to add.
- The settings file is `~/.config/rex/gascity.lua`, and the installer never
  writes it. Only macOS and Linux are supported.

`docs/rex-gascity-ui.md` is the operator reference. `ui/AGENTS.md` has the
version rule.

## Consequences

- There is one release pipeline and one version for provider and UI.
- An `init.lua` the user owns stays theirs: the installer adds one block or
  prints one line.

## Alternatives rejected

- **A second repository and release pipeline for the UI.** That is two tags to
  keep in step for one user-visible feature.
- **Edit a file that `init.lua` loads.** Another extension may own it.
- **Probe the server's action list to detect a load.** It goes stale until
  `rex config reload`. The config file as it is now is the truth.
- **Ship the Lua inside `pack/`.** `pack/` is a runtime pack, and the doctor fix
  fetches the release asset instead.
- **Preserve the old repository's history in the move.** That was not worth a
  subtree merge for a small Lua tree.
- **A Rex extension directory, or a single-file bundle.** The first needs Rex to
  change. The second needs a build step for no user-visible gain.
