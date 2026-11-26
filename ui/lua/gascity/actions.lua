-- The actions the extension adds, the keys they are bound to, and the menu that
-- gathers them.

local gc = require("gascity.gc")
local text = require("gascity.text")

local M = {}

-- The prompt API landed in control API revision 17. An older server has no
-- rex.ui, or a rex.ui without the function. That is "unsupported", not a crash
-- and not an error: the caller returns without sending anything.
local function prompt(kind, opts)
	local ui = rex.ui
	local fn = type(ui) == "table" and ui[kind] or nil
	if type(fn) ~= "function" then
		rex.log(
			"warn",
			"rex.ui." .. kind .. " needs control API revision 17. This server is older"
		)
		return nil, "unsupported"
	end
	return fn(opts)
end

-- Asks which city to send to when the pane is in none of them and the machine
-- has more than one. Returns the chosen path, or nil when nothing was chosen.
local function choose(cities)
	if #cities == 1 then
		return cities[1]
	end

	local items = {}
	for _, path in ipairs(cities) do
		items[#items + 1] = {
			id = path,
			title = text.basename(path),
			subtitle = path,
			icon = "folder",
		}
	end

	local picked, why = prompt("pick", {
		title = "Which city?",
		placeholder = "Filter cities",
		confirm = "Send",
		items = items,
	})
	if picked == nil then
		if why ~= "cancelled" and why ~= "unsupported" then
			rex.log("warn", "gascity_mail_mayor: " .. tostring(why or "no answer"))
		end
		return nil
	end
	return picked
end

