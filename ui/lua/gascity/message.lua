-- A message: its subject, its `gc mail send` words, and the floating pane
-- that runs them.

local pane = require("gascity.pane")
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

-- Returns the new layer's id, or nil and an error.
function M.message(app, ctx, opts)
	local subject = opts.subject
	if subject == nil or subject == "" then
		subject = M.subject_for(opts.body, app.config.subject_width)
	end

	local script = app.run.script(app.config, { M.args(app.config, opts.body, subject) }, {
		city = opts.city,
	})

	local layer_id, err = app.run.pane(ctx, {
		script = script,
		label = "gascity-mail",
		cwd = pane.cwd(ctx.block_id),
		bounds = { 0.15, 0.25, 0.7, 0.4 },
	})
	if layer_id == nil then
		return nil, err
	end

	return layer_id, nil, subject
end

return M
