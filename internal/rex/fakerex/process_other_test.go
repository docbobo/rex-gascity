//go:build !linux

package fakerex_test

import "syscall"

// processGone reports whether pid has stopped. Non-Linux platforms have no
// /proc state to read, so a successful signal 0 means alive.
func processGone(pid int) bool {
	return syscall.Kill(pid, 0) != nil
}
