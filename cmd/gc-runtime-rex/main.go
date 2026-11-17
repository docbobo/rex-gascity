// Command gc-runtime-rex is the RPP v0 dispatcher for the Gas City Rex
// provider. gc runs it once per operation:
//
//	gc-runtime-rex <op> <session-name> [args...]
//
// Exit codes follow RPP v0: 0 success, 1 failure (message on stderr), 2 unknown
// or unimplemented operation. See docs/REX-CONTRACT.md §9.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/docbobo/rex-gascity/internal/provider"
	"github.com/docbobo/rex-gascity/internal/rex"
	"github.com/docbobo/rex-gascity/internal/rex/cli"
)

const (
	exitOK      = 0
	exitFail    = 1
	exitUnknown = 2
)

// handshake is the protocol op's reply. It declares only what is implemented:
// no proc.exec, proc.provision, proc.stream or tty.attach, and no locality.
const handshake = `{"version":0,"capabilities":["report-attachment","report-activity"]}`

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr, liveDeps()))
}

// deps are the dispatcher's external dependencies, injected so tests can swap
// the Rex client for a fake.
type deps struct {
	newClient func() rex.Client
}

// liveDeps drives the real rex CLI. GC_REX_BIN overrides the binary; empty
// means the `rex` on PATH.
func liveDeps() deps {
	return deps{newClient: func() rex.Client { return &cli.Client{Bin: os.Getenv("GC_REX_BIN")} }}
}

// startConfig is the start op's stdin JSON. Other fields gc sends are ignored.
type startConfig struct {
	WorkDir           string            `json:"work_dir"`
	Command           string            `json:"command"`
	ReadyPromptPrefix string            `json:"ready_prompt_prefix"`
	Env               map[string]string `json:"env"`
	PreStart          []string          `json:"pre_start"`
}

// opSpec describes a known op: how many positional args it requires after the
// op name, and its handler. args passed to the handler start at the session
// name (or prefix for list-running).
type opSpec struct {
	required int
	usage    string
	handle   func(ctx context.Context, p *provider.Provider, args []string, stdin io.Reader, stdout io.Writer) error
}

// unimplemented ops exit 2. They are listed so the intent is explicit; any
// other unrecognized op also exits 2.
var unimplemented = map[string]bool{
	"exec":          true,
	"provision":     true,
	"attach":        true,
	"watch-startup": true,
}

var ops = map[string]opSpec{
	"start":             {1, "start <name>", opStart},
	"stop":              {1, "stop <name>", opStop},
	"is-running":        {1, "is-running <name>", opIsRunning},
	"interrupt":         {1, "interrupt <name>", opInterrupt},
	"nudge":             {1, "nudge <name>", opNudge},
	"send-keys":         {1, "send-keys <name> [keys...]", opSendKeys},
	"clear-scrollback":  {1, "clear-scrollback <name>", opClearScrollback},
	"process-alive":     {1, "process-alive <name>", opProcessAlive},
	"peek":              {1, "peek <name> [lines]", opPeek},
	"list-running":      {0, "list-running [prefix]", opListRunning},
	"get-last-activity": {1, "get-last-activity <name>", opGetLastActivity},
	"is-attached":       {1, "is-attached <name>", opIsAttached},
	"set-meta":          {2, "set-meta <name> <key>", opSetMeta},
	"get-meta":          {2, "get-meta <name> <key>", opGetMeta},
	"remove-meta":       {2, "remove-meta <name> <key>", opRemoveMeta},
}

// run dispatches one RPP op and returns the process exit code. Failures write
// nothing to stdout; the op's output is buffered and flushed only on success.
func run(args []string, stdin io.Reader, stdout, stderr io.Writer, d deps) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "gc-runtime-rex: no operation given")
		return exitUnknown
	}
	op, rest := args[0], args[1:]

	if op == "protocol" {
		if _, err := fmt.Fprintln(stdout, handshake); err != nil {
			fmt.Fprintf(stderr, "gc-runtime-rex: protocol: write output: %v\n", err)
			return exitFail
		}
		return exitOK
	}
	spec, known := ops[op]
	if !known {
		if unimplemented[op] {
			fmt.Fprintf(stderr, "gc-runtime-rex: operation %q not implemented\n", op)
		} else {
			fmt.Fprintf(stderr, "gc-runtime-rex: unknown operation %q\n", op)
		}
		return exitUnknown
	}
	if len(rest) < spec.required || (spec.required > 0 && rest[0] == "") {
		fmt.Fprintf(stderr, "gc-runtime-rex: %s: missing argument; usage: gc-runtime-rex %s\n", op, spec.usage)
		return exitFail
	}

	city := provider.CityFromEnv()
	dir, err := provider.StateDir(city)
	if err != nil {
		fmt.Fprintf(stderr, "gc-runtime-rex: %s: state dir: %v\n", op, err)
		return exitFail
	}
	prov := provider.New(city, d.newClient(), provider.WithStateDir(dir))

	var out bytes.Buffer
	if err := spec.handle(context.Background(), prov, rest, stdin, &out); err != nil {
		fmt.Fprintf(stderr, "gc-runtime-rex: %s: %v\n", op, err)
		return exitFail
	}
	if _, err := stdout.Write(out.Bytes()); err != nil {
		fmt.Fprintf(stderr, "gc-runtime-rex: %s: write output: %v\n", op, err)
		return exitFail
	}
	return exitOK
}

