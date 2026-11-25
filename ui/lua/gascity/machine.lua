-- What the machine that owns the pane is: its name, the city the pane is in,
-- and the cities it has.
--
-- The question is asked of that machine, in a block, because the action's own
-- Lua may run somewhere else entirely.

local gc = require("gascity.gc")
local probe = require("gascity.probe")

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

	add('printf \'host|%s\\n\' "$(hostname 2>/dev/null)"')

	add('d="$(pwd)"')
	add('while [ -n "$d" ]; do')
	add('\tif [ -f "$d/city.toml" ] || [ -d "$d/.gc" ]; then printf \'pane|%s\\n\' "$d"; break; fi')
	add('\t[ "$d" = "/" ] && break')
	add('\td="$(dirname "$d")"')
	add("done")

	-- Any absolute path in gc's list counts, so this does not depend on how gc
	-- lines its table up. A gc that does not answer simply lists nothing.
	add('"$gc" cities 2>/dev/null | awk \'{ sub(/^[ \\t]+/, ""); if ($0 ~ /^\\//) print "city|" $0 }\'')
	return table.concat(lines, "\n")
end

-- Reads the answer into `{ host = name or nil, city = path or nil, cities = { paths } }`.
function M.parse(output)
	local answer = { host = nil, city = nil, cities = {} }
	local seen = {}

	for line in (output or ""):gmatch("[^\n]+") do
		local kind, value = line:match("^(%a+)|(.+)$")
		if kind == "host" then
			answer.host = value
		elseif kind == "pane" then
			answer.city = value
		elseif kind == "city" and not seen[value] then
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