-- The rigs named in an argument, such as "alpha, beta", or nil when there is
-- no argument to name them with.
local function named_rigs(list)
	if list == nil or text.trim(list) == "" then
		return nil
	end
	local names = {}
	for name in tostring(list):gmatch("[^,]+") do
		name = text.trim(name)
		if name ~= "" then
			names[#names + 1] = name
		end
	end
	return names
end

-- The Rex icon set is closed: an unknown name is dropped.
local RIG_ICONS = { suspend = "clock", resume = "plus" }

-- Asks which rigs to act on. Returns the names, or nil when nothing was chosen.
local function ask(rigs, kind)
	local suspending = kind == "suspend"

	local items = {}
	for _, rig in ipairs(rigs) do
		local detail = rig.suspended and "suspended" or "running"
		if rig.hq then
			detail = detail .. ", HQ"
		end
		items[#items + 1] = {
			id = rig.name,
			title = rig.name,
			subtitle = rig.path,
			detail = detail,
			icon = RIG_ICONS[kind],
		}
	end

	local names, why = prompt("pick", {
		title = suspending and "Suspend which rigs?" or "Resume which rigs?",
		placeholder = "Filter rigs",
		multiple = true,
		confirm = suspending and "Suspend" or "Resume",
		items = items,
	})
	if names == nil then
		if why ~= "cancelled" and why ~= "unsupported" then
			rex.log("warn", "gascity_rigs_" .. kind .. ": " .. tostring(why or "no answer"))
		end
		return nil
	end
	if type(names) ~= "table" then
		return nil
	end
	return names
end

-- The commands, in the order the menu shows them.
--
-- Each is one action's work: the palette, a key, and the menu all reach it
-- through `run`. The menu calls these directly rather than through
-- rex.invoke, because a prompt does not work in an action that rex.invoke
-- runs, and every one of these asks something.
local function commands(app, settings)
	local function mail(ctx, args)
		local body = args.body
		if body == nil then
			local answer, why = prompt("input", {
				title = "Message the mayor",
				placeholder = "What should the mayor know?",
				confirm = "Send to " .. settings.recipient,
			})
			if answer == nil then
				if why ~= "cancelled" and why ~= "unsupported" then
					rex.log("warn", "gascity_mail_mayor: " .. tostring(why or "no answer"))
				end
				return nil
			end
			body = answer
		end

		body = text.trim(body)
		if body == "" then
			return nil
		end

		-- A named city wins. Otherwise the pane's own city does, and only when
		-- the pane is in none does it ask - which takes a question to the
		-- pane's machine, since the cities are there.
		local city = args.city or settings.city
		if city == nil then
			local found = app.machine.ask(settings, ctx, { cwd = app.pane.cwd(ctx.block_id) })
			if found ~= nil and found.city == nil and #found.cities > 0 then
				city = choose(found.cities)
				if city == nil then
					return nil
				end
			end
		end

		local layer_id, err, subject = app.send.message(app, ctx, {
			body = body,
			subject = args.subject,
			city = city,
		})
		if layer_id == nil then
			error(err, 0)
		end

		rex.log("info", "gascity: sending to " .. settings.recipient .. ": " .. subject)
		return {
			output = "Sending to " .. settings.recipient .. " in " .. (city or "the pane's city"),
			recipient = settings.recipient,
			subject = subject,
			city = city,
			layer_id = layer_id,
		}
	end

	-- Ask the mayor for a status update. The pane's machine names the pack's
	-- binding and the session's server; the mail names them to the mayor,
	-- whose answer opens a floating pane through the pack's status-update
	-- command. This action renders nothing itself.
	local function status_update(ctx, args)
		args = args or {}
		local cwd = app.pane.cwd(ctx.block_id)

		-- The city, in the same order as mail.
		local city = args.city or settings.city
		if city == nil then
			local found = app.machine.ask(settings, ctx, { cwd = cwd })
			if found ~= nil and found.city == nil and #found.cities > 0 then
				city = choose(found.cities)
				if city == nil then
					return nil
				end
			end
		end

		local display = args.display or settings.status_display or "text"

		local found, err = app.status.ask(settings, ctx, { city = city, cwd = cwd })
		if found == nil then
			error(err, 0)
		end
		local where = city or found.city or "the pane's city"

		-- gc first, before the binding: pack_binding skips only the import
		-- lookup, so a missing gc is an error with or without it.
		if found.gc_missing then
			error(gc.MISSING, 0)
		end

		local binding = app.status.binding(found.imports, settings.pack_binding)
		if binding == nil then
			if found.config_status ~= nil and found.config_status ~= 0 then
				error(
					"gc config show failed for " .. where .. " (exit " .. found.config_status .. "): "
						.. (found.error or ""),
					0
				)
			end
			error(
				where .. " does not import the rex-gascity pack. Import it (docs/rex-runtime-pack.md), or set pack_binding in "
					.. settings.file,
				0
			)
		end
		if found.server == nil then
			error(
				"could not find the Rex server that owns session "
					.. tostring(ctx.session_id)
					.. (found.server_error and (": " .. found.server_error) or ""),
				0
			)
		end

		local layer_id, send_err = app.send.message(app, ctx, {
			body = app.status.body(binding, found.server, ctx.session_id, display),
			subject = app.status.SUBJECT,
			city = city,
		})
		if layer_id == nil then
			error(send_err, 0)
		end

		rex.log("info", "gascity: asked " .. settings.recipient .. " for a status update")
		return {
			output = "Asked " .. settings.recipient .. " for a status update in " .. where,
			city = city,
			binding = binding,
			server = found.server,
			session_id = ctx.session_id,
			display = display,
		}
	end

	-- Suspend or resume rigs. Without names, the pane's machine is asked which
	-- rigs it has, since the city and its rigs are there.
	local function rigs_move(ctx, args, kind)
		local suspending = kind == "suspend"
		local city = args.city or settings.city
		local names = named_rigs(args.rigs)

		-- Naming the rigs skips both the question and the trip to the pane's
		-- machine: gc finds the city from the pane's directory.
		if names == nil then
			local found, err = app.rigs.find(settings, ctx, {
				city = city,
				cwd = app.pane.cwd(ctx.block_id),
			})
			if found == nil then
				error(err, 0)
			end
			if #found.rigs == 0 then
				error("no rigs listed in " .. (found.city or "the city of this pane"), 0)
			end
			city = found.city
			names = ask(found.rigs, kind)
			if names == nil then
				return nil
			end
		end
		if #names == 0 then
			return nil
		end

		-- Hidden: the work runs in a block that is never placed, so nothing
		-- flashes up. Its output is the answer instead, and a failure comes
		-- back as this action's error.
		local answer, run_err = app.run.quiet(ctx, settings, app.rigs.commands(kind, names), {
			city = city,
			cwd = app.pane.cwd(ctx.block_id),
			patience = true,
		})
		if answer == nil then
			error(run_err, 0)
		end

		local printed = text.trim(answer.output or "")
		if answer.status ~= 0 then
			rex.log("error", "gascity: " .. kind .. ": " .. printed)
			local reason = printed
			if reason == "" then
				reason = "gc exited with status " .. answer.status
			end
			error(reason, 0)
		end

		local list = table.concat(names, ", ")
		rex.log("info", "gascity: " .. kind .. " " .. list)
		return {
			output = printed ~= "" and printed or ((suspending and "Suspended " or "Resumed ") .. list),
			city = city,
			rigs = names,
		}
	end

	local function rigs_open(ctx, args)
		args = args or {}
		local found, err = app.rigs.find(settings, ctx, {
			city = args.city or settings.city,
			cwd = app.pane.cwd(ctx.block_id),
		})
		if found == nil then
			error(err, 0)
		end
		if #found.rigs == 0 then
			error("no rigs listed in " .. (found.city or "the city of this pane"), 0)
		end

		-- A rig without a path cannot be opened, so it is not offered.
		local items = {}
		for _, rig in ipairs(found.rigs) do
			if type(rig.path) == "string" then
				items[#items + 1] = {
					id = rig.name,
					title = rig.name,
					subtitle = rig.path,
					icon = "package",
				}
			end
		end

		local name = args.rig
		if type(name) == "string" and text.trim(name) ~= "" then
			name = text.trim(name)
		else
			if #items == 0 then
				error("no rig listed in " .. (found.city or "the city of this pane") .. " has a path", 0)
			end
			local picked, why = prompt("pick", {
				title = "Open which rig?",
				placeholder = "Filter rigs",
				confirm = "Open",
				items = items,
			})
			if picked == nil then
				if why ~= "cancelled" and why ~= "unsupported" then
					rex.log("warn", "gascity_rigs_open: " .. tostring(why or "no answer"))
				end
				return nil
			end
			name = picked
		end

		local rig
		for _, candidate in ipairs(found.rigs) do
			if candidate.name == name then
				rig = candidate
				break
			end
		end
		if rig == nil then
			error(
				"no rig named " .. tostring(name) .. " in " .. (found.city or "the city of this pane"),
				0
			)
		end
		if type(rig.path) ~= "string" then
			error("rig " .. tostring(name) .. " has no path", 0)
		end

		local block_id, open_err = app.run.shell(ctx, {
			cwd = rig.path,
			label = "gascity:" .. rig.name,
		})
		if block_id == nil then
			error(open_err, 0)
		end

		rex.log("info", "gascity: opened " .. rig.name .. " in " .. rig.path)
		return {
			output = "Opened " .. rig.name .. " in " .. rig.path,
			city = found.city,
			rig = rig.name,
			cwd = rig.path,
			block_id = block_id,
		}
	end

	-- Open the workstream dashboard of the city or rig this pane is in. The
	-- pane's directory names the rig, so nothing is picked and no path is
	-- rewritten. The binding is the city's import key, never a literal.
	local function dashboard(ctx, args)
		args = args or {}
		local cwd = app.pane.cwd(ctx.block_id)
		if cwd == nil then
			error("could not read the directory of this pane", 0)
		end

		local found, err = app.status.ask(settings, ctx, { cwd = cwd })
		if found == nil then
			error(err, 0)
		end

		-- gc first, as in status_update: a missing gc is its own message.
		if found.gc_missing then
			error(gc.MISSING, 0)
		end

		-- No ancestor city.toml/.gc, or gc reported no city: do not open an
		-- unrelated dashboard.
		if found.city == nil then
			local detail = (found.error ~= nil and found.error ~= "")
					and (" (" .. found.error .. ")")
				or ""
			error("no Gas City city or rig contains " .. cwd .. detail, 0)
		end

		local binding = app.status.binding(found.imports, settings.pack_binding)
		if binding == nil then
			if found.config_status ~= nil and found.config_status ~= 0 then
				error(
					"gc config show failed for " .. found.city .. " (exit " .. found.config_status .. "): "
						.. (found.error or ""),
					0
				)
			end
			error(
				found.city .. " does not import the rex-gascity pack. Import it (docs/rex-runtime-pack.md), or set pack_binding in "
					.. settings.file,
				0
			)
		end

		local script = app.run.script(settings, { { binding, "dashboard" } })
		local block_id, open_err = app.run.shell(ctx, {
			cwd = cwd,
			label = "gascity:dashboard",
			command = { "/bin/sh", "-c", script },
			exit = { on_completion = false },
		})
		if block_id == nil then
			error(open_err, 0)
		end

		rex.log("info", "gascity: opened the workstream dashboard in " .. cwd)
		return {
			output = "Opened the workstream dashboard in " .. cwd,
			cwd = cwd,
			binding = binding,
			block_id = block_id,
		}
	end

	local function doctor(ctx)
		local facts = app.report.facts(app, ctx.block_id, ctx)
		local printed = app.report.text(facts)
		rex.log("info", "gascity report:\n" .. printed)

		-- The app does not show what an action returns, so a pane shows the
		-- report to whoever is at the app. Anything run without a person -
		-- rex do, a script - gets the text as its result instead.
		if ctx.origin == "key" or ctx.origin == "palette" then
			local _, err = app.run.show(ctx, {
				text = app.report.terminal(facts, { footer = "Close this pane when you are done." }),
				label = "gascity-report",
				cwd = app.pane.cwd(ctx.block_id),
			})
			if err ~= nil then
				rex.log("error", "gascity_doctor: " .. tostring(err))
				error(err, 0)
			end
		end

		return { output = printed }
	end

	return {
		{
			id = "mail_mayor",
			name = "gascity_mail_mayor",
			title = "Message the Mayor…",
			description = "Send a mail message to the mayor of the city the pane is in.",
			keywords = "gascity mayor mail message send nudge",
			icon = "person",
			args = { body = "string?", subject = "string?", city = "string?" },
			run = function(ctx, args)
				return mail(ctx, args)
			end,
		},
		{
			id = "status_update",
			name = "gascity_status_update",
			title = "Ask the Mayor for a Status Update…",
			description = "Ask the mayor for a status update, shown in a floating pane of this session.",
			keywords = "gascity mayor status update report progress",
			icon = "document",
			args = { city = "string?", display = "string?" },
			run = function(ctx, args)
				return status_update(ctx, args)
			end,
		},
		{
			id = "rigs_suspend",
			name = "gascity_rigs_suspend",
			title = "Suspend a Rig…",
			description = "Suspend rigs of the city the pane belongs to.",
			keywords = "gascity rig suspend agents pause stop",
			icon = RIG_ICONS.suspend,
			args = { city = "string?", rigs = "string?" },
			run = function(ctx, args)
				return rigs_move(ctx, args, "suspend")
			end,
		},
		{
			id = "rigs_resume",
			name = "gascity_rigs_resume",
			title = "Resume a Rig…",
			description = "Resume rigs of the city the pane belongs to.",
			keywords = "gascity rig resume agents start",
			icon = RIG_ICONS.resume,
			args = { city = "string?", rigs = "string?" },
			run = function(ctx, args)
				return rigs_move(ctx, args, "resume")
			end,
		},
		{
			id = "rigs_open",
			name = "gascity_rigs_open",
			title = "Open a Rig…",
			description = "Open a terminal pane in a rig's directory.",
			keywords = "gascity rig open pane shell terminal directory cd",
			icon = "package",
			args = { city = "string?", rig = "string?" },
			run = function(ctx, args)
				return rigs_open(ctx, args)
			end,
		},
		{
			id = "dashboard",
			name = "gascity_dashboard",
			title = "Open the Workstream Dashboard",
			description = "Open the workstream dashboard of the city or rig this pane is in.",
			keywords = "gascity dashboard workstream status board hq",
			icon = "document",
			run = function(ctx, args)
				return dashboard(ctx, args)
			end,
		},
		{
			id = "doctor",
			name = "gascity_doctor",
			title = "Where This Runs…",
			description = "Report the machine, checkout, settings, and pane that this extension sees.",
			keywords = "gascity doctor where host machine diagnose",
			icon = "server",
			run = function(ctx)
				return doctor(ctx)
			end,
		},
	}
end

-- Registers the actions. `app` holds the resolved `config` and the modules.
function M.register(app)
	local list = commands(app, app.config)

	for _, command in ipairs(list) do
		rex.action({
			name = command.name,
			title = "Gas City: " .. command.title,
			description = command.description,
			category = "Gas City",
			keywords = command.keywords,
			args = command.args,
			run = command.run,
		})
	end

	-- The command palette is the app's, and it opens with no filter: its action
	-- takes no arguments, so a key cannot ask it for one category. This is that
	-- key instead - the Gas City commands, and nothing else.
	rex.action({
		name = "gascity_menu",
		title = "Gas City: Commands…",
		description = "Every Gas City command in one list.",
		category = "Gas City",
		keywords = "gascity menu commands list all",
		run = function(ctx)
			local items = {}
			for _, command in ipairs(list) do
				items[#items + 1] = {
					id = command.id,
					title = command.title,
					subtitle = command.description,
					icon = command.icon,
				}
			end

			local picked, why = prompt("pick", {
				title = "Gas City",
				placeholder = "Filter commands",
				confirm = "Run",
				items = items,
			})
			if picked == nil then
				if why ~= "cancelled" and why ~= "unsupported" then
					rex.log("warn", "gascity_menu: " .. tostring(why or "no answer"))
				end
				return nil
			end

			for _, command in ipairs(list) do
				if command.id == picked then
					-- Called here, not through rex.invoke: only a direct call
					-- may ask a question.
					return command.run(ctx, {})
				end
			end
			return nil
		end,
	})

	rex.bind("ctrl+b>g", "gascity_menu")
end

return M
