-- The pane-machine answer format, in both directions.
--
-- A hidden block runs a script between two markers. The end marker carries the
-- script's exit status. The script prints fields as `@@name|escaped-value`
-- rows. This module builds the envelope and the field rows, and reads both
-- back. It is the one owner of the row framing.

local M = {}

local BEGIN_MARK = "<<gascity-begin>>"
local END_MARK = "<<gascity-end"

-- The script between the markers. The end marker carries the exit status.
-- The shell `esc` function goes first, so every generated script can call it.
-- The `esc` function must be one line: the reader takes a line without `@@`
-- as a malformed row. `read` drops that first line when a row follows it.
function M.wrap(script)
	return table.concat({
		"printf '\\n" .. BEGIN_MARK .. "\\n'",
		M.SHELL_ESCAPE,
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

	local output = screen:sub(first + #BEGIN_MARK, last - 1):gsub("^[\r\n]+", "")
	-- `wrap` defines the `esc` function before the script. A shell prints that
	-- line only when it echoes commands, so drop that one line and keep any
	-- text the script printed after it, such as gc's own error.
	output = output:gsub("^esc%([^\n]*\n?", "", 1):gsub("[\r\n]+$", "")
	local status = screen:sub(last + #END_MARK):match("^%s*(%-?%d+)>>")
	if status == nil then
		return nil, "the machine's answer ended without a readable exit status"
	end
	return { output = output, status = tonumber(status) }
end

-- The row marker. Every row starts its own line with it, so a row split
-- across lines rejoins when the lines are joined. A value never holds a raw
-- `@@`.
M.ROW_MARK = "@@"

-- The awk program cannot hold a newline where its text follows a closing
-- quote, so the escape table is re-stated here as a table. The test "the shell
-- esc function matches the Lua escape table" keeps the two in step.
local AWK_ESCAPES = {
	{ "%", "%25" },
	{ "@", "%40" },
	{ "<", "%3C" },
	{ "|", "%7C" },
	{ "\\t", "%09" },
	{ "\\n", "%0A" },
	{ "\\r", "%0D" },
}

-- The awk `enc` function. It is the one escape table: the shell encoder and
-- the Lua decoder both come from it.
local awk_enc = {
	"function enc(v, r, i, n, ch) {",
	'r = ""; n = length(v);',
	"for (i = 1; i <= n; i++) {",
	"ch = substr(v, i, 1);",
}
for _, pair in ipairs(AWK_ESCAPES) do
	awk_enc[#awk_enc + 1] = 'if (ch == "' .. pair[1] .. '") r = r "' .. pair[2] .. '"; else'
end
awk_enc[#awk_enc + 1] = "r = r ch }"
awk_enc[#awk_enc + 1] = "return r }"
M.AWK_ENCODE = table.concat(awk_enc, "\n")

-- The POSIX sh `esc` function. It runs the awk `enc` over one value, read
-- through ENVIRON so backslashes and newlines stay intact. The awk program
-- sits on one line so the function stays one line: a line without `@@` is a
-- malformed row.
M.SHELL_ESCAPE = "esc() { V=$1 awk '"
	.. M.AWK_ENCODE:gsub("\n", " ")
	.. " BEGIN { printf \"%s\", enc(ENVIRON[\"V\"]) }'; }"

local ESCAPES = {
	["%"] = "%25",
	["@"] = "%40",
	["<"] = "%3C",
	["|"] = "%7C",
	["\t"] = "%09",
	["\n"] = "%0A",
	["\r"] = "%0D",
}

local UNESCAPES = {
	["25"] = "%",
	["40"] = "@",
	["3C"] = "<",
	["7C"] = "|",
	["09"] = "\t",
	["0A"] = "\n",
	["0D"] = "\r",
}

-- Escapes a value so it cannot split a row at `@@` or `|`, or end the answer
-- early.
function M.escape(value)
	return (tostring(value):gsub("[%%@<|\t\n\r]", ESCAPES))
end

-- Reads an escaped value back. Returns nil and a reason for a `%` that is not
-- followed by two hex digits, or for an unknown `%xx` code.
function M.unescape(value)
	local bad = nil
	local out = (value or ""):gsub("%%(..)", function(code)
		if code:match("^%x%x$") and UNESCAPES[code:upper()] then
			return UNESCAPES[code:upper()]
		end
		bad = bad or code
		return ""
	end)
	if bad == nil then
		-- A `%` at the very end has no two hex digits to read. The tail is
		-- `%` and at most two hex digits. A `%` followed by two hex digits
		-- was already read above, so a leftover tail is a bad escape.
		-- A `%` followed by two hex digits was read above. A longer tail is
		-- two hex digits and then more text, so only a short tail is left.
		local tail = (value or ""):match("%%([^%%]*)$")
		if tail ~= nil and tail ~= "" and #tail < 2 and tail:match("^%x*$") then
			bad = tail
		end
	end
	if bad ~= nil then
		return nil, "bad escape in a row: %" .. bad
	end
	return out
end

-- Prints one field row: `@@name|escaped-value`. The value is a shell word.
function M.emit(name, shell_word)
	return "printf '@@" .. name .. "|%s\\n' \"$(esc " .. shell_word .. ")\""
end

-- Splits output into an ordered list of `{ kind, value }`. `value` is the raw
-- escaped rest. A line that starts with the marker begins a row, and so does a
-- line of the form `name|` for a known name, so an answer from a program that
-- prints no marker still reads. Any other line continues the row above,
-- because a terminal wrap adds a newline that is not part of the value.
-- Values are escaped, so no value can start a row of its own. Text before the
-- first row is dropped. Returns nil and a reason for a malformed row.
-- With `opts.marker_only`, only the marker begins a row. A wrapped value can
-- then carry a tail such as `city|...` without posing as a bare row; a caller
-- whose program always marks its rows uses this mode (status).
function M.rows(output, names, opts)
	local marker_only = opts ~= nil and opts.marker_only == true
	local known = {}
	for _, name in ipairs(names) do
		known[name] = true
	end
	local rows = {}
	local current = nil
	local carry = ""
	local function flush()
		if current == nil then
			return true
		end
		local blob = current
		local kind, rest = blob:match("^([%a_]+)|(.*)$")
		if kind == nil then
			return nil, "a row has no name and bar: " .. blob
		end
		if known[kind] then
			local _, reason = M.unescape(rest)
			if reason ~= nil then
				return nil, reason
			end
			rows[#rows + 1] = { kind = kind, value = rest }
		end
		current = nil
		return true
	end
	-- Bind the string to a local first: the server's Lua VM panics when
	-- gmatch iterates a temporary that the loop body allocates over.
	local text = (output or ""):gsub("\r", "") .. "\n"
	for line in text:gmatch("([^\n]*)\n") do
		line = carry .. line
		carry = ""
		-- A wrap between the marker's two characters leaves a lone `@`.
		if #line < #M.ROW_MARK and M.ROW_MARK:sub(1, #line) == line and line ~= "" then
			carry = line
		else
			local marked = line:sub(1, #M.ROW_MARK) == M.ROW_MARK
			local body = marked and line:sub(#M.ROW_MARK + 1) or line
			local kind = body:match("^([%a_]+)%|")
			if marked or (not marker_only and kind ~= nil and known[kind]) then
				local ok, reason = flush()
				if not ok then
					return nil, reason
				end
				current = body
			elseif current ~= nil then
				current = current .. line
			end
		end
	end
	local ok, reason = flush()
	if not ok then
		return nil, reason
	end
	return rows
end

-- The unescaped value of a row. Returns nil and a reason for a bad escape.
function M.value(row)
	return M.unescape(row.value)
end

-- Splits a row's escaped rest at `|` and unescapes each part. The parts are
-- escaped, so the split is exact.
function M.parts(value)
	local parts = {}
	local text = (value or "") .. "|"
	for part in text:gmatch("(.-)|") do
		local value, reason = M.unescape(part)
		if reason ~= nil then
			return nil, reason
		end
		parts[#parts + 1] = value
	end
	return parts
end

return M
