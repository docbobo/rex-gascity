-- Gas City UI for Rex.
--
-- ~/.config/rex/user.lua loads this file with dofile. Settings come from
-- ~/.config/rex/gascity.lua, or from the file $REX_GASCITY_CONFIG names.
--
-- Rex has no notion of an extension directory, so the checkout is found from
-- this file's own path. Modules are resolved against lua/ and their cache
-- entries are dropped first, so `rex config reload` picks up edits.

-- Rex reports the source without the leading "@" the stand-alone Lua
-- interpreter adds, so both forms are accepted.
local source = (debug.getinfo(1, "S") or {}).source or ""
local root = source:gsub("^@", ""):match("^(.*)/[^/]+$")
if root == nil then
	rex.log("error", "gascity: cannot find the extension directory from " .. source)
	return
end

package.path = root .. "/lua/?.lua;" .. root .. "/lua/?/init.lua;" .. package.path
for _, name in ipairs({
	"text",
	"ansi",
	"config",
	"pane",
	"gc",
	"probe",
	"machine",
	"rigs",
	"run",
	"mail",
	"send",
	"report",
	"status",
	"actions",
}) do
	package.loaded["gascity." .. name] = nil
end

local config = require("gascity.config")
local actions = require("gascity.actions")

actions.register({
	root = root,
	config = config.resolve(os.getenv("REX_GASCITY_CONFIG") or config.default_path()),
	pane = require("gascity.pane"),
	machine = require("gascity.machine"),
	rigs = require("gascity.rigs"),
	run = require("gascity.run"),
	send = require("gascity.send"),
	report = require("gascity.report"),
	status = require("gascity.status"),
})
