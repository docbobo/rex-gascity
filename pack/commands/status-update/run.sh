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

while [ "$#" -gt 0 ]; do
	case $1 in
		--session) [ "$#" -ge 2 ] || usage "--session needs a value"; session=$2; shift 2 ;;
		--session=*) session=${1#*=}; shift ;;
		--server) [ "$#" -ge 2 ] || usage "--server needs a value"; server=$2; shift 2 ;;
		--server=*) server=${1#*=}; shift ;;
		--display) [ "$#" -ge 2 ] || usage "--display needs a value"; display=$2; shift 2 ;;
		--display=*) display=${1#*=}; shift ;;
		--body-file) [ "$#" -ge 2 ] || usage "--body-file needs a value"; body_file=$2; shift 2 ;;
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

if [ -n "$body_file" ]; then
	[ -r "$body_file" ] || die "cannot read $body_file"
	body=$(cat < "$body_file")
else
	body=$(cat)
fi

[ -n "$(printf '%s' "$body" | tr -d ' \t\n')" ] || die "the status update is empty"

# --- displays ----------------------------------------------------------------

# Quotes one word for /bin/sh: the result is single-quoted, and a ' inside
# becomes '\''.
sh_quote() {
	printf "'%s'" "$(printf '%s' "$1" | sed "s/'/'\\\\''/g")"
}

# Task 2 replaces this with the doctor report's look.
render_text() {
	cat
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
# UTF-8 passed through byte for byte.
json_string() {
	printf '%s' "$1" | LC_ALL=C awk '
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
