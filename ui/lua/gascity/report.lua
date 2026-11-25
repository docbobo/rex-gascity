-- What the extension sees, for the question "why is this not working here?".
--
-- The settings and the checkout belong to the machine that evaluates the
-- action. The pane belongs to whichever machine owns the session, and the mail
-- runs there. Those can be different machines, so each line says which is
-- which.
--
-- The report is built as facts, so it can be shown as a list in the app or
-- printed as text by the CLI.

local ansi = require("gascity.ansi")
local text = require("gascity.text")

local M = {}

local function readable(path)
	local f = io.open(path, "r")
	if f then
		f:close()
		return true
	end
	return false
end

local function hostname()
	local result = rex.exec({ "hostname" }, { timeout = 2 })
	if result ~= nil and result.ok then
		local name = text.trim(result.stdout or "")
		if name ~= "" then
			return name
		end
	end
	return "unknown"
end

-- The lines of the report. Each is `{ label, value, icon }`. `ctx` is the
-- action's context, when there is one.
function M.facts(app, block_id, ctx)
	local settings = app.config
	local facts = {}
	local function add(label, value, icon)
		facts[#facts + 1] = { label = label, value = value, icon = icon }
	end

	-- Two machines are in play: the one running this action, with its settings
	-- and checkout, and the one that owns the pane. Naming both is the point of
	-- this report - one name on its own reads like the wrong machine.
	local this_machine = hostname()
	add("runs on", this_machine .. "  (this action)", "laptop")

	-- The machine with the panes answers for itself, and for its cities.
	local cwd = app.pane.cwd(block_id)
	local found, find_err = app.machine.ask(settings, ctx or {}, { cwd = cwd })
	if found == nil then
		add("panes", "the machine did not answer: " .. tostring(find_err), "screens")
	elseif found.host ~= nil and found.host == this_machine then
		add("panes", found.host .. "  (the same machine)", "screens")
	else
		add("panes", (found.host or "unnamed") .. "  (the session's machine)", "screens")
	end

	add("checkout", app.root, "folder")
	add("settings", settings.file .. (readable(settings.file) and "" or "  (missing)"), "document")

	if cwd == nil then
		add("directory", "the pane reported none", "terminal")
	else
		-- Base this on the machine that owns the pane, not on whether the local
		-- process can read the path: a remote pane can name a path that happens
		-- to be readable here, and that path is still not on this machine.
		local where
		if found == nil then
			where = "machine not named"
		elseif found.host == this_machine then
			where = "on this machine"
		else
			where = "on another machine"
		end
		add("directory", cwd .. "  (" .. where .. ")", "terminal")
	end

	if found == nil then
		add("cities", "not asked: the machine did not answer", "globe")
	else
		add("city", found.city or "none above the pane", "star")
		add(
			"cities",
			#found.cities > 0 and table.concat(found.cities, ", ") or "none listed by gc",
			"globe"
		)
	end

	add("mail runs", "in a pane of this session, on the machine that owns it", "link")
	add("gc", settings.gc_bin or "found on PATH, then the usual Nix and Go places", "gear")
	add(
		"sends to",
		settings.recipient .. (settings.notify and "  (with --notify)" or "  (without --notify)"),
		"person"
	)
	if ctx ~= nil then
		add(
			"ran from",
			(ctx.origin or "unknown") .. (ctx.client_id ~= nil and ", as a client" or ""),
			"clock"
		)
	end

	return facts
end

-- The facts as two aligned columns, for the CLI and the log.
function M.text(facts)
	local width = 0
	for _, fact in ipairs(facts) do
		width = math.max(width, #fact.label)
	end

	local lines = {}
	for _, fact in ipairs(facts) do
		lines[#lines + 1] = fact.label .. string.rep(" ", width - #fact.label + 2) .. fact.value
	end
	return table.concat(lines, "\n")
end

-- The icons, two cells wide each, so the columns line up under them.
local ICONS = {
	["runs on"] = "💻",
	panes = "📺",
	checkout = "📁",
	settings = "📄",
	directory = "📂",
	city = "📍",
	cities = "🌇",
	["mail runs"] = "🔗",
	gc = "🔧",
	["sends to"] = "📨",
	["ran from"] = "🕒",
}

-- A line that says something is wrong is worth seeing first.
local function trouble(value)
	return value:find("missing", 1, true) ~= nil
		or value:find("did not answer", 1, true) ~= nil
		or value:find("cannot", 1, true) ~= nil
end

-- The facts for a terminal: icons, colour, and two aligned columns.
--
-- `opts.footer` adds a line at the bottom, dimmed, such as how to close the
-- pane.
function M.terminal(facts, opts)
	opts = opts or {}

	local width = 0
	for _, fact in ipairs(facts) do
		width = math.max(width, #fact.label)
	end

	local lines = {
		"",
		"  " .. ansi.paint("1;38;5;81", "Gas City")
			.. ansi.paint("38;5;245", "    where this runs"),
		"  " .. ansi.paint("38;5;238", string.rep("─", width + 20)),
	}

	for _, fact in ipairs(facts) do
		local icon = ICONS[fact.label] or "•"
		local label = fact.label .. string.rep(" ", width - #fact.label)
		local value = trouble(fact.value) and ansi.color(203, fact.value) or fact.value
		lines[#lines + 1] = "  " .. icon .. "  " .. ansi.color(109, label) .. "  " .. value
	end

	lines[#lines + 1] = ""
	if opts.footer then
		lines[#lines + 1] = "  " .. ansi.color(240, opts.footer)
		lines[#lines + 1] = ""
	end

	return table.concat(lines, "\n")
end

-- The whole report as text. Kept for the CLI and for tests.
function M.build(app, block_id, ctx)
	return M.text(M.facts(app, block_id, ctx))
end

return M
