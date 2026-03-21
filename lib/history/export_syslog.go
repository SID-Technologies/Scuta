//go:build !windows

package history

import (
	"encoding/json"
	"log/syslog"

	"github.com/sid-technologies/scuta/lib/errors"
)

// ExportToSyslog writes history entries to the local syslog daemon.
func ExportToSyslog(entries []Entry, tag string) error {
	if tag == "" {
		tag = "scuta"
	}

	w, err := syslog.New(syslog.LOG_INFO|syslog.LOG_USER, tag)
	if err != nil {
		return errors.Wrap(err, "connecting to syslog")
	}
	defer w.Close()

	for _, entry := range entries {
		data, err := json.Marshal(entry)
		if err != nil {
			return errors.Wrap(err, "marshaling entry for syslog")
		}
		if err := w.Info(string(data)); err != nil {
			return errors.Wrap(err, "writing to syslog")
		}
	}

	return nil
}
