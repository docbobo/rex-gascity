-- Asking the mayor for a status update.
--
-- The UI renders nothing. It finds two things on the pane's machine - the key
-- this city imports the rex-gascity pack under, and the Rex server that owns
-- the session - and mails the mayor one instruction that names them. The
-- mayor then runs the pack's `status-update` command, which opens the pane.

local text = require("gascity.text")

local M = {}

M.SUBJECT = "Status update requested"

-- The probe escapes every %, @, and < in a value, so a value cannot split a
-- row at @@ or end the answer early at the probe's end marker. This reads it
-- back: %25 is %, %40 is @, %3C is <.
local function unescape(value)
	return (value:gsub("%%(%x%x)", function(code)
		if code == "25" then
			return "%"
		elseif code == "40" then
			return "@"
		elseif code == "3C" then
			return "<"
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

return M
