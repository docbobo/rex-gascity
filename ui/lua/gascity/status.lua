-- Asking the mayor for a status update.
--
-- The UI renders nothing. It finds two things on the pane's machine - the key
-- this city imports the rex-gascity pack under, and the Rex server that owns
-- the session - and mails the mayor one instruction that names them. The
-- mayor then runs the pack's `status-update` command, which opens the pane.

local codec = require("gascity.codec")
local gc = require("gascity.gc")
local probe = require("gascity.probe")
local shell = require("gascity.shell")
local text = require("gascity.text")

local M = {}

M.SUBJECT = "Status update requested"

-- Prints `@@city|<city_path>`, then `@@import|<key>|<source>` for each entry
-- of the top-level config.Imports in `gc config show --json`. It prints
-- nothing for the rig-level imports (Defaults.Rig.Imports, Rigs[].Imports),
-- which can reuse a key. The whole JSON is too large to read back off a
-- probe's screen, so only these lines leave the machine. It tracks strings,
-- escapes, and the key at each depth; that is all it needs from JSON. A
-- string is decoded from its JSON escapes before it is escaped for the row.
-- The program holds no single quote, so it can sit in a single-quoted shell
-- word.

-- The shell that answers, one fact per line. Each line starts with @@, so the
-- rows of the screen can be joined and split again: a value such as an
-- import key cannot look like a marker, because every value is escaped.
--
--   @@city|<path>             the city gc read (its city_path)
--   @@server|<address or>     the endpoint that owns the session: $REX_SERVER when
--                             it is set, otherwise the endpoint rex itself uses
--   @@server_error|<reason>   why the check did not pass, when the row above is empty
--   @@gc|missing              no gc, so no import lookup
--   @@config|<status>         how `gc config show --json` exited
--   @@error|<stderr>          its stderr on one line, when that status is not 0
--   @@import|<key>|<source>   one per top-level import
--
-- It never runs `exit`: the probe needs its end marker. With `pack_binding`
-- set there is no import lookup, but the server check and the gc check still
-- run: the override names the binding, not gc (REQ-002, REQ-003).
function M.script(settings, city, session_id)
	local lines = {}
	local function add(line)
		lines[#lines + 1] = line
	end

	-- The shell `esc` function comes from codec.wrap, so it is not added here.

	-- A named city is passed to gc. Without one, gc finds the city from the
	-- pane's directory itself, which also works from inside a rig; the city
	-- it found comes back as city_path.
	if city ~= nil and city ~= "" then
		add("city=" .. text.quote(city))
		for _, line in ipairs(shell.tilde("city")) do
			add(line)
		end
		add('set -- --city "$city"')
	else
		add("set --")
	end

	-- The server. The app's rex comes first: the pane lives in the app, and a
	-- different rex on PATH (or one without --autostart) must not decide.
	-- --autostart=false keeps the check read-only: an address with no server
	-- behind it fails instead of starting one. A check that does not pass puts
	-- the reason in @@server_error, so the action can name it.
	--
	-- $REX_SERVER is the pane's environment, and a pane on a remote server may
	-- have none: its server process can start without it. So REX_SERVER is the
	-- first candidate only when it is set. Otherwise the endpoint rex connects
	-- to on this machine (`whoami --json`) is the candidate, and the same
	-- ownership check decides. The session lives on the machine the probe runs
	-- on (probe.run makes its block in that session), so that endpoint is the
	-- one that owns it.
	add('rex=')
	add('if [ -n "${REX_BIN_DIR:-}" ] && [ -x "$REX_BIN_DIR/rex" ]; then rex="$REX_BIN_DIR/rex"; fi')
	add('if [ -z "$rex" ]; then rex=$(command -v rex 2>/dev/null) || rex=; fi')
	add("server=")
	add("why=")
	add('if [ -z "$rex" ]; then why="no rex on PATH and none in REX_BIN_DIR${REX_SERVER:+ (REX_SERVER=$REX_SERVER)}"')
	add('elif [ -n "${REX_SERVER:-}" ]; then')
	add("\tcode=0")
	add('\tout=$("$rex" -S "$REX_SERVER" --autostart=false api call session.list_blocks -s '
		.. text.quote(session_id or "")
		.. ' </dev/null 2>&1) || code=$?')
	add('\tif [ "$code" -eq 0 ]; then server=$REX_SERVER')
	add('\telse why="rex -S $REX_SERVER api call session.list_blocks failed (exit $code): $out"')
	add("\tfi")
	add("else")
	add("\tcode=0")
	add('\tout=$("$rex" --autostart=false whoami --json 2>&1) || code=$?')
	add("\tendpoint=")
	add('\tif [ "$code" -eq 0 ]; then')
	add("\t\tendpoint=$(printf '%s\\n' \"$out\" | sed -n 's/.*\"endpoint\"[[:space:]]*:[[:space:]]*\"\\([^\"]*\\)\".*/\\1/p')")
	add("\tfi")
	add('\tif [ "$code" -ne 0 ]; then why="rex --autostart=false whoami --json failed (exit $code): $out"')
	add('\telif [ -z "$endpoint" ]; then why="rex whoami reported no endpoint: $out"')
	add("\telse")
	add("\t\tcode=0")
	add('\t\tout=$("$rex" -S "$endpoint" --autostart=false api call session.list_blocks -s '
		.. text.quote(session_id or "")
		.. ' </dev/null 2>&1) || code=$?')
	add('\t\tif [ "$code" -eq 0 ]; then server=$endpoint')
	add('\t\telse why="rex -S $endpoint api call session.list_blocks failed (exit $code): $out"')
	add("\t\tfi")
	add("\tfi")
	add("fi")
	add('if [ -n "$why" ]; then printf \'@@server_error|%s\\n\' "$(esc "$(printf \'%s\' "$why" | tr \'\\n\' \' \')")"; fi')
	add('printf \'@@server|%s\\n\' "$(esc "$server")"')

	-- gc. Checked with or without pack_binding: the override skips only the
	-- import lookup (REQ-002), and a missing gc must still stop the action
	-- before any mail goes (REQ-003).
	for _, line in ipairs(gc.words(settings)) do
		add(line)
	end
	add("has_gc() {")
	for _, line in ipairs(gc.guard({ exiting = false })) do
		add("\t" .. line)
	end
	add("}")
	add("if ! has_gc 2>/dev/null; then")
	add("\tprintf '@@gc|missing\\n'")
	if type(settings.pack_binding) == "string" and text.trim(settings.pack_binding) ~= "" then
		add("fi")
		return table.concat(lines, "\n")
	end
	add("else")
	-- The lookup's output and stderr go into a private directory. mktemp -d
	-- makes it with a random name and mode 0700, so no other process can put
	-- a file or a symlink at a path inside it. It is removed on every path
	-- that made it.
	add('\ttmp=$(mktemp -d "${TMPDIR:-/tmp}/gascity-status.XXXXXX" 2>/dev/null) || tmp=')
	add('\tif [ -z "$tmp" ]; then')
	add("\t\tprintf '@@config|1\\n@@error|cannot make a temporary directory\\n'")
	add("\telse")
	add('\t\tcode=0')
	add('\t\t"$gc" "$@" config show --json >"$tmp/out" 2>"$tmp/err" || code=$?')
	add('\t\tprintf \'@@config|%s\\n\' "$code"')
	add('\t\tif [ "$code" -eq 0 ]; then')
	-- A filter that fails must not look like a city with no imports: it
	-- reports its own error, and M.ask returns it.
	add('\t\t\tif awk \'' .. M.FILTER .. '\' "$tmp/out" >"$tmp/rows" 2>"$tmp/err"; then')
	add('\t\t\t\tcat "$tmp/rows"')
	add("\t\t\telse")
	add('\t\t\t\tprintf \'@@error|the config filter failed: %s\\n\' "$(esc "$(tr \'\\n\' \' \' <"$tmp/err")")"')
	add("\t\t\tfi")
	add("\t\telse")
	add('\t\t\tprintf \'@@error|%s\\n\' "$(esc "$(tr \'\\n\' \' \' <"$tmp/err")")"')
	add("\t\tfi")
	add('\t\trm -rf "$tmp"')
	add("\tfi")
	add("fi")

	return table.concat(lines, "\n")
end

-- Reads the answer into `{ city, server, server_error, gc_missing,
-- config_status, error, imports = { { key = ..., source = ... }, ... } }`.
--
-- The screen wraps a long line into rows, so the rows are joined and the
-- answer is split again at each @@. The probe escapes a value that holds @@.
function M.parse(output)
	local rows = codec.rows(output, { "city", "server", "server_error", "gc", "config", "error", "import" }) or {}
	local answer = { gc_missing = false, imports = {} }
	for _, row in ipairs(rows) do
		local kind = row.kind
		if kind == "city" then
			local value = codec.value(row)
			answer.city = (value ~= "" and value ~= "-") and value or nil
		elseif kind == "server" then
			local value = codec.value(row)
			answer.server = value ~= "" and value or nil
		elseif kind == "server_error" then
			local value = text.trim(codec.value(row))
			answer.server_error = value ~= "" and value or nil
		elseif kind == "gc" then
			answer.gc_missing = row.value == "missing"
		elseif kind == "config" then
			answer.config_status = tonumber(row.value)
		elseif kind == "error" then
			answer.error = text.trim(codec.value(row))
		elseif kind == "import" then
			local parts = codec.parts(row.value) or {}
			if parts[1] ~= nil and parts[1] ~= "" then
				answer.imports[#answer.imports + 1] = { key = parts[1], source = parts[2] or "" }
			end
		end
	end
	return answer
end

-- Whether an import `Source` is this pack. Both live spellings count:
-- github.com/docbobo/rex-gascity//pack and .../tree/<ref>/pack, with or
-- without a scheme, a .git on the repository, repeated slashes, or a
-- trailing slash.
function M.is_this_pack(source)
	if type(source) ~= "string" then
		return false
	end
	local s = source:gsub("^%a[%w+.%-]*://", ""):gsub("/+$", ""):gsub("/+", "/")
	s = s:gsub("^(github%.com/docbobo/rex%-gascity)%.git", "%1")
	if s == "github.com/docbobo/rex-gascity/pack" then
		return true
	end
	return s:match("^github%.com/docbobo/rex%-gascity/tree/[^/]+/pack$") ~= nil
end

-- The key to run the pack's commands under: `override` when it is set,
-- otherwise the first matching import key in sorted order, otherwise nil.
function M.binding(imports, override)
	if type(override) == "string" and text.trim(override) ~= "" then
		return text.trim(override)
	end
	local keys = {}
	for _, entry in ipairs(imports or {}) do
		if M.is_this_pack(entry.source) then
			keys[#keys + 1] = entry.key
		end
	end
	table.sort(keys)
	return keys[1]
end

-- The mail. Every value is one quoted shell word, so the mayor can paste the
-- command as it is.
function M.body(binding, server, session_id, display)
	local command = "gc " .. text.quote(binding) .. " status-update"
	return table.concat({
		"Please post a status update to my Rex pane.",
		"",
		"Print the template with:",
		"",
		"    " .. command .. " --template",
		"",
		"Fill in every section, then pipe the filled-in update to:",
		"",
		"    " .. command
			.. " --server " .. text.quote(server)
			.. " --session " .. text.quote(session_id)
			.. " --display " .. text.quote(display),
	}, "\n")
end

-- Asks the pane's machine. `opts.city` names the city, `opts.cwd` the
-- directory to ask from. Returns the parsed answer, or nil and a reason.
-- A probe error or a failed config filter is a reason, not an answer with no
-- imports (REQ-003). A config show that failed with a status still comes back
-- as the answer, so the caller can name the exit and the message.
function M.ask(settings, ctx, opts)
	opts = opts or {}
	local answer, err = probe.run(ctx, M.script(settings, opts.city, ctx.session_id), {
		cwd = opts.cwd,
		patience = true,
	})
	if answer == nil then
		return nil, err
	end
	local parsed = M.parse(answer.output)
	if parsed.error ~= nil and (parsed.config_status == nil or parsed.config_status == 0) then
		return nil, parsed.error
	end
	return parsed
end

return M
