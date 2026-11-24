package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeRex is a fake `rex` executable. It records every invocation and prints a
// canned reply keyed by the operation, so the adapter's argv and JSON mapping
// can be asserted without a live Rex server.
type fakeRex struct {
	bin       string
	log       string
	responses string
}

func newFakeRex(t *testing.T) *fakeRex {
	t.Helper()
	dir := t.TempDir()
	f := &fakeRex{
		bin:       filepath.Join(dir, "rex"),
		log:       filepath.Join(dir, "args.log"),
		responses: filepath.Join(dir, "responses"),
	}
	if err := os.MkdirAll(f.responses, 0o755); err != nil {
		t.Fatal(err)
	}
	script := `#!/bin/sh
log=` + shellQuote(f.log) + `
dir=` + shellQuote(f.responses) + `
{
  for a in "$@"; do printf '%s\n' "$a"; done
  printf '%s\n' '---'
} >> "$log"

# A leading -S <server> comes from cli.Client{Server}; the rest of the script
# looks at the command that follows it.
if [ "$1" = "-S" ]; then
  shift 2
fi

if [ "$1" = "events" ]; then
  key=events
elif [ "$1" = "send-key" ]; then
  key=send-key
else
  key="$3"
fi
if [ "$key" = "block.method" ]; then
  case "$*" in
    *'"method":"format"'*) key=format ;;
    *'"method":"blob_read"'*) key=blob_read ;;
    *'"method":"process"'*) key=process ;;
    *'"method":"write"'*) key=write ;;
    *'"method":"clear"'*) key=clear ;;
    *'"method":"size"'*) key=size ;;
    *'"method":"program_status"'*) key=program_status ;;
    *'"method":"input_mode"'*) key=input_mode ;;
  esac
fi
# A scripted failure: <key>.exit holds the exit code to fail with.
if [ -f "$dir/$key.exit" ]; then
  cat "$dir/$key.exit" >&2
  exit "$(cat "$dir/$key.exit")"
fi
# Serve <key>.<n>.json for the nth call when present, else <key>.json, so a
# test can script a multi-call sequence such as two blob_reads.
n=$(cat "$dir/$key.n" 2>/dev/null || echo 0)
n=$((n + 1))
printf '%s' "$n" > "$dir/$key.n"
if [ -f "$dir/$key.$n.json" ]; then
  cat "$dir/$key.$n.json"
elif [ -f "$dir/$key.json" ]; then
  cat "$dir/$key.json"
fi
`
	if err := os.WriteFile(f.bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return f
}

// respond sets the canned stdout for a key.
func (f *fakeRex) respond(t *testing.T, key, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(f.responses, key+".json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// calls returns each recorded argv, in order.
func (f *fakeRex) calls(t *testing.T) [][]string {
	t.Helper()
	data, err := os.ReadFile(f.log)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatal(err)
	}
	var calls [][]string
	var cur []string
	for _, line := range strings.Split(string(data), "\n") {
		switch {
		case line == "---":
			calls = append(calls, cur)
			cur = nil
		case line != "" || len(cur) > 0:
			cur = append(cur, line)
		}
	}
	return calls
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
