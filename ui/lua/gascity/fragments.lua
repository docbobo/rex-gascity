-- The shell fragments the extension writes, in one place.
--
-- Each fragment is POSIX sh, like the scripts that embed it. One home means a
-- test can run the same text a pane runs, instead of comparing strings.

local shell = require("gascity.shell")

local M = {}

-- The lines that walk up from the working directory to the city. The rule
-- lives in `gascity.shell`; this name is what the scripts already call.
function M.walk_up_city(variable)
	return shell.city_walk(variable)
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
