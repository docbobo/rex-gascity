-- The pane an action was run from.

local M = {}

local TERMINAL = "com.superlogical.terminal"

-- The directory the block's program is in, as the shell last reported it.
-- Returns nil when no block is known or the program never reported one.
--
-- The pane can belong to a session on another machine. That is fine: the calls
-- in this action go to the server that owns the session, so this directory is
-- meaningful to whatever runs there.
function M.cwd(block_id)
	local args = {}
	if block_id ~= nil then
		args.block_id = block_id
	end
	local info = rex.block.call(TERMINAL, "process", args)
	if info == nil then
		return nil
	end
	local foreground = info.foreground or info
	return foreground and foreground.cwd or nil
end

return M
