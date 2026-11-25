-- The arguments for `gc mail send`. run.lua turns them into the pane command.

local text = require("gascity.text")

local M = {}

-- The subject line shown in the recipient's inbox: the whole message collapsed
-- to one line, cut short when it is long.
function M.subject_for(body, width)
	return text.truncate(body, width or 72)
end

-- The `gc mail send` words, for tests and for the shell script below.
function M.args(settings, body, subject)
	local args = { "mail", "send", settings.recipient, "-s", subject, "-m", body }
	if settings.notify then
		args[#args + 1] = "--notify"
	end
	return args
end

return M
