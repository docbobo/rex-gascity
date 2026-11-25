-- Asking the mayor for a status update.
--
-- The UI renders nothing. It finds two things on the pane's machine - the key
-- this city imports the rex-gascity pack under, and the Rex server that owns
-- the session - and mails the mayor one instruction that names them. The
-- mayor then runs the pack's `status-update` command, which opens the pane.

local gc = require("gascity.gc")
local probe = require("gascity.probe")
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

-- The row encoding: % is %25, @ is %40, < is %3C, | is %7C, a tab %09, a
-- newline %0A, a carriage return %0D. The filter and the probe's shell share
-- this one function, so the two encoders cannot drift.
local ENCODE = [[
function enc(v,   r, i, n, ch) {
	r = ""
	n = length(v)
	for (i = 1; i <= n; i++) {
		ch = substr(v, i, 1)
		if (ch == "%") r = r "%25"
		else if (ch == "@") r = r "%40"
		else if (ch == "<") r = r "%3C"
		else if (ch == "|") r = r "%7C"
		else if (ch == "\t") r = r "%09"
		else if (ch == "\n") r = r "%0A"
		else if (ch == "\r") r = r "%0D"
		else r = r ch
	}
	return r
}
]]

M.FILTER = [==[
function hex(s,   i, v, c) {
	if (length(s) < 4) return -1
	v = 0
	for (i = 1; i <= 4; i++) {
		c = index("0123456789abcdef", tolower(substr(s, i, 1)))
		if (c == 0) return -1
		v = v * 16 + c - 1
	}
	return v
}
function utf8(n) {
	if (n < 128) return sprintf("%c", n)
	if (n < 2048) return sprintf("%c%c", 192 + int(n / 64), 128 + n % 64)
	if (n < 65536) return sprintf("%c%c%c", 224 + int(n / 4096), 128 + int(n / 64) % 64, 128 + n % 64)
	return sprintf("%c%c%c%c", 240 + int(n / 262144), 128 + int(n / 4096) % 64, 128 + int(n / 64) % 64, 128 + n % 64)
}
# The JSON string content with escapes turned back into characters.
function dec(v,   r, i, n, c, e, u, l) {
	r = ""
	n = length(v)
	for (i = 1; i <= n; i++) {
		c = substr(v, i, 1)
		if (c != "\\") { r = r c; continue }
		i++
		e = substr(v, i, 1)
		if (e == "\"" || e == "\\" || e == "/") r = r e
		else if (e == "b") r = r sprintf("%c", 8)
		else if (e == "f") r = r sprintf("%c", 12)
		else if (e == "n") r = r sprintf("%c", 10)
		else if (e == "r") r = r sprintf("%c", 13)
		else if (e == "t") r = r sprintf("%c", 9)
		else if (e == "u") {
			u = hex(substr(v, i + 1, 4))
			if (u < 0) { r = r e; continue }
			i += 4
			if (u >= 55296 && u <= 56319 && substr(v, i + 1, 2) == "\\u") {
				l = hex(substr(v, i + 3, 4))
				if (l >= 56320 && l <= 57343) { u = 65536 + (u - 55296) * 1024 + (l - 56320); i += 6 }
			}
			if (u >= 55296 && u <= 57343) u = 65533
			r = r utf8(u)
		}
		else r = r e
	}
	return r
}
]==] .. ENCODE .. [==[
{
	n = length($0)
	for (i = 1; i <= n; i++) {
		c = substr($0, i, 1)
		if (instr) {
			if (esc) { esc = 0; s = s c }
			else if (c == "\\") { esc = 1; s = s c }
			else if (c == "\"") { instr = 0; have = 1 }
			else s = s c
			continue
		}
		if (c == "\"") { instr = 1; s = ""; have = 0 }
		else if (c == ":") { if (have) key[d] = s; have = 0 }
		else if (c == "," || c == "}" || c == "]") {
			if (have && d == 1 && key[1] == "city_path")
				print "@@city|" enc(dec(s))
			if (have && d == 4 && key[1] == "config" && key[2] == "Imports" && key[4] == "Source")
				print "@@import|" enc(dec(key[3])) "|" enc(dec(s))
			have = 0
			if (c != ",") d--
		}
		else if (c == "{" || c == "[") { d++; key[d] = "" }
	}
}
]==]

