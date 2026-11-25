-- Sending a message by putting the command in a small floating pane.

local mail = require("gascity.mail")
local pane = require("gascity.pane")

local M = {}

-- Returns the new layer's id, or nil and an error.
function M.message(app, ctx, opts)
	local subject = opts.subject
	if subject == nil or subject == "" then
		subject = mail.subject_for(opts.body, app.config.subject_width)
	end

	local script = app.run.script(app.config, { mail.args(app.config, opts.body, subject) }, {
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