// bestEffort maps a missing session to success: input to an agent that is
// not there is a no-op for gc, not a failure.
func bestEffort(err error) error {
	if errors.Is(err, provider.ErrSessionNotFound) {
		return nil
	}
	return err
}

func printBool(w io.Writer, b bool) {
	fmt.Fprintln(w, strconv.FormatBool(b))
}

func opStart(ctx context.Context, p *provider.Provider, args []string, stdin io.Reader, _ io.Writer) error {
	name := args[0]
	raw, err := io.ReadAll(stdin)
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}
	var cfg startConfig
	if len(bytes.TrimSpace(raw)) > 0 {
		if err := json.Unmarshal(raw, &cfg); err != nil {
			return fmt.Errorf("parse config: %w", err)
		}
	}
	err = p.StartNew(ctx, name, provider.Config{
		Argv:              commandArgv(cfg.Command),
		WorkDir:           cfg.WorkDir,
		ReadyPromptPrefix: cfg.ReadyPromptPrefix,
		Env:               cfg.Env,
		PreStart:          cfg.PreStart,
	})
	if errors.Is(err, provider.ErrAlreadyRunning) {
		// gc's exec proxy recognizes "already running" as a start collision.
		return fmt.Errorf("session %q already running", name)
	}
	return err
}

func opStop(ctx context.Context, p *provider.Provider, args []string, _ io.Reader, _ io.Writer) error {
	return p.Stop(ctx, args[0])
}

func opIsRunning(ctx context.Context, p *provider.Provider, args []string, _ io.Reader, out io.Writer) error {
	running, err := p.IsRunning(ctx, args[0])
	if err != nil {
		return err
	}
	printBool(out, running)
	return nil
}

func opInterrupt(ctx context.Context, p *provider.Provider, args []string, _ io.Reader, _ io.Writer) error {
	return bestEffort(p.Interrupt(ctx, args[0]))
}

func opNudge(ctx context.Context, p *provider.Provider, args []string, stdin io.Reader, _ io.Writer) error {
	text, err := io.ReadAll(stdin)
	if err != nil {
		return fmt.Errorf("read message: %w", err)
	}
	return bestEffort(p.Nudge(ctx, args[0], string(text)))
}

func opSendKeys(ctx context.Context, p *provider.Provider, args []string, _ io.Reader, _ io.Writer) error {
	keys := args[1:]
	if len(keys) == 0 {
		return nil
	}
	return bestEffort(p.SendKeys(ctx, args[0], keys...))
}

func opClearScrollback(ctx context.Context, p *provider.Provider, args []string, _ io.Reader, _ io.Writer) error {
	return bestEffort(p.ClearScrollback(ctx, args[0]))
}

func opProcessAlive(ctx context.Context, p *provider.Provider, args []string, stdin io.Reader, out io.Writer) error {
	raw, err := io.ReadAll(stdin)
	if err != nil {
		return fmt.Errorf("read process names: %w", err)
	}
	names := strings.Split(string(raw), "\n")
	if names[len(names)-1] == "" {
		names = names[:len(names)-1]
	}
	alive, err := p.ProcessAlive(ctx, args[0], names)
	if err != nil {
		return err
	}
	printBool(out, alive)
	return nil
}

func opPeek(ctx context.Context, p *provider.Provider, args []string, _ io.Reader, out io.Writer) error {
	lines := 0
	if len(args) > 1 && args[1] != "" {
		n, err := strconv.Atoi(args[1])
		if err != nil {
			return fmt.Errorf("invalid line count %q", args[1])
		}
		lines = n
	}
	text, err := p.PeekLines(ctx, args[0], lines)
	if err != nil {
		return err
	}
	_, err = io.WriteString(out, text)
	return err
}

