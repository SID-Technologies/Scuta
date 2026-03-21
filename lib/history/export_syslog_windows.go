//go:build windows

package history

import "github.com/sid-technologies/scuta/lib/errors"

// ExportToSyslog is not supported on Windows.
func ExportToSyslog(entries []Entry, tag string) error {
	return errors.New("syslog export is not supported on Windows")
}
