// Package installer handles downloading, verifying, and installing tool binaries.
package installer

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"strings"

	"github.com/sid-technologies/scuta/lib/errors"
)

// VerifyChecksum computes the SHA256 of the file at filePath and compares it
// case-insensitively to expectedSHA256. Returns nil on match, error on mismatch.
func VerifyChecksum(filePath string, expectedSHA256 string) error {
	f, err := os.Open(filePath)
	if err != nil {
		return errors.Wrap(err, "opening file for checksum")
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return errors.Wrap(err, "computing SHA256")
	}

	actual := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(actual, expectedSHA256) {
		return errors.New(
			"checksum mismatch for %s: expected %s, got %s",
			filePath, strings.ToLower(expectedSHA256), actual,
		)
	}

	return nil
}

// ParseChecksumFile parses standard sha256sum output (lines of "<sha256>  <filename>")
// and returns a map of filename to lowercase hex SHA256.
func ParseChecksumFile(data []byte) map[string]string {
	result := make(map[string]string)
	scanner := bufio.NewScanner(bytes.NewReader(data))

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Format: "<sha256>  <filename>" or "<sha256> <filename>"
		parts := strings.SplitN(line, " ", 2)
		if len(parts) != 2 {
			continue
		}

		hash := strings.TrimSpace(parts[0])
		filename := strings.TrimSpace(parts[1])

		// sha256sum uses two spaces; trim any leading spaces/asterisk from filename
		filename = strings.TrimLeft(filename, " *")

		if hash == "" || filename == "" {
			continue
		}

		result[filename] = strings.ToLower(hash)
	}

	return result
}
