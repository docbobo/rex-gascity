-- The rigs of the city the pane belongs to.
--
-- The question goes to the pane's machine, in a block, because the city and
-- its rigs are there.

local gc = require("gascity.gc")
local probe = require("gascity.probe")
local text = require("gascity.text")

local M = {}

-- The shell that answers it: the city it settled on, then gc's rig list.
--
-- The list is printed as JSON with its newlines taken out, because the answer
-- is read back off the screen, where a long line is broken into rows. Joining
-- the rows gives the JSON back exactly; a pretty-printed list would not survive
-- that, since its own line breaks look the same as the screen's.
local function script(settings, city)
	local lines = {}
	local function add(line)
		lines[#lines + 1] = line
	end

	if city ~= nil and city ~= "" then
		add("city=" .. text.quote(city))
		add('case "$city" in')
		add('\t"~/"*) city="$HOME/${city#\\~/}" ;;')
		add("esac")
	else
		-- No city named: the pane's directory decides, as it does for gc.
		add("city=")
		add('d="$(pwd)"')
		add('while [ -n "$d" ]; do')
		add('\tif [ -f "$d/city.toml" ] || [ -d "$d/.gc" ]; then city="$d"; break; fi')
		add('\t[ "$d" = "/" ] && break')
		add('\td="$(dirname "$d")"')
		add("done")
	end

	for _, line in ipairs(gc.words(settings)) do
		add(line)
	end

	add('if [ -n "$city" ]; then set -- --city "$city"; else set --; fi')
	add('printf \'city|%s\\n\' "${city:--}"')
	add("printf 'rigs|'")
	add('"$gc" "$@" rig list --json 2>/dev/null | tr -d \'\\n\'')
	add("printf '\\n'")

	return table.concat(lines, "\n")
end

-- Reads the answer into `{ city = path or nil, rigs = { ... } }`.
--
-- `decode` turns the JSON into a value; it is rex.json.decode unless a test
-- gives another one.
function M.parse(answer, decode)
	decode = decode or function(source)
		return rex.json.decode(source)
	end

	-- The terminal wraps a long city path across rows, so the path is read up
	-- to the next marker and then unfolded. The rigs JSON below does the same
	-- thing by dropping its newlines before decoding.
	local city = answer:match("city|(.-)rigs|")
	if city ~= nil then
		city = city:gsub("\n", "")
	end
	if city == "-" then
		city = nil
	end

	local json = answer:gsub("^.-rigs|", ""):gsub("\n", "")
	local data, err = decode(json)
	if data == nil then
		return nil, "gc's rig list could not be read: " .. tostring(err)
	end

	local rigs = {}
	for _, rig in ipairs(type(data.rigs) == "table" and data.rigs or {}) do
		if type(rig) == "table" and type(rig.name) == "string" then
			rigs[#rigs + 1] = {
				name = rig.name,
				path = rig.path,
				suspended = rig.suspended == true,
				hq = rig.hq == true,
			}
		end
	end

	return { city = city, rigs = rigs }
end

-- Asks the pane's machine. `opts.city` names the city, `opts.cwd` the directory
-- to ask from. Returns the answer, or nil and a reason.
function M.find(settings, ctx, opts)
	opts = opts or {}
	local answer, err = probe.run(ctx, script(settings, opts.city), { cwd = opts.cwd })
	if answer == nil then
		return nil, err
	end
	return M.parse(answer.output)
end

-- The gc arguments for suspending or resuming each named rig.
function M.commands(kind, names)
	local commands = {}
	for _, name in ipairs(names) do
		commands[#commands + 1] = { "rig", kind, name }
	end
	return commands
end

return M
