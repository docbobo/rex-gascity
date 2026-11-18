package provider

import (
	"context"
	"os"
	"os/exec"
	"sync"
	"time"
)

// Runner runs an external command and returns its stdout. It is the seam for
// the provider's probe and diagnostic shell-outs: the `gc` claim probe
// (nudge.go) and the `git` calls of the drain diagnostic (drain_diag.go).
// pre_start hooks do not use it: runPreStart runs /bin/sh directly because it
// needs the hook's combined output (provider.go). Production uses ExecRunner;
// tests inject a FakeRunner with WithRunner instead of installing PATH shims.
//
// dir is the working directory ("" keeps the caller's). env holds extra
// KEY=VALUE entries added to the inherited environment. A non-zero exit or a
// start failure is returned as an error.
type Runner interface {
	Run(ctx context.Context, dir string, env []string, name string, args ...string) (string, error)
}

// ExecRunner is the real Runner. It runs name through os/exec.
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

// RunCall is one recorded FakeRunner invocation.
type RunCall struct {
	Dir  string
	Env  []string
	Name string
	Args []string
}

// FakeRunner is a Runner for tests. It records every call and answers through
// Handle; with a nil Handle every call succeeds with empty output.
type FakeRunner struct {
	// Handle answers one call. It receives the call's context so it can honor
	// deadlines.
	Handle func(ctx context.Context, c RunCall) (string, error)

	mu    sync.Mutex
	calls []RunCall
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

// Calls returns the recorded calls in order.
func (f *FakeRunner) Calls() []RunCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]RunCall(nil), f.calls...)
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
