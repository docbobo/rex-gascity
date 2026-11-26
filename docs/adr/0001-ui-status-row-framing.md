# 0001. The UI keeps a second row format for the status probe

- Status: Accepted
- Date: 2026-11-26 (backfilled; the decision predates it)

## Context

A pane-machine probe answers through a hidden block. `codec.lua` wraps the script
between two markers and builds fields as `name|value` rows, and `codec.rows()`
reads them back (`ui/lua/gascity/codec.lua:13`, `:47`, `:60`).

`codec.rows()` cannot tell a field boundary from a value. It starts a new entry at
any line that begins with a known `name|`, which its own note admits
(`ui/lua/gascity/codec.lua:59`): "a row that starts with a known `name|` inside
JSON reads as a new field. Upgrade to a length-prefixed or base64 field when a
value can contain one."

The status probe carries values decoded from arbitrary JSON: an import key, a
city path, the stderr of `gc config show --json`
(`ui/lua/gascity/status.lua:16-24`, `:96-120`). Any of them can hold a newline or
a `|`, so the probe prints its own rows, `@@name|value`, after escaping `%`, `@`,
`<`, `|`, tab, LF and CR to `%25`, `%40`, `%3C`, `%7C`, `%09`, `%0A` and `%0D`
(`ui/lua/gascity/status.lua:26-28`). The reader strips the newlines and splits at
`@@` (`ui/lua/gascity/status.lua:258-262`), so no value can forge a row.

`status.lua` does not require `codec.lua` at all, unlike `machine.lua`,
`rigs.lua` and `probe.lua`.

## Decision

`status.lua` keeps its own escaped `@@` row format and its own encoder and
decoder. The encoder is one Lua snippet shared by the awk filter and the probe's
shell, so the two writers cannot drift apart
(`ui/lua/gascity/status.lua:26-28`, `:95`).

## Consequences

The extension has two row formats, and only the newer one is collision-safe. A
reader who finds the second format reads it as duplication; it is not. Do not
route the status rows through `codec.rows()` until the framing itself is
unambiguous (#139).

`codec.rows()` keeps its documented limit. Every new probe that carries
JSON-derived values needs the same escaping, and a probe that carries only fixed
kinds does not.

## Alternatives rejected

- **Route the status rows through `codec.rows()`.** Not collision-safe: an import
  key or a city path holding a newline and a known `name|` forges rows
  (`ui/lua/gascity/codec.lua:59`). Revisit only after a length-prefixed or
  base64 field lands (#139).
- **Keep the format but leave it unnamed.** It already caused one misread, as
  duplication of `codec`. Naming it here is the fix.
- **Print the whole `gc config show --json` and parse it in the UI.** The JSON is
  too large to read back off a probe's screen (`ui/lua/gascity/status.lua:19-20`).
  Only `city_path` and the four levels to `config.Imports[].Source` leave the
  machine.
