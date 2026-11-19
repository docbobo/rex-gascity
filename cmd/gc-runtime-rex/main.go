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
	"os/signal"
	"runtime/debug"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/docbobo/rex-gascity/internal/launch"
	"github.com/docbobo/rex-gascity/internal/provider"
	"github.com/docbobo/rex-gascity/internal/rex"
	"github.com/docbobo/rex-gascity/internal/rex/cli"
)

const (
	exitOK      = 0
	exitFail    = 1
	exitUnknown = 2
	// exitInputOutcome is RPP's typed input-outcome code (herdr-core's exit
	// 80): the op prints one {"outcome":...,"detail":...} line on stdout.
	// gc 1.5.1 has no input-outcome capability and reads it as a plain error.
	exitInputOutcome = 80
)

// handshake is the protocol op's reply. It declares only what is implemented:
// no proc.exec, proc.provision, proc.stream or tty.attach, and no locality.
// idle-boundary gates the `idle` snapshot op (GCREX-63). gc 1.5.1 has no such
// token and ignores it; a gc that consumes it reads the point-in-time reading.
const handshake = `{"version":0,"capabilities":["report-attachment","report-activity","idle-boundary"]}`

// buildVersion is the version the `version` op reports. A pack build may set
// it with -ldflags "-X main.buildVersion=<version>"; a bare `go install`
// leaves it empty and the module version embedded in the binary is used
// instead. The pack's Makefile sets it to the version pack.toml declares.
var buildVersion string

// versionReport is the `version` op's reply: the build identity the
// rex:rex-binary doctor check compares against the pack's declared version.
// Version is empty when the build carries no module version (a local build
// reports the Go placeholder "(devel)", which is not a version).
type versionReport struct {
	Module   string `json:"module"`
	Version  string `json:"version"`
	Revision string `json:"revision,omitempty"`
	Modified bool   `json:"modified,omitempty"`
}

// currentBuildInfo returns the running binary's build info, or nil when the
// toolchain embedded none.
func currentBuildInfo() *debug.BuildInfo {
	if bi, ok := debug.ReadBuildInfo(); ok {
		return bi
	}
	return nil
}

// versionReportFrom resolves the report from build info and an ldflags
// override. override wins; otherwise the embedded module version is used, and
// the placeholder "(devel)" reads as unknown (empty). bi may be nil (a binary
// built without module info).
func versionReportFrom(bi *debug.BuildInfo, override string) versionReport {
	var rep versionReport
	if bi != nil {
		rep.Module = bi.Main.Path
		rep.Version = bi.Main.Version
		for _, s := range bi.Settings {
			switch s.Key {
			case "vcs.revision":
				rep.Revision = s.Value
			case "vcs.modified":
				rep.Modified = s.Value == "true"
			}
		}
	}
	if v := strings.TrimSpace(override); v != "" {
		rep.Version = v
	}
	if rep.Version == "(devel)" {
		rep.Version = ""
	}
	return rep
}

