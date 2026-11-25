-- Tests for the Gas City extension. Runs outside Rex, with a stub `rex`.
--
--   lua tests/run.lua      (from the repository root; Lua 5.1 or LuaJIT)

-- The repository root, worked out from the path this script was called with.
local dir = ((arg and arg[0]) or ""):match("^(.*)/[^/]+$")
local root = dir and dir:gsub("/?tests$", "") or "."
if root == "" then
	root = "."
end

package.path = root .. "/lua/?.lua;" .. root .. "/lua/?/init.lua;" .. package.path

local text = require("gascity.text")
local config = require("gascity.config")
local pane = require("gascity.pane")
local mail = require("gascity.mail")
local rigs = require("gascity.rigs")
local run = require("gascity.run")
local machine = require("gascity.machine")
local probe = require("gascity.probe")
local send = require("gascity.send")
local report = require("gascity.report")
local status = require("gascity.status")
local actions = require("gascity.actions")

local fixture = root .. "/tests/fixtures"

-- The closed icon set Rex accepts. A name outside it is dropped.
local REX_ICONS = {}
for _, name in ipairs({
	"document",
	"folder",
	"branch",
	"terminal",
	"code",
	"server",
	"database",
	"package",
	"globe",
	"link",
	"person",
	"tag",
	"clock",
	"star",
	"gear",
	"plus",
}) do
	REX_ICONS[name] = true
end

-- The stub that stands in for Rex -----------------------------------------

local stub = {
	execs = {},
	logs = {},
	actions = {},
	binds = {},
	layers = {},
	pane_cwd = nil,
	prompt = nil,
	answer = nil,
	why = nil,
	exec_error = nil,
	layer_error = nil,
	format = nil,
	block_error = nil,
	split_error = nil,
	json_text = nil,
	json_value = nil,
	json_error = nil,
	blocks = {},
	splits = {},
	closed = {},
	picks = nil,
	pick_count = 0,
	picked = nil,
	pick_why = nil,
}

function stub.reset()
	stub.execs = {}
	stub.logs = {}
	stub.actions = {}
	stub.binds = {}
	stub.layers = {}
	stub.pane_cwd = nil
	stub.prompt = nil
	stub.answer = nil
	stub.why = nil
	stub.exec_error = nil
	stub.layer_error = nil
	stub.format = nil
	stub.blocks = {}
	stub.splits = {}
	stub.closed = {}
	stub.picks = nil
	stub.pick_count = 0
	stub.picked = nil
	stub.pick_why = nil
	stub.block_error = nil
	stub.split_error = nil
	stub.json_text = nil
	stub.json_value = nil
	stub.json_error = nil
	rex.ui = stub.ui_saved
end

function stub.logged(level, needle)
	for _, entry in ipairs(stub.logs) do
		if entry[1] == level and tostring(entry[2]):find(needle, 1, true) then
			return true
		end
	end
	return false
end

-- The block in the only layer that was created.
function stub.block()
	local layer = stub.layers[1]
	return layer and layer.layout and layer.layout.block or nil
end

local BEGIN_MARK = "<<gascity-begin>>"
local END_MARK = "<<gascity-end"

local function probe_answer(lines, status)
	return "Last login: sometime\n"
		.. BEGIN_MARK
		.. "\n"
		.. table.concat(lines, "\n")
		.. "\n"
		.. END_MARK
		.. " "
		.. tostring(status or 0)
		.. ">>\nsome prompt"
end


