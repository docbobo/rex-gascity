-- String helpers shared by the Gas City extension.

local M = {}

-- Removes leading and trailing whitespace.
function M.trim(s)
	if type(s) ~= "string" then
		return ""
	end
	return (s:gsub("^%s+", ""):gsub("%s+$", ""))
end

-- Collapses runs of whitespace, newlines included, into single spaces.
function M.collapse(s)
	return (M.trim(s):gsub("%s+", " "))
end

function M.basename(path)
	if type(path) ~= "string" then
		return ""
	end
	return (path:gsub("/+$", ""):match("[^/]+$")) or path
end

-- Quotes one word for a POSIX shell, so it arrives as a single argument with
-- no expansion. The result is always single-quoted.
function M.quote(word)
	return "'" .. (tostring(word):gsub("'", "'\\''")) .. "'"
end

-- The longest prefix of `s` that is at most `width` bytes and ends on a whole
-- UTF-8 character, so cutting a message never leaves half a letter behind.
local function safe_prefix(s, width)
	if #s <= width then
		return s
	end
	local cut = width
	while cut > 0 do
		local byte = s:byte(cut + 1)
		if byte ~= nil and byte >= 0x80 and byte <= 0xBF then
			cut = cut - 1 -- a continuation byte: the character starts earlier
		else
			break
		end
	end
	return s:sub(1, cut)
end

-- Cuts `s` to one line of at most `width` bytes, preferring a word boundary,
-- and marks the cut with an ellipsis.
function M.truncate(s, width)
	s = M.collapse(s)
	if #s <= width then
		return s
	end
	local head = safe_prefix(s, width)
	local space = head:match("^.*() ") -- last space in the prefix
	if space ~= nil and space > width / 2 then
		head = head:sub(1, space - 1)
	end
	return (head:gsub("[%s,;:%-]+$", "")) .. "…"
end

return M
