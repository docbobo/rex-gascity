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
  PATH="$tools:/usr/bin:/bin" REX_FAKE_LOG=$log REX_BIN_DIR= \
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

body=$work/body.md
printf '%s\n' '# Status update — alpha' '## Done' '- shipped **it**' '## Blocked' '- waiting on `ci`' > "$body"

# --template prints the template, from next to the script and from GC_PACK_DIR.
run --template < /dev/null
code_is 0 "--template"
has "$work/out" "## Needs you" "--template"
code=0
GC_PACK_DIR=$repo/pack sh "$cmd" --template > "$work/out" 2> "$work/err" < /dev/null || code=$?
code_is 0 "--template with GC_PACK_DIR"
has "$work/out" "## Summary" "--template with GC_PACK_DIR"

# Usage errors exit 2 before anything else.
run < "$body"
code_is 2 "no --session"
has "$work/err" "--session is required" "no --session"
no_rex "no --session"

run --session ID --bogus < "$body"
code_is 2 "unknown flag"
has "$work/err" "unknown argument: --bogus" "unknown flag"
no_rex "unknown flag"

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

echo "check-status-update: ok"
