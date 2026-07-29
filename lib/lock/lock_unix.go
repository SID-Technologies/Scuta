//go:build !windows

package lock

import (
	"os"
	"syscall"
)

// processAlive reports whether a process with the given PID exists, using the
// conventional signal-0 probe.
func processAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	return proc.Signal(syscall.Signal(0)) == nil
}
