# rex-gascity-ui

A [Rex](https://www.superlogical.com/rex/docs/) extension that talks to
[Gas City](https://docs.gascityhall.com) from a pane. The first action sends
mail to the city's mayor: it asks what to send, then runs
`gc mail send mayor …` in a small floating pane.

The code is in this repository's `ui/` directory. `ui/install.sh` copies it to
a versioned directory in the cache and wires it into the Rex config. Changes
take effect with `rex config reload`. The installer runs that reload once when
it adds the block and a running server has a stale action list.

## Install

The extension belongs to the machine that runs the Rex server, not to a city.
Install it once on each such machine. `ui/version` equals `[pack] version` in
`pack/pack.toml`, and release `v<version>` carries both the provider and the UI.

### Alone, from a checkout

```sh
sh ui/install.sh --from-source
```

`--from-source` installs the tree the script sits in. It needs no network.

### Alone, on a bare machine

Download the release tree, then run its installer. The installer fetches the
same asset again and checks its sha256 against `checksums.txt` before it
installs, so the first download is only the way to get the script.

```sh
mkdir -p ~/gascity-ui-install && cd ~/gascity-ui-install
curl -fsSL -o gascity-ui.tar.gz https://github.com/docbobo/rex-gascity/releases/download/v2026.11.04/gascity-ui.tar.gz
tar -xzf gascity-ui.tar.gz
sh ui/install.sh
```

A private repository needs `gh` with `GITHUB_TOKEN` or `GH_TOKEN`. The installer
falls back to `gh` when `curl` fails.

### With the provider

Import the pack, then let the doctor fix install the UI. The fix runs
`ui/install.sh` for you. See [`docs/rex-gascity-ui.md`](../docs/rex-gascity-ui.md).

```sh
gc import add https://github.com/docbobo/rex-gascity/tree/main/pack --name rex --version 2026.11.04
gc import install
gc doctor --check rex-gascity-runtime:rex-ui --fix
```

### What the installer writes

| Path | Content | When |
| --- | --- | --- |
| `${GC_RUNTIME_CACHE_DIR:-${XDG_CACHE_HOME:-$HOME/.cache}/rex-gascity}/<version>/ui/` | the extension | each install |
| `~/.config/rex/gascity-ui.lua` (`$REX_GASCITY_UI`) | the load stub, with the installed root | each install, rewritten |
| `${REX_CONFIG:-${XDG_CONFIG_HOME:-$HOME/.config}/rex/init.lua}` | one marked block that loads the stub | once, see below |

The installer never writes `~/.config/rex/gascity.lua` (the settings file) and
never edits a file that `init.lua` loads. It needs `rex` on `PATH`: it stops
before the first write without it.

### When the installer writes nothing

Before it edits anything, the installer runs `rex config check --json`. That
loads the config file as it is on disk and lists each action with its source
file. The server's action list is not used, because it is stale until
`rex config reload`.

| What `rex config check` shows | Result |
| --- | --- |
| no `gascity_*` action | the stub is written, then the block is appended to `init.lua` |
| actions from the installed copy, same version | nothing is written |
| actions from the installed copy, another version | the tree and stub are replaced. The block exists already |
| actions from another copy (a checkout, a line in `user.lua`) | nothing is written. The installer prints the live path and stops |
| the config does not load | nothing is written. The installer prints the error and stops |

The block is also skipped, and the stub is still written, when `init.lua` does
not exist, is a symlink (a Nix store file, for example), or is not writable. The
installer then prints the line to add where `init.lua` is generated, and exits 0.
`gc doctor --check rex-gascity-runtime:rex-ui` reports that state as a warning
(exit 1), and `--fix` cannot resolve it.

An append backs `init.lua` up once to `init.lua.rex-gascity-ui.bak`. If the
file does not load after the append, the backup is restored. A different
existing backup stops the install.

### Environment

| Variable | Meaning |
| --- | --- |
| `REX_GASCITY_ROOT` | Read by the stub at load time. Loads this tree instead of the installed root, for trying a branch. The server reads it, so set it in the server's environment. |
| `REX_GASCITY_UI` | Path of the stub file. Read by the installer and by the `init.lua` block. |

### Upgrade

Run the install again with the new version. The tree lands in a new
`<version>` directory, the stub points at it, and `init.lua` is not edited
again. With the provider, re-pin the import and rerun the fix. Then reload:

```sh
rex config reload
```

`rex config reload` is refused over a remote connection. Run it on the
machine that runs the server.

### Uninstall

```sh
sh ui/install.sh --uninstall
```

This removes the marked block from `init.lua` and deletes the stub. It leaves
the cache copy and `gascity.lua` alone. When `init.lua` is a symlink, remove
the line you added where that file is generated.

## Use it

- Key: <kbd>Ctrl</kbd>+<kbd>B</kbd> then <kbd>G</kbd> opens **Gas City: Commands…**,
  one list of everything below.

  It is a menu of this extension's own, not the app's command palette: that
  palette opens with no filter, because `client.palette.toggle` takes no
  arguments, so a key cannot ask it for one category. The menu is a `rex.ui.pick`
  over the Gas City commands, and each of them still stands on its own in the
  app's palette under **Gas City**.

  Prompts need control API revision 17. On an older server the menu and every
  command that needs a prompt log one warning and send nothing.
- CLI, without a prompt:

  ```sh
  rex do gascity_mail_mayor body="Build is green"
  rex do gascity_mail_mayor body="Build is green" subject="Build"
  rex do gascity_mail_mayor body="Hello" city="~/cities/two"
  rex do gascity_rigs_suspend rigs="alpha, beta"
  rex do gascity_status_update city="~/cities/two"
  rex do gascity_doctor
  ```

The message is trimmed at both ends and becomes the subject line, cut to 72
characters at a word boundary. Cancelling sends nothing, and an empty message
sends nothing.

## How the message is sent

To ask the pane's machine about its cities, the action runs a short command in
a block that is never placed anywhere, reads what it printed, and closes it
again. That costs about a tenth of a second, and it is the reason a question
about that machine can be answered at all: the action's own Lua may run
somewhere else.

Then the action puts the command in a small floating pane of the session it was
run for:

```
gc mail send mayor -s <subject> -m <body> --notify
```

Rex routes that pane to the server that owns the session, so the command runs
on the machine that owns the pane, in that pane's directory, and `gc` finds the
city by walking up from there. Nothing in the extension has to know which
machine that is, and no city path is configured anywhere.

Rex closes the pane when the command exits 0, and keeps it, with what it
printed, when it does not. So a message that was sent leaves nothing behind,
and a failure leaves the error on screen.

### Which city

A pane inside a city sends to that city: `gc` walks up from the directory the
command runs in. Two panes in two cities send to their own, and nothing is
asked.

When the pane is in no city, the extension asks the pane's machine which cities
it has, and then:

- **one city** - it is used, without asking;
- **several** - a picker names them, with the path under each name;
- **none** - the message is sent anyway and `gc` says what it found.

The list comes from `gc cities`, which is the supervisor's list of registered
cities, so a city only appears there once it is registered:

```sh
gc register /path/to/city
```

To name a city yourself, pass one or set it:

```sh
rex do gascity_mail_mayor body="Hello" city="~/cities/two"
```

```lua
return { city = "~/cities/two" }
```

A named city becomes `gc --city`, and a leading `~` is expanded by the script,
on the machine that has the city - not by whichever machine asked for it. A
named city skips the question entirely.

## Suspending and resuming rigs

- Command palette: **Gas City: Suspend a Rig…** and **Gas City: Resume a Rig…**
- CLI:

  ```sh
  rex do gascity_rigs_suspend rigs="alpha, beta"
  rex do gascity_rigs_resume  rigs="alpha" city="~/cities/two"
  ```

The picker lists the rigs of the city the pane belongs to, each with its path
and whether it is running, suspended, or the HQ, and it takes several at once.
Ticking none, or cancelling, does nothing. A suspended rig's agents are skipped
by the reconciler; its beads database stays reachable, and resuming lets the
reconciler start them on its next tick.

Nothing appears on the screen for this. The work runs in a block that is never
placed, the same kind the rig list is asked in: it reports its exit status, the
action reads that and the output, and then closes it. A failure therefore comes
back as the action's error, one line, with the whole output in the Rex log.

Which rigs exist is asked of the pane's machine, in a hidden block, the same
way the city list is: `gc rig list --json`, with its newlines taken out first,
because the answer is read back off the screen and a pretty-printed list would
be indistinguishable from the screen's own line breaks. The city that answered
is the city the changes go to.

Naming the rigs with `rigs=` skips both the question and that trip to the pane's
machine: `gc` then finds the city from the pane's directory.

Suspending the whole city is not here. It is `gc suspend` and `gc resume` on
its own, and would be the same action with the rig question taken out.

## Settings

Optional. The extension reads `~/.config/rex/gascity.lua`, or the file named
by `$REX_GASCITY_CONFIG`. It must return a table:

```lua
return {
	recipient = "mayor",   -- who the action sends to
	notify = true,         -- nudge the recipient after sending
	city = "~/cities/two", -- default: the city the pane is in
	gc_bin = "/run/current-system/sw/bin/gc", -- default: found from PATH
	subject_width = 72,    -- longest subject line made from a message
	pack_binding = "rex",  -- default: found from the city's imports
	status_display = "text", -- how the mayor's status update is shown
}
```

| Setting | Default | Meaning |
| --- | --- | --- |
| `recipient` | `mayor` | Mail recipient. A rig agent would be `myrig/witness`. |
| `notify` | `true` | Pass `--notify` to `gc mail send`. |
| `city` | none | City to send to, when it should not be the pane's own. |
| `gc_bin` | none | Absolute path to `gc`, for a PATH that misses it. Without it the script uses `gc` from `PATH`, then looks in `/run/current-system/sw/bin`, `/etc/profiles/per-user/$USER/bin`, `~/.nix-profile/bin`, `~/.local/bin`, and `~/go/bin`. |
| `subject_width` | `72` | Longest subject line made from a message. |
| `pack_binding` | none | The binding to use. Set it when the city imports the pack from a local path. With it set, the import lookup is skipped. |
| `status_display` | `text` | The `--display` the mail asks for. The action's `display` argument wins over it. |

A settings file with a mistake is reported in the Rex log and the rest of the
settings still apply.

## Remote servers and other machines

An action can be evaluated by the app, by the server, or by `rex do`, and which
one it is differs between how you got there. That is why this extension does
not run commands itself: it asks the server that owns the pane to run one, so
the machine with the city is always the machine that runs `gc`.

Consequences worth knowing:

- A **client** needs no configuration. If your app is attached to a server on
  another machine, the action still runs on that machine, against the city
  there.
- The **installed tree** and the **settings file** belong to whichever machine
  evaluates the action. If the app evaluates it, that is the app's machine, in
  which case only the settings matter - the send still happens on the pane's
  machine.
- `rex config reload` is refused over a remote connection, so reload on the
  machine that runs the server, for example through a pane there:

  ```sh
  rex -S "HOST" run --label rex-config-reload --keep-open -- rex config reload
  ```

**Gas City: Where This Runs…** (`rex do gascity_doctor`) names both machines
that matter - the one running the action, with its settings and checkout, and
the one that owns the panes - and says when they are the same. It also reports
the pane's directory, the city it is in, the cities gc lists, and the recipient.

From the palette or its key it opens a pane with the facts, an icon for each,
labels in grey and the heading in cyan; anything in trouble, such as a missing
settings file, stands out in red. The pane stays up so it can be read, and says
how to close it. Run it with `rex do` - anything without a person at the app -
and the same facts are printed plainly instead, with no pane.

## Layout

```
init.lua                 entry point; finds its own root and loads the modules
install.sh               the only host-side installer (POSIX sh)
version                  equals [pack] version in pack/pack.toml
lua/gascity/config.lua   settings and the settings file
lua/gascity/pane.lua     the directory of the pane the action was run from
lua/gascity/probe.lua    asks the pane's machine a question, in a hidden block
lua/gascity/machine.lua  the pane's machine: its name, its city, its cities
lua/gascity/gc.lua       how a script finds the gc program
lua/gascity/mail.lua     the `gc mail send` arguments
lua/gascity/send.lua     sending a message, from question to pane
lua/gascity/rigs.lua     the rigs of the pane's city, and suspending them
lua/gascity/run.lua      the shell script a pane runs, and the pane itself
lua/gascity/report.lua   the `gascity_doctor` report
lua/gascity/status.lua   asks the mayor for a status update: binding, server, mail
lua/gascity/actions.lua  the commands, the menu, and the key binding
lua/gascity/text.lua     string helpers
tests/run.lua            tests, with a stub `rex`
```

## Development

```sh
lua ui/tests/run.lua                   # Lua 5.1 or LuaJIT, from the repo root
rex config check                       # loads the config as Rex would
rex config reload
rex actions | grep gascity             # the actions are registered
```

To try an action without reloading:

```sh
rex do ui/init.lua --action gascity_mail_mayor body="Hello"
```

Two traps worth knowing, both of which have caught this extension:

- **A script bypasses the server.** `rex do file.lua` loads the files itself, so
  it runs new code while the palette and the keys still run the copy the server
  read last. A change can look finished and be invisible from the palette until
  `rex config reload`.
- **A path that only runs from the palette needs `origin`.** An action can check
  `ctx.origin`, and `rex do` reports `api`. To exercise the palette path without
  clicking, say so:

  ```sh
  rex api call client.action \
    "$(jq -cn --arg s "$REX_SESSION" --arg b "$REX_BLOCK" \
      '{action:"gascity_doctor", origin:"palette", session_id:$s, block_id:$b}')"
  ```
