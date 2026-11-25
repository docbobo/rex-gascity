#!/bin/sh
# pack/commands/status-update/run.sh against a fake `rex`.
#
# The fake logs its argv (one word per line) and its stdin, the
# session.new_layer request, so each case can read exactly what reached Rex.
# REX_FAKE_EXIT makes it fail with that status and "boom" on stderr.
#
# Usage: sh pack/tests/check-status-update.sh
# Exits non-zero with a one-line reason on the first failed assertion.
set -eu

repo=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd -P)
cmd=$repo/pack/commands/status-update/run.sh
work=$(mktemp -d "${TMPDIR:-/tmp}/check-status-update.XXXXXX")
trap 'rm -rf "$work"' EXIT HUP INT TERM

fail() {
  echo "check-status-update: $*" >&2
  exit 1
}

tools=$work/tools
mkdir -p "$tools"
cat > "$tools/rex" <<'FAKE'
#!/bin/sh
for word in "$@"; do printf '%s\n' "$word"; done > "$REX_FAKE_LOG.argv"
cat > "$REX_FAKE_LOG.stdin"
if [ -n "${REX_FAKE_EXIT:-}" ]; then
  echo boom >&2
  exit "$REX_FAKE_EXIT"
fi
printf '%s\n' '{"layer_id":"layer:1","block_ids":["block:1"],"revision":1}'
FAKE
chmod +x "$tools/rex"

log=$work/rex
# run <args...>: runs the command with the fake rex first on PATH. Its stdout
# goes to $work/out, its stderr to $work/err, and its status to $code.
run() {
  rm -f "$log.argv" "$log.stdin"
  code=0
  PATH="$tools:/usr/bin:/bin" REX_FAKE_LOG=$log REX_BIN_DIR= GC_PACK_DIR= \
    sh "$cmd" "$@" > "$work/out" 2> "$work/err" || code=$?
}

has() { # has <file> <text> <what>
  grep -qF -- "$2" "$1" || fail "$3: $(basename "$1") lacks: $2"
}
lacks() { # lacks <file> <text> <what>
  if grep -qF -- "$2" "$1"; then fail "$3: $(basename "$1") has: $2"; fi
}
code_is() { # code_is <want> <what>
  [ "$code" -eq "$1" ] || fail "$2: exit $code, want $1 ($(cat "$work/err"))"
}
no_rex() { # no_rex <what>
  [ ! -e "$log.argv" ] || fail "$1: rex was called"
}

# json_ok <file> <what>: the captured request parses and has the layer shape.
json_ok() { # json_ok <file> <what>
  python3 - "$1" <<'PY' || fail "$2: request is not the expected JSON: $(head -c 300 "$1")"
import json, sys

req = json.load(open(sys.argv[1]))
assert isinstance(req["session_id"], str), req
assert req["bounds"] == [0.08, 0.16, 0.84, 0.56], req
assert req["focus"] is True, req
block = req["layout"]["block"]
assert block["flavor"] == "com.superlogical.terminal.shell", block
assert block["label"] == "gascity-status", block
opts = block["options"]
assert opts["exit"] == {"on_completion": False}, opts
assert opts["command"][:2] == ["/bin/sh", "-c"], opts
PY
}

# pane_look <request> <what>: the pane command runs and prints the report look.
pane_look() { # pane_look <request> <what>
  python3 - "$1" > "$work/pane-script" <<'PY' || fail "$2: request has no pane command"
import json, sys
print(json.load(open(sys.argv[1]))["layout"]["block"]["options"]["command"][2])
PY
  sh "$work/pane-script" > "$work/pane-out" || fail "$2: the pane command failed"
  has "$work/pane-out" "Close this pane when you are done." "$2 pane"
}

body=$work/body.md
printf '%s\n' '# Status update — alpha' '## Done' '- shipped **it**' '## Blocked' \
  '- waiting on `ci` before deploy' '## Needs you' '- approve **the change** today' > "$body"

