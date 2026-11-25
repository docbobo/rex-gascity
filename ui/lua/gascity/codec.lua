-- The pane-machine answer format, in both directions.
--
-- A hidden block runs a script between two markers. The end marker carries the
-- script's exit status. The script prints fields as `name|value` rows. This
-- module builds the envelope and the field rows, and reads both back.

local M = {}

local BEGIN_MARK = "<<gascity-begin>>"
local END_MARK = "<<gascity-end"

-- The script between the markers. The end marker carries the exit status.
function M.wrap(script)
	return table.concat({
		"printf '\\n" .. BEGIN_MARK .. "\\n'",
		script,
		"status=$?",
		'printf "\\n' .. END_MARK .. ' %s>>\\n" "$status"',
		-- Rex keeps a block that still has a program in it; this one waits to
		-- be closed, so the answer is there to read.
		"sleep 30",
	}, "\n")
end

-- Reads the markers back. Returns `{ output = ..., status = ... }`, or nil
-- when the answer is not complete. A complete answer with an unreadable status
-- returns nil and a reason.
function M.read(screen)
	local first = screen:find(BEGIN_MARK, 1, true)
	if first == nil then
		return nil
	end
	local last = screen:find(END_MARK, first, true)
	if last == nil then
		return nil
	end

	local output = screen:sub(first + #BEGIN_MARK, last - 1):gsub("^%s+", ""):gsub("%s+$", "")
	local status = screen:sub(last + #END_MARK):match("^%s*(%-?%d+)>>")
	if status == nil then
		return nil, "the machine's answer ended without a readable exit status"
	end
	return { output = output, status = tonumber(status) }
end

-- Prints one field row: `name|value`.
function M.emit(name, shell_expression)
	return "printf '" .. name .. "|%s\\n' " .. shell_expression
end

-- Prints the start of a field row, for a value that follows on the next line.
function M.emit_pipe(name)
	return "printf '" .. name .. "|'"
end

-- Splits output into an ordered list of `{ kind, value }`. A row that starts
-- with a known `name|` begins an entry; any other row continues the previous
-- one. Rows before the first known kind are dropped.
-- ponytail: a row that starts with a known `name|` inside JSON reads as a new field. Upgrade to a length-prefixed or base64 field when a value can contain one.
function M.rows(output, names)
	local known = {}
	for _, name in ipairs(names) do
		known[name] = true
	end
	local rows = {}
	for line in (output .. "\n"):gmatch("([^\n]*)\n") do
		local kind, value = line:match("^(%a+)|(.*)$")
		if kind ~= nil and known[kind] then
			rows[#rows + 1] = { kind = kind, value = value }
		elseif #rows > 0 then
			rows[#rows].value = rows[#rows].value .. line
		end
	end
	return rows
end

return M
