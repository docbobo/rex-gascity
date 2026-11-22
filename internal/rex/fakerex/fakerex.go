// Package fakerex builds a fake `rex` executable and gives a test a fresh
// state dir to drive it. It is the process-level fake: the real cli.Client
// runs unmodified against this binary, so argv, wire shapes, paging and child
// processes are exercised rather than canned replies.
//
// The binary is built once per test process from internal/rex/fakerex/
// fakebinary and is never part of the shipped pack. New installs the state dir
// as FAKEREX_STATE for the test process, which every child inherits:
//
//	f := fakerex.New(t)
//	c := &cli.Client{Bin: f.Bin}
package fakerex

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/docbobo/rex-gascity/internal/rex/fakerex/fakestate"
)

// Fake is one fake rex server: the binary and its private state dir.
type Fake struct {
	Bin string
	Dir string
}

var build struct {
	once sync.Once
	bin  string
	err  error
}

// binary builds the fake rex once per test process. The build runs in the
// fakebinary package dir, so it works from any caller's working directory.
func binary(t *testing.T) string {
	t.Helper()
	build.once.Do(func() {
		_, file, _, ok := runtime.Caller(0)
		if !ok {
			build.err = fmt.Errorf("fakerex: cannot locate the package source dir")
			return
		}
		dir, err := os.MkdirTemp("", "fakerex-bin-")
		if err != nil {
			build.err = err
			return
		}
		build.bin = filepath.Join(dir, "rex")
		cmd := exec.Command("go", "build", "-o", build.bin, ".")
		cmd.Dir = filepath.Join(filepath.Dir(file), "fakebinary")
		if out, err := cmd.CombinedOutput(); err != nil {
			build.err = fmt.Errorf("fakerex: go build: %w\n%s", err, out)
		}
	})
	if build.err != nil {
		t.Fatal(build.err)
	}
	return build.bin
}

// New returns a fake rex with a fresh state dir and points FAKEREX_STATE at it
// for this test. Its cleanup kills every pane the test left running.
func New(t *testing.T) *Fake {
	t.Helper()
	f := &Fake{Bin: binary(t), Dir: t.TempDir()}
	t.Setenv("FAKEREX_STATE", f.Dir)
	t.Cleanup(f.KillAll)
	return f
}

// Exec runs the fake binary with args and returns stdout, stderr and the exit
// code. It is for raw surface checks; tests that exercise the adapter use
// cli.Client{Bin: f.Bin}.
func (f *Fake) Exec(t *testing.T, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var out, errOut strings.Builder
	cmd := exec.Command(f.Bin, args...)
	cmd.Stdout, cmd.Stderr = &out, &errOut
	err := cmd.Run()
	if err == nil {
		return out.String(), errOut.String(), 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return out.String(), errOut.String(), exitErr.ExitCode()
	}
	t.Fatalf("exec fake rex %v: %v", args, err)
	return "", "", -1
}

// Fault kinds a test can queue with Fault.
const (
	FaultError       = fakestate.FaultError
	FaultUnreachable = fakestate.FaultUnreachable
	FaultLostReply   = fakestate.FaultLostReply
	FaultDelay       = fakestate.FaultDelay
	FaultGarbled     = fakestate.FaultGarbled
)

// Fault queues one scripted fault for the next call to method. Method keys are
// an api method ("session.create"), a block sub-method
// ("block.method.format"), or "send-key".
func (f *Fake) Fault(t *testing.T, method, kind string) {
	f.FaultN(t, method, kind, 1)
}

// FaultN is Fault for the next count calls.
func (f *Fake) FaultN(t *testing.T, method, kind string, count int) {
	t.Helper()
	if err := fakestate.AddFaults(f.Dir, fakestate.Fault{Method: method, Kind: kind, Count: count}); err != nil {
		t.Fatalf("queue fault %s/%s: %v", method, kind, err)
	}
}

// Update applies fn to the state under the fake's file lock. Tests use it to
// seed things the CLI cannot create, such as attached clients or program
// status records.
func (f *Fake) Update(t *testing.T, fn func(*fakestate.State)) {
	t.Helper()
	if err := fakestate.Update(f.Dir, func(st *fakestate.State) error {
		fn(st)
		return nil
	}); err != nil {
		t.Fatalf("update fake state: %v", err)
	}
}

// State reads the current state.
func (f *Fake) State(t *testing.T) *fakestate.State {
	t.Helper()
	st, err := fakestate.Load(f.Dir)
	if err != nil {
		t.Fatalf("load fake state: %v", err)
	}
	return st
}

// WaitExit waits until the block's program has exited.
func (f *Fake) WaitExit(t *testing.T, blockID string) {
	t.Helper()
	path := fakestate.ExitPath(f.Dir, blockID)
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("block %s did not exit within 10s", blockID)
}

// KillAll terminates every pane process the fake launched. It is registered as
// a test cleanup; tests never need to call it.
func (f *Fake) KillAll() {
	st, err := fakestate.Load(f.Dir)
	if err != nil {
		return
	}
	for _, s := range st.Sessions {
		for _, w := range s.Windows {
			for _, b := range w.Blocks {
				if b.PID > 0 {
					_ = syscall.Kill(-b.PID, syscall.SIGKILL)
				}
			}
		}
	}
}