# --template prints the template next to the script, or from GC_PACK_DIR.
run --template < /dev/null
code_is 0 "--template"
has "$work/out" "## Needs you" "--template"

# A GC_PACK_DIR template wins over the sibling, so only content that the pack
# dir has proves the lookup. Without GC_PACK_DIR, the sibling template is used.
altpack=$work/altpack
mkdir -p "$altpack/commands/status-update"
printf '%s\n' '# Template from the pack dir' > "$altpack/commands/status-update/template.md"
code=0
GC_PACK_DIR=$altpack sh "$cmd" --template > "$work/out" 2> "$work/err" < /dev/null || code=$?
code_is 0 "--template with GC_PACK_DIR"
has "$work/out" "# Template from the pack dir" "--template with GC_PACK_DIR"
lacks "$work/out" "## Needs you" "--template with GC_PACK_DIR"

# Usage errors exit 2 before anything else.
run < "$body"
code_is 2 "no --session"
has "$work/err" "--session is required" "no --session"
no_rex "no --session"

run --session ID --bogus < "$body"
code_is 2 "unknown flag"
has "$work/err" "unknown argument: --bogus" "unknown flag"
no_rex "unknown flag"

# An option is not a value: a missing value exits 2 without calling rex.
# A literal value that starts with - stays available through --name=value.
for opt in --session --server --display --body-file; do
  run "$opt" --bogus < "$body"
  code_is 2 "missing value for $opt"
  has "$work/err" "$opt needs a value" "missing value for $opt"
  no_rex "missing value for $opt"
done
run --session=-ID < "$body"
code_is 0 "--session= with a - value"
has "$log.stdin" '"session_id":"-ID"' "--session= with a - value"

# An empty update and an unknown display exit 1, and rex is never called.
printf '  \n\n' > "$work/empty"
run --session ID < "$work/empty"
code_is 1 "empty body"
has "$work/err" "the status update is empty" "empty body"
no_rex "empty body"

run --session ID --display bogus < "$body"
code_is 1 "unknown display"
has "$work/err" "display bogus is not available (text)" "unknown display"
no_rex "unknown display"

# The request: argv, session, keep-open, and the report look.
run --server S --session ID < "$body"
code_is 0 "a body on stdin"
has "$work/out" "Opened the status update in session ID" "a body on stdin"
printf '%s\n' -S S api call session.new_layer -s ID - > "$work/want-argv"
cmp -s "$work/want-argv" "$log.argv" || fail "argv: got $(tr '\n' ' ' < "$log.argv")"
req=$log.stdin
has "$req" '"session_id":"ID"' "request"
has "$req" '"bounds":[0.08,0.16,0.84,0.56],"focus":true' "request"
has "$req" '"layout":{"block":{"flavor":"com.superlogical.terminal.shell","label":"gascity-status"' "request"
has "$req" '"command":["/bin/sh","-c","printf '"'"'%s\\n'"'"' '"'" "request"
has "$req" '"exit":{"on_completion":false}' "request"
has "$req" '\u001b[1;38;5;81mGas City' "title"
has "$req" '\u001b[38;5;245m    Status update — alpha' "title"
has "$req" '✅  \u001b[1;38;5;109mDone' "Done section"
has "$req" '⛔  \u001b[1;38;5;109mBlocked' "Blocked section"
has "$req" '    • shipped \u001b[1mit\u001b[0m' "bold item"
has "$req" '\u001b[38;5;203mwaiting on \u001b[38;5;81mci\u001b[0m' "red Blocked item with code"
has "$req" '\u001b[38;5;203mwaiting on \u001b[38;5;81mci\u001b[0m\u001b[38;5;203m before deploy' "red after a code span"
has "$req" '\u001b[38;5;203mapprove \u001b[1mthe change\u001b[0m\u001b[38;5;203m today' "red after a bold span"
has "$req" '\u001b[38;5;240mClose this pane when you are done.' "footer"
json_ok "$req" "request"
pane_look "$req" "request"
has "$work/pane-out" "$(printf '\033')[38;5;203m before deploy" "pane red after a code span"
has "$work/pane-out" "$(printf '\033')[1mthe change$(printf '\033')[0m$(printf '\033')[38;5;203m today" "pane red after a bold span"

