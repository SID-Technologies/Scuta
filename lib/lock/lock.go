// Package lock provides install locking to prevent concurrent Scuta operations.
// Lock files are stored at ~/.scuta/install.lock and contain metadata about
// the lock holder for debugging and stale lock detection.
package lock

import (
	"encoding/json"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/sid-technologies/scuta/lib/errors"
)

const (
	lockFile = "install.lock"

	// staleDuration is the maximum age of a lock before it's considered stale.
	staleDuration = 1 * time.Hour
)

// Info contains metadata about who holds the install lock.
type Info struct {
	PID       int       `json:"pid"`
	Hostname  string    `json:"hostname"`
	Timestamp time.Time `json:"timestamp"`
	Tools     []string  `json:"tools"`
	Command   string    `json:"command"`
}

// FilePath returns the lock file path under the given scuta directory.
func FilePath(scutaDir string) string {
	return filepath.Join(scutaDir, lockFile)
}

// Acquire attempts to create an install lock. If force is true, any existing
// lock is removed first. Returns an error with lock holder details if another
// active install is running.
func Acquire(scutaDir, command string, toolNames []string, force bool) error {
	return acquireWithRetry(scutaDir, command, toolNames, force, 1)
}

func acquireWithRetry(scutaDir, command string, toolNames []string, force bool, retries int) error {
	fp := FilePath(scutaDir)

	if force {
		_ = os.Remove(fp)
	}

	if err := os.MkdirAll(scutaDir, 0o700); err != nil {
		return errors.Wrap(err, "creating scuta directory")
	}

	f, err := os.OpenFile(fp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		if !os.IsExist(err) {
			return errors.Wrap(err, "creating lock file")
		}

		existing, readErr := readLock(fp)
		if readErr != nil {
			_ = os.Remove(fp)
			if retries > 0 {
				return acquireWithRetry(scutaDir, command, toolNames, false, retries-1)
			}
			return errors.New("could not acquire lock: another process may have raced")
		}

		if isStale(existing) {
			_ = os.Remove(fp)
			if retries > 0 {
				return acquireWithRetry(scutaDir, command, toolNames, false, retries-1)
			}
			return errors.New("could not acquire lock: another process won the race after stale lock cleanup")
		}

		return errors.New(
			"install locked by PID %d on %s (started %s, command: %s, tools: %v). Use --force to override",
			existing.PID, existing.Hostname,
			existing.Timestamp.Format(time.RFC3339),
			existing.Command, existing.Tools,
		)
	}
	defer f.Close()

	hostname, _ := os.Hostname()
	info := Info{
		PID:       os.Getpid(),
		Hostname:  hostname,
		Timestamp: time.Now(),
		Tools:     toolNames,
		Command:   command,
	}

	data, err := json.Marshal(info)
	if err != nil {
		_ = os.Remove(fp)
		return errors.Wrap(err, "marshaling lock info")
	}

	if _, err := f.Write(data); err != nil {
		_ = os.Remove(fp)
		return errors.Wrap(err, "writing lock file")
	}

	return nil
}

// Release removes the install lock. It is safe to call even if no lock exists.
func Release(scutaDir string) {
	_ = os.Remove(FilePath(scutaDir))
}

func readLock(fp string) (Info, error) {
	data, err := os.ReadFile(fp)
	if err != nil {
		return Info{}, err
	}
	var info Info
	if err := json.Unmarshal(data, &info); err != nil {
		return Info{}, err
	}
	return info, nil
}

func isStale(info Info) bool {
	if time.Since(info.Timestamp) > staleDuration {
		return true
	}

	currentHostname, _ := os.Hostname()
	if info.Hostname != currentHostname {
		return false
	}

	if info.PID > 0 {
		proc, err := os.FindProcess(info.PID)
		if err != nil {
			return true
		}
		err = proc.Signal(syscall.Signal(0))
		if err != nil {
			return true
		}
	}

	return false
}
