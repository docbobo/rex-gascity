-- Shell lines that leave the gc program in $gc.
--
-- The command runs in a pane of the session, so it runs on the machine that
-- owns the pane, with that machine's PATH. A login shell is not used, so a Nix
-- or devshell install can be missing from it.

local text = require("gascity.text")

local M = {}

-- What a script says, and an action raises, when the machine has no gc.
M.MISSING = "gc is not on this machine. Install Gas City, or set gc_bin in ~/.config/rex/gascity.lua."

-- Where gc might be when PATH misses it.
M.places = {
	"/run/current-system/sw/bin/gc",
	"/etc/profiles/per-user/$USER/bin/gc",
	"$HOME/.nix-profile/bin/gc",
	"$HOME/.local/bin/gc",
	"$HOME/go/bin/gc",
}

-- The lines to put in a shell script before gc is used.
function M.words(settings)
	if settings.gc_bin then
		return { "gc=" .. text.quote(settings.gc_bin) }
	end
	-- Each place is quoted so a space in $HOME or $USER cannot split it into
	-- two words; the double quotes still let the shell expand those variables.
	local places = {}
	for _, place in ipairs(M.places) do
		places[#places + 1] = '"' .. place .. '"'
	end
	return {
		"gc=gc",
		"if ! command -v gc >/dev/null 2>&1; then",
		"\tfor p in " .. table.concat(places, " ") .. "; do",
		'\t\tif [ -x "$p" ]; then gc="$p"; break; fi',
		"\tdone",
		"fi",
	}
end

-- The lines that stop a script with a clear message when there is no gc. Not
-- part of `words`, because a script that exits early leaves its end marker
-- unprinted, and a hidden block would then wait out its patience for an answer
-- that is not coming.
--
-- `opts.exiting = false` returns instead, for a script that must carry on to
-- print its status.
function M.guard(opts)
	opts = opts or {}
	local stop = opts.exiting == false and "return 127" or "exit 127"
	return {
		'if ! command -v "$gc" >/dev/null 2>&1; then',
		"\tprintf '%s\\n' " .. text.quote(M.MISSING) .. " >&2",
		"\t" .. stop,
		"fi",
	}
end

return M
