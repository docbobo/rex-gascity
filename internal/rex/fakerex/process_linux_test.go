//go:build linux

package fakerex_test

import (
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"testing"
	"time"
)

// processState returns the state letter of /proc/<pid>/stat, or 0.
func processState(pid int) byte {
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return 0
	}
	return parseProcState(b)
}

// processGone reports whether pid has stopped. A zombie still answers
// signal 0, so treat state Z as gone: its parent has not reaped it yet.
func processGone(pid int) bool {
	if syscall.Kill(pid, 0) != nil {
		return true
	}
	return processState(pid) == 'Z'
}

// TestProcessGoneTreatsZombieAsGone proves an unreaped child counts as gone.
// A container whose PID 1 does not reap leaves one behind.
func TestProcessGoneTreatsZombieAsGone(t *testing.T) {
	cmd := exec.Command("/bin/sh", "-c", "exit 0")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer cmd.Wait()
	pid := cmd.Process.Pid

	deadline := time.Now().Add(5 * time.Second)
	for processState(pid) != 'Z' {
		if time.Now().After(deadline) {
			t.Fatalf("pid %d did not become a zombie", pid)
		}
		time.Sleep(5 * time.Millisecond)
	}
	if err := syscall.Kill(pid, 0); err != nil {
		t.Fatalf("kill(%d, 0) = %v, want a zombie that still answers signal 0", pid, err)
	}
	if !processGone(pid) {
		t.Fatalf("processGone(%d) = false, want true for a zombie", pid)
	}
}
