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
local actions = require("gascity.actions")

local fixture = root .. "/tests/fixtures"

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
	json_text = nil,
	json_value = nil,
	json_error = nil,
	blocks = {},
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
	stub.closed = {}
	stub.picks = nil
	stub.pick_count = 0
	stub.picked = nil
	stub.pick_why = nil
	stub.block_error = nil
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
	equal(#stub.picks.items, 4, "four commands")
	equal(stub.picks.items[1].title, "Message the Mayor…", "the first")
	equal(stub.picks.items[2].title, "Suspend a Rig…", "the second")
	equal(stub.picks.items[3].title, "Resume a Rig…", "the third")
	equal(stub.picks.items[4].title, "Where This Runs…", "the fourth")
	truthy(stub.picks.items[1].subtitle ~= nil, "each says what it does")
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
	equal(stub.binds["ctrl+b>g"], "gascity_menu", "binds the key to the menu")
end)

-- Result -----------------------------------------------------------------

print("")
if failures > 0 then
	print(string.format("%d of %d checks failed", failures, checks))
	os.exit(1)
end
print(string.format("%d checks passed", checks))
