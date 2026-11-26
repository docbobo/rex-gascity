# Installing the Gas City UI

How to install the Gas City UI, a Rex extension, and what
`gc doctor --check rex-gascity-runtime:rex-ui` reports. The UI is optional:
Rex and the provider run without it. [`rex-runtime-pack.md`](rex-runtime-pack.md)
is the provider quickstart. [`ui/README.md`](../ui/README.md) covers using the
extension.

## What gets installed

The extension belongs to the machine that runs the Rex server, not to a city.
Install it once on each such machine. One installer, `ui/install.sh`, does the
work on both paths below. It writes:

| Path | Content |
|---|---|
| `${GC_RUNTIME_CACHE_DIR:-${XDG_CACHE_HOME:-$HOME/.cache}/rex-gascity}/<version>/ui/` | the extension |
| `~/.config/rex/gascity-ui.lua` (or `$REX_GASCITY_UI`) | the load stub |
| `${REX_CONFIG:-${XDG_CONFIG_HOME:-$HOME/.config}/rex/init.lua}` | one marked block that loads the stub |

It never edits `~/.config/rex/gascity.lua` (the extension's settings) and never
edits a file that `init.lua` loads. The `rex` CLI must be on `PATH`: the
installer asks `rex config check --json` what the config already loads, and
stops before the first write without it.

Release `v<version>` of `docbobo/rex-gascity` carries the provider binaries,
`gascity-ui.tar.gz` and one `checksums.txt` that lists all of them. The UI
version is the pack version.

## With the provider

Import the pack as in [`rex-runtime-pack.md`](rex-runtime-pack.md), then run the
UI check with `--fix`:

```sh
gc import add https://github.com/docbobo/rex-gascity/tree/main/pack --name rex --version 2026.11.06
gc import install
gc doctor --check rex-gascity-runtime:rex-ui --fix
gc doctor --check rex-gascity-runtime:rex-ui
```

The fix picks the first source that exists: `ui/install.sh` next to the pack
(a checkout), the copy already in the cache, or `gascity-ui.tar.gz` fetched
from release `v<version>` and verified against `checksums.txt`. A fetch writes
only to a temporary directory, so a failed fetch leaves the pack directory as
it was. Then it runs the installer with `--from-source`.

`gc import install` alone does not install the UI.

## Alone, on a bare machine

No pack, no city, no checkout. Download the release tree and run its
installer. The installer fetches the same asset again and checks its sha256
against `checksums.txt` before it installs.

```sh
mkdir -p ~/gascity-ui-install && cd ~/gascity-ui-install
curl -fsSL -o gascity-ui.tar.gz https://github.com/docbobo/rex-gascity/releases/download/v2026.11.06/gascity-ui.tar.gz
tar -xzf gascity-ui.tar.gz
sh ui/install.sh
```

In a checkout of this repository, `sh ui/install.sh --from-source` installs
the tree it sits in and needs no network. A private repository needs `gh`
with `GITHUB_TOKEN` or `GH_TOKEN`; the installer falls back to it when `curl`
fails.

## What the doctor check reports

The check is read-only: it never starts a server, writes a file or downloads.
It runs `rex config check --json` and looks at the source file of each
`gascity_*` action. Exit 0 is OK, 1 is a warning and 2 is an error.

| Exit | State | What to do |
|---|---|---|
| 0 | Loaded from the installed copy, at the pack version. | Nothing. |
| 1 | Not installed: no stub and no `gascity_*` action. | `--fix`. Leaving the UI out is fine. |
| 1 | The stub exists, but the config does not load it. | `--fix` appends the block. If `init.lua` is a symlink or not writable, see [`init.lua` is not writable](#initlua-is-not-writable). |
| 1 | The config loads another version from the cache. | `--fix` installs the pack's version and repoints the stub. |
| 1 | The config loads another copy (a checkout, a line in `user.lua`). | Nothing is written. The check names both paths. Remove that load, then `--fix`. |
| 2 | The config does not load. | Fix the complaint the check prints. Neither the check nor `--fix` writes into a broken config. |
| 2 | `rex` is not on `PATH` (`GC_REX_BIN` overrides the lookup). | Install Rex, then run the check again. |

The installer follows the same rows. It writes nothing when the config already
loads the installed copy, or loads another copy. A running server keeps the old
actions until `rex config reload`. When the installer adds the block and the
list is stale, it runs that reload once. `rex config reload` is refused over a
remote connection, so run it on the machine that runs the server.

## `init.lua` is not writable

When `init.lua` does not exist, is a symlink (a Nix store file is the common
case), or is not writable, the installer places the stub, edits nothing, and
exits 0. It prints the reason and one line to add where `init.lua` is generated:

```text
dofile((os.getenv("REX_GASCITY_UI") or (os.getenv("XDG_CONFIG_HOME") or ((os.getenv("HOME") or "") .. "/.config")) .. "/rex/gascity-ui.lua"))
```

Add it, then run `rex config reload`. The doctor check keeps reporting exit 1
until the line is in place, and `--fix` cannot add it. The installer never
edits `user.lua` or another file that `init.lua` loads.

When it does append, the installer backs `init.lua` up once to
`init.lua.rex-gascity-ui.bak` and restores it if the file no longer loads. A
different existing backup stops the install.

## Upgrade

The installed tree is keyed by version. An upgrade installs a new directory and
rewrites the stub. It does not edit `init.lua` again.

With the provider, re-pin the import to the new version, then fix. The import
steps are in [`rex-binary-install.md`](rex-binary-install.md#sha-imports-and-versions).

```sh
gc import remove rex
gc import add https://github.com/docbobo/rex-gascity/tree/main/pack --name rex --version 2026.11.06
gc import install
gc doctor --check rex-gascity-runtime:rex-ui --fix
```

Alone, repeat the bare-machine commands with the new version in the URL.
Old `<version>/ui` directories stay until you remove them by hand.

## Uninstall

`--uninstall` removes the marked block from `init.lua`. It deletes the stub
only after that succeeds. It leaves the cache copy and `gascity.lua` alone. The
installed tree carries its own installer. Substitute the version you installed
for `<version>`, as in the Upgrade section:

```sh
sh "${GC_RUNTIME_CACHE_DIR:-${XDG_CACHE_HOME:-$HOME/.cache}/rex-gascity}/<version>/ui/install.sh" --uninstall
```

When `init.lua` is a symlink, the installer cannot edit it. It keeps the stub
and prints the block to remove. Remove that line where the file is generated,
then delete the stub. To reclaim the space, remove the cache copy:

```sh
rm -rf "${GC_RUNTIME_CACHE_DIR:-${XDG_CACHE_HOME:-$HOME/.cache}/rex-gascity}/<version>/ui"
```

## Ask the mayor for a status update

**Gas City: Ask the Mayor for a Status Update…** mails the mayor one
instruction: print the template with `gc <binding> status-update --template`,
fill it in, and pipe it to `gc <binding> status-update` with this session's
server, session id, and display. The mayor's command then opens the update in
a floating pane of this session. The action itself renders nothing and does
not wait for the reply.

Before it sends anything, the action asks the pane's machine two things:

- The binding: the key under which `gc config show --json` lists this pack in
  the city's top-level imports. Rig-level imports do not count. Both source
  spellings match: `https://github.com/docbobo/rex-gascity//pack` and
  `https://github.com/docbobo/rex-gascity/tree/<ref>/pack`.
- The server: `$REX_SERVER` when it is set, and otherwise the endpoint that
  `rex whoami --json` reports on the pane's machine. A read-only check then
  shows that the chosen endpoint owns this session. The check runs
  `rex … --autostart=false`, so it never starts a server. A server process can
  start without `REX_SERVER` in its environment, so the fallback is what makes
  a pane on a remote server work.

If any of these fail, the action raises an error and sends no mail: `gc` is
missing, `gc config show` fails, the city does not import the pack, or the
server cannot be confirmed.

Two settings in `~/.config/rex/gascity.lua` change it:

| Setting | Default | Meaning |
|---|---|---|
| `pack_binding` | none | The binding to use. Set it when the city imports the pack from a local path. With it set, the import lookup is skipped. |
| `status_display` | `text` | The `--display` the mail asks for. The action's `display` argument wins over it. |

## Settings and overrides

| Variable | Meaning |
|---|---|
| `REX_GASCITY_ROOT` | Read by the stub when Rex loads it. Loads that tree instead of the installed one, for trying a branch. Set it in the server's environment. |
| `REX_GASCITY_UI` | Path of the stub. Read by the installer and by the `init.lua` block. |
| `REX_CONFIG` | The Rex config file to wire, and the file `rex config check` reads. |
| `GC_RUNTIME_CACHE_DIR`, `XDG_CACHE_HOME` | Move the cache root, as for the provider. |

Extension settings (`recipient`, `city`, `gc_bin` and others) live in
`~/.config/rex/gascity.lua`. [`ui/README.md`](../ui/README.md) lists them.

## Tests

- `pack/tests/check-docs.sh` runs every `sh` fence in this file against a
  scratch city and a scratch `HOME`, with a fake release that carries
  `gascity-ui.tar.gz`.
- `pack/tests/check-ui-installer.sh` covers the installer, including the
  unwritable `init.lua`.
- `pack/tests/check-doctor-rex-ui.sh` covers the exit codes above.
- `pack/tests/check-ui-version.sh` keeps `ui/version` equal to `[pack] version`.