-- The shell that answers, one fact per line. Each line starts with @@, so the
-- rows of the screen can be joined and split again: a value such as an
-- import key cannot look like a marker, because every value is escaped.
--
--   @@city|<path>             the city gc read (its city_path)
--   @@server|<address or>     $REX_SERVER, only once it is seen to own the session
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

	-- Escape a value so it cannot split a row at @@ or | or end the answer
	-- early. The awk `enc` above is the only mapping. awk reads records, so
	-- it joins their lines first and cannot keep a trailing newline: a value
	-- for this helper carries none.
	add("esc() { printf '%s' \"$1\" | awk '" .. ENCODE .. " { if (NR > 1) v = v \"\\n\"; v = v $0 } END { printf \"%s\", enc(v) }'; }")

	-- A named city is passed to gc. Without one, gc finds the city from the
	-- pane's directory itself, which also works from inside a rig; the city
	-- it found comes back as city_path.
	if city ~= nil and city ~= "" then
		add("city=" .. text.quote(city))
		add('case "$city" in')
		add('\t"~/"*) city="$HOME/${city#\\~/}" ;;')
		add("esac")
		add('set -- --city "$city"')
	else
		add("set --")
	end

	-- The server. --autostart=false keeps the check read-only: an address with
	-- no server behind it fails instead of starting one. Only the exit status
	-- is used, so nothing it prints reaches the screen.
	add('rex=$(command -v rex 2>/dev/null) || rex=')
	add('if [ -z "$rex" ] && [ -n "${REX_BIN_DIR:-}" ] && [ -x "$REX_BIN_DIR/rex" ]; then rex="$REX_BIN_DIR/rex"; fi')
	add("server=")
	add('if [ -n "${REX_SERVER:-}" ] && [ -n "$rex" ] && "$rex" -S "$REX_SERVER" --autostart=false api call session.list_blocks -s '
		.. text.quote(session_id or "")
		.. ' </dev/null >/dev/null 2>&1; then server=$REX_SERVER; fi')
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

-- The probe escapes every %, @, <, |, tab, newline, and carriage return in
-- a value, so a value cannot split a row at @@ or | or end the answer early
-- at the probe's end marker. This reads it back: %25 is %, %40 is @, %3C is
-- <, %7C is |, %09 is a tab, %0A a newline, %0D a carriage return.
local function unescape(value)
	return (value:gsub("%%(%x%x)", function(code)
		if code == "25" then
			return "%"
		elseif code == "40" then
			return "@"
		elseif code == "3C" then
			return "<"
		elseif code == "7C" then
			return "|"
		elseif code == "09" then
			return "\t"
		elseif code == "0A" then
			return "\n"
		elseif code == "0D" then
			return "\r"
		end
		return "%" .. code
	end))
end

-- Reads the answer into `{ city, server, gc_missing, config_status, error,
-- imports = { { key = ..., source = ... }, ... } }`.
--
-- The screen wraps a long line into rows, so the rows are joined and the
-- answer is split again at each @@. The probe escapes a value that holds @@.
function M.parse(output)
	local joined = (output or ""):gsub("\n", "")
	local answer = { gc_missing = false, imports = {} }
	for line in (joined .. "@@"):gmatch("(.-)@@") do
		local kind, rest = line:match("^(%a+)|(.*)$")
		if kind == "city" then
			local value = unescape(rest)
			answer.city = (value ~= "" and value ~= "-") and value or nil
		elseif kind == "server" then
			local value = unescape(rest)
			answer.server = value ~= "" and value or nil
		elseif kind == "gc" then
			answer.gc_missing = rest == "missing"
		elseif kind == "config" then
			answer.config_status = tonumber(rest)
		elseif kind == "error" then
			answer.error = text.trim(unescape(rest))
		elseif kind == "import" then
			local key, source = rest:match("^([^|]*)|(.*)$")
			if key ~= nil and key ~= "" then
				answer.imports[#answer.imports + 1] = { key = unescape(key), source = unescape(source) }
			end
		end
	end
	return answer
end

-- Whether an import `Source` is this pack. Both live spellings count:
-- github.com/docbobo/rex-gascity//pack and .../tree/<ref>/pack, with or
-- without a scheme, a .git on the repository, or a trailing slash.
function M.is_this_pack(source)
	if type(source) ~= "string" then
		return false
	end
	local s = source:gsub("^%a[%w+.%-]*://", ""):gsub("/+$", "")
	s = s:gsub("^(github%.com/docbobo/rex%-gascity)%.git", "%1")
	if s == "github.com/docbobo/rex-gascity//pack" then
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
-- A probe error (a failed lookup or filter) is a reason, not an answer with
-- no imports (REQ-003).
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
	if parsed.error ~= nil then
		return nil, parsed.error
	end
	return parsed
end

return M
