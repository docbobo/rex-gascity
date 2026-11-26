-- The shell fragments the extension writes, in one place.
--
-- Each fragment is POSIX sh, like the scripts that embed it. One home means a
-- test can run the same text a pane runs, instead of comparing strings.

local M = {}

-- The lines that walk up from the working directory to the city, leaving its
-- directory in `variable`, or nothing when no marker is found.
function M.walk_up_city(variable)
	return {
		variable .. "=",
		'd="$(pwd)"',
		'while [ -n "$d" ]; do',
		'\tif [ -f "$d/city.toml" ] || [ -d "$d/.gc" ]; then ' .. variable .. '="$d"; break; fi',
		'\t[ "$d" = "/" ] && break',
		'\td="$(dirname "$d")"',
		"done",
	}
end

-- gc's city list, one absolute path per line. Any absolute path counts, so
-- this does not depend on how gc lines its table up.
function M.cities_awk()
	return "awk '{ sub(/^[ \\t]+/, \"\"); if ($0 ~ /^\\//) print }'"
end

-- Reads rows back as one line, since the answer is read off the screen, where
-- a long line is broken into rows.
function M.one_line_json()
	return "tr -d '\\n'"
end

return M
