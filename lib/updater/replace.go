package updater

import (
	"os"

	"github.com/sid-technologies/scuta/lib/errors"
)

// ReplaceBinary replaces the currently running executable with a new binary.
// It writes to a temporary .new file first, then atomically renames over the
// original to avoid partial-write corruption.
func ReplaceBinary(newBinaryPath string) error {
	currentExe, err := os.Executable()
	if err != nil {
		return errors.Wrap(err, "resolving current executable")
	}

	tmpPath := currentExe + ".new"

	data, err := os.ReadFile(newBinaryPath)
	if err != nil {
		return errors.Wrap(err, "reading new binary")
	}

	if err := os.WriteFile(tmpPath, data, 0o755); err != nil {
		return errors.Wrap(err, "writing temporary binary")
	}

	if err := os.Rename(tmpPath, currentExe); err != nil {
		os.Remove(tmpPath)
		return errors.Wrap(err, "replacing binary")
	}

	return nil
}
