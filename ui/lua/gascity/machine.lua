-- What the machine that owns the pane is: its name, the city the pane is in,
-- and the cities it has.
--
-- The question is asked of that machine, in a block, because the action's own
-- Lua may run somewhere else entirely.

local codec = require("gascity.codec")
local fragments = require("gascity.fragments")
local gc = require("gascity.gc")
local probe = require("gascity.probe")
local shell = require("gascity.shell")

local M = {}

-- The shell that answers it, one fact per line, separated by a bar.
--
-- Not a tab: the block's answer is read back as the screen the terminal
-- rendered, where tabs are already widened to spaces.
local function script(settings)
	local lines = gc.words(settings)
	local function add(line)
		lines[#lines + 1] = line
	end

	add("has_gc() {")
	for _, line in ipairs(gc.guard({ exiting = false })) do
		add("\t" .. line)
	end
	add("}")
	add("missing=")
	add("if ! has_gc")
	add("then missing=1")
	add("fi")

	add(codec.emit("host", '"$(hostname 2>/dev/null)"'))

	for _, line in ipairs(shell.city_walk("d", { codec.emit("pane", '"$d"') })) do
		add(line)
	end

	-- Any absolute path in gc's list counts, so this does not depend on how gc
	-- lines its table up. The list is best-effort, so its stderr is dropped:
	-- it would land after the pane row and corrupt that row.
	add('if [ -z "$missing" ]; then')
	add('\t"$gc" cities 2>/dev/null | ' .. fragments.cities_awk() .. ' | while IFS= read -r path; do')
	add('\t\t' .. codec.emit("city", '"$path"'))
	add("\tdone")
	add("fi")
	return table.concat(lines, "\n")
end

-- Reads the answer into `{ host = name or nil, city = path or nil, cities = { paths } }`.
-- A malformed answer returns nil and a reason.
function M.parse(output)
	local rows, reason = codec.rows(output or "", { "host", "pane", "city" })
	if rows == nil then
		return nil, reason
	end
	local answer = { host = nil, city = nil, cities = {} }
	local seen = {}

	for _, row in ipairs(rows) do
		local value = codec.value(row)
		if row.kind == "host" then
			answer.host = value
		elseif row.kind == "pane" then
			answer.city = value
		elseif row.kind == "city" and not seen[value] then
			seen[value] = true
			answer.cities[#answer.cities + 1] = value
		end
	end

	return answer
end

-- Asks the pane's machine. `opts.cwd` is the directory to ask from. Returns the
-- answer, or nil and a reason.
function M.ask(settings, ctx, opts)
	opts = opts or {}
	local answer, err = probe.run(ctx, script(settings), { cwd = opts.cwd })
	if answer == nil then
		return nil, err
	end
	return M.parse(answer.output)
end

return M
