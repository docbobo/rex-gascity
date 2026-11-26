package provider

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

// Runner is the seam for the provider's external commands: the probe and
// diagnostic shell-outs (nudge.go, drain_diag.go) and the pre_start and
// session_setup hooks. Run runs a command and returns its stdout. RunHook runs
// one shell hook and owns the hook's combined output and process group.
// Production uses ExecRunner; tests inject a FakeRunner with WithRunner instead
// of installing PATH shims.
//
// For Run, dir is the working directory ("" keeps the caller's) and env holds
// extra KEY=VALUE entries added to the inherited environment. A non-zero exit
// or a start failure is returned as an error. For RunHook, call carries the
// command, its working directory, its full environment, and the effective
// deadline; a non-zero exit, a timeout, or a cancellation is returned as an
// error.
type Runner interface {
	Run(ctx context.Context, dir string, env []string, name string, args ...string) (string, error)
	RunHook(ctx context.Context, call HookCall) error
}

// HookCall is one RunHook invocation. Deadline is the effective deadline of
// ctx; it is informational for an implementation and lets tests assert the
// budget without starting a shell.
type HookCall struct {
	Command  string
	Dir      string
	Env      []string
	Deadline time.Time
}

// ExecRunner is the real Runner. It runs commands through os/exec.
type ExecRunner struct{}

// Run implements Runner.
func (ExecRunner) Run(ctx context.Context, dir string, env []string, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	// A child that outlives the kill must not hold stdout open past the
	// deadline.
	cmd.WaitDelay = 100 * time.Millisecond
	out, err := cmd.Output()
	return string(out), err
}

// RunHook implements Runner. It runs the hook's command with /bin/sh -c and
// honours ctx cancellation.
func (ExecRunner) RunHook(ctx context.Context, call HookCall) error {
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", call.Command)
	cmd.Dir = call.Dir
	cmd.Env = call.Env
	// Run the hook in its own process group so a cancel kills the whole
	// group: killing only the shell leaves a child (e.g. sleep) holding the
	// output pipe open.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }

	var out lockedBuffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("%s: %w", call.Command, err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	var err error
	select {
	case err = <-done:
	case <-ctx.Done():
		// A live ctx keeps the wait open, so a hook that backgrounds its work
		// and exits still runs until that work closes the pipe. Only a
		// cancelled hook is bounded, so a child that escaped the process group
		// cannot hold the start past the hook deadline.
		select {
		case err = <-done:
		case <-time.After(preStartCancelGrace):
			// cmd.Cancel stops running once cmd.Wait has seen the shell exit,
			// so a child that escaped the process group would survive it and
			// keep the output pipe open. Kill the group again; ESRCH only
			// means it is already gone.
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			err = ctx.Err()
		}
	}
	if err != nil {
		// A cancelled or timed-out ctx means the command did not fail on its
		// own; surface that so the caller can name the timeout or cancellation.
		if ctxErr := ctx.Err(); ctxErr != nil {
			err = ctxErr
		}
		if got := out.String(); got != "" {
			return fmt.Errorf("%s: %w\n%s", call.Command, err, got)
		}
		return fmt.Errorf("%s: %w", call.Command, err)
	}
	return nil
}

// preStartCancelGrace bounds the pipe wait after a hook's context ends. The
// group kill normally closes the pipes at once; the grace covers a child that
// left the process group.
const preStartCancelGrace = 200 * time.Millisecond

// lockedBuffer collects a command's output while a copy goroutine may still
// write into it after RunHook stops waiting.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// RunCall is one recorded FakeRunner invocation.
type RunCall struct {
	Dir  string
	Env  []string
	Name string
	Args []string
}

// FakeRunner is a Runner for tests. It records every call and answers through
// Handle (Run) and HookHandle (RunHook); with a nil handler every call
// succeeds.
type FakeRunner struct {
	// Handle answers one Run call. It receives the call's context so it can
	// honor deadlines.
	Handle func(ctx context.Context, c RunCall) (string, error)
	// HookHandle answers one RunHook call. It receives the call's context so
	// it can honor deadlines.
	HookHandle func(ctx context.Context, c HookCall) error

	mu        sync.Mutex
	calls     []RunCall
	hookCalls []HookCall
}

// Run implements Runner.
func (f *FakeRunner) Run(ctx context.Context, dir string, env []string, name string, args ...string) (string, error) {
	c := RunCall{Dir: dir, Env: append([]string(nil), env...), Name: name, Args: append([]string(nil), args...)}
	f.mu.Lock()
	f.calls = append(f.calls, c)
	f.mu.Unlock()
	if f.Handle == nil {
		return "", nil
	}
	return f.Handle(ctx, c)
}

// RunHook implements Runner.
func (f *FakeRunner) RunHook(ctx context.Context, call HookCall) error {
	c := HookCall{
		Command: call.Command,
		Dir:     call.Dir,
		Env:     append([]string(nil), call.Env...),
	}
	if d, ok := ctx.Deadline(); ok {
		c.Deadline = d
	}
	f.mu.Lock()
	f.hookCalls = append(f.hookCalls, c)
	f.mu.Unlock()
	if f.HookHandle == nil {
		return nil
	}
	return f.HookHandle(ctx, c)
}

// Calls returns the recorded Run calls in order.
func (f *FakeRunner) Calls() []RunCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]RunCall(nil), f.calls...)
}

// HookCalls returns the recorded RunHook calls in order.
func (f *FakeRunner) HookCalls() []HookCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]HookCall(nil), f.hookCalls...)
}

// WithRunner sets the Runner the provider shells out through. A nil runner
// keeps the default (ExecRunner).
func WithRunner(r Runner) Option {
	return func(p *Provider) {
		if r != nil {
			p.runner = r
		}
	}
}