func main() {
	// gc cancels a start by sending SIGTERM to the adapter's process group and
	// force-kills the group after 2 s (gc internal/runtime/exec/exec.go;
	// execgrace). Catching the signal here cancels the op's context, so a start
	// in flight aborts its Rex or pre_start call and the provider's rollback
	// defer runs before gc's forced kill, instead of depending on gc's
	// follow-up stop.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	code := runWithContext(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr, liveDeps())
	stop()
	os.Exit(code)
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

// startConfig is the start op's stdin JSON. Unknown fields are ignored. The
// field set mirrors gc v1.5.1's inner startConfig
// (internal/runtime/exec/json.go:26-42): every field gc sends except
// process_names, which needs a Rex process-appeared signal this provider does
// not have, plus fresh_only, which the pack sends and gc v1.5.1 never does.
// work_dir_exclude is pack-only as well.
type startConfig struct {
	WorkDir            string              `json:"work_dir"`
	Command            string              `json:"command"`
	Env                map[string]string   `json:"env"`
	PreStart           []string            `json:"pre_start"`
	Nudge              string              `json:"nudge"`
	Lifecycle          string              `json:"lifecycle"`
	PackOverlayDirs    []string            `json:"pack_overlay_dirs"`
	SessionSetup       []string            `json:"session_setup"`
	SessionSetupScript string              `json:"session_setup_script"`
	SessionLive        []string            `json:"session_live"`
	OverlayDir         string              `json:"overlay_dir"`
	CopyFiles          []provider.CopyFile `json:"copy_files"`
	FreshOnly          bool                `json:"fresh_only"`
	ReadyPromptPrefix  string              `json:"ready_prompt_prefix"`
	ReadyDelayMS       int                 `json:"ready_delay_ms"`
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
	"watch-startup": true,
}

// keyedOps take a <key> argument that must not be empty. herdr-core rejects an
// empty metadata key with exit 1; matching it keeps the meta ops conformant.
var keyedOps = map[string]bool{
	"set-meta":    true,
	"get-meta":    true,
	"remove-meta": true,
}

var ops = map[string]opSpec{
	"start":             {1, "start <name>", opStart},
	"stop":              {1, "stop <name>", opStop},
	"attach":            {1, "attach <name>", opAttach},
	"is-running":        {1, "is-running <name>", opIsRunning},
	"interrupt":         {1, "interrupt <name>", opInterrupt},
	"nudge":             {1, "nudge <name>", opNudge},
	"send-keys":         {1, "send-keys <name> [keys...]", opSendKeys},
	"clear-scrollback":  {1, "clear-scrollback <name>", opClearScrollback},
	"process-alive":     {1, "process-alive <name>", opProcessAlive},
	"peek":              {1, "peek <name> [lines]", opPeek},
	"list-running":      {0, "list-running [prefix]", opListRunning},
	"get-last-activity": {1, "get-last-activity <name>", opGetLastActivity},
	"idle":              {1, "idle <name>", opIdle},
	"program-status":    {1, "program-status <name>", opProgramStatus},
	"is-attached":       {1, "is-attached <name>", opIsAttached},
	"set-meta":          {2, "set-meta <name> <key>", opSetMeta},
	"get-meta":          {2, "get-meta <name> <key>", opGetMeta},
	"remove-meta":       {2, "remove-meta <name> <key>", opRemoveMeta},
}

// runWithContext dispatches one RPP op and returns the process exit code.
// Failures write nothing to stdout; the op's output is buffered and flushed
// only on success. ctx is the op's parent: its cancellation (a SIGTERM caught
// by main) aborts the in-flight provider call and lets the start rollback run.
func runWithContext(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer, d deps) int {
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
	// version reports the build identity for the pack's doctor check. It is
	// local and needs no city, state dir or Rex server, so it is answered
	// before the provider is built.
	if op == "version" {
		line, err := json.Marshal(versionReportFrom(currentBuildInfo(), buildVersion))
		if err != nil {
			fmt.Fprintf(stderr, "gc-runtime-rex: version: encode report: %v\n", err)
			return exitFail
		}
		if _, err := fmt.Fprintln(stdout, string(line)); err != nil {
			fmt.Fprintf(stderr, "gc-runtime-rex: version: write output: %v\n", err)
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
	if keyedOps[op] && rest[1] == "" {
		fmt.Fprintf(stderr, "gc-runtime-rex: %s: metadata key must not be empty\n", op)
		return exitFail
	}

	// The city and its namespace are derived once: see NamespaceForCaller.
	city, ns, err := provider.NamespaceForCaller()
	if err != nil {
		fmt.Fprintf(stderr, "gc-runtime-rex: %s: state dir: %v\n", op, err)
		return exitFail
	}
	prov := provider.New(city, d.newClient(), provider.WithNamespace(ns), provider.WithWarnings(stderr))

	var out bytes.Buffer
	if err := spec.handle(ctx, prov, rest, stdin, &out); err != nil {
		var rejected *provider.KeyRejectedError
		if errors.As(err, &rejected) {
			return writeInputOutcome(stdout, stderr, "rejected", rejected.Error())
		}
		var outcome *provider.InputOutcomeError
		if errors.As(err, &outcome) {
			return writeInputOutcome(stdout, stderr, outcome.Outcome, outcome.Detail)
		}
		var coded *exitError
		if errors.As(err, &coded) {
			fmt.Fprintf(stderr, "gc-runtime-rex: %s: %v\n", op, coded.err)
			return coded.code
		}
		fmt.Fprintf(stderr, "gc-runtime-rex: %s: %v\n", op, err)
		return exitFail
	}
	if _, err := stdout.Write(out.Bytes()); err != nil {
		fmt.Fprintf(stderr, "gc-runtime-rex: %s: write output: %v\n", op, err)
		return exitFail
	}
	return exitOK
}

// exitError carries a non-default RPP exit code for an op failure. It is how a
// handler asks for a code the dispatcher does not assign itself, such as
// herdr-core's exit 73 for a fresh_only name collision.
type exitError struct {
	code int
	err  error
}

func (e *exitError) Error() string { return e.err.Error() }
func (e *exitError) Unwrap() error { return e.err }

// exitFreshCollision is herdr-core's FRESH_COLLISION code (rpp/exitcodes.py:24
// at pack 0f07496): `fresh_only` refused a name any record holds.
const exitFreshCollision = 73

// writeInputOutcome answers an RPP input-outcome: one
// {"outcome":...,"detail":...} line on stdout and "<outcome>: <detail>" on
// stderr, with exit 80. gc 1.5.1 reads exit 80 as a plain error, but a caller
// that declares input-outcome reads the word.
func writeInputOutcome(stdout, stderr io.Writer, outcome, detail string) int {
	line, err := json.Marshal(struct {
		Outcome string `json:"outcome"`
		Detail  string `json:"detail"`
	}{outcome, detail})
	if err != nil {
		fmt.Fprintf(stderr, "gc-runtime-rex: encode %s outcome: %v\n", outcome, err)
		return exitFail
	}
	fmt.Fprintln(stdout, string(line))
	if detail != "" {
		fmt.Fprintf(stderr, "%s: %s\n", outcome, detail)
	} else {
		fmt.Fprintln(stderr, outcome)
	}
	return exitInputOutcome
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
	if len(bytes.TrimSpace(raw)) == 0 {
		return errors.New("parse start config: empty stdin")
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return fmt.Errorf("parse config: %w", err)
	}
	err = p.StartNew(ctx, name, provider.Config{
		Argv:               launch.CommandArgv(cfg.Command),
		WorkDir:            cfg.WorkDir,
		Env:                cfg.Env,
		PreStart:           cfg.PreStart,
		Nudge:              cfg.Nudge,
		Lifecycle:          cfg.Lifecycle,
		PackOverlayDirs:    cfg.PackOverlayDirs,
		SessionSetup:       cfg.SessionSetup,
		SessionSetupScript: cfg.SessionSetupScript,
		SessionLive:        cfg.SessionLive,
		OverlayDir:         cfg.OverlayDir,
		CopyFiles:          cfg.CopyFiles,
		FreshOnly:          cfg.FreshOnly,
		ReadyPromptPrefix:  cfg.ReadyPromptPrefix,
		ReadyDelayMS:       cfg.ReadyDelayMS,
	})
	if errors.Is(err, provider.ErrFreshCollision) {
		return &exitError{code: exitFreshCollision, err: fmt.Errorf("session %q already exists (fresh_only)", name)}
	}
	if errors.Is(err, provider.ErrAlreadyRunning) {
		// gc's exec proxy recognizes "already running" as a start collision.
		return fmt.Errorf("session %q already running", name)
	}
	return err
}

func opStop(ctx context.Context, p *provider.Provider, args []string, _ io.Reader, _ io.Writer) error {
	return p.Stop(ctx, args[0])
}

// opAttach answers gc's attach call with a hint instead of a pty passthrough.
// gc runs attach through runWithTTY, which does not apply the exit-2-is-success
// rule, so a Rex-backed session cannot answer exit 2. Rex has no in-terminal
// attach and the provider declares no tty.attach, so the op always exits 0 and
// points the user at the Rex app. It never contacts Rex, so an unknown session
// or an unreachable server cannot turn into a failed `gc session attach`.
func opAttach(_ context.Context, _ *provider.Provider, args []string, _ io.Reader, out io.Writer) error {
	_, err := io.WriteString(out, attachHint(args[0]))
	return err
}

// attachHint is the one-line guidance attach prints for session name.
func attachHint(name string) string {
	return fmt.Sprintf("Rex has no in-terminal attach; open the Rex app and select the session %q to attach\n", name)
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
	return p.Interrupt(ctx, args[0])
}

func opNudge(ctx context.Context, p *provider.Provider, args []string, stdin io.Reader, out io.Writer) error {
	text, err := io.ReadAll(stdin)
	if err != nil {
		return fmt.Errorf("read message: %w", err)
	}
	if !utf8.Valid(text) {
		// json.Marshal would rewrite the bytes to U+FFFD and type corrupted
		// text. Reject before touching Rex so nothing reaches the pane. The
		// detail matches herdr-core's failed nudge wording.
		return &provider.InputOutcomeError{
			Outcome: "rejected",
			Detail:  "the nudge text is not UTF-8; nothing was typed",
		}
	}
	report, err := p.NudgeReport(ctx, args[0], string(text), provider.NudgeModeWait)
	if err != nil {
		// A missing session stays best effort; a busy, blocked or failed-send
		// outcome is an *InputOutcomeError and maps to exit 80.
		return bestEffort(err)
	}
	if report.Delivered && !report.Confirmed {
		return writeDeliveredUnobserved(out, args[0])
	}
	return nil
}

// writeDeliveredUnobserved answers a nudge that reached the pane but whose
// turn start was not observed: exit 0 and one
// {"outcome":"delivered_unobserved","detail":...} line, matching the pack's
// success envelope. gc discards nudge stdout, so the line is informational for
// other callers; the unconfirmed diagnostic file is still written by the
// provider.
func writeDeliveredUnobserved(out io.Writer, name string) error {
	line, err := json.Marshal(struct {
		Outcome string `json:"outcome"`
		Detail  string `json:"detail"`
	}{Outcome: "delivered_unobserved", Detail: fmt.Sprintf("nudge %s: delivered but no turn start was observed", name)})
	if err != nil {
		return fmt.Errorf("encode delivered_unobserved outcome: %w", err)
	}
	_, err = fmt.Fprintln(out, string(line))
	return err
}

func opSendKeys(ctx context.Context, p *provider.Provider, args []string, _ io.Reader, _ io.Writer) error {
	keys := args[1:]
	if len(keys) == 0 {
		return nil
	}
	return bestEffort(p.SendKeys(ctx, args[0], keys...))
}

// opClearScrollback returns Provider.ClearScrollback directly (GCREX-57): a
// missing or dead pane is an *provider.InputOutcomeError answered as exit 80
// `gone`, not swallowed as a silent success.
func opClearScrollback(ctx context.Context, p *provider.Provider, args []string, _ io.Reader, _ io.Writer) error {
	return p.ClearScrollback(ctx, args[0])
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
	state, err := p.ProcessAlive(ctx, args[0], names)
	if err != nil {
		return err
	}
	switch state {
	case provider.LivenessAlive:
		printBool(out, true)
	case provider.LivenessDead:
		printBool(out, false)
	}
	// LivenessUnknown prints nothing: exit 0 with empty stdout, which gc's
	// reader treats as alive (gc v1.5.1 exec/exec.go:709-718).
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

// opIdle answers gc's point-in-time idle snapshot (runtime.IdleSnapshotProvider):
// one word, idle, busy or indeterminate, and exit 0 for every state because the
// state is the payload. A failed read is an error and exits 1, never idle.
func opIdle(ctx context.Context, p *provider.Provider, args []string, _ io.Reader, out io.Writer) error {
	state, err := p.IdleSnapshot(ctx, args[0])
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, state)
	return err
}

// opProgramStatus answers gc's point-in-time judged program-status reading
// (runtime.ProgramStatusProvider): one JSON line, exit 0 for every reading
// because the reading is the payload. A failed read is an error and exits 1,
// never a reading.
func opProgramStatus(ctx context.Context, p *provider.Provider, args []string, _ io.Reader, out io.Writer) error {
	reading, err := p.ProgramStatus(ctx, args[0])
	if err != nil {
		return err
	}
	line, err := json.Marshal(reading)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, string(line))
	return err
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
