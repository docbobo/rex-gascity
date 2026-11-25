#!/bin/sh
# gc <binding> status-update: show a status update in a floating Rex pane.
#
# The mayor fills in template.md and pipes it here. The update is rendered in
# the look of the Gas City UI's doctor report and opened in a floating,
# kept-open terminal pane of the named session through
# `rex api call session.new_layer`. See help.md.
set -eu

here=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
dir=$here
if [ -n "${GC_PACK_DIR:-}" ] && [ -f "$GC_PACK_DIR/commands/status-update/template.md" ]; then
	dir=$GC_PACK_DIR/commands/status-update
fi

die() {
	printf 'status-update: %s\n' "$*" >&2
	exit 1
}

usage() {
	[ "$#" -eq 0 ] || printf 'status-update: %s\n' "$*" >&2
	cat >&2 <<'USAGE'
usage: gc <binding> status-update --session ID [--server SERVER] [--display text] [--body-file FILE]
       gc <binding> status-update --template
USAGE
	exit 2
}

# --- arguments ---------------------------------------------------------------

session=
server=
display=text
body_file=
template=0

# has_value: the next word exists and does not look like another option.
has_value() {
	[ "$#" -ge 2 ] && [ "${2#-}" = "$2" ]
}

while [ "$#" -gt 0 ]; do
	case $1 in
		--session) has_value "$@" || usage "--session needs a value"; session=$2; shift 2 ;;
		--session=*) session=${1#*=}; shift ;;
		--server) has_value "$@" || usage "--server needs a value"; server=$2; shift 2 ;;
		--server=*) server=${1#*=}; shift ;;
		--display) has_value "$@" || usage "--display needs a value"; display=$2; shift 2 ;;
		--display=*) display=${1#*=}; shift ;;
		--body-file) has_value "$@" || usage "--body-file needs a value"; body_file=$2; shift 2 ;;
		--body-file=*) body_file=${1#*=}; shift ;;
		--template) template=1; shift ;;
		-h | --help) cat "$dir/help.md"; exit 0 ;;
		*) usage "unknown argument: $1" ;;
	esac
done

if [ "$template" -eq 1 ]; then
	cat "$dir/template.md"
	exit 0
fi

[ -n "$session" ] || usage "--session is required"

# --- the body ----------------------------------------------------------------

# Terminal controls in the body could drive the pane (clear it, retitle it,
# move the cursor). Drop every C0 byte except tab and newline, DEL, and the
# two-byte UTF-8 C1 controls (U+0080-U+009F). What is left of an escape
# sequence, such as "[2J", stays as plain text.
c1_lead=$(printf '\302')
c1_low=$(printf '\200')
c1_high=$(printf '\237')
strip_controls() {
	LC_ALL=C tr -d '\000-\010\013-\037\177' | LC_ALL=C sed "s/${c1_lead}[${c1_low}-${c1_high}]//g"
}

if [ -n "$body_file" ]; then
	[ -r "$body_file" ] || die "cannot read $body_file"
	body=$(strip_controls < "$body_file")
else
	body=$(strip_controls)
fi

[ -n "$(printf '%s' "$body" | tr -d ' \t\n')" ] || die "the status update is empty"

# --- displays ----------------------------------------------------------------

# Quotes one word for /bin/sh: the result is single-quoted, and a ' inside
# becomes '\''.
sh_quote() {
	printf "'%s'" "$(printf '%s' "$1" | sed "s/'/'\\\\''/g")"
}