func opListRunning(ctx context.Context, p *provider.Provider, args []string, _ io.Reader, out io.Writer) error {
	prefix := ""
	if len(args) > 0 {
		prefix = args[0]
	}
	names, err := p.ListRunning(ctx, prefix)
	if err != nil {
		return err
	}
	for _, n := range names {
		fmt.Fprintln(out, n)
	}
	return nil
}

func opGetLastActivity(ctx context.Context, p *provider.Provider, args []string, _ io.Reader, out io.Writer) error {
	act, err := p.GetLastActivity(ctx, args[0])
	if err != nil {
		return err
	}
	if !act.At.IsZero() {
		fmt.Fprintln(out, act.At.UTC().Format(time.RFC3339))
	}
	return nil
}

func opIsAttached(ctx context.Context, p *provider.Provider, args []string, _ io.Reader, out io.Writer) error {
	attached, err := p.IsAttached(ctx, args[0])
	if err != nil {
		return err
	}
	printBool(out, attached)
	return nil
}

func opSetMeta(ctx context.Context, p *provider.Provider, args []string, stdin io.Reader, _ io.Writer) error {
	value, err := io.ReadAll(stdin)
	if err != nil {
		return fmt.Errorf("read value: %w", err)
	}
	return p.SetMeta(ctx, args[0], args[1], string(value))
}

func opGetMeta(ctx context.Context, p *provider.Provider, args []string, _ io.Reader, out io.Writer) error {
	value, err := p.GetMeta(ctx, args[0], args[1])
	if err != nil {
		return err
	}
	_, err = io.WriteString(out, value)
	return err
}

func opRemoveMeta(ctx context.Context, p *provider.Provider, args []string, _ io.Reader, _ io.Writer) error {
	return p.RemoveMeta(ctx, args[0], args[1])
}

// commandArgv turns the start config's command string into an argv. A plain
// command is split with shell quoting rules; anything a plain split cannot
// represent faithfully (unbalanced quotes, operators, expansions, globs, env
// assignments) runs under /bin/sh -lc instead. An empty command yields nil:
// the pane starts its default shell.
func commandArgv(command string) []string {
	if strings.TrimSpace(command) == "" {
		return nil
	}
	argv, ok := splitCommand(command)
	if !ok || len(argv) == 0 || strings.Contains(argv[0], "=") {
		return []string{"/bin/sh", "-lc", command}
	}
	return argv
}

// splitCommand splits s into words using POSIX shell quoting: single quotes
// are literal, double quotes allow backslash escapes of $ ` " \ and newline,
// and an unquoted backslash escapes the next byte. It reports false when s
// does not parse (unterminated quote, trailing backslash) or needs a shell
// (an unquoted operator, glob or comment, or an expansion outside single
// quotes).
func splitCommand(s string) ([]string, bool) {
	var (
		words  []string
		cur    strings.Builder
		inWord bool
	)
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == ' ' || c == '\t':
			if inWord {
				words = append(words, cur.String())
				cur.Reset()
				inWord = false
			}
		case c == '\\':
			if i+1 >= len(s) {
				return nil, false
			}
			i++
			if s[i] != '\n' { // backslash-newline is a line continuation
				cur.WriteByte(s[i])
			}
			inWord = true
		case c == '\'':
			end := strings.IndexByte(s[i+1:], '\'')
			if end < 0 {
				return nil, false
			}
			cur.WriteString(s[i+1 : i+1+end])
			i += end + 1
			inWord = true
		case c == '"':
			i++
			for ; i < len(s) && s[i] != '"'; i++ {
				switch s[i] {
				case '$', '`':
					return nil, false
				case '\\':
					if i+1 < len(s) && strings.IndexByte("$`\"\\\n", s[i+1]) >= 0 {
						i++
						if s[i] != '\n' {
							cur.WriteByte(s[i])
						}
						continue
					}
				}
				cur.WriteByte(s[i])
			}
			if i >= len(s) {
				return nil, false
			}
			inWord = true
		case strings.IndexByte("|&;<>()$`*?[]{}~#!\n", c) >= 0:
			return nil, false
		default:
			cur.WriteByte(c)
			inWord = true
		}
	}
	if inWord {
		words = append(words, cur.String())
	}
	return words, true
}
