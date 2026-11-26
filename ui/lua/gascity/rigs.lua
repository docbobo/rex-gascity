-- The rigs of the city the pane belongs to.
--
-- The question goes to the pane's machine, in a block, because the city and
-- its rigs are there.

local codec = require("gascity.codec")
local fragments = require("gascity.fragments")
local gc = require("gascity.gc")
local probe = require("gascity.probe")
local shell = require("gascity.shell")
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
		for _, line in ipairs(shell.tilde("city")) do
			add(line)
		end
	else
		-- No city named: the pane's directory decides, as it does for gc.
		add("city=")
		for _, line in ipairs(shell.city_walk("city")) do
			add(line)
		end
	end

	for _, line in ipairs(gc.words(settings)) do
		add(line)
	end

	-- The guard returns instead of exiting, so the script still prints its end
	-- marker and the warning reaches the screen.
	add("has_gc() {")
	for _, line in ipairs(gc.guard({ exiting = false })) do
		add("\t" .. line)
	end
	add("}")
	add("missing=")
	add("if ! has_gc; then")
	add("\tmissing=1")
	add("fi")

	add('if [ -n "$city" ]; then set -- --city "$city"; else set --; fi')
	add(codec.emit("city", '"${city:--}"'))
	add('if [ -n "$missing" ]; then')
	add("\tprintf '@@gc|missing\\n'")
	add("\tprintf '@@rigs|\\n'")
	add("else")
	-- gc's stderr is kept, so a real failure is reported in gc's own words.
	add('\trigs_json=$("$gc" "$@" rig list --json 2>&1 | ' .. fragments.one_line_json() .. ')')
	add("\t" .. codec.emit("rigs", '"$rigs_json"'))
	add("fi")

	return table.concat(lines, "\n")
end

-- Reads the answer into `{ city = path or nil, rigs = { ... } }`.
function M.parse(answer)
	-- The codec joins the rows the terminal wrapped. Take the first value of
	-- each field.
	local rows, reason = codec.rows(answer, { "city", "rigs", "gc" })
	if rows == nil then
		return nil, reason
	end
	local values = {}
	for _, row in ipairs(rows) do
		if values[row.kind] == nil then
			values[row.kind] = codec.value(row)
		end
	end

	local city = values.city
	if city == "-" then
		city = nil
	end

	if values.gc == "missing" then
		return nil, gc.MISSING
	end

	local data, err = rex.json.decode(values.rigs or "")
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