# --body-file sends the same request as stdin.
cp "$req" "$work/stdin-request"
run --server S --session ID --body-file "$body" < /dev/null
code_is 0 "--body-file"
cmp -s "$work/stdin-request" "$log.stdin" || fail "--body-file: request differs from stdin"

# Quotes, backslashes, and tabs are escaped for JSON; ' for the pane's sh.
printf '%s\n' '## Summary' 'say "hi" \ it'"'"'s' "a	tab" > "$work/quotes"
run --session ID < "$work/quotes"
code_is 0 "escapes"
has "$req" 'say \"hi\" \\ it'"'"'\\'"'"''"'"'s' "escapes"
has "$req" 'a\ttab' "tab"
json_ok "$req" "escapes"
pane_look "$req" "escapes"

# A value that ends with a newline keeps it as a JSON \n.
nl='
'
run --session "ID${nl}" < "$body"
code_is 0 "trailing newline"
has "$log.stdin" '"session_id":"ID\n"' "trailing newline"
lacks "$log.stdin" '"session_id":"ID"' "trailing newline"

# No --server, no -S: rex uses its own default server.
run --session ID < "$body"
code_is 0 "no --server"
lacks "$log.argv" "-S" "no --server"

# rex's failure is the command's failure.
REX_FAKE_EXIT=3 run --session ID < "$body"
code_is 3 "rex fails"
has "$work/err" "boom" "rex fails"
lacks "$work/out" "Opened" "rex fails"

# No rex anywhere: 127.
code=0
PATH=/usr/bin:/bin REX_BIN_DIR= sh "$cmd" --session ID < "$body" > "$work/out" 2> "$work/err" || code=$?
code_is 127 "no rex"
has "$work/err" "rex is not on PATH" "no rex"

# rex from REX_BIN_DIR when PATH has none.
code=0
rm -f "$log.argv"
PATH=/usr/bin:/bin REX_BIN_DIR=$tools REX_FAKE_LOG=$log sh "$cmd" --session ID < "$body" > "$work/out" 2> "$work/err" || code=$?
code_is 0 "REX_BIN_DIR"
[ -e "$log.argv" ] || fail "REX_BIN_DIR: the fake rex was not called"

# The body cannot drive the terminal: its controls are stripped, and every
# ESC left in the request starts one of the renderer's SGR sequences.
printf '# Status update\n## Done\n- a\033[2Jb\033]0;t\007c\302\233x\n' > "$work/controls"
run --session ID < "$work/controls"
code_is 0 "controls"
has "$req" 'a[2Jb]0;tcx' "controls stripped"
has "$req" '\u001b[1;38;5;81mGas City' "renderer colors kept"
esc=$(printf '\033')
if LC_ALL=C grep -q "$esc" "$req"; then fail "controls: a raw ESC reached the request"; fi
c1=$(printf '\302\233')
if LC_ALL=C grep -q "$c1" "$req"; then fail "controls: a C1 control reached the request"; fi
LC_ALL=C awk '
  { n = split($0, part, /\\u001b/)
    for (i = 2; i <= n; i++) if (part[i] !~ /^\[[0-9;]*m/) { print "bad: " substr(part[i], 1, 12); bad = 1 } }
  END { exit bad }
' "$req" || fail "controls: an ESC in the request is not an SGR sequence"

# A filter that fails after it reads part of the body is a read error: exit 1
# and no rex call, so a partial body can never become a partial update.
cat > "$tools/tr" <<'FAKE'
#!/bin/sh
printf '%s\n' '## Blocked' '- partial'
exit 1
FAKE
chmod +x "$tools/tr"
run --session ID < "$body"
code_is 1 "read error"
has "$work/err" "cannot read the status update" "read error"
no_rex "read error"
rm -f "$tools/tr"

echo "check-status-update: ok"
