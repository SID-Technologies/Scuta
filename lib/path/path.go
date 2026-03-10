// Package path provides directory discovery for Scuta's local state.
package path

import (
	"os"
	"path/filepath"

	"github.com/sid-technologies/scuta/lib/errors"
)

const scutaDir = ".scuta"

// ScutaDir returns the path to ~/.scuta/.
func ScutaDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", errors.Wrap(err, "getting home directory")
	}

	return filepath.Join(home, scutaDir), nil
}

// BinDir returns the path to ~/.scuta/bin/.
func BinDir() (string, error) {
	dir, err := ScutaDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(dir, "bin"), nil
}

// EnsureDir creates the scuta directory structure if it doesn't exist.
func EnsureDir() (string, error) {
	dir, err := ScutaDir()
	if err != nil {
		return "", err
	}

	binDir := filepath.Join(dir, "bin")
	if err := os.MkdirAll(binDir, 0o700); err != nil {
		return "", errors.Wrap(err, "creating scuta directories")
	}

	return dir, nil
}
