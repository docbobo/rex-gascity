-- The city rules the extension writes as shell text, in one place.
--
-- Each builder returns POSIX sh lines. The caller places them and emits its own
-- output, so a pane and a test run the same text. The literal `case` text for
-- `~` lives only here.

local M = {}

-- The lines that walk up from the working directory to the city, leaving its
-- path in `name`, or nothing when no ancestor marks one.
--
-- A directory with `city.toml` or `.gc` marks the city. `on_found` runs on a
-- match, before the loop breaks. It defaults to assigning `name`.
function M.city_walk(name, on_found)
	local body = on_found or { name .. '="$d"' }
	local lines = {
		name .. "=",
		'd="$(pwd)"',
		'while [ -n "$d" ]; do',
		'\tif [ -f "$d/city.toml" ] || [ -d "$d/.gc" ]; then',
	}
	for _, line in ipairs(body) do
		lines[#lines + 1] = "\t\t" .. line
	end
	lines[#lines + 1] = "\t\tbreak"
	lines[#lines + 1] = "\tfi"
	lines[#lines + 1] = '\t[ "$d" = "/" ] && break'
	lines[#lines + 1] = '\td="$(dirname "$d")"'
	lines[#lines + 1] = "done"
	return lines
end

-- The lines that expand a leading `~` in `$name`, with the home of the shell
-- that runs the script. A bare `~` and `~/...` both expand. Other paths stay.
function M.tilde(name)
	return {
		'case "$' .. name .. '" in',
		'\t"~") ' .. name .. '="$HOME" ;;',
		'\t"~/"*) ' .. name .. '="$HOME/${' .. name .. '#\\~/}" ;;',
		"esac",
	}
end

return M
