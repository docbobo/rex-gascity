-- Running gc on the machine that owns the pane.
--
-- Every call here is made against the session the action was run for, and Rex
-- routes it to the server that owns that session. So the command runs on the
-- machine that owns the pane, whatever machine the action itself was evaluated
-- on. That is why nothing here has to know which machine it is running on.

local gc = require("gascity.gc")
local probe = require("gascity.probe")
local text = require("gascity.text")

local M = {}

local TERMINAL = "com.superlogical.terminal.shell"

-- The lines that put the city in $city, expanding a leading ~. This runs on the
-- machine that has the city, so that is the home it means.
local function city_lines(city)
	return {
		"city=" .. text.quote(city),
		'case "$city" in',
		'\t"~/"*) city="$HOME/${city#\\~/}" ;;',
		"esac",
	}
end

-- A POSIX shell script that runs gc commands in order, stopping at the first
-- that fails.
--
-- `commands` is a list of argument lists, each following the gc program name.
-- `opts.city` makes every one of them use that city.
--
-- The work sits in a function so that a missing gc returns instead of exiting:
-- the reader of a hidden block needs the status printed after the work, and an
-- `exit` would skip it.
function M.script(settings, commands, opts)
	opts = opts or {}
	local city = opts.city ~= nil and opts.city ~= ""

	local lines = {}
	local function add(line)
		lines[#lines + 1] = line
	end

	if city then
		for _, line in ipairs(city_lines(opts.city)) do
			add(line)
		end
	end
	for _, line in ipairs(gc.words(settings)) do
		add(line)
	end

	add("work() {")
	for _, line in ipairs(gc.guard({ exiting = false })) do
		add("\t" .. line)
	end

	local parts = {}
	for _, words in ipairs(commands) do
		local quoted = {}
		for _, word in ipairs(words) do
			quoted[#quoted + 1] = text.quote(word)
		end
		local prefix = city and '--city "$city" ' or ""
		parts[#parts + 1] = '"$gc" ' .. prefix .. table.concat(quoted, " ")
	end
	add("\t" .. table.concat(parts, " && "))
	add("}")
	add("work")

	return table.concat(lines, "\n")
end

-- Opens `opts.script` in a small floating pane of the session `ctx` names.
-- `opts.cwd`, `opts.label`, and `opts.bounds` shape it. Returns the layer id,
-- or nil and an error.
function M.pane(ctx, opts)
	local result, err = rex.session.new_layer({
		session_id = ctx.session_id,
		bounds = opts.bounds or { 0.15, 0.25, 0.7, 0.4 },
		focus = false,
		layout = rex.layout.block({
			flavor = TERMINAL,
			label = opts.label or "gascity",
			options = {
				cwd = opts.cwd,
				command = { "/bin/sh", "-c", opts.script },
			},
		}),
	})
	if result == nil then
		return nil, tostring(err)
	end
	return result.layer_id
end

-- Opens an interactive shell block, a split in the session `ctx` names.
-- `opts.cwd` is the directory it starts in. Returns the block id, or nil
-- and an error.
function M.shell(ctx, opts)
	local block, err = rex.session.new_block({
		session_id = ctx.session_id,
		flavor = TERMINAL,
		label = opts.label or "gascity",
		options = { cwd = opts.cwd },
	})
	if block == nil then
		return nil, tostring(err)
	end
	return block.block_id
end

-- Shows `opts.text` in a floating pane that stays up until it is closed.
--
-- The text is printed with `%s`, so escape sequences in it are colours rather
-- than something to interpret, and the pane is kept after the program exits so
-- there is something to read.
function M.show(ctx, opts)
	local result, err = rex.session.new_layer({
		session_id = ctx.session_id,
		bounds = opts.bounds or { 0.08, 0.16, 0.84, 0.56 },
		focus = true,
		layout = rex.layout.block({
			flavor = TERMINAL,
			label = opts.label or "gascity",
			options = {
				cwd = opts.cwd,
				command = { "/bin/sh", "-c", "printf '%s\\n' " .. text.quote(opts.text) },
				exit = { on_completion = false },
			},
		}),
	})
	if result == nil then
		return nil, tostring(err)
	end
	return result.layer_id
end

-- Runs the commands in a hidden block and waits for them, so nothing flashes up
-- on the screen. Returns `{ output = ..., status = ... }`, or nil and an error.
--
-- `opts` is the same as `M.script`, plus `opts.cwd` and `opts.patience`.
function M.quiet(ctx, settings, commands, opts)
	opts = opts or {}
	local answer, err = probe.run(ctx, M.script(settings, commands, opts), {
		cwd = opts.cwd,
		patience = opts.patience,
	})
	if answer == nil then
		return nil, err
	end
	return answer
end

return M
