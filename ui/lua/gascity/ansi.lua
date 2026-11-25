-- Colours for a terminal pane.
--
-- Escape sequences are written into the text itself and printed with `%s`, so
-- nothing reinterprets them: a path with a backslash stays a path.

local M = {}

M.reset = "\27[0m"

-- `codes` is one or more SGR parameters, such as "1" or "38;5;81".
function M.paint(codes, s)
	return "\27[" .. codes .. "m" .. s .. M.reset
end

function M.bold(s)
	return M.paint("1", s)
end

function M.dim(s)
	return M.paint("2", s)
end

-- A colour from the 256-colour palette.
function M.color(index, s)
	return M.paint("38;5;" .. index, s)
end

return M
