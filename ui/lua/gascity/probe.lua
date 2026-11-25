-- Running a command on the machine that owns the session, without a pane.
--
-- Sometimes the action needs something from the pane's machine that its own Lua
-- cannot see: whether the pane is inside a city, which cities or rigs that
-- machine has, or the result of a command that should not show a pane at all.
-- A block runs there. This makes one that is never placed anywhere, runs a
-- shell command in it, reads what it printed and how it exited, and closes it
-- again.
--
-- An action cannot wait: Rex has no sleep or wait for one. It can run a command
-- that waits, so each turn of the loop below runs /bin/sleep, then reads the
-- block.

local M = {}

local codec = require("gascity.codec")

local TERMINAL = "com.superlogical.terminal"
local NAP = "0.05"
local TURNS = 60 -- a turn is 50 ms, so three seconds at most
local PATIENCE = 400 -- and twenty seconds for something slow

-- Runs `script` in a hidden block of the session `ctx` names.
--
-- `opts.cwd` sets the directory it runs in, and `opts.patience` allows a longer
-- wait for a slow command. Returns `{ output = ..., status = ... }`, or nil and
-- a reason when there is no answer.
function M.run(ctx, script, opts)
	opts = opts or {}

	local block, err = rex.session.new_block({
		session_id = ctx.session_id,
		flavor = TERMINAL .. ".shell",
		label = "gascity-probe",
		options = {
			cwd = opts.cwd,
			shell = "none",
			command = { "/bin/sh", "-c", codec.wrap(script) },
		},
	})
	if block == nil then
		return nil, tostring(err)
	end

	local block_id = block.block_id
	local function close()
		rex.call("block.close", { session_id = ctx.session_id, block_id = block_id })
	end

	for _ = 1, opts.patience and PATIENCE or TURNS do
		rex.exec({ "/bin/sleep", NAP })
		local screen = rex.block.call(TERMINAL, "format", { block_id = block_id, trim = true })
		local content = screen and (screen.content or screen.text)
		if content ~= nil then
			local answer, reason = codec.read(content)
			if reason ~= nil then
				close()
				return nil, reason
			end
			if answer ~= nil then
				close()
				return answer
			end
		end
	end

	close()
	return nil, "the machine did not answer in time"
end

return M
