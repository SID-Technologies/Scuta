package history

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/sid-technologies/scuta/lib/errors"
)

// ExportFormat defines the output format for export.
type ExportFormat string

const (
	// FormatJSON exports as a JSON array.
	FormatJSON ExportFormat = "json"
	// FormatJSONL exports as newline-delimited JSON (one entry per line).
	FormatJSONL ExportFormat = "jsonl"
)

// ExportToWriter writes history entries to the given writer in the specified format.
func ExportToWriter(w io.Writer, entries []Entry, format ExportFormat) error {
	switch format {
	case FormatJSON:
		data, err := json.MarshalIndent(entries, "", "  ")
		if err != nil {
			return errors.Wrap(err, "marshaling entries to JSON")
		}
		if _, err := w.Write(data); err != nil {
			return errors.Wrap(err, "writing JSON")
		}
		if _, err := w.Write([]byte("\n")); err != nil {
			return errors.Wrap(err, "writing newline")
		}
	case FormatJSONL:
		for _, entry := range entries {
			data, err := json.Marshal(entry)
			if err != nil {
				return errors.Wrap(err, "marshaling entry")
			}
			if _, err := w.Write(data); err != nil {
				return errors.Wrap(err, "writing JSONL line")
			}
			if _, err := w.Write([]byte("\n")); err != nil {
				return errors.Wrap(err, "writing newline")
			}
		}
	default:
		return errors.New("unknown export format: %q", format)
	}
	return nil
}

// ExportToStdout exports history entries to stdout.
func ExportToStdout(entries []Entry, format ExportFormat) error {
	return ExportToWriter(os.Stdout, entries, format)
}

// ExportToFile exports history entries to the given file path.
func ExportToFile(entries []Entry, filePath string, format ExportFormat) error {
	f, err := os.Create(filePath)
	if err != nil {
		return errors.Wrap(err, "creating export file")
	}
	defer f.Close()

	return ExportToWriter(f, entries, format)
}

// ExportToWebhook POSTs history entries as JSON to the given URL.
// Retries up to 3 times on transient failures.
func ExportToWebhook(entries []Entry, webhookURL string) error {
	data, err := json.Marshal(entries)
	if err != nil {
		return errors.Wrap(err, "marshaling entries for webhook")
	}

	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * time.Second)
		}

		resp, err := http.Post(webhookURL, "application/json", bytes.NewReader(data)) //nolint:gosec,noctx // webhook URL is user-configured
		if err != nil {
			lastErr = errors.Wrap(err, "POST to webhook")
			continue
		}
		resp.Body.Close()

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return nil
		}

		if resp.StatusCode >= 500 {
			lastErr = errors.New("webhook returned %d (attempt %d/3)", resp.StatusCode, attempt+1)
			continue
		}

		// Client error — don't retry
		return errors.New("webhook returned %d", resp.StatusCode)
	}

	return lastErr
}
