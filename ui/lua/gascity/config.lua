-- Settings for the Gas City extension.
--
-- Defaults are here; a person overrides them in ~/.config/rex/gascity.lua,
-- which is plain Lua returning one table:
--
--   return {
--     city = "~/Sources/gas-city-00",
--     recipient = "mayor",
--     notify = false,
--   }

local M = {}

M.defaults = {
	gc_bin = nil, -- the gc program; nil finds it on PATH or in a known place
	city = nil, -- the city to send to; nil uses the city the pane is in
	recipient = "mayor", -- who the mayor action sends to
	notify = true, -- nudge the recipient after sending
	subject_width = 72, -- longest subject line made from the message
}

-- Every setting name, including the ones that default to nil.
local KNOWN = {
	gc_bin = "string",
	city = "string",
	recipient = "string",
	notify = "boolean",
	subject_width = "number",
}

-- Where the settings file lives when nothing names another one.
function M.default_path()
	local base = os.getenv("XDG_CONFIG_HOME")
	if base == nil or base == "" then
		base = (os.getenv("HOME") or "") .. "/.config"
	end
	return base .. "/rex/gascity.lua"
end

-- Expands a leading ~ to the home directory. Paths reach a program, so they
-- are expanded here rather than by a shell. `home` defaults to $HOME and is a
-- parameter so a test can name a home that holds a "%".
function M.expand(path, home)
	if type(path) ~= "string" or path == "" then
		return path
	end
	home = home or os.getenv("HOME") or ""
	if path == "~" then
		return home
	end
	-- Plain concatenation, not a gsub replacement: a "%" in HOME would be read
	-- as the start of an escape.
	if path:sub(1, 2) == "~/" then
		return home .. "/" .. path:sub(3)
	end
	return path
end

local function readable(path)
	local f = io.open(path, "r")
	if f then
		f:close()
		return true
	end
	return false
end

-- Reads a settings file and returns the settings it names over the defaults.
-- A broken file is reported and ignored: the extension keeps working with
-- what it can, rather than leaving no actions at all.
function M.resolve(path)
	local settings = {}
	for key, value in pairs(M.defaults) do
		settings[key] = value
	end

	path = M.expand(path)
	if path ~= nil and path ~= "" and readable(path) then
		local chunk, load_err = loadfile(path)
		local loaded, run_err
		if chunk == nil then
			run_err = load_err
		else
			local ok
			ok, loaded = pcall(chunk)
			if not ok then
				run_err = loaded
				loaded = nil
			end
		end

		if run_err ~= nil then
			rex.log("warn", "gascity: cannot read " .. path .. ": " .. tostring(run_err))
		elseif type(loaded) ~= "table" then
			rex.log("warn", "gascity: " .. path .. " must return a table of settings")
		else
			for key, value in pairs(loaded) do
				local kind = KNOWN[key]
				if kind == nil then
					rex.log("warn", "gascity: unknown setting '" .. tostring(key) .. "' in " .. path)
				elseif type(value) ~= kind then
					rex.log("warn", "gascity: " .. key .. " in " .. path .. " must be a " .. kind)
				else
					settings[key] = value
				end
			end
		end
	end

	settings.gc_bin = M.expand(settings.gc_bin)
	settings.city = M.expand(settings.city)
	-- Where the settings came from, for messages that tell the person what to
	-- edit. Not a setting of its own.
	settings.file = path ~= nil and path ~= "" and path or M.default_path()
	return settings
end

return M