# The doctor report's look (ui/lua/gascity/report.lua, M.terminal) for the
# template's Markdown subset. Reads the sanitized body on stdin.
render_text() {
	awk '
	BEGIN {
		E = "\033"
		rule = ""
		for (i = 0; i < 48; i++) rule = rule "─"
		section = ""
	}
	function paint(code, s) { return E "[" code "m" s E "[0m" }
	# span: a styled run inside the base style of the line. Its reset turns the
	# base back on, so the text after the span keeps the base color.
	function span(base, code, s) {
		return E "[" code "m" s E "[0m" (base == "" ? "" : E "[" base "m")
	}
	function inline(s, base) {
		if (base != "") s = E "[" base "m" s
		while (match(s, /\*\*[^*]+\*\*/))
			s = substr(s, 1, RSTART - 1) span(base, "1", substr(s, RSTART + 2, RLENGTH - 4)) substr(s, RSTART + RLENGTH)
		while (match(s, /`[^`]+`/))
			s = substr(s, 1, RSTART - 1) span(base, "38;5;81", substr(s, RSTART + 1, RLENGTH - 2)) substr(s, RSTART + RLENGTH)
		if (base != "") s = s E "[0m"
		return s
	}
	function icon(name) {
		if (name == "Summary") return "📋"
		if (name == "Done") return "✅"
		if (name == "In progress") return "🔄"
		if (name == "Blocked") return "⛔"
		if (name == "Needs you") return "🙋"
		if (name == "Next") return "⏭️"
		return "•"
	}
	/^# / {
		title = substr($0, 3)
		print ""
		print "  " paint("1;38;5;81", "Gas City") paint("38;5;245", "    " title)
		print "  " paint("38;5;238", rule)
		next
	}
	/^## / {
		section = substr($0, 4)
		sub(/[ \t]+$/, "", section)
		print ""
		print "  " icon(section) "  " paint("1;38;5;109", section)
		next
	}
	/^[-*] / {
		base = (section == "Blocked" || section == "Needs you") ? "38;5;203" : ""
		print "    • " inline(substr($0, 3), base)
		next
	}
	/^[ \t]*$/ { print ""; next }
	{ print "  " inline($0) }
	END {
		print ""
		print "  " paint("38;5;240", "Close this pane when you are done.")
		print ""
	}
	'
}

# Each display turns the body into the script the pane runs.
display_text() {
	printf "printf '%%s\\\\n' %s" "$(sh_quote "$(printf '%s\n' "$1" | render_text)")"
}

# The one place a display is chosen. A new display adds a branch here and a
# display_<mode> function above; nothing else changes.
case $display in
	text) script=$(display_text "$body") ;;
	*) die "display $display is not available (text)" ;;
esac

# --- the request ---------------------------------------------------------------

# A JSON string: \ " newline tab and ESC escaped, other C0 bytes dropped,
# UTF-8 passed through byte for byte. The added newline turns a value's own
# final newline into an empty awk record, which keeps it as a JSON \n.
json_string() {
	printf '%s\n' "$1" | LC_ALL=C awk '
	BEGIN { ORS = ""; printf "\"" }
	{
		if (NR > 1) printf "\\n"
		n = length($0)
		for (i = 1; i <= n; i++) {
			c = substr($0, i, 1)
			if (c == "\\") printf "\\\\"
			else if (c == "\"") printf "\\\""
			else if (c == "\t") printf "\\t"
			else if (c == "\033") printf "\\u001b"
			else if (c < " ") continue
			else printf "%s", c
		}
	}
	END { printf "\"" }
	'
}

request=$(printf '{"session_id":%s,"bounds":[0.08,0.16,0.84,0.56],"focus":true,"layout":{"block":{"flavor":"com.superlogical.terminal.shell","label":"gascity-status","options":{"command":["/bin/sh","-c",%s],"exit":{"on_completion":false}}}}}' \
	"$(json_string "$session")" "$(json_string "$script")")

# --- the pane ------------------------------------------------------------------

rex=$(command -v rex 2>/dev/null || true)
if [ -z "$rex" ] && [ -n "${REX_BIN_DIR:-}" ] && [ -x "$REX_BIN_DIR/rex" ]; then
	rex=$REX_BIN_DIR/rex
fi
if [ -z "$rex" ]; then
	printf 'status-update: rex is not on PATH, and REX_BIN_DIR names none\n' >&2
	exit 127
fi

status=0
printf '%s' "$request" | "$rex" ${server:+-S "$server"} api call session.new_layer -s "$session" - >/dev/null || status=$?
[ "$status" -eq 0 ] || exit "$status"

printf 'Opened the status update in session %s\n' "$session"
