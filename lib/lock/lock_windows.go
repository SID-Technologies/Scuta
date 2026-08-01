//go:build windows

package lock

import "os"

// processAlive reports whether a process with the given PID exists.
//
// Signal(0) probing is not supported on Windows (it always errors, which
// previously made every live lock look stale — disabling locking entirely on
// Windows). os.FindProcess is the right probe here: on Windows it calls
// OpenProcess, which fails for nonexistent PIDs.
func processAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	_ = proc.Release()

	return true
}