rex = {
	exec = function(argv, opts)
		stub.execs[#stub.execs + 1] = { argv = argv, opts = opts }
		if argv[1] == "hostname" then
			return { code = 0, stdout = "test-machine\n", stderr = "", ok = true }
		end
		if stub.exec_error ~= nil then
			return nil, stub.exec_error
		end
		return { code = 0, stdout = "ok\n", stderr = "", ok = true }
	end,
	log = function(level, message)
		stub.logs[#stub.logs + 1] = { level, message }
	end,
	block = {
		call = function(creator, method)
			if method == "format" then
				if type(stub.format) == "table" then
					return { content = table.remove(stub.format, 1) or stub.format[1] }
				end
				return { content = stub.format }
			end
			if stub.pane_cwd == nil then
				return nil
			end
			return { foreground = { cwd = stub.pane_cwd } }
		end,
	},
	call = function(method, args)
		if method == "block.close" then
			stub.closed[#stub.closed + 1] = args.block_id
			return { closed = true }
		end
		return nil, "unexpected call " .. tostring(method)
	end,
	session = {
		new_block = function(args)
			if stub.block_error ~= nil then
				return nil, stub.block_error
			end
			stub.blocks[#stub.blocks + 1] = args
			return { block_id = "block:probe" }
		end,
		new_split = function(args)
			if stub.split_error ~= nil then
				return nil, stub.split_error
			end
			if args.anchor_id == nil and args.window_id == nil then
				return nil, "session.new_split needs an anchor_id or a window_id"
			end
			stub.splits[#stub.splits + 1] = args
			return { window_id = "window:1", block_ids = { "block:probe" } }
		end,
		new_layer = function(args)
			if stub.layer_error ~= nil then
				return nil, stub.layer_error
			end
			stub.layers[#stub.layers + 1] = args
			return { layer_id = "layer:1" }
		end,
	},
	layout = {
		block = function(spec)
			return { block = spec }
		end,
	},
	action = function(definition)
		stub.actions[definition.name] = definition
	end,
	bind = function(key, name)
		stub.binds[key] = name
	end,
	json = {
		decode = function(source)
			stub.json_text = source
			return stub.json_value, stub.json_error
		end,
	},
	ui = {
		pick = function(opts)
			stub.picks = opts
			stub.pick_count = stub.pick_count + 1
			if stub.picked == nil then
				return nil, stub.pick_why or "cancelled"
			end
			return stub.picked
		end,
		input = function(opts)
			stub.prompt = opts
			if stub.answer == nil then
				return nil, stub.why or "cancelled"
			end
			return stub.answer
		end,
	},
}

-- The prompt API is saved so a case can remove it and reset restores it.
stub.ui_saved = rex.ui

-- How many warnings a case logged.
local function warns()
	local count = 0
	for _, entry in ipairs(stub.logs) do
		if entry[1] == "warn" then
			count = count + 1
		end
	end
	return count
end

-- A tiny test runner -----------------------------------------------------

local failures = 0
local checks = 0

local function fail(message)
	failures = failures + 1
	print("  FAIL " .. message)
end

local function equal(got, want, what)
	checks = checks + 1
	if got ~= want then
		fail(string.format("%s: want %s, got %s", what, tostring(want), tostring(got)))
	end
end

local function truthy(got, what)
	checks = checks + 1
	if not got then
		fail(what .. ": want a value, got " .. tostring(got))
	end
end

local function falsy(got, what)
	checks = checks + 1
	if got then
		fail(what .. ": want nil or false, got " .. tostring(got))
	end
end

local function contains(haystack, needle, what)
	checks = checks + 1
	if type(haystack) ~= "string" or haystack:find(needle, 1, true) == nil then
		fail(string.format("%s: %q is not in:\n%s", what, needle, tostring(haystack)))
	end
end

local function same(got, want, what)
	checks = checks + 1
	local ok = got ~= nil
	if ok then
		for index, value in ipairs(want) do
			if got[index] ~= value then
				ok = false
			end
		end
		if #got ~= #want then
			ok = false
		end
	end
	if not ok then
		fail(
			string.format(
				"%s: want [%s], got [%s]",
				what,
				table.concat(want, " | "),
				table.concat(got or {}, " | ")
			)
		)
	end
end

local function test(name, fn)
	print(name)
	stub.reset()
	fn()
end

-- Settings ---------------------------------------------------------------

test("config: defaults when no file is named", function()
	local settings = config.resolve(nil)
	equal(settings.recipient, "mayor", "recipient")
	equal(settings.notify, true, "notify")
	equal(settings.gc_bin, nil, "gc_bin")
end)

test("config: a settings file overrides the defaults", function()
	local settings = config.resolve(fixture .. "/settings.lua")
	equal(settings.recipient, "mayor", "recipient")
	equal(settings.notify, false, "notify")
	equal(settings.subject_width, 40, "subject_width")
	equal(settings.gc_bin, "/opt/gc/bin/gc", "gc_bin")
end)

test("config: a missing file is not an error", function()
	local settings = config.resolve(fixture .. "/nope.lua")
	equal(settings.recipient, "mayor", "recipient")
	equal(#stub.logs, 0, "no warnings")
end)

test("config: a broken file is reported and ignored", function()
	local settings = config.resolve(fixture .. "/broken.lua")
	equal(settings.recipient, "mayor", "recipient")
	truthy(stub.logged("warn", "cannot read"), "warns about the broken file")
end)

test("config: an unknown setting is reported", function()
	config.resolve(fixture .. "/unknown.lua")
	truthy(stub.logged("warn", "unknown setting 'colour'"), "warns about the unknown setting")
end)

test("config: a setting of the wrong type is reported", function()
	local settings = config.resolve(fixture .. "/wrongtype.lua")
	equal(settings.notify, true, "keeps the default")
	truthy(stub.logged("warn", "notify"), "names the setting")
end)

test("config: a leading ~ is expanded", function()
	local settings = config.resolve(nil)
	settings.gc_bin = "~/bin/gc"
	settings = config.resolve(fixture .. "/tilde.lua")
	equal(settings.gc_bin, (os.getenv("HOME") or "") .. "/bin/gc", "expanded")
end)

test("config: a city keeps its ~ for the session machine to expand", function()
	local settings = config.resolve(fixture .. "/tilde.lua")
	equal(settings.city, "~/cities/two", "the city is not expanded here")
end)

test("config: a home with a percent is expanded literally", function()
	equal(config.expand("~/city", "/home/a%0"), "/home/a%0/city", "the percent is kept")
	equal(config.expand("~", "/home/a%0"), "/home/a%0", "a bare tilde too")
end)

test("config: the status settings have defaults", function()
	local settings = config.resolve(nil)
	equal(settings.pack_binding, nil, "pack_binding")
	equal(settings.status_display, "text", "status_display")
end)

test("config: the status settings can be set", function()
	local settings = config.resolve(fixture .. "/status.lua")
	equal(settings.pack_binding, "mine", "pack_binding")
	equal(settings.status_display, "markdown", "status_display")
	equal(#stub.logs, 0, "no warnings")
end)

test("config: a status setting of the wrong type is reported", function()
	local settings = config.resolve(fixture .. "/status-wrongtype.lua")
	equal(settings.pack_binding, nil, "pack_binding keeps its default")
	equal(settings.status_display, "text", "status_display keeps its default")
	truthy(stub.logged("warn", "pack_binding"), "names pack_binding")
	truthy(stub.logged("warn", "status_display"), "names status_display")
end)

-- Text -------------------------------------------------------------------

test("text: trim and collapse", function()
	equal(text.trim("  a b  "), "a b", "trim")
	equal(text.collapse(" a\n\n b\tc "), "a b c", "collapse")
	equal(text.basename("/a/b/city"), "city", "basename")
	equal(text.basename("/a/b/city/"), "city", "basename with a trailing slash")
end)

test("text: quote keeps one word one argument", function()
	equal(text.quote("plain"), "'plain'", "plain")
	equal(text.quote("it's"), "'it'\\''s'", "an inner quote")
	equal(text.quote("$HOME `x`"), "'$HOME `x`'", "no expansion")
end)

test("text: truncate keeps short text", function()
	equal(text.truncate("Build is green", 20), "Build is green", "short text")
end)

test("text: truncate cuts at a word and marks it", function()
	equal(text.truncate("the quick brown fox jumps", 14), "the quick…", "cuts at a space")
	equal(text.truncate("the quick brown fox jumps", 9), "the quick…", "a space at the edge counts")
end)

test("text: truncate never splits a UTF-8 character", function()
	local subject = text.truncate("Grüße aus der Stadt", 8)
	equal(subject:sub(-3), "…", "ends with an ellipsis")
	truthy(subject:find("Grüße", 1, true) == 1, "keeps whole characters")
end)

-- The pane ---------------------------------------------------------------

test("pane: returns the directory the program reported", function()
	stub.pane_cwd = "/work"
	equal(pane.cwd("block:1"), "/work", "cwd")
end)

test("pane: returns nil when nothing reported one", function()
	equal(pane.cwd("block:1"), nil, "cwd")
end)

-- The shell helper -------------------------------------------------------

test("The shell helper: a visible split in the chosen directory", function()
	local id = run.shell({ session_id = "session:1", block_id = "block:1" }, { cwd = "/work/alpha", label = "gascity:alpha" })
	equal(id, "block:probe", "returns the block id")

	local split = stub.splits[1]
	equal(split.session_id, "session:1", "the session it was asked for")
	equal(split.anchor_id, "block:1", "split beside the block in ctx")
	equal(split.direction, "horizontal", "side by side")
	equal(split.side, "after", "to the right")
	equal(split.focus, true, "the window is made active")
	local block = split.layout.block
	equal(block.flavor, "com.superlogical.terminal.shell", "a terminal")
	equal(block.label, "gascity:alpha", "the label")
	equal(block.options.cwd, "/work/alpha", "the directory, unchanged")
	equal(block.options.command, nil, "no command, so it is interactive")
end)

test("The shell helper: an error from the session is returned", function()
	stub.split_error = "boom"
	local id, err = run.shell({ session_id = "session:1", block_id = "block:1" }, { cwd = "/work/alpha" })
	equal(id, nil, "no block")
	equal(err, "boom", "the error, as a string")
end)

-- Mail -------------------------------------------------------------------

test("mail: the subject is the message on one line", function()
	equal(mail.subject_for("Build is green", 72), "Build is green", "subject")
	equal(mail.subject_for("build\nis\tgreen", 72), "build is green", "collapsed")
end)

test("mail: the argv for gc mail send", function()
	local settings = config.resolve(nil)
	same(
		mail.args(settings, "Build is green", "Build is green"),
		{ "mail", "send", "mayor", "-s", "Build is green", "-m", "Build is green", "--notify" },
		"argv"
	)

	settings.notify = false
	same(
		mail.args(settings, "hello", "hello"),
		{ "mail", "send", "mayor", "-s", "hello", "-m", "hello" },
		"argv without --notify"
	)
end)

-- The script a pane runs ------------------------------------------------

test("run: quotes every word, so a message arrives as it was typed", function()
	local script = run.script(config.resolve(nil), {
		mail.args(config.resolve(nil), "it's $HOME", "it's $HOME"),
	})
	contains(script, "'mail' 'send' 'mayor'", "quoted words")
	contains(script, "'it'\\''s $HOME'", "the message as one quoted argument")
	contains(script, "--notify", "the nudge")
end)

test("run: several commands run in order, stopping at the first failure", function()
	local script = run.script(config.resolve(nil), {
		{ "rig", "suspend", "alpha" },
		{ "rig", "suspend", "beta" },
	})
	contains(script, "'rig' 'suspend' 'alpha' && \"$gc\" 'rig' 'suspend' 'beta'", "one after the other")
end)

test("run: a city adds --city to every command and expands a leading ~", function()
	local script = run.script(
		config.resolve(nil),
		{ { "rig", "suspend", "alpha" }, { "rig", "suspend", "beta" } },
		{ city = "~/cities/two" }
	)
	contains(script, "city='~/cities/two'", "the city as given")
	contains(script, 'city="$HOME/${city#\\~/}"', "expanded on the machine that runs it")
	contains(script, '--city "$city" \'rig\' \'suspend\' \'alpha\'', "before the first command")
	contains(script, '&& "$gc" --city "$city" \'rig\' \'suspend\' \'beta\'', "and the second")
end)

test("run: without a city the pane decides", function()
	local script = run.script(config.resolve(nil), { { "rig", "list" } })
	falsy(script:find("--city", 1, true), "no --city")
	falsy(script:find("city=", 1, true), "no city variable")
end)

test("run: a set gc_bin is used as it is", function()
	local settings = config.resolve(nil)
	settings.gc_bin = "/opt/gc/bin/gc"
	local script = run.script(settings, { { "rig", "list" } })
	contains(script, "gc='/opt/gc/bin/gc'", "the configured program")
	falsy(script:find("command -v gc", 1, true), "no search for a program")
end)

test("run: a script that cannot find gc says so and stops", function()
	local script = run.script(config.resolve(nil), { { "rig", "list" } })
	contains(script, "gc is not on this machine", "the message")
	contains(script, "return 127", "stops")
	equal(script:sub(-5), "\nwork", "and the work is what the script runs")
end)

test("run: without gc_bin the script looks in the usual places", function()
	local script = run.script(config.resolve(nil), { { "rig", "list" } })
	contains(script, "gc=gc", "the PATH name")
	contains(script, "/run/current-system/sw/bin/gc", "the Nix place")
	contains(script, "$HOME/.nix-profile/bin/gc", "the profile place")
end)

test("run: each fallback program path is quoted", function()
	local script = run.script(config.resolve(nil), { { "rig", "list" } })
	contains(script, '"$HOME/.local/bin/gc"', "a space in HOME cannot split the path")
	contains(script, '"$HOME/go/bin/gc"', "and the next one")
end)

test("probe: never stops before its end marker", function()
	stub.format = probe_answer({})
	machine.ask(config.resolve(nil), { session_id = "session:1" }, { cwd = "/work" })
	falsy(stub.blocks[1].options.command[3]:find("exit", 1, true), "no early exit")
end)

-- Sending ----------------------------------------------------------------

local function app_with(overrides)
	local app = {
		root = "/repo",
		config = config.resolve(nil),
		pane = pane,
		machine = machine,
		rigs = rigs,
		run = run,
		send = send,
		report = report,
		status = status,
	}
	for key, value in pairs(overrides or {}) do
		app[key] = value
	end
	return app
end

local function run_action(args, ctx)
	actions.register(app_with({}))
	local context = ctx or { block_id = "block:1", session_id = "session:1" }
	local ok, result = pcall(stub.actions.gascity_mail_mayor.run, context, args or {})
	return ok, result
end

test("action: it is registered, bound, and takes optional arguments", function()
	run_action({})
	local definition = stub.actions.gascity_mail_mayor
	truthy(definition, "registered")
	equal(definition.title, "Gas City: Message the Mayor…", "title")
	equal(definition.args.body, "string?", "body is optional")
	equal(stub.binds["ctrl+b>g"], "gascity_menu", "the key opens the Gas City menu")
end)

test("action: puts the command in a pane of the same session", function()
	stub.pane_cwd = "/work/city/rig"
	local ok, result = run_action({ body = "Build is green" })
	truthy(ok, "no error: " .. tostring(result))
	equal(result.recipient, "mayor", "recipient")
	equal(result.subject, "Build is green", "subject")
	equal(result.layer_id, "layer:1", "layer")

	local layer = stub.layers[1]
	equal(layer.session_id, "session:1", "the session it was run for")
	equal(layer.focus, false, "does not take the focus")

	local block = stub.block()
	equal(block.flavor, "com.superlogical.terminal.shell", "flavor")
	equal(block.label, "gascity-mail", "label")
	equal(block.options.cwd, "/work/city/rig", "runs in the pane's directory")
	equal(block.options.shell, nil, "Rex runs the program and closes the pane on success")
	equal(block.options.command[1], "/bin/sh", "the shell doing the work")
	contains(block.options.command[3], "'Build is green'", "the message")
	contains(block.options.command[3], "'mail' 'send' 'mayor'", "the command")
end)

test("action: a city argument overrides the pane", function()
	stub.pane_cwd = "/work/one"
	local ok, result = run_action({ body = "hello", city = "/cities/two" })
	truthy(ok, "no error: " .. tostring(result))
	equal(result.city, "/cities/two", "the city")
	contains(stub.block().options.command[3], "city='/cities/two'", "passed to the script")
	contains(stub.block().options.command[3], "--city", "as --city")
end)

test("action: the city setting is the default for that argument", function()
	stub.pane_cwd = "/work/one"
	local app = app_with({})
	app.config.city = "/cities/set"
	actions.register(app)
	local ok, result = pcall(stub.actions.gascity_mail_mayor.run, { block_id = "block:1" }, { body = "hello" })
	truthy(ok, "no error: " .. tostring(result))
	equal(result.city, "/cities/set", "the setting")
	contains(stub.block().options.command[3], "--city", "as --city")
end)

test("action: a pane inside a city sends there without asking", function()
	stub.pane_cwd = "/work/one"
	stub.format = probe_answer({ "pane|/work/one", "city|/cities/two" })
	local ok, result = run_action({ body = "hello" })
	truthy(ok, "no error: " .. tostring(result))
	equal(stub.picks, nil, "did not ask")
	equal(result.city, nil, "no city named")
	falsy(stub.block().options.command[3]:find("--city", 1, true), "gc walks up itself")
end)

test("action: one city on the machine is used without asking", function()
	stub.pane_cwd = "/work/nowhere"
	stub.format = probe_answer({ "city|/cities/one" })
	local ok, result = run_action({ body = "hello" })
	truthy(ok, "no error: " .. tostring(result))
	equal(stub.picks, nil, "did not ask")
	equal(result.city, "/cities/one", "the only city")
end)

test("action: several cities are offered", function()
	stub.pane_cwd = "/work/nowhere"
	stub.format = probe_answer({ "city|/cities/one", "city|/cities/two" })
	stub.picked = "/cities/two"
	local ok, result = run_action({ body = "hello" })
	truthy(ok, "no error: " .. tostring(result))
	truthy(stub.picks ~= nil, "asked")
	equal(stub.picks.title, "Which city?", "the question")
	equal(#stub.picks.items, 2, "both cities")
	equal(stub.picks.items[1].id, "/cities/one", "the first")
	equal(stub.picks.items[2].title, "two", "named after its directory")
	equal(result.city, "/cities/two", "the chosen city")
	contains(stub.block().options.command[3], "--city", "passed on")
end)

test("action: cancelling the city question sends nothing", function()
	stub.pane_cwd = "/work/nowhere"
	stub.format = probe_answer({ "city|/cities/one", "city|/cities/two" })
	stub.picked = nil
	local ok, result = run_action({ body = "hello" })
	truthy(ok, "no error")
	equal(result, nil, "no result")
	equal(#stub.layers, 0, "no pane")
end)

test("action: a machine that does not answer sends without a city", function()
	stub.pane_cwd = "/work/one"
	stub.block_error = "session not found"
	local ok, result = run_action({ body = "hello" })
	truthy(ok, "no error: " .. tostring(result))
	equal(result.city, nil, "no city named")
	equal(#stub.layers, 1, "still sends")
end)

test("action: a subject argument is kept", function()
	stub.pane_cwd = "/work/city"
	local ok, result = run_action({ body = "a long message", subject = "Short" })
	truthy(ok, "no error: " .. tostring(result))
	equal(result.subject, "Short", "subject")
	contains(stub.block().options.command[3], "'Short'", "the subject in the command")
end)

test("action: without a body it asks, then sends the answer", function()
	stub.pane_cwd = "/work/city"
	stub.answer = "  Status: all green  "
	local ok, result = run_action({})
	truthy(ok, "no error: " .. tostring(result))
	truthy(stub.prompt ~= nil, "asked")
	equal(result.subject, "Status: all green", "the answer is trimmed")
end)

test("action: a cancelled prompt sends nothing", function()
	stub.answer = nil
	local ok, result = run_action({})
	truthy(ok, "no error")
	equal(result, nil, "no result")
	equal(#stub.layers, 0, "no pane")
	equal(#stub.logs, 0, "cancelling is quiet")
end)

test("action: an empty message sends nothing", function()
	local ok, result = run_action({ body = "   " })
	truthy(ok, "no error")
	equal(result, nil, "no result")
	equal(#stub.layers, 0, "no pane")
end)

test("action: a pane that cannot be made is reported", function()
	stub.pane_cwd = "/work"
	stub.layer_error = "session not found"
	local ok, result = run_action({ body = "hello" })
	falsy(ok, "reports an error")
	contains(tostring(result), "session not found", "the reason")
end)

test("action: it works without a pane directory", function()
	stub.pane_cwd = nil
	local ok, result = run_action({ body = "hello" })
	truthy(ok, "no error: " .. tostring(result))
	equal(stub.block().options.cwd, nil, "no directory passed on")
end)

-- Which city -------------------------------------------------------------

test("probe: returns what the block printed, and how it exited", function()
	stub.format = probe_answer({ "pane|/work/city" }, 0)
	local answer, err = probe.run({ session_id = "session:1" }, "echo hi")
	equal(err, nil, "no error")
	equal(answer.output, "pane|/work/city", "the output")
	equal(answer.status, 0, "the status")
	equal(#stub.blocks, 1, "one block")
	equal(stub.blocks[1].options.cwd, nil, "no directory asked for")
	equal(stub.closed[1], "block:probe", "the block is closed again")
end)

test("probe: a failed command reports its status", function()
	stub.format = probe_answer({ "gc: nope" }, 1)
	local answer = probe.run({ session_id = "session:1" }, "false")
	equal(answer.status, 1, "the status")
	equal(answer.output, "gc: nope", "the output")
end)

test("probe: runs the script in the directory it is given", function()
	stub.format = probe_answer({ "x" })
	probe.run({ session_id = "session:1" }, "echo hi", { cwd = "/work" })
	equal(stub.blocks[1].options.cwd, "/work", "the directory")
	contains(stub.blocks[1].options.command[3], "echo hi", "the script")
end)

test("probe: gives up and closes the block when nothing answers", function()
	stub.format = BEGIN_MARK .. "\nbut no end"
	local answer, err = probe.run({ session_id = "session:1" }, "sleep")
	equal(answer, nil, "no answer")
	contains(err, "did not answer", "the reason")
	equal(stub.closed[1], "block:probe", "the block is closed again")
end)

test("probe: reports a block that could not be made", function()
	stub.block_error = "session not found"
	local answer, err = probe.run({ session_id = "session:1" }, "echo hi")
	equal(answer, nil, "no answer")
	contains(err, "session not found", "the reason")
end)

test("machine: reads the name, the pane's city, and the listed cities", function()
	local answer = machine.parse(
		"host|Mac-Mini\npane|/work/city\ncity|/a\ncity|/b\ncity|/a"
	)
	equal(answer.host, "Mac-Mini", "the machine's name")
	equal(answer.city, "/work/city", "the city the pane is in")
	same(answer.cities, { "/a", "/b" }, "the listed cities, without repeats")
end)

test("machine: an empty answer has nothing", function()
	local answer = machine.parse("")
	equal(answer.host, nil, "no name")
	equal(answer.city, nil, "no pane city")
	equal(#answer.cities, 0, "no cities")
end)

test("machine: asks for its name, the pane's city, and gc's list", function()
	stub.format = probe_answer({ "host|Mac-Mini", "pane|/work/city" })
	local answer = machine.ask(config.resolve(nil), { session_id = "session:1" }, { cwd = "/work" })
	equal(answer.host, "Mac-Mini", "the name")
	local script = stub.blocks[1].options.command[3]
	contains(script, "hostname", "asks for the name")
	contains(script, "city.toml", "looks for the pane's city")
	contains(script, "cities", "asks gc for its list")
end)

test("machine: reads a city path with a space in it", function()
	local answer = machine.parse("city|/srv/Gas City")
	equal(answer.cities[1], "/srv/Gas City", "the space is kept")
end)

test("machine: the city list is parsed without splitting on spaces", function()
	stub.format = probe_answer({ "host|Mac-Mini" })
	machine.ask(config.resolve(nil), { session_id = "session:1" }, { cwd = "/work" })
	local script = stub.blocks[1].options.command[3]
	contains(script, "awk", "parses whole path fields")
	falsy(script:find("tr -s", 1, true), "does not split fields on white space")
end)

-- Rigs -------------------------------------------------------------------

local RIG_JSON = '{"city_name":"one","rigs":[{"name":"alpha","path":"/work/alpha","suspended":false},{"name":"beta","path":"/work/beta","suspended":true,"hq":true}]}'

test("rigs: reads the city and the rigs out of the answer", function()
	local answer = rigs.parse("city|/cities/one\nrigs|" .. RIG_JSON, function(source)
		stub.json_text = source
		return { rigs = {
			{ name = "alpha", path = "/work/alpha", suspended = false },
			{ name = "beta", path = "/work/beta", suspended = true, hq = true },
		} }
	end)

	equal(answer.city, "/cities/one", "the city")
	equal(#answer.rigs, 2, "two rigs")
	equal(answer.rigs[1].name, "alpha", "the first")
	equal(answer.rigs[1].suspended, false, "running")
	equal(answer.rigs[2].suspended, true, "suspended")
	equal(answer.rigs[2].hq, true, "and the HQ")
end)

test("rigs: the JSON is joined back up across the rows of the screen", function()
	rigs.parse("city|-\nrigs|{\"rigs\":[\n{\"name\":\"alpha\"}\n]}", function(source)
		stub.json_text = source
		return { rigs = {} }
	end)
	equal(stub.json_text, '{"rigs":[{"name":"alpha"}]}', "no newlines left")
end)

test("rigs: a city path wrapped across rows is recovered", function()
	local answer = rigs.parse("city|/srv/very-long/\nGas City\nrigs|{}", function()
		return { rigs = {} }
	end)
	equal(answer.city, "/srv/very-long/Gas City", "the wrapped rows are joined")
end)

test("rigs: a city of - means the pane was in none", function()
	local answer = rigs.parse("city|-\nrigs|{}", function()
		return { rigs = {} }
	end)
	equal(answer.city, nil, "no city")
	equal(#answer.rigs, 0, "no rigs")
end)

test("rigs: an unreadable list is reported", function()
	local answer, err = rigs.parse("city|/c\nrigs|nonsense", function()
		return nil, "text is not valid JSON"
	end)
	equal(answer, nil, "no answer")
	contains(err, "not valid JSON", "the reason")
end)

test("rigs: the commands name each rig", function()
	local commands = rigs.commands("suspend", { "alpha", "beta" })
	same(commands[1], { "rig", "suspend", "alpha" }, "the first")
	same(commands[2], { "rig", "suspend", "beta" }, "the second")
end)

test("rigs: asks the pane's machine, from the pane's directory", function()
	stub.format = probe_answer({ "city|/cities/one", "rigs|{}" })
	stub.json_value = { rigs = {} }
	rigs.find(config.resolve(nil), { session_id = "session:1" }, { cwd = "/work/alpha" })
	-- a typo above would throw, so check what was asked for
	equal(stub.blocks[1].options.cwd, "/work/alpha", "the pane's directory")
	contains(stub.blocks[1].options.command[3], "rig list --json", "asks gc for the list")
	contains(stub.blocks[1].options.command[3], 'printf \'city|%s\\n\' "${city:--}"', "reports the city")
end)

-- Suspending and resuming rigs -------------------------------------------

local function run_rig(kind, args)
	actions.register(app_with({}))
	return pcall(
		stub.actions["gascity_rigs_" .. kind].run,
		{ block_id = "block:1", session_id = "session:1" },
		args or {}
	)
end

test("rigs: suspending offers every rig, by name and state", function()
	stub.pane_cwd = "/work/alpha"
	-- Two hidden blocks answer in turn: the one that lists the rigs, then the
	-- one that does the work.
	stub.format = {
		probe_answer({ "city|/cities/one", "rigs|" .. RIG_JSON }, 0),
		probe_answer({ "suspended: alpha, beta" }, 0),
	}
	stub.json_value = { rigs = {
		{ name = "alpha", path = "/work/alpha", suspended = false },
		{ name = "beta", path = "/work/beta", suspended = true, hq = true },
	} }
	stub.picked = { "alpha", "beta" }

	local ok, result = run_rig("suspend")
	truthy(ok, "no error: " .. tostring(result))
	truthy(stub.picks ~= nil, "asked")
	equal(stub.picks.title, "Suspend which rigs?", "the question")
	equal(stub.picks.multiple, true, "several can be ticked")
	equal(stub.picks.confirm, "Suspend", "the label")
	equal(stub.picks.items[1].title, "alpha", "the first rig")
	equal(stub.picks.items[1].detail, "running", "its state")
	equal(stub.picks.items[2].detail, "suspended, HQ", "and the second's")

	for _, item in ipairs(stub.picks.items) do
		truthy(REX_ICONS[item.icon], "a known Rex icon: " .. tostring(item.icon))
		equal(item.icon, "clock", "the suspend icon")
	end

	equal(result.city, "/cities/one", "the city it listed")
	equal(result.output, "suspended: alpha, beta", "what gc printed")
	equal(#stub.layers, 0, "no pane on the screen")
	equal(stub.closed[#stub.closed], "block:probe", "the hidden block is closed")

	local run_block = stub.blocks[2]
	contains(
		run_block.options.command[3],
		'--city "$city" \'rig\' \'suspend\' \'alpha\' && "$gc" --city "$city" \'rig\' \'suspend\' \'beta\'',
		"both rigs, in that city"
	)
	equal(run_block.options.cwd, "/work/alpha", "runs in the pane's directory")
	equal(run_block.label, "gascity-probe", "a hidden block")
end)

test("rigs: resuming says resume", function()
	stub.pane_cwd = "/work/alpha"
	stub.format = {
		probe_answer({ "city|/cities/one", "rigs|" .. RIG_JSON }, 0),
		probe_answer({ "resumed" }, 0),
	}
	stub.json_value = { rigs = { { name = "beta", path = "/work/beta", suspended = true } } }
	stub.picked = { "beta" }

	local ok = run_rig("resume")
	truthy(ok, "no error")
	equal(stub.picks.title, "Resume which rigs?", "the question")
	for _, item in ipairs(stub.picks.items) do
		truthy(REX_ICONS[item.icon], "a known Rex icon: " .. tostring(item.icon))
		equal(item.icon, "plus", "the resume icon")
	end
	contains(stub.blocks[2].options.command[3], "'rig' 'resume' 'beta'", "resume, not suspend")
end)

test("rigs: one rig can be picked and sent on its own", function()
	stub.pane_cwd = "/work/alpha"
	stub.format = {
		probe_answer({ "city|/cities/one", "rigs|" .. RIG_JSON }, 0),
		probe_answer({ "ok" }, 0),
	}
	stub.json_value = { rigs = { { name = "alpha", path = "/work/alpha" } } }
	stub.picked = { "alpha" }

	run_rig("suspend")
	falsy(stub.blocks[2].options.command[3]:find(" && ", 1, true), "no chain for one rig")
end)

test("rigs: ticking nothing sends nothing", function()
	stub.pane_cwd = "/work/alpha"
	stub.format = probe_answer({ "city|/cities/one", "rigs|" .. RIG_JSON })
	stub.json_value = { rigs = { { name = "alpha" } } }
	stub.picked = {}

	local ok, result = run_rig("suspend")
	truthy(ok, "no error")
	equal(result, nil, "no result")
	equal(#stub.layers, 0, "no pane")
	equal(#stub.blocks, 1, "only the probe ran")
end)

test("rigs: cancelling sends nothing", function()
	stub.pane_cwd = "/work/alpha"
	stub.format = probe_answer({ "city|/cities/one", "rigs|" .. RIG_JSON })
	stub.json_value = { rigs = { { name = "alpha" } } }
	stub.picked = nil

	local ok, result = run_rig("suspend")
	truthy(ok, "no error")
	equal(result, nil, "no result")
	equal(#stub.layers, 0, "no pane")
end)

test("rigs: a city with no rigs is reported", function()
	stub.pane_cwd = "/work/alpha"
	stub.format = probe_answer({ "city|/cities/one", "rigs|{}" })
	stub.json_value = { rigs = {} }

	local ok, result = run_rig("suspend")
	falsy(ok, "reports an error")
	contains(tostring(result), "no rigs listed in /cities/one", "the reason")
end)

test("rigs: a machine that does not answer is reported", function()
	stub.pane_cwd = "/work/alpha"
	stub.block_error = "session not found"

	local ok, result = run_rig("suspend")
	falsy(ok, "reports an error")
	contains(tostring(result), "session not found", "the reason")
end)

test("rigs: named rigs skip the question and the probe", function()
	stub.pane_cwd = "/work/alpha"
	stub.format = probe_answer({ "suspended: alpha, beta" }, 0)
	local ok, result = run_rig("suspend", { rigs = "alpha, beta" })
	truthy(ok, "no error: " .. tostring(result))
	equal(stub.picks, nil, "did not ask")
	equal(#stub.blocks, 1, "one block, for the work itself")
	same(result.rigs, { "alpha", "beta" }, "both rigs")
	equal(result.city, nil, "no city named, so gc walks up")
	contains(
		stub.blocks[1].options.command[3],
		"\'rig\' \'suspend\' \'alpha\' && \"$gc\" \'rig\' \'suspend\' \'beta\'",
		"both, in order"
	)
end)

test("rigs: a named city is used with named rigs", function()
	stub.pane_cwd = "/work/alpha"
	stub.format = probe_answer({ "resumed" }, 0)
	run_rig("resume", { rigs = "alpha", city = "/cities/two" })
	local script = stub.blocks[1].options.command[3]
	contains(script, "city='/cities/two'", "the city")
	contains(script, '--city "$city"', "on the command")
	contains(script, "'rig' 'resume' 'alpha'", "the rig")
end)

test("rigs: gc failing is the action's error, in one line", function()
	stub.pane_cwd = "/work/alpha"
	stub.format = probe_answer({ "gc rig suspend: no such rig alpha", "second line" }, 1)
	local ok, result = run_rig("suspend", { rigs = "alpha" })
	falsy(ok, "reports an error")
	equal(result, "gc rig suspend: no such rig alpha", "the first line")
	truthy(stub.logged("error", "second line"), "the whole output is logged")
end)

test("rigs: a silent failure still reports the exit status", function()
	stub.pane_cwd = "/work/alpha"
	stub.format = probe_answer({}, 2)
	local ok, result = run_rig("suspend", { rigs = "alpha" })
	falsy(ok, "reports an error")
	contains(tostring(result), "status 2", "names the exit status")
end)

test("rigs: a command that never answers is reported", function()
	stub.pane_cwd = "/work/alpha"
	stub.format = "<<gascity-begin>>\nnothing yet"
	local ok, result = run_rig("suspend", { rigs = "alpha" })
	falsy(ok, "reports an error")
	contains(tostring(result), "did not answer", "the reason")
	equal(stub.closed[1], "block:probe", "the hidden block is closed anyway")
end)

-- Opening a rig -----------------------------------------------------------

local function run_open(args)
	actions.register(app_with({}))
	return pcall(
		stub.actions.gascity_rigs_open.run,
		{ block_id = "block:1", session_id = "session:1" },
		args or {}
	)
end

local function rigs_fixture(rigs)
	stub.pane_cwd = "/work/alpha"
	stub.format = probe_answer({ "city|/cities/one", "rigs|{}" }, 0)
	stub.json_value = { rigs = rigs }
end

test("rigs open: the picker lists the rigs and the pane gets the path", function()
	rigs_fixture({
		{ name = "alpha", path = "/work/alpha" },
		{ name = "beta", path = "/work/beta" },
	})
	stub.picked = "alpha"

	local ok, result = run_open()
	truthy(ok, "no error: " .. tostring(result))
	equal(stub.picks.title, "Open which rig?", "the question")
	equal(stub.picks.multiple, nil, "one rig at a time")
	equal(stub.picks.confirm, "Open", "the label")
	equal(stub.picks.items[1].subtitle, "/work/alpha", "the rig's path")

	local split = stub.splits[#stub.splits]
	equal(split.session_id, "session:1", "the pane's session")
	local block = split.layout.block
	equal(block.flavor, "com.superlogical.terminal.shell", "a terminal")
	equal(block.label, "gascity:alpha", "the label")
	equal(block.options.cwd, "/work/alpha", "the directory")
	equal(block.options.command, nil, "an interactive shell")
	equal(result.cwd, "/work/alpha", "the result names the path")
end)

test("rigs open: rig= skips the picker", function()
	rigs_fixture({
		{ name = "alpha", path = "/work/alpha" },
		{ name = "beta", path = "/work/beta" },
	})
	stub.picked = nil

	local ok, result = run_open({ rig = "beta" })
	truthy(ok, "no error: " .. tostring(result))
	equal(stub.pick_count, 0, "no question")
	equal(result.cwd, "/work/beta", "the rig's path")
end)

test("rigs open: an unknown name is an error and opens nothing", function()
	rigs_fixture({ { name = "alpha", path = "/work/alpha" } })
	local ok, result = run_open({ rig = "gamma" })
	falsy(ok, "reports an error")
	contains(tostring(result), "no rig named gamma", "the reason")
	equal(#stub.blocks, 1, "only the rig list was asked for")
end)

test("rigs open: no rigs is an error", function()
	rigs_fixture({})
	local ok, result = run_open()
	falsy(ok, "reports an error")
	contains(tostring(result), "no rigs listed", "the reason")
	equal(#stub.blocks, 1, "only the rig list was asked for")
end)

test("rigs open: a rig with no path is not offered", function()
	rigs_fixture({ { name = "alpha" }, { name = "beta", path = "/work/beta" } })
	stub.picked = "beta"
	local ok = run_open()
	truthy(ok, "no error")
	equal(#stub.picks.items, 1, "only the rig with a path")
	equal(stub.picks.items[1].title, "beta", "the one with a path")

	local ok_named, result = run_open({ rig = "alpha" })
	falsy(ok_named, "an unopenable rig is an error")
	contains(tostring(result), "has no path", "the reason")
end)

test("rigs open: no rig has a path, so no picker", function()
	rigs_fixture({ { name = "alpha" }, { name = "beta" } })
	local ok, result = run_open()
	falsy(ok, "reports an error")
	contains(tostring(result), "has a path", "the reason")
	equal(stub.pick_count, 0, "no question")
	equal(#stub.splits, 0, "nothing opened")
end)

test("rigs open: cancel is quiet", function()
	rigs_fixture({ { name = "alpha", path = "/work/alpha" } })
	stub.picked = nil
	local ok, result = run_open()
	truthy(ok, "no error: " .. tostring(result))
	equal(result, nil, "nothing opened")
	equal(#stub.logs, 0, "nothing logged")
	equal(#stub.blocks, 1, "only the rig list was asked for")
end)

test("rigs open: a tilde path is passed through", function()
	rigs_fixture({ { name = "alpha", path = "~/src/alpha" } })
	stub.picked = "alpha"
	local ok = run_open()
	truthy(ok, "no error")
	equal(stub.splits[#stub.splits].layout.block.options.cwd, "~/src/alpha", "byte for byte")
end)

test("rigs open: a picker failure that is not a cancel is logged", function()
	rigs_fixture({ { name = "alpha", path = "/work/alpha" } })
	stub.picked = nil
	stub.pick_why = "no ui"
	local ok, result = run_open()
	truthy(ok, "no error: " .. tostring(result))
	equal(result, nil, "nothing opened")
	truthy(stub.logged("warn", "gascity_rigs_open"), "the failure is logged")
end)

-- The status update -------------------------------------------------------

-- Runs `command` in /bin/sh on this machine and returns what it printed.
local function sh(command)
	local pipe = assert(io.popen(command))
	local out = pipe:read("*a")
	pipe:close()
	return out
end

local function write_file(path, content)
	local file = assert(io.open(path, "w"))
	file:write(content)
	file:close()
end

test("status: both live spellings of the pack's source match", function()
	truthy(status.is_this_pack("https://github.com/docbobo/rex-gascity//pack"), "//pack")
	truthy(status.is_this_pack("https://github.com/docbobo/rex-gascity//pack/"), "trailing slash")
	truthy(status.is_this_pack("https://github.com/docbobo/rex-gascity.git//pack"), ".git")
	truthy(status.is_this_pack("https://github.com/docbobo/rex-gascity/tree/main/pack"), "/tree/main/pack")
	truthy(status.is_this_pack("github.com/docbobo/rex-gascity/tree/v2026.11.06/pack"), "no scheme, a tag")
	falsy(status.is_this_pack("https://github.com/gastownhall/gascity-packs/tree/main/gascity"), "another pack")
	falsy(status.is_this_pack("https://github.com/docbobo/rex-gascity//ui"), "another directory")
	falsy(status.is_this_pack("https://github.com/docbobo/rex-gascity-fork//pack"), "another repository")
	falsy(status.is_this_pack(nil), "no source")
end)

test("status: the override wins, then the first matching key in sorted order", function()
	local rex_pack = "https://github.com/docbobo/rex-gascity//pack"
	local other = "https://github.com/gastownhall/gascity-packs/tree/main/gascity"
	equal(status.binding({}, "mine"), "mine", "the override needs no import")
	equal(status.binding({ { key = "rex", source = rex_pack } }, nil), "rex", "one match")
	equal(
		status.binding({
			{ key = "zz", source = "https://github.com/docbobo/rex-gascity/tree/main/pack" },
			{ key = "aa", source = rex_pack },
		}, nil),
		"aa",
		"two matches"
	)
	equal(status.binding({ { key = "gc", source = other } }, nil), nil, "no match")
	equal(status.binding({ { key = "rex", source = rex_pack } }, ""), "rex", "an empty override is no override")
end)

test("status: the mail names the template, the command, and its target", function()
	local body = status.body("rex", "/tmp/rex.sock", "s1", "text")
	contains(body, "gc 'rex' status-update --template", "the template command")
	contains(
		body,
		"gc 'rex' status-update --server '/tmp/rex.sock' --session 's1' --display 'text'",
		"the command"
	)
end)

test("status: every value in the mail arrives as one word", function()
	local body = status.body("my pack;x", "/tmp/a b.sock", "s 1'x", "te'xt")
	local line = body:match("\n%s*(gc [^\n]*%-%-display [^\n]*)$")
	truthy(line, "the command line")
	local words = sh("set -- " .. line .. "; for w; do printf '[%s]\\n' \"$w\"; done")
	equal(
		words,
		"[gc]\n[my pack;x]\n[status-update]\n[--server]\n[/tmp/a b.sock]\n[--session]\n[s 1'x]\n[--display]\n[te'xt]\n",
		"the words"
	)
end)

test("status: the answer is read across wrapped rows", function()
	local found = status.parse(table.concat({
		"@@city|/cities/alp",
		"ha",
		"@@server|unix:///Users/me/Library/Application%20Sup",
		"port/rex/server.sock",
		"@@config|0",
		"@@import|rex|https://github.com/docbobo/rex-ga",
		"scity//pack",
		"@@import|gc|https://github.com/gastownhall/gascity-packs/tree/main/gascity",
	}, "\n"))
	equal(found.city, "/cities/alpha", "the city")
	equal(found.server, "unix:///Users/me/Library/Application%20Support/rex/server.sock", "the server")
	equal(found.config_status, 0, "the lookup status")
	equal(#found.imports, 2, "two imports")
	equal(found.imports[1].key, "rex", "the first key")
	equal(found.imports[1].source, "https://github.com/docbobo/rex-gascity//pack", "its source")
	falsy(found.gc_missing, "gc was there")
end)

test("status: an empty server line is no server", function()
	local found = status.parse("@@city|-\n@@server|\n@@config|0")
	equal(found.city, nil, "no city")
	equal(found.server, nil, "no server")
end)

test("status: the reason the server could not be verified is read back", function()
	local found = status.parse("@@server|\n@@server_error|rex -S /tmp/x.sock failed (exit 3): no session")
	equal(found.server, nil, "no server")
	equal(found.server_error, "rex -S /tmp/x.sock failed (exit 3): no session", "the reason")
end)

-- A scratch directory with a fake gc and a fake rex in it.
--
-- The fake gc prints the fixture for `config show --json`, or fails like a
-- broken city.toml when FAKE_GC_FAIL is set. The fake rex succeeds only for
-- a read-only check (--autostart=false) of session s1.
local function probe_tools()
	local dir = text.trim(sh('mktemp -d "${TMPDIR:-/tmp}/gascity-status-test.XXXXXX"'))
	write_file(dir .. "/gc", table.concat({
		"#!/bin/sh",
		'printf "%s\\n" "$*" > gc.args',
		'case " $* " in *" config show --json "*) ;; *) exit 9 ;; esac',
		'if [ -n "${FAKE_GC_FAIL:-}" ]; then printf "bad city.toml\\nline two\\n" >&2; exit 1; fi',
		'cat "$FIXTURE"',
	}, "\n") .. "\n")
	write_file(dir .. "/rex", table.concat({
		"#!/bin/sh",
		'case " $* " in *" --autostart=false "*) ;; *) exit 8 ;; esac',
		'case " $* " in *" -s s1 "*) exit 0 ;; esac',
		"exit 3",
	}, "\n") .. "\n")
	sh("chmod +x " .. text.quote(dir .. "/gc") .. " " .. text.quote(dir .. "/rex"))
	return dir
end

-- Runs a probe script the way the hidden block does: /bin/sh, in `dir`, with
-- `dir` first on PATH and only the environment named in `env`. A last line
-- shows whether the script ran to its end.
-- The fixture by its absolute path, since the probe runs in another directory.
local function fixture_path(name)
	local path = fixture .. "/" .. name
	if path:sub(1, 1) ~= "/" then
		path = text.trim(sh("pwd")) .. "/" .. path
	end
	return path
end

local function run_probe(script, dir, env)
	write_file(dir .. "/probe.sh", script .. "\nprintf 'END\\n'\n")
	return sh(
		"cd " .. text.quote(dir) .. " && env -i HOME=\"$HOME\" PATH=" .. text.quote(dir .. ":/usr/bin:/bin")
			.. " FIXTURE=" .. text.quote(fixture_path("config-show.json"))
			.. " " .. (env or "")
			.. " /bin/sh probe.sh 2>/dev/null"
	)
end

local function remove(dir)
	sh("rm -rf " .. text.quote(dir))
end

test("status: the probe lists only the top-level imports", function()
	local dir = probe_tools()
	local output = run_probe(status.script(config.resolve(nil), "/cities/named", "s1"), dir)
	local args = sh("cat " .. text.quote(dir .. "/gc.args"))
	remove(dir)
	equal(args, "--city /cities/named config show --json\n", "the named city is passed to gc")
	contains(output, "@@city|/cities/alpha\n", "the city gc read")
	contains(output, "@@config|0\n", "the lookup succeeded")
	contains(output, "@@import|gc|https://github.com/gastownhall/gascity-packs/tree/main/gascity\n", "gc")
	contains(output, "@@import|rex|https://github.com/docbobo/rex-gascity//pack\n", "rex")
	falsy(output:find("rig-rex", 1, true), "no rig-level import")
	falsy(output:find("roles", 1, true), "no default rig import")
	local _, count = output:gsub("@@import|", "")
	equal(count, 2, "two imports")
	contains(output, "END", "the script ran to its end")
end)

test("status: without a named city, gc finds it from the pane's directory", function()
	local script = status.script(config.resolve(nil), nil, "s1")
	falsy(script:find("--city", 1, true), "no --city")
	local dir = probe_tools()
	local output = run_probe(script, dir)
	local args = sh("cat " .. text.quote(dir .. "/gc.args"))
	remove(dir)
	equal(args, "config show --json\n", "gc gets no --city")
	contains(output, "@@city|/cities/alpha\n", "the city gc found")
end)

test("status: the probe verifies the server without starting one", function()
	local script = status.script(config.resolve(nil), "/cities/alpha", "s1")
	contains(script, "--autostart=false api call session.list_blocks -s 's1' </dev/null", "read-only")

	local dir = probe_tools()
	local owned = run_probe(script, dir, "REX_SERVER=/tmp/other.sock")
	local foreign = run_probe(status.script(config.resolve(nil), "/cities/alpha", "s2"), dir, "REX_SERVER=/tmp/other.sock")
	local unset = run_probe(script, dir)
	remove(dir)
	contains(owned, "@@server|/tmp/other.sock\n", "the server that owns s1")
	falsy(owned:find("@@server_error|", 1, true), "no reason when the server answers")
	contains(foreign, "@@server|\n", "a server that does not own s2")
	contains(foreign, "@@server_error|", "a server that did not answer is a reason")
	contains(foreign, "/tmp/other.sock", "the reason names the address")
	contains(foreign, "exit 3", "and the CLI's exit status")
	contains(unset, "@@server|\n", "no REX_SERVER")
	contains(unset, "REX_SERVER is not set", "and says so")
end)

test("status: a failed config show reports its status and stderr", function()
	local dir = probe_tools()
	local output = run_probe(status.script(config.resolve(nil), "/cities/alpha", "s1"), dir, "FAKE_GC_FAIL=1")
	remove(dir)
	contains(output, "@@config|1\n", "the status")
	contains(output, "@@error|bad city.toml line two", "stderr on one line")
	falsy(output:find("@@import|", 1, true), "no imports")
	contains(output, "END", "the script ran to its end")
end)

test("status: no gc says so and still reaches the end", function()
	local dir = probe_tools()
	local settings = config.resolve(nil)
	settings.gc_bin = "/nowhere/gc"
	local output = run_probe(status.script(settings, "/cities/alpha", "s1"), dir)
	remove(dir)
	contains(output, "@@gc|missing\n", "gc is missing")
	falsy(output:find("@@config|", 1, true), "no lookup")
	contains(output, "END", "the script ran to its end")
end)

test("status: pack_binding skips the import lookup, not the server check or the gc check", function()
	local settings = config.resolve(nil)
	settings.pack_binding = "mine"
	local script = status.script(settings, nil, "s1")
	falsy(script:find("config show", 1, true), "no config show")
	contains(script, "--autostart=false", "the server check")
	contains(script, "has_gc", "the gc check")

	local dir = probe_tools()
	local found = run_probe(script, dir, "REX_SERVER=/tmp/other.sock")
	settings.gc_bin = "/nowhere/gc"
	local missing = run_probe(status.script(settings, nil, "s1"), dir, "REX_SERVER=/tmp/other.sock")
	remove(dir)
	falsy(found:find("@@gc|", 1, true), "gc is there")
	falsy(found:find("@@config|", 1, true), "no lookup with gc")
	contains(missing, "@@server|/tmp/other.sock\n", "the server is still checked")
	contains(missing, "@@gc|missing\n", "a missing gc is still reported")
	falsy(missing:find("@@config|", 1, true), "no lookup without gc")
	contains(missing, "END", "the script ran to its end")
end)

test("status: the lookup keeps its files in a private directory and removes it", function()
	local script = status.script(config.resolve(nil), "/cities/alpha", "s1")
	contains(script, 'mktemp -d "${TMPDIR:-/tmp}/gascity-status.XXXXXX"', "a private directory")
	falsy(script:find(".err", 1, true), "no predictable sibling file")

	local dir = probe_tools()
	sh("mkdir " .. text.quote(dir .. "/tmp"))
	local tmpdir = "TMPDIR=" .. text.quote(dir .. "/tmp")
	local passed = run_probe(script, dir, tmpdir)
	local left_after_pass = sh("ls -A " .. text.quote(dir .. "/tmp"))
	local failed = run_probe(script, dir, tmpdir .. " FAKE_GC_FAIL=1")
	local left_after_fail = sh("ls -A " .. text.quote(dir .. "/tmp"))
	local no_dir = run_probe(script, dir, "TMPDIR=" .. text.quote(dir .. "/missing"))
	remove(dir)
	contains(passed, "@@config|0\n", "the lookup ran")
	equal(left_after_pass, "", "nothing left after a lookup that passed")
	contains(failed, "@@error|bad city.toml line two", "stderr was read from the private directory")
	equal(left_after_fail, "", "nothing left after a lookup that failed")
	contains(no_dir, "@@config|1\n", "no directory is a failed lookup")
	contains(no_dir, "@@error|cannot make a temporary directory\n", "and says why")
	falsy(no_dir:find("@@import|", 1, true), "no imports")
	contains(no_dir, "END", "the script ran to its end")
end)

test("status: a value that holds the probe's end marker survives", function()
	local server = "unix:///tmp/a<<gascity-end.sock"
	local dir = probe_tools()
	local output = run_probe(
		status.script(config.resolve(nil), "/cities/alpha", "s1"),
		dir,
		"REX_SERVER=" .. text.quote(server)
	)
	remove(dir)

	-- The terminal shows the writer's rows between the probe's markers.
	stub.format = probe_answer({ output })
	local answer = assert(probe.run({ session_id = "s1" }, "true"))
	local found = status.parse(answer.output)
	equal(found.server, server, "the whole server comes back")
	equal(#found.imports, 2, "the imports after it survive")
end)

-- Writes a `gc config show --json` answer into the scratch directory and
-- returns its path, so a probe can run against a value the checked-in
-- fixture does not hold.
local function probe_fixture(dir, json)
	local path = dir .. "/fixture.json"
	write_file(path, json)
	return path
end

test("status: an import key that holds a pipe keeps its own value", function()
	local dir = probe_tools()
	local path = probe_fixture(
		dir,
		'{"city_path":"/cities/alpha","config":{"Imports":{"custom|name":{"Source":"https://github.com/docbobo/rex-gascity//pack"}}}}'
	)
	local output = run_probe(
		status.script(config.resolve(nil), "/cities/alpha", "s1"),
		dir,
		"FIXTURE=" .. text.quote(path)
	)
	remove(dir)
	local found = status.parse((output:gsub("END\n$", "")))
	equal(#found.imports, 1, "one import")
	equal(found.imports[1].key, "custom|name", "the key comes back whole")
	equal(found.imports[1].source, "https://github.com/docbobo/rex-gascity//pack", "the source is its own value")
	equal(status.binding(found.imports, nil), "custom|name", "the binding is found")
end)

test("status: a JSON escape in a value is decoded", function()
	local dir = probe_tools()
	local path = probe_fixture(
		dir,
		'{"city_path":"/cities/city\\"literal\\u003cb","config":{"Imports":{"rex":{"Source":"https://github.com/docbobo/rex-gascity//pack"}}}}'
	)
	local output = run_probe(
		status.script(config.resolve(nil), "/cities/alpha", "s1"),
		dir,
		"FIXTURE=" .. text.quote(path)
	)
	remove(dir)
	local found = status.parse(output)
	equal(found.city, '/cities/city"literal<b', "the quote and the code point come back")
	equal(#found.imports, 1, "the import is still read")
	equal(found.imports[1].key, "rex", "its key")
end)

test("status: rex is found in REX_BIN_DIR when it is not on PATH", function()
	local dir = probe_tools()
	sh("rm " .. text.quote(dir .. "/rex"))
	local bin = dir .. "/bin dir"
	sh("mkdir " .. text.quote(bin))
	write_file(bin .. "/rex", "#!/bin/sh\ncase \" $* \" in *\" -s s1 \"*) exit 0 ;; esac\nexit 3\n")
	sh("chmod +x " .. text.quote(bin .. "/rex"))
	local script = status.script(config.resolve(nil), "/cities/alpha", "s1")
	local found = run_probe(script, dir, "REX_SERVER=/tmp/other.sock REX_BIN_DIR=" .. text.quote(bin))
	local absent = run_probe(script, dir, "REX_SERVER=/tmp/other.sock")
	remove(dir)
	contains(found, "@@server|/tmp/other.sock\n", "the fallback finds rex")
	contains(absent, "@@server|\n", "no rex anywhere is no server")
	contains(absent, "no rex on PATH and none in REX_BIN_DIR", "and says that no rex was found")
	contains(absent, "/tmp/other.sock", "the reason names the address")
	contains(absent, "END", "the script ran to its end")
end)

test("status: the app's rex is used before a rex on PATH", function()
	local dir = probe_tools()
	-- An older rex on PATH, as an install without the app's binary leaves it.
	write_file(dir .. "/rex", "#!/bin/sh\nprintf 'unknown flag: --autostart\\n' >&2\nexit 2\n")
	sh("chmod +x " .. text.quote(dir .. "/rex"))
	local bin = dir .. "/bin dir"
	sh("mkdir " .. text.quote(bin))
	write_file(bin .. "/rex", "#!/bin/sh\ncase \" $* \" in *\" -s s1 \"*) exit 0 ;; esac\nexit 3\n")
	sh("chmod +x " .. text.quote(bin .. "/rex"))
	local script = status.script(config.resolve(nil), "/cities/alpha", "s1")
	local app = run_probe(script, dir, "REX_SERVER=/tmp/other.sock REX_BIN_DIR=" .. text.quote(bin))
	local path_only = run_probe(script, dir, "REX_SERVER=/tmp/other.sock")
	remove(dir)
	contains(app, "@@server|/tmp/other.sock\n", "the app's rex answers")
	contains(path_only, "@@server|\n", "a rex on PATH that fails is no server")
	contains(path_only, "unknown flag: --autostart", "the CLI's reason is named")
	contains(path_only, "/tmp/other.sock", "the address is named")
end)

test("status: a session the server does not list names the CLI reason", function()
	local dir = probe_tools()
	local output = run_probe(status.script(config.resolve(nil), "/cities/alpha", "s2"), dir, "REX_SERVER=/tmp/other.sock")
	remove(dir)
	contains(output, "@@server|\n", "no server")
	contains(output, "@@server_error|", "the reason row")
	contains(output, "/tmp/other.sock", "the address")
	contains(output, "exit 3", "the exit status")
	contains(output, "END", "the script ran to its end")
end)

test("status: a percent, a tab, a newline, and a carriage return survive", function()
	local dir = probe_tools()
	local path = probe_fixture(
		dir,
		'{"city_path":"/cities/a\\tb","config":{"Imports":{"rex":{"Source":"https://github.com/docbobo/rex-gascity//pack"}}}}'
	)
	local server = "unix:///tmp/a%0Ab\nc\rd\te"
	local output = run_probe(
		status.script(config.resolve(nil), "/cities/alpha", "s1"),
		dir,
		"FIXTURE=" .. text.quote(path) .. " REX_SERVER=" .. text.quote(server)
	)
	remove(dir)
	local found = status.parse((output:gsub("END\n$", "")))
	equal(found.city, "/cities/a\tb", "a tab from the JSON comes back")
	equal(found.server, server, "the percent, newline, tab, and carriage return come back")
end)

test("status: a filter that fails is a probe error, not an empty lookup", function()
	local script = status.script(config.resolve(nil), "/cities/alpha", "s1")
	local dir = probe_tools()
	write_file(dir .. "/awk", "#!/bin/sh\necho 'awk: boom' >&2\nexit 2\n")
	sh("chmod +x " .. text.quote(dir .. "/awk"))
	sh("mkdir " .. text.quote(dir .. "/tmp"))
	local output = run_probe(script, dir, "TMPDIR=" .. text.quote(dir .. "/tmp"))
	local left = sh("ls -A " .. text.quote(dir .. "/tmp"))
	remove(dir)
	contains(output, "@@config|0\n", "the lookup itself succeeded")
	contains(output, "@@error|the config filter failed", "the filter failure has its own error")
	falsy(output:find("@@import|", 1, true), "no imports")
	contains(output, "END", "the script ran to its end")
	equal(left, "", "the private directory was removed")
end)

test("status: ask returns the filter error and no answer", function()
	stub.format = probe_answer({ "@@config|0", "@@error|the config filter failed" })
	local answer, why = status.ask(config.resolve(nil), { session_id = "s1" }, {})
	equal(answer, nil, "no answer")
	equal(why, "the config filter failed", "the reason")
end)

test("status: a wrapped import source may hold the row separator", function()
	local found = status.parse(table.concat({
		"@@config|0",
		"@@import|rex|https://github.com/docbobo/rex-gascity/tree/release%40%40sta",
		"ble/pack",
	}, "\n"))
	equal(#found.imports, 1, "one import")
	equal(
		found.imports[1].source,
		"https://github.com/docbobo/rex-gascity/tree/release@@stable/pack",
		"the @ signs come back"
	)
	equal(status.binding(found.imports, nil), "rex", "the binding is found")
end)

test("status: a percent sign in a value is not read as an escape", function()
	local found = status.parse("@@server|unix:///tmp/a%2540.sock")
	equal(found.server, "unix:///tmp/a%40.sock", "the percent comes back")
end)

test("status: a value may hold the probe's end marker", function()
	local found = status.parse(
		"@@server|unix:///tmp/a%3C%3Cgascity-end.sock\n@@import|rex|https://github.com/docbobo/rex-gascity//pack"
	)
	equal(found.server, "unix:///tmp/a<<gascity-end.sock", "the < signs come back")
	equal(status.binding(found.imports, nil), "rex", "the next row is read")
end)

-- The status update action -------------------------------------------------

local REX_PACK = "https://github.com/docbobo/rex-gascity//pack"

-- Runs the action with a spy in place of send.message. Returns ok, the result,
-- and the messages it sent.
local function run_status(args, settings)
	local sent = {}
	local app = app_with({
		send = {
			message = function(_, _, message)
				sent[#sent + 1] = message
				return "layer:1", nil, message.subject
			end,
		},
	})
	for key, value in pairs(settings or {}) do
		app.config[key] = value
	end
	actions.register(app)
	local ok, result = pcall(
		stub.actions.gascity_status_update.run,
		{ block_id = "block:1", session_id = "s1" },
		args or {}
	)
	return ok, result, sent
end

test("status action: it is registered with a title, an icon, and optional arguments", function()
	run_status({})
	local definition = stub.actions.gascity_status_update
	truthy(definition, "registered")
	equal(definition.title, "Gas City: Ask the Mayor for a Status Update…", "title")
	equal(definition.args.city, "string?", "city")
	equal(definition.args.display, "string?", "display")
end)

test("status action: without a city argument it asks and uses the chosen city", function()
	stub.pane_cwd = "/work/nowhere"
	stub.format = {
		probe_answer({ "city|/cities/one", "city|/cities/two" }),
		probe_answer({ "@@city|/cities/two", "@@server|/tmp/rex.sock", "@@config|0", "@@import|rex|" .. REX_PACK }),
	}
	stub.picked = "/cities/two"
	local ok, result, sent = run_status({})
	truthy(ok, "no error: " .. tostring(result))
	truthy(stub.picks ~= nil, "asked")
	equal(stub.picks.title, "Which city?", "the question")
	equal(#stub.picks.items, 2, "both cities")
	equal(result.city, "/cities/two", "the chosen city")
	contains(stub.blocks[2].options.command[3], "city='/cities/two'", "the probe checks the chosen city")
	equal(sent[1].city, "/cities/two", "the mail names the chosen city")
end)

test("status action: it mails the mayor the command, the session, the server, and the display", function()
	stub.format = probe_answer({ "@@city|/cities/alpha", "@@server|/tmp/rex.sock", "@@config|0", "@@import|rex|" .. REX_PACK })
	local ok, result, sent = run_status({ city = "/cities/alpha" })
	truthy(ok, "no error: " .. tostring(result))
	equal(#sent, 1, "one mail")
	equal(sent[1].subject, "Status update requested", "the subject")
	equal(sent[1].city, "/cities/alpha", "the city")
	contains(sent[1].body, "gc 'rex' status-update --template", "the template")
	contains(sent[1].body, "--server '/tmp/rex.sock' --session 's1' --display 'text'", "the target")
	equal(result.binding, "rex", "binding")
	equal(result.server, "/tmp/rex.sock", "server")
	equal(result.session_id, "s1", "session")
	equal(result.display, "text", "display")
	equal(#stub.layers, 0, "the action opens no pane of its own")
	contains(stub.blocks[1].options.command[3], "-s 's1'", "the probe checks this session")
end)

test("status action: a non-default server is the one named", function()
	stub.format = probe_answer({ "@@city|/cities/alpha", "@@server|/tmp/other.sock", "@@config|0", "@@import|rex|" .. REX_PACK })
	local ok, result, sent = run_status({ city = "/cities/alpha" })
	truthy(ok, "no error: " .. tostring(result))
	contains(sent[1].body, "--server '/tmp/other.sock'", "the other server")
end)

test("status action: the display comes from the argument, then the setting", function()
	stub.format = probe_answer({ "@@city|-", "@@server|S", "@@config|0", "@@import|rex|" .. REX_PACK })
	local _, _, sent = run_status({ city = "/c", display = "markdown" }, { status_display = "other" })
	contains(sent[1].body, "--display 'markdown'", "the argument")
	stub.reset()
	stub.format = probe_answer({ "@@city|-", "@@server|S", "@@config|0", "@@import|rex|" .. REX_PACK })
	_, _, sent = run_status({ city = "/c" }, { status_display = "other" })
	contains(sent[1].body, "--display 'other'", "the setting")
end)

test("status action: pack_binding names the binding with no import", function()
	stub.format = probe_answer({ "@@city|-", "@@server|S" })
	local ok, result, sent = run_status({ city = "/c" }, { pack_binding = "mine" })
	truthy(ok, "no error: " .. tostring(result))
	contains(sent[1].body, "gc 'mine' status-update", "the override")
	falsy(stub.blocks[1].options.command[3]:find("config show", 1, true), "no lookup")
end)

test("status action: pack_binding does not skip the gc check, and no mail goes", function()
	stub.format = probe_answer({ "@@server|/tmp/rex.sock", "@@gc|missing" })
	local ok, err, sent = run_status({ city = "/c" }, { pack_binding = "mine" })
	falsy(ok, "an error")
	contains(err, "gc is not on this machine", "the gc.guard message")
	equal(#sent, 0, "no mail")
	contains(stub.blocks[1].options.command[3], "has_gc", "the probe checks gc")
end)

test("status action: a city without the pack is told how to fix it, and no mail goes", function()
	stub.format = probe_answer({
		"@@city|/cities/alpha",
		"@@server|S",
		"@@config|0",
		"@@import|gc|https://github.com/gastownhall/gascity-packs/tree/main/gascity",
	})
	local ok, err, sent = run_status({ city = "alpha" }, { file = "/home/me/.config/rex/gascity.lua" })
	falsy(ok, "an error")
	equal(
		err,
		"alpha does not import the rex-gascity pack. Import it (docs/rex-runtime-pack.md), or set pack_binding in /home/me/.config/rex/gascity.lua",
		"the error"
	)
	equal(#sent, 0, "no mail")
end)

test("status action: a failed config show is named as that, and no mail goes", function()
	stub.format = probe_answer({ "@@city|/cities/alpha", "@@server|S", "@@config|1", "@@error|bad city.toml" })
	local ok, err, sent = run_status({ city = "alpha" })
	falsy(ok, "an error")
	equal(err, "gc config show failed for alpha (exit 1): bad city.toml", "the error")
	equal(#sent, 0, "no mail")
end)

test("status action: no gc on the machine is named, and no mail goes", function()
	stub.format = probe_answer({ "@@city|/cities/alpha", "@@server|S", "@@gc|missing" })
	local ok, err, sent = run_status({ city = "alpha" })
	falsy(ok, "an error")
	contains(err, "gc is not on this machine", "the gc.guard message")
	equal(#sent, 0, "no mail")
end)

test("status action: an unverified server is an error, and no mail goes", function()
	stub.format = probe_answer({ "@@city|/cities/alpha", "@@server|", "@@config|0", "@@import|rex|" .. REX_PACK })
	local ok, err, sent = run_status({ city = "alpha" })
	falsy(ok, "an error")
	equal(err, "could not find the Rex server that owns session s1", "the error")
	equal(#sent, 0, "no mail")
end)

test("status action: an unverified server names the CLI reason and the address", function()
	stub.format = probe_answer({
		"@@city|/cities/alpha",
		"@@server|",
		"@@server_error|rex -S /tmp/other.sock api call session.list_blocks failed (exit 3): no such session",
		"@@config|0",
		"@@import|rex|" .. REX_PACK,
	})
	local ok, err, sent = run_status({ city = "alpha" })
	falsy(ok, "an error")
	contains(err, "could not find the Rex server that owns session s1", "the session")
	contains(err, "/tmp/other.sock", "the address")
	contains(err, "exit 3", "the CLI's exit status")
	contains(err, "no such session", "the CLI's reason")
	equal(#sent, 0, "no mail")
end)

test("status action: a machine that does not answer is an error, and no mail goes", function()
	stub.format = "nothing yet"
	local ok, err, sent = run_status({ city = "alpha" })
	falsy(ok, "an error")
	equal(err, "the machine did not answer in time", "the probe's reason")
	equal(#sent, 0, "no mail")
end)

-- The Gas City menu ------------------------------------------------------

local function run_menu(choice, ctx)
	actions.register(app_with({}))
	stub.picked = choice
	return pcall(
		stub.actions.gascity_menu.run,
		ctx or { block_id = "block:1", session_id = "session:1" }
	)
end

test("menu: it lists the Gas City commands and nothing else", function()
	stub.pane_cwd = "/work"
	stub.answer = nil
	run_menu(nil)

	truthy(stub.picks ~= nil, "asked")
	equal(stub.picks.title, "Gas City", "the title")
	equal(#stub.picks.items, 6, "six commands")
	equal(stub.picks.items[1].title, "Message the Mayor…", "the first")
	equal(stub.picks.items[2].title, "Ask the Mayor for a Status Update…", "the second")
	equal(stub.picks.items[3].title, "Suspend a Rig…", "the third")
	equal(stub.picks.items[4].title, "Resume a Rig…", "the fourth")
	equal(stub.picks.items[5].title, "Open a Rig…", "the fifth")
	equal(stub.picks.items[6].title, "Where This Runs…", "the sixth")
	truthy(stub.picks.items[1].subtitle ~= nil, "each says what it does")
	equal(stub.picks.items[2].icon, "document", "the status update uses the document")
	equal(stub.picks.items[3].icon, "clock", "suspend uses the clock")
	equal(stub.picks.items[4].icon, "plus", "resume uses the plus")
	truthy(stub.picks.items[3].icon ~= stub.picks.items[4].icon, "and they differ")
	for _, item in ipairs(stub.picks.items) do
		truthy(REX_ICONS[item.icon], "a known Rex icon: " .. tostring(item.icon))
	end
end)

test("menu: choosing a command runs it, questions and all", function()
	stub.pane_cwd = "/work/city"
	stub.answer = "Build is green"
	local ok, result = run_menu("mail_mayor")
	truthy(ok, "no error: " .. tostring(result))
	equal(stub.prompt.title, "Message the mayor", "the message question came from the command")
	equal(result.subject, "Build is green", "and it sent")
	equal(#stub.layers, 1, "one pane")
end)

test("menu: a rig command reaches its own picker", function()
	stub.pane_cwd = "/work/alpha"
	stub.format = probe_answer({ "city|/cities/one", "rigs|{}" }, 0)
	stub.json_value = { rigs = { { name = "alpha", path = "/work/alpha" } } }
	local ok, result = run_menu("rigs_suspend")
	truthy(ok, "no error: " .. tostring(result))
	-- the first pick was the menu, the second the rigs
	equal(stub.picks.title, "Suspend which rigs?", "the rig question")
	equal(result, nil, "and nothing was ticked")
end)

test("menu: the rig open command reaches its own picker", function()
	stub.pane_cwd = "/work/alpha"
	stub.format = probe_answer({ "city|/cities/one", "rigs|{}" }, 0)
	stub.json_value = { rigs = { { name = "alpha", path = "/work/alpha" } } }
	run_menu("rigs_open")
	-- the first pick was the menu, the second the rigs
	equal(stub.picks.title, "Open which rig?", "the rig question")
end)

test("doctor: the report is a pane with icons and colour", function()
	stub.pane_cwd = "/work"
	stub.format = probe_answer({ "city|-", "rigs|{}" })
	local ok, result = run_menu("doctor", {
		block_id = "block:1",
		session_id = "session:1",
		origin = "palette",
	})
	truthy(ok, "no error: " .. tostring(result))
	truthy(result.output ~= nil, "the text still comes back for the CLI")
	equal(stub.pick_count, 1, "only the menu was asked")

	equal(#stub.layers, 1, "a pane")
	equal(stub.layers[1].focus, true, "which takes the focus")
	local block = stub.block()
	equal(block.label, "gascity-report", "its label")
	equal(block.options.exit.on_completion, false, "it stays up")

	local script = block.options.command[3]
	contains(script, "\27[", "escape sequences")
	contains(script, "Gas City", "the heading")
	contains(script, "test-machine", "the report")
	contains(script, "Close this pane", "and how to close it")
end)

test("doctor: run without a person, it opens nothing", function()
	stub.pane_cwd = "/work"
	stub.format = probe_answer({ "city|-", "rigs|{}" })
	local ok, result = run_menu("doctor", {
		block_id = "block:1",
		session_id = "session:1",
		origin = "api",
	})
	truthy(ok, "no error: " .. tostring(result))
	equal(stub.pick_count, 1, "only the menu was asked")
	equal(#stub.layers, 0, "no pane")
	truthy(result.output ~= nil, "the text is the result")
end)

test("menu: cancelling runs nothing", function()
	stub.pane_cwd = "/work"
	local ok, result = run_menu(nil)
	truthy(ok, "no error")
	equal(result, nil, "no result")
	equal(#stub.layers, 0, "no pane")
	equal(#stub.logs, 0, "cancelling is quiet")
end)

-- Without the prompt API, an older server ---------------------------------

test("prompt without rex.ui logs once and does not crash", function()
	actions.register(app_with({}))
	stub.pane_cwd = "/work/city"
	rex.ui = nil
	stub.logs = {}

	local ok, result = pcall(
		stub.actions.gascity_menu.run,
		{ block_id = "block:1", session_id = "session:1" }
	)
	truthy(ok, "the menu does not crash: " .. tostring(result))
	equal(result, nil, "the menu sends nothing")
	equal(warns(), 1, "one warning for the menu try")
	truthy(
		stub.logged("warn", "rex.ui.pick needs control API revision 17. This server is older"),
		"names the prompt and the revision"
	)

	stub.logs = {}
	ok, result = pcall(
		stub.actions.gascity_mail_mayor.run,
		{ block_id = "block:1", session_id = "session:1" },
		{}
	)
	truthy(ok, "the mail action does not crash: " .. tostring(result))
	equal(result, nil, "the mail action sends nothing")
	equal(warns(), 1, "one warning for the mail try")
	truthy(
		stub.logged("warn", "rex.ui.input needs control API revision 17. This server is older"),
		"names the input prompt and the revision"
	)
	equal(#stub.execs, 0, "no side effect ran")
	equal(#stub.layers, 0, "no pane was made")
end)

test("prompt with a partial rex.ui degrades the same way", function()
	actions.register(app_with({}))
	local saved = rex.ui
	rex.ui = {}
	stub.logs = {}

	local ok, result = pcall(
		stub.actions.gascity_menu.run,
		{ block_id = "block:1", session_id = "session:1" }
	)
	rex.ui = saved
	truthy(ok, "no crash: " .. tostring(result))
	equal(result, nil, "nothing sent")
	equal(warns(), 1, "one warning")
	truthy(stub.logged("warn", "revision 17"), "names the revision")
	equal(#stub.layers, 0, "no pane")
end)


-- The report's look ------------------------------------------------------

local function some_facts()
	local settings = config.resolve(nil)
	settings.city = nil
	stub.pane_cwd = "/work"
	stub.format = probe_answer({ "city|-", "rigs|{}" })
	return report.facts(app_with({}), "block:1", { origin = "palette" })
end

test("report: the terminal form has an icon for every fact", function()
	local printed = report.terminal(some_facts())
	for _, icon in ipairs({ "💻", "📁", "📄", "📺", "🔗", "🔧", "📨" }) do
		contains(printed, icon, "an icon")
	end
end)

test("report: the terminal form colours the heading and labels", function()
	local printed = report.terminal(some_facts())
	contains(printed, "\27[1;38;5;81mGas City\27[0m", "the heading is bold and coloured")
	contains(printed, "\27[38;5;109m", "labels are coloured")
	contains(printed, "where this runs", "and there is a subtitle")
end)

test("report: the terminal form aligns the values under one column", function()
	local facts = some_facts()
	local printed = report.terminal(facts)

	local width = 0
	for _, fact in ipairs(facts) do
		width = math.max(width, #fact.label)
	end

	-- Every label is padded to the widest, so the values start together.
	for _, fact in ipairs(facts) do
		contains(
			printed,
			fact.label .. string.rep(" ", width - #fact.label),
			"the label " .. fact.label .. " is padded"
		)
	end
end)

test("report: the terminal form can warn in colour", function()
	local facts = {
		{ label = "settings", value = "/nowhere (missing)", icon = "document" },
		{ label = "gc", value = "found on PATH", icon = "gear" },
	}
	local printed = report.terminal(facts)
	contains(printed, "\27[38;5;203m/nowhere (missing)", "the trouble is coloured")
	falsy(printed:find("\27[38;5;203mfound on PATH", 1, true), "the rest is not")
end)

test("report: a footer is included when asked for", function()
	local printed = report.terminal(some_facts(), { footer = "Close this pane when you are done." })
	contains(printed, "\27[38;5;240mClose this pane", "the footer, dimmed")
end)

-- The report -------------------------------------------------------------

test("report: names the machine, checkout, settings, and pane", function()
	stub.pane_cwd = "/work"
	stub.format = probe_answer({ "host|other-machine" })
	local app = app_with({})
	local output = report.build(app, "block:1", { origin = "palette", client_id = "client:1" })

	contains(output, "test-machine", "host")
	contains(output, "/repo", "checkout")
	contains(output, app.config.file, "settings file")
	contains(output, "/work", "pane directory")
	contains(output, "on another machine", "says the pane is not here")
	contains(output, "palette, as a client", "where it ran from")
	contains(output, "none listed by gc", "says no cities are listed")
end)

test("report: lists the cities the pane's machine offers", function()
	stub.pane_cwd = "/work/nowhere"
	stub.format = probe_answer({ "city|/cities/one", "city|/cities/two" })
	local output = report.build(app_with({}), "block:1", nil)
	contains(output, "none above the pane", "no pane city")
	contains(output, "/cities/one, /cities/two", "both cities")
end)

test("report: says when the pane's machine does not answer", function()
	stub.pane_cwd = "/work"
	stub.block_error = "session not found"
	local output = report.build(app_with({}), "block:1", nil)
	contains(output, "did not answer", "says so")
end)

test("report: names both machines when they differ", function()
	stub.pane_cwd = "/work"
	stub.format = probe_answer({ "host|Mac-Mini", "pane|/work/city" })
	local output = report.build(app_with({}), "block:1", nil)
	contains(output, "runs on", "the machine running the action")
	contains(output, "(the session's machine)", "and the one with the panes")
	contains(output, "Mac-Mini", "named")
end)

test("report: says when both are the same machine", function()
	stub.pane_cwd = "/work"
	stub.format = probe_answer({ "host|test-machine", "pane|/work/city" })
	local output = report.build(app_with({}), "block:1", nil)
	contains(output, "(the same machine)", "one machine, said once")
end)

test("report: says when the pane is on this machine", function()
	stub.pane_cwd = fixture .. "/city/rig"
	stub.format = probe_answer({ "host|test-machine" })
	local output = report.build(app_with({}), "block:1", nil)
	contains(output, "on this machine", "says the pane is here")
end)

test("report: a readable path on another machine is not on this machine", function()
	stub.pane_cwd = fixture .. "/city/rig"
	stub.format = probe_answer({ "host|other-machine" })
	local output = report.build(app_with({}), "block:1", nil)
	contains(output, "on another machine", "the pane's machine is not this one")
	falsy(output:find("on this machine", 1, true), "never claims the local machine")
end)

test("report: says when the settings file is missing", function()
	local app = app_with({ config = config.resolve(fixture .. "/nope.lua") })
	contains(report.build(app, "block:1", nil), "(missing)", "says it is missing")
end)

-- The entry point --------------------------------------------------------

test("entry: init.lua registers the actions and the binding", function()
	local ok, err = pcall(dofile, root .. "/init.lua")
	truthy(ok, "loads: " .. tostring(err))
	truthy(stub.actions.gascity_mail_mayor ~= nil, "registers the message action")
	truthy(stub.actions.gascity_doctor ~= nil, "registers the report action")
	truthy(stub.actions.gascity_status_update ~= nil, "registers the status update action")
	equal(stub.binds["ctrl+b>g"], "gascity_menu", "binds the key to the menu")
end)

-- Result -----------------------------------------------------------------

print("")
if failures > 0 then
	print(string.format("%d of %d checks failed", failures, checks))
	os.exit(1)
end
print(string.format("%d checks passed", checks))
