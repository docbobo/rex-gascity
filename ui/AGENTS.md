## Scope

`ui/` is the Gas City UI, a Rex Lua extension. The repo-wide rules in the root
`AGENTS.md` apply here; this file adds only local invariants. User docs are
`ui/README.md` and `docs/rex-gascity-ui.md`.

## Tests

- Run `lua ui/tests/run.lua` from the repo root (Lua 5.1 or LuaJIT). It uses a
  stub `rex`, so it needs no Rex install.
- The installer is POSIX sh. Its tests are shell tests:
  `pack/tests/check-ui-installer.sh` and `pack/tests/check-doctor-rex-ui.sh`.
- A behavior change lands with the named test that fails without it.

## Invariants

- `ui/version` must equal `[pack] version` in `pack/pack.toml`.
  `pack/tests/check-ui-version.sh` enforces it. Bump both in one change.
- The extension never edits `init.lua` or any other Rex config file. Lua code
  here only reads settings (`~/.config/rex/gascity.lua`).
- `ui/install.sh` is the only host-side writer. It writes the stub
  `gascity-ui.lua`, one marked block in `init.lua`, and the cache copy. It never
  writes `gascity.lua` or a file that `init.lua` loads. Do not add a second
  writer, and do not copy this logic into `pack/doctor/rex-ui/`: the doctor
  fix calls `ui/install.sh`.
- The installer decides with `rex config check --json`, never with the
  server's action list. Keep it that way: the list is stale until
  `rex config reload`. The list may only decide whether to try that reload
  once after the block is appended. A failed reload never changes the verdict.
- Never index `rex.ui` outside the `prompt` helper in
  `lua/gascity/actions.lua`. A server older than control API revision 17 has no
  `rex.ui`; the helper logs once and returns `nil, "unsupported"`.
- `ui/` carries no Go and no pack files. `pack/` carries no Lua.
